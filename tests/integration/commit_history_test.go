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

func TestCommitAndHistory(t *testing.T) {
	tmpDir := t.TempDir()

	// 1. Init repo
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

	// 2. Add some files
	os.WriteFile(filepath.Join(tmpDir, "file1.txt"), []byte("Hello 1"), 0644)
	err := commands.RunAdd(ctx, []string{"."})
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	// 3. Commit
	commit1ID, err := commands.RunCommit(ctx, "First commit")
	if err != nil {
		t.Fatalf("Commit 1 failed: %v", err)
	}

	// 4. Modify and Add again
	os.WriteFile(filepath.Join(tmpDir, "file1.txt"), []byte("Hello 1 updated"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "file2.txt"), []byte("Hello 2"), 0644)
	
	// Reload index as add saves to disk
	idxBytes, _ := os.ReadFile(filepath.Join(repoDir, "index"))
	ctx.Index, _ = index.Deserialize(idxBytes)

	err = commands.RunAdd(ctx, []string{"."})
	if err != nil {
		t.Fatalf("Add 2 failed: %v", err)
	}

	// 5. Commit 2
	commit2ID, err := commands.RunCommit(ctx, "Second commit")
	if err != nil {
		t.Fatalf("Commit 2 failed: %v", err)
	}

	// 6. Run History
	out, err := commands.RunHistory(ctx)
	if err != nil {
		t.Fatalf("History failed: %v", err)
	}

	// Output should contain both commit messages and their IDs
	if !strings.Contains(out, commit2ID.String()) || !strings.Contains(out, "Second commit") {
		t.Errorf("History missing commit 2 details:\n%s", out)
	}
	if !strings.Contains(out, commit1ID.String()) || !strings.Contains(out, "First commit") {
		t.Errorf("History missing commit 1 details:\n%s", out)
	}
}
