package web

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v3"
)

// ---------- Figma Data Model ----------

// FigmaComponent represents a widget element (INSTANCE) extracted from a Figma design.
type FigmaComponent struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Screen      string `json:"screen"`      // parent frame (screen name)
	ComponentID string `json:"componentId"` // Figma component definition ID
	Type        string `json:"type"`        // Figma node type (INSTANCE, FRAME, etc.)
}

// FigmaPage represents a page (canvas) from a Figma file.
type FigmaPage struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Children int    `json:"children"` // count of top-level frames
}

// FigmaScreen represents a distinct screen found inside a Figma section.
type FigmaScreen struct {
	Name    string `json:"name"`
	ID      string `json:"id"`
	Widgets int    `json:"widgets"` // count of INSTANCE elements
}

// FigmaImportResult captures results from a Figma import operation.
type FigmaImportResult struct {
	FileKey        string            `json:"fileKey"`
	FileName       string            `json:"fileName"`
	NodeID         string            `json:"nodeId,omitempty"`
	SectionName    string            `json:"sectionName,omitempty"`
	ImportedAt     time.Time         `json:"importedAt"`
	TotalPages     int               `json:"totalPages,omitempty"`
	TotalScreens   int               `json:"totalScreens"`
	Screens        []FigmaScreen     `json:"screens"`
	TotalWidgets   int               `json:"totalWidgets"`
	UniqueWidgets  int               `json:"uniqueWidgets"`
	Components     []FigmaComponent  `json:"components"`
	AutoMapped     int               `json:"autoMapped"`
	Unmapped       int               `json:"unmapped"`
	MappingDetails []FigmaMappingRow `json:"mappingDetails"`
}

// FigmaMappingRow shows how a Figma component was mapped to a widget type.
type FigmaMappingRow struct {
	ComponentName  string `json:"componentName"`
	ComponentID    string `json:"componentId"`
	Screen         string `json:"screen"`
	MappedType     string `json:"mappedType"` // widget type ID or empty
	MappedTypeName string `json:"mappedTypeName"`
	Confidence     string `json:"confidence"` // high, medium, low, none
	Reason         string `json:"reason"`
}

// ---------- Figma configuration ----------

type figmaConfig struct {
	Token   string `yaml:"-" json:"-"`
	FileKey string `yaml:"fileKey" json:"fileKey"`
}

func (s *Server) loadFigmaConfig() (*figmaConfig, error) {
	cfgPath := filepath.Join("config", "config.yaml")
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return nil, err
	}
	var cfg struct {
		Figma struct {
			FileKey string `yaml:"fileKey"`
		} `yaml:"figma"`
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	token := os.Getenv("FIGMA_API_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("FIGMA_API_TOKEN environment variable not set")
	}

	return &figmaConfig{
		Token:   token,
		FileKey: cfg.Figma.FileKey,
	}, nil
}

// ---------- Figma REST API Client ----------

const figmaBaseURL = "https://api.figma.com/v1"

func figmaRequest(token, path string) ([]byte, int, error) {
	client := &http.Client{Timeout: 120 * time.Second}
	maxRetries := 3

	for attempt := 0; attempt <= maxRetries; attempt++ {
		req, err := http.NewRequest("GET", figmaBaseURL+path, nil)
		if err != nil {
			return nil, 0, err
		}
		req.Header.Set("X-FIGMA-TOKEN", token)

		resp, err := client.Do(req)
		if err != nil {
			return nil, 0, fmt.Errorf("figma request failed: %w", err)
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, resp.StatusCode, fmt.Errorf("reading figma response: %w", err)
		}

		if resp.StatusCode == 429 && attempt < maxRetries {
			wait := time.Duration(2<<uint(attempt)) * time.Second // 2s, 4s, 8s
			log.Printf("[figma] rate limited (429), retrying in %v (attempt %d/%d)", wait, attempt+1, maxRetries)
			time.Sleep(wait)
			continue
		}

		return body, resp.StatusCode, nil
	}
	return nil, 429, fmt.Errorf("figma rate limit exceeded after %d retries", maxRetries)
}

// figmaFileResponse is the structure of GET /v1/files/:key
type figmaFileResponse struct {
	Name          string                        `json:"name"`
	LastModified  string                        `json:"lastModified"`
	Document      figmaNode                     `json:"document"`
	Components    map[string]figmaComponentMeta `json:"components"`
	ComponentSets map[string]figmaComponentMeta `json:"componentSets"`
}

type figmaNode struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Type        string      `json:"type"`
	ComponentID string      `json:"componentId,omitempty"`
	Children    []figmaNode `json:"children"`
}

type figmaComponentMeta struct {
	Key             string `json:"key"`
	Name            string `json:"name"`
	Description     string `json:"description"`
	ComponentSetID  string `json:"componentSetId"`
	ContainingFrame struct {
		PageID   string `json:"pageId"`
		PageName string `json:"pageName"`
	} `json:"containingFrame"`
}

// figmaNodesResponse is the structure of GET /v1/files/:key/nodes?ids=...
type figmaNodesResponse struct {
	Name  string                             `json:"name"`
	Nodes map[string]figmaNodesResponseEntry `json:"nodes"`
}

type figmaNodesResponseEntry struct {
	Document figmaNode `json:"document"`
}

// walkInstances traverses the Figma node tree and collects all INSTANCE elements
// grouped by their parent screen (top-level FRAME).
func walkInstances(node figmaNode) []FigmaComponent {
	var results []FigmaComponent
	// Top-level children of a SECTION are screens (FRAME)
	for _, screen := range node.Children {
		if screen.Type != "FRAME" {
			continue
		}
		walkNode(screen, screen.Name, &results)
	}
	return results
}

func walkNode(node figmaNode, screen string, results *[]FigmaComponent) {
	if node.Type == "INSTANCE" {
		*results = append(*results, FigmaComponent{
			ID:          node.ID,
			Name:        node.Name,
			Screen:      screen,
			ComponentID: node.ComponentID,
			Type:        node.Type,
		})
	}
	for _, child := range node.Children {
		walkNode(child, screen, results)
	}
}

// ---------- Figma MinIO Storage ----------

const figmaImportKey = "figma/last-import.json"

func (s *MinIOStore) LoadFigmaImport() (*FigmaImportResult, error) {
	var result FigmaImportResult
	if err := s.loadJSON(figmaImportKey, &result); err != nil {
		return nil, err
	}
	if result.FileKey == "" {
		return nil, nil // no import yet
	}
	return &result, nil
}

func (s *MinIOStore) SaveFigmaImport(result *FigmaImportResult) error {
	return s.saveJSON(figmaImportKey, result)
}

// ---------- Auto-Mapping Logic ----------

// componentTypePatterns maps Figma component name keywords → widget type IDs.
var componentTypePatterns = map[string][]string{
	"text-field":         {"text field", "textfield", "text input", "textinput", "input field", "text_field", "text_input"},
	"text-area":          {"textarea", "text area", "text_area", "multiline"},
	"dropdown":           {"dropdown", "drop down", "select", "combobox", "combo box", "combo_box"},
	"multi-select":       {"multi select", "multiselect", "multi-select", "multi_select", "tag input", "tag_input"},
	"checkbox":           {"checkbox", "check box", "check_box"},
	"radio-group":        {"radio", "radio group", "radio_group", "radiogroup"},
	"toggle-switch":      {"toggle", "switch", "toggle switch"},
	"button":             {"button", "btn", "cta"},
	"icon-button":        {"icon button", "icon_button", "iconbutton", "action icon"},
	"data-table":         {"table", "data table", "datagrid", "data grid", "grid view"},
	"modal-dialog":       {"modal", "dialog", "popup", "overlay"},
	"tab-group":          {"tab", "tabs", "tab group", "tab_group", "tabgroup"},
	"breadcrumb":         {"breadcrumb", "breadcrumbs"},
	"search-bar":         {"search", "search bar", "searchbar", "search_bar", "search field"},
	"toast-notification": {"toast", "notification", "snackbar", "alert", "banner"},
	"ip-address-field":   {"ip address", "ip field", "ip_address", "ipaddress", "ip input"},
	"vlan-selector":      {"vlan", "vlan id", "vlan_id"},
	"port-selector":      {"port", "port selector", "port_selector"},
	"file-upload":        {"upload", "file upload", "file_upload", "fileupload"},
	"date-picker":        {"date", "date picker", "datepicker", "date_picker", "calendar"},
}

func autoMapComponent(name string, widgetTypes []WidgetType) (typeID, typeName, confidence, reason string) {
	lowerName := strings.ToLower(name)

	// Exact component name match first
	for _, wt := range widgetTypes {
		if strings.EqualFold(wt.Name, name) {
			return wt.ID, wt.Name, "high", "exact name match"
		}
	}

	// Pattern-based matching
	bestMatch := ""
	bestMatchName := ""
	bestScore := 0
	for typeID, patterns := range componentTypePatterns {
		for _, pattern := range patterns {
			if strings.Contains(lowerName, pattern) {
				score := len(pattern) // longer pattern = more specific = better match
				if score > bestScore {
					bestScore = score
					bestMatch = typeID
					// Find the display name
					for _, wt := range widgetTypes {
						if wt.ID == typeID {
							bestMatchName = wt.Name
							break
						}
					}
				}
			}
		}
	}

	if bestMatch != "" {
		conf := "medium"
		if bestScore >= 8 {
			conf = "high"
		}
		return bestMatch, bestMatchName, conf, fmt.Sprintf("keyword match: %q in component name", lowerName)
	}

	return "", "", "none", "no matching pattern found"
}

// ---------- HTTP Handlers ----------

// GET /api/figma/config
func (s *Server) handleFigmaConfig(c *gin.Context) {
	fcfg, err := s.loadFigmaConfig()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"configured": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"configured": true,
		"fileKey":    fcfg.FileKey,
	})
}

// GET /api/figma/file — fetch file info from Figma
func (s *Server) handleFigmaFile(c *gin.Context) {
	fcfg, err := s.loadFigmaConfig()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	fileKey := c.Query("fileKey")
	if fileKey == "" {
		fileKey = fcfg.FileKey
	}
	if fileKey == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no Figma file key configured or provided"})
		return
	}

	body, status, err := figmaRequest(fcfg.Token, "/files/"+fileKey+"?depth=2")
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	if status != 200 {
		c.JSON(status, gin.H{"error": fmt.Sprintf("Figma API returned %d: %s", status, truncate(string(body), 500))})
		return
	}

	var fileResp figmaFileResponse
	if err := json.Unmarshal(body, &fileResp); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "parsing Figma response: " + err.Error()})
		return
	}

	// Extract pages and frame counts
	var pages []FigmaPage
	totalFrames := 0
	for _, child := range fileResp.Document.Children {
		frameCount := len(child.Children)
		totalFrames += frameCount
		pages = append(pages, FigmaPage{
			ID:       child.ID,
			Name:     child.Name,
			Children: frameCount,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"name":          fileResp.Name,
		"lastModified":  fileResp.LastModified,
		"pages":         pages,
		"totalPages":    len(pages),
		"totalFrames":   totalFrames,
		"components":    len(fileResp.Components),
		"componentSets": len(fileResp.ComponentSets),
	})
}

// GET /api/figma/components — list widget instances from a Figma section node
func (s *Server) handleFigmaComponents(c *gin.Context) {
	fcfg, err := s.loadFigmaConfig()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	fileKey := c.Query("fileKey")
	if fileKey == "" {
		fileKey = fcfg.FileKey
	}
	nodeID := c.Query("nodeId")
	if fileKey == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no Figma file key"})
		return
	}
	if nodeID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "nodeId query parameter required"})
		return
	}

	path := fmt.Sprintf("/files/%s/nodes?ids=%s&depth=6", fileKey, url.QueryEscape(nodeID))
	body, status, err := figmaRequest(fcfg.Token, path)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	if status != 200 {
		c.JSON(status, gin.H{"error": fmt.Sprintf("Figma API returned %d", status)})
		return
	}

	var nodesResp figmaNodesResponse
	if err := json.Unmarshal(body, &nodesResp); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "parsing Figma response: " + err.Error()})
		return
	}

	var allWidgets []FigmaComponent
	for _, entry := range nodesResp.Nodes {
		allWidgets = append(allWidgets, walkInstances(entry.Document)...)
	}

	// Deduplicate by name
	seen := make(map[string]bool)
	var unique []FigmaComponent
	for _, w := range allWidgets {
		if !seen[w.Name] {
			seen[w.Name] = true
			unique = append(unique, w)
		}
	}
	sort.Slice(unique, func(i, j int) bool { return unique[i].Name < unique[j].Name })

	c.JSON(http.StatusOK, gin.H{
		"total":      len(allWidgets),
		"unique":     len(unique),
		"components": unique,
	})
}

// POST /api/figma/import — import widgets from a Figma section and auto-map to widget types
func (s *Server) handleFigmaImport(c *gin.Context) {
	fcfg, err := s.loadFigmaConfig()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var req struct {
		FileKey    string `json:"fileKey"`
		NodeID     string `json:"nodeId"`     // Figma node ID (e.g., "3240:336907")
		AutoCreate bool   `json:"autoCreate"` // auto-create widget instances for mapped components
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		// Allow query params too
		req.AutoCreate = c.Query("autoCreate") == "true"
	}
	fileKey := req.FileKey
	if fileKey == "" {
		fileKey = c.Query("fileKey")
	}
	if fileKey == "" {
		fileKey = fcfg.FileKey
	}
	nodeID := req.NodeID
	if nodeID == "" {
		nodeID = c.Query("nodeId")
	}
	if fileKey == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no Figma file key"})
		return
	}
	if nodeID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "nodeId is required — provide the Figma section node ID from the URL"})
		return
	}

	log.Printf("Figma import: fetching file %s node %s", fileKey, nodeID)

	// Fetch the specific node with enough depth
	path := fmt.Sprintf("/files/%s/nodes?ids=%s&depth=6", fileKey, url.QueryEscape(nodeID))
	body, status, err := figmaRequest(fcfg.Token, path)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	if status != 200 {
		c.JSON(status, gin.H{"error": fmt.Sprintf("Figma API returned %d: %s", status, truncate(string(body), 500))})
		return
	}

	var nodesResp figmaNodesResponse
	if err := json.Unmarshal(body, &nodesResp); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "parsing: " + err.Error()})
		return
	}

	// Extract section name and walk for INSTANCE elements
	var sectionName string
	var allWidgets []FigmaComponent
	for _, entry := range nodesResp.Nodes {
		sectionName = entry.Document.Name
		allWidgets = append(allWidgets, walkInstances(entry.Document)...)
	}

	// Build screen list
	screenMap := make(map[string]int)
	for _, w := range allWidgets {
		screenMap[w.Screen]++
	}
	var screens []FigmaScreen
	for name, count := range screenMap {
		screens = append(screens, FigmaScreen{Name: name, Widgets: count})
	}
	sort.Slice(screens, func(i, j int) bool { return screens[i].Name < screens[j].Name })

	// Deduplicate widgets by name for mapping (keep first occurrence per name)
	seen := make(map[string]bool)
	var uniqueWidgets []FigmaComponent
	for _, w := range allWidgets {
		if !seen[w.Name] {
			seen[w.Name] = true
			uniqueWidgets = append(uniqueWidgets, w)
		}
	}
	sort.Slice(uniqueWidgets, func(i, j int) bool { return uniqueWidgets[i].Name < uniqueWidgets[j].Name })

	// Load widget types for auto-mapping
	widgetTypes, err := s.store.ListWidgetTypes()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "loading widget types: " + err.Error()})
		return
	}

	// Auto-map each unique widget
	var mappings []FigmaMappingRow
	autoMapped := 0
	unmapped := 0
	for _, comp := range uniqueWidgets {
		typeID, typeName, conf, reason := autoMapComponent(comp.Name, widgetTypes)
		mappings = append(mappings, FigmaMappingRow{
			ComponentName:  comp.Name,
			ComponentID:    comp.ComponentID,
			Screen:         comp.Screen,
			MappedType:     typeID,
			MappedTypeName: typeName,
			Confidence:     conf,
			Reason:         reason,
		})
		if typeID != "" {
			autoMapped++
		} else {
			unmapped++
		}
	}

	// Auto-create widget instances if requested
	if req.AutoCreate && autoMapped > 0 {
		instances, err := s.store.ListWidgetInstances()
		if err != nil {
			log.Printf("Warning: couldn't load existing instances: %v", err)
			instances = []WidgetInstance{}
		}
		existingIDs := make(map[string]bool)
		for _, inst := range instances {
			existingIDs[inst.ID] = true
		}

		now := time.Now().UTC()
		created := 0
		for _, m := range mappings {
			if m.MappedType == "" || m.Confidence == "none" {
				continue
			}
			id := fmt.Sprintf("figma-%s", toSlug(m.ComponentName))
			if existingIDs[id] {
				continue // already exists
			}
			instances = append(instances, WidgetInstance{
				ID:         id,
				WidgetType: m.MappedType,
				Screen:     m.Screen,
				Section:    "Figma Import",
				Label:      m.ComponentName,
				Selector:   fmt.Sprintf("[data-figma-id='%s']", m.ComponentID),
				Properties: map[string]string{
					"figmaComponentId": m.ComponentID,
					"importedAt":       now.Format(time.RFC3339),
					"confidence":       m.Confidence,
				},
				CreatedAt: now,
				UpdatedAt: now,
			})
			existingIDs[id] = true
			created++
		}

		if created > 0 {
			if err := s.store.SaveWidgetInstances(instances); err != nil {
				log.Printf("Warning: failed to save auto-created instances: %v", err)
			} else {
				log.Printf("Figma import: auto-created %d widget instances", created)
			}
		}
	}

	result := &FigmaImportResult{
		FileKey:        fileKey,
		FileName:       nodesResp.Name,
		NodeID:         nodeID,
		SectionName:    sectionName,
		ImportedAt:     time.Now().UTC(),
		TotalScreens:   len(screens),
		Screens:        screens,
		TotalWidgets:   len(allWidgets),
		UniqueWidgets:  len(uniqueWidgets),
		Components:     uniqueWidgets,
		AutoMapped:     autoMapped,
		Unmapped:       unmapped,
		MappingDetails: mappings,
	}

	// Save import result to MinIO
	if err := s.store.SaveFigmaImport(result); err != nil {
		log.Printf("Warning: failed to save Figma import: %v", err)
	}

	c.JSON(http.StatusOK, result)
}

// GET /api/figma/import — get the last import result
func (s *Server) handleFigmaLastImport(c *gin.Context) {
	result, err := s.store.LoadFigmaImport()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if result == nil {
		c.JSON(http.StatusOK, gin.H{"imported": false})
		return
	}
	c.JSON(http.StatusOK, gin.H{"imported": true, "result": result})
}

// POST /api/figma/map — manually map a Figma component to a widget type
func (s *Server) handleFigmaManualMap(c *gin.Context) {
	var req struct {
		ComponentID   string `json:"componentId" binding:"required"`
		ComponentName string `json:"componentName" binding:"required"`
		Screen        string `json:"screen"`
		WidgetTypeID  string `json:"widgetTypeId" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Verify widget type exists
	types, err := s.store.ListWidgetTypes()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	var typeName string
	for _, t := range types {
		if t.ID == req.WidgetTypeID {
			typeName = t.Name
			break
		}
	}
	if typeName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("widget type %q not found", req.WidgetTypeID)})
		return
	}

	// Create widget instance
	instances, err := s.store.ListWidgetInstances()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	id := fmt.Sprintf("figma-%s", toSlug(req.ComponentName))
	// Check for existing
	for i, inst := range instances {
		if inst.ID == id {
			// Update existing
			instances[i].WidgetType = req.WidgetTypeID
			instances[i].UpdatedAt = time.Now().UTC()
			if err := s.store.SaveWidgetInstances(instances); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, gin.H{"action": "updated", "instance": instances[i]})
			return
		}
	}

	now := time.Now().UTC()
	inst := WidgetInstance{
		ID:         id,
		WidgetType: req.WidgetTypeID,
		Screen:     req.Screen,
		Section:    "Figma Import",
		Label:      req.ComponentName,
		Selector:   fmt.Sprintf("[data-figma-id='%s']", req.ComponentID),
		Properties: map[string]string{
			"figmaComponentId": req.ComponentID,
			"importedAt":       now.Format(time.RFC3339),
			"manualMapping":    "true",
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	instances = append(instances, inst)

	if err := s.store.SaveWidgetInstances(instances); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"action": "created", "instance": inst})
}

// ---------- Helpers ----------

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
