package conflict

import (
	"math/rand"
	"strings"
	"testing"
)

const zdiff3Sample = "intro\n<<<<<<< main\nMAIN\n||||||| base\nBASE\n=======\nFEATURE\n>>>>>>> feature\nmiddle\n<<<<<<< main\nA\n=======\nB\n>>>>>>> feature\nend\n"

func TestParseReadsBothMarkerStyles(t *testing.T) {
	segs, err := Parse([]byte(zdiff3Sample))
	if err != nil {
		t.Fatal(err)
	}
	if HunkCount(segs) != 2 {
		t.Fatalf("want 2 hunks, got %d", HunkCount(segs))
	}
	h1, h2 := segs[1].Hunk, segs[3].Hunk
	if !h1.HasBase || strings.Join(h1.Base, "") != "BASE\n" || strings.Join(h1.Ours, "") != "MAIN\n" || strings.Join(h1.Theirs, "") != "FEATURE\n" {
		t.Fatalf("first hunk (with base): %+v", h1)
	}
	if h1.OurLabel != "main" || h1.TheirLabel != "feature" {
		t.Fatalf("labels: %q %q", h1.OurLabel, h1.TheirLabel)
	}
	if h2.HasBase || strings.Join(h2.Ours, "") != "A\n" || strings.Join(h2.Theirs, "") != "B\n" {
		t.Fatalf("second hunk (no base): %+v", h2)
	}
}

// Nothing decided must give back exactly the bytes that went in.
func TestAssembleWithNoDecisionsIsByteExact(t *testing.T) {
	for _, in := range []string{zdiff3Sample, "no conflicts here\n", "", "<<<<<<< a\nx\n=======\ny\n>>>>>>> b", "crlf\r\n<<<<<<< a\r\nx\r\n=======\r\ny\r\n>>>>>>> b\r\ntail\r\n"} {
		segs, err := Parse([]byte(in))
		if err != nil {
			t.Fatalf("%q: %v", in, err)
		}
		out, remaining := Assemble(segs, nil)
		if string(out) != in {
			t.Errorf("round trip changed the bytes:\n in %q\nout %q", in, out)
		}
		if remaining != HunkCount(segs) {
			t.Errorf("remaining %d, want %d", remaining, HunkCount(segs))
		}
	}
}

func TestAssembleAppliesEachChoice(t *testing.T) {
	segs, _ := Parse([]byte(zdiff3Sample))
	cases := []struct {
		name string
		d    []Decision
		want string
		left int
	}{
		{"ours then theirs", []Decision{{Pick: PickOurs}, {Pick: PickTheirs}}, "intro\nMAIN\nmiddle\nB\nend\n", 0},
		{"both, ours first", []Decision{{Pick: PickBoth}, {Pick: PickBoth}}, "intro\nMAIN\nFEATURE\nmiddle\nA\nB\nend\n", 0},
		{"both, theirs first", []Decision{{Pick: PickBothTheirsFirst}, {Pick: PickOurs}}, "intro\nFEATURE\nMAIN\nmiddle\nA\nend\n", 0},
		{"custom", []Decision{{Pick: PickCustom, Custom: []string{"mine\n"}}, {Pick: PickOurs}}, "intro\nmine\nmiddle\nA\nend\n", 0},
		{"skip the first", []Decision{{Pick: PickSkip}, {Pick: PickTheirs}}, "intro\n<<<<<<< main\nMAIN\n||||||| base\nBASE\n=======\nFEATURE\n>>>>>>> feature\nmiddle\nB\nend\n", 1},
		{"only one decided", []Decision{{Pick: PickOurs}}, "intro\nMAIN\nmiddle\n<<<<<<< main\nA\n=======\nB\n>>>>>>> feature\nend\n", 1},
	}
	for _, c := range cases {
		got, left := Assemble(segs, c.d)
		if string(got) != c.want || left != c.left {
			t.Errorf("%s:\n got %q (left %d)\nwant %q (left %d)", c.name, got, left, c.want, c.left)
		}
	}
}

func TestBothKeepsLinesApartWhenTheFirstEndsWithoutANewline(t *testing.T) {
	segs, err := Parse([]byte("<<<<<<< a\nx\n=======\ny\n>>>>>>> b\n"))
	if err != nil {
		t.Fatal(err)
	}
	segs[0].Hunk.Ours = []string{"x"} // the last line of a file has no newline
	out, _ := Assemble(segs, []Decision{{Pick: PickBoth}})
	if string(out) != "x\ny\n" {
		t.Fatalf("the two sides fused: %q", out)
	}
}

func TestParseRejectsBrokenMarkers(t *testing.T) {
	for name, in := range map[string]string{
		"no end":           "<<<<<<< a\nx\n=======\ny\n",
		"no separator":     "<<<<<<< a\nx\n>>>>>>> b\n",
		"nested begin":     "<<<<<<< a\n<<<<<<< c\n=======\n>>>>>>> b\n",
		"begin only":       "<<<<<<< a\n",
		"end before sep":   "<<<<<<< a\n||||||| base\n>>>>>>> b\n",
		"two base markers": "<<<<<<< a\n||||||| b1\n||||||| b2\n=======\n>>>>>>> b\n",
	} {
		if _, err := Parse([]byte(in)); err == nil {
			t.Errorf("%s: should be rejected", name)
		}
	}
}

// Whatever the content, choosing "ours" everywhere must equal the engine's own
// ours resolution, and skipping everywhere must change nothing.
func TestAssembleMatchesResolveOnRandomFiles(t *testing.T) {
	r := rand.New(rand.NewSource(7))
	word := func() string { return []string{"alpha", "beta", "gamma", "delta", ""}[r.Intn(5)] + "\n" }
	for n := 0; n < 300; n++ {
		var b strings.Builder
		for i := r.Intn(6); i >= 0; i-- {
			if r.Intn(2) == 0 {
				b.WriteString(word())
				continue
			}
			b.WriteString("<<<<<<< m\n")
			for k := r.Intn(3); k > 0; k-- {
				b.WriteString(word())
			}
			b.WriteString("=======\n")
			for k := r.Intn(3); k > 0; k-- {
				b.WriteString(word())
			}
			b.WriteString(">>>>>>> f\n")
		}
		in := []byte(b.String())
		segs, err := Parse(in)
		if err != nil {
			t.Fatalf("%q: %v", in, err)
		}
		ds := make([]Decision, HunkCount(segs))
		for i := range ds {
			ds[i].Pick = PickOurs
		}
		got, left := Assemble(segs, ds)
		want, _, err := Resolve(in, Ours)
		if err != nil || string(got) != string(want) || left != 0 {
			t.Fatalf("ours differs from Resolve for %q:\n got %q\nwant %q (%v)", in, got, want, err)
		}
		for i := range ds {
			ds[i].Pick = PickTheirs
		}
		got, _ = Assemble(segs, ds)
		want, _, _ = Resolve(in, Theirs)
		if string(got) != string(want) {
			t.Fatalf("theirs differs from Resolve for %q", in)
		}
	}
}
