package model

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// MarshalYAML splits the structured Validations slice into two output
// fields for readability of the generated test plan:
//
//   - validations: a plain-text list of human-readable expectations
//     (the Description of each Validation), suitable for a manual or
//     planning audience.
//   - assertions:  the structured entries (type / path / expected) used
//     by automation harnesses to assert against the response.
//
// A validation that has only a Description (no Type) is emitted only
// under `validations`. A validation that has a Type but no Description
// is emitted only under `assertions` (with a generated description).
func (s TestStep) MarshalYAML() (interface{}, error) {
	type stepAlias struct {
		Name           string            `yaml:"name"`
		Description    string            `yaml:"description,omitempty"`
		Method         string            `yaml:"method,omitempty"`
		API            APIType           `yaml:"api,omitempty"`
		Path           string            `yaml:"path,omitempty"`
		PathParams     map[string]string `yaml:"pathParams,omitempty"`
		QueryParams    map[string]string `yaml:"queryParams,omitempty"`
		Headers        map[string]string `yaml:"headers,omitempty"`
		Body           interface{}       `yaml:"body,omitempty"`
		ExpectedStatus int               `yaml:"expectedStatus,omitempty"`
		Validations    []string          `yaml:"validations,omitempty"`
		Assertions     []Validation      `yaml:"assertions,omitempty"`
		Timeout        int               `yaml:"timeout,omitempty"`
		DevicesScope   string            `yaml:"devicesScope,omitempty"`
	}

	out := stepAlias{
		Name:           s.Name,
		Description:    s.Description,
		Method:         s.Method,
		API:            s.API,
		Path:           s.Path,
		PathParams:     s.PathParams,
		QueryParams:    s.QueryParams,
		Headers:        s.Headers,
		Body:           s.Body,
		ExpectedStatus: s.ExpectedStatus,
		Timeout:        s.Timeout,
		DevicesScope:   s.DevicesScope,
	}

	for _, v := range s.Validations {
		desc := v.Description
		if desc == "" {
			desc = describeValidation(v)
		}
		if desc != "" {
			out.Validations = append(out.Validations, desc)
		}
		if v.Type != "" {
			out.Assertions = append(out.Assertions, v)
		}
	}

	node := &yaml.Node{}
	if err := node.Encode(out); err != nil {
		return nil, err
	}
	return node, nil
}

// describeValidation produces a fallback human-readable sentence for a
// structured Validation that did not carry a Description.
func describeValidation(v Validation) string {
	if v.CaptureAs != "" && v.Path != "" {
		return fmt.Sprintf("Capture response value '%s' as %s", v.Path, v.CaptureAs)
	}
	switch v.Type {
	case ValidationTypeStatusCode:
		if v.Expected != nil {
			return fmt.Sprintf("Verify HTTP status code is %v", v.Expected)
		}
		return "Verify HTTP status code matches expected value"
	case ValidationTypeJSONPathExists:
		return fmt.Sprintf("Verify response contains '%s'", v.Path)
	case ValidationTypeJSONPathEquals:
		return fmt.Sprintf("Verify '%s' equals %v", v.Path, v.Expected)
	case ValidationTypeNosConfigMatchesExpected:
		return "Verify NOS device configuration matches expected"
	case ValidationTypeResponseTimeUnder:
		return fmt.Sprintf("Verify response time is under %v", v.Expected)
	default:
		if v.Path != "" {
			return fmt.Sprintf("Verify %s on '%s'", v.Type, v.Path)
		}
		return fmt.Sprintf("Verify %s", v.Type)
	}
}
