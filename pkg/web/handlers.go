package web

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
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

	version, err := s.version.CreateVersion(req.Tag, req.Description, files)
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
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported format, use yaml/json/csv"})
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

// handleGenerate triggers test plan generation (placeholder)
func (s *Server) handleGenerate(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "generation queued",
		"message": "Test plan generation has been triggered. Check back for results.",
	})
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
	TotalFeatures  int                       `json:"totalFeatures"`
	TotalTests     int                       `json:"totalTests"`
	TestsByCategory map[string]int           `json:"testsByCategory"`
	FeaturesByGroup map[string][]FeatureInfo `json:"featuresByGroup"`
	GeneratedAt    string                    `json:"generatedAt"`
}

// FeatureInfo is a summary of a single feature's test counts
type FeatureInfo struct {
	Name           string         `json:"name"`
	Category       string         `json:"category"`
	TotalTests     int            `json:"totalTests"`
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
			Name:           featureName,
			Category:       group,
			TotalTests:     counts.total,
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
	sb.WriteString("TestCaseID,FeatureName,Type,Priority,Automation,Description,IsDeploymentTest\n")

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
				sb.WriteString(fmt.Sprintf("%s,%s,%s,%s,%s,%q,%v\n",
					safeStr(tc["testCaseID"]),
					safeStr(tc["featureName"]),
					safeStr(tc["type"]),
					safeStr(tc["priority"]),
					safeStr(tc["automation"]),
					safeStr(tc["description"]),
					tc["isDeploymentTest"],
				))
			}
		}
	}

	return sb.String()
}

func safeStr(v interface{}) string {
	if v == nil {
		return ""
	}
	return fmt.Sprintf("%v", v)
}
