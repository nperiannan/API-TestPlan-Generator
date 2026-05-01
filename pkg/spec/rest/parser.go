package rest

import (
	"fmt"
	"strings"

	"github.com/extremenetworks/testcase-generator/pkg/model"
	"github.com/getkin/kin-openapi/openapi3"
)

// Parser parses OpenAPI/REST specifications
type Parser struct {
	specPath string
	doc      *openapi3.T
	paths    map[string]*model.FeaturePath
}

// NewParser creates a new REST API parser
func NewParser(specPath string) *Parser {
	return &Parser{
		specPath: specPath,
		paths:    make(map[string]*model.FeaturePath),
	}
}

// Parse loads and parses the OpenAPI specification
func (p *Parser) Parse() error {
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = true
	doc, err := loader.LoadFromFile(p.specPath)
	if err != nil {
		return fmt.Errorf("failed to load OpenAPI spec: %w", err)
	}

	// Skip validation to handle specs with invalid examples
	// We only need the structure for test generation

	p.doc = doc

	// Extract feature paths from API endpoints
	if err := p.extractFeaturePaths(); err != nil {
		return err
	}

	// DEEP SCAN: Extract features from nested schemas (GlobalProfile, ServiceProfile, Blueprints)
	if err := p.extractFeaturesFromSchemas(); err != nil {
		return err
	}

	return nil
}

// extractFeaturePaths extracts feature paths from the OpenAPI spec
func (p *Parser) extractFeaturePaths() error {
	for path, pathItem := range p.doc.Paths {
		// Process each HTTP method
		for method, operation := range pathItem.Operations() {
			fp := &model.FeaturePath{
				HTTPMethod:    strings.ToUpper(method),
				Path:          path,
				PathParams:    []model.PathParameter{},
				ProfileType:   p.detectProfileType(path),
				OperationType: p.detectOperationType(method, path),
			}

			// Extract path parameters
			for _, param := range pathItem.Parameters {
				if param.Value != nil && param.Value.In == "path" {
					fp.PathParams = append(fp.PathParams, model.PathParameter{
						Name:        param.Value.Name,
						Type:        p.schemaTypeToString(param.Value.Schema),
						Description: param.Value.Description,
						Required:    param.Value.Required,
					})
				}
			}

			// Extract operation-specific parameters
			if operation != nil {
				for _, param := range operation.Parameters {
					if param.Value != nil && param.Value.In == "path" {
						fp.PathParams = append(fp.PathParams, model.PathParameter{
							Name:        param.Value.Name,
							Type:        p.schemaTypeToString(param.Value.Schema),
							Description: param.Value.Description,
							Required:    param.Value.Required,
						})
					}
				}

				// Extract request body schema
				if operation.RequestBody != nil && operation.RequestBody.Value != nil {
					content := operation.RequestBody.Value.Content
					if jsonContent, ok := content["application/json"]; ok && jsonContent.Schema != nil {
						fp.RequestSchema = p.schemaToMap(jsonContent.Schema.Value)
					}
				}

				// Extract response schema
				if responses := operation.Responses; responses != nil {
					if successResp := responses["200"]; successResp != nil && successResp.Value != nil {
						content := successResp.Value.Content
						if jsonContent, ok := content["application/json"]; ok && jsonContent.Schema != nil {
							fp.ResponseSchema = p.schemaToMap(jsonContent.Schema.Value)
						}
					}
					// Also check 201 for create operations
					if successResp := responses["201"]; successResp != nil && successResp.Value != nil {
						content := successResp.Value.Content
						if jsonContent, ok := content["application/json"]; ok && jsonContent.Schema != nil {
							fp.ResponseSchema = p.schemaToMap(jsonContent.Schema.Value)
						}
					}
				}
			}

			// Detect deployment-related capabilities
			p.detectDeploymentCapabilities(fp, path)

			p.paths[fmt.Sprintf("%s %s", fp.HTTPMethod, fp.Path)] = fp
		}
	}

	return nil
}

// detectProfileType determines the profile type from the path
func (p *Parser) detectProfileType(path string) model.ProfileType {
	if strings.Contains(path, "global-profile") {
		return model.ProfileTypeGlobal
	}
	if strings.Contains(path, "configuration-profile") {
		return model.ProfileTypeConfiguration
	}
	if strings.Contains(path, "service-profile") {
		return model.ProfileTypeService
	}
	return model.ProfileTypeConfiguration // default
}

// detectOperationType determines the operation type from method and path
func (p *Parser) detectOperationType(method, path string) model.OperationType {
	methodUpper := strings.ToUpper(method)
	pathLower := strings.ToLower(path)

	if strings.Contains(pathLower, "/scope") {
		return model.OperationTypeScope
	}
	if strings.Contains(pathLower, "/target") {
		return model.OperationTypeTarget
	}
	if strings.Contains(pathLower, "/deploy") {
		if methodUpper == "GET" {
			return model.OperationTypeStatus
		}
		return model.OperationTypeDeploy
	}

	switch methodUpper {
	case "POST":
		return model.OperationTypeCreate
	case "GET":
		if strings.Contains(pathLower, "{") {
			return model.OperationTypeRead
		}
		return model.OperationTypeList
	case "PUT", "PATCH":
		return model.OperationTypeUpdate
	case "DELETE":
		return model.OperationTypeDelete
	default:
		return model.OperationTypeRead
	}
}

// detectDeploymentCapabilities identifies deployment-related capabilities
func (p *Parser) detectDeploymentCapabilities(fp *model.FeaturePath, path string) {
	pathLower := strings.ToLower(path)

	// Check for scope support
	if strings.Contains(pathLower, "/scope") {
		fp.SupportsScope = true
		if strings.Contains(pathLower, "site-group") {
			fp.SupportedScopeTypes = append(fp.SupportedScopeTypes, model.ScopeTypeSiteGroup)
		}
		if strings.Contains(pathLower, "device") {
			fp.SupportedScopeTypes = append(fp.SupportedScopeTypes, model.ScopeTypeDevice)
		}
	}

	// Check for target support
	if strings.Contains(pathLower, "/target") {
		fp.SupportsTarget = true
		if strings.Contains(pathLower, "site-group") {
			fp.SupportedTargetTypes = append(fp.SupportedTargetTypes, model.TargetTypeSiteGroup)
		}
		if strings.Contains(pathLower, "device") {
			fp.SupportedTargetTypes = append(fp.SupportedTargetTypes, model.TargetTypeDevice)
		}
	}

	// Check for deployment support
	if strings.Contains(pathLower, "/deploy") {
		fp.SupportsDeployment = true
		// Add all deployment methods as supported (can be refined based on spec)
		fp.SupportedDeploymentMethods = []model.DeploymentMethod{
			model.DeploymentMethodRolling,
			model.DeploymentMethodImmediate,
			model.DeploymentMethodStaged,
		}
	}
}

// schemaTypeToString converts OpenAPI schema type to string
func (p *Parser) schemaTypeToString(schemaRef *openapi3.SchemaRef) string {
	if schemaRef == nil || schemaRef.Value == nil {
		return "string"
	}

	schema := schemaRef.Value
	if schema.Type != "" {
		return schema.Type
	}
	return "string"
}

// schemaToMap converts OpenAPI schema to a map structure
func (p *Parser) schemaToMap(schema *openapi3.Schema) map[string]interface{} {
	if schema == nil {
		return nil
	}

	result := make(map[string]interface{})

	// Handle object type
	if schema.Type == "object" {
		for propName, propSchema := range schema.Properties {
			if propSchema.Value != nil {
				propType := "string"
				if propSchema.Value.Type != "" {
					propType = propSchema.Value.Type
				}

				result[propName] = map[string]interface{}{
					"type":        propType,
					"description": propSchema.Value.Description,
					"required":    contains(schema.Required, propName),
				}

				// Add enum values if present
				if len(propSchema.Value.Enum) > 0 {
					result[propName].(map[string]interface{})["enum"] = propSchema.Value.Enum
				}

				// Add min/max for numbers
				if propSchema.Value.Min != nil {
					result[propName].(map[string]interface{})["min"] = *propSchema.Value.Min
				}
				if propSchema.Value.Max != nil {
					result[propName].(map[string]interface{})["max"] = *propSchema.Value.Max
				}
			}
		}
	}

	return result
}

// GetFeaturePaths returns all parsed feature paths
func (p *Parser) GetFeaturePaths() map[string]*model.FeaturePath {
	return p.paths
}

// extractFeaturesFromSchemas performs deep scanning of nested schemas to extract features
// This finds DNS, NTP, Syslog, DHCP, VLAN and other features buried in blueprint schemas
func (p *Parser) extractFeaturesFromSchemas() error {
	if p.doc.Components == nil || p.doc.Components.Schemas == nil {
		return nil
	}

	// Scan GlobalProfile schema for DNS, NTP, Syslog, DHCP features
	p.extractGlobalProfileFeatures()

	// Scan ServiceProfile schema for L2Feature (VLAN) and other service features
	p.extractServiceProfileFeatures()

	// Scan ConfigurationProfile/Blueprint schemas for wired features
	p.extractConfigurationProfileFeatures()

	// Scan ConfigurationProfile/Blueprint schemas for wireless features
	p.extractWirelessProfileFeatures()

	return nil
}

// extractGlobalProfileFeatures extracts features from GlobalProfile schema
// Features like DNS Server, NTP Server, Syslog, DHCP Server
func (p *Parser) extractGlobalProfileFeatures() {
	// GlobalProfile features use paths like:
	// /global-profile/feature/object/modify with featurePath parameter
	// /global-profile/feature/object/retrieve with featurePath parameter

	globalFeatures := []struct {
		name        string
		featurePath string
	}{
		{"dns-server", "/dns-server-feature"},
		{"dns-suffix", "/dns-server-feature"},
		{"ntp-server", "/ntp-server-feature"},
		{"syslog-server", "/syslog-server-feature"},
		{"dhcp-server", "/dhcp-server-feature"},
		{"radius-server", "/radius-server-feature"},
	}

	// Global-profile features are deployed through a configuration profile.
	// Register the profile name param once for re-use in scope/deploy/status paths.
	globalCfgProfileNameParam := model.PathParameter{
		Name:        "name",
		Type:        "string",
		Description: "Configuration profile name used to deploy global-profile settings",
		Required:    true,
	}

	for _, gf := range globalFeatures {
		featurePathParam := model.PathParameter{
			Name:        "featurePath",
			Type:        "string",
			Description: "Path to the feature",
			Required:    true,
			FixedValue:  gf.featurePath,
		}
		commonParams := []model.PathParameter{featurePathParam}

		// READ
		retrieveKey := "POST /global-profile/feature/object/retrieve"
		if _, exists := p.paths[retrieveKey]; exists {
			p.paths[fmt.Sprintf("READ /global-profile/%s", gf.name)] = &model.FeaturePath{
				FeatureName:       gf.name,
				BlueprintCategory: model.BlueprintCategoryGlobal,
				HTTPMethod:        "POST",
				Path:              "/global-profile/feature/object/retrieve",
				PathParams:        commonParams,
				ProfileType:       model.ProfileTypeGlobal,
				OperationType:     model.OperationTypeRead,
			}
		}

		// CREATE
		modifyKey := "POST /global-profile/feature/object/modify"
		if _, exists := p.paths[modifyKey]; exists {
			p.paths[fmt.Sprintf("POST /global-profile/%s", gf.name)] = &model.FeaturePath{
				FeatureName:       gf.name,
				BlueprintCategory: model.BlueprintCategoryGlobal,
				HTTPMethod:        "POST",
				Path:              "/global-profile/feature/object/modify",
				PathParams:        commonParams,
				ProfileType:       model.ProfileTypeGlobal,
				OperationType:     model.OperationTypeCreate,
			}

			// UPDATE
			p.paths[fmt.Sprintf("PUT /global-profile/%s", gf.name)] = &model.FeaturePath{
				FeatureName:       gf.name,
				BlueprintCategory: model.BlueprintCategoryGlobal,
				HTTPMethod:        "POST",
				Path:              "/global-profile/feature/object/modify",
				PathParams:        commonParams,
				ProfileType:       model.ProfileTypeGlobal,
				OperationType:     model.OperationTypeUpdate,
			}
		}

		// DELETE
		deleteKey := "POST /global-profile/feature/object/delete"
		if _, exists := p.paths[deleteKey]; exists {
			p.paths[fmt.Sprintf("DELETE /global-profile/%s", gf.name)] = &model.FeaturePath{
				FeatureName:       gf.name,
				BlueprintCategory: model.BlueprintCategoryGlobal,
				HTTPMethod:        "POST",
				Path:              "/global-profile/feature/object/delete",
				PathParams: append(commonParams, model.PathParameter{
					Name:        "objectId",
					Type:        "string",
					Description: "ID of the object to delete",
					Required:    true,
				}),
				ProfileType:   model.ProfileTypeGlobal,
				OperationType: model.OperationTypeDelete,
			}
		}

		// --- Deployment paths (global-profile features deploy through a configuration profile) ---

		// SCOPE: POST /configuration-profile/{name}/scope
		p.paths[fmt.Sprintf("POST /global-profile/%s/scope", gf.name)] = &model.FeaturePath{
			FeatureName:       gf.name,
			BlueprintCategory: model.BlueprintCategoryGlobal,
			HTTPMethod:        "POST",
			Path:              "/configuration-profile/{name}/scope",
			PathParams:        []model.PathParameter{globalCfgProfileNameParam},
			ProfileType:       model.ProfileTypeGlobal,
			OperationType:     model.OperationTypeScope,
			SupportsScope:     true,
			SupportedScopeTypes: []model.ScopeType{
				model.ScopeTypeDevice,
				model.ScopeTypeSite,
				model.ScopeTypeSiteGroup,
			},
		}

		// DEPLOY: sites
		p.paths[fmt.Sprintf("POST /global-profile/%s/sites/deploy", gf.name)] = &model.FeaturePath{
			FeatureName:        gf.name,
			BlueprintCategory:  model.BlueprintCategoryGlobal,
			HTTPMethod:         "POST",
			Path:               "/configuration-profile/{name}/sites/deploy",
			PathParams:         []model.PathParameter{globalCfgProfileNameParam},
			ProfileType:        model.ProfileTypeGlobal,
			OperationType:      model.OperationTypeDeploy,
			SupportsDeployment: true,
			SupportedTargetTypes: []model.TargetType{
				model.TargetTypeSite,
				model.TargetTypeSiteGroup,
			},
		}

		// DEPLOY: devices
		p.paths[fmt.Sprintf("POST /global-profile/%s/devices/deploy", gf.name)] = &model.FeaturePath{
			FeatureName:          gf.name,
			BlueprintCategory:    model.BlueprintCategoryGlobal,
			HTTPMethod:           "POST",
			Path:                 "/configuration-profile/{name}/devices/deploy",
			PathParams:           []model.PathParameter{globalCfgProfileNameParam},
			ProfileType:          model.ProfileTypeGlobal,
			OperationType:        model.OperationTypeDeploy,
			SupportsDeployment:   true,
			SupportedTargetTypes: []model.TargetType{model.TargetTypeDevice},
		}

		// STATUS: site
		p.paths[fmt.Sprintf("GET /global-profile/%s/site/status", gf.name)] = &model.FeaturePath{
			FeatureName:       gf.name,
			BlueprintCategory: model.BlueprintCategoryGlobal,
			HTTPMethod:        "GET",
			Path:              "/configuration-profile/{name}/site/{siteName}/deploy/status",
			PathParams: []model.PathParameter{globalCfgProfileNameParam, {
				Name: "siteName", Type: "string", Description: "Site name", Required: true,
			}},
			ProfileType:   model.ProfileTypeGlobal,
			OperationType: model.OperationTypeStatus,
		}

		// STATUS: device
		p.paths[fmt.Sprintf("GET /global-profile/%s/device/status", gf.name)] = &model.FeaturePath{
			FeatureName:       gf.name,
			BlueprintCategory: model.BlueprintCategoryGlobal,
			HTTPMethod:        "GET",
			Path:              "/configuration-profile/{name}/device/{hostName}/deploy/status",
			PathParams: []model.PathParameter{globalCfgProfileNameParam, {
				Name: "hostName", Type: "string", Description: "Device host name", Required: true,
			}},
			ProfileType:   model.ProfileTypeGlobal,
			OperationType: model.OperationTypeStatus,
		}
	}
}

// extractServiceProfileFeatures extracts features from ServiceProfile schema
// Features from extreme-service-profile-blueprint.yang
func (p *Parser) extractServiceProfileFeatures() {
	// Service profile nested features.
	// yangFeatureName must match the YANG list name from the respective intent module.
	// Source: extreme-service-profile-blueprint.yang + qaopenapi.yaml
	serviceFeatures := []struct {
		yangFeatureName string // YANG list name for exact linking
		featurePath     string // featurePath value in API request body
	}{
		{"vlan", "/l2-service-feature"},                        // extreme-intent-vlan.yang (list vlan)
		{"vrf", "/vrf-feature"},                                // extreme-intent-vrf.yang (list vrf)
		{"router-group", "/router-group-feature"},              // extreme-intent-router-group.yang (list router-group)
		{"common-settings", "/global-feature"},                 // extreme-intent-global.yang (list common-settings)
		{"virtual-routing-domain", "/virtual-service-feature"}, // extreme-intent-virtual-routing-domain.yang
		{"static-route", "/virtual-service-feature"},           // extreme-intent-static-route.yang (under virtual service)
		{"ip-subnet", "/l2-service-feature"},                   // extreme-intent-ip-subnet.yang (under l2-service)
	}

	for _, sf := range serviceFeatures {
		profileNameParam := model.PathParameter{
			Name:        "name",
			Type:        "string",
			Description: "Service profile name",
			Required:    true,
		}
		featurePathParam := model.PathParameter{
			Name:        "featurePath",
			Type:        "string",
			Description: "Nested feature path in the service profile hierarchy",
			Required:    true,
			FixedValue:  sf.featurePath,
		}

		// READ
		fpRead := &model.FeaturePath{
			FeatureName:       sf.yangFeatureName,
			BlueprintCategory: model.BlueprintCategoryService,
			HTTPMethod:        "POST",
			Path:              "/service-profile/{name}/feature/object/retrieve",
			PathParams:        []model.PathParameter{profileNameParam, featurePathParam},
			ProfileType:       model.ProfileTypeService,
			OperationType:     model.OperationTypeRead,
		}
		p.paths[fmt.Sprintf("READ /service-profile/%s", sf.yangFeatureName)] = fpRead

		// CREATE
		fpCreate := &model.FeaturePath{
			FeatureName:       sf.yangFeatureName,
			BlueprintCategory: model.BlueprintCategoryService,
			HTTPMethod:        "POST",
			Path:              "/service-profile/{name}/feature/object/modify",
			PathParams:        []model.PathParameter{profileNameParam, featurePathParam},
			ProfileType:       model.ProfileTypeService,
			OperationType:     model.OperationTypeCreate,
		}
		p.paths[fmt.Sprintf("POST /service-profile/%s", sf.yangFeatureName)] = fpCreate

		// UPDATE
		fpUpdate := &model.FeaturePath{
			FeatureName:       sf.yangFeatureName,
			BlueprintCategory: model.BlueprintCategoryService,
			HTTPMethod:        "POST",
			Path:              "/service-profile/{name}/feature/object/modify",
			PathParams:        []model.PathParameter{profileNameParam, featurePathParam},
			ProfileType:       model.ProfileTypeService,
			OperationType:     model.OperationTypeUpdate,
		}
		p.paths[fmt.Sprintf("PUT /service-profile/%s", sf.yangFeatureName)] = fpUpdate

		// DELETE
		fpDelete := &model.FeaturePath{
			FeatureName:       sf.yangFeatureName,
			BlueprintCategory: model.BlueprintCategoryService,
			HTTPMethod:        "POST",
			Path:              "/service-profile/{name}/feature/object/delete",
			PathParams:        []model.PathParameter{profileNameParam, featurePathParam},
			ProfileType:       model.ProfileTypeService,
			OperationType:     model.OperationTypeDelete,
		}
		p.paths[fmt.Sprintf("DELETE /service-profile/%s", sf.yangFeatureName)] = fpDelete
	}
}

// extractConfigurationProfileFeatures extracts features from ConfigurationProfile/Blueprint schemas.
// These map to the nested featurePath hierarchy defined in extreme-wired-blueprint.yang.
// The yangFeatureName MUST match the YANG list node name so linkFeaturesWithPaths() can do an
// exact match and avoid incorrect fuzzy linking to unrelated API paths.
func (p *Parser) extractConfigurationProfileFeatures() {
	// Wired blueprint nested features.
	// Source: extreme-wired-blueprint.yang + qaopenapi.yaml (feature/object endpoints).
	//
	// Mapping: yangFeatureName (YANG list name) → API featurePath → API objectType
	//   port-feature          → extreme-intent-port.yang       (list port)
	//   spbm-global-feature   → extreme-intent-fabric-spbm     (list fabric-spbm-global-settings-config)
	//   spbm-instance         → extreme-intent-fabric-spbm     (list fabric-spbm-instance)
	//   isis-feature          → extreme-intent-fabric-spbm     (list fabric-isis-global-config)
	//   auto-sense-feature    → extreme-intent-fabric-auto-sense (list fabric-auto-sense-global, fabric-auto-sense-fabric-attachment, fabric-auto-sense-isis)
	//   device-profile-feature→ extreme-intent-device-profile  (list device-profile)
	//   snmp-feature          → extreme-intent-snmp.yang       (list snmp-global-config, snmp-access-config, snmp-v3-access-config)
	//   isis-global-feature   → extreme-intent-isis.yang       (list isis-global-config)
	wiredNestedFeatures := []struct {
		yangFeatureName string // must match YANG list name for exact linking in linkFeaturesWithPaths
		featurePath     string // the featurePath value sent in the API request body
		objectType      string // the objectType value sent in the API request body
	}{
		// /network-feature/interface-feature/port-feature
		{"port", "/network-feature/interface-feature/port-feature", "port"},
		// Port sub-features (same API path, different objectType)
		{"port-poe", "/network-feature/interface-feature/port-feature", "port-poe"},
		{"port-slpp", "/network-feature/interface-feature/port-feature", "port-slpp"},
		{"port-elrp", "/network-feature/interface-feature/port-feature", "port-elrp"},
		{"port-cdp", "/network-feature/interface-feature/port-feature", "port-cdp"},
		{"port-lldp", "/network-feature/interface-feature/port-feature", "port-lldp"},
		{"port-storm-control", "/network-feature/interface-feature/port-feature", "port-storm-control"},
		{"port-mac-locking", "/network-feature/interface-feature/port-feature", "port-mac-locking"},
		{"advanced-port", "/network-feature/interface-feature/port-feature", "advanced-port"},
		// /network-feature/fabric-feature/spbm-global-feature
		{"fabric-spbm-global-settings-config", "/network-feature/fabric-feature/spbm-global-feature", "fabric-spbm-global-settings-config"},
		{"fabric-spbm-instance", "/network-feature/fabric-feature/spbm-instance-feature", "fabric-spbm-instance"},
		// /network-feature/fabric-feature/isis-feature (fabric-specific ISIS, from extreme-intent-fabric-spbm.yang)
		{"fabric-isis-global-config", "/network-feature/fabric-feature/isis-feature", "fabric-isis-global-config"},
		// /network-feature/fabric-feature/isis-feature (global ISIS, from extreme-intent-isis.yang)
		{"isis-global-config", "/network-feature/fabric-feature/isis-feature", "isis-global-config"},
		// /network-feature/fabric-feature/auto-sense-feature (all sub-features)
		{"fabric-auto-sense-global", "/network-feature/fabric-feature/auto-sense-feature", "fabric-auto-sense-global"},
		{"fabric-auto-sense-fabric-attach", "/network-feature/fabric-feature/auto-sense-feature", "fabric-auto-sense-fabric-attach"},
		{"fabric-auto-sense-isis", "/network-feature/fabric-feature/auto-sense-feature", "fabric-auto-sense-isis"},
		// /infrastructure-feature/device-profile-feature
		{"device-profile", "/infrastructure-feature/device-profile-feature", "device-profile"},
		// /infrastructure-feature/snmp-feature (from extreme-intent-snmp.yang)
		{"snmp-global-config", "/infrastructure-feature/snmp-feature", "snmp-global-config"},
		{"snmp-access-config", "/infrastructure-feature/snmp-feature", "snmp-access-config"},
		{"snmp-v3-access-config", "/infrastructure-feature/snmp-feature", "snmp-v3-access-config"},
		// /infrastructure-feature/qos-feature (QoS policy configuration)
		{"qos-global-config", "/infrastructure-feature/qos-feature", "qos-global-config"},
		{"qos-classifier-profile-config", "/infrastructure-feature/qos-feature", "qos-classifier-profile-config"},
	}

	for _, cf := range wiredNestedFeatures {
		// PathParam entries shared by retrieve/modify/delete (profile name + featurePath fixed value)
		profileNameParam := model.PathParameter{
			Name:        "name",
			Type:        "string",
			Description: "Configuration profile name",
			Required:    true,
		}
		featurePathParam := model.PathParameter{
			Name:        "featurePath",
			Type:        "string",
			Description: "Nested feature path in the wired blueprint hierarchy",
			Required:    true,
			FixedValue:  cf.featurePath, // fixed value tells body-builder what featurePath to embed
		}

		// Build common params list; include objectType override when it differs from feature name
		commonParams := []model.PathParameter{profileNameParam, featurePathParam}
		if cf.objectType != cf.yangFeatureName {
			commonParams = append(commonParams, model.PathParameter{
				Name:       "objectType",
				Type:       "string",
				FixedValue: cf.objectType,
			})
		}

		// READ: POST /configuration-profile/{name}/feature/object/retrieve
		fpRead := &model.FeaturePath{
			FeatureName:       cf.yangFeatureName,
			BlueprintCategory: model.BlueprintCategoryWired,
			HTTPMethod:        "POST",
			Path:              "/configuration-profile/{name}/feature/object/retrieve",
			PathParams:        commonParams,
			ProfileType:       model.ProfileTypeConfiguration,
			OperationType:     model.OperationTypeRead,
		}
		p.paths[fmt.Sprintf("READ /configuration-profile/%s", cf.yangFeatureName)] = fpRead

		// CREATE: POST /configuration-profile/{name}/feature/object/modify  (operation=add)
		fpCreate := &model.FeaturePath{
			FeatureName:       cf.yangFeatureName,
			BlueprintCategory: model.BlueprintCategoryWired,
			HTTPMethod:        "POST",
			Path:              "/configuration-profile/{name}/feature/object/modify",
			PathParams:        commonParams,
			ProfileType:       model.ProfileTypeConfiguration,
			OperationType:     model.OperationTypeCreate,
		}
		p.paths[fmt.Sprintf("POST /configuration-profile/%s", cf.yangFeatureName)] = fpCreate

		// UPDATE: POST /configuration-profile/{name}/feature/object/modify  (operation=update)
		fpUpdate := &model.FeaturePath{
			FeatureName:       cf.yangFeatureName,
			BlueprintCategory: model.BlueprintCategoryWired,
			HTTPMethod:        "POST",
			Path:              "/configuration-profile/{name}/feature/object/modify",
			PathParams:        commonParams,
			ProfileType:       model.ProfileTypeConfiguration,
			OperationType:     model.OperationTypeUpdate,
		}
		p.paths[fmt.Sprintf("PUT /configuration-profile/%s", cf.yangFeatureName)] = fpUpdate

		// DELETE: POST /configuration-profile/{name}/feature/object/delete
		fpDelete := &model.FeaturePath{
			FeatureName:       cf.yangFeatureName,
			BlueprintCategory: model.BlueprintCategoryWired,
			HTTPMethod:        "POST",
			Path:              "/configuration-profile/{name}/feature/object/delete",
			PathParams:        commonParams,
			ProfileType:       model.ProfileTypeConfiguration,
			OperationType:     model.OperationTypeDelete,
		}
		p.paths[fmt.Sprintf("DELETE /configuration-profile/%s", cf.yangFeatureName)] = fpDelete

		// SCOPE: POST /configuration-profile/{name}/scope
		fpScope := &model.FeaturePath{
			FeatureName:       cf.yangFeatureName,
			BlueprintCategory: model.BlueprintCategoryWired,
			HTTPMethod:        "POST",
			Path:              "/configuration-profile/{name}/scope",
			PathParams:        []model.PathParameter{profileNameParam},
			ProfileType:       model.ProfileTypeConfiguration,
			OperationType:     model.OperationTypeScope,
			SupportsScope:     true,
			SupportedScopeTypes: []model.ScopeType{
				model.ScopeTypeDevice,
				model.ScopeTypeSite,
				model.ScopeTypeSiteGroup,
			},
		}
		p.paths[fmt.Sprintf("POST /configuration-profile/%s/scope", cf.yangFeatureName)] = fpScope

		// DEPLOY: POST /configuration-profile/{name}/sites/deploy
		fpSiteDeploy := &model.FeaturePath{
			FeatureName:        cf.yangFeatureName,
			BlueprintCategory:  model.BlueprintCategoryWired,
			HTTPMethod:         "POST",
			Path:               "/configuration-profile/{name}/sites/deploy",
			PathParams:         []model.PathParameter{profileNameParam},
			ProfileType:        model.ProfileTypeConfiguration,
			OperationType:      model.OperationTypeDeploy,
			SupportsDeployment: true,
			SupportedTargetTypes: []model.TargetType{
				model.TargetTypeSite,
				model.TargetTypeSiteGroup,
			},
		}
		p.paths[fmt.Sprintf("POST /configuration-profile/%s/sites/deploy", cf.yangFeatureName)] = fpSiteDeploy

		fpDeviceDeploy := &model.FeaturePath{
			FeatureName:          cf.yangFeatureName,
			BlueprintCategory:    model.BlueprintCategoryWired,
			HTTPMethod:           "POST",
			Path:                 "/configuration-profile/{name}/devices/deploy",
			PathParams:           []model.PathParameter{profileNameParam},
			ProfileType:          model.ProfileTypeConfiguration,
			OperationType:        model.OperationTypeDeploy,
			SupportsDeployment:   true,
			SupportedTargetTypes: []model.TargetType{model.TargetTypeDevice},
		}
		p.paths[fmt.Sprintf("POST /configuration-profile/%s/devices/deploy", cf.yangFeatureName)] = fpDeviceDeploy

		siteNameParam := model.PathParameter{
			Name:        "siteName",
			Type:        "string",
			Description: "Site name",
			Required:    true,
		}
		hostNameParam := model.PathParameter{
			Name:        "hostName",
			Type:        "string",
			Description: "Device host name",
			Required:    true,
		}
		p.paths[fmt.Sprintf("GET /configuration-profile/%s/site/status", cf.yangFeatureName)] = &model.FeaturePath{
			FeatureName:       cf.yangFeatureName,
			BlueprintCategory: model.BlueprintCategoryWired,
			HTTPMethod:        "GET",
			Path:              "/configuration-profile/{name}/site/{siteName}/deploy/status",
			PathParams:        []model.PathParameter{profileNameParam, siteNameParam},
			ProfileType:       model.ProfileTypeConfiguration,
			OperationType:     model.OperationTypeStatus,
		}
		p.paths[fmt.Sprintf("GET /configuration-profile/%s/device/status", cf.yangFeatureName)] = &model.FeaturePath{
			FeatureName:       cf.yangFeatureName,
			BlueprintCategory: model.BlueprintCategoryWired,
			HTTPMethod:        "GET",
			Path:              "/configuration-profile/{name}/device/{hostName}/deploy/status",
			PathParams:        []model.PathParameter{profileNameParam, hostNameParam},
			ProfileType:       model.ProfileTypeConfiguration,
			OperationType:     model.OperationTypeStatus,
		}
	}
}

// extractWirelessProfileFeatures extracts features from the Wireless Blueprint schema.
// Source: extreme-wireless-blueprint.yang + qaopenapi.yaml (feature/object endpoints).
// These use the same /configuration-profile/{name}/feature/object/* API paths as wired,
// but with BlueprintCategoryWireless and wireless-specific featurePaths.
func (p *Parser) extractWirelessProfileFeatures() {
	// Wireless blueprint nested features.
	// Mapping: yangFeatureName (YANG list name) → API featurePath → objectType
	//   wireless-network-feature → extreme-intent-wireless-wlan.yang (list wlan)  → /wlan-feature
	wirelessNestedFeatures := []struct {
		yangFeatureName string
		featurePath     string
		objectType      string
	}{
		// /wlan-feature (from extreme-wireless-blueprint.yang → wireless-network-feature)
		{"wlan", "/wlan-feature", "wlan"},
		// wlan-security is a sub-object of wlan, same featurePath different objectType
		{"wlan-security", "/wlan-feature", "wlan-security"},
	}

	for _, wf := range wirelessNestedFeatures {
		profileNameParam := model.PathParameter{
			Name:        "name",
			Type:        "string",
			Description: "Configuration profile name",
			Required:    true,
		}
		featurePathParam := model.PathParameter{
			Name:        "featurePath",
			Type:        "string",
			Description: "Nested feature path in the wireless blueprint hierarchy",
			Required:    true,
			FixedValue:  wf.featurePath,
		}

		// READ: POST /configuration-profile/{name}/feature/object/retrieve
		fpRead := &model.FeaturePath{
			FeatureName:       wf.yangFeatureName,
			BlueprintCategory: model.BlueprintCategoryWireless,
			HTTPMethod:        "POST",
			Path:              "/configuration-profile/{name}/feature/object/retrieve",
			PathParams:        []model.PathParameter{profileNameParam, featurePathParam},
			ProfileType:       model.ProfileTypeConfiguration,
			OperationType:     model.OperationTypeRead,
		}
		p.paths[fmt.Sprintf("READ /wireless-profile/%s", wf.yangFeatureName)] = fpRead

		// CREATE: POST /configuration-profile/{name}/feature/object/modify
		fpCreate := &model.FeaturePath{
			FeatureName:       wf.yangFeatureName,
			BlueprintCategory: model.BlueprintCategoryWireless,
			HTTPMethod:        "POST",
			Path:              "/configuration-profile/{name}/feature/object/modify",
			PathParams:        []model.PathParameter{profileNameParam, featurePathParam},
			ProfileType:       model.ProfileTypeConfiguration,
			OperationType:     model.OperationTypeCreate,
		}
		p.paths[fmt.Sprintf("POST /wireless-profile/%s", wf.yangFeatureName)] = fpCreate

		// UPDATE: POST /configuration-profile/{name}/feature/object/modify
		fpUpdate := &model.FeaturePath{
			FeatureName:       wf.yangFeatureName,
			BlueprintCategory: model.BlueprintCategoryWireless,
			HTTPMethod:        "POST",
			Path:              "/configuration-profile/{name}/feature/object/modify",
			PathParams:        []model.PathParameter{profileNameParam, featurePathParam},
			ProfileType:       model.ProfileTypeConfiguration,
			OperationType:     model.OperationTypeUpdate,
		}
		p.paths[fmt.Sprintf("PUT /wireless-profile/%s", wf.yangFeatureName)] = fpUpdate

		// DELETE: POST /configuration-profile/{name}/feature/object/delete
		fpDelete := &model.FeaturePath{
			FeatureName:       wf.yangFeatureName,
			BlueprintCategory: model.BlueprintCategoryWireless,
			HTTPMethod:        "POST",
			Path:              "/configuration-profile/{name}/feature/object/delete",
			PathParams:        []model.PathParameter{profileNameParam, featurePathParam},
			ProfileType:       model.ProfileTypeConfiguration,
			OperationType:     model.OperationTypeDelete,
		}
		p.paths[fmt.Sprintf("DELETE /wireless-profile/%s", wf.yangFeatureName)] = fpDelete

		// SCOPE: POST /configuration-profile/{name}/scope
		fpScope := &model.FeaturePath{
			FeatureName:       wf.yangFeatureName,
			BlueprintCategory: model.BlueprintCategoryWireless,
			HTTPMethod:        "POST",
			Path:              "/configuration-profile/{name}/scope",
			PathParams:        []model.PathParameter{profileNameParam},
			ProfileType:       model.ProfileTypeConfiguration,
			OperationType:     model.OperationTypeScope,
			SupportsScope:     true,
			SupportedScopeTypes: []model.ScopeType{
				model.ScopeTypeDevice,
				model.ScopeTypeSite,
				model.ScopeTypeSiteGroup,
			},
		}
		p.paths[fmt.Sprintf("POST /wireless-profile/%s/scope", wf.yangFeatureName)] = fpScope

		// DEPLOY: POST /configuration-profile/{name}/sites/deploy
		fpSiteDeploy := &model.FeaturePath{
			FeatureName:        wf.yangFeatureName,
			BlueprintCategory:  model.BlueprintCategoryWireless,
			HTTPMethod:         "POST",
			Path:               "/configuration-profile/{name}/sites/deploy",
			PathParams:         []model.PathParameter{profileNameParam},
			ProfileType:        model.ProfileTypeConfiguration,
			OperationType:      model.OperationTypeDeploy,
			SupportsDeployment: true,
			SupportedTargetTypes: []model.TargetType{
				model.TargetTypeSite,
				model.TargetTypeSiteGroup,
			},
		}
		p.paths[fmt.Sprintf("POST /wireless-profile/%s/sites/deploy", wf.yangFeatureName)] = fpSiteDeploy

		fpDeviceDeploy := &model.FeaturePath{
			FeatureName:          wf.yangFeatureName,
			BlueprintCategory:    model.BlueprintCategoryWireless,
			HTTPMethod:           "POST",
			Path:                 "/configuration-profile/{name}/devices/deploy",
			PathParams:           []model.PathParameter{profileNameParam},
			ProfileType:          model.ProfileTypeConfiguration,
			OperationType:        model.OperationTypeDeploy,
			SupportsDeployment:   true,
			SupportedTargetTypes: []model.TargetType{model.TargetTypeDevice},
		}
		p.paths[fmt.Sprintf("POST /wireless-profile/%s/devices/deploy", wf.yangFeatureName)] = fpDeviceDeploy

		siteNameParam := model.PathParameter{
			Name:        "siteName",
			Type:        "string",
			Description: "Site name",
			Required:    true,
		}
		hostNameParam := model.PathParameter{
			Name:        "hostName",
			Type:        "string",
			Description: "Device host name",
			Required:    true,
		}
		p.paths[fmt.Sprintf("GET /wireless-profile/%s/site/status", wf.yangFeatureName)] = &model.FeaturePath{
			FeatureName:       wf.yangFeatureName,
			BlueprintCategory: model.BlueprintCategoryWireless,
			HTTPMethod:        "GET",
			Path:              "/configuration-profile/{name}/site/{siteName}/deploy/status",
			PathParams:        []model.PathParameter{profileNameParam, siteNameParam},
			ProfileType:       model.ProfileTypeConfiguration,
			OperationType:     model.OperationTypeStatus,
		}
		p.paths[fmt.Sprintf("GET /wireless-profile/%s/device/status", wf.yangFeatureName)] = &model.FeaturePath{
			FeatureName:       wf.yangFeatureName,
			BlueprintCategory: model.BlueprintCategoryWireless,
			HTTPMethod:        "GET",
			Path:              "/configuration-profile/{name}/device/{hostName}/deploy/status",
			PathParams:        []model.PathParameter{profileNameParam, hostNameParam},
			ProfileType:       model.ProfileTypeConfiguration,
			OperationType:     model.OperationTypeStatus,
		}
	}
}

// Helper function
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
