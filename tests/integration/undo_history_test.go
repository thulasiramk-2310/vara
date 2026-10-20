package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thulasiramk-2310/vara/internal/commands"
	"github.com/thulasiramk-2310/vara/pkg/refs"
)

// threeCommits makes commits v1, v2 and v3 of f.txt on main.
func threeCommits(t *testing.T) (*commands.Context, string) {
	t.Helper()
	ctx, dir := setupRepo(t)
	for _, v := range []string{"v1", "v2", "v3"} {
		writeFile(t, dir, "f.txt", v+"\n")
		makeCommit(t, ctx, v)
	}
	return reloadCtx(t, ctx), dir
}

func undoOK(t *testing.T, ctx *commands.Context) *commands.Context {
	t.Helper()
	if _, err := commands.RunUndo(ctx, false); err != nil {
		t.Fatalf("undo: %v", err)
	}
	return reloadCtx(t, ctx)
}

// TestUndoStepsBackOneCommitAtATime: each undo reverses one more commit, and
// the walk stops at the first commit instead of repeating the last step.
func TestUndoStepsBackOneCommitAtATime(t *testing.T) {
	ctx, dir := threeCommits(t)
	ctx = undoOK(t, ctx)
	if got := readFile(dir, "f.txt"); got != "v2\n" {
		t.Fatalf("after one undo f.txt = %q, want v2", got)
	}
	ctx = undoOK(t, ctx)
	if got := readFile(dir, "f.txt"); got != "v1\n" {
		t.Fatalf("after two undos f.txt = %q, want v1", got)
	}
	_, err := commands.RunUndo(ctx, false)
	if err == nil || !strings.Contains(err.Error(), "nothing earlier to undo") {
		t.Fatalf("undo past the first commit: err = %v", err)
	}
	if got := readFile(dir, "f.txt"); got != "v1\n" {
		t.Fatalf("refused undo changed f.txt to %q", got)
	}
}

// TestUndoAfterNewCommitReversesTheNewCommitFirst: a commit made after an undo
// is the next thing undone, then the walk continues below the earlier undo.
func TestUndoAfterNewCommitReversesTheNewCommitFirst(t *testing.T) {
	ctx, dir := threeCommits(t)
	ctx = undoOK(t, ctx) // v3 undone
	writeFile(t, dir, "f.txt", "v4\n")
	makeCommit(t, ctx, "v4")
	ctx = undoOK(t, reloadCtx(t, ctx))
	if got := readFile(dir, "f.txt"); got != "v2\n" {
		t.Fatalf("undoing v4: f.txt = %q, want v2", got)
	}
	undoOK(t, ctx)
	if got := readFile(dir, "f.txt"); got != "v1\n" {
		t.Fatalf("next undo: f.txt = %q, want v1", got)
	}
}

// TestUndoReversesTheCurrentBranchOnly: undo reads the current branch's own
// history, so a switch neither blocks it nor lets it move another branch.
func TestUndoReversesTheCurrentBranchOnly(t *testing.T) {
	ctx, dir := setupRepo(t)
	writeFile(t, dir, "a.txt", "base\n")
	makeCommit(t, ctx, "base")
	if _, err := commands.RunBranch(ctx, "feature"); err != nil {
		t.Fatalf("branch: %v", err)
	}
	if _, err := commands.RunSwitch(ctx, "feature"); err != nil {
		t.Fatalf("switch: %v", err)
	}
	writeFile(t, dir, "a.txt", "feature\n")
	makeCommit(t, ctx, "feature work")
	if _, err := commands.RunSwitch(ctx, "main"); err != nil {
		t.Fatalf("switch main: %v", err)
	}
	writeFile(t, dir, "b.txt", "main\n")
	makeCommit(t, ctx, "main work")
	if _, err := commands.RunSwitch(ctx, "feature"); err != nil {
		t.Fatalf("switch feature: %v", err)
	}
	ctx = reloadCtx(t, ctx)
	r := refs.NewFSResolver(ctx.Repository.VaraDir)
	mainBefore, _ := r.Resolve("refs/heads/main")

	ctx = undoOK(t, ctx)
	if got := readFile(dir, "a.txt"); got != "base\n" {
		t.Fatalf("undo on feature: a.txt = %q, want base", got)
	}
	if after, _ := r.Resolve("refs/heads/main"); after != mainBefore {
		t.Fatalf("undo on feature moved main from %s to %s", mainBefore, after)
	}
	if _, err := commands.RunSwitch(ctx, "main"); err != nil {
		t.Fatalf("switch back to main: %v", err)
	}
	undoOK(t, reloadCtx(t, ctx))
	if got := readFile(dir, "b.txt"); got != "" {
		t.Fatalf("undo on main kept b.txt = %q", got)
	}
	if got := readFile(dir, "a.txt"); got != "base\n" {
		t.Fatalf("undo on main: a.txt = %q", got)
	}
}

// TestUndoRefusesWhenHeadMovedWithoutARecord: if HEAD is not where the last
// reflog entry left it, undo can't know what it would discard, so it stops.
func TestUndoRefusesWhenHeadMovedWithoutARecord(t *testing.T) {
	ctx, dir := threeCommits(t)
	logPath := filepath.Join(ctx.Repository.VaraDir, "logs", "refs", "heads", "main")
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read reflog: %v", err)
	}
	lines := strings.SplitAfter(strings.TrimSuffix(string(data), "\n"), "\n")
	// Drop the v3 entry, as a commit from VARA 0.4.3 or earlier would be missing.
	if err := os.WriteFile(logPath, []byte(strings.Join(lines[:len(lines)-1], "")), 0644); err != nil {
		t.Fatalf("write reflog: %v", err)
	}
	_, err = commands.RunUndo(ctx, false)
	if err == nil || !strings.Contains(err.Error(), "can't tell what the last change was") {
		t.Fatalf("undo with a gap in the reflog: err = %v", err)
	}
	if got := readFile(dir, "f.txt"); got != "v3\n" {
		t.Fatalf("refused undo changed f.txt to %q", got)
	}
}

// TestUndoWithUnrecordedCommitsRestoresNothing: a branch whose commits predate
// the reflog must not fall back to restoring an unrelated snapshot.
func TestUndoWithUnrecordedCommitsRestoresNothing(t *testing.T) {
	ctx, dir := cleanMerge(t)
	logs := filepath.Join(ctx.Repository.VaraDir, "logs")
	if err := os.RemoveAll(logs); err != nil {
		t.Fatalf("remove reflogs: %v", err)
	}
	before := readFile(dir, "a.txt")
	_, err := commands.RunUndo(ctx, false)
	if err == nil || !strings.Contains(err.Error(), "no recorded change") {
		t.Fatalf("undo without reflogs: err = %v", err)
	}
	if got := readFile(dir, "a.txt"); got != before {
		t.Fatalf("refused undo changed a.txt from %q to %q", before, got)
	}
}
