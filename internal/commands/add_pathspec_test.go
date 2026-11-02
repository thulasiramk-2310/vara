package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// addFixture makes a repository with top.txt and src/main.go (both untracked) and returns its root.
func addFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	initRepo(t, root)
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeWorktree(t, root, "top.txt", "top")
	writeWorktree(t, root, "src/main.go", "main")
	return root
}

func staged(ctx *Context) map[string]bool {
	m := map[string]bool{}
	for _, e := range ctx.Index.Entries {
		m[e.Path] = true
	}
	return m
}

// Arguments are typed relative to where the user is standing, not to the repository root.
func TestAddResolvesNameRelativeToCurrentDirectory(t *testing.T) {
	root := addFixture(t)
	chdir(t, filepath.Join(root, "src"))
	ctx := ctxFor(t, root)
	if err := RunAddFromCwd(ctx, []string{"main.go"}); err != nil {
		t.Fatalf("add main.go from src/: %v", err)
	}
	if got := staged(ctx); !got["src/main.go"] || got["top.txt"] {
		t.Fatalf("want only src/main.go staged, got %v", got)
	}
}

func TestAddDotInASubdirectoryStagesOnlyThatDirectory(t *testing.T) {
	root := addFixture(t)
	chdir(t, filepath.Join(root, "src"))
	ctx := ctxFor(t, root)
	if err := RunAddFromCwd(ctx, []string{"."}); err != nil {
		t.Fatal(err)
	}
	if got := staged(ctx); !got["src/main.go"] || got["top.txt"] {
		t.Fatalf("add . inside src/ must not stage files outside src/, got %v", got)
	}
}

func TestAddDotAtTheRootStagesEverything(t *testing.T) {
	root := addFixture(t)
	chdir(t, root)
	ctx := ctxFor(t, root)
	if err := RunAdd(ctx, []string{"."}); err != nil {
		t.Fatal(err)
	}
	if got := staged(ctx); !got["src/main.go"] || !got["top.txt"] {
		t.Fatalf("add . at the root must stage everything, got %v", got)
	}
}

func TestAddParentRelativePath(t *testing.T) {
	root := addFixture(t)
	chdir(t, filepath.Join(root, "src"))
	ctx := ctxFor(t, root)
	if err := RunAddFromCwd(ctx, []string{"../top.txt"}); err != nil {
		t.Fatal(err)
	}
	if got := staged(ctx); !got["top.txt"] || got["src/main.go"] {
		t.Fatalf("want only top.txt, got %v", got)
	}
}

// A typo must not look like success.
func TestAddMissingPathspecIsAnError(t *testing.T) {
	root := addFixture(t)
	chdir(t, root)
	ctx := ctxFor(t, root)
	err := RunAdd(ctx, []string{"nope.txt"})
	if err == nil || !strings.Contains(err.Error(), "did not match") || !strings.Contains(err.Error(), "nope.txt") {
		t.Fatalf("want a 'did not match' error naming the path, got %v", err)
	}
}

func TestAddOutsideTheRepositoryIsAnError(t *testing.T) {
	root := addFixture(t)
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	chdir(t, root)
	ctx := ctxFor(t, root)
	err := RunAddFromCwd(ctx, []string{outside})
	if err == nil || !strings.Contains(err.Error(), "outside the repository") {
		t.Fatalf("want an 'outside the repository' error, got %v", err)
	}
	if len(staged(ctx)) != 0 {
		t.Fatalf("nothing may be staged, got %v", staged(ctx))
	}
}

// A file that is already staged and unchanged is a real path: naming it again is fine.
func TestAddAnUnchangedTrackedFileIsNotAnError(t *testing.T) {
	root := addFixture(t)
	chdir(t, root)
	ctx := ctxFor(t, root)
	if err := RunAdd(ctx, []string{"top.txt"}); err != nil {
		t.Fatal(err)
	}
	if err := RunAdd(ctx, []string{"top.txt"}); err != nil {
		t.Fatalf("adding an unchanged tracked file again must be fine, got %v", err)
	}
}

// One bad name among good ones stages nothing, so the user does not get half of what they asked for.
func TestAddIsAllOrNothing(t *testing.T) {
	root := addFixture(t)
	chdir(t, root)
	ctx := ctxFor(t, root)
	if err := RunAdd(ctx, []string{"top.txt", "nope.txt"}); err == nil {
		t.Fatal("want an error")
	}
	if len(staged(ctx)) != 0 {
		t.Fatalf("a failed add must stage nothing, got %v", staged(ctx))
	}
}
