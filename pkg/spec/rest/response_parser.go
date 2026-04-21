package rest

import (
	"fmt"
	"strings"

	"github.com/extremenetworks/testcase-generator/pkg/model"
	"github.com/getkin/kin-openapi/openapi3"
)

// ResponseSchema represents the expected response structure for an endpoint
type ResponseSchema struct {
	StatusCode    int
	SchemaRef     string // e.g., "#/components/schemas/ConfigObject"
	ContentType   string
	Properties    []ResponseProperty
	IsArray       bool
	ArrayItemType string
}

// ResponseProperty represents a property in a response
type ResponseProperty struct {
	Name        string
	Type        string // string, number, boolean, object, array
	Required    bool
	Description string
	Enum        []string
	NestedProps []ResponseProperty // For nested objects
}

// GetResponseSchema extracts response schema for a specific operation and status code
func (p *Parser) GetResponseSchema(operationID string, statusCode int) (*ResponseSchema, error) {
	// Find the operation in paths
	for _, pathItem := range p.doc.Paths {
		for _, operation := range pathItem.Operations() {
			if operation != nil && operation.OperationID == operationID {
				return p.extractResponseSchema(operation, statusCode)
			}
		}
	}

	return nil, fmt.Errorf("operation %s not found", operationID)
}

// extractResponseSchema parses the response schema from operation
func (p *Parser) extractResponseSchema(operation *openapi3.Operation, statusCode int) (*ResponseSchema, error) {
	statusCodeStr := fmt.Sprintf("%d", statusCode)
	
	response, exists := operation.Responses[statusCodeStr]
	if !exists {
		// Try default response
		response, exists = operation.Responses["default"]
		if !exists {
			return nil, fmt.Errorf("no response defined for status code %d", statusCode)
		}
	}

	rs := &ResponseSchema{
		StatusCode:  statusCode,
		ContentType: "application/json",
	}

	// Extract schema from content
	if response.Value != nil && response.Value.Content != nil {
		if jsonContent, hasJSON := response.Value.Content["application/json"]; hasJSON {
			if jsonContent.Schema != nil && jsonContent.Schema.Value != nil {
				schema := jsonContent.Schema.Value
				
				if schema.Items != nil && schema.Items.Value != nil {
					rs.SchemaRef = schema.Items.Ref
				} else {
					rs.SchemaRef = jsonContent.Schema.Ref
				}

				// Check if it's an array
				if schema.Type == "array" {
					rs.IsArray = true
					if schema.Items != nil {
						rs.ArrayItemType = schema.Items.Ref
					}
				}

				// Parse properties from schema
				if jsonContent.Schema.Ref != "" {
					props, err := p.parseSchemaProperties(jsonContent.Schema.Ref)
					if err == nil {
						rs.Properties = props
					}
				} else if schema.Type == "object" && schema.Properties != nil {
					rs.Properties = p.parseInlineProperties(schema.Properties)
				}
			}
		}
	}

	return rs, nil
}

// parseSchemaProperties extracts properties from a schema reference
func (p *Parser) parseSchemaProperties(schemaRef string) ([]ResponseProperty, error) {
	// Extract schema name from reference (#/components/schemas/SchemaName)
	parts := strings.Split(schemaRef, "/")
	if len(parts) < 4 {
		return nil, fmt.Errorf("invalid schema reference: %s", schemaRef)
	}
	schemaName := parts[len(parts)-1]

	// Look up schema in components
	if p.doc.Components == nil || p.doc.Components.Schemas == nil {
		return nil, fmt.Errorf("no components/schemas found")
	}

	schemaRef2, exists := p.doc.Components.Schemas[schemaName]
	if !exists || schemaRef2 == nil || schemaRef2.Value == nil {
		return nil, fmt.Errorf("schema %s not found in components", schemaName)
	}

	return p.parseInlineProperties(schemaRef2.Value.Properties), nil
}

// parseInlineProperties parses properties from inline schema
func (p *Parser) parseInlineProperties(props map[string]*openapi3.SchemaRef) []ResponseProperty {
	var result []ResponseProperty

	for propName, propRef := range props {
		if propRef == nil || propRef.Value == nil {
			continue
		}
		
		prop := propRef.Value
		rp := ResponseProperty{
			Name:        propName,
			Type:        prop.Type,
			Description: prop.Description,
		}

		// Handle enum
		if len(prop.Enum) > 0 {
			for _, e := range prop.Enum {
				if str, ok := e.(string); ok {
					rp.Enum = append(rp.Enum, str)
				}
			}
		}

		// Handle nested objects
		if prop.Type == "object" && len(prop.Properties) > 0 {
			rp.NestedProps = p.parseInlineProperties(prop.Properties)
		}

		// Handle array items
		if prop.Type == "array" && prop.Items != nil && prop.Items.Value != nil {
			if prop.Items.Ref != "" {
				// Reference to another schema
				rp.Type = "array"
				rp.Description = fmt.Sprintf("Array of %s", prop.Items.Ref)
			} else if prop.Items.Value.Type != "" {
				rp.Type = fmt.Sprintf("array[%s]", prop.Items.Value.Type)
			}
		}

		// Handle schema references
		if propRef.Ref != "" {
			nestedProps, err := p.parseSchemaProperties(propRef.Ref)
			if err == nil {
				rp.NestedProps = nestedProps
			}
		}

		result = append(result, rp)
	}

	return result
}

// GenerateResponseValidations creates validation steps based on feature parameters and response schema
func GenerateResponseValidations(feature *model.Feature, responseSchema *ResponseSchema) []model.Validation {
	var validations []model.Validation

	// Always validate status code
	validations = append(validations, model.Validation{
		Type:        model.ValidationTypeStatusCode,
		Expected:    responseSchema.StatusCode,
		Description: fmt.Sprintf("Verify HTTP status code is %d", responseSchema.StatusCode),
	})

	// For create/update operations (201), validate response contains ID
	if responseSchema.StatusCode == 201 {
		validations = append(validations, model.Validation{
			Type:        model.ValidationTypeJSONPathExists,
			Path:        "$[0].id",
			Description: "Verify response contains object ID",
		})

		validations = append(validations, model.Validation{
			Type:        model.ValidationTypeJSONPathEquals,
			Path:        "$[0].status",
			Expected:    "success",
			Description: "Verify operation status is success",
		})
	}

	// For read operations (200), validate response body contains expected properties
	if responseSchema.StatusCode == 200 {
		// Generate validations for each required property from feature
		if feature.Parameters != nil {
			for _, param := range feature.Parameters {
				if param.Required {
					jsonPath := fmt.Sprintf("$.objects[0].properties[?(@.name=='%s')]", param.Name)
					validations = append(validations, model.Validation{
						Type:        model.ValidationTypeJSONPathExists,
						Path:        jsonPath,
						Description: fmt.Sprintf("Verify %s property exists in response", param.Name),
					})
				}
			}
		}

		// Validate nested properties for complex objects
		if feature.Parameters != nil {
			for _, param := range feature.Parameters {
				if len(param.NestedProperties) > 0 {
					// Validate nested structure exists
					jsonPath := fmt.Sprintf("$.objects[0].properties[?(@.name=='%s')].value", param.Name)
					validations = append(validations, model.Validation{
						Type:        model.ValidationTypeJSONPathExists,
						Path:        jsonPath,
						Description: fmt.Sprintf("Verify %s nested object exists", param.Name),
					})
				}
			}
		}
	}

	// For batch operations, validate array structure
	if responseSchema.IsArray {
		validations = append(validations, model.Validation{
			Type:        model.ValidationTypeJSONPathExists,
			Path:        "$",
			Expected:    "array",
			Description: "Verify response is an array",
		})
	}

	return validations
}

// GetOperationIDForPath finds the operation ID for a given path and HTTP method
func (p *Parser) GetOperationIDForPath(path, method string) string {
	pathItem, exists := p.doc.Paths[path]
	if !exists || pathItem == nil {
		return ""
	}

	operation := pathItem.GetOperation(method)
	if operation != nil {
		return operation.OperationID
	}

	return ""
}
