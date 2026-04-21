package yang

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/extremenetworks/testcase-generator/pkg/model"
)

// Parser parses YANG models and extracts features
type Parser struct {
	yangDir  string
	features map[string]*model.Feature
	typedefs map[string]*Typedef // Registry of all typedef definitions
}

// NewParser creates a new YANG parser
func NewParser(yangDir string) *Parser {
	return &Parser{
		yangDir:  yangDir,
		features: make(map[string]*model.Feature),
		typedefs: make(map[string]*Typedef),
	}
}

// Parse loads and parses all YANG files in the directory (recursively)
func (p *Parser) Parse() error {
	var yangFiles []string

	// Walk directory recursively to find all .yang files
	err := filepath.Walk(p.yangDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(path, ".yang") {
			yangFiles = append(yangFiles, path)
		}
		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to walk YANG directory: %w", err)
	}

	if len(yangFiles) == 0 {
		return fmt.Errorf("no YANG files found in %s", p.yangDir)
	}

	for _, yangFile := range yangFiles {
		if err := p.parseFile(yangFile); err != nil {
			// Log error but continue with other files
			fmt.Printf("Warning: failed to parse %s: %v\n", yangFile, err)
		}
	}

	return nil
}

// parseFile parses a single YANG file
func (p *Parser) parseFile(filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var currentModule string
	var currentFeature *model.Feature
	var inFeature bool
	var braceDepth int
	var featureDepth int

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Skip comments and empty lines
		if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "#") || trimmed == "" {
			continue
		}

		// Track brace depth
		braceDepth += strings.Count(line, "{")
		braceDepth -= strings.Count(line, "}")

		// Parse module
		if strings.HasPrefix(trimmed, "module ") {
			currentModule = extractModuleName(trimmed)
			continue
		}

		// Parse typedef (collect type definitions for later resolution)
		if strings.HasPrefix(trimmed, "typedef ") && !inFeature {
			typedef := p.parseTypedef(scanner, trimmed, currentModule)
			if typedef != nil {
				// Register with both short name and module-prefixed name
				p.typedefs[typedef.Name] = typedef
				if currentModule != "" {
					p.typedefs[currentModule+":"+typedef.Name] = typedef
				}
			}
			continue
		}

		// Parse container or list (both represent features)
		if (strings.HasPrefix(trimmed, "container ") || strings.HasPrefix(trimmed, "list ")) && !inFeature {
			var featureName string
			isListType := false

			if strings.HasPrefix(trimmed, "container ") {
				featureName = extractContainerName(trimmed)
			} else {
				featureName = extractListName(trimmed)
				isListType = true
			}

			inFeature = true
			featureDepth = braceDepth

			currentFeature = &model.Feature{
				Name:       featureName,
				YangPath:   fmt.Sprintf("/%s/%s", currentModule, featureName),
				Parameters: []model.Parameter{},
				IsListType: isListType,
			}
			continue
		}

		// Parse leaf (parameter)
		if inFeature && strings.HasPrefix(trimmed, "leaf ") {
			param := p.parseLeaf(scanner, trimmed)
			if currentFeature != nil {
				currentFeature.Parameters = append(currentFeature.Parameters, param.ToModelParameter())
			}
			continue
		}

		// Parse leaf-list (array parameter)
		if inFeature && strings.HasPrefix(trimmed, "leaf-list ") {
			param := p.parseLeafList(scanner, trimmed)
			if currentFeature != nil {
				currentFeature.Parameters = append(currentFeature.Parameters, param.ToModelParameter())
			}
			continue
		}

		// Parse nested container (nested parameter group)
		if inFeature && strings.HasPrefix(trimmed, "container ") {
			param := p.parseNestedContainer(scanner, trimmed)
			if currentFeature != nil {
				currentFeature.Parameters = append(currentFeature.Parameters, param.ToModelParameter())
			}
			continue
		}

		// Parse nested list (array of nested parameter groups)
		if inFeature && strings.HasPrefix(trimmed, "list ") {
			param := p.parseNestedList(scanner, trimmed)
			if currentFeature != nil {
				currentFeature.Parameters = append(currentFeature.Parameters, param.ToModelParameter())
			}
			continue
		}

		// End of feature (container/list)
		if inFeature && braceDepth < featureDepth {
			if currentFeature != nil && len(currentFeature.Parameters) > 0 {
				p.features[currentFeature.Name] = currentFeature
				currentFeature = nil
			}
			inFeature = false
		}
	}

	// Handle case where file ends while in a feature
	if currentFeature != nil && len(currentFeature.Parameters) > 0 {
		p.features[currentFeature.Name] = currentFeature
	}

	return scanner.Err()
}

// parseLeaf parses a leaf statement and returns a Parameter
func (p *Parser) parseLeaf(scanner *bufio.Scanner, leafLine string) Parameter {
	param := Parameter{
		Name:        extractLeafName(leafLine),
		Constraints: []model.Constraint{},
	}

	// Continue reading until we find the closing brace
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if line == "}" {
			break
		}

		// Parse type
		if strings.HasPrefix(line, "type ") {
			param.YangType = extractType(line)

			// Check if this is a typedef reference (contains : or ends with -enum)
			if typedef := p.resolveTypedef(param.YangType); typedef != nil {
				// Use the base type for GoType conversion
				param.GoType = yangTypeToGoType(typedef.BaseType)
				// Add enum constraints from typedef
				if len(typedef.EnumValues) > 0 {
					param.Constraints = append(param.Constraints, model.Constraint{
						Type:  model.ConstraintTypeEnum,
						Value: typedef.EnumValues,
					})
				}
				// Add any other constraints from typedef
				param.Constraints = append(param.Constraints, typedef.Constraints...)
			} else {
				param.GoType = yangTypeToGoType(param.YangType)
			}

			// Parse inline type constraints (e.g., type string { length "1..255"; })
			if strings.Contains(line, "{") {
				p.parseInlineTypeConstraints(scanner, &param)
			}
		}

		// Parse description
		if strings.HasPrefix(line, "description ") {
			param.Description = extractQuotedString(line)
		}

		// Parse mandatory
		if strings.Contains(line, "mandatory true") {
			param.Required = true
			param.Constraints = append(param.Constraints, model.Constraint{
				Type:  model.ConstraintTypeRequired,
				Value: true,
			})
		}

		// Parse default
		if strings.HasPrefix(line, "default ") {
			param.DefaultValue = extractQuotedString(line)
		}

		// Parse pattern constraint
		if strings.HasPrefix(line, "pattern ") {
			param.Constraints = append(param.Constraints, model.Constraint{
				Type:  model.ConstraintTypePattern,
				Value: extractQuotedString(line),
			})
		}

		// Parse length constraint
		if strings.HasPrefix(line, "length ") {
			lengthStr := extractQuotedString(line)
			min, max := parseRange(lengthStr)
			if min > 0 {
				param.Constraints = append(param.Constraints, model.Constraint{
					Type:  model.ConstraintTypeMinLength,
					Value: min,
				})
			}
			if max > 0 {
				param.Constraints = append(param.Constraints, model.Constraint{
					Type:  model.ConstraintTypeMaxLength,
					Value: max,
				})
			}
		}

		// Parse range constraint
		if strings.HasPrefix(line, "range ") {
			rangeStr := extractQuotedString(line)
			min, max := parseRange(rangeStr)
			if min > 0 {
				param.Constraints = append(param.Constraints, model.Constraint{
					Type:  model.ConstraintTypeMin,
					Value: min,
				})
			}
			if max > 0 {
				param.Constraints = append(param.Constraints, model.Constraint{
					Type:  model.ConstraintTypeMax,
					Value: max,
				})
			}
		}

		// Parse enum values
		if strings.HasPrefix(line, "enum ") {
			enumValue := extractQuotedString(line)
			if enumValue == "" {
				// Handle enum without quotes
				re := regexp.MustCompile(`enum\\s+(\\S+)`)
				matches := re.FindStringSubmatch(line)
				if len(matches) > 1 {
					enumValue = strings.TrimSuffix(matches[1], ";")
				}
			}
			if enumValue != "" {
				// Collect enum values
				var enumValues []string
				if len(param.Constraints) > 0 && param.Constraints[len(param.Constraints)-1].Type == model.ConstraintTypeEnum {
					enumValues = param.Constraints[len(param.Constraints)-1].Value.([]string)
					param.Constraints = param.Constraints[:len(param.Constraints)-1]
				}
				enumValues = append(enumValues, enumValue)
				param.Constraints = append(param.Constraints, model.Constraint{
					Type:  model.ConstraintTypeEnum,
					Value: enumValues,
				})
			}
		}
	}

	return param
}

// parseLeafList parses a leaf-list statement
func (p *Parser) parseLeafList(scanner *bufio.Scanner, leafListLine string) Parameter {
	param := Parameter{
		Name:        extractLeafName(leafListLine),
		IsArray:     true,
		Constraints: []model.Constraint{},
	}

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if line == "}" {
			break
		}

		if strings.HasPrefix(line, "type ") {
			param.YangType = extractType(line)
			param.GoType = "[]" + yangTypeToGoType(param.YangType)
		}

		if strings.HasPrefix(line, "min-elements ") {
			minItems := extractNumber(line)
			param.ArrayMinItems = minItems
			param.Constraints = append(param.Constraints, model.Constraint{
				Type:  model.ConstraintTypeMinItems,
				Value: minItems,
			})
		}

		if strings.HasPrefix(line, "max-elements ") {
			maxItems := extractNumber(line)
			param.ArrayMaxItems = maxItems
			param.Constraints = append(param.Constraints, model.Constraint{
				Type:  model.ConstraintTypeMaxItems,
				Value: maxItems,
			})
		}
	}

	return param
}

// parseNestedContainer parses a nested container within a feature
func (p *Parser) parseNestedContainer(scanner *bufio.Scanner, containerLine string) Parameter {
	param := Parameter{
		Name:             extractContainerName(containerLine),
		YangType:         "container",
		GoType:           "map[string]interface{}",
		Constraints:      []model.Constraint{},
		NestedProperties: []Parameter{},
	}

	nestedDepth := 0
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Track brace depth
		nestedDepth += strings.Count(line, "{")
		nestedDepth -= strings.Count(line, "}")

		if nestedDepth < 0 {
			break
		}

		// Parse description
		if strings.HasPrefix(trimmed, "description ") {
			param.Description = extractQuotedString(trimmed)
		}

		// Parse nested leaf
		if strings.HasPrefix(trimmed, "leaf ") {
			nestedParam := p.parseLeaf(scanner, trimmed)
			param.NestedProperties = append(param.NestedProperties, nestedParam)
		}

		// Parse nested leaf-list
		if strings.HasPrefix(trimmed, "leaf-list ") {
			nestedParam := p.parseLeafList(scanner, trimmed)
			param.NestedProperties = append(param.NestedProperties, nestedParam)
		}

		// Parse recursively nested container
		if strings.HasPrefix(trimmed, "container ") {
			nestedParam := p.parseNestedContainer(scanner, trimmed)
			param.NestedProperties = append(param.NestedProperties, nestedParam)
		}

		// Parse recursively nested list
		if strings.HasPrefix(trimmed, "list ") {
			nestedParam := p.parseNestedList(scanner, trimmed)
			param.NestedProperties = append(param.NestedProperties, nestedParam)
		}
	}

	return param
}

// parseNestedList parses a nested list within a feature
func (p *Parser) parseNestedList(scanner *bufio.Scanner, listLine string) Parameter {
	param := Parameter{
		Name:             extractListName(listLine),
		YangType:         "list",
		GoType:           "[]map[string]interface{}",
		IsArray:          true,
		Constraints:      []model.Constraint{},
		NestedProperties: []Parameter{},
	}

	nestedDepth := 0
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Track brace depth
		nestedDepth += strings.Count(line, "{")
		nestedDepth -= strings.Count(line, "}")

		if nestedDepth < 0 {
			break
		}

		// Parse description
		if strings.HasPrefix(trimmed, "description ") {
			param.Description = extractQuotedString(trimmed)
		}

		// Parse min-elements
		if strings.HasPrefix(trimmed, "min-elements ") {
			minItems := extractNumber(trimmed)
			param.ArrayMinItems = minItems
			param.Constraints = append(param.Constraints, model.Constraint{
				Type:  model.ConstraintTypeMinItems,
				Value: minItems,
			})
		}

		// Parse max-elements
		if strings.HasPrefix(trimmed, "max-elements ") {
			maxItems := extractNumber(trimmed)
			param.ArrayMaxItems = maxItems
			param.Constraints = append(param.Constraints, model.Constraint{
				Type:  model.ConstraintTypeMaxItems,
				Value: maxItems,
			})
		}

		// Parse nested leaf
		if strings.HasPrefix(trimmed, "leaf ") {
			nestedParam := p.parseLeaf(scanner, trimmed)
			param.NestedProperties = append(param.NestedProperties, nestedParam)
		}

		// Parse nested leaf-list
		if strings.HasPrefix(trimmed, "leaf-list ") {
			nestedParam := p.parseLeafList(scanner, trimmed)
			param.NestedProperties = append(param.NestedProperties, nestedParam)
		}

		// Parse recursively nested container
		if strings.HasPrefix(trimmed, "container ") {
			nestedParam := p.parseNestedContainer(scanner, trimmed)
			param.NestedProperties = append(param.NestedProperties, nestedParam)
		}

		// Parse recursively nested list
		if strings.HasPrefix(trimmed, "list ") {
			nestedParam := p.parseNestedList(scanner, trimmed)
			param.NestedProperties = append(param.NestedProperties, nestedParam)
		}
	}

	return param
}

// parseTypedef parses a typedef statement and returns a Typedef
func (p *Parser) parseTypedef(scanner *bufio.Scanner, typedefLine string, currentModule string) *Typedef {
	typedefName := extractTypedefName(typedefLine)
	if typedefName == "" {
		return nil
	}

	typedef := &Typedef{
		Name:        typedefName,
		EnumValues:  []string{},
		Constraints: []model.Constraint{},
	}

	nestedDepth := 0
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Track brace depth
		nestedDepth += strings.Count(line, "{")
		nestedDepth -= strings.Count(line, "}")

		if nestedDepth < 0 {
			break
		}

		// Parse base type
		if strings.HasPrefix(trimmed, "type ") {
			typedef.BaseType = extractType(trimmed)
		}

		// Parse description
		if strings.HasPrefix(trimmed, "description ") {
			typedef.Description = extractQuotedString(trimmed)
		}

		// Parse enum values
		if strings.HasPrefix(trimmed, "enum ") {
			enumValue := extractQuotedString(trimmed)
			if enumValue == "" {
				// Handle enum without quotes
				re := regexp.MustCompile(`enum\\s+(\\S+)`)
				matches := re.FindStringSubmatch(trimmed)
				if len(matches) > 1 {
					enumValue = strings.TrimSuffix(matches[1], ";")
				}
			}
			if enumValue != "" {
				typedef.EnumValues = append(typedef.EnumValues, enumValue)
			}
		}

		// Parse range constraint
		if strings.HasPrefix(trimmed, "range ") {
			rangeStr := extractQuotedString(trimmed)
			min, max := parseRange(rangeStr)
			if min > 0 {
				typedef.Constraints = append(typedef.Constraints, model.Constraint{
					Type:  model.ConstraintTypeMin,
					Value: min,
				})
			}
			if max > 0 {
				typedef.Constraints = append(typedef.Constraints, model.Constraint{
					Type:  model.ConstraintTypeMax,
					Value: max,
				})
			}
		}

		// Parse length constraint
		if strings.HasPrefix(trimmed, "length ") {
			lengthStr := extractQuotedString(trimmed)
			min, max := parseRange(lengthStr)
			if min > 0 {
				typedef.Constraints = append(typedef.Constraints, model.Constraint{
					Type:  model.ConstraintTypeMinLength,
					Value: min,
				})
			}
			if max > 0 {
				typedef.Constraints = append(typedef.Constraints, model.Constraint{
					Type:  model.ConstraintTypeMaxLength,
					Value: max,
				})
			}
		}

		// Parse pattern constraint
		if strings.HasPrefix(trimmed, "pattern ") {
			typedef.Constraints = append(typedef.Constraints, model.Constraint{
				Type:  model.ConstraintTypePattern,
				Value: extractQuotedString(trimmed),
			})
		}
	}

	return typedef
}

// resolveTypedef resolves a type reference to a typedef definition
func (p *Parser) resolveTypedef(typeName string) *Typedef {
	// Try direct lookup first
	if typedef, ok := p.typedefs[typeName]; ok {
		return typedef
	}

	// Try with common module prefixes if it contains a colon
	if strings.Contains(typeName, ":") {
		// Already has module prefix, try as-is
		if typedef, ok := p.typedefs[typeName]; ok {
			return typedef
		}
		// Try just the type name without prefix
		parts := strings.Split(typeName, ":")
		if len(parts) == 2 {
			if typedef, ok := p.typedefs[parts[1]]; ok {
				return typedef
			}
		}
	}

	// Try common naming patterns for enums
	if strings.HasSuffix(typeName, "-enum") {
		// Try various module prefixes
		possibleNames := []string{
			typeName,
			"intent-types:" + typeName,
			"common-types:" + typeName,
			"asset-types:" + typeName,
		}
		for _, name := range possibleNames {
			if typedef, ok := p.typedefs[name]; ok {
				return typedef
			}
		}
	}

	return nil
}

// parseInlineTypeConstraints parses constraints within type definition
func (p *Parser) parseInlineTypeConstraints(scanner *bufio.Scanner, param *Parameter) {
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if line == "}" || strings.HasSuffix(line, "};") {
			break
		}

		// Parse length
		if strings.HasPrefix(line, "length ") {
			lengthStr := extractQuotedString(line)
			min, max := parseRange(lengthStr)
			if min > 0 {
				param.Constraints = append(param.Constraints, model.Constraint{
					Type:  model.ConstraintTypeMinLength,
					Value: min,
				})
			}
			if max > 0 {
				param.Constraints = append(param.Constraints, model.Constraint{
					Type:  model.ConstraintTypeMaxLength,
					Value: max,
				})
			}
		}

		// Parse range
		if strings.HasPrefix(line, "range ") {
			rangeStr := extractQuotedString(line)
			min, max := parseRange(rangeStr)
			if min > 0 {
				param.Constraints = append(param.Constraints, model.Constraint{
					Type:  model.ConstraintTypeMin,
					Value: min,
				})
			}
			if max > 0 {
				param.Constraints = append(param.Constraints, model.Constraint{
					Type:  model.ConstraintTypeMax,
					Value: max,
				})
			}
		}

		// Parse pattern
		if strings.HasPrefix(line, "pattern ") {
			param.Constraints = append(param.Constraints, model.Constraint{
				Type:  model.ConstraintTypePattern,
				Value: extractQuotedString(line),
			})
		}
	}
}

// GetFeatures returns all parsed features
func (p *Parser) GetFeatures() map[string]*model.Feature {
	return p.features
}

// Helper functions for parsing YANG syntax

func extractModuleName(line string) string {
	re := regexp.MustCompile(`module\s+(\S+)`)
	matches := re.FindStringSubmatch(line)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}

func extractContainerName(line string) string {
	re := regexp.MustCompile(`container\s+(\S+)`)
	matches := re.FindStringSubmatch(line)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}

func extractListName(line string) string {
	re := regexp.MustCompile(`list\s+(\S+)`)
	matches := re.FindStringSubmatch(line)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}

func extractTypedefName(line string) string {
	re := regexp.MustCompile(`typedef\s+(\S+)`)
	matches := re.FindStringSubmatch(line)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}

func extractLeafName(line string) string {
	re := regexp.MustCompile(`leaf(?:-list)?\s+(\S+)`)
	matches := re.FindStringSubmatch(line)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}

func extractType(line string) string {
	re := regexp.MustCompile(`type\s+(\S+)`)
	matches := re.FindStringSubmatch(line)
	if len(matches) > 1 {
		return strings.TrimSuffix(matches[1], ";")
	}
	return ""
}

func extractQuotedString(line string) string {
	re := regexp.MustCompile(`"([^"]*)"`)
	matches := re.FindStringSubmatch(line)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}

func extractNumber(line string) int {
	re := regexp.MustCompile(`\d+`)
	matches := re.FindString(line)
	var num int
	fmt.Sscanf(matches, "%d", &num)
	return num
}

// parseRange parses YANG range/length format like "1..255" or "0..max"
func parseRange(rangeStr string) (min int, max int) {
	if rangeStr == "" {
		return 0, 0
	}

	// Handle "min..max" format
	if strings.Contains(rangeStr, "..") {
		parts := strings.Split(rangeStr, "..")
		if len(parts) == 2 {
			if parts[0] != "min" {
				fmt.Sscanf(parts[0], "%d", &min)
			}
			if parts[1] != "max" {
				fmt.Sscanf(parts[1], "%d", &max)
			}
		}
	} else {
		// Single value
		fmt.Sscanf(rangeStr, "%d", &max)
		min = max
	}

	return min, max
}

func yangTypeToGoType(yangType string) string {
	typeMap := map[string]string{
		"string":      "string",
		"int8":        "int8",
		"int16":       "int16",
		"int32":       "int32",
		"int64":       "int64",
		"uint8":       "uint8",
		"uint16":      "uint16",
		"uint32":      "uint32",
		"uint64":      "uint64",
		"boolean":     "bool",
		"enumeration": "string",
		"decimal64":   "float64",
		"binary":      "[]byte",
		"bits":        "string",
		"leafref":     "string",
		"identityref": "string",
		"union":       "interface{}",
	}

	if goType, ok := typeMap[yangType]; ok {
		return goType
	}
	return "string" // default
}
