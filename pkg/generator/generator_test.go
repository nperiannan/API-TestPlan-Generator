package generator

import (
	"testing"

	"github.com/extremenetworks/testcase-generator/pkg/model"
)

func TestGeneratorCreation(t *testing.T) {
	config := model.NewDefaultConfig()
	features := make(map[string]*model.Feature)
	featurePaths := make(map[string]*model.FeaturePath)

	// Create mock NOSAPI parser (nil for this test)
	gen := NewGenerator(config, features, featurePaths, nil, nil)

	if gen == nil {
		t.Fatal("NewGenerator returned nil")
	}

	if gen.config != config {
		t.Error("Config not set correctly")
	}

	if gen.testIDCounter != config.StartingIDNumber {
		t.Errorf("Expected testIDCounter %d, got %d", config.StartingIDNumber, gen.testIDCounter)
	}
}

func TestNextTestID(t *testing.T) {
	config := model.NewDefaultConfig()
	config.TestIDPrefix = "TEST"
	config.StartingIDNumber = 1

	gen := NewGenerator(config, nil, nil, nil, nil)

	testID1 := gen.nextTestID()
	if testID1 != "TEST_0001" {
		t.Errorf("Expected TEST_0001, got %s", testID1)
	}

	testID2 := gen.nextTestID()
	if testID2 != "TEST_0002" {
		t.Errorf("Expected TEST_0002, got %s", testID2)
	}
}

func TestExtractFeatureName(t *testing.T) {
	config := model.NewDefaultConfig()
	gen := NewGenerator(config, nil, nil, nil, nil)

	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{
			name:     "Configuration profile path",
			path:     "/configuration/v1/configuration-profile/{name}",
			expected: "configuration-profile",
		},
		{
			name:     "Nested feature path",
			path:     "/configuration/v1/configuration-profile/{name}/vlan",
			expected: "vlan",
		},
		{
			name:     "Simple path",
			path:     "/api/v1/devices",
			expected: "devices",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fp := &model.FeaturePath{Path: tt.path}
			result := gen.extractFeatureName(fp)
			if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestGenerateSampleBody(t *testing.T) {
	config := model.NewDefaultConfig()
	gen := NewGenerator(config, nil, nil, nil, nil)

	feature := &model.Feature{
		Name: "TestFeature",
		Parameters: []model.Parameter{
			{
				Name:     "requiredField",
				Required: true,
				GoType:   "string",
			},
			{
				Name:     "optionalField",
				Required: false,
				GoType:   "int",
			},
		},
	}

	body := gen.generateSampleBody(feature, nil)

	if body["name"] != "TestResource" {
		t.Error("Expected name field to be TestResource")
	}

	if _, ok := body["requiredField"]; !ok {
		t.Error("Expected requiredField to be present")
	}

	// Optional field should not be in body
	if _, ok := body["optionalField"]; ok {
		t.Error("Optional field should not be in body")
	}
}

func TestGetSampleValue(t *testing.T) {
	config := model.NewDefaultConfig()
	gen := NewGenerator(config, nil, nil, nil, nil)

	tests := []struct {
		name     string
		param    model.Parameter
		expected interface{}
	}{
		{
			name:     "String type",
			param:    model.Parameter{GoType: "string"},
			expected: "testValue",
		},
		{
			name:     "Int type",
			param:    model.Parameter{GoType: "int"},
			expected: 100,
		},
		{
			name:     "Bool type",
			param:    model.Parameter{GoType: "bool"},
			expected: true,
		},
		{
			name:     "With default",
			param:    model.Parameter{GoType: "string", DefaultValue: "custom"},
			expected: "custom",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := gen.getSampleValue(tt.param)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestGetSampleValueUsesYANGSemanticsBeforeNameHeuristics(t *testing.T) {
	config := model.NewDefaultConfig()
	gen := NewGenerator(config, nil, nil, nil, nil)

	tests := []struct {
		name     string
		param    model.Parameter
		expected interface{}
	}{
		{
			name:     "IP subnet name is generated as a name",
			param:    model.Parameter{Name: "ip-subnet-name", GoType: "string", Description: "Name of the IP Subnet instance."},
			expected: "test-ip-subnet",
		},
		{
			name:     "VRD name is generated as a name",
			param:    model.Parameter{Name: "vrd-name", GoType: "string", Description: "Virtual Routing domain name where this static route will be configured."},
			expected: "test-vrd",
		},
		{
			name:     "VRD association remains a UUID reference",
			param:    model.Parameter{Name: "vrd-association", GoType: "string", Description: "UUID of Virtual Routing Domain associated with this subnet."},
			expected: "123e4567-e89b-12d3-a456-426614174000",
		},
		{
			name:     "Static route VRD association remains a UUID reference",
			param:    model.Parameter{Name: "sr-vrd-association", YangType: "leafref", GoType: "string", Description: "UUID of the associated VRD object."},
			expected: "123e4567-e89b-12d3-a456-426614174000",
		},
		{
			name:     "CIDR subnet address includes prefix",
			param:    model.Parameter{Name: "ipv4-subnet-address", GoType: "string", Description: "IPv4 subnet address in CIDR notation."},
			expected: "192.168.1.0/24",
		},
		{
			name:     "Subnet mask is a mask",
			param:    model.Parameter{Name: "ipv4-subnet-mask", GoType: "string", Description: "IPv4 subnet mask."},
			expected: "255.255.255.0",
		},
		{
			name:     "Gateway address is in subnet host range",
			param:    model.Parameter{Name: "ipv4-gateway-address", GoType: "string", Description: "IPv4 gateway address for the subnet."},
			expected: "192.168.1.1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := gen.getSampleValue(tt.param); got != tt.expected {
				t.Fatalf("expected %v, got %v", tt.expected, got)
			}
		})
	}
}

func TestDeepScannedStaticRouteUsesVRDNameValue(t *testing.T) {
	config := model.NewDefaultConfig()
	gen := NewGenerator(config, nil, nil, nil, nil)
	feature := &model.Feature{
		Name: "static-route",
		Keys: []string{"vrd-name", "route-name"},
		Parameters: []model.Parameter{
			{Name: "vrd-name", GoType: "string", Required: true, Description: "Virtual Routing domain name where this static route will be configured."},
			{Name: "route-name", GoType: "string", Required: true, Description: "Unique name identifier for this static route within the VRF."},
			{Name: "destination-subnet", GoType: "string", Required: true, Description: "Destination IP subnet in CIDR notation or IP address format."},
			{Name: "mask", GoType: "string", Required: true, Description: "Subnet mask length (prefix length) for the destination subnet."},
			{Name: "next-hop-ip", GoType: "string", Required: true, Description: "Next hop gateway IP address for reaching the destination subnet."},
		},
	}
	path := &model.FeaturePath{
		BlueprintCategory: model.BlueprintCategoryService,
		PathParams: []model.PathParameter{
			{Name: "featurePath", FixedValue: "/virtual-service-feature"},
			{Name: "objectType", FixedValue: "static-route"},
		},
	}

	body := gen.generateRequestBody(feature, path)
	expected := expectedValuesFromBody(feature, body)
	if expected["vrd-name"] != "test-vrd" {
		t.Fatalf("expected vrd-name to use a name value, got %#v", expected)
	}
	if expected["route-name"] != "test-route" {
		t.Fatalf("expected route-name to use route name value, got %#v", expected)
	}
}

func TestGetUpdatedValueUsesYANGSemanticsBeforeStringFallback(t *testing.T) {
	config := model.NewDefaultConfig()
	gen := NewGenerator(config, nil, nil, nil, nil)

	tests := []struct {
		name     string
		param    model.Parameter
		expected interface{}
	}{
		{
			name:     "VRD name update remains a name",
			param:    model.Parameter{Name: "vrd-name", GoType: "string", Description: "Virtual Routing domain name where this static route will be configured."},
			expected: "updated-vrd",
		},
		{
			name:     "System name update remains a name",
			param:    model.Parameter{Name: "sys-name", GoType: "string", Description: "System Name."},
			expected: "updated-sys",
		},
		{
			name:     "UUID association update remains a UUID reference",
			param:    model.Parameter{Name: "vrd-association", GoType: "string", Description: "UUID of Virtual Routing Domain associated with this subnet."},
			expected: "550e8400-e29b-41d4-a716-446655440000",
		},
		{
			name:     "Leafref update remains a UUID reference",
			param:    model.Parameter{Name: "sr-vrd-association", YangType: "leafref", GoType: "string", Description: "UUID of the associated VRD object."},
			expected: "550e8400-e29b-41d4-a716-446655440000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := gen.getUpdatedValue(tt.param); got != tt.expected {
				t.Fatalf("expected %v, got %v", tt.expected, got)
			}
		})
	}
}

func TestOverrideModifyUsesMatchingPropertyParameter(t *testing.T) {
	config := model.NewDefaultConfig()
	gen := NewGenerator(config, nil, nil, nil, nil)
	feature := &model.Feature{
		Name: "snmp-global-config",
		Parameters: []model.Parameter{
			{Name: "device-id", GoType: "string", Description: "UUID of the device."},
			{Name: "sys-name", GoType: "string", Required: true, Description: "System Name."},
		},
	}
	path := &model.FeaturePath{
		HTTPMethod:        "POST",
		Path:              "/configuration-profile/{name}/feature/object/modify",
		BlueprintCategory: model.BlueprintCategoryWired,
		PathParams: []model.PathParameter{
			{Name: "featurePath", FixedValue: "/snmp-feature"},
			{Name: "objectType", FixedValue: "snmp-global-config"},
		},
	}

	testCase := gen.generateOverrideCRUDLifecycleTest(feature, path, "TestProfile", "device", "/snmp-feature", "snmp-global-config")
	var modifyStep *model.TestStep
	for i := range testCase.Steps {
		if testCase.Steps[i].Name == "modifyDeviceOverride" {
			modifyStep = &testCase.Steps[i]
			break
		}
	}
	if modifyStep == nil {
		t.Fatalf("modify override step not generated: %#v", testCase.Steps)
	}
	body, ok := modifyStep.Body.(map[string]interface{})
	if !ok {
		t.Fatalf("expected body map, got %#v", modifyStep.Body)
	}
	props, ok := body["updateProperties"].([]map[string]interface{})
	if !ok || len(props) == 0 {
		t.Fatalf("expected updateProperties, got %#v", body)
	}
	if props[0]["name"] != "sys-name" || props[0]["value"] != "test-sys" {
		t.Fatalf("expected sys-name to use its own sample value, got %#v", props[0])
	}
}

func TestReadValidationsOnlyAssertExpectedAndIdentityFields(t *testing.T) {
	config := model.NewDefaultConfig()
	gen := NewGenerator(config, nil, nil, nil, nil)
	feature := &model.Feature{
		Name: "ip-subnet",
		Keys: []string{"ip-subnet-name", "vrd-association"},
		Parameters: []model.Parameter{
			{Name: "ip-subnet-name", GoType: "string"},
			{Name: "vrd-association", GoType: "string"},
			{Name: "description", GoType: "string"},
			{Name: "ipv4-subnet-address", GoType: "string"},
		},
	}

	validations := gen.generateReadResponseValidationsWithValues(feature, nil, map[string]interface{}{
		"ip-subnet-name": "test-ip-subnet",
	})

	for _, validation := range validations {
		if validation.Expected == "192.168.1.0" || validation.Description == "Verify 'description' value is '192.168.1.0' as set during create" {
			t.Fatalf("unexpected generated sample assertion: %+v", validation)
		}
	}
}

func TestDeepScannedDeleteBodyUsesObjectID(t *testing.T) {
	config := model.NewDefaultConfig()
	gen := NewGenerator(config, nil, nil, nil, nil)
	feature := &model.Feature{Name: "dns-server"}
	path := &model.FeaturePath{
		BlueprintCategory: model.BlueprintCategoryGlobal,
		PathParams: []model.PathParameter{
			{Name: "featurePath", FixedValue: "/dns-server-feature"},
		},
	}

	body := gen.generateDeleteBody(feature, path)
	objectIDs, ok := body["objectIds"].([]string)
	if !ok || len(objectIDs) != 1 || objectIDs[0] != "{{OBJECT_ID}}" {
		t.Fatalf("expected objectIds placeholder, got %#v", body)
	}
	if _, exists := body["featurePath"]; exists {
		t.Fatalf("delete body should not delete all objects by featurePath: %#v", body)
	}
}

func TestDeepScannedBodyIncludesDeclaredKeysMissingFromParameters(t *testing.T) {
	config := model.NewDefaultConfig()
	gen := NewGenerator(config, nil, nil, nil, nil)
	feature := &model.Feature{
		Name: "ip-subnet",
		Keys: []string{"ip-subnet-name", "vrd-association"},
		Parameters: []model.Parameter{
			{Name: "ip-subnet-name", GoType: "string"},
		},
	}
	path := &model.FeaturePath{
		BlueprintCategory: model.BlueprintCategoryService,
		PathParams: []model.PathParameter{
			{Name: "featurePath", FixedValue: "/l2-service-feature"},
		},
	}

	body := gen.generateRequestBody(feature, path)
	expected := expectedValuesFromBody(feature, body)
	if expected["ip-subnet-name"] != "test-ip-subnet" {
		t.Fatalf("expected parsed key value, got %#v", expected)
	}
	if expected["vrd-association"] != "123e4567-e89b-12d3-a456-426614174000" {
		t.Fatalf("expected synthetic key value, got %#v", expected)
	}

	readBody := gen.generateReadBody(feature, path)
	filters, ok := readBody["filters"].([]map[string]interface{})
	if !ok || len(filters) != 2 {
		t.Fatalf("expected filters for both YANG keys, got %#v", readBody)
	}
}

func TestDeepScannedBodySkipsSystemManagedIDKey(t *testing.T) {
	config := model.NewDefaultConfig()
	gen := NewGenerator(config, nil, nil, nil, nil)
	feature := &model.Feature{
		Name: "common-settings",
		Keys: []string{"id"},
		Parameters: []model.Parameter{
			{Name: "id", GoType: "string", Required: true},
			{Name: "enabled", GoType: "bool", Required: true},
		},
	}
	path := &model.FeaturePath{
		BlueprintCategory: model.BlueprintCategoryService,
		PathParams: []model.PathParameter{
			{Name: "featurePath", FixedValue: "/common-settings-feature"},
		},
	}

	body := gen.generateRequestBody(feature, path)
	objects := body["objects"].([]interface{})
	object := objects[0].(map[string]interface{})
	properties := object["properties"].([]map[string]interface{})
	for _, property := range properties {
		if property["name"] == "id" {
			t.Fatalf("system-managed id must not be emitted as payload property: %#v", body)
		}
	}
	expected := expectedValuesFromBody(feature, body)
	if _, exists := expected["id"]; exists {
		t.Fatalf("system-managed id must not be emitted as payload property: %#v", body)
	}
	if expected["enabled"] != true {
		t.Fatalf("expected non-system required field, got %#v", expected)
	}
}

func TestDeleteNonExistentUsesEndpointMethodAndObjectIds(t *testing.T) {
	config := model.NewDefaultConfig()
	gen := NewGenerator(config, nil, nil, nil, nil)
	feature := &model.Feature{Name: "dns-server"}
	deletePath := &model.FeaturePath{
		HTTPMethod:        "POST",
		Path:              "/global-profile/{name}/feature/object/delete",
		BlueprintCategory: model.BlueprintCategoryGlobal,
		PathParams: []model.PathParameter{
			{Name: "featurePath", FixedValue: "/dns-server-feature"},
		},
	}

	testCase := gen.generateDeleteNonExistentTest(feature, deletePath)
	if len(testCase.Steps) != 1 {
		t.Fatalf("expected one step, got %#v", testCase.Steps)
	}
	step := testCase.Steps[0]
	if step.Method != "POST" {
		t.Fatalf("expected endpoint method POST, got %s", step.Method)
	}
	body, ok := step.Body.(map[string]interface{})
	if !ok {
		t.Fatalf("expected body map, got %#v", step.Body)
	}
	if _, exists := body["operation"]; exists {
		t.Fatalf("deep-scanned non-existent delete must use objectIds, got %#v", body)
	}
	objectIDs, ok := body["objectIds"].([]string)
	if !ok || len(objectIDs) != 1 || objectIDs[0] == "" {
		t.Fatalf("expected non-empty objectIds body, got %#v", body)
	}
}

func TestSelectPrimaryObjectPathSkipsDeployStatusPaths(t *testing.T) {
	paths := []*model.FeaturePath{
		{
			Path:          "/configuration-profile/{name}/device/{hostName}/deploy/status",
			OperationType: model.OperationTypeStatus,
			PathParams: []model.PathParameter{
				{Name: "featurePath", FixedValue: "/wrong"},
			},
		},
		{
			Path:          "/service-profile/{name}/feature/object/modify",
			OperationType: model.OperationTypeCreate,
			PathParams: []model.PathParameter{
				{Name: "featurePath", FixedValue: "/l2-service-feature"},
			},
		},
	}

	selected := selectPrimaryObjectPath(paths)
	if selected == nil || selected.Path != "/service-profile/{name}/feature/object/modify" {
		t.Fatalf("expected object modify path, got %#v", selected)
	}
}

func TestBasicDeleteUsesCreatePathForSetup(t *testing.T) {
	config := model.NewDefaultConfig()
	gen := NewGenerator(config, nil, nil, nil, nil)
	feature := &model.Feature{Name: "dns-server"}
	createPath := &model.FeaturePath{
		HTTPMethod:        "POST",
		Path:              "/configuration-profile/{name}/feature/object/modify",
		BlueprintCategory: model.BlueprintCategoryGlobal,
		PathParams: []model.PathParameter{
			{Name: "featurePath", FixedValue: "/dns-server-feature"},
			{Name: "objectType", FixedValue: "dns-server"},
		},
	}
	deletePath := &model.FeaturePath{
		HTTPMethod:        "POST",
		Path:              "/configuration-profile/{name}/feature/object/delete",
		BlueprintCategory: model.BlueprintCategoryGlobal,
		PathParams: []model.PathParameter{
			{Name: "featurePath", FixedValue: "/dns-server-feature"},
		},
	}
	readPath := &model.FeaturePath{
		HTTPMethod:        "POST",
		Path:              "/configuration-profile/{name}/feature/object/retrieve",
		BlueprintCategory: model.BlueprintCategoryGlobal,
		PathParams: []model.PathParameter{
			{Name: "featurePath", FixedValue: "/dns-server-feature"},
		},
	}

	testCase := gen.generateBasicDeleteTest(feature, createPath, deletePath, readPath)
	if len(testCase.Steps) != 3 {
		t.Fatalf("expected three delete-flow steps, got %d", len(testCase.Steps))
	}
	if testCase.Steps[0].Path != createPath.Path {
		t.Fatalf("setup step should use create path, got %s", testCase.Steps[0].Path)
	}
	if testCase.Steps[1].Path != deletePath.Path {
		t.Fatalf("delete step should use delete path, got %s", testCase.Steps[1].Path)
	}
	if testCase.Steps[2].Path != readPath.Path {
		t.Fatalf("verify step should use read path, got %s", testCase.Steps[2].Path)
	}
}

func TestNormalizeSuiteAddsPathParamsForPlaceholders(t *testing.T) {
	config := model.NewDefaultConfig()
	gen := NewGenerator(config, nil, nil, nil, nil)
	suite := &model.TestSuite{
		Features: []model.FeatureTestGroup{{
			FeatureName: "port",
			Tests: map[model.TestCategory][]model.TestCase{
				model.TestCategoryFunctional: {{
					TestCaseID: "TC_0001",
					Steps: []model.TestStep{{
						Name: "getPort",
						Path: "/configuration-profile/{name}/ports/{port}/status",
					}},
				}},
			},
		}},
	}

	gen.normalizeSuite(suite)
	params := suite.Features[0].Tests[model.TestCategoryFunctional][0].Steps[0].PathParams
	if params["name"] != "TestProfile" {
		t.Fatalf("expected name path param, got %#v", params)
	}
	if params["port"] != "1/1" {
		t.Fatalf("expected port path param, got %#v", params)
	}
}

func TestObjectIDCaptureIncludesStructuredMetadata(t *testing.T) {
	validations := withObjectIDCapture(statusValidation(201))
	last := validations[len(validations)-1]
	if last.CaptureAs != "OBJECT_ID" || last.Path != "$[0].id" {
		t.Fatalf("expected structured object ID capture, got %#v", last)
	}
}

func TestKeyTransitionDeleteUsesCapturedObjectIDBody(t *testing.T) {
	config := model.NewDefaultConfig()
	gen := NewGenerator(config, nil, nil, nil, nil)
	feature := &model.Feature{
		Name:       "static-route",
		Keys:       []string{"vr-name"},
		Parameters: []model.Parameter{{Name: "vr-name", GoType: "string", Required: true}},
	}
	createPath := &model.FeaturePath{
		HTTPMethod:        "POST",
		Path:              "/service-profile/{name}/feature/object/modify",
		BlueprintCategory: model.BlueprintCategoryService,
		PathParams: []model.PathParameter{
			{Name: "featurePath", FixedValue: "/static-route-feature"},
			{Name: "objectType", FixedValue: "static-route"},
		},
	}
	deletePath := &model.FeaturePath{
		HTTPMethod:        "POST",
		Path:              "/service-profile/{name}/feature/object/delete",
		BlueprintCategory: model.BlueprintCategoryService,
		PathParams:        []model.PathParameter{{Name: "featurePath", FixedValue: "/static-route-feature"}},
	}

	testCase := gen.generateKeyFieldTransitionTest(feature, createPath, nil, deletePath, feature.Parameters[0], []string{"vr-default", "vr-mgmt"})
	var deleteStep *model.TestStep
	for i := range testCase.Steps {
		if testCase.Steps[i].Name == "deleteKeyA" {
			deleteStep = &testCase.Steps[i]
			break
		}
	}
	if deleteStep == nil {
		t.Fatalf("delete step not generated: %#v", testCase.Steps)
	}
	if deleteStep.ExpectedStatus != 200 {
		t.Fatalf("expected delete status 200, got %d", deleteStep.ExpectedStatus)
	}
	body, ok := deleteStep.Body.(map[string]interface{})
	if !ok {
		t.Fatalf("expected delete body map, got %#v", deleteStep.Body)
	}
	objectIDs, ok := body["objectIds"].([]string)
	if !ok || len(objectIDs) != 1 || objectIDs[0] != "{{OBJECT_ID_A}}" {
		t.Fatalf("expected captured objectIds body, got %#v", body)
	}
	if _, exists := body["objects"]; exists {
		t.Fatalf("delete body must not use add/update objects payload: %#v", body)
	}
}

func TestShouldIncludeFeatureExcludesUnclassifiedWhenCategoryFiltered(t *testing.T) {
	config := model.NewDefaultConfig()
	config.FeatureCategories = []model.BlueprintCategory{model.BlueprintCategoryWired}
	gen := NewGenerator(config, nil, nil, nil, nil)

	if gen.shouldIncludeFeature(&model.FeaturePath{Path: "/inventory/models/retrieve"}) {
		t.Fatalf("unclassified REST endpoint should not be included in category-filtered generation")
	}
	if !gen.shouldIncludeFeature(&model.FeaturePath{Path: "/configuration-profile/{name}/feature/object/modify", BlueprintCategory: model.BlueprintCategoryWired}) {
		t.Fatalf("matching categorized path should be included")
	}
}

func TestEnumCrossProductReadBodyUsesYangFeaturePath(t *testing.T) {
	config := model.NewDefaultConfig()
	gen := NewGenerator(config, nil, nil, nil, nil)
	feature := &model.Feature{
		Name: "device-profile",
		Parameters: []model.Parameter{
			{Name: "mode", GoType: "string", Constraints: []model.Constraint{{Type: model.ConstraintTypeEnum, Value: []string{"DHCP", "STATIC"}}}},
			{Name: "mtu", GoType: "string", Constraints: []model.Constraint{{Type: model.ConstraintTypeEnum, Value: []string{"1500", "1522"}}}},
		},
	}
	createPath := &model.FeaturePath{
		HTTPMethod:        "POST",
		Path:              "/configuration-profile/{name}/feature/object/modify",
		BlueprintCategory: model.BlueprintCategoryWired,
		PathParams: []model.PathParameter{
			{Name: "featurePath", FixedValue: "/infrastructure-feature/device-profile-feature"},
			{Name: "objectType", FixedValue: "device-profile"},
		},
	}
	readPath := &model.FeaturePath{
		HTTPMethod:        "POST",
		Path:              "/configuration-profile/{name}/feature/object/retrieve",
		BlueprintCategory: model.BlueprintCategoryWired,
	}

	tests := gen.generateEnumCrossProductTests(feature, createPath, readPath)
	if len(tests) == 0 || len(tests[0].Steps) < 2 {
		t.Fatalf("expected enum cross-product test with verify step, got %#v", tests)
	}
	body, ok := tests[0].Steps[1].Body.(map[string]interface{})
	if !ok {
		t.Fatalf("expected read body map, got %#v", tests[0].Steps[1].Body)
	}
	if body["featurePath"] != "/infrastructure-feature/device-profile-feature" {
		t.Fatalf("expected YANG featurePath, got %#v", body)
	}
}
