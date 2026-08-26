package conflict

import "testing"

func TestFormatUnifiedBasic(t *testing.T) {
	a := []byte("line1\nline2\nline3\nline4\nline5\n")
	b := []byte("line1\nCHANGED\nline3\nline4\nNEW5\n")
	got := string(FormatUnified(a, b, "a/f", "b/f"))
	want := "--- a/f\n+++ b/f\n@@ -1,5 +1,5 @@\n line1\n-line2\n+CHANGED\n line3\n line4\n-line5\n+NEW5\n"
	if got != want {
		t.Fatalf("unified =\n%q\nwant\n%q", got, want)
	}
}

func TestFormatUnifiedIdentical(t *testing.T) {
	a := []byte("x\ny\n")
	if d := FormatUnified(a, a, "a/f", "b/f"); d != nil {
		t.Fatalf("identical inputs must produce no diff, got %q", d)
	}
}

func TestFormatUnifiedNewFile(t *testing.T) {
	got := string(FormatUnified(nil, []byte("only\n"), "a/g", "b/g"))
	want := "--- a/g\n+++ b/g\n@@ -0,0 +1,1 @@\n+only\n"
	if got != want {
		t.Fatalf("new-file unified =\n%q\nwant\n%q", got, want)
	}
}

func TestFormatUnifiedDeletedFile(t *testing.T) {
	got := string(FormatUnified([]byte("a\nb\n"), nil, "a/g", "b/g"))
	want := "--- a/g\n+++ b/g\n@@ -1,2 +0,0 @@\n-a\n-b\n"
	if got != want {
		t.Fatalf("deleted-file unified =\n%q\nwant\n%q", got, want)
	}
}

func TestFormatUnifiedSeparateHunks(t *testing.T) {
	// Two changes more than 2*context apart must become two hunks.
	a := []byte("1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n11\n12\n13\n14\n")
	b := []byte("1\nX\n3\n4\n5\n6\n7\n8\n9\n10\n11\n12\nY\n14\n")
	got := string(FormatUnified(a, b, "a/f", "b/f"))
	// Expect two @@ headers.
	count := 0
	for i := 0; i+2 < len(got); i++ {
		if got[i] == '@' && got[i+1] == '@' {
			count++
		}
	}
	// Each header contributes two "@@" (open and close on the same line).
	if count != 4 {
		t.Fatalf("expected 2 hunk headers (4 '@@'), got %d in:\n%s", count, got)
	}
}
