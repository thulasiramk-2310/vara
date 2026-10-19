package integration

import (
	"strings"
	"testing"

	"github.com/thulasiramk-2310/vara/internal/commands"
)

// cleanMerge builds main and feature with independent changes and merges them
// without conflict.
func cleanMerge(t *testing.T) (*commands.Context, string) {
	t.Helper()
	ctx, dir := setupRepo(t)
	writeFile(t, dir, "a.txt", "hello\n")
	makeCommit(t, ctx, "base")
	if _, err := commands.RunBranch(ctx, "feature"); err != nil {
		t.Fatalf("branch: %v", err)
	}
	if _, err := commands.RunSwitch(ctx, "feature"); err != nil {
		t.Fatalf("switch: %v", err)
	}
	writeFile(t, dir, "a.txt", "hello\nfeature line\n")
	makeCommit(t, ctx, "feature")
	if _, err := commands.RunSwitch(ctx, "main"); err != nil {
		t.Fatalf("switch main: %v", err)
	}
	writeFile(t, dir, "b.txt", "b\n")
	makeCommit(t, ctx, "main b")
	if _, err := commands.RunMerge(ctx, "feature"); err != nil {
		t.Fatalf("merge: %v", err)
	}
	return reloadCtx(t, ctx), dir
}

// TestStatusIsCleanAfterMerge: merge rewrites files with their committed
// content; status must not report them as modified.
func TestStatusIsCleanAfterMerge(t *testing.T) {
	ctx, _ := cleanMerge(t)
	out, err := commands.RunStatus(ctx)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if strings.Contains(out, "modified:") {
		t.Fatalf("status after a clean merge reports changes:\n%s", out)
	}
}
