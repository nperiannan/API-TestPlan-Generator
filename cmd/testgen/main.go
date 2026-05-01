package main

import (
	"fmt"
	"os"
	"sort"
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
	rootCmd.Flags().StringVar(&outDir, "out-dir", "./Testplans", "Output directory for generated tests")

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

	// Write example
	if err := writer.WriteExampleTest(suite); err != nil {
		return fmt.Errorf("failed to write example: %w", err)
	}

	// Generate timestamped summary HTML
	fmt.Println("Generating summary report...")
	if err := writer.WriteSummaryHTML(suite); err != nil {
		return fmt.Errorf("failed to write summary HTML: %w", err)
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
	// Sort path keys so fp.Feature assignments are made in a deterministic order.
	pathKeys := make([]string, 0, len(paths))
	for k := range paths {
		pathKeys = append(pathKeys, k)
	}
	sort.Strings(pathKeys)

	// Pre-sort feature names for the fuzzy-match loop so ties are broken
	// consistently: prefer the longest (most specific) name; if equal length,
	// prefer lexicographically smaller name.
	featureNames := make([]string, 0, len(features))
	for k := range features {
		featureNames = append(featureNames, k)
	}
	sort.Slice(featureNames, func(i, j int) bool {
		a, b := featureNames[i], featureNames[j]
		if len(a) != len(b) {
			return len(a) > len(b) // longer (more specific) name first
		}
		return a < b
	})

	for _, key := range pathKeys {
		fp := paths[key]
		// First, try exact match by FeatureName (for deep-scanned features)
		if fp.FeatureName != "" {
			if feature, exists := features[fp.FeatureName]; exists {
				fp.Feature = feature
				continue
			}
			// Feature not found in YANG parser output (may have no parseable parameters,
			// or be a sub-list inside a grouping). Try a prefix match to find a parent
			// feature whose parameters can seed the synthetic feature.
			synthetic := &model.Feature{
				Name:       fp.FeatureName,
				Parameters: []model.Parameter{},
			}
			// Try to inherit parameters from a parent/related YANG feature.
			// e.g. "fabric-auto-sense-isis" might borrow from "fabric-auto-sense".
			for _, fname := range featureNames {
				if strings.HasPrefix(fp.FeatureName, fname+"-") || strings.HasPrefix(fname, fp.FeatureName+"-") {
					if existing := features[fname]; existing != nil && len(existing.Parameters) > 0 {
						synthetic.Parameters = existing.Parameters
						synthetic.Keys = existing.Keys
						break
					}
				}
			}
			features[fp.FeatureName] = synthetic
			fp.Feature = synthetic
			continue
		}

		// Try to find matching feature with multiple strategies.
		// Iterate in pre-sorted (longest-name-first) order so that when two
		// features tie on score the more specific (longer) name always wins,
		// making the result deterministic.
		var bestMatch *model.Feature
		bestScore := 0

		for _, name := range featureNames {
			feature := features[name]
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

// calculateMatchScore scores how well a feature matches an endpoint.
// Higher scores indicate a stronger match. The algorithm rewards exact,
// full-segment matches and penalises partial substring hits to prevent
// short names like "port" from stealing paths intended for "port-poe".
func calculateMatchScore(fp *model.FeaturePath, feature *model.Feature) int {
	score := 0
	pathLower := strings.ToLower(fp.Path)
	featureNameLower := strings.ToLower(feature.Name)

	// Split the path into its constituent segments for segment-level matching.
	pathParts := strings.FieldsFunc(pathLower, func(r rune) bool {
		return r == '/' || r == '{' || r == '}'
	})

	// 1. Exact whole-segment match (strongest signal).
	//    e.g. feature "port-poe" matches segment "port-poe"
	exactSegmentMatch := false
	for _, part := range pathParts {
		if part == featureNameLower {
			exactSegmentMatch = true
			score += 200
			break
		}
	}

	// 2. Feature name appears in the path as a substring (weaker).
	if !exactSegmentMatch && strings.Contains(pathLower, featureNameLower) {
		score += 80
	}

	// 3. Plural/singular variations (only if no exact segment match).
	if !exactSegmentMatch {
		if strings.Contains(pathLower, featureNameLower+"s") {
			score += 60
		}
		if strings.HasSuffix(featureNameLower, "s") && strings.Contains(pathLower, strings.TrimSuffix(featureNameLower, "s")) {
			score += 60
		}
	}

	// 4. Composite-name segment match: does any path segment START with the
	//    feature name followed by a separator? e.g. "port" matches "port-poe"
	//    but this is weak — we only give partial credit.
	if !exactSegmentMatch {
		for _, part := range pathParts {
			if strings.HasPrefix(part, featureNameLower+"-") {
				score += 20
				break
			}
		}
	}

	// 5. Match based on operation type (small bonus)
	if fp.OperationType == model.OperationTypeList && strings.Contains(pathLower, featureNameLower) {
		score += 10
	}

	// 6. Penalty for very generic / short names to prevent false positives.
	if len(featureNameLower) < 4 {
		score = score / 3
	} else if len(featureNameLower) < 6 {
		score = score * 2 / 3
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
