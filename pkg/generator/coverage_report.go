package generator

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/extremenetworks/testcase-generator/pkg/model"
)

// CoverageReport represents coverage analysis across all generated tests
type CoverageReport struct {
	// REST API Coverage
	TotalRESTEndpoints     int
	CoveredRESTEndpoints   int
	UncoveredRESTEndpoints []string
	EndpointUsageCount     map[string]int

	// Feature Coverage
	TotalFeatures     int
	CoveredFeatures   int
	UncoveredFeatures []string

	// Path Parameters Coverage
	TotalPathParams   map[string][]string // endpoint -> param names
	CoveredPathParams map[string][]string // endpoint -> covered param names

	// Object Properties Coverage (per feature)
	FeaturePropertyCoverage map[string]*PropertyCoverage

	// Deployment API Coverage
	DeploymentEndpoints map[string]int

	// Test Statistics
	TotalTests      int
	DeploymentTests int
	TestsByFeature  map[string]int
	TestsByCategory map[model.TestCategory]int
}

// PropertyCoverage tracks property coverage for a feature
type PropertyCoverage struct {
	FeatureName         string
	TotalProperties     []string
	CoveredProperties   map[string]int // property -> usage count
	UncoveredProperties []string
}

// GenerateCoverageReport analyzes all generated tests and creates a coverage report
func (g *Generator) GenerateCoverageReport(suite *model.TestSuite) *CoverageReport {
	report := &CoverageReport{
		EndpointUsageCount:      make(map[string]int),
		TotalPathParams:         make(map[string][]string),
		CoveredPathParams:       make(map[string][]string),
		FeaturePropertyCoverage: make(map[string]*PropertyCoverage),
		DeploymentEndpoints:     make(map[string]int),
		TestsByFeature:          make(map[string]int),
		TestsByCategory:         make(map[model.TestCategory]int),
	}

	// Analyze REST API endpoints available
	allEndpoints := make(map[string]bool)
	for _, fp := range g.featurePaths {
		endpoint := fp.Path
		allEndpoints[endpoint] = true

		// Track path parameters defined in the API
		if len(fp.PathParams) > 0 {
			var paramNames []string
			for _, param := range fp.PathParams {
				paramNames = append(paramNames, param.Name)
			}
			report.TotalPathParams[endpoint] = paramNames
		}
	}
	report.TotalRESTEndpoints = len(allEndpoints)

	// First pass: identify which features actually have tests
	testedFeatures := make(map[string]bool)
	for _, featureGroup := range suite.Features {
		testedFeatures[featureGroup.FeatureName] = true
	}

	// Analyze feature properties available - ONLY for tested features
	for featureName, feature := range g.features {
		// Skip features that don't have any generated tests
		if !testedFeatures[featureName] {
			continue
		}

		propCoverage := &PropertyCoverage{
			FeatureName:       featureName,
			TotalProperties:   []string{},
			CoveredProperties: make(map[string]int),
		}

		// Recursively count all properties including nested ones
		for _, param := range feature.Parameters {
			g.addParameterToPropertyList(&propCoverage.TotalProperties, param, "")
		}

		report.FeaturePropertyCoverage[featureName] = propCoverage
	}
	report.TotalFeatures = len(testedFeatures)

	// Analyze test suite coverage
	coveredEndpoints := make(map[string]bool)
	coveredFeatures := make(map[string]bool)
	coveredParams := make(map[string]map[string]bool) // endpoint -> param -> true

	for _, featureGroup := range suite.Features {
		coveredFeatures[featureGroup.FeatureName] = true

		// Analyze each test category
		for category, tests := range featureGroup.Tests {
			report.TestsByCategory[category] += len(tests)
			report.TestsByFeature[featureGroup.FeatureName] += len(tests)
			report.TotalTests += len(tests)

			// Count deployment tests and analyze steps
			for _, tc := range tests {
				if tc.IsDeploymentTest {
					report.DeploymentTests++
				}

				// Analyze each test step
				for _, step := range tc.Steps {
					// Track endpoint usage
					endpoint := step.Path
					report.EndpointUsageCount[endpoint]++
					coveredEndpoints[endpoint] = true

					// Track deployment endpoints specifically
					if strings.Contains(endpoint, "/deploy") || strings.Contains(endpoint, "/clone") {
						report.DeploymentEndpoints[endpoint]++
					}

					// Track path parameter usage
					if len(step.PathParams) > 0 {
						if coveredParams[endpoint] == nil {
							coveredParams[endpoint] = make(map[string]bool)
						}
						for paramName := range step.PathParams {
							coveredParams[endpoint][paramName] = true
						}
					}

					// Track object property usage
					if step.Body != nil {
						g.analyzeBodyProperties(step.Body, featureGroup.FeatureName, report)
					}
				}
			}
		}
	}

	// Calculate coverage metrics
	report.CoveredRESTEndpoints = len(coveredEndpoints)
	report.CoveredFeatures = len(coveredFeatures)

	// Identify uncovered endpoints
	for endpoint := range allEndpoints {
		if !coveredEndpoints[endpoint] {
			report.UncoveredRESTEndpoints = append(report.UncoveredRESTEndpoints, endpoint)
		}
	}
	sort.Strings(report.UncoveredRESTEndpoints)

	// Identify uncovered features
	for featureName := range g.features {
		if !coveredFeatures[featureName] {
			report.UncoveredFeatures = append(report.UncoveredFeatures, featureName)
		}
	}
	sort.Strings(report.UncoveredFeatures)

	// Calculate path parameter coverage
	for endpoint := range report.TotalPathParams {
		var covered []string
		if coveredParams[endpoint] != nil {
			for paramName := range coveredParams[endpoint] {
				covered = append(covered, paramName)
			}
		}
		report.CoveredPathParams[endpoint] = covered
	}

	// Calculate uncovered properties for each feature
	for _, propCov := range report.FeaturePropertyCoverage {
		for _, propName := range propCov.TotalProperties {
			if propCov.CoveredProperties[propName] == 0 {
				propCov.UncoveredProperties = append(propCov.UncoveredProperties, propName)
			}
		}
		sort.Strings(propCov.UncoveredProperties)
	}

	return report
}

// addParameterToPropertyList recursively adds parameter and all nested properties to the list
func (g *Generator) addParameterToPropertyList(propList *[]string, param model.Parameter, prefix string) {
	paramName := param.Name
	if prefix != "" {
		paramName = prefix + "." + param.Name
	}
	*propList = append(*propList, paramName)

	// Recursively add nested properties
	for _, nested := range param.NestedProperties {
		g.addParameterToPropertyList(propList, nested, paramName)
	}
}

// analyzeBodyProperties recursively analyzes request body to track property usage
func (g *Generator) analyzeBodyProperties(body interface{}, featureName string, report *CoverageReport) {
	propCov, exists := report.FeaturePropertyCoverage[featureName]
	if !exists {
		return
	}

	switch v := body.(type) {
	case map[string]interface{}:
		// Check for properties array (common in our test structure)
		if properties, ok := v["properties"].([]map[string]interface{}); ok {
			for _, prop := range properties {
				if propName, ok := prop["name"].(string); ok {
					propCov.CoveredProperties[propName]++
				}
			}
		}

		// Check for objects array
		if objects, ok := v["objects"].([]interface{}); ok {
			for _, obj := range objects {
				g.analyzeBodyProperties(obj, featureName, report)
			}
		}

		// Recursively analyze nested structures
		for key, val := range v {
			// Check if key matches a property name
			if contains(propCov.TotalProperties, key) {
				propCov.CoveredProperties[key]++
			}
			g.analyzeBodyProperties(val, featureName, report)
		}

	case []interface{}:
		for _, item := range v {
			g.analyzeBodyProperties(item, featureName, report)
		}
	}
}

// FormatCoverageReport generates a formatted text report
func (report *CoverageReport) FormatCoverageReport() string {
	var sb strings.Builder

	sb.WriteString("=" + strings.Repeat("=", 78) + "=\n")
	sb.WriteString("                        TEST COVERAGE REPORT\n")
	sb.WriteString("=" + strings.Repeat("=", 78) + "=\n\n")

	// Overall Statistics
	sb.WriteString("OVERALL TEST STATISTICS\n")
	sb.WriteString(strings.Repeat("-", 80) + "\n")
	sb.WriteString(fmt.Sprintf("Total Tests Generated:           %d\n", report.TotalTests))
	sb.WriteString(fmt.Sprintf("Deployment Tests:                %d (%.1f%%)\n",
		report.DeploymentTests, float64(report.DeploymentTests)/float64(report.TotalTests)*100))
	sb.WriteString(fmt.Sprintf("Non-Deployment Tests:            %d (%.1f%%)\n\n",
		report.TotalTests-report.DeploymentTests,
		float64(report.TotalTests-report.DeploymentTests)/float64(report.TotalTests)*100))

	// Tests by Category
	sb.WriteString("Tests by Category:\n")
	for category, count := range report.TestsByCategory {
		sb.WriteString(fmt.Sprintf("  %-20s %d\n", category, count))
	}
	sb.WriteString("\n")

	// Tests by Feature
	sb.WriteString("Tests by Feature:\n")
	features := make([]string, 0, len(report.TestsByFeature))
	for feature := range report.TestsByFeature {
		features = append(features, feature)
	}
	sort.Strings(features)
	for _, feature := range features {
		sb.WriteString(fmt.Sprintf("  %-20s %d tests\n", feature, report.TestsByFeature[feature]))
	}
	sb.WriteString("\n")

	// REST API Endpoint Coverage
	sb.WriteString("=" + strings.Repeat("=", 78) + "=\n")
	sb.WriteString("REST API ENDPOINT COVERAGE\n")
	sb.WriteString(strings.Repeat("-", 80) + "\n")
	sb.WriteString(fmt.Sprintf("Total REST Endpoints:            %d\n", report.TotalRESTEndpoints))
	sb.WriteString(fmt.Sprintf("Covered Endpoints:               %d (%.1f%%)\n",
		report.CoveredRESTEndpoints,
		float64(report.CoveredRESTEndpoints)/float64(report.TotalRESTEndpoints)*100))
	sb.WriteString(fmt.Sprintf("Uncovered Endpoints:             %d (%.1f%%)\n\n",
		len(report.UncoveredRESTEndpoints),
		float64(len(report.UncoveredRESTEndpoints))/float64(report.TotalRESTEndpoints)*100))

	// Most Used Endpoints
	sb.WriteString("Top 20 Most Tested Endpoints:\n")
	type endpointCount struct {
		endpoint string
		count    int
	}
	var sortedEndpoints []endpointCount
	for endpoint, count := range report.EndpointUsageCount {
		sortedEndpoints = append(sortedEndpoints, endpointCount{endpoint, count})
	}
	sort.Slice(sortedEndpoints, func(i, j int) bool {
		return sortedEndpoints[i].count > sortedEndpoints[j].count
	})
	for i, ec := range sortedEndpoints {
		if i >= 20 {
			break
		}
		sb.WriteString(fmt.Sprintf("  %3d tests: %s\n", ec.count, ec.endpoint))
	}
	sb.WriteString("\n")

	// Uncovered Endpoints
	if len(report.UncoveredRESTEndpoints) > 0 {
		sb.WriteString("Uncovered REST Endpoints:\n")
		for _, endpoint := range report.UncoveredRESTEndpoints {
			sb.WriteString(fmt.Sprintf("  ✗ %s\n", endpoint))
		}
		sb.WriteString("\n")
	}

	// Deployment API Coverage
	sb.WriteString("=" + strings.Repeat("=", 78) + "=\n")
	sb.WriteString("DEPLOYMENT & CLONE API COVERAGE\n")
	sb.WriteString(strings.Repeat("-", 80) + "\n")

	deployCategories := map[string][]string{
		"Immediate Deployment": {},
		"Scheduled Deployment": {},
		"Schedule Management":  {},
		"Deployment Status":    {},
		"Clone Operations":     {},
	}

	for endpoint := range report.DeploymentEndpoints {
		if strings.Contains(endpoint, "/deploy") && !strings.Contains(endpoint, "/status") &&
			!strings.Contains(endpoint, "/edit-schedule") && !strings.Contains(endpoint, "/clear-schedule") {
			if strings.Contains(endpoint, "deployAt") || strings.Contains(endpoint, "schedule") {
				deployCategories["Scheduled Deployment"] = append(deployCategories["Scheduled Deployment"], endpoint)
			} else {
				deployCategories["Immediate Deployment"] = append(deployCategories["Immediate Deployment"], endpoint)
			}
		} else if strings.Contains(endpoint, "/edit-schedule") {
			deployCategories["Schedule Management"] = append(deployCategories["Schedule Management"], endpoint)
		} else if strings.Contains(endpoint, "/clear-schedule") {
			deployCategories["Schedule Management"] = append(deployCategories["Schedule Management"], endpoint)
		} else if strings.Contains(endpoint, "/status") {
			deployCategories["Deployment Status"] = append(deployCategories["Deployment Status"], endpoint)
		} else if strings.Contains(endpoint, "/clone") {
			deployCategories["Clone Operations"] = append(deployCategories["Clone Operations"], endpoint)
		}
	}

	for category, endpoints := range deployCategories {
		uniqueEndpoints := uniqueStrings(endpoints)
		if len(uniqueEndpoints) > 0 {
			sb.WriteString(fmt.Sprintf("\n%s:\n", category))
			sort.Strings(uniqueEndpoints)
			for _, endpoint := range uniqueEndpoints {
				count := report.DeploymentEndpoints[endpoint]
				sb.WriteString(fmt.Sprintf("  ✓ %3d tests: %s\n", count, endpoint))
			}
		}
	}
	sb.WriteString("\n")

	// Feature Coverage
	sb.WriteString("=" + strings.Repeat("=", 78) + "=\n")
	sb.WriteString("FEATURE COVERAGE\n")
	sb.WriteString(strings.Repeat("-", 80) + "\n")
	sb.WriteString(fmt.Sprintf("Total Features:                  %d\n", report.TotalFeatures))
	sb.WriteString(fmt.Sprintf("Covered Features:                %d (%.1f%%)\n",
		report.CoveredFeatures,
		float64(report.CoveredFeatures)/float64(report.TotalFeatures)*100))
	sb.WriteString(fmt.Sprintf("Uncovered Features:              %d\n\n", len(report.UncoveredFeatures)))

	if len(report.UncoveredFeatures) > 0 {
		sb.WriteString("Uncovered Features:\n")
		for _, feature := range report.UncoveredFeatures {
			sb.WriteString(fmt.Sprintf("  ✗ %s\n", feature))
		}
		sb.WriteString("\n")
	}

	// Property Coverage per Feature
	sb.WriteString("=" + strings.Repeat("=", 78) + "=\n")
	sb.WriteString("PROPERTY COVERAGE BY FEATURE\n")
	sb.WriteString(strings.Repeat("-", 80) + "\n\n")

	featureNames := make([]string, 0, len(report.FeaturePropertyCoverage))
	for name := range report.FeaturePropertyCoverage {
		featureNames = append(featureNames, name)
	}
	sort.Strings(featureNames)

	for _, featureName := range featureNames {
		propCov := report.FeaturePropertyCoverage[featureName]
		totalProps := len(propCov.TotalProperties)
		coveredProps := len(propCov.CoveredProperties)

		if totalProps == 0 {
			continue
		}

		sb.WriteString(fmt.Sprintf("Feature: %s\n", propCov.FeatureName))
		sb.WriteString(fmt.Sprintf("  Total Properties:    %d\n", totalProps))
		sb.WriteString(fmt.Sprintf("  Covered Properties:  %d (%.1f%%)\n",
			coveredProps, float64(coveredProps)/float64(totalProps)*100))
		sb.WriteString(fmt.Sprintf("  Uncovered:           %d\n", len(propCov.UncoveredProperties)))

		// Show covered properties with usage counts
		if len(propCov.CoveredProperties) > 0 {
			sb.WriteString("  Covered:\n")
			var coveredList []string
			for prop := range propCov.CoveredProperties {
				coveredList = append(coveredList, prop)
			}
			sort.Strings(coveredList)
			for _, prop := range coveredList {
				count := propCov.CoveredProperties[prop]
				sb.WriteString(fmt.Sprintf("    ✓ %-30s (used in %d tests)\n", prop, count))
			}
		}

		// Show uncovered properties
		if len(propCov.UncoveredProperties) > 0 {
			sb.WriteString("  Uncovered:\n")
			for _, prop := range propCov.UncoveredProperties {
				sb.WriteString(fmt.Sprintf("    ✗ %s\n", prop))
			}
		}
		sb.WriteString("\n")
	}

	// Path Parameters Coverage
	sb.WriteString("=" + strings.Repeat("=", 78) + "=\n")
	sb.WriteString("PATH PARAMETER COVERAGE\n")
	sb.WriteString(strings.Repeat("-", 80) + "\n\n")

	totalParams := 0
	coveredParamsCount := 0
	for endpoint, allParams := range report.TotalPathParams {
		totalParams += len(allParams)
		if covered, exists := report.CoveredPathParams[endpoint]; exists {
			coveredParamsCount += len(covered)
		}
	}

	if totalParams > 0 {
		sb.WriteString(fmt.Sprintf("Total Path Parameters:           %d\n", totalParams))
		sb.WriteString(fmt.Sprintf("Covered Parameters:              %d (%.1f%%)\n\n",
			coveredParamsCount, float64(coveredParamsCount)/float64(totalParams)*100))

		// Show parameter coverage by endpoint
		endpointList := make([]string, 0, len(report.TotalPathParams))
		for endpoint := range report.TotalPathParams {
			endpointList = append(endpointList, endpoint)
		}
		sort.Strings(endpointList)

		for _, endpoint := range endpointList {
			allParams := report.TotalPathParams[endpoint]
			covered := report.CoveredPathParams[endpoint]

			sb.WriteString(fmt.Sprintf("Endpoint: %s\n", endpoint))
			sb.WriteString(fmt.Sprintf("  Parameters: %d total, %d covered\n", len(allParams), len(covered)))

			coveredMap := make(map[string]bool)
			for _, param := range covered {
				coveredMap[param] = true
			}

			for _, param := range allParams {
				if coveredMap[param] {
					sb.WriteString(fmt.Sprintf("    ✓ %s\n", param))
				} else {
					sb.WriteString(fmt.Sprintf("    ✗ %s (NOT COVERED)\n", param))
				}
			}
			sb.WriteString("\n")
		}
	}

	sb.WriteString("=" + strings.Repeat("=", 78) + "=\n")
	sb.WriteString("END OF COVERAGE REPORT\n")
	sb.WriteString("=" + strings.Repeat("=", 78) + "=\n")

	return sb.String()
}

// Helper functions
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func uniqueStrings(slice []string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, s := range slice {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	return result
}

// FormatCoverageReportJSON formats the coverage report as JSON
func (report *CoverageReport) FormatCoverageReportJSON() (string, error) {
	type EndpointStats struct {
		Endpoint string `json:"endpoint"`
		Count    int    `json:"count"`
	}

	type FeaturePropertyStats struct {
		FeatureName         string         `json:"featureName"`
		TotalProperties     int            `json:"totalProperties"`
		CoveredProperties   int            `json:"coveredProperties"`
		UncoveredProperties int            `json:"uncoveredProperties"`
		CoveragePercent     float64        `json:"coveragePercent"`
		PropertyUsage       map[string]int `json:"propertyUsage"`
		UncoveredList       []string       `json:"uncoveredList"`
	}

	type JSONReport struct {
		// Overall Statistics
		TotalTests           int            `json:"totalTests"`
		DeploymentTests      int            `json:"deploymentTests"`
		DeploymentPercent    float64        `json:"deploymentPercent"`
		NonDeploymentTests   int            `json:"nonDeploymentTests"`
		NonDeploymentPercent float64        `json:"nonDeploymentPercent"`
		TestsByCategory      map[string]int `json:"testsByCategory"`
		TestsByFeature       map[string]int `json:"testsByFeature"`

		// REST API Coverage
		TotalRESTEndpoints      int             `json:"totalRESTEndpoints"`
		CoveredRESTEndpoints    int             `json:"coveredRESTEndpoints"`
		EndpointCoveragePercent float64         `json:"endpointCoveragePercent"`
		UncoveredRESTEndpoints  []string        `json:"uncoveredRESTEndpoints"`
		TopEndpoints            []EndpointStats `json:"topEndpoints"`
		AllEndpointUsage        map[string]int  `json:"allEndpointUsage"`

		// Deployment API Coverage
		DeploymentEndpoints map[string]int `json:"deploymentEndpoints"`

		// Feature Coverage
		TotalFeatures          int      `json:"totalFeatures"`
		CoveredFeatures        int      `json:"coveredFeatures"`
		FeatureCoveragePercent float64  `json:"featureCoveragePercent"`
		UncoveredFeatures      []string `json:"uncoveredFeatures"`

		// Property Coverage
		PropertyCoverage []FeaturePropertyStats `json:"propertyCoverage"`

		// Path Parameters
		PathParameterCoverage map[string]map[string]interface{} `json:"pathParameterCoverage"`
	}

	// Calculate percentages
	deployPercent := 0.0
	if report.TotalTests > 0 {
		deployPercent = float64(report.DeploymentTests) / float64(report.TotalTests) * 100
	}

	endpointCoveragePercent := 0.0
	if report.TotalRESTEndpoints > 0 {
		endpointCoveragePercent = float64(report.CoveredRESTEndpoints) / float64(report.TotalRESTEndpoints) * 100
	}

	featureCoveragePercent := 0.0
	if report.TotalFeatures > 0 {
		featureCoveragePercent = float64(report.CoveredFeatures) / float64(report.TotalFeatures) * 100
	}

	// Convert category keys to strings
	testsByCategory := make(map[string]int)
	for cat, count := range report.TestsByCategory {
		testsByCategory[string(cat)] = count
	}

	// Sort and get top endpoints
	type endpointCount struct {
		endpoint string
		count    int
	}
	var sortedEndpoints []endpointCount
	for endpoint, count := range report.EndpointUsageCount {
		sortedEndpoints = append(sortedEndpoints, endpointCount{endpoint, count})
	}
	sort.Slice(sortedEndpoints, func(i, j int) bool {
		return sortedEndpoints[i].count > sortedEndpoints[j].count
	})

	topEndpoints := make([]EndpointStats, 0, 20)
	for i, ec := range sortedEndpoints {
		if i >= 20 {
			break
		}
		topEndpoints = append(topEndpoints, EndpointStats{
			Endpoint: ec.endpoint,
			Count:    ec.count,
		})
	}

	// Build property coverage stats
	var propertyCoverage []FeaturePropertyStats
	for featureName, coverage := range report.FeaturePropertyCoverage {
		coveragePercent := 0.0
		if len(coverage.TotalProperties) > 0 {
			coveragePercent = float64(len(coverage.CoveredProperties)) / float64(len(coverage.TotalProperties)) * 100
		}

		propertyCoverage = append(propertyCoverage, FeaturePropertyStats{
			FeatureName:         featureName,
			TotalProperties:     len(coverage.TotalProperties),
			CoveredProperties:   len(coverage.CoveredProperties),
			UncoveredProperties: len(coverage.UncoveredProperties),
			CoveragePercent:     coveragePercent,
			PropertyUsage:       coverage.CoveredProperties,
			UncoveredList:       coverage.UncoveredProperties,
		})
	}

	// Sort property coverage by feature name
	sort.Slice(propertyCoverage, func(i, j int) bool {
		return propertyCoverage[i].FeatureName < propertyCoverage[j].FeatureName
	})

	// Build path parameter coverage
	pathParamCoverage := make(map[string]map[string]interface{})
	for endpoint, params := range report.TotalPathParams {
		covered := report.CoveredPathParams[endpoint]
		uncovered := []string{}
		for _, param := range params {
			if !contains(covered, param) {
				uncovered = append(uncovered, param)
			}
		}

		pathParamCoverage[endpoint] = map[string]interface{}{
			"total":     params,
			"covered":   covered,
			"uncovered": uncovered,
		}
	}

	jsonReport := JSONReport{
		TotalTests:              report.TotalTests,
		DeploymentTests:         report.DeploymentTests,
		DeploymentPercent:       deployPercent,
		NonDeploymentTests:      report.TotalTests - report.DeploymentTests,
		NonDeploymentPercent:    100 - deployPercent,
		TestsByCategory:         testsByCategory,
		TestsByFeature:          report.TestsByFeature,
		TotalRESTEndpoints:      report.TotalRESTEndpoints,
		CoveredRESTEndpoints:    report.CoveredRESTEndpoints,
		EndpointCoveragePercent: endpointCoveragePercent,
		UncoveredRESTEndpoints:  report.UncoveredRESTEndpoints,
		TopEndpoints:            topEndpoints,
		AllEndpointUsage:        report.EndpointUsageCount,
		DeploymentEndpoints:     report.DeploymentEndpoints,
		TotalFeatures:           report.TotalFeatures,
		CoveredFeatures:         report.CoveredFeatures,
		FeatureCoveragePercent:  featureCoveragePercent,
		UncoveredFeatures:       report.UncoveredFeatures,
		PropertyCoverage:        propertyCoverage,
		PathParameterCoverage:   pathParamCoverage,
	}

	jsonBytes, err := json.MarshalIndent(jsonReport, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal JSON: %w", err)
	}

	return string(jsonBytes), nil
}
