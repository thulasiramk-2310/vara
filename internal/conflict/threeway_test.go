package conflict

import "testing"

// TestMergeThreeWayDisjointAdjacentEditsMergeCleanly is the headline base-aware
// win: ours edits line 2, theirs edits the adjacent line 3. The frozen engine
// reports these as one conflict (it lumps abutting blocks); the base-aware merge
// combines them with no conflict.
func TestMergeThreeWayDisjointAdjacentEditsMergeCleanly(t *testing.T) {
	base := []byte("A\nB\nC\n")
	ours := []byte("A\nB2\nC\n")
	theirs := []byte("A\nB\nC2\n")

	out, stats, err := MergeThreeWay(base, ours, theirs, "main", "feature")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(out), "A\nB2\nC2\n"; got != want {
		t.Fatalf("merge = %q, want %q", got, want)
	}
	if stats.Remaining != 0 {
		t.Fatalf("expected a clean merge, got Remaining=%d (%+v)", stats.Remaining, stats)
	}
	if HasMarkers(out) {
		t.Fatalf("clean merge must not contain markers: %q", out)
	}
}

// TestMergeThreeWayNonAdjacentDisjointEdits: edits separated by an untouched line
// also merge cleanly (the simpler, always-clean case).
func TestMergeThreeWayNonAdjacentDisjointEdits(t *testing.T) {
	base := []byte("A\nB\nC\nD\nE\n")
	ours := []byte("A\nB2\nC\nD\nE\n")
	theirs := []byte("A\nB\nC\nD2\nE\n")

	out, stats, err := MergeThreeWay(base, ours, theirs, "main", "feature")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(out), "A\nB2\nC\nD2\nE\n"; got != want {
		t.Fatalf("merge = %q, want %q", got, want)
	}
	if stats.Remaining != 0 {
		t.Fatalf("expected clean, got %+v", stats)
	}
}

// TestMergeThreeWayOverlappingEditsConflict: both sides change the SAME line
// differently — a genuine conflict that must be left as markers.
func TestMergeThreeWayOverlappingEditsConflict(t *testing.T) {
	base := []byte("A\nB\nC\n")
	ours := []byte("A\nOURS\nC\n")
	theirs := []byte("A\nTHEIRS\nC\n")

	out, stats, err := MergeThreeWay(base, ours, theirs, "main", "feature")
	if err != nil {
		t.Fatal(err)
	}
	if stats.Remaining != 1 || stats.Total != 1 {
		t.Fatalf("expected one unresolved conflict, got %+v", stats)
	}
	want := "A\n<<<<<<< main\nOURS\n=======\nTHEIRS\n>>>>>>> feature\nC\n"
	if got := string(out); got != want {
		t.Fatalf("merge = %q, want %q", got, want)
	}
}

// TestMergeThreeWayIdenticalChangeSettles: both sides made the same edit.
func TestMergeThreeWayIdenticalChangeSettles(t *testing.T) {
	base := []byte("A\nB\nC\n")
	ours := []byte("A\nSAME\nC\n")
	theirs := []byte("A\nSAME\nC\n")

	out, stats, err := MergeThreeWay(base, ours, theirs, "main", "feature")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(out), "A\nSAME\nC\n"; got != want {
		t.Fatalf("merge = %q, want %q", got, want)
	}
	if stats.Remaining != 0 {
		t.Fatalf("identical change must settle, got %+v", stats)
	}
}

// TestMergeThreeWayHunkModifyDeleteRefused: ours edits a region, theirs deletes
// it. This hunk-level modify/delete must NOT be auto-picked; it stays a conflict
// (an empty side inside the markers), consistent with the file-level rule.
func TestMergeThreeWayHunkModifyDeleteRefused(t *testing.T) {
	base := []byte("A\nB\nC\nD\n")
	ours := []byte("A\nB-EDIT\nC-EDIT\nD\n")
	theirs := []byte("A\nD\n") // deleted B and C

	out, stats, err := MergeThreeWay(base, ours, theirs, "main", "feature")
	if err != nil {
		t.Fatal(err)
	}
	if stats.Remaining != 1 {
		t.Fatalf("hunk modify/delete must be refused (left conflicted), got %+v\n%s", stats, out)
	}
	if !HasMarkers(out) {
		t.Fatalf("expected markers, got %q", out)
	}
	// The engine's own settle would have silently taken the non-empty side; here
	// the deletion (empty side) must appear as an empty theirs section, not be
	// dropped.
	want := "A\n<<<<<<< main\nB-EDIT\nC-EDIT\n=======\n>>>>>>> feature\nD\n"
	if got := string(out); got != want {
		t.Fatalf("merge = %q, want %q", got, want)
	}
}

// TestMergeThreeWaySameGapInsertionsConflict: both sides insert *different* lines
// at the same base gap. Order is ambiguous, so it must be a conflict, not a
// silently-ordered combination.
func TestMergeThreeWaySameGapInsertionsConflict(t *testing.T) {
	base := []byte("A\nB\n")
	ours := []byte("A\nOURS-INS\nB\n")
	theirs := []byte("A\nTHEIRS-INS\nB\n")

	out, stats, err := MergeThreeWay(base, ours, theirs, "main", "feature")
	if err != nil {
		t.Fatal(err)
	}
	if stats.Remaining != 1 {
		t.Fatalf("same-gap differing insertions must conflict, got %+v\n%s", stats, out)
	}
	want := "A\n<<<<<<< main\nOURS-INS\n=======\nTHEIRS-INS\n>>>>>>> feature\nB\n"
	if got := string(out); got != want {
		t.Fatalf("merge = %q, want %q", got, want)
	}
}

// TestMergeThreeWayIdenticalSameGapInsertion: identical insertions at the same
// gap collapse to one (both made the same change).
func TestMergeThreeWayIdenticalSameGapInsertion(t *testing.T) {
	base := []byte("A\nB\n")
	ours := []byte("A\nINS\nB\n")
	theirs := []byte("A\nINS\nB\n")

	out, _, err := MergeThreeWay(base, ours, theirs, "main", "feature")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(out), "A\nINS\nB\n"; got != want {
		t.Fatalf("merge = %q, want %q", got, want)
	}
}

// TestMergeThreeWayEmptyBaseIdenticalAdds: add/add of identical content merges
// clean; differing content is one whole-file conflict.
func TestMergeThreeWayEmptyBase(t *testing.T) {
	// Identical add/add.
	out, stats, err := MergeThreeWay(nil, []byte("X\nY\n"), []byte("X\nY\n"), "main", "feature")
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != "X\nY\n" || stats.Remaining != 0 {
		t.Fatalf("identical add/add: got %q %+v", out, stats)
	}
	// Differing add/add.
	out, stats, err = MergeThreeWay(nil, []byte("X\n"), []byte("Y\n"), "main", "feature")
	if err != nil {
		t.Fatal(err)
	}
	if stats.Remaining != 1 || !HasMarkers(out) {
		t.Fatalf("differing add/add must conflict: %q %+v", out, stats)
	}
}

// TestMergeThreeWayInsertionAdjacentToEditMergesClean: ours inserts a line right
// before a line theirs edits — disjoint, should merge cleanly.
func TestMergeThreeWayInsertionAdjacentToEditMergesClean(t *testing.T) {
	base := []byte("A\nB\nC\n")
	ours := []byte("A\nNEW\nB\nC\n") // insert NEW before B
	theirs := []byte("A\nB\nC2\n")   // edit C

	out, stats, err := MergeThreeWay(base, ours, theirs, "main", "feature")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(out), "A\nNEW\nB\nC2\n"; got != want {
		t.Fatalf("merge = %q, want %q", got, want)
	}
	if stats.Remaining != 0 {
		t.Fatalf("expected clean, got %+v", stats)
	}
}
