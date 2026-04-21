package yamlout

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/extremenetworks/testcase-generator/pkg/model"
	"gopkg.in/yaml.v3"
)

// Writer writes test suites to YAML files
type Writer struct {
	outputDir         string
	oneFilePerFeature bool
}

// NewWriter creates a new YAML writer
func NewWriter(outputDir string, oneFilePerFeature bool) *Writer {
	return &Writer{
		outputDir:         outputDir,
		oneFilePerFeature: oneFilePerFeature,
	}
}

// Write writes the test suite to YAML files
func (w *Writer) Write(suite *model.TestSuite) error {
	// Create output directory if it doesn't exist
	if err := os.MkdirAll(w.outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	if w.oneFilePerFeature {
		return w.writePerFeature(suite)
	}

	return w.writeAll(suite)
}

// writeAll writes all tests to a single file
func (w *Writer) writeAll(suite *model.TestSuite) error {
	filePath := filepath.Join(w.outputDir, "test-suite.yaml")

	data, err := yaml.Marshal(suite)
	if err != nil {
		return fmt.Errorf("failed to marshal test suite: %w", err)
	}

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write file %s: %w", filePath, err)
	}

	fmt.Printf("Generated test suite: %s\n", filePath)
	return nil
}

// writePerFeature writes tests to separate files per feature
func (w *Writer) writePerFeature(suite *model.TestSuite) error {
	for _, feature := range suite.Features {
		fileName := sanitizeFilename(feature.FeatureName) + ".yaml"
		filePath := filepath.Join(w.outputDir, fileName)

		// Create a mini suite for this feature
		featureSuite := &model.TestSuite{
			Version:       suite.Version,
			GeneratedAt:   suite.GeneratedAt,
			SourceYangDir: suite.SourceYangDir,
			SourceRESTAPI: suite.SourceRESTAPI,
			SourceNOSAPI:  suite.SourceNOSAPI,
			Features:      []model.FeatureTestGroup{feature},
		}

		data, err := yaml.Marshal(featureSuite)
		if err != nil {
			return fmt.Errorf("failed to marshal feature %s: %w", feature.FeatureName, err)
		}

		if err := os.WriteFile(filePath, data, 0644); err != nil {
			return fmt.Errorf("failed to write file %s: %w", filePath, err)
		}

		// Count tests
		testCount := 0
		for _, tests := range feature.Tests {
			testCount += len(tests)
		}

		fmt.Printf("Generated %d tests for feature %s: %s\n", testCount, feature.FeatureName, filePath)
	}

	return nil
}

// WriteSummary writes a summary of generated tests
func (w *Writer) WriteSummary(suite *model.TestSuite) error {
	summaryPath := filepath.Join(w.outputDir, "test-summary.txt")

	f, err := os.Create(summaryPath)
	if err != nil {
		return fmt.Errorf("failed to create summary file: %w", err)
	}
	defer f.Close()

	fmt.Fprintf(f, "Test Suite Summary\n")
	fmt.Fprintf(f, "==================\n\n")
	fmt.Fprintf(f, "Generated: %s\n", suite.GeneratedAt)
	fmt.Fprintf(f, "YANG Directory: %s\n", suite.SourceYangDir)
	fmt.Fprintf(f, "REST API Spec: %s\n", suite.SourceRESTAPI)
	fmt.Fprintf(f, "NOSAPI Spec: %s\n\n", suite.SourceNOSAPI)

	totalTests := 0
	categoryCounts := make(map[model.TestCategory]int)
	deploymentTestCount := 0

	for _, feature := range suite.Features {
		fmt.Fprintf(f, "Feature: %s\n", feature.FeatureName)
		fmt.Fprintf(f, "  Path: %s\n", feature.FeaturePath)
		fmt.Fprintf(f, "  Profile Type: %s\n", feature.ProfileType)

		featureTestCount := 0
		featureDeploymentCount := 0
		for category, tests := range feature.Tests {
			count := len(tests)
			featureTestCount += count
			categoryCounts[category] += count

			for _, test := range tests {
				if test.IsDeploymentTest {
					deploymentTestCount++
					featureDeploymentCount++
				}
			}

			if count > 0 {
				fmt.Fprintf(f, "  %s: %d tests\n", category, count)
			}
		}

		totalTests += featureTestCount
		featureNonDeploymentCount := featureTestCount - featureDeploymentCount
		fmt.Fprintf(f, "  Total: %d tests\n", featureTestCount)
		fmt.Fprintf(f, "    Deployment: %d tests (%.1f%%)\n", featureDeploymentCount, float64(featureDeploymentCount)*100/float64(featureTestCount))
		fmt.Fprintf(f, "    Non-Deployment: %d tests (%.1f%%)\n\n", featureNonDeploymentCount, float64(featureNonDeploymentCount)*100/float64(featureTestCount))
	}

	fmt.Fprintf(f, "Overall Summary\n")
	fmt.Fprintf(f, "===============\n")
	fmt.Fprintf(f, "Total Features: %d\n", len(suite.Features))
	fmt.Fprintf(f, "Total Tests: %d\n", totalTests)
	fmt.Fprintf(f, "Deployment Tests: %d\n\n", deploymentTestCount)

	fmt.Fprintf(f, "Tests by Category:\n")
	for category, count := range categoryCounts {
		fmt.Fprintf(f, "  %s: %d\n", category, count)
	}

	fmt.Printf("Generated summary: %s\n", summaryPath)
	return nil
}

// WriteExampleTest writes a detailed example test to a separate file
func (w *Writer) WriteExampleTest(suite *model.TestSuite) error {
	// Find a deployment test as the example
	var exampleTest *model.TestCase
	var exampleFeature *model.FeatureTestGroup

	for i := range suite.Features {
		for _, tests := range suite.Features[i].Tests {
			for j := range tests {
				if tests[j].IsDeploymentTest {
					exampleTest = &tests[j]
					exampleFeature = &suite.Features[i]
					break
				}
			}
			if exampleTest != nil {
				break
			}
		}
		if exampleTest != nil {
			break
		}
	}

	if exampleTest == nil {
		// No deployment test found, just pick the first test
		if len(suite.Features) > 0 {
			for _, tests := range suite.Features[0].Tests {
				if len(tests) > 0 {
					exampleTest = &tests[0]
					exampleFeature = &suite.Features[0]
					break
				}
			}
		}
	}

	if exampleTest == nil {
		return nil // No tests to write
	}

	examplePath := filepath.Join(w.outputDir, "example-test.yaml")

	example := map[string]interface{}{
		"featurePath": exampleFeature.FeaturePath,
		"profileType": exampleFeature.ProfileType,
		"test":        exampleTest,
	}

	data, err := yaml.Marshal(example)
	if err != nil {
		return fmt.Errorf("failed to marshal example test: %w", err)
	}

	if err := os.WriteFile(examplePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write example file: %w", err)
	}

	fmt.Printf("Generated example test: %s\n", examplePath)
	return nil
}

// sanitizeFilename converts a string to a safe filename
func sanitizeFilename(name string) string {
	// Replace common problematic characters
	replacements := map[rune]rune{
		' ':  '-',
		'/':  '-',
		'\\': '-',
		':':  '-',
		'*':  '-',
		'?':  '-',
		'"':  '-',
		'<':  '-',
		'>':  '-',
		'|':  '-',
	}

	result := make([]rune, 0, len(name))
	for _, ch := range name {
		if replacement, ok := replacements[ch]; ok {
			result = append(result, replacement)
		} else {
			result = append(result, ch)
		}
	}

	return string(result)
}

// WriteCoverageReport writes the coverage report to a text file
func (w *Writer) WriteCoverageReport(reportContent string) error {
	reportPath := filepath.Join(w.outputDir, "coverage-report.txt")

	if err := os.WriteFile(reportPath, []byte(reportContent), 0644); err != nil {
		return fmt.Errorf("failed to write coverage report: %w", err)
	}

	fmt.Printf("Generated coverage report: %s\n", reportPath)
	return nil
}

// WriteCoverageReportJSON writes the coverage report as JSON
func (w *Writer) WriteCoverageReportJSON(jsonContent string) error {
	reportPath := filepath.Join(w.outputDir, "coverage-report.json")

	if err := os.WriteFile(reportPath, []byte(jsonContent), 0644); err != nil {
		return fmt.Errorf("failed to write JSON coverage report: %w", err)
	}

	fmt.Printf("Generated JSON coverage report: %s\n", reportPath)
	return nil
}
