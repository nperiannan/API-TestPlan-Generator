package model

import (
	"testing"
)

func TestNewDefaultConfig(t *testing.T) {
	config := NewDefaultConfig()

	if config == nil {
		t.Fatal("NewDefaultConfig returned nil")
	}

	if len(config.IncludeCategories) != 5 {
		t.Errorf("Expected 5 categories, got %d", len(config.IncludeCategories))
	}

	if config.ScaleFactor != 100 {
		t.Errorf("Expected scale factor 100, got %d", config.ScaleFactor)
	}

	if config.TestIDPrefix != "TCXM" {
		t.Errorf("Expected prefix TCXM, got %s", config.TestIDPrefix)
	}

	if config.StartingIDNumber != 1000 {
		t.Errorf("Expected starting ID 1000, got %d", config.StartingIDNumber)
	}
}

func TestProfileTypes(t *testing.T) {
	tests := []struct {
		name     string
		pType    ProfileType
		expected string
	}{
		{"Global", ProfileTypeGlobal, "global"},
		{"Configuration", ProfileTypeConfiguration, "configuration"},
		{"Service", ProfileTypeService, "service"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.pType) != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, tt.pType)
			}
		})
	}
}

func TestScopeTypes(t *testing.T) {
	tests := []struct {
		name     string
		sType    ScopeType
		expected string
	}{
		{"SiteGroup", ScopeTypeSiteGroup, "site-group"},
		{"Device", ScopeTypeDevice, "device"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.sType) != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, tt.sType)
			}
		})
	}
}

func TestTestCategories(t *testing.T) {
	categories := []TestCategory{
		TestCategoryFunctional,
		TestCategoryBoundary,
		TestCategoryNegative,
		TestCategoryScale,
		TestCategoryPerformance,
	}

	if len(categories) != 5 {
		t.Errorf("Expected 5 test categories, got %d", len(categories))
	}
}
