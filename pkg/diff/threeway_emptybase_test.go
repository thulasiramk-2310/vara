package diff

import (
	"strings"
	"testing"
	"time"
)

// TestThreeWayMergeEmptyBaseTerminates is a regression guard: an empty base once
// caused ThreeWayMerge to spin forever (zero-width insertion blocks at position 0
// that the merge loop could not advance past). It must terminate and produce a
// sensible result. Run under a watchdog so a regression fails instead of hanging.
func TestThreeWayMergeEmptyBaseTerminates(t *testing.T) {
	done := make(chan struct{})
	var merged []byte
	var conflict bool
	go func() {
		merged, conflict = ThreeWayMerge(nil, []byte("OURS\n"), []byte("THEIRS\n"), "ours", "theirs")
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("ThreeWayMerge did not terminate on empty base")
	}

	if !conflict {
		t.Fatal("differing add/add with empty base should conflict")
	}
	s := string(merged)
	for _, want := range []string{"<<<<<<< ours", "OURS", "=======", "THEIRS", ">>>>>>> theirs"} {
		if !strings.Contains(s, want) {
			t.Fatalf("merged output missing %q:\n%s", want, s)
		}
	}
}

func TestThreeWayMergeEmptyBaseIdenticalSidesMergeClean(t *testing.T) {
	merged, conflict := ThreeWayMerge(nil, []byte("SAME\n"), []byte("SAME\n"), "ours", "theirs")
	if conflict {
		t.Fatal("identical add/add should not conflict")
	}
	if string(merged) != "SAME\n" {
		t.Fatalf("merged = %q, want %q", merged, "SAME\n")
	}
}

// TestThreeWayMergeEmptyBaseFileBothModified covers the engine's own latent hang:
// a previously-empty tracked file modified on both sides has an empty base blob.
func TestThreeWayMergeEmptyBaseFileBothModified(t *testing.T) {
	done := make(chan struct{})
	go func() {
		ThreeWayMerge([]byte{}, []byte("a\nb\n"), []byte("c\nd\n"), "ours", "theirs")
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("ThreeWayMerge hung on empty base file")
	}
}
