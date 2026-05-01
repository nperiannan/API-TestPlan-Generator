package main

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// ── Local YAML parse types ─────────────────────────────────────────

type yamlSuite struct {
	Features []yamlFeature `yaml:"features"`
}

type yamlFeature struct {
	FeatureName string                    `yaml:"featureName"`
	Tests       map[string][]yamlTestCase `yaml:"tests"`
}

type yamlTestCase struct {
	TestCaseID       string `yaml:"testCaseID"`
	Type             string `yaml:"type"`
	Priority         string `yaml:"priority"`
	Automation       string `yaml:"automation"`
	Description      string `yaml:"description"`
	IsDeploymentTest bool   `yaml:"isDeploymentTest"`
}

var columns = []string{
	"TestCaseID", "Type", "Priority", "Automation", "Description", "IsDeploymentTest",
}

var categoryOrder = []string{"functional", "boundary", "negative", "performance", "scale"}

func findYAMLByFeature(feature, testplansDir string) (string, error) {
	var matches []string
	_ = filepath.Walk(testplansDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		base := strings.TrimSuffix(info.Name(), ".yaml")
		if base == feature {
			matches = append(matches, path)
		}
		return nil
	})
	if len(matches) == 0 {
		_ = filepath.Walk(testplansDir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			if strings.Contains(info.Name(), feature) && strings.HasSuffix(info.Name(), ".yaml") {
				matches = append(matches, path)
			}
			return nil
		})
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("no YAML file found for feature %q under %s", feature, testplansDir)
	}
	if len(matches) > 1 {
		fmt.Printf("Multiple matches found for %q:\n", feature)
		for _, m := range matches {
			fmt.Printf("  %s\n", m)
		}
		fmt.Printf("Using: %s\n", matches[0])
	}
	return matches[0], nil
}

// ── Single-file conversion ─────────────────────────────────────────

func convertFile(yamlPath, outputPath string) (int, error) {
	data, err := os.ReadFile(yamlPath)
	if err != nil {
		return 0, fmt.Errorf("reading %s: %w", yamlPath, err)
	}
	var suite yamlSuite
	if err := yaml.Unmarshal(data, &suite); err != nil {
		return 0, fmt.Errorf("parsing %s: %w", yamlPath, err)
	}

	out, err := os.Create(outputPath)
	if err != nil {
		return 0, fmt.Errorf("creating %s: %w", outputPath, err)
	}
	defer out.Close()

	w := csv.NewWriter(out)
	defer w.Flush()
	_ = w.Write(columns)

	count := 0
	for _, feat := range suite.Features {
		for cat, cases := range feat.Tests {
			for _, tc := range cases {
				typ := tc.Type
				if typ == "" {
					typ = cat
				}
				deploy := "false"
				if tc.IsDeploymentTest {
					deploy = "true"
				}
				_ = w.Write([]string{tc.TestCaseID, typ, tc.Priority, tc.Automation, tc.Description, deploy})
				count++
			}
		}
	}
	return count, nil
}

func collectYAMLs(dir string) []string {
	var out []string
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.HasSuffix(info.Name(), ".yaml") {
			out = append(out, path)
		}
		return nil
	})
	return out
}

func cleanOutputFiles(dir string, ext string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() || filepath.Ext(info.Name()) != ext {
			return err
		}
		return os.Remove(path)
	})
}

func main() {
	projectRoot, _ := os.Getwd()
	testplansDir := filepath.Join(projectRoot, "Testplans")
	csvDir := filepath.Join(projectRoot, "TestplansCsv")

	// No args → batch convert every YAML in Testplans/
	if len(os.Args) < 2 {
		yamls := collectYAMLs(testplansDir)
		if len(yamls) == 0 {
			fmt.Fprintf(os.Stderr, "No YAML files found under %s\n", testplansDir)
			os.Exit(1)
		}
		if err := cleanOutputFiles(csvDir, ".csv"); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to clean %s: %v\n", csvDir, err)
			os.Exit(1)
		}
		fmt.Printf("Batch converting %d YAML files → %s\n\n", len(yamls), csvDir)
		totalAll, ok := 0, 0
		for _, y := range yamls {
			base := strings.TrimSuffix(filepath.Base(y), ".yaml") + ".csv"
			out := filepath.Join(csvDir, base)
			n, err := convertFile(y, out)
			if err != nil {
				fmt.Fprintf(os.Stderr, "  ERROR %-38s %v\n", filepath.Base(y), err)
				continue
			}
			fmt.Printf("  %-40s → %-40s (%d tests)\n", filepath.Base(y), base, n)
			totalAll += n
			ok++
		}
		fmt.Printf("\nDone. %d files converted, %d total test cases.\n", ok, totalAll)
		return
	}

	// Single file or feature name
	inputArg := os.Args[1]
	var yamlPath string
	if info, err := os.Stat(inputArg); err == nil && !info.IsDir() {
		yamlPath = inputArg
	} else {
		var err error
		yamlPath, err = findYAMLByFeature(inputArg, testplansDir)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}

	var outputPath string
	if len(os.Args) >= 3 {
		outputPath = os.Args[2]
	} else {
		_ = os.MkdirAll(csvDir, 0o755)
		base := strings.TrimSuffix(filepath.Base(yamlPath), ".yaml") + ".csv"
		outputPath = filepath.Join(csvDir, base)
	}

	n, err := convertFile(yamlPath, outputPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("Wrote %d test cases to %s\n", n, outputPath)
}
