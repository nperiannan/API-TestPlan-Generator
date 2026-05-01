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

	"github.com/extremenetworks/testcase-generator/pkg/model"
	"github.com/extremenetworks/testcase-generator/pkg/yang"
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
	YangModel        string `json:"yangModel,omitempty" yaml:"yangModel,omitempty"`
	YangFields       int    `json:"yangFields,omitempty" yaml:"yangFields,omitempty"`
	YangKeys         string `json:"yangKeys,omitempty" yaml:"yangKeys,omitempty"`
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
//   - API test plan (functional scenarios: CRUD, validation, boundary)
//   - Figma import (screens + widgets — used for step targets)
//   - Widget catalog (widget type awareness)
//   - Jira story (optional — acceptance criteria)
//
// The resulting tests are FUNCTIONAL tests exercised through the GUI,
// not widget-level GUI tests.
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

	// 1. Load and parse API test plan
	apiPlanPath := filepath.Join(s.config.TestPlansDir, req.Category, req.Feature+".yaml")
	apiPlanData, err := os.ReadFile(apiPlanPath)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("API test plan not found: %s/%s.yaml", req.Category, req.Feature)})
		return
	}
	var apiPlan map[string]interface{}
	if err := yaml.Unmarshal(apiPlanData, &apiPlan); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to parse API test plan"})
		return
	}
	fields := extractFieldsFromAPIPlan(apiPlan)
	apiTestCases := extractAPITestCases(apiPlan)
	featureLabel := humanize(req.Feature) // "radius-server" -> "Radius Server"

	// 1b. Load YANG model for this feature and enrich field metadata
	yangFeature := s.loadYangFeature(req.Feature)
	if yangFeature != nil {
		fields = enrichFieldsWithYANG(fields, yangFeature)
		log.Printf("GUI test gen: enriched %d fields with YANG data (keys=%v, params=%d)",
			len(fields), yangFeature.Keys, len(yangFeature.Parameters))
		// Also enrich fields inside each apiTestCaseInfo
		for i := range apiTestCases {
			apiTestCases[i].Fields = enrichFieldsWithYANG(apiTestCases[i].Fields, yangFeature)
		}
	}

	// 2. Load Figma import results (screens + widgets)
	figmaImport, err := s.store.LoadFigmaImport()
	if err != nil || figmaImport == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no Figma import found — import Figma components first"})
		return
	}

	// Build screen inventory from Figma
	screenNames := []string{}
	screenWidgets := map[string][]FigmaComponent{}
	for _, sd := range figmaImport.Screens {
		screenNames = append(screenNames, sd.Name)
	}
	for _, comp := range figmaImport.Components {
		screenWidgets[comp.Screen] = append(screenWidgets[comp.Screen], comp)
	}

	// Classify screens by purpose (Add/Edit/Delete/Index/Detail)
	screensByRole := classifyScreens(screenNames, featureLabel)

	// 3. Load widget types for mapping awareness
	widgetTypes, _ := s.store.ListWidgetTypes()
	typeMap := make(map[string]WidgetType)
	for _, wt := range widgetTypes {
		typeMap[wt.ID] = wt
	}
	mappingByName := make(map[string]FigmaMappingRow)
	for _, m := range figmaImport.MappingDetails {
		mappingByName[m.ComponentName] = m
	}

	// 4. Optionally fetch Jira story
	var jiraSummary, jiraDescription string
	var acceptanceCriteria []string
	if req.JiraIssueKey != "" {
		jiraSummary, jiraDescription, acceptanceCriteria = s.fetchJiraStoryDetails(req.JiraIssueKey)
	}

	// 5. Generate functional GUI test cases from API scenarios
	tcCounter := 1
	featureSlug := strings.ToUpper(toSlug(req.Feature))
	var allTests []GUITestCase
	catCounts := map[string]int{}
	priCounts := map[string]int{}

	// --- A. CRUD Workflow Tests (derived from API functional tests) ---
	for _, apiTC := range apiTestCases {
		guiTC := translateAPIToGUI(apiTC, featureLabel, featureSlug, screensByRole,
			fields, screenWidgets, mappingByName, &tcCounter)
		if guiTC != nil {
			allTests = append(allTests, *guiTC)
			catCounts[guiTC.Type]++
			priCounts[guiTC.Priority]++
		}
	}

	// --- B. Form Validation Tests (from negative API tests → GUI form errors) ---
	for _, apiTC := range apiTestCases {
		if apiTC.Type != "negative" {
			continue
		}
		guiTC := translateNegativeToGUI(apiTC, featureLabel, featureSlug, screensByRole,
			fields, screenWidgets, mappingByName, &tcCounter)
		if guiTC != nil {
			allTests = append(allTests, *guiTC)
			catCounts[guiTC.Type]++
			priCounts[guiTC.Priority]++
		}
	}

	// --- C. Field Boundary Tests via GUI (from boundary API tests) ---
	for _, apiTC := range apiTestCases {
		if apiTC.Type != "boundary" {
			continue
		}
		guiTC := translateBoundaryToGUI(apiTC, featureLabel, featureSlug, screensByRole,
			fields, screenWidgets, mappingByName, &tcCounter)
		if guiTC != nil {
			allTests = append(allTests, *guiTC)
			catCounts[guiTC.Type]++
			priCounts[guiTC.Priority]++
		}
	}

	// --- D. Acceptance Criteria Tests from Jira ---
	if len(acceptanceCriteria) > 0 {
		listScreen := screensByRole["list"]
		if listScreen == "" && len(screenNames) > 0 {
			listScreen = screenNames[0]
		}
		for _, ac := range acceptanceCriteria {
			tc := GUITestCase{
				TestCaseID:  fmt.Sprintf("GUI_%s_%04d", featureSlug, tcCounter),
				FeatureName: req.Feature,
				Priority:    "P1",
				Automation:  "Manual",
				Type:        "functional",
				Description: fmt.Sprintf("Verify acceptance criteria: %s", ac),
				Screen:      listScreen,
				Widget:      "",
				WidgetType:  "acceptance-criteria",
				Steps: []GUITestStep{
					{StepNumber: 1, Action: "navigate", Target: listScreen, Expected: fmt.Sprintf("%s screen is displayed", listScreen)},
					{StepNumber: 2, Action: "verify", Target: "UI behavior", Expected: ac},
				},
			}
			allTests = append(allTests, tc)
			catCounts["functional"]++
			priCounts["P1"]++
			tcCounter++
		}
	}

	// --- E. YANG-driven GUI tests (default values, key field immutability, enum dropdowns) ---
	if yangFeature != nil {
		addScreen := screensByRole["add"]
		if addScreen == "" {
			addScreen = pickScreen(screensByRole, "add", "list")
		}
		editScreen := screensByRole["edit"]
		if editScreen == "" {
			editScreen = pickScreen(screensByRole, "edit", "list")
		}

		// Test: Verify default values are pre-filled in Add form
		var defaultFields []string
		for _, f := range fields {
			if f.DefaultValue != "" {
				defaultFields = append(defaultFields, f.Name)
			}
		}
		if len(defaultFields) > 0 {
			var defaultSteps []GUITestStep
			defaultSteps = append(defaultSteps, GUITestStep{
				StepNumber: 1, Action: "navigate", Target: addScreen,
				Expected: fmt.Sprintf("%s screen is displayed", addScreen),
			})
			defaultSteps = append(defaultSteps, GUITestStep{
				StepNumber: 2, Action: "click", Target: "Add / Create button",
				Expected: "Add form opens with default values pre-filled",
			})
			stepN := 3
			for _, f := range fields {
				if f.DefaultValue != "" && stepN <= 7 {
					defaultSteps = append(defaultSteps, GUITestStep{
						StepNumber: stepN, Action: "verify_prefilled", Target: f.Name,
						Value: f.DefaultValue,
						Expected: fmt.Sprintf("'%s' field shows YANG default value '%s'", f.Name, f.DefaultValue),
					})
					stepN++
				}
			}
			tc := GUITestCase{
				TestCaseID:  fmt.Sprintf("GUI_%s_%04d", featureSlug, tcCounter),
				FeatureName: req.Feature,
				Priority:    "P2",
				Automation:  "Automatable",
				Type:        "functional",
				Description: fmt.Sprintf("Open Add %s form and verify YANG default values are pre-filled for: %s", featureLabel, strings.Join(defaultFields, ", ")),
				Screen:      addScreen,
				Steps:       defaultSteps,
			}
			allTests = append(allTests, tc)
			catCounts["functional"]++
			priCounts["P2"]++
			tcCounter++
		}

		// Test: Verify key fields are non-editable on Edit form
		if len(yangFeature.Keys) > 0 && editScreen != "" {
			var keySteps []GUITestStep
			keySteps = append(keySteps,
				GUITestStep{StepNumber: 1, Action: "navigate", Target: pickScreen(screensByRole, "list", "add"),
					Expected: fmt.Sprintf("%s list screen is displayed", featureLabel)},
				GUITestStep{StepNumber: 2, Action: "select_row", Target: fmt.Sprintf("existing %s", featureLabel),
					Expected: "Row is selected"},
				GUITestStep{StepNumber: 3, Action: "click", Target: "Edit button",
					Expected: fmt.Sprintf("%s opens with current values", editScreen)},
			)
			stepN := 4
			for _, key := range yangFeature.Keys {
				if stepN <= 7 {
					keySteps = append(keySteps, GUITestStep{
						StepNumber: stepN, Action: "verify_readonly", Target: key,
						Expected: fmt.Sprintf("Key field '%s' is read-only/disabled on edit form (YANG list key)", key),
					})
					stepN++
				}
			}
			tc := GUITestCase{
				TestCaseID:  fmt.Sprintf("GUI_%s_%04d", featureSlug, tcCounter),
				FeatureName: req.Feature,
				Priority:    "P1",
				Automation:  "Automatable",
				Type:        "functional",
				Description: fmt.Sprintf("Edit existing %s and verify YANG key fields (%s) are non-editable", featureLabel, strings.Join(yangFeature.Keys, ", ")),
				Screen:      editScreen,
				Steps:       keySteps,
			}
			allTests = append(allTests, tc)
			catCounts["functional"]++
			priCounts["P1"]++
			tcCounter++
		}

		// Test: Verify each enum dropdown shows all valid YANG options
		for _, f := range fields {
			if len(f.EnumValues) > 0 {
				tc := GUITestCase{
					TestCaseID:  fmt.Sprintf("GUI_%s_%04d", featureSlug, tcCounter),
					FeatureName: req.Feature,
					Priority:    "P2",
					Automation:  "Automatable",
					Type:        "functional",
					Description: fmt.Sprintf("Open Add %s form and verify '%s' dropdown contains all valid YANG enum options: %s", featureLabel, f.Name, strings.Join(f.EnumValues, ", ")),
					Screen:      addScreen,
					Steps: []GUITestStep{
						{StepNumber: 1, Action: "navigate", Target: addScreen, Expected: fmt.Sprintf("%s screen is displayed", addScreen)},
						{StepNumber: 2, Action: "click", Target: "Add / Create button", Expected: "Add form opens"},
						{StepNumber: 3, Action: "click", Target: f.Name + " dropdown", Expected: "Dropdown options are displayed"},
						{StepNumber: 4, Action: "verify_options", Target: f.Name, Value: strings.Join(f.EnumValues, ", "), Expected: fmt.Sprintf("Dropdown shows exactly %d options: %s", len(f.EnumValues), strings.Join(f.EnumValues, ", "))},
					},
				}
				allTests = append(allTests, tc)
				catCounts["functional"]++
				priCounts["P2"]++
				tcCounter++
			}
		}

		log.Printf("GUI test gen: added YANG-driven tests (defaults=%d, keys=%d, enums=%d)",
			len(defaultFields), len(yangFeature.Keys), countEnumFields(fields))
	}

	// 6. Group tests by screen for output
	testsByScreen := map[string][]GUITestCase{}
	for _, tc := range allTests {
		testsByScreen[tc.Screen] = append(testsByScreen[tc.Screen], tc)
	}

	var screens []GUIScreen
	screenCounts := map[string]int{}
	totalWidgets := 0
	for _, sName := range screenNames {
		tests := testsByScreen[sName]
		if len(tests) == 0 {
			continue
		}
		widgets := screenWidgets[sName]
		var guiWidgets []GUIWidget
		seen := map[string]bool{}
		for _, w := range widgets {
			if seen[w.Name] {
				continue
			}
			seen[w.Name] = true
			typeName := "Unknown"
			typeID := "unknown"
			if m, ok := mappingByName[w.Name]; ok && m.MappedType != "" {
				if wt, ok2 := typeMap[m.MappedType]; ok2 {
					typeName = wt.Name
					typeID = wt.ID
				}
			}
			guiWidgets = append(guiWidgets, GUIWidget{
				Name:       w.Name,
				WidgetType: typeID,
				TypeName:   typeName,
			})
		}
		totalWidgets += len(guiWidgets)
		screenCounts[sName] = len(tests)
		screens = append(screens, GUIScreen{
			ScreenName: sName,
			Widgets:    guiWidgets,
			Tests:      tests,
		})
	}

	// Add tests for screens not in Figma screens list (fallback)
	for sName, tests := range testsByScreen {
		if screenCounts[sName] > 0 {
			continue
		}
		screenCounts[sName] = len(tests)
		screens = append(screens, GUIScreen{
			ScreenName: sName,
			Tests:      tests,
		})
	}

	sort.Slice(screens, func(i, j int) bool { return screens[i].ScreenName < screens[j].ScreenName })

	totalTests := len(allTests)
	plan := &GUITestPlan{
		Version:     "2.0",
		GeneratedAt: time.Now().Format(time.RFC3339),
		TestType:    "gui-functional",
		SourceInfo: GUISourceInfo{
			FeatureName:      req.Feature,
			Category:         req.Category,
			APITestPlan:      fmt.Sprintf("%s/%s.yaml", req.Category, req.Feature),
			FigmaFileKey:     figmaImport.FileKey,
			FigmaNodeID:      figmaImport.NodeID,
			FigmaSection:     figmaImport.SectionName,
			JiraIssueKey:     req.JiraIssueKey,
			JiraIssueSummary: jiraSummary,
			YangModel:        func() string { if yangFeature != nil { return yangFeature.Name } ; return "" }(),
			YangFields:       func() int { if yangFeature != nil { return len(yangFeature.Parameters) } ; return 0 }(),
			YangKeys:         func() string { if yangFeature != nil { return strings.Join(yangFeature.Keys, ", ") } ; return "" }(),
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

	// 7. Save to disk as YAML
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

	// 8. Save to MinIO
	minioKey := guiTestPlanMinIOPrefix + req.Category + "/" + req.Feature + "-gui.json"
	if err := s.store.saveJSON(minioKey, plan); err != nil {
		log.Printf("Warning: failed to save GUI test plan to MinIO: %v", err)
	}

	_ = jiraDescription

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

// apiTestCaseInfo captures key metadata from an API test case.
type apiTestCaseInfo struct {
	TestCaseID  string
	Type        string // functional, negative, boundary
	Priority    string
	Description string
	Method      string // POST, PUT, PATCH, DELETE, GET
	Fields      []apiField
	Validations []string
}

// extractAPITestCases pulls test case metadata from the parsed API test plan.
func extractAPITestCases(plan map[string]interface{}) []apiTestCaseInfo {
	var results []apiTestCaseInfo

	features, ok := plan["features"]
	if !ok {
		return results
	}
	featureList, ok := features.([]interface{})
	if !ok {
		return results
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
		for category, testList := range tests {
			tcList, ok := testList.([]interface{})
			if !ok {
				continue
			}
			for _, tc := range tcList {
				tcMap, ok := tc.(map[string]interface{})
				if !ok {
					continue
				}
				info := apiTestCaseInfo{
					TestCaseID:  fmt.Sprintf("%v", tcMap["testCaseID"]),
					Type:        category,
					Priority:    fmt.Sprintf("%v", tcMap["priority"]),
					Description: fmt.Sprintf("%v", tcMap["description"]),
				}
				// Extract method and fields from steps
				if steps, ok := tcMap["steps"].([]interface{}); ok {
					for _, step := range steps {
						stepMap, ok := step.(map[string]interface{})
						if !ok {
							continue
						}
						if m, ok := stepMap["method"].(string); ok && info.Method == "" {
							info.Method = m
						}
						if vals, ok := stepMap["validations"].([]interface{}); ok {
							for _, v := range vals {
								info.Validations = append(info.Validations, fmt.Sprintf("%v", v))
							}
						}
						if body, ok := stepMap["body"].(map[string]interface{}); ok {
							if objects, ok := body["objects"].([]interface{}); ok {
								for _, obj := range objects {
									objMap, ok := obj.(map[string]interface{})
									if !ok {
										continue
									}
									if props, ok := objMap["properties"].([]interface{}); ok {
										for _, prop := range props {
											propMap, ok := prop.(map[string]interface{})
											if !ok {
												continue
											}
											af := apiField{
												Name:   fmt.Sprintf("%v", propMap["name"]),
												Type:   fmt.Sprintf("%v", propMap["type"]),
												Origin: fmt.Sprintf("%v", propMap["origin"]),
											}
											if v, ok := propMap["value"]; ok {
												af.SampleValue = fmt.Sprintf("%v", v)
											}
											info.Fields = append(info.Fields, af)
										}
									}
								}
							}
						}
					}
				}
				results = append(results, info)
			}
		}
	}
	return results
}

// classifyScreens maps screen names to roles: add, edit, delete, list, detail, index.
func classifyScreens(screenNames []string, featureLabel string) map[string]string {
	roles := map[string]string{}
	for _, name := range screenNames {
		lower := strings.ToLower(name)
		switch {
		case strings.Contains(lower, "add") || strings.Contains(lower, "create") || strings.Contains(lower, "new"):
			roles["add"] = name
		case strings.Contains(lower, "edit") || strings.Contains(lower, "modify") || strings.Contains(lower, "update"):
			roles["edit"] = name
		case strings.Contains(lower, "delete") || strings.Contains(lower, "remove"):
			roles["delete"] = name
		case strings.Contains(lower, "detail") || strings.Contains(lower, "default detail"):
			roles["detail"] = name
		case strings.Contains(lower, "bulk delete"):
			roles["bulk_delete"] = name
		case strings.Contains(lower, "index"):
			roles["index"] = name
		default:
			// Main list screen is typically the base name
			if roles["list"] == "" {
				roles["list"] = name
			}
		}
	}
	return roles
}

// humanize converts "radius-server" to "Radius Server"
func humanize(slug string) string {
	parts := strings.Split(slug, "-")
	for i, p := range parts {
		if len(p) > 0 {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, " ")
}

// translateAPIToGUI converts a functional API test case into a GUI functional test.
func translateAPIToGUI(apiTC apiTestCaseInfo, featureLabel, featureSlug string,
	screensByRole map[string]string, fields []apiField,
	screenWidgets map[string][]FigmaComponent, mappings map[string]FigmaMappingRow,
	counter *int) *GUITestCase {

	if apiTC.Type != "functional" {
		return nil
	}

	desc := apiTC.Description
	lowerDesc := strings.ToLower(desc)

	var screen, guiDesc string
	var steps []GUITestStep

	switch {
	case strings.Contains(lowerDesc, "create") && strings.Contains(lowerDesc, "delete") && strings.Contains(lowerDesc, "update"):
		// Full CRUD lifecycle
		screen = pickScreen(screensByRole, "add", "list")
		guiDesc = fmt.Sprintf("Full CRUD lifecycle: Create %s via Add form, verify in list, edit fields, verify updates, delete and confirm removal", featureLabel)
		steps = buildCRUDLifecycleSteps(featureLabel, screensByRole, apiTC.Fields)

	case strings.Contains(lowerDesc, "create") && strings.Contains(lowerDesc, "update"):
		screen = pickScreen(screensByRole, "add", "list")
		guiDesc = fmt.Sprintf("Create %s via Add form with valid data, then update fields via Edit form and verify changes persist", featureLabel)
		steps = buildCreateUpdateSteps(featureLabel, screensByRole, apiTC.Fields)

	case strings.Contains(lowerDesc, "create") && strings.Contains(lowerDesc, "delete"):
		screen = pickScreen(screensByRole, "add", "list")
		guiDesc = fmt.Sprintf("Create %s via Add form, verify it appears in the list, then delete and confirm removal", featureLabel)
		steps = buildCreateDeleteSteps(featureLabel, screensByRole, apiTC.Fields)

	case strings.Contains(lowerDesc, "partial update"):
		screen = pickScreen(screensByRole, "edit", "list")
		guiDesc = fmt.Sprintf("Open existing %s in Edit form, modify only specific fields, save and verify only changed fields are updated", featureLabel)
		steps = buildPartialUpdateSteps(featureLabel, screensByRole, apiTC.Fields)

	case strings.Contains(lowerDesc, "idempotent"):
		screen = pickScreen(screensByRole, "add", "list")
		guiDesc = fmt.Sprintf("Create %s via Add form, then attempt to create a duplicate with same values and verify appropriate error/conflict message", featureLabel)
		steps = buildIdempotentCreateSteps(featureLabel, screensByRole, apiTC.Fields)

	case strings.Contains(lowerDesc, "deploy"):
		screen = pickScreen(screensByRole, "list", "add")
		guiDesc = fmt.Sprintf("Create %s, configure deployment scope, deploy to device and verify deployment status in UI", featureLabel)
		steps = buildDeploySteps(featureLabel, screensByRole, apiTC.Fields, desc)

	case strings.Contains(lowerDesc, "create") && strings.Contains(lowerDesc, "verify"):
		screen = pickScreen(screensByRole, "add", "list")
		guiDesc = fmt.Sprintf("Create a new %s by filling the Add form with valid values and verify it appears in the list with correct data", featureLabel)
		steps = buildCreateVerifySteps(featureLabel, screensByRole, apiTC.Fields)

	default:
		// Generic functional test
		screen = pickScreen(screensByRole, "list", "add")
		guiDesc = fmt.Sprintf("Via GUI: %s", desc)
		steps = []GUITestStep{
			{StepNumber: 1, Action: "navigate", Target: screen, Expected: fmt.Sprintf("%s screen is displayed", screen)},
			{StepNumber: 2, Action: "perform", Target: "feature action", Expected: desc},
		}
	}

	tc := &GUITestCase{
		TestCaseID:  fmt.Sprintf("GUI_%s_%04d", featureSlug, *counter),
		FeatureName: featureLabel,
		Priority:    apiTC.Priority,
		Automation:  "Automatable",
		Type:        "functional",
		Description: guiDesc,
		Screen:      screen,
		Steps:       steps,
	}
	*counter++
	return tc
}

// translateNegativeToGUI converts a negative API test into a GUI form validation test.
func translateNegativeToGUI(apiTC apiTestCaseInfo, featureLabel, featureSlug string,
	screensByRole map[string]string, fields []apiField,
	screenWidgets map[string][]FigmaComponent, mappings map[string]FigmaMappingRow,
	counter *int) *GUITestCase {

	screen := pickScreen(screensByRole, "add", "list")
	desc := apiTC.Description
	lowerDesc := strings.ToLower(desc)

	var guiDesc string
	var steps []GUITestStep

	switch {
	case strings.Contains(lowerDesc, "without required field"):
		// Extract field name from description
		fieldName := extractFieldNameFromDesc(desc, "without required field")
		guiDesc = fmt.Sprintf("Submit Add %s form without filling required field '%s' and verify validation error is displayed", featureLabel, fieldName)
		steps = []GUITestStep{
			{StepNumber: 1, Action: "navigate", Target: screen, Expected: fmt.Sprintf("%s screen is displayed", screen)},
			{StepNumber: 2, Action: "click", Target: "Add button", Expected: "Add form/dialog opens"},
			{StepNumber: 3, Action: "fill_form", Target: "all fields except " + fieldName, Expected: "Form fields populated"},
			{StepNumber: 4, Action: "leave_empty", Target: fieldName, Expected: fmt.Sprintf("'%s' field is left empty", fieldName)},
			{StepNumber: 5, Action: "click", Target: "Save / Submit button", Expected: "Form submission attempted"},
			{StepNumber: 6, Action: "verify_error", Target: fieldName, Expected: fmt.Sprintf("Validation error displayed for required field '%s'", fieldName)},
			{StepNumber: 7, Action: "verify", Target: "list", Expected: fmt.Sprintf("No new %s is created", featureLabel)},
		}

	case strings.Contains(lowerDesc, "invalid enum"):
		fieldName := extractFieldNameFromDesc(desc, "")
		// Find the field to get enum values from YANG
		var enumHint string
		for _, f := range fields {
			if strings.EqualFold(f.Name, fieldName) && len(f.EnumValues) > 0 {
				enumHint = fmt.Sprintf(" (valid options: %s)", strings.Join(f.EnumValues, ", "))
				break
			}
		}
		guiDesc = fmt.Sprintf("Attempt to set '%s' to an invalid option on the Add %s form and verify it is rejected", fieldName, featureLabel)
		steps = []GUITestStep{
			{StepNumber: 1, Action: "navigate", Target: screen, Expected: fmt.Sprintf("%s screen is displayed", screen)},
			{StepNumber: 2, Action: "click", Target: "Add button", Expected: "Add form/dialog opens"},
			{StepNumber: 3, Action: "select_invalid", Target: fieldName, Value: "invalid-value", Expected: "Invalid option is not selectable or error is shown" + enumHint},
			{StepNumber: 4, Action: "verify", Target: "dropdown/select", Expected: fmt.Sprintf("Only valid options are available for '%s'%s", fieldName, enumHint)},
		}

	case strings.Contains(lowerDesc, "pattern violation"):
		fieldName := extractFieldNameFromDesc(desc, "")
		// Find the field to get pattern from YANG
		var patternHint string
		for _, f := range fields {
			if strings.EqualFold(f.Name, fieldName) && f.Pattern != "" {
				patternHint = fmt.Sprintf(" (YANG pattern: %s)", f.Pattern)
				break
			}
		}
		guiDesc = fmt.Sprintf("Enter value with invalid pattern/format in '%s' field on Add %s form and verify validation error", fieldName, featureLabel)
		steps = []GUITestStep{
			{StepNumber: 1, Action: "navigate", Target: screen, Expected: fmt.Sprintf("%s screen is displayed", screen)},
			{StepNumber: 2, Action: "click", Target: "Add button", Expected: "Add form/dialog opens"},
			{StepNumber: 3, Action: "type", Target: fieldName, Value: "invalid-format-value", Expected: "Value entered" + patternHint},
			{StepNumber: 4, Action: "click", Target: "Save / Submit button", Expected: "Form submission attempted"},
			{StepNumber: 5, Action: "verify_error", Target: fieldName, Expected: fmt.Sprintf("Format/pattern validation error shown for '%s'%s", fieldName, patternHint)},
		}

	default:
		guiDesc = fmt.Sprintf("Via GUI: %s", desc)
		steps = []GUITestStep{
			{StepNumber: 1, Action: "navigate", Target: screen, Expected: fmt.Sprintf("%s screen is displayed", screen)},
			{StepNumber: 2, Action: "perform_negative", Target: "form", Expected: desc},
			{StepNumber: 3, Action: "verify_error", Target: "form", Expected: "Appropriate error message is displayed"},
		}
	}

	tc := &GUITestCase{
		TestCaseID:  fmt.Sprintf("GUI_%s_%04d", featureSlug, *counter),
		FeatureName: featureLabel,
		Priority:    apiTC.Priority,
		Automation:  "Automatable",
		Type:        "negative",
		Description: guiDesc,
		Screen:      screen,
		Steps:       steps,
	}
	*counter++
	return tc
}

// translateBoundaryToGUI converts a boundary API test into a GUI field boundary test.
func translateBoundaryToGUI(apiTC apiTestCaseInfo, featureLabel, featureSlug string,
	screensByRole map[string]string, fields []apiField,
	screenWidgets map[string][]FigmaComponent, mappings map[string]FigmaMappingRow,
	counter *int) *GUITestCase {

	screen := pickScreen(screensByRole, "add", "list")
	desc := apiTC.Description

	// Extract the field being tested and value from the boundary description
	fieldName := extractFieldNameFromDesc(desc, "")
	boundaryValue := extractBoundaryValue(desc)

	guiDesc := fmt.Sprintf("Enter boundary value '%s' in '%s' field via Add %s form and verify it is accepted/handled correctly",
		boundaryValue, fieldName, featureLabel)
	if strings.Contains(strings.ToLower(desc), "min") {
		guiDesc = fmt.Sprintf("Enter minimum boundary value '%s' in '%s' field via Add %s form and verify it is accepted",
			boundaryValue, fieldName, featureLabel)
	} else if strings.Contains(strings.ToLower(desc), "max") {
		guiDesc = fmt.Sprintf("Enter maximum boundary value '%s' in '%s' field via Add %s form and verify it is accepted or properly truncated",
			boundaryValue, fieldName, featureLabel)
	}

	steps := []GUITestStep{
		{StepNumber: 1, Action: "navigate", Target: screen, Expected: fmt.Sprintf("%s screen is displayed", screen)},
		{StepNumber: 2, Action: "click", Target: "Add button", Expected: "Add form/dialog opens"},
		{StepNumber: 3, Action: "fill_required_fields", Target: "form", Expected: "Required fields populated with valid values"},
		{StepNumber: 4, Action: "type", Target: fieldName, Value: boundaryValue, Expected: fmt.Sprintf("Value '%s' entered in '%s'", boundaryValue, fieldName)},
		{StepNumber: 5, Action: "click", Target: "Save / Submit button", Expected: "Form is submitted"},
		{StepNumber: 6, Action: "verify", Target: "result", Expected: fmt.Sprintf("%s is created/updated with boundary value in '%s'", featureLabel, fieldName)},
	}

	tc := &GUITestCase{
		TestCaseID:  fmt.Sprintf("GUI_%s_%04d", featureSlug, *counter),
		FeatureName: featureLabel,
		Priority:    apiTC.Priority,
		Automation:  "Automatable",
		Type:        "boundary",
		Description: guiDesc,
		Screen:      screen,
		Steps:       steps,
	}
	*counter++
	return tc
}

// ---- Step Builders for Functional Scenarios ----

func pickScreen(roles map[string]string, preferred ...string) string {
	for _, role := range preferred {
		if s, ok := roles[role]; ok {
			return s
		}
	}
	for _, s := range roles {
		return s
	}
	return "Main screen"
}

func fieldFillSteps(fields []apiField, startStep int) []GUITestStep {
	var steps []GUITestStep
	for i, f := range fields {
		if i >= 5 { // cap at 5 fields to keep steps manageable
			break
		}
		val := f.SampleValue
		if val == "" {
			val = "test-value"
		}

		// Choose action based on YANG type
		action := "fill"
		expected := fmt.Sprintf("'%s' field set to '%s'", f.Name, val)

		switch {
		case f.YangType == "boolean":
			action = "toggle"
			if val == "" || val == "false" {
				val = "true"
			}
			expected = fmt.Sprintf("'%s' toggle/switch set to %s", f.Name, val)
		case f.YangType == "enumeration" || len(f.EnumValues) > 0:
			action = "select"
			if len(f.EnumValues) > 0 && val == "" {
				val = f.EnumValues[0]
			}
			expected = fmt.Sprintf("'%s' dropdown set to '%s'", f.Name, val)
		case strings.Contains(f.YangType, "int") || f.YangType == "uint32" || f.YangType == "uint16" || f.YangType == "uint8":
			action = "fill"
			expected = fmt.Sprintf("'%s' number field set to '%s'", f.Name, val)
			if f.Min != "" && f.Max != "" {
				expected += fmt.Sprintf(" (valid range: %s–%s)", f.Min, f.Max)
			}
		case f.IsKey:
			action = "fill"
			expected = fmt.Sprintf("'%s' key/identity field set to '%s' (non-editable after create)", f.Name, val)
		}

		steps = append(steps, GUITestStep{
			StepNumber: startStep + i,
			Action:     action,
			Target:     f.Name,
			Value:      val,
			Expected:   expected,
		})
	}
	return steps
}

func buildCreateVerifySteps(feature string, roles map[string]string, fields []apiField) []GUITestStep {
	addScreen := pickScreen(roles, "add", "list")
	listScreen := pickScreen(roles, "list", "add")
	steps := []GUITestStep{
		{StepNumber: 1, Action: "navigate", Target: listScreen, Expected: fmt.Sprintf("%s list screen is displayed", feature)},
		{StepNumber: 2, Action: "click", Target: "Add / Create button", Expected: fmt.Sprintf("%s form opens", addScreen)},
	}
	fills := fieldFillSteps(fields, 3)
	steps = append(steps, fills...)
	next := 3 + len(fills)
	steps = append(steps,
		GUITestStep{StepNumber: next, Action: "click", Target: "Save / Submit button", Expected: fmt.Sprintf("%s is created successfully", feature)},
		GUITestStep{StepNumber: next + 1, Action: "verify_toast", Target: "success notification", Expected: "Success message is displayed"},
		GUITestStep{StepNumber: next + 2, Action: "verify_row", Target: listScreen, Expected: fmt.Sprintf("New %s appears in the list with correct values", feature)},
	)
	return steps
}

func buildCreateDeleteSteps(feature string, roles map[string]string, fields []apiField) []GUITestStep {
	addScreen := pickScreen(roles, "add", "list")
	listScreen := pickScreen(roles, "list", "add")
	deleteScreen := pickScreen(roles, "delete", "list")
	steps := []GUITestStep{
		{StepNumber: 1, Action: "navigate", Target: listScreen, Expected: fmt.Sprintf("%s list screen is displayed", feature)},
		{StepNumber: 2, Action: "click", Target: "Add / Create button", Expected: fmt.Sprintf("%s form opens", addScreen)},
	}
	fills := fieldFillSteps(fields, 3)
	steps = append(steps, fills...)
	next := 3 + len(fills)
	steps = append(steps,
		GUITestStep{StepNumber: next, Action: "click", Target: "Save / Submit button", Expected: fmt.Sprintf("%s is created", feature)},
		GUITestStep{StepNumber: next + 1, Action: "verify_row", Target: listScreen, Expected: fmt.Sprintf("New %s appears in list", feature)},
		GUITestStep{StepNumber: next + 2, Action: "select_row", Target: fmt.Sprintf("created %s", feature), Expected: "Row is selected"},
		GUITestStep{StepNumber: next + 3, Action: "click", Target: "Delete button", Expected: fmt.Sprintf("%s opens", deleteScreen)},
		GUITestStep{StepNumber: next + 4, Action: "confirm", Target: "Delete confirmation dialog", Expected: "Deletion is confirmed"},
		GUITestStep{StepNumber: next + 5, Action: "verify_removed", Target: listScreen, Expected: fmt.Sprintf("%s is removed from the list", feature)},
	)
	return steps
}

func buildCreateUpdateSteps(feature string, roles map[string]string, fields []apiField) []GUITestStep {
	addScreen := pickScreen(roles, "add", "list")
	listScreen := pickScreen(roles, "list", "add")
	editScreen := pickScreen(roles, "edit", "list")
	steps := []GUITestStep{
		{StepNumber: 1, Action: "navigate", Target: listScreen, Expected: fmt.Sprintf("%s list screen is displayed", feature)},
		{StepNumber: 2, Action: "click", Target: "Add / Create button", Expected: fmt.Sprintf("%s form opens", addScreen)},
	}
	fills := fieldFillSteps(fields, 3)
	steps = append(steps, fills...)
	next := 3 + len(fills)
	steps = append(steps,
		GUITestStep{StepNumber: next, Action: "click", Target: "Save / Submit button", Expected: fmt.Sprintf("%s is created", feature)},
		GUITestStep{StepNumber: next + 1, Action: "select_row", Target: fmt.Sprintf("created %s", feature), Expected: "Row is selected"},
		GUITestStep{StepNumber: next + 2, Action: "click", Target: "Edit button", Expected: fmt.Sprintf("%s opens with current values", editScreen)},
		GUITestStep{StepNumber: next + 3, Action: "modify_fields", Target: "editable fields", Value: "updated values", Expected: "Fields are updated with new values"},
		GUITestStep{StepNumber: next + 4, Action: "click", Target: "Save / Submit button", Expected: "Changes are saved"},
		GUITestStep{StepNumber: next + 5, Action: "verify_row", Target: listScreen, Expected: fmt.Sprintf("%s shows updated values in list", feature)},
	)
	return steps
}

func buildCRUDLifecycleSteps(feature string, roles map[string]string, fields []apiField) []GUITestStep {
	addScreen := pickScreen(roles, "add", "list")
	listScreen := pickScreen(roles, "list", "add")
	editScreen := pickScreen(roles, "edit", "list")
	deleteScreen := pickScreen(roles, "delete", "list")
	steps := []GUITestStep{
		{StepNumber: 1, Action: "navigate", Target: listScreen, Expected: fmt.Sprintf("%s list screen is displayed", feature)},
		{StepNumber: 2, Action: "click", Target: "Add / Create button", Expected: fmt.Sprintf("%s form opens", addScreen)},
	}
	fills := fieldFillSteps(fields, 3)
	steps = append(steps, fills...)
	next := 3 + len(fills)
	steps = append(steps,
		GUITestStep{StepNumber: next, Action: "click", Target: "Save / Submit button", Expected: fmt.Sprintf("%s is created successfully", feature)},
		GUITestStep{StepNumber: next + 1, Action: "verify_row", Target: listScreen, Expected: fmt.Sprintf("New %s visible in list", feature)},
		GUITestStep{StepNumber: next + 2, Action: "click_row", Target: fmt.Sprintf("created %s", feature), Expected: "Detail/edit view opens"},
		GUITestStep{StepNumber: next + 3, Action: "verify_details", Target: "detail view", Expected: "All field values match what was entered"},
		GUITestStep{StepNumber: next + 4, Action: "click", Target: "Edit button", Expected: fmt.Sprintf("%s opens", editScreen)},
		GUITestStep{StepNumber: next + 5, Action: "modify_fields", Target: "editable fields", Value: "updated values", Expected: "Fields updated"},
		GUITestStep{StepNumber: next + 6, Action: "click", Target: "Save / Submit button", Expected: "Changes saved"},
		GUITestStep{StepNumber: next + 7, Action: "verify_row", Target: listScreen, Expected: "Updated values reflected in list"},
		GUITestStep{StepNumber: next + 8, Action: "select_row", Target: fmt.Sprintf("updated %s", feature), Expected: "Row selected"},
		GUITestStep{StepNumber: next + 9, Action: "click", Target: "Delete button", Expected: fmt.Sprintf("%s opens", deleteScreen)},
		GUITestStep{StepNumber: next + 10, Action: "confirm", Target: "Delete confirmation", Expected: "Deletion confirmed"},
		GUITestStep{StepNumber: next + 11, Action: "verify_removed", Target: listScreen, Expected: fmt.Sprintf("%s no longer in list", feature)},
	)
	return steps
}

func buildPartialUpdateSteps(feature string, roles map[string]string, fields []apiField) []GUITestStep {
	listScreen := pickScreen(roles, "list", "add")
	editScreen := pickScreen(roles, "edit", "list")
	return []GUITestStep{
		{StepNumber: 1, Action: "navigate", Target: listScreen, Expected: fmt.Sprintf("%s list screen is displayed", feature)},
		{StepNumber: 2, Action: "select_row", Target: fmt.Sprintf("existing %s", feature), Expected: "Row is selected"},
		{StepNumber: 3, Action: "click", Target: "Edit button", Expected: fmt.Sprintf("%s opens with current values", editScreen)},
		{StepNumber: 4, Action: "verify_prefilled", Target: "all fields", Expected: "Form fields show current values"},
		{StepNumber: 5, Action: "modify_fields", Target: "one or two fields only", Value: "new value", Expected: "Only selected fields changed, others untouched"},
		{StepNumber: 6, Action: "click", Target: "Save / Submit button", Expected: "Changes saved"},
		{StepNumber: 7, Action: "verify_row", Target: listScreen, Expected: "Only modified fields show new values; unchanged fields retain original values"},
	}
}

func buildIdempotentCreateSteps(feature string, roles map[string]string, fields []apiField) []GUITestStep {
	addScreen := pickScreen(roles, "add", "list")
	listScreen := pickScreen(roles, "list", "add")
	steps := []GUITestStep{
		{StepNumber: 1, Action: "navigate", Target: listScreen, Expected: fmt.Sprintf("%s list screen is displayed", feature)},
		{StepNumber: 2, Action: "click", Target: "Add / Create button", Expected: fmt.Sprintf("%s form opens", addScreen)},
	}
	fills := fieldFillSteps(fields, 3)
	steps = append(steps, fills...)
	next := 3 + len(fills)
	steps = append(steps,
		GUITestStep{StepNumber: next, Action: "click", Target: "Save / Submit button", Expected: fmt.Sprintf("%s is created", feature)},
		GUITestStep{StepNumber: next + 1, Action: "click", Target: "Add / Create button", Expected: "Add form opens again"},
		GUITestStep{StepNumber: next + 2, Action: "fill_same_values", Target: "form", Expected: "Same values entered as first creation"},
		GUITestStep{StepNumber: next + 3, Action: "click", Target: "Save / Submit button", Expected: "Form submission attempted"},
		GUITestStep{StepNumber: next + 4, Action: "verify_error", Target: "form", Expected: "Duplicate/conflict error message is displayed"},
		GUITestStep{StepNumber: next + 5, Action: "verify_count", Target: listScreen, Expected: fmt.Sprintf("Only one %s exists, no duplicate created", feature)},
	)
	return steps
}

func buildDeploySteps(feature string, roles map[string]string, fields []apiField, desc string) []GUITestStep {
	listScreen := pickScreen(roles, "list", "add")
	// Per CONVENTIONS.md: scope→target-query→conflict-check→deploy→status→NOS-verify
	return []GUITestStep{
		{StepNumber: 1, Action: "navigate", Target: listScreen, Expected: fmt.Sprintf("%s list screen is displayed", feature)},
		{StepNumber: 2, Action: "create_or_select", Target: feature, Expected: fmt.Sprintf("%s is available in list", feature)},
		{StepNumber: 3, Action: "navigate", Target: "Deployment / Scope settings", Expected: "Deployment scope configuration screen is displayed"},
		{StepNumber: 4, Action: "select", Target: "Site Group / Site", Value: "authorized site", Expected: "Configuration profile scoped to site group/site"},
		{StepNumber: 5, Action: "configure", Target: "Target query", Value: "target device/site", Expected: "Target device resolved via site/device query"},
		{StepNumber: 6, Action: "click", Target: "Check Conflicts button", Expected: "Conflict check runs — no conflicts detected for target device"},
		{StepNumber: 7, Action: "click", Target: "Deploy button", Expected: "Deployment is initiated to target device"},
		{StepNumber: 8, Action: "verify_status", Target: "deployment status indicator", Expected: "Deployment status shows 'Success' or 'Completed'"},
		{StepNumber: 9, Action: "verify", Target: "device configuration", Expected: "Deployed configuration on device matches cloud intent values"},
	}
}

// countEnumFields counts fields that have enum values.
func countEnumFields(fields []apiField) int {
	n := 0
	for _, f := range fields {
		if len(f.EnumValues) > 0 {
			n++
		}
	}
	return n
}

// extractFieldNameFromDesc extracts a field name from test description text.
func extractFieldNameFromDesc(desc, marker string) string {
	// Try "field 'xxx'" or "field \"xxx\""
	re := regexp.MustCompile(`(?:field|Test)\s+['"]?([a-zA-Z0-9_-]+)['"]?`)
	if m := re.FindStringSubmatch(desc); len(m) > 1 {
		return m[1]
	}
	// Try "without required field 'xxx'"
	if marker != "" {
		re2 := regexp.MustCompile(marker + `\s+['"]?([a-zA-Z0-9_-]+)['"]?`)
		if m := re2.FindStringSubmatch(desc); len(m) > 1 {
			return m[1]
		}
	}
	return "field"
}

// extractBoundaryValue extracts the test value from a boundary description like "server='1.0.0.1'"
func extractBoundaryValue(desc string) string {
	re := regexp.MustCompile(`=\s*'([^']+)'`)
	if m := re.FindStringSubmatch(desc); len(m) > 1 {
		return m[1]
	}
	re2 := regexp.MustCompile(`value\s+'([^']+)'`)
	if m := re2.FindStringSubmatch(desc); len(m) > 1 {
		return m[1]
	}
	return "boundary-value"
}

// apiField captures field metadata extracted from the API test plan, enriched with YANG data.
type apiField struct {
	Name        string
	Type        string
	SampleValue string
	MaxLength   string
	MinLength   string
	Required    bool
	Origin      string
	// YANG enrichment
	YangType     string   // e.g. "string", "uint32", "enumeration", "boolean", "union"
	IsKey        bool     // YANG list key field (identity field, not editable after create)
	EnumValues   []string // valid enum options from YANG
	Pattern      string   // regex pattern constraint from YANG
	DefaultValue string   // YANG default value
	Description  string   // YANG description for the field
	Min          string   // numeric min or minLength
	Max          string   // numeric max or maxLength
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

// ---------- YANG Enrichment ----------

// loadYangFeature attempts to load YANG models and find the matching feature.
func (s *Server) loadYangFeature(featureName string) *model.Feature {
	cfg := s.resolveSourceConfig()
	if cfg.YangDir == "" || !dirExists(cfg.YangDir) {
		log.Printf("GUI test gen: YANG dir not found (%s), skipping YANG enrichment", cfg.YangDir)
		return nil
	}

	parser := yang.NewParser(cfg.YangDir)
	if err := parser.Parse(); err != nil {
		log.Printf("GUI test gen: failed to parse YANG: %v", err)
		return nil
	}

	features := parser.GetFeatures()
	// Try exact match first
	if f, ok := features[featureName]; ok {
		log.Printf("GUI test gen: YANG feature '%s' found (%d params)", featureName, len(f.Parameters))
		return f
	}

	// Try normalized name (e.g., "radius-server" matches "radius-server")
	slug := strings.ToLower(strings.ReplaceAll(featureName, " ", "-"))
	for name, f := range features {
		if strings.ToLower(name) == slug {
			log.Printf("GUI test gen: YANG feature '%s' matched as '%s' (%d params)", featureName, name, len(f.Parameters))
			return f
		}
	}

	// Try partial match
	for name, f := range features {
		if strings.Contains(strings.ToLower(name), slug) || strings.Contains(slug, strings.ToLower(name)) {
			log.Printf("GUI test gen: YANG feature '%s' partially matched as '%s' (%d params)", featureName, name, len(f.Parameters))
			return f
		}
	}

	log.Printf("GUI test gen: no YANG feature match for '%s' among %d features", featureName, len(features))
	return nil
}

// enrichFieldsWithYANG enriches API-extracted fields with YANG model data.
func enrichFieldsWithYANG(fields []apiField, yangFeature *model.Feature) []apiField {
	if yangFeature == nil {
		return fields
	}

	// Build parameter lookup by name
	paramByName := make(map[string]model.Parameter)
	for _, p := range yangFeature.Parameters {
		paramByName[strings.ToLower(p.Name)] = p
		// Also index nested properties
		for _, np := range p.NestedProperties {
			paramByName[strings.ToLower(np.Name)] = np
		}
	}

	keySet := make(map[string]bool)
	for _, k := range yangFeature.Keys {
		keySet[strings.ToLower(k)] = true
	}

	for i := range fields {
		lowerName := strings.ToLower(fields[i].Name)
		param, found := paramByName[lowerName]
		if !found {
			continue
		}

		// Enrich with YANG type
		fields[i].YangType = param.YangType
		fields[i].Description = param.Description
		fields[i].IsKey = keySet[lowerName]
		if param.Required {
			fields[i].Required = true
		}
		if param.DefaultValue != nil {
			fields[i].DefaultValue = fmt.Sprintf("%v", param.DefaultValue)
		}

		// Extract constraints
		for _, c := range param.Constraints {
			switch c.Type {
			case model.ConstraintTypeEnum:
				if vals, ok := c.Value.([]string); ok {
					fields[i].EnumValues = vals
				}
			case model.ConstraintTypePattern:
				fields[i].Pattern = fmt.Sprintf("%v", c.Value)
			case model.ConstraintTypeMinLength:
				fields[i].MinLength = fmt.Sprintf("%v", c.Value)
				fields[i].Min = fmt.Sprintf("%v", c.Value)
			case model.ConstraintTypeMaxLength:
				fields[i].MaxLength = fmt.Sprintf("%v", c.Value)
				fields[i].Max = fmt.Sprintf("%v", c.Value)
			case model.ConstraintTypeMin:
				fields[i].Min = fmt.Sprintf("%v", c.Value)
			case model.ConstraintTypeMax:
				fields[i].Max = fmt.Sprintf("%v", c.Value)
			}
		}

		// If no sample value, use YANG default or first enum
		if fields[i].SampleValue == "" {
			if fields[i].DefaultValue != "" {
				fields[i].SampleValue = fields[i].DefaultValue
			} else if len(fields[i].EnumValues) > 0 {
				fields[i].SampleValue = fields[i].EnumValues[0]
			}
		}
	}

	// Add any YANG fields not already in the API-extracted list
	seen := make(map[string]bool)
	for _, f := range fields {
		seen[strings.ToLower(f.Name)] = true
	}
	for _, p := range yangFeature.Parameters {
		if seen[strings.ToLower(p.Name)] {
			continue
		}
		af := apiField{
			Name:     p.Name,
			YangType: p.YangType,
			Required: p.Required,
			IsKey:    keySet[strings.ToLower(p.Name)],
			Origin:   "yang",
		}
		if p.DefaultValue != nil {
			af.DefaultValue = fmt.Sprintf("%v", p.DefaultValue)
			af.SampleValue = af.DefaultValue
		}
		af.Description = p.Description
		for _, c := range p.Constraints {
			switch c.Type {
			case model.ConstraintTypeEnum:
				if vals, ok := c.Value.([]string); ok {
					af.EnumValues = vals
					if af.SampleValue == "" && len(vals) > 0 {
						af.SampleValue = vals[0]
					}
				}
			case model.ConstraintTypePattern:
				af.Pattern = fmt.Sprintf("%v", c.Value)
			case model.ConstraintTypeMinLength:
				af.MinLength = fmt.Sprintf("%v", c.Value)
			case model.ConstraintTypeMaxLength:
				af.MaxLength = fmt.Sprintf("%v", c.Value)
			case model.ConstraintTypeMin:
				af.Min = fmt.Sprintf("%v", c.Value)
			case model.ConstraintTypeMax:
				af.Max = fmt.Sprintf("%v", c.Value)
			}
		}
		fields = append(fields, af)
	}

	return fields
}
