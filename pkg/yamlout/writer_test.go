package yamlout

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/extremenetworks/testcase-generator/pkg/model"
)

func TestWriteRemovesStaleYAMLFiles(t *testing.T) {
	tempDir := t.TempDir()
	staleDir := filepath.Join(tempDir, "uncategorized")
	if err := os.MkdirAll(staleDir, 0o755); err != nil {
		t.Fatal(err)
	}
	stalePath := filepath.Join(staleDir, "stale.yaml")
	if err := os.WriteFile(stalePath, []byte("stale: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	writer := NewWriter(tempDir, true)
	suite := &model.TestSuite{
		Version: "1.0",
		Features: []model.FeatureTestGroup{
			{FeatureName: "fresh"},
		},
	}

	if err := writer.Write(suite); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	if _, err := os.Stat(stalePath); !os.IsNotExist(err) {
		t.Fatalf("expected stale YAML to be removed, stat error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tempDir, "uncategorized", "fresh.yaml")); err != nil {
		t.Fatalf("expected fresh YAML to be written: %v", err)
	}
}
