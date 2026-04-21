package yamlout

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/extremenetworks/testcase-generator/pkg/model"
)

// CoverageData represents the structure from coverage-report.json
type CoverageData struct {
	TotalTests            int                            `json:"totalTests"`
	DeploymentTests       int                            `json:"deploymentTests"`
	DeploymentPercent     float64                        `json:"deploymentPercent"`
	NonDeploymentTests    int                            `json:"nonDeploymentTests"`
	NonDeploymentPercent  float64                        `json:"nonDeploymentPercent"`
	TestsByCategory       map[string]int                 `json:"testsByCategory"`
	TestsByFeature        map[string]int                 `json:"testsByFeature"`
	PropertyCoverage      []PropertyCoverageItem         `json:"propertyCoverage"`
	PathParameterCoverage map[string]PathParameterDetail `json:"pathParameterCoverage"`
	TopEndpoints          []EndpointUsage                `json:"topEndpoints"`
}

type PropertyCoverageItem struct {
	FeatureName         string         `json:"featureName"`
	TotalProperties     int            `json:"totalProperties"`
	CoveredProperties   int            `json:"coveredProperties"`
	UncoveredProperties int            `json:"uncoveredProperties"`
	CoveragePercent     float64        `json:"coveragePercent"`
	PropertyUsage       map[string]int `json:"propertyUsage"`
	UncoveredList       []string       `json:"uncoveredList"`
}

type PathParameterDetail struct {
	Covered   []string `json:"covered"`
	Total     []string `json:"total"`
	Uncovered []string `json:"uncovered"`
}

type EndpointUsage struct {
	Endpoint string `json:"endpoint"`
	Count    int    `json:"count"`
}

// FeatureSummary represents test summary data for a feature
type FeatureSummary struct {
	Name                 string
	Path                 string
	ProfileType          string
	TotalTests           int
	DeploymentTests      int
	NonDeploymentTests   int
	DeploymentPercent    float64
	NonDeploymentPercent float64
	TestsByCategory      map[model.TestCategory]int
}

// WriteHTMLReport generates a comprehensive HTML report
func (w *Writer) WriteHTMLReport(suite *model.TestSuite, coverageJSONPath string) error {
	// Read coverage data
	coverageData, err := w.readCoverageJSON(coverageJSONPath)
	if err != nil {
		return fmt.Errorf("failed to read coverage JSON: %w", err)
	}

	// Build feature summaries
	featureSummaries := w.buildFeatureSummaries(suite)

	// Generate HTML
	html := w.generateHTML(featureSummaries, coverageData)

	// Write HTML file
	htmlPath := filepath.Join(w.outputDir, "test-coverage-report.html")
	if err := os.WriteFile(htmlPath, []byte(html), 0644); err != nil {
		return fmt.Errorf("failed to write HTML report: %w", err)
	}

	fmt.Printf("Generated HTML report: %s\n", htmlPath)
	return nil
}

func (w *Writer) readCoverageJSON(path string) (*CoverageData, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var coverage CoverageData
	if err := json.Unmarshal(data, &coverage); err != nil {
		return nil, err
	}

	return &coverage, nil
}

func (w *Writer) buildFeatureSummaries(suite *model.TestSuite) []FeatureSummary {
	var summaries []FeatureSummary

	for _, feature := range suite.Features {
		summary := FeatureSummary{
			Name:            feature.FeatureName,
			Path:            feature.FeaturePath,
			ProfileType:     string(feature.ProfileType),
			TestsByCategory: make(map[model.TestCategory]int),
		}

		for category, tests := range feature.Tests {
			count := len(tests)
			summary.TotalTests += count
			summary.TestsByCategory[category] = count

			for _, test := range tests {
				if test.IsDeploymentTest {
					summary.DeploymentTests++
				}
			}
		}

		summary.NonDeploymentTests = summary.TotalTests - summary.DeploymentTests
		if summary.TotalTests > 0 {
			summary.DeploymentPercent = float64(summary.DeploymentTests) * 100 / float64(summary.TotalTests)
			summary.NonDeploymentPercent = float64(summary.NonDeploymentTests) * 100 / float64(summary.TotalTests)
		}

		summaries = append(summaries, summary)
	}

	return summaries
}

func (w *Writer) generateHTML(summaries []FeatureSummary, coverage *CoverageData) string {
	var html strings.Builder

	// HTML Header
	html.WriteString(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Test Coverage Report</title>
    <style>
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }
        
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, Cantarell, sans-serif;
            background: #f5f7fa;
            color: #2c3e50;
            line-height: 1.6;
        }
        
        .container {
            max-width: 1400px;
            margin: 0 auto;
            padding: 20px;
        }
        
        header {
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            color: white;
            padding: 30px;
            border-radius: 10px;
            margin-bottom: 30px;
            box-shadow: 0 4px 6px rgba(0,0,0,0.1);
        }
        
        header h1 {
            font-size: 2.5em;
            margin-bottom: 10px;
        }
        
        header p {
            font-size: 1.1em;
            opacity: 0.9;
        }
        
        .summary-cards {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(250px, 1fr));
            gap: 20px;
            margin-bottom: 30px;
        }
        
        .card {
            background: white;
            padding: 20px;
            border-radius: 8px;
            box-shadow: 0 2px 4px rgba(0,0,0,0.1);
            transition: transform 0.2s, box-shadow 0.2s;
        }
        
        .card:hover {
            transform: translateY(-2px);
            box-shadow: 0 4px 8px rgba(0,0,0,0.15);
        }
        
        .card-title {
            font-size: 0.9em;
            color: #7f8c8d;
            text-transform: uppercase;
            letter-spacing: 0.5px;
            margin-bottom: 10px;
        }
        
        .card-value {
            font-size: 2.5em;
            font-weight: bold;
            color: #2c3e50;
        }
        
        .card-subtitle {
            font-size: 0.9em;
            color: #95a5a6;
            margin-top: 5px;
        }
        
        .feature-section {
            background: white;
            padding: 25px;
            border-radius: 8px;
            margin-bottom: 25px;
            box-shadow: 0 2px 4px rgba(0,0,0,0.1);
        }
        
        .feature-header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            margin-bottom: 20px;
            padding-bottom: 15px;
            border-bottom: 2px solid #ecf0f1;
        }
        
        .feature-name {
            font-size: 1.8em;
            font-weight: bold;
            color: #2c3e50;
        }
        
        .feature-path {
            font-size: 0.9em;
            color: #7f8c8d;
            font-family: 'Courier New', monospace;
            background: #ecf0f1;
            padding: 5px 10px;
            border-radius: 4px;
        }
        
        .stats-grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
            gap: 15px;
            margin-bottom: 20px;
        }
        
        .stat-box {
            background: #f8f9fa;
            padding: 15px;
            border-radius: 6px;
            border-left: 4px solid #3498db;
        }
        
        .stat-label {
            font-size: 0.85em;
            color: #7f8c8d;
            margin-bottom: 5px;
        }
        
        .stat-value {
            font-size: 1.8em;
            font-weight: bold;
            color: #2c3e50;
        }
        
        .progress-bar-container {
            background: #ecf0f1;
            height: 30px;
            border-radius: 15px;
            overflow: hidden;
            margin: 10px 0;
            position: relative;
        }
        
        .progress-bar {
            height: 100%;
            background: linear-gradient(90deg, #3498db, #2ecc71);
            transition: width 0.3s ease;
            display: flex;
            align-items: center;
            justify-content: center;
            color: white;
            font-weight: bold;
            font-size: 0.9em;
        }
        
        .progress-bar.deployment {
            background: linear-gradient(90deg, #667eea, #764ba2);
        }
        
        .progress-bar.non-deployment {
            background: linear-gradient(90deg, #f093fb, #f5576c);
        }
        
        .category-breakdown {
            display: flex;
            flex-wrap: wrap;
            gap: 10px;
            margin: 15px 0;
        }
        
        .category-badge {
            padding: 8px 15px;
            border-radius: 20px;
            font-size: 0.9em;
            font-weight: 600;
            display: inline-flex;
            align-items: center;
            gap: 5px;
        }
        
        .category-badge.functional { background: #e3f2fd; color: #1976d2; }
        .category-badge.negative { background: #ffebee; color: #c62828; }
        .category-badge.boundary { background: #fff3e0; color: #ef6c00; }
        .category-badge.scale { background: #e8f5e9; color: #2e7d32; }
        .category-badge.performance { background: #f3e5f5; color: #7b1fa2; }
        
        .property-table {
            width: 100%;
            border-collapse: collapse;
            margin-top: 15px;
        }
        
        .property-table th {
            background: #34495e;
            color: white;
            padding: 12px;
            text-align: left;
            font-weight: 600;
        }
        
        .property-table td {
            padding: 10px 12px;
            border-bottom: 1px solid #ecf0f1;
        }
        
        .property-table tr:hover {
            background: #f8f9fa;
        }
        
        .coverage-good { color: #27ae60; font-weight: bold; }
        .coverage-medium { color: #f39c12; font-weight: bold; }
        .coverage-poor { color: #e74c3c; font-weight: bold; }
        
        .uncovered-list {
            display: flex;
            flex-wrap: wrap;
            gap: 5px;
            margin-top: 5px;
        }
        
        .uncovered-item {
            background: #ecf0f1;
            padding: 4px 8px;
            border-radius: 4px;
            font-size: 0.85em;
            font-family: 'Courier New', monospace;
            color: #7f8c8d;
        }
        
        .accordion {
            margin-top: 15px;
        }
        
        .accordion-header {
            background: #ecf0f1;
            padding: 12px 15px;
            cursor: pointer;
            border-radius: 6px;
            font-weight: 600;
            display: flex;
            justify-content: space-between;
            align-items: center;
            transition: background 0.2s;
        }
        
        .accordion-header:hover {
            background: #d5dbdb;
        }
        
        .accordion-content {
            padding: 15px;
            display: none;
        }
        
        .accordion-content.active {
            display: block;
        }
        
        .toggle-icon {
            transition: transform 0.3s;
        }
        
        .toggle-icon.active {
            transform: rotate(180deg);
        }
        
        @media (max-width: 768px) {
            .stats-grid {
                grid-template-columns: 1fr;
            }
            
            .feature-header {
                flex-direction: column;
                align-items: flex-start;
                gap: 10px;
            }
        }
    </style>
</head>
<body>
    <div class="container">
        <header>
            <h1>📊 Test Coverage Report</h1>
            <p>Comprehensive test coverage analysis for all features</p>
        </header>
`)

	// Overall Summary Cards
	html.WriteString(`
        <div class="summary-cards">
            <div class="card">
                <div class="card-title">Total Tests</div>
                <div class="card-value">` + fmt.Sprintf("%d", coverage.TotalTests) + `</div>
                <div class="card-subtitle">Generated test cases</div>
            </div>
            <div class="card">
                <div class="card-title">Deployment Tests</div>
                <div class="card-value">` + fmt.Sprintf("%d", coverage.DeploymentTests) + `</div>
                <div class="card-subtitle">` + fmt.Sprintf("%.1f%%", coverage.DeploymentPercent) + ` of total</div>
            </div>
            <div class="card">
                <div class="card-title">Non-Deployment Tests</div>
                <div class="card-value">` + fmt.Sprintf("%d", coverage.NonDeploymentTests) + `</div>
                <div class="card-subtitle">` + fmt.Sprintf("%.1f%%", coverage.NonDeploymentPercent) + ` of total</div>
            </div>
            <div class="card">
                <div class="card-title">Features Tested</div>
                <div class="card-value">` + fmt.Sprintf("%d", len(summaries)) + `</div>
                <div class="card-subtitle">Feature coverage</div>
            </div>
        </div>
`)

	// Feature Details
	for _, summary := range summaries {
		w.writeFeatureSection(&html, summary, coverage)
	}

	// JavaScript for accordions
	html.WriteString(`
        <script>
            document.querySelectorAll('.accordion-header').forEach(header => {
                header.addEventListener('click', () => {
                    const content = header.nextElementSibling;
                    const icon = header.querySelector('.toggle-icon');
                    
                    content.classList.toggle('active');
                    icon.classList.toggle('active');
                });
            });
        </script>
    </div>
</body>
</html>
`)

	return html.String()
}

func (w *Writer) writeFeatureSection(html *strings.Builder, summary FeatureSummary, coverage *CoverageData) {
	html.WriteString(`
        <div class="feature-section">
            <div class="feature-header">
                <div>
                    <div class="feature-name">` + summary.Name + `</div>
                    <div class="feature-path">` + summary.Path + `</div>
                </div>
                <div class="category-badge" style="background: #e3f2fd; color: #1976d2;">
                    ` + summary.ProfileType + `
                </div>
            </div>
`)

	// Stats Grid
	html.WriteString(`
            <div class="stats-grid">
                <div class="stat-box">
                    <div class="stat-label">Total Tests</div>
                    <div class="stat-value">` + fmt.Sprintf("%d", summary.TotalTests) + `</div>
                </div>
                <div class="stat-box" style="border-left-color: #667eea;">
                    <div class="stat-label">Deployment Tests</div>
                    <div class="stat-value">` + fmt.Sprintf("%d", summary.DeploymentTests) + `</div>
                </div>
                <div class="stat-box" style="border-left-color: #f5576c;">
                    <div class="stat-label">Non-Deployment</div>
                    <div class="stat-value">` + fmt.Sprintf("%d", summary.NonDeploymentTests) + `</div>
                </div>
            </div>
`)

	// Progress Bars
	html.WriteString(`
            <div>
                <div style="font-size: 0.9em; color: #7f8c8d; margin-bottom: 5px;">Deployment Coverage</div>
                <div class="progress-bar-container">
                    <div class="progress-bar deployment" style="width: ` + fmt.Sprintf("%.1f%%", summary.DeploymentPercent) + `">
                        ` + fmt.Sprintf("%.1f%%", summary.DeploymentPercent) + `
                    </div>
                </div>
                <div style="font-size: 0.9em; color: #7f8c8d; margin-bottom: 5px; margin-top: 15px;">Non-Deployment Coverage</div>
                <div class="progress-bar-container">
                    <div class="progress-bar non-deployment" style="width: ` + fmt.Sprintf("%.1f%%", summary.NonDeploymentPercent) + `">
                        ` + fmt.Sprintf("%.1f%%", summary.NonDeploymentPercent) + `
                    </div>
                </div>
            </div>
`)

	// Category Breakdown
	html.WriteString(`
            <div style="margin-top: 20px;">
                <div style="font-size: 0.9em; color: #7f8c8d; margin-bottom: 10px;">Tests by Category</div>
                <div class="category-breakdown">
`)

	// Sort categories for consistent display
	categories := []model.TestCategory{
		model.TestCategoryFunctional,
		model.TestCategoryNegative,
		model.TestCategoryBoundary,
		model.TestCategoryScale,
		model.TestCategoryPerformance,
	}

	for _, cat := range categories {
		if count, ok := summary.TestsByCategory[cat]; ok && count > 0 {
			html.WriteString(`
                    <div class="category-badge ` + string(cat) + `">
                        ` + string(cat) + `: ` + fmt.Sprintf("%d", count) + `
                    </div>
`)
		}
	}

	html.WriteString(`
                </div>
            </div>
`)

	// Property Coverage Section
	w.writePropertyCoverage(html, summary.Name, coverage)

	// Path Parameter Coverage Section
	w.writePathParameterCoverage(html, summary.Name, coverage)

	html.WriteString(`
        </div>
`)
}

func (w *Writer) writePropertyCoverage(html *strings.Builder, featureName string, coverage *CoverageData) {
	var propCoverage *PropertyCoverageItem
	for i := range coverage.PropertyCoverage {
		if coverage.PropertyCoverage[i].FeatureName == featureName {
			propCoverage = &coverage.PropertyCoverage[i]
			break
		}
	}

	if propCoverage == nil || propCoverage.TotalProperties == 0 {
		return
	}

	coverageClass := "coverage-poor"
	if propCoverage.CoveragePercent >= 80 {
		coverageClass = "coverage-good"
	} else if propCoverage.CoveragePercent >= 50 {
		coverageClass = "coverage-medium"
	}

	html.WriteString(`
            <div class="accordion">
                <div class="accordion-header">
                    <span>📋 Object Properties Coverage: <span class="` + coverageClass + `">` +
		fmt.Sprintf("%.1f%%", propCoverage.CoveragePercent) + `</span> (` +
		fmt.Sprintf("%d/%d", propCoverage.CoveredProperties, propCoverage.TotalProperties) + ` properties)</span>
                    <span class="toggle-icon">▼</span>
                </div>
                <div class="accordion-content">
`)

	if len(propCoverage.PropertyUsage) > 0 {
		html.WriteString(`
                    <h4 style="margin-bottom: 10px;">Covered Properties:</h4>
                    <table class="property-table">
                        <tr>
                            <th>Property Name</th>
                            <th>Usage Count</th>
                        </tr>
`)

		// Sort properties by usage count
		type propUsage struct {
			name  string
			count int
		}
		var props []propUsage
		for name, count := range propCoverage.PropertyUsage {
			props = append(props, propUsage{name, count})
		}
		sort.Slice(props, func(i, j int) bool {
			return props[i].count > props[j].count
		})

		for _, prop := range props {
			html.WriteString(`
                        <tr>
                            <td><code>` + prop.name + `</code></td>
                            <td>` + fmt.Sprintf("%d tests", prop.count) + `</td>
                        </tr>
`)
		}

		html.WriteString(`
                    </table>
`)
	}

	if len(propCoverage.UncoveredList) > 0 {
		html.WriteString(`
                    <h4 style="margin: 20px 0 10px 0; color: #e74c3c;">Uncovered Properties (` +
			fmt.Sprintf("%d", len(propCoverage.UncoveredList)) + `):</h4>
                    <div class="uncovered-list">
`)

		for _, prop := range propCoverage.UncoveredList {
			html.WriteString(`
                        <span class="uncovered-item">` + prop + `</span>
`)
		}

		html.WriteString(`
                    </div>
`)
	}

	html.WriteString(`
                </div>
            </div>
`)
}

func (w *Writer) writePathParameterCoverage(html *strings.Builder, featureName string, coverage *CoverageData) {
	// Path parameter coverage is organized by endpoint, not feature
	// We'll skip this section for now as it's endpoint-based, not feature-based
	// This could be enhanced in the future to show path parameters used by each feature
}
