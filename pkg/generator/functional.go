package generator

import (
	"fmt"
	"strings"

	"github.com/extremenetworks/testcase-generator/pkg/model"
)

// generateFunctionalTests generates functional test cases
func (g *Generator) generateFunctionalTests(feature *model.Feature, paths []*model.FeaturePath) []model.TestCase {
	var tests []model.TestCase

	// Find CRUD operations.
	// Prefer paths with BlueprintCategory set (explicitly registered for this feature via
	// exact FeatureName match) over paths that arrived via fuzzy matching.
	var createPath, readPath, updatePath, deletePath *model.FeaturePath
	var scopePaths, targetPaths, deployPaths []*model.FeaturePath

	preferPath := func(current, candidate *model.FeaturePath) *model.FeaturePath {
		if current == nil {
			return candidate
		}
		// Explicit (BlueprintCategory != "") beats fuzzy (BlueprintCategory == "")
		if candidate.BlueprintCategory != "" && current.BlueprintCategory == "" {
			return candidate
		}
		return current
	}

	for _, path := range paths {
		switch path.OperationType {
		case model.OperationTypeCreate:
			createPath = preferPath(createPath, path)
		case model.OperationTypeRead:
			readPath = preferPath(readPath, path)
		case model.OperationTypeUpdate:
			updatePath = preferPath(updatePath, path)
		case model.OperationTypeDelete:
			deletePath = preferPath(deletePath, path)
		case model.OperationTypeScope:
			scopePaths = append(scopePaths, path)
		case model.OperationTypeTarget:
			targetPaths = append(targetPaths, path)
		case model.OperationTypeDeploy:
			deployPaths = append(deployPaths, path)
		}
	}

	// Generate basic CRUD tests (non-deployment)
	if createPath != nil {
		tests = append(tests, g.generateBasicCreateTest(feature, createPath, readPath))
	}

	if updatePath != nil && readPath != nil {
		tests = append(tests, g.generateBasicUpdateTest(feature, updatePath, readPath))
	}

	if deletePath != nil {
		tests = append(tests, g.generateBasicDeleteTest(feature, deletePath))
	}

	// Additional non-deployment functional CRUD tests for better coverage

	// Full CRUD lifecycle test (Create -> Read -> Update -> Read -> Delete)
	if createPath != nil && readPath != nil && updatePath != nil && deletePath != nil {
		tests = append(tests, g.generateFullCRUDLifecycleTest(feature, createPath, readPath, updatePath, deletePath))
	}

	// List all resources test
	var listPath *model.FeaturePath
	for _, path := range paths {
		if path.OperationType == model.OperationTypeList {
			listPath = path
			break
		}
	}
	if listPath != nil {
		tests = append(tests, g.generateListAllResourcesTest(feature, createPath, listPath))
	}

	// Partial update test (only update specific fields)
	if createPath != nil && updatePath != nil && readPath != nil {
		tests = append(tests, g.generatePartialUpdateTest(feature, createPath, updatePath, readPath))
	}

	// Idempotent create test (creating same resource twice)
	if createPath != nil {
		tests = append(tests, g.generateIdempotentCreateTest(feature, createPath))
	}

	// Generate deployment tests for configuration profiles
	if createPath != nil && feature.Parameters != nil && len(feature.Parameters) > 0 {
		// Generate full deployment test with all steps (scope, target, deploy, verify)
		tests = append(tests, g.generateFullDeploymentTest(
			feature, createPath, readPath,
			model.ScopeTypeDevice, model.TargetTypeDevice,
		))

		// Generate site-group deployment if supported
		if hasScopeType(scopePaths, model.ScopeTypeSiteGroup) {
			tests = append(tests, g.generateFullDeploymentTest(
				feature, createPath, readPath,
				model.ScopeTypeSiteGroup, model.TargetTypeSiteGroup,
			))
		}

		// Generate simplified deployment test (create and verify only, no scope/target/deploy)
		tests = append(tests, g.generateSimplifiedDeploymentTest(
			feature, createPath,
			model.ScopeTypeDevice,
		))
	}

	// YANG model coverage: non-deployment tests for required-only, all-fields, datatypes, defaults
	if createPath != nil {
		// Create with required fields only (verify optional fields are truly optional)
		tests = append(tests, g.generateCreateWithRequiredFieldsOnlyTest(feature, createPath, readPath))

		// Create with all fields including optional (verify all fields accepted)
		tests = append(tests, g.generateCreateWithAllFieldsTest(feature, createPath, readPath))

		// Per optional field: one test WITH the field, one test WITHOUT the field
		// (the without-case is covered in negative tests; the with-case is here)
		tests = append(tests, g.generateOptionalFieldInclusionTests(feature, createPath, readPath)...)

		// YANG datatype non-deployment tests (one per distinct type)
		tests = append(tests, g.generateYANGDatatypeNonDeploymentTests(feature, createPath, readPath)...)
	}

	// Default value validation tests (non-deployment): omit defaulted field, read back, verify default
	if createPath != nil && readPath != nil {
		tests = append(tests, g.generateDefaultValueTests(feature, createPath, readPath)...)
	}

	// Generate tests for sub-object types (e.g., dns-suffix under dns-server)
	if feature.SubObjectTypes != nil && len(feature.SubObjectTypes) > 0 {
		for subObjName, subObjType := range feature.SubObjectTypes {
			// Create test for sub-object type
			if createPath != nil {
				tests = append(tests, g.generateSubObjectCreateTest(feature, subObjType, subObjName, createPath, readPath))
			}
			// Update test for sub-object type
			if updatePath != nil {
				tests = append(tests, g.generateSubObjectUpdateTest(feature, subObjType, subObjName, updatePath, readPath))
			}
			// Delete test for sub-object type
			if deletePath != nil {
				tests = append(tests, g.generateSubObjectDeleteTest(feature, subObjType, subObjName, deletePath))
			}
		}
	}

	return tests
}

// generateBasicCreateTest generates a basic create test
func (g *Generator) generateBasicCreateTest(feature *model.Feature, createPath, readPath *model.FeaturePath) model.TestCase {
	tc := model.TestCase{
		TestCaseID:       g.nextTestID(),
		FeatureName:      feature.Name,
		Priority:         model.TestPriorityP1,
		Type:             model.TestCategoryFunctional,
		Description:      fmt.Sprintf("Create a %s and verify it exists", feature.Name),
		IsDeploymentTest: false,
		Steps:            []model.TestStep{},
	}

	// Create step
	createStep := model.TestStep{
		Name:           "createResource",
		Description:    fmt.Sprintf("Create %s", feature.Name),
		Method:         createPath.HTTPMethod,
		API:            model.APITypeREST,
		Path:           createPath.Path,
		Body:           g.generateRequestBody(feature, createPath),
		ExpectedStatus: 201,
		Validations:    g.generateResponseValidations(feature, createPath, 201),
	}
	tc.Steps = append(tc.Steps, createStep)

	// Read step to verify
	if readPath != nil {
		readStep := model.TestStep{
			Name:           "verifyCreation",
			Description:    "Verify resource was created",
			Method:         readPath.HTTPMethod,
			API:            model.APITypeREST,
			Path:           readPath.Path,
			PathParams:     map[string]string{"name": "TestResource"},
			Body:           g.generateReadBody(feature, createPath),
			ExpectedStatus: 200,
			Validations:    g.generateResponseValidations(feature, readPath, 200),
		}
		tc.Steps = append(tc.Steps, readStep)
	}

	return tc
}

// generateBasicUpdateTest generates a basic update test
// This test is INDEPENDENT - it creates the resource first, then updates it
func (g *Generator) generateBasicUpdateTest(feature *model.Feature, updatePath, readPath *model.FeaturePath) model.TestCase {
	tc := model.TestCase{
		TestCaseID:       g.nextTestID(),
		FeatureName:      feature.Name,
		Priority:         model.TestPriorityP2,
		Type:             model.TestCategoryFunctional,
		Description:      fmt.Sprintf("Create, update, and verify %s (independent test)", feature.Name),
		IsDeploymentTest: false,
		Steps:            []model.TestStep{},
	}

	// Step 1: Create the resource first (making this test independent)
	createBody := g.generateRequestBody(feature, updatePath)
	keyDesc := describeKeyValues(createBody, feature)
	createStep := model.TestStep{
		Name:           "createResourceForUpdate",
		Description:    fmt.Sprintf("Create %s with %s before updating", feature.Name, keyDesc),
		Method:         "POST",
		API:            model.APITypeREST,
		Path:           updatePath.Path, // Use same path, POST will create
		Body:           createBody,
		ExpectedStatus: 201,
		Validations:    crudValidations(201, "create", feature.Name, keyDesc),
	}
	tc.Steps = append(tc.Steps, createStep)

	// Step 2: Update the resource
	updateBody := g.generateUpdateRequestBody(feature, updatePath)
	updateKeyDesc := describeModifiedFields(updateBody, feature)
	updateStep := model.TestStep{
		Name:           "updateResource",
		Description:    fmt.Sprintf("Update %s — %s", feature.Name, updateKeyDesc),
		Method:         updatePath.HTTPMethod,
		API:            model.APITypeREST,
		Path:           updatePath.Path,
		Body:           updateBody,
		ExpectedStatus: 200,
		Validations:    crudValidations(200, "update", feature.Name, updateKeyDesc),
	}
	tc.Steps = append(tc.Steps, updateStep)

	// Step 3: Verify update
	if readPath != nil {
		verifyStep := model.TestStep{
			Name:           "verifyUpdate",
			Description:    fmt.Sprintf("Verify %s was updated — %s", feature.Name, updateKeyDesc),
			Method:         readPath.HTTPMethod,
			API:            model.APITypeREST,
			Path:           readPath.Path,
			Body:           g.generateReadBody(feature, updatePath),
			ExpectedStatus: 200,
			Validations:    crudValidations(200, "read", feature.Name, updateKeyDesc),
		}
		tc.Steps = append(tc.Steps, verifyStep)
	}

	return tc
}

// generateBasicDeleteTest generates a basic delete test
// This test is INDEPENDENT - it creates the resource first, then deletes it
func (g *Generator) generateBasicDeleteTest(feature *model.Feature, deletePath *model.FeaturePath) model.TestCase {
	tc := model.TestCase{
		TestCaseID:       g.nextTestID(),
		FeatureName:      feature.Name,
		Priority:         model.TestPriorityP2,
		Type:             model.TestCategoryFunctional,
		Description:      fmt.Sprintf("Create and delete a %s (independent test)", feature.Name),
		IsDeploymentTest: false,
		Steps:            []model.TestStep{},
	}

	// Step 1: Create the resource first (making this test independent)
	createBody := g.generateRequestBody(feature, deletePath)
	keyDesc := describeKeyValues(createBody, feature)
	createStep := model.TestStep{
		Name:           "createResourceForDeletion",
		Description:    fmt.Sprintf("Create %s with %s before deleting", feature.Name, keyDesc),
		Method:         "POST",
		API:            model.APITypeREST,
		Path:           deletePath.Path, // Use same path, POST will create
		Body:           createBody,
		ExpectedStatus: 201,
		Validations:    crudValidations(201, "create", feature.Name, keyDesc),
	}
	tc.Steps = append(tc.Steps, createStep)

	// Step 2: Delete the resource
	deleteBody := g.generateDeleteBody(feature, deletePath)
	deleteStep := model.TestStep{
		Name:           "deleteResource",
		Description:    fmt.Sprintf("Delete %s with %s", feature.Name, keyDesc),
		Method:         deletePath.HTTPMethod,
		API:            model.APITypeREST,
		Path:           deletePath.Path,
		Body:           deleteBody,
		ExpectedStatus: 200,
		Validations:    crudValidations(200, "delete", feature.Name, keyDesc),
	}
	tc.Steps = append(tc.Steps, deleteStep)

	// Step 3: Verify deletion (retrieve should return empty or 404)
	verifyStep := model.TestStep{
		Name:           "verifyDeletion",
		Description:    fmt.Sprintf("Verify %s with %s was deleted", feature.Name, keyDesc),
		Method:         "POST",
		API:            model.APITypeREST,
		Path:           strings.Replace(deletePath.Path, "/delete", "/retrieve", 1),
		Body:           g.generateReadBody(feature, deletePath),
		ExpectedStatus: 200,
		Validations: []model.Validation{
			{
				Type:        model.ValidationTypeStatusCode,
				Expected:    200,
				Description: fmt.Sprintf("Verify %s with %s no longer exists in the response", feature.Name, keyDesc),
			},
		},
	}
	tc.Steps = append(tc.Steps, verifyStep)

	return tc
}

// generateSampleBody generates a sample request body
func (g *Generator) generateSampleBody(feature *model.Feature, schema interface{}) map[string]interface{} {
	body := make(map[string]interface{})

	// Use feature parameters if available
	if len(feature.Parameters) > 0 {
		body["name"] = "TestResource"
		for _, param := range feature.Parameters {
			if param.Required {
				body[param.Name] = g.getSampleValue(param)
			}
		}
	} else {
		// Generate from schema
		body["name"] = "TestResource"
		body["description"] = "Test resource created by automated test"
	}

	return body
}

// generateDeepScannedBody generates the proper body format for deep-scanned features.
// Format: { "featurePath": "...", "objectType": "...", "operation": "add", "objects": [...] }
// origin is "CC" for wired/service-profile features and "Global" for global-profile features.
func (g *Generator) generateDeepScannedBody(feature *model.Feature, featurePath string, objectType string) map[string]interface{} {
	return g.generateDeepScannedBodyWithOrigin(feature, featurePath, objectType, "CC")
}

// generateDeepScannedBodyWithOrigin builds a deep-scanned body with an explicit origin value.
func (g *Generator) generateDeepScannedBodyWithOrigin(feature *model.Feature, featurePath string, objectType string, origin string) map[string]interface{} {
	properties := []map[string]interface{}{}

	// Build a set of key fields for quick lookup
	keySet := make(map[string]bool, len(feature.Keys))
	for _, k := range feature.Keys {
		keySet[k] = true
	}

	for _, param := range feature.Parameters {
		// Skip system-managed fields — these are set internally, not via payload
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

	// If no required or key fields produced properties, include a representative
	// set of non-system optional fields so the body is never empty.
	if len(properties) == 0 {
		for _, param := range feature.Parameters {
			if isSystemManagedField(param.Name) {
				continue
			}
			properties = append(properties, map[string]interface{}{
				"name":   param.Name,
				"type":   g.mapYangTypeToJsonType(param.GoType),
				"value":  g.getSampleValue(param),
				"origin": origin,
			})
			if len(properties) >= 5 {
				break // cap at 5 to keep body readable
			}
		}
	}

	object := map[string]interface{}{
		"type":       objectType,
		"operation":  "add",
		"properties": properties,
	}

	return map[string]interface{}{
		"featurePath": featurePath,
		"objectType":  objectType,
		"operation":   "add",
		"objects":     []interface{}{object},
	}
}

// mapYangTypeToJsonType maps YANG/Go types to JSON schema types
func (g *Generator) mapYangTypeToJsonType(yangType string) string {
	switch yangType {
	case "int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64",
		"float32", "float64":
		return "number"
	case "bool", "boolean":
		return "boolean"
	case "string":
		return "string"
	default:
		return "string"
	}
}

// generateRequestBody generates the appropriate request body format.
// For deep-scanned features (global-profile, wired-blueprint, service-profile), use the special format.
// For regular features, use the simple format.
func (g *Generator) generateRequestBody(feature *model.Feature, featurePath *model.FeaturePath) map[string]interface{} {
	if featurePath.BlueprintCategory != "" {
		var fpValue string
		// objectType defaults to feature name but is overridable via an "objectType" PathParam FixedValue
		objectType := feature.Name

		for _, param := range featurePath.PathParams {
			if param.Name == "featurePath" && param.FixedValue != "" {
				fpValue = param.FixedValue
			}
			if param.Name == "objectType" && param.FixedValue != "" {
				objectType = param.FixedValue
			}
		}

		if fpValue != "" {
			// Choose correct origin: "Global" for global-profile, "CC" for wired/service profile
			origin := "CC"
			if featurePath.BlueprintCategory == model.BlueprintCategoryGlobal {
				origin = "Global"
			}
			return g.generateDeepScannedBodyWithOrigin(feature, fpValue, objectType, origin)
		}
	}

	return g.generateSampleBody(feature, featurePath.RequestSchema)
}

// getSampleValue generates a sample value for a parameter
func (g *Generator) getSampleValue(param model.Parameter) interface{} {
	if param.DefaultValue != nil {
		return param.DefaultValue
	}

	// Generate context-aware sample values based on parameter name and YANG description
	paramNameLower := strings.ToLower(param.Name)
	descLower := strings.ToLower(param.Description)

	// Prefix-length / mask fields (e.g. "mask" with YANG description mentioning "prefix length")
	if strings.Contains(descLower, "prefix length") || strings.Contains(descLower, "prefix-length") {
		return "24"
	}

	// IP subnet / CIDR address fields (e.g. "destination-subnet")
	if strings.Contains(paramNameLower, "subnet") ||
		strings.Contains(descLower, "cidr notation") || strings.Contains(descLower, "ip subnet") {
		return "192.168.1.0"
	}

	// Check for specific parameter patterns
	if strings.Contains(paramNameLower, "server") || strings.Contains(paramNameLower, "address") ||
		strings.Contains(paramNameLower, "host") || strings.Contains(paramNameLower, "ip") {
		return "8.8.8.8"
	}
	if strings.Contains(paramNameLower, "vr-name") || strings.Contains(paramNameLower, "vrf") {
		return "VR-Mgmt"
	}
	if strings.Contains(paramNameLower, "priority") {
		return 1
	}
	if strings.Contains(paramNameLower, "port") && (param.GoType == "int" || param.GoType == "int32" || param.GoType == "uint16") {
		return 53
	}
	if strings.Contains(paramNameLower, "name") && param.GoType == "string" {
		return "test-resource"
	}
	if strings.Contains(paramNameLower, "description") {
		return "Test resource created by automated test"
	}

	// Fallback to type-based values
	switch param.GoType {
	case "string":
		return "testValue"
	case "int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64":
		return 100
	case "bool", "boolean":
		return true
	case "float32", "float64":
		return 1.0
	default:
		return "testValue"
	}
}

// setBodyParameterValue sets a parameter value in the body, handling both simple and deep-scanned formats
func (g *Generator) setBodyParameterValue(body map[string]interface{}, paramName string, value interface{}) {
	// Check if this is a deep-scanned body (has objects array)
	if objects, ok := body["objects"].([]interface{}); ok && len(objects) > 0 {
		if obj, ok := objects[0].(map[string]interface{}); ok {
			if props, ok := obj["properties"].([]map[string]interface{}); ok {
				// Find the property and update its value
				for i, prop := range props {
					if prop["name"] == paramName {
						props[i]["value"] = value
						return
					}
				}
			}
		}
	} else {
		// Simple body format
		body[paramName] = value
	}
}

// setOrAddBodyParameter sets a parameter's value in the body, adding the
// property to the deep-scanned objects[].properties array when it is not
// already present. This is required for boundary / negative tests that
// target an optional parameter — the generated payload must include the
// parameter under test for the test to be meaningful.
func (g *Generator) setOrAddBodyParameter(body map[string]interface{}, param model.Parameter, value interface{}) {
	if objects, ok := body["objects"].([]interface{}); ok && len(objects) > 0 {
		obj, ok := objects[0].(map[string]interface{})
		if !ok {
			body[param.Name] = value
			return
		}
		props, ok := obj["properties"].([]map[string]interface{})
		if !ok {
			props = []map[string]interface{}{}
		}

		// Update in place if already present
		for i, prop := range props {
			if prop["name"] == param.Name {
				props[i]["value"] = value
				obj["properties"] = props
				return
			}
		}

		// Otherwise append a new property entry, inheriting origin from
		// existing entries so global vs CC payloads stay consistent.
		origin := "Global"
		if len(props) > 0 {
			if o, ok := props[0]["origin"].(string); ok && o != "" {
				origin = o
			}
		}
		props = append(props, map[string]interface{}{
			"name":   param.Name,
			"type":   g.mapYangTypeToJsonType(param.GoType),
			"value":  value,
			"origin": origin,
		})
		obj["properties"] = props
		return
	}

	// Simple body format — set or add directly
	body[param.Name] = value
}

// Helper function
func hasScopeType(paths []*model.FeaturePath, scopeType model.ScopeType) bool {
	for _, path := range paths {
		for _, st := range path.SupportedScopeTypes {
			if st == scopeType {
				return true
			}
		}
	}
	return false
}

// generateFullCRUDLifecycleTest generates a comprehensive CRUD lifecycle test
func (g *Generator) generateFullCRUDLifecycleTest(
	feature *model.Feature,
	createPath *model.FeaturePath,
	readPath *model.FeaturePath,
	updatePath *model.FeaturePath,
	deletePath *model.FeaturePath,
) model.TestCase {
	tc := model.TestCase{
		TestCaseID:       g.nextTestID(),
		FeatureName:      feature.Name,
		Priority:         model.TestPriorityP3,
		Type:             model.TestCategoryFunctional,
		Description:      fmt.Sprintf("Full CRUD lifecycle test for %s (Create -> Read -> Update -> Read -> Delete)", feature.Name),
		IsDeploymentTest: false,
		Steps:            []model.TestStep{},
	}

	createBody := g.generateRequestBody(feature, createPath)
	keyDesc := describeKeyValues(createBody, feature)

	// Step 1: Create
	createStep := model.TestStep{
		Name:           "createResource",
		Description:    fmt.Sprintf("Create %s with %s", feature.Name, keyDesc),
		Method:         createPath.HTTPMethod,
		API:            model.APITypeREST,
		Path:           createPath.Path,
		Body:           createBody,
		ExpectedStatus: 201,
		Validations:    crudValidations(201, "create", feature.Name, keyDesc),
	}

	// Step 2: Read (verify creation)
	readStep1 := model.TestStep{
		Name:           "verifyCreation",
		Description:    fmt.Sprintf("Verify %s with %s was created", feature.Name, keyDesc),
		Method:         readPath.HTTPMethod,
		API:            model.APITypeREST,
		Path:           readPath.Path,
		Body:           g.generateReadBody(feature, createPath),
		ExpectedStatus: 200,
		Validations:    crudValidations(200, "read", feature.Name, keyDesc),
	}

	// Step 3: Update
	updateBody := g.generateUpdateRequestBody(feature, updatePath)
	updateKeyDesc := describeModifiedFields(updateBody, feature)
	updateStep := model.TestStep{
		Name:           "updateResource",
		Description:    fmt.Sprintf("Update %s — %s", feature.Name, updateKeyDesc),
		Method:         updatePath.HTTPMethod,
		API:            model.APITypeREST,
		Path:           updatePath.Path,
		Body:           updateBody,
		ExpectedStatus: 200,
		Validations:    crudValidations(200, "update", feature.Name, updateKeyDesc),
	}

	// Step 4: Read (verify update)
	readStep2 := model.TestStep{
		Name:           "verifyUpdate",
		Description:    fmt.Sprintf("Verify %s was updated — %s", feature.Name, updateKeyDesc),
		Method:         readPath.HTTPMethod,
		API:            model.APITypeREST,
		Path:           readPath.Path,
		Body:           g.generateReadBody(feature, updatePath),
		ExpectedStatus: 200,
		Validations:    crudValidations(200, "read", feature.Name, updateKeyDesc),
	}

	// Step 5: Delete
	deleteBody := g.generateDeleteBody(feature, deletePath)
	deleteStep := model.TestStep{
		Name:           "deleteResource",
		Description:    fmt.Sprintf("Delete %s with %s", feature.Name, keyDesc),
		Method:         deletePath.HTTPMethod,
		API:            model.APITypeREST,
		Path:           deletePath.Path,
		Body:           deleteBody,
		ExpectedStatus: 200,
		Validations:    crudValidations(200, "delete", feature.Name, keyDesc),
	}

	tc.Steps = []model.TestStep{createStep, readStep1, updateStep, readStep2, deleteStep}
	return tc
}

// generateListAllResourcesTest generates a test for listing all resources
func (g *Generator) generateListAllResourcesTest(
	feature *model.Feature,
	createPath *model.FeaturePath,
	listPath *model.FeaturePath,
) model.TestCase {
	tc := model.TestCase{
		TestCaseID:       g.nextTestID(),
		FeatureName:      feature.Name,
		Priority:         model.TestPriorityP3,
		Type:             model.TestCategoryFunctional,
		Description:      fmt.Sprintf("List all %s resources", feature.Name),
		IsDeploymentTest: false,
		Steps:            []model.TestStep{},
	}

	// Step 1: Create resource to ensure list has content
	if createPath != nil {
		body := g.generateRequestBody(feature, createPath)
		keyDesc := describeKeyValues(body, feature)
		createStep := model.TestStep{
			Name:           "createResourceForList",
			Description:    fmt.Sprintf("Create %s with %s for list test", feature.Name, keyDesc),
			Method:         createPath.HTTPMethod,
			API:            model.APITypeREST,
			Path:           createPath.Path,
			Body:           body,
			ExpectedStatus: 201,
			Validations:    crudValidations(201, "create", feature.Name, keyDesc),
		}
		tc.Steps = append(tc.Steps, createStep)
	}

	// Step 2: List all resources
	listStep := model.TestStep{
		Name:           "listAllResources",
		Description:    fmt.Sprintf("Retrieve all %s resources and verify the list is non-empty", feature.Name),
		Method:         listPath.HTTPMethod,
		API:            model.APITypeREST,
		Path:           listPath.Path,
		Body:           g.generateReadBody(feature, listPath),
		ExpectedStatus: 200,
		Validations: []model.Validation{
			{
				Type:        model.ValidationTypeStatusCode,
				Expected:    200,
				Description: fmt.Sprintf("Verify HTTP 200 returned when listing all %s resources", feature.Name),
			},
			{
				Type:        model.ValidationTypeJSONPathExists,
				Path:        "$.objects",
				Description: fmt.Sprintf("Verify response contains objects array with at least one %s entry", feature.Name),
			},
		},
	}

	tc.Steps = append(tc.Steps, listStep)
	return tc
}

// generatePartialUpdateTest generates a test for partial update (PATCH-like behavior)
func (g *Generator) generatePartialUpdateTest(
	feature *model.Feature,
	createPath *model.FeaturePath,
	updatePath *model.FeaturePath,
	readPath *model.FeaturePath,
) model.TestCase {
	tc := model.TestCase{
		TestCaseID:       g.nextTestID(),
		FeatureName:      feature.Name,
		Priority:         model.TestPriorityP3,
		Type:             model.TestCategoryFunctional,
		Description:      fmt.Sprintf("Partial update test for %s (only update specific fields)", feature.Name),
		IsDeploymentTest: false,
		Steps:            []model.TestStep{},
	}

	// Step 1: Create resource
	createBody := g.generateRequestBody(feature, createPath)
	keyDesc := describeKeyValues(createBody, feature)
	createStep := model.TestStep{
		Name:           "createResourceForPartialUpdate",
		Description:    fmt.Sprintf("Create %s with %s for partial update", feature.Name, keyDesc),
		Method:         createPath.HTTPMethod,
		API:            model.APITypeREST,
		Path:           createPath.Path,
		Body:           createBody,
		ExpectedStatus: 201,
		Validations:    crudValidations(201, "create", feature.Name, keyDesc),
	}

	// Step 2: Partial update (only modify one optional field)
	updateBody := g.generateUpdateRequestBody(feature, updatePath)
	updateKeyDesc := describeModifiedFields(updateBody, feature)

	updateStep := model.TestStep{
		Name:           "partialUpdate",
		Description:    fmt.Sprintf("Partially update %s — %s", feature.Name, updateKeyDesc),
		Method:         updatePath.HTTPMethod,
		API:            model.APITypeREST,
		Path:           updatePath.Path,
		Body:           updateBody,
		ExpectedStatus: 200,
		Validations:    crudValidations(200, "update", feature.Name, updateKeyDesc),
	}

	// Step 3: Read and verify
	readStep := model.TestStep{
		Name:           "verifyPartialUpdate",
		Description:    fmt.Sprintf("Verify partial update of %s — %s", feature.Name, updateKeyDesc),
		Method:         readPath.HTTPMethod,
		API:            model.APITypeREST,
		Path:           readPath.Path,
		Body:           g.generateReadBody(feature, updatePath),
		ExpectedStatus: 200,
		Validations:    crudValidations(200, "read", feature.Name, updateKeyDesc),
	}

	tc.Steps = []model.TestStep{createStep, updateStep, readStep}
	return tc
}

// generateIdempotentCreateTest generates a test for idempotent create operations
func (g *Generator) generateIdempotentCreateTest(
	feature *model.Feature,
	createPath *model.FeaturePath,
) model.TestCase {
	tc := model.TestCase{
		TestCaseID:       g.nextTestID(),
		FeatureName:      feature.Name,
		Priority:         model.TestPriorityP3,
		Type:             model.TestCategoryFunctional,
		Description:      fmt.Sprintf("Idempotent create test for %s (creating same resource twice should return 409 Conflict)", feature.Name),
		IsDeploymentTest: false,
		Steps:            []model.TestStep{},
	}

	body := g.generateRequestBody(feature, createPath)
	keyDesc := describeKeyValues(body, feature)

	// Step 1: Create resource (first time)
	createStep1 := model.TestStep{
		Name:           "createResourceFirstTime",
		Description:    fmt.Sprintf("Create %s with %s (first attempt — should succeed)", feature.Name, keyDesc),
		Method:         createPath.HTTPMethod,
		API:            model.APITypeREST,
		Path:           createPath.Path,
		Body:           body,
		ExpectedStatus: 201,
		Validations:    crudValidations(201, "create", feature.Name, keyDesc),
	}

	// Step 2: Create same resource again (should return 409 Conflict)
	createStep2 := model.TestStep{
		Name:           "createResourceAgain",
		Description:    fmt.Sprintf("Create same %s with %s again (should fail with 409 Conflict — duplicate key)", feature.Name, keyDesc),
		Method:         createPath.HTTPMethod,
		API:            model.APITypeREST,
		Path:           createPath.Path,
		Body:           deepCopyBody(body),
		ExpectedStatus: 409,
		Validations:    crudValidations(409, "conflict", feature.Name, keyDesc),
	}

	tc.Steps = []model.TestStep{createStep1, createStep2}
	return tc
}

// generateSubObjectCreateTest generates a create test for a sub-object type (e.g., dns-suffix)
func (g *Generator) generateSubObjectCreateTest(feature *model.Feature, subObjType *model.SubObjectType, subObjName string, createPath, readPath *model.FeaturePath) model.TestCase {
	tc := model.TestCase{
		TestCaseID:       g.nextTestID(),
		FeatureName:      fmt.Sprintf("%s/%s", feature.Name, subObjName),
		Priority:         model.TestPriorityP1,
		Type:             model.TestCategoryFunctional,
		Description:      fmt.Sprintf("Create a %s (sub-object type: %s) and verify it exists", subObjName, feature.Name),
		IsDeploymentTest: false,
		Steps:            []model.TestStep{},
	}

	// Build request body for sub-object type
	body := g.generateSubObjectRequestBody(subObjType, subObjName, "add")

	// Create step
	createStep := model.TestStep{
		Name:           fmt.Sprintf("create_%s", subObjName),
		Description:    fmt.Sprintf("Create %s sub-object under %s", subObjName, feature.Name),
		Method:         createPath.HTTPMethod,
		API:            model.APITypeREST,
		Path:           createPath.Path,
		Body:           body,
		ExpectedStatus: 201,
		Validations:    crudValidations(201, "create", fmt.Sprintf("%s/%s", feature.Name, subObjName), ""),
	}
	tc.Steps = append(tc.Steps, createStep)

	// Read step to verify (if read path available)
	if readPath != nil {
		readStep := model.TestStep{
			Name:        fmt.Sprintf("verify_%s_creation", subObjName),
			Description: fmt.Sprintf("Verify %s sub-object was created under %s", subObjName, feature.Name),
			Method:      readPath.HTTPMethod,
			API:         model.APITypeREST,
			Path:        readPath.Path,
			Body: map[string]interface{}{
				"featurePath": g.getFeaturePath(createPath),
				"objectType":  subObjName,
			},
			ExpectedStatus: 200,
			Validations:    crudValidations(200, "read", fmt.Sprintf("%s/%s", feature.Name, subObjName), ""),
		}
		tc.Steps = append(tc.Steps, readStep)
	}

	return tc
}

// generateSubObjectUpdateTest generates an update test for a sub-object type
func (g *Generator) generateSubObjectUpdateTest(feature *model.Feature, subObjType *model.SubObjectType, subObjName string, updatePath, readPath *model.FeaturePath) model.TestCase {
	tc := model.TestCase{
		TestCaseID:       g.nextTestID(),
		FeatureName:      fmt.Sprintf("%s/%s", feature.Name, subObjName),
		Priority:         model.TestPriorityP2,
		Type:             model.TestCategoryFunctional,
		Description:      fmt.Sprintf("Create, update, and verify %s (sub-object type)", subObjName),
		IsDeploymentTest: false,
		Steps:            []model.TestStep{},
	}

	// Step 1: Create the sub-object first
	createBody := g.generateSubObjectRequestBody(subObjType, subObjName, "add")
	createStep := model.TestStep{
		Name:           fmt.Sprintf("create_%s_for_update", subObjName),
		Description:    fmt.Sprintf("Create %s sub-object under %s before updating", subObjName, feature.Name),
		Method:         updatePath.HTTPMethod,
		API:            model.APITypeREST,
		Path:           updatePath.Path,
		Body:           createBody,
		ExpectedStatus: 201,
		Validations:    crudValidations(201, "create", fmt.Sprintf("%s/%s", feature.Name, subObjName), ""),
	}
	tc.Steps = append(tc.Steps, createStep)

	// Step 2: Update the sub-object
	updateBody := g.generateSubObjectRequestBody(subObjType, subObjName, "update")
	// Add object ID for update (placeholder)
	if bodyMap, ok := updateBody.(map[string]interface{}); ok {
		if objects, ok := bodyMap["objects"].([]map[string]interface{}); ok && len(objects) > 0 {
			objects[0]["id"] = "{{OBJECT_ID}}"
		}
	}

	updateStep := model.TestStep{
		Name:           fmt.Sprintf("update_%s", subObjName),
		Description:    fmt.Sprintf("Update %s sub-object under %s", subObjName, feature.Name),
		Method:         updatePath.HTTPMethod,
		API:            model.APITypeREST,
		Path:           updatePath.Path,
		Body:           updateBody,
		ExpectedStatus: 200,
		Validations:    crudValidations(200, "update", fmt.Sprintf("%s/%s", feature.Name, subObjName), ""),
	}
	tc.Steps = append(tc.Steps, updateStep)

	// Step 3: Verify update
	if readPath != nil {
		readStep := model.TestStep{
			Name:        fmt.Sprintf("verify_%s_update", subObjName),
			Description: fmt.Sprintf("Verify %s sub-object was updated under %s", subObjName, feature.Name),
			Method:      readPath.HTTPMethod,
			API:         model.APITypeREST,
			Path:        readPath.Path,
			Body: map[string]interface{}{
				"featurePath": g.getFeaturePath(updatePath),
				"objectType":  subObjName,
				"objectIds":   []string{"{{OBJECT_ID}}"},
			},
			ExpectedStatus: 200,
			Validations:    crudValidations(200, "read", fmt.Sprintf("%s/%s", feature.Name, subObjName), ""),
		}
		tc.Steps = append(tc.Steps, readStep)
	}

	return tc
}

// generateSubObjectDeleteTest generates a delete test for a sub-object type
func (g *Generator) generateSubObjectDeleteTest(feature *model.Feature, subObjType *model.SubObjectType, subObjName string, deletePath *model.FeaturePath) model.TestCase {
	tc := model.TestCase{
		TestCaseID:       g.nextTestID(),
		FeatureName:      fmt.Sprintf("%s/%s", feature.Name, subObjName),
		Priority:         model.TestPriorityP2,
		Type:             model.TestCategoryFunctional,
		Description:      fmt.Sprintf("Create and delete %s (sub-object type)", subObjName),
		IsDeploymentTest: false,
		Steps:            []model.TestStep{},
	}

	// Step 1: Create the sub-object first
	createBody := g.generateSubObjectRequestBody(subObjType, subObjName, "add")
	createStep := model.TestStep{
		Name:           fmt.Sprintf("create_%s_for_delete", subObjName),
		Description:    fmt.Sprintf("Create %s sub-object under %s before deleting", subObjName, feature.Name),
		Method:         "POST",
		API:            model.APITypeREST,
		Path:           deletePath.Path,
		Body:           createBody,
		ExpectedStatus: 201,
		Validations:    crudValidations(201, "create", fmt.Sprintf("%s/%s", feature.Name, subObjName), ""),
	}
	tc.Steps = append(tc.Steps, createStep)

	// Step 2: Delete the sub-object
	deleteStep := model.TestStep{
		Name:        fmt.Sprintf("delete_%s", subObjName),
		Description: fmt.Sprintf("Delete %s sub-object under %s", subObjName, feature.Name),
		Method:      deletePath.HTTPMethod,
		API:         model.APITypeREST,
		Path:        deletePath.Path,
		Body: map[string]interface{}{
			"featurePath": g.getFeaturePath(deletePath),
			"objectType":  subObjName,
			"objectId":    "{{OBJECT_ID}}",
		},
		ExpectedStatus: 200,
		Validations:    crudValidations(200, "delete", fmt.Sprintf("%s/%s", feature.Name, subObjName), ""),
	}
	tc.Steps = append(tc.Steps, deleteStep)

	return tc
}

// generateSubObjectRequestBody generates a request body for sub-object type operations
func (g *Generator) generateSubObjectRequestBody(subObjType *model.SubObjectType, objectTypeName string, operation string) interface{} {
	properties := []map[string]interface{}{}

	// Generate properties from sub-object type parameters
	for _, param := range subObjType.Parameters {
		prop := map[string]interface{}{
			"name":   param.Name,
			"type":   g.getPropertyType(param.GoType),
			"value":  g.getSampleValue(param),
			"origin": "Global",
		}
		properties = append(properties, prop)
	}

	// Build request body
	body := map[string]interface{}{
		"featurePath": g.getFeaturePathFromObjectType(objectTypeName),
		"objectType":  objectTypeName,
		"operation":   operation,
		"objects": []map[string]interface{}{
			{
				"type":       objectTypeName,
				"operation":  operation,
				"properties": properties,
			},
		},
	}

	return body
}

// getFeaturePathFromObjectType maps object type names to their feature paths
func (g *Generator) getFeaturePathFromObjectType(objectType string) string {
	// Map based on qaopenapi.yaml structure
	featurePathMap := map[string]string{
		// Global Profile sub-objects
		"dns-suffix": "/dns-server-feature",
		// SNMP sub-objects
		"snmp-v3-user": "/infrastructure-feature/snmp-feature",
		"snmp-trap":    "/infrastructure-feature/snmp-feature",
		// WLAN sub-objects
		"wlan-security": "/wlan-feature",
	}

	if path, exists := featurePathMap[objectType]; exists {
		return path
	}
	return fmt.Sprintf("/%s-feature", objectType)
}

// getFeaturePath extracts the feature path from a FeaturePath
func (g *Generator) getFeaturePath(fp *model.FeaturePath) string {
	// Check for fixed value in path params (for deep-scanned features)
	for _, param := range fp.PathParams {
		if param.Name == "featurePath" && param.FixedValue != "" {
			return param.FixedValue
		}
	}
	return fmt.Sprintf("/%s", fp.FeatureName)
}

// getPropertyType converts Go type to property type string
func (g *Generator) getPropertyType(goType string) string {
	typeMap := map[string]string{
		"string":  "string",
		"int":     "number",
		"int8":    "number",
		"int16":   "number",
		"int32":   "number",
		"int64":   "number",
		"float64": "number",
		"bool":    "boolean",
	}
	if propType, exists := typeMap[goType]; exists {
		return propType
	}
	return "string"
}

// ─── YANG Model Coverage: Non-Deployment Tests ───────────────────────────────

// generateCreateWithRequiredFieldsOnlyTest verifies that a resource can be created
// using ONLY the required fields (all optional fields omitted), confirming optional
// fields are genuinely optional per the YANG model.
// The subsequent GET step validates the actual values of required fields in the response.
func (g *Generator) generateCreateWithRequiredFieldsOnlyTest(
	feature *model.Feature,
	createPath *model.FeaturePath,
	readPath *model.FeaturePath,
) model.TestCase {
	tc := model.TestCase{
		TestCaseID:       g.nextTestID(),
		FeatureName:      feature.Name,
		Priority:         model.TestPriorityP2,
		Type:             model.TestCategoryFunctional,
		Description:      fmt.Sprintf("Create %s with required fields only (verify optional fields are optional per YANG model)", feature.Name),
		IsDeploymentTest: false,
		Steps:            []model.TestStep{},
	}

	// Build body with ONLY required parameters and record the sent values
	body := g.generateRequiredOnlyBody(feature, createPath)
	// Collect expected values for each required field
	requiredExpectedValues := map[string]interface{}{}
	for _, param := range feature.Parameters {
		if param.Required {
			requiredExpectedValues[param.Name] = g.getSampleValue(param)
		}
	}

	createStep := model.TestStep{
		Name:           "createWithRequiredFieldsOnly",
		Description:    fmt.Sprintf("Create %s providing only required YANG fields", feature.Name),
		Method:         createPath.HTTPMethod,
		API:            model.APITypeREST,
		Path:           createPath.Path,
		Body:           body,
		ExpectedStatus: 201,
		Validations: []model.Validation{
			{
				Type:        model.ValidationTypeStatusCode,
				Expected:    201,
				Description: "Verify resource is created with required fields only",
			},
		},
	}
	tc.Steps = append(tc.Steps, createStep)

	if readPath != nil {
		// Build field-value validations for every required parameter
		var readValidations []model.Validation
		readValidations = append(readValidations, model.Validation{
			Type:        model.ValidationTypeStatusCode,
			Expected:    200,
			Description: "Verify HTTP 200 for resource read",
		})
		readValidations = append(readValidations, model.Validation{
			Type:        model.ValidationTypeJSONPathExists,
			Path:        "$.objects",
			Description: "Verify response contains objects array",
		})
		for _, param := range feature.Parameters {
			if param.Required {
				expectedVal := g.getSampleValue(param)
				readValidations = append(readValidations, model.Validation{
					Type:        model.ValidationTypeJSONPathExists,
					Path:        fmt.Sprintf("$.objects[0].properties[?(@.name=='%s')]", param.Name),
					Description: fmt.Sprintf("Verify required field '%s' is present in GET response", param.Name),
				})
				readValidations = append(readValidations, model.Validation{
					Type:        model.ValidationTypeJSONPathEquals,
					Path:        fmt.Sprintf("$.objects[0].properties[?(@.name=='%s')].value", param.Name),
					Expected:    expectedVal,
					Description: fmt.Sprintf("Verify required field '%s' has value '%v' as created", param.Name, expectedVal),
				})
			}
		}

		readStep := model.TestStep{
			Name:           "verifyCreatedWithRequiredOnly",
			Description:    "Verify resource exists and required field values match what was sent on creation",
			Method:         readPath.HTTPMethod,
			API:            model.APITypeREST,
			Path:           readPath.Path,
			Body:           g.generateReadBody(feature, createPath),
			ExpectedStatus: 200,
			Validations:    readValidations,
		}
		tc.Steps = append(tc.Steps, readStep)
	}

	return tc
}

// generateCreateWithAllFieldsTest verifies that a resource can be created with ALL
// fields (required + optional) provided, ensuring the YANG model accepts all defined fields.
// The subsequent GET step validates every field value matches what was sent.
func (g *Generator) generateCreateWithAllFieldsTest(
	feature *model.Feature,
	createPath *model.FeaturePath,
	readPath *model.FeaturePath,
) model.TestCase {
	tc := model.TestCase{
		TestCaseID:       g.nextTestID(),
		FeatureName:      feature.Name,
		Priority:         model.TestPriorityP2,
		Type:             model.TestCategoryFunctional,
		Description:      fmt.Sprintf("Create %s with all fields (required + optional) per YANG model definition", feature.Name),
		IsDeploymentTest: false,
		Steps:            []model.TestStep{},
	}

	// Build body with ALL parameters (required + optional)
	body := g.generateAllFieldsBody(feature, createPath)

	createStep := model.TestStep{
		Name:           "createWithAllFields",
		Description:    fmt.Sprintf("Create %s providing all YANG-defined fields including optional", feature.Name),
		Method:         createPath.HTTPMethod,
		API:            model.APITypeREST,
		Path:           createPath.Path,
		Body:           body,
		ExpectedStatus: 201,
		Validations: []model.Validation{
			{
				Type:        model.ValidationTypeStatusCode,
				Expected:    201,
				Description: "Verify resource is created with all fields",
			},
		},
	}
	tc.Steps = append(tc.Steps, createStep)

	if readPath != nil {
		// Build field-value validations for ALL parameters (required + optional)
		var readValidations []model.Validation
		readValidations = append(readValidations, model.Validation{
			Type:        model.ValidationTypeStatusCode,
			Expected:    200,
			Description: "Verify HTTP 200 for resource read",
		})
		readValidations = append(readValidations, model.Validation{
			Type:        model.ValidationTypeJSONPathExists,
			Path:        "$.objects",
			Description: "Verify response contains objects array",
		})
		for _, param := range feature.Parameters {
			if isSystemManagedField(param.Name) {
				continue
			}
			expectedVal := g.getSampleValue(param)
			readValidations = append(readValidations, model.Validation{
				Type:        model.ValidationTypeJSONPathExists,
				Path:        fmt.Sprintf("$.objects[0].properties[?(@.name=='%s')]", param.Name),
				Description: fmt.Sprintf("Verify field '%s' is present in GET response", param.Name),
			})
			readValidations = append(readValidations, model.Validation{
				Type:        model.ValidationTypeJSONPathEquals,
				Path:        fmt.Sprintf("$.objects[0].properties[?(@.name=='%s')].value", param.Name),
				Expected:    expectedVal,
				Description: fmt.Sprintf("Verify field '%s' has value '%v' as sent on create", param.Name, expectedVal),
			})
		}

		readStep := model.TestStep{
			Name:           "verifyAllFieldsPersisted",
			Description:    "Verify all fields (required + optional) are persisted with correct values",
			Method:         readPath.HTTPMethod,
			API:            model.APITypeREST,
			Path:           readPath.Path,
			Body:           g.generateReadBody(feature, createPath),
			ExpectedStatus: 200,
			Validations:    readValidations,
		}
		tc.Steps = append(tc.Steps, readStep)
	}

	return tc
}

// generateDefaultValueTests generates one test per YANG parameter that declares a default value.
// Each test: creates the resource without the defaulted field → reads back → verifies the default.
func (g *Generator) generateDefaultValueTests(
	feature *model.Feature,
	createPath *model.FeaturePath,
	readPath *model.FeaturePath,
) []model.TestCase {
	var tests []model.TestCase

	for _, param := range feature.Parameters {
		if param.DefaultValue == nil || param.Required {
			// Only test optional fields that have explicit defaults
			continue
		}

		// Enrich description with YANG leaf description when available
		var defaultTcDesc string
		if param.Description != "" {
			defaultTcDesc = fmt.Sprintf("Verify YANG default value for '%s' is applied when field is omitted (expected default: %v) — %s", param.Name, param.DefaultValue, param.Description)
		} else {
			defaultTcDesc = fmt.Sprintf("Verify YANG default value for '%s' is applied when field is omitted (expected default: %v)", param.Name, param.DefaultValue)
		}

		tc := model.TestCase{
			TestCaseID:       g.nextTestID(),
			FeatureName:      feature.Name,
			Priority:         model.TestPriorityP2,
			Type:             model.TestCategoryFunctional,
			Description:      defaultTcDesc,
			IsDeploymentTest: false,
			Steps:            []model.TestStep{},
		}

		// Build body WITHOUT the defaulted parameter
		body := g.generateBodyWithoutParam(feature, createPath, param.Name)

		createStep := model.TestStep{
			Name:           fmt.Sprintf("createWithout%s", strings.Title(strings.ReplaceAll(param.Name, "-", ""))),
			Description:    fmt.Sprintf("Create %s without '%s' field to trigger YANG default (%v)", feature.Name, param.Name, param.DefaultValue),
			Method:         createPath.HTTPMethod,
			API:            model.APITypeREST,
			Path:           createPath.Path,
			Body:           body,
			ExpectedStatus: 201,
			Validations: []model.Validation{
				{
					Type:        model.ValidationTypeStatusCode,
					Expected:    201,
					Description: "Verify resource created without the defaulted field",
				},
			},
		}
		tc.Steps = append(tc.Steps, createStep)

		// Read back and verify the default value is returned
		readStep := model.TestStep{
			Name:           fmt.Sprintf("verifyDefault%s", strings.Title(strings.ReplaceAll(param.Name, "-", ""))),
			Description:    fmt.Sprintf("Read back %s and verify '%s' equals YANG default value %v", feature.Name, param.Name, param.DefaultValue),
			Method:         readPath.HTTPMethod,
			API:            model.APITypeREST,
			Path:           readPath.Path,
			Body:           g.generateReadBody(feature, createPath),
			ExpectedStatus: 200,
			Validations: []model.Validation{
				{
					Type:        model.ValidationTypeStatusCode,
					Expected:    200,
					Description: "Verify HTTP 200",
				},
				{
					Type:        model.ValidationTypeJSONPathEquals,
					Path:        fmt.Sprintf("$.%s", param.Name),
					Expected:    param.DefaultValue,
					Description: fmt.Sprintf("Verify '%s' equals YANG-defined default value %v", param.Name, param.DefaultValue),
				},
			},
		}
		tc.Steps = append(tc.Steps, readStep)

		tests = append(tests, tc)
	}

	return tests
}

// generateYANGDatatypeNonDeploymentTests generates one non-deployment test per distinct YANG datatype
// found in the feature's parameters, validating that each type is handled correctly.
func (g *Generator) generateYANGDatatypeNonDeploymentTests(
	feature *model.Feature,
	createPath *model.FeaturePath,
	readPath *model.FeaturePath,
) []model.TestCase {
	var tests []model.TestCase

	// Collect unique YANG types and a representative parameter for each
	seenTypes := make(map[string]bool)
	type typeEntry struct {
		yangType string
		param    model.Parameter
	}
	var typeEntries []typeEntry

	allParams := collectAllParameters(feature.Parameters)
	for _, param := range allParams {
		// Skip system-managed fields — they can't be set via API
		if isSystemManagedField(param.Name) {
			continue
		}
		yangType := param.YangType
		if yangType == "" {
			yangType = param.GoType
		}
		if yangType == "" || seenTypes[yangType] {
			continue
		}
		seenTypes[yangType] = true
		typeEntries = append(typeEntries, typeEntry{yangType: yangType, param: param})
	}

	for _, entry := range typeEntries {
		param := entry.param
		yangType := entry.yangType

		// Generate 2 valid value variations for this type (non-deployment)
		for variation := 1; variation <= 2; variation++ {
			value := g.getYANGTypeValue(param, yangType, variation)

			// Enrich description with YANG leaf description when available
			var yangDatatypeTcDesc string
			if param.Description != "" {
				yangDatatypeTcDesc = fmt.Sprintf("Verify YANG datatype '%s' for field '%s' with value variation %d (%v) — %s", yangType, param.Name, variation, value, param.Description)
			} else {
				yangDatatypeTcDesc = fmt.Sprintf("Verify YANG datatype '%s' for field '%s' with value variation %d (%v)", yangType, param.Name, variation, value)
			}

			tc := model.TestCase{
				TestCaseID:       g.nextTestID(),
				FeatureName:      feature.Name,
				Priority:         model.TestPriorityP2,
				Type:             model.TestCategoryFunctional,
				Description:      yangDatatypeTcDesc,
				IsDeploymentTest: false,
				Steps:            []model.TestStep{},
			}

			body := g.generateRequestBody(feature, createPath)
			g.setOrAddBodyParameter(body, param, value)

			createStep := model.TestStep{
				Name:           fmt.Sprintf("createWith%sType_v%d", strings.ReplaceAll(yangType, "-", ""), variation),
				Description:    fmt.Sprintf("Create %s with YANG type '%s' value: %v", feature.Name, yangType, value),
				Method:         createPath.HTTPMethod,
				API:            model.APITypeREST,
				Path:           createPath.Path,
				Body:           body,
				ExpectedStatus: 201,
				Validations: []model.Validation{
					{
						Type:        model.ValidationTypeStatusCode,
						Expected:    201,
						Description: fmt.Sprintf("Verify %s type '%s' value is accepted", param.Name, yangType),
					},
				},
			}
			tc.Steps = append(tc.Steps, createStep)

			if readPath != nil {
				readStep := model.TestStep{
					Name:           fmt.Sprintf("verify%sTypeValue_v%d", strings.ReplaceAll(yangType, "-", ""), variation),
					Description:    fmt.Sprintf("Verify '%s' field persisted with YANG type '%s' value %v", param.Name, yangType, value),
					Method:         readPath.HTTPMethod,
					API:            model.APITypeREST,
					Path:           readPath.Path,
					Body:           g.generateReadBody(feature, createPath),
					ExpectedStatus: 200,
					Validations: []model.Validation{
						{
							Type:        model.ValidationTypeStatusCode,
							Expected:    200,
							Description: "Verify HTTP 200",
						},
						{
							Type:        model.ValidationTypeJSONPathEquals,
							Path:        fmt.Sprintf("$.%s", param.Name),
							Expected:    value,
							Description: fmt.Sprintf("Verify '%s' YANG type '%s' value is persisted", param.Name, yangType),
						},
					},
				}
				tc.Steps = append(tc.Steps, readStep)
			}

			tests = append(tests, tc)
		}
	}

	return tests
}

// ─── Body builders for YANG coverage tests ───────────────────────────────────

// generateRequiredOnlyBody builds a request body with ONLY the required YANG parameters.
func (g *Generator) generateRequiredOnlyBody(feature *model.Feature, fp *model.FeaturePath) map[string]interface{} {
	if fp.BlueprintCategory != "" {
		var fpValue string
		for _, p := range fp.PathParams {
			if p.Name == "featurePath" && p.FixedValue != "" {
				fpValue = p.FixedValue
				break
			}
		}
		if fpValue != "" {
			properties := []map[string]interface{}{}
			for _, param := range feature.Parameters {
				if param.Required {
					properties = append(properties, map[string]interface{}{
						"name":   param.Name,
						"type":   g.mapYangTypeToJsonType(param.GoType),
						"value":  g.getSampleValue(param),
						"origin": "Global",
					})
				}
			}
			object := map[string]interface{}{
				"type":       feature.Name,
				"operation":  "add",
				"properties": properties,
			}
			return map[string]interface{}{
				"featurePath": fpValue,
				"objectType":  feature.Name,
				"operation":   "add",
				"objects":     []interface{}{object},
			}
		}
	}
	// Simple format: include only required fields
	body := make(map[string]interface{})
	for _, param := range feature.Parameters {
		if param.Required {
			body[param.Name] = g.getSampleValue(param)
		}
	}
	return body
}

// generateAllFieldsBody builds a request body including ALL YANG parameters (required + optional).
func (g *Generator) generateAllFieldsBody(feature *model.Feature, fp *model.FeaturePath) map[string]interface{} {
	if fp.BlueprintCategory != "" {
		var fpValue string
		for _, p := range fp.PathParams {
			if p.Name == "featurePath" && p.FixedValue != "" {
				fpValue = p.FixedValue
				break
			}
		}
		if fpValue != "" {
			properties := []map[string]interface{}{}
			for _, param := range feature.Parameters {
				if isSystemManagedField(param.Name) {
					continue
				}
				properties = append(properties, map[string]interface{}{
					"name":   param.Name,
					"type":   g.mapYangTypeToJsonType(param.GoType),
					"value":  g.getSampleValue(param),
					"origin": "Global",
				})
			}
			object := map[string]interface{}{
				"type":       feature.Name,
				"operation":  "add",
				"properties": properties,
			}
			return map[string]interface{}{
				"featurePath": fpValue,
				"objectType":  feature.Name,
				"operation":   "add",
				"objects":     []interface{}{object},
			}
		}
	}
	// Simple format: include all fields
	body := make(map[string]interface{})
	for _, param := range feature.Parameters {
		if isSystemManagedField(param.Name) {
			continue
		}
		body[param.Name] = g.getSampleValue(param)
	}
	return body
}

// generateBodyWithoutParam builds a request body omitting a specific parameter (to test defaults).
func (g *Generator) generateBodyWithoutParam(feature *model.Feature, fp *model.FeaturePath, excludeParam string) map[string]interface{} {
	if fp.BlueprintCategory != "" {
		var fpValue string
		for _, p := range fp.PathParams {
			if p.Name == "featurePath" && p.FixedValue != "" {
				fpValue = p.FixedValue
				break
			}
		}
		if fpValue != "" {
			properties := []map[string]interface{}{}
			for _, param := range feature.Parameters {
				if param.Name == excludeParam {
					continue // omit the defaulted field
				}
				if param.Required {
					properties = append(properties, map[string]interface{}{
						"name":   param.Name,
						"type":   g.mapYangTypeToJsonType(param.GoType),
						"value":  g.getSampleValue(param),
						"origin": "Global",
					})
				}
			}
			object := map[string]interface{}{
				"type":       feature.Name,
				"operation":  "add",
				"properties": properties,
			}
			return map[string]interface{}{
				"featurePath": fpValue,
				"objectType":  feature.Name,
				"operation":   "add",
				"objects":     []interface{}{object},
			}
		}
	}
	// Simple format
	body := make(map[string]interface{})
	for _, param := range feature.Parameters {
		if param.Name == excludeParam {
			continue
		}
		if param.Required {
			body[param.Name] = g.getSampleValue(param)
		}
	}
	return body
}

// generateReadBody builds the body for a read (retrieve) request.
// For global-profile features the retrieve endpoint is also POST with a featurePath body.
func (g *Generator) generateReadBody(feature *model.Feature, createPath *model.FeaturePath) map[string]interface{} {
	if createPath == nil || createPath.BlueprintCategory == "" {
		return nil
	}
	var fpValue string
	for _, p := range createPath.PathParams {
		if p.Name == "featurePath" && p.FixedValue != "" {
			fpValue = p.FixedValue
			break
		}
	}
	if fpValue != "" {
		return map[string]interface{}{
			"featurePath": fpValue,
			"objectType":  feature.Name,
		}
	}
	return nil
}

// generateDeleteBody builds the body for a delete request.
// For global-profile features the delete endpoint requires featurePath.
func (g *Generator) generateDeleteBody(feature *model.Feature, anyPath *model.FeaturePath) map[string]interface{} {
	if anyPath == nil || anyPath.BlueprintCategory == "" {
		return nil
	}
	var fpValue string
	for _, p := range anyPath.PathParams {
		if p.Name == "featurePath" && p.FixedValue != "" {
			fpValue = p.FixedValue
			break
		}
	}
	if fpValue != "" {
		return map[string]interface{}{
			"featurePath": fpValue,
			"objectType":  feature.Name,
		}
	}
	return nil
}

// generateUpdateRequestBody builds a proper update body.
// For deep-scanned features, it uses operation="update" and modifies one optional field.
func (g *Generator) generateUpdateRequestBody(feature *model.Feature, updatePath *model.FeaturePath) map[string]interface{} {
	if updatePath.BlueprintCategory != "" {
		var fpValue string
		objectType := feature.Name
		for _, param := range updatePath.PathParams {
			if param.Name == "featurePath" && param.FixedValue != "" {
				fpValue = param.FixedValue
			}
			if param.Name == "objectType" && param.FixedValue != "" {
				objectType = param.FixedValue
			}
		}
		if fpValue != "" {
			origin := "CC"
			if updatePath.BlueprintCategory == model.BlueprintCategoryGlobal {
				origin = "Global"
			}
			properties := []map[string]interface{}{}

			// Include required/key fields so the system knows which object to update
			for _, param := range feature.Parameters {
				if isSystemManagedField(param.Name) {
					continue
				}
				if param.Required {
					properties = append(properties, map[string]interface{}{
						"name":   param.Name,
						"type":   g.mapYangTypeToJsonType(param.GoType),
						"value":  g.getSampleValue(param),
						"origin": origin,
					})
				}
			}

			// Add/modify one optional field to differentiate from the create body
			for _, param := range feature.Parameters {
				if isSystemManagedField(param.Name) || param.Required {
					continue
				}
				updatedVal := g.getUpdatedValue(param)
				properties = append(properties, map[string]interface{}{
					"name":   param.Name,
					"type":   g.mapYangTypeToJsonType(param.GoType),
					"value":  updatedVal,
					"origin": origin,
				})
				break // one optional field is enough to show the update
			}

			object := map[string]interface{}{
				"type":       objectType,
				"operation":  "update",
				"properties": properties,
			}
			return map[string]interface{}{
				"featurePath": fpValue,
				"objectType":  objectType,
				"operation":   "update",
				"objects":     []interface{}{object},
			}
		}
	}
	// Fallback to simple body with one modified field
	body := g.generateSampleBody(feature, updatePath.RequestSchema)
	body["description"] = "Updated via automated test"
	return body
}

// getUpdatedValue returns a different value than getSampleValue, suitable for update tests.
func (g *Generator) getUpdatedValue(param model.Parameter) interface{} {
	paramNameLower := strings.ToLower(param.Name)

	// Booleans: flip the default
	if param.GoType == "bool" || param.GoType == "boolean" {
		return false
	}

	// Enums: pick the second value if available
	for _, c := range param.Constraints {
		if c.Type == model.ConstraintTypeEnum {
			if vals, ok := c.Value.([]string); ok && len(vals) > 1 {
				return vals[1]
			}
		}
	}

	// Ports/numbers: use a different value
	if strings.Contains(paramNameLower, "port") {
		return 8080
	}
	if param.GoType == "int" || param.GoType == "int32" || param.GoType == "uint16" || param.GoType == "uint8" || param.GoType == "uint32" {
		return 50
	}

	// Strings: return a clearly updated value
	return "updated-test-value"
}

// describeKeyValues returns a human-readable string of key field values from the body.
// e.g., "server=8.8.8.8, vr-name=VR-Mgmt"
func describeKeyValues(body map[string]interface{}, feature *model.Feature) string {
	if body == nil {
		return ""
	}
	var parts []string
	// Deep-scanned format
	if objects, ok := body["objects"].([]interface{}); ok && len(objects) > 0 {
		if obj, ok := objects[0].(map[string]interface{}); ok {
			if props, ok := obj["properties"].([]map[string]interface{}); ok {
				keySet := make(map[string]bool, len(feature.Keys))
				for _, k := range feature.Keys {
					keySet[k] = true
				}
				for _, prop := range props {
					name, _ := prop["name"].(string)
					val := prop["value"]
					if len(keySet) == 0 || keySet[name] {
						parts = append(parts, fmt.Sprintf("%s=%v", name, val))
					}
				}
			}
		}
	} else {
		// Simple format
		for _, p := range feature.Parameters {
			if v, ok := body[p.Name]; ok {
				parts = append(parts, fmt.Sprintf("%s=%v", p.Name, v))
			}
		}
	}
	if len(parts) == 0 {
		return feature.Name
	}
	return strings.Join(parts, ", ")
}

// describeModifiedFields returns a human-readable string of non-key/non-required fields
// being modified in an update body, e.g. "metric=50 (was: 100)".
// This makes update test descriptions clearly state what is being updated.
func describeModifiedFields(body map[string]interface{}, feature *model.Feature) string {
	if body == nil {
		return ""
	}
	keySet := make(map[string]bool, len(feature.Keys))
	for _, k := range feature.Keys {
		keySet[k] = true
	}
	requiredSet := make(map[string]bool)
	for _, p := range feature.Parameters {
		if p.Required {
			requiredSet[p.Name] = true
		}
	}

	var modified []string
	var keys []string

	// Deep-scanned format
	if objects, ok := body["objects"].([]interface{}); ok && len(objects) > 0 {
		if obj, ok := objects[0].(map[string]interface{}); ok {
			if props, ok := obj["properties"].([]map[string]interface{}); ok {
				for _, prop := range props {
					name, _ := prop["name"].(string)
					val := prop["value"]
					if isSystemManagedField(name) {
						continue
					}
					if keySet[name] || requiredSet[name] {
						keys = append(keys, fmt.Sprintf("%s=%v", name, val))
					} else {
						modified = append(modified, fmt.Sprintf("%s=%v", name, val))
					}
				}
			}
		}
	} else {
		for _, p := range feature.Parameters {
			if v, ok := body[p.Name]; ok {
				if isSystemManagedField(p.Name) {
					continue
				}
				if keySet[p.Name] || requiredSet[p.Name] {
					keys = append(keys, fmt.Sprintf("%s=%v", p.Name, v))
				} else {
					modified = append(modified, fmt.Sprintf("%s=%v", p.Name, v))
				}
			}
		}
	}

	if len(modified) == 0 && len(keys) > 0 {
		return "update entry where " + strings.Join(keys, ", ")
	}
	if len(modified) == 0 {
		return feature.Name
	}
	desc := "modify " + strings.Join(modified, ", ")
	if len(keys) > 0 {
		desc += " (entry: " + strings.Join(keys, ", ") + ")"
	}
	return desc
}

// crudValidations generates descriptive validations for CRUD steps.
func crudValidations(statusCode int, operation string, featureName string, keyDescription string) []model.Validation {
	var desc string
	switch operation {
	case "create":
		desc = fmt.Sprintf("Verify %s created successfully with %s", featureName, keyDescription)
	case "read":
		desc = fmt.Sprintf("Verify %s retrieved — response contains %s", featureName, keyDescription)
	case "update":
		desc = fmt.Sprintf("Verify %s updated successfully with %s", featureName, keyDescription)
	case "delete":
		desc = fmt.Sprintf("Verify %s with %s deleted successfully", featureName, keyDescription)
	case "conflict":
		desc = fmt.Sprintf("Verify duplicate create of %s with %s is rejected (409 Conflict)", featureName, keyDescription)
	default:
		desc = fmt.Sprintf("Verify HTTP %d for %s %s", statusCode, operation, featureName)
	}
	return []model.Validation{
		{
			Type:        model.ValidationTypeStatusCode,
			Expected:    statusCode,
			Description: desc,
		},
	}
}

// getYANGTypeValue returns a representative valid value for a given YANG type and variation index.
func (g *Generator) getYANGTypeValue(param model.Parameter, yangType string, variation int) interface{} {
	// If param has enum constraints, use the enum values
	for _, c := range param.Constraints {
		if c.Type == model.ConstraintTypeEnum {
			if vals, ok := c.Value.([]string); ok && len(vals) > 0 {
				idx := (variation - 1) % len(vals)
				return vals[idx]
			}
		}
	}

	lowerType := strings.ToLower(yangType)

	// Boolean
	if lowerType == "boolean" || lowerType == "bool" {
		return variation%2 == 1
	}

	// Integer types
	if strings.HasPrefix(lowerType, "int") || strings.HasPrefix(lowerType, "uint") ||
		lowerType == "counter32" || lowerType == "counter64" || lowerType == "gauge32" {
		values := []int{1, 100}
		// Respect min/max constraints
		var minVal, maxVal int
		hasMin, hasMax := false, false
		for _, c := range param.Constraints {
			if c.Type == model.ConstraintTypeMin {
				if v, ok := c.Value.(int); ok {
					minVal = v
					hasMin = true
				}
			}
			if c.Type == model.ConstraintTypeMax {
				if v, ok := c.Value.(int); ok {
					maxVal = v
					hasMax = true
				}
			}
		}
		if hasMin && hasMax {
			values = []int{minVal, maxVal}
		} else if hasMin {
			values = []int{minVal, minVal + 10}
		} else if hasMax {
			values = []int{maxVal / 2, maxVal}
		}
		if variation <= len(values) {
			return values[variation-1]
		}
		return variation * 10
	}

	// IP address types
	if strings.Contains(lowerType, "ip") || strings.Contains(lowerType, "inet") ||
		strings.Contains(lowerType, "address") {
		addrs := []string{"192.168.1.1", "10.0.0.1"}
		return addrs[(variation-1)%len(addrs)]
	}

	// String with pattern constraint — use pattern-derived value via getSampleValue
	if lowerType == "string" {
		for _, c := range param.Constraints {
			if c.Type == model.ConstraintTypePattern {
				return g.getSampleValue(param)
			}
		}
		variants := []string{"test-value-1", "test-value-2"}
		return variants[(variation-1)%len(variants)]
	}

	// Fallback to existing sample value logic
	return g.getSampleValue(param)
}

// generateOptionalFieldInclusionTests generates one non-deployment test per optional YANG field
// where that field IS included (so we verify the server stores and returns the value correctly).
// The companion "without" test is in generateNegativeTests → generateOptionalFieldNullTest.
func (g *Generator) generateOptionalFieldInclusionTests(
	feature *model.Feature,
	createPath *model.FeaturePath,
	readPath *model.FeaturePath,
) []model.TestCase {
	var tests []model.TestCase

	for _, param := range feature.Parameters {
		if param.Required {
			continue // only optional fields here
		}
		if isSystemManagedField(param.Name) {
			continue // skip id, created-at, updated-at, deleted-at, customer-id, owner-id
		}

		expectedVal := g.getSampleValue(param)

		// Build a richer description using the YANG leaf description when available
		paramDesc := param.Description
		var tcDesc string
		if paramDesc != "" {
			tcDesc = fmt.Sprintf("Verify optional field '%s' is accepted and persisted (YANG type: %s) — %s", param.Name, param.YangType, paramDesc)
		} else {
			tcDesc = fmt.Sprintf("Verify optional field '%s' is accepted and persisted when provided (YANG type: %s)", param.Name, param.YangType)
		}

		tc := model.TestCase{
			TestCaseID:       g.nextTestID(),
			FeatureName:      feature.Name,
			Priority:         model.TestPriorityP3,
			Type:             model.TestCategoryFunctional,
			Description:      tcDesc,
			IsDeploymentTest: false,
			Steps:            []model.TestStep{},
		}

		// Body: all required fields + this optional field
		body := g.generateRequiredOnlyBody(feature, createPath)
		g.setBodyParameterValue(body, param.Name, expectedVal)
		// For deep-scanned format we need to add it to the properties array
		if createPath.BlueprintCategory != "" {
			body = g.generateBodyWithOptionalField(feature, createPath, param.Name, expectedVal)
		}

		var createStepDesc string
		if paramDesc != "" {
			createStepDesc = fmt.Sprintf("Create %s with optional field '%s'=%v — %s", feature.Name, param.Name, expectedVal, paramDesc)
		} else {
			createStepDesc = fmt.Sprintf("Create %s with optional field '%s'=%v explicitly provided", feature.Name, param.Name, expectedVal)
		}

		createStep := model.TestStep{
			Name:           fmt.Sprintf("createWith_optional_%s", strings.ReplaceAll(param.Name, "-", "_")),
			Description:    createStepDesc,
			Method:         createPath.HTTPMethod,
			API:            model.APITypeREST,
			Path:           createPath.Path,
			Body:           body,
			ExpectedStatus: 201,
			Validations: []model.Validation{
				{
					Type:        model.ValidationTypeStatusCode,
					Expected:    201,
					Description: fmt.Sprintf("Verify resource is created when optional field '%s' is provided", param.Name),
				},
			},
		}
		tc.Steps = append(tc.Steps, createStep)

		// GET and verify the optional field value was stored correctly
		if readPath != nil {
			readStep := model.TestStep{
				Name:           fmt.Sprintf("verify_optional_%s_persisted", strings.ReplaceAll(param.Name, "-", "_")),
				Description:    fmt.Sprintf("GET %s and verify optional field '%s' was persisted with value '%v'", feature.Name, param.Name, expectedVal),
				Method:         readPath.HTTPMethod,
				API:            model.APITypeREST,
				Path:           readPath.Path,
				Body:           g.generateReadBody(feature, createPath),
				ExpectedStatus: 200,
				Validations: []model.Validation{
					{
						Type:        model.ValidationTypeStatusCode,
						Expected:    200,
						Description: "Verify HTTP 200",
					},
					{
						Type:        model.ValidationTypeJSONPathExists,
						Path:        fmt.Sprintf("$.objects[0].properties[?(@.name=='%s')]", param.Name),
						Description: fmt.Sprintf("Verify optional field '%s' is present in GET response", param.Name),
					},
					{
						Type:        model.ValidationTypeJSONPathEquals,
						Path:        fmt.Sprintf("$.objects[0].properties[?(@.name=='%s')].value", param.Name),
						Expected:    expectedVal,
						Description: fmt.Sprintf("Verify optional field '%s' value is '%v' as set on create", param.Name, expectedVal),
					},
				},
			}
			tc.Steps = append(tc.Steps, readStep)
		}

		tests = append(tests, tc)
	}

	return tests
}

// generateBodyWithOptionalField builds a deep-scanned body that includes all required fields
// plus one specific optional field with the given value.
func (g *Generator) generateBodyWithOptionalField(feature *model.Feature, fp *model.FeaturePath, optParamName string, optParamValue interface{}) map[string]interface{} {
	var fpValue string
	for _, p := range fp.PathParams {
		if p.Name == "featurePath" && p.FixedValue != "" {
			fpValue = p.FixedValue
			break
		}
	}
	if fpValue == "" {
		// Fallback to simple format
		body := g.generateRequiredOnlyBody(feature, fp)
		body[optParamName] = optParamValue
		return body
	}

	properties := []map[string]interface{}{}
	for _, param := range feature.Parameters {
		var value interface{}
		if param.Name == optParamName {
			value = optParamValue
		} else if param.Required {
			value = g.getSampleValue(param)
		} else {
			continue // skip other optional fields
		}
		properties = append(properties, map[string]interface{}{
			"name":   param.Name,
			"type":   g.mapYangTypeToJsonType(param.GoType),
			"value":  value,
			"origin": "Global",
		})
	}
	object := map[string]interface{}{
		"type":       feature.Name,
		"operation":  "add",
		"properties": properties,
	}
	return map[string]interface{}{
		"featurePath": fpValue,
		"objectType":  feature.Name,
		"operation":   "add",
		"objects":     []interface{}{object},
	}
}
