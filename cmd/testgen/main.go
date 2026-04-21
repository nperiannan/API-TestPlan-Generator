package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/extremenetworks/testcase-generator/pkg/generator"
	"github.com/extremenetworks/testcase-generator/pkg/model"
	"github.com/extremenetworks/testcase-generator/pkg/spec/nosapi"
	"github.com/extremenetworks/testcase-generator/pkg/spec/rest"
	"github.com/extremenetworks/testcase-generator/pkg/yamlout"
	"github.com/extremenetworks/testcase-generator/pkg/yang"
	"github.com/spf13/cobra"
)

var (
	yangDir                string
	restSpec               string
	nosapiSpec             string
	outDir                 string
	includeCategories      []string
	features               []string
	featureCategories      []string
	scopeTypes             []string
	targetTypes            []string
	deploymentMethods      []string
	scaleFactor            int
	performanceIterations  int
	performanceConcurrency int
	oneFilePerFeature      bool
	testIDPrefix           string
	startingIDNumber       int
)

var rootCmd = &cobra.Command{
	Use:   "testgen",
	Short: "Generate API test definitions from YANG models and API specs",
	Long: `testgen is a tool for auto-generating comprehensive API test definitions
for network management systems. It uses YANG models, REST API specs, and NOSAPI
specs to generate functional, boundary, negative, scale, and performance tests
including deployment scenarios with device verification.`,
	RunE: run,
}

func init() {
	rootCmd.Flags().StringVar(&yangDir, "yang-dir", "", "Path to YANG models directory (required)")
	rootCmd.Flags().StringVar(&restSpec, "rest-spec", "", "Path to REST API OpenAPI spec file (required)")
	rootCmd.Flags().StringVar(&nosapiSpec, "nosapi-spec", "", "Path to NOSAPI OpenAPI spec file (required)")
	rootCmd.Flags().StringVar(&outDir, "out-dir", "./generated-tests", "Output directory for generated tests")

	rootCmd.Flags().StringSliceVar(&includeCategories, "include-categories",
		[]string{"functional", "boundary", "negative", "scale", "performance"},
		"Test categories to include (functional,boundary,negative,scale,performance)")

	rootCmd.Flags().StringSliceVar(&features, "features",
		[]string{},
		"Specific features to generate tests for (dns-server,ntp-server,syslog-server,dhcp-server,vlan,port). If empty, all features are included.")

	rootCmd.Flags().StringSliceVar(&featureCategories, "feature-categories",
		[]string{"global-profile", "wired-blueprint", "service-profile"},
		"Blueprint categories to generate tests for (all,global-profile,wired-blueprint,wireless-blueprint,service-profile)")

	rootCmd.Flags().StringSliceVar(&scopeTypes, "scope-types",
		[]string{"site-group", "device"},
		"Scope types to generate tests for")

	rootCmd.Flags().StringSliceVar(&targetTypes, "target-types",
		[]string{"site-group", "device"},
		"Target types to generate tests for")

	rootCmd.Flags().StringSliceVar(&deploymentMethods, "deployment-methods",
		[]string{"rolling", "immediate"},
		"Deployment methods to generate tests for")

	rootCmd.Flags().IntVar(&scaleFactor, "scale-factor", 100, "Scale factor for scale tests")
	rootCmd.Flags().IntVar(&performanceIterations, "performance-iterations", 10, "Number of iterations for performance tests")
	rootCmd.Flags().IntVar(&performanceConcurrency, "performance-concurrency", 5, "Concurrency level for performance tests")

	rootCmd.Flags().BoolVar(&oneFilePerFeature, "one-file-per-feature", true, "Generate one YAML file per feature")
	rootCmd.Flags().StringVar(&testIDPrefix, "test-id-prefix", "TCXM", "Prefix for test case IDs")
	rootCmd.Flags().IntVar(&startingIDNumber, "starting-id-number", 1000, "Starting number for test case IDs")

	rootCmd.MarkFlagRequired("yang-dir")
	rootCmd.MarkFlagRequired("rest-spec")
	rootCmd.MarkFlagRequired("nosapi-spec")
}

func run(cmd *cobra.Command, args []string) error {
	fmt.Println("=================================================")
	fmt.Println("API Test Generator")
	fmt.Println("=================================================")
	fmt.Println()

	// Build configuration
	config := buildConfig()

	// Parse YANG models
	fmt.Println("Parsing YANG models...")
	yangParser := yang.NewParser(yangDir)
	if err := yangParser.Parse(); err != nil {
		return fmt.Errorf("failed to parse YANG models: %w", err)
	}
	features := yangParser.GetFeatures()
	fmt.Printf("Found %d features from YANG models\n\n", len(features))

	// Parse REST API spec
	fmt.Println("Parsing REST API specification...")
	restParser := rest.NewParser(restSpec)
	if err := restParser.Parse(); err != nil {
		return fmt.Errorf("failed to parse REST API spec: %w", err)
	}
	featurePaths := restParser.GetFeaturePaths()
	fmt.Printf("Found %d API endpoints\n\n", len(featurePaths))

	// Parse NOSAPI spec
	fmt.Println("Parsing NOSAPI specification...")
	nosapiParser := nosapi.NewParser(nosapiSpec)
	if err := nosapiParser.Parse(); err != nil {
		return fmt.Errorf("failed to parse NOSAPI spec: %w", err)
	}
	nosEndpoints := nosapiParser.GetEndpoints()
	fmt.Printf("Found %d NOSAPI endpoints\n\n", len(nosEndpoints))

	// Link sub-object types to parent features
	linkSubObjectTypes(features)

	// Link features with feature paths
	linkFeaturesWithPaths(features, featurePaths)

	// Generate tests
	fmt.Println("Generating test cases...")
	gen := generator.NewGenerator(config, features, featurePaths, nosapiParser, restParser)
	suite, err := gen.Generate()
	if err != nil {
		return fmt.Errorf("failed to generate tests: %w", err)
	}
	fmt.Printf("Generated test suite with %d features\n\n", len(suite.Features))

	// Write YAML output
	fmt.Println("Writing YAML files...")
	writer := yamlout.NewWriter(outDir, oneFilePerFeature)
	if err := writer.Write(suite); err != nil {
		return fmt.Errorf("failed to write YAML output: %w", err)
	}

	// Write summary
	if err := writer.WriteSummary(suite); err != nil {
		return fmt.Errorf("failed to write summary: %w", err)
	}

	// Write example
	if err := writer.WriteExampleTest(suite); err != nil {
		return fmt.Errorf("failed to write example: %w", err)
	}

	// Generate and write coverage report
	fmt.Println("Generating coverage report...")
	coverageReport := gen.GenerateCoverageReport(suite)
	reportContent := coverageReport.FormatCoverageReport()
	if err := writer.WriteCoverageReport(reportContent); err != nil {
		return fmt.Errorf("failed to write coverage report: %w", err)
	}

	// Generate and write JSON coverage report
	fmt.Println("Generating JSON coverage report...")
	jsonContent, err := coverageReport.FormatCoverageReportJSON()
	if err != nil {
		return fmt.Errorf("failed to format JSON coverage report: %w", err)
	}
	if err := writer.WriteCoverageReportJSON(jsonContent); err != nil {
		return fmt.Errorf("failed to write JSON coverage report: %w", err)
	}

	// Generate HTML coverage report
	fmt.Println("Generating HTML coverage report...")
	coverageJSONPath := filepath.Join(outDir, "coverage-report.json")
	if err := writer.WriteHTMLReport(suite, coverageJSONPath); err != nil {
		return fmt.Errorf("failed to write HTML report: %w", err)
	}

	fmt.Println()
	fmt.Println("=================================================")
	fmt.Println("Test generation completed successfully!")
	fmt.Printf("Output directory: %s\n", outDir)
	fmt.Println("=================================================")

	return nil
}

func buildConfig() *model.GeneratorConfig {
	config := model.NewDefaultConfig()

	config.YangDir = yangDir
	config.RESTSpecPath = restSpec
	config.NOSAPISpecPath = nosapiSpec
	config.OutputDir = outDir

	// Parse categories
	config.IncludeCategories = parseCategories(includeCategories)

	// Parse feature filter
	config.Features = features

	// Parse feature categories
	config.FeatureCategories = parseFeatureCategories(featureCategories)

	// Parse scope types
	config.ScopeTypes = parseScopeTypes(scopeTypes)

	// Parse target types
	config.TargetTypes = parseTargetTypes(targetTypes)

	// Parse deployment methods
	config.DeploymentMethods = parseDeploymentMethods(deploymentMethods)

	config.ScaleFactor = scaleFactor
	config.PerformanceIterations = performanceIterations
	config.PerformanceConcurrency = performanceConcurrency
	config.OneFilePerFeature = oneFilePerFeature
	config.TestIDPrefix = testIDPrefix
	config.StartingIDNumber = startingIDNumber

	return config
}

func parseCategories(categories []string) []model.TestCategory {
	var result []model.TestCategory
	for _, cat := range categories {
		switch strings.ToLower(cat) {
		case "functional":
			result = append(result, model.TestCategoryFunctional)
		case "boundary":
			result = append(result, model.TestCategoryBoundary)
		case "negative":
			result = append(result, model.TestCategoryNegative)
		case "scale":
			result = append(result, model.TestCategoryScale)
		case "performance":
			result = append(result, model.TestCategoryPerformance)
		}
	}
	return result
}

func parseFeatureCategories(categories []string) []model.BlueprintCategory {
	var result []model.BlueprintCategory
	for _, cat := range categories {
		switch strings.ToLower(cat) {
		case "all":
			// Include all categories
			return []model.BlueprintCategory{
				model.BlueprintCategoryGlobal,
				model.BlueprintCategoryWired,
				model.BlueprintCategoryWireless,
				model.BlueprintCategoryService,
			}
		case "global-profile":
			result = append(result, model.BlueprintCategoryGlobal)
		case "wired-blueprint":
			result = append(result, model.BlueprintCategoryWired)
		case "wireless-blueprint":
			result = append(result, model.BlueprintCategoryWireless)
		case "service-profile":
			result = append(result, model.BlueprintCategoryService)
		}
	}
	return result
}

func parseScopeTypes(types []string) []model.ScopeType {
	var result []model.ScopeType
	for _, t := range types {
		switch strings.ToLower(t) {
		case "site-group":
			result = append(result, model.ScopeTypeSiteGroup)
		case "device":
			result = append(result, model.ScopeTypeDevice)
		}
	}
	return result
}

func parseTargetTypes(types []string) []model.TargetType {
	var result []model.TargetType
	for _, t := range types {
		switch strings.ToLower(t) {
		case "site-group":
			result = append(result, model.TargetTypeSiteGroup)
		case "device":
			result = append(result, model.TargetTypeDevice)
		}
	}
	return result
}

func parseDeploymentMethods(methods []string) []model.DeploymentMethod {
	var result []model.DeploymentMethod
	for _, m := range methods {
		switch strings.ToLower(m) {
		case "rolling":
			result = append(result, model.DeploymentMethodRolling)
		case "immediate":
			result = append(result, model.DeploymentMethodImmediate)
		case "staged":
			result = append(result, model.DeploymentMethodStaged)
		}
	}
	return result
}

func linkFeaturesWithPaths(features map[string]*model.Feature, paths map[string]*model.FeaturePath) {
	for _, fp := range paths {
		// First, try exact match by FeatureName (for deep-scanned features)
		if fp.FeatureName != "" {
			if feature, exists := features[fp.FeatureName]; exists {
				fp.Feature = feature
				continue
			}
		}

		// Try to find matching feature with multiple strategies
		var bestMatch *model.Feature
		bestScore := 0

		for _, feature := range features {
			score := calculateMatchScore(fp, feature)
			if score > bestScore {
				bestScore = score
				bestMatch = feature
			}
		}

		if bestMatch != nil && bestScore > 0 {
			fp.Feature = bestMatch
		}
	}
}

// calculateMatchScore scores how well a feature matches an endpoint
func calculateMatchScore(fp *model.FeaturePath, feature *model.Feature) int {
	score := 0
	pathLower := strings.ToLower(fp.Path)
	featureNameLower := strings.ToLower(feature.Name)

	// Exact match in path
	if strings.Contains(pathLower, featureNameLower) {
		score += 100
	}

	// Plural/singular variations
	if strings.Contains(pathLower, featureNameLower+"s") {
		score += 80
	}
	if strings.HasSuffix(featureNameLower, "s") && strings.Contains(pathLower, strings.TrimSuffix(featureNameLower, "s")) {
		score += 80
	}

	// Word parts match (e.g., "port-profile" matches "port")
	pathParts := strings.FieldsFunc(pathLower, func(r rune) bool {
		return r == '/' || r == '-' || r == '_'
	})
	for _, part := range pathParts {
		if part == featureNameLower {
			score += 90
			break
		}
		if len(featureNameLower) > 3 && strings.Contains(part, featureNameLower) {
			score += 50
		}
		if len(part) > 3 && strings.Contains(featureNameLower, part) {
			score += 40
		}
	}

	// Match based on operation type and feature name
	if fp.OperationType == model.OperationTypeList && strings.Contains(pathLower, featureNameLower) {
		score += 30
	}

	// Penalty for very generic names (avoid false positives)
	if len(featureNameLower) < 4 {
		score = score / 2
	}

	return score
}

// linkSubObjectTypes links sub-object types (like dns-suffix) to their parent features (like dns-server)
func linkSubObjectTypes(features map[string]*model.Feature) {
	// Define sub-object type mappings based on qaopenapi.yaml
	subObjectMappings := map[string]map[string][]model.Parameter{
		"dns-server": {
			"dns-suffix": {
				{
					Name:        "fqdn",
					YangType:    "string",
					GoType:      "string",
					Description: "Fully qualified domain name (FQDN) for DNS suffix",
					Required:    true,
					Constraints: []model.Constraint{
						{Type: model.ConstraintTypeMinLength, Value: 1},
						{Type: model.ConstraintTypeMaxLength, Value: 255},
						{Type: model.ConstraintTypePattern, Value: `([a-zA-Z0-9]([-a-zA-Z0-9]*[a-zA-Z0-9])?\.)*([A-Za-z]{2,})`},
						{Type: model.ConstraintTypeRequired, Value: true},
					},
				},
			},
		},
		// SNMP sub-object types under infrastructure feature
		"snmp": {
			"snmp-v3-user": {
				{
					Name:        "username",
					YangType:    "string",
					GoType:      "string",
					Description: "SNMPv3 username",
					Required:    true,
				},
				{
					Name:        "authType",
					YangType:    "string",
					GoType:      "string",
					Description: "Authentication type (SHA, MD5)",
					Required:    false,
				},
				{
					Name:        "authPassword",
					YangType:    "string",
					GoType:      "string",
					Description: "Authentication password",
					Required:    false,
				},
				{
					Name:        "privType",
					YangType:    "string",
					GoType:      "string",
					Description: "Privacy type (AES, DES)",
					Required:    false,
				},
				{
					Name:        "privPassword",
					YangType:    "string",
					GoType:      "string",
					Description: "Privacy password",
					Required:    false,
				},
			},
			"snmp-trap": {
				{
					Name:        "enabled",
					YangType:    "boolean",
					GoType:      "bool",
					Description: "Enable SNMP trap",
					Required:    false,
				},
				{
					Name:        "trapServer",
					YangType:    "string",
					GoType:      "string",
					Description: "SNMP trap server IP address",
					Required:    true,
				},
				{
					Name:         "trapPort",
					YangType:     "uint16",
					GoType:       "int",
					Description:  "SNMP trap port (default 162)",
					Required:     false,
					DefaultValue: 162,
				},
				{
					Name:        "communityString",
					YangType:    "string",
					GoType:      "string",
					Description: "SNMP community string",
					Required:    false,
				},
			},
		},
		// WLAN sub-object type
		"wlan": {
			"wlan-security": {
				{
					Name:        "securityType",
					YangType:    "string",
					GoType:      "string",
					Description: "Security type (WPA2, WPA3, Open)",
					Required:    true,
				},
				{
					Name:        "encryption",
					YangType:    "string",
					GoType:      "string",
					Description: "Encryption type (AES, TKIP)",
					Required:    false,
				},
				{
					Name:        "passphrase",
					YangType:    "string",
					GoType:      "string",
					Description: "Security passphrase",
					Required:    false,
				},
			},
		},
	}

	// Apply sub-object types to features
	for parentName, subObjects := range subObjectMappings {
		if parentFeature, exists := features[parentName]; exists {
			if parentFeature.SubObjectTypes == nil {
				parentFeature.SubObjectTypes = make(map[string]*model.SubObjectType)
			}
			for subObjName, params := range subObjects {
				parentFeature.SubObjectTypes[subObjName] = &model.SubObjectType{
					Name:        subObjName,
					Parameters:  params,
					Description: fmt.Sprintf("Sub-object type for %s", subObjName),
				}
			}
		}
	}
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
