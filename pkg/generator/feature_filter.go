package generator

import (
	"strings"

	"github.com/extremenetworks/testcase-generator/pkg/model"
)

func (g *Generator) featureWithoutDuplicatedChildParams(feature *model.Feature, paths []*model.FeaturePath) *model.Feature {
	if feature == nil || len(feature.Parameters) == 0 || g == nil || len(g.features) == 0 || len(g.featurePaths) == 0 {
		return feature
	}

	featurePathValues := fixedFeaturePathValues(paths)
	if len(featurePathValues) == 0 {
		return feature
	}

	childParamNames := make(map[string]bool)
	for childName, childFeature := range g.features {
		if childFeature == nil || childName == feature.Name || !isSeparateChildFeature(feature.Name, childName) {
			continue
		}
		if !g.featureHasAnyPathValue(childName, featurePathValues) {
			continue
		}
		for _, param := range collectAllParameters(childFeature.Parameters) {
			childParamNames[param.Name] = true
		}
	}

	if len(childParamNames) == 0 {
		return feature
	}

	// Build a set of the parent's own YANG key fields — these are identity
	// fields that MUST remain even if a child feature reuses the same name.
	parentKeys := make(map[string]bool, len(feature.Keys))
	for _, k := range feature.Keys {
		parentKeys[k] = true
	}
	// Also keep required fields — the parent's required params are its own.
	parentRequired := make(map[string]bool)
	for _, p := range feature.Parameters {
		if p.Required {
			parentRequired[p.Name] = true
		}
	}

	// Only prune parameters that belong exclusively to children — skip the
	// parent's own key and required fields.
	prunable := make(map[string]bool, len(childParamNames))
	for name := range childParamNames {
		if parentKeys[name] || parentRequired[name] {
			continue
		}
		prunable[name] = true
	}

	if len(prunable) == 0 {
		return feature
	}

	filteredParams, changed := filterParametersByName(feature.Parameters, prunable)
	if !changed {
		return feature
	}

	clone := *feature
	clone.Parameters = filteredParams
	return &clone
}

func fixedFeaturePathValues(paths []*model.FeaturePath) map[string]bool {
	values := make(map[string]bool)
	for _, path := range paths {
		if value := fixedFeaturePathValue(path); value != "" {
			values[value] = true
		}
	}
	return values
}

func fixedFeaturePathValue(path *model.FeaturePath) string {
	if path == nil {
		return ""
	}
	for _, param := range path.PathParams {
		if param.Name == "featurePath" && param.FixedValue != "" {
			return param.FixedValue
		}
	}
	return ""
}

func (g *Generator) featureHasAnyPathValue(featureName string, pathValues map[string]bool) bool {
	for _, path := range g.featurePaths {
		if path == nil || featurePathFeatureName(path) != featureName {
			continue
		}
		if pathValues[fixedFeaturePathValue(path)] {
			return true
		}
	}
	return false
}

func featurePathFeatureName(path *model.FeaturePath) string {
	if path == nil {
		return ""
	}
	if path.FeatureName != "" {
		return path.FeatureName
	}
	if path.Feature != nil {
		return path.Feature.Name
	}
	return ""
}

func isSeparateChildFeature(parentName string, childName string) bool {
	if parentName == "" || childName == "" || parentName == childName {
		return false
	}
	return strings.HasPrefix(childName, parentName+"-") || strings.HasSuffix(childName, "-"+parentName)
}

func filterParametersByName(params []model.Parameter, excluded map[string]bool) ([]model.Parameter, bool) {
	filtered := make([]model.Parameter, 0, len(params))
	changed := false
	for _, param := range params {
		if excluded[param.Name] {
			changed = true
			continue
		}
		if len(param.NestedProperties) > 0 {
			var nestedChanged bool
			param.NestedProperties, nestedChanged = filterParametersByName(param.NestedProperties, excluded)
			changed = changed || nestedChanged
		}
		filtered = append(filtered, param)
	}
	return filtered, changed
}
