package yang

import (
	"testing"

	"github.com/extremenetworks/testcase-generator/pkg/model"
)

func TestParseDefaultValueSupportsUnquotedYANGDefaults(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		goType   string
		expected interface{}
	}{
		{name: "bool true", line: "default true;", goType: "bool", expected: true},
		{name: "bool false", line: "default false;", goType: "bool", expected: false},
		{name: "uint8", line: "default 8;", goType: "uint8", expected: 8},
		{name: "quoted enum", line: "default \"AnyCast Gateway\";", goType: "string", expected: "AnyCast Gateway"},
		{name: "quoted empty string", line: "default \"\";", goType: "string", expected: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseDefaultValue(tt.line, tt.goType); got != tt.expected {
				t.Fatalf("expected %#v, got %#v", tt.expected, got)
			}
		})
	}
}

func TestParseRangeWithPresencePreservesZeroMinimum(t *testing.T) {
	min, max, hasMin, hasMax := parseRangeWithPresence("0..255")
	if !hasMin || min != 0 {
		t.Fatalf("expected explicit zero minimum, got min=%d hasMin=%v", min, hasMin)
	}
	if !hasMax || max != 255 {
		t.Fatalf("expected max 255, got max=%d hasMax=%v", max, hasMax)
	}
}

func TestParseLeafListCapturesConstraintsAndDescription(t *testing.T) {
	parser := NewParser("")
	scanner := newSliceScanner([]string{
		`type string {`,
		`length "0..64";`,
		`pattern "[a-z]+";`,
		`}`,
		`description "Allowed names.";`,
		`min-elements 1;`,
		`}`,
	})

	param := parser.parseLeafList(scanner, `leaf-list allowed-name {`)
	if !param.IsArray || param.GoType != "[]string" {
		t.Fatalf("expected string array leaf-list, got %#v", param)
	}
	if param.Description != "Allowed names." {
		t.Fatalf("expected description, got %q", param.Description)
	}
	if param.ArrayMinItems != 1 {
		t.Fatalf("expected min-elements 1, got %d", param.ArrayMinItems)
	}
	assertConstraint(t, param.Constraints, model.ConstraintTypeMinLength, 0)
	assertConstraint(t, param.Constraints, model.ConstraintTypeMaxLength, 64)
	assertConstraint(t, param.Constraints, model.ConstraintTypePattern, "[a-z]+")
}

func TestParseNestedListCapturesKeysAndMarksKeyLeavesRequired(t *testing.T) {
	parser := NewParser("")
	scanner := newSliceScanner([]string{
		`key "vr-name route-id";`,
		`leaf vr-name {`,
		`type string;`,
		`}`,
		`leaf route-id {`,
		`type string;`,
		`}`,
		`leaf description {`,
		`type string;`,
		`}`,
		`}`,
	})

	param := parser.parseNestedList(scanner, `list route {`)
	if len(param.Keys) != 2 || param.Keys[0] != "vr-name" || param.Keys[1] != "route-id" {
		t.Fatalf("expected nested list keys, got %#v", param.Keys)
	}
	if len(param.NestedProperties) != 3 {
		t.Fatalf("expected three nested properties, got %#v", param.NestedProperties)
	}
	for _, nested := range param.NestedProperties {
		if nested.Name == "vr-name" || nested.Name == "route-id" {
			if !nested.Required {
				t.Fatalf("expected key leaf %s to be required", nested.Name)
			}
		}
		if nested.Name == "description" && nested.Required {
			t.Fatalf("non-key leaf should not be required")
		}
	}
}

func TestParseFeaturesSkipsExtensionDescriptionExamplesAndKeepsGroupingLists(t *testing.T) {
	parser := NewParser("")
	lines := []string{
		`module extreme-intent-test {`,
		`extension example-extension {`,
		`description`,
		`"Example only:`,
		`list snmp-global-config {`,
		`key \"sys-name\";`,
		`leaf sys-name {`,
		`type string;`,
		`}`,
		`}`,
		`";`,
		`}`,
		`grouping grp-real-config {`,
		`list real-config {`,
		`key "id";`,
		`leaf id {`,
		`type string;`,
		`mandatory true;`,
		`}`,
		`leaf enabled {`,
		`type boolean;`,
		`}`,
		`}`,
		`}`,
		`uses grp-real-config;`,
		`}`,
	}

	if err := parser.collectTypesAndGroupings("test.yang", lines); err != nil {
		t.Fatalf("collectTypesAndGroupings failed: %v", err)
	}
	if err := parser.parseFeaturesFromLines("test.yang", lines); err != nil {
		t.Fatalf("parseFeaturesFromLines failed: %v", err)
	}

	if _, exists := parser.features["snmp-global-config"]; exists {
		t.Fatalf("example YANG inside extension description must not be parsed as a feature")
	}
	realFeature := parser.features["real-config"]
	if realFeature == nil {
		t.Fatalf("expected real list feature from grouping to be parsed")
	}
	if len(realFeature.Keys) != 1 || realFeature.Keys[0] != "id" {
		t.Fatalf("expected real feature key id, got %#v", realFeature.Keys)
	}
	if len(realFeature.Parameters) != 2 {
		t.Fatalf("expected real feature parameters, got %#v", realFeature.Parameters)
	}
	if !realFeature.Parameters[0].Required {
		t.Fatalf("expected mandatory key parameter to remain required")
	}
}

func assertConstraint(t *testing.T, constraints []model.Constraint, constraintType model.ConstraintType, expected interface{}) {
	t.Helper()
	for _, constraint := range constraints {
		if constraint.Type == constraintType && constraint.Value == expected {
			return
		}
	}
	t.Fatalf("expected constraint %s=%#v in %#v", constraintType, expected, constraints)
}
