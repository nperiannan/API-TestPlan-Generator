package web

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// VersionManager handles version comparison and diff generation
type VersionManager struct {
	store *MinIOStore
}

// VersionDiff represents the diff between two versions
type VersionDiff struct {
	OldVersion  string          `json:"oldVersion"`
	NewVersion  string          `json:"newVersion"`
	OldTag      string          `json:"oldTag"`
	NewTag      string          `json:"newTag"`
	Summary     DiffSummary     `json:"summary"`
	FileChanges []FileChange    `json:"fileChanges"`
	TestDiffs   []FeatureTestDiff `json:"testDiffs"`
}

// DiffSummary provides high-level diff statistics
type DiffSummary struct {
	TotalTestsOld     int            `json:"totalTestsOld"`
	TotalTestsNew     int            `json:"totalTestsNew"`
	TestsDelta        int            `json:"testsDelta"`
	FeaturesAdded     int            `json:"featuresAdded"`
	FeaturesRemoved   int            `json:"featuresRemoved"`
	FeaturesModified  int            `json:"featuresModified"`
	FilesAdded        int            `json:"filesAdded"`
	FilesRemoved      int            `json:"filesRemoved"`
	FilesModified     int            `json:"filesModified"`
	CategoryChanges   map[string]int `json:"categoryChanges"`
}

// FileChange represents a change to a specific file
type FileChange struct {
	Path       string `json:"path"`
	ChangeType string `json:"changeType"` // "added", "removed", "modified"
	OldSize    int    `json:"oldSize,omitempty"`
	NewSize    int    `json:"newSize,omitempty"`
}

// FeatureTestDiff shows test count changes per feature
type FeatureTestDiff struct {
	FeatureName    string         `json:"featureName"`
	Category       string         `json:"category"`
	ChangeType     string         `json:"changeType"` // "added", "removed", "modified"
	OldTotalTests  int            `json:"oldTotalTests"`
	NewTotalTests  int            `json:"newTotalTests"`
	Delta          int            `json:"delta"`
	CategoryDeltas map[string]int `json:"categoryDeltas"` // e.g., {"functional": +5, "boundary": -1}
}

// NewVersionManager creates a version manager
func NewVersionManager(store *MinIOStore) *VersionManager {
	return &VersionManager{store: store}
}

// CreateVersion creates a new version from current test plan files
func (vm *VersionManager) CreateVersion(tag, description string, files map[string][]byte) (*StoredVersion, error) {
	id := generateVersionID(tag)

	// Count tests from YAML files
	totalTests := 0
	featureCount := 0
	categories := make(map[string]int)

	for name, data := range files {
		if !strings.HasSuffix(name, ".yaml") {
			continue
		}
		counts := countTestsInYAML(data)
		totalTests += counts.total
		featureCount++
		for cat, count := range counts.byCategory {
			categories[cat] += count
		}
	}

	version := &StoredVersion{
		ID:          id,
		Tag:         tag,
		CreatedAt:   time.Now().UTC(),
		Description: description,
		TotalTests:  totalTests,
		Features:    featureCount,
		Categories:  categories,
	}

	if err := vm.store.StoreVersion(version, files); err != nil {
		return nil, err
	}

	return version, nil
}

// DiffVersions compares two versions
func (vm *VersionManager) DiffVersions(oldID, newID string) (*VersionDiff, error) {
	oldVersion, err := vm.store.GetVersion(oldID)
	if err != nil {
		return nil, fmt.Errorf("failed to get old version: %w", err)
	}
	newVersion, err := vm.store.GetVersion(newID)
	if err != nil {
		return nil, fmt.Errorf("failed to get new version: %w", err)
	}

	oldFiles, err := vm.store.GetVersionFiles(oldID)
	if err != nil {
		return nil, fmt.Errorf("failed to get old version files: %w", err)
	}
	newFiles, err := vm.store.GetVersionFiles(newID)
	if err != nil {
		return nil, fmt.Errorf("failed to get new version files: %w", err)
	}

	diff := &VersionDiff{
		OldVersion: oldID,
		NewVersion: newID,
		OldTag:     oldVersion.Tag,
		NewTag:     newVersion.Tag,
		Summary: DiffSummary{
			TotalTestsOld:   oldVersion.TotalTests,
			TotalTestsNew:   newVersion.TotalTests,
			TestsDelta:      newVersion.TotalTests - oldVersion.TotalTests,
			CategoryChanges: make(map[string]int),
		},
	}

	// Compare files
	allPaths := make(map[string]bool)
	for path := range oldFiles {
		allPaths[path] = true
	}
	for path := range newFiles {
		allPaths[path] = true
	}

	sortedPaths := make([]string, 0, len(allPaths))
	for p := range allPaths {
		sortedPaths = append(sortedPaths, p)
	}
	sort.Strings(sortedPaths)

	for _, path := range sortedPaths {
		oldData, inOld := oldFiles[path]
		newData, inNew := newFiles[path]

		switch {
		case inOld && !inNew:
			diff.FileChanges = append(diff.FileChanges, FileChange{
				Path: path, ChangeType: "removed", OldSize: len(oldData),
			})
			diff.Summary.FilesRemoved++
		case !inOld && inNew:
			diff.FileChanges = append(diff.FileChanges, FileChange{
				Path: path, ChangeType: "added", NewSize: len(newData),
			})
			diff.Summary.FilesAdded++
		default:
			oldHash := sha256.Sum256(oldData)
			newHash := sha256.Sum256(newData)
			if oldHash != newHash {
				diff.FileChanges = append(diff.FileChanges, FileChange{
					Path: path, ChangeType: "modified", OldSize: len(oldData), NewSize: len(newData),
				})
				diff.Summary.FilesModified++
			}
		}
	}

	// Compare test counts per feature (YAML files only)
	oldTestCounts := buildFeatureTestCounts(oldFiles)
	newTestCounts := buildFeatureTestCounts(newFiles)

	allFeatures := make(map[string]bool)
	for f := range oldTestCounts {
		allFeatures[f] = true
	}
	for f := range newTestCounts {
		allFeatures[f] = true
	}

	for feature := range allFeatures {
		oldCounts, inOld := oldTestCounts[feature]
		newCounts, inNew := newTestCounts[feature]

		ftd := FeatureTestDiff{
			FeatureName:    feature,
			CategoryDeltas: make(map[string]int),
		}

		switch {
		case inOld && !inNew:
			ftd.ChangeType = "removed"
			ftd.OldTotalTests = oldCounts.total
			ftd.Delta = -oldCounts.total
			ftd.Category = oldCounts.category
			diff.Summary.FeaturesRemoved++
		case !inOld && inNew:
			ftd.ChangeType = "added"
			ftd.NewTotalTests = newCounts.total
			ftd.Delta = newCounts.total
			ftd.Category = newCounts.category
			diff.Summary.FeaturesAdded++
		default:
			ftd.ChangeType = "modified"
			ftd.OldTotalTests = oldCounts.total
			ftd.NewTotalTests = newCounts.total
			ftd.Delta = newCounts.total - oldCounts.total
			ftd.Category = newCounts.category
			if ftd.Delta != 0 {
				diff.Summary.FeaturesModified++
			}
		}

		// Category-level deltas
		allCats := make(map[string]bool)
		if inOld {
			for c := range oldCounts.byCategory {
				allCats[c] = true
			}
		}
		if inNew {
			for c := range newCounts.byCategory {
				allCats[c] = true
			}
		}
		for cat := range allCats {
			oldC := 0
			newC := 0
			if inOld {
				oldC = oldCounts.byCategory[cat]
			}
			if inNew {
				newC = newCounts.byCategory[cat]
			}
			delta := newC - oldC
			if delta != 0 {
				ftd.CategoryDeltas[cat] = delta
				diff.Summary.CategoryChanges[cat] += delta
			}
		}

		diff.TestDiffs = append(diff.TestDiffs, ftd)
	}

	sort.Slice(diff.TestDiffs, func(i, j int) bool {
		return diff.TestDiffs[i].FeatureName < diff.TestDiffs[j].FeatureName
	})

	return diff, nil
}

type testCounts struct {
	total      int
	category   string
	byCategory map[string]int
}

func countTestsInYAML(data []byte) testCounts {
	tc := testCounts{byCategory: make(map[string]int)}

	var parsed map[string]interface{}
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		return tc
	}

	// Navigate to features[0].tests
	featuresRaw, ok := parsed["features"]
	if !ok {
		return tc
	}
	features, ok := featuresRaw.([]interface{})
	if !ok || len(features) == 0 {
		return tc
	}
	feature, ok := features[0].(map[string]interface{})
	if !ok {
		return tc
	}

	if cat, ok := feature["blueprintCategory"].(string); ok {
		tc.category = cat
	}

	testsRaw, ok := feature["tests"]
	if !ok {
		return tc
	}
	tests, ok := testsRaw.(map[string]interface{})
	if !ok {
		return tc
	}

	for catName, catTests := range tests {
		if arr, ok := catTests.([]interface{}); ok {
			tc.byCategory[catName] = len(arr)
			tc.total += len(arr)
		}
	}

	return tc
}

func buildFeatureTestCounts(files map[string][]byte) map[string]testCounts {
	result := make(map[string]testCounts)
	for name, data := range files {
		if !strings.HasSuffix(name, ".yaml") {
			continue
		}
		// Feature name from filename
		feature := strings.TrimSuffix(name, ".yaml")
		if parts := strings.SplitN(feature, "/", 2); len(parts) == 2 {
			feature = parts[1]
		}
		result[feature] = countTestsInYAML(data)
	}
	return result
}

func generateVersionID(tag string) string {
	ts := time.Now().UTC().Format("20060102-150405")
	h := sha256.Sum256([]byte(tag + ts))
	return fmt.Sprintf("%s-%x", ts, h[:4])
}

// VersionSummaryJSON returns the version summary as JSON bytes
func VersionSummaryJSON(versions []StoredVersion) ([]byte, error) {
	return json.Marshal(versions)
}
