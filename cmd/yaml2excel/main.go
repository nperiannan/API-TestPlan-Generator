package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/xuri/excelize/v2"
	"gopkg.in/yaml.v3"
)

// ── Local YAML parse types (handle string validations) ─────────────

type yamlSuite struct {
	Features []yamlFeature `yaml:"features"`
}

type yamlFeature struct {
	FeatureName string                    `yaml:"featureName"`
	Tests       map[string][]yamlTestCase `yaml:"tests"`
}

type yamlTestCase struct {
	TestCaseID       string     `yaml:"testCaseID"`
	FeatureName      string     `yaml:"featureName"`
	Priority         string     `yaml:"priority"`
	Automation       string     `yaml:"automation"`
	Type             string     `yaml:"type"`
	Description      string     `yaml:"description"`
	IsDeploymentTest bool       `yaml:"isDeploymentTest"`
	Steps            []yamlStep `yaml:"steps"`
}

type yamlStep struct {
	Name           string          `yaml:"name"`
	Description    string          `yaml:"description"`
	Method         string          `yaml:"method"`
	Path           string          `yaml:"path"`
	PathParams     interface{}     `yaml:"pathParams"`
	Body           interface{}     `yaml:"body"`
	ExpectedStatus int             `yaml:"expectedStatus"`
	Validations    []string        `yaml:"validations"`
	Assertions     []yamlAssertion `yaml:"assertions"`
	Timeout        int             `yaml:"timeout"`
}

type yamlAssertion struct {
	Type      string      `yaml:"type"`
	Path      string      `yaml:"path"`
	Expected  interface{} `yaml:"expected"`
	CaptureAs string      `yaml:"captureAs"`
}

var categoryOrder = []string{"functional", "boundary", "negative", "performance", "scale"}

// ── Title compression ──────────────────────────────────────────────

var (
	rePrefixes       = regexp.MustCompile(`(?i)^(?:Boundary|Negative|Performance|Scale)\s+test:\s*`)
	reLongQuoted     = regexp.MustCompile(`(\w[\w-]*)='[^']{20,}'\s*[—–-]\s*(.*)`)
	reCreateToMax    = regexp.MustCompile(`\s*\(create to maximum allowed\)`)
	reCRUD           = regexp.MustCompile(`\s*\(Create -> Read -> Update -> Read -> Delete\)`)
	reOnlyUpdate     = regexp.MustCompile(`\s*\(only update specific fields\)`)
	reTrailingDash   = regexp.MustCompile(`\s*[—–-]\s*$`)
	reMultipleSpaces = regexp.MustCompile(`\s{2,}`)
)

var removePhrases = []string{
	"(independent test)",
	"(above 0.0.0.0/8 reserved range)",
	"(last address before multicast 224/4)",
	"(RFC 3849 documentation prefix)",
	"within documentation prefix",
	"(253 characters per RFC 1035)",
	"(single-character label + TLD)",
}

var replacePairs = [][2]string{
	{"Minimum valid", "Min valid"},
	{"Maximum valid", "Max valid"},
	{"Minimum-length", "Min-length"},
	{"Maximum-length", "Max-length"},
	{"minimum", "min"},
	{"maximum", "max"},
}

func compressTitle(desc string) string {
	t := rePrefixes.ReplaceAllString(desc, "")
	t = reLongQuoted.ReplaceAllString(t, `$1 — $2`)
	for _, p := range removePhrases {
		t = strings.ReplaceAll(t, p, "")
	}
	t = reCreateToMax.ReplaceAllString(t, "")
	t = reCRUD.ReplaceAllString(t, "")
	t = reOnlyUpdate.ReplaceAllString(t, "")
	for _, rp := range replacePairs {
		t = strings.ReplaceAll(t, rp[0], rp[1])
	}
	t = reTrailingDash.ReplaceAllString(t, "")
	t = reMultipleSpaces.ReplaceAllString(t, " ")
	return strings.TrimSpace(t)
}

// ── Step formatting ────────────────────────────────────────────────

func formatBodyAsPayload(body interface{}) string {
	if body == nil {
		return ""
	}
	b, err := json.MarshalIndent(body, "", "  ")
	if err != nil {
		return fmt.Sprintf("%v", body)
	}
	return string(b)
}

func formatStepDescription(steps []yamlStep) string {
	var lines []string
	for i, s := range steps {
		lines = append(lines, fmt.Sprintf("%d) %s", i+1, s.Description))
		lines = append(lines, fmt.Sprintf("   %s {base_url}%s", s.Method, s.Path))
		if s.PathParams != nil {
			lines = append(lines, fmt.Sprintf("   Path Params: %s", formatBodyAsPayload(s.PathParams)))
		}
		if s.Body != nil {
			lines = append(lines, fmt.Sprintf("   Payload: %s", formatBodyAsPayload(s.Body)))
		}
		if s.ExpectedStatus != 0 {
			lines = append(lines, fmt.Sprintf("   Expected HTTP Status: %d", s.ExpectedStatus))
		}
		if s.Timeout != 0 {
			lines = append(lines, fmt.Sprintf("   Timeout: %dms", s.Timeout))
		}
		lines = append(lines, "")
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func formatExpectedResults(steps []yamlStep) string {
	var results []string
	for i, s := range steps {
		if len(s.Validations) == 0 {
			continue
		}
		if len(steps) > 1 {
			results = append(results, fmt.Sprintf("Step %d:", i+1))
		}
		for _, v := range s.Validations {
			results = append(results, fmt.Sprintf("- %s", v))
		}
		for _, a := range s.Assertions {
			if a.CaptureAs != "" && a.Path != "" {
				results = append(results, fmt.Sprintf("- Capture %s from response path %s", a.CaptureAs, a.Path))
			}
		}
		if len(steps) > 1 {
			results = append(results, "")
		}
	}
	return strings.TrimSpace(strings.Join(results, "\n"))
}

// ── Excel generation ───────────────────────────────────────────────

var headers = []string{
	"Test Case ID", "Testcase Title", "Status", "Type", "Description",
	"Precondition", "Test Step Description", "Test Step Expected Result", "Priority",
}

var colWidths = []float64{16, 50, 18, 12, 60, 50, 80, 60, 10}

func colName(col int) string {
	// 0-based col -> Excel column letter
	name, _ := excelize.ColumnNumberToName(col + 1)
	return name
}

func cellRef(col, row int) string {
	return fmt.Sprintf("%s%d", colName(col), row)
}

func createSheet(f *excelize.File, sheetName string, cases []yamlTestCase) int {
	idx, _ := f.NewSheet(sheetName)
	_ = idx

	// Header style
	headerStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Size: 11, Color: "FFFFFF"},
		Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"4472C4"}},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center", WrapText: true},
		Border: []excelize.Border{
			{Type: "left", Style: 1, Color: "000000"},
			{Type: "right", Style: 1, Color: "000000"},
			{Type: "top", Style: 1, Color: "000000"},
			{Type: "bottom", Style: 1, Color: "000000"},
		},
	})

	// Data style
	dataStyle, _ := f.NewStyle(&excelize.Style{
		Alignment: &excelize.Alignment{Vertical: "top", WrapText: true},
		Border: []excelize.Border{
			{Type: "left", Style: 1, Color: "000000"},
			{Type: "right", Style: 1, Color: "000000"},
			{Type: "top", Style: 1, Color: "000000"},
			{Type: "bottom", Style: 1, Color: "000000"},
		},
	})

	// Write headers
	for c, h := range headers {
		cell := cellRef(c, 1)
		_ = f.SetCellValue(sheetName, cell, h)
		_ = f.SetCellStyle(sheetName, cell, cell, headerStyle)
	}

	// Write data
	for i, tc := range cases {
		row := i + 2
		title := compressTitle(tc.Description)
		stepDesc := formatStepDescription(tc.Steps)
		expected := formatExpectedResults(tc.Steps)
		priority := string(tc.Priority)
		if priority == "" {
			priority = "P3"
		}

		vals := []string{
			tc.TestCaseID, title, "To Be Automated", "Manual",
			tc.Description,
			"QA environment available with EP1-NGC Framework integration",
			stepDesc, expected, priority,
		}
		for c, v := range vals {
			cell := cellRef(c, row)
			_ = f.SetCellValue(sheetName, cell, v)
			_ = f.SetCellStyle(sheetName, cell, cell, dataStyle)
		}
	}

	// Column widths
	for c, w := range colWidths {
		cn := colName(c)
		_ = f.SetColWidth(sheetName, cn, cn, w)
	}

	return len(cases)
}

// ── Feature file lookup ────────────────────────────────────────────

func findYAMLByFeature(feature, testplansDir string) (string, error) {
	// Exact match first
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
		// Partial match
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

	f := excelize.NewFile()
	total := 0

	for _, feat := range suite.Features {
		for _, cat := range categoryOrder {
			cases := feat.Tests[cat]
			if len(cases) == 0 {
				continue
			}
			sheetName := strings.ToUpper(cat[:1]) + cat[1:]
			if len(sheetName) > 31 {
				sheetName = sheetName[:31]
			}
			total += createSheet(f, sheetName, cases)
		}
		for cat, cases := range feat.Tests {
			found := false
			for _, o := range categoryOrder {
				if cat == o {
					found = true
					break
				}
			}
			if found || len(cases) == 0 {
				continue
			}
			sheetName := strings.ToUpper(cat[:1]) + cat[1:]
			if len(sheetName) > 31 {
				sheetName = sheetName[:31]
			}
			total += createSheet(f, sheetName, cases)
		}
	}

	f.DeleteSheet("Sheet1")
	if err := f.SaveAs(outputPath); err != nil {
		return 0, fmt.Errorf("saving %s: %w", outputPath, err)
	}
	return total, nil
}

func projectRootDir() string {
	exe, _ := os.Executable()
	root := filepath.Dir(filepath.Dir(exe))
	if _, err := os.Stat(filepath.Join(root, "Testplans")); err != nil {
		root, _ = os.Getwd()
	}
	return root
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
	projectRoot := projectRootDir()
	testplansDir := filepath.Join(projectRoot, "Testplans")
	xlsxDir := filepath.Join(projectRoot, "TestplansXlsx")

	// No args → batch convert every YAML in Testplans/
	if len(os.Args) < 2 {
		yamls := collectYAMLs(testplansDir)
		if len(yamls) == 0 {
			fmt.Fprintf(os.Stderr, "No YAML files found under %s\n", testplansDir)
			os.Exit(1)
		}
		if err := cleanOutputFiles(xlsxDir, ".xlsx"); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to clean %s: %v\n", xlsxDir, err)
			os.Exit(1)
		}
		fmt.Printf("Batch converting %d YAML files → %s\n\n", len(yamls), xlsxDir)
		totalAll, ok := 0, 0
		for _, y := range yamls {
			base := strings.TrimSuffix(filepath.Base(y), ".yaml") + ".xlsx"
			out := filepath.Join(xlsxDir, base)
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
		_ = os.MkdirAll(xlsxDir, 0o755)
		base := strings.TrimSuffix(filepath.Base(yamlPath), ".yaml") + ".xlsx"
		outputPath = filepath.Join(xlsxDir, base)
	}

	fmt.Printf("Reading YAML: %s\n", yamlPath)
	n, err := convertFile(yamlPath, outputPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("\nDone! Generated %s\n", outputPath)
	fmt.Printf("Total test cases: %d\n", n)
}
