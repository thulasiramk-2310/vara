package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thulasiramk-2310/vara/internal/commands"
	"github.com/thulasiramk-2310/vara/internal/repository"
	"github.com/thulasiramk-2310/vara/pkg/index"
)

func TestBranchCommand(t *testing.T) {
	tmpDir := t.TempDir()

	repoDir := filepath.Join(tmpDir, ".vara")
	os.Mkdir(repoDir, 0755)
	os.Mkdir(filepath.Join(repoDir, "objects"), 0755)

	repo := &repository.Repository{
		RootDir: tmpDir,
		VaraDir: repoDir,
	}
	idx := index.New()
	ctx := &commands.Context{
		Repository: repo,
		Index:      idx,
	}

	// 1. Initial Commit
	os.WriteFile(filepath.Join(tmpDir, "file.txt"), []byte("data"), 0644)
	err := commands.RunAdd(ctx, []string{"."})
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	_, err = commands.RunCommit(ctx, "Initial commit")
	if err != nil {
		t.Fatalf("Commit failed: %v", err)
	}

	// 2. Create Branch
	_, err = commands.RunBranch(ctx, "feature")
	if err != nil {
		t.Fatalf("Branch creation failed: %v", err)
	}

	// 3. List Branches
	out, err := commands.RunBranch(ctx, "")
	if err != nil {
		t.Fatalf("List branches failed: %v", err)
	}

	// Output should have `* main` and `  feature`
	if !strings.Contains(out, "* main") {
		t.Errorf("Expected '* main' in output, got:\n%s", out)
	}
	if !strings.Contains(out, "  feature") {
		t.Errorf("Expected '  feature' in output, got:\n%s", out)
	}

	// 4. Verify reflog for the new branch
	logData, err := os.ReadFile(filepath.Join(repoDir, "logs", "refs", "heads", "feature"))
	if err != nil {
		t.Fatalf("Reflog for feature branch not created: %v", err)
	}
	if !strings.Contains(string(logData), "branch: Created from HEAD") {
		t.Errorf("Reflog missing branch creation message")
	}
}
