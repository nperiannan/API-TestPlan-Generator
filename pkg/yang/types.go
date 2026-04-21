package yang

import "github.com/extremenetworks/testcase-generator/pkg/model"

// Typedef represents a YANG typedef definition
type Typedef struct {
	Name        string
	BaseType    string
	EnumValues  []string
	Constraints []model.Constraint
	Description string
}

// Parameter represents a YANG parameter (temporary type for parsing)
type Parameter struct {
	Name             string
	YangType         string
	GoType           string
	Description      string
	Required         bool
	DefaultValue     interface{}
	Constraints      []model.Constraint
	IsArray          bool
	ArrayMinItems    int
	ArrayMaxItems    int
	NestedProperties []Parameter
}

// ToModelParameter converts YANG Parameter to model.Parameter
func (p Parameter) ToModelParameter() model.Parameter {
	nestedModelParams := make([]model.Parameter, 0, len(p.NestedProperties))
	for _, nested := range p.NestedProperties {
		nestedModelParams = append(nestedModelParams, nested.ToModelParameter())
	}

	return model.Parameter{
		Name:             p.Name,
		YangType:         p.YangType,
		GoType:           p.GoType,
		Description:      p.Description,
		Required:         p.Required,
		DefaultValue:     p.DefaultValue,
		Constraints:      p.Constraints,
		IsArray:          p.IsArray,
		ArrayMinItems:    p.ArrayMinItems,
		ArrayMaxItems:    p.ArrayMaxItems,
		NestedProperties: nestedModelParams,
	}
}
