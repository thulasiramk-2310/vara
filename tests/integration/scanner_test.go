package integration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/thulasiramk-2310/vara/internal/worktree"
	"github.com/thulasiramk-2310/vara/pkg/index"
	"github.com/thulasiramk-2310/vara/pkg/scanner"
)

func TestScanner(t *testing.T) {
	workDir := t.TempDir()

	// 1. Create a file on disk
	file1Path := filepath.Join(workDir, "untracked.txt")
	os.WriteFile(file1Path, []byte("hello"), 0644)

	// 2. Create index representing previous state
	idx := index.New()
	idx.Entries = append(idx.Entries, index.Entry{
		Path:  "deleted.txt",
		State: index.StateUnmodified, // It was tracked, now missing from disk
	})

	// 3. Scan
	s := scanner.New(idx)
	wt, err := worktree.New(workDir)
	if err != nil {
		t.Fatalf("Worktree init failed: %v", err)
	}

	res, err := s.Scan(wt)
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	// 4. Verify differences
	if res.Files["untracked.txt"] != scanner.StatusUntracked {
		t.Errorf("Expected untracked.txt to be Untracked, got %v", res.Files["untracked.txt"])
	}

	if res.Files["deleted.txt"] != scanner.StatusDeleted {
		t.Errorf("Expected deleted.txt to be Deleted, got %v", res.Files["deleted.txt"])
	}
}
