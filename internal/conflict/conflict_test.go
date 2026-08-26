package conflict

import "testing"

// sample is a file with one real conflict between two context lines, in the
// exact marker form the merge engine (pkg/diff) emits.
const sample = "top\n" +
	"<<<<<<< main\n" +
	"our line\n" +
	"=======\n" +
	"their line\n" +
	">>>>>>> feature\n" +
	"bottom\n"

func TestHasMarkersAndCount(t *testing.T) {
	if !HasMarkers([]byte(sample)) {
		t.Fatal("sample should have markers")
	}
	if n := Count([]byte(sample)); n != 1 {
		t.Fatalf("Count = %d, want 1", n)
	}
	if HasMarkers([]byte("no conflict here\n")) {
		t.Fatal("clean file should have no markers")
	}
}

func TestResolveOurs(t *testing.T) {
	out, stats, err := Resolve([]byte(sample), Ours)
	if err != nil {
		t.Fatal(err)
	}
	want := "top\nour line\nbottom\n"
	if string(out) != want {
		t.Fatalf("ours = %q, want %q", out, want)
	}
	if stats.Total != 1 || stats.Resolved != 1 || stats.Remaining != 0 {
		t.Fatalf("stats = %+v", stats)
	}
}

func TestResolveTheirs(t *testing.T) {
	out, _, err := Resolve([]byte(sample), Theirs)
	if err != nil {
		t.Fatal(err)
	}
	want := "top\ntheir line\nbottom\n"
	if string(out) != want {
		t.Fatalf("theirs = %q, want %q", out, want)
	}
}

func TestResolveUnion(t *testing.T) {
	out, _, err := Resolve([]byte(sample), Union)
	if err != nil {
		t.Fatal(err)
	}
	want := "top\nour line\ntheir line\nbottom\n"
	if string(out) != want {
		t.Fatalf("union = %q, want %q", out, want)
	}
}

func TestResolveAutoLeavesGenuineConflict(t *testing.T) {
	out, stats, err := Resolve([]byte(sample), Auto)
	if err != nil {
		t.Fatal(err)
	}
	// A genuine two-sided conflict is left intact under auto.
	if string(out) != sample {
		t.Fatalf("auto changed a genuine conflict:\n%q", out)
	}
	if stats.Remaining != 1 || stats.Resolved != 0 {
		t.Fatalf("stats = %+v, want 1 remaining", stats)
	}
}

func TestResolveAutoSettlesOneEmptySide(t *testing.T) {
	// Ours added lines, theirs added nothing at this hunk → auto takes ours.
	in := "a\n<<<<<<< main\nnew ours\n=======\n>>>>>>> feature\nb\n"
	out, stats, err := Resolve([]byte(in), Auto)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != "a\nnew ours\nb\n" {
		t.Fatalf("auto one-empty-side = %q", out)
	}
	if stats.Resolved != 1 || stats.Remaining != 0 {
		t.Fatalf("stats = %+v", stats)
	}
}

func TestResolveAutoCollapsesIdenticalSides(t *testing.T) {
	in := "a\n<<<<<<< main\nsame\n=======\nsame\n>>>>>>> feature\nb\n"
	out, stats, err := Resolve([]byte(in), Auto)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != "a\nsame\nb\n" {
		t.Fatalf("auto identical = %q", out)
	}
	if stats.Resolved != 1 {
		t.Fatalf("stats = %+v", stats)
	}
}

func TestResolveMultipleHunks(t *testing.T) {
	in := "<<<<<<< a\nx\n=======\ny\n>>>>>>> b\n" +
		"middle\n" +
		"<<<<<<< a\np\n=======\nq\n>>>>>>> b\n"
	out, stats, err := Resolve([]byte(in), Ours)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != "x\nmiddle\np\n" {
		t.Fatalf("multi ours = %q", out)
	}
	if stats.Total != 2 || stats.Resolved != 2 {
		t.Fatalf("stats = %+v", stats)
	}
}

func TestResolveNoConflictIsIdentity(t *testing.T) {
	in := "just some\nplain text\n"
	out, stats, err := Resolve([]byte(in), Ours)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != in {
		t.Fatalf("clean file changed: %q", out)
	}
	if stats.Total != 0 {
		t.Fatalf("stats = %+v", stats)
	}
}

func TestResolveMalformedErrors(t *testing.T) {
	cases := []string{
		"top\n<<<<<<< a\nours\nbottom\n",                  // no separator, no end
		"top\n<<<<<<< a\nours\n=======\ntheirs\n",         // no end marker
		"top\n<<<<<<< a\n<<<<<<< b\n=======\n>>>>>>> c\n", // nested begin
	}
	for _, in := range cases {
		if _, _, err := Resolve([]byte(in), Ours); err == nil {
			t.Fatalf("expected error for malformed input %q", in)
		}
	}
}

func TestFinalLineWithoutNewline(t *testing.T) {
	in := "a\n<<<<<<< main\nours\n=======\ntheirs\n>>>>>>> feature" // no trailing \n on end marker
	out, _, err := Resolve([]byte(in), Ours)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != "a\nours\n" {
		t.Fatalf("no-trailing-newline = %q", out)
	}
}

func TestParseStrategy(t *testing.T) {
	for _, s := range []string{"ours", "theirs", "union", "auto"} {
		if _, err := ParseStrategy(s); err != nil {
			t.Fatalf("ParseStrategy(%q): %v", s, err)
		}
	}
	if _, err := ParseStrategy("bogus"); err == nil {
		t.Fatal("bogus strategy should error")
	}
}
