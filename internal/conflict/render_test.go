package conflict

import "testing"

func TestRenderDiff3ShowsBase(t *testing.T) {
	base := []byte("A\nB\nC\n")
	ours := []byte("A\nOURS\nC\n")
	theirs := []byte("A\nTHEIRS\nC\n")

	out, stats := Render(base, ours, theirs, "main", "feature", StyleDiff3)
	want := "A\n<<<<<<< main\nOURS\n||||||| base\nB\n=======\nTHEIRS\n>>>>>>> feature\nC\n"
	if got := string(out); got != want {
		t.Fatalf("diff3 render =\n%q\nwant\n%q", got, want)
	}
	if stats.Remaining != 1 {
		t.Fatalf("expected one conflict, got %+v", stats)
	}
}

func TestRenderZdiff3HoistsSharedLines(t *testing.T) {
	// Both sides share the leading "X" and trailing "Z" inside the conflict region;
	// zdiff3 must hoist them out of the markers, leaving only the divergent middle.
	base := []byte("X\nB\nZ\n")
	ours := []byte("X\nOURS\nZ\n")
	theirs := []byte("X\nTHEIRS\nZ\n")

	out, _ := Render(base, ours, theirs, "main", "feature", StyleZdiff3)
	want := "X\n<<<<<<< main\nOURS\n||||||| base\nB\n=======\nTHEIRS\n>>>>>>> feature\nZ\n"
	if got := string(out); got != want {
		t.Fatalf("zdiff3 render =\n%q\nwant\n%q", got, want)
	}
}

// TestRenderOrderIndependence: swapping ours/theirs must not change WHETHER a
// region conflicts (only which side is which). This is the fix for the engine's
// adjacent-edit order-dependence.
func TestRenderOrderIndependence(t *testing.T) {
	base := []byte("A\nB\nC\n")
	x := []byte("A\nB2\nC\n") // edits line 2
	y := []byte("A\nB\nC2\n") // edits adjacent line 3

	for _, style := range []MarkerStyle{StyleMerge, StyleDiff3, StyleZdiff3} {
		_, s1 := Render(base, x, y, "main", "feature", style)
		_, s2 := Render(base, y, x, "feature", "main", style)
		if s1.Remaining != 0 || s2.Remaining != 0 {
			t.Fatalf("style %d: disjoint adjacent edits must merge clean both ways, got %+v / %+v", style, s1, s2)
		}
	}

	// A genuine overlap conflicts regardless of order, in every style.
	o := []byte("A\nOURS\nC\n")
	th := []byte("A\nTHEIRS\nC\n")
	for _, style := range []MarkerStyle{StyleMerge, StyleDiff3, StyleZdiff3} {
		_, s1 := Render(base, o, th, "main", "feature", style)
		_, s2 := Render(base, th, o, "feature", "main", style)
		if s1.Remaining != 1 || s2.Remaining != 1 {
			t.Fatalf("style %d: genuine overlap must conflict both ways, got %+v / %+v", style, s1, s2)
		}
	}
}

func TestParseMarkerStyle(t *testing.T) {
	cases := map[string]MarkerStyle{"": StyleMerge, "merge": StyleMerge, "diff3": StyleDiff3, "zdiff3": StyleZdiff3}
	for in, want := range cases {
		got, ok := ParseMarkerStyle(in)
		if !ok || got != want {
			t.Fatalf("ParseMarkerStyle(%q) = %v,%v want %v,true", in, got, ok, want)
		}
	}
	if _, ok := ParseMarkerStyle("bogus"); ok {
		t.Fatal("bogus style must not parse")
	}
}
