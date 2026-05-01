package generator

import (
	"fmt"

	"github.com/extremenetworks/testcase-generator/pkg/model"
)

const (
	deploymentBoundarySampleLimit = 2
	deploymentScaleSampleLimit    = 5
)

type deploymentOperationPaths struct {
	create *model.FeaturePath
	read   *model.FeaturePath
	update *model.FeaturePath
	delete *model.FeaturePath
	list   *model.FeaturePath
}

type deploymentBoundaryCandidate struct {
	param      model.Parameter
	constraint model.Constraint
	value      interface{}
}

func (g *Generator) addRepresentativeDeploymentSamples(group *model.FeatureTestGroup, feature *model.Feature, paths []*model.FeaturePath) {
	if group == nil || feature == nil {
		return
	}
	ops := selectDeploymentOperationPaths(paths)
	if !isRepresentativeDeploymentPath(ops.create) || len(feature.Parameters) == 0 {
		return
	}

	if g.includesCategory(model.TestCategoryFunctional) && ops.read != nil && ops.update != nil && ops.delete != nil {
		group.Tests[model.TestCategoryFunctional] = append(group.Tests[model.TestCategoryFunctional],
			g.generateDeploymentCRUDLifecycleTest(feature, ops.create, ops.read, ops.update, ops.delete))
	}

	if g.includesCategory(model.TestCategoryBoundary) {
		group.Tests[model.TestCategoryBoundary] = append(group.Tests[model.TestCategoryBoundary],
			g.generateDeploymentBoundarySampleTests(feature, ops.create, ops.read)...)
	}

	if g.includesCategory(model.TestCategoryScale) {
		group.Tests[model.TestCategoryScale] = append(group.Tests[model.TestCategoryScale],
			g.generateDeploymentScaleSampleTest(feature, ops.create, ops.list))
	}
}

func (g *Generator) includesCategory(category model.TestCategory) bool {
	for _, configured := range g.config.IncludeCategories {
		if configured == category {
			return true
		}
	}
	return false
}

func selectDeploymentOperationPaths(paths []*model.FeaturePath) deploymentOperationPaths {
	var ops deploymentOperationPaths
	for _, path := range paths {
		switch path.OperationType {
		case model.OperationTypeCreate:
			ops.create = preferRepresentativePath(ops.create, path)
		case model.OperationTypeRead:
			ops.read = preferRepresentativePath(ops.read, path)
		case model.OperationTypeUpdate:
			ops.update = preferRepresentativePath(ops.update, path)
		case model.OperationTypeDelete:
			ops.delete = preferRepresentativePath(ops.delete, path)
		case model.OperationTypeList:
			ops.list = preferRepresentativePath(ops.list, path)
		}
	}
	return ops
}

func preferRepresentativePath(current, candidate *model.FeaturePath) *model.FeaturePath {
	if candidate == nil {
		return current
	}
	if current == nil {
		return candidate
	}
	currentExplicit := current.BlueprintCategory != ""
	candidateExplicit := candidate.BlueprintCategory != ""
	if candidateExplicit != currentExplicit {
		if candidateExplicit {
			return candidate
		}
		return current
	}
	currentObject := isFeatureObjectPath(current)
	candidateObject := isFeatureObjectPath(candidate)
	if candidateObject != currentObject {
		if candidateObject {
			return candidate
		}
		return current
	}
	if candidate.Path < current.Path {
		return candidate
	}
	return current
}

func isRepresentativeDeploymentPath(path *model.FeaturePath) bool {
	return isConfigurationDeploymentPath(path) || isServiceProfileDeploymentPath(path)
}

func isServiceProfileDeploymentPath(path *model.FeaturePath) bool {
	if path == nil {
		return false
	}
	return path.ProfileType == model.ProfileTypeService || path.BlueprintCategory == model.BlueprintCategoryService
}

func (g *Generator) generateDeploymentCRUDLifecycleTest(feature *model.Feature, createPath, readPath, updatePath, deletePath *model.FeaturePath) model.TestCase {
	profileName := fmt.Sprintf("TestProfile-%s-CRUD", feature.Name)
	tc := model.TestCase{
		TestCaseID:       g.nextTestID(),
		FeatureName:      feature.Name,
		Priority:         model.TestPriorityP1,
		Type:             model.TestCategoryFunctional,
		Description:      fmt.Sprintf("Deployed CRUD lifecycle for %s (Create -> Read -> Update -> Read -> Deploy -> Delete -> Read -> Redeploy removal)", feature.Name),
		ScopeType:        model.ScopeTypeDevice,
		TargetType:       model.TargetTypeDevice,
		DeploymentMethod: model.DeploymentMethodImmediate,
		IsDeploymentTest: true,
		Steps:            []model.TestStep{},
		Metadata:         map[string]interface{}{"deploymentSample": "crud-lifecycle"},
	}

	featureProfileName := profileName
	deploymentProfileName := profileName
	if isServiceProfileDeploymentPath(createPath) {
		featureProfileName = fmt.Sprintf("TestServiceProfile-%s-CRUD", feature.Name)
		deploymentProfileName = fmt.Sprintf("TestConfigProfile-%s-CRUD", feature.Name)
		tc.Steps = append(tc.Steps, serviceProfileContainerStep(feature, featureProfileName))
	}

	createBody := g.generateRequestBody(feature, createPath)
	keyDesc := describeKeyValues(createBody, feature)
	tc.Steps = append(tc.Steps,
		g.createCRUDCreateStep(feature, createPath, featureProfileName, createBody, keyDesc),
		g.createCRUDReadStep(feature, readPath, createPath, featureProfileName, "verifyCreation", fmt.Sprintf("Verify %s with %s was created", feature.Name, keyDesc), keyDesc, createBody),
	)

	updateBody := g.generateUpdateRequestBody(feature, updatePath)
	updateDesc := describeModifiedFields(updateBody, feature)
	tc.Steps = append(tc.Steps,
		g.createCRUDUpdateStep(feature, updatePath, featureProfileName, updateBody, updateDesc),
		g.createCRUDReadStep(feature, readPath, updatePath, featureProfileName, "verifyUpdate", fmt.Sprintf("Verify %s was updated — %s", feature.Name, updateDesc), updateDesc, updateBody),
	)

	if isServiceProfileDeploymentPath(createPath) {
		tc.Steps = append(tc.Steps, configurationProfileWithServiceProfileStep(feature, deploymentProfileName, featureProfileName))
	}
	g.addDeploymentSteps(&tc, feature, deploymentProfileName, model.ScopeTypeDevice, model.TargetTypeDevice)

	tc.Steps = append(tc.Steps,
		g.createCRUDDeleteStep(feature, deletePath, featureProfileName, keyDesc),
		g.createCRUDDeletionReadStep(feature, readPath, createPath, featureProfileName, "verifyDeletion", fmt.Sprintf("Verify %s with %s is no longer returned after delete", feature.Name, keyDesc), keyDesc, createBody),
	)
	g.appendDeploymentRemovalExecutionSteps(&tc, feature, deploymentProfileName, model.TargetTypeDevice)

	return tc
}

func (g *Generator) appendDeploymentRemovalExecutionSteps(tc *model.TestCase, feature *model.Feature, profileName string, targetType model.TargetType) {
	tc.Steps = append(tc.Steps, noConflictBeforeDeploymentStep(profileName))
	tc.Steps = append(tc.Steps, deploymentRequestStep(profileName, targetType))
	tc.Steps = append(tc.Steps, deploymentStatusStep(profileName, targetType))
	g.appendNOSRemovalVerificationStep(tc, feature, targetType)
}

func (g *Generator) appendNOSRemovalVerificationStep(tc *model.TestCase, feature *model.Feature, targetType model.TargetType) {
	if g.nosParser == nil {
		return
	}
	nosEndpoint := g.nosParser.FindEndpointForFeature(feature.Name, model.ScopeType(targetType))
	if nosEndpoint == nil {
		return
	}
	tc.Steps = append(tc.Steps, model.TestStep{
		Name:        fmt.Sprintf("verifyNosConfigRemovedFor%s", targetType),
		Description: fmt.Sprintf("Verify the deleted %s configuration is no longer present on the NOS device", feature.Name),
		Method:      nosEndpoint.Method,
		API:         model.APITypeNOSAPI,
		Path:        nosEndpoint.Path,
		PathParams: map[string]string{
			"siteGroupId": locationSiteGroupID,
			"siteId":      locationSiteID,
			"deviceId":    locationDeviceID,
			"hostName":    locationDeviceName,
		},
		ExpectedStatus: 200,
		DevicesScope:   nosEndpoint.DeviceScope,
		Validations: []model.Validation{
			{Type: model.ValidationTypeStatusCode, Expected: 200},
			{Type: model.ValidationTypeNosConfigMatchesExpected, Description: "Verify NOS device configuration matches the deleted expected state"},
		},
	})
}

func (g *Generator) createCRUDCreateStep(feature *model.Feature, createPath *model.FeaturePath, profileName string, body map[string]interface{}, keyDesc string) model.TestStep {
	return model.TestStep{
		Name:           "createResource",
		Description:    fmt.Sprintf("Create %s with %s", feature.Name, keyDesc),
		Method:         createPath.HTTPMethod,
		API:            model.APITypeREST,
		Path:           createPath.Path,
		PathParams:     pathParamsFor(createPath.Path, profileName),
		Body:           body,
		ExpectedStatus: 201,
		Validations:    withObjectIDCapture(crudValidations(201, "create", feature.Name, keyDesc)),
	}
}

func (g *Generator) createCRUDReadStep(feature *model.Feature, readPath, bodySourcePath *model.FeaturePath, profileName, name, description, validationDesc string, sourceBody map[string]interface{}) model.TestStep {
	return model.TestStep{
		Name:           name,
		Description:    description,
		Method:         readPath.HTTPMethod,
		API:            model.APITypeREST,
		Path:           readPath.Path,
		PathParams:     pathParamsFor(readPath.Path, profileName),
		Body:           g.generateReadBodyForRequestBody(feature, bodySourcePath, sourceBody),
		ExpectedStatus: 200,
		Validations:    crudValidations(200, "read", feature.Name, validationDesc),
	}
}

func (g *Generator) createCRUDDeletionReadStep(feature *model.Feature, readPath, bodySourcePath *model.FeaturePath, profileName, name, description, validationDesc string, sourceBody map[string]interface{}) model.TestStep {
	return model.TestStep{
		Name:           name,
		Description:    description,
		Method:         readPath.HTTPMethod,
		API:            model.APITypeREST,
		Path:           readPath.Path,
		PathParams:     pathParamsFor(readPath.Path, profileName),
		Body:           g.generateReadBodyForRequestBody(feature, bodySourcePath, sourceBody),
		ExpectedStatus: 200,
		Validations:    deletionReadBackValidations(200, feature.Name, validationDesc),
	}
}

func (g *Generator) createCRUDUpdateStep(feature *model.Feature, updatePath *model.FeaturePath, profileName string, body map[string]interface{}, updateDesc string) model.TestStep {
	return model.TestStep{
		Name:           "updateResource",
		Description:    fmt.Sprintf("Update %s — %s", feature.Name, updateDesc),
		Method:         updatePath.HTTPMethod,
		API:            model.APITypeREST,
		Path:           updatePath.Path,
		PathParams:     pathParamsFor(updatePath.Path, profileName),
		Body:           body,
		ExpectedStatus: 200,
		Validations:    crudValidations(200, "update", feature.Name, updateDesc),
	}
}

func (g *Generator) createCRUDDeleteStep(feature *model.Feature, deletePath *model.FeaturePath, profileName, keyDesc string) model.TestStep {
	return model.TestStep{
		Name:           "deleteResource",
		Description:    fmt.Sprintf("Delete %s with %s", feature.Name, keyDesc),
		Method:         deletePath.HTTPMethod,
		API:            model.APITypeREST,
		Path:           deletePath.Path,
		PathParams:     pathParamsFor(deletePath.Path, profileName),
		Body:           g.generateDeleteBody(feature, deletePath),
		ExpectedStatus: 200,
		Validations:    crudValidations(200, "delete", feature.Name, keyDesc),
	}
}

func (g *Generator) generateDeploymentBoundarySampleTests(feature *model.Feature, createPath, readPath *model.FeaturePath) []model.TestCase {
	candidates := g.deploymentBoundaryCandidates(feature)
	if len(candidates) > deploymentBoundarySampleLimit {
		candidates = candidates[:deploymentBoundarySampleLimit]
	}
	tests := make([]model.TestCase, 0, len(candidates))
	for i, candidate := range candidates {
		profileName := fmt.Sprintf("TestProfile-%s-Boundary-%d", feature.Name, i+1)
		featureProfileName := profileName
		deploymentProfileName := profileName
		tc := model.TestCase{
			TestCaseID:       g.nextTestID(),
			FeatureName:      feature.Name,
			Priority:         model.TestPriorityP2,
			Type:             model.TestCategoryBoundary,
			Description:      fmt.Sprintf("Deploy %s with boundary value for '%s' (%s)", feature.Name, candidate.param.Name, candidate.constraint.Type),
			ScopeType:        model.ScopeTypeDevice,
			TargetType:       model.TargetTypeDevice,
			DeploymentMethod: model.DeploymentMethodImmediate,
			IsDeploymentTest: true,
			Steps:            []model.TestStep{},
			Metadata:         map[string]interface{}{"deploymentSample": "boundary", "field": candidate.param.Name, "constraint": candidate.constraint.Type},
		}

		if isServiceProfileDeploymentPath(createPath) {
			featureProfileName = fmt.Sprintf("TestServiceProfile-%s-Boundary-%d", feature.Name, i+1)
			deploymentProfileName = fmt.Sprintf("TestConfigProfile-%s-Boundary-%d", feature.Name, i+1)
			tc.Steps = append(tc.Steps, serviceProfileContainerStep(feature, featureProfileName))
		}

		body := g.generateRequestBody(feature, createPath)
		g.setOrAddBodyParameter(body, candidate.param, candidate.value)
		keyDesc := describeKeyValues(body, feature)
		tc.Steps = append(tc.Steps, g.createCRUDCreateStep(feature, createPath, featureProfileName, body, keyDesc))
		if readPath != nil {
			tc.Steps = append(tc.Steps, g.createCRUDReadStep(feature, readPath, createPath, featureProfileName, "verifyBoundaryCreate", fmt.Sprintf("Verify %s accepted boundary value for '%s'", feature.Name, candidate.param.Name), keyDesc, body))
		}
		if isServiceProfileDeploymentPath(createPath) {
			tc.Steps = append(tc.Steps, configurationProfileWithServiceProfileStep(feature, deploymentProfileName, featureProfileName))
		}
		g.addDeploymentSteps(&tc, feature, deploymentProfileName, model.ScopeTypeDevice, model.TargetTypeDevice)
		tests = append(tests, tc)
	}
	return tests
}

func (g *Generator) deploymentBoundaryCandidates(feature *model.Feature) []deploymentBoundaryCandidate {
	var candidates []deploymentBoundaryCandidate
	seen := map[string]bool{}
	for _, param := range collectAllParameters(feature.Parameters) {
		if isSystemManagedField(param.Name) {
			continue
		}
		for _, constraint := range param.Constraints {
			value, ok := g.boundaryDeploymentValue(param, constraint)
			if !ok {
				continue
			}
			key := fmt.Sprintf("%s:%s", param.Name, constraint.Type)
			if seen[key] {
				continue
			}
			seen[key] = true
			candidates = append(candidates, deploymentBoundaryCandidate{param: param, constraint: constraint, value: value})
		}
	}
	return candidates
}

func (g *Generator) boundaryDeploymentValue(param model.Parameter, constraint model.Constraint) (interface{}, bool) {
	switch constraint.Type {
	case model.ConstraintTypeMinLength, model.ConstraintTypeMaxLength:
		length, ok := constraint.Value.(int)
		if !ok || length < 0 {
			return nil, false
		}
		return generateString(length), true
	case model.ConstraintTypeMin, model.ConstraintTypeMax:
		return constraint.Value, constraint.Value != nil
	case model.ConstraintTypeMinItems, model.ConstraintTypeMaxItems:
		count, ok := constraint.Value.(int)
		if !ok || count < 0 {
			return nil, false
		}
		items := make([]interface{}, count)
		for i := 0; i < count; i++ {
			items[i] = fmt.Sprintf("item-%d", i)
		}
		return items, true
	case model.ConstraintTypeEnum:
		values := getEnumValues(param)
		if len(values) == 0 {
			return nil, false
		}
		return values[0], true
	}
	return nil, false
}

func (g *Generator) generateDeploymentScaleSampleTest(feature *model.Feature, createPath, listPath *model.FeaturePath) model.TestCase {
	instanceCount := g.config.ScaleFactor
	if instanceCount > deploymentScaleSampleLimit {
		instanceCount = deploymentScaleSampleLimit
	}
	if instanceCount < 1 {
		instanceCount = 1
	}

	profileName := fmt.Sprintf("TestProfile-%s-Scale", feature.Name)
	featureProfileName := profileName
	deploymentProfileName := profileName
	tc := model.TestCase{
		TestCaseID:       g.nextTestID(),
		FeatureName:      feature.Name,
		Priority:         model.TestPriorityP2,
		Type:             model.TestCategoryScale,
		Description:      fmt.Sprintf("Deploy %d representative %s instances", instanceCount, feature.Name),
		ScopeType:        model.ScopeTypeDevice,
		TargetType:       model.TargetTypeDevice,
		DeploymentMethod: model.DeploymentMethodImmediate,
		IsDeploymentTest: true,
		Steps:            []model.TestStep{},
		Metadata:         map[string]interface{}{"deploymentSample": "scale", "instanceCount": instanceCount},
	}

	if isServiceProfileDeploymentPath(createPath) {
		featureProfileName = fmt.Sprintf("TestServiceProfile-%s-Scale", feature.Name)
		deploymentProfileName = fmt.Sprintf("TestConfigProfile-%s-Scale", feature.Name)
		tc.Steps = append(tc.Steps, serviceProfileContainerStep(feature, featureProfileName))
	}

	for i := 0; i < instanceCount; i++ {
		body := g.generateRequestBody(feature, createPath)
		g.setBodyResourceName(feature, body, fmt.Sprintf("DeployScale-%s-%d", feature.Name, i+1))
		g.incrementUniqueBodyValues(body, feature, i)
		keyDesc := describeKeyValues(body, feature)
		step := g.createCRUDCreateStep(feature, createPath, featureProfileName, body, keyDesc)
		step.Name = fmt.Sprintf("createDeploymentScaleInstance%d", i+1)
		step.Description = fmt.Sprintf("Create deployment scale instance %d for %s with %s", i+1, feature.Name, keyDesc)
		tc.Steps = append(tc.Steps, step)
	}

	if listPath != nil {
		tc.Steps = append(tc.Steps, model.TestStep{
			Name:           "listDeploymentScaleInstances",
			Description:    fmt.Sprintf("List %s instances before deployment", feature.Name),
			Method:         listPath.HTTPMethod,
			API:            model.APITypeREST,
			Path:           listPath.Path,
			PathParams:     pathParamsFor(listPath.Path, featureProfileName),
			Body:           g.generateReadBody(feature, listPath),
			ExpectedStatus: 200,
			Validations:    statusValidation(200),
		})
	}

	if isServiceProfileDeploymentPath(createPath) {
		tc.Steps = append(tc.Steps, configurationProfileWithServiceProfileStep(feature, deploymentProfileName, featureProfileName))
	}
	g.addDeploymentSteps(&tc, feature, deploymentProfileName, model.ScopeTypeDevice, model.TargetTypeDevice)

	return tc
}

func serviceProfileContainerStep(feature *model.Feature, serviceProfileName string) model.TestStep {
	return model.TestStep{
		Name:           "createServiceProfile",
		Description:    "Create service profile container before adding service feature configuration",
		Method:         "POST",
		API:            model.APITypeREST,
		Path:           serviceProfileCreatePath,
		PathParams:     map[string]string{"name": serviceProfileName},
		Body:           map[string]interface{}{"description": fmt.Sprintf("Service profile for %s deployment test", feature.Name)},
		ExpectedStatus: 201,
		Validations:    statusValidation(201),
	}
}

func configurationProfileWithServiceProfileStep(feature *model.Feature, configurationProfileName, serviceProfileName string) model.TestStep {
	return model.TestStep{
		Name:        "createConfigurationProfileWithServiceProfile",
		Description: "Create configuration profile and link the service profile so its configuration can be deployed",
		Method:      "POST",
		API:         model.APITypeREST,
		Path:        configurationProfilePath,
		PathParams:  map[string]string{"name": configurationProfileName},
		Body: map[string]interface{}{
			"blueprints":          []string{"wired-blueprint"},
			"serviceProfiles":     []string{serviceProfileName},
			"networkArchitecture": "standard",
			"description":         fmt.Sprintf("Configuration profile for %s service-profile deployment", feature.Name),
		},
		ExpectedStatus: 201,
		Validations:    statusValidation(201),
	}
}
