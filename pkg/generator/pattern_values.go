package generator

import (
	"fmt"
	"strings"

	"github.com/extremenetworks/testcase-generator/pkg/model"
)

// Pattern-based value generation for parameters with pattern constraints

// getPatternBasedValue generates a value based on the pattern constraint
func (g *Generator) getPatternBasedValue(param model.Parameter, variation int) (interface{}, bool) {
	// Check if parameter has a pattern constraint
	var patternConstraint *model.Constraint
	for i := range param.Constraints {
		if param.Constraints[i].Type == model.ConstraintTypePattern {
			patternConstraint = &param.Constraints[i]
			break
		}
	}

	if patternConstraint == nil {
		return nil, false
	}

	pattern, ok := patternConstraint.Value.(string)
	if !ok {
		return nil, false
	}

	// Detect pattern type and generate appropriate value
	if isIPAddressPattern(pattern) {
		return getIPAddressValue(variation), true
	}

	if isFQDNPattern(pattern) {
		return getFQDNValue(variation), true
	}

	if isMACAddressPattern(pattern) {
		return getMACAddressValue(variation), true
	}

	if isUUIDPattern(pattern) {
		return getUUIDValue(variation), true
	}

	// Add more pattern types as needed

	return nil, false
}

// isIPAddressPattern checks if the pattern matches IPv4 or IPv6 addresses
func isIPAddressPattern(pattern string) bool {
	// Check for common IPv4/IPv6 pattern indicators
	indicators := []string{
		"25[0-5]", "2[0-4][0-9]", "1[0-9]{2}", "1[0-9]", "[1-9]?[0-9]", // IPv4 octets
		"[0-9].[0-9]",       // Dot notation (may or may not be escaped)
		"\\.",               // Escaped dot
		"[0-9A-Fa-f]{1,4}:", // IPv6 hex groups
		"::",                // IPv6 compression
		"[0-9A-Fa-f]",       // Hex digits (common in both IPv4 and IPv6)
	}

	matchCount := 0
	for _, indicator := range indicators {
		if strings.Contains(pattern, indicator) {
			matchCount++
		}
	}

	// If we match  at the 2+ IPv4/IPv6 indicators, it's likely an IP address pattern
	return matchCount >= 2
}

// isFQDNPattern checks if pattern matches Fully Qualified Domain Names
func isFQDNPattern(pattern string) bool {
	indicators := []string{
		"[A-Za-z0-9]", "[A-Za-z]", // Letters for domain names
		"\\.",     // Dots between labels
		"-{0,61}", // Hyphens in labels
	}

	// Must have at least 2 indicators to be considered FQDN pattern
	count := 0
	for _, indicator := range indicators {
		if strings.Contains(pattern, indicator) {
			count++
		}
	}

	return count >= 2 && !isIPAddressPattern(pattern)
}

// isMACAddressPattern checks if pattern matches MAC addresses
func isMACAddressPattern(pattern string) bool {
	return strings.Contains(pattern, "[0-9A-Fa-f]{2}") &&
		(strings.Contains(pattern, ":") || strings.Contains(pattern, "-"))
}

// isUUIDPattern checks if pattern matches UUIDs
func isUUIDPattern(pattern string) bool {
	return strings.Contains(pattern, "[0-9a-f]{8}-[0-9a-f]{4}")
}

// getIPAddressValue returns a variety of valid IP addresses (IPv4, IPv6, or FQDN)
func getIPAddressValue(variation int) string {
	// Mix of IPv4, IPv6, and FQDN values
	values := []string{
		// IPv4 addresses
		"8.8.8.8",         // Google DNS
		"1.1.1.1",         // Cloudflare DNS
		"192.168.1.1",     // Private network
		"10.0.0.1",        // Private network
		"172.16.0.1",      // Private network
		"208.67.222.222",  // OpenDNS
		"127.0.0.1",       // Loopback
		"255.255.255.255", // Broadcast
		"0.0.0.0",         // Any address
		"100.64.0.1",      // Shared address space
		// IPv6 addresses
		"2001:4860:4860::8888", // Google DNS IPv6
		"2606:4700:4700::1111", // Cloudflare DNS IPv6
		"::1",                  // IPv6 loopback
		"::",                   // IPv6 any address
		"fe80::1",              // Link-local
		// FQDNs
		"dns.google.com",
		"dhcp.example.org",
		"server.local",
		"ns1.example.net",
		"primary.domain.com",
	}

	if variation >= 0 && variation < len(values) {
		return values[variation]
	}

	// For variations beyond our list, generate IPv4
	return generateIPv4(variation)
}

// generateIPv4 generates a valid IPv4 address based on variation
func generateIPv4(variation int) string {
	// Generate varied IPv4 addresses
	octet1 := (variation % 254) + 1
	octet2 := ((variation / 254) % 254) + 1
	octet3 := ((variation / (254 * 254)) % 254) + 1
	octet4 := ((variation / (254 * 254 * 254)) % 254) + 1

	return fmt.Sprintf("%d.%d.%d.%d", octet1, octet2, octet3, octet4)
}

// getFQDNValue returns varied FQDN values
func getFQDNValue(variation int) string {
	values := []string{
		"server.example.com",
		"primary.domain.org",
		"host.network.net",
		"service.company.io",
		"node.cluster.local",
		"api.service.example",
		"db.backend.internal",
		"web.frontend.local",
		"mail.example.com",
		"dns.server.org",
	}

	if variation >= 0 && variation < len(values) {
		return values[variation]
	}

	return fmt.Sprintf("host%d.example.com", variation)
}

// getMACAddressValue returns varied MAC addresses
func getMACAddressValue(variation int) string {
	values := []string{
		"00:11:22:33:44:55",
		"AA:BB:CC:DD:EE:FF",
		"00:1A:2B:3C:4D:5E",
		"08:00:27:12:34:56",
		"52:54:00:AB:CD:EF",
	}

	if variation >= 0 && variation < len(values) {
		return values[variation]
	}

	// Generate MAC based on variation
	b1 := (variation % 256)
	b2 := ((variation / 256) % 256)
	return fmt.Sprintf("00:11:22:%02X:%02X:%02X", b1, b2, (b1+b2)%256)
}

// getUUIDValue returns varied UUID values
func getUUIDValue(variation int) string {
	values := []string{
		"550e8400-e29b-41d4-a716-446655440000",
		"6ba7b810-9dad-11d1-80b4-00c04fd430c8",
		"d8e8fca2-dc0f-11d1-a7e3-00a0c91e6bf6",
		"123e4567-e89b-12d3-a456-426614174000",
		"a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11",
	}

	if variation >= 0 && variation < len(values) {
		return values[variation]
	}

	return fmt.Sprintf("00000000-0000-4000-8000-%012d", variation)
}

// getPatternBoundaryValue generates min/max boundary values that respect pattern
func (g *Generator) getPatternBoundaryValue(param model.Parameter, constraint model.Constraint, isMin bool) (interface{}, bool) {
	// Check if parameter has a pattern constraint
	var patternConstraint *model.Constraint
	for i := range param.Constraints {
		if param.Constraints[i].Type == model.ConstraintTypePattern {
			patternConstraint = &param.Constraints[i]
			break
		}
	}

	if patternConstraint == nil {
		return nil, false
	}

	pattern, ok := patternConstraint.Value.(string)
	if !ok {
		return nil, false
	}

	// Generate boundary value based on pattern type
	if isIPAddressPattern(pattern) {
		if isMin {
			// Minimum valid values for different types
			return "0.0.0.0", true // Minimum IPv4
		} else {
			// Maximum valid values
			return "255.255.255.255", true // Maximum IPv4
		}
	}

	if isFQDNPattern(pattern) {
		if isMin {
			return "a.b", true // Minimum FQDN (1 char labels)
		} else {
			// Generate max length FQDN within constraint
			if constraint.Type == model.ConstraintTypeMaxLength {
				if maxLen, ok := constraint.Value.(int); ok && maxLen > 10 {
					// Generate FQDN near max length
					labelCount := maxLen / 20
					fqdn := ""
					for i := 0; i < labelCount; i++ {
						if i > 0 {
							fqdn += "."
						}
						fqdn += g.generateStringOfLength(15)
					}
					fqdn += ".com"
					if len(fqdn) > maxLen {
						fqdn = fqdn[:maxLen-4] + ".com"
					}
					return fqdn, true
				}
			}
			return "very-long-subdomain-name.example-domain.com", true
		}
	}

	if isMACAddressPattern(pattern) {
		if isMin {
			return "00:00:00:00:00:00", true
		} else {
			return "FF:FF:FF:FF:FF:FF", true
		}
	}

	return nil, false
}

// getPatternForDescription returns a human-readable description of the pattern type
func getPatternForDescription(param model.Parameter) string {
	for _, constraint := range param.Constraints {
		if constraint.Type == model.ConstraintTypePattern {
			if pattern, ok := constraint.Value.(string); ok {
				if isIPAddressPattern(pattern) {
					return "IP address (IPv4/IPv6/FQDN)"
				}
				if isMACAddressPattern(pattern) {
					return "MAC address"
				}
				if isUUIDPattern(pattern) {
					return "UUID"
				}
				if isFQDNPattern(pattern) {
					return "FQDN"
				}
			}
		}
	}
	return ""
}
