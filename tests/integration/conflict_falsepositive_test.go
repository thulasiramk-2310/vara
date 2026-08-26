package integration

import (
	"testing"

	"github.com/thulasiramk-2310/vara/internal/commands"
	"github.com/thulasiramk-2310/vara/internal/conflict"
	"github.com/thulasiramk-2310/vara/pkg/recovery"
)

// docWithMarkers is a legitimate tracked file whose *content* happens to contain
// conflict-marker lines — e.g. documentation that explains how a merge conflict
// looks (VARA's own internal/conflict/conflict_test.go is exactly this). It is
// NOT a conflicted file; it is identical on both branches.
const docWithMarkers = "How to read a conflict:\n" +
	"<<<<<<< ours\n" +
	"the version on your branch\n" +
	"=======\n" +
	"the version from the other branch\n" +
	">>>>>>> theirs\n" +
	"Pick one and delete the markers.\n"

// TestResolveMustNotTouchNonConflictedMarkerFiles is a regression guard for a
// data-loss defect. The index has no stage 1/2/3 slots (see pkg/index/index.go),
// so an earlier `vara resolve` inferred "conflicted" by scanning every tracked
// file for "<<<<<<<" — and rewrote any file that merely contained marker text,
// even documentation or test fixtures the merge never touched.
//
// The conflict sidecar (.vara/CONFLICTS) now records the exact merge-flagged
// paths, so resolve only touches those. This test asserts a file not involved in
// the merge is left byte-for-byte unchanged.
func TestResolveMustNotTouchNonConflictedMarkerFiles(t *testing.T) {
	ctx, dir := setupRepo(t)

	// Base commit carries a normal file AND a doc that contains marker text.
	writeFile(t, dir, "shared.txt", "line1\nline2\nline3\n")
	writeFile(t, dir, "MERGING.md", docWithMarkers)
	makeCommit(t, ctx, "base")

	// Diverge only shared.txt; MERGING.md is untouched on both sides so it can
	// never be part of the conflict.
	if _, err := commands.RunBranch(ctx, "feature"); err != nil {
		t.Fatalf("branch: %v", err)
	}
	if _, err := commands.RunSwitch(ctx, "feature"); err != nil {
		t.Fatalf("switch feature: %v", err)
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
	if !recovery.MergeInProgress(ctx.Repository.VaraDir) {
		t.Fatal("expected a conflict on shared.txt")
	}

	if _, err := commands.RunResolve(ctx, commands.ResolveArgs{Strategy: conflict.Theirs}); err != nil {
		t.Fatalf("resolve: %v", err)
	}

	// MERGING.md was never in conflict. It must be byte-identical to what we wrote.
	if got := readFile(dir, "MERGING.md"); got != docWithMarkers {
		t.Fatalf("`vara resolve` corrupted a file that was not in conflict.\n"+
			" want (unchanged) = %q\n got (mangled)   = %q", docWithMarkers, got)
	}
}
