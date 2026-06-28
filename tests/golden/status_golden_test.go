package golden

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/thulasiramk-2310/vara/internal/status"
)

func TestStatusFormatShort_Golden(t *testing.T) {
	sr := &status.StatusResult{
		Clean:      []string{"docs/rfc-0001.md"},
		Modified:   []string{"main.go"},
		Staged:     []string{"new_file.txt"},
		Deleted:    []string{"old_code.go"},
		Untracked:  []string{"secret.txt", "untracked.go"},
		Conflicted: []string{"merge_conflict.go"},
	}

	got := status.FormatShort(sr)

	goldenPath := filepath.Join("testdata", "status_short.golden")
	
	// If the UPDATE_GOLDEN env var is set, update the golden file
	if os.Getenv("UPDATE_GOLDEN") == "true" {
		os.MkdirAll(filepath.Dir(goldenPath), 0755)
		os.WriteFile(goldenPath, []byte(got), 0644)
	}

	wantBytes, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("Failed reading golden file %s: %v. Run with UPDATE_GOLDEN=true to create it.", goldenPath, err)
	}
	want := string(wantBytes)

	if got != want {
		t.Errorf("FormatShort() mismatch.\nGot:\n%s\nWant:\n%s", got, want)
	}
}
