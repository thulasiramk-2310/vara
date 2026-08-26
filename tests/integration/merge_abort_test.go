package integration

import (
	"strings"
	"testing"

	"github.com/thulasiramk-2310/vara/internal/commands"
	"github.com/thulasiramk-2310/vara/internal/conflict"
	"github.com/thulasiramk-2310/vara/internal/mergestate"
	"github.com/thulasiramk-2310/vara/pkg/recovery"
)

// TestResolveIsReRunnableAcrossStrategies proves the sidecar makes resolve
// idempotent: after --theirs, running --ours recovers our original side. With the
// old path-list/marker model this was impossible — the first run destroyed the
// other side.
func TestResolveIsReRunnableAcrossStrategies(t *testing.T) {
	ctx, dir := makeConflict(t)

	if _, err := commands.RunResolve(ctx, commands.ResolveArgs{Strategy: conflict.Theirs}); err != nil {
		t.Fatalf("resolve theirs: %v", err)
	}
	if got := readFile(dir, "shared.txt"); got != "line1\nFEATURE\nline3\n" {
		t.Fatalf("after --theirs = %q", got)
	}

	// Change our mind: the original sides must still be recoverable.
	ctx = reloadCtx(t, ctx)
	if _, err := commands.RunResolve(ctx, commands.ResolveArgs{Strategy: conflict.Ours}); err != nil {
		t.Fatalf("resolve ours: %v", err)
	}
	if got := readFile(dir, "shared.txt"); got != "line1\nMAIN\nline3\n" {
		t.Fatalf("after --ours (re-run) = %q; the original 'ours' side was lost", got)
	}
}

// TestMergeAbortRestoresPreMergeStateExactly is the headline abort flow: merge →
// conflict → partial resolve → abort → the tree is byte-identical to pre-merge.
func TestMergeAbortRestoresPreMergeStateExactly(t *testing.T) {
	ctx, dir := setupRepo(t)

	writeFile(t, dir, "shared.txt", "line1\nline2\nline3\n")
	writeFile(t, dir, "keep.txt", "untouched\n")
	makeCommit(t, ctx, "base")

	if _, err := commands.RunBranch(ctx, "feature"); err != nil {
		t.Fatalf("branch: %v", err)
	}
	if _, err := commands.RunSwitch(ctx, "feature"); err != nil {
		t.Fatalf("switch: %v", err)
	}
	writeFile(t, dir, "shared.txt", "line1\nFEATURE\nline3\n")
	makeCommit(t, ctx, "feature change")

	if _, err := commands.RunSwitch(ctx, "main"); err != nil {
		t.Fatalf("switch main: %v", err)
	}
	writeFile(t, dir, "shared.txt", "line1\nMAIN\nline3\n")
	makeCommit(t, ctx, "main change")

	// Pre-merge snapshot of tracked content.
	preShared := readFile(dir, "shared.txt")
	preKeep := readFile(dir, "keep.txt")

	if _, err := commands.RunMerge(ctx, "feature"); err != nil {
		t.Fatalf("merge: %v", err)
	}
	if !recovery.MergeInProgress(ctx.Repository.VaraDir) {
		t.Fatal("expected conflict")
	}
	if strings.Contains(readFile(dir, "shared.txt"), "<<<<<<<") == false {
		t.Fatal("expected markers in shared.txt after conflict")
	}

	// Partially resolve, then abort.
	ctx = reloadCtx(t, ctx)
	if _, err := commands.RunResolve(ctx, commands.ResolveArgs{Strategy: conflict.Theirs}); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	ctx = reloadCtx(t, ctx)
	out, err := commands.RunMergeAbort(ctx)
	if err != nil {
		t.Fatalf("abort: %v", err)
	}
	if !strings.Contains(out, "Merge aborted") {
		t.Fatalf("abort output: %q", out)
	}

	// Invariant: tracked files byte-identical to pre-merge; markers gone.
	if got := readFile(dir, "shared.txt"); got != preShared {
		t.Fatalf("shared.txt not restored: got %q want %q", got, preShared)
	}
	if got := readFile(dir, "keep.txt"); got != preKeep {
		t.Fatalf("keep.txt changed: got %q want %q", got, preKeep)
	}
	if recovery.MergeInProgress(ctx.Repository.VaraDir) {
		t.Fatal("MERGE_HEAD should be cleared after abort")
	}
	if _, ok, _ := mergestate.Read(ctx.Repository.VaraDir); ok {
		t.Fatal("conflict sidecar should be cleared after abort")
	}
}

// TestMergeAbortRefusesToClobberUnrelatedEdit proves abort will not silently
// discard edits the user made, after the conflict, to a file the merge never
// touched. It must refuse and name the file.
func TestMergeAbortRefusesToClobberUnrelatedEdit(t *testing.T) {
	ctx, dir := setupRepo(t)

	writeFile(t, dir, "shared.txt", "line1\nline2\nline3\n")
	writeFile(t, dir, "notes.txt", "original notes\n")
	makeCommit(t, ctx, "base")

	if _, err := commands.RunBranch(ctx, "feature"); err != nil {
		t.Fatalf("branch: %v", err)
	}
	if _, err := commands.RunSwitch(ctx, "feature"); err != nil {
		t.Fatalf("switch: %v", err)
	}
	writeFile(t, dir, "shared.txt", "line1\nFEATURE\nline3\n")
	makeCommit(t, ctx, "feature change")

	if _, err := commands.RunSwitch(ctx, "main"); err != nil {
		t.Fatalf("switch main: %v", err)
	}
	writeFile(t, dir, "shared.txt", "line1\nMAIN\nline3\n")
	makeCommit(t, ctx, "main change")

	if _, err := commands.RunMerge(ctx, "feature"); err != nil {
		t.Fatalf("merge: %v", err)
	}
	ctx = reloadCtx(t, ctx)

	// User edits an unrelated file AFTER the conflict appeared.
	writeFile(t, dir, "notes.txt", "IMPORTANT unsaved work\n")

	_, err := commands.RunMergeAbort(ctx)
	if err == nil {
		t.Fatal("abort should refuse to clobber the unrelated edit")
	}
	if !strings.Contains(err.Error(), "notes.txt") {
		t.Fatalf("error must name the endangered file, got: %v", err)
	}
	// Nothing changed: the merge is still in progress and the edit survives.
	if !recovery.MergeInProgress(ctx.Repository.VaraDir) {
		t.Fatal("refused abort must leave the merge in progress")
	}
	if got := readFile(dir, "notes.txt"); got != "IMPORTANT unsaved work\n" {
		t.Fatalf("refused abort must not touch the file, got %q", got)
	}
}

// TestMergeAbortRefusesToClobberUnrelatedDeletion is the deletion analogue of the
// edit-clobber guard: a user who deletes an unrelated tracked file after the
// conflict must not have it silently resurrected by the restore-to-HEAD. This is
// the case a current-index-based "merge-involved" check would miss, since `vara
// rm` also makes the path differ from HEAD.
func TestMergeAbortRefusesToClobberUnrelatedDeletion(t *testing.T) {
	ctx, _ := setupRepo(t)
	dir := ctx.Repository.RootDir

	writeFile(t, dir, "shared.txt", "line1\nline2\nline3\n")
	writeFile(t, dir, "obsolete.txt", "delete me later\n")
	makeCommit(t, ctx, "base")

	if _, err := commands.RunBranch(ctx, "feature"); err != nil {
		t.Fatalf("branch: %v", err)
	}
	if _, err := commands.RunSwitch(ctx, "feature"); err != nil {
		t.Fatalf("switch: %v", err)
	}
	writeFile(t, dir, "shared.txt", "line1\nFEATURE\nline3\n")
	makeCommit(t, ctx, "feature change")

	if _, err := commands.RunSwitch(ctx, "main"); err != nil {
		t.Fatalf("switch main: %v", err)
	}
	writeFile(t, dir, "shared.txt", "line1\nMAIN\nline3\n")
	makeCommit(t, ctx, "main change")

	if _, err := commands.RunMerge(ctx, "feature"); err != nil {
		t.Fatalf("merge: %v", err)
	}
	ctx = reloadCtx(t, ctx)

	// User deletes an unrelated tracked file via the real command, after the conflict.
	if _, err := commands.RunRm(ctx, []string{"obsolete.txt"}); err != nil {
		t.Fatalf("rm: %v", err)
	}
	ctx = reloadCtx(t, ctx)

	_, err := commands.RunMergeAbort(ctx)
	if err == nil {
		t.Fatal("abort must refuse to resurrect the user's unrelated deletion")
	}
	if !strings.Contains(err.Error(), "obsolete.txt") {
		t.Fatalf("error must name the deleted file, got: %v", err)
	}
	// Nothing changed: still merging, file still gone.
	if !recovery.MergeInProgress(ctx.Repository.VaraDir) {
		t.Fatal("refused abort must leave the merge in progress")
	}
	if readFile(dir, "obsolete.txt") != "" {
		t.Fatal("refused abort must not resurrect the file")
	}
}

// TestMergeAbortWithNoMergeErrors covers the no-op-with-clear-error case.
func TestMergeAbortWithNoMergeErrors(t *testing.T) {
	ctx, dir := setupRepo(t)
	writeFile(t, dir, "a.txt", "hi\n")
	makeCommit(t, ctx, "base")

	if _, err := commands.RunMergeAbort(ctx); err == nil {
		t.Fatal("abort with no merge in progress should error")
	} else if !strings.Contains(err.Error(), "no merge in progress") {
		t.Fatalf("unexpected error: %v", err)
	}
}
