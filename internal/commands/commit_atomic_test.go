package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thulasiramk-2310/vara/internal/repository"
)

func stateFiles(t *testing.T, root string) (leftovers []string) {
	t.Helper()
	_ = filepath.Walk(filepath.Join(root, repository.VaraDir), func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && (strings.HasSuffix(p, ".tmp") || strings.HasSuffix(p, ".lock")) {
			leftovers = append(leftovers, p)
		}
		return nil
	})
	return leftovers
}

func commitOne(t *testing.T, root, name, content, msg string) {
	t.Helper()
	writeWorktree(t, root, name, content)
	ctx := ctxFor(t, root)
	if err := RunAdd(ctx, []string{"."}); err != nil {
		t.Fatal(err)
	}
	if _, err := RunCommit(ctx, msg); err != nil {
		t.Fatalf("commit %q: %v", msg, err)
	}
}

// A commit that could not move the branch must not say it succeeded. The failure used to be ignored,
// so the user was told the work was saved when the branch still pointed at the old commit.
func TestCommitReportsAFailedBranchUpdate(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permissions do not stop root")
	}
	root := identRepo(t)
	chdir(t, root)
	commitOne(t, root, "a.txt", "1", "first")
	before := headOf(t, root)

	heads := filepath.Join(root, repository.VaraDir, "refs", "heads")
	if err := os.Chmod(heads, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(heads, 0o755) })

	writeWorktree(t, root, "a.txt", "2")
	ctx := ctxFor(t, root)
	if err := RunAdd(ctx, []string{"."}); err != nil {
		t.Fatal(err)
	}
	if _, err := RunCommit(ctx, "second"); err == nil {
		t.Fatal("a commit that could not update the branch must return an error")
	}
	if headOf(t, root) != before {
		t.Fatal("the branch must be unchanged after the failed commit")
	}
}

func TestCommitLeavesNoTemporaryOrLockFiles(t *testing.T) {
	root := identRepo(t)
	chdir(t, root)
	commitOne(t, root, "a.txt", "1", "first")
	commitOne(t, root, "a.txt", "2", "second")
	if left := stateFiles(t, root); len(left) != 0 {
		t.Fatalf("leftover temporary or lock files: %v", left)
	}
}

// With a detached HEAD the commit moves HEAD itself and leaves every branch alone.
func TestCommitOnADetachedHeadMovesHeadNotTheBranch(t *testing.T) {
	root := identRepo(t)
	chdir(t, root)
	commitOne(t, root, "a.txt", "1", "first")
	branchBefore := headOf(t, root)
	if err := os.WriteFile(filepath.Join(root, repository.VaraDir, "HEAD"), []byte(branchBefore+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	commitOne(t, root, "a.txt", "2", "on a detached head")

	head, _ := os.ReadFile(filepath.Join(root, repository.VaraDir, "HEAD"))
	if strings.HasPrefix(string(head), "ref:") || strings.TrimSpace(string(head)) == branchBefore {
		t.Fatalf("HEAD should now hold the new commit directly, got %q", head)
	}
	if headOf(t, root) != branchBefore {
		t.Fatal("the branch must not move while HEAD is detached")
	}
}

// A repository with no HEAD file at all starts on main.
func TestCommitWithoutAHeadFileStartsMain(t *testing.T) {
	root := identRepo(t)
	chdir(t, root)
	if err := os.Remove(filepath.Join(root, repository.VaraDir, "HEAD")); err != nil {
		t.Fatal(err)
	}
	commitOne(t, root, "a.txt", "1", "first")
	if headOf(t, root) == "" {
		t.Fatal("refs/heads/main should exist")
	}
	head, _ := os.ReadFile(filepath.Join(root, repository.VaraDir, "HEAD"))
	if strings.TrimSpace(string(head)) != "ref: refs/heads/main" {
		t.Fatalf("HEAD should point at main, got %q", head)
	}
}
