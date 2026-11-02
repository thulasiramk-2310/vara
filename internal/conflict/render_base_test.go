package conflict

import "testing"

// A conflict that reaches the end of the file used to lose its base: a replacement
// at the end was found as a delete plus a separate insert, so the ||||||| section
// came out empty even though the common ancestor had a line there.
func TestWholeFileConflictKeepsItsBase(t *testing.T) {
	cases := []struct {
		name               string
		base, ours, theirs string
		style              MarkerStyle
		want               string
	}{
		{"zdiff3, one line", "BASE\n", "MAIN\n", "FEATURE\n", StyleZdiff3,
			"<<<<<<< m\nMAIN\n||||||| base\nBASE\n=======\nFEATURE\n>>>>>>> f\n"},
		{"diff3, one line", "BASE\n", "MAIN\n", "FEATURE\n", StyleDiff3,
			"<<<<<<< m\nMAIN\n||||||| base\nBASE\n=======\nFEATURE\n>>>>>>> f\n"},
		{"last line of a longer file", "a\nBASE\n", "a\nMAIN\n", "a\nFEATURE\n", StyleZdiff3,
			"a\n<<<<<<< m\nMAIN\n||||||| base\nBASE\n=======\nFEATURE\n>>>>>>> f\n"},
		{"several lines at the end", "a\nb1\nb2\n", "a\nm1\nm2\n", "a\nf1\nf2\n", StyleDiff3,
			"a\n<<<<<<< m\nm1\nm2\n||||||| base\nb1\nb2\n=======\nf1\nf2\n>>>>>>> f\n"},
		{"middle of a file (always worked)", "a\nBASE\nz\n", "a\nMAIN\nz\n", "a\nFEATURE\nz\n", StyleZdiff3,
			"a\n<<<<<<< m\nMAIN\n||||||| base\nBASE\n=======\nFEATURE\n>>>>>>> f\nz\n"},
	}
	for _, c := range cases {
		got, _ := Render([]byte(c.base), []byte(c.ours), []byte(c.theirs), "m", "f", c.style)
		if string(got) != c.want {
			t.Errorf("%s:\n got %q\nwant %q", c.name, got, c.want)
		}
	}
}

// Replacing the ending on one side only, or both sides agreeing, is not a conflict.
func TestEndOfFileEditsStillMergeCleanly(t *testing.T) {
	cases := []struct{ name, base, ours, theirs, want string }{
		{"only one side edits the last line", "a\nb\n", "a\nB\n", "a\nb\n", "a\nB\n"},
		{"both make the same change", "a\nb\n", "a\nX\n", "a\nX\n", "a\nX\n"},
		{"disjoint: ours edits the top, theirs the last line", "a\nb\nc\nd\n", "A\nb\nc\nd\n", "a\nb\nc\nD\n", "A\nb\nc\nD\n"},
	}
	for _, c := range cases {
		got, st := Render([]byte(c.base), []byte(c.ours), []byte(c.theirs), "m", "f", StyleZdiff3)
		if string(got) != c.want || st.Remaining != 0 {
			t.Errorf("%s: got %q (unresolved %d), want %q", c.name, got, st.Remaining, c.want)
		}
	}
}
