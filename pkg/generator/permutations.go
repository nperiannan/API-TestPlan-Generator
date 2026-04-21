package generator

import (
	"fmt"
	"strings"

	"github.com/extremenetworks/testcase-generator/pkg/model"
)

// getDeploymentLevels returns all valid deployment target types based on actual REST API
// API supports: /configuration-profile/{name}/sites/deploy and /configuration-profile/{name}/devices/deploy
// NOTE: scopeType is kept for backward compatibility but not used in actual deployment
func getDeploymentLevels() []struct {
	scopeType  model.ScopeType
	targetType model.TargetType
} {
	return []struct {
		scopeType  model.ScopeType
		targetType model.TargetType
	}{
		// Deploy to devices via /configuration-profile/{name}/devices/deploy
		{model.ScopeTypeDevice, model.TargetTypeDevice},

		// Deploy to sites via /configuration-profile/{name}/sites/deploy
		{model.ScopeTypeSite, model.TargetTypeSite},
	}
}

// generatePermutationTests generates comprehensive test variations
// based on parameter combinations, query params, and path params
// This creates ~1000+ valid test cases across all features
func (g *Generator) generatePermutationTests(feature *model.Feature, paths []*model.FeaturePath) []model.TestCase {
	var tests []model.TestCase

	// Find CREATE and READ paths for permutation testing
	var createPath, readPath *model.FeaturePath
	for _, path := range paths {
		if path.OperationType == model.OperationTypeCreate {
			createPath = path
		}
		if path.OperationType == model.OperationTypeRead {
			readPath = path
		}
	}

	if createPath == nil {
		return tests
	}

	// Generate permutations based on:
	// 1. Required parameters with different valid values
	// 2. Optional parameters (include/exclude combinations)
	// 3. Path parameters (different resource identifiers)
	// 4. Query parameters (different filter combinations)

	// Strategy: For each feature, generate multiple test variations
	// to reach ~30-50 tests per feature (for 30 features = ~1000+ tests)

	tests = append(tests, g.generateRequiredParamPermutations(feature, createPath, readPath)...)
	tests = append(tests, g.generateOptionalParamPermutations(feature, createPath, readPath)...)
	tests = append(tests, g.generatePathParamVariations(feature, createPath, readPath)...)
	tests = append(tests, g.generateBoundaryValuePermutations(feature, createPath, readPath)...)
	tests = append(tests, g.generateEnumValuePermutations(feature, createPath, readPath)...)
	tests = append(tests, g.generateCombinedParamPermutations(feature, createPath, readPath)...)
	tests = append(tests, g.generateDataTypeVariations(feature, createPath, readPath)...)

	return tests
}

// generateRequiredParamPermutations generates test variations for required parameters
func (g *Generator) generateRequiredParamPermutations(feature *model.Feature, createPath, readPath *model.FeaturePath) []model.TestCase {
	var tests []model.TestCase

	if len(feature.Parameters) == 0 {
		return tests
	}

	// Get all required parameters
	requiredParams := []model.Parameter{}
	for _, param := range feature.Parameters {
		if param.Required {
			requiredParams = append(requiredParams, param)
		}
	}

	if len(requiredParams) == 0 {
		return tests
	}

	// Generate test for each required parameter with different valid values
	// Increased from 3 to 5 variations per parameter for more comprehensive coverage
	// For each variation, generate tests for all 3 deployment levels (device, site, site-group)
	deploymentLevels := getDeploymentLevels()

	for i, param := range requiredParams {
		// Generate 5 variations per required parameter with different valid values
		for variation := 1; variation <= 5; variation++ {
			// Generate tests for all 3 deployment levels
			for _, level := range deploymentLevels {
				tc := model.TestCase{
					TestCaseID:       g.nextTestID(),
					FeatureName:      feature.Name,
					Priority:         model.TestPriorityP0,
					Type:             model.TestCategoryFunctional,
					Description:      fmt.Sprintf("Create %s with valid value variation %d for required param '%s', scope to %s, target to %s, deploy and verify", feature.Name, variation, param.Name, level.scopeType, level.targetType),
					ScopeType:        level.scopeType,
					TargetType:       level.targetType,
					DeploymentMethod: model.DeploymentMethodImmediate,
					IsDeploymentTest: true,
					Steps:            []model.TestStep{},
				}

				// Create with varied parameter value
				body := g.generateVariedBody(feature, createPath, param, variation)
				createStep := model.TestStep{
					Name:           fmt.Sprintf("createWithParam%dVar%d", i, variation),
					Description:    fmt.Sprintf("Create with variation %d of %s", variation, param.Name),
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

				// Add deployment steps for the specific deployment level
				resourceName := fmt.Sprintf("TestResource-%s-Var%d", param.Name, variation)
				if nameVal, ok := body["name"].(string); ok {
					resourceName = nameVal
				}
				g.addDeploymentSteps(&tc, feature, resourceName, level.scopeType, level.targetType)

				tests = append(tests, tc)
			}
		}
	}

	return tests
}

// generateOptionalParamPermutations generates test variations with different optional parameter combinations
func (g *Generator) generateOptionalParamPermutations(feature *model.Feature, createPath, readPath *model.FeaturePath) []model.TestCase {
	var tests []model.TestCase

	// Get optional parameters
	optionalParams := []model.Parameter{}
	for _, param := range feature.Parameters {
		if !param.Required {
			optionalParams = append(optionalParams, param)
		}
	}

	if len(optionalParams) == 0 {
		return tests
	}

	// Generate tests with different combinations of optional parameters
	// Strategy: Include 0, 1, 2, ..., all optional params
	// Increased variations from 2 to 3 per combination level
	maxCombinations := len(optionalParams)
	if maxCombinations > 5 {
		maxCombinations = 5 // Limit to prevent explosion
	}

	deploymentLevels := getDeploymentLevels()

	for numOptional := 0; numOptional <= maxCombinations; numOptional++ {
		// Generate 3 variations per combination level (increased from 2)
		for variation := 1; variation <= 3; variation++ {
			// Generate tests for all 3 deployment levels
			for _, level := range deploymentLevels {
				tc := model.TestCase{
					TestCaseID:       g.nextTestID(),
					FeatureName:      feature.Name,
					Priority:         model.TestPriorityP1,
					Type:             model.TestCategoryFunctional,
					Description:      fmt.Sprintf("Deploy and verify %s with %d optional parameters (variation %d), scope to %s, target to %s", feature.Name, numOptional, variation, level.scopeType, level.targetType),
					ScopeType:        level.scopeType,
					TargetType:       level.targetType,
					DeploymentMethod: model.DeploymentMethodImmediate,
					IsDeploymentTest: true,
					Steps:            []model.TestStep{},
				}

				// Build body with required + selected optional params
				body := g.generateRequestBody(feature, createPath)
				resourceName := fmt.Sprintf("TestResource-Opt%d-Var%d", numOptional, variation)
				body["name"] = resourceName

				// Add selected optional parameters
				for i := 0; i < numOptional && i < len(optionalParams); i++ {
					param := optionalParams[i]
					g.setBodyParameterValue(body, param.Name, g.getVariedValue(param, variation))
				}

				createStep := model.TestStep{
					Name:           fmt.Sprintf("createWithOptionals%d", numOptional),
					Description:    fmt.Sprintf("Create with %d optional parameters", numOptional),
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
				g.addDeploymentSteps(&tc, feature, resourceName, level.scopeType, level.targetType)

				tests = append(tests, tc)
			}
		}
	}

	return tests
}

// generatePathParamVariations generates tests with different path parameter values
func (g *Generator) generatePathParamVariations(feature *model.Feature, createPath, readPath *model.FeaturePath) []model.TestCase {
	var tests []model.TestCase

	if len(createPath.PathParams) == 0 {
		return tests
	}

	// For features with path params (like /vlan/{vlan-id}, /syslog/{server-id}),
	// generate multiple tests with different resource identifiers
	// Increased from 5 to 10 variations
	variations := []string{
		"Resource-A", "Resource-B", "Resource-C", "Resource-Alpha", "Resource-Beta",
		"Resource-Gamma", "Resource-Delta", "Resource-Epsilon", "Resource-Test1", "Resource-Test2",
	}

	for i, resourceID := range variations {
		tc := model.TestCase{
			TestCaseID:       g.nextTestID(),
			FeatureName:      feature.Name,
			Priority:         model.TestPriorityP1,
			Type:             model.TestCategoryFunctional,
			Description:      fmt.Sprintf("Deploy and verify %s with path param variation %d (%s)", feature.Name, i+1, resourceID),
			ScopeType:        model.ScopeTypeDevice,
			TargetType:       model.TargetTypeDevice,
			DeploymentMethod: model.DeploymentMethodImmediate,
			IsDeploymentTest: true,
			Steps:            []model.TestStep{},
		}

		body := g.generateRequestBody(feature, createPath)
		body["name"] = resourceID

		createStep := model.TestStep{
			Name:           fmt.Sprintf("createResource%d", i+1),
			Description:    fmt.Sprintf("Create resource with ID: %s", resourceID),
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

		// Add path params if needed
		if len(createPath.PathParams) > 0 {
			createStep.PathParams = make(map[string]string)
			for _, param := range createPath.PathParams {
				// Use FixedValue if set (for featurePath in deep-scanned features)
				if param.FixedValue != "" {
					createStep.PathParams[param.Name] = param.FixedValue
				} else {
					createStep.PathParams[param.Name] = resourceID
				}
			}
		}

		tc.Steps = append(tc.Steps, createStep)
		g.addDeploymentSteps(&tc, feature, resourceID, model.ScopeTypeDevice, model.TargetTypeDevice)

		tests = append(tests, tc)
	}

	return tests
}

// generateBoundaryValuePermutations generates tests with boundary values for each constrained parameter
func (g *Generator) generateBoundaryValuePermutations(feature *model.Feature, createPath, readPath *model.FeaturePath) []model.TestCase {
	var tests []model.TestCase

	// For each parameter with constraints, generate boundary value tests
	for _, param := range feature.Parameters {
		if len(param.Constraints) == 0 {
			continue
		}

		for _, constraint := range param.Constraints {
			switch constraint.Type {
			case model.ConstraintTypeMinLength, model.ConstraintTypeMin:
				// Test with minimum value
				tests = append(tests, g.generateBoundaryTest(feature, createPath, readPath, param, constraint, "min"))

			case model.ConstraintTypeMaxLength, model.ConstraintTypeMax:
				// Test with maximum value
				tests = append(tests, g.generateBoundaryTest(feature, createPath, readPath, param, constraint, "max"))
			}
		}
	}

	return tests
}

// generateBoundaryTest creates a single boundary value test
func (g *Generator) generateBoundaryTest(feature *model.Feature, createPath, readPath *model.FeaturePath, param model.Parameter, constraint model.Constraint, boundaryType string) model.TestCase {
	tc := model.TestCase{
		TestCaseID:       g.nextTestID(),
		FeatureName:      feature.Name,
		Priority:         model.TestPriorityP1,
		Type:             model.TestCategoryFunctional,
		Description:      fmt.Sprintf("Deploy and verify %s with %s boundary value for '%s'", feature.Name, boundaryType, param.Name),
		ScopeType:        model.ScopeTypeDevice,
		TargetType:       model.TargetTypeDevice,
		DeploymentMethod: model.DeploymentMethodImmediate,
		IsDeploymentTest: true,
		Steps:            []model.TestStep{},
	}

	body := g.generateRequestBody(feature, createPath)
	resourceName := fmt.Sprintf("TestResource-%s-%s", param.Name, boundaryType)
	body["name"] = resourceName

	// Set boundary value
	if boundaryType == "min" {
		g.setBodyParameterValue(body, param.Name, g.getMinBoundaryValue(param, constraint))
	} else {
		g.setBodyParameterValue(body, param.Name, g.getMaxBoundaryValue(param, constraint))
	}

	createStep := model.TestStep{
		Name:           "createWithBoundaryValue",
		Description:    fmt.Sprintf("Create with %s value for %s", boundaryType, param.Name),
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
	g.addDeploymentSteps(&tc, feature, resourceName, model.ScopeTypeDevice, model.TargetTypeDevice)

	return tc
}

// generateEnumValuePermutations generates tests for each enum value
func (g *Generator) generateEnumValuePermutations(feature *model.Feature, createPath, readPath *model.FeaturePath) []model.TestCase {
	var tests []model.TestCase

	// Recursively collect all parameters with enum constraints (including nested)
	enumParams := g.collectEnumParameters(feature.Parameters, "")

	// For each parameter with enum constraint, generate a test for each enum value
	for _, enumParam := range enumParams {
		for i, enumVal := range enumParam.enumValues {
			tc := model.TestCase{
				TestCaseID:       g.nextTestID(),
				FeatureName:      feature.Name,
				Priority:         model.TestPriorityP1,
				Type:             model.TestCategoryFunctional,
				Description:      fmt.Sprintf("Deploy and verify %s with enum value '%s' for '%s'", feature.Name, enumVal, enumParam.fullPath),
				ScopeType:        model.ScopeTypeDevice,
				TargetType:       model.TargetTypeDevice,
				DeploymentMethod: model.DeploymentMethodImmediate,
				IsDeploymentTest: true,
				Steps:            []model.TestStep{},
			}

			body := g.generateRequestBody(feature, createPath)
			resourceName := fmt.Sprintf("TestResource-%s-Enum%d", enumParam.name, i+1)
			body["name"] = resourceName
			g.setBodyParameterValue(body, enumParam.fullPath, enumVal)

			createStep := model.TestStep{
				Name:           fmt.Sprintf("createWithEnum%d", i+1),
				Description:    fmt.Sprintf("Create with enum value: %s", enumVal),
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
			g.addDeploymentSteps(&tc, feature, resourceName, model.ScopeTypeDevice, model.TargetTypeDevice)
			tests = append(tests, tc)
		}
	}

	return tests
}

// enumParameterInfo holds information about a parameter with enum constraint
type enumParameterInfo struct {
	name       string
	fullPath   string
	enumValues []string
}

// collectEnumParameters recursively collects all parameters with enum constraints
func (g *Generator) collectEnumParameters(params []model.Parameter, parentPath string) []enumParameterInfo {
	var enumParams []enumParameterInfo

	for _, param := range params {
		currentPath := param.Name
		if parentPath != "" {
			currentPath = parentPath + "." + param.Name
		}

		// Check if this parameter has enum constraint
		for _, constraint := range param.Constraints {
			if constraint.Type == model.ConstraintTypeEnum {
				if enumValues, ok := constraint.Value.([]string); ok && len(enumValues) > 0 {
					enumParams = append(enumParams, enumParameterInfo{
						name:       param.Name,
						fullPath:   currentPath,
						enumValues: enumValues,
					})
				}
			}
		}

		// Recursively check nested properties
		if len(param.NestedProperties) > 0 {
			nestedEnums := g.collectEnumParameters(param.NestedProperties, currentPath)
			enumParams = append(enumParams, nestedEnums...)
		}
	}

	return enumParams
}

// Helper functions for generating varied values

func (g *Generator) generateVariedBody(feature *model.Feature, createPath *model.FeaturePath, param model.Parameter, variation int) map[string]interface{} {
	body := g.generateRequestBody(feature, createPath)
	resourceName := fmt.Sprintf("TestResource-%s-Var%d", param.Name, variation)
	body["name"] = resourceName
	g.setBodyParameterValue(body, param.Name, g.getVariedValue(param, variation))
	return body
}

func (g *Generator) getVariedValue(param model.Parameter, variation int) interface{} {
	// First, check if parameter has a pattern constraint and generate appropriate value
	if value, ok := g.getPatternBasedValue(param, variation); ok {
		return value
	}

	// Second, check parameter name for common patterns (like "server", "address", "ip")
	// This handles cases where pattern constraints might not be detected
	if param.GoType == "string" {
		paramNameLower := strings.ToLower(param.Name)
		if paramNameLower == "server" || paramNameLower == "address" ||
			paramNameLower == "ip" || paramNameLower == "host" ||
			strings.Contains(paramNameLower, "-server") ||
			strings.Contains(paramNameLower, "-address") {
			// Return varied IP addresses
			return getIPAddressValue(variation)
		}
	}

	// Fall back to type-based generation
	switch param.GoType {
	case "string":
		return fmt.Sprintf("testValue-%d", variation)
	case "int", "int32", "int64":
		return 100 * variation
	case "bool":
		return variation%2 == 0
	case "float64":
		return float64(variation) * 1.5
	default:
		return fmt.Sprintf("value-%d", variation)
	}
}

func (g *Generator) getMinBoundaryValue(param model.Parameter, constraint model.Constraint) interface{} {
	// First, check if we need to respect a pattern constraint
	if value, ok := g.getPatternBoundaryValue(param, constraint, true); ok {
		return value
	}

	// Second, check parameter name for common IP/address patterns
	if param.GoType == "string" {
		paramNameLower := strings.ToLower(param.Name)
		if paramNameLower == "server" || paramNameLower == "address" ||
			paramNameLower == "ip" || paramNameLower == "host" ||
			strings.Contains(paramNameLower, "-server") ||
			strings.Contains(paramNameLower, "-address") {
			// Return minimum valid IP address
			return "0.0.0.0"
		}
	}

	// Fall back to standard boundary handling
	switch constraint.Type {
	case model.ConstraintTypeMinLength:
		if minLen, ok := constraint.Value.(int); ok {
			return g.generateStringOfLength(minLen)
		}
	case model.ConstraintTypeMin:
		return constraint.Value
	}
	return g.getSampleValue(param)
}

func (g *Generator) getMaxBoundaryValue(param model.Parameter, constraint model.Constraint) interface{} {
	// First, check if we need to respect a pattern constraint
	if value, ok := g.getPatternBoundaryValue(param, constraint, false); ok {
		return value
	}

	// Second, check parameter name for common IP/address patterns
	if param.GoType == "string" {
		paramNameLower := strings.ToLower(param.Name)
		if paramNameLower == "server" || paramNameLower == "address" ||
			paramNameLower == "ip" || paramNameLower == "host" ||
			strings.Contains(paramNameLower, "-server") ||
			strings.Contains(paramNameLower, "-address") {
			// Return maximum valid IP address
			return "255.255.255.255"
		}
	}

	// Fall back to standard boundary handling
	switch constraint.Type {
	case model.ConstraintTypeMaxLength:
		if maxLen, ok := constraint.Value.(int); ok {
			return g.generateStringOfLength(maxLen)
		}
	case model.ConstraintTypeMax:
		return constraint.Value
	}
	return g.getSampleValue(param)
}

func (g *Generator) generateStringOfLength(length int) string {
	result := ""
	for i := 0; i < length; i++ {
		result += "a"
	}
	return result
}

// generateCombinedParamPermutations generates tests with combinations of multiple parameters
func (g *Generator) generateCombinedParamPermutations(feature *model.Feature, createPath, readPath *model.FeaturePath) []model.TestCase {
	var tests []model.TestCase

	if len(feature.Parameters) < 2 {
		return tests
	}

	// Generate tests with combinations of 2-3 parameters having different values
	// This creates realistic scenarios where multiple parameters interact
	for i := 0; i < len(feature.Parameters) && i < 3; i++ {
		for j := i + 1; j < len(feature.Parameters) && j < 4; j++ {
			param1 := feature.Parameters[i]
			param2 := feature.Parameters[j]

			// Generate 3 variations of this parameter pair
			for variation := 1; variation <= 3; variation++ {
				tc := model.TestCase{
					TestCaseID:       g.nextTestID(),
					FeatureName:      feature.Name,
					Priority:         model.TestPriorityP1,
					Type:             model.TestCategoryFunctional,
					Description:      fmt.Sprintf("Deploy and verify %s with combined values for '%s' and '%s' (var %d)", feature.Name, param1.Name, param2.Name, variation),
					ScopeType:        model.ScopeTypeDevice,
					TargetType:       model.TargetTypeDevice,
					DeploymentMethod: model.DeploymentMethodImmediate,
					IsDeploymentTest: true,
					Steps:            []model.TestStep{},
				}

				body := g.generateRequestBody(feature, createPath)
				resourceName := fmt.Sprintf("TestResource-Combo-%s-%s-V%d", param1.Name, param2.Name, variation)
				body["name"] = resourceName
				body[param1.Name] = g.getVariedValue(param1, variation)
				body[param2.Name] = g.getVariedValue(param2, variation+1) // Different variation

				createStep := model.TestStep{
					Name:           fmt.Sprintf("createWithCombination%d", variation),
					Description:    fmt.Sprintf("Create with %s=%v and %s=%v", param1.Name, body[param1.Name], param2.Name, body[param2.Name]),
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
				g.addDeploymentSteps(&tc, feature, resourceName, model.ScopeTypeDevice, model.TargetTypeDevice)

				tests = append(tests, tc)
			}
		}
	}

	return tests
}

// generateDataTypeVariations generates tests with different valid values for each data type
func (g *Generator) generateDataTypeVariations(feature *model.Feature, createPath, readPath *model.FeaturePath) []model.TestCase {
	var tests []model.TestCase

	// Group parameters by data type
	paramsByType := make(map[string][]model.Parameter)
	for _, param := range feature.Parameters {
		paramsByType[param.GoType] = append(paramsByType[param.GoType], param)
	}

	// For each data type, generate multiple tests with different valid values
	for dataType, params := range paramsByType {
		if len(params) == 0 {
			continue
		}

		// Generate 3 tests per data type
		for variation := 1; variation <= 3; variation++ {
			param := params[0] // Use first param of this type

			tc := model.TestCase{
				TestCaseID:       g.nextTestID(),
				FeatureName:      feature.Name,
				Priority:         model.TestPriorityP1,
				Type:             model.TestCategoryFunctional,
				Description:      fmt.Sprintf("Deploy and verify %s with %s value variation %d for '%s'", feature.Name, dataType, variation, param.Name),
				ScopeType:        model.ScopeTypeDevice,
				TargetType:       model.TargetTypeDevice,
				DeploymentMethod: model.DeploymentMethodImmediate,
				IsDeploymentTest: true,
				Steps:            []model.TestStep{},
			}

			body := g.generateRequestBody(feature, createPath)
			resourceName := fmt.Sprintf("TestResource-%s-Type%s-V%d", param.Name, dataType, variation)
			body["name"] = resourceName

			// Generate type-specific variations
			g.setBodyParameterValue(body, param.Name, g.getDataTypeVariation(param, dataType, variation))

			createStep := model.TestStep{
				Name:           fmt.Sprintf("createWithTypeVar%d", variation),
				Description:    fmt.Sprintf("Create with %s type variation: %v", dataType, body[param.Name]),
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
			g.addDeploymentSteps(&tc, feature, resourceName, model.ScopeTypeDevice, model.TargetTypeDevice)

			tests = append(tests, tc)
		}
	}

	return tests
}

// getDataTypeVariation generates varied values based on data type
func (g *Generator) getDataTypeVariation(param model.Parameter, dataType string, variation int) interface{} {
	switch dataType {
	case "string":
		// Return different string patterns
		patterns := []string{
			"test-value-" + fmt.Sprint(variation),
			"TestValue" + fmt.Sprint(variation),
			"test_value_" + fmt.Sprint(variation),
		}
		if variation <= len(patterns) {
			return patterns[variation-1]
		}
		return fmt.Sprintf("value-%d", variation)

	case "int", "int32", "int64":
		// Return different numeric ranges
		values := []int{10, 100, 1000, 50, 500}
		if variation <= len(values) {
			return values[variation-1] * variation
		}
		return 100 * variation

	case "bool":
		// Alternate boolean values
		return variation%2 == 1

	case "float64":
		// Return different float values
		values := []float64{1.5, 2.7, 3.14, 10.5, 100.25}
		if variation <= len(values) {
			return values[variation-1]
		}
		return float64(variation) * 1.5

	default:
		return fmt.Sprintf("value-%d", variation)
	}
}
