package yang

import "testing"

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
