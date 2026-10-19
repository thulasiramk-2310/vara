package integration

import (
	"strings"
	"testing"

	"github.com/thulasiramk-2310/vara/internal/commands"
	"github.com/thulasiramk-2310/vara/pkg/snapshot"
)

// cleanMerge builds main and feature with independent changes and merges them
// without conflict, the state in which an unguarded undo used to destroy edits.
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

// TestUndoRefusesToOverwriteUncommittedWork: undo must not silently replace an
// edited tracked file; --force goes ahead but keeps the edit in a snapshot.
func TestUndoRefusesToOverwriteUncommittedWork(t *testing.T) {
	ctx, dir := cleanMerge(t)
	writeFile(t, dir, "a.txt", "hello\nfeature line\nMY UNSAVED WORK\n")

	_, err := commands.RunUndo(ctx, false)
	if err == nil || !strings.Contains(err.Error(), "uncommitted changes") {
		t.Fatalf("undo with an edited file: err = %v, want a refusal", err)
	}
	if !strings.Contains(err.Error(), "a.txt") || strings.Contains(err.Error(), "b.txt") {
		t.Fatalf("refusal should name only the edited file: %v", err)
	}
	if got := readFile(dir, "a.txt"); !strings.Contains(got, "MY UNSAVED WORK") {
		t.Fatalf("refused undo still changed a.txt: %q", got)
	}

	if _, err := commands.RunUndo(reloadCtx(t, ctx), true); err != nil {
		t.Fatalf("undo --force: %v", err)
	}
	if got := readFile(dir, "a.txt"); got != "hello\n" {
		t.Fatalf("after undo --force a.txt = %q, want the pre-merge content", got)
	}
	snaps, err := snapshot.List(ctx.Repository.VaraDir)
	if err != nil {
		t.Fatalf("list snapshots: %v", err)
	}
	found := false
	for _, s := range snaps {
		if strings.Contains(s, "-undo-") {
			found = true
		}
	}
	if !found {
		t.Fatalf("undo --force saved no snapshot of the edited work: %v", snaps)
	}
}

// TestUndoWithCleanTreeNeedsNoForce: the guard only triggers on real edits.
func TestUndoWithCleanTreeNeedsNoForce(t *testing.T) {
	ctx, dir := cleanMerge(t)
	if _, err := commands.RunUndo(ctx, false); err != nil {
		t.Fatalf("undo on a clean tree: %v", err)
	}
	if got := readFile(dir, "a.txt"); got != "hello\n" {
		t.Fatalf("a.txt = %q", got)
	}
	out, err := commands.RunStatus(reloadCtx(t, ctx))
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if strings.Contains(out, "modified:") {
		t.Fatalf("status after undo reports changes:\n%s", out)
	}
}
