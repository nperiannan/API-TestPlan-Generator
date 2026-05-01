package web

import (
	"bufio"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/xuri/excelize/v2"
	"gopkg.in/yaml.v3"
)

// handleDashboard serves the main dashboard page
func (s *Server) handleDashboard(c *gin.Context) {
	c.HTML(http.StatusOK, "index.html", nil)
}

// handleListVersions returns all stored versions
func (s *Server) handleListVersions(c *gin.Context) {
	versions, err := s.store.ListVersions()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, versions)
}

// handleCreateVersion stores the current test plans as a new version
func (s *Server) handleCreateVersion(c *gin.Context) {
	var req struct {
		Tag         string `json:"tag" binding:"required"`
		Description string `json:"description"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Read all test plan files from disk
	files, err := readTestPlanFiles(s.config.TestPlansDir)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to read test plans: %v", err)})
		return
	}

	// Also include the summary report if it exists
	reportPath := filepath.Join(s.config.ReportsDir, "summary_report.html")
	if data, err := os.ReadFile(reportPath); err == nil {
		files["summary_report.html"] = data
	}

	// Build generation context: extract from YAML headers + compute source fingerprints
	genCtx := extractGenerationContextFromFiles(files)
	genCtx.ToolCommit = getGitCommit()
	genCtx.SourceFingerprints = computeSourceFingerprints(s.config.SourcesDir)

	version, err := s.version.CreateVersion(req.Tag, req.Description, files, genCtx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, version)
}

// handleGetVersion returns a specific version's details with file listing
func (s *Server) handleGetVersion(c *gin.Context) {
	id := c.Param("id")
	version, err := s.store.GetVersion(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	planFiles, err := s.store.ListTestPlanFiles(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"version": version,
		"files":   planFiles,
	})
}

// handleDiffVersions compares two versions
func (s *Server) handleDiffVersions(c *gin.Context) {
	oldID := c.Param("id")
	newID := c.Param("otherId")

	diff, err := s.version.DiffVersions(oldID, newID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, diff)
}

// handleListTestPlans lists all current test plan files from disk
func (s *Server) handleListTestPlans(c *gin.Context) {
	versionID := c.Query("version")

	if versionID != "" {
		// List from stored version
		files, err := s.store.ListTestPlanFiles(versionID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, files)
		return
	}

	// List from disk
	files, err := listTestPlanFilesFromDisk(s.config.TestPlansDir)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, files)
}

// handleGetTestPlan returns a specific test plan's content
func (s *Server) handleGetTestPlan(c *gin.Context) {
	category := c.Param("category")
	feature := c.Param("feature")
	versionID := c.Query("version")

	var data []byte
	var err error

	if versionID != "" {
		filePath := fmt.Sprintf("%s/%s.yaml", category, feature)
		data, err = s.store.GetFile(versionID, filePath)
	} else {
		path := filepath.Join(s.config.TestPlansDir, category, feature+".yaml")
		data, err = os.ReadFile(path)
	}

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("test plan not found: %v", err)})
		return
	}

	// Parse YAML into structured form
	var parsed interface{}
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		c.Data(http.StatusOK, "application/x-yaml", data)
		return
	}

	format := c.Query("format")
	switch format {
	case "yaml":
		c.Data(http.StatusOK, "application/x-yaml", data)
	case "json":
		c.JSON(http.StatusOK, parsed)
	default:
		c.JSON(http.StatusOK, parsed)
	}
}

// handleDownloadTestPlan sends a test plan file for download
func (s *Server) handleDownloadTestPlan(c *gin.Context) {
	category := c.Param("category")
	feature := c.Param("feature")
	format := c.DefaultQuery("format", "yaml")
	versionID := c.Query("version")

	var data []byte
	var err error

	if versionID != "" {
		filePath := fmt.Sprintf("%s/%s.yaml", category, feature)
		data, err = s.store.GetFile(versionID, filePath)
	} else {
		path := filepath.Join(s.config.TestPlansDir, category, feature+".yaml")
		data, err = os.ReadFile(path)
	}

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "test plan not found"})
		return
	}

	filename := fmt.Sprintf("%s-%s", category, feature)

	switch format {
	case "yaml":
		c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s.yaml", filename))
		c.Data(http.StatusOK, "application/x-yaml", data)
	case "json":
		var parsed interface{}
		if err := yaml.Unmarshal(data, &parsed); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to parse YAML"})
			return
		}
		jsonData, err := json.MarshalIndent(parsed, "", "  ")
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to convert to JSON"})
			return
		}
		c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s.json", filename))
		c.Data(http.StatusOK, "application/json", jsonData)
	case "csv":
		csvData := yamlToCSV(data)
		c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s.csv", filename))
		c.Data(http.StatusOK, "text/csv", []byte(csvData))
	case "xlsx":
		xlsxData, err := yamlToExcel(data)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to convert to Excel: " + err.Error()})
			return
		}
		c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s.xlsx", filename))
		c.Data(http.StatusOK, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", xlsxData)
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported format, use yaml/json/csv/xlsx"})
	}
}

// handleGetSummary returns the current test plan summary statistics
func (s *Server) handleGetSummary(c *gin.Context) {
	summary, err := buildSummaryFromDisk(s.config.TestPlansDir)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, summary)
}

// handleGetSummaryReport returns the HTML summary report
func (s *Server) handleGetSummaryReport(c *gin.Context) {
	reportPath := filepath.Join(s.config.ReportsDir, "summary_report.html")
	data, err := os.ReadFile(reportPath)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "summary report not found"})
		return
	}
	c.Data(http.StatusOK, "text/html", data)
}

// handleSourceChanges detects changes in source YANG/API spec files
func (s *Server) handleSourceChanges(c *gin.Context) {
	changes, err := detectSourceChanges(s.config.SourcesDir)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, changes)
}

// --- Generation state ---

type GenerationStatus struct {
	mu       sync.Mutex
	Running  bool      `json:"running"`
	Status   string    `json:"status"`
	Started  time.Time `json:"startedAt,omitempty"`
	Finished time.Time `json:"finishedAt,omitempty"`
	Output   []string  `json:"output,omitempty"`
	Error    string    `json:"error,omitempty"`
}

var genStatus = &GenerationStatus{}

// handleGenerate triggers test plan generation by running testgen binary
func (s *Server) handleGenerate(c *gin.Context) {
	genStatus.mu.Lock()
	if genStatus.Running {
		genStatus.mu.Unlock()
		c.JSON(http.StatusConflict, gin.H{"error": "generation already in progress", "status": genStatus.Status})
		return
	}
	genStatus.Running = true
	genStatus.Status = "starting"
	genStatus.Started = time.Now()
	genStatus.Finished = time.Time{}
	genStatus.Output = nil
	genStatus.Error = ""
	genStatus.mu.Unlock()

	// Resolve source paths from config
	cfg := s.resolveSourceConfig()

	// Find testgen binary
	testgenBin := s.findTestgenBinary()
	if testgenBin == "" {
		genStatus.mu.Lock()
		genStatus.Running = false
		genStatus.Status = "error"
		genStatus.Error = "testgen binary not found"
		genStatus.Finished = time.Now()
		genStatus.mu.Unlock()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "testgen binary not found"})
		return
	}

	// Return immediately, run generation in background
	c.JSON(http.StatusAccepted, gin.H{
		"status":  "generation started",
		"message": "Test plan generation is running. Check /api/generate/status for progress.",
	})

	go s.runGeneration(testgenBin, cfg)
}

// handleGenerateStatus returns the current generation status
func (s *Server) handleGenerateStatus(c *gin.Context) {
	genStatus.mu.Lock()
	defer genStatus.mu.Unlock()
	c.JSON(http.StatusOK, genStatus)
}

// handleGetConfig returns the current source configuration
func (s *Server) handleGetConfig(c *gin.Context) {
	cfg := s.resolveSourceConfig()
	c.JSON(http.StatusOK, cfg)
}

type sourceConfig struct {
	YangDir   string `json:"yangDir"`
	RestSpec  string `json:"restSpec"`
	NosapiSpec string `json:"nosapiSpec"`
	OutDir    string `json:"outDir"`
	YangExists   bool `json:"yangExists"`
	RestExists   bool `json:"restExists"`
	NosapiExists bool `json:"nosapiExists"`
}

func (s *Server) resolveSourceConfig() sourceConfig {
	// Read config/config.yaml
	configPath := filepath.Join("config", "config.yaml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return sourceConfig{OutDir: s.config.TestPlansDir}
	}

	type configSources struct {
		SourcesDir string `yaml:"sourcesDir"`
		Sources    map[string]struct {
			Sparse    string `yaml:"sparse"`
			LocalDir  string `yaml:"localDir"`
			LocalFile string `yaml:"localFile"`
		} `yaml:"sources"`
	}
	var cfg configSources
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return sourceConfig{OutDir: s.config.TestPlansDir}
	}
	if cfg.SourcesDir == "" {
		cfg.SourcesDir = "./sources"
	}

	sc := sourceConfig{OutDir: s.config.TestPlansDir}

	// YANG dir
	if yang, ok := cfg.Sources["yang"]; ok {
		sc.YangDir = filepath.Join(cfg.SourcesDir, yang.LocalDir, yang.Sparse)
	}
	// REST spec
	if rest, ok := cfg.Sources["restSpec"]; ok {
		sc.RestSpec = filepath.Join(cfg.SourcesDir, rest.LocalDir, rest.Sparse, rest.LocalFile)
	}
	// NOSAPI spec
	if nos, ok := cfg.Sources["nosapiSpec"]; ok {
		sc.NosapiSpec = filepath.Join(cfg.SourcesDir, nos.LocalDir, nos.LocalFile)
	}

	sc.YangExists = dirExists(sc.YangDir)
	sc.RestExists = fileExists(sc.RestSpec)
	sc.NosapiExists = fileExists(sc.NosapiSpec)

	return sc
}

func (s *Server) findTestgenBinary() string {
	// Try several locations
	candidates := []string{
		"./testgen",
		"./bin/linux/testgen",
		"./bin/windows/testgen.exe",
		filepath.Join(filepath.Dir(os.Args[0]), "testgen"),
	}
	for _, c := range candidates {
		if fileExists(c) {
			return c
		}
	}
	return ""
}

func (s *Server) runGeneration(testgenBin string, cfg sourceConfig) {
	genStatus.mu.Lock()
	genStatus.Status = "running"
	genStatus.mu.Unlock()

	args := []string{
		"--yang-dir", cfg.YangDir,
		"--rest-spec", cfg.RestSpec,
		"--nosapi-spec", cfg.NosapiSpec,
		"--out-dir", cfg.OutDir,
		"--feature-categories", "global-profile,wired-blueprint,service-profile",
		"--include-categories", "functional,boundary,negative,scale,performance",
		"--scope-types", "site-group,device",
		"--target-types", "site-group,device",
		"--deployment-methods", "rolling,immediate",
		"--scale-factor", "100",
		"--performance-iterations", "10",
		"--one-file-per-feature", "true",
		"--features", "",
	}

	cmd := exec.Command(testgenBin, args...)
	cmd.Dir = filepath.Dir(testgenBin)
	if filepath.IsAbs(testgenBin) || strings.HasPrefix(testgenBin, ".") {
		cmd.Dir = "."
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		genStatus.mu.Lock()
		genStatus.Running = false
		genStatus.Status = "error"
		genStatus.Error = err.Error()
		genStatus.Finished = time.Now()
		genStatus.mu.Unlock()
		return
	}
	cmd.Stderr = cmd.Stdout

	if err := cmd.Start(); err != nil {
		genStatus.mu.Lock()
		genStatus.Running = false
		genStatus.Status = "error"
		genStatus.Error = err.Error()
		genStatus.Finished = time.Now()
		genStatus.mu.Unlock()
		return
	}

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		genStatus.mu.Lock()
		genStatus.Output = append(genStatus.Output, line)
		// Keep only last 200 lines
		if len(genStatus.Output) > 200 {
			genStatus.Output = genStatus.Output[len(genStatus.Output)-200:]
		}
		genStatus.mu.Unlock()
	}

	err = cmd.Wait()
	genStatus.mu.Lock()
	genStatus.Running = false
	genStatus.Finished = time.Now()
	if err != nil {
		genStatus.Status = "error"
		genStatus.Error = err.Error()
	} else {
		genStatus.Status = "completed"
	}
	genStatus.mu.Unlock()
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// --- Helper functions ---

func readTestPlanFiles(dir string) (map[string][]byte, error) {
	files := make(map[string][]byte)
	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if filepath.Ext(entry.Name()) != ".yaml" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		relPath, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(relPath)] = data
		return nil
	})
	return files, err
}

func listTestPlanFilesFromDisk(dir string) ([]TestPlanFile, error) {
	var result []TestPlanFile
	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		if filepath.Ext(entry.Name()) != ".yaml" {
			return nil
		}
		relPath, _ := filepath.Rel(dir, path)
		relPath = filepath.ToSlash(relPath)
		info, _ := entry.Info()
		size := 0
		if info != nil {
			size = int(info.Size())
		}

		parts := strings.SplitN(relPath, "/", 2)
		category := ""
		feature := strings.TrimSuffix(relPath, ".yaml")
		if len(parts) == 2 {
			category = parts[0]
			feature = strings.TrimSuffix(parts[1], ".yaml")
		}

		result = append(result, TestPlanFile{
			Category: category,
			Feature:  feature,
			FileName: entry.Name(),
			Path:     relPath,
			Size:     size,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// TestPlanSummary represents aggregate statistics
type TestPlanSummary struct {
	TotalFeatures   int                      `json:"totalFeatures"`
	TotalTests      int                      `json:"totalTests"`
	TestsByCategory map[string]int           `json:"testsByCategory"`
	FeaturesByGroup map[string][]FeatureInfo `json:"featuresByGroup"`
	GeneratedAt     string                   `json:"generatedAt"`
}

// FeatureInfo is a summary of a single feature's test counts
type FeatureInfo struct {
	Name            string         `json:"name"`
	Category        string         `json:"category"`
	TotalTests      int            `json:"totalTests"`
	TestsByCategory map[string]int `json:"testsByCategory"`
}

func buildSummaryFromDisk(dir string) (*TestPlanSummary, error) {
	summary := &TestPlanSummary{
		TestsByCategory: make(map[string]int),
		FeaturesByGroup: make(map[string][]FeatureInfo),
		GeneratedAt:     time.Now().UTC().Format(time.RFC3339),
	}

	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
			return err
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil // skip unreadable files
		}

		relPath, _ := filepath.Rel(dir, path)
		relPath = filepath.ToSlash(relPath)
		parts := strings.SplitN(relPath, "/", 2)
		group := "uncategorized"
		featureName := strings.TrimSuffix(filepath.Base(relPath), ".yaml")
		if len(parts) == 2 {
			group = parts[0]
		}

		counts := countTestsInYAML(data)
		summary.TotalFeatures++
		summary.TotalTests += counts.total
		for cat, count := range counts.byCategory {
			summary.TestsByCategory[cat] += count
		}

		summary.FeaturesByGroup[group] = append(summary.FeaturesByGroup[group], FeatureInfo{
			Name:            featureName,
			Category:        group,
			TotalTests:      counts.total,
			TestsByCategory: counts.byCategory,
		})

		return nil
	})

	return summary, err
}

// SourceChange represents a detected change in source files
type SourceChange struct {
	File         string `json:"file"`
	LastModified string `json:"lastModified"`
	Size         int64  `json:"size"`
	Hash         string `json:"hash"`
}

func detectSourceChanges(sourcesDir string) ([]SourceChange, error) {
	var changes []SourceChange
	err := filepath.WalkDir(sourcesDir, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		ext := filepath.Ext(entry.Name())
		if ext != ".yaml" && ext != ".yang" && ext != ".json" {
			return nil
		}

		info, err := entry.Info()
		if err != nil {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		hash := sha256.Sum256(data)

		relPath, _ := filepath.Rel(sourcesDir, path)
		changes = append(changes, SourceChange{
			File:         filepath.ToSlash(relPath),
			LastModified: info.ModTime().UTC().Format(time.RFC3339),
			Size:         info.Size(),
			Hash:         fmt.Sprintf("%x", hash[:8]),
		})
		return nil
	})
	return changes, err
}

// yamlToCSV converts test plan YAML to a simple CSV format
func yamlToCSV(data []byte) string {
	var parsed map[string]interface{}
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		return "Error parsing YAML"
	}

	var sb strings.Builder
	sb.WriteString("TestCaseID,FeatureName,Type,Priority,Automation,Description,IsDeploymentTest,Method,Path,Payload,ExpectedStatus,Validations\n")

	featuresRaw, ok := parsed["features"]
	if !ok {
		return sb.String()
	}
	features, ok := featuresRaw.([]interface{})
	if !ok {
		return sb.String()
	}

	for _, f := range features {
		feature, ok := f.(map[string]interface{})
		if !ok {
			continue
		}
		testsRaw, ok := feature["tests"]
		if !ok {
			continue
		}
		tests, ok := testsRaw.(map[string]interface{})
		if !ok {
			continue
		}
		for _, catTests := range tests {
			arr, ok := catTests.([]interface{})
			if !ok {
				continue
			}
			for _, t := range arr {
				tc, ok := t.(map[string]interface{})
				if !ok {
					continue
				}
				// Extract step details
				method, path, payload, expectedStatus, validations := extractStepInfo(tc)
				sb.WriteString(fmt.Sprintf("%s,%s,%s,%s,%s,%q,%v,%s,%s,%q,%s,%q\n",
					safeStr(tc["testCaseID"]),
					safeStr(tc["featureName"]),
					safeStr(tc["type"]),
					safeStr(tc["priority"]),
					safeStr(tc["automation"]),
					safeStr(tc["description"]),
					tc["isDeploymentTest"],
					method, path, payload, expectedStatus, validations,
				))
			}
		}
	}

	return sb.String()
}

func extractStepInfo(tc map[string]interface{}) (method, path, payload, expectedStatus, validations string) {
	stepsRaw, ok := tc["steps"]
	if !ok {
		return
	}
	steps, ok := stepsRaw.([]interface{})
	if !ok || len(steps) == 0 {
		return
	}
	// Use first step for primary method/path/payload
	step, ok := steps[0].(map[string]interface{})
	if !ok {
		return
	}
	method = safeStr(step["method"])
	path = safeStr(step["path"])
	if body, ok := step["body"]; ok && body != nil {
		b, err := json.Marshal(body)
		if err == nil {
			payload = string(b)
		}
	}
	if es, ok := step["expectedStatus"]; ok {
		expectedStatus = safeStr(es)
	}
	if vals, ok := step["validations"]; ok {
		if arr, ok := vals.([]interface{}); ok {
			var parts []string
			for _, v := range arr {
				parts = append(parts, safeStr(v))
			}
			validations = strings.Join(parts, "; ")
		}
	}
	return
}

func safeStr(v interface{}) string {
	if v == nil {
		return ""
	}
	return fmt.Sprintf("%v", v)
}

// extractGenerationContextFromFiles pulls generation metadata from YAML test plan headers
func extractGenerationContextFromFiles(files map[string][]byte) *GenerationContext {
	for name, data := range files {
		if !strings.HasSuffix(name, ".yaml") {
			continue
		}
		var header struct {
			Version       string `yaml:"version"`
			GeneratedAt   string `yaml:"generatedAt"`
			SourceYangDir string `yaml:"sourceYangDir"`
			SourceRESTAPI string `yaml:"sourceRESTAPI"`
			SourceNOSAPI  string `yaml:"sourceNOSAPI"`
		}
		if err := yaml.Unmarshal(data, &header); err != nil {
			continue
		}
		if header.GeneratedAt != "" {
			return &GenerationContext{
				GeneratedAt:   header.GeneratedAt,
				ToolVersion:   header.Version,
				SourceYangDir: header.SourceYangDir,
				SourceRESTAPI: header.SourceRESTAPI,
				SourceNOSAPI:  header.SourceNOSAPI,
			}
		}
	}
	return &GenerationContext{}
}

// getGitCommit returns the current git HEAD commit hash
func getGitCommit() string {
	data, err := os.ReadFile(".git/HEAD")
	if err != nil {
		return ""
	}
	head := strings.TrimSpace(string(data))
	// If it's a ref, resolve it
	if strings.HasPrefix(head, "ref: ") {
		refPath := strings.TrimPrefix(head, "ref: ")
		data, err = os.ReadFile(filepath.Join(".git", refPath))
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(data))[:12]
	}
	if len(head) >= 12 {
		return head[:12]
	}
	return head
}

// computeSourceFingerprints calculates SHA256 hashes for all source spec files
func computeSourceFingerprints(sourcesDir string) map[string]string {
	fingerprints := make(map[string]string)
	filepath.WalkDir(sourcesDir, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		ext := filepath.Ext(entry.Name())
		if ext != ".yaml" && ext != ".yang" && ext != ".json" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		hash := sha256.Sum256(data)
		relPath, _ := filepath.Rel(sourcesDir, path)
		fingerprints[filepath.ToSlash(relPath)] = fmt.Sprintf("%x", hash[:16])
		return nil
	})
	return fingerprints
}

// --- YAML to Excel conversion ---

type excelTestCase struct {
	TestCaseID       string
	FeatureName      string
	Priority         string
	Automation       string
	Type             string
	Description      string
	IsDeploymentTest bool
	Steps            []excelStep
}

type excelStep struct {
	Name           string
	Description    string
	Method         string
	Path           string
	PathParams     interface{}
	Body           interface{}
	ExpectedStatus int
	Validations    []string
	Timeout        int
}

var excelCategoryOrder = []string{"functional", "boundary", "negative", "performance", "scale"}

var rePrefixes = regexp.MustCompile(`(?i)^(?:Boundary|Negative|Performance|Scale)\s+test:\s*`)

func compressExcelTitle(desc string) string {
	t := rePrefixes.ReplaceAllString(desc, "")
	if len(t) > 100 {
		t = t[:97] + "..."
	}
	return strings.TrimSpace(t)
}

func formatBodyAsJSON(body interface{}) string {
	if body == nil {
		return ""
	}
	b, err := json.MarshalIndent(body, "", "  ")
	if err != nil {
		return fmt.Sprintf("%v", body)
	}
	return string(b)
}

func formatExcelSteps(steps []excelStep) string {
	var lines []string
	for i, s := range steps {
		lines = append(lines, fmt.Sprintf("%d) %s", i+1, s.Description))
		lines = append(lines, fmt.Sprintf("   %s {base_url}%s", s.Method, s.Path))
		if s.PathParams != nil {
			lines = append(lines, fmt.Sprintf("   Path Params: %s", formatBodyAsJSON(s.PathParams)))
		}
		if s.Body != nil {
			lines = append(lines, fmt.Sprintf("   Payload: %s", formatBodyAsJSON(s.Body)))
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

func formatExcelExpected(steps []excelStep) string {
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
	}
	return strings.TrimSpace(strings.Join(results, "\n"))
}

func yamlToExcel(data []byte) ([]byte, error) {
	var parsed struct {
		Features []struct {
			FeatureName string                          `yaml:"featureName"`
			Tests       map[string][]excelTestCase      `yaml:"tests"`
		} `yaml:"features"`
	}
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		return nil, fmt.Errorf("parsing YAML: %w", err)
	}

	f := excelize.NewFile()

	excelHeaders := []string{
		"Test Case ID", "Testcase Title", "Status", "Type", "Description",
		"Precondition", "Test Step Description", "Test Step Expected Result", "Priority",
	}
	colWidths := []float64{16, 50, 18, 12, 60, 50, 80, 60, 10}

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
	dataStyle, _ := f.NewStyle(&excelize.Style{
		Alignment: &excelize.Alignment{Vertical: "top", WrapText: true},
		Border: []excelize.Border{
			{Type: "left", Style: 1, Color: "000000"},
			{Type: "right", Style: 1, Color: "000000"},
			{Type: "top", Style: 1, Color: "000000"},
			{Type: "bottom", Style: 1, Color: "000000"},
		},
	})

	for _, feat := range parsed.Features {
		for _, cat := range excelCategoryOrder {
			cases := feat.Tests[cat]
			if len(cases) == 0 {
				continue
			}
			sheetName := strings.ToUpper(cat[:1]) + cat[1:]
			if len(sheetName) > 31 {
				sheetName = sheetName[:31]
			}
			f.NewSheet(sheetName)

			for ci, h := range excelHeaders {
				cn, _ := excelize.ColumnNumberToName(ci + 1)
				cell := fmt.Sprintf("%s1", cn)
				f.SetCellValue(sheetName, cell, h)
				f.SetCellStyle(sheetName, cell, cell, headerStyle)
			}
			for ci, w := range colWidths {
				cn, _ := excelize.ColumnNumberToName(ci + 1)
				f.SetColWidth(sheetName, cn, cn, w)
			}

			for i, tc := range cases {
				row := i + 2
				title := compressExcelTitle(tc.Description)
				stepDesc := formatExcelSteps(tc.Steps)
				expected := formatExcelExpected(tc.Steps)
				priority := tc.Priority
				if priority == "" {
					priority = "P3"
				}
				vals := []string{
					tc.TestCaseID, title, "To Be Automated", "Manual",
					tc.Description,
					"QA environment available with EP1-NGC Framework integration",
					stepDesc, expected, priority,
				}
				for ci, v := range vals {
					cn, _ := excelize.ColumnNumberToName(ci + 1)
					cell := fmt.Sprintf("%s%d", cn, row)
					f.SetCellValue(sheetName, cell, v)
					f.SetCellStyle(sheetName, cell, cell, dataStyle)
				}
			}
		}
	}

	f.DeleteSheet("Sheet1")

	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("writing Excel: %w", err)
	}
	return buf.Bytes(), nil
}
