package web

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v3"
)

// ---------- Figma Data Model ----------

// FigmaComponent represents a component extracted from a Figma file.
type FigmaComponent struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Description    string `json:"description"`
	ContainingPage string `json:"containingPage"`
	ComponentSetID string `json:"componentSetId,omitempty"`
	Type           string `json:"type"` // COMPONENT, COMPONENT_SET
}

// FigmaPage represents a page (canvas) from a Figma file.
type FigmaPage struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Children int    `json:"children"` // count of top-level frames
}

// FigmaImportResult captures results from a Figma import operation.
type FigmaImportResult struct {
	FileKey        string            `json:"fileKey"`
	FileName       string            `json:"fileName"`
	ImportedAt     time.Time         `json:"importedAt"`
	TotalPages     int               `json:"totalPages"`
	TotalFrames    int               `json:"totalFrames"`
	TotalComponents int              `json:"totalComponents"`
	Components     []FigmaComponent  `json:"components"`
	Pages          []FigmaPage       `json:"pages"`
	AutoMapped     int               `json:"autoMapped"`
	Unmapped       int               `json:"unmapped"`
	MappingDetails []FigmaMappingRow `json:"mappingDetails"`
}

// FigmaMappingRow shows how a Figma component was mapped to a widget type.
type FigmaMappingRow struct {
	ComponentName string `json:"componentName"`
	ComponentID   string `json:"componentId"`
	Page          string `json:"page"`
	MappedType    string `json:"mappedType"`   // widget type ID or empty
	MappedTypeName string `json:"mappedTypeName"`
	Confidence    string `json:"confidence"`   // high, medium, low, none
	Reason        string `json:"reason"`
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
	req, err := http.NewRequest("GET", figmaBaseURL+path, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("X-FIGMA-TOKEN", token)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("figma request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("reading figma response: %w", err)
	}
	return body, resp.StatusCode, nil
}

// figmaFileResponse is the structure of GET /v1/files/:key
type figmaFileResponse struct {
	Name       string          `json:"name"`
	LastModified string        `json:"lastModified"`
	Document   figmaNode       `json:"document"`
	Components map[string]figmaComponentMeta `json:"components"`
	ComponentSets map[string]figmaComponentMeta `json:"componentSets"`
}

type figmaNode struct {
	ID       string      `json:"id"`
	Name     string      `json:"name"`
	Type     string      `json:"type"`
	Children []figmaNode `json:"children"`
}

type figmaComponentMeta struct {
	Key            string `json:"key"`
	Name           string `json:"name"`
	Description    string `json:"description"`
	ComponentSetID string `json:"componentSetId"`
	ContainingFrame struct {
		PageID   string `json:"pageId"`
		PageName string `json:"pageName"`
	} `json:"containingFrame"`
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
	"text-field":       {"text field", "textfield", "text input", "textinput", "input field", "text_field", "text_input"},
	"text-area":        {"textarea", "text area", "text_area", "multiline"},
	"dropdown":         {"dropdown", "drop down", "select", "combobox", "combo box", "combo_box"},
	"multi-select":     {"multi select", "multiselect", "multi-select", "multi_select", "tag input", "tag_input"},
	"checkbox":         {"checkbox", "check box", "check_box"},
	"radio-group":      {"radio", "radio group", "radio_group", "radiogroup"},
	"toggle-switch":    {"toggle", "switch", "toggle switch"},
	"button":           {"button", "btn", "cta"},
	"icon-button":      {"icon button", "icon_button", "iconbutton", "action icon"},
	"data-table":       {"table", "data table", "datagrid", "data grid", "grid view"},
	"modal-dialog":     {"modal", "dialog", "popup", "overlay"},
	"tab-group":        {"tab", "tabs", "tab group", "tab_group", "tabgroup"},
	"breadcrumb":       {"breadcrumb", "breadcrumbs"},
	"search-bar":       {"search", "search bar", "searchbar", "search_bar", "search field"},
	"toast-notification": {"toast", "notification", "snackbar", "alert", "banner"},
	"ip-address-field": {"ip address", "ip field", "ip_address", "ipaddress", "ip input"},
	"vlan-selector":    {"vlan", "vlan id", "vlan_id"},
	"port-selector":    {"port", "port selector", "port_selector"},
	"file-upload":      {"upload", "file upload", "file_upload", "fileupload"},
	"date-picker":      {"date", "date picker", "datepicker", "date_picker", "calendar"},
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
		"name":         fileResp.Name,
		"lastModified": fileResp.LastModified,
		"pages":        pages,
		"totalPages":   len(pages),
		"totalFrames":  totalFrames,
		"components":   len(fileResp.Components),
		"componentSets": len(fileResp.ComponentSets),
	})
}

// GET /api/figma/components — list all components from a Figma file
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
	if fileKey == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no Figma file key"})
		return
	}

	body, status, err := figmaRequest(fcfg.Token, "/files/"+fileKey)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	if status != 200 {
		c.JSON(status, gin.H{"error": fmt.Sprintf("Figma API returned %d", status)})
		return
	}

	var fileResp figmaFileResponse
	if err := json.Unmarshal(body, &fileResp); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "parsing Figma response: " + err.Error()})
		return
	}

	var components []FigmaComponent
	for id, meta := range fileResp.Components {
		components = append(components, FigmaComponent{
			ID:             id,
			Name:           meta.Name,
			Description:    meta.Description,
			ContainingPage: meta.ContainingFrame.PageName,
			ComponentSetID: meta.ComponentSetID,
			Type:           "COMPONENT",
		})
	}
	for id, meta := range fileResp.ComponentSets {
		components = append(components, FigmaComponent{
			ID:             id,
			Name:           meta.Name,
			Description:    meta.Description,
			ContainingPage: meta.ContainingFrame.PageName,
			Type:           "COMPONENT_SET",
		})
	}

	sort.Slice(components, func(i, j int) bool {
		if components[i].ContainingPage != components[j].ContainingPage {
			return components[i].ContainingPage < components[j].ContainingPage
		}
		return components[i].Name < components[j].Name
	})

	c.JSON(http.StatusOK, gin.H{
		"total":      len(components),
		"components": components,
	})
}

// POST /api/figma/import — import components and auto-map to widget types
func (s *Server) handleFigmaImport(c *gin.Context) {
	fcfg, err := s.loadFigmaConfig()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var req struct {
		FileKey    string `json:"fileKey"`
		AutoCreate bool   `json:"autoCreate"` // auto-create widget instances for mapped components
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		// Allow empty body
		req.AutoCreate = false
	}
	fileKey := req.FileKey
	if fileKey == "" {
		fileKey = fcfg.FileKey
	}
	if fileKey == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no Figma file key"})
		return
	}

	log.Printf("Figma import: fetching file %s", fileKey)

	// Fetch full file
	body, status, err := figmaRequest(fcfg.Token, "/files/"+fileKey)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	if status != 200 {
		c.JSON(status, gin.H{"error": fmt.Sprintf("Figma API returned %d", status)})
		return
	}

	var fileResp figmaFileResponse
	if err := json.Unmarshal(body, &fileResp); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "parsing: " + err.Error()})
		return
	}

	// Extract pages
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

	// Extract components
	var components []FigmaComponent
	for id, meta := range fileResp.Components {
		components = append(components, FigmaComponent{
			ID:             id,
			Name:           meta.Name,
			Description:    meta.Description,
			ContainingPage: meta.ContainingFrame.PageName,
			ComponentSetID: meta.ComponentSetID,
			Type:           "COMPONENT",
		})
	}
	for id, meta := range fileResp.ComponentSets {
		components = append(components, FigmaComponent{
			ID:             id,
			Name:           meta.Name,
			Description:    meta.Description,
			ContainingPage: meta.ContainingFrame.PageName,
			Type:           "COMPONENT_SET",
		})
	}

	sort.Slice(components, func(i, j int) bool {
		return components[i].Name < components[j].Name
	})

	// Load widget types for auto-mapping
	widgetTypes, err := s.store.ListWidgetTypes()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "loading widget types: " + err.Error()})
		return
	}

	// Auto-map each component
	var mappings []FigmaMappingRow
	autoMapped := 0
	unmapped := 0
	for _, comp := range components {
		typeID, typeName, conf, reason := autoMapComponent(comp.Name, widgetTypes)
		mappings = append(mappings, FigmaMappingRow{
			ComponentName:  comp.Name,
			ComponentID:    comp.ID,
			Page:           comp.ContainingPage,
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
				Screen:     m.Page,
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
		FileKey:         fileKey,
		FileName:        fileResp.Name,
		ImportedAt:      time.Now().UTC(),
		TotalPages:      len(pages),
		TotalFrames:     totalFrames,
		TotalComponents: len(components),
		Components:      components,
		Pages:           pages,
		AutoMapped:      autoMapped,
		Unmapped:        unmapped,
		MappingDetails:  mappings,
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
		Page          string `json:"page"`
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
		Screen:     req.Page,
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
