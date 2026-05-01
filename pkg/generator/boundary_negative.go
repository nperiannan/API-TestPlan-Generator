package generator

import (
	"fmt"
	"strings"

	"github.com/extremenetworks/testcase-generator/pkg/model"
)

// generateBoundaryTests generates boundary test cases
func (g *Generator) generateBoundaryTests(feature *model.Feature, paths []*model.FeaturePath) []model.TestCase {
	var tests []model.TestCase

	// Find create path
	var createPath *model.FeaturePath
	for _, path := range paths {
		if path.OperationType == model.OperationTypeCreate {
			createPath = path
			break
		}
	}

	// Generate boundary tests for each parameter with constraints (top-level and nested)
	allParams := collectAllParameters(feature.Parameters)

	// IP-typed parameters get a dedicated set of valid IPv4 / IPv6 / FQDN
	// boundary tests instead of generic string-length boundary tests, which
	// are misleading for address fields. We emit them only once per param
	// even if the YANG model declares both a minLength and a maxLength.
	emittedIPBoundary := map[string]bool{}

	for _, param := range allParams {
		ipParam := isStringIPParameter(param)

		for _, constraint := range param.Constraints {
			switch constraint.Type {
			case model.ConstraintTypeMinLength, model.ConstraintTypeMaxLength:
				if createPath == nil {
					continue
				}
				if ipParam {
					if !emittedIPBoundary[param.Name] {
						tests = append(tests, g.generateIPBoundaryTests(feature, param, createPath)...)
						emittedIPBoundary[param.Name] = true
					}
					continue
				}
				tests = append(tests, g.generateStringLengthBoundaryTest(feature, param, constraint, createPath))
			case model.ConstraintTypeMin, model.ConstraintTypeMax:
				if createPath != nil {
					tests = append(tests, g.generateNumericBoundaryTest(feature, param, constraint, createPath))
				}
			case model.ConstraintTypeMinItems, model.ConstraintTypeMaxItems:
				if createPath != nil {
					tests = append(tests, g.generateArrayBoundaryTest(feature, param, constraint, createPath))
				}
			}
		}
	}

	return tests
}

// isStringIPParameter returns true when the parameter is a string-typed
// field that accepts an IP address / hostname (used to choose IP-aware
// boundary value generation).
func isStringIPParameter(param model.Parameter) bool {
	if param.GoType != "" && param.GoType != "string" {
		return false
	}
	return isIPParameter(param)
}

// collectAllParameters flattens a parameter list including all nested properties.
func collectAllParameters(params []model.Parameter) []model.Parameter {
	var result []model.Parameter
	for _, p := range params {
		result = append(result, p)
		if len(p.NestedProperties) > 0 {
			result = append(result, collectAllParameters(p.NestedProperties)...)
		}
	}
	return result
}

// generateStringLengthBoundaryTest generates boundary test for string length
func (g *Generator) generateStringLengthBoundaryTest(
	feature *model.Feature,
	param model.Parameter,
	constraint model.Constraint,
	createPath *model.FeaturePath,
) model.TestCase {

	tc := model.TestCase{
		TestCaseID:  g.nextTestID(),
		FeatureName: feature.Name,
		Priority:    model.TestPriorityP3,
		Type:        model.TestCategoryBoundary,
		Description: func() string {
			if param.Description != "" {
				return fmt.Sprintf("Test %s parameter with %s constraint — %s", param.Name, constraint.Type, param.Description)
			}
			return fmt.Sprintf("Test %s parameter with %s constraint", param.Name, constraint.Type)
		}(),
		Steps: []model.TestStep{},
	}

	body := g.generateRequestBody(feature, createPath)

	// Set parameter to boundary value
	if constraint.Type == model.ConstraintTypeMaxLength {
		maxLen := constraint.Value.(int)
		g.setOrAddBodyParameter(body, param, generateString(maxLen))
	} else if constraint.Type == model.ConstraintTypeMinLength {
		minLen := constraint.Value.(int)
		g.setOrAddBodyParameter(body, param, generateString(minLen))
	}

	step := model.TestStep{
		Name:           "testBoundary",
		Description:    fmt.Sprintf("Test %s with %s", param.Name, constraint.Type),
		Method:         createPath.HTTPMethod,
		API:            model.APITypeREST,
		Path:           createPath.Path,
		Body:           body,
		ExpectedStatus: 201,
		Validations:    g.generateResponseValidations(feature, createPath, 201),
	}
	tc.Steps = append(tc.Steps, step)

	return tc
}

// generateNumericBoundaryTest generates boundary test for numeric values
func (g *Generator) generateNumericBoundaryTest(
	feature *model.Feature,
	param model.Parameter,
	constraint model.Constraint,
	createPath *model.FeaturePath,
) model.TestCase {

	tc := model.TestCase{
		TestCaseID:  g.nextTestID(),
		FeatureName: feature.Name,
		Priority:    model.TestPriorityP3,
		Type:        model.TestCategoryBoundary,
		Description: func() string {
			if param.Description != "" {
				return fmt.Sprintf("Test %s parameter with %s constraint — %s", param.Name, constraint.Type, param.Description)
			}
			return fmt.Sprintf("Test %s parameter with %s constraint", param.Name, constraint.Type)
		}(),
		Steps: []model.TestStep{},
	}

	body := g.generateRequestBody(feature, createPath)
	g.setOrAddBodyParameter(body, param, constraint.Value)

	step := model.TestStep{
		Name:           "testBoundary",
		Description:    fmt.Sprintf("Test %s with %s value", param.Name, constraint.Type),
		Method:         createPath.HTTPMethod,
		API:            model.APITypeREST,
		Path:           createPath.Path,
		Body:           body,
		ExpectedStatus: 201,
		Validations:    g.generateResponseValidations(feature, createPath, 201),
	}
	tc.Steps = append(tc.Steps, step)

	return tc
}

// generateArrayBoundaryTest generates boundary test for array constraints
func (g *Generator) generateArrayBoundaryTest(
	feature *model.Feature,
	param model.Parameter,
	constraint model.Constraint,
	createPath *model.FeaturePath,
) model.TestCase {

	tc := model.TestCase{
		TestCaseID:  g.nextTestID(),
		FeatureName: feature.Name,
		Priority:    model.TestPriorityP3,
		Type:        model.TestCategoryBoundary,
		Description: fmt.Sprintf("Test %s array with %s constraint", param.Name, constraint.Type),
		Steps:       []model.TestStep{},
	}

	body := g.generateRequestBody(feature, createPath)

	count := constraint.Value.(int)
	arr := make([]interface{}, count)
	for i := 0; i < count; i++ {
		arr[i] = fmt.Sprintf("item-%d", i)
	}
	g.setOrAddBodyParameter(body, param, arr)

	step := model.TestStep{
		Name:           "testBoundary",
		Description:    fmt.Sprintf("Test %s with %s items", param.Name, constraint.Type),
		Method:         createPath.HTTPMethod,
		API:            model.APITypeREST,
		Path:           createPath.Path,
		Body:           body,
		ExpectedStatus: 201,
		Validations: []model.Validation{
			{
				Type:     model.ValidationTypeStatusCode,
				Expected: 201,
			},
		},
	}
	tc.Steps = append(tc.Steps, step)

	return tc
}

// generateNegativeTests generates negative test cases
func (g *Generator) generateNegativeTests(feature *model.Feature, paths []*model.FeaturePath) []model.TestCase {
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

	// Test missing required fields
	for _, param := range feature.Parameters {
		if param.Required {
			tests = append(tests, g.generateMissingRequiredFieldTest(feature, param, createPath))
		}
	}

	// Test invalid enum values
	for _, param := range feature.Parameters {
		for _, constraint := range param.Constraints {
			if constraint.Type == model.ConstraintTypeEnum {
				tests = append(tests, g.generateInvalidEnumTest(feature, param, createPath))
				break
			}
		}
	}

	// Test pattern violations
	for _, param := range feature.Parameters {
		for _, constraint := range param.Constraints {
			if constraint.Type == model.ConstraintTypePattern {
				tests = append(tests, g.generatePatternViolationTest(feature, param, constraint, createPath))
				break
			}
		}
	}

	// Test exceeding max length
	for _, param := range feature.Parameters {
		for _, constraint := range param.Constraints {
			if constraint.Type == model.ConstraintTypeMaxLength {
				tests = append(tests, g.generateExceedMaxLengthTest(feature, param, constraint, createPath))
				break
			}
		}
	}

	// Test below min length
	for _, param := range feature.Parameters {
		for _, constraint := range param.Constraints {
			if constraint.Type == model.ConstraintTypeMinLength {
				tests = append(tests, g.generateBelowMinLengthTest(feature, param, constraint, createPath))
				break
			}
		}
	}

	// Additional negative tests for better coverage

	// Test with values below/above numeric min/max
	for _, param := range feature.Parameters {
		for _, constraint := range param.Constraints {
			if constraint.Type == model.ConstraintTypeMin {
				tests = append(tests, g.generateBelowMinValueTest(feature, param, constraint, createPath))
			}
			if constraint.Type == model.ConstraintTypeMax {
				tests = append(tests, g.generateAboveMaxValueTest(feature, param, constraint, createPath))
			}
		}
	}

	// Test with empty/null values for required fields
	for _, param := range feature.Parameters {
		if param.Required {
			tests = append(tests, g.generateEmptyValueTest(feature, param, createPath))
			tests = append(tests, g.generateNullValueTest(feature, param, createPath))
		}
	}

	// Negative tests for optional fields: sending null/empty for each optional field
	// to verify the server accepts it (optional means absent is OK, null should also be safe)
	for _, param := range feature.Parameters {
		if !param.Required && !isSystemManagedField(param.Name) {
			tests = append(tests, g.generateOptionalFieldNullTest(feature, param, createPath))
		}
	}

	// Test with invalid type (string where number expected, etc.)
	for _, param := range feature.Parameters {
		if param.GoType == "int" || param.GoType == "int8" || param.GoType == "int16" || param.GoType == "int32" || param.GoType == "int64" ||
			param.GoType == "uint" || param.GoType == "uint8" || param.GoType == "uint16" || param.GoType == "uint32" || param.GoType == "uint64" ||
			param.GoType == "float32" || param.GoType == "float64" {
			tests = append(tests, g.generateInvalidTypeTest(feature, param, createPath))
		}
	}

	// Test with missing operation field
	if createPath != nil {
		tests = append(tests, g.generateMissingOperationTest(feature, createPath))
	}

	// Test deployment-specific negative scenarios
	if createPath != nil && createPath.SupportsDeployment && isConfigurationDeploymentPath(createPath) {
		tests = append(tests, g.generateDeployWithoutScopeTest(feature, createPath))
	}

	// Test DELETE of a resource that does not exist (expect 404)
	var deletePath *model.FeaturePath
	for _, path := range paths {
		if path.OperationType == model.OperationTypeDelete {
			deletePath = path
			break
		}
	}
	if deletePath != nil {
		tests = append(tests, g.generateDeleteNonExistentTest(feature, deletePath))
	}

	return tests
}

// generateMissingRequiredFieldTest generates a negative test for each required field.
// It creates a request body with all required fields EXCEPT the one under test,
// then verifies the server returns a 4xx error with a meaningful error message.
// Works for both simple and deep-scanned (objects-array) body formats.
func (g *Generator) generateMissingRequiredFieldTest(
	feature *model.Feature,
	param model.Parameter,
	createPath *model.FeaturePath,
) model.TestCase {

	tc := model.TestCase{
		TestCaseID:  g.nextTestID(),
		FeatureName: feature.Name,
		Priority:    model.TestPriorityP2,
		Type:        model.TestCategoryNegative,
		Description: func() string {
			if param.Description != "" {
				return fmt.Sprintf("Negative test: create %s without required field '%s' — expect 4xx validation error — %s", feature.Name, param.Name, param.Description)
			}
			return fmt.Sprintf("Negative test: create %s without required field '%s' — expect 4xx validation error", feature.Name, param.Name)
		}(),
		IsDeploymentTest: false,
		Steps:            []model.TestStep{},
	}

	// Build body omitting the required field under test
	body := g.generateBodyOmittingRequiredField(feature, createPath, param.Name)

	step := model.TestStep{
		Name:           fmt.Sprintf("createWithout_%s", strings.ReplaceAll(param.Name, "-", "_")),
		Description:    fmt.Sprintf("Attempt to create %s without required field '%s'", feature.Name, param.Name),
		Method:         createPath.HTTPMethod,
		API:            model.APITypeREST,
		Path:           createPath.Path,
		Body:           body,
		ExpectedStatus: 400,
		Validations: []model.Validation{
			{
				Type:        model.ValidationTypeStatusCode,
				Expected:    400,
				Description: fmt.Sprintf("Verify server rejects request missing required field '%s' with 400 Bad Request", param.Name),
			},
			{
				Type:        model.ValidationTypeJSONPathExists,
				Path:        "$.error",
				Description: fmt.Sprintf("Verify error response body contains error details for missing '%s'", param.Name),
			},
		},
	}
	tc.Steps = append(tc.Steps, step)

	return tc
}

// generateInvalidEnumTest generates test for invalid enum value
func (g *Generator) generateInvalidEnumTest(
	feature *model.Feature,
	param model.Parameter,
	createPath *model.FeaturePath,
) model.TestCase {

	tc := model.TestCase{
		TestCaseID:  g.nextTestID(),
		FeatureName: feature.Name,
		Priority:    model.TestPriorityP2,
		Type:        model.TestCategoryNegative,
		Description: fmt.Sprintf("Test %s with invalid enum value", param.Name),
		Steps:       []model.TestStep{},
	}

	body := g.generateRequestBody(feature, createPath)
	g.setOrAddBodyParameter(body, param, "INVALID_ENUM_VALUE")

	step := model.TestStep{
		Name:           "testInvalidEnum",
		Description:    fmt.Sprintf("Attempt to create with invalid %s", param.Name),
		Method:         createPath.HTTPMethod,
		API:            model.APITypeREST,
		Path:           createPath.Path,
		Body:           body,
		ExpectedStatus: 400,
		Validations: []model.Validation{
			{
				Type:     model.ValidationTypeStatusCode,
				Expected: 400,
			},
		},
	}
	tc.Steps = append(tc.Steps, step)

	return tc
}

// Helper function to generate string of specific length
func generateString(length int) string {
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	result := make([]byte, length)
	for i := 0; i < length; i++ {
		result[i] = chars[i%len(chars)]
	}
	return string(result)
}

// generatePatternViolationTest generates test for pattern constraint violation.
// Uses a context-aware invalid value based on the detected pattern type.
func (g *Generator) generatePatternViolationTest(
	feature *model.Feature,
	param model.Parameter,
	constraint model.Constraint,
	createPath *model.FeaturePath,
) model.TestCase {

	tc := model.TestCase{
		TestCaseID:  g.nextTestID(),
		FeatureName: feature.Name,
		Priority:    model.TestPriorityP2,
		Type:        model.TestCategoryNegative,
		Description: fmt.Sprintf("Test %s with pattern violation", param.Name),
		Steps:       []model.TestStep{},
	}

	// Choose an invalid value that specifically violates the detected pattern type
	invalidValue := getSmartInvalidValue(param, constraint)

	body := g.generateRequestBody(feature, createPath)
	g.setOrAddBodyParameter(body, param, invalidValue)

	step := model.TestStep{
		Name:           "testPatternViolation",
		Description:    fmt.Sprintf("Attempt to create with invalid pattern for %s (value: '%s')", param.Name, invalidValue),
		Method:         createPath.HTTPMethod,
		API:            model.APITypeREST,
		Path:           createPath.Path,
		Body:           body,
		ExpectedStatus: 400,
		Validations: []model.Validation{
			{
				Type:     model.ValidationTypeStatusCode,
				Expected: 400,
			},
		},
	}
	tc.Steps = append(tc.Steps, step)

	return tc
}

// getSmartInvalidValue returns an invalid value that specifically violates the pattern,
// based on the detected constraint / parameter type.
func getSmartInvalidValue(param model.Parameter, constraint model.Constraint) string {
	pattern, _ := constraint.Value.(string)

	if isIPAddressPattern(pattern) {
		return "256.0.0.1" // Out-of-range octet – specifically invalidates IPv4 pattern
	}
	if isMACAddressPattern(pattern) {
		return "ZZ:ZZ:ZZ:ZZ:ZZ:ZZ" // Invalid hex digits
	}
	if isUUIDPattern(pattern) {
		return "not-a-valid-uuid-format" // Missing UUID structure
	}
	if isFQDNPattern(pattern) {
		return "invalid..domain..name" // Double dots violate FQDN pattern
	}

	// VR-name style: must start with letter
	if strings.Contains(strings.ToLower(param.Name), "vr-name") ||
		strings.Contains(strings.ToLower(param.Name), "vrf") {
		return "123-starts-with-digit" // Violates pattern requiring leading alpha
	}

	// Generic fallback – contains characters that break most patterns
	return "!!!INVALID-PATTERN@@@"
}

// generateDeleteNonExistentTest generates a test that attempts to delete a resource
// that does not exist, expecting 404.
func (g *Generator) generateDeleteNonExistentTest(
	feature *model.Feature,
	path *model.FeaturePath,
) model.TestCase {
	tc := model.TestCase{
		TestCaseID:  g.nextTestID(),
		FeatureName: feature.Name,
		Priority:    model.TestPriorityP2,
		Type:        model.TestCategoryNegative,
		Description: fmt.Sprintf("Verify deleting non-existent %s returns 404", feature.Name),
		Steps:       []model.TestStep{},
	}

	deletePath := path.Path
	method := path.HTTPMethod
	deleteBody := map[string]interface{}{
		"name":      "does-not-exist-9999",
		"operation": "delete",
	}
	if _, _, _, ok := deepScannedMetadata(feature, path); ok {
		deleteBody = map[string]interface{}{
			"objectIds": []string{"00000000-0000-0000-0000-000000000000"},
		}
	}

	tc.Steps = append(tc.Steps, model.TestStep{
		Name:           "deleteNonExistent",
		Description:    fmt.Sprintf("Attempt to delete %s 'does-not-exist-9999' which was never created", feature.Name),
		Method:         method,
		API:            model.APITypeREST,
		Path:           deletePath,
		PathParams:     pathParamsFor(deletePath, "does-not-exist-9999"),
		Body:           deleteBody,
		ExpectedStatus: 404,
		Validations: []model.Validation{
			{
				Type:     model.ValidationTypeStatusCode,
				Expected: 404,
			},
		},
	})

	return tc
}

// generateBodyOmittingRequiredField builds a request body that omits one specific required field.
// Works for both simple bodies (map[string]interface{}) and deep-scanned bodies (objects-array format).
func (g *Generator) generateBodyOmittingRequiredField(feature *model.Feature, fp *model.FeaturePath, excludeParam string) map[string]interface{} {
	if fpValue, objectType, origin, ok := deepScannedMetadata(feature, fp); ok {
		properties := []map[string]interface{}{}
		keySet := keySetForFeature(feature)
		for _, param := range feature.Parameters {
			if param.Name == excludeParam {
				continue // omit this required field to trigger validation error
			}
			if isSystemManagedField(param.Name) {
				continue
			}
			if param.Required || keySet[param.Name] {
				properties = append(properties, map[string]interface{}{
					"name":   param.Name,
					"type":   g.mapYangTypeToJsonType(param.GoType),
					"value":  g.getSampleValue(param),
					"origin": origin,
				})
			}
		}
		properties = g.appendMissingKeyProperties(feature, properties, origin)
		object := map[string]interface{}{
			"type":       objectType,
			"operation":  "add",
			"properties": properties,
		}
		return map[string]interface{}{
			"featurePath": fpValue,
			"objectType":  objectType,
			"operation":   "add",
			"objects":     []interface{}{object},
		}
	}
	// Simple format: include all required fields except the excluded one
	body := g.generateRequestBody(feature, fp)
	delete(body, excludeParam)
	return body
}

// generateExceedMaxLengthTest generates test exceeding max length
func (g *Generator) generateExceedMaxLengthTest(
	feature *model.Feature,
	param model.Parameter,
	constraint model.Constraint,
	createPath *model.FeaturePath,
) model.TestCase {

	tc := model.TestCase{
		TestCaseID:  g.nextTestID(),
		FeatureName: feature.Name,
		Priority:    model.TestPriorityP2,
		Type:        model.TestCategoryNegative,
		Description: fmt.Sprintf("Test %s exceeding max length", param.Name),
		Steps:       []model.TestStep{},
	}

	body := g.generateRequestBody(feature, createPath)
	maxLen := constraint.Value.(int)
	g.setOrAddBodyParameter(body, param, generateString(maxLen+10)) // Exceed max length

	step := model.TestStep{
		Name:           "testExceedMaxLength",
		Description:    fmt.Sprintf("Attempt to create with %s exceeding max length", param.Name),
		Method:         createPath.HTTPMethod,
		API:            model.APITypeREST,
		Path:           createPath.Path,
		Body:           body,
		ExpectedStatus: 400,
		Validations: []model.Validation{
			{
				Type:     model.ValidationTypeStatusCode,
				Expected: 400,
			},
		},
	}
	tc.Steps = append(tc.Steps, step)

	return tc
}

// generateBelowMinLengthTest generates test below min length
func (g *Generator) generateBelowMinLengthTest(
	feature *model.Feature,
	param model.Parameter,
	constraint model.Constraint,
	createPath *model.FeaturePath,
) model.TestCase {

	tc := model.TestCase{
		TestCaseID:  g.nextTestID(),
		FeatureName: feature.Name,
		Priority:    model.TestPriorityP2,
		Type:        model.TestCategoryNegative,
		Description: fmt.Sprintf("Test %s below min length", param.Name),
		Steps:       []model.TestStep{},
	}

	body := g.generateRequestBody(feature, createPath)
	minLen := constraint.Value.(int)
	if minLen > 1 {
		g.setOrAddBodyParameter(body, param, generateString(minLen-1)) // Below min length
	} else {
		g.setOrAddBodyParameter(body, param, "") // Empty string
	}

	step := model.TestStep{
		Name:           "testBelowMinLength",
		Description:    fmt.Sprintf("Attempt to create with %s below min length", param.Name),
		Method:         createPath.HTTPMethod,
		API:            model.APITypeREST,
		Path:           createPath.Path,
		Body:           body,
		ExpectedStatus: 400,
		Validations: []model.Validation{
			{
				Type:     model.ValidationTypeStatusCode,
				Expected: 400,
			},
		},
	}
	tc.Steps = append(tc.Steps, step)

	return tc
}

// generateBelowMinValueTest generates test with value below minimum
func (g *Generator) generateBelowMinValueTest(
	feature *model.Feature,
	param model.Parameter,
	constraint model.Constraint,
	createPath *model.FeaturePath,
) model.TestCase {
	tc := model.TestCase{
		TestCaseID:  g.nextTestID(),
		FeatureName: feature.Name,
		Priority:    model.TestPriorityP2,
		Type:        model.TestCategoryNegative,
		Description: fmt.Sprintf("Test %s value below minimum (%v)", param.Name, constraint.Value),
		Steps:       []model.TestStep{},
	}

	body := g.generateRequestBody(feature, createPath)
	minValue := constraint.Value.(int)
	g.setOrAddBodyParameter(body, param, minValue-1)

	step := model.TestStep{
		Name:           "testBelowMinValue",
		Description:    fmt.Sprintf("Attempt to create with %s below minimum", param.Name),
		Method:         createPath.HTTPMethod,
		API:            model.APITypeREST,
		Path:           createPath.Path,
		Body:           body,
		ExpectedStatus: 400,
		Validations: []model.Validation{
			{
				Type:     model.ValidationTypeStatusCode,
				Expected: 400,
			},
			{
				Type: model.ValidationTypeJSONPathExists,
				Path: "$.error",
			},
		},
	}
	tc.Steps = append(tc.Steps, step)
	return tc
}

// generateAboveMaxValueTest generates test with value above maximum
func (g *Generator) generateAboveMaxValueTest(
	feature *model.Feature,
	param model.Parameter,
	constraint model.Constraint,
	createPath *model.FeaturePath,
) model.TestCase {
	tc := model.TestCase{
		TestCaseID:  g.nextTestID(),
		FeatureName: feature.Name,
		Priority:    model.TestPriorityP2,
		Type:        model.TestCategoryNegative,
		Description: fmt.Sprintf("Test %s value above maximum (%v)", param.Name, constraint.Value),
		Steps:       []model.TestStep{},
	}

	body := g.generateRequestBody(feature, createPath)
	maxValue := constraint.Value.(int)
	g.setOrAddBodyParameter(body, param, maxValue+1)

	step := model.TestStep{
		Name:           "testAboveMaxValue",
		Description:    fmt.Sprintf("Attempt to create with %s above maximum", param.Name),
		Method:         createPath.HTTPMethod,
		API:            model.APITypeREST,
		Path:           createPath.Path,
		Body:           body,
		ExpectedStatus: 400,
		Validations: []model.Validation{
			{
				Type:     model.ValidationTypeStatusCode,
				Expected: 400,
			},
			{
				Type: model.ValidationTypeJSONPathExists,
				Path: "$.error",
			},
		},
	}
	tc.Steps = append(tc.Steps, step)
	return tc
}

// generateEmptyValueTest generates test with empty string value
func (g *Generator) generateEmptyValueTest(
	feature *model.Feature,
	param model.Parameter,
	createPath *model.FeaturePath,
) model.TestCase {
	tc := model.TestCase{
		TestCaseID:  g.nextTestID(),
		FeatureName: feature.Name,
		Priority:    model.TestPriorityP2,
		Type:        model.TestCategoryNegative,
		Description: fmt.Sprintf("Test with empty %s field", param.Name),
		Steps:       []model.TestStep{},
	}

	body := g.generateRequestBody(feature, createPath)
	g.setOrAddBodyParameter(body, param, "")

	step := model.TestStep{
		Name:           "testEmptyValue",
		Description:    fmt.Sprintf("Attempt to create with empty %s field", param.Name),
		Method:         createPath.HTTPMethod,
		API:            model.APITypeREST,
		Path:           createPath.Path,
		Body:           body,
		ExpectedStatus: 400,
		Validations: []model.Validation{
			{
				Type:     model.ValidationTypeStatusCode,
				Expected: 400,
			},
			{
				Type: model.ValidationTypeJSONPathExists,
				Path: "$.error",
			},
		},
	}
	tc.Steps = append(tc.Steps, step)
	return tc
}

// generateNullValueTest generates test with null value
func (g *Generator) generateNullValueTest(
	feature *model.Feature,
	param model.Parameter,
	createPath *model.FeaturePath,
) model.TestCase {
	tc := model.TestCase{
		TestCaseID:  g.nextTestID(),
		FeatureName: feature.Name,
		Priority:    model.TestPriorityP2,
		Type:        model.TestCategoryNegative,
		Description: fmt.Sprintf("Test with null %s value", param.Name),
		Steps:       []model.TestStep{},
	}

	body := g.generateRequestBody(feature, createPath)
	g.setOrAddBodyParameter(body, param, nil)

	step := model.TestStep{
		Name:           "testNullValue",
		Description:    fmt.Sprintf("Attempt to create with null %s", param.Name),
		Method:         createPath.HTTPMethod,
		API:            model.APITypeREST,
		Path:           createPath.Path,
		Body:           body,
		ExpectedStatus: 400,
		Validations: []model.Validation{
			{
				Type:     model.ValidationTypeStatusCode,
				Expected: 400,
			},
			{
				Type: model.ValidationTypeJSONPathExists,
				Path: "$.error",
			},
		},
	}
	tc.Steps = append(tc.Steps, step)
	return tc
}

// generateInvalidTypeTest generates test with invalid type
func (g *Generator) generateInvalidTypeTest(
	feature *model.Feature,
	param model.Parameter,
	createPath *model.FeaturePath,
) model.TestCase {
	tc := model.TestCase{
		TestCaseID:  g.nextTestID(),
		FeatureName: feature.Name,
		Priority:    model.TestPriorityP2,
		Type:        model.TestCategoryNegative,
		Description: fmt.Sprintf("Test with invalid %s type (string instead of number)", param.Name),
		Steps:       []model.TestStep{},
	}

	body := g.generateRequestBody(feature, createPath)
	g.setOrAddBodyParameter(body, param, "not-a-number")

	step := model.TestStep{
		Name:           "testInvalidType",
		Description:    fmt.Sprintf("Attempt to create with wrong type for %s", param.Name),
		Method:         createPath.HTTPMethod,
		API:            model.APITypeREST,
		Path:           createPath.Path,
		Body:           body,
		ExpectedStatus: 400,
		Validations: []model.Validation{
			{
				Type:     model.ValidationTypeStatusCode,
				Expected: 400,
			},
			{
				Type: model.ValidationTypeJSONPathExists,
				Path: "$.error",
			},
		},
	}
	tc.Steps = append(tc.Steps, step)
	return tc
}

// generateMissingOperationTest generates test with missing operation field
func (g *Generator) generateMissingOperationTest(
	feature *model.Feature,
	createPath *model.FeaturePath,
) model.TestCase {
	tc := model.TestCase{
		TestCaseID:  g.nextTestID(),
		FeatureName: feature.Name,
		Priority:    model.TestPriorityP2,
		Type:        model.TestCategoryNegative,
		Description: "Test with missing operation field",
		Steps:       []model.TestStep{},
	}

	body := g.generateRequestBody(feature, createPath)
	// Remove operation field if it exists
	delete(body, "operation")

	step := model.TestStep{
		Name:           "testMissingOperation",
		Description:    "Attempt to create without operation field",
		Method:         createPath.HTTPMethod,
		API:            model.APITypeREST,
		Path:           createPath.Path,
		Body:           body,
		ExpectedStatus: 400,
		Validations: []model.Validation{
			{
				Type:     model.ValidationTypeStatusCode,
				Expected: 400,
			},
			{
				Type: model.ValidationTypeJSONPathExists,
				Path: "$.error",
			},
		},
	}
	tc.Steps = append(tc.Steps, step)
	return tc
}

// generateDeployWithoutScopeTest generates test attempting deployment without scoping
func (g *Generator) generateDeployWithoutScopeTest(
	feature *model.Feature,
	createPath *model.FeaturePath,
) model.TestCase {
	tc := model.TestCase{
		TestCaseID:  g.nextTestID(),
		FeatureName: feature.Name,
		Priority:    model.TestPriorityP2,
		Type:        model.TestCategoryNegative,
		Description: "Test deployment without scoping",
		Steps:       []model.TestStep{},
	}

	// Create resource
	body := g.generateRequestBody(feature, createPath)
	g.setBodyResourceName(feature, body, "TestProfile-NoScope")

	createStep := model.TestStep{
		Name:           "createResource",
		Description:    "Create configuration",
		Method:         createPath.HTTPMethod,
		API:            model.APITypeREST,
		Path:           createPath.Path,
		Body:           body,
		ExpectedStatus: 201,
		Validations: []model.Validation{
			{
				Type:     model.ValidationTypeStatusCode,
				Expected: 201,
			},
		},
	}
	tc.Steps = append(tc.Steps, createStep)

	// Attempt deploy without scope
	deployStep := model.TestStep{
		Name:        "deployWithoutScope",
		Description: "Attempt to deploy without scoping",
		Method:      "POST",
		API:         model.APITypeREST,
		Path:        "/global-profile/TestProfile-NoScope/deploy",
		PathParams: map[string]string{
			"profileName": "TestProfile-NoScope",
		},
		Body: map[string]interface{}{
			"deploymentMethod": "immediate",
		},
		ExpectedStatus: 400,
		Validations: []model.Validation{
			{
				Type:     model.ValidationTypeStatusCode,
				Expected: 400,
			},
			{
				Type: model.ValidationTypeJSONPathExists,
				Path: "$.error",
			},
		},
	}
	tc.Steps = append(tc.Steps, deployStep)

	return tc
}

// generateOptionalFieldNullTest verifies that sending null/omitted for an optional field
// is accepted by the server (the field is optional, so the server must not require it).
// This is a negative-style validation: the server MUST return 201 (not 400).
func (g *Generator) generateOptionalFieldNullTest(
	feature *model.Feature,
	param model.Parameter,
	createPath *model.FeaturePath,
) model.TestCase {
	tc := model.TestCase{
		TestCaseID:       g.nextTestID(),
		FeatureName:      feature.Name,
		Priority:         model.TestPriorityP3,
		Type:             model.TestCategoryNegative,
		Description:      fmt.Sprintf("Verify optional field '%s' can be omitted — server must accept without it (non-required per YANG model)", param.Name),
		IsDeploymentTest: false,
		Steps:            []model.TestStep{},
	}

	// Build body without this optional field
	body := g.generateBodyOmittingOptionalField(feature, createPath, param.Name)

	step := model.TestStep{
		Name:           fmt.Sprintf("createWithout_optional_%s", strings.ReplaceAll(param.Name, "-", "_")),
		Description:    fmt.Sprintf("Create %s without optional field '%s' — server must accept (201)", feature.Name, param.Name),
		Method:         createPath.HTTPMethod,
		API:            model.APITypeREST,
		Path:           createPath.Path,
		Body:           body,
		ExpectedStatus: 201,
		Validations: []model.Validation{
			{
				Type:        model.ValidationTypeStatusCode,
				Expected:    201,
				Description: fmt.Sprintf("Verify server accepts request when optional field '%s' is omitted", param.Name),
			},
		},
	}
	tc.Steps = append(tc.Steps, step)

	return tc
}

// generateBodyOmittingOptionalField builds a request body that includes all required
// fields but excludes the specified optional field.
func (g *Generator) generateBodyOmittingOptionalField(feature *model.Feature, fp *model.FeaturePath, excludeParam string) map[string]interface{} {
	if fpValue, objectType, origin, ok := deepScannedMetadata(feature, fp); ok {
		properties := []map[string]interface{}{}
		keySet := keySetForFeature(feature)
		for _, param := range feature.Parameters {
			if param.Name == excludeParam {
				continue // skip this optional field
			}
			if isSystemManagedField(param.Name) {
				continue
			}
			if param.Required || keySet[param.Name] {
				properties = append(properties, map[string]interface{}{
					"name":   param.Name,
					"type":   g.mapYangTypeToJsonType(param.GoType),
					"value":  g.getSampleValue(param),
					"origin": origin,
				})
			}
		}
		properties = g.appendMissingKeyProperties(feature, properties, origin)
		object := map[string]interface{}{
			"type":       objectType,
			"operation":  "add",
			"properties": properties,
		}
		return map[string]interface{}{
			"featurePath": fpValue,
			"objectType":  objectType,
			"operation":   "add",
			"objects":     []interface{}{object},
		}
	}
	// Simple format: all required fields, no optional field under test
	body := make(map[string]interface{})
	for _, param := range feature.Parameters {
		if param.Name == excludeParam {
			continue
		}
		if isSystemManagedField(param.Name) {
			continue
		}
		if param.Required {
			body[param.Name] = g.getSampleValue(param)
		}
	}
	return body
}
