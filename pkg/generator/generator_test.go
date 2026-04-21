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
	gen := NewGenerator(config, features, featurePaths, nil)

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

	gen := NewGenerator(config, nil, nil, nil)

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
	gen := NewGenerator(config, nil, nil, nil)

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
	gen := NewGenerator(config, nil, nil, nil)

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
	gen := NewGenerator(config, nil, nil, nil)

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
