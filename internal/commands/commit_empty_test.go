package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thulasiramk-2310/vara/internal/devidentity"
	"github.com/thulasiramk-2310/vara/internal/repository"
)

// identRepo makes a repository with a repository-level identity, so commits can be made.
func identRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if r, err := filepath.EvalSymlinks(root); err == nil {
		root = r
	}
	initRepo(t, root)
	if err := devidentity.SetRepo(filepath.Join(root, repository.VaraDir), devidentity.Identity{Name: "T", Email: "t@example.com"}); err != nil {
		t.Fatal(err)
	}
	return root
}

func headOf(t *testing.T, root string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, repository.VaraDir, "refs", "heads", "main"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// Committing again with nothing new is almost always a slip; it must not add a duplicate commit.
func TestCommitWithNothingNewIsRefused(t *testing.T) {
	root := identRepo(t)
	chdir(t, root)
	writeWorktree(t, root, "a.txt", "a")
	ctx := ctxFor(t, root)
	if err := RunAdd(ctx, []string{"."}); err != nil {
		t.Fatal(err)
	}
	if _, err := RunCommit(ctx, "first"); err != nil {
		t.Fatal(err)
	}
	before := headOf(t, root)
	_, err := RunCommit(ctx, "again")
	if err == nil || !strings.Contains(err.Error(), "nothing to commit") {
		t.Fatalf("want a 'nothing to commit' error, got %v", err)
	}
	if headOf(t, root) != before {
		t.Fatal("a refused commit must not move the branch")
	}
}

func TestFirstCommitWithNothingStagedIsRefused(t *testing.T) {
	root := identRepo(t)
	chdir(t, root)
	_, err := RunCommit(ctxFor(t, root), "empty start")
	if err == nil || !strings.Contains(err.Error(), "nothing to commit") {
		t.Fatalf("want a 'nothing to commit' error, got %v", err)
	}
}

func TestAllowEmptyRecordsAnEmptyCommit(t *testing.T) {
	root := identRepo(t)
	chdir(t, root)
	writeWorktree(t, root, "a.txt", "a")
	ctx := ctxFor(t, root)
	if err := RunAdd(ctx, []string{"."}); err != nil {
		t.Fatal(err)
	}
	if _, err := RunCommit(ctx, "first"); err != nil {
		t.Fatal(err)
	}
	before := headOf(t, root)
	if _, err := RunCommitWith(ctx, "marker", CommitOptions{AllowEmpty: true}); err != nil {
		t.Fatalf("--allow-empty must record the commit, got %v", err)
	}
	if headOf(t, root) == before {
		t.Fatal("the empty commit should have moved the branch")
	}
}

func TestACommitWithRealChangesStillWorks(t *testing.T) {
	root := identRepo(t)
	chdir(t, root)
	writeWorktree(t, root, "a.txt", "a")
	ctx := ctxFor(t, root)
	_ = RunAdd(ctx, []string{"."})
	if _, err := RunCommit(ctx, "one"); err != nil {
		t.Fatal(err)
	}
	writeWorktree(t, root, "a.txt", "changed")
	_ = RunAdd(ctx, []string{"."})
	if _, err := RunCommit(ctx, "two"); err != nil {
		t.Fatalf("a commit with a real change must work, got %v", err)
	}
}
