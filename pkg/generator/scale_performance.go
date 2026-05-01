package generator

import (
	"fmt"
	"net"
	"strings"

	"github.com/extremenetworks/testcase-generator/pkg/model"
)

// featureScaleLimits defines the maximum number of instances per feature.
// These come from NOS/device documentation, not YANG (which may be unbounded).
// Map key is feature name, value is [EXOS/SwitchEngine limit, VOSS/FabricEngine limit].
var featureScaleLimits = map[string][2]int{
	"radius-server": {32, 10},
}

// getScaleLimit returns the scale limit for a feature.
// If a specific limit is defined, it uses the lower of the two platform limits.
// Otherwise it falls back to the configured ScaleFactor.
func (g *Generator) getScaleLimit(featureName string) int {
	if limits, ok := featureScaleLimits[featureName]; ok {
		// Use the lower limit (VOSS) as the safe default for scale tests
		minLimit := limits[1]
		if limits[0] < limits[1] {
			minLimit = limits[0]
		}
		return minLimit
	}
	return g.config.ScaleFactor
}

// getMaxCapacityLimit returns the max capacity for a feature.
// Uses the higher platform limit if defined, otherwise 2x scale factor.
func (g *Generator) getMaxCapacityLimit(featureName string) int {
	if limits, ok := featureScaleLimits[featureName]; ok {
		// Use the higher limit (EXOS) for max capacity testing
		maxLimit := limits[0]
		if limits[1] > limits[0] {
			maxLimit = limits[1]
		}
		return maxLimit
	}
	return g.config.ScaleFactor * 2
}

// incrementUniqueBodyValues updates unique/key field values in the request body
// so that each instance in a scale/performance test has distinct values.
// It uses the feature's YANG key fields to determine which properties need
// unique values. For IP-address keys, the last octet is incremented from the
// base sample value (e.g., 8.8.8.8 → 8.8.8.9 → 8.8.8.10).
// For string keys, the index is appended. For numeric keys, the value is incremented.
func (g *Generator) incrementUniqueBodyValues(body map[string]interface{}, feature *model.Feature, index int) {
	keySet := make(map[string]bool, len(feature.Keys))
	for _, k := range feature.Keys {
		keySet[k] = true
	}

	// If no YANG keys are defined, fall back to heuristic matching
	if len(keySet) == 0 {
		keySet = guessKeyFields(feature)
	}

	// Handle deep-scanned body format (objects[].properties[])
	if objects, ok := body["objects"].([]interface{}); ok && len(objects) > 0 {
		if obj, ok := objects[0].(map[string]interface{}); ok {
			if props, ok := obj["properties"].([]map[string]interface{}); ok {
				for i, prop := range props {
					name, _ := prop["name"].(string)
					if !keySet[name] {
						continue
					}
					props[i]["value"] = incrementFieldValue(prop["value"], name, index)
				}
				obj["properties"] = props
			}
		}
	} else {
		// Simple body format
		for _, param := range feature.Parameters {
			if !keySet[param.Name] {
				continue
			}
			if val, ok := body[param.Name]; ok {
				body[param.Name] = incrementFieldValue(val, param.Name, index)
			}
		}
	}
}

// guessKeyFields returns a set of likely key fields by name heuristic,
// used when the Feature has no YANG keys parsed.
func guessKeyFields(feature *model.Feature) map[string]bool {
	keys := make(map[string]bool)
	for _, p := range feature.Parameters {
		nl := strings.ToLower(p.Name)
		if strings.Contains(nl, "server") || strings.Contains(nl, "address") ||
			strings.Contains(nl, "host") || strings.Contains(nl, "ip") {
			keys[p.Name] = true
		}
	}
	return keys
}

// incrementFieldValue returns a new value for the given field incremented by
// index.  IP addresses are incremented by last octet(s); integers are added;
// strings get the index appended.
func incrementFieldValue(baseValue interface{}, fieldName string, index int) interface{} {
	switch v := baseValue.(type) {
	case string:
		ip := net.ParseIP(v)
		if ip != nil && ip.To4() != nil {
			// Increment last octet(s) of the IPv4 address
			ip4 := ip.To4()
			offset := index
			ip4[3] = byte(int(ip4[3]) + offset%256)
			if int(ip4[3]) < int(ip.To4()[3]) { // wrapped
				ip4[2]++
			}
			return ip4.String()
		}
		// Non-IP string: append index
		if index == 0 {
			return v
		}
		return fmt.Sprintf("%s-%d", v, index)
	case int:
		return v + index
	case float64:
		return int(v) + index
	default:
		return baseValue
	}
}

// generateScaleTests generates scale test cases
func (g *Generator) generateScaleTests(feature *model.Feature, paths []*model.FeaturePath) []model.TestCase {
	var tests []model.TestCase

	var createPath, listPath, updatePath *model.FeaturePath
	for _, path := range paths {
		if path.OperationType == model.OperationTypeCreate {
			createPath = path
		} else if path.OperationType == model.OperationTypeUpdate {
			updatePath = path
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
	if deployPath != nil && createPath != nil && isConfigurationDeploymentPath(createPath) {
		tests = append(tests, g.generateMultiDeviceDeploymentTest(feature, createPath))
	}

	// Rapid updates test
	if createPath != nil && updatePath != nil {
		tests = append(tests, g.generateRapidUpdatesTest(feature, createPath, updatePath))
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
		Priority:    model.TestPriorityP3,
		Type:        model.TestCategoryScale,
		Steps:       []model.TestStep{},
		Metadata:    map[string]interface{}{},
	}

	scaleLimit := g.getScaleLimit(feature.Name)
	tc.Description = fmt.Sprintf("Create %d instances of %s", scaleLimit, feature.Name)
	tc.Metadata["instanceCount"] = scaleLimit

	// Create multiple instances with unique values
	for i := 0; i < scaleLimit; i++ {
		body := g.generateRequestBody(feature, createPath)
		g.setBodyResourceName(feature, body, fmt.Sprintf("TestResource-%d", i))
		g.incrementUniqueBodyValues(body, feature, i)

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
		Priority:    model.TestPriorityP3,
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
			g.setOrAddBodyParameter(body, param, items)
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
		tests = append(tests, g.generateUpdatePerformanceTest(feature, createPath, updatePath))
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
		tests = append(tests, g.generateDeletePerformanceTest(feature, createPath, deletePath))
	}

	// Deployment performance test
	var deployPath *model.FeaturePath
	for _, path := range paths {
		if path.OperationType == model.OperationTypeDeploy {
			deployPath = path
			break
		}
	}
	if deployPath != nil && createPath != nil && isConfigurationDeploymentPath(createPath) {
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
		Priority:    model.TestPriorityP4,
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
		g.setBodyResourceName(feature, body, fmt.Sprintf("PerfTest-%d", i))
		g.incrementUniqueBodyValues(body, feature, i)

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
		Priority:    model.TestPriorityP4,
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
			PathParams:     pathParamsFor(readPath.Path, "TestProfile"),
			Body:           g.generateReadBody(feature, readPath),
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
		Priority:    model.TestPriorityP4,
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
		Priority:    model.TestPriorityP3,
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
	g.setBodyResourceName(feature, profileBody, "MultiDeviceProfile")

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
			Path:        "/configuration-profile/{name}/devices/deploy",
			PathParams:  map[string]string{"name": "MultiDeviceProfile"},
			Body: map[string]interface{}{
				"devices":   []string{fmt.Sprintf("device-%d", i)},
				"deployNow": true,
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
	updatePath *model.FeaturePath,
) model.TestCase {
	tc := model.TestCase{
		TestCaseID:  g.nextTestID(),
		FeatureName: feature.Name,
		Priority:    model.TestPriorityP3,
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
	g.setBodyResourceName(feature, body, "RapidUpdateTest")

	createStep := model.TestStep{
		Name:           "createResource",
		Description:    "Create initial resource",
		Method:         createPath.HTTPMethod,
		API:            model.APITypeREST,
		Path:           createPath.Path,
		Body:           body,
		ExpectedStatus: 201,
		Validations: withObjectIDCapture([]model.Validation{
			{
				Type:     model.ValidationTypeStatusCode,
				Expected: 201,
			},
		}),
	}
	tc.Steps = append(tc.Steps, createStep)

	// Perform rapid updates
	for i := 0; i < 10; i++ {
		updateBody := g.generateUpdateRequestBody(feature, updatePath)
		g.setBodyResourceName(feature, updateBody, "RapidUpdateTest")
		if param, ok := findFeatureParameter(feature, "description"); ok {
			g.setOrAddBodyParameter(updateBody, param, fmt.Sprintf("Update iteration %d", i))
		}

		updateStep := model.TestStep{
			Name:           fmt.Sprintf("rapidUpdate%d", i),
			Description:    fmt.Sprintf("Rapid update %d", i),
			Method:         updatePath.HTTPMethod,
			API:            model.APITypeREST,
			Path:           updatePath.Path,
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
		Priority:    model.TestPriorityP3,
		Type:        model.TestCategoryScale,
		Description: fmt.Sprintf("Test maximum capacity for %s (create to maximum allowed)", feature.Name),
		Steps:       []model.TestStep{},
		Metadata: map[string]interface{}{
			"testType": "max-capacity",
		},
	}

	// Create maximum number of instances using feature-specific limits
	maxInstances := g.getMaxCapacityLimit(feature.Name)
	tc.Description = fmt.Sprintf("Test maximum capacity for %s (create %d instances)", feature.Name, maxInstances)
	tc.Metadata["maxInstances"] = maxInstances
	for i := 0; i < maxInstances; i++ {
		body := g.generateRequestBody(feature, createPath)
		g.setBodyResourceName(feature, body, fmt.Sprintf("MaxCapacityTest-%d", i))
		g.incrementUniqueBodyValues(body, feature, i)

		// If feature has priority parameter, use different priorities
		if param, ok := findFeatureParameter(feature, "priority"); ok {
			g.setOrAddBodyParameter(body, param, i%10+1) // Cycle through priorities 1-10
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
	updatePath *model.FeaturePath,
) model.TestCase {
	tc := model.TestCase{
		TestCaseID:  g.nextTestID(),
		FeatureName: feature.Name,
		Priority:    model.TestPriorityP3,
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
	g.setBodyResourceName(feature, body, "UpdatePerfTest")

	createStep := model.TestStep{
		Name:           "createResource",
		Description:    "Create initial resource for performance test",
		Method:         createPath.HTTPMethod,
		API:            model.APITypeREST,
		Path:           createPath.Path,
		Body:           body,
		ExpectedStatus: 201,
		Validations: withObjectIDCapture([]model.Validation{
			{
				Type:     model.ValidationTypeStatusCode,
				Expected: 201,
			},
		}),
	}
	tc.Steps = append(tc.Steps, createStep)

	// Perform update performance test
	for i := 0; i < g.config.PerformanceIterations; i++ {
		updateBody := g.generateUpdateRequestBody(feature, updatePath)
		g.setBodyResourceName(feature, updateBody, "UpdatePerfTest")
		if param, ok := findFeatureParameter(feature, "description"); ok {
			g.setOrAddBodyParameter(updateBody, param, fmt.Sprintf("Performance test iteration %d", i))
		}

		updateStep := model.TestStep{
			Name:           fmt.Sprintf("update%d", i),
			Description:    fmt.Sprintf("Update iteration %d", i),
			Method:         updatePath.HTTPMethod,
			API:            model.APITypeREST,
			Path:           updatePath.Path,
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
	deletePath *model.FeaturePath,
) model.TestCase {
	tc := model.TestCase{
		TestCaseID:  g.nextTestID(),
		FeatureName: feature.Name,
		Priority:    model.TestPriorityP3,
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
		g.setBodyResourceName(feature, body, fmt.Sprintf("DeletePerfTest-%d", i))
		g.incrementUniqueBodyValues(body, feature, i)

		captureName := fmt.Sprintf("OBJECT_ID_DELETE_%d", i)
		createStep := model.TestStep{
			Name:           fmt.Sprintf("create%d", i),
			Description:    fmt.Sprintf("Create resource %d for delete test", i),
			Method:         createPath.HTTPMethod,
			API:            model.APITypeREST,
			Path:           createPath.Path,
			Body:           body,
			ExpectedStatus: 201,
			Validations: withNamedObjectIDCapture([]model.Validation{
				{
					Type:     model.ValidationTypeStatusCode,
					Expected: 201,
				},
			}, captureName),
		}
		tc.Steps = append(tc.Steps, createStep)

		// Delete resource
		deleteBody := g.generateDeleteBodyForObjectID(feature, deletePath, captureName)
		deleteStep := model.TestStep{
			Name:           fmt.Sprintf("delete%d", i),
			Description:    fmt.Sprintf("Delete resource %d", i),
			Method:         deletePath.HTTPMethod,
			API:            model.APITypeREST,
			Path:           deletePath.Path,
			Body:           deleteBody,
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
		Priority:    model.TestPriorityP3,
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
	g.setBodyResourceName(feature, profileBody, "DeploymentPerfTest")

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
			Path:        "/configuration-profile/{name}/devices/deploy",
			PathParams:  map[string]string{"name": "DeploymentPerfTest"},
			Body: map[string]interface{}{
				"devices":   []string{fmt.Sprintf("perf-device-%d", i)},
				"deployNow": true,
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
