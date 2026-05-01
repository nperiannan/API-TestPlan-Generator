package web

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v3"
)

// ---------- GUI Test Plan Data Model ----------

// GUITestPlan is the top-level structure for a generated GUI test plan.
type GUITestPlan struct {
	Version     string         `json:"version" yaml:"version"`
	GeneratedAt string         `json:"generatedAt" yaml:"generatedAt"`
	TestType    string         `json:"testType" yaml:"testType"` // "gui"
	SourceInfo  GUISourceInfo  `json:"sourceInfo" yaml:"sourceInfo"`
	Screens     []GUIScreen    `json:"screens" yaml:"screens"`
	Summary     GUITestSummary `json:"summary" yaml:"summary"`
}

// GUISourceInfo captures what inputs were used to generate the test plan.
type GUISourceInfo struct {
	FeatureName      string `json:"featureName" yaml:"featureName"`
	Category         string `json:"category" yaml:"category"`
	APITestPlan      string `json:"apiTestPlan" yaml:"apiTestPlan"` // path to API test plan
	FigmaFileKey     string `json:"figmaFileKey" yaml:"figmaFileKey"`
	FigmaNodeID      string `json:"figmaNodeId" yaml:"figmaNodeId"`
	FigmaSection     string `json:"figmaSection" yaml:"figmaSection"`
	JiraIssueKey     string `json:"jiraIssueKey,omitempty" yaml:"jiraIssueKey,omitempty"`
	JiraIssueSummary string `json:"jiraIssueSummary,omitempty" yaml:"jiraIssueSummary,omitempty"`
}

// GUIScreen groups test cases by screen.
type GUIScreen struct {
	ScreenName string        `json:"screenName" yaml:"screenName"`
	Widgets    []GUIWidget   `json:"widgets" yaml:"widgets"`
	Tests      []GUITestCase `json:"tests" yaml:"tests"`
}

// GUIWidget describes a widget found on a screen.
type GUIWidget struct {
	Name       string `json:"name" yaml:"name"`
	WidgetType string `json:"widgetType" yaml:"widgetType"`
	TypeName   string `json:"typeName" yaml:"typeName"`
	Selector   string `json:"selector,omitempty" yaml:"selector,omitempty"`
}

// GUITestCase is a single GUI test case.
type GUITestCase struct {
	TestCaseID  string        `json:"testCaseID" yaml:"testCaseID"`
	FeatureName string        `json:"featureName" yaml:"featureName"`
	Priority    string        `json:"priority" yaml:"priority"`
	Automation  string        `json:"automation" yaml:"automation"`
	Type        string        `json:"type" yaml:"type"` // functional, boundary, negative
	Description string        `json:"description" yaml:"description"`
	Screen      string        `json:"screen" yaml:"screen"`
	Widget      string        `json:"widget" yaml:"widget"`
	WidgetType  string        `json:"widgetType" yaml:"widgetType"`
	Steps       []GUITestStep `json:"steps" yaml:"steps"`
}

// GUITestStep is a single step within a GUI test case.
type GUITestStep struct {
	StepNumber  int      `json:"stepNumber" yaml:"stepNumber"`
	Action      string   `json:"action" yaml:"action"`
	Target      string   `json:"target" yaml:"target"`
	Value       string   `json:"value,omitempty" yaml:"value,omitempty"`
	Expected    string   `json:"expected" yaml:"expected"`
	Validations []string `json:"validations,omitempty" yaml:"validations,omitempty"`
}

// GUITestSummary has aggregate stats.
type GUITestSummary struct {
	TotalScreens    int            `json:"totalScreens" yaml:"totalScreens"`
	TotalWidgets    int            `json:"totalWidgets" yaml:"totalWidgets"`
	TotalTests      int            `json:"totalTests" yaml:"totalTests"`
	TestsByCategory map[string]int `json:"testsByCategory" yaml:"testsByCategory"`
	TestsByPriority map[string]int `json:"testsByPriority" yaml:"testsByPriority"`
	TestsByScreen   map[string]int `json:"testsByScreen" yaml:"testsByScreen"`
}

// ---------- GUI Test Plan Generation ----------

// GUIGenRequest is the request body for POST /api/gui/generate
type GUIGenRequest struct {
	Category     string `json:"category"`     // e.g. "global-profile"
	Feature      string `json:"feature"`      // e.g. "radius-server"
	JiraIssueKey string `json:"jiraIssueKey"` // optional, e.g. "UX-2404"
}

const guiTestPlanMinIOPrefix = "gui-testplans/"

// handleGUIGenerate generates a GUI test plan by combining:
//   - Figma import (screens + widgets)
//   - Widget catalog (test actions per widget type)
//   - API test plan (field names, values, constraints)
//   - Jira story (optional — acceptance criteria)
func (s *Server) handleGUIGenerate(c *gin.Context) {
	var req GUIGenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "provide category and feature: " + err.Error()})
		return
	}
	if req.Category == "" || req.Feature == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "category and feature are required"})
		return
	}

	log.Printf("GUI test gen: starting for %s/%s (jira: %s)", req.Category, req.Feature, req.JiraIssueKey)

	// 1. Load API test plan from disk
	apiPlanPath := filepath.Join(s.config.TestPlansDir, req.Category, req.Feature+".yaml")
	apiPlanData, err := os.ReadFile(apiPlanPath)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("API test plan not found: %s/%s.yaml", req.Category, req.Feature)})
		return
	}

	// Parse API test plan to extract field metadata
	var apiPlan map[string]interface{}
	if err := yaml.Unmarshal(apiPlanData, &apiPlan); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to parse API test plan"})
		return
	}
	fields := extractFieldsFromAPIPlan(apiPlan)

	// 2. Load Figma import results
	figmaImport, err := s.store.LoadFigmaImport()
	if err != nil || figmaImport == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no Figma import found — import Figma components first"})
		return
	}

	// 3. Load widget types
	widgetTypes, err := s.store.ListWidgetTypes()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load widget types"})
		return
	}
	typeMap := make(map[string]WidgetType)
	for _, wt := range widgetTypes {
		typeMap[wt.ID] = wt
	}

	// 4. Optionally fetch Jira story
	var jiraSummary, jiraDescription string
	var acceptanceCriteria []string
	if req.JiraIssueKey != "" {
		jiraSummary, jiraDescription, acceptanceCriteria = s.fetchJiraStoryDetails(req.JiraIssueKey)
	}

	// 5. Build the mapping: Figma widgets → widget types → test actions
	// Use the auto-mapping from the import results
	mappingByName := make(map[string]FigmaMappingRow)
	for _, m := range figmaImport.MappingDetails {
		mappingByName[m.ComponentName] = m
	}

	// 6. Group widgets by screen
	screenWidgets := make(map[string][]FigmaComponent)
	for _, comp := range figmaImport.Components {
		screenWidgets[comp.Screen] = append(screenWidgets[comp.Screen], comp)
	}

	// 7. Generate test cases per screen
	tcCounter := 1
	var screens []GUIScreen
	catCounts := map[string]int{}
	priCounts := map[string]int{}
	screenCounts := map[string]int{}
	totalWidgets := 0

	for _, screenDef := range figmaImport.Screens {
		screenName := screenDef.Name
		widgets := screenWidgets[screenName]
		if len(widgets) == 0 {
			continue
		}

		var guiWidgets []GUIWidget
		var tests []GUITestCase

		// Track which widget names we've already processed on this screen
		processed := make(map[string]bool)

		for _, w := range widgets {
			if processed[w.Name] {
				continue
			}
			processed[w.Name] = true

			mapping, hasMapped := mappingByName[w.Name]
			if !hasMapped || mapping.MappedType == "" {
				// Unmapped widget — still include it with basic visibility test
				guiWidgets = append(guiWidgets, GUIWidget{
					Name:       w.Name,
					WidgetType: "unknown",
					TypeName:   "Unknown",
					Selector:   fmt.Sprintf("[data-figma-id='%s']", w.ComponentID),
				})
				tc := GUITestCase{
					TestCaseID:  fmt.Sprintf("GUI_%s_%04d", strings.ToUpper(toSlug(req.Feature)), tcCounter),
					FeatureName: req.Feature,
					Priority:    "P2",
					Automation:  "Automatable",
					Type:        "functional",
					Description: fmt.Sprintf("Verify %q is visible on %s screen", w.Name, screenName),
					Screen:      screenName,
					Widget:      w.Name,
					WidgetType:  "unknown",
					Steps: []GUITestStep{
						{StepNumber: 1, Action: "navigate", Target: screenName, Expected: fmt.Sprintf("%s screen is displayed", screenName)},
						{StepNumber: 2, Action: "verify_visible", Target: w.Name, Expected: fmt.Sprintf("%q widget is visible", w.Name)},
					},
				}
				tests = append(tests, tc)
				catCounts["functional"]++
				priCounts["P2"]++
				tcCounter++
				continue
			}

			wt, hasType := typeMap[mapping.MappedType]
			if !hasType {
				continue
			}

			guiWidgets = append(guiWidgets, GUIWidget{
				Name:       w.Name,
				WidgetType: wt.ID,
				TypeName:   wt.Name,
				Selector:   fmt.Sprintf("[data-figma-id='%s']", w.ComponentID),
			})

			// Generate test cases from widget type's test actions
			for _, action := range wt.TestActions {
				tc := buildGUITestCase(
					req.Feature, screenName, w.Name, wt,
					action, fields, tcCounter,
				)
				tests = append(tests, tc)
				catCounts[action.Category]++
				priCounts[action.Priority]++
				tcCounter++
			}
		}

		totalWidgets += len(guiWidgets)
		screenCounts[screenName] = len(tests)
		screens = append(screens, GUIScreen{
			ScreenName: screenName,
			Widgets:    guiWidgets,
			Tests:      tests,
		})
	}

	// 8. Add acceptance criteria tests from Jira if available
	if len(acceptanceCriteria) > 0 && len(screens) > 0 {
		for _, ac := range acceptanceCriteria {
			tc := GUITestCase{
				TestCaseID:  fmt.Sprintf("GUI_%s_%04d", strings.ToUpper(toSlug(req.Feature)), tcCounter),
				FeatureName: req.Feature,
				Priority:    "P1",
				Automation:  "Automatable",
				Type:        "functional",
				Description: fmt.Sprintf("Acceptance criteria: %s", ac),
				Screen:      screens[0].ScreenName,
				Widget:      "",
				WidgetType:  "acceptance-criteria",
				Steps: []GUITestStep{
					{StepNumber: 1, Action: "navigate", Target: screens[0].ScreenName, Expected: "Screen is displayed"},
					{StepNumber: 2, Action: "verify", Target: "acceptance criteria", Value: ac, Expected: ac},
				},
			}
			screens[0].Tests = append(screens[0].Tests, tc)
			catCounts["functional"]++
			priCounts["P1"]++
			screenCounts[screens[0].ScreenName]++
			tcCounter++
		}
	}

	// Sort screens by name
	sort.Slice(screens, func(i, j int) bool { return screens[i].ScreenName < screens[j].ScreenName })

	totalTests := tcCounter - 1
	plan := &GUITestPlan{
		Version:     "1.0",
		GeneratedAt: time.Now().Format(time.RFC3339),
		TestType:    "gui",
		SourceInfo: GUISourceInfo{
			FeatureName:      req.Feature,
			Category:         req.Category,
			APITestPlan:      fmt.Sprintf("%s/%s.yaml", req.Category, req.Feature),
			FigmaFileKey:     figmaImport.FileKey,
			FigmaNodeID:      figmaImport.NodeID,
			FigmaSection:     figmaImport.SectionName,
			JiraIssueKey:     req.JiraIssueKey,
			JiraIssueSummary: jiraSummary,
		},
		Screens: screens,
		Summary: GUITestSummary{
			TotalScreens:    len(screens),
			TotalWidgets:    totalWidgets,
			TotalTests:      totalTests,
			TestsByCategory: catCounts,
			TestsByPriority: priCounts,
			TestsByScreen:   screenCounts,
		},
	}

	// 9. Save to disk as YAML
	outDir := filepath.Join(s.config.TestPlansDir, req.Category)
	os.MkdirAll(outDir, 0755)
	outPath := filepath.Join(outDir, req.Feature+"-gui.yaml")
	yamlData, err := yaml.Marshal(plan)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to marshal GUI test plan"})
		return
	}
	if err := os.WriteFile(outPath, yamlData, 0644); err != nil {
		log.Printf("Warning: failed to write GUI test plan to disk: %v", err)
	} else {
		log.Printf("GUI test plan written to %s", outPath)
	}

	// 10. Save to MinIO
	minioKey := guiTestPlanMinIOPrefix + req.Category + "/" + req.Feature + "-gui.json"
	jsonData, _ := json.Marshal(plan)
	if err := s.store.saveJSON(minioKey, plan); err != nil {
		log.Printf("Warning: failed to save GUI test plan to MinIO: %v", err)
	}

	_ = jiraDescription // used for context enrichment in future
	_ = jsonData

	log.Printf("GUI test gen complete: %s/%s-gui — %d screens, %d widgets, %d tests",
		req.Category, req.Feature, len(screens), totalWidgets, totalTests)

	c.JSON(http.StatusOK, gin.H{
		"plan":       plan,
		"filePath":   fmt.Sprintf("%s/%s-gui.yaml", req.Category, req.Feature),
		"totalTests": totalTests,
	})
}

// handleGUITestPlanList lists available GUI test plans
func (s *Server) handleGUITestPlanList(c *gin.Context) {
	var guiPlans []map[string]interface{}

	files, err := listTestPlanFilesFromDisk(s.config.TestPlansDir)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	for _, f := range files {
		if strings.HasSuffix(f.Feature, "-gui") {
			guiPlans = append(guiPlans, map[string]interface{}{
				"category": f.Category,
				"feature":  f.Feature,
				"fileName": f.FileName,
				"path":     f.Path,
				"size":     f.Size,
			})
		}
	}

	c.JSON(http.StatusOK, gin.H{"plans": guiPlans, "total": len(guiPlans)})
}

// ---------- Helper Functions ----------

// buildGUITestCase creates a test case from a widget's test action, enriched with API field data.
func buildGUITestCase(feature, screen, widgetName string, wt WidgetType, action TestAction, fields []apiField, counter int) GUITestCase {
	tcID := fmt.Sprintf("GUI_%s_%04d", strings.ToUpper(toSlug(feature)), counter)

	// Find matching field from API test plan
	matchedField := findMatchingField(widgetName, fields)

	steps := buildTestSteps(screen, widgetName, wt, action, matchedField)

	return GUITestCase{
		TestCaseID:  tcID,
		FeatureName: feature,
		Priority:    action.Priority,
		Automation:  "Automatable",
		Type:        action.Category,
		Description: fmt.Sprintf("%s — %s on %s screen (%s)", action.Description, widgetName, screen, wt.Name),
		Screen:      screen,
		Widget:      widgetName,
		WidgetType:  wt.ID,
		Steps:       steps,
	}
}

// buildTestSteps generates test steps appropriate for the widget type and action.
func buildTestSteps(screen, widgetName string, wt WidgetType, action TestAction, field *apiField) []GUITestStep {
	steps := []GUITestStep{
		{StepNumber: 1, Action: "navigate", Target: screen, Expected: fmt.Sprintf("%s screen is displayed", screen)},
	}

	switch action.Name {
	case "type_text":
		value := "test-value"
		if field != nil && field.SampleValue != "" {
			value = field.SampleValue
		}
		steps = append(steps,
			GUITestStep{StepNumber: 2, Action: "click", Target: widgetName, Expected: fmt.Sprintf("%s field is focused", widgetName)},
			GUITestStep{StepNumber: 3, Action: "type", Target: widgetName, Value: value, Expected: fmt.Sprintf("Value %q is entered in %s", value, widgetName)},
			GUITestStep{StepNumber: 4, Action: "verify_value", Target: widgetName, Value: value, Expected: fmt.Sprintf("Field displays %q", value)},
		)

	case "clear_text":
		steps = append(steps,
			GUITestStep{StepNumber: 2, Action: "click", Target: widgetName, Expected: fmt.Sprintf("%s field is focused", widgetName)},
			GUITestStep{StepNumber: 3, Action: "clear", Target: widgetName, Expected: "Field is cleared"},
			GUITestStep{StepNumber: 4, Action: "verify_value", Target: widgetName, Value: "", Expected: "Field is empty"},
		)

	case "test_max_length":
		maxLen := "255"
		if field != nil && field.MaxLength != "" {
			maxLen = field.MaxLength
		}
		steps = append(steps,
			GUITestStep{StepNumber: 2, Action: "click", Target: widgetName, Expected: fmt.Sprintf("%s field is focused", widgetName)},
			GUITestStep{StepNumber: 3, Action: "type", Target: widgetName, Value: fmt.Sprintf("[string exceeding %s chars]", maxLen), Expected: "Text is truncated or error shown"},
			GUITestStep{StepNumber: 4, Action: "verify", Target: widgetName, Expected: fmt.Sprintf("Field enforces max length of %s", maxLen)},
		)

	case "test_empty":
		steps = append(steps,
			GUITestStep{StepNumber: 2, Action: "clear", Target: widgetName, Expected: "Field is empty"},
			GUITestStep{StepNumber: 3, Action: "click", Target: "Submit / Save button", Expected: "Form submission attempted"},
			GUITestStep{StepNumber: 4, Action: "verify_error", Target: widgetName, Expected: "Validation error is shown for required field"},
		)

	case "test_special_chars":
		steps = append(steps,
			GUITestStep{StepNumber: 2, Action: "type", Target: widgetName, Value: `<script>alert('xss')</script>`, Expected: "Special characters handled safely"},
			GUITestStep{StepNumber: 3, Action: "verify", Target: widgetName, Expected: "Input is sanitized or rejected"},
		)

	case "select_option", "select_multiple":
		value := "Option 1"
		if field != nil && field.SampleValue != "" {
			value = field.SampleValue
		}
		steps = append(steps,
			GUITestStep{StepNumber: 2, Action: "click", Target: widgetName, Expected: "Dropdown opens"},
			GUITestStep{StepNumber: 3, Action: "select", Target: widgetName, Value: value, Expected: fmt.Sprintf("%q is selected", value)},
			GUITestStep{StepNumber: 4, Action: "verify_selected", Target: widgetName, Value: value, Expected: fmt.Sprintf("Selected value is %q", value)},
		)

	case "check", "uncheck":
		expected := "checked"
		if action.Name == "uncheck" {
			expected = "unchecked"
		}
		steps = append(steps,
			GUITestStep{StepNumber: 2, Action: action.Name, Target: widgetName, Expected: fmt.Sprintf("Checkbox is %s", expected)},
			GUITestStep{StepNumber: 3, Action: "verify_state", Target: widgetName, Value: expected, Expected: fmt.Sprintf("Checkbox state is %s", expected)},
		)

	case "click":
		steps = append(steps,
			GUITestStep{StepNumber: 2, Action: "click", Target: widgetName, Expected: "Button click action is triggered"},
			GUITestStep{StepNumber: 3, Action: "verify", Target: "page state", Expected: "Expected action occurred (dialog/navigation/form submit)"},
		)

	case "verify_visible", "verify_value", "verify_selected", "verify_state",
		"verify_options", "verify_disabled", "verify_placeholder":
		steps = append(steps,
			GUITestStep{StepNumber: 2, Action: action.Name, Target: widgetName, Expected: action.Description},
		)

	case "toggle_on", "toggle_off":
		state := "ON"
		if action.Name == "toggle_off" {
			state = "OFF"
		}
		steps = append(steps,
			GUITestStep{StepNumber: 2, Action: "click", Target: widgetName, Expected: fmt.Sprintf("Toggle is switched %s", state)},
			GUITestStep{StepNumber: 3, Action: "verify_state", Target: widgetName, Value: state, Expected: fmt.Sprintf("Toggle shows %s state", state)},
		)

	case "search_option":
		steps = append(steps,
			GUITestStep{StepNumber: 2, Action: "click", Target: widgetName, Expected: "Dropdown/search opens"},
			GUITestStep{StepNumber: 3, Action: "type", Target: widgetName + " search", Value: "search term", Expected: "Options are filtered"},
			GUITestStep{StepNumber: 4, Action: "verify", Target: "filtered results", Expected: "Only matching options are shown"},
		)

	case "test_no_selection":
		steps = append(steps,
			GUITestStep{StepNumber: 2, Action: "verify", Target: widgetName, Expected: "No option is selected"},
			GUITestStep{StepNumber: 3, Action: "click", Target: "Submit / Save button", Expected: "Form submission attempted"},
			GUITestStep{StepNumber: 4, Action: "verify_error", Target: widgetName, Expected: "Required field validation error shown"},
		)

	default:
		// Generic action
		steps = append(steps,
			GUITestStep{StepNumber: 2, Action: action.Name, Target: widgetName, Expected: action.Description},
		)
	}

	return steps
}

// apiField captures field metadata extracted from the API test plan.
type apiField struct {
	Name        string
	Type        string
	SampleValue string
	MaxLength   string
	MinLength   string
	Required    bool
	Origin      string
}

// extractFieldsFromAPIPlan parses the API test plan YAML and extracts field metadata.
func extractFieldsFromAPIPlan(plan map[string]interface{}) []apiField {
	var fields []apiField
	seen := make(map[string]bool)

	features, ok := plan["features"]
	if !ok {
		return fields
	}

	featureList, ok := features.([]interface{})
	if !ok {
		return fields
	}

	for _, f := range featureList {
		fMap, ok := f.(map[string]interface{})
		if !ok {
			continue
		}
		tests, ok := fMap["tests"].(map[string]interface{})
		if !ok {
			continue
		}
		// Walk all test categories
		for _, testList := range tests {
			tcList, ok := testList.([]interface{})
			if !ok {
				continue
			}
			for _, tc := range tcList {
				tcMap, ok := tc.(map[string]interface{})
				if !ok {
					continue
				}
				steps, ok := tcMap["steps"].([]interface{})
				if !ok {
					continue
				}
				for _, step := range steps {
					stepMap, ok := step.(map[string]interface{})
					if !ok {
						continue
					}
					body, ok := stepMap["body"].(map[string]interface{})
					if !ok {
						continue
					}
					objects, ok := body["objects"].([]interface{})
					if !ok {
						continue
					}
					for _, obj := range objects {
						objMap, ok := obj.(map[string]interface{})
						if !ok {
							continue
						}
						props, ok := objMap["properties"].([]interface{})
						if !ok {
							continue
						}
						for _, prop := range props {
							propMap, ok := prop.(map[string]interface{})
							if !ok {
								continue
							}
							name := fmt.Sprintf("%v", propMap["name"])
							if seen[name] {
								continue
							}
							seen[name] = true
							af := apiField{
								Name:   name,
								Type:   fmt.Sprintf("%v", propMap["type"]),
								Origin: fmt.Sprintf("%v", propMap["origin"]),
							}
							if v, ok := propMap["value"]; ok {
								af.SampleValue = fmt.Sprintf("%v", v)
							}
							fields = append(fields, af)
						}
					}
				}
			}
		}
	}
	return fields
}

// findMatchingField tries to match a Figma widget name to an API field.
func findMatchingField(widgetName string, fields []apiField) *apiField {
	lowerWidget := strings.ToLower(widgetName)

	// Direct name match
	for i := range fields {
		if strings.EqualFold(fields[i].Name, widgetName) {
			return &fields[i]
		}
	}

	// Partial match: widget name contains field name or vice versa
	for i := range fields {
		lowerField := strings.ToLower(fields[i].Name)
		if strings.Contains(lowerWidget, lowerField) || strings.Contains(lowerField, lowerWidget) {
			return &fields[i]
		}
	}

	// Slug match
	widgetSlug := toSlug(widgetName)
	for i := range fields {
		fieldSlug := toSlug(fields[i].Name)
		if widgetSlug == fieldSlug || strings.Contains(widgetSlug, fieldSlug) || strings.Contains(fieldSlug, widgetSlug) {
			return &fields[i]
		}
	}

	return nil
}

// fetchJiraStoryDetails fetches summary, description text, and acceptance criteria from a Jira issue.
func (s *Server) fetchJiraStoryDetails(issueKey string) (summary, description string, acceptanceCriteria []string) {
	issueKeyRe := regexp.MustCompile(`^[A-Z]+-\d+$`)
	if !issueKeyRe.MatchString(issueKey) {
		return "", "", nil
	}

	jcfg, err := s.loadJiraConfig()
	if err != nil {
		log.Printf("GUI test gen: Jira not configured, skipping: %v", err)
		return "", "", nil
	}

	result, _, err := s.fetchJiraIssue(jcfg, issueKey)
	if err != nil {
		log.Printf("GUI test gen: failed to fetch Jira issue %s: %v", issueKey, err)
		return "", "", nil
	}

	// Parse the result
	data, err := json.Marshal(result)
	if err != nil {
		return "", "", nil
	}

	var issue struct {
		Fields struct {
			Summary     string      `json:"summary"`
			Description interface{} `json:"description"`
		} `json:"fields"`
	}
	if err := json.Unmarshal(data, &issue); err != nil {
		return "", "", nil
	}

	summary = issue.Fields.Summary

	// Extract text from Atlassian Document Format description
	if issue.Fields.Description != nil {
		description = extractADFText(issue.Fields.Description)
		acceptanceCriteria = extractAcceptanceCriteria(description)
	}

	log.Printf("GUI test gen: Jira %s — %q, %d acceptance criteria", issueKey, summary, len(acceptanceCriteria))
	return summary, description, acceptanceCriteria
}

// extractADFText extracts plain text from Atlassian Document Format.
func extractADFText(node interface{}) string {
	switch v := node.(type) {
	case map[string]interface{}:
		if text, ok := v["text"].(string); ok {
			return text
		}
		if content, ok := v["content"].([]interface{}); ok {
			var parts []string
			for _, child := range content {
				if t := extractADFText(child); t != "" {
					parts = append(parts, t)
				}
			}
			nodeType, _ := v["type"].(string)
			if nodeType == "paragraph" || nodeType == "heading" || nodeType == "listItem" {
				return strings.Join(parts, " ") + "\n"
			}
			return strings.Join(parts, "")
		}
	}
	return ""
}

// extractAcceptanceCriteria looks for lines starting with AC patterns.
func extractAcceptanceCriteria(text string) []string {
	var criteria []string
	lines := strings.Split(text, "\n")
	acPattern := regexp.MustCompile(`(?i)^(?:ac\s*\d*[:.)\-]?\s*|acceptance\s+criteria?\s*[:.)\-]?\s*|given\s+|when\s+|then\s+|should\s+|must\s+|\*\s+|\-\s+)(.+)`)

	inACSection := false
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		lower := strings.ToLower(line)
		if strings.Contains(lower, "acceptance criteria") || strings.Contains(lower, "acceptance criterion") {
			inACSection = true
			continue
		}
		if inACSection {
			if matches := acPattern.FindStringSubmatch(line); len(matches) > 1 {
				criteria = append(criteria, strings.TrimSpace(matches[1]))
			} else if strings.HasPrefix(line, "-") || strings.HasPrefix(line, "*") || strings.HasPrefix(line, "•") {
				criteria = append(criteria, strings.TrimSpace(strings.TrimLeft(line, "-*• ")))
			}
		}
	}

	// If no AC section found, try to find individual AC lines anywhere
	if len(criteria) == 0 {
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if matched, _ := regexp.MatchString(`(?i)^ac\s*\d+`, line); matched {
				criteria = append(criteria, line)
			}
		}
	}

	return criteria
}
