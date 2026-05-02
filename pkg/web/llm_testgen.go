package web

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// ---------- LLM Configuration ----------

type llmConfig struct {
	Provider string // "anthropic" or "openai"
	APIKey   string
	Model    string
}

func loadLLMConfig() (*llmConfig, error) {
	// Try Anthropic first
	if key := os.Getenv("ANTHROPIC_API_KEY"); key != "" {
		model := os.Getenv("ANTHROPIC_MODEL")
		if model == "" {
			model = "claude-sonnet-4-20250514"
		}
		return &llmConfig{Provider: "anthropic", APIKey: key, Model: model}, nil
	}
	// Try OpenAI
	if key := os.Getenv("OPENAI_API_KEY"); key != "" {
		model := os.Getenv("OPENAI_MODEL")
		if model == "" {
			model = "gpt-4o"
		}
		return &llmConfig{Provider: "openai", APIKey: key, Model: model}, nil
	}
	return nil, fmt.Errorf("no LLM API key configured. Set ANTHROPIC_API_KEY or OPENAI_API_KEY environment variable")
}

// ---------- LLM Analysis Request/Response ----------

// handleLLMAnalyzeScreens receives Figma screen images and sends them to an LLM
// to extract UI patterns, generating the JSON pattern file automatically.
//
// POST /api/llm/analyze-screens
// Body: { "fileKey": "...", "nodeId": "...", "featureName": "radius-server", "category": "global-profile" }
//
// The flow:
// 1. Export Figma screens as images (or use already-exported ones)
// 2. Load YANG model data for the feature
// 3. Send images + YANG context to LLM with a detailed prompt
// 4. LLM returns UI pattern JSON
// 5. Save to config/ui-patterns/<category>.json
func (s *Server) handleLLMAnalyzeScreens(c *gin.Context) {
	var req struct {
		FileKey     string `json:"fileKey"`
		NodeID      string `json:"nodeId"`
		FeatureName string `json:"featureName"`
		Category    string `json:"category"`
		JiraKey     string `json:"jiraKey,omitempty"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	if req.FeatureName == "" || req.Category == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "featureName and category are required"})
		return
	}

	llmCfg, err := loadLLMConfig()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	log.Printf("[llm] Starting UI analysis for %s/%s (LLM: %s/%s)", req.Category, req.FeatureName, llmCfg.Provider, llmCfg.Model)

	// Step 1: Get or export Figma screen images
	var imagePaths []string

	if req.FileKey != "" {
		imgDir := filepath.Join("data", "figma-screens", req.FileKey)
		if entries, err := os.ReadDir(imgDir); err == nil && len(entries) > 0 {
			// Use existing images
			for _, e := range entries {
				if !e.IsDir() && (strings.HasSuffix(e.Name(), ".png") || strings.HasSuffix(e.Name(), ".jpg")) {
					imagePaths = append(imagePaths, filepath.Join(imgDir, e.Name()))
				}
			}
			log.Printf("[llm] Using %d existing Figma screen images from %s", len(imagePaths), imgDir)
		}

		// If no images found, try to export them
		if len(imagePaths) == 0 {
			fcfg, err := s.loadFigmaConfig()
			if err == nil {
				exported := s.exportFigmaImages(fcfg, req.FileKey, req.NodeID)
				imagePaths = exported
			}
		}
	}

	// Also check for manually-placed screenshots
	manualDir := filepath.Join("data", "screenshots", req.Category, req.FeatureName)
	if entries, err := os.ReadDir(manualDir); err == nil {
		for _, e := range entries {
			if !e.IsDir() && (strings.HasSuffix(e.Name(), ".png") || strings.HasSuffix(e.Name(), ".jpg")) {
				imagePaths = append(imagePaths, filepath.Join(manualDir, e.Name()))
			}
		}
	}

	if len(imagePaths) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "No screen images available. Either provide a Figma fileKey/nodeId, " +
				"or place screenshots in data/screenshots/" + req.Category + "/" + req.FeatureName + "/",
		})
		return
	}

	// Step 2: Load YANG model context for the feature
	yangContext := s.getYANGContextForFeature(req.FeatureName)

	// Step 3: Load existing API test plan context
	apiContext := s.getAPITestPlanContext(req.Category, req.FeatureName)

	// Step 4: Build the LLM prompt
	prompt := buildUIAnalysisPrompt(req.FeatureName, req.Category, yangContext, apiContext)

	// Step 5: Send to LLM with images
	log.Printf("[llm] Sending %d images + prompt to %s/%s", len(imagePaths), llmCfg.Provider, llmCfg.Model)
	startTime := time.Now()

	var llmResponse string
	switch llmCfg.Provider {
	case "anthropic":
		llmResponse, err = callAnthropicVision(llmCfg, imagePaths, prompt)
	case "openai":
		llmResponse, err = callOpenAIVision(llmCfg, imagePaths, prompt)
	default:
		err = fmt.Errorf("unsupported LLM provider: %s", llmCfg.Provider)
	}

	elapsed := time.Since(startTime)
	if err != nil {
		log.Printf("[llm] LLM call failed after %v: %v", elapsed, err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "LLM analysis failed: " + err.Error()})
		return
	}
	log.Printf("[llm] LLM responded in %v (%d chars)", elapsed, len(llmResponse))

	// Step 6: Extract JSON from LLM response
	patternJSON := extractJSONFromResponse(llmResponse)
	if patternJSON == "" {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":       "LLM did not return valid JSON",
			"rawResponse": llmResponse,
		})
		return
	}

	// Step 7: Validate the JSON structure
	var patternData map[string]interface{}
	if err := json.Unmarshal([]byte(patternJSON), &patternData); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":       "LLM returned invalid JSON: " + err.Error(),
			"rawResponse": llmResponse,
		})
		return
	}

	// Step 8: Save to config/ui-patterns/<category>.json
	patternDir := filepath.Join("config", "ui-patterns")
	os.MkdirAll(patternDir, 0755)
	patternPath := filepath.Join(patternDir, req.Category+".json")

	// Merge with existing pattern file if it exists
	existingData := make(map[string]interface{})
	if existingBytes, err := os.ReadFile(patternPath); err == nil {
		json.Unmarshal(existingBytes, &existingData)
	}

	// Merge the new feature's data into existing
	mergedJSON := mergeUIPatternData(existingData, patternData, req.FeatureName)

	prettyJSON, _ := json.MarshalIndent(mergedJSON, "", "  ")
	if err := os.WriteFile(patternPath, prettyJSON, 0644); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save pattern file: " + err.Error()})
		return
	}

	log.Printf("[llm] UI pattern saved to %s for feature %s", patternPath, req.FeatureName)

	c.JSON(http.StatusOK, gin.H{
		"status":      "success",
		"provider":    llmCfg.Provider,
		"model":       llmCfg.Model,
		"elapsed":     elapsed.String(),
		"imagesUsed":  len(imagePaths),
		"patternFile": patternPath,
		"featureName": req.FeatureName,
		"category":    req.Category,
		"pattern":     mergedJSON,
	})
}

// handleLLMStatus returns whether LLM is configured and which provider.
func (s *Server) handleLLMStatus(c *gin.Context) {
	cfg, err := loadLLMConfig()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"configured": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"configured": true,
		"provider":   cfg.Provider,
		"model":      cfg.Model,
	})
}

// ---------- Figma Image Export Helper ----------

func (s *Server) exportFigmaImages(fcfg *figmaConfig, fileKey, nodeID string) []string {
	var nodeIDs []string

	if nodeID != "" {
		encodedNode := strings.ReplaceAll(nodeID, "-", ":")
		nodesPath := fmt.Sprintf("/files/%s/nodes?ids=%s&depth=2", fileKey, url.QueryEscape(encodedNode))
		body, status, err := figmaRequest(fcfg.Token, nodesPath)
		if err != nil || status != 200 {
			log.Printf("[llm] failed to fetch Figma nodes: %v (status %d)", err, status)
			return nil
		}

		var nodesResp figmaNodesResponse
		json.Unmarshal(body, &nodesResp)

		for _, entry := range nodesResp.Nodes {
			for _, child := range entry.Document.Children {
				if child.Type == "FRAME" || child.Type == "COMPONENT" {
					nodeIDs = append(nodeIDs, child.ID)
				}
			}
			if entry.Document.Type == "FRAME" {
				nodeIDs = append(nodeIDs, entry.Document.ID)
			}
		}
	}

	if len(nodeIDs) == 0 {
		return nil
	}
	if len(nodeIDs) > 20 {
		nodeIDs = nodeIDs[:20]
	}

	idsParam := strings.Join(nodeIDs, ",")
	imgPath := fmt.Sprintf("/images/%s?ids=%s&format=png&scale=2", fileKey, url.QueryEscape(idsParam))
	body, status, err := figmaRequest(fcfg.Token, imgPath)
	if err != nil || status != 200 {
		log.Printf("[llm] Figma image export failed: %v (status %d)", err, status)
		return nil
	}

	var imgResp struct {
		Images map[string]string `json:"images"`
	}
	json.Unmarshal(body, &imgResp)

	imgDir := filepath.Join("data", "figma-screens", fileKey)
	os.MkdirAll(imgDir, 0755)

	var paths []string
	for id, imgURL := range imgResp.Images {
		if imgURL == "" {
			continue
		}
		safeName := strings.ReplaceAll(id, ":", "-")
		localPath := filepath.Join(imgDir, safeName+".png")
		if err := downloadFile(imgURL, localPath); err != nil {
			log.Printf("[llm] failed to download image %s: %v", id, err)
			continue
		}
		paths = append(paths, localPath)
	}

	log.Printf("[llm] exported %d Figma screen images to %s", len(paths), imgDir)
	return paths
}

// ---------- YANG Context Helper ----------

func (s *Server) getYANGContextForFeature(featureName string) string {
	// Try to find and parse the YANG model for this feature
	yangDirs := []string{
		filepath.Join("sources", "PlatformCommonModels", "ConfigState", "etc", "yang", "intent"),
		filepath.Join("sources", "PlatformCommonModels", "ConfigState", "etc", "yang", "asset"),
	}

	slug := strings.ReplaceAll(featureName, " ", "-")
	slug = strings.ToLower(slug)

	for _, dir := range yangDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if strings.Contains(e.Name(), slug) && strings.HasSuffix(e.Name(), ".yang") {
				data, err := os.ReadFile(filepath.Join(dir, e.Name()))
				if err != nil {
					continue
				}
				// Truncate very large YANG files
				content := string(data)
				if len(content) > 8000 {
					content = content[:8000] + "\n... (truncated)"
				}
				return fmt.Sprintf("YANG model (%s):\n%s", e.Name(), content)
			}
		}
	}
	return ""
}

// ---------- API Test Plan Context Helper ----------

func (s *Server) getAPITestPlanContext(category, featureName string) string {
	yamlPath := filepath.Join("Testplans", category, featureName+".yaml")
	data, err := os.ReadFile(yamlPath)
	if err != nil {
		return ""
	}

	// Return a summary, not the full content
	content := string(data)
	if len(content) > 5000 {
		content = content[:5000] + "\n... (truncated)"
	}
	return fmt.Sprintf("Existing API test plan (first 5000 chars):\n%s", content)
}

// ---------- LLM Prompt Builder ----------

func buildUIAnalysisPrompt(featureName, category, yangContext, apiContext string) string {
	var sb strings.Builder

	sb.WriteString(`You are a UI test automation expert analyzing screenshots of a network management web application (ExtremeCloud IQ / Extreme Platform ONE).

Your task: Analyze the provided screenshots and generate a comprehensive UI pattern JSON definition that captures every UI element, interaction pattern, and workflow visible in the screens.

## Feature Information
- Feature Name: ` + featureName + `
- Category: ` + category + `

## Output Format
Return a JSON object with this exact structure:
` + "```json" + `
{
  "patternName": "` + category + `",
  "module": "Extreme Platform ONE",
  "navigation": "Configure > ` + category + `",
  "commonUI": {
    "listPage": {
      "searchBar": true/false,
      "addButtonLabel": "Add <Feature Name>",
      "refreshButton": true/false,
      "exportButton": true/false,
      "columnsSidebar": {
        "toggle": "icon button label",
        "searchBar": true/false,
        "checkboxPerColumn": true/false,
        "dragToReorder": true/false,
        "allColumnsCheckedByDefault": true/false
      },
      "filtersSidebar": {
        "toggle": "icon button label",
        "searchBar": true/false,
        "expandablePerColumn": true/false,
        "filterInputType": "text|dropdown|checkbox",
        "buttons": ["Apply", "Reset"]
      },
      "tableCheckboxes": true/false,
      "emptyStateMessage": "No records found",
      "pagination": { "enabled": true/false, "pageSizes": [10, 25, 50, 100] }
    },
    "addModal": {
      "title": "Add <Feature Name>",
      "tabs": ["Tab Name 1", "Tab Name 2"],
      "buttons": ["Save", "Save & Add Another", "Cancel"],
      "closeXButton": true/false
    },
    "toastMessages": {
      "create": "message text",
      "update": "message text",
      "delete": "message text",
      "refresh": "message text",
      "download": "message text",
      "subtitle": "subtitle pattern if any"
    }
  },
  "features": {
    "` + featureName + `": {
      "displayName": "Human Readable Name",
      "listPage": {
        "tableColumns": ["Column1", "Column2", ...],
        "viewUsageLink": {
          "label": "View Usage",
          "modalTitle": "...",
          "searchBar": true/false,
          "tableColumns": ["Col1", "Col2"],
          "emptyState": "No records found"
        },
        "rowInlineToggle": {
          "column": "Column Name",
          "defaultState": "OFF",
          "description": "tooltip text"
        },
        "rowKebabMenu": ["Edit", "Delete", ...],
        "toolbarMoreMenu": {
          "requiresSelection": true/false,
          "actions": ["Action1", "Action2"]
        },
        "bulkActions": {
          "selectAll": true/false,
          "actions": ["Delete"]
        },
        "pagination": { "enabled": true/false }
      },
      "addModal": {
        "title": "Add Feature Name",
        "tabs": ["Tab1", "Tab2"],
        "fields": [
          {
            "name": "field-api-name",
            "label": "UI Label",
            "widget": "text-field|dropdown|toggle|ip-address|number|textarea",
            "required": true/false,
            "placeholder": "placeholder text",
            "defaultValue": "default if any",
            "validation": "validation rules if visible",
            "toggleDependent": "parent-toggle-name if field is dependent on a toggle",
            "advanced": true/false
          }
        ],
        "toggleGroups": [
          {
            "name": "toggle-group-name",
            "label": "Toggle Label",
            "defaultState": "ON|OFF",
            "dependentFields": ["field1", "field2"]
          }
        ],
        "advancedToggle": {
          "label": "Advanced",
          "defaultState": "OFF",
          "fields": ["field1", "field2"]
        },
        "pageToggles": ["Toggle Name 1"],
        "siblingFeatures": []
      },
      "toastMessages": {
        "create": "override if different from common",
        "update": "override if different",
        "delete": "override if different"
      }
    }
  }
}
` + "```" + `

## CRITICAL INSTRUCTIONS:
1. Use EXACT labels, button text, column headers, placeholder text, and field names as shown in the screenshots
2. Identify ALL toggle switches and their dependent fields (fields that get disabled when toggle is OFF)
3. Note any "Advanced" sections that reveal additional fields
4. Capture the exact toast message text if visible
5. Identify kebab menus (⋮) and their menu items
6. Note search bars, filters, column visibility controls
7. List all table columns in the exact order shown
8. Identify required fields (usually marked with *)
9. Capture tab names in modals/forms
10. Note any "View Usage" or similar links and their modal content

`)

	if yangContext != "" {
		sb.WriteString("\n## YANG Data Model Reference\nUse this to map API field names to UI labels:\n")
		sb.WriteString(yangContext)
		sb.WriteString("\n\n")
	}

	if apiContext != "" {
		sb.WriteString("\n## Existing API Test Plan Reference\nUse field names from here for the 'name' property in fields:\n")
		sb.WriteString(apiContext)
		sb.WriteString("\n\n")
	}

	sb.WriteString(`
Return ONLY the JSON object, no markdown formatting, no explanation. The JSON must be valid and parseable.
`)

	return sb.String()
}

// ---------- Anthropic Claude Vision API ----------

func callAnthropicVision(cfg *llmConfig, imagePaths []string, prompt string) (string, error) {
	// Build content array with images and text
	var content []map[string]interface{}

	for _, imgPath := range imagePaths {
		imgData, err := os.ReadFile(imgPath)
		if err != nil {
			log.Printf("[llm] skipping image %s: %v", imgPath, err)
			continue
		}

		mediaType := "image/png"
		if strings.HasSuffix(imgPath, ".jpg") || strings.HasSuffix(imgPath, ".jpeg") {
			mediaType = "image/jpeg"
		}

		content = append(content, map[string]interface{}{
			"type": "image",
			"source": map[string]interface{}{
				"type":       "base64",
				"media_type": mediaType,
				"data":       base64.StdEncoding.EncodeToString(imgData),
			},
		})
	}

	content = append(content, map[string]interface{}{
		"type": "text",
		"text": prompt,
	})

	body := map[string]interface{}{
		"model":      cfg.Model,
		"max_tokens": 8192,
		"messages": []map[string]interface{}{
			{
				"role":    "user",
				"content": content,
			},
		},
	}

	bodyJSON, _ := json.Marshal(body)
	req, err := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", bytes.NewReader(bodyJSON))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", cfg.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	client := &http.Client{Timeout: 300 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("Anthropic API request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Anthropic API returned %d: %s", resp.StatusCode, truncate(string(respBody), 500))
	}

	var result struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("failed to parse Anthropic response: %w", err)
	}

	for _, c := range result.Content {
		if c.Type == "text" {
			return c.Text, nil
		}
	}
	return "", fmt.Errorf("no text content in Anthropic response")
}

// ---------- OpenAI GPT-4 Vision API ----------

func callOpenAIVision(cfg *llmConfig, imagePaths []string, prompt string) (string, error) {
	var content []map[string]interface{}

	for _, imgPath := range imagePaths {
		imgData, err := os.ReadFile(imgPath)
		if err != nil {
			continue
		}

		mediaType := "image/png"
		if strings.HasSuffix(imgPath, ".jpg") || strings.HasSuffix(imgPath, ".jpeg") {
			mediaType = "image/jpeg"
		}

		dataURI := fmt.Sprintf("data:%s;base64,%s", mediaType, base64.StdEncoding.EncodeToString(imgData))
		content = append(content, map[string]interface{}{
			"type": "image_url",
			"image_url": map[string]interface{}{
				"url":    dataURI,
				"detail": "high",
			},
		})
	}

	content = append(content, map[string]interface{}{
		"type": "text",
		"text": prompt,
	})

	body := map[string]interface{}{
		"model":      cfg.Model,
		"max_tokens": 8192,
		"messages": []map[string]interface{}{
			{
				"role":    "user",
				"content": content,
			},
		},
	}

	bodyJSON, _ := json.Marshal(body)
	req, err := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", bytes.NewReader(bodyJSON))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)

	client := &http.Client{Timeout: 300 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("OpenAI API request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("OpenAI API returned %d: %s", resp.StatusCode, truncate(string(respBody), 500))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("failed to parse OpenAI response: %w", err)
	}

	if len(result.Choices) > 0 {
		return result.Choices[0].Message.Content, nil
	}
	return "", fmt.Errorf("no choices in OpenAI response")
}

// ---------- JSON Extraction from LLM Response ----------

func extractJSONFromResponse(response string) string {
	// Try to find JSON in code blocks first
	if idx := strings.Index(response, "```json"); idx >= 0 {
		start := idx + 7
		if end := strings.Index(response[start:], "```"); end >= 0 {
			return strings.TrimSpace(response[start : start+end])
		}
	}
	if idx := strings.Index(response, "```"); idx >= 0 {
		start := idx + 3
		// Skip language identifier if present
		if nl := strings.Index(response[start:], "\n"); nl >= 0 {
			start += nl + 1
		}
		if end := strings.Index(response[start:], "```"); end >= 0 {
			candidate := strings.TrimSpace(response[start : start+end])
			if json.Valid([]byte(candidate)) {
				return candidate
			}
		}
	}

	// Try to find raw JSON (starts with { and ends with })
	start := strings.Index(response, "{")
	if start >= 0 {
		// Find the matching closing brace
		depth := 0
		for i := start; i < len(response); i++ {
			switch response[i] {
			case '{':
				depth++
			case '}':
				depth--
				if depth == 0 {
					candidate := response[start : i+1]
					if json.Valid([]byte(candidate)) {
						return candidate
					}
				}
			}
		}
	}

	return ""
}

// ---------- UI Pattern Merge Helper ----------

func mergeUIPatternData(existing, newData map[string]interface{}, featureName string) map[string]interface{} {
	if len(existing) == 0 {
		return newData
	}

	// If new data has a "features" map, merge just the feature into existing
	if newFeatures, ok := newData["features"].(map[string]interface{}); ok {
		existFeatures, _ := existing["features"].(map[string]interface{})
		if existFeatures == nil {
			existFeatures = make(map[string]interface{})
		}

		for k, v := range newFeatures {
			existFeatures[k] = v
		}
		existing["features"] = existFeatures
	}

	// Update commonUI if new data has it and existing doesn't have comprehensive data
	if newCommon, ok := newData["commonUI"]; ok {
		if _, hasCommon := existing["commonUI"]; !hasCommon {
			existing["commonUI"] = newCommon
		}
	}

	return existing
}
