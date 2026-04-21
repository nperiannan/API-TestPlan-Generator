package nosapi

import (
	"fmt"
	"strings"

	"github.com/extremenetworks/testcase-generator/pkg/model"
	"github.com/getkin/kin-openapi/openapi3"
)

// Parser parses NOSAPI specifications for device verification
type Parser struct {
	specPath  string
	doc       *openapi3.T
	endpoints map[string]*model.NOSAPIEndpoint
}

// NewParser creates a new NOSAPI parser
func NewParser(specPath string) *Parser {
	return &Parser{
		specPath:  specPath,
		endpoints: make(map[string]*model.NOSAPIEndpoint),
	}
}

// Parse loads and parses the NOSAPI specification
func (p *Parser) Parse() error {
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = true
	doc, err := loader.LoadFromFile(p.specPath)
	if err != nil {
		return fmt.Errorf("failed to load NOSAPI spec: %w", err)
	}

	// Skip validation to handle specs with invalid examples
	// We only need the structure for test generation

	p.doc = doc
	return p.extractEndpoints()
}

// extractEndpoints extracts NOSAPI endpoints from the spec
func (p *Parser) extractEndpoints() error {
	for path, pathItem := range p.doc.Paths {
		for method, operation := range pathItem.Operations() {
			endpoint := &model.NOSAPIEndpoint{
				Path:        path,
				Method:      strings.ToUpper(method),
				DeviceScope: p.detectDeviceScope(path),
			}

			if operation != nil {
				endpoint.Description = operation.Description
				if endpoint.Description == "" {
					endpoint.Description = operation.Summary
				}

				// Extract response schema
				if responses := operation.Responses; responses != nil {
					if successResp := responses["200"]; successResp != nil && successResp.Value != nil {
						content := successResp.Value.Content
						if jsonContent, ok := content["application/json"]; ok && jsonContent.Schema != nil {
							endpoint.ResponseSchema = p.schemaToMap(jsonContent.Schema.Value)
						}
					}
				}
			}

			key := fmt.Sprintf("%s %s", endpoint.Method, endpoint.Path)
			p.endpoints[key] = endpoint
		}
	}

	return nil
}

// detectDeviceScope determines the scope of the NOSAPI endpoint
func (p *Parser) detectDeviceScope(path string) string {
	pathLower := strings.ToLower(path)

	if strings.Contains(pathLower, "site-group") || strings.Contains(pathLower, "sitegroup") {
		return "siteGroup"
	}
	if strings.Contains(pathLower, "/device/") || strings.Contains(pathLower, "/devices/") {
		return "device"
	}
	if strings.Contains(pathLower, "/global") {
		return "global"
	}

	return "device" // default
}

// schemaToMap converts OpenAPI schema to a map structure
func (p *Parser) schemaToMap(schema *openapi3.Schema) map[string]interface{} {
	if schema == nil {
		return nil
	}

	result := make(map[string]interface{})

	// Handle object type
	if schema.Type == "object" {
		for propName, propSchema := range schema.Properties {
			if propSchema.Value != nil {
				propType := "string"
				if propSchema.Value.Type != "" {
					propType = propSchema.Value.Type
				}

				result[propName] = map[string]interface{}{
					"type":        propType,
					"description": propSchema.Value.Description,
				}

				// Recursively handle nested objects
				if propType == "object" && len(propSchema.Value.Properties) > 0 {
					result[propName].(map[string]interface{})["properties"] = p.schemaToMap(propSchema.Value)
				}

				// Handle arrays
				if propType == "array" && propSchema.Value.Items != nil && propSchema.Value.Items.Value != nil {
					result[propName].(map[string]interface{})["items"] = p.schemaToMap(propSchema.Value.Items.Value)
				}
			}
		}
	}

	// Handle array type at root level
	if schema.Type == "array" {
		if schema.Items != nil && schema.Items.Value != nil {
			result["type"] = "array"
			result["items"] = p.schemaToMap(schema.Items.Value)
		}
	}

	return result
}

// GetEndpoints returns all parsed NOSAPI endpoints
func (p *Parser) GetEndpoints() map[string]*model.NOSAPIEndpoint {
	return p.endpoints
}

// FindEndpointForFeature finds a NOSAPI endpoint suitable for verifying a feature
func (p *Parser) FindEndpointForFeature(featureName string, scopeType model.ScopeType) *model.NOSAPIEndpoint {
	featureLower := strings.ToLower(featureName)

	// Extract base feature name by removing common suffixes
	baseFeature := featureLower
	baseFeature = strings.TrimSuffix(baseFeature, "-server")
	baseFeature = strings.TrimSuffix(baseFeature, "-config")
	baseFeature = strings.TrimSuffix(baseFeature, "-feature")
	baseFeature = strings.TrimSuffix(baseFeature, "-profile")

	// Try exact match first
	for _, endpoint := range p.endpoints {
		pathLower := strings.ToLower(endpoint.Path)

		// Match by full feature name in path
		if strings.Contains(pathLower, featureLower) && endpoint.Method == "GET" {
			// Match scope type
			if scopeType == model.ScopeTypeSiteGroup && endpoint.DeviceScope == "siteGroup" {
				return endpoint
			}
			if scopeType == model.ScopeTypeDevice && endpoint.DeviceScope == "device" {
				return endpoint
			}
		}
	}

	// Try base feature name match (e.g., "dns" for "dns-server")
	for _, endpoint := range p.endpoints {
		pathLower := strings.ToLower(endpoint.Path)

		// Match by base feature name in path
		if strings.Contains(pathLower, "/"+baseFeature) && endpoint.Method == "GET" {
			// Match scope type
			if scopeType == model.ScopeTypeSiteGroup && endpoint.DeviceScope == "siteGroup" {
				return endpoint
			}
			if scopeType == model.ScopeTypeDevice && endpoint.DeviceScope == "device" {
				return endpoint
			}
		}
	}

	// No match found - return nil instead of a random endpoint
	return nil
}
