package web

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/xuri/excelize/v2"
	"gopkg.in/yaml.v3"
)

// handleDashboard serves the main dashboard page
func (s *Server) handleDashboard(c *gin.Context) {
	c.Header("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	c.Header("Pragma", "no-cache")
	c.Header("Expires", "0")
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
	YangDir      string `json:"yangDir"`
	RestSpec     string `json:"restSpec"`
	NosapiSpec   string `json:"nosapiSpec"`
	OutDir       string `json:"outDir"`
	YangExists   bool   `json:"yangExists"`
	RestExists   bool   `json:"restExists"`
	NosapiExists bool   `json:"nosapiExists"`
	OutDirExists bool   `json:"outDirExists"`
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
	sc.OutDirExists = dirExists(sc.OutDir)

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
			FeatureName string                     `yaml:"featureName"`
			Tests       map[string][]excelTestCase `yaml:"tests"`
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

// --- Pull Sources ---

type pullResult struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

func (s *Server) handlePullSources(c *gin.Context) {
	configPath := filepath.Join("config", "config.yaml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "cannot read config.yaml"})
		return
	}

	type sourceEntry struct {
		Repo     string `yaml:"repo"`
		Branch   string `yaml:"branch"`
		Sparse   string `yaml:"sparse"`
		LocalDir string `yaml:"localDir"`
	}
	var cfg struct {
		SourcesDir string                 `yaml:"sourcesDir"`
		Sources    map[string]sourceEntry `yaml:"sources"`
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "cannot parse config.yaml"})
		return
	}
	if cfg.SourcesDir == "" {
		cfg.SourcesDir = "./sources"
	}

	var results []pullResult
	for name, src := range cfg.Sources {
		if src.Repo == "" {
			continue // skip manual sources
		}
		cloneDir := filepath.Join(cfg.SourcesDir, src.LocalDir)
		pr := pullResult{Name: name}

		if dirExists(cloneDir) {
			// Already cloned — pull latest
			cmd := exec.Command("git", "-C", cloneDir, "pull", "origin", src.Branch, "--quiet")
			out, err := cmd.CombinedOutput()
			if err != nil {
				pr.Status = "pull failed"
				pr.Error = strings.TrimSpace(string(out))
			} else {
				pr.Status = "updated"
			}
		} else {
			// Fresh sparse checkout
			cmd := exec.Command("git", "clone", "--filter=blob:none", "--sparse", "--branch", src.Branch, src.Repo, cloneDir)
			out, err := cmd.CombinedOutput()
			if err != nil {
				pr.Status = "clone failed"
				pr.Error = strings.TrimSpace(string(out))
			} else {
				if src.Sparse != "" {
					cmd2 := exec.Command("git", "-C", cloneDir, "sparse-checkout", "set", src.Sparse)
					if out2, err2 := cmd2.CombinedOutput(); err2 != nil {
						pr.Status = "sparse-checkout failed"
						pr.Error = strings.TrimSpace(string(out2))
					} else {
						pr.Status = "cloned"
					}
				} else {
					pr.Status = "cloned"
				}
			}
		}
		results = append(results, pr)
	}

	c.JSON(http.StatusOK, gin.H{"results": results})
}

// --- Jira Integration ---

type jiraConfig struct {
	URL     string `yaml:"url" json:"url"`
	Project string `yaml:"project" json:"project"`
	Email   string `yaml:"email" json:"email"`
	Token   string `yaml:"token" json:"-"` // never expose token
}

func (s *Server) loadJiraConfig() (*jiraConfig, error) {
	configPath := filepath.Join("config", "config.yaml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}
	var cfg struct {
		Jira jiraConfig `yaml:"jira"`
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	if cfg.Jira.URL == "" {
		return nil, fmt.Errorf("jira URL not configured")
	}
	// Token from env var takes precedence, fallback to config file
	if envToken := os.Getenv("JIRA_API_TOKEN"); envToken != "" {
		cfg.Jira.Token = envToken
	}
	if cfg.Jira.Token == "" {
		return nil, fmt.Errorf("JIRA_API_TOKEN environment variable not set")
	}
	return &cfg.Jira, nil
}

func (s *Server) handleJiraConfig(c *gin.Context) {
	jcfg, err := s.loadJiraConfig()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"configured": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"configured": true,
		"url":        jcfg.URL,
		"project":    jcfg.Project,
		"email":      jcfg.Email,
	})
}

// handleJiraSearch searches Jira issues by JQL query
func (s *Server) handleJiraSearch(c *gin.Context) {
	jcfg, err := s.loadJiraConfig()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Jira not configured"})
		return
	}

	// Detect if user entered an issue key (e.g. NVO-7491, TCXM-123)
	issueKeyRe := regexp.MustCompile(`^[A-Z]+-\d+$`)
	rawQuery := c.Query("jql")
	feature := c.Query("feature")
	if rawQuery == "" && feature != "" {
		rawQuery = feature
	}
	if rawQuery != "" && issueKeyRe.MatchString(strings.TrimSpace(rawQuery)) {
		// Fetch single issue and wrap as search result
		key := strings.TrimSpace(rawQuery)
		issue, statusCode, fetchErr := s.fetchJiraIssue(jcfg, key)
		if fetchErr != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": fetchErr.Error()})
			return
		}
		if statusCode != http.StatusOK {
			c.JSON(statusCode, gin.H{"error": fmt.Sprintf("Jira returned HTTP %d for %s", statusCode, key)})
			return
		}
		// Wrap as search result format
		c.JSON(http.StatusOK, gin.H{
			"total":  1,
			"issues": []interface{}{issue},
		})
		return
	}

	jql := c.Query("jql")
	if jql == "" {
		if feature != "" {
			if jcfg.Project != "" {
				jql = fmt.Sprintf("project = %s AND summary ~ \"%s\" ORDER BY created DESC", jcfg.Project, feature)
			} else {
				jql = fmt.Sprintf("summary ~ \"%s\" ORDER BY created DESC", feature)
			}
		} else {
			if jcfg.Project != "" {
				jql = fmt.Sprintf("project = %s ORDER BY created DESC", jcfg.Project)
			} else {
				jql = "updated >= -30d ORDER BY updated DESC"
			}
		}
	}

	maxResultsStr := c.DefaultQuery("maxResults", "50")
	maxResultsInt, _ := strconv.Atoi(maxResultsStr)
	if maxResultsInt <= 0 {
		maxResultsInt = 50
	}

	result, statusCode, searchErr := s.searchJiraIssues(jcfg, jql, maxResultsInt, []string{"summary", "status", "priority", "assignee", "created", "updated", "issuetype", "labels", "parent"})
	if searchErr != nil {
		if statusCode == 0 {
			statusCode = http.StatusBadGateway
		}
		c.JSON(statusCode, gin.H{"error": searchErr.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) searchJiraIssues(jcfg *jiraConfig, jql string, maxResults int, fields []string) (interface{}, int, error) {
	if maxResults <= 0 {
		maxResults = 50
	}
	body := map[string]interface{}{
		"jql":        jql,
		"maxResults": maxResults,
		"fields":     fields,
	}
	bodyBytes, _ := json.Marshal(body)

	apiURL := fmt.Sprintf("%s/rest/api/3/search/jql", jcfg.URL)
	req, err := http.NewRequest("POST", apiURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, 0, err
	}
	auth := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", jcfg.Email, jcfg.Token)))
	req.Header.Set("Authorization", "Basic "+auth)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Atlassian-Token", "no-check")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to reach Jira: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode, fmt.Errorf("Jira returned HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var result interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, resp.StatusCode, fmt.Errorf("failed to parse Jira response: %s", string(respBody))
	}
	return result, resp.StatusCode, nil
}

// fetchJiraIssue fetches a single issue by key, returns parsed JSON, status code, and error
func (s *Server) fetchJiraIssue(jcfg *jiraConfig, key string) (interface{}, int, error) {
	apiURL := fmt.Sprintf("%s/rest/api/3/issue/%s", jcfg.URL, url.PathEscape(key))
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, 0, err
	}
	auth := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", jcfg.Email, jcfg.Token)))
	req.Header.Set("Authorization", "Basic "+auth)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Atlassian-Token", "no-check")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to reach Jira: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	var result interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, resp.StatusCode, fmt.Errorf("failed to parse Jira response: %s", string(respBody))
	}
	if resp.StatusCode != http.StatusOK {
		return result, resp.StatusCode, fmt.Errorf("Jira returned HTTP %d: %s", resp.StatusCode, string(respBody))
	}
	return result, resp.StatusCode, nil
}

// handleJiraIssue gets a single Jira issue by key
func (s *Server) handleJiraIssue(c *gin.Context) {
	jcfg, err := s.loadJiraConfig()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Jira not configured"})
		return
	}

	key := c.Param("key")
	result, statusCode, fetchErr := s.fetchJiraIssue(jcfg, key)
	if fetchErr != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": fetchErr.Error()})
		return
	}
	c.JSON(statusCode, result)
}

// handleJiraIssueRelated returns child stories/tasks for an issue key.
func (s *Server) handleJiraIssueRelated(c *gin.Context) {
	jcfg, err := s.loadJiraConfig()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Jira not configured"})
		return
	}

	key := strings.TrimSpace(c.Param("key"))
	issueKeyRe := regexp.MustCompile(`^[A-Z]+-\d+$`)
	if !issueKeyRe.MatchString(key) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid Jira issue key"})
		return
	}

	children, statusCode, searchErr := s.searchJiraIssues(
		jcfg,
		fmt.Sprintf("parent = %s ORDER BY created DESC", key),
		100,
		[]string{"summary", "status", "priority", "assignee", "created", "updated", "issuetype", "labels", "parent"},
	)
	if searchErr != nil {
		if statusCode == 0 {
			statusCode = http.StatusBadGateway
		}
		c.JSON(statusCode, gin.H{"error": searchErr.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"children": children})
}

// handleJiraProjects lists accessible Jira projects
func (s *Server) handleJiraProjects(c *gin.Context) {
	jcfg, err := s.loadJiraConfig()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Jira not configured"})
		return
	}

	apiURL := fmt.Sprintf("%s/rest/api/3/project", jcfg.URL)

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	auth := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", jcfg.Email, jcfg.Token)))
	req.Header.Set("Authorization", "Basic "+auth)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Atlassian-Token", "no-check")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to reach Jira: " + err.Error()})
		return
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	var result interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to parse Jira response"})
		return
	}
	c.JSON(resp.StatusCode, result)
}

// handleJiraFigmaLinks extracts Figma URLs from a Jira issue description, comments,
// remote links, and custom fields (including the Figma for Jira "Designs" section).
func (s *Server) handleJiraFigmaLinks(c *gin.Context) {
	jcfg, err := s.loadJiraConfig()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Jira not configured"})
		return
	}

	key := strings.TrimSpace(c.Param("key"))
	issueKeyRe := regexp.MustCompile(`^[A-Z]+-\d+$`)
	if !issueKeyRe.MatchString(key) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid Jira issue key"})
		return
	}

	result, _, err := s.fetchJiraIssue(jcfg, key)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}

	data, _ := json.Marshal(result)
	var issue struct {
		Fields json.RawMessage `json:"fields"`
	}
	json.Unmarshal(data, &issue)

	// Parse known fields
	var knownFields struct {
		Summary     string      `json:"summary"`
		Description interface{} `json:"description"`
	}
	json.Unmarshal(issue.Fields, &knownFields)

	// Extract all text from the issue (description in ADF format)
	descText := ""
	if knownFields.Description != nil {
		descText = extractADFText(knownFields.Description)
	}

	// Also extract raw URLs from ADF marks (hyperlinks)
	var allURLs []string
	extractADFURLs(knownFields.Description, &allURLs)

	// ── Scan ALL custom fields for Figma URLs ──
	// Figma for Jira stores design URLs in custom fields or as embedded data.
	// Walk the entire fields JSON to find any Figma URLs.
	var fieldsMap map[string]interface{}
	json.Unmarshal(issue.Fields, &fieldsMap)
	for fieldKey, fieldVal := range fieldsMap {
		if fieldKey == "description" || fieldKey == "summary" {
			continue
		}
		extractFigmaURLsFromValue(fieldVal, &allURLs)
	}

	// ── Fetch remote links (Figma for Jira "Designs" section) ──
	remoteLinks := s.fetchJiraRemoteLinks(jcfg, key)
	allURLs = append(allURLs, remoteLinks...)

	// ── Fetch issue properties (some Figma integrations store data here) ──
	propURLs := s.fetchJiraIssuePropertyFigmaURLs(jcfg, key)
	allURLs = append(allURLs, propURLs...)

	// ── Also check sub-tasks and parent for Figma links ──
	// (The Figma design may be attached to a parent epic/story)

	// Search for Figma URLs in text + extracted URLs
	// URL-decode all URLs first (Jira stores node-id with %3A instead of :)
	for i, u := range allURLs {
		if decoded, err := url.QueryUnescape(u); err == nil {
			allURLs[i] = decoded
		}
	}
	figmaRe := regexp.MustCompile(`https?://(?:www\.)?figma\.com/(?:file|design|proto|board)/([a-zA-Z0-9]+)(?:/[^?\s"'\]>)]*)?(?:\?[^\s"'\]>)]*node-id=([0-9]+(?:[:-][0-9]+)?))?`)

	allText := descText + "\n" + strings.Join(allURLs, "\n")
	matches := figmaRe.FindAllStringSubmatch(allText, -1)

	type figmaLink struct {
		URL     string `json:"url"`
		FileKey string `json:"fileKey"`
		NodeID  string `json:"nodeId"`
		Source  string `json:"source"`
	}
	seen := map[string]bool{}
	var links []figmaLink
	for _, m := range matches {
		fileKey := m[1]
		nodeID := ""
		if len(m) > 2 {
			nodeID = strings.ReplaceAll(m[2], ":", "-")
		}
		dedupKey := fileKey + "|" + nodeID
		if seen[dedupKey] {
			continue
		}
		seen[dedupKey] = true

		source := "description"
		fullURL := m[0]
		for _, rl := range remoteLinks {
			if strings.Contains(rl, fileKey) {
				source = "designs"
				if len(fullURL) < len(rl) {
					fullURL = rl
				}
				break
			}
		}
		for _, pl := range propURLs {
			if strings.Contains(pl, fileKey) {
				source = "properties"
				break
			}
		}

		links = append(links, figmaLink{URL: fullURL, FileKey: fileKey, NodeID: nodeID, Source: source})
	}

	log.Printf("[figma-links] %s: found %d link(s) from description(%d URLs), remoteLinks(%d), properties(%d)",
		key, len(links), len(allURLs)-len(remoteLinks)-len(propURLs), len(remoteLinks), len(propURLs))

	c.JSON(http.StatusOK, gin.H{
		"issueKey": key,
		"summary":  knownFields.Summary,
		"links":    links,
	})
}

// fetchJiraRemoteLinks gets remote links from a Jira issue (used by Figma for Jira "Designs" section).
func (s *Server) fetchJiraRemoteLinks(jcfg *jiraConfig, key string) []string {
	apiURL := fmt.Sprintf("%s/rest/api/3/issue/%s/remotelink", jcfg.URL, url.PathEscape(key))
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil
	}
	auth := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", jcfg.Email, jcfg.Token)))
	req.Header.Set("Authorization", "Basic "+auth)
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("[figma-links] failed to fetch remote links for %s: %v", key, err)
		return nil
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		log.Printf("[figma-links] remote links API returned %d for %s", resp.StatusCode, key)
		return nil
	}

	var remoteLinks []struct {
		Object struct {
			URL   string `json:"url"`
			Title string `json:"title"`
		} `json:"object"`
	}
	json.Unmarshal(body, &remoteLinks)

	var urls []string
	for _, rl := range remoteLinks {
		if rl.Object.URL != "" {
			urls = append(urls, rl.Object.URL)
		}
	}
	log.Printf("[figma-links] %s: %d remote link(s) found", key, len(urls))
	return urls
}

// fetchJiraIssuePropertyFigmaURLs checks issue properties for Figma-related data.
func (s *Server) fetchJiraIssuePropertyFigmaURLs(jcfg *jiraConfig, key string) []string {
	apiURL := fmt.Sprintf("%s/rest/api/3/issue/%s/properties", jcfg.URL, url.PathEscape(key))
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil
	}
	auth := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", jcfg.Email, jcfg.Token)))
	req.Header.Set("Authorization", "Basic "+auth)
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil
	}

	// Parse properties list
	var propList struct {
		Keys []struct {
			Key string `json:"key"`
		} `json:"keys"`
	}
	json.Unmarshal(body, &propList)

	var urls []string
	figmaRe := regexp.MustCompile(`https?://(?:www\.)?figma\.com/[^\s"'\]>)]+`)

	for _, prop := range propList.Keys {
		// Fetch each property that might contain Figma data
		if !strings.Contains(strings.ToLower(prop.Key), "figma") &&
			!strings.Contains(strings.ToLower(prop.Key), "design") {
			continue
		}

		propURL := fmt.Sprintf("%s/rest/api/3/issue/%s/properties/%s",
			jcfg.URL, url.PathEscape(key), url.PathEscape(prop.Key))
		propReq, err := http.NewRequest("GET", propURL, nil)
		if err != nil {
			continue
		}
		propReq.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString(
			[]byte(fmt.Sprintf("%s:%s", jcfg.Email, jcfg.Token))))
		propReq.Header.Set("Accept", "application/json")

		propResp, err := http.DefaultClient.Do(propReq)
		if err != nil {
			continue
		}
		propBody, _ := io.ReadAll(propResp.Body)
		propResp.Body.Close()

		// Extract any Figma URLs from the property value
		propMatches := figmaRe.FindAllString(string(propBody), -1)
		urls = append(urls, propMatches...)
	}
	return urls
}

// extractFigmaURLsFromValue recursively walks a JSON value looking for Figma URLs.
func extractFigmaURLsFromValue(val interface{}, urls *[]string) {
	figmaRe := regexp.MustCompile(`https?://(?:www\.)?figma\.com/[^\s"'\]>)]+`)
	switch v := val.(type) {
	case string:
		matches := figmaRe.FindAllString(v, -1)
		*urls = append(*urls, matches...)
	case map[string]interface{}:
		for _, child := range v {
			extractFigmaURLsFromValue(child, urls)
		}
	case []interface{}:
		for _, child := range v {
			extractFigmaURLsFromValue(child, urls)
		}
	}
}

// extractADFURLs extracts hyperlink URLs from Atlassian Document Format nodes.
func extractADFURLs(node interface{}, urls *[]string) {
	switch v := node.(type) {
	case map[string]interface{}:
		// Check marks for links
		if marks, ok := v["marks"].([]interface{}); ok {
			for _, mark := range marks {
				m, _ := mark.(map[string]interface{})
				if m["type"] == "link" {
					if attrs, ok := m["attrs"].(map[string]interface{}); ok {
						if href, ok := attrs["href"].(string); ok {
							*urls = append(*urls, href)
						}
					}
				}
			}
		}
		// Check for inlineCard with url
		if v["type"] == "inlineCard" {
			if attrs, ok := v["attrs"].(map[string]interface{}); ok {
				if url, ok := attrs["url"].(string); ok {
					*urls = append(*urls, url)
				}
			}
		}
		// Recurse into content
		if content, ok := v["content"].([]interface{}); ok {
			for _, child := range content {
				extractADFURLs(child, urls)
			}
		}
	case []interface{}:
		for _, item := range v {
			extractADFURLs(item, urls)
		}
	}
}

// ---------- GUI Test Plan Download ----------

func (s *Server) handleDownloadGUITestPlan(c *gin.Context) {
	category := c.Param("category")
	feature := c.Param("feature")
	format := c.DefaultQuery("format", "xlsx")

	path := filepath.Join(s.config.TestPlansDir, category, feature+".yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "GUI test plan not found"})
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
		jsonData, _ := json.MarshalIndent(parsed, "", "  ")
		c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s.json", filename))
		c.Data(http.StatusOK, "application/json", jsonData)
	case "xlsx":
		xlsxData, err := guiYamlToExcel(data)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to convert to Excel: " + err.Error()})
			return
		}
		c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s.xlsx", filename))
		c.Data(http.StatusOK, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", xlsxData)
	case "csv":
		csvData := guiYamlToCSV(data)
		c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s.csv", filename))
		c.Data(http.StatusOK, "text/csv", []byte(csvData))
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported format, use yaml/json/csv/xlsx"})
	}
}

func (s *Server) handleGetGUITestPlan(c *gin.Context) {
	category := c.Param("category")
	feature := c.Param("feature")

	path := filepath.Join(s.config.TestPlansDir, category, feature+".yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "GUI test plan not found"})
		return
	}

	var parsed interface{}
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to parse"})
		return
	}
	c.JSON(http.StatusOK, parsed)
}

// guiYamlToExcel converts a GUI test plan YAML to an Excel workbook.
func guiYamlToExcel(data []byte) ([]byte, error) {
	var plan struct {
		Version     string `yaml:"version"`
		GeneratedAt string `yaml:"generatedAt"`
		TestType    string `yaml:"testType"`
		SourceInfo  struct {
			FeatureName  string `yaml:"featureName"`
			Category     string `yaml:"category"`
			APITestPlan  string `yaml:"apiTestPlan"`
			FigmaSection string `yaml:"figmaSection"`
			JiraIssueKey string `yaml:"jiraIssueKey"`
		} `yaml:"sourceInfo"`
		Screens []struct {
			ScreenName string `yaml:"screenName"`
			Widgets    []struct {
				Name     string `yaml:"name"`
				TypeName string `yaml:"typeName"`
			} `yaml:"widgets"`
			Tests []struct {
				TestCaseID  string `yaml:"testCaseID"`
				FeatureName string `yaml:"featureName"`
				Priority    string `yaml:"priority"`
				Type        string `yaml:"type"`
				Description string `yaml:"description"`
				Screen      string `yaml:"screen"`
				Steps       []struct {
					StepNumber int    `yaml:"stepNumber"`
					Action     string `yaml:"action"`
					Target     string `yaml:"target"`
					Value      string `yaml:"value"`
					Expected   string `yaml:"expected"`
				} `yaml:"steps"`
			} `yaml:"tests"`
		} `yaml:"screens"`
		Summary struct {
			TotalScreens    int            `yaml:"totalScreens"`
			TotalWidgets    int            `yaml:"totalWidgets"`
			TotalTests      int            `yaml:"totalTests"`
			TestsByCategory map[string]int `yaml:"testsByCategory"`
		} `yaml:"summary"`
	}
	if err := yaml.Unmarshal(data, &plan); err != nil {
		return nil, fmt.Errorf("parsing YAML: %w", err)
	}

	f := excelize.NewFile()

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

	// Summary sheet
	f.SetSheetName("Sheet1", "Summary")
	summaryData := [][]string{
		{"GUI Test Plan Summary"},
		{"Feature", plan.SourceInfo.FeatureName},
		{"Category", plan.SourceInfo.Category},
		{"Generated At", plan.GeneratedAt},
		{"API Test Plan", plan.SourceInfo.APITestPlan},
		{"Figma Section", plan.SourceInfo.FigmaSection},
		{"Jira Issue", plan.SourceInfo.JiraIssueKey},
		{""},
		{"Total Screens", fmt.Sprintf("%d", plan.Summary.TotalScreens)},
		{"Total Widgets", fmt.Sprintf("%d", plan.Summary.TotalWidgets)},
		{"Total Tests", fmt.Sprintf("%d", plan.Summary.TotalTests)},
	}
	for cat, count := range plan.Summary.TestsByCategory {
		summaryData = append(summaryData, []string{fmt.Sprintf("Tests (%s)", cat), fmt.Sprintf("%d", count)})
	}
	for i, row := range summaryData {
		for j, val := range row {
			cn, _ := excelize.ColumnNumberToName(j + 1)
			cell := fmt.Sprintf("%s%d", cn, i+1)
			f.SetCellValue("Summary", cell, val)
			if i == 0 {
				f.SetCellStyle("Summary", cell, cell, headerStyle)
			}
		}
	}
	f.SetColWidth("Summary", "A", "A", 20)
	f.SetColWidth("Summary", "B", "B", 60)

	// All Tests sheet — flat table of every test case
	allSheet := "All Tests"
	f.NewSheet(allSheet)
	headers := []string{"Test Case ID", "Screen", "Type", "Priority", "Description", "Precondition", "Test Steps", "Expected Results"}
	colWidths := []float64{22, 30, 12, 8, 70, 40, 80, 60}
	for ci, h := range headers {
		cn, _ := excelize.ColumnNumberToName(ci + 1)
		cell := fmt.Sprintf("%s1", cn)
		f.SetCellValue(allSheet, cell, h)
		f.SetCellStyle(allSheet, cell, cell, headerStyle)
	}
	for ci, w := range colWidths {
		cn, _ := excelize.ColumnNumberToName(ci + 1)
		f.SetColWidth(allSheet, cn, cn, w)
	}

	allRow := 2
	for _, screen := range plan.Screens {
		for _, tc := range screen.Tests {
			// Build step descriptions and expected results
			var stepLines, expectedLines []string
			for _, step := range tc.Steps {
				stepLine := fmt.Sprintf("%d. %s → %s", step.StepNumber, step.Action, step.Target)
				if step.Value != "" {
					stepLine += fmt.Sprintf(" [value: %s]", step.Value)
				}
				stepLines = append(stepLines, stepLine)
				expectedLines = append(expectedLines, fmt.Sprintf("%d. %s", step.StepNumber, step.Expected))
			}

			vals := []string{
				tc.TestCaseID,
				tc.Screen,
				tc.Type,
				tc.Priority,
				tc.Description,
				fmt.Sprintf("Navigate to %s screen. Feature data available.", screen.ScreenName),
				strings.Join(stepLines, "\n"),
				strings.Join(expectedLines, "\n"),
			}
			for ci, v := range vals {
				cn, _ := excelize.ColumnNumberToName(ci + 1)
				cell := fmt.Sprintf("%s%d", cn, allRow)
				f.SetCellValue(allSheet, cell, v)
				f.SetCellStyle(allSheet, cell, cell, dataStyle)
			}
			allRow++
		}
	}

	// Per-screen sheets
	for _, screen := range plan.Screens {
		if len(screen.Tests) == 0 {
			continue
		}
		sheetName := screen.ScreenName
		if len(sheetName) > 31 {
			sheetName = sheetName[:31]
		}
		f.NewSheet(sheetName)
		for ci, h := range headers {
			cn, _ := excelize.ColumnNumberToName(ci + 1)
			cell := fmt.Sprintf("%s1", cn)
			f.SetCellValue(sheetName, cell, h)
			f.SetCellStyle(sheetName, cell, cell, headerStyle)
		}
		for ci, w := range colWidths {
			cn, _ := excelize.ColumnNumberToName(ci + 1)
			f.SetColWidth(sheetName, cn, cn, w)
		}

		for i, tc := range screen.Tests {
			row := i + 2
			var stepLines, expectedLines []string
			for _, step := range tc.Steps {
				stepLine := fmt.Sprintf("%d. %s → %s", step.StepNumber, step.Action, step.Target)
				if step.Value != "" {
					stepLine += fmt.Sprintf(" [value: %s]", step.Value)
				}
				stepLines = append(stepLines, stepLine)
				expectedLines = append(expectedLines, fmt.Sprintf("%d. %s", step.StepNumber, step.Expected))
			}

			vals := []string{
				tc.TestCaseID,
				tc.Screen,
				tc.Type,
				tc.Priority,
				tc.Description,
				fmt.Sprintf("Navigate to %s screen. Feature data available.", screen.ScreenName),
				strings.Join(stepLines, "\n"),
				strings.Join(expectedLines, "\n"),
			}
			for ci, v := range vals {
				cn, _ := excelize.ColumnNumberToName(ci + 1)
				cell := fmt.Sprintf("%s%d", cn, row)
				f.SetCellValue(sheetName, cell, v)
				f.SetCellStyle(sheetName, cell, cell, dataStyle)
			}
		}
	}

	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("writing Excel: %w", err)
	}
	return buf.Bytes(), nil
}

// guiYamlToCSV converts a GUI test plan YAML to CSV format.
func guiYamlToCSV(data []byte) string {
	var plan struct {
		Screens []struct {
			ScreenName string `yaml:"screenName"`
			Tests      []struct {
				TestCaseID  string `yaml:"testCaseID"`
				Priority    string `yaml:"priority"`
				Type        string `yaml:"type"`
				Description string `yaml:"description"`
				Screen      string `yaml:"screen"`
				Steps       []struct {
					StepNumber int    `yaml:"stepNumber"`
					Action     string `yaml:"action"`
					Target     string `yaml:"target"`
					Value      string `yaml:"value"`
					Expected   string `yaml:"expected"`
				} `yaml:"steps"`
			} `yaml:"tests"`
		} `yaml:"screens"`
	}
	if err := yaml.Unmarshal(data, &plan); err != nil {
		return "Error parsing YAML"
	}

	var sb strings.Builder
	sb.WriteString("Test Case ID,Screen,Type,Priority,Description,Precondition,Test Steps,Expected Results\n")

	csvQuote := func(s string) string {
		if strings.ContainsAny(s, ",\"\n") {
			return "\"" + strings.ReplaceAll(s, "\"", "\"\"") + "\""
		}
		return s
	}

	for _, screen := range plan.Screens {
		for _, tc := range screen.Tests {
			var stepLines, expectedLines []string
			for _, step := range tc.Steps {
				stepLine := fmt.Sprintf("%d. %s → %s", step.StepNumber, step.Action, step.Target)
				if step.Value != "" {
					stepLine += fmt.Sprintf(" [value: %s]", step.Value)
				}
				stepLines = append(stepLines, stepLine)
				expectedLines = append(expectedLines, fmt.Sprintf("%d. %s", step.StepNumber, step.Expected))
			}
			precond := fmt.Sprintf("Navigate to %s screen. Feature data available.", screen.ScreenName)
			sb.WriteString(fmt.Sprintf("%s,%s,%s,%s,%s,%s,%s,%s\n",
				csvQuote(tc.TestCaseID),
				csvQuote(tc.Screen),
				csvQuote(tc.Type),
				csvQuote(tc.Priority),
				csvQuote(tc.Description),
				csvQuote(precond),
				csvQuote(strings.Join(stepLines, "\n")),
				csvQuote(strings.Join(expectedLines, "\n")),
			))
		}
	}
	return sb.String()
}
