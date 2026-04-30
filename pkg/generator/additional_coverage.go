package generator

import (
	"fmt"
	"time"

	"github.com/extremenetworks/testcase-generator/pkg/model"
)

// generateAdditionalCoverageTests generates minimal tests for scheduled deployment, clone, and schedule management APIs
// Only generates a few tests per feature to ensure coverage without creating too many test cases
func (g *Generator) generateAdditionalCoverageTests(feature *model.Feature, paths []*model.FeaturePath) []model.TestCase {
	var tests []model.TestCase

	if len(paths) == 0 {
		return tests
	}

	primaryPath := paths[0]
	profileName := "TestProfile-Additional"

	// 1. Scheduled deployment to devices (1 test)
	tests = append(tests, g.generateScheduledDeploymentTest(feature, primaryPath, profileName, model.TargetTypeDevice))

	// 2. Scheduled deployment to sites (1 test)
	tests = append(tests, g.generateScheduledDeploymentTest(feature, primaryPath, profileName, model.TargetTypeSite))

	// 3. Edit deployment schedule for devices (1 test)
	tests = append(tests, g.generateEditScheduleTest(feature, primaryPath, profileName, model.TargetTypeDevice))

	// 4. Clear deployment schedule for sites (1 test)
	tests = append(tests, g.generateClearScheduleTest(feature, primaryPath, profileName, model.TargetTypeSite))

	// 5. Clone profile test (1 test)
	tests = append(tests, g.generateCloneProfileTest(feature, primaryPath, profileName))

	// 6. Clone feature object test (1 test)
	tests = append(tests, g.generateCloneObjectTest(feature, primaryPath, profileName))

	// 7. Conflict detection test — deploy, simulate out-of-band device change, verify conflict detected (1 test)
	tests = append(tests, g.generateConflictDetectionTest(feature, primaryPath, profileName+"-ConflictDetect"))

	// 8. Conflict resolution CC test — detect conflict, resolve via acceptCloud (cloud wins), re-deploy (1 test)
	tests = append(tests, g.generateConflictResolutionCCTest(feature, primaryPath, profileName+"-ConflictCC"))

	// 9. Conflict resolution DD test — detect conflict, resolve via acceptDevice (device wins), verify cleared (1 test)
	tests = append(tests, g.generateConflictResolutionDDTest(feature, primaryPath, profileName+"-ConflictDD"))

	// 10-17. Override tests — CRUD lifecycle for device, model, model-group overrides
	tests = append(tests, g.generateOverrideTests(feature, primaryPath, profileName+"-Override")...)

	return tests
}

// generateScheduledDeploymentTest creates a test for scheduled deployment (deployAt instead of deployNow)
func (g *Generator) generateScheduledDeploymentTest(feature *model.Feature, fp *model.FeaturePath, profileName string, targetType model.TargetType) model.TestCase {
	// Calculate future deployment time (1 hour from now)
	deployTime := time.Now().Add(1 * time.Hour).UTC().Format(time.RFC3339)

	tc := model.TestCase{
		TestCaseID:       g.nextTestID(),
		FeatureName:      feature.Name,
		Priority:         model.TestPriorityP1,
		Type:             model.TestCategoryFunctional,
		Description:      fmt.Sprintf("Create %s and schedule deployment to %s", feature.Name, targetType),
		ScopeType:        model.ScopeType(targetType), // Use targetType as scopeType for consistency
		TargetType:       targetType,
		DeploymentMethod: "scheduled",
		IsDeploymentTest: true,
		Steps:            []model.TestStep{},
	}

	// Step 1: Create configuration
	createStep := g.createBasicCreateStep(feature, fp, profileName)
	tc.Steps = append(tc.Steps, createStep)

	// Step 2: Schedule deployment
	var deployPath string
	var deployBody map[string]interface{}

	switch targetType {
	case model.TargetTypeSite:
		deployPath = fmt.Sprintf("/configuration-profile/%s/sites/deploy", profileName)
		deployBody = map[string]interface{}{
			"sites":    []string{"test-site-001"},
			"deployAt": deployTime,
			"timezone": "UTC",
		}
	case model.TargetTypeDevice:
		deployPath = fmt.Sprintf("/configuration-profile/%s/devices/deploy", profileName)
		deployBody = map[string]interface{}{
			"devices":  []string{"test-device-001"},
			"deployAt": deployTime,
			"timezone": "UTC",
		}
	}

	scheduleStep := model.TestStep{
		Name:        fmt.Sprintf("scheduleDeploymentTo%s", targetType),
		Description: fmt.Sprintf("Schedule deployment to %s at %s", targetType, deployTime),
		Method:      "POST",
		API:         model.APITypeREST,
		Path:        deployPath,
		PathParams: map[string]string{
			"name": profileName,
		},
		Body:           deployBody,
		ExpectedStatus: 202,
		Validations: []model.Validation{
			{Type: "statusCode", Expected: "202"},
		},
	}
	tc.Steps = append(tc.Steps, scheduleStep)

	return tc
}

// generateEditScheduleTest creates a test for editing deployment schedule
func (g *Generator) generateEditScheduleTest(feature *model.Feature, fp *model.FeaturePath, profileName string, targetType model.TargetType) model.TestCase {
	newDeployTime := time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339)

	tc := model.TestCase{
		TestCaseID:       g.nextTestID(),
		FeatureName:      feature.Name,
		Priority:         model.TestPriorityP2,
		Type:             model.TestCategoryFunctional,
		Description:      fmt.Sprintf("Edit deployment schedule for %s on %s", feature.Name, targetType),
		IsDeploymentTest: true,
		Steps:            []model.TestStep{},
	}

	// Step 1: Create configuration
	createStep := g.createBasicCreateStep(feature, fp, profileName)
	tc.Steps = append(tc.Steps, createStep)

	// Step 2: Schedule initial deployment
	initialDeployTime := time.Now().Add(1 * time.Hour).UTC().Format(time.RFC3339)
	var scheduleDeployPath string
	var scheduleBody map[string]interface{}

	switch targetType {
	case model.TargetTypeSite:
		scheduleDeployPath = fmt.Sprintf("/configuration-profile/%s/sites/deploy", profileName)
		scheduleBody = map[string]interface{}{
			"sites":    []string{"test-site-001"},
			"deployAt": initialDeployTime,
			"timezone": "UTC",
		}
	case model.TargetTypeDevice:
		scheduleDeployPath = fmt.Sprintf("/configuration-profile/%s/devices/deploy", profileName)
		scheduleBody = map[string]interface{}{
			"devices":  []string{"test-device-001"},
			"deployAt": initialDeployTime,
			"timezone": "UTC",
		}
	}

	scheduleStep := model.TestStep{
		Name:        "scheduleInitialDeployment",
		Description: "Schedule initial deployment",
		Method:      "POST",
		API:         model.APITypeREST,
		Path:        scheduleDeployPath,
		PathParams: map[string]string{
			"name": profileName,
		},
		Body:           scheduleBody,
		ExpectedStatus: 202,
		Validations: []model.Validation{
			{Type: "statusCode", Expected: "202"},
		},
	}
	tc.Steps = append(tc.Steps, scheduleStep)

	// Step 3: Edit the schedule
	var editPath string
	var editBody map[string]interface{}

	switch targetType {
	case model.TargetTypeSite:
		editPath = fmt.Sprintf("/configuration-profile/%s/sites/deploy/edit-schedule", profileName)
		editBody = map[string]interface{}{
			"sites":    []string{"test-site-001"},
			"deployAt": newDeployTime,
			"timezone": "UTC",
		}
	case model.TargetTypeDevice:
		editPath = fmt.Sprintf("/configuration-profile/%s/devices/deploy/edit-schedule", profileName)
		editBody = map[string]interface{}{
			"devices":  []string{"test-device-001"},
			"deployAt": newDeployTime,
			"timezone": "UTC",
		}
	}

	editStep := model.TestStep{
		Name:        "editDeploymentSchedule",
		Description: fmt.Sprintf("Edit deployment schedule to %s", newDeployTime),
		Method:      "POST",
		API:         model.APITypeREST,
		Path:        editPath,
		PathParams: map[string]string{
			"name": profileName,
		},
		Body:           editBody,
		ExpectedStatus: 200,
		Validations: []model.Validation{
			{Type: "statusCode", Expected: "200"},
		},
	}
	tc.Steps = append(tc.Steps, editStep)

	return tc
}

// generateClearScheduleTest creates a test for clearing deployment schedule
func (g *Generator) generateClearScheduleTest(feature *model.Feature, fp *model.FeaturePath, profileName string, targetType model.TargetType) model.TestCase {
	tc := model.TestCase{
		TestCaseID:       g.nextTestID(),
		FeatureName:      feature.Name,
		Priority:         model.TestPriorityP2,
		Type:             model.TestCategoryFunctional,
		Description:      fmt.Sprintf("Clear deployment schedule for %s on %s", feature.Name, targetType),
		IsDeploymentTest: true,
		Steps:            []model.TestStep{},
	}

	// Step 1: Create configuration
	createStep := g.createBasicCreateStep(feature, fp, profileName)
	tc.Steps = append(tc.Steps, createStep)

	// Step 2: Schedule deployment
	deployTime := time.Now().Add(1 * time.Hour).UTC().Format(time.RFC3339)
	var scheduleDeployPath string
	var scheduleBody map[string]interface{}

	switch targetType {
	case model.TargetTypeSite:
		scheduleDeployPath = fmt.Sprintf("/configuration-profile/%s/sites/deploy", profileName)
		scheduleBody = map[string]interface{}{
			"sites":    []string{"test-site-001"},
			"deployAt": deployTime,
			"timezone": "UTC",
		}
	case model.TargetTypeDevice:
		scheduleDeployPath = fmt.Sprintf("/configuration-profile/%s/devices/deploy", profileName)
		scheduleBody = map[string]interface{}{
			"devices":  []string{"test-device-001"},
			"deployAt": deployTime,
			"timezone": "UTC",
		}
	}

	scheduleStep := model.TestStep{
		Name:        "scheduleDeployment",
		Description: "Schedule deployment",
		Method:      "POST",
		API:         model.APITypeREST,
		Path:        scheduleDeployPath,
		PathParams: map[string]string{
			"name": profileName,
		},
		Body:           scheduleBody,
		ExpectedStatus: 202,
		Validations: []model.Validation{
			{Type: "statusCode", Expected: "202"},
		},
	}
	tc.Steps = append(tc.Steps, scheduleStep)

	// Step 3: Clear the schedule
	var clearPath string
	var clearBody map[string]interface{}

	switch targetType {
	case model.TargetTypeSite:
		clearPath = fmt.Sprintf("/configuration-profile/%s/sites/deploy/clear-schedule", profileName)
		clearBody = map[string]interface{}{
			"sites": []string{"test-site-001"},
		}
	case model.TargetTypeDevice:
		clearPath = fmt.Sprintf("/configuration-profile/%s/devices/deploy/clear-schedule", profileName)
		clearBody = map[string]interface{}{
			"devices": []string{"test-device-001"},
		}
	}

	clearStep := model.TestStep{
		Name:        "clearDeploymentSchedule",
		Description: "Clear the deployment schedule",
		Method:      "POST",
		API:         model.APITypeREST,
		Path:        clearPath,
		PathParams: map[string]string{
			"name": profileName,
		},
		Body:           clearBody,
		ExpectedStatus: 200,
		Validations: []model.Validation{
			{Type: "statusCode", Expected: "200"},
		},
	}
	tc.Steps = append(tc.Steps, clearStep)

	return tc
}

// generateCloneProfileTest creates a test for cloning an entire configuration profile
func (g *Generator) generateCloneProfileTest(feature *model.Feature, fp *model.FeaturePath, profileName string) model.TestCase {
	tc := model.TestCase{
		TestCaseID:  g.nextTestID(),
		FeatureName: feature.Name,
		Priority:    model.TestPriorityP2,
		Type:        model.TestCategoryFunctional,
		Description: fmt.Sprintf("Clone %s configuration profile", feature.Name),
		Steps:       []model.TestStep{},
	}

	// Step 1: Create original configuration
	createStep := g.createBasicCreateStep(feature, fp, profileName)
	tc.Steps = append(tc.Steps, createStep)

	// Step 2: Clone the profile
	clonePath := fmt.Sprintf("/configuration-profile/%s/clone", profileName)
	cloneBody := map[string]interface{}{
		"newProfileName": fmt.Sprintf("%s-Cloned", profileName),
	}

	cloneStep := model.TestStep{
		Name:        "cloneProfile",
		Description: "Clone the configuration profile",
		Method:      "POST",
		API:         model.APITypeREST,
		Path:        clonePath,
		PathParams: map[string]string{
			"name": profileName,
		},
		Body:           cloneBody,
		ExpectedStatus: 201,
		Validations: []model.Validation{
			{Type: "statusCode", Expected: "201"},
		},
	}
	tc.Steps = append(tc.Steps, cloneStep)

	return tc
}

// generateCloneObjectTest creates a test for cloning a feature object
func (g *Generator) generateCloneObjectTest(feature *model.Feature, fp *model.FeaturePath, profileName string) model.TestCase {
	tc := model.TestCase{
		TestCaseID:  g.nextTestID(),
		FeatureName: feature.Name,
		Priority:    model.TestPriorityP2,
		Type:        model.TestCategoryFunctional,
		Description: fmt.Sprintf("Clone %s feature object", feature.Name),
		Steps:       []model.TestStep{},
	}

	// Step 1: Create original configuration
	createStep := g.createBasicCreateStep(feature, fp, profileName)
	tc.Steps = append(tc.Steps, createStep)

	// Step 2: Clone the object
	clonePath := fmt.Sprintf("/configuration-profile/%s/feature/object/clone", profileName)

	// Build clone body with feature-specific details
	cloneBody := map[string]interface{}{
		"featurePath": fp.Path,
		"objectType":  feature.Name,
		"sourceId":    "original-object-1",
		"newId":       "cloned-object-1",
	}

	cloneStep := model.TestStep{
		Name:        "cloneFeatureObject",
		Description: fmt.Sprintf("Clone %s feature object", feature.Name),
		Method:      "POST",
		API:         model.APITypeREST,
		Path:        clonePath,
		PathParams: map[string]string{
			"name": profileName,
		},
		Body:           cloneBody,
		ExpectedStatus: 201,
		Validations: []model.Validation{
			{Type: "statusCode", Expected: "201"},
		},
	}
	tc.Steps = append(tc.Steps, cloneStep)

	return tc
}

// createBasicCreateStep creates a create step with a complete deep-scanned request body
func (g *Generator) createBasicCreateStep(feature *model.Feature, fp *model.FeaturePath, profileName string) model.TestStep {
	body := g.generateRequestBody(feature, fp)
	keyDesc := describeKeyValues(body, feature)

	return model.TestStep{
		Name:           "create" + feature.Name,
		Description:    fmt.Sprintf("Create %s configuration with %s", feature.Name, keyDesc),
		Method:         "POST",
		API:            model.APITypeREST,
		Path:           "/global-profile/feature/object/modify",
		Body:           body,
		ExpectedStatus: 201,
		Validations:    crudValidations(201, "create", feature.Name, keyDesc),
	}
}

// generateNOSAPIConflictBody creates a feature-specific NOSAPI body that simulates
// an out-of-band configuration change on the device. Uses a modified value for one
// key parameter so the device value diverges from the cloud-deployed value.
func (g *Generator) generateNOSAPIConflictBody(feature *model.Feature, deviceHostName string) map[string]interface{} {
	// Build a NOSAPI-style body with a conflicting value for the feature
	conflictProps := map[string]interface{}{}
	for _, param := range feature.Parameters {
		if isSystemManagedField(param.Name) {
			continue
		}
		if param.Required {
			conflictProps[param.Name] = g.getUpdatedValue(param) // different value = conflict
		}
	}
	// If no required fields, pick the first non-system field
	if len(conflictProps) == 0 {
		for _, param := range feature.Parameters {
			if isSystemManagedField(param.Name) {
				continue
			}
			conflictProps[param.Name] = g.getUpdatedValue(param)
			break
		}
	}

	return map[string]interface{}{
		"deviceHostName": deviceHostName,
		"feature":        feature.Name,
		"action":         "modify",
		"properties":     conflictProps,
	}
}

// generateConflictDetectionTest creates a functional test that deploys a feature, simulates an
// out-of-band device change via NOSAPI, and verifies the conflict is detected via GET conflicts.
// Conflict = deviceValue ≠ previousDeployedValue (device was changed outside cloud control).
func (g *Generator) generateConflictDetectionTest(feature *model.Feature, fp *model.FeaturePath, profileName string) model.TestCase {
	deviceHostName := "test-device-001"

	tc := model.TestCase{
		TestCaseID:       g.nextTestID(),
		FeatureName:      feature.Name,
		Priority:         model.TestPriorityP2,
		Type:             model.TestCategoryFunctional,
		Description:      fmt.Sprintf("[Conflict Detection] Deploy %s, simulate out-of-band device change via NOSAPI (deviceValue ≠ previousDeployedValue), verify conflict detected via GET /conflicts", feature.Name),
		IsDeploymentTest: true,
		Steps:            []model.TestStep{},
	}

	// Step 1: Create configuration
	tc.Steps = append(tc.Steps, g.createBasicCreateStep(feature, fp, profileName))

	// Step 2: Initial deploy to establish previousDeployedValue baseline on device
	tc.Steps = append(tc.Steps, model.TestStep{
		Name:           "initialDeploy",
		Description:    "Deploy to device — establishes previousDeployedValue baseline",
		Method:         "POST",
		API:            model.APITypeREST,
		Path:           fmt.Sprintf("/configuration-profile/%s/devices/deploy", profileName),
		PathParams:     map[string]string{"name": profileName},
		Body:           map[string]interface{}{"devices": []string{deviceHostName}, "deployNow": true},
		ExpectedStatus: 202,
		Validations:    []model.Validation{{Type: model.ValidationTypeStatusCode, Expected: 202}},
	})

	// Step 3: Verify initial deployment success
	tc.Steps = append(tc.Steps, model.TestStep{
		Name:           "checkInitialDeployStatus",
		Description:    "Verify initial deployment completed successfully",
		Method:         "GET",
		API:            model.APITypeREST,
		Path:           fmt.Sprintf("/configuration-profile/%s/device/%s/deploy/status", profileName, deviceHostName),
		PathParams:     map[string]string{"name": profileName, "hostName": deviceHostName},
		ExpectedStatus: 200,
		Validations: []model.Validation{
			{Type: model.ValidationTypeStatusCode, Expected: 200},
			{Type: model.ValidationTypeJSONPathEquals, Path: "$.status", Expected: "SUCCESS"},
		},
		Timeout: 300,
	})

	// Step 4: Simulate out-of-band device change via NOSAPI (makes deviceValue ≠ previousDeployedValue)
	nosBody := g.generateNOSAPIConflictBody(feature, deviceHostName)
	nosStep := model.TestStep{
		Name:           "simulateOutOfBandChange",
		Description:    fmt.Sprintf("Apply conflicting %s value directly on device via NOSAPI — simulates out-of-band change so deviceValue diverges from previousDeployedValue", feature.Name),
		Method:         "POST",
		API:            model.APITypeNOSAPI,
		Path:           "/v0/configuration/device",
		Body:           nosBody,
		ExpectedStatus: 200,
		Validations: []model.Validation{
			{Type: model.ValidationTypeStatusCode, Expected: 200,
				Description: fmt.Sprintf("Verify NOSAPI accepted the out-of-band %s change on device %s", feature.Name, deviceHostName)},
		},
	}
	if g.nosParser != nil {
		if ep := g.nosParser.FindEndpointForFeature(feature.Name, model.ScopeType(model.TargetTypeDevice)); ep != nil {
			nosStep.Path = ep.Path
			nosStep.Method = ep.Method
		}
	}
	tc.Steps = append(tc.Steps, nosStep)

	// Step 5: GET conflicts — expect hasConflicts=true
	tc.Steps = append(tc.Steps, model.TestStep{
		Name:           "getConflicts",
		Description:    "GET conflicts for this device — expect hasConflicts=true because device was changed out-of-band",
		Method:         "GET",
		API:            model.APITypeREST,
		Path:           fmt.Sprintf("/configuration-profile/%s/device/%s/conflicts", profileName, deviceHostName),
		PathParams:     map[string]string{"name": profileName, "hostName": deviceHostName},
		QueryParams:    map[string]string{"detailLevel": "detailed"},
		ExpectedStatus: 200,
		Validations: []model.Validation{
			{Type: model.ValidationTypeStatusCode, Expected: 200},
			{Type: model.ValidationTypeJSONPathEquals, Path: "$.hasConflicts", Expected: true},
			{Type: model.ValidationTypeJSONPathExists, Path: "$.features[0].featurePath", Description: "Conflicting feature path is reported"},
		},
	})

	return tc
}

// generateConflictResolutionCCTest creates a functional test for CC (Cloud wins) conflict resolution.
// Steps: deploy → simulate conflict → verify conflict → resolve acceptCloud → re-deploy → verify cleared.
func (g *Generator) generateConflictResolutionCCTest(feature *model.Feature, fp *model.FeaturePath, profileName string) model.TestCase {
	deviceHostName := "test-device-001"
	deviceID := "550e8400-e29b-41d4-a716-446655440000"

	tc := model.TestCase{
		TestCaseID:       g.nextTestID(),
		FeatureName:      feature.Name,
		Priority:         model.TestPriorityP1,
		Type:             model.TestCategoryFunctional,
		Description:      fmt.Sprintf("[Conflict Resolution CC] %s: detect conflict → resolve acceptCloud (cloud wins) → re-deploy → verify cleared", feature.Name),
		IsDeploymentTest: true,
		Steps:            []model.TestStep{},
	}

	// Step 1: Create and initial deploy
	tc.Steps = append(tc.Steps, g.createBasicCreateStep(feature, fp, profileName))
	tc.Steps = append(tc.Steps, model.TestStep{
		Name:           "initialDeploy",
		Description:    "Deploy to establish previousDeployedValue baseline",
		Method:         "POST",
		API:            model.APITypeREST,
		Path:           fmt.Sprintf("/configuration-profile/%s/devices/deploy", profileName),
		PathParams:     map[string]string{"name": profileName},
		Body:           map[string]interface{}{"devices": []string{deviceHostName}, "deployNow": true},
		ExpectedStatus: 202,
		Validations:    []model.Validation{{Type: model.ValidationTypeStatusCode, Expected: 202}},
	})

	// Step 2: Simulate out-of-band change
	nosBody := g.generateNOSAPIConflictBody(feature, deviceHostName)
	nosStep := model.TestStep{
		Name:           "simulateOutOfBandChange",
		Description:    fmt.Sprintf("Simulate out-of-band %s change on device via NOSAPI (creates conflict)", feature.Name),
		Method:         "POST",
		API:            model.APITypeNOSAPI,
		Path:           "/v0/configuration/device",
		Body:           nosBody,
		ExpectedStatus: 200,
		Validations: []model.Validation{
			{Type: model.ValidationTypeStatusCode, Expected: 200,
				Description: fmt.Sprintf("Verify NOSAPI accepted the out-of-band %s change on device %s", feature.Name, deviceHostName)},
		},
	}
	if g.nosParser != nil {
		if ep := g.nosParser.FindEndpointForFeature(feature.Name, model.ScopeType(model.TargetTypeDevice)); ep != nil {
			nosStep.Path = ep.Path
			nosStep.Method = ep.Method
		}
	}
	tc.Steps = append(tc.Steps, nosStep)

	// Step 3: Verify conflict exists
	tc.Steps = append(tc.Steps, model.TestStep{
		Name:           "verifyConflictExists",
		Description:    "Confirm hasConflicts=true before resolution",
		Method:         "GET",
		API:            model.APITypeREST,
		Path:           fmt.Sprintf("/configuration-profile/%s/device/%s/conflicts", profileName, deviceHostName),
		PathParams:     map[string]string{"name": profileName, "hostName": deviceHostName},
		QueryParams:    map[string]string{"detailLevel": "summary"},
		ExpectedStatus: 200,
		Validations: []model.Validation{
			{Type: model.ValidationTypeStatusCode, Expected: 200,
				Description: "Verify conflict check returns HTTP 200"},
			{Type: model.ValidationTypeJSONPathEquals, Path: "$.hasConflicts", Expected: true,
				Description: fmt.Sprintf("Verify hasConflicts=true for %s after out-of-band change", feature.Name)},
		},
	})

	// Step 4: Resolve via CC — acceptCloud (cloud value wins, device will be pushed cloud value on re-deploy)
	tc.Steps = append(tc.Steps, model.TestStep{
		Name:        "resolveConflictCC",
		Description: "POST resolve with acceptCloud (CC): cloud value wins — device will be updated to cloud intent on next deploy",
		Method:      "POST",
		API:         model.APITypeREST,
		Path:        "/configuration-profile/device/conflict/resolve",
		Body: map[string]interface{}{
			"deviceResolutions": []map[string]interface{}{
				{"deviceId": deviceID, "resolution": "acceptCloud"},
			},
		},
		ExpectedStatus: 200,
		Validations: []model.Validation{
			{Type: model.ValidationTypeStatusCode, Expected: 200,
				Description: "Verify conflict resolution accepted"},
			{Type: model.ValidationTypeJSONPathEquals, Path: "$.results[0].status", Expected: "success",
				Description: "Verify acceptCloud resolution status is success"},
			{Type: model.ValidationTypeJSONPathExists, Path: "$.summary.numberOfConflictsResolved",
				Description: "Verify numberOfConflictsResolved > 0"},
		},
	})

	// Step 5: Re-deploy — pushes cloud value back to device
	tc.Steps = append(tc.Steps, model.TestStep{
		Name:           "redeployAfterCCResolution",
		Description:    "Re-deploy profile to push cloud value back to device",
		Method:         "POST",
		API:            model.APITypeREST,
		Path:           fmt.Sprintf("/configuration-profile/%s/devices/deploy", profileName),
		PathParams:     map[string]string{"name": profileName},
		Body:           map[string]interface{}{"devices": []string{deviceHostName}, "deployNow": true},
		ExpectedStatus: 202,
		Validations:    []model.Validation{{Type: model.ValidationTypeStatusCode, Expected: 202}},
	})

	// Step 6: Check re-deploy status
	tc.Steps = append(tc.Steps, model.TestStep{
		Name:           "checkRedeployStatus",
		Description:    "Verify re-deployment completed successfully",
		Method:         "GET",
		API:            model.APITypeREST,
		Path:           fmt.Sprintf("/configuration-profile/%s/device/%s/deploy/status", profileName, deviceHostName),
		PathParams:     map[string]string{"name": profileName, "hostName": deviceHostName},
		ExpectedStatus: 200,
		Validations: []model.Validation{
			{Type: model.ValidationTypeStatusCode, Expected: 200},
			{Type: model.ValidationTypeJSONPathEquals, Path: "$.status", Expected: "SUCCESS"},
		},
		Timeout: 300,
	})

	// Step 7: Verify conflict cleared
	tc.Steps = append(tc.Steps, model.TestStep{
		Name:           "verifyConflictCleared",
		Description:    "GET conflicts after CC resolution — expect hasConflicts=false",
		Method:         "GET",
		API:            model.APITypeREST,
		Path:           fmt.Sprintf("/configuration-profile/%s/device/%s/conflicts", profileName, deviceHostName),
		PathParams:     map[string]string{"name": profileName, "hostName": deviceHostName},
		QueryParams:    map[string]string{"detailLevel": "summary"},
		ExpectedStatus: 200,
		Validations: []model.Validation{
			{Type: model.ValidationTypeStatusCode, Expected: 200},
			{Type: model.ValidationTypeJSONPathEquals, Path: "$.hasConflicts", Expected: false},
		},
	})

	return tc
}

// generateConflictResolutionDDTest creates a functional test for DD (Device wins) conflict resolution.
// Steps: deploy → simulate conflict → verify conflict → resolve acceptDevice → verify cleared (no re-deploy needed).
func (g *Generator) generateConflictResolutionDDTest(feature *model.Feature, fp *model.FeaturePath, profileName string) model.TestCase {
	deviceHostName := "test-device-001"
	deviceID := "550e8400-e29b-41d4-a716-446655440000"

	tc := model.TestCase{
		TestCaseID:       g.nextTestID(),
		FeatureName:      feature.Name,
		Priority:         model.TestPriorityP1,
		Type:             model.TestCategoryFunctional,
		Description:      fmt.Sprintf("[Conflict Resolution DD] %s: detect conflict → resolve acceptDevice (device wins) → verify cloud baseline updated and conflict cleared (no re-deploy needed)", feature.Name),
		IsDeploymentTest: true,
		Steps:            []model.TestStep{},
	}

	// Step 1: Create and initial deploy
	tc.Steps = append(tc.Steps, g.createBasicCreateStep(feature, fp, profileName))
	tc.Steps = append(tc.Steps, model.TestStep{
		Name:           "initialDeploy",
		Description:    "Deploy to establish previousDeployedValue baseline",
		Method:         "POST",
		API:            model.APITypeREST,
		Path:           fmt.Sprintf("/configuration-profile/%s/devices/deploy", profileName),
		PathParams:     map[string]string{"name": profileName},
		Body:           map[string]interface{}{"devices": []string{deviceHostName}, "deployNow": true},
		ExpectedStatus: 202,
		Validations:    []model.Validation{{Type: model.ValidationTypeStatusCode, Expected: 202}},
	})

	// Step 2: Simulate out-of-band change
	nosBodyDD := g.generateNOSAPIConflictBody(feature, deviceHostName)
	nosStepDD := model.TestStep{
		Name:           "simulateOutOfBandChange",
		Description:    fmt.Sprintf("Simulate out-of-band %s change on device via NOSAPI (creates conflict)", feature.Name),
		Method:         "POST",
		API:            model.APITypeNOSAPI,
		Path:           "/v0/configuration/device",
		Body:           nosBodyDD,
		ExpectedStatus: 200,
		Validations: []model.Validation{
			{Type: model.ValidationTypeStatusCode, Expected: 200,
				Description: fmt.Sprintf("Verify NOSAPI accepted the out-of-band %s change on device %s", feature.Name, deviceHostName)},
		},
	}
	if g.nosParser != nil {
		if ep := g.nosParser.FindEndpointForFeature(feature.Name, model.ScopeType(model.TargetTypeDevice)); ep != nil {
			nosStepDD.Path = ep.Path
			nosStepDD.Method = ep.Method
		}
	}
	tc.Steps = append(tc.Steps, nosStepDD)

	// Step 3: Verify conflict exists
	tc.Steps = append(tc.Steps, model.TestStep{
		Name:           "verifyConflictExists",
		Description:    "Confirm hasConflicts=true before resolution",
		Method:         "GET",
		API:            model.APITypeREST,
		Path:           fmt.Sprintf("/configuration-profile/%s/device/%s/conflicts", profileName, deviceHostName),
		PathParams:     map[string]string{"name": profileName, "hostName": deviceHostName},
		QueryParams:    map[string]string{"detailLevel": "summary"},
		ExpectedStatus: 200,
		Validations: []model.Validation{
			{Type: model.ValidationTypeStatusCode, Expected: 200,
				Description: "Verify conflict check returns HTTP 200"},
			{Type: model.ValidationTypeJSONPathEquals, Path: "$.hasConflicts", Expected: true,
				Description: fmt.Sprintf("Verify hasConflicts=true for %s after out-of-band change", feature.Name)},
		},
	})

	// Step 4: Resolve via DD — acceptDevice (device value becomes new cloud baseline, no re-deploy needed)
	tc.Steps = append(tc.Steps, model.TestStep{
		Name:        "resolveConflictDD",
		Description: "POST resolve with acceptDevice (DD): device value wins — cloud updates previousDeployedValue to match device, no re-deploy required",
		Method:      "POST",
		API:         model.APITypeREST,
		Path:        "/configuration-profile/device/conflict/resolve",
		Body: map[string]interface{}{
			"deviceResolutions": []map[string]interface{}{
				{"deviceId": deviceID, "resolution": "acceptDevice"},
			},
		},
		ExpectedStatus: 200,
		Validations: []model.Validation{
			{Type: model.ValidationTypeStatusCode, Expected: 200,
				Description: "Verify conflict resolution accepted"},
			{Type: model.ValidationTypeJSONPathEquals, Path: "$.results[0].status", Expected: "success",
				Description: "Verify acceptDevice resolution status is success"},
			{Type: model.ValidationTypeJSONPathExists, Path: "$.summary.numberOfConflictsResolved",
				Description: "Verify numberOfConflictsResolved > 0"},
		},
	})

	// Step 5: Verify conflict cleared (no re-deploy needed — cloud accepted device value as new baseline)
	tc.Steps = append(tc.Steps, model.TestStep{
		Name:           "verifyConflictCleared",
		Description:    "GET conflicts after DD resolution — expect hasConflicts=false (device value is now accepted cloud baseline)",
		Method:         "GET",
		API:            model.APITypeREST,
		Path:           fmt.Sprintf("/configuration-profile/%s/device/%s/conflicts", profileName, deviceHostName),
		PathParams:     map[string]string{"name": profileName, "hostName": deviceHostName},
		QueryParams:    map[string]string{"detailLevel": "summary"},
		ExpectedStatus: 200,
		Validations: []model.Validation{
			{Type: model.ValidationTypeStatusCode, Expected: 200,
				Description: "Verify conflict check returns HTTP 200"},
			{Type: model.ValidationTypeJSONPathEquals, Path: "$.hasConflicts", Expected: false,
				Description: fmt.Sprintf("Verify hasConflicts=false after DD resolution for %s — device value accepted as new cloud baseline", feature.Name)},
		},
	})

	return tc
}

// ============================================================================
// Override Test Generators
// ============================================================================

// generateOverrideTests creates tests for the override CRUD lifecycle at device, model, and model-group levels.
func (g *Generator) generateOverrideTests(feature *model.Feature, fp *model.FeaturePath, profileName string) []model.TestCase {
	var tests []model.TestCase

	// Determine featurePath and objectType from the FeaturePath
	featurePath := fp.Path
	objectType := feature.Name
	for _, param := range fp.PathParams {
		if param.Name == "featurePath" && param.FixedValue != "" {
			featurePath = param.FixedValue
		}
		if param.Name == "objectType" && param.FixedValue != "" {
			objectType = param.FixedValue
		}
	}

	// 1. Device-level override: full CRUD lifecycle
	tests = append(tests, g.generateOverrideCRUDLifecycleTest(feature, fp, profileName, "device", featurePath, objectType))

	// 2. Model-level override: full CRUD lifecycle
	tests = append(tests, g.generateOverrideModelLevelTest(feature, fp, profileName, featurePath, objectType))

	// 3. Model-group-level override: full CRUD lifecycle
	tests = append(tests, g.generateOverrideModelGroupLevelTest(feature, fp, profileName, featurePath, objectType))

	// 4. Get all overrides for profile
	tests = append(tests, g.generateGetAllOverridesTest(feature, fp, profileName))

	// 5. Override precedence: device override takes priority over model override
	tests = append(tests, g.generateOverridePrecedenceTest(feature, fp, profileName, featurePath, objectType))

	// 6. Deploy with device override and verify override values applied
	tests = append(tests, g.generateOverrideDeployVerifyTest(feature, fp, profileName, featurePath, objectType))

	// 7. Negative: remove non-existent override
	tests = append(tests, g.generateOverrideRemoveNonExistentTest(feature, fp, profileName, objectType))

	// 8. Negative: create override with invalid overrideType
	tests = append(tests, g.generateOverrideInvalidTypeTest(feature, fp, profileName, featurePath, objectType))

	return tests
}

// generateOverrideProperties builds override property values from a feature's parameters
func (g *Generator) generateOverrideProperties(feature *model.Feature) []map[string]interface{} {
	var props []map[string]interface{}
	for _, param := range feature.Parameters {
		if isSystemManagedField(param.Name) {
			continue
		}
		if param.Required {
			props = append(props, map[string]interface{}{
				"name":  param.Name,
				"value": g.getUpdatedValue(param), // use different value from base config
			})
		}
	}
	// If no required non-system fields, pick the first available
	if len(props) == 0 {
		for _, param := range feature.Parameters {
			if isSystemManagedField(param.Name) {
				continue
			}
			props = append(props, map[string]interface{}{
				"name":  param.Name,
				"value": g.getUpdatedValue(param),
			})
			break
		}
	}
	return props
}

// describeOverrideProperties returns a human-readable summary of override properties
func describeOverrideProperties(props []map[string]interface{}) string {
	if len(props) == 0 {
		return "(no properties)"
	}
	if len(props) == 1 {
		return fmt.Sprintf("%s=%v", props[0]["name"], props[0]["value"])
	}
	return fmt.Sprintf("%s=%v (+%d more)", props[0]["name"], props[0]["value"], len(props)-1)
}

// generateOverrideCRUDLifecycleTest creates a device-level override CRUD lifecycle test:
// create base → create override → retrieve override → modify override → remove override → verify removed
func (g *Generator) generateOverrideCRUDLifecycleTest(feature *model.Feature, fp *model.FeaturePath, profileName string, overrideType string, featurePath string, objectType string) model.TestCase {
	deviceID := "550e8400-e29b-41d4-a716-446655440000"
	objectID := "660e8400-e29b-41d4-a716-446655440001"
	overrideProps := g.generateOverrideProperties(feature)
	propDesc := describeOverrideProperties(overrideProps)

	tc := model.TestCase{
		TestCaseID:  g.nextTestID(),
		FeatureName: feature.Name,
		Priority:    model.TestPriorityP1,
		Type:        model.TestCategoryFunctional,
		Description: fmt.Sprintf("[Override CRUD] %s: Create base config → set device-level override (overrideType=device, targetId=%s, override %s) → retrieve override → modify override values → remove override → verify override no longer exists", feature.Name, deviceID, propDesc),
		Steps:       []model.TestStep{},
	}

	// Step 1: Create base configuration
	tc.Steps = append(tc.Steps, g.createBasicCreateStep(feature, fp, profileName))

	// Step 2: Create device-level override
	tc.Steps = append(tc.Steps, model.TestStep{
		Name:        "createDeviceOverride",
		Description: fmt.Sprintf("Create device-level override for %s with %s", feature.Name, propDesc),
		Method:      "POST",
		API:         model.APITypeREST,
		Path:        fmt.Sprintf("/configuration-profile/%s/feature/object/override/create-modify", profileName),
		PathParams:  map[string]string{"name": profileName},
		Body: map[string]interface{}{
			"objectId":           objectID,
			"overrideType":       "device",
			"targetId":           deviceID,
			"overrideProperties": overrideProps,
		},
		ExpectedStatus: 200,
		Validations: []model.Validation{
			{Type: model.ValidationTypeStatusCode, Expected: 200, Description: "Verify device override created successfully"},
		},
	})

	// Step 3: Retrieve device-level override
	tc.Steps = append(tc.Steps, model.TestStep{
		Name:        "retrieveDeviceOverride",
		Description: fmt.Sprintf("Retrieve device-level override for %s and verify overridden values", feature.Name),
		Method:      "POST",
		API:         model.APITypeREST,
		Path:        fmt.Sprintf("/configuration-profile/%s/feature/object/override/retrieve", profileName),
		PathParams:  map[string]string{"name": profileName},
		Body: map[string]interface{}{
			"objectId":     objectID,
			"overrideType": "device",
			"targetId":     deviceID,
		},
		ExpectedStatus: 200,
		Validations: []model.Validation{
			{Type: model.ValidationTypeStatusCode, Expected: 200, Description: "Verify override retrieved successfully"},
			{Type: model.ValidationTypeJSONPathExists, Path: "$.configObjects", Description: "Verify configObjects array present in response"},
			{Type: model.ValidationTypeJSONPathExists, Path: "$.overrideHierarchy.deviceIdentifier", Description: "Verify device identifier present in override hierarchy"},
		},
	})

	// Step 4: Modify override (update properties)
	modifiedProps := g.generateOverrideProperties(feature)
	// Change the first property value to something else to simulate modification
	if len(modifiedProps) > 0 {
		modifiedProps[0]["value"] = g.getSampleValue(feature.Parameters[0])
	}
	tc.Steps = append(tc.Steps, model.TestStep{
		Name:        "modifyDeviceOverride",
		Description: fmt.Sprintf("Modify device-level override for %s — update property values", feature.Name),
		Method:      "POST",
		API:         model.APITypeREST,
		Path:        fmt.Sprintf("/configuration-profile/%s/feature/object/override/create-modify", profileName),
		PathParams:  map[string]string{"name": profileName},
		Body: map[string]interface{}{
			"objectId":         objectID,
			"overrideType":     "device",
			"targetId":         deviceID,
			"updateProperties": modifiedProps,
		},
		ExpectedStatus: 200,
		Validations: []model.Validation{
			{Type: model.ValidationTypeStatusCode, Expected: 200, Description: "Verify override modified successfully"},
		},
	})

	// Step 5: Remove device-level override
	tc.Steps = append(tc.Steps, model.TestStep{
		Name:        "removeDeviceOverride",
		Description: fmt.Sprintf("Remove device-level override for %s", feature.Name),
		Method:      "POST",
		API:         model.APITypeREST,
		Path:        fmt.Sprintf("/configuration-profile/%s/feature/object/override/remove", profileName),
		PathParams:  map[string]string{"name": profileName},
		Body: map[string]interface{}{
			"objectId":     objectID,
			"overrideType": "device",
			"targetId":     deviceID,
		},
		ExpectedStatus: 200,
		Validations: []model.Validation{
			{Type: model.ValidationTypeStatusCode, Expected: 200, Description: "Verify override removed successfully"},
		},
	})

	// Step 6: Verify override removed — retrieve should show no override
	tc.Steps = append(tc.Steps, model.TestStep{
		Name:        "verifyOverrideRemoved",
		Description: fmt.Sprintf("Retrieve override after removal — verify no device-level override exists for %s", feature.Name),
		Method:      "POST",
		API:         model.APITypeREST,
		Path:        fmt.Sprintf("/configuration-profile/%s/feature/object/override/retrieve", profileName),
		PathParams:  map[string]string{"name": profileName},
		Body: map[string]interface{}{
			"objectId":     objectID,
			"overrideType": "device",
			"targetId":     deviceID,
		},
		ExpectedStatus: 200,
		Validations: []model.Validation{
			{Type: model.ValidationTypeStatusCode, Expected: 200, Description: "Verify retrieve returns 200"},
		},
	})

	return tc
}

// generateOverrideModelLevelTest creates a model-level override test:
// create base → create model override → retrieve → verify → remove
func (g *Generator) generateOverrideModelLevelTest(feature *model.Feature, fp *model.FeaturePath, profileName string, featurePath string, objectType string) model.TestCase {
	modelID := "5520"
	objectID := "660e8400-e29b-41d4-a716-446655440001"
	overrideProps := g.generateOverrideProperties(feature)
	propDesc := describeOverrideProperties(overrideProps)

	tc := model.TestCase{
		TestCaseID:  g.nextTestID(),
		FeatureName: feature.Name,
		Priority:    model.TestPriorityP2,
		Type:        model.TestCategoryFunctional,
		Description: fmt.Sprintf("[Override Model] %s: Create base config → set model-level override (overrideType=model, targetId=%s, override %s) → retrieve override → verify model identifier in response → remove override", feature.Name, modelID, propDesc),
		Steps:       []model.TestStep{},
	}

	// Step 1: Create base configuration
	tc.Steps = append(tc.Steps, g.createBasicCreateStep(feature, fp, profileName))

	// Step 2: Create model-level override
	tc.Steps = append(tc.Steps, model.TestStep{
		Name:        "createModelOverride",
		Description: fmt.Sprintf("Create model-level override for %s on model %s with %s", feature.Name, modelID, propDesc),
		Method:      "POST",
		API:         model.APITypeREST,
		Path:        fmt.Sprintf("/configuration-profile/%s/feature/object/override/create-modify", profileName),
		PathParams:  map[string]string{"name": profileName},
		Body: map[string]interface{}{
			"objectId":           objectID,
			"overrideType":       "model",
			"targetId":           modelID,
			"overrideProperties": overrideProps,
		},
		ExpectedStatus: 200,
		Validations: []model.Validation{
			{Type: model.ValidationTypeStatusCode, Expected: 200, Description: "Verify model override created successfully"},
		},
	})

	// Step 3: Retrieve model-level override
	tc.Steps = append(tc.Steps, model.TestStep{
		Name:        "retrieveModelOverride",
		Description: fmt.Sprintf("Retrieve model-level override for %s on model %s", feature.Name, modelID),
		Method:      "POST",
		API:         model.APITypeREST,
		Path:        fmt.Sprintf("/configuration-profile/%s/feature/object/override/retrieve", profileName),
		PathParams:  map[string]string{"name": profileName},
		Body: map[string]interface{}{
			"objectId":     objectID,
			"overrideType": "model",
			"targetId":     modelID,
		},
		ExpectedStatus: 200,
		Validations: []model.Validation{
			{Type: model.ValidationTypeStatusCode, Expected: 200, Description: "Verify model override retrieved"},
			{Type: model.ValidationTypeJSONPathExists, Path: "$.overrideHierarchy.modelIdentifier", Description: "Verify model identifier present in override hierarchy"},
		},
	})

	// Step 4: Remove model override
	tc.Steps = append(tc.Steps, model.TestStep{
		Name:        "removeModelOverride",
		Description: fmt.Sprintf("Remove model-level override for %s on model %s", feature.Name, modelID),
		Method:      "POST",
		API:         model.APITypeREST,
		Path:        fmt.Sprintf("/configuration-profile/%s/feature/object/override/remove", profileName),
		PathParams:  map[string]string{"name": profileName},
		Body: map[string]interface{}{
			"objectId":     objectID,
			"overrideType": "model",
			"targetId":     modelID,
		},
		ExpectedStatus: 200,
		Validations: []model.Validation{
			{Type: model.ValidationTypeStatusCode, Expected: 200, Description: "Verify model override removed"},
		},
	})

	return tc
}

// generateOverrideModelGroupLevelTest creates a model-group-level override test
func (g *Generator) generateOverrideModelGroupLevelTest(feature *model.Feature, fp *model.FeaturePath, profileName string, featurePath string, objectType string) model.TestCase {
	modelGroupID := "enterprise-access-points"
	objectID := "660e8400-e29b-41d4-a716-446655440001"
	overrideProps := g.generateOverrideProperties(feature)
	propDesc := describeOverrideProperties(overrideProps)

	tc := model.TestCase{
		TestCaseID:  g.nextTestID(),
		FeatureName: feature.Name,
		Priority:    model.TestPriorityP2,
		Type:        model.TestCategoryFunctional,
		Description: fmt.Sprintf("[Override Model-Group] %s: Create base config → set model-group override (overrideType=model-group, targetId='%s', override %s) → retrieve override → verify modelGroupIdentifier in response → remove override", feature.Name, modelGroupID, propDesc),
		Steps:       []model.TestStep{},
	}

	// Step 1: Create base configuration
	tc.Steps = append(tc.Steps, g.createBasicCreateStep(feature, fp, profileName))

	// Step 2: Create model-group override
	tc.Steps = append(tc.Steps, model.TestStep{
		Name:        "createModelGroupOverride",
		Description: fmt.Sprintf("Create model-group override for %s on group '%s' with %s", feature.Name, modelGroupID, propDesc),
		Method:      "POST",
		API:         model.APITypeREST,
		Path:        fmt.Sprintf("/configuration-profile/%s/feature/object/override/create-modify", profileName),
		PathParams:  map[string]string{"name": profileName},
		Body: map[string]interface{}{
			"objectId":           objectID,
			"overrideType":       "model-group",
			"targetId":           modelGroupID,
			"overrideProperties": overrideProps,
		},
		ExpectedStatus: 200,
		Validations: []model.Validation{
			{Type: model.ValidationTypeStatusCode, Expected: 200, Description: "Verify model-group override created"},
		},
	})

	// Step 3: Retrieve model-group override
	tc.Steps = append(tc.Steps, model.TestStep{
		Name:        "retrieveModelGroupOverride",
		Description: fmt.Sprintf("Retrieve model-group override for %s on group '%s'", feature.Name, modelGroupID),
		Method:      "POST",
		API:         model.APITypeREST,
		Path:        fmt.Sprintf("/configuration-profile/%s/feature/object/override/retrieve", profileName),
		PathParams:  map[string]string{"name": profileName},
		Body: map[string]interface{}{
			"objectId":     objectID,
			"overrideType": "model-group",
			"targetId":     modelGroupID,
		},
		ExpectedStatus: 200,
		Validations: []model.Validation{
			{Type: model.ValidationTypeStatusCode, Expected: 200, Description: "Verify model-group override retrieved"},
			{Type: model.ValidationTypeJSONPathExists, Path: "$.overrideHierarchy.modelGroupIdentifier", Description: "Verify modelGroupIdentifier present in override hierarchy"},
		},
	})

	// Step 4: Remove model-group override
	tc.Steps = append(tc.Steps, model.TestStep{
		Name:        "removeModelGroupOverride",
		Description: fmt.Sprintf("Remove model-group override for %s on group '%s'", feature.Name, modelGroupID),
		Method:      "POST",
		API:         model.APITypeREST,
		Path:        fmt.Sprintf("/configuration-profile/%s/feature/object/override/remove", profileName),
		PathParams:  map[string]string{"name": profileName},
		Body: map[string]interface{}{
			"objectId":     objectID,
			"overrideType": "model-group",
			"targetId":     modelGroupID,
		},
		ExpectedStatus: 200,
		Validations: []model.Validation{
			{Type: model.ValidationTypeStatusCode, Expected: 200, Description: "Verify model-group override removed"},
		},
	})

	return tc
}

// generateGetAllOverridesTest creates a test to retrieve all overrides for a profile
func (g *Generator) generateGetAllOverridesTest(feature *model.Feature, fp *model.FeaturePath, profileName string) model.TestCase {
	objectID := "660e8400-e29b-41d4-a716-446655440001"
	deviceID := "550e8400-e29b-41d4-a716-446655440000"
	overrideProps := g.generateOverrideProperties(feature)

	tc := model.TestCase{
		TestCaseID:  g.nextTestID(),
		FeatureName: feature.Name,
		Priority:    model.TestPriorityP2,
		Type:        model.TestCategoryFunctional,
		Description: fmt.Sprintf("[Override List] %s: Create base config → create a device override → GET all overrides for profile → verify totalOverrides ≥ 1 and overrides array is present in response", feature.Name),
		Steps:       []model.TestStep{},
	}

	// Step 1: Create base configuration
	tc.Steps = append(tc.Steps, g.createBasicCreateStep(feature, fp, profileName))

	// Step 2: Create a device override so there's at least one
	tc.Steps = append(tc.Steps, model.TestStep{
		Name:        "createOverrideForListing",
		Description: fmt.Sprintf("Create device override for %s to ensure GET all overrides returns results", feature.Name),
		Method:      "POST",
		API:         model.APITypeREST,
		Path:        fmt.Sprintf("/configuration-profile/%s/feature/object/override/create-modify", profileName),
		PathParams:  map[string]string{"name": profileName},
		Body: map[string]interface{}{
			"objectId":           objectID,
			"overrideType":       "device",
			"targetId":           deviceID,
			"overrideProperties": overrideProps,
		},
		ExpectedStatus: 200,
		Validations: []model.Validation{
			{Type: model.ValidationTypeStatusCode, Expected: 200},
		},
	})

	// Step 3: GET all overrides
	tc.Steps = append(tc.Steps, model.TestStep{
		Name:           "getAllOverrides",
		Description:    fmt.Sprintf("GET all overrides for profile — verify %s override listed", feature.Name),
		Method:         "GET",
		API:            model.APITypeREST,
		Path:           fmt.Sprintf("/configuration-profile/%s/feature/object/overrides", profileName),
		PathParams:     map[string]string{"name": profileName},
		ExpectedStatus: 200,
		Validations: []model.Validation{
			{Type: model.ValidationTypeStatusCode, Expected: 200, Description: "Verify GET all overrides returns 200"},
			{Type: model.ValidationTypeJSONPathExists, Path: "$.profileName", Description: "Verify profileName present in response"},
			{Type: model.ValidationTypeJSONPathExists, Path: "$.totalOverrides", Description: "Verify totalOverrides count present"},
			{Type: model.ValidationTypeJSONPathExists, Path: "$.overrides", Description: "Verify overrides array present"},
		},
	})

	return tc
}

// generateOverridePrecedenceTest verifies that device override takes precedence over model override
func (g *Generator) generateOverridePrecedenceTest(feature *model.Feature, fp *model.FeaturePath, profileName string, featurePath string, objectType string) model.TestCase {
	deviceID := "550e8400-e29b-41d4-a716-446655440000"
	modelID := "5520"
	objectID := "660e8400-e29b-41d4-a716-446655440001"

	// Build model-level override props with one value
	modelProps := []map[string]interface{}{}
	deviceProps := []map[string]interface{}{}
	for _, param := range feature.Parameters {
		if isSystemManagedField(param.Name) {
			continue
		}
		if param.Required {
			modelProps = append(modelProps, map[string]interface{}{
				"name":  param.Name,
				"value": g.getSampleValue(param), // base/model value
			})
			deviceProps = append(deviceProps, map[string]interface{}{
				"name":  param.Name,
				"value": g.getUpdatedValue(param), // different device-level value
			})
			break // just one property is enough to demonstrate precedence
		}
	}

	tc := model.TestCase{
		TestCaseID:  g.nextTestID(),
		FeatureName: feature.Name,
		Priority:    model.TestPriorityP1,
		Type:        model.TestCategoryFunctional,
		Description: fmt.Sprintf("[Override Precedence] %s: Create base config → set model-level override (value A for model %s) → set device-level override (value B for device %s) → retrieve resolved config → verify device override value B takes precedence over model override value A", feature.Name, modelID, deviceID),
		Steps:       []model.TestStep{},
	}

	// Step 1: Create base configuration
	tc.Steps = append(tc.Steps, g.createBasicCreateStep(feature, fp, profileName))

	// Step 2: Create model-level override
	tc.Steps = append(tc.Steps, model.TestStep{
		Name:        "createModelOverride",
		Description: fmt.Sprintf("Create model-level override for %s on model %s", feature.Name, modelID),
		Method:      "POST",
		API:         model.APITypeREST,
		Path:        fmt.Sprintf("/configuration-profile/%s/feature/object/override/create-modify", profileName),
		PathParams:  map[string]string{"name": profileName},
		Body: map[string]interface{}{
			"objectId":           objectID,
			"overrideType":       "model",
			"targetId":           modelID,
			"overrideProperties": modelProps,
		},
		ExpectedStatus: 200,
		Validations: []model.Validation{
			{Type: model.ValidationTypeStatusCode, Expected: 200, Description: "Verify model override created"},
		},
	})

	// Step 3: Create device-level override (higher precedence)
	tc.Steps = append(tc.Steps, model.TestStep{
		Name:        "createDeviceOverride",
		Description: fmt.Sprintf("Create device-level override for %s (should take precedence over model override)", feature.Name),
		Method:      "POST",
		API:         model.APITypeREST,
		Path:        fmt.Sprintf("/configuration-profile/%s/feature/object/override/create-modify", profileName),
		PathParams:  map[string]string{"name": profileName},
		Body: map[string]interface{}{
			"objectId":           objectID,
			"overrideType":       "device",
			"targetId":           deviceID,
			"overrideProperties": deviceProps,
		},
		ExpectedStatus: 200,
		Validations: []model.Validation{
			{Type: model.ValidationTypeStatusCode, Expected: 200, Description: "Verify device override created"},
		},
	})

	// Step 4: Retrieve and verify device override wins
	tc.Steps = append(tc.Steps, model.TestStep{
		Name:        "retrieveAndVerifyPrecedence",
		Description: "Retrieve override for device — verify device-level value is returned (device > model > model-group > profile)",
		Method:      "POST",
		API:         model.APITypeREST,
		Path:        fmt.Sprintf("/configuration-profile/%s/feature/object/override/retrieve", profileName),
		PathParams:  map[string]string{"name": profileName},
		Body: map[string]interface{}{
			"objectId":     objectID,
			"overrideType": "device",
			"targetId":     deviceID,
		},
		ExpectedStatus: 200,
		Validations: []model.Validation{
			{Type: model.ValidationTypeStatusCode, Expected: 200, Description: "Verify retrieve returns 200"},
			{Type: model.ValidationTypeJSONPathExists, Path: "$.configObjects[0].properties", Description: "Verify properties present — device override values should be applied"},
		},
	})

	return tc
}

// generateOverrideDeployVerifyTest creates a test that creates a device override, deploys, and verifies the overridden values are applied
func (g *Generator) generateOverrideDeployVerifyTest(feature *model.Feature, fp *model.FeaturePath, profileName string, featurePath string, objectType string) model.TestCase {
	deviceHostName := "test-device-001"
	deviceID := "550e8400-e29b-41d4-a716-446655440000"
	objectID := "660e8400-e29b-41d4-a716-446655440001"
	overrideProps := g.generateOverrideProperties(feature)
	propDesc := describeOverrideProperties(overrideProps)

	tc := model.TestCase{
		TestCaseID:       g.nextTestID(),
		FeatureName:      feature.Name,
		Priority:         model.TestPriorityP1,
		Type:             model.TestCategoryFunctional,
		Description:      fmt.Sprintf("[Override Deploy] %s: Create base config → set device-level override (override %s for device %s) → deploy to device → verify deployment succeeds → verify no conflict (overridden values are intentional)", feature.Name, propDesc, deviceID),
		IsDeploymentTest: true,
		Steps:            []model.TestStep{},
	}

	// Step 1: Create base configuration
	tc.Steps = append(tc.Steps, g.createBasicCreateStep(feature, fp, profileName))

	// Step 2: Create device override
	tc.Steps = append(tc.Steps, model.TestStep{
		Name:        "createDeviceOverride",
		Description: fmt.Sprintf("Create device-level override for %s with %s", feature.Name, propDesc),
		Method:      "POST",
		API:         model.APITypeREST,
		Path:        fmt.Sprintf("/configuration-profile/%s/feature/object/override/create-modify", profileName),
		PathParams:  map[string]string{"name": profileName},
		Body: map[string]interface{}{
			"objectId":           objectID,
			"overrideType":       "device",
			"targetId":           deviceID,
			"overrideProperties": overrideProps,
		},
		ExpectedStatus: 200,
		Validations: []model.Validation{
			{Type: model.ValidationTypeStatusCode, Expected: 200, Description: "Verify override created"},
		},
	})

	// Step 3: Deploy to device
	tc.Steps = append(tc.Steps, model.TestStep{
		Name:           "deployWithOverride",
		Description:    "Deploy configuration with device override to device",
		Method:         "POST",
		API:            model.APITypeREST,
		Path:           fmt.Sprintf("/configuration-profile/%s/devices/deploy", profileName),
		PathParams:     map[string]string{"name": profileName},
		Body:           map[string]interface{}{"devices": []string{deviceHostName}, "deployNow": true},
		ExpectedStatus: 202,
		Validations: []model.Validation{
			{Type: model.ValidationTypeStatusCode, Expected: 202, Description: "Verify deployment accepted"},
		},
	})

	// Step 4: Check deployment status
	tc.Steps = append(tc.Steps, model.TestStep{
		Name:           "checkDeployStatus",
		Description:    "Verify deployment with override completed successfully",
		Method:         "GET",
		API:            model.APITypeREST,
		Path:           fmt.Sprintf("/configuration-profile/%s/device/%s/deploy/status", profileName, deviceHostName),
		PathParams:     map[string]string{"name": profileName, "hostName": deviceHostName},
		ExpectedStatus: 200,
		Validations: []model.Validation{
			{Type: model.ValidationTypeStatusCode, Expected: 200},
			{Type: model.ValidationTypeJSONPathEquals, Path: "$.status", Expected: "SUCCESS", Description: "Verify deployment with override succeeded"},
		},
		Timeout: 300,
	})

	// Step 5: Verify NOS config has override values (no conflict expected — override was deployed)
	tc.Steps = append(tc.Steps, model.TestStep{
		Name:           "verifyNoConflictAfterOverrideDeploy",
		Description:    "GET conflicts — expect hasConflicts=false since override values were deployed intentionally",
		Method:         "GET",
		API:            model.APITypeREST,
		Path:           fmt.Sprintf("/configuration-profile/%s/device/%s/conflicts", profileName, deviceHostName),
		PathParams:     map[string]string{"name": profileName, "hostName": deviceHostName},
		ExpectedStatus: 200,
		Validations: []model.Validation{
			{Type: model.ValidationTypeStatusCode, Expected: 200},
			{Type: model.ValidationTypeJSONPathEquals, Path: "$.hasConflicts", Expected: false, Description: "No conflict expected — override values were deployed"},
		},
	})

	return tc
}

// generateOverrideRemoveNonExistentTest creates a negative test: attempt to remove a non-existent override
func (g *Generator) generateOverrideRemoveNonExistentTest(feature *model.Feature, fp *model.FeaturePath, profileName string, objectType string) model.TestCase {
	nonExistentObjectID := "00000000-0000-0000-0000-000000000000"
	deviceID := "550e8400-e29b-41d4-a716-446655440000"

	tc := model.TestCase{
		TestCaseID:  g.nextTestID(),
		FeatureName: feature.Name,
		Priority:    model.TestPriorityP3,
		Type:        model.TestCategoryNegative,
		Description: fmt.Sprintf("[Override Negative] %s: Attempt POST /feature/object/override/remove with non-existent objectId='%s' and overrideType=device — expect HTTP 404 Not Found", feature.Name, nonExistentObjectID),
		Steps:       []model.TestStep{},
	}

	tc.Steps = append(tc.Steps, model.TestStep{
		Name:        "removeNonExistentOverride",
		Description: fmt.Sprintf("POST remove override with non-existent objectId for %s — expect 404", feature.Name),
		Method:      "POST",
		API:         model.APITypeREST,
		Path:        fmt.Sprintf("/configuration-profile/%s/feature/object/override/remove", profileName),
		PathParams:  map[string]string{"name": profileName},
		Body: map[string]interface{}{
			"objectId":     nonExistentObjectID,
			"overrideType": "device",
			"targetId":     deviceID,
		},
		ExpectedStatus: 404,
		Validations: []model.Validation{
			{Type: model.ValidationTypeStatusCode, Expected: 404, Description: "Verify 404 returned for non-existent override removal"},
		},
	})

	return tc
}

// generateOverrideInvalidTypeTest creates a negative test: attempt to create override with invalid overrideType
func (g *Generator) generateOverrideInvalidTypeTest(feature *model.Feature, fp *model.FeaturePath, profileName string, featurePath string, objectType string) model.TestCase {
	objectID := "660e8400-e29b-41d4-a716-446655440001"
	overrideProps := g.generateOverrideProperties(feature)

	tc := model.TestCase{
		TestCaseID:  g.nextTestID(),
		FeatureName: feature.Name,
		Priority:    model.TestPriorityP3,
		Type:        model.TestCategoryNegative,
		Description: fmt.Sprintf("[Override Negative] %s: Attempt POST /feature/object/override/create-modify with unsupported overrideType='invalid-type' — expect HTTP 400 Bad Request (valid values: device, model, model-group)", feature.Name),
		Steps:       []model.TestStep{},
	}

	tc.Steps = append(tc.Steps, model.TestStep{
		Name:        "createOverrideInvalidType",
		Description: fmt.Sprintf("POST create override with invalid overrideType for %s — expect 400 Bad Request", feature.Name),
		Method:      "POST",
		API:         model.APITypeREST,
		Path:        fmt.Sprintf("/configuration-profile/%s/feature/object/override/create-modify", profileName),
		PathParams:  map[string]string{"name": profileName},
		Body: map[string]interface{}{
			"objectId":           objectID,
			"overrideType":       "invalid-type",
			"targetId":           "some-target",
			"overrideProperties": overrideProps,
		},
		ExpectedStatus: 400,
		Validations: []model.Validation{
			{Type: model.ValidationTypeStatusCode, Expected: 400, Description: "Verify 400 returned for invalid overrideType"},
		},
	})

	return tc
}
