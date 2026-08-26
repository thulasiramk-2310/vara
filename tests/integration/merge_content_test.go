package integration

import (
	"strings"
	"testing"

	"github.com/thulasiramk-2310/vara/internal/commands"
	"github.com/thulasiramk-2310/vara/internal/conflict"
	"github.com/thulasiramk-2310/vara/internal/mergestate"
)

// setupDisjointAdjacent builds base A/B/C, a feature branch that edits line 3
// (C2), and main that edits the adjacent line 2 (B2). Both branches exist on
// return; the working tree is left on `mergeInto` having merged the other branch.
func setupDisjointAdjacent(t *testing.T, mergeInto, mergeFrom string) (string, *commands.Context, string) {
	t.Helper()
	ctx, dir := setupRepo(t)
	writeFile(t, dir, "code.txt", "A\nB\nC\n")
	makeCommit(t, ctx, "base")

	if _, err := commands.RunBranch(ctx, "feature"); err != nil {
		t.Fatalf("branch: %v", err)
	}
	if _, err := commands.RunSwitch(ctx, "feature"); err != nil {
		t.Fatalf("switch feature: %v", err)
	}
	writeFile(t, dir, "code.txt", "A\nB\nC2\n") // feature edits line 3
	makeCommit(t, ctx, "feature edits C")

	if _, err := commands.RunSwitch(ctx, "main"); err != nil {
		t.Fatalf("switch main: %v", err)
	}
	writeFile(t, dir, "code.txt", "A\nB2\nC\n") // main edits adjacent line 2
	makeCommit(t, ctx, "main edits B")

	if _, err := commands.RunSwitch(ctx, mergeInto); err != nil {
		t.Fatalf("switch %s: %v", mergeInto, err)
	}
	out, err := commands.RunMerge(ctx, mergeFrom)
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	return out, ctx, dir
}

// TestMergeAutoResolvesDisjointAdjacentEdits: `vara merge` itself now combines
// disjoint-adjacent edits into a clean two-parent commit — no manual resolve — and
// does so regardless of which branch is current (the engine's order-dependence is
// gone). Both directions must auto-complete to A/B2/C2.
func TestMergeAutoResolvesDisjointAdjacentEdits(t *testing.T) {
	// Direction 1: on main, merge feature (the ordering that used to conflict) —
	// hits the auto-resolve path.
	out, _, dir := setupDisjointAdjacent(t, "main", "feature")
	if !strings.Contains(out, "Auto-merged all conflicts") {
		t.Fatalf("merge should auto-resolve and complete, got %q", out)
	}
	if strings.Contains(readFile(dir, "code.txt"), "<<<<<<<") {
		t.Fatalf("no markers should remain, got %q", readFile(dir, "code.txt"))
	}
	if got := readFile(dir, "code.txt"); got != "A\nB2\nC2\n" {
		t.Fatalf("auto-merged content = %q, want A/B2/C2", got)
	}

	// Direction 2: on feature, merge main (the ordering that already merged clean) —
	// same final content. Order-independence.
	_, _, dir2 := setupDisjointAdjacent(t, "feature", "main")
	if strings.Contains(readFile(dir2, "code.txt"), "<<<<<<<") {
		t.Fatalf("no markers should remain in the reverse direction, got %q", readFile(dir2, "code.txt"))
	}
	if got := readFile(dir2, "code.txt"); got != "A\nB2\nC2\n" {
		t.Fatalf("reverse-direction merged content = %q, want A/B2/C2", got)
	}
}

// TestMergeWritesZdiff3Markers: a genuine overlap conflict is written to the
// working tree in the default zdiff3 style — a ||||||| base section, with lines
// common to both sides hoisted out of the markers as context.
func TestMergeWritesZdiff3Markers(t *testing.T) {
	ctx, dir := setupRepo(t)
	writeFile(t, dir, "f.txt", "top\nMIDDLE\nbottom\n")
	makeCommit(t, ctx, "base")

	if _, err := commands.RunBranch(ctx, "feature"); err != nil {
		t.Fatalf("branch: %v", err)
	}
	if _, err := commands.RunSwitch(ctx, "feature"); err != nil {
		t.Fatalf("switch: %v", err)
	}
	writeFile(t, dir, "f.txt", "top\nFEATURE\nbottom\n")
	makeCommit(t, ctx, "feature")

	if _, err := commands.RunSwitch(ctx, "main"); err != nil {
		t.Fatalf("switch main: %v", err)
	}
	writeFile(t, dir, "f.txt", "top\nMAIN\nbottom\n")
	makeCommit(t, ctx, "main")

	if _, err := commands.RunMerge(ctx, "feature"); err != nil {
		t.Fatalf("merge: %v", err)
	}
	got := readFile(dir, "f.txt")
	want := "top\n<<<<<<< main\nMAIN\n||||||| base\nMIDDLE\n=======\nFEATURE\n>>>>>>> feature\nbottom\n"
	if got != want {
		t.Fatalf("zdiff3 markers =\n%q\nwant\n%q", got, want)
	}
}

// TestAutoLeavesGenuineOverlapConflict: when both sides change the SAME line
// differently, base-aware auto must still refuse and leave it for an explicit
// choice — the finer merge settles disjoint edits without becoming reckless.
func TestAutoLeavesGenuineOverlapConflict(t *testing.T) {
	ctx, dir := makeConflict(t) // both edit line 2 differently

	ctx = reloadCtx(t, ctx)
	out, err := commands.RunResolve(ctx, commands.ResolveArgs{Strategy: conflict.Auto})
	if err != nil {
		t.Fatalf("resolve --auto: %v", err)
	}
	if !strings.Contains(out, "still have conflicts") {
		t.Fatalf("auto should leave the genuine overlap unresolved, got %q", out)
	}
	if !strings.Contains(readFile(dir, "shared.txt"), "<<<<<<<") {
		t.Fatal("genuine overlap must remain marked")
	}
	// Commit stays refused.
	ctx = reloadCtx(t, ctx)
	if _, err := commands.RunCommit(ctx, "premature"); err == nil {
		t.Fatal("commit must remain refused while a genuine conflict is unresolved")
	}
}

// TestAddAddConflictBlocksCommitAndResolves covers an add/add conflict: the same
// new path is added on both branches with different content. Like modify/delete,
// the engine leaves NO markers (the working tree just holds our version), so this
// exercises the sidecar-as-source-of-truth refusal, then a strategy resolution.
func TestAddAddConflictBlocksCommitAndResolves(t *testing.T) {
	ctx, dir := setupRepo(t)

	writeFile(t, dir, "seed.txt", "seed\n")
	makeCommit(t, ctx, "base")

	if _, err := commands.RunBranch(ctx, "feature"); err != nil {
		t.Fatalf("branch: %v", err)
	}
	if _, err := commands.RunSwitch(ctx, "feature"); err != nil {
		t.Fatalf("switch: %v", err)
	}
	writeFile(t, dir, "new.txt", "FEATURE side\n")
	makeCommit(t, ctx, "add new on feature")

	if _, err := commands.RunSwitch(ctx, "main"); err != nil {
		t.Fatalf("switch main: %v", err)
	}
	writeFile(t, dir, "new.txt", "MAIN side\n")
	makeCommit(t, ctx, "add new on main")

	if _, err := commands.RunMerge(ctx, "feature"); err != nil {
		t.Fatalf("merge: %v", err)
	}
	st, ok, _ := mergestate.Read(ctx.Repository.VaraDir)
	if !ok {
		t.Fatal("sidecar missing")
	}
	e, _ := st.Lookup("new.txt")
	if e.Kind != mergestate.KindContent || e.Base.Present {
		t.Fatalf("add/add should be content with no base: %+v", e)
	}

	// No markers, but commit must still refuse.
	ctx = reloadCtx(t, ctx)
	if _, err := commands.RunCommit(ctx, "premature"); err == nil {
		t.Fatal("commit must refuse an unresolved add/add conflict")
	}

	// Resolve by taking theirs.
	ctx = reloadCtx(t, ctx)
	if _, err := commands.RunResolve(ctx, commands.ResolveArgs{Strategy: conflict.Theirs}); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got := readFile(dir, "new.txt"); got != "FEATURE side\n" {
		t.Fatalf("new.txt = %q", got)
	}
	ctx = reloadCtx(t, ctx)
	if _, err := commands.RunCommit(ctx, "merge feature"); err != nil {
		t.Fatalf("commit after resolve: %v", err)
	}
}

// TestAddMarksHandResolvedConflict verifies the Git-style "add means resolved":
// hand-editing a conflicted file to remove markers and running `vara add` marks
// it resolved so commit succeeds — but adding it WITH markers still present does
// not, preserving VARA's guard against committing markers.
func TestAddMarksHandResolvedConflict(t *testing.T) {
	ctx, dir := makeConflict(t) // edit/edit on shared.txt, markers present

	// Adding while markers remain must NOT mark it resolved.
	if err := commands.RunAdd(ctx, []string{"shared.txt"}); err != nil {
		t.Fatalf("add (with markers): %v", err)
	}
	st, _, _ := mergestate.Read(ctx.Repository.VaraDir)
	if e, _ := st.Lookup("shared.txt"); e.Resolved {
		t.Fatal("adding a file that still has markers must not mark it resolved")
	}
	ctx = reloadCtx(t, ctx)
	if _, err := commands.RunCommit(ctx, "still conflicted"); err == nil {
		t.Fatal("commit must still refuse while markers remain")
	}

	// Hand-resolve: write clean content, add, commit.
	writeFile(t, dir, "shared.txt", "line1\nRECONCILED\nline3\n")
	if err := commands.RunAdd(ctx, []string{"shared.txt"}); err != nil {
		t.Fatalf("add (resolved): %v", err)
	}
	st, _, _ = mergestate.Read(ctx.Repository.VaraDir)
	if e, _ := st.Lookup("shared.txt"); !e.Resolved {
		t.Fatal("adding the marker-free resolution should mark it resolved")
	}
	ctx = reloadCtx(t, ctx)
	if _, err := commands.RunCommit(ctx, "hand-resolved merge"); err != nil {
		t.Fatalf("commit after hand-resolve: %v", err)
	}
}
