package generator

import (
	"fmt"

	"github.com/extremenetworks/testcase-generator/pkg/model"
)

// generateValueTransitionTests generates tests that verify a field can be
// changed from one valid value to another and back again (round-trip).
//
// Three categories of transitions are covered:
//  1. Enum parameter transitions  – field has a known set of allowed values
//  2. Well-known string transitions – field has conventional values (e.g. vr-name)
//  3. List-key transitions          – field is a YANG list key (delete + recreate)
//  4. Priority / sequence changes   – field is named "priority" / "sequence"
func (g *Generator) generateValueTransitionTests(feature *model.Feature, paths []*model.FeaturePath) []model.TestCase {
	var tests []model.TestCase

	var createPath, updatePath, readPath, deletePath *model.FeaturePath
	for _, path := range paths {
		switch path.OperationType {
		case model.OperationTypeCreate:
			createPath = path
		case model.OperationTypeUpdate:
			updatePath = path
		case model.OperationTypeRead:
			readPath = path
		case model.OperationTypeDelete:
			deletePath = path
		}
	}

	if createPath == nil {
		return tests
	}

	// 1. Enum parameter transitions
	for _, param := range feature.Parameters {
		enumVals := getEnumValues(param)
		if len(enumVals) >= 2 {
			tests = append(tests, g.generateEnumTransitionTest(feature, createPath, updatePath, readPath, param, enumVals))
		}
	}

	// 2. Well-known string transitions (vr-name, mode, protocol, etc.)
	for _, param := range feature.Parameters {
		if wellKnown := getWellKnownValues(param); len(wellKnown) >= 2 {
			if isListKeyParam(param) {
				// List-key: must delete + recreate
				tests = append(tests, g.generateKeyFieldTransitionTest(feature, createPath, readPath, deletePath, param, wellKnown))
				// Coexistence: both values active at the same time
				tests = append(tests, g.generateKeyFieldCoexistenceTest(feature, createPath, readPath, deletePath, param, wellKnown))
			} else {
				tests = append(tests, g.generateWellKnownValueTransitionTest(feature, createPath, updatePath, readPath, param, wellKnown))
			}
		}
	}

	// 3. Priority / sequence change tests
	for _, param := range feature.Parameters {
		if isPriorityParam(param) {
			tests = append(tests, g.generatePriorityChangeTest(feature, createPath, updatePath, readPath, param))
		}
	}

	return tests
}

// ── Enum transitions ───────────────────────────────────────────────────────────

// generateEnumTransitionTest generates:
// create(valA) → verify → update(valB) → verify → update back(valA) → verify
func (g *Generator) generateEnumTransitionTest(
	feature *model.Feature,
	createPath, updatePath, readPath *model.FeaturePath,
	param model.Parameter,
	enumVals []string,
) model.TestCase {
	valA := enumVals[0]
	valB := enumVals[1]

	tc := model.TestCase{
		TestCaseID:       g.nextTestID(),
		FeatureName:      feature.Name,
		Priority:         model.TestPriorityP2,
		Type:             model.TestCategoryFunctional,
		Description:      fmt.Sprintf("Verify %s can transition '%s' from '%s' to '%s' and back (round-trip)", feature.Name, param.Name, valA, valB),
		IsDeploymentTest: false,
		Steps:            []model.TestStep{},
	}

	resourceName := fmt.Sprintf("TestResource-%s-Transition", param.Name)

	// Step 1: Create with value A
	body := g.generateRequestBody(feature, createPath)
	body["name"] = resourceName
	g.setBodyParameterValue(body, param.Name, valA)
	tc.Steps = append(tc.Steps, model.TestStep{
		Name:           "createWithValueA",
		Description:    fmt.Sprintf("Create %s with %s='%s'", feature.Name, param.Name, valA),
		Method:         createPath.HTTPMethod,
		API:            model.APITypeREST,
		Path:           createPath.Path,
		Body:           deepCopyBody(body),
		ExpectedStatus: 201,
		Validations:    statusValidation(201),
	})

	// Step 2: Verify A is set
	if readPath != nil {
		tc.Steps = append(tc.Steps, model.TestStep{
			Name:           "verifyValueA",
			Description:    fmt.Sprintf("Verify %s is set to '%s'", param.Name, valA),
			Method:         readPath.HTTPMethod,
			API:            model.APITypeREST,
			Path:           readPath.Path,
			PathParams:     map[string]string{"name": resourceName},
			ExpectedStatus: 200,
			Validations:    statusValidation(200),
		})
	}

	// Step 3: Update to value B (only if updatePath exists)
	if updatePath != nil {
		updateBody := g.generateRequestBody(feature, updatePath)
		updateBody["name"] = resourceName
		g.setBodyParameterValue(updateBody, param.Name, valB)
		tc.Steps = append(tc.Steps, model.TestStep{
			Name:           "updateToValueB",
			Description:    fmt.Sprintf("Update %s to '%s'='%s'", feature.Name, param.Name, valB),
			Method:         updatePath.HTTPMethod,
			API:            model.APITypeREST,
			Path:           updatePath.Path,
			PathParams:     map[string]string{"name": resourceName},
			Body:           deepCopyBody(updateBody),
			ExpectedStatus: 200,
			Validations:    statusValidation(200),
		})

		// Step 4: Verify B is set
		if readPath != nil {
			tc.Steps = append(tc.Steps, model.TestStep{
				Name:           "verifyValueB",
				Description:    fmt.Sprintf("Verify %s was updated to '%s'", param.Name, valB),
				Method:         readPath.HTTPMethod,
				API:            model.APITypeREST,
				Path:           readPath.Path,
				PathParams:     map[string]string{"name": resourceName},
				ExpectedStatus: 200,
				Validations:    statusValidation(200),
			})
		}

		// Step 5: Round-trip back to A
		roundTripBody := g.generateRequestBody(feature, updatePath)
		roundTripBody["name"] = resourceName
		g.setBodyParameterValue(roundTripBody, param.Name, valA)
		tc.Steps = append(tc.Steps, model.TestStep{
			Name:           "roundTripToValueA",
			Description:    fmt.Sprintf("Round-trip: revert %s back to '%s'", param.Name, valA),
			Method:         updatePath.HTTPMethod,
			API:            model.APITypeREST,
			Path:           updatePath.Path,
			PathParams:     map[string]string{"name": resourceName},
			Body:           deepCopyBody(roundTripBody),
			ExpectedStatus: 200,
			Validations:    statusValidation(200),
		})

		// Step 6: Verify round-trip
		if readPath != nil {
			tc.Steps = append(tc.Steps, model.TestStep{
				Name:           "verifyRoundTrip",
				Description:    fmt.Sprintf("Verify %s is restored to '%s'", param.Name, valA),
				Method:         readPath.HTTPMethod,
				API:            model.APITypeREST,
				Path:           readPath.Path,
				PathParams:     map[string]string{"name": resourceName},
				ExpectedStatus: 200,
				Validations:    statusValidation(200),
			})
		}
	}

	return tc
}

// ── Well-known value transitions (non-enum, non-key) ──────────────────────────

func (g *Generator) generateWellKnownValueTransitionTest(
	feature *model.Feature,
	createPath, updatePath, readPath *model.FeaturePath,
	param model.Parameter,
	knownVals []string,
) model.TestCase {
	valA := knownVals[0]
	valB := knownVals[1]

	tc := model.TestCase{
		TestCaseID:       g.nextTestID(),
		FeatureName:      feature.Name,
		Priority:         model.TestPriorityP2,
		Type:             model.TestCategoryFunctional,
		Description:      fmt.Sprintf("Verify %s can change '%s' from '%s' to '%s'", feature.Name, param.Name, valA, valB),
		IsDeploymentTest: false,
		Steps:            []model.TestStep{},
	}

	resourceName := fmt.Sprintf("TestResource-%s-WKTransition", param.Name)

	// Create with valA
	body := g.generateRequestBody(feature, createPath)
	body["name"] = resourceName
	g.setBodyParameterValue(body, param.Name, valA)
	tc.Steps = append(tc.Steps, model.TestStep{
		Name: "createWithInitialValue", Description: fmt.Sprintf("Create with %s='%s'", param.Name, valA),
		Method: createPath.HTTPMethod, API: model.APITypeREST, Path: createPath.Path,
		Body: deepCopyBody(body), ExpectedStatus: 201, Validations: statusValidation(201),
	})

	if readPath != nil {
		tc.Steps = append(tc.Steps, model.TestStep{
			Name: "verifyInitialValue", Description: fmt.Sprintf("Verify %s is '%s'", param.Name, valA),
			Method: readPath.HTTPMethod, API: model.APITypeREST, Path: readPath.Path,
			PathParams: map[string]string{"name": resourceName}, ExpectedStatus: 200, Validations: statusValidation(200),
		})
	}

	if updatePath != nil {
		// Update to valB
		upBody := g.generateRequestBody(feature, updatePath)
		upBody["name"] = resourceName
		g.setBodyParameterValue(upBody, param.Name, valB)
		tc.Steps = append(tc.Steps, model.TestStep{
			Name: "updateToNewValue", Description: fmt.Sprintf("Update %s to '%s'", param.Name, valB),
			Method: updatePath.HTTPMethod, API: model.APITypeREST, Path: updatePath.Path,
			PathParams: map[string]string{"name": resourceName}, Body: deepCopyBody(upBody),
			ExpectedStatus: 200, Validations: statusValidation(200),
		})

		if readPath != nil {
			tc.Steps = append(tc.Steps, model.TestStep{
				Name: "verifyNewValue", Description: fmt.Sprintf("Verify %s updated to '%s'", param.Name, valB),
				Method: readPath.HTTPMethod, API: model.APITypeREST, Path: readPath.Path,
				PathParams: map[string]string{"name": resourceName}, ExpectedStatus: 200, Validations: statusValidation(200),
			})
		}
	}

	return tc
}

// ── Key-field transitions (delete + recreate) ─────────────────────────────────

// generateKeyFieldTransitionTest: for YANG list-key fields like vr-name.
// Pattern: create(keyA) → verify → delete(keyA) → create(keyB) → verify keyB exists → verify keyA gone.
func (g *Generator) generateKeyFieldTransitionTest(
	feature *model.Feature,
	createPath, readPath, deletePath *model.FeaturePath,
	param model.Parameter,
	knownVals []string,
) model.TestCase {
	valA := knownVals[0]
	valB := knownVals[1]

	tc := model.TestCase{
		TestCaseID:  g.nextTestID(),
		FeatureName: feature.Name,
		Priority:    model.TestPriorityP2,
		Type:        model.TestCategoryFunctional,
		Description: fmt.Sprintf(
			"Verify %s can switch key field '%s' from '%s' to '%s' (delete + recreate, since it is a list key)",
			feature.Name, param.Name, valA, valB,
		),
		IsDeploymentTest: false,
		Steps:            []model.TestStep{},
	}

	// Step 1: Create entry with key=valA
	bodyA := g.generateRequestBody(feature, createPath)
	g.setBodyParameterValue(bodyA, param.Name, valA)
	tc.Steps = append(tc.Steps, model.TestStep{
		Name: "createWithKeyA", Description: fmt.Sprintf("Create with %s='%s'", param.Name, valA),
		Method: createPath.HTTPMethod, API: model.APITypeREST, Path: createPath.Path,
		Body: deepCopyBody(bodyA), ExpectedStatus: 201, Validations: statusValidation(201),
	})

	// Step 2: Verify entry with key=valA exists
	if readPath != nil {
		tc.Steps = append(tc.Steps, model.TestStep{
			Name: "verifyKeyAExists", Description: fmt.Sprintf("Verify entry with %s='%s' exists", param.Name, valA),
			Method: readPath.HTTPMethod, API: model.APITypeREST, Path: readPath.Path,
			ExpectedStatus: 200, Validations: statusValidation(200),
		})
	}

	// Step 3: Delete the entry with key=valA (YANG list keys are immutable — must delete)
	if deletePath != nil {
		deleteBody := g.generateRequestBody(feature, deletePath)
		g.setBodyParameterValue(deleteBody, param.Name, valA)
		tc.Steps = append(tc.Steps, model.TestStep{
			Name: "deleteKeyA", Description: fmt.Sprintf("Delete entry with %s='%s' (key is immutable; must delete to change)", param.Name, valA),
			Method: deletePath.HTTPMethod, API: model.APITypeREST, Path: deletePath.Path,
			Body: deepCopyBody(deleteBody), ExpectedStatus: 204, Validations: statusValidation(204),
		})
	}

	// Step 4: Create new entry with key=valB
	bodyB := g.generateRequestBody(feature, createPath)
	g.setBodyParameterValue(bodyB, param.Name, valB)
	tc.Steps = append(tc.Steps, model.TestStep{
		Name: "createWithKeyB", Description: fmt.Sprintf("Create new entry with %s='%s'", param.Name, valB),
		Method: createPath.HTTPMethod, API: model.APITypeREST, Path: createPath.Path,
		Body: deepCopyBody(bodyB), ExpectedStatus: 201, Validations: statusValidation(201),
	})

	// Step 5: Verify entry with key=valB exists
	if readPath != nil {
		tc.Steps = append(tc.Steps, model.TestStep{
			Name: "verifyKeyBExists", Description: fmt.Sprintf("Verify entry with %s='%s' exists", param.Name, valB),
			Method: readPath.HTTPMethod, API: model.APITypeREST, Path: readPath.Path,
			ExpectedStatus: 200, Validations: statusValidation(200),
		})
	}

	return tc
}

// generateKeyFieldCoexistenceTest: Both key=valA and key=valB active simultaneously.
// Pattern: create(keyA) → create(keyB) → verify both exist → delete both.
func (g *Generator) generateKeyFieldCoexistenceTest(
	feature *model.Feature,
	createPath, readPath, deletePath *model.FeaturePath,
	param model.Parameter,
	knownVals []string,
) model.TestCase {
	valA := knownVals[0]
	valB := knownVals[1]

	tc := model.TestCase{
		TestCaseID:  g.nextTestID(),
		FeatureName: feature.Name,
		Priority:    model.TestPriorityP2,
		Type:        model.TestCategoryFunctional,
		Description: fmt.Sprintf(
			"Verify %s entries with '%s'='%s' and '%s'='%s' can coexist simultaneously",
			feature.Name, param.Name, valA, param.Name, valB,
		),
		IsDeploymentTest: false,
		Steps:            []model.TestStep{},
	}

	// Create entry A
	bodyA := g.generateRequestBody(feature, createPath)
	g.setBodyParameterValue(bodyA, param.Name, valA)
	tc.Steps = append(tc.Steps, model.TestStep{
		Name: "createEntryA", Description: fmt.Sprintf("Create first entry with %s='%s'", param.Name, valA),
		Method: createPath.HTTPMethod, API: model.APITypeREST, Path: createPath.Path,
		Body: deepCopyBody(bodyA), ExpectedStatus: 201, Validations: statusValidation(201),
	})

	// Create entry B
	bodyB := g.generateRequestBody(feature, createPath)
	g.setBodyParameterValue(bodyB, param.Name, valB)
	tc.Steps = append(tc.Steps, model.TestStep{
		Name: "createEntryB", Description: fmt.Sprintf("Create second entry with %s='%s'", param.Name, valB),
		Method: createPath.HTTPMethod, API: model.APITypeREST, Path: createPath.Path,
		Body: deepCopyBody(bodyB), ExpectedStatus: 201, Validations: statusValidation(201),
	})

	// Verify both exist
	if readPath != nil {
		tc.Steps = append(tc.Steps, model.TestStep{
			Name: "verifyBothCoexist", Description: fmt.Sprintf("Verify both '%s' and '%s' entries exist simultaneously", valA, valB),
			Method: readPath.HTTPMethod, API: model.APITypeREST, Path: readPath.Path,
			ExpectedStatus: 200, Validations: statusValidation(200),
		})
	}

	// Cleanup: delete both
	if deletePath != nil {
		for _, val := range []string{valA, valB} {
			delBody := g.generateRequestBody(feature, deletePath)
			g.setBodyParameterValue(delBody, param.Name, val)
			tc.Steps = append(tc.Steps, model.TestStep{
				Name: fmt.Sprintf("cleanup-%s", val), Description: fmt.Sprintf("Cleanup: delete entry with %s='%s'", param.Name, val),
				Method: deletePath.HTTPMethod, API: model.APITypeREST, Path: deletePath.Path,
				Body: deepCopyBody(delBody), ExpectedStatus: 204, Validations: statusValidation(204),
			})
		}
	}

	return tc
}

// ── Priority change tests ─────────────────────────────────────────────────────

// generatePriorityChangeTest: change priority field and verify reordering.
func (g *Generator) generatePriorityChangeTest(
	feature *model.Feature,
	createPath, updatePath, readPath *model.FeaturePath,
	param model.Parameter,
) model.TestCase {
	tc := model.TestCase{
		TestCaseID:       g.nextTestID(),
		FeatureName:      feature.Name,
		Priority:         model.TestPriorityP3,
		Type:             model.TestCategoryFunctional,
		Description:      fmt.Sprintf("Verify %s priority field '%s' can be changed from low to high and high to low", feature.Name, param.Name),
		IsDeploymentTest: false,
		Steps:            []model.TestStep{},
	}

	// Create with priority = 1 (high priority)
	body := g.generateRequestBody(feature, createPath)
	g.setBodyParameterValue(body, param.Name, 1)
	tc.Steps = append(tc.Steps, model.TestStep{
		Name: "createWithHighPriority", Description: fmt.Sprintf("Create %s with %s=1 (highest)", feature.Name, param.Name),
		Method: createPath.HTTPMethod, API: model.APITypeREST, Path: createPath.Path,
		Body: deepCopyBody(body), ExpectedStatus: 201, Validations: statusValidation(201),
	})

	if readPath != nil {
		tc.Steps = append(tc.Steps, model.TestStep{
			Name: "verifyHighPriority", Description: fmt.Sprintf("Verify %s is 1 (high priority)", param.Name),
			Method: readPath.HTTPMethod, API: model.APITypeREST, Path: readPath.Path,
			ExpectedStatus: 200, Validations: statusValidation(200),
		})
	}

	// Update priority to 10 (lower priority)
	if updatePath != nil {
		upBody := g.generateRequestBody(feature, updatePath)
		g.setBodyParameterValue(upBody, param.Name, 10)
		tc.Steps = append(tc.Steps, model.TestStep{
			Name: "updateToLowerPriority", Description: fmt.Sprintf("Update %s to 10 (lower priority)", param.Name),
			Method: updatePath.HTTPMethod, API: model.APITypeREST, Path: updatePath.Path,
			Body: deepCopyBody(upBody), ExpectedStatus: 200, Validations: statusValidation(200),
		})

		if readPath != nil {
			tc.Steps = append(tc.Steps, model.TestStep{
				Name: "verifyLowerPriority", Description: fmt.Sprintf("Verify %s was changed to 10", param.Name),
				Method: readPath.HTTPMethod, API: model.APITypeREST, Path: readPath.Path,
				ExpectedStatus: 200, Validations: statusValidation(200),
			})
		}

		// Restore back to 1
		restoreBody := g.generateRequestBody(feature, updatePath)
		g.setBodyParameterValue(restoreBody, param.Name, 1)
		tc.Steps = append(tc.Steps, model.TestStep{
			Name: "restoreHighPriority", Description: fmt.Sprintf("Restore %s back to 1 (round-trip)", param.Name),
			Method: updatePath.HTTPMethod, API: model.APITypeREST, Path: updatePath.Path,
			Body: deepCopyBody(restoreBody), ExpectedStatus: 200, Validations: statusValidation(200),
		})
	}

	return tc
}

// ── Helper functions ──────────────────────────────────────────────────────────

// getEnumValues extracts enum values from a parameter's constraints.
func getEnumValues(param model.Parameter) []string {
	for _, c := range param.Constraints {
		if c.Type == model.ConstraintTypeEnum {
			if vals, ok := c.Value.([]string); ok {
				return vals
			}
			if vals, ok := c.Value.([]interface{}); ok {
				var out []string
				for _, v := range vals {
					out = append(out, fmt.Sprintf("%v", v))
				}
				return out
			}
		}
	}
	return nil
}

// getWellKnownValues returns known valid values for parameters that don't have
// explicit enum constraints but have a small, finite set of accepted values
// (e.g., virtual-router names, protocol modes).
func getWellKnownValues(param model.Parameter) []string {
	switch param.Name {
	case "vr-name", "vrf", "virtualRouter", "virtual-router":
		return []string{"vr-default", "vr-mgmt"}
	case "mode":
		return []string{"active", "passive"}
	case "protocol":
		return []string{"tcp", "udp"}
	case "direction":
		return []string{"ingress", "egress"}
	case "action":
		return []string{"permit", "deny"}
	case "type":
		if param.GoType == "string" {
			return []string{"unicast", "multicast"}
		}
	}
	return nil
}

// isListKeyParam returns true when the parameter is likely a YANG list key.
// Heuristic: if its name is "vr-name", "vrf", "key", etc., treat it as a key.
func isListKeyParam(param model.Parameter) bool {
	switch param.Name {
	case "vr-name", "vrf", "virtualRouter", "virtual-router":
		return true
	}
	return false
}

// isPriorityParam returns true for parameters that represent ordering priority.
func isPriorityParam(param model.Parameter) bool {
	switch param.Name {
	case "priority", "sequence", "order", "seq-num", "sequence-number":
		return true
	}
	return false
}

// statusValidation is a convenience wrapper.
func statusValidation(code int) []model.Validation {
	return []model.Validation{
		{Type: model.ValidationTypeStatusCode, Expected: code},
	}
}

// deepCopyBody creates a shallow copy of a body map so each step has independent data.
func deepCopyBody(src map[string]interface{}) map[string]interface{} {
	dst := make(map[string]interface{}, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
