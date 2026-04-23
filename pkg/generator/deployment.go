package generator

import (
	"fmt"

	"github.com/extremenetworks/testcase-generator/pkg/model"
)

// addDeploymentSteps adds deployment and verification steps to a test case
// Based on actual REST API: /configuration-profile/{name}/sites/deploy or /configuration-profile/{name}/devices/deploy
func (g *Generator) addDeploymentSteps(tc *model.TestCase, feature *model.Feature, profileName string, scopeType model.ScopeType, targetType model.TargetType) {
	// Determine deploy endpoint and body based on target type
	var deployPath string
	var deployBody map[string]interface{}
	var statusPath string
	
	switch targetType {
	case model.TargetTypeSite:
		// Deploy to sites
		deployPath = fmt.Sprintf("/configuration-profile/%s/sites/deploy", profileName)
		deployBody = map[string]interface{}{
			"sites":     []string{"test-site-001"},
			"deployNow": true,
		}
		statusPath = fmt.Sprintf("/configuration-profile/%s/site/test-site-001/deploy/status", profileName)
		
	case model.TargetTypeDevice:
		// Deploy to devices
		deployPath = fmt.Sprintf("/configuration-profile/%s/devices/deploy", profileName)
		deployBody = map[string]interface{}{
			"devices":   []string{"test-device-001"},
			"deployNow": true,
		}
		statusPath = fmt.Sprintf("/configuration-profile/%s/device/test-device-001/deploy/status", profileName)
		
	default:
		// For any other target type, use overall profile deployment
		deployPath = fmt.Sprintf("/configuration-profile/%s/deploy", profileName)
		deployBody = map[string]interface{}{
			"deployNow": true,
		}
		statusPath = fmt.Sprintf("/configuration-profile/%s/deploy/status", profileName)
	}

	// Deploy step
	deployStep := model.TestStep{
		Name:        fmt.Sprintf("deployTo%s", targetType),
		Description: fmt.Sprintf("Deploy configuration to %s", targetType),
		Method:      "POST",
		API:         model.APITypeREST,
		Path:        deployPath,
		PathParams:  map[string]string{"name": profileName},
		Body:        deployBody,
		ExpectedStatus: 202,
		Validations: []model.Validation{
			{
				Type:     model.ValidationTypeStatusCode,
				Expected: 202,
			},
		},
	}
	tc.Steps = append(tc.Steps, deployStep)

	// Status check step
	statusStep := model.TestStep{
		Name:           "checkDeploymentStatus",
		Description:    "Verify deployment status",
		Method:         "POST",
		API:            model.APITypeREST,
		Path:           statusPath,
		PathParams:     map[string]string{"name": profileName},
		Body:           map[string]interface{}{},
		ExpectedStatus: 200,
		Validations: []model.Validation{
			{
				Type:     model.ValidationTypeStatusCode,
				Expected: 200,
			},
			{
				Type:     model.ValidationTypeJSONPathEquals,
				Path:     "$.status",
				Expected: "SUCCESS",
			},
		},
		Timeout: 300,
	}
	tc.Steps = append(tc.Steps, statusStep)

	// NOS verification step
	// Convert targetType to scopeType for NOS endpoint lookup
	nosScopeType := model.ScopeType(targetType)
	nosEndpoint := g.nosParser.FindEndpointForFeature(feature.Name, nosScopeType)
	if nosEndpoint != nil {
		deviceID := "test-device-001"
		if targetType == model.TargetTypeSite {
			deviceID = "test-site-001"
		}
		
		nosStep := model.TestStep{
			Name:        fmt.Sprintf("verifyNosConfigFor%s", targetType),
			Description: fmt.Sprintf("Verify configuration on NOS %s", targetType),
			Method:      nosEndpoint.Method,
			API:         model.APITypeNOSAPI,
			Path:        nosEndpoint.Path,
			PathParams: map[string]string{
				"siteId":   deviceID,
				"deviceId": deviceID,
			},
			ExpectedStatus: 200,
			DevicesScope:   nosEndpoint.DeviceScope,
			Validations: []model.Validation{
				{
					Type:     model.ValidationTypeStatusCode,
					Expected: 200,
				},
				{
					Type:        model.ValidationTypeNosConfigMatchesExpected,
					Description: "Verify NOS device configuration matches expected state",
				},
			},
		}
		tc.Steps = append(tc.Steps, nosStep)
	}
}

// generateDeploymentTest generates a full deployment test scenario
func (g *Generator) generateDeploymentTest(
	feature *model.Feature,
	createPath, readPath, deletePath *model.FeaturePath,
	scopePaths, targetPaths, deployPaths []*model.FeaturePath,
	scopeType model.ScopeType,
	targetType model.TargetType,
) model.TestCase {

	tc := model.TestCase{
		TestCaseID:       g.nextTestID(),
		FeatureName:      feature.Name,
		Priority:         model.TestPriorityP1,
		Type:             model.TestCategoryFunctional,
		Description:      fmt.Sprintf("Create %s, scope to %s, target to %s, deploy, and verify on NOS devices", feature.Name, scopeType, targetType),
		ScopeType:        scopeType,
		TargetType:       targetType,
		DeploymentMethod: model.DeploymentMethodImmediate,
		IsDeploymentTest: true,
		Steps:            []model.TestStep{},
	}

	profileName := "TestProfile"
	scopeID := "test-scope-001"

	// Step 1: Create configuration using the feature's actual body structure
	createBody := g.generateRequestBody(feature, createPath)
	createBody["name"] = profileName

	createStep := model.TestStep{
		Name:           fmt.Sprintf("create%s", feature.Name),
		Description:    fmt.Sprintf("Create %s configuration", feature.Name),
		Method:         createPath.HTTPMethod,
		API:            model.APITypeREST,
		Path:           createPath.Path,
		Body:           createBody,
		ExpectedStatus: 201,
		Validations: []model.Validation{
			{
				Type:     model.ValidationTypeStatusCode,
				Expected: 201,
			},
		},
	}
	tc.Steps = append(tc.Steps, createStep)

	// Step 2: Get profile to verify creation
	if readPath != nil {
		getStep := model.TestStep{
			Name:           "getProfile",
			Description:    "Retrieve created profile",
			Method:         readPath.HTTPMethod,
			API:            model.APITypeREST,
			Path:           readPath.Path,
			PathParams:     map[string]string{"profileName": profileName},
			ExpectedStatus: 200,
			Validations: []model.Validation{
				{
					Type:     model.ValidationTypeStatusCode,
					Expected: 200,
				},
			},
		}
		tc.Steps = append(tc.Steps, getStep)
	}

	// Step 3: Scope the profile
	scopePath := findPathForScopeType(scopePaths, scopeType)
	if scopePath != nil {
		scopeStep := model.TestStep{
			Name:        fmt.Sprintf("scopeProfileTo%s", scopeType),
			Description: fmt.Sprintf("Scope profile to %s", scopeType),
			Method:      scopePath.HTTPMethod,
			API:         model.APITypeREST,
			Path:        scopePath.Path,
			PathParams:  map[string]string{"profileName": profileName},
			Body: map[string]interface{}{
				"scopeId":   scopeID,
				"scopeType": string(scopeType),
			},
			ExpectedStatus: 200,
			Validations: []model.Validation{
				{
					Type:     model.ValidationTypeStatusCode,
					Expected: 200,
				},
			},
		}
		tc.Steps = append(tc.Steps, scopeStep)
	}

	// Step 4: Target the profile
	targetPath := findPathForTargetType(targetPaths, targetType)
	if targetPath != nil {
		targetStep := model.TestStep{
			Name:        fmt.Sprintf("targetProfileTo%s", targetType),
			Description: fmt.Sprintf("Target profile to %s", targetType),
			Method:      targetPath.HTTPMethod,
			API:         model.APITypeREST,
			Path:        targetPath.Path,
			PathParams:  map[string]string{"profileName": profileName},
			Body: map[string]interface{}{
				"targetId":   scopeID,
				"targetType": string(targetType),
			},
			ExpectedStatus: 200,
			Validations: []model.Validation{
				{
					Type:     model.ValidationTypeStatusCode,
					Expected: 200,
				},
			},
		}
		tc.Steps = append(tc.Steps, targetStep)
	}

	// Step 5: Deploy the profile
	deployPath := findDeployPath(deployPaths)
	if deployPath != nil {
		deployStep := model.TestStep{
			Name:        fmt.Sprintf("deployProfileTo%s", scopeType),
			Description: fmt.Sprintf("Deploy profile to %s", scopeType),
			Method:      deployPath.HTTPMethod,
			API:         model.APITypeREST,
			Path:        deployPath.Path,
			PathParams:  map[string]string{"profileName": profileName},
			Body: map[string]interface{}{
				"deploymentMethod": string(model.DeploymentMethodImmediate),
			},
			ExpectedStatus: 202,
			Validations: []model.Validation{
				{
					Type:     model.ValidationTypeStatusCode,
					Expected: 202,
				},
			},
		}
		tc.Steps = append(tc.Steps, deployStep)
	}

	// Step 6: Check deployment status
	statusPath := findStatusPath(deployPaths)
	if statusPath != nil {
		statusStep := model.TestStep{
			Name:           "checkDeploymentStatus",
			Description:    "Verify deployment status",
			Method:         statusPath.HTTPMethod,
			API:            model.APITypeREST,
			Path:           statusPath.Path,
			PathParams:     map[string]string{"profileName": profileName},
			ExpectedStatus: 200,
			Validations: []model.Validation{
				{
					Type:     model.ValidationTypeStatusCode,
					Expected: 200,
				},
				{
					Type:     model.ValidationTypeJSONPathEquals,
					Path:     "$.status",
					Expected: "SUCCESS",
				},
			},
			Timeout: 300, // 5 minutes for deployment
		}
		tc.Steps = append(tc.Steps, statusStep)
	}

	// Step 7: Verify configuration on NOS devices via NOSAPI
	nosEndpoint := g.nosParser.FindEndpointForFeature(feature.Name, scopeType)
	if nosEndpoint != nil {
		nosStep := model.TestStep{
			Name:        fmt.Sprintf("verifyNosConfigFor%s", scopeType),
			Description: fmt.Sprintf("Verify configuration on NOS devices in %s", scopeType),
			Method:      nosEndpoint.Method,
			API:         model.APITypeNOSAPI,
			Path:        nosEndpoint.Path,
			PathParams: map[string]string{
				"siteGroupId": scopeID,
				"deviceId":    scopeID,
			},
			ExpectedStatus: 200,
			DevicesScope:   nosEndpoint.DeviceScope,
			Validations: []model.Validation{
				{
					Type:     model.ValidationTypeStatusCode,
					Expected: 200,
				},
				{
					Type:        model.ValidationTypeNosConfigMatchesExpected,
					Description: "Verify NOS device configuration matches expected state",
				},
			},
		}
		tc.Steps = append(tc.Steps, nosStep)
	}

	return tc
}

// findPathForScopeType finds a scope path matching the scope type
func findPathForScopeType(paths []*model.FeaturePath, scopeType model.ScopeType) *model.FeaturePath {
	for _, path := range paths {
		for _, st := range path.SupportedScopeTypes {
			if st == scopeType {
				return path
			}
		}
	}
	return nil
}

// findPathForTargetType finds a target path matching the target type
func findPathForTargetType(paths []*model.FeaturePath, targetType model.TargetType) *model.FeaturePath {
	for _, path := range paths {
		for _, tt := range path.SupportedTargetTypes {
			if tt == targetType {
				return path
			}
		}
	}
	return nil
}

// findDeployPath finds a deployment path
func findDeployPath(paths []*model.FeaturePath) *model.FeaturePath {
	for _, path := range paths {
		if path.OperationType == model.OperationTypeDeploy {
			return path
		}
	}
	return nil
}

// findStatusPath finds a deployment status path
func findStatusPath(paths []*model.FeaturePath) *model.FeaturePath {
	for _, path := range paths {
		if path.OperationType == model.OperationTypeStatus {
			return path
		}
	}
	return nil
}

// generateFullDeploymentTest generates a complete deployment test with all steps
func (g *Generator) generateFullDeploymentTest(
	feature *model.Feature,
	createPath, readPath *model.FeaturePath,
	scopeType model.ScopeType,
	targetType model.TargetType,
) model.TestCase {
	
	tc := model.TestCase{
		TestCaseID:       g.nextTestID(),
		FeatureName:      feature.Name,
		Priority:         model.TestPriorityP1,
		Type:             model.TestCategoryFunctional,
		Description:      fmt.Sprintf("Create %s, scope to %s, target to %s, deploy, and verify on NOS devices", feature.Name, scopeType, targetType),
		ScopeType:        scopeType,
		TargetType:       targetType,
		DeploymentMethod: model.DeploymentMethodImmediate,
		IsDeploymentTest: true,
		Steps:            []model.TestStep{},
	}

	profileName := "TestProfile"
	scopeID := "test-scope-001"

	// Step 1: Create configuration
	createBody := g.generateRequestBody(feature, createPath)
	createBody["name"] = profileName

	createStep := model.TestStep{
		Name:           fmt.Sprintf("create%s", feature.Name),
		Description:    fmt.Sprintf("Create %s configuration", feature.Name),
		Method:         createPath.HTTPMethod,
		API:            model.APITypeREST,
		Path:           createPath.Path,
		Body:           createBody,
		ExpectedStatus: 201,
		Validations: []model.Validation{
			{
				Type:     model.ValidationTypeStatusCode,
				Expected: 201,
			},
		},
	}
	tc.Steps = append(tc.Steps, createStep)

	// Step 2: Get profile to verify creation
	if readPath != nil {
		getStep := model.TestStep{
			Name:           "getProfile",
			Description:    "Retrieve created profile",
			Method:         readPath.HTTPMethod,
			API:            model.APITypeREST,
			Path:           readPath.Path,
			PathParams:     map[string]string{"profileName": profileName},
			ExpectedStatus: 200,
			Validations: []model.Validation{
				{
					Type:     model.ValidationTypeStatusCode,
					Expected: 200,
				},
			},
		}
		tc.Steps = append(tc.Steps, getStep)
	}

	// Step 3: Scope the profile (always add this step)
	scopeStep := model.TestStep{
		Name:        fmt.Sprintf("scopeProfileTo%s", scopeType),
		Description: fmt.Sprintf("Scope profile to %s", scopeType),
		Method:      "POST",
		API:         model.APITypeREST,
		Path:        fmt.Sprintf("/global-profile/%s/scope", profileName),
		PathParams:  map[string]string{"profileName": profileName},
		Body: map[string]interface{}{
			"scopeId":   scopeID,
			"scopeType": string(scopeType),
		},
		ExpectedStatus: 200,
		Validations: []model.Validation{
			{
				Type:     model.ValidationTypeStatusCode,
				Expected: 200,
			},
		},
	}
	tc.Steps = append(tc.Steps, scopeStep)

	// Step 4: Target the profile (always add this step)
	targetStep := model.TestStep{
		Name:        fmt.Sprintf("targetProfileTo%s", targetType),
		Description: fmt.Sprintf("Target profile to %s", targetType),
		Method:      "POST",
		API:         model.APITypeREST,
		Path:        fmt.Sprintf("/global-profile/%s/target", profileName),
		PathParams:  map[string]string{"profileName": profileName},
		Body: map[string]interface{}{
			"targetId":   scopeID,
			"targetType": string(targetType),
		},
		ExpectedStatus: 200,
		Validations: []model.Validation{
			{
				Type:     model.ValidationTypeStatusCode,
				Expected: 200,
			},
		},
	}
	tc.Steps = append(tc.Steps, targetStep)

	// Step 5: Deploy the profile (always add this step)
	deployStep := model.TestStep{
		Name:        fmt.Sprintf("deployProfileTo%s", scopeType),
		Description: fmt.Sprintf("Deploy profile to %s", scopeType),
		Method:      "POST",
		API:         model.APITypeREST,
		Path:        fmt.Sprintf("/global-profile/%s/deploy", profileName),
		PathParams:  map[string]string{"profileName": profileName},
		Body: map[string]interface{}{
			"deploymentMethod": string(model.DeploymentMethodImmediate),
		},
		ExpectedStatus: 202,
		Validations: []model.Validation{
			{
				Type:     model.ValidationTypeStatusCode,
				Expected: 202,
			},
		},
	}
	tc.Steps = append(tc.Steps, deployStep)

	// Step 6: Check deployment status (always add this step)
	statusStep := model.TestStep{
		Name:           "checkDeploymentStatus",
		Description:    "Verify deployment status",
		Method:         "GET",
		API:            model.APITypeREST,
		Path:           fmt.Sprintf("/global-profile/%s/deployment/status", profileName),
		PathParams:     map[string]string{"profileName": profileName},
		ExpectedStatus: 200,
		Validations: []model.Validation{
			{
				Type:     model.ValidationTypeStatusCode,
				Expected: 200,
			},
			{
				Type:     model.ValidationTypeJSONPathEquals,
				Path:     "$.status",
				Expected: "SUCCESS",
			},
		},
		Timeout: 300,
	}
	tc.Steps = append(tc.Steps, statusStep)

	// Step 7: Verify configuration on NOS devices via NOSAPI
	nosEndpoint := g.nosParser.FindEndpointForFeature(feature.Name, scopeType)
	if nosEndpoint != nil {
		nosStep := model.TestStep{
			Name:        fmt.Sprintf("verifyNosConfigFor%s", scopeType),
			Description: fmt.Sprintf("Verify configuration on NOS devices in %s", scopeType),
			Method:      nosEndpoint.Method,
			API:         model.APITypeNOSAPI,
			Path:        nosEndpoint.Path,
			PathParams: map[string]string{
				"siteGroupId": scopeID,
				"deviceId":    scopeID,
			},
			ExpectedStatus: 200,
			DevicesScope:   nosEndpoint.DeviceScope,
			Validations: []model.Validation{
				{
					Type:     model.ValidationTypeStatusCode,
					Expected: 200,
				},
				{
					Type:        model.ValidationTypeNosConfigMatchesExpected,
					Description: "Verify NOS device configuration matches expected state",
				},
			},
		}
		tc.Steps = append(tc.Steps, nosStep)
	}

	return tc
}

// generateSimplifiedDeploymentTest generates a simplified deployment test (create and verify only)
func (g *Generator) generateSimplifiedDeploymentTest(
	feature *model.Feature,
	createPath *model.FeaturePath,
	scopeType model.ScopeType,
) model.TestCase {
	
	tc := model.TestCase{
		TestCaseID:       g.nextTestID(),
		FeatureName:      feature.Name,
		Priority:         model.TestPriorityP2,
		Type:             model.TestCategoryFunctional,
		Description:      fmt.Sprintf("Create %s and verify configuration on NOS devices (without explicit scope/target/deploy)", feature.Name),
		ScopeType:        scopeType,
		DeploymentMethod: model.DeploymentMethodImmediate,
		IsDeploymentTest: true,
		Steps:            []model.TestStep{},
	}

	profileName := "TestProfile-Simple"
	scopeID := "test-scope-001"

	// Step 1: Create configuration
	createBody := g.generateRequestBody(feature, createPath)
	createBody["name"] = profileName

	createStep := model.TestStep{
		Name:           fmt.Sprintf("create%s", feature.Name),
		Description:    fmt.Sprintf("Create %s configuration", feature.Name),
		Method:         createPath.HTTPMethod,
		API:            model.APITypeREST,
		Path:           createPath.Path,
		Body:           createBody,
		ExpectedStatus: 201,
		Validations: []model.Validation{
			{
				Type:     model.ValidationTypeStatusCode,
				Expected: 201,
			},
		},
	}
	tc.Steps = append(tc.Steps, createStep)

	// Step 2: Verify configuration on NOS devices via NOSAPI
	nosEndpoint := g.nosParser.FindEndpointForFeature(feature.Name, scopeType)
	if nosEndpoint != nil {
		nosStep := model.TestStep{
			Name:        fmt.Sprintf("verifyNosConfigFor%s", scopeType),
			Description: fmt.Sprintf("Verify configuration on NOS devices in %s", scopeType),
			Method:      nosEndpoint.Method,
			API:         model.APITypeNOSAPI,
			Path:        nosEndpoint.Path,
			PathParams: map[string]string{
				"siteGroupId": scopeID,
				"deviceId":    scopeID,
			},
			ExpectedStatus: 200,
			DevicesScope:   nosEndpoint.DeviceScope,
			Validations: []model.Validation{
				{
					Type:     model.ValidationTypeStatusCode,
					Expected: 200,
				},
				{
					Type:        model.ValidationTypeNosConfigMatchesExpected,
					Description: "Verify NOS device configuration matches expected state",
				},
			},
		}
		tc.Steps = append(tc.Steps, nosStep)
	}

	return tc
}
