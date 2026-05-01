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
		g.setBodyResourceName(feature, body, fmt.Sprintf("TestResource-IP-%s", ipCase.label))
		g.setOrAddBodyParameter(body, param, ipCase.value)
		// Set companion mask field to correct type for the IP version used
		setMaskForIPVersion(body, feature.Parameters, ipCase.value)

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
		g.setBodyResourceName(feature, body, fmt.Sprintf("TestResource-BadIP-%s", ipCase.label))
		g.setOrAddBodyParameter(body, param, ipCase.value)

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

	// Disqualifiers — names with these substrings are clearly not IP/host
	// fields even when they share a token like "server" with one (e.g.
	// "auth-server-secret", "accounting-server-port").
	nonIPHints := []string{"secret", "password", "passwd", "port", "-key", "key-",
		"-id", "id-", "-name", "name-", "-type", "type-", "domain-name",
		"community", "token", "username", "user-name", "shared-key", "hash",
		"timeout", "interval", "retries", "count", "duration", "encrypted",
		"enable", "enabled", "disable", "disabled", "mode"}
	for _, bad := range nonIPHints {
		if strings.Contains(lower, bad) {
			return false
		}
	}

	// Name-based heuristics: tokens that strongly imply an address-style
	// field. We match against dash-delimited tokens to avoid false hits
	// inside other words (e.g. "shared-key" containing "key").
	tokens := strings.Split(lower, "-")
	tokenSet := make(map[string]bool, len(tokens))
	for _, t := range tokens {
		tokenSet[t] = true
	}
	ipTokens := []string{"server", "address", "ip", "host", "gateway",
		"nameserver", "dns", "ntp", "syslog", "radius", "tacacs", "subnet"}
	for _, t := range ipTokens {
		if tokenSet[t] {
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

	// Description-based detection: catch fields like "destination-subnet" whose
	// YANG description explicitly states CIDR / IP address format.
	descLower := strings.ToLower(param.Description)
	ipDescHints := []string{"cidr notation", "ip subnet", "ip address format"}
	for _, hint := range ipDescHints {
		if strings.Contains(descLower, hint) {
			return true
		}
	}

	return false
}

// ── IP boundary cases ─────────────────────────────────────────────────────────
//
// For string fields that accept an IP address / hostname, generic
// minLength / maxLength boundary values are not meaningful — and the
// "edge" IPv4 values 0.0.0.0 (network) and 255.255.255.255 (broadcast)
// are not valid host addresses. The cases below cover the smallest and
// largest semantically valid IPv4 unicast, IPv6 unicast and FQDN values,
// plus FQDN length boundaries.
var ipBoundaryCases = []ipTestCase{
	{"IPv4MinValid", "1.0.0.1", "Minimum valid IPv4 unicast host (above 0.0.0.0/8 reserved range)"},
	{"IPv4MaxValid", "223.255.255.254", "Maximum valid IPv4 unicast host (last address before multicast 224/4)"},
	{"IPv6MinValid", "2001:db8::1", "Minimum valid IPv6 global unicast (RFC 3849 documentation prefix)"},
	{"IPv6MaxValid", "2001:db8:ffff:ffff:ffff:ffff:ffff:fffe", "Maximum valid IPv6 unicast within documentation prefix"},
	{"FQDNMinLength", "a.io", "Minimum-length FQDN (single-character label + TLD)"},
	{"FQDNMaxLength", buildMaxLengthFQDN(), "Maximum-length FQDN (253 characters per RFC 1035)"},
}

// buildMaxLengthFQDN returns a 253-character FQDN built from labels of
// length 63 (the per-label DNS limit) joined with dots.
func buildMaxLengthFQDN() string {
	label63 := strings.Repeat("a", 63)
	// 63 + 1 + 63 + 1 + 63 + 1 + 61 = 253
	return label63 + "." + label63 + "." + label63 + "." + strings.Repeat("b", 61)
}

// generateIPBoundaryTests produces boundary test cases for an IP-typed
// string parameter, covering valid IPv4, IPv6 and FQDN edge values.
// Used in place of generic string-length boundary tests for IP fields.
func (g *Generator) generateIPBoundaryTests(
	feature *model.Feature,
	param model.Parameter,
	createPath *model.FeaturePath,
) []model.TestCase {
	var tests []model.TestCase

	for _, ipCase := range ipBoundaryCases {
		tc := model.TestCase{
			TestCaseID:  g.nextTestID(),
			FeatureName: feature.Name,
			Priority:    model.TestPriorityP3,
			Type:        model.TestCategoryBoundary,
			Description: fmt.Sprintf("Boundary test: %s='%s' — %s",
				param.Name, ipCase.value, ipCase.description),
			IsDeploymentTest: false,
			Steps:            []model.TestStep{},
		}

		body := g.generateRequestBody(feature, createPath)
		g.setOrAddBodyParameter(body, param, ipCase.value)
		setMaskForIPVersion(body, feature.Parameters, ipCase.value)

		tc.Steps = append(tc.Steps, model.TestStep{
			Name:           fmt.Sprintf("testBoundary_%s", ipCase.label),
			Description:    fmt.Sprintf("Test %s with %s value '%s'", param.Name, ipCase.label, ipCase.value),
			Method:         createPath.HTTPMethod,
			API:            model.APITypeREST,
			Path:           createPath.Path,
			Body:           body,
			ExpectedStatus: 201,
			Validations:    g.generateResponseValidations(feature, createPath, 201),
		})

		tests = append(tests, tc)
	}

	return tests
}

// setMaskForIPVersion looks for a "mask" parameter in the feature and sets it
// to a value appropriate for the IP version of the given ipValue.
//
// Rules:
//   - IPv6 address (contains ":") -> prefix length string e.g. "64"
//   - IPv4 address or FQDN        -> prefix length string e.g. "24"
//   - If no mask parameter exists in the feature, this is a no-op.
func setMaskForIPVersion(body map[string]interface{}, params []model.Parameter, ipValue string) {
	var maskParamName string
	for _, p := range params {
		lower := strings.ToLower(p.Name)
		if lower == "mask" || lower == "subnet-mask" || lower == "network-mask" || lower == "prefix-length" {
			maskParamName = p.Name
			break
		}
	}
	if maskParamName == "" {
		return
	}

	var maskValue string
	if strings.Contains(ipValue, ":") {
		// IPv6 -- use prefix length
		maskValue = "64"
	} else {
		// IPv4 or FQDN -- use prefix length
		maskValue = "24"
	}

	// Update in deep-scanned format (objects[].properties) or simple map format
	if objects, ok := body["objects"].([]interface{}); ok && len(objects) > 0 {
		if obj, ok := objects[0].(map[string]interface{}); ok {
			if props, ok := obj["properties"].([]map[string]interface{}); ok {
				for i, prop := range props {
					if strings.EqualFold(fmt.Sprintf("%v", prop["name"]), maskParamName) {
						props[i]["value"] = maskValue
						obj["properties"] = props
						return
					}
				}
			}
		}
	} else {
		body[maskParamName] = maskValue
	}
}
