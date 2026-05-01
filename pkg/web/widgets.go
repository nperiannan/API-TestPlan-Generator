package web

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/minio/minio-go/v7"
)

// ---------- Data Model ----------

// WidgetType defines a category of UI widget and the test actions it supports.
type WidgetType struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Category    string       `json:"category"` // input, action, display, container, navigation
	Description string       `json:"description"`
	TestActions []TestAction `json:"testActions"`
	Properties  []string     `json:"properties"` // common properties (label, placeholder, maxLength, …)
	CreatedAt   time.Time    `json:"createdAt"`
	UpdatedAt   time.Time    `json:"updatedAt"`
}

// TestAction describes one test-action pattern applicable to a widget type.
type TestAction struct {
	Name        string `json:"name"`        // click, type, select, verify_visible, …
	Description string `json:"description"` // human-readable explanation
	Priority    string `json:"priority"`    // P1, P2, P3
	Category    string `json:"category"`    // functional, boundary, negative
}

// WidgetInstance is a concrete widget on a specific XIQ screen.
type WidgetInstance struct {
	ID         string            `json:"id"`
	WidgetType string            `json:"widgetType"` // references WidgetType.ID
	Screen     string            `json:"screen"`     // XIQ screen/page
	Section    string            `json:"section"`    // section within the screen
	Label      string            `json:"label"`      // visible text
	Selector   string            `json:"selector"`   // CSS / XPath locator
	Properties map[string]string `json:"properties"` // extra key-value pairs
	CreatedAt  time.Time         `json:"createdAt"`
	UpdatedAt  time.Time         `json:"updatedAt"`
}

// ---------- MinIO Storage ----------

const (
	widgetTypesKey     = "widgets/types.json"
	widgetInstancesKey = "widgets/instances.json"
)

func (s *MinIOStore) loadJSON(key string, dst interface{}) error {
	data, err := s.getObject(context.Background(), key)
	if err != nil {
		// If the object doesn't exist, treat as empty
		if isNotFound(err) {
			return nil
		}
		return err
	}
	return json.Unmarshal(data, dst)
}

func (s *MinIOStore) saveJSON(key string, src interface{}) error {
	data, err := json.MarshalIndent(src, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %s: %w", key, err)
	}
	_, err = s.client.PutObject(context.Background(), s.bucket, key,
		bytes.NewReader(data), int64(len(data)),
		minio.PutObjectOptions{ContentType: "application/json"})
	if err != nil {
		return fmt.Errorf("put %s: %w", key, err)
	}
	return nil
}

func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "NoSuchKey") || strings.Contains(msg, "The specified key does not exist")
}

// --- Widget Types ---

func (s *MinIOStore) ListWidgetTypes() ([]WidgetType, error) {
	var types []WidgetType
	if err := s.loadJSON(widgetTypesKey, &types); err != nil {
		return nil, err
	}
	if types == nil {
		types = []WidgetType{}
	}
	return types, nil
}

func (s *MinIOStore) SaveWidgetTypes(types []WidgetType) error {
	return s.saveJSON(widgetTypesKey, types)
}

// --- Widget Instances ---

func (s *MinIOStore) ListWidgetInstances() ([]WidgetInstance, error) {
	var instances []WidgetInstance
	if err := s.loadJSON(widgetInstancesKey, &instances); err != nil {
		return nil, err
	}
	if instances == nil {
		instances = []WidgetInstance{}
	}
	return instances, nil
}

func (s *MinIOStore) SaveWidgetInstances(instances []WidgetInstance) error {
	return s.saveJSON(widgetInstancesKey, instances)
}

// ---------- HTTP Handlers ----------

// GET /api/widgets/types
func (s *Server) handleListWidgetTypes(c *gin.Context) {
	types, err := s.store.ListWidgetTypes()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, types)
}

// POST /api/widgets/types
func (s *Server) handleCreateWidgetType(c *gin.Context) {
	var wt WidgetType
	if err := c.ShouldBindJSON(&wt); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if wt.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}

	types, err := s.store.ListWidgetTypes()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Generate ID from name
	wt.ID = toSlug(wt.Name)
	for _, t := range types {
		if t.ID == wt.ID {
			c.JSON(http.StatusConflict, gin.H{"error": "widget type with this name already exists"})
			return
		}
	}
	now := time.Now().UTC()
	wt.CreatedAt = now
	wt.UpdatedAt = now
	if wt.TestActions == nil {
		wt.TestActions = []TestAction{}
	}
	if wt.Properties == nil {
		wt.Properties = []string{}
	}

	types = append(types, wt)
	if err := s.store.SaveWidgetTypes(types); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, wt)
}

// PUT /api/widgets/types/:id
func (s *Server) handleUpdateWidgetType(c *gin.Context) {
	id := c.Param("id")
	var wt WidgetType
	if err := c.ShouldBindJSON(&wt); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	types, err := s.store.ListWidgetTypes()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	idx := -1
	for i, t := range types {
		if t.ID == id {
			idx = i
			break
		}
	}
	if idx == -1 {
		c.JSON(http.StatusNotFound, gin.H{"error": "widget type not found"})
		return
	}

	wt.ID = id
	wt.CreatedAt = types[idx].CreatedAt
	wt.UpdatedAt = time.Now().UTC()
	if wt.TestActions == nil {
		wt.TestActions = []TestAction{}
	}
	if wt.Properties == nil {
		wt.Properties = []string{}
	}
	types[idx] = wt

	if err := s.store.SaveWidgetTypes(types); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, wt)
}

// DELETE /api/widgets/types/:id
func (s *Server) handleDeleteWidgetType(c *gin.Context) {
	id := c.Param("id")
	types, err := s.store.ListWidgetTypes()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	found := false
	var updated []WidgetType
	for _, t := range types {
		if t.ID == id {
			found = true
			continue
		}
		updated = append(updated, t)
	}
	if !found {
		c.JSON(http.StatusNotFound, gin.H{"error": "widget type not found"})
		return
	}

	if err := s.store.SaveWidgetTypes(updated); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": id})
}

// GET /api/widgets/instances
func (s *Server) handleListWidgetInstances(c *gin.Context) {
	instances, err := s.store.ListWidgetInstances()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Optional filters
	screen := c.Query("screen")
	widgetType := c.Query("type")
	if screen != "" || widgetType != "" {
		var filtered []WidgetInstance
		for _, inst := range instances {
			if screen != "" && !strings.EqualFold(inst.Screen, screen) {
				continue
			}
			if widgetType != "" && inst.WidgetType != widgetType {
				continue
			}
			filtered = append(filtered, inst)
		}
		if filtered == nil {
			filtered = []WidgetInstance{}
		}
		c.JSON(http.StatusOK, filtered)
		return
	}
	c.JSON(http.StatusOK, instances)
}

// POST /api/widgets/instances
func (s *Server) handleCreateWidgetInstance(c *gin.Context) {
	var wi WidgetInstance
	if err := c.ShouldBindJSON(&wi); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if wi.WidgetType == "" || wi.Screen == "" || wi.Label == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "widgetType, screen, and label are required"})
		return
	}

	// Verify widget type exists
	types, err := s.store.ListWidgetTypes()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	typeFound := false
	for _, t := range types {
		if t.ID == wi.WidgetType {
			typeFound = true
			break
		}
	}
	if !typeFound {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("widget type %q not found", wi.WidgetType)})
		return
	}

	instances, err := s.store.ListWidgetInstances()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Generate ID
	wi.ID = fmt.Sprintf("%s-%s-%s", toSlug(wi.Screen), toSlug(wi.Section), toSlug(wi.Label))
	for _, inst := range instances {
		if inst.ID == wi.ID {
			c.JSON(http.StatusConflict, gin.H{"error": "widget instance with this screen/section/label already exists"})
			return
		}
	}

	now := time.Now().UTC()
	wi.CreatedAt = now
	wi.UpdatedAt = now
	if wi.Properties == nil {
		wi.Properties = map[string]string{}
	}

	instances = append(instances, wi)
	if err := s.store.SaveWidgetInstances(instances); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, wi)
}

// PUT /api/widgets/instances/:id
func (s *Server) handleUpdateWidgetInstance(c *gin.Context) {
	id := c.Param("id")
	var wi WidgetInstance
	if err := c.ShouldBindJSON(&wi); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	instances, err := s.store.ListWidgetInstances()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	idx := -1
	for i, inst := range instances {
		if inst.ID == id {
			idx = i
			break
		}
	}
	if idx == -1 {
		c.JSON(http.StatusNotFound, gin.H{"error": "widget instance not found"})
		return
	}

	wi.ID = id
	wi.CreatedAt = instances[idx].CreatedAt
	wi.UpdatedAt = time.Now().UTC()
	if wi.Properties == nil {
		wi.Properties = map[string]string{}
	}
	instances[idx] = wi

	if err := s.store.SaveWidgetInstances(instances); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, wi)
}

// DELETE /api/widgets/instances/:id
func (s *Server) handleDeleteWidgetInstance(c *gin.Context) {
	id := c.Param("id")
	instances, err := s.store.ListWidgetInstances()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	found := false
	var updated []WidgetInstance
	for _, inst := range instances {
		if inst.ID == id {
			found = true
			continue
		}
		updated = append(updated, inst)
	}
	if !found {
		c.JSON(http.StatusNotFound, gin.H{"error": "widget instance not found"})
		return
	}

	if err := s.store.SaveWidgetInstances(updated); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": id})
}

// GET /api/widgets/stats
func (s *Server) handleWidgetStats(c *gin.Context) {
	types, err := s.store.ListWidgetTypes()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	instances, err := s.store.ListWidgetInstances()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Count by category
	catCounts := map[string]int{}
	totalActions := 0
	for _, t := range types {
		catCounts[t.Category]++
		totalActions += len(t.TestActions)
	}

	// Count instances by screen
	screenCounts := map[string]int{}
	for _, inst := range instances {
		screenCounts[inst.Screen]++
	}

	c.JSON(http.StatusOK, gin.H{
		"totalTypes":        len(types),
		"totalInstances":    len(instances),
		"totalTestActions":  totalActions,
		"typesByCategory":   catCounts,
		"instancesByScreen": screenCounts,
	})
}

// POST /api/widgets/seed — populate with default XIQ widget types
func (s *Server) handleSeedWidgetTypes(c *gin.Context) {
	existing, err := s.store.ListWidgetTypes()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if len(existing) > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "catalog already has widget types; clear first or add individually"})
		return
	}

	now := time.Now().UTC()
	seed := defaultXIQWidgetTypes(now)

	if err := s.store.SaveWidgetTypes(seed); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"seeded": len(seed)})
}

// ---------- Helpers ----------

func toSlug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			return r
		}
		return '-'
	}, s)
	// collapse multiple dashes
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	return strings.Trim(s, "-")
}

// defaultXIQWidgetTypes returns pre-built widget types for ExtremeCloud IQ.
func defaultXIQWidgetTypes(now time.Time) []WidgetType {
	return []WidgetType{
		{
			ID: "text-field", Name: "Text Field", Category: "input",
			Description: "Single-line text input field",
			Properties:  []string{"label", "placeholder", "maxLength", "pattern", "required", "disabled"},
			TestActions: []TestAction{
				{Name: "type_text", Description: "Enter text into the field", Priority: "P1", Category: "functional"},
				{Name: "clear_text", Description: "Clear the field content", Priority: "P2", Category: "functional"},
				{Name: "verify_value", Description: "Verify the current field value", Priority: "P1", Category: "functional"},
				{Name: "verify_placeholder", Description: "Verify placeholder text is shown", Priority: "P3", Category: "functional"},
				{Name: "test_max_length", Description: "Enter text exceeding maxLength", Priority: "P2", Category: "boundary"},
				{Name: "test_empty", Description: "Submit with required field empty", Priority: "P1", Category: "negative"},
				{Name: "test_special_chars", Description: "Enter special characters", Priority: "P2", Category: "boundary"},
				{Name: "verify_disabled", Description: "Verify field is disabled when expected", Priority: "P3", Category: "functional"},
			},
			CreatedAt: now, UpdatedAt: now,
		},
		{
			ID: "text-area", Name: "Text Area", Category: "input",
			Description: "Multi-line text input area",
			Properties:  []string{"label", "placeholder", "maxLength", "rows", "required", "disabled"},
			TestActions: []TestAction{
				{Name: "type_text", Description: "Enter multi-line text", Priority: "P1", Category: "functional"},
				{Name: "clear_text", Description: "Clear the text area", Priority: "P2", Category: "functional"},
				{Name: "verify_value", Description: "Verify the current text content", Priority: "P1", Category: "functional"},
				{Name: "test_max_length", Description: "Enter text exceeding maxLength", Priority: "P2", Category: "boundary"},
				{Name: "test_empty", Description: "Submit with required textarea empty", Priority: "P1", Category: "negative"},
			},
			CreatedAt: now, UpdatedAt: now,
		},
		{
			ID: "dropdown", Name: "Dropdown / Select", Category: "input",
			Description: "Single-selection dropdown list",
			Properties:  []string{"label", "options", "defaultValue", "required", "disabled", "searchable"},
			TestActions: []TestAction{
				{Name: "select_option", Description: "Select an option from the dropdown", Priority: "P1", Category: "functional"},
				{Name: "verify_selected", Description: "Verify currently selected option", Priority: "P1", Category: "functional"},
				{Name: "verify_options", Description: "Verify all available options are listed", Priority: "P2", Category: "functional"},
				{Name: "select_default", Description: "Verify default selection", Priority: "P2", Category: "functional"},
				{Name: "search_option", Description: "Search for an option in searchable dropdown", Priority: "P2", Category: "functional"},
				{Name: "test_no_selection", Description: "Submit without selecting (required)", Priority: "P1", Category: "negative"},
			},
			CreatedAt: now, UpdatedAt: now,
		},
		{
			ID: "multi-select", Name: "Multi-Select", Category: "input",
			Description: "Multiple-selection dropdown or list",
			Properties:  []string{"label", "options", "maxSelections", "required", "disabled"},
			TestActions: []TestAction{
				{Name: "select_multiple", Description: "Select multiple options", Priority: "P1", Category: "functional"},
				{Name: "deselect_option", Description: "Deselect a previously selected option", Priority: "P2", Category: "functional"},
				{Name: "select_all", Description: "Select all available options", Priority: "P2", Category: "boundary"},
				{Name: "verify_selections", Description: "Verify all current selections", Priority: "P1", Category: "functional"},
				{Name: "test_max_selections", Description: "Try to exceed max selections", Priority: "P2", Category: "boundary"},
			},
			CreatedAt: now, UpdatedAt: now,
		},
		{
			ID: "checkbox", Name: "Checkbox", Category: "input",
			Description: "Boolean toggle checkbox",
			Properties:  []string{"label", "checked", "disabled", "required"},
			TestActions: []TestAction{
				{Name: "check", Description: "Check the checkbox", Priority: "P1", Category: "functional"},
				{Name: "uncheck", Description: "Uncheck the checkbox", Priority: "P1", Category: "functional"},
				{Name: "verify_state", Description: "Verify checked/unchecked state", Priority: "P1", Category: "functional"},
				{Name: "verify_disabled", Description: "Verify checkbox is disabled when expected", Priority: "P3", Category: "functional"},
			},
			CreatedAt: now, UpdatedAt: now,
		},
		{
			ID: "radio-group", Name: "Radio Button Group", Category: "input",
			Description: "Mutually exclusive radio button selection",
			Properties:  []string{"label", "options", "defaultValue", "required", "disabled"},
			TestActions: []TestAction{
				{Name: "select_option", Description: "Select a radio option", Priority: "P1", Category: "functional"},
				{Name: "verify_selected", Description: "Verify selected radio option", Priority: "P1", Category: "functional"},
				{Name: "switch_option", Description: "Change from one option to another", Priority: "P1", Category: "functional"},
				{Name: "verify_default", Description: "Verify default selection on load", Priority: "P2", Category: "functional"},
			},
			CreatedAt: now, UpdatedAt: now,
		},
		{
			ID: "toggle-switch", Name: "Toggle Switch", Category: "input",
			Description: "On/off toggle switch (e.g., enable/disable features)",
			Properties:  []string{"label", "enabled", "disabled"},
			TestActions: []TestAction{
				{Name: "toggle_on", Description: "Turn the toggle on", Priority: "P1", Category: "functional"},
				{Name: "toggle_off", Description: "Turn the toggle off", Priority: "P1", Category: "functional"},
				{Name: "verify_state", Description: "Verify current toggle state", Priority: "P1", Category: "functional"},
				{Name: "verify_side_effects", Description: "Verify UI changes when toggled", Priority: "P2", Category: "functional"},
			},
			CreatedAt: now, UpdatedAt: now,
		},
		{
			ID: "button", Name: "Button", Category: "action",
			Description: "Clickable action button",
			Properties:  []string{"label", "type", "disabled", "icon"},
			TestActions: []TestAction{
				{Name: "click", Description: "Click the button", Priority: "P1", Category: "functional"},
				{Name: "verify_label", Description: "Verify button label text", Priority: "P2", Category: "functional"},
				{Name: "verify_disabled", Description: "Verify button is disabled when expected", Priority: "P2", Category: "functional"},
				{Name: "verify_enabled", Description: "Verify button is enabled when expected", Priority: "P2", Category: "functional"},
				{Name: "double_click", Description: "Double-click to check idempotency", Priority: "P3", Category: "negative"},
			},
			CreatedAt: now, UpdatedAt: now,
		},
		{
			ID: "icon-button", Name: "Icon Button", Category: "action",
			Description: "Button with icon only (edit, delete, etc.)",
			Properties:  []string{"icon", "tooltip", "disabled"},
			TestActions: []TestAction{
				{Name: "click", Description: "Click the icon button", Priority: "P1", Category: "functional"},
				{Name: "verify_tooltip", Description: "Hover and verify tooltip text", Priority: "P3", Category: "functional"},
				{Name: "verify_disabled", Description: "Verify button is disabled when expected", Priority: "P2", Category: "functional"},
			},
			CreatedAt: now, UpdatedAt: now,
		},
		{
			ID: "data-table", Name: "Data Table", Category: "display",
			Description: "Tabular data display with sorting, filtering, pagination",
			Properties:  []string{"columns", "sortable", "filterable", "paginated", "selectable", "pageSize"},
			TestActions: []TestAction{
				{Name: "verify_columns", Description: "Verify table column headers", Priority: "P1", Category: "functional"},
				{Name: "verify_row_count", Description: "Verify expected number of rows", Priority: "P1", Category: "functional"},
				{Name: "sort_column", Description: "Sort by a column and verify order", Priority: "P2", Category: "functional"},
				{Name: "filter_data", Description: "Apply a filter and verify results", Priority: "P2", Category: "functional"},
				{Name: "paginate", Description: "Navigate between pages", Priority: "P2", Category: "functional"},
				{Name: "select_row", Description: "Select a row for actions", Priority: "P1", Category: "functional"},
				{Name: "select_all_rows", Description: "Select all rows", Priority: "P2", Category: "functional"},
				{Name: "test_empty_state", Description: "Verify table with no data", Priority: "P2", Category: "boundary"},
				{Name: "test_large_dataset", Description: "Verify with maximum rows", Priority: "P3", Category: "scale"},
			},
			CreatedAt: now, UpdatedAt: now,
		},
		{
			ID: "modal-dialog", Name: "Modal Dialog", Category: "container",
			Description: "Overlay modal for forms or confirmations",
			Properties:  []string{"title", "closable", "size", "hasFooter"},
			TestActions: []TestAction{
				{Name: "open_modal", Description: "Trigger the modal to open", Priority: "P1", Category: "functional"},
				{Name: "close_modal", Description: "Close the modal via X button", Priority: "P1", Category: "functional"},
				{Name: "close_outside", Description: "Click outside to close (if allowed)", Priority: "P2", Category: "functional"},
				{Name: "verify_title", Description: "Verify modal title text", Priority: "P2", Category: "functional"},
				{Name: "escape_key", Description: "Press Escape to close", Priority: "P3", Category: "functional"},
				{Name: "verify_focus_trap", Description: "Verify focus stays within modal", Priority: "P3", Category: "functional"},
			},
			CreatedAt: now, UpdatedAt: now,
		},
		{
			ID: "tab-group", Name: "Tab Group", Category: "navigation",
			Description: "Tab navigation within a page or section",
			Properties:  []string{"tabs", "defaultTab", "disabled"},
			TestActions: []TestAction{
				{Name: "switch_tab", Description: "Click a tab to switch", Priority: "P1", Category: "functional"},
				{Name: "verify_active_tab", Description: "Verify the currently active tab", Priority: "P1", Category: "functional"},
				{Name: "verify_tab_content", Description: "Verify content changes on tab switch", Priority: "P1", Category: "functional"},
				{Name: "verify_default_tab", Description: "Verify default tab on page load", Priority: "P2", Category: "functional"},
			},
			CreatedAt: now, UpdatedAt: now,
		},
		{
			ID: "breadcrumb", Name: "Breadcrumb", Category: "navigation",
			Description: "Navigation breadcrumb trail",
			Properties:  []string{"items", "separator"},
			TestActions: []TestAction{
				{Name: "click_link", Description: "Click a breadcrumb link to navigate", Priority: "P1", Category: "functional"},
				{Name: "verify_path", Description: "Verify breadcrumb shows correct path", Priority: "P2", Category: "functional"},
				{Name: "verify_current", Description: "Verify current page is not clickable", Priority: "P3", Category: "functional"},
			},
			CreatedAt: now, UpdatedAt: now,
		},
		{
			ID: "search-bar", Name: "Search Bar", Category: "input",
			Description: "Search input with filtering capability",
			Properties:  []string{"placeholder", "debounceMs", "minChars"},
			TestActions: []TestAction{
				{Name: "search_term", Description: "Enter a search term and verify results", Priority: "P1", Category: "functional"},
				{Name: "clear_search", Description: "Clear search and verify reset", Priority: "P2", Category: "functional"},
				{Name: "search_no_results", Description: "Search for non-existent term", Priority: "P2", Category: "negative"},
				{Name: "search_special_chars", Description: "Search with special characters", Priority: "P2", Category: "boundary"},
				{Name: "test_min_chars", Description: "Type fewer than minimum characters", Priority: "P3", Category: "boundary"},
			},
			CreatedAt: now, UpdatedAt: now,
		},
		{
			ID: "toast-notification", Name: "Toast / Notification", Category: "display",
			Description: "Temporary notification message (success, error, warning, info)",
			Properties:  []string{"type", "message", "duration", "dismissable"},
			TestActions: []TestAction{
				{Name: "verify_shown", Description: "Verify notification appears", Priority: "P1", Category: "functional"},
				{Name: "verify_message", Description: "Verify notification message text", Priority: "P1", Category: "functional"},
				{Name: "verify_type", Description: "Verify notification type (success/error/warning)", Priority: "P2", Category: "functional"},
				{Name: "dismiss", Description: "Dismiss the notification", Priority: "P2", Category: "functional"},
				{Name: "verify_auto_close", Description: "Verify notification auto-closes", Priority: "P3", Category: "functional"},
			},
			CreatedAt: now, UpdatedAt: now,
		},
		{
			ID: "ip-address-field", Name: "IP Address Field", Category: "input",
			Description: "Specialized input for IPv4/IPv6 addresses (common in XIQ network config)",
			Properties:  []string{"label", "version", "required", "withSubnet"},
			TestActions: []TestAction{
				{Name: "enter_ipv4", Description: "Enter a valid IPv4 address", Priority: "P1", Category: "functional"},
				{Name: "enter_ipv6", Description: "Enter a valid IPv6 address", Priority: "P1", Category: "functional"},
				{Name: "test_invalid_ip", Description: "Enter an invalid IP address", Priority: "P1", Category: "negative"},
				{Name: "enter_subnet", Description: "Enter IP with subnet mask", Priority: "P2", Category: "functional"},
				{Name: "test_broadcast", Description: "Enter broadcast address", Priority: "P2", Category: "boundary"},
				{Name: "test_multicast", Description: "Enter multicast address", Priority: "P3", Category: "boundary"},
			},
			CreatedAt: now, UpdatedAt: now,
		},
		{
			ID: "vlan-selector", Name: "VLAN Selector", Category: "input",
			Description: "VLAN ID input (1-4094 range, common in XIQ)",
			Properties:  []string{"label", "min", "max", "required"},
			TestActions: []TestAction{
				{Name: "enter_valid_vlan", Description: "Enter a valid VLAN ID (1-4094)", Priority: "P1", Category: "functional"},
				{Name: "test_zero", Description: "Enter VLAN 0 (invalid)", Priority: "P1", Category: "negative"},
				{Name: "test_above_max", Description: "Enter VLAN above 4094", Priority: "P1", Category: "boundary"},
				{Name: "test_non_numeric", Description: "Enter non-numeric value", Priority: "P2", Category: "negative"},
				{Name: "verify_range", Description: "Verify allowed VLAN range display", Priority: "P2", Category: "functional"},
			},
			CreatedAt: now, UpdatedAt: now,
		},
		{
			ID: "port-selector", Name: "Port Selector", Category: "input",
			Description: "Network port selection widget (XIQ device port picker)",
			Properties:  []string{"label", "multiSelect", "deviceType", "portTypes"},
			TestActions: []TestAction{
				{Name: "select_port", Description: "Select a single port", Priority: "P1", Category: "functional"},
				{Name: "select_multiple_ports", Description: "Select multiple ports", Priority: "P1", Category: "functional"},
				{Name: "select_port_range", Description: "Select a contiguous port range", Priority: "P2", Category: "functional"},
				{Name: "verify_available_ports", Description: "Verify only valid ports are selectable", Priority: "P2", Category: "functional"},
				{Name: "deselect_port", Description: "Deselect a previously selected port", Priority: "P2", Category: "functional"},
			},
			CreatedAt: now, UpdatedAt: now,
		},
		{
			ID: "file-upload", Name: "File Upload", Category: "input",
			Description: "File upload widget for firmware, configs, etc.",
			Properties:  []string{"label", "accept", "maxSize", "multiple"},
			TestActions: []TestAction{
				{Name: "upload_valid", Description: "Upload a valid file", Priority: "P1", Category: "functional"},
				{Name: "verify_progress", Description: "Verify upload progress indicator", Priority: "P2", Category: "functional"},
				{Name: "test_invalid_type", Description: "Upload a file with wrong extension", Priority: "P1", Category: "negative"},
				{Name: "test_max_size", Description: "Upload a file exceeding max size", Priority: "P1", Category: "boundary"},
				{Name: "test_empty_file", Description: "Upload an empty file", Priority: "P2", Category: "boundary"},
			},
			CreatedAt: now, UpdatedAt: now,
		},
		{
			ID: "date-picker", Name: "Date Picker", Category: "input",
			Description: "Date/time selection widget for scheduling",
			Properties:  []string{"label", "format", "minDate", "maxDate", "required"},
			TestActions: []TestAction{
				{Name: "select_date", Description: "Select a date from the calendar", Priority: "P1", Category: "functional"},
				{Name: "type_date", Description: "Type a date manually", Priority: "P2", Category: "functional"},
				{Name: "verify_date", Description: "Verify the selected date value", Priority: "P1", Category: "functional"},
				{Name: "test_past_date", Description: "Select a date in the past (if restricted)", Priority: "P2", Category: "boundary"},
				{Name: "test_invalid_format", Description: "Enter date in wrong format", Priority: "P2", Category: "negative"},
			},
			CreatedAt: now, UpdatedAt: now,
		},
	}
}
