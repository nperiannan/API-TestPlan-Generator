package generator

import (
	"fmt"
	"strings"

	"github.com/extremenetworks/testcase-generator/pkg/model"
)

// generateIPClassificationTests generates tests covering different IP address categories
// for any parameter that accepts IP addresses (detected via pattern or name heuristic).
//
// Positive tests (expect 201 / 200) cover:
//   - Private Class A (10.x.x.x)
//   - Private Class B (172.16.x.x – 172.31.x.x)
//   - Private Class C (192.168.x.x)
//   - Loopback (127.0.0.1)
//   - Public IPs
//   - IPv6 global unicast, link-local, loopback
//
// Negative tests (expect 400) cover:
//   - Invalid octets (256.x.x.x)
//   - Incomplete address (192.168.1)
//   - Plain text strings (not-an-ip)
//   - IPv6 multicast used as unicast server (ff00::1)
func (g *Generator) generateIPClassificationTests(feature *model.Feature, paths []*model.FeaturePath) []model.TestCase {
	var tests []model.TestCase

	var createPath *model.FeaturePath
	for _, path := range paths {
		if path.OperationType == model.OperationTypeCreate {
			createPath = path
			break
		}
	}
	if createPath == nil {
		return tests
	}

	// Collect all IP-type parameters (top-level and nested)
	ipParams := g.collectIPParameters(feature.Parameters)
	for _, param := range ipParams {
		tests = append(tests, g.generateIPPositiveTests(feature, createPath, param)...)
	}

	return tests
}

// generateIPNegativeClassificationTests generates negative tests for IP parameters.
// Called separately so they land in the "negative" category.
func (g *Generator) generateIPNegativeClassificationTests(feature *model.Feature, paths []*model.FeaturePath) []model.TestCase {
	var tests []model.TestCase

	var createPath *model.FeaturePath
	for _, path := range paths {
		if path.OperationType == model.OperationTypeCreate {
			createPath = path
			break
		}
	}
	if createPath == nil {
		return tests
	}

	ipParams := g.collectIPParameters(feature.Parameters)
	for _, param := range ipParams {
		tests = append(tests, g.generateIPNegativeTests(feature, createPath, param)...)
	}

	return tests
}

// ── Positive IP tests ─────────────────────────────────────────────────────────

type ipTestCase struct {
	label       string
	value       string
	description string
}

// ipPositiveCases lists all valid IP address categories with representative values.
var ipPositiveCases = []ipTestCase{
	// Private ranges
	{"PrivateClassA", "10.0.0.1", "Private Class A address (RFC 1918 10.0.0.0/8)"},
	{"PrivateClassA2", "10.255.255.1", "Private Class A – high end of range"},
	{"PrivateClassB", "172.16.0.1", "Private Class B address (RFC 1918 172.16.0.0/12)"},
	{"PrivateClassB2", "172.31.255.1", "Private Class B – high end of range"},
	{"PrivateClassC", "192.168.0.1", "Private Class C address (RFC 1918 192.168.0.0/16)"},
	{"PrivateClassC2", "192.168.255.1", "Private Class C – high end of range"},
	// Loopback
	{"Loopback", "127.0.0.1", "IPv4 loopback address"},
	// Public IPs (Class A, B, C public ranges)
	{"PublicClassA", "8.8.8.8", "Public Class A – Google DNS primary"},
	{"PublicClassA2", "1.1.1.1", "Public Class A – Cloudflare DNS primary"},
	{"PublicClassB", "208.67.222.222", "Public Class B – OpenDNS primary"},
	{"PublicClassC", "198.51.100.1", "Public Class C – TEST-NET-2 (RFC 5737)"},
	// IPv6
	{"IPv6GlobalUnicast", "2001:4860:4860::8888", "IPv6 global unicast – Google DNS"},
	{"IPv6GlobalUnicast2", "2606:4700:4700::1111", "IPv6 global unicast – Cloudflare DNS"},
	{"IPv6Loopback", "::1", "IPv6 loopback address"},
	{"IPv6LinkLocal", "fe80::1", "IPv6 link-local address"},
	// FQDN (also accepted by most IP parameters)
	{"FQDN", "dns.google.com", "Fully qualified domain name"},
	{"FQDNSubdomain", "ns1.example.org", "FQDN with subdomain"},
}

func (g *Generator) generateIPPositiveTests(
	feature *model.Feature,
	createPath *model.FeaturePath,
	param model.Parameter,
) []model.TestCase {
	var tests []model.TestCase

	for _, ipCase := range ipPositiveCases {
		tc := model.TestCase{
			TestCaseID:  g.nextTestID(),
			FeatureName: feature.Name,
			Priority:    model.TestPriorityP3,
			Type:        model.TestCategoryFunctional,
			Description: fmt.Sprintf("Verify %s accepts %s IP for '%s': %s (%s)",
				feature.Name, ipCase.label, param.Name, ipCase.value, ipCase.description),
			IsDeploymentTest: false,
			Steps:            []model.TestStep{},
		}

		body := g.generateRequestBody(feature, createPath)
		body["name"] = fmt.Sprintf("TestResource-IP-%s", ipCase.label)
		g.setBodyParameterValue(body, param.Name, ipCase.value)

		tc.Steps = append(tc.Steps, model.TestStep{
			Name:           fmt.Sprintf("createWith%s", ipCase.label),
			Description:    fmt.Sprintf("Create %s with %s='%s' (%s)", feature.Name, param.Name, ipCase.value, ipCase.description),
			Method:         createPath.HTTPMethod,
			API:            model.APITypeREST,
			Path:           createPath.Path,
			Body:           body,
			ExpectedStatus: 201,
			Validations:    statusValidation(201),
		})

		tests = append(tests, tc)
	}

	return tests
}

// ── Negative IP tests ─────────────────────────────────────────────────────────

// ipNegativeCases lists invalid / problematic IP values that should be rejected.
var ipNegativeCases = []ipTestCase{
	{"InvalidOctet256", "256.0.0.1", "Invalid IPv4 – first octet 256 (out of range 0-255)"},
	{"InvalidOctet999", "999.999.999.999", "Invalid IPv4 – all octets out of range"},
	{"NegativeOctet", "-1.0.0.1", "Invalid IPv4 – negative octet"},
	{"IncompleteIPv4", "192.168.1", "Incomplete IPv4 – missing last octet"},
	{"OnlyTwoOctets", "10.0", "Incomplete IPv4 – only two octets"},
	{"PlainText", "not-an-ip-address", "Plain text – not a valid IP address"},
	{"EmptyString", "", "Empty string – must be rejected"},
	{"SpacesOnly", "   ", "Whitespace string – must be rejected"},
	{"IPWithPort", "8.8.8.8:53", "IP with port suffix – invalid format for IP-only field"},
	{"CIDRNotation", "192.168.1.0/24", "CIDR notation – networks are not valid server IPs"},
	{"NetworkAddress", "192.168.1.0", "Network address (host part all zeros) – semantically invalid"},
	{"BroadcastAddress", "255.255.255.255", "IPv4 broadcast – should be rejected as server address"},
	{"IPv6Multicast", "ff00::1", "IPv6 multicast – not a valid unicast server address"},
	{"IPv6Invalid", "zzzz::1", "IPv6 with invalid hex characters"},
	{"LeadingZeroOctet", "010.0.0.1", "Leading zeros in octet – ambiguous/invalid"},
}

func (g *Generator) generateIPNegativeTests(
	feature *model.Feature,
	createPath *model.FeaturePath,
	param model.Parameter,
) []model.TestCase {
	var tests []model.TestCase

	for _, ipCase := range ipNegativeCases {
		tc := model.TestCase{
			TestCaseID:  g.nextTestID(),
			FeatureName: feature.Name,
			Priority:    model.TestPriorityP2,
			Type:        model.TestCategoryNegative,
			Description: fmt.Sprintf("Verify %s rejects invalid IP '%s' for '%s': %s",
				feature.Name, ipCase.value, param.Name, ipCase.description),
			IsDeploymentTest: false,
			Steps:            []model.TestStep{},
		}

		body := g.generateRequestBody(feature, createPath)
		body["name"] = fmt.Sprintf("TestResource-BadIP-%s", ipCase.label)
		g.setBodyParameterValue(body, param.Name, ipCase.value)

		tc.Steps = append(tc.Steps, model.TestStep{
			Name:           fmt.Sprintf("attemptWith%s", ipCase.label),
			Description:    fmt.Sprintf("Attempt to create %s with invalid %s='%s'", feature.Name, param.Name, ipCase.value),
			Method:         createPath.HTTPMethod,
			API:            model.APITypeREST,
			Path:           createPath.Path,
			Body:           body,
			ExpectedStatus: 400,
			Validations:    statusValidation(400),
		})

		tests = append(tests, tc)
	}

	return tests
}

// ── Parameter detection ───────────────────────────────────────────────────────

// collectIPParameters returns all parameters (top-level and nested) that
// accept IP addresses, identified by name heuristic or pattern constraint.
func (g *Generator) collectIPParameters(params []model.Parameter) []model.Parameter {
	var result []model.Parameter
	for _, param := range params {
		if isIPParameter(param) {
			result = append(result, param)
		}
		// Recurse into nested properties
		if len(param.NestedProperties) > 0 {
			result = append(result, g.collectIPParameters(param.NestedProperties)...)
		}
	}
	return result
}

// isIPParameter returns true if the parameter accepts IP addresses.
func isIPParameter(param model.Parameter) bool {
	lower := strings.ToLower(param.Name)

	// Name-based heuristics
	ipNameHints := []string{"server", "address", "-ip", "ip-", "host", "gateway",
		"nameserver", "dns", "ntp", "syslog", "radius", "tacacs"}
	for _, hint := range ipNameHints {
		if strings.Contains(lower, hint) {
			return true
		}
	}

	// Pattern-based detection (uses existing helper from pattern_values.go)
	for _, c := range param.Constraints {
		if c.Type == model.ConstraintTypePattern {
			if pattern, ok := c.Value.(string); ok {
				if isIPAddressPattern(pattern) {
					return true
				}
			}
		}
	}

	return false
}
