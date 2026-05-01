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
