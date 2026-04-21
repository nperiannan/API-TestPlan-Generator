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
	
	return tests
}

// generateScheduledDeploymentTest creates a test for scheduled deployment (deployAt instead of deployNow)
func (g *Generator) generateScheduledDeploymentTest(feature *model.Feature, fp *model.FeaturePath, profileName string, targetType model.TargetType) model.TestCase {
	// Calculate future deployment time (1 hour from now)
	deployTime := time.Now().Add(1 * time.Hour).UTC().Format(time.RFC3339)
	
	tc := model.TestCase{
		TestCaseID:       g.nextTestID(),
		FeatureName:      feature.Name,
		Priority:         model.TestPriorityP0,
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
		Priority:         model.TestPriorityP1,
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
		Priority:         model.TestPriorityP1,
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
		Priority:    model.TestPriorityP1,
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
		Priority:    model.TestPriorityP1,
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

// createBasicCreateStep creates a simple create step for setting up a configuration
func (g *Generator) createBasicCreateStep(feature *model.Feature, fp *model.FeaturePath, profileName string) model.TestStep {
	// Get a basic parameter value for this feature
	var testValue interface{} = "test-value"
	var testParam string = "name"
	
	// Try to find a good parameter from the feature
	if len(feature.Parameters) > 0 {
		for _, param := range feature.Parameters {
			if param.Required {
				testParam = param.Name
				switch param.YangType {
				case "string":
					testValue = "test-" + param.Name
				case "number", "integer", "uint32", "uint64", "int32", "int64":
					testValue = 1
				case "boolean":
					testValue = true
				}
				break
			}
		}
	}
	
	body := map[string]interface{}{
		"featurePath": fp.Path,
		"name":        profileName,
		"objectType":  feature.Name,
		"objects": []map[string]interface{}{
			{
				"operation": "add",
				"properties": []map[string]interface{}{
					{
						"name":   testParam,
						"origin": "Global",
						"type":   "string",
						"value":  testValue,
					},
				},
				"type": feature.Name,
			},
		},
		"operation": "add",
	}
	
	return model.TestStep{
		Name:        "create" + feature.Name,
		Description: fmt.Sprintf("Create %s configuration", feature.Name),
		Method:      "POST",
		API:         model.APITypeREST,
		Path:        "/global-profile/feature/object/modify",
		Body:        body,
		ExpectedStatus: 201,
		Validations: []model.Validation{
			{Type: "statusCode", Expected: "201"},
		},
	}
}
