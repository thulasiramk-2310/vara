package snapshot

import (
	"os"
	"path/filepath"
	"testing"
)

// Two snapshots of the same command in the same second must both survive.
func TestCreateNeverOverwritesASnapshotFromTheSameSecond(t *testing.T) {
	root := t.TempDir()
	vara := filepath.Join(root, ".vara")
	if err := os.MkdirAll(vara, 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(root, "a.txt"), []byte("first"), 0o644)
	commit := "3026564000000000000000000000000000000000000000000000000000000000"
	if _, err := Create(vara, root, "switch", commit); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(root, "a.txt"), []byte("second"), 0o644)
	if _, err := Create(vara, root, "switch", commit); err != nil {
		t.Fatal(err)
	}
	names, err := List(vara)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 2 {
		t.Fatalf("want 2 snapshots, got %v", names)
	}
}
