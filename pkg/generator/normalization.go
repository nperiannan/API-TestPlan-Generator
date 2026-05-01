package generator

import (
	"regexp"
	"strings"

	"github.com/extremenetworks/testcase-generator/pkg/model"
)

var pathPlaceholderRE = regexp.MustCompile(`\{([^}]+)\}`)

func (g *Generator) normalizeSuite(suite *model.TestSuite) {
	if suite == nil {
		return
	}
	for featureIndex := range suite.Features {
		for category := range suite.Features[featureIndex].Tests {
			tests := suite.Features[featureIndex].Tests[category]
			for testIndex := range tests {
				for stepIndex := range tests[testIndex].Steps {
					step := &tests[testIndex].Steps[stepIndex]
					normalizeDeepScannedBody(step.Body)
					g.ensureStepPathParams(step)
					ensureCaptureMetadata(step)
				}
			}
			suite.Features[featureIndex].Tests[category] = tests
		}
	}
}

func normalizeDeepScannedBody(body interface{}) {
	bodyMap, ok := body.(map[string]interface{})
	if !ok {
		return
	}
	if _, ok := bodyMap["objects"]; !ok {
		return
	}
	allowed := map[string]bool{
		"featurePath": true,
		"objectType":  true,
		"operation":   true,
		"objects":     true,
		"filters":     true,
		"objectIds":   true,
	}
	for key := range bodyMap {
		if !allowed[key] {
			delete(bodyMap, key)
		}
	}
}

func (g *Generator) ensureStepPathParams(step *model.TestStep) {
	if step == nil || step.Path == "" {
		return
	}
	matches := pathPlaceholderRE.FindAllStringSubmatch(step.Path, -1)
	if len(matches) == 0 {
		return
	}
	if step.PathParams == nil {
		step.PathParams = map[string]string{}
	}
	for _, match := range matches {
		if len(match) < 2 || match[1] == "" {
			continue
		}
		name := match[1]
		if isProfileNamePlaceholder(step.Path, name) {
			step.PathParams[name] = defaultPathParamValue(name, step.Path)
			continue
		}
		if _, exists := step.PathParams[name]; !exists || step.PathParams[name] == "" {
			step.PathParams[name] = defaultPathParamValue(name, step.Path)
		}
	}
}

func ensureCaptureMetadata(step *model.TestStep) {
	if step == nil {
		return
	}
	for i := range step.Validations {
		validation := &step.Validations[i]
		if validation.CaptureAs != "" {
			continue
		}
		if validation.Path == "$[0].id" && strings.Contains(strings.ToLower(validation.Description), "capture") {
			validation.CaptureAs = "OBJECT_ID"
		}
	}
}

func isProfileNamePlaceholder(path string, name string) bool {
	if name != "name" && name != "profileName" && name != "profile_name" {
		return false
	}
	return strings.Contains(path, "/configuration-profile/{"+name+"}") ||
		strings.Contains(path, "/service-profile/{"+name+"}") ||
		strings.Contains(path, "/global-profile/{"+name+"}") ||
		strings.Contains(path, "/blueprint/{"+name+"}")
}

func defaultPathParamValue(name string, path string) string {
	switch name {
	case "name", "profileName", "profile_name":
		return "TestProfile"
	case "siteName", "siteId", "siteGroupId":
		return "test-site-001"
	case "hostName", "deviceId":
		return "test-device-001"
	case "serialNumber":
		return "test-serial-001"
	case "vlan_id", "vlanId":
		return "1"
	case "secure_profile_name":
		return "test-secure-profile"
	case "vr_name":
		return "vr-default"
	case "port":
		return "1/1"
	case "iftype":
		return "ethernet"
	case "stp_name":
		return "mstp"
	case "acl_name":
		return "test-acl"
	case "objectGroupId":
		return "550e8400-e29b-41d4-a716-446655440000"
	}
	lower := strings.ToLower(name)
	if strings.Contains(lower, "id") {
		return "550e8400-e29b-41d4-a716-446655440000"
	}
	if strings.Contains(lower, "name") {
		return "test-" + strings.ReplaceAll(lower, "_", "-")
	}
	return "test-" + strings.ReplaceAll(lower, "_", "-")
}
