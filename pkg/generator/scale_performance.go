package generator

import (
	"fmt"

	"github.com/extremenetworks/testcase-generator/pkg/model"
)

// generateScaleTests generates scale test cases
func (g *Generator) generateScaleTests(feature *model.Feature, paths []*model.FeaturePath) []model.TestCase {
	var tests []model.TestCase

	var createPath, listPath *model.FeaturePath
	for _, path := range paths {
		if path.OperationType == model.OperationTypeCreate {
			createPath = path
		} else if path.OperationType == model.OperationTypeList {
			listPath = path
		}
	}

	if createPath != nil {
		tests = append(tests, g.generateMultipleInstancesTest(feature, createPath, listPath))
	}

	if feature.IsListType && createPath != nil {
		tests = append(tests, g.generateLargeListTest(feature, createPath))
	}

	// Additional scale tests for better coverage

	// Multi-device deployment test
	var deployPath *model.FeaturePath
	for _, path := range paths {
		if path.OperationType == model.OperationTypeDeploy {
			deployPath = path
			break
		}
	}
	if deployPath != nil && createPath != nil {
		tests = append(tests, g.generateMultiDeviceDeploymentTest(feature, createPath))
	}

	// Rapid updates test
	if createPath != nil {
		tests = append(tests, g.generateRapidUpdatesTest(feature, createPath))
	}

	// Maximum capacity test (test all priority slots, etc.)
	if createPath != nil {
		tests = append(tests, g.generateMaxCapacityTest(feature, createPath))
	}

	return tests
}

// generateMultipleInstancesTest generates test with multiple instances
func (g *Generator) generateMultipleInstancesTest(
	feature *model.Feature,
	createPath, listPath *model.FeaturePath,
) model.TestCase {

	tc := model.TestCase{
		TestCaseID:  g.nextTestID(),
		FeatureName: feature.Name,
		Priority:    model.TestPriorityP2,
		Type:        model.TestCategoryScale,
		Description: fmt.Sprintf("Create %d instances of %s", g.config.ScaleFactor, feature.Name),
		Steps:       []model.TestStep{},
		Metadata: map[string]interface{}{
			"instanceCount": g.config.ScaleFactor,
		},
	}

	// Create multiple instances
	for i := 0; i < g.config.ScaleFactor; i++ {
		body := g.generateRequestBody(feature, createPath)
		body["name"] = fmt.Sprintf("TestResource-%d", i)

		step := model.TestStep{
			Name:           fmt.Sprintf("createInstance%d", i),
			Description:    fmt.Sprintf("Create instance %d", i),
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
	}

	// List all instances to verify
	if listPath != nil {
		listStep := model.TestStep{
			Name:           "listAllInstances",
			Description:    "List all created instances",
			Method:         listPath.HTTPMethod,
			API:            model.APITypeREST,
			Path:           listPath.Path,
			ExpectedStatus: 200,
			Validations: []model.Validation{
				{
					Type:     model.ValidationTypeStatusCode,
					Expected: 200,
				},
			},
		}
		tc.Steps = append(tc.Steps, listStep)
	}

	return tc
}

// generateLargeListTest generates test with large list in single object
func (g *Generator) generateLargeListTest(
	feature *model.Feature,
	createPath *model.FeaturePath,
) model.TestCase {

	tc := model.TestCase{
		TestCaseID:  g.nextTestID(),
		FeatureName: feature.Name,
		Priority:    model.TestPriorityP2,
		Type:        model.TestCategoryScale,
		Description: fmt.Sprintf("Create %s with large list (%d items)", feature.Name, g.config.ScaleFactor),
		Steps:       []model.TestStep{},
		Metadata: map[string]interface{}{
			"listSize": g.config.ScaleFactor,
		},
	}

	body := g.generateRequestBody(feature, createPath)

	// Find list parameters and populate with many items
	for _, param := range feature.Parameters {
		if param.IsArray {
			items := make([]interface{}, g.config.ScaleFactor)
			for i := 0; i < g.config.ScaleFactor; i++ {
				items[i] = fmt.Sprintf("item-%d", i)
			}
			body[param.Name] = items
		}
	}

	step := model.TestStep{
		Name:           "createWithLargeList",
		Description:    fmt.Sprintf("Create with %d items", g.config.ScaleFactor),
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

// generatePerformanceTests generates performance test cases
func (g *Generator) generatePerformanceTests(feature *model.Feature, paths []*model.FeaturePath) []model.TestCase {
	var tests []model.TestCase

	var createPath, readPath, listPath *model.FeaturePath
	for _, path := range paths {
		switch path.OperationType {
		case model.OperationTypeCreate:
			createPath = path
		case model.OperationTypeRead:
			readPath = path
		case model.OperationTypeList:
			listPath = path
		}
	}

	if createPath != nil {
		tests = append(tests, g.generateCreatePerformanceTest(feature, createPath))
	}

	if readPath != nil {
		tests = append(tests, g.generateReadPerformanceTest(feature, readPath))
	}

	if listPath != nil {
		tests = append(tests, g.generateListPerformanceTest(feature, listPath))
	}

	// Additional performance tests

	// Update performance test
	var updatePath *model.FeaturePath
	for _, path := range paths {
		if path.OperationType == model.OperationTypeUpdate {
			updatePath = path
			break
		}
	}
	if updatePath != nil && createPath != nil {
		tests = append(tests, g.generateUpdatePerformanceTest(feature, createPath))
	}

	// Delete performance test
	var deletePath *model.FeaturePath
	for _, path := range paths {
		if path.OperationType == model.OperationTypeDelete {
			deletePath = path
			break
		}
	}
	if deletePath != nil && createPath != nil {
		tests = append(tests, g.generateDeletePerformanceTest(feature, createPath))
	}

	// Deployment performance test
	var deployPath *model.FeaturePath
	for _, path := range paths {
		if path.OperationType == model.OperationTypeDeploy {
			deployPath = path
			break
		}
	}
	if deployPath != nil && createPath != nil {
		tests = append(tests, g.generateDeploymentPerformanceTest(feature, createPath))
	}

	return tests
}

// generateCreatePerformanceTest generates performance test for create operations
func (g *Generator) generateCreatePerformanceTest(
	feature *model.Feature,
	createPath *model.FeaturePath,
) model.TestCase {

	tc := model.TestCase{
		TestCaseID:  g.nextTestID(),
		FeatureName: feature.Name,
		Priority:    model.TestPriorityP3,
		Type:        model.TestCategoryPerformance,
		Description: fmt.Sprintf("Performance test: create %d instances of %s", g.config.PerformanceIterations, feature.Name),
		Steps:       []model.TestStep{},
		Metadata: map[string]interface{}{
			"iterations":  g.config.PerformanceIterations,
			"concurrency": g.config.PerformanceConcurrency,
			"operation":   "create",
		},
	}

	for i := 0; i < g.config.PerformanceIterations; i++ {
		body := g.generateRequestBody(feature, createPath)
		body["name"] = fmt.Sprintf("PerfTest-%d", i)

		step := model.TestStep{
			Name:           fmt.Sprintf("create%d", i),
			Description:    fmt.Sprintf("Create iteration %d", i),
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
				{
					Type:        model.ValidationTypeResponseTimeUnder,
					Expected:    5000, // 5 seconds
					Description: "Response time should be under 5 seconds",
				},
			},
			Timeout: 10,
		}
		tc.Steps = append(tc.Steps, step)
	}

	return tc
}

// generateReadPerformanceTest generates performance test for read operations
func (g *Generator) generateReadPerformanceTest(
	feature *model.Feature,
	readPath *model.FeaturePath,
) model.TestCase {

	tc := model.TestCase{
		TestCaseID:  g.nextTestID(),
		FeatureName: feature.Name,
		Priority:    model.TestPriorityP3,
		Type:        model.TestCategoryPerformance,
		Description: fmt.Sprintf("Performance test: read %s %d times", feature.Name, g.config.PerformanceIterations),
		Steps:       []model.TestStep{},
		Metadata: map[string]interface{}{
			"iterations":  g.config.PerformanceIterations,
			"concurrency": g.config.PerformanceConcurrency,
			"operation":   "read",
		},
	}

	for i := 0; i < g.config.PerformanceIterations; i++ {
		step := model.TestStep{
			Name:           fmt.Sprintf("read%d", i),
			Description:    fmt.Sprintf("Read iteration %d", i),
			Method:         readPath.HTTPMethod,
			API:            model.APITypeREST,
			Path:           readPath.Path,
			PathParams:     map[string]string{"name": "TestResource"},
			ExpectedStatus: 200,
			Validations: []model.Validation{
				{
					Type:     model.ValidationTypeStatusCode,
					Expected: 200,
				},
				{
					Type:        model.ValidationTypeResponseTimeUnder,
					Expected:    1000, // 1 second
					Description: "Response time should be under 1 second",
				},
			},
			Timeout: 5,
		}
		tc.Steps = append(tc.Steps, step)
	}

	return tc
}

// generateListPerformanceTest generates performance test for list operations
func (g *Generator) generateListPerformanceTest(
	feature *model.Feature,
	listPath *model.FeaturePath,
) model.TestCase {

	tc := model.TestCase{
		TestCaseID:  g.nextTestID(),
		FeatureName: feature.Name,
		Priority:    model.TestPriorityP3,
		Type:        model.TestCategoryPerformance,
		Description: fmt.Sprintf("Performance test: list %s %d times", feature.Name, g.config.PerformanceIterations),
		Steps:       []model.TestStep{},
		Metadata: map[string]interface{}{
			"iterations":  g.config.PerformanceIterations,
			"concurrency": g.config.PerformanceConcurrency,
			"operation":   "list",
		},
	}

	for i := 0; i < g.config.PerformanceIterations; i++ {
		step := model.TestStep{
			Name:           fmt.Sprintf("list%d", i),
			Description:    fmt.Sprintf("List iteration %d", i),
			Method:         listPath.HTTPMethod,
			API:            model.APITypeREST,
			Path:           listPath.Path,
			ExpectedStatus: 200,
			Validations: []model.Validation{
				{
					Type:     model.ValidationTypeStatusCode,
					Expected: 200,
				},
				{
					Type:        model.ValidationTypeResponseTimeUnder,
					Expected:    3000, // 3 seconds
					Description: "Response time should be under 3 seconds",
				},
			},
			Timeout: 10,
		}
		tc.Steps = append(tc.Steps, step)
	}

	return tc
}

// generateMultiDeviceDeploymentTest generates a test for multi-device deployment scale
func (g *Generator) generateMultiDeviceDeploymentTest(
	feature *model.Feature,
	createPath *model.FeaturePath,
) model.TestCase {
	tc := model.TestCase{
		TestCaseID:  g.nextTestID(),
		FeatureName: feature.Name,
		Priority:    model.TestPriorityP2,
		Type:        model.TestCategoryScale,
		Description: fmt.Sprintf("Deploy %s to multiple devices simultaneously", feature.Name),
		Steps:       []model.TestStep{},
		Metadata: map[string]interface{}{
			"deviceCount": g.config.ScaleFactor,
			"testType":    "multi-device-deployment",
		},
	}

	// Create profile
	profileBody := g.generateRequestBody(feature, createPath)
	profileBody["name"] = "MultiDeviceProfile"

	createStep := model.TestStep{
		Name:           "createProfile",
		Description:    "Create configuration profile",
		Method:         createPath.HTTPMethod,
		API:            model.APITypeREST,
		Path:           createPath.Path,
		Body:           profileBody,
		ExpectedStatus: 201,
		Validations: []model.Validation{
			{
				Type:     model.ValidationTypeStatusCode,
				Expected: 201,
			},
		},
	}
	tc.Steps = append(tc.Steps, createStep)

	// Deploy to multiple devices
	for i := 0; i < g.config.ScaleFactor; i++ {
		deployStep := model.TestStep{
			Name:        fmt.Sprintf("deployToDevice%d", i),
			Description: fmt.Sprintf("Deploy to device %d", i),
			Method:      "POST",
			API:         model.APITypeREST,
			Path:        "/api/v1/deployment",
			Body: map[string]interface{}{
				"profileName": "MultiDeviceProfile",
				"deviceName":  fmt.Sprintf("device-%d", i),
			},
			ExpectedStatus: 200,
			Validations: []model.Validation{
				{
					Type:     model.ValidationTypeStatusCode,
					Expected: 200,
				},
			},
		}
		tc.Steps = append(tc.Steps, deployStep)
	}

	return tc
}

// generateRapidUpdatesTest generates a test for rapid consecutive updates
func (g *Generator) generateRapidUpdatesTest(
	feature *model.Feature,
	createPath *model.FeaturePath,
) model.TestCase {
	tc := model.TestCase{
		TestCaseID:  g.nextTestID(),
		FeatureName: feature.Name,
		Priority:    model.TestPriorityP2,
		Type:        model.TestCategoryScale,
		Description: fmt.Sprintf("Test rapid consecutive updates to %s", feature.Name),
		Steps:       []model.TestStep{},
		Metadata: map[string]interface{}{
			"updateCount": 10,
			"testType":    "rapid-updates",
		},
	}

	// Create initial resource
	body := g.generateRequestBody(feature, createPath)
	body["name"] = "RapidUpdateTest"

	createStep := model.TestStep{
		Name:           "createResource",
		Description:    "Create initial resource",
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

	// Perform rapid updates
	for i := 0; i < 10; i++ {
		updateBody := g.generateRequestBody(feature, createPath)
		updateBody["name"] = "RapidUpdateTest"
		updateBody["description"] = fmt.Sprintf("Update iteration %d", i)

		updateStep := model.TestStep{
			Name:           fmt.Sprintf("rapidUpdate%d", i),
			Description:    fmt.Sprintf("Rapid update %d", i),
			Method:         "PUT",
			API:            model.APITypeREST,
			Path:           createPath.Path,
			Body:           updateBody,
			ExpectedStatus: 200,
			Validations: []model.Validation{
				{
					Type:     model.ValidationTypeStatusCode,
					Expected: 200,
				},
			},
		}
		tc.Steps = append(tc.Steps, updateStep)
	}

	return tc
}

// generateMaxCapacityTest generates a test for maximum capacity (e.g., all priority slots)
func (g *Generator) generateMaxCapacityTest(
	feature *model.Feature,
	createPath *model.FeaturePath,
) model.TestCase {
	tc := model.TestCase{
		TestCaseID:  g.nextTestID(),
		FeatureName: feature.Name,
		Priority:    model.TestPriorityP2,
		Type:        model.TestCategoryScale,
		Description: fmt.Sprintf("Test maximum capacity for %s (create to maximum allowed)", feature.Name),
		Steps:       []model.TestStep{},
		Metadata: map[string]interface{}{
			"maxInstances": g.config.ScaleFactor * 2,
			"testType":     "max-capacity",
		},
	}

	// Create maximum number of instances
	maxInstances := g.config.ScaleFactor * 2 // Use 2x scale factor for max capacity
	for i := 0; i < maxInstances; i++ {
		body := g.generateRequestBody(feature, createPath)
		body["name"] = fmt.Sprintf("MaxCapacityTest-%d", i)

		// If feature has priority parameter, use different priorities
		if hasParameter(feature, "priority") {
			body["priority"] = i%10 + 1 // Cycle through priorities 1-10
		}

		step := model.TestStep{
			Name:           fmt.Sprintf("createInstance%d", i),
			Description:    fmt.Sprintf("Create instance %d for max capacity test", i),
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
	}

	return tc
}

// generateUpdatePerformanceTest generates a performance test for update operations
func (g *Generator) generateUpdatePerformanceTest(
	feature *model.Feature,
	createPath *model.FeaturePath,
) model.TestCase {
	tc := model.TestCase{
		TestCaseID:  g.nextTestID(),
		FeatureName: feature.Name,
		Priority:    model.TestPriorityP2,
		Type:        model.TestCategoryPerformance,
		Description: fmt.Sprintf("Performance test for %s update operations", feature.Name),
		Steps:       []model.TestStep{},
		Metadata: map[string]interface{}{
			"iterations": g.config.PerformanceIterations,
			"operation":  "update",
		},
	}

	// Create initial resource
	body := g.generateRequestBody(feature, createPath)
	body["name"] = "UpdatePerfTest"

	createStep := model.TestStep{
		Name:           "createResource",
		Description:    "Create initial resource for performance test",
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

	// Perform update performance test
	for i := 0; i < g.config.PerformanceIterations; i++ {
		updateBody := g.generateRequestBody(feature, createPath)
		updateBody["name"] = "UpdatePerfTest"
		updateBody["description"] = fmt.Sprintf("Performance test iteration %d", i)

		updateStep := model.TestStep{
			Name:           fmt.Sprintf("update%d", i),
			Description:    fmt.Sprintf("Update iteration %d", i),
			Method:         "PUT",
			API:            model.APITypeREST,
			Path:           createPath.Path,
			Body:           updateBody,
			ExpectedStatus: 200,
			Validations: []model.Validation{
				{
					Type:     model.ValidationTypeStatusCode,
					Expected: 200,
				},
				{
					Type:        model.ValidationTypeResponseTimeUnder,
					Expected:    2000, // 2 seconds
					Description: "Update should complete within 2 seconds",
				},
			},
			Timeout: 10,
		}
		tc.Steps = append(tc.Steps, updateStep)
	}

	return tc
}

// generateDeletePerformanceTest generates a performance test for delete operations
func (g *Generator) generateDeletePerformanceTest(
	feature *model.Feature,
	createPath *model.FeaturePath,
) model.TestCase {
	tc := model.TestCase{
		TestCaseID:  g.nextTestID(),
		FeatureName: feature.Name,
		Priority:    model.TestPriorityP2,
		Type:        model.TestCategoryPerformance,
		Description: fmt.Sprintf("Performance test for %s delete operations", feature.Name),
		Steps:       []model.TestStep{},
		Metadata: map[string]interface{}{
			"iterations": g.config.PerformanceIterations,
			"operation":  "delete",
		},
	}

	// Create and delete multiple resources to test delete performance
	for i := 0; i < g.config.PerformanceIterations; i++ {
		// Create resource
		body := g.generateRequestBody(feature, createPath)
		body["name"] = fmt.Sprintf("DeletePerfTest-%d", i)

		createStep := model.TestStep{
			Name:           fmt.Sprintf("create%d", i),
			Description:    fmt.Sprintf("Create resource %d for delete test", i),
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

		// Delete resource
		deleteStep := model.TestStep{
			Name:           fmt.Sprintf("delete%d", i),
			Description:    fmt.Sprintf("Delete resource %d", i),
			Method:         "DELETE",
			API:            model.APITypeREST,
			Path:           createPath.Path,
			ExpectedStatus: 200,
			Validations: []model.Validation{
				{
					Type:     model.ValidationTypeStatusCode,
					Expected: 200,
				},
				{
					Type:        model.ValidationTypeResponseTimeUnder,
					Expected:    2000, // 2 seconds
					Description: "Delete should complete within 2 seconds",
				},
			},
			Timeout: 10,
		}
		tc.Steps = append(tc.Steps, deleteStep)
	}

	return tc
}

// generateDeploymentPerformanceTest generates a performance test for deployment operations
func (g *Generator) generateDeploymentPerformanceTest(
	feature *model.Feature,
	createPath *model.FeaturePath,
) model.TestCase {
	tc := model.TestCase{
		TestCaseID:  g.nextTestID(),
		FeatureName: feature.Name,
		Priority:    model.TestPriorityP2,
		Type:        model.TestCategoryPerformance,
		Description: fmt.Sprintf("Performance test for %s deployment operations", feature.Name),
		Steps:       []model.TestStep{},
		Metadata: map[string]interface{}{
			"iterations": g.config.PerformanceIterations,
			"operation":  "deployment",
		},
	}

	// Create profile
	profileBody := g.generateRequestBody(feature, createPath)
	profileBody["name"] = "DeploymentPerfTest"

	createStep := model.TestStep{
		Name:           "createProfile",
		Description:    "Create profile for deployment performance test",
		Method:         createPath.HTTPMethod,
		API:            model.APITypeREST,
		Path:           createPath.Path,
		Body:           profileBody,
		ExpectedStatus: 201,
		Validations: []model.Validation{
			{
				Type:     model.ValidationTypeStatusCode,
				Expected: 201,
			},
		},
	}
	tc.Steps = append(tc.Steps, createStep)

	// Perform multiple deployments
	for i := 0; i < g.config.PerformanceIterations; i++ {
		deployStep := model.TestStep{
			Name:        fmt.Sprintf("deploy%d", i),
			Description: fmt.Sprintf("Deployment iteration %d", i),
			Method:      "POST",
			API:         model.APITypeREST,
			Path:        "/api/v1/deployment",
			Body: map[string]interface{}{
				"profileName": "DeploymentPerfTest",
				"deviceName":  fmt.Sprintf("perf-device-%d", i),
			},
			ExpectedStatus: 200,
			Validations: []model.Validation{
				{
					Type:     model.ValidationTypeStatusCode,
					Expected: 200,
				},
				{
					Type:        model.ValidationTypeResponseTimeUnder,
					Expected:    5000, // 5 seconds
					Description: "Deployment should complete within 5 seconds",
				},
			},
			Timeout: 30,
		}
		tc.Steps = append(tc.Steps, deployStep)
	}

	return tc
}

// hasParameter checks if a feature has a specific parameter
func hasParameter(feature *model.Feature, paramName string) bool {
	if feature.Parameters == nil {
		return false
	}
	for _, param := range feature.Parameters {
		if param.Name == paramName {
			return true
		}
	}
	return false
}
