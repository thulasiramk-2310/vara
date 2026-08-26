package integration

import (
	"strings"
	"testing"

	"github.com/thulasiramk-2310/vara/internal/commands"
)

// TestDiffWorkingTreeVsIndex shows unstaged edits.
func TestDiffWorkingTreeVsIndex(t *testing.T) {
	ctx, dir := setupRepo(t)
	writeFile(t, dir, "f.txt", "a\nb\nc\n")
	makeCommit(t, ctx, "base")

	writeFile(t, dir, "f.txt", "a\nB\nc\n")
	ctx = reloadCtx(t, ctx)
	out, err := commands.RunDiff(ctx, commands.DiffArgs{})
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	if !strings.Contains(out, "-b\n") || !strings.Contains(out, "+B\n") {
		t.Fatalf("expected the b→B change, got:\n%s", out)
	}
	if !strings.Contains(out, "@@") {
		t.Fatalf("expected a hunk header, got:\n%s", out)
	}
}

// TestDiffStagedVsHead shows staged additions with --staged.
func TestDiffStagedVsHead(t *testing.T) {
	ctx, dir := setupRepo(t)
	writeFile(t, dir, "f.txt", "x\n")
	makeCommit(t, ctx, "base")

	writeFile(t, dir, "g.txt", "new\n")
	if err := commands.RunAdd(ctx, []string{"g.txt"}); err != nil {
		t.Fatalf("add: %v", err)
	}
	ctx = reloadCtx(t, ctx)
	out, err := commands.RunDiff(ctx, commands.DiffArgs{Staged: true})
	if err != nil {
		t.Fatalf("diff --staged: %v", err)
	}
	if !strings.Contains(out, "g.txt") || !strings.Contains(out, "new file") || !strings.Contains(out, "+new\n") {
		t.Fatalf("expected g.txt as a new staged file, got:\n%s", out)
	}
	// f.txt is unchanged between index and HEAD — must not appear.
	if strings.Contains(out, "f.txt") {
		t.Fatalf("unchanged f.txt must not appear in --staged diff:\n%s", out)
	}
}

// TestDiffCleanIsEmpty: no changes → empty output.
func TestDiffCleanIsEmpty(t *testing.T) {
	ctx, dir := setupRepo(t)
	writeFile(t, dir, "f.txt", "x\n")
	makeCommit(t, ctx, "base")

	ctx = reloadCtx(t, ctx)
	out, err := commands.RunDiff(ctx, commands.DiffArgs{})
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	if out != "" {
		t.Fatalf("clean tree should produce empty diff, got:\n%s", out)
	}
}

// TestDiffConflictAwareShowsOursVsTheirs: during a merge, diff surfaces unmerged
// paths as an ours-vs-theirs diff instead of a marker-laden working-tree diff.
func TestDiffConflictAwareShowsOursVsTheirs(t *testing.T) {
	ctx, _ := makeConflict(t) // shared.txt: ours MAIN vs theirs FEATURE

	ctx = reloadCtx(t, ctx)
	out, err := commands.RunDiff(ctx, commands.DiffArgs{})
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	if !strings.Contains(out, "Unmerged paths (ours vs theirs)") {
		t.Fatalf("expected the unmerged preface, got:\n%s", out)
	}
	if !strings.Contains(out, "shared.txt") || !strings.Contains(out, "both modified") {
		t.Fatalf("expected shared.txt both-modified, got:\n%s", out)
	}
	if !strings.Contains(out, "-MAIN\n") || !strings.Contains(out, "+FEATURE\n") {
		t.Fatalf("expected an ours(MAIN)→theirs(FEATURE) diff, got:\n%s", out)
	}
}
