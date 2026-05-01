package yang

import "github.com/extremenetworks/testcase-generator/pkg/model"

// lineScanner is a minimal interface satisfied by both *bufio.Scanner and *sliceScanner,
// enabling two-pass parsing without re-opening files.
type lineScanner interface {
	Scan() bool
	Text() string
}

// sliceScanner implements lineScanner over an in-memory slice of lines.
type sliceScanner struct {
	lines []string
	idx   int
}

func newSliceScanner(lines []string) *sliceScanner {
	return &sliceScanner{lines: lines, idx: -1}
}

func (s *sliceScanner) Scan() bool {
	s.idx++
	return s.idx < len(s.lines)
}

func (s *sliceScanner) Text() string {
	if s.idx < 0 || s.idx >= len(s.lines) {
		return ""
	}
	return s.lines[s.idx]
}

// Grouping represents a YANG grouping definition, including its direct parameters
// and any nested `uses` references that need recursive expansion.
type Grouping struct {
	Name     string
	Params   []Parameter // leaves/containers/lists directly inside the grouping
	UsesRefs []string    // names of other groupings referenced via `uses`
}

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
	Keys             []string
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
		Keys:             p.Keys,
		NestedProperties: nestedModelParams,
	}
}
