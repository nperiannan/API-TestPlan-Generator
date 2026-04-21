package model

// ProfileType represents the type of configuration profile
type ProfileType string

const (
	ProfileTypeGlobal        ProfileType = "global"
	ProfileTypeConfiguration ProfileType = "configuration"
	ProfileTypeService       ProfileType = "service"
)

// BlueprintCategory represents the blueprint category for deep-scanned features
type BlueprintCategory string

const (
	BlueprintCategoryGlobal   BlueprintCategory = "global-profile"
	BlueprintCategoryWired    BlueprintCategory = "wired-blueprint"
	BlueprintCategoryWireless BlueprintCategory = "wireless-blueprint"
	BlueprintCategoryService  BlueprintCategory = "service-profile"
)

// ScopeType represents the scoping strategy for profiles
type ScopeType string

const (
	ScopeTypeSite      ScopeType = "site"
	ScopeTypeSiteGroup ScopeType = "site-group"
	ScopeTypeDevice    ScopeType = "device"
)

// TargetType represents the targeting strategy for profiles
type TargetType string

const (
	TargetTypeSite      TargetType = "site"
	TargetTypeSiteGroup TargetType = "site-group"
	TargetTypeDevice    TargetType = "device"
)

// DeploymentMethod represents how configuration is deployed
type DeploymentMethod string

const (
	DeploymentMethodRolling   DeploymentMethod = "rolling"
	DeploymentMethodImmediate DeploymentMethod = "immediate"
	DeploymentMethodStaged    DeploymentMethod = "staged"
)

// TestCategory represents the category/type of test
type TestCategory string

const (
	TestCategoryFunctional  TestCategory = "functional"
	TestCategoryBoundary    TestCategory = "boundary"
	TestCategoryNegative    TestCategory = "negative"
	TestCategoryScale       TestCategory = "scale"
	TestCategoryPerformance TestCategory = "performance"
)

// TestPriority represents test priority levels
type TestPriority string

const (
	TestPriorityP0 TestPriority = "P0"
	TestPriorityP1 TestPriority = "P1"
	TestPriorityP2 TestPriority = "P2"
	TestPriorityP3 TestPriority = "P3"
)

// APIType represents which API surface is being tested
type APIType string

const (
	APITypeREST   APIType = "REST"
	APITypeNOSAPI APIType = "NOSAPI"
)

// ValidationType represents different validation strategies
type ValidationType string

const (
	ValidationTypeStatusCode               ValidationType = "statusCode"
	ValidationTypeJSONPathEquals           ValidationType = "jsonPathEquals"
	ValidationTypeJSONPathExists           ValidationType = "jsonPathExists"
	ValidationTypeNosConfigMatchesExpected ValidationType = "nosConfigMatchesExpected"
	ValidationTypeResponseTimeUnder        ValidationType = "responseTimeUnder"
)

// TestSuite is the top-level structure containing all generated tests
type TestSuite struct {
	Version       string             `yaml:"version"`
	GeneratedAt   string             `yaml:"generatedAt"`
	SourceYangDir string             `yaml:"sourceYangDir"`
	SourceRESTAPI string             `yaml:"sourceRESTAPI"`
	SourceNOSAPI  string             `yaml:"sourceNOSAPI"`
	Features      []FeatureTestGroup `yaml:"features"`
}

// FeatureTestGroup groups all tests for a single feature path
type FeatureTestGroup struct {
	FeatureName string                      `yaml:"featureName"`
	FeaturePath string                      `yaml:"featurePath"`
	ProfileType ProfileType                 `yaml:"profileType"`
	Description string                      `yaml:"description"`
	Tests       map[TestCategory][]TestCase `yaml:"tests"`
}

// TestCase represents a single test case with metadata and steps
type TestCase struct {
	TestCaseID       string                 `yaml:"testCaseID"`
	FeatureName      string                 `yaml:"featureName"`
	Priority         TestPriority           `yaml:"priority"`
	Type             TestCategory           `yaml:"type"`
	Description      string                 `yaml:"description"`
	ScopeType        ScopeType              `yaml:"scopeType,omitempty"`
	TargetType       TargetType             `yaml:"targetType,omitempty"`
	DeploymentMethod DeploymentMethod       `yaml:"deploymentMethod,omitempty"`
	IsDeploymentTest bool                   `yaml:"isDeploymentTest"`
	Steps            []TestStep             `yaml:"steps"`
	Metadata         map[string]interface{} `yaml:"metadata,omitempty"`
}

// TestStep represents a single step in a test case
type TestStep struct {
	Name           string            `yaml:"name"`
	Description    string            `yaml:"description,omitempty"`
	Method         string            `yaml:"method"`
	API            APIType           `yaml:"api"`
	Path           string            `yaml:"path"`
	PathParams     map[string]string `yaml:"pathParams,omitempty"`
	QueryParams    map[string]string `yaml:"queryParams,omitempty"`
	Headers        map[string]string `yaml:"headers,omitempty"`
	Body           interface{}       `yaml:"body,omitempty"`
	ExpectedStatus int               `yaml:"expectedStatus"`
	Validations    []Validation      `yaml:"validations,omitempty"`
	Timeout        int               `yaml:"timeout,omitempty"`      // in seconds
	DevicesScope   string            `yaml:"devicesScope,omitempty"` // for NOSAPI: "siteGroup", "device"
}

// Validation represents a validation check on a response
type Validation struct {
	Type        ValidationType `yaml:"type"`
	Path        string         `yaml:"path,omitempty"` // JSONPath for response validation
	Expected    interface{}    `yaml:"expected,omitempty"`
	Description string         `yaml:"description,omitempty"`
}
