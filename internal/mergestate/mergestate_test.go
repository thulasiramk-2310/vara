package mergestate

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteReadClearRoundTrip(t *testing.T) {
	dir := t.TempDir()

	if _, ok, err := Read(dir); err != nil || ok {
		t.Fatalf("Read of absent sidecar: ok=%v err=%v, want ok=false err=nil", ok, err)
	}

	in := &State{
		OurLabel:   "main",
		TheirLabel: "feature",
		Entries: []Entry{
			{
				Path:   "a.txt",
				Kind:   KindContent,
				Base:   Side{Present: true, Blob: "aa"},
				Ours:   Side{Present: true, Blob: "bb"},
				Theirs: Side{Present: true, Blob: "cc"},
			},
			{
				Path:   "gone.txt",
				Kind:   KindModifyDelete,
				Base:   Side{Present: true, Blob: "dd"},
				Ours:   Side{Present: true, Blob: "ee"},
				Theirs: Side{Present: false}, // deleted by them — distinct from empty
			},
		},
	}
	if err := Write(dir, in); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if in.Version != Version {
		t.Fatalf("Write should stamp Version, got %d", in.Version)
	}

	got, ok, err := Read(dir)
	if err != nil || !ok {
		t.Fatalf("Read after Write: ok=%v err=%v", ok, err)
	}
	if got.OurLabel != "main" || got.TheirLabel != "feature" || len(got.Entries) != 2 {
		t.Fatalf("round-trip mismatch: %+v", got)
	}

	// Absence must survive the round-trip as Present=false, NOT as an empty blob.
	md, ok := got.Lookup("gone.txt")
	if !ok {
		t.Fatal("gone.txt missing after round-trip")
	}
	if md.Kind != KindModifyDelete {
		t.Fatalf("kind = %q, want modify_delete", md.Kind)
	}
	if md.Theirs.Present {
		t.Fatal("theirs should be absent (Present=false) after round-trip")
	}
	if !md.Ours.Present || md.Ours.Blob != "ee" {
		t.Fatalf("ours side corrupted: %+v", md.Ours)
	}

	if paths := got.Paths(); len(paths) != 2 {
		t.Fatalf("Paths() = %v", paths)
	}

	// Resolved tracking.
	if got.AllResolved() {
		t.Fatal("nothing resolved yet")
	}
	if unresolved := got.UnresolvedPaths(); len(unresolved) != 2 {
		t.Fatalf("UnresolvedPaths = %v", unresolved)
	}
	if !got.SetResolved("a.txt", true) {
		t.Fatal("SetResolved should find a.txt")
	}
	if got.SetResolved("missing", true) {
		t.Fatal("SetResolved of missing path should be false")
	}
	if got.AllResolved() {
		t.Fatal("gone.txt still unresolved")
	}
	got.SetResolved("gone.txt", true)
	if !got.AllResolved() {
		t.Fatal("both resolved now")
	}

	if err := Clear(dir); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, FileName)); !os.IsNotExist(err) {
		t.Fatalf("sidecar should be gone after Clear, stat err=%v", err)
	}
	if err := Clear(dir); err != nil {
		t.Fatalf("Clear on absent file should be nil, got %v", err)
	}
}

func TestWriteIsAtomicMode(t *testing.T) {
	dir := t.TempDir()
	if err := Write(dir, &State{Entries: []Entry{{Path: "x", Kind: KindContent}}}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, FileName+".tmp")); !os.IsNotExist(err) {
		t.Fatalf("temp file should not remain: %v", err)
	}
}
