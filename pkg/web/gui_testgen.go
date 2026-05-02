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

// ---------- UI Pattern Data Model ----------

// uiPattern is the top-level structure for a category's UI pattern definition.
type uiPattern struct {
	PatternName string                     `json:"patternName"`
	Module      string                     `json:"module"`
	Navigation  string                     `json:"navigation"`
	CommonUI    uiCommonUI                 `json:"commonUI"`
	Features    map[string]uiFeatureDef    `json:"features"`
}

type uiCommonUI struct {
	ListPage uiListPageCommon `json:"listPage"`
	AddModal uiAddModalCommon `json:"addModal"`
	Toasts   uiToastMessages  `json:"toastMessages"`
}

type uiListPageCommon struct {
	SearchBar       bool              `json:"searchBar"`
	AddButtonLabel  string            `json:"addButtonLabel"`
	RefreshButton   bool              `json:"refreshButton"`
	ExportButton    bool              `json:"exportButton"`
	ColumnsSidebar  *uiColumnsSidebar `json:"columnsSidebar"`
	FiltersSidebar  *uiFiltersSidebar `json:"filtersSidebar"`
	TableCheckboxes bool              `json:"tableCheckboxes"`
	EmptyStateMsg   string            `json:"emptyStateMessage"`
}

type uiColumnsSidebar struct {
	Toggle                  string `json:"toggle"`
	SearchBar               bool   `json:"searchBar"`
	CheckboxPerColumn       bool   `json:"checkboxPerColumn"`
	DragToReorder           bool   `json:"dragToReorder"`
	AllColumnsCheckedDefault bool  `json:"allColumnsCheckedByDefault"`
}

type uiFiltersSidebar struct {
	Toggle              string   `json:"toggle"`
	SearchBar           bool     `json:"searchBar"`
	ExpandablePerColumn bool     `json:"expandablePerColumn"`
	FilterInputType     string   `json:"filterInputType"`
	Buttons             []string `json:"buttons"`
}

type uiAddModalCommon struct {
	TitleTemplate     string `json:"titleTemplate"`
	CloseButton       bool   `json:"closeButton"`
	DefaultTab        string `json:"defaultTab"`
	SyncMessage       string `json:"syncMessage"`
	IPHelperText      string `json:"ipHelperText"`
	DefaultButtons    []string `json:"defaultButtons"`
}

type uiToastMessages struct {
	Create   string `json:"create"`
	Update   string `json:"update"`
	Delete   string `json:"delete"`
	Refresh  string `json:"refresh"`
	Download string `json:"download"`
	Subtitle string `json:"subtitle"`
}

type uiFeatureDef struct {
	FeatureLabel          string          `json:"featureLabel"`
	FeatureLabelPlural    string          `json:"featureLabelPlural"`
	ConfigProfilesTabLabel string         `json:"configProfilesTabLabel"`
	SyncVerb              string          `json:"syncVerb"`
	ListPage              uiListPage      `json:"listPage"`
	AddModal              uiAddModal      `json:"addModal"`
	SystemManagedFields   []string        `json:"systemManagedFields"`
	Toasts                *uiToastMessages `json:"toastMessages,omitempty"`
}

type uiListPage struct {
	PageTitle        string             `json:"pageTitle"`
	ViewUsageLink    *uiViewUsage       `json:"viewUsageLink,omitempty"`
	TableColumns     []string           `json:"tableColumns"`
	EmptyStateMsg    string             `json:"emptyStateMessage"`
	PageToggles      []uiPageToggle     `json:"pageToggles,omitempty"`
	SiblingFeature   *uiSiblingFeature  `json:"siblingFeature,omitempty"`
	Layout           string             `json:"layout,omitempty"`
	RowInlineToggle  *uiRowInlineToggle `json:"rowInlineToggle,omitempty"`
	RowKebabMenu     []string           `json:"rowKebabMenu,omitempty"`
	ToolbarMoreMenu  *uiToolbarMoreMenu `json:"toolbarMoreMenu,omitempty"`
	BulkActions      *uiBulkActions     `json:"bulkActions,omitempty"`
	Pagination       *uiPagination      `json:"pagination,omitempty"`
}

type uiViewUsage struct {
	Label        string   `json:"label"`
	ModalTitle   string   `json:"modalTitle"`
	SearchBar    bool     `json:"searchBar"`
	TableColumns []string `json:"tableColumns"`
	EmptyState   string   `json:"emptyState"`
}

type uiToolbarMoreMenu struct {
	RequiresSelection bool     `json:"requiresSelection"`
	Actions           []string `json:"actions"`
}

type uiRowInlineToggle struct {
	Column       string `json:"column"`
	DefaultState string `json:"defaultState"`
	Description  string `json:"description"`
}

type uiBulkActions struct {
	SelectionLabel string   `json:"selectionLabel"`
	Actions        []string `json:"actions"`
}

type uiPagination struct {
	PageSizeOptions []int  `json:"pageSizeOptions"`
	Format          string `json:"format"`
}

type uiPageToggle struct {
	Label         string          `json:"label"`
	DefaultState  string          `json:"defaultState"`
	ConfirmDialog *uiConfirmDialog `json:"confirmDialog,omitempty"`
}

type uiConfirmDialog struct {
	Title      string   `json:"title"`
	Message    string   `json:"message"`
	DeployNote string   `json:"deployNote"`
	Expandable string   `json:"expandable"`
	Buttons    []string `json:"buttons"`
}

type uiSiblingFeature struct {
	Name           string `json:"name"`
	Layout         string `json:"layout"`
	EmptyState     string `json:"emptyState"`
	AddButtonLabel string `json:"addButtonLabel"`
}

type uiAddModal struct {
	Buttons             []string          `json:"buttons"`
	AdvancedToggle      bool              `json:"advancedToggle,omitempty"`
	MultiRowAdd         bool              `json:"multiRowAdd,omitempty"`
	MultiRowAddButton   string            `json:"multiRowAddButton,omitempty"`
	MultiRowDeleteButton string           `json:"multiRowDeleteButton,omitempty"`
	StandardFields      []uiFieldDef      `json:"standardFields"`
	AdvancedFields      []uiFieldDef      `json:"advancedFields,omitempty"`
	ToggleGroups        []uiToggleGroup   `json:"toggleGroups,omitempty"`
}

type uiFieldDef struct {
	UILabel      string `json:"uiLabel"`
	APIProperty  string `json:"apiProperty"`
	WidgetType   string `json:"widgetType"` // text, number, dropdown, password, toggle, multi-tag-input
	Required     bool   `json:"required"`
	HelperText   string `json:"helperText,omitempty"`
	DefaultValue interface{} `json:"defaultValue,omitempty"`
}

type uiToggleGroup struct {
	ToggleLabel     string       `json:"toggleLabel"`
	APIProperty     string       `json:"apiProperty"`
	DefaultState    string       `json:"defaultState"`
	DependentFields []uiFieldDef `json:"dependentFields"`
}

// loadUIPattern loads the UI pattern definition for a category.
func (s *Server) loadUIPattern(category string) *uiPattern {
	patternPath := filepath.Join("config", "ui-patterns", category+".json")
	data, err := os.ReadFile(patternPath)
	if err != nil {
		log.Printf("GUI test gen: no UI pattern file for '%s': %v", category, err)
		return nil
	}
	var pattern uiPattern
	if err := json.Unmarshal(data, &pattern); err != nil {
		log.Printf("GUI test gen: failed to parse UI pattern '%s': %v", category, err)
		return nil
	}
	log.Printf("GUI test gen: loaded UI pattern '%s' with %d features", category, len(pattern.Features))
	return &pattern
}

// getUIFeatureDef looks up the feature definition from the UI pattern.
func getUIFeatureDef(pattern *uiPattern, featureName string) *uiFeatureDef {
	if pattern == nil {
		return nil
	}
	if def, ok := pattern.Features[featureName]; ok {
		return &def
	}
	// Try slug variations
	slug := strings.ToLower(strings.ReplaceAll(featureName, " ", "-"))
	for name, def := range pattern.Features {
		if strings.ToLower(name) == slug {
			return &def
		}
	}
	return nil
}

// isSystemManagedField checks if a field should be excluded from GUI form fill.
func isSystemManagedField(fieldName string, uiDef *uiFeatureDef) bool {
	if uiDef == nil {
		return false
	}
	lower := strings.ToLower(fieldName)
	for _, smf := range uiDef.SystemManagedFields {
		if strings.ToLower(smf) == lower {
			return true
		}
	}
	return false
}

// findUILabel returns the UI label for an API property, or the property name if not mapped.
func findUILabel(apiProp string, uiDef *uiFeatureDef) string {
	if uiDef == nil {
		return apiProp
	}
	for _, f := range uiDef.AddModal.StandardFields {
		if strings.EqualFold(f.APIProperty, apiProp) {
			return f.UILabel
		}
	}
	for _, f := range uiDef.AddModal.AdvancedFields {
		if strings.EqualFold(f.APIProperty, apiProp) {
			return f.UILabel
		}
	}
	for _, tg := range uiDef.AddModal.ToggleGroups {
		if strings.EqualFold(tg.APIProperty, apiProp) {
			return tg.ToggleLabel
		}
		for _, f := range tg.DependentFields {
			if strings.EqualFold(f.APIProperty, apiProp) {
				return f.UILabel
			}
		}
	}
	return apiProp
}

// findWidgetType returns the widget type for an API property from the UI definition.
func findWidgetType(apiProp string, uiDef *uiFeatureDef) string {
	if uiDef == nil {
		return "text"
	}
	for _, f := range uiDef.AddModal.StandardFields {
		if strings.EqualFold(f.APIProperty, apiProp) {
			return f.WidgetType
		}
	}
	for _, f := range uiDef.AddModal.AdvancedFields {
		if strings.EqualFold(f.APIProperty, apiProp) {
			return f.WidgetType
		}
	}
	for _, tg := range uiDef.AddModal.ToggleGroups {
		for _, f := range tg.DependentFields {
			if strings.EqualFold(f.APIProperty, apiProp) {
				return f.WidgetType
			}
		}
	}
	return "text"
}

// isToggleDependentField checks if a field is controlled by a toggle group.
func isToggleDependentField(apiProp string, uiDef *uiFeatureDef) (bool, string) {
	if uiDef == nil {
		return false, ""
	}
	for _, tg := range uiDef.AddModal.ToggleGroups {
		for _, f := range tg.DependentFields {
			if strings.EqualFold(f.APIProperty, apiProp) {
				return true, tg.ToggleLabel
			}
		}
	}
	return false, ""
}

// isAdvancedField checks if a field is only visible in Advanced mode.
func isAdvancedField(apiProp string, uiDef *uiFeatureDef) bool {
	if uiDef == nil {
		return false
	}
	for _, f := range uiDef.AddModal.AdvancedFields {
		if strings.EqualFold(f.APIProperty, apiProp) {
			return true
		}
	}
	return false
}

// resolveToast returns the toast message for a CRUD operation, using feature-specific override or common template.
func resolveToast(pattern *uiPattern, uiDef *uiFeatureDef, op string) (title, subtitle string) {
	var t *uiToastMessages
	if uiDef != nil && uiDef.Toasts != nil {
		t = uiDef.Toasts
	} else if pattern != nil {
		t = &pattern.CommonUI.Toasts
	}
	if t == nil {
		return fmt.Sprintf("%s {identifier} %s", uiDef.FeatureLabel, strings.Title(op)), ""
	}
	switch op {
	case "create":
		title = t.Create
	case "update":
		title = t.Update
	case "delete":
		title = t.Delete
	}
	if title == "" {
		title = fmt.Sprintf("%s {identifier} %s", uiDef.FeatureLabel, strings.Title(op))
	}
	return title, t.Subtitle
}

// getAllUIFieldDefs returns all fields from the UI definition (standard + toggle + advanced).
func getAllUIFieldDefs(uiDef *uiFeatureDef) []uiFieldDef {
	if uiDef == nil {
		return nil
	}
	var all []uiFieldDef
	all = append(all, uiDef.AddModal.StandardFields...)
	for _, tg := range uiDef.AddModal.ToggleGroups {
		all = append(all, uiFieldDef{
			UILabel: tg.ToggleLabel, APIProperty: tg.APIProperty,
			WidgetType: "toggle", Required: false,
		})
		all = append(all, tg.DependentFields...)
	}
	all = append(all, uiDef.AddModal.AdvancedFields...)
	return all
}

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

	// 1c. Load UI pattern definition for this category + feature
	uiPatternDef := s.loadUIPattern(req.Category)
	uiDef := getUIFeatureDef(uiPatternDef, req.Feature)
	if uiDef != nil {
		featureLabel = uiDef.FeatureLabel // Use exact UI label from screenshots
		log.Printf("GUI test gen: loaded UI definition for '%s' (%d std fields, %d toggle groups, %d adv fields)",
			uiDef.FeatureLabel, len(uiDef.AddModal.StandardFields),
			len(uiDef.AddModal.ToggleGroups), len(uiDef.AddModal.AdvancedFields))
		// Filter out system-managed fields from form fill
		var filteredFields []apiField
		for _, f := range fields {
			if !isSystemManagedField(f.Name, uiDef) {
				filteredFields = append(filteredFields, f)
			}
		}
		fields = filteredFields
		for i := range apiTestCases {
			var ff []apiField
			for _, f := range apiTestCases[i].Fields {
				if !isSystemManagedField(f.Name, uiDef) {
					ff = append(ff, f)
				}
			}
			apiTestCases[i].Fields = ff
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
						Value:    f.DefaultValue,
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

	// --- F. UI Pattern-driven tests (page load, modal, toggle deps, advanced mode, multi-row, save flows) ---
	if uiDef != nil {
		listScreen := pickScreen(screensByRole, "list", "add")
		addScreen := pickScreen(screensByRole, "add", "list")
		featurePlural := uiDef.FeatureLabelPlural
		if featurePlural == "" {
			featurePlural = featureLabel + "s"
		}

		// F1. Page Load / Empty State
		emptyMsg := uiDef.ListPage.EmptyStateMsg
		if emptyMsg == "" {
			emptyMsg = fmt.Sprintf("No %s Found.", featurePlural)
		}
		allTests = append(allTests, GUITestCase{
			TestCaseID: fmt.Sprintf("GUI_%s_%04d", featureSlug, tcCounter),
			FeatureName: req.Feature, Priority: "P1", Automation: "Automatable",
			Type: "functional", Screen: listScreen,
			Description: fmt.Sprintf("Verify %s page loads with correct layout, table columns, and empty state", featurePlural),
			Steps: buildPageLoadSteps(uiDef, listScreen, featurePlural, emptyMsg),
		})
		catCounts["functional"]++; priCounts["P1"]++; tcCounter++

		// F2. Add Modal structure
		allTests = append(allTests, GUITestCase{
			TestCaseID: fmt.Sprintf("GUI_%s_%04d", featureSlug, tcCounter),
			FeatureName: req.Feature, Priority: "P1", Automation: "Automatable",
			Type: "functional", Screen: addScreen,
			Description: fmt.Sprintf("Verify Add %s modal opens with correct tabs, fields, and buttons", featureLabel),
			Steps: buildModalStructureSteps(uiDef, listScreen, addScreen, featureLabel),
		})
		catCounts["functional"]++; priCounts["P1"]++; tcCounter++

		// F3. Create with all visible form fields (using real UI labels)
		allTests = append(allTests, GUITestCase{
			TestCaseID: fmt.Sprintf("GUI_%s_%04d", featureSlug, tcCounter),
			FeatureName: req.Feature, Priority: "P1", Automation: "Automatable",
			Type: "functional", Screen: addScreen,
			Description: fmt.Sprintf("Create %s via Add form filling all visible fields with valid data and verify in table", featureLabel),
			Steps: buildUIFormFillSteps(uiDef, listScreen, featureLabel, featurePlural, fields),
		})
		catCounts["functional"]++; priCounts["P1"]++; tcCounter++

		// F4. Toggle dependency tests (e.g., Authentication toggle → port + secret)
		for _, tg := range uiDef.AddModal.ToggleGroups {
			// Test: toggle OFF → dependent fields disabled
			allTests = append(allTests, GUITestCase{
				TestCaseID: fmt.Sprintf("GUI_%s_%04d", featureSlug, tcCounter),
				FeatureName: req.Feature, Priority: "P1", Automation: "Automatable",
				Type: "functional", Screen: addScreen,
				Description: fmt.Sprintf("Verify '%s' toggle OFF disables dependent fields: %s", tg.ToggleLabel, toggleDepNames(tg)),
				Steps: buildToggleOffSteps(uiDef, listScreen, featureLabel, tg),
			})
			catCounts["functional"]++; priCounts["P1"]++; tcCounter++

			// Test: toggle ON → dependent fields enabled
			allTests = append(allTests, GUITestCase{
				TestCaseID: fmt.Sprintf("GUI_%s_%04d", featureSlug, tcCounter),
				FeatureName: req.Feature, Priority: "P1", Automation: "Automatable",
				Type: "functional", Screen: addScreen,
				Description: fmt.Sprintf("Verify '%s' toggle ON enables dependent fields: %s", tg.ToggleLabel, toggleDepNames(tg)),
				Steps: buildToggleOnSteps(uiDef, listScreen, featureLabel, tg),
			})
			catCounts["functional"]++; priCounts["P1"]++; tcCounter++
		}

		// F5. Advanced toggle tests
		if uiDef.AddModal.AdvancedToggle {
			advFieldNames := make([]string, len(uiDef.AddModal.AdvancedFields))
			for i, f := range uiDef.AddModal.AdvancedFields {
				advFieldNames[i] = f.UILabel
			}
			allTests = append(allTests, GUITestCase{
				TestCaseID: fmt.Sprintf("GUI_%s_%04d", featureSlug, tcCounter),
				FeatureName: req.Feature, Priority: "P1", Automation: "Automatable",
				Type: "functional", Screen: addScreen,
				Description: fmt.Sprintf("Verify Advanced toggle reveals additional fields: %s", strings.Join(advFieldNames, ", ")),
				Steps: buildAdvancedToggleSteps(uiDef, listScreen, featureLabel),
			})
			catCounts["functional"]++; priCounts["P1"]++; tcCounter++
		}

		// F6. Multi-row add tests
		if uiDef.AddModal.MultiRowAdd {
			allTests = append(allTests, GUITestCase{
				TestCaseID: fmt.Sprintf("GUI_%s_%04d", featureSlug, tcCounter),
				FeatureName: req.Feature, Priority: "P1", Automation: "Automatable",
				Type: "functional", Screen: addScreen,
				Description: fmt.Sprintf("Verify multi-row add: click [+] to add another %s row in Add form", featureLabel),
				Steps: buildMultiRowAddSteps(uiDef, listScreen, featureLabel),
			})
			catCounts["functional"]++; priCounts["P1"]++; tcCounter++
		}

		// F7. Page toggle tests (e.g., Platform ONE Security)
		for _, pt := range uiDef.ListPage.PageToggles {
			if pt.ConfirmDialog != nil {
				allTests = append(allTests, GUITestCase{
					TestCaseID: fmt.Sprintf("GUI_%s_%04d", featureSlug, tcCounter),
					FeatureName: req.Feature, Priority: "P1", Automation: "Automatable",
					Type: "functional", Screen: listScreen,
					Description: fmt.Sprintf("Verify '%s' toggle shows confirmation dialog and can be enabled/canceled", pt.Label),
					Steps: buildPageToggleSteps(pt, listScreen, featurePlural),
				})
				catCounts["functional"]++; priCounts["P1"]++; tcCounter++
			}
		}

		// F8. Save & Add Another flow (if button exists)
		for _, btn := range uiDef.AddModal.Buttons {
			if strings.Contains(strings.ToLower(btn), "add another") {
				allTests = append(allTests, GUITestCase{
					TestCaseID: fmt.Sprintf("GUI_%s_%04d", featureSlug, tcCounter),
					FeatureName: req.Feature, Priority: "P1", Automation: "Automatable",
					Type: "functional", Screen: addScreen,
					Description: fmt.Sprintf("Verify '%s' saves current entry and resets form for next entry", btn),
					Steps: buildSaveAndAddAnotherSteps(uiDef, listScreen, featureLabel, btn),
				})
				catCounts["functional"]++; priCounts["P1"]++; tcCounter++
				break
			}
		}

		// F9. Cancel button test
		allTests = append(allTests, GUITestCase{
			TestCaseID: fmt.Sprintf("GUI_%s_%04d", featureSlug, tcCounter),
			FeatureName: req.Feature, Priority: "P1", Automation: "Automatable",
			Type: "negative", Screen: addScreen,
			Description: fmt.Sprintf("Verify Cancel discards form data and returns to %s list", featurePlural),
			Steps: []GUITestStep{
				{StepNumber: 1, Action: "navigate", Target: listScreen, Expected: fmt.Sprintf("%s page is displayed", featurePlural)},
				{StepNumber: 2, Action: "click", Target: fmt.Sprintf("+ Add %s", featureLabel), Expected: "Add modal opens"},
				{StepNumber: 3, Action: "fill", Target: uiDef.AddModal.StandardFields[0].UILabel, Value: "test-value", Expected: "Field populated"},
				{StepNumber: 4, Action: "click", Target: "Cancel", Expected: fmt.Sprintf("Modal closes, returns to %s list, no entry created", featurePlural)},
			},
		})
		catCounts["negative"]++; priCounts["P1"]++; tcCounter++

		// F10. Close modal with X
		allTests = append(allTests, GUITestCase{
			TestCaseID: fmt.Sprintf("GUI_%s_%04d", featureSlug, tcCounter),
			FeatureName: req.Feature, Priority: "P2", Automation: "Automatable",
			Type: "negative", Screen: addScreen,
			Description: fmt.Sprintf("Verify closing Add %s modal with X button discards data", featureLabel),
			Steps: []GUITestStep{
				{StepNumber: 1, Action: "navigate", Target: listScreen, Expected: fmt.Sprintf("%s page is displayed", featurePlural)},
				{StepNumber: 2, Action: "click", Target: fmt.Sprintf("+ Add %s", featureLabel), Expected: "Add modal opens"},
				{StepNumber: 3, Action: "fill", Target: uiDef.AddModal.StandardFields[0].UILabel, Value: "test-value", Expected: "Field populated"},
				{StepNumber: 4, Action: "click", Target: "X close button", Expected: fmt.Sprintf("Modal closes, no %s created", featureLabel)},
			},
		})
		catCounts["negative"]++; priCounts["P2"]++; tcCounter++

		// F11. Config Profiles tab in modal
		allTests = append(allTests, GUITestCase{
			TestCaseID: fmt.Sprintf("GUI_%s_%04d", featureSlug, tcCounter),
			FeatureName: req.Feature, Priority: "P2", Automation: "Automatable",
			Type: "functional", Screen: addScreen,
			Description: fmt.Sprintf("Verify '%s' tab in Add modal shows profile table", uiDef.ConfigProfilesTabLabel),
			Steps: []GUITestStep{
				{StepNumber: 1, Action: "navigate", Target: listScreen, Expected: fmt.Sprintf("%s page is displayed", featurePlural)},
				{StepNumber: 2, Action: "click", Target: fmt.Sprintf("+ Add %s", featureLabel), Expected: "Add modal opens"},
				{StepNumber: 3, Action: "click", Target: uiDef.ConfigProfilesTabLabel, Expected: "Tab becomes active"},
				{StepNumber: 4, Action: "verify", Target: "table columns", Expected: "Table shows 'Configuration Profile' and 'Deployment Status' columns"},
				{StepNumber: 5, Action: "verify", Target: "empty state", Expected: "Shows 'No configuration profiles found' when none exist"},
				{StepNumber: 6, Action: "click", Target: fmt.Sprintf("%s Details", featureLabel), Expected: "Returns to form tab with data preserved"},
			},
		})
		catCounts["functional"]++; priCounts["P2"]++; tcCounter++

		// F12. Required field validation for each required standard field
		for _, sf := range uiDef.AddModal.StandardFields {
			if sf.Required {
				allTests = append(allTests, GUITestCase{
					TestCaseID: fmt.Sprintf("GUI_%s_%04d", featureSlug, tcCounter),
					FeatureName: req.Feature, Priority: "P1", Automation: "Automatable",
					Type: "negative", Screen: addScreen,
					Description: fmt.Sprintf("Verify '%s' is required — submit form without it and verify validation error", sf.UILabel),
					Steps: []GUITestStep{
						{StepNumber: 1, Action: "navigate", Target: listScreen, Expected: fmt.Sprintf("%s page is displayed", featurePlural)},
						{StepNumber: 2, Action: "click", Target: fmt.Sprintf("+ Add %s", featureLabel), Expected: "Add modal opens"},
						{StepNumber: 3, Action: "leave_empty", Target: sf.UILabel, Expected: fmt.Sprintf("'%s' field is left empty", sf.UILabel)},
						{StepNumber: 4, Action: "click", Target: "Save", Expected: "Form submission attempted"},
						{StepNumber: 5, Action: "verify_error", Target: sf.UILabel, Expected: fmt.Sprintf("Validation error shown for required field '%s'", sf.UILabel)},
					},
				})
				catCounts["negative"]++; priCounts["P1"]++; tcCounter++
			}
		}

		// F13. Toast message verification (create, update, delete)
		toastCreateTitle, toastSubtitle := resolveToast(uiPatternDef, uiDef, "create")
		toastUpdateTitle, _ := resolveToast(uiPatternDef, uiDef, "update")
		toastDeleteTitle, _ := resolveToast(uiPatternDef, uiDef, "delete")
		allTests = append(allTests, GUITestCase{
			TestCaseID: fmt.Sprintf("GUI_%s_%04d", featureSlug, tcCounter),
			FeatureName: req.Feature, Priority: "P1", Automation: "Automatable",
			Type: "functional", Screen: listScreen,
			Description: fmt.Sprintf("Verify toast notifications for Create, Update, and Delete of %s", featureLabel),
			Steps: buildToastVerificationSteps(uiDef, listScreen, featureLabel, featurePlural,
				toastCreateTitle, toastUpdateTitle, toastDeleteTitle, toastSubtitle),
		})
		catCounts["functional"]++; priCounts["P1"]++; tcCounter++

		// F14. Row inline toggle (Enabled/Disabled per entry)
		if uiDef.ListPage.RowInlineToggle != nil {
			rit := uiDef.ListPage.RowInlineToggle
			allTests = append(allTests, GUITestCase{
				TestCaseID: fmt.Sprintf("GUI_%s_%04d", featureSlug, tcCounter),
				FeatureName: req.Feature, Priority: "P1", Automation: "Automatable",
				Type: "functional", Screen: listScreen,
				Description: fmt.Sprintf("Verify '%s' inline toggle in table row — default %s, can be toggled ON/OFF", rit.Column, rit.DefaultState),
				Steps: buildRowInlineToggleSteps(uiDef, listScreen, featureLabel, featurePlural, rit),
			})
			catCounts["functional"]++; priCounts["P1"]++; tcCounter++
		}

		// F15. Bulk select and delete
		if uiPatternDef != nil && uiPatternDef.CommonUI.ListPage.TableCheckboxes {
			allTests = append(allTests, GUITestCase{
				TestCaseID: fmt.Sprintf("GUI_%s_%04d", featureSlug, tcCounter),
				FeatureName: req.Feature, Priority: "P1", Automation: "Automatable",
				Type: "functional", Screen: listScreen,
				Description: fmt.Sprintf("Verify bulk selection and Delete of multiple %s entries", featurePlural),
				Steps: buildBulkDeleteSteps(uiDef, listScreen, featureLabel, featurePlural, toastDeleteTitle, toastSubtitle),
			})
			catCounts["functional"]++; priCounts["P1"]++; tcCounter++
		}

		// F16. Search bar functionality
		if uiPatternDef != nil && uiPatternDef.CommonUI.ListPage.SearchBar {
			allTests = append(allTests, GUITestCase{
				TestCaseID: fmt.Sprintf("GUI_%s_%04d", featureSlug, tcCounter),
				FeatureName: req.Feature, Priority: "P1", Automation: "Automatable",
				Type: "functional", Screen: listScreen,
				Description: fmt.Sprintf("Verify Search bar filters %s table by matching text", featurePlural),
				Steps: buildSearchBarSteps(listScreen, featureLabel, featurePlural),
			})
			catCounts["functional"]++; priCounts["P1"]++; tcCounter++
		}

		// F17. Columns sidebar — show/hide/reorder
		if uiPatternDef != nil && uiPatternDef.CommonUI.ListPage.ColumnsSidebar != nil {
			allTests = append(allTests, GUITestCase{
				TestCaseID: fmt.Sprintf("GUI_%s_%04d", featureSlug, tcCounter),
				FeatureName: req.Feature, Priority: "P2", Automation: "Automatable",
				Type: "functional", Screen: listScreen,
				Description: fmt.Sprintf("Verify Columns sidebar — toggle column visibility, search, and drag-to-reorder for %s table", featurePlural),
				Steps: buildColumnsSidebarSteps(uiDef, listScreen, featurePlural, uiPatternDef.CommonUI.ListPage.ColumnsSidebar),
			})
			catCounts["functional"]++; priCounts["P2"]++; tcCounter++
		}

		// F18. Filters sidebar — apply/reset per-column filter
		if uiPatternDef != nil && uiPatternDef.CommonUI.ListPage.FiltersSidebar != nil {
			allTests = append(allTests, GUITestCase{
				TestCaseID: fmt.Sprintf("GUI_%s_%04d", featureSlug, tcCounter),
				FeatureName: req.Feature, Priority: "P2", Automation: "Automatable",
				Type: "functional", Screen: listScreen,
				Description: fmt.Sprintf("Verify Filters sidebar — expand column filter, enter text, Apply, and Reset for %s table", featurePlural),
				Steps: buildFiltersSidebarSteps(uiDef, listScreen, featurePlural, uiPatternDef.CommonUI.ListPage.FiltersSidebar),
			})
			catCounts["functional"]++; priCounts["P2"]++; tcCounter++
		}

		// F19. Row kebab menu actions
		if len(uiDef.ListPage.RowKebabMenu) > 0 {
			allTests = append(allTests, GUITestCase{
				TestCaseID: fmt.Sprintf("GUI_%s_%04d", featureSlug, tcCounter),
				FeatureName: req.Feature, Priority: "P1", Automation: "Automatable",
				Type: "functional", Screen: listScreen,
				Description: fmt.Sprintf("Verify row kebab menu (⋮) shows correct actions: %s", strings.Join(uiDef.ListPage.RowKebabMenu, ", ")),
				Steps: buildRowKebabMenuSteps(uiDef, listScreen, featureLabel, featurePlural),
			})
			catCounts["functional"]++; priCounts["P1"]++; tcCounter++
		}

		// F20. Toolbar more-options menu (Enable/Disable)
		if uiDef.ListPage.ToolbarMoreMenu != nil {
			allTests = append(allTests, GUITestCase{
				TestCaseID: fmt.Sprintf("GUI_%s_%04d", featureSlug, tcCounter),
				FeatureName: req.Feature, Priority: "P1", Automation: "Automatable",
				Type: "functional", Screen: listScreen,
				Description: fmt.Sprintf("Verify toolbar ⋮ more-options menu shows: %s (requires row selection)", strings.Join(uiDef.ListPage.ToolbarMoreMenu.Actions, ", ")),
				Steps: buildToolbarMoreMenuSteps(uiDef, listScreen, featureLabel, featurePlural),
			})
			catCounts["functional"]++; priCounts["P1"]++; tcCounter++
		}

		// F21. View Usage modal
		if uiDef.ListPage.ViewUsageLink != nil {
			allTests = append(allTests, GUITestCase{
				TestCaseID: fmt.Sprintf("GUI_%s_%04d", featureSlug, tcCounter),
				FeatureName: req.Feature, Priority: "P2", Automation: "Automatable",
				Type: "functional", Screen: listScreen,
				Description: fmt.Sprintf("Verify 'View Usage' opens '%s' modal with config profile table", uiDef.ListPage.ViewUsageLink.ModalTitle),
				Steps: buildViewUsageSteps(uiDef, listScreen, featurePlural),
			})
			catCounts["functional"]++; priCounts["P2"]++; tcCounter++
		}

		// F22. Refresh button + toast
		if uiPatternDef != nil && uiPatternDef.CommonUI.ListPage.RefreshButton {
			refreshToast := "Grid refreshed!"
			if uiPatternDef.CommonUI.Toasts.Refresh != "" {
				refreshToast = uiPatternDef.CommonUI.Toasts.Refresh
			}
			allTests = append(allTests, GUITestCase{
				TestCaseID: fmt.Sprintf("GUI_%s_%04d", featureSlug, tcCounter),
				FeatureName: req.Feature, Priority: "P2", Automation: "Automatable",
				Type: "functional", Screen: listScreen,
				Description: fmt.Sprintf("Verify Refresh button reloads %s table and shows toast", featurePlural),
				Steps: []GUITestStep{
					{StepNumber: 1, Action: "navigate", Target: listScreen, Expected: fmt.Sprintf("%s page is displayed", featurePlural)},
					{StepNumber: 2, Action: "click", Target: "Refresh button (↻)", Expected: "Table data reloads"},
					{StepNumber: 3, Action: "verify_toast", Target: "success notification", Expected: fmt.Sprintf("Toast: '%s'", refreshToast)},
				},
			})
			catCounts["functional"]++; priCounts["P2"]++; tcCounter++
		}

		// F23. Download/Export button + toast
		if uiPatternDef != nil && uiPatternDef.CommonUI.ListPage.ExportButton {
			downloadToast := uiPatternDef.CommonUI.Toasts.Download
			if downloadToast == "" {
				downloadToast = featurePlural + " list downloaded!"
			}
			allTests = append(allTests, GUITestCase{
				TestCaseID: fmt.Sprintf("GUI_%s_%04d", featureSlug, tcCounter),
				FeatureName: req.Feature, Priority: "P2", Automation: "Automatable",
				Type: "functional", Screen: listScreen,
				Description: fmt.Sprintf("Verify Export/Download button downloads %s list and shows toast", featurePlural),
				Steps: []GUITestStep{
					{StepNumber: 1, Action: "navigate", Target: listScreen, Expected: fmt.Sprintf("%s page is displayed with entries", featurePlural)},
					{StepNumber: 2, Action: "click", Target: "Download button (↓)", Expected: "File download initiated"},
					{StepNumber: 3, Action: "verify_toast", Target: "success notification", Expected: fmt.Sprintf("Toast: '%s'", downloadToast)},
					{StepNumber: 4, Action: "verify", Target: "downloaded file", Expected: "Downloaded file contains correct data"},
				},
			})
			catCounts["functional"]++; priCounts["P2"]++; tcCounter++
		}

		log.Printf("GUI test gen: added %d UI pattern-driven tests", tcCounter-1-len(allTests)+len(allTests))
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
			YangModel: func() string {
				if yangFeature != nil {
					return yangFeature.Name
				}
				return ""
			}(),
			YangFields: func() int {
				if yangFeature != nil {
					return len(yangFeature.Parameters)
				}
				return 0
			}(),
			YangKeys: func() string {
				if yangFeature != nil {
					return strings.Join(yangFeature.Keys, ", ")
				}
				return ""
			}(),
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

// ---------- UI Pattern Step Builders ----------

func toggleDepNames(tg uiToggleGroup) string {
	names := make([]string, len(tg.DependentFields))
	for i, f := range tg.DependentFields {
		names[i] = f.UILabel
	}
	return strings.Join(names, ", ")
}

func buildPageLoadSteps(uiDef *uiFeatureDef, listScreen, featurePlural, emptyMsg string) []GUITestStep {
	steps := []GUITestStep{
		{StepNumber: 1, Action: "navigate", Target: listScreen, Expected: fmt.Sprintf("%s page is displayed", featurePlural)},
		{StepNumber: 2, Action: "verify", Target: "page title", Expected: fmt.Sprintf("Page title shows '%s'", uiDef.ListPage.PageTitle)},
		{StepNumber: 3, Action: "verify", Target: "Search bar", Expected: "Search bar is present and functional"},
		{StepNumber: 4, Action: "verify", Target: fmt.Sprintf("+ Add %s button", uiDef.FeatureLabel), Expected: "Add button is visible and clickable"},
	}
	n := 5
	if len(uiDef.ListPage.TableColumns) > 0 {
		steps = append(steps, GUITestStep{
			StepNumber: n, Action: "verify", Target: "table columns",
			Expected: fmt.Sprintf("Table columns: %s", strings.Join(uiDef.ListPage.TableColumns, ", ")),
		})
		n++
	}
	steps = append(steps, GUITestStep{
		StepNumber: n, Action: "verify", Target: "empty state",
		Expected: fmt.Sprintf("Empty state message: '%s'", emptyMsg),
	})
	return steps
}

func buildModalStructureSteps(uiDef *uiFeatureDef, listScreen, addScreen, featureLabel string) []GUITestStep {
	steps := []GUITestStep{
		{StepNumber: 1, Action: "navigate", Target: listScreen, Expected: fmt.Sprintf("%s page is displayed", uiDef.FeatureLabelPlural)},
		{StepNumber: 2, Action: "click", Target: fmt.Sprintf("+ Add %s", featureLabel), Expected: fmt.Sprintf("'Add %s' modal opens", featureLabel)},
		{StepNumber: 3, Action: "verify", Target: "modal title", Expected: fmt.Sprintf("Modal title is 'Add %s'", featureLabel)},
		{StepNumber: 4, Action: "verify", Target: "modal tabs", Expected: fmt.Sprintf("Left tabs: '%s Details' (active), '%s'", featureLabel, uiDef.ConfigProfilesTabLabel)},
		{StepNumber: 5, Action: "verify", Target: "X close button", Expected: "X close button is visible at top-right"},
	}
	n := 6
	for _, sf := range uiDef.AddModal.StandardFields {
		steps = append(steps, GUITestStep{
			StepNumber: n, Action: "verify", Target: sf.UILabel,
			Expected: fmt.Sprintf("'%s' %s field is present", sf.UILabel, sf.WidgetType),
		})
		n++
	}
	steps = append(steps, GUITestStep{
		StepNumber: n, Action: "verify", Target: "buttons",
		Expected: fmt.Sprintf("Buttons visible: %s", strings.Join(uiDef.AddModal.Buttons, ", ")),
	})
	return steps
}

func buildUIFormFillSteps(uiDef *uiFeatureDef, listScreen, featureLabel, featurePlural string, fields []apiField) []GUITestStep {
	steps := []GUITestStep{
		{StepNumber: 1, Action: "navigate", Target: listScreen, Expected: fmt.Sprintf("%s page is displayed", featurePlural)},
		{StepNumber: 2, Action: "click", Target: fmt.Sprintf("+ Add %s", featureLabel), Expected: fmt.Sprintf("'Add %s' modal opens", featureLabel)},
	}
	n := 3

	fieldByAPI := make(map[string]apiField)
	for _, f := range fields {
		fieldByAPI[strings.ToLower(f.Name)] = f
	}

	for _, sf := range uiDef.AddModal.StandardFields {
		action := widgetAction(sf.WidgetType)
		val := sampleValue(sf, fieldByAPI)
		steps = append(steps, GUITestStep{
			StepNumber: n, Action: action, Target: sf.UILabel, Value: val,
			Expected: fmt.Sprintf("'%s' set to '%s'", sf.UILabel, val),
		})
		n++
	}

	for _, tg := range uiDef.AddModal.ToggleGroups {
		steps = append(steps, GUITestStep{
			StepNumber: n, Action: "toggle", Target: tg.ToggleLabel, Value: "ON",
			Expected: fmt.Sprintf("'%s' toggle enabled — dependent fields become editable", tg.ToggleLabel),
		})
		n++
		for _, df := range tg.DependentFields {
			action := widgetAction(df.WidgetType)
			val := sampleValue(df, fieldByAPI)
			steps = append(steps, GUITestStep{
				StepNumber: n, Action: action, Target: df.UILabel, Value: val,
				Expected: fmt.Sprintf("'%s' set to '%s'", df.UILabel, val),
			})
			n++
		}
	}

	steps = append(steps,
		GUITestStep{StepNumber: n, Action: "click", Target: "Save", Expected: fmt.Sprintf("%s created successfully — success notification displayed", featureLabel)},
		GUITestStep{StepNumber: n + 1, Action: "verify_row", Target: listScreen, Expected: fmt.Sprintf("New %s appears in table with correct values", featureLabel)},
	)
	return steps
}

func buildToggleOffSteps(uiDef *uiFeatureDef, listScreen, featureLabel string, tg uiToggleGroup) []GUITestStep {
	steps := []GUITestStep{
		{StepNumber: 1, Action: "navigate", Target: listScreen, Expected: fmt.Sprintf("%s page is displayed", uiDef.FeatureLabelPlural)},
		{StepNumber: 2, Action: "click", Target: fmt.Sprintf("+ Add %s", featureLabel), Expected: "Add modal opens"},
		{StepNumber: 3, Action: "verify", Target: tg.ToggleLabel + " toggle", Expected: fmt.Sprintf("'%s' toggle is OFF by default", tg.ToggleLabel)},
	}
	n := 4
	for _, df := range tg.DependentFields {
		steps = append(steps, GUITestStep{
			StepNumber: n, Action: "verify_disabled", Target: df.UILabel,
			Expected: fmt.Sprintf("'%s' field is disabled/greyed out (toggle OFF)", df.UILabel),
		})
		n++
	}
	return steps
}

func buildToggleOnSteps(uiDef *uiFeatureDef, listScreen, featureLabel string, tg uiToggleGroup) []GUITestStep {
	steps := []GUITestStep{
		{StepNumber: 1, Action: "navigate", Target: listScreen, Expected: fmt.Sprintf("%s page is displayed", uiDef.FeatureLabelPlural)},
		{StepNumber: 2, Action: "click", Target: fmt.Sprintf("+ Add %s", featureLabel), Expected: "Add modal opens"},
		{StepNumber: 3, Action: "toggle", Target: tg.ToggleLabel, Value: "ON", Expected: fmt.Sprintf("'%s' toggle set to ON (purple)", tg.ToggleLabel)},
	}
	n := 4
	for _, df := range tg.DependentFields {
		steps = append(steps, GUITestStep{
			StepNumber: n, Action: "verify_enabled", Target: df.UILabel,
			Expected: fmt.Sprintf("'%s' field becomes enabled and editable", df.UILabel),
		})
		n++
	}
	return steps
}

func buildAdvancedToggleSteps(uiDef *uiFeatureDef, listScreen, featureLabel string) []GUITestStep {
	steps := []GUITestStep{
		{StepNumber: 1, Action: "navigate", Target: listScreen, Expected: fmt.Sprintf("%s page is displayed", uiDef.FeatureLabelPlural)},
		{StepNumber: 2, Action: "click", Target: fmt.Sprintf("+ Add %s", featureLabel), Expected: "Add modal opens"},
		{StepNumber: 3, Action: "verify", Target: "Advanced toggle", Expected: "Advanced toggle is OFF by default"},
	}
	n := 4
	for _, af := range uiDef.AddModal.AdvancedFields {
		steps = append(steps, GUITestStep{
			StepNumber: n, Action: "verify_hidden", Target: af.UILabel,
			Expected: fmt.Sprintf("'%s' is not visible (Advanced OFF)", af.UILabel),
		})
		n++
	}
	steps = append(steps, GUITestStep{
		StepNumber: n, Action: "toggle", Target: "Advanced", Value: "ON",
		Expected: "Advanced toggle set to ON (purple)",
	})
	n++
	for _, af := range uiDef.AddModal.AdvancedFields {
		steps = append(steps, GUITestStep{
			StepNumber: n, Action: "verify_visible", Target: af.UILabel,
			Expected: fmt.Sprintf("'%s' %s field appears", af.UILabel, af.WidgetType),
		})
		n++
	}
	return steps
}

func buildMultiRowAddSteps(uiDef *uiFeatureDef, listScreen, featureLabel string) []GUITestStep {
	firstField := "IP Address"
	if len(uiDef.AddModal.StandardFields) > 0 {
		firstField = uiDef.AddModal.StandardFields[0].UILabel
	}
	return []GUITestStep{
		{StepNumber: 1, Action: "navigate", Target: listScreen, Expected: fmt.Sprintf("%s page is displayed", uiDef.FeatureLabelPlural)},
		{StepNumber: 2, Action: "click", Target: fmt.Sprintf("+ Add %s", featureLabel), Expected: "Add modal opens with one row"},
		{StepNumber: 3, Action: "fill", Target: firstField + " (row 1)", Value: "10.0.0.1", Expected: "First row populated"},
		{StepNumber: 4, Action: "click", Target: "[+] button", Expected: "Second row of fields appears"},
		{StepNumber: 5, Action: "fill", Target: firstField + " (row 2)", Value: "10.0.0.2", Expected: "Second row populated"},
		{StepNumber: 6, Action: "verify", Target: "delete icon (row 1)", Expected: "Trash/delete icon appears for each row"},
		{StepNumber: 7, Action: "click", Target: "Save", Expected: fmt.Sprintf("Both %ss created successfully", featureLabel)},
		{StepNumber: 8, Action: "verify", Target: "table", Expected: "Table shows both entries with correct data"},
	}
}

func buildPageToggleSteps(pt uiPageToggle, listScreen, featurePlural string) []GUITestStep {
	steps := []GUITestStep{
		{StepNumber: 1, Action: "navigate", Target: listScreen, Expected: fmt.Sprintf("%s page is displayed", featurePlural)},
		{StepNumber: 2, Action: "verify", Target: pt.Label + " toggle", Expected: fmt.Sprintf("'%s' toggle is %s by default", pt.Label, pt.DefaultState)},
		{StepNumber: 3, Action: "click", Target: pt.Label + " toggle", Expected: "Confirmation dialog appears"},
	}
	n := 4
	if pt.ConfirmDialog != nil {
		steps = append(steps,
			GUITestStep{StepNumber: n, Action: "verify", Target: "dialog title", Expected: fmt.Sprintf("Dialog title: '%s'", pt.ConfirmDialog.Title)},
			GUITestStep{StepNumber: n + 1, Action: "verify", Target: "dialog buttons", Expected: fmt.Sprintf("Buttons: %s", strings.Join(pt.ConfirmDialog.Buttons, ", "))},
		)
		n += 2
		if pt.ConfirmDialog.Expandable != "" {
			steps = append(steps, GUITestStep{
				StepNumber: n, Action: "expand", Target: pt.ConfirmDialog.Expandable,
				Expected: "Expandable section shows inheriting profiles table",
			})
			n++
		}
		steps = append(steps, GUITestStep{
			StepNumber: n, Action: "click", Target: "Cancel",
			Expected: fmt.Sprintf("Dialog closes, '%s' toggle remains %s", pt.Label, pt.DefaultState),
		})
		n++
		steps = append(steps,
			GUITestStep{StepNumber: n, Action: "click", Target: pt.Label + " toggle", Expected: "Confirmation dialog appears again"},
			GUITestStep{StepNumber: n + 1, Action: "click", Target: pt.ConfirmDialog.Buttons[len(pt.ConfirmDialog.Buttons)-1], Expected: fmt.Sprintf("Dialog closes, '%s' toggle is now ON", pt.Label)},
		)
	}
	return steps
}

func buildSaveAndAddAnotherSteps(uiDef *uiFeatureDef, listScreen, featureLabel, btnLabel string) []GUITestStep {
	firstField := "IP Address/FQDN"
	if len(uiDef.AddModal.StandardFields) > 0 {
		firstField = uiDef.AddModal.StandardFields[0].UILabel
	}
	return []GUITestStep{
		{StepNumber: 1, Action: "navigate", Target: listScreen, Expected: fmt.Sprintf("%s page is displayed", uiDef.FeatureLabelPlural)},
		{StepNumber: 2, Action: "click", Target: fmt.Sprintf("+ Add %s", featureLabel), Expected: "Add modal opens"},
		{StepNumber: 3, Action: "fill", Target: firstField, Value: "10.0.0.1", Expected: "Field populated"},
		{StepNumber: 4, Action: "click", Target: btnLabel, Expected: fmt.Sprintf("First %s saved (success notification), form resets for next entry", featureLabel)},
		{StepNumber: 5, Action: "verify", Target: firstField, Expected: fmt.Sprintf("'%s' is cleared/empty for next entry", firstField)},
		{StepNumber: 6, Action: "fill", Target: firstField, Value: "10.0.0.2", Expected: "Second entry populated"},
		{StepNumber: 7, Action: "click", Target: "Save", Expected: fmt.Sprintf("Second %s saved, modal closes", featureLabel)},
		{StepNumber: 8, Action: "verify", Target: "table", Expected: "Table shows both entries"},
	}
}

// widgetAction returns the appropriate test step action for a widget type.
func widgetAction(widgetType string) string {
	switch widgetType {
	case "dropdown":
		return "select"
	case "toggle":
		return "toggle"
	case "password":
		return "fill_secret"
	case "number":
		return "fill"
	case "multi-tag-input":
		return "type_and_enter"
	default:
		return "fill"
	}
}

// sampleValue returns a sample value for a UI field, using YANG/API data when available.
func sampleValue(sf uiFieldDef, fieldByAPI map[string]apiField) string {
	apiProp := strings.ToLower(sf.APIProperty)
	if f, ok := fieldByAPI[apiProp]; ok {
		if f.SampleValue != "" {
			return f.SampleValue
		}
	}
	if sf.DefaultValue != nil {
		return fmt.Sprintf("%v", sf.DefaultValue)
	}
	switch sf.WidgetType {
	case "number":
		return "1"
	case "password":
		return "secret123"
	case "dropdown":
		return "(select option)"
	case "multi-tag-input":
		return "10.0.0.1"
	default:
		return "test-value"
	}
}

func buildToastVerificationSteps(uiDef *uiFeatureDef, listScreen, featureLabel, featurePlural, createToast, updateToast, deleteToast, subtitle string) []GUITestStep {
	steps := []GUITestStep{
		{StepNumber: 1, Action: "precondition", Target: listScreen, Expected: fmt.Sprintf("At least one %s exists; note its identifier (e.g. IP)", featureLabel)},
	}
	n := 2
	// Create toast
	steps = append(steps, GUITestStep{
		StepNumber: n, Action: "create_entry", Target: featureLabel,
		Expected: fmt.Sprintf("Create a new %s via Add form", featureLabel),
	})
	n++
	steps = append(steps, GUITestStep{
		StepNumber: n, Action: "verify_toast", Target: "success notification",
		Expected: fmt.Sprintf("Toast title: '%s' (with actual identifier)", createToast),
	})
	n++
	if subtitle != "" {
		steps = append(steps, GUITestStep{
			StepNumber: n, Action: "verify_toast_subtitle", Target: "toast subtitle",
			Expected: fmt.Sprintf("Toast subtitle: '%s'", subtitle),
		})
		n++
	}
	// Update toast
	steps = append(steps, GUITestStep{
		StepNumber: n, Action: "edit_entry", Target: featureLabel,
		Expected: fmt.Sprintf("Edit existing %s via kebab menu → Edit", featureLabel),
	})
	n++
	steps = append(steps, GUITestStep{
		StepNumber: n, Action: "verify_toast", Target: "success notification",
		Expected: fmt.Sprintf("Toast title: '%s' (with actual identifier)", updateToast),
	})
	n++
	// Delete toast
	steps = append(steps, GUITestStep{
		StepNumber: n, Action: "delete_entry", Target: featureLabel,
		Expected: fmt.Sprintf("Delete %s via kebab menu → Delete", featureLabel),
	})
	n++
	steps = append(steps, GUITestStep{
		StepNumber: n, Action: "verify_toast", Target: "success notification",
		Expected: fmt.Sprintf("Toast title: '%s' (with actual identifier)", deleteToast),
	})
	return steps
}

func buildRowInlineToggleSteps(uiDef *uiFeatureDef, listScreen, featureLabel, featurePlural string, rit *uiRowInlineToggle) []GUITestStep {
	return []GUITestStep{
		{StepNumber: 1, Action: "precondition", Target: listScreen, Expected: fmt.Sprintf("At least one %s exists in table", featureLabel)},
		{StepNumber: 2, Action: "verify", Target: fmt.Sprintf("'%s' column toggle", rit.Column), Expected: fmt.Sprintf("Inline toggle is %s by default for the entry", rit.DefaultState)},
		{StepNumber: 3, Action: "click", Target: fmt.Sprintf("'%s' toggle on row", rit.Column), Expected: fmt.Sprintf("Toggle switches to ON — %s is now enabled", featureLabel)},
		{StepNumber: 4, Action: "verify_toast", Target: "success notification", Expected: fmt.Sprintf("Toast confirms %s updated", featureLabel)},
		{StepNumber: 5, Action: "verify", Target: fmt.Sprintf("'%s' column toggle", rit.Column), Expected: "Toggle now shows ON (blue/purple)"},
		{StepNumber: 6, Action: "click", Target: fmt.Sprintf("'%s' toggle on row", rit.Column), Expected: fmt.Sprintf("Toggle switches back to OFF — %s is now disabled", featureLabel)},
		{StepNumber: 7, Action: "verify_toast", Target: "success notification", Expected: fmt.Sprintf("Toast confirms %s updated", featureLabel)},
		{StepNumber: 8, Action: "verify", Target: fmt.Sprintf("'%s' column toggle", rit.Column), Expected: "Toggle now shows OFF (grey)"},
	}
}

func buildBulkDeleteSteps(uiDef *uiFeatureDef, listScreen, featureLabel, featurePlural, deleteToast, subtitle string) []GUITestStep {
	return []GUITestStep{
		{StepNumber: 1, Action: "precondition", Target: listScreen, Expected: fmt.Sprintf("At least 2 %s exist in table", featurePlural)},
		{StepNumber: 2, Action: "click", Target: "header checkbox", Expected: fmt.Sprintf("All rows selected — toolbar shows '%s Selected'", featurePlural)},
		{StepNumber: 3, Action: "verify", Target: "selection count", Expected: fmt.Sprintf("Shows 'N %s Selected' with correct count", featurePlural)},
		{StepNumber: 4, Action: "verify", Target: "Delete link", Expected: "'Delete' action appears in toolbar (red text)"},
		{StepNumber: 5, Action: "click", Target: "Delete", Expected: "Confirmation dialog or all selected entries are deleted"},
		{StepNumber: 6, Action: "verify_toast", Target: "success notification", Expected: fmt.Sprintf("Toast: '%s'", deleteToast)},
		{StepNumber: 7, Action: "verify", Target: "table", Expected: fmt.Sprintf("Deleted %s no longer appear in table", featurePlural)},
	}
}

func buildSearchBarSteps(listScreen, featureLabel, featurePlural string) []GUITestStep {
	return []GUITestStep{
		{StepNumber: 1, Action: "precondition", Target: listScreen, Expected: fmt.Sprintf("Multiple %s exist in table", featurePlural)},
		{StepNumber: 2, Action: "click", Target: "Search bar", Expected: "Search input is focused with 'Search' placeholder"},
		{StepNumber: 3, Action: "fill", Target: "Search bar", Value: "10.1.1.1", Expected: "Table filters to show only matching rows"},
		{StepNumber: 4, Action: "verify", Target: "table rows", Expected: "Only rows containing '10.1.1.1' are displayed"},
		{StepNumber: 5, Action: "fill", Target: "Search bar", Value: "nonexistent-value", Expected: "Table shows no results / empty state"},
		{StepNumber: 6, Action: "clear", Target: "Search bar", Expected: fmt.Sprintf("All %s re-appear in table", featurePlural)},
	}
}

func buildColumnsSidebarSteps(uiDef *uiFeatureDef, listScreen, featurePlural string, cs *uiColumnsSidebar) []GUITestStep {
	firstCol := "Priority"
	if len(uiDef.ListPage.TableColumns) > 0 {
		firstCol = uiDef.ListPage.TableColumns[0]
	}
	return []GUITestStep{
		{StepNumber: 1, Action: "navigate", Target: listScreen, Expected: fmt.Sprintf("%s page is displayed", featurePlural)},
		{StepNumber: 2, Action: "click", Target: "Columns sidebar toggle", Expected: "Columns panel opens on right side"},
		{StepNumber: 3, Action: "verify", Target: "column checkboxes", Expected: fmt.Sprintf("All %d columns shown with checkboxes (all checked by default)", len(uiDef.ListPage.TableColumns))},
		{StepNumber: 4, Action: "verify", Target: "Search field", Expected: "Search box is present in Columns sidebar"},
		{StepNumber: 5, Action: "verify", Target: "drag handles", Expected: "Drag handles (⊞) visible for reordering columns"},
		{StepNumber: 6, Action: "uncheck", Target: fmt.Sprintf("'%s' checkbox", firstCol), Expected: fmt.Sprintf("'%s' column is hidden from table", firstCol)},
		{StepNumber: 7, Action: "verify", Target: "table columns", Expected: fmt.Sprintf("Table no longer shows '%s' column", firstCol)},
		{StepNumber: 8, Action: "check", Target: fmt.Sprintf("'%s' checkbox", firstCol), Expected: fmt.Sprintf("'%s' column reappears in table", firstCol)},
		{StepNumber: 9, Action: "click", Target: "Columns sidebar toggle", Expected: "Columns panel closes"},
	}
}

func buildFiltersSidebarSteps(uiDef *uiFeatureDef, listScreen, featurePlural string, fs *uiFiltersSidebar) []GUITestStep {
	firstCol := "IP Address/FQDN"
	if len(uiDef.ListPage.TableColumns) > 1 {
		firstCol = uiDef.ListPage.TableColumns[1]
	}
	return []GUITestStep{
		{StepNumber: 1, Action: "precondition", Target: listScreen, Expected: fmt.Sprintf("Multiple %s exist in table", featurePlural)},
		{StepNumber: 2, Action: "click", Target: "Filters sidebar toggle", Expected: "Filters panel opens on right side"},
		{StepNumber: 3, Action: "verify", Target: "expandable columns", Expected: "Filter sections shown for each column (collapsed by default)"},
		{StepNumber: 4, Action: "expand", Target: fmt.Sprintf("'%s' filter section", firstCol), Expected: fmt.Sprintf("'%s' filter expands showing dropdown and 'Filter...' text input", firstCol)},
		{StepNumber: 5, Action: "fill", Target: "Filter... input", Value: "10.1.1.1", Expected: "Filter value entered"},
		{StepNumber: 6, Action: "click", Target: "Apply", Expected: fmt.Sprintf("Table filters to show only %s matching '%s' = '10.1.1.1'", featurePlural, firstCol)},
		{StepNumber: 7, Action: "verify", Target: "table rows", Expected: "Only rows matching the filter are displayed"},
		{StepNumber: 8, Action: "click", Target: "Reset", Expected: fmt.Sprintf("Filter cleared — all %s re-appear", featurePlural)},
		{StepNumber: 9, Action: "click", Target: "Filters sidebar toggle", Expected: "Filters panel closes"},
	}
}

func buildRowKebabMenuSteps(uiDef *uiFeatureDef, listScreen, featureLabel, featurePlural string) []GUITestStep {
	steps := []GUITestStep{
		{StepNumber: 1, Action: "precondition", Target: listScreen, Expected: fmt.Sprintf("At least one %s exists in table", featureLabel)},
		{StepNumber: 2, Action: "click", Target: "row ⋮ (kebab menu)", Expected: "Dropdown menu appears"},
	}
	n := 3
	for _, action := range uiDef.ListPage.RowKebabMenu {
		steps = append(steps, GUITestStep{
			StepNumber: n, Action: "verify", Target: fmt.Sprintf("'%s' menu item", action),
			Expected: fmt.Sprintf("'%s' option is visible and clickable", action),
		})
		n++
	}
	steps = append(steps, GUITestStep{
		StepNumber: n, Action: "click", Target: "outside menu",
		Expected: "Kebab menu closes",
	})
	return steps
}

func buildToolbarMoreMenuSteps(uiDef *uiFeatureDef, listScreen, featureLabel, featurePlural string) []GUITestStep {
	tmm := uiDef.ListPage.ToolbarMoreMenu
	steps := []GUITestStep{
		{StepNumber: 1, Action: "precondition", Target: listScreen, Expected: fmt.Sprintf("At least one %s exists in table", featureLabel)},
	}
	n := 2
	if tmm.RequiresSelection {
		steps = append(steps, GUITestStep{
			StepNumber: n, Action: "click", Target: "row checkbox", Expected: fmt.Sprintf("1 %s Selected shown in toolbar", featureLabel),
		})
		n++
	}
	steps = append(steps, GUITestStep{
		StepNumber: n, Action: "click", Target: "toolbar ⋮ (more options)", Expected: "Dropdown menu appears",
	})
	n++
	for _, action := range tmm.Actions {
		steps = append(steps, GUITestStep{
			StepNumber: n, Action: "verify", Target: fmt.Sprintf("'%s' menu item", action),
			Expected: fmt.Sprintf("'%s' option is visible", action),
		})
		n++
	}
	steps = append(steps, GUITestStep{
		StepNumber: n, Action: "click", Target: fmt.Sprintf("'%s'", tmm.Actions[0]),
		Expected: fmt.Sprintf("'%s' action applied to selected %s", tmm.Actions[0], featureLabel),
	})
	n++
	steps = append(steps, GUITestStep{
		StepNumber: n, Action: "verify_toast", Target: "success notification",
		Expected: fmt.Sprintf("Toast confirms %s updated", featureLabel),
	})
	return steps
}

func buildViewUsageSteps(uiDef *uiFeatureDef, listScreen, featurePlural string) []GUITestStep {
	vu := uiDef.ListPage.ViewUsageLink
	return []GUITestStep{
		{StepNumber: 1, Action: "navigate", Target: listScreen, Expected: fmt.Sprintf("%s page is displayed", featurePlural)},
		{StepNumber: 2, Action: "click", Target: "View Usage link", Expected: fmt.Sprintf("'%s' modal opens", vu.ModalTitle)},
		{StepNumber: 3, Action: "verify", Target: "modal title", Expected: fmt.Sprintf("Title: '%s'", vu.ModalTitle)},
		{StepNumber: 4, Action: "verify", Target: "X close button", Expected: "X close button is visible"},
		{StepNumber: 5, Action: "verify", Target: "Search bar", Expected: "Search bar is present in modal"},
		{StepNumber: 6, Action: "verify", Target: "table columns", Expected: fmt.Sprintf("Columns: %s", strings.Join(vu.TableColumns, ", "))},
		{StepNumber: 7, Action: "verify", Target: "empty state", Expected: fmt.Sprintf("Shows '%s' when no profiles exist", vu.EmptyState)},
		{StepNumber: 8, Action: "click", Target: "X close button", Expected: "Modal closes, returns to list page"},
	}
}
