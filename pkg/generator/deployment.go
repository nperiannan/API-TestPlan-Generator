package generator

import (
	"fmt"
	"strings"

	"github.com/extremenetworks/testcase-generator/pkg/model"
)

const (
	locationLookupPath       = "/authorized/sites-sitegroups/retrieve"
	profileScopePath         = "/configuration-profile/{name}/scope"
	profileTargetQueryPath   = "/configuration-profile/{name}/target-query/profile"
	deviceDeployPath         = "/configuration-profile/{name}/devices/deploy"
	siteDeployPath           = "/configuration-profile/{name}/sites/deploy"
	deviceDeployStatusPath   = "/configuration-profile/{name}/device/{hostName}/deploy/status"
	siteDeployStatusPath     = "/configuration-profile/{name}/site/{siteName}/deploy/status"
	deviceConflictCheckPath  = "/configuration-profile/{name}/device/{hostName}/conflicts"
	serviceProfileCreatePath = "/service-profile/{name}"
	configurationProfilePath = "/configuration-profile/{name}"
	globalProfilePath        = "/global-profile/{name}"

	locationSiteGroupID   = "{{SITE_GROUP_ID}}"
	locationSiteGroupName = "{{SITE_GROUP_NAME}}"
	locationSiteID        = "{{SITE_ID}}"
	locationSiteName      = "{{SITE_NAME}}"
	locationDeviceID      = "{{DEVICE_ID}}"
	locationDeviceName    = "{{DEVICE_HOSTNAME}}"
)

// wrapDeploymentProfileSetup inserts the profile-type-specific setup steps around
// the existing steps in tc and returns the deployment (configuration) profile name
// that should be used for scope / target-query / deploy operations.
//
//   - Configuration-profile features: no extra setup needed → returns profileName as-is.
//   - Service-profile features: prepends a service-profile container step, appends a
//     configuration-profile-with-service-profile step.
//   - Global-profile features: appends a configuration-profile-with-global-profile step.
func (g *Generator) wrapDeploymentProfileSetup(tc *model.TestCase, feature *model.Feature, createPath *model.FeaturePath, profileName string) string {
	if isServiceProfileDeploymentPath(createPath) {
		serviceProfileName := "SvcProf-" + profileName
		configProfileName := "CfgProf-" + profileName
		// Prepend: create the service-profile container before the feature-create step.
		tc.Steps = append([]model.TestStep{serviceProfileContainerStep(feature, serviceProfileName)}, tc.Steps...)
		// Append: create a configuration profile that links the service profile.
		tc.Steps = append(tc.Steps, configurationProfileWithServiceProfileStep(feature, configProfileName, serviceProfileName))
		return configProfileName
	}
	if isGlobalProfileDeploymentPath(createPath) {
		configProfileName := "CfgProf-" + profileName
		// Append: create a configuration profile that references the global profile.
		tc.Steps = append(tc.Steps, configurationProfileWithGlobalProfileStep(feature, configProfileName, profileName))
		return configProfileName
	}
	return profileName
}

// addDeploymentSteps adds the profile scope, target-query, deployment, status,
// and NOS verification workflow required before a configuration reaches devices.
func (g *Generator) addDeploymentSteps(tc *model.TestCase, feature *model.Feature, profileName string, scopeType model.ScopeType, targetType model.TargetType) {
	tc.Steps = append(tc.Steps, scopedTargetQuerySteps(profileName, scopeType)...)
	g.appendDeploymentExecutionSteps(tc, feature, profileName, targetType, true)
}

func scopedTargetQuerySteps(profileName string, scopeType model.ScopeType) []model.TestStep {
	return []model.TestStep{
		{
			Name:           "retrieveDeviceLocation",
			Description:    "Retrieve authorized site groups and sites, then capture the site group and site where the onboarded device is located",
			Method:         "POST",
			API:            model.APITypeREST,
			Path:           locationLookupPath,
			Body:           map[string]interface{}{},
			ExpectedStatus: 200,
			Validations: []model.Validation{
				{Type: model.ValidationTypeStatusCode, Expected: 200},
				{Type: model.ValidationTypeJSONPathExists, Path: "$.siteGroups[0].siteGroupId", CaptureAs: "SITE_GROUP_ID", Description: "Capture siteGroupId for the device location"},
				{Type: model.ValidationTypeJSONPathExists, Path: "$.siteGroups[0].siteGroupName", CaptureAs: "SITE_GROUP_NAME", Description: "Capture siteGroupName for the device location"},
				{Type: model.ValidationTypeJSONPathExists, Path: "$.siteGroups[0].sites[0].siteId", CaptureAs: "SITE_ID", Description: "Capture siteId where the device is onboarded"},
				{Type: model.ValidationTypeJSONPathExists, Path: "$.siteGroups[0].sites[0].siteName", CaptureAs: "SITE_NAME", Description: "Capture siteName where the device is onboarded"},
			},
		},
		{
			Name:        fmt.Sprintf("scopeProfileTo%sLocation", scopeType),
			Description: fmt.Sprintf("Scope configuration profile to the selected %s location using the captured site group and site", scopeType),
			Method:      "POST",
			API:         model.APITypeREST,
			Path:        profileScopePath,
			PathParams:  map[string]string{"name": profileName},
			Body: map[string]interface{}{
				"selectedSiteGroups": []map[string]interface{}{
					{
						"siteGroupId":   locationSiteGroupID,
						"siteGroupName": locationSiteGroupName,
						"sites": []map[string]interface{}{
							{
								"siteId":   locationSiteID,
								"siteName": locationSiteName,
							},
						},
					},
				},
				"selectedSites": []map[string]interface{}{},
			},
			ExpectedStatus: 201,
			Validations: []model.Validation{
				{Type: model.ValidationTypeStatusCode, Expected: 201},
				{Type: model.ValidationTypeJSONPathExists, Path: "$.scopeId", CaptureAs: "SCOPE_ID", Description: "Capture created target-query scope id"},
			},
		},
		{
			Name:        "applySiteTargetQuery",
			Description: "Apply target-query to the configuration profile using the captured siteId",
			Method:      "POST",
			API:         model.APITypeREST,
			Path:        profileTargetQueryPath,
			PathParams:  map[string]string{"name": profileName},
			Body: map[string]interface{}{
				"includeQuery": map[string]interface{}{
					"criteria": []map[string]interface{}{
						{
							"field":    "siteId",
							"operator": "IN",
							"values":   []string{locationSiteID},
						},
					},
					"logicalOperator": "AND",
				},
			},
			ExpectedStatus: 200,
			Validations: []model.Validation{
				{Type: model.ValidationTypeStatusCode, Expected: 200},
				{Type: model.ValidationTypeJSONPathExists, Path: "$.resolvedDevices[0].deviceId", CaptureAs: "DEVICE_ID", Description: "Capture resolved device id from the scoped target-query"},
				{Type: model.ValidationTypeJSONPathExists, Path: "$.resolvedDevices[0].deviceName", CaptureAs: "DEVICE_HOSTNAME", Description: "Capture resolved device hostname from the scoped target-query"},
			},
		},
	}
}

func (g *Generator) appendDeploymentExecutionSteps(tc *model.TestCase, feature *model.Feature, profileName string, targetType model.TargetType, verifyNoConflict bool) {
	if verifyNoConflict {
		tc.Steps = append(tc.Steps, noConflictBeforeDeploymentStep(profileName))
	}
	tc.Steps = append(tc.Steps, deploymentRequestStep(profileName, targetType))
	tc.Steps = append(tc.Steps, deploymentStatusStep(profileName, targetType))
	g.appendNOSVerificationStep(tc, feature, targetType)
}

func noConflictBeforeDeploymentStep(profileName string) model.TestStep {
	return model.TestStep{
		Name:        "verifyNoConflictBeforeDeployment",
		Description: "Validate that no device conflict is detected before deployment is initiated",
		Method:      "GET",
		API:         model.APITypeREST,
		Path:        deviceConflictCheckPath,
		PathParams: map[string]string{
			"name":     profileName,
			"hostName": locationDeviceName,
		},
		QueryParams:    map[string]string{"detailLevel": "summary"},
		ExpectedStatus: 200,
		Validations: []model.Validation{
			{Type: model.ValidationTypeStatusCode, Expected: 200},
			{Type: model.ValidationTypeJSONPathEquals, Path: "$.hasConflicts", Expected: false, Description: "Deployment must start only when no conflict is detected"},
		},
	}
}

func deploymentRequestStep(profileName string, targetType model.TargetType) model.TestStep {
	deployPath := deviceDeployPath
	deployBody := map[string]interface{}{"devices": []string{locationDeviceName}, "deployNow": true}
	if targetType == model.TargetTypeSite || targetType == model.TargetTypeSiteGroup {
		deployPath = siteDeployPath
		deployBody = map[string]interface{}{"sites": []string{locationSiteName}, "deployNow": true}
	}

	return model.TestStep{
		Name:           fmt.Sprintf("deployProfileTo%s", targetType),
		Description:    fmt.Sprintf("Initiate configuration-profile deployment to %s using the deployment API", targetType),
		Method:         "POST",
		API:            model.APITypeREST,
		Path:           deployPath,
		PathParams:     map[string]string{"name": profileName},
		Body:           deployBody,
		ExpectedStatus: 202,
		Validations: []model.Validation{
			{Type: model.ValidationTypeStatusCode, Expected: 202},
		},
	}
}

func deploymentStatusStep(profileName string, targetType model.TargetType) model.TestStep {
	statusPath := deviceDeployStatusPath
	pathParams := map[string]string{"name": profileName, "hostName": locationDeviceName}
	if targetType == model.TargetTypeSite || targetType == model.TargetTypeSiteGroup {
		statusPath = siteDeployStatusPath
		pathParams = map[string]string{"name": profileName, "siteName": locationSiteName}
	}

	return model.TestStep{
		Name:           "checkDeploymentStatus",
		Description:    "Check deployment status and verify the deployment succeeded",
		Method:         "GET",
		API:            model.APITypeREST,
		Path:           statusPath,
		PathParams:     pathParams,
		ExpectedStatus: 200,
		Validations: []model.Validation{
			{Type: model.ValidationTypeStatusCode, Expected: 200},
			{Type: model.ValidationTypeJSONPathEquals, Path: "$.status", Expected: "SUCCESS"},
		},
		Timeout: 300,
	}
}

func conflictBlocksDeploymentStep(profileName string) model.TestStep {
	return model.TestStep{
		Name:        "verifyDeploymentBlockedByConflict",
		Description: "Attempt deployment while conflict is unresolved; deployment must not succeed until conflict resolution is selected",
		Method:      "POST",
		API:         model.APITypeREST,
		Path:        deviceDeployPath,
		PathParams:  map[string]string{"name": profileName},
		Body: map[string]interface{}{
			"devices":   []string{locationDeviceName},
			"deployNow": true,
		},
		ExpectedStatus: 409,
		Validations: []model.Validation{
			{Type: model.ValidationTypeStatusCode, Expected: 409, Description: "Verify deployment is rejected while device conflict exists"},
		},
	}
}

func (g *Generator) appendNOSVerificationStep(tc *model.TestCase, feature *model.Feature, targetType model.TargetType) {
	if g.nosParser != nil {
		nosScopeType := model.ScopeType(targetType)
		nosEndpoint := g.nosParser.FindEndpointForFeature(feature.Name, nosScopeType)
		if nosEndpoint != nil {
			nosStep := model.TestStep{
				Name:        fmt.Sprintf("verifyNosConfigFor%s", targetType),
				Description: fmt.Sprintf("Verify the deployed %s configuration is present on the NOS device", feature.Name),
				Method:      nosEndpoint.Method,
				API:         model.APITypeNOSAPI,
				Path:        nosEndpoint.Path,
				PathParams: map[string]string{
					"siteGroupId": locationSiteGroupID,
					"siteId":      locationSiteID,
					"deviceId":    locationDeviceID,
					"hostName":    locationDeviceName,
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
	// Step 1: Create configuration using the feature's actual body structure
	createBody := g.generateRequestBody(feature, createPath)
	g.setBodyResourceName(feature, createBody, profileName)

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
			PathParams:     pathParamsFor(readPath.Path, profileName),
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

	g.addDeploymentSteps(&tc, feature, profileName, scopeType, targetType)

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

func findDeployPathForTargetType(paths []*model.FeaturePath, targetType model.TargetType) *model.FeaturePath {
	for _, path := range paths {
		if path.OperationType != model.OperationTypeDeploy {
			continue
		}
		for _, supported := range path.SupportedTargetTypes {
			if supported == targetType || (targetType == model.TargetTypeSiteGroup && supported == model.TargetTypeSite) {
				return path
			}
		}
		pathLower := strings.ToLower(path.Path)
		if targetType == model.TargetTypeDevice && strings.Contains(pathLower, "/devices/") {
			return path
		}
		if (targetType == model.TargetTypeSite || targetType == model.TargetTypeSiteGroup) && strings.Contains(pathLower, "/sites/") {
			return path
		}
	}
	return findDeployPath(paths)
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

func findStatusPathForTargetType(paths []*model.FeaturePath, targetType model.TargetType) *model.FeaturePath {
	for _, path := range paths {
		if path.OperationType != model.OperationTypeStatus {
			continue
		}
		pathLower := strings.ToLower(path.Path)
		if targetType == model.TargetTypeDevice && strings.Contains(pathLower, "/device/") {
			return path
		}
		if (targetType == model.TargetTypeSite || targetType == model.TargetTypeSiteGroup) && strings.Contains(pathLower, "/site/") {
			return path
		}
	}
	return findStatusPath(paths)
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

	// Step 1: Create configuration
	createBody := g.generateRequestBody(feature, createPath)
	g.setBodyResourceName(feature, createBody, profileName)

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

	g.addDeploymentSteps(&tc, feature, profileName, scopeType, targetType)

	return tc
}

func (g *Generator) generateServiceProfileScopedDeploymentTest(feature *model.Feature, createPath, readPath *model.FeaturePath, targetType model.TargetType) model.TestCase {
	serviceProfileName := fmt.Sprintf("TestServiceProfile-%s", feature.Name)
	configurationProfileName := fmt.Sprintf("TestConfigProfile-%s", feature.Name)

	tc := model.TestCase{
		TestCaseID:       g.nextTestID(),
		FeatureName:      feature.Name,
		Priority:         model.TestPriorityP1,
		Type:             model.TestCategoryFunctional,
		Description:      fmt.Sprintf("Create %s in a service profile, link it to a configuration profile, scope by device location, apply target-query, deploy, and verify", feature.Name),
		ScopeType:        model.ScopeTypeDevice,
		TargetType:       targetType,
		DeploymentMethod: model.DeploymentMethodImmediate,
		IsDeploymentTest: true,
		Steps:            []model.TestStep{},
	}

	g.appendServiceProfileConfigurationSetup(&tc, feature, createPath, readPath, serviceProfileName, configurationProfileName)
	g.addDeploymentSteps(&tc, feature, configurationProfileName, model.ScopeTypeDevice, targetType)

	return tc
}

func (g *Generator) appendServiceProfileConfigurationSetup(tc *model.TestCase, feature *model.Feature, createPath, readPath *model.FeaturePath, serviceProfileName, configurationProfileName string) {
	tc.Steps = append(tc.Steps, serviceProfileContainerStep(feature, serviceProfileName))

	tc.Steps = append(tc.Steps, g.createBasicCreateStep(feature, createPath, serviceProfileName))

	if readPath != nil {
		tc.Steps = append(tc.Steps, model.TestStep{
			Name:           "verifyServiceProfileFeature",
			Description:    fmt.Sprintf("Verify %s is present in the service profile before linking it to configuration profile", feature.Name),
			Method:         readPath.HTTPMethod,
			API:            model.APITypeREST,
			Path:           readPath.Path,
			PathParams:     pathParamsFor(readPath.Path, serviceProfileName),
			Body:           g.generateReadBody(feature, createPath),
			ExpectedStatus: 200,
			Validations:    []model.Validation{{Type: model.ValidationTypeStatusCode, Expected: 200}},
		})
	}

	tc.Steps = append(tc.Steps, configurationProfileWithServiceProfileStep(feature, configurationProfileName, serviceProfileName))

}

// generateGlobalProfileDeploymentTest generates a deployment test for a global-profile feature.
// Global-profile features are deployed through a configuration profile that references
// the global profile, similar to how service-profile features are deployed.
func (g *Generator) generateGlobalProfileDeploymentTest(feature *model.Feature, createPath, readPath *model.FeaturePath, targetType model.TargetType) model.TestCase {
	globalProfileName := fmt.Sprintf("TestGlobalProfile-%s", feature.Name)
	configurationProfileName := fmt.Sprintf("TestConfigProfile-%s", feature.Name)

	tc := model.TestCase{
		TestCaseID:       g.nextTestID(),
		FeatureName:      feature.Name,
		Priority:         model.TestPriorityP1,
		Type:             model.TestCategoryFunctional,
		Description:      fmt.Sprintf("Create %s in a global profile, reference it from a configuration profile, scope by device location, deploy to %s, and verify on NOS devices", feature.Name, targetType),
		ScopeType:        model.ScopeTypeDevice,
		TargetType:       targetType,
		DeploymentMethod: model.DeploymentMethodImmediate,
		IsDeploymentTest: true,
		Steps:            []model.TestStep{},
	}

	g.appendGlobalProfileConfigurationSetup(&tc, feature, createPath, readPath, globalProfileName, configurationProfileName)
	g.addDeploymentSteps(&tc, feature, configurationProfileName, model.ScopeTypeDevice, targetType)

	return tc
}

// appendGlobalProfileConfigurationSetup adds steps to create a global-profile feature,
// verify it, then create a configuration profile that references the global profile.
func (g *Generator) appendGlobalProfileConfigurationSetup(tc *model.TestCase, feature *model.Feature, createPath, readPath *model.FeaturePath, globalProfileName, configurationProfileName string) {
	// Step 1: Create the feature inside the global profile
	tc.Steps = append(tc.Steps, g.createBasicCreateStep(feature, createPath, globalProfileName))

	// Step 2: Verify the feature was created in the global profile
	if readPath != nil {
		tc.Steps = append(tc.Steps, model.TestStep{
			Name:           "verifyGlobalProfileFeature",
			Description:    fmt.Sprintf("Verify %s is present in the global profile before linking it to a configuration profile", feature.Name),
			Method:         readPath.HTTPMethod,
			API:            model.APITypeREST,
			Path:           readPath.Path,
			PathParams:     pathParamsFor(readPath.Path, globalProfileName),
			Body:           g.generateReadBody(feature, createPath),
			ExpectedStatus: 200,
			Validations:    []model.Validation{{Type: model.ValidationTypeStatusCode, Expected: 200}},
		})
	}

	// Step 3: Create configuration profile referencing the global profile
	tc.Steps = append(tc.Steps, configurationProfileWithGlobalProfileStep(feature, configurationProfileName, globalProfileName))
}

func configurationProfileWithGlobalProfileStep(feature *model.Feature, configurationProfileName, globalProfileName string) model.TestStep {
	return model.TestStep{
		Name:        "createConfigurationProfileWithGlobalProfile",
		Description: "Create configuration profile and reference the global profile so its configuration can be deployed",
		Method:      "POST",
		API:         model.APITypeREST,
		Path:        configurationProfilePath,
		PathParams:  map[string]string{"name": configurationProfileName},
		Body: map[string]interface{}{
			"blueprints":          []string{"wired-blueprint"},
			"globalProfiles":      []string{globalProfileName},
			"networkArchitecture": "standard",
			"description":         fmt.Sprintf("Configuration profile for %s global-profile deployment", feature.Name),
		},
		ExpectedStatus: 201,
		Validations:    []model.Validation{{Type: model.ValidationTypeStatusCode, Expected: 201}},
	}
}

// generateSimplifiedDeploymentTest generates a compact scoped deployment test.
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
		Description:      fmt.Sprintf("Create %s, scope configuration profile by device location, apply target-query, deploy, and verify on NOS devices", feature.Name),
		ScopeType:        scopeType,
		TargetType:       model.TargetTypeDevice,
		DeploymentMethod: model.DeploymentMethodImmediate,
		IsDeploymentTest: true,
		Steps:            []model.TestStep{},
	}

	profileName := "TestProfile-Simple"

	// Step 1: Create configuration
	createBody := g.generateRequestBody(feature, createPath)
	g.setBodyResourceName(feature, createBody, profileName)

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

	g.addDeploymentSteps(&tc, feature, profileName, scopeType, model.TargetTypeDevice)

	return tc
}
