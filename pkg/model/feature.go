package model

// Feature represents a feature object derived from YANG models
type Feature struct {
	Name           string
	YangPath       string
	Description    string
	Parameters     []Parameter
	Constraints    []Constraint
	ProfileType    ProfileType
	IsListType     bool
	Keys           []string                  // YANG list key field names (e.g., ["server", "vr-name"])
	Parent         string                    // parent feature name if nested
	SubObjectTypes map[string]*SubObjectType // Sub-object types (e.g., dns-suffix under dns-server)
}

// SubObjectType represents a sub-object variation of a feature
type SubObjectType struct {
	Name        string      // Object type name (e.g., "dns-suffix")
	Parameters  []Parameter // Properties specific to this sub-object type
	Description string
}

// Parameter represents a parameter/field in a feature
type Parameter struct {
	Name             string
	YangType         string
	GoType           string
	Description      string
	Required         bool
	DefaultValue     interface{}
	Constraints      []Constraint
	IsArray          bool
	ArrayMinItems    int
	ArrayMaxItems    int
	Keys             []string    // YANG keys when this parameter represents a nested list
	NestedProperties []Parameter // For container/list types that have nested parameters
}

// Constraint represents a validation constraint on a parameter
type Constraint struct {
	Type        ConstraintType
	Value       interface{}
	Description string
}

// ConstraintType defines the type of constraint
type ConstraintType string

const (
	ConstraintTypeMinLength ConstraintType = "minLength"
	ConstraintTypeMaxLength ConstraintType = "maxLength"
	ConstraintTypeMin       ConstraintType = "min"
	ConstraintTypeMax       ConstraintType = "max"
	ConstraintTypePattern   ConstraintType = "pattern"
	ConstraintTypeEnum      ConstraintType = "enum"
	ConstraintTypeRequired  ConstraintType = "required"
	ConstraintTypeMinItems  ConstraintType = "minItems"
	ConstraintTypeMaxItems  ConstraintType = "maxItems"
	ConstraintTypeUnique    ConstraintType = "unique"
)

// FeaturePath represents an API operation on a feature
type FeaturePath struct {
	Feature           *Feature
	FeatureName       string // Name of the YANG feature this path corresponds to (for deep-scanned features)
	HTTPMethod        string
	Path              string
	PathParams        []PathParameter
	OperationType     OperationType
	RequestSchema     interface{}
	ResponseSchema    interface{}
	ProfileType       ProfileType
	BlueprintCategory BlueprintCategory // Category for deep-scanned features (global-profile, wired-blueprint, wireless-blueprint, service-profile)

	// Deployment-related metadata
	SupportsScope              bool
	SupportedScopeTypes        []ScopeType
	SupportsTarget             bool
	SupportedTargetTypes       []TargetType
	SupportsDeployment         bool
	SupportedDeploymentMethods []DeploymentMethod
}

// PathParameter represents a path parameter in a REST endpoint
type PathParameter struct {
	Name        string
	Type        string
	Description string
	Required    bool
	FixedValue  string // For deep-scanned features, the actual value to use (e.g., "/DNS Server")
}

// OperationType represents the type of operation
type OperationType string

const (
	OperationTypeCreate OperationType = "create"
	OperationTypeRead   OperationType = "read"
	OperationTypeUpdate OperationType = "update"
	OperationTypeDelete OperationType = "delete"
	OperationTypeList   OperationType = "list"
	OperationTypeScope  OperationType = "scope"
	OperationTypeTarget OperationType = "target"
	OperationTypeDeploy OperationType = "deploy"
	OperationTypeStatus OperationType = "status"
)

// NOSAPIEndpoint represents a NOSAPI endpoint for device verification
type NOSAPIEndpoint struct {
	Path           string
	Method         string
	Description    string
	DeviceScope    string // "siteGroup", "device", "global"
	ResponseSchema interface{}
}
