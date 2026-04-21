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

	// Scan ConfigurationProfile/Blueprint schemas for wired/wireless features
	p.extractConfigurationProfileFeatures()

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
		{"ntp-server", "/ntp-server-feature"},
		{"syslog-server", "/syslog-server-feature"},
		{"dhcp-server", "/dhcp-server-feature"},
		{"radius-server", "/radius-server-feature"},
	}

	for _, gf := range globalFeatures {
		// Create FeaturePath for retrieve operation
		// NOTE: The actual endpoint is POST /global-profile/feature/object/retrieve (not GET)
		retrieveKey := fmt.Sprintf("POST /global-profile/feature/object/retrieve")
		if _, exists := p.paths[retrieveKey]; exists {
			// Create a specific feature path for this feature
			fp := &model.FeaturePath{
				FeatureName:       gf.name,                       // Set the YANG feature name for linkage
				BlueprintCategory: model.BlueprintCategoryGlobal, // Mark as global-profile feature
				HTTPMethod:        "POST",
				Path:              "/global-profile/feature/object/retrieve",
				PathParams: []model.PathParameter{
					{
						Name:        "featurePath",
						Type:        "string",
						Description: "Path to the feature",
						Required:    true,
						FixedValue:  gf.featurePath, // Actual featurePath value
					},
				},
				ProfileType:   model.ProfileTypeGlobal,
				OperationType: model.OperationTypeRead,
			}
			p.paths[fmt.Sprintf("READ /global-profile/%s", gf.name)] = fp
		}

		// Create FeaturePath for modify (create/update) operation
		modifyKey := fmt.Sprintf("POST /global-profile/feature/object/modify")
		if _, exists := p.paths[modifyKey]; exists {
			// Create path for CREATE operation
			fpCreate := &model.FeaturePath{
				FeatureName:       gf.name,                       // Set the YANG feature name for linkage
				BlueprintCategory: model.BlueprintCategoryGlobal, // Mark as global-profile feature
				HTTPMethod:        "POST",
				Path:              "/global-profile/feature/object/modify",
				PathParams: []model.PathParameter{
					{
						Name:        "featurePath",
						Type:        "string",
						Description: "Path to the feature",
						Required:    true,
						FixedValue:  gf.featurePath, // Actual featurePath value
					},
				},
				ProfileType:   model.ProfileTypeGlobal,
				OperationType: model.OperationTypeCreate,
			}
			p.paths[fmt.Sprintf("POST /global-profile/%s", gf.name)] = fpCreate

			// Create path for UPDATE operation (same endpoint, different operation type)
			fpUpdate := &model.FeaturePath{
				FeatureName:       gf.name,
				BlueprintCategory: model.BlueprintCategoryGlobal,
				HTTPMethod:        "POST",
				Path:              "/global-profile/feature/object/modify",
				PathParams: []model.PathParameter{
					{
						Name:        "featurePath",
						Type:        "string",
						Description: "Path to the feature",
						Required:    true,
						FixedValue:  gf.featurePath,
					},
				},
				ProfileType:   model.ProfileTypeGlobal,
				OperationType: model.OperationTypeUpdate,
			}
			p.paths[fmt.Sprintf("PUT /global-profile/%s", gf.name)] = fpUpdate
		}

		// Create FeaturePath for delete operation (uses POST method)
		deleteKey := fmt.Sprintf("POST /global-profile/feature/object/delete")
		if _, exists := p.paths[deleteKey]; exists {
			fp := &model.FeaturePath{
				FeatureName:       gf.name,                       // Set the YANG feature name for linkage
				BlueprintCategory: model.BlueprintCategoryGlobal, // Mark as global-profile feature
				HTTPMethod:        "POST",
				Path:              "/global-profile/feature/object/delete",
				PathParams: []model.PathParameter{
					{
						Name:        "featurePath",
						Type:        "string",
						Description: "Path to the feature",
						Required:    true,
						FixedValue:  gf.featurePath, // Actual featurePath value
					},
					{
						Name:        "objectId",
						Type:        "string",
						Description: "ID of the object to delete",
						Required:    true,
					},
				},
				ProfileType:   model.ProfileTypeGlobal,
				OperationType: model.OperationTypeDelete,
			}
			p.paths[fmt.Sprintf("DELETE /global-profile/%s", gf.name)] = fp
		}
	}
}

// extractServiceProfileFeatures extracts features from ServiceProfile schema
// Features from extreme-service-profile-blueprint.yang
func (p *Parser) extractServiceProfileFeatures() {
	serviceFeatures := []struct {
		name        string
		featurePath string
	}{
		{"l2-service-feature", "/l2-service-feature"},     // VLAN configuration (uses extreme-intent-vlan.yang)
		{"vrf-feature", "/vrf-feature"},                   // Virtual Routing and Forwarding
		{"router-group-feature", "/router-group-feature"}, // Router group configuration
		{"global-feature", "/global-feature"},             // Advanced/Global settings
	}

	for _, sf := range serviceFeatures {
		// Service profile features use /service-profile/{name}/feature/object/* paths

		// Retrieve
		fp := &model.FeaturePath{
			FeatureName:       sf.name,                        // Set the YANG feature name for linkage
			BlueprintCategory: model.BlueprintCategoryService, // Mark as service-profile feature
			HTTPMethod:        "GET",
			Path:              "/service-profile/{name}/feature/object/retrieve",
			PathParams: []model.PathParameter{
				{
					Name:        "name",
					Type:        "string",
					Description: "Service profile name",
					Required:    true,
				},
				{
					Name:        "featurePath",
					Type:        "string",
					Description: "Path to the feature",
					Required:    true,
				},
			},
			ProfileType:   model.ProfileTypeService,
			OperationType: model.OperationTypeRead,
		}
		p.paths[fmt.Sprintf("GET /service-profile/%s", sf.name)] = fp

		// Modify
		fp2 := &model.FeaturePath{
			FeatureName:       sf.name,                        // Set the YANG feature name for linkage
			BlueprintCategory: model.BlueprintCategoryService, // Mark as service-profile feature
			HTTPMethod:        "POST",
			Path:              "/service-profile/{name}/feature/object/modify",
			PathParams: []model.PathParameter{
				{
					Name:        "name",
					Type:        "string",
					Description: "Service profile name",
					Required:    true,
				},
				{
					Name:        "featurePath",
					Type:        "string",
					Description: "Path to the feature",
					Required:    true,
				},
			},
			ProfileType:   model.ProfileTypeService,
			OperationType: model.OperationTypeCreate,
		}
		p.paths[fmt.Sprintf("POST /service-profile/%s", sf.name)] = fp2
	}
}

// extractConfigurationProfileFeatures extracts features from ConfigurationProfile/Blueprint schemas
// Features from wired blueprint, wireless blueprint, global blueprint
func (p *Parser) extractConfigurationProfileFeatures() {
	// Wired blueprint features
	wiredFeatures := []struct {
		name        string
		featurePath string
	}{
		{"vlan", "/VLAN"},
		{"port-mapping", "/Port Configuration"},
		{"lag-config", "/LAG Configuration"},
		{"acl", "/ACL"},
		{"qos", "/QoS"},
	}

	// Wireless blueprint features (optional - can be enabled later with --feature-categories)
	// wirelessFeatures := []struct{
	// 	name string
	// 	featurePath string
	// }{
	// 	{"wireless-ssid", "/Wireless/SSID"},
	// 	{"wireless-radio", "/Wireless/Radio"},
	// }

	// Process wired features
	for _, cf := range wiredFeatures {
		// Configuration profile features use /configuration-profile/{name}/feature/object/* paths

		// Retrieve
		fp := &model.FeaturePath{
			FeatureName:       cf.name,                      // Set the YANG feature name for linkage
			BlueprintCategory: model.BlueprintCategoryWired, // Mark as wired-blueprint feature
			HTTPMethod:        "GET",
			Path:              "/configuration-profile/{name}/feature/object/retrieve",
			PathParams: []model.PathParameter{
				{
					Name:        "name",
					Type:        "string",
					Description: "Configuration profile name",
					Required:    true,
				},
				{
					Name:        "featurePath",
					Type:        "string",
					Description: "Path to the feature",
					Required:    true,
				},
			},
			ProfileType:   model.ProfileTypeConfiguration,
			OperationType: model.OperationTypeRead,
		}
		p.paths[fmt.Sprintf("GET /configuration-profile/%s", cf.name)] = fp

		// Modify
		fp2 := &model.FeaturePath{
			FeatureName:       cf.name,                      // Set the YANG feature name for linkage
			BlueprintCategory: model.BlueprintCategoryWired, // Mark as wired-blueprint feature
			HTTPMethod:        "POST",
			Path:              "/configuration-profile/{name}/feature/object/modify",
			PathParams: []model.PathParameter{
				{
					Name:        "name",
					Type:        "string",
					Description: "Configuration profile name",
					Required:    true,
				},
				{
					Name:        "featurePath",
					Type:        "string",
					Description: "Path to the feature",
					Required:    true,
				},
			},
			ProfileType:   model.ProfileTypeConfiguration,
			OperationType: model.OperationTypeCreate,
		}
		p.paths[fmt.Sprintf("POST /configuration-profile/%s", cf.name)] = fp2

		// Deployment paths for configuration features
		// Scope
		fpScope := &model.FeaturePath{
			FeatureName:       cf.name,                      // Set the YANG feature name for linkage
			BlueprintCategory: model.BlueprintCategoryWired, // Mark as wired-blueprint feature
			HTTPMethod:        "PUT",
			Path:              "/configuration-profile/{name}/scope",
			PathParams: []model.PathParameter{
				{
					Name:        "name",
					Type:        "string",
					Description: "Configuration profile name",
					Required:    true,
				},
			},
			ProfileType:   model.ProfileTypeConfiguration,
			OperationType: model.OperationTypeScope,
		}
		p.paths[fmt.Sprintf("PUT /configuration-profile/%s/scope", cf.name)] = fpScope

		// Target
		fpTarget := &model.FeaturePath{
			FeatureName:       cf.name,                      // Set the YANG feature name for linkage
			BlueprintCategory: model.BlueprintCategoryWired, // Mark as wired-blueprint feature
			HTTPMethod:        "PUT",
			Path:              "/configuration-profile/{name}/target",
			PathParams: []model.PathParameter{
				{
					Name:        "name",
					Type:        "string",
					Description: "Configuration profile name",
					Required:    true,
				},
			},
			ProfileType:   model.ProfileTypeConfiguration,
			OperationType: model.OperationTypeTarget,
		}
		p.paths[fmt.Sprintf("PUT /configuration-profile/%s/target", cf.name)] = fpTarget

		// Deploy
		fpDeploy := &model.FeaturePath{
			FeatureName:       cf.name,                      // Set the YANG feature name for linkage
			BlueprintCategory: model.BlueprintCategoryWired, // Mark as wired-blueprint feature
			HTTPMethod:        "POST",
			Path:              "/configuration-profile/{name}/deploy",
			PathParams: []model.PathParameter{
				{
					Name:        "name",
					Type:        "string",
					Description: "Configuration profile name",
					Required:    true,
				},
			},
			ProfileType:   model.ProfileTypeConfiguration,
			OperationType: model.OperationTypeDeploy,
		}
		p.paths[fmt.Sprintf("POST /configuration-profile/%s/deploy", cf.name)] = fpDeploy
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
