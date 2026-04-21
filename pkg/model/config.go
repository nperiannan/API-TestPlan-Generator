package model

// GeneratorConfig holds configuration for test generation
type GeneratorConfig struct {
	YangDir        string
	RESTSpecPath   string
	NOSAPISpecPath string
	OutputDir      string

	// Test generation options
	IncludeCategories []TestCategory
	Features          []string            // Filter by specific feature names (e.g., dns-server, ntp-server)
	FeatureCategories []BlueprintCategory // Filter deep-scanned features by blueprint category
	ScopeTypes        []ScopeType
	TargetTypes       []TargetType
	DeploymentMethods []DeploymentMethod

	// Scale and performance settings
	ScaleFactor            int
	PerformanceIterations  int
	PerformanceConcurrency int

	// Output options
	OneFilePerFeature bool
	IncludeMetadata   bool

	// ID generation
	TestIDPrefix     string
	StartingIDNumber int
}

// NewDefaultConfig creates a default generator configuration
func NewDefaultConfig() *GeneratorConfig {
	return &GeneratorConfig{
		IncludeCategories: []TestCategory{
			TestCategoryFunctional,
			TestCategoryBoundary,
			TestCategoryNegative,
			TestCategoryScale,
			TestCategoryPerformance,
		},
		FeatureCategories: []BlueprintCategory{
			BlueprintCategoryGlobal,
			BlueprintCategoryWired,
			BlueprintCategoryService,
			// BlueprintCategoryWireless excluded by default
		},
		ScopeTypes: []ScopeType{
			ScopeTypeSiteGroup,
			ScopeTypeDevice,
		},
		TargetTypes: []TargetType{
			TargetTypeSiteGroup,
			TargetTypeDevice,
		},
		DeploymentMethods: []DeploymentMethod{
			DeploymentMethodRolling,
			DeploymentMethodImmediate,
		},
		ScaleFactor:            100,
		PerformanceIterations:  10,
		PerformanceConcurrency: 5,
		OneFilePerFeature:      true,
		IncludeMetadata:        true,
		TestIDPrefix:           "TCXM",
		StartingIDNumber:       1000,
	}
}
