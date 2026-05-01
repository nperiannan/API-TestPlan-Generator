package generator

import (
	"fmt"
	"strings"

	"github.com/extremenetworks/testcase-generator/pkg/model"
	restparser "github.com/extremenetworks/testcase-generator/pkg/spec/rest"
)

// systemManagedFields are fields from base:primary / base / tenant groupings
// that are internally managed by the system and should NOT be validated in
// functional test cases. Users never send these in payloads.
var systemManagedFields = map[string]bool{
	"id":          true,
	"created-at":  true,
	"updated-at":  true,
	"deleted-at":  true,
	"customer-id": true,
	"owner-id":    true,
}

// isSystemManagedField returns true if the parameter is a system-managed
// field from base:primary that should be excluded from test validations.
func isSystemManagedField(paramName string) bool {
	return systemManagedFields[strings.ToLower(paramName)]
}

// generateResponseValidations creates comprehensive response validations based on:
// 1. Expected HTTP status code from OpenAPI spec
// 2. Response schema properties from OpenAPI spec
// 3. YANG model parameters that should appear in response
func (g *Generator) generateResponseValidations(
	feature *model.Feature,
	path *model.FeaturePath,
	statusCode int,
) []model.Validation {
	return g.generateResponseValidationsWithExpectedValues(feature, path, statusCode, nil)
}

func (g *Generator) generateResponseValidationsWithExpectedValues(
	feature *model.Feature,
	path *model.FeaturePath,
	statusCode int,
	expectedValues map[string]interface{},
) []model.Validation {
	var validations []model.Validation

	// Always validate status code first
	validations = append(validations, model.Validation{
		Type:        model.ValidationTypeStatusCode,
		Expected:    statusCode,
		Description: fmt.Sprintf("Verify HTTP status code is %d", statusCode),
	})

	// If we don't have a REST parser, return basic validation
	if g.restParser == nil {
		return validations
	}

	// Get operation ID for this path
	operationID := g.restParser.GetOperationIDForPath(path.Path, path.HTTPMethod)
	if operationID == "" {
		return validations
	}

	// Get response schema from OpenAPI spec
	responseSchema, err := g.restParser.GetResponseSchema(operationID, statusCode)
	if err != nil || responseSchema == nil {
		return validations
	}

	// Add response-type-specific validations
	switch statusCode {
	case 201: // Created
		validations = append(validations, g.generateCreateResponseValidations(feature, responseSchema)...)
	case 200: // OK
		if path.OperationType == model.OperationTypeRead {
			validations = append(validations, g.generateReadResponseValidationsWithValues(feature, responseSchema, expectedValues)...)
		} else if path.OperationType == model.OperationTypeUpdate {
			validations = append(validations, g.generateUpdateResponseValidations(feature, responseSchema)...)
		} else if path.OperationType == model.OperationTypeList {
			validations = append(validations, g.generateListResponseValidations(feature, responseSchema)...)
		}
	case 204: // No Content
		validations = append(validations, model.Validation{
			Type:        model.ValidationTypeStatusCode,
			Expected:    204,
			Description: "Verify successful deletion with no content",
		})
	}

	return validations
}

// generateCreateResponseValidations generates validations for CREATE (201) responses
func (g *Generator) generateCreateResponseValidations(
	feature *model.Feature,
	responseSchema *restparser.ResponseSchema,
) []model.Validation {
	var validations []model.Validation

	// Batch response format: [{ id, status, operation }]
	validations = append(validations, model.Validation{
		Type:        model.ValidationTypeJSONPathExists,
		Path:        "$[0].id",
		Description: "Verify response contains created object ID",
	})

	validations = append(validations, model.Validation{
		Type:        model.ValidationTypeJSONPathEquals,
		Path:        "$[0].status",
		Expected:    "success",
		Description: "Verify operation status is 'success'",
	})

	validations = append(validations, model.Validation{
		Type:        model.ValidationTypeJSONPathEquals,
		Path:        "$[0].operation",
		Expected:    "create",
		Description: "Verify operation type is 'create'",
	})

	return validations
}

// generateReadResponseValidations generates validations for READ (200) responses.
// It validates both the structure of the response AND the actual values of each
// YANG-defined parameter that was set during the create step.
func (g *Generator) generateReadResponseValidations(
	feature *model.Feature,
	responseSchema *restparser.ResponseSchema,
) []model.Validation {
	return g.generateReadResponseValidationsWithValues(feature, responseSchema, nil)
}

// generateReadResponseValidationsWithValues generates validations for READ (200) responses.
// When expectedValues is provided, only those sent/proven values are asserted.
// Without expectedValues, it validates structure plus required/key field presence only.
func (g *Generator) generateReadResponseValidationsWithValues(
	feature *model.Feature,
	responseSchema *restparser.ResponseSchema,
	expectedValues map[string]interface{},
) []model.Validation {
	var validations []model.Validation

	// Response format: { objects: [{ id, type, properties: [...] }], pagination, featurePath, objectType }
	validations = append(validations, model.Validation{
		Type:        model.ValidationTypeJSONPathExists,
		Path:        "$.objects",
		Description: "Verify response contains objects array",
	})

	validations = append(validations, model.Validation{
		Type:        model.ValidationTypeJSONPathExists,
		Path:        "$.featurePath",
		Description: "Verify response contains feature path",
	})

	validations = append(validations, model.Validation{
		Type:        model.ValidationTypeJSONPathExists,
		Path:        "$.objectType",
		Description: "Verify response contains object type",
	})

	keySet := keySetForFeature(feature)

	// For each YANG parameter: validate only fields that are required/key or explicitly expected.
	// Skip system-managed fields (id, created-at, updated-at, deleted-at, customer-id, owner-id)
	if feature.Parameters != nil {
		for _, param := range feature.Parameters {
			// Skip system-managed fields from base:primary grouping
			if isSystemManagedField(param.Name) {
				continue
			}

			_, hasExpectedValue := expectedValues[param.Name]
			shouldExist := param.Required || keySet[param.Name] || hasExpectedValue
			if !shouldExist {
				continue
			}

			existsPath := fmt.Sprintf("$.objects[0].properties[?(@.name=='%s')]", param.Name)
			validations = append(validations, model.Validation{
				Type:        model.ValidationTypeJSONPathExists,
				Path:        existsPath,
				Description: fmt.Sprintf("Verify property '%s' exists in response", param.Name),
			})

			if expectedValues != nil && hasExpectedValue {
				// Validate the actual value stored in the property
				valuePath := fmt.Sprintf("$.objects[0].properties[?(@.name=='%s')].value", param.Name)
				validations = append(validations, model.Validation{
					Type:        model.ValidationTypeJSONPathEquals,
					Path:        valuePath,
					Expected:    expectedValues[param.Name],
					Description: fmt.Sprintf("Verify '%s' value is '%v' as set during create", param.Name, expectedValues[param.Name]),
				})
			}
		}
	}

	return validations
}

// generateUpdateResponseValidations generates validations for UPDATE (200) responses
func (g *Generator) generateUpdateResponseValidations(
	feature *model.Feature,
	responseSchema *restparser.ResponseSchema,
) []model.Validation {
	var validations []model.Validation

	// Update response is similar to create: [{ id, status, operation }]
	validations = append(validations, model.Validation{
		Type:        model.ValidationTypeJSONPathExists,
		Path:        "$[0].id",
		Description: "Verify response contains updated object ID",
	})

	validations = append(validations, model.Validation{
		Type:        model.ValidationTypeJSONPathEquals,
		Path:        "$[0].status",
		Expected:    "success",
		Description: "Verify operation status is 'success'",
	})

	validations = append(validations, model.Validation{
		Type:        model.ValidationTypeJSONPathEquals,
		Path:        "$[0].operation",
		Expected:    "update",
		Description: "Verify operation type is 'update'",
	})

	return validations
}

// generateListResponseValidations generates validations for LIST (200) responses
func (g *Generator) generateListResponseValidations(
	feature *model.Feature,
	responseSchema *restparser.ResponseSchema,
) []model.Validation {
	var validations []model.Validation

	// List response format: { objects: [], pagination: {...}, featurePath, objectType }
	validations = append(validations, model.Validation{
		Type:        model.ValidationTypeJSONPathExists,
		Path:        "$.objects",
		Description: "Verify response contains objects array",
	})

	validations = append(validations, model.Validation{
		Type:        model.ValidationTypeJSONPathExists,
		Path:        "$.pagination",
		Description: "Verify response contains pagination metadata",
	})

	validations = append(validations, model.Validation{
		Type:        model.ValidationTypeJSONPathExists,
		Path:        "$.pagination.total_count",
		Description: "Verify pagination contains total count",
	})

	return validations
}

// generatePropertyValueValidations creates validations for specific property values
func (g *Generator) generatePropertyValueValidations(
	propertyName string,
	expectedValue interface{},
) model.Validation {
	jsonPath := fmt.Sprintf("$.objects[0].properties[?(@.name=='%s')].value", propertyName)
	return model.Validation{
		Type:        model.ValidationTypeJSONPathEquals,
		Path:        jsonPath,
		Expected:    expectedValue,
		Description: fmt.Sprintf("Verify '%s' property has expected value", propertyName),
	}
}
