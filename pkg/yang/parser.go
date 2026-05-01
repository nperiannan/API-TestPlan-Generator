package yang

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/extremenetworks/testcase-generator/pkg/model"
)

// Parser parses YANG models and extracts features
type Parser struct {
	yangDir   string
	features  map[string]*model.Feature
	typedefs  map[string]*Typedef  // Registry of all typedef definitions
	groupings map[string]*Grouping // Registry of all grouping definitions
}

// NewParser creates a new YANG parser
func NewParser(yangDir string) *Parser {
	return &Parser{
		yangDir:   yangDir,
		features:  make(map[string]*model.Feature),
		typedefs:  make(map[string]*Typedef),
		groupings: make(map[string]*Grouping),
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

	// Read all files into memory for two-pass parsing
	fileLines := make(map[string][]string, len(yangFiles))
	for _, yangFile := range yangFiles {
		lines, err := p.readLines(yangFile)
		if err != nil {
			fmt.Printf("Warning: failed to read %s: %v\n", yangFile, err)
			continue
		}
		fileLines[yangFile] = lines
	}

	// Phase 1: collect all typedefs and groupings from every file
	for _, yangFile := range yangFiles {
		lines, ok := fileLines[yangFile]
		if !ok {
			continue
		}
		if err := p.collectTypesAndGroupings(yangFile, lines); err != nil {
			fmt.Printf("Warning: failed to collect types from %s: %v\n", yangFile, err)
		}
	}

	// Phase 2: parse features from every file, expanding `uses` via collected groupings
	for _, yangFile := range yangFiles {
		lines, ok := fileLines[yangFile]
		if !ok {
			continue
		}
		if err := p.parseFeaturesFromLines(yangFile, lines); err != nil {
			fmt.Printf("Warning: failed to parse features from %s: %v\n", yangFile, err)
		}
	}

	return nil
}

// readLines reads a YANG file and returns its lines as a string slice.
func (p *Parser) readLines(filePath string) ([]string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var lines []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	return lines, sc.Err()
}

// collectTypesAndGroupings does a first pass over a file to collect typedef and grouping definitions.
func (p *Parser) collectTypesAndGroupings(filePath string, lines []string) error {
	scanner := newSliceScanner(lines)
	var currentModule string
	braceDepth := 0

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "#") || trimmed == "" {
			continue
		}

		if strings.HasPrefix(trimmed, "extension ") {
			consumeCurrentBlock(scanner, trimmed)
			continue
		}

		if isDescriptionStatement(trimmed) {
			consumeDescriptionStatement(scanner, trimmed)
			continue
		}

		braceDepth += strings.Count(line, "{")
		braceDepth -= strings.Count(line, "}")

		if strings.HasPrefix(trimmed, "module ") {
			currentModule = extractModuleName(trimmed)
			continue
		}

		// Collect typedef at module level.
		// braceDepth is 2 here: 1 from module { + 1 from typedef {
		if strings.HasPrefix(trimmed, "typedef ") && braceDepth == 2 {
			typedef := p.parseTypedef(scanner, trimmed, currentModule)
			braceDepth-- // parseTypedef consumed the closing }
			if typedef != nil {
				p.typedefs[typedef.Name] = typedef
				if currentModule != "" {
					p.typedefs[currentModule+":"+typedef.Name] = typedef
				}
			}
			continue
		}

		// Collect grouping at module level.
		// braceDepth is 2 here: 1 from module { + 1 from grouping {
		if strings.HasPrefix(trimmed, "grouping ") && braceDepth == 2 {
			grouping := p.parseGroupingBlock(scanner, trimmed, currentModule)
			braceDepth-- // parseGroupingBlock consumed the closing }
			if grouping != nil {
				p.groupings[grouping.Name] = grouping
				if currentModule != "" {
					p.groupings[currentModule+":"+grouping.Name] = grouping
				}
			}
			continue
		}
	}

	return nil
}

// parseGroupingBlock parses a grouping { ... } block and returns a Grouping.
func (p *Parser) parseGroupingBlock(scanner lineScanner, groupingLine string, currentModule string) *Grouping {
	name := extractGroupingName(groupingLine)
	if name == "" {
		return nil
	}

	grouping := &Grouping{Name: name}
	depth := 0

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "#") || trimmed == "" {
			continue
		}

		if isDescriptionStatement(trimmed) {
			consumeDescriptionStatement(scanner, trimmed)
			continue
		}

		depth += strings.Count(line, "{")
		depth -= strings.Count(line, "}")

		if depth < 0 {
			break // end of grouping block
		}

		if strings.HasPrefix(trimmed, "leaf ") {
			// Opening { already counted; parseLeaf will consume closing } — compensate
			depth -= strings.Count(trimmed, "{")
			param := p.parseLeaf(scanner, trimmed)
			grouping.Params = append(grouping.Params, param)
		} else if strings.HasPrefix(trimmed, "leaf-list ") {
			depth -= strings.Count(trimmed, "{")
			param := p.parseLeafList(scanner, trimmed)
			grouping.Params = append(grouping.Params, param)
		} else if strings.HasPrefix(trimmed, "container ") {
			depth -= strings.Count(trimmed, "{")
			param := p.parseNestedContainer(scanner, trimmed)
			grouping.Params = append(grouping.Params, param)
		} else if strings.HasPrefix(trimmed, "list ") {
			depth -= strings.Count(trimmed, "{")
			param := p.parseNestedList(scanner, trimmed)
			grouping.Params = append(grouping.Params, param)
		} else if strings.HasPrefix(trimmed, "uses ") {
			usesName := extractUsesName(trimmed)
			if usesName != "" {
				grouping.UsesRefs = append(grouping.UsesRefs, usesName)
			}
			// If uses has its own block, consume it
			if strings.Contains(trimmed, "{") && !strings.Contains(trimmed, "}") {
				blockDepth := 1
				for scanner.Scan() {
					bl := scanner.Text()
					blockDepth += strings.Count(bl, "{")
					blockDepth -= strings.Count(bl, "}")
					depth += strings.Count(bl, "{")
					depth -= strings.Count(bl, "}")
					if blockDepth <= 0 {
						break
					}
				}
			}
		}
	}

	return grouping
}

// parseFeaturesFromLines is the second-pass parser that reads features and expands `uses`.
func (p *Parser) parseFeaturesFromLines(filePath string, lines []string) error {
	scanner := newSliceScanner(lines)
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

		if strings.HasPrefix(trimmed, "extension ") {
			consumeCurrentBlock(scanner, trimmed)
			continue
		}

		if isDescriptionStatement(trimmed) {
			consumeDescriptionStatement(scanner, trimmed)
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

		// Skip typedef in second pass (already collected)
		if strings.HasPrefix(trimmed, "typedef ") && !inFeature {
			braceDepth -= strings.Count(trimmed, "{")
			p.parseTypedef(scanner, trimmed, currentModule) // consume the block
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

		// Parse YANG list key statement (e.g., key "server vr-name";)
		if inFeature && currentFeature != nil && strings.HasPrefix(trimmed, "key ") {
			keyStr := extractQuotedString(trimmed)
			if keyStr != "" {
				currentFeature.Keys = strings.Fields(keyStr)
			}
			continue
		}

		// Parse leaf (parameter)
		if inFeature && strings.HasPrefix(trimmed, "leaf ") {
			braceDepth -= strings.Count(trimmed, "{")
			param := p.parseLeaf(scanner, trimmed)
			if currentFeature != nil {
				currentFeature.Parameters = append(currentFeature.Parameters, param.ToModelParameter())
			}
			continue
		}

		// Parse leaf-list (array parameter)
		if inFeature && strings.HasPrefix(trimmed, "leaf-list ") {
			braceDepth -= strings.Count(trimmed, "{")
			param := p.parseLeafList(scanner, trimmed)
			if currentFeature != nil {
				currentFeature.Parameters = append(currentFeature.Parameters, param.ToModelParameter())
			}
			continue
		}

		// Parse nested container (nested parameter group)
		if inFeature && strings.HasPrefix(trimmed, "container ") {
			braceDepth -= strings.Count(trimmed, "{")
			param := p.parseNestedContainer(scanner, trimmed)
			if currentFeature != nil {
				currentFeature.Parameters = append(currentFeature.Parameters, param.ToModelParameter())
			}
			continue
		}

		// Parse nested list (array of nested parameter groups)
		if inFeature && strings.HasPrefix(trimmed, "list ") {
			braceDepth -= strings.Count(trimmed, "{")
			param := p.parseNestedList(scanner, trimmed)
			if currentFeature != nil {
				currentFeature.Parameters = append(currentFeature.Parameters, param.ToModelParameter())
			}
			continue
		}

		// Expand `uses` inside a feature by resolving the referenced grouping
		if strings.HasPrefix(trimmed, "uses ") {
			usesName := extractUsesName(trimmed)

			if inFeature && usesName != "" && currentFeature != nil {
				// Inside an existing feature — expand grouping into it
				p.expandGrouping(usesName, currentFeature, make(map[string]bool))
			} else if !inFeature && usesName != "" && currentModule != "" {
				// Module-level uses (grouping-only YANG files like port sub-features):
				// Create a feature from the module name and expand the grouping into it.
				// Skip if a feature with this name already exists (e.g. from an inner list/container).
				featureName := deriveFeatureNameFromModule(currentModule)
				if _, exists := p.features[featureName]; !exists {
					feat := &model.Feature{
						Name:       featureName,
						YangPath:   fmt.Sprintf("/%s", currentModule),
						Parameters: []model.Parameter{},
					}
					p.expandGrouping(usesName, feat, make(map[string]bool))
					if len(feat.Parameters) > 0 {
						p.features[feat.Name] = feat
					}
				}
			}

			// If uses has its own block, consume it (conditions/augmentations)
			if strings.Contains(trimmed, "{") && !strings.Contains(trimmed, "}") {
				blockDepth := 1
				for scanner.Scan() {
					bl := scanner.Text()
					blockDepth += strings.Count(bl, "{")
					blockDepth -= strings.Count(bl, "}")
					braceDepth += strings.Count(bl, "{")
					braceDepth -= strings.Count(bl, "}")
					if blockDepth <= 0 {
						break
					}
				}
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

	return nil
}

// expandGrouping recursively expands a grouping's parameters into a feature.
func (p *Parser) expandGrouping(name string, feature *model.Feature, visited map[string]bool) {
	key := normalizeGroupingName(name)
	if visited[key] {
		return // cycle detection
	}
	visited[key] = true

	grouping := p.lookupGrouping(name)
	if grouping == nil {
		return
	}

	// Add direct parameters
	for _, param := range grouping.Params {
		feature.Parameters = append(feature.Parameters, param.ToModelParameter())
	}

	// Recursively expand nested uses
	for _, ref := range grouping.UsesRefs {
		p.expandGrouping(ref, feature, visited)
	}
}

// lookupGrouping finds a grouping by name, trying prefixed and unprefixed forms.
func (p *Parser) lookupGrouping(name string) *Grouping {
	if g, ok := p.groupings[name]; ok {
		return g
	}
	// Strip module prefix (e.g. "port-poe:grp-port-poe" → "grp-port-poe")
	if idx := strings.Index(name, ":"); idx >= 0 {
		short := name[idx+1:]
		if g, ok := p.groupings[short]; ok {
			return g
		}
	}
	return nil
}

// normalizeGroupingName strips prefix for cycle-detection keys.
func normalizeGroupingName(name string) string {
	if idx := strings.Index(name, ":"); idx >= 0 {
		return name[idx+1:]
	}
	return name
}

// parseLeaf parses a leaf statement and returns a Parameter
func (p *Parser) parseLeaf(scanner lineScanner, leafLine string) Parameter {
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

		// Parse description (inline: description "text"; or multi-line: description\n  "text")
		if strings.HasPrefix(line, "description ") {
			param.Description = extractQuotedString(line)
		} else if line == "description" {
			param.Description = parseMultiLineDescription(scanner)
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
			param.DefaultValue = parseDefaultValue(line, param.GoType)
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
			min, max, hasMin, hasMax := parseRangeWithPresence(lengthStr)
			param.Constraints = appendBounds(param.Constraints, min, max, hasMin, hasMax, model.ConstraintTypeMinLength, model.ConstraintTypeMaxLength)
		}

		// Parse range constraint
		if strings.HasPrefix(line, "range ") {
			rangeStr := extractQuotedString(line)
			min, max, hasMin, hasMax := parseRangeWithPresence(rangeStr)
			param.Constraints = appendBounds(param.Constraints, min, max, hasMin, hasMax, model.ConstraintTypeMin, model.ConstraintTypeMax)
		}

		// Parse enum values
		if strings.HasPrefix(line, "enum ") {
			addEnumConstraint(&param, extractEnumValue(line))
		}
	}

	return param
}

// parseLeafList parses a leaf-list statement
func (p *Parser) parseLeafList(scanner lineScanner, leafListLine string) Parameter {
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
			if typedef := p.resolveTypedef(param.YangType); typedef != nil {
				param.GoType = "[]" + yangTypeToGoType(typedef.BaseType)
				if len(typedef.EnumValues) > 0 {
					param.Constraints = append(param.Constraints, model.Constraint{
						Type:  model.ConstraintTypeEnum,
						Value: typedef.EnumValues,
					})
				}
				param.Constraints = append(param.Constraints, typedef.Constraints...)
			} else {
				param.GoType = "[]" + yangTypeToGoType(param.YangType)
			}

			if strings.Contains(line, "{") {
				p.parseInlineTypeConstraints(scanner, &param)
			}
		}

		if strings.HasPrefix(line, "description ") {
			param.Description = extractQuotedString(line)
		} else if line == "description" {
			param.Description = parseMultiLineDescription(scanner)
		}

		if strings.HasPrefix(line, "default ") {
			param.DefaultValue = parseDefaultValue(line, strings.TrimPrefix(param.GoType, "[]"))
		}

		if strings.HasPrefix(line, "pattern ") {
			param.Constraints = append(param.Constraints, model.Constraint{
				Type:  model.ConstraintTypePattern,
				Value: extractQuotedString(line),
			})
		}

		if strings.HasPrefix(line, "length ") {
			lengthStr := extractQuotedString(line)
			min, max, hasMin, hasMax := parseRangeWithPresence(lengthStr)
			param.Constraints = appendBounds(param.Constraints, min, max, hasMin, hasMax, model.ConstraintTypeMinLength, model.ConstraintTypeMaxLength)
		}

		if strings.HasPrefix(line, "range ") {
			rangeStr := extractQuotedString(line)
			min, max, hasMin, hasMax := parseRangeWithPresence(rangeStr)
			param.Constraints = appendBounds(param.Constraints, min, max, hasMin, hasMax, model.ConstraintTypeMin, model.ConstraintTypeMax)
		}

		if strings.HasPrefix(line, "enum ") {
			addEnumConstraint(&param, extractEnumValue(line))
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
func (p *Parser) parseNestedContainer(scanner lineScanner, containerLine string) Parameter {
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

		// Parse description (inline or multi-line)
		if strings.HasPrefix(trimmed, "description ") {
			param.Description = extractQuotedString(trimmed)
		} else if trimmed == "description" {
			param.Description = parseMultiLineDescription(scanner)
		}

		if strings.HasPrefix(trimmed, "key ") {
			keyStr := extractQuotedString(trimmed)
			if keyStr != "" {
				param.Keys = strings.Fields(keyStr)
			}
			continue
		}

		// Parse nested leaf
		if strings.HasPrefix(trimmed, "leaf ") {
			nestedDepth -= strings.Count(trimmed, "{")
			nestedParam := p.parseLeaf(scanner, trimmed)
			if containsString(param.Keys, nestedParam.Name) {
				nestedParam.Required = true
				nestedParam.Constraints = append(nestedParam.Constraints, model.Constraint{Type: model.ConstraintTypeRequired, Value: true})
			}
			param.NestedProperties = append(param.NestedProperties, nestedParam)
		}

		// Parse nested leaf-list
		if strings.HasPrefix(trimmed, "leaf-list ") {
			nestedDepth -= strings.Count(trimmed, "{")
			nestedParam := p.parseLeafList(scanner, trimmed)
			param.NestedProperties = append(param.NestedProperties, nestedParam)
		}

		// Parse recursively nested container
		if strings.HasPrefix(trimmed, "container ") {
			nestedDepth -= strings.Count(trimmed, "{")
			nestedParam := p.parseNestedContainer(scanner, trimmed)
			param.NestedProperties = append(param.NestedProperties, nestedParam)
		}

		// Parse recursively nested list
		if strings.HasPrefix(trimmed, "list ") {
			nestedDepth -= strings.Count(trimmed, "{")
			nestedParam := p.parseNestedList(scanner, trimmed)
			param.NestedProperties = append(param.NestedProperties, nestedParam)
		}
	}

	return param
}

// parseNestedList parses a nested list within a feature
func (p *Parser) parseNestedList(scanner lineScanner, listLine string) Parameter {
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

		// Parse description (inline or multi-line)
		if strings.HasPrefix(trimmed, "description ") {
			param.Description = extractQuotedString(trimmed)
		} else if trimmed == "description" {
			param.Description = parseMultiLineDescription(scanner)
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

		if strings.HasPrefix(trimmed, "key ") {
			keyStr := extractQuotedString(trimmed)
			if keyStr != "" {
				param.Keys = strings.Fields(keyStr)
			}
			continue
		}

		// Parse nested leaf
		if strings.HasPrefix(trimmed, "leaf ") {
			nestedDepth -= strings.Count(trimmed, "{")
			nestedParam := p.parseLeaf(scanner, trimmed)
			if containsString(param.Keys, nestedParam.Name) {
				nestedParam.Required = true
				nestedParam.Constraints = append(nestedParam.Constraints, model.Constraint{Type: model.ConstraintTypeRequired, Value: true})
			}
			param.NestedProperties = append(param.NestedProperties, nestedParam)
		}

		// Parse nested leaf-list
		if strings.HasPrefix(trimmed, "leaf-list ") {
			nestedDepth -= strings.Count(trimmed, "{")
			nestedParam := p.parseLeafList(scanner, trimmed)
			param.NestedProperties = append(param.NestedProperties, nestedParam)
		}

		// Parse recursively nested container
		if strings.HasPrefix(trimmed, "container ") {
			nestedDepth -= strings.Count(trimmed, "{")
			nestedParam := p.parseNestedContainer(scanner, trimmed)
			param.NestedProperties = append(param.NestedProperties, nestedParam)
		}

		// Parse recursively nested list
		if strings.HasPrefix(trimmed, "list ") {
			nestedDepth -= strings.Count(trimmed, "{")
			nestedParam := p.parseNestedList(scanner, trimmed)
			param.NestedProperties = append(param.NestedProperties, nestedParam)
		}
	}

	return param
}

// parseTypedef parses a typedef statement and returns a Typedef
func (p *Parser) parseTypedef(scanner lineScanner, typedefLine string, currentModule string) *Typedef {
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
			enumValue := extractEnumValue(trimmed)
			if enumValue != "" {
				typedef.EnumValues = append(typedef.EnumValues, enumValue)
			}
		}

		// Parse range constraint
		if strings.HasPrefix(trimmed, "range ") {
			rangeStr := extractQuotedString(trimmed)
			min, max, hasMin, hasMax := parseRangeWithPresence(rangeStr)
			typedef.Constraints = appendBounds(typedef.Constraints, min, max, hasMin, hasMax, model.ConstraintTypeMin, model.ConstraintTypeMax)
		}

		// Parse length constraint
		if strings.HasPrefix(trimmed, "length ") {
			lengthStr := extractQuotedString(trimmed)
			min, max, hasMin, hasMax := parseRangeWithPresence(lengthStr)
			typedef.Constraints = appendBounds(typedef.Constraints, min, max, hasMin, hasMax, model.ConstraintTypeMinLength, model.ConstraintTypeMaxLength)
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
func (p *Parser) parseInlineTypeConstraints(scanner lineScanner, param *Parameter) {
	depth := 1
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Parse length
		if strings.HasPrefix(line, "length ") {
			lengthStr := extractQuotedString(line)
			min, max, hasMin, hasMax := parseRangeWithPresence(lengthStr)
			param.Constraints = appendBounds(param.Constraints, min, max, hasMin, hasMax, model.ConstraintTypeMinLength, model.ConstraintTypeMaxLength)
		}

		// Parse range
		if strings.HasPrefix(line, "range ") {
			rangeStr := extractQuotedString(line)
			min, max, hasMin, hasMax := parseRangeWithPresence(rangeStr)
			param.Constraints = appendBounds(param.Constraints, min, max, hasMin, hasMax, model.ConstraintTypeMin, model.ConstraintTypeMax)
		}

		// Parse pattern
		if strings.HasPrefix(line, "pattern ") {
			param.Constraints = append(param.Constraints, model.Constraint{
				Type:  model.ConstraintTypePattern,
				Value: extractQuotedString(line),
			})
		}

		if strings.HasPrefix(line, "enum ") {
			addEnumConstraint(param, extractEnumValue(line))
		}

		depth += strings.Count(line, "{")
		depth -= strings.Count(line, "}")
		if depth <= 0 {
			break
		}
	}
}

// GetFeatures returns all parsed features
func (p *Parser) GetFeatures() map[string]*model.Feature {
	return p.features
}

// Helper functions for parsing YANG syntax

func extractGroupingName(line string) string {
	re := regexp.MustCompile(`grouping\s+(\S+)`)
	matches := re.FindStringSubmatch(line)
	if len(matches) > 1 {
		return strings.TrimSuffix(matches[1], "{")
	}
	return ""
}

// extractUsesName extracts the grouping name from a `uses grp-name;` or `uses prefix:grp-name {` line.
func extractUsesName(line string) string {
	re := regexp.MustCompile(`uses\s+(\S+?)[\s{;]`)
	matches := re.FindStringSubmatch(line + " ")
	if len(matches) > 1 {
		return strings.TrimSuffix(strings.TrimSuffix(matches[1], ";"), "{")
	}
	return ""
}

func extractModuleName(line string) string {
	re := regexp.MustCompile(`module\s+(\S+)`)
	matches := re.FindStringSubmatch(line)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}

// deriveFeatureNameFromModule converts a YANG module name to a feature name by
// stripping common prefixes like "extreme-intent-", "extreme-asset-", "extreme-inferred-".
// e.g. "extreme-intent-port-slpp" → "port-slpp", "extreme-asset-cdp" → "cdp"
func deriveFeatureNameFromModule(moduleName string) string {
	prefixes := []string{
		"extreme-intent-",
		"extreme-asset-",
		"extreme-inferred-",
		"extreme-",
	}
	name := moduleName
	for _, prefix := range prefixes {
		if strings.HasPrefix(name, prefix) {
			name = strings.TrimPrefix(name, prefix)
			break
		}
	}
	// Remove trailing " {" if present
	name = strings.TrimSuffix(name, " {")
	return name
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

// parseMultiLineDescription reads the scanner until a complete YANG description
// value is consumed. Used when the description keyword appears alone on a line
// with the quoted text on subsequent lines:
//
//	description
//	  "First line of text.
//	   Continuation line.";
func parseMultiLineDescription(scanner lineScanner) string {
	var buf strings.Builder
	inQuote := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !inQuote {
			if idx := strings.IndexByte(line, '"'); idx >= 0 {
				inQuote = true
				line = line[idx+1:] // strip up to and including the opening quote
			} else {
				continue
			}
		}
		// Check for closing quote (may be followed by ";")
		if end := strings.LastIndexByte(line, '"'); end >= 0 {
			buf.WriteString(line[:end])
			return buf.String()
		}
		// No closing quote yet — append this line
		if buf.Len() > 0 {
			buf.WriteByte(' ')
		}
		buf.WriteString(line)
	}
	return buf.String()
}

func extractQuotedString(line string) string {
	for _, quote := range []byte{'"', '\''} {
		start := strings.IndexByte(line, quote)
		if start < 0 {
			continue
		}
		end := -1
		for i := len(line) - 1; i > start; i-- {
			if line[i] == quote && line[i-1] != '\\' {
				end = i
				break
			}
		}
		if end > start {
			return line[start+1 : end]
		}
	}
	return ""
}

func extractEnumValue(line string) string {
	if value := extractQuotedString(line); value != "" {
		return value
	}
	re := regexp.MustCompile(`enum\s+([^\s{;]+)`)
	matches := re.FindStringSubmatch(line)
	if len(matches) > 1 {
		return strings.Trim(matches[1], `"'`)
	}
	return ""
}

func addEnumConstraint(param *Parameter, enumValue string) {
	if enumValue == "" {
		return
	}
	var enumValues []string
	if len(param.Constraints) > 0 && param.Constraints[len(param.Constraints)-1].Type == model.ConstraintTypeEnum {
		if existing, ok := param.Constraints[len(param.Constraints)-1].Value.([]string); ok {
			enumValues = existing
			param.Constraints = param.Constraints[:len(param.Constraints)-1]
		}
	}
	enumValues = append(enumValues, enumValue)
	param.Constraints = append(param.Constraints, model.Constraint{
		Type:  model.ConstraintTypeEnum,
		Value: enumValues,
	})
}

func parseDefaultValue(line string, goType string) interface{} {
	raw := extractQuotedString(line)
	if raw == "" && !strings.Contains(line, `""`) {
		raw = strings.TrimSpace(strings.TrimPrefix(line, "default"))
		raw = strings.TrimSuffix(raw, ";")
		raw = strings.TrimSpace(raw)
		raw = strings.Trim(raw, `"'`)
	}

	switch strings.ToLower(goType) {
	case "bool", "boolean":
		if value, err := strconv.ParseBool(raw); err == nil {
			return value
		}
	case "int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64":
		if value, err := strconv.Atoi(raw); err == nil {
			return value
		}
	}

	return raw
}

func extractNumber(line string) int {
	re := regexp.MustCompile(`\d+`)
	matches := re.FindString(line)
	var num int
	fmt.Sscanf(matches, "%d", &num)
	return num
}

func consumeCurrentBlock(scanner lineScanner, firstLine string) {
	blockDepth := strings.Count(firstLine, "{") - strings.Count(firstLine, "}")
	for blockDepth > 0 && scanner.Scan() {
		line := scanner.Text()
		blockDepth += strings.Count(line, "{")
		blockDepth -= strings.Count(line, "}")
	}
}

func isDescriptionStatement(trimmed string) bool {
	return trimmed == "description" || strings.HasPrefix(trimmed, "description ")
}

func consumeDescriptionStatement(scanner lineScanner, firstLine string) {
	inQuote := false
	if descriptionStatementComplete(firstLine, &inQuote) {
		return
	}
	for scanner.Scan() {
		if descriptionStatementComplete(strings.TrimSpace(scanner.Text()), &inQuote) {
			return
		}
	}
}

func descriptionStatementComplete(line string, inQuote *bool) bool {
	for i := 0; i < len(line); i++ {
		switch line[i] {
		case '"':
			if i == 0 || line[i-1] != '\\' {
				*inQuote = !*inQuote
			}
		case ';':
			if !*inQuote {
				return true
			}
		}
	}
	return false
}

func containsString(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func appendBounds(constraints []model.Constraint, min int, max int, hasMin bool, hasMax bool, minType model.ConstraintType, maxType model.ConstraintType) []model.Constraint {
	if hasMin {
		constraints = append(constraints, model.Constraint{Type: minType, Value: min})
	}
	if hasMax {
		constraints = append(constraints, model.Constraint{Type: maxType, Value: max})
	}
	return constraints
}

// parseRange parses YANG range/length format like "1..255" or "0..max"
func parseRange(rangeStr string) (min int, max int) {
	min, max, _, _ = parseRangeWithPresence(rangeStr)
	return min, max
}

func parseRangeWithPresence(rangeStr string) (min int, max int, hasMin bool, hasMax bool) {
	if rangeStr == "" {
		return 0, 0, false, false
	}

	// Handle "min..max" format
	if strings.Contains(rangeStr, "..") {
		parts := strings.Split(rangeStr, "..")
		if len(parts) == 2 {
			if parts[0] != "min" {
				fmt.Sscanf(parts[0], "%d", &min)
				hasMin = true
			}
			if parts[1] != "max" {
				fmt.Sscanf(parts[1], "%d", &max)
				hasMax = true
			}
		}
	} else {
		// Single value
		fmt.Sscanf(rangeStr, "%d", &max)
		min = max
		hasMin = true
		hasMax = true
	}

	return min, max, hasMin, hasMax
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
