package transfer

import (
	"bytes"
	"testing"

	"github.com/thulasiramk-2310/vara/pkg/object"
	"github.com/thulasiramk-2310/vara/pkg/types"
)

// helpers ---------------------------------------------------------------

func newStore(t *testing.T) *object.Store {
	t.Helper()
	return object.NewStore(t.TempDir())
}

func writeBlob(t *testing.T, s *object.Store, data string) types.ObjectID {
	t.Helper()
	id, err := s.Write(object.NewBlob([]byte(data)))
	if err != nil {
		t.Fatalf("write blob: %v", err)
	}
	return id
}

func writeTree(t *testing.T, s *object.Store, entries ...object.TreeEntry) types.ObjectID {
	t.Helper()
	id, err := s.Write(object.NewTree(entries))
	if err != nil {
		t.Fatalf("write tree: %v", err)
	}
	return id
}

func writeCommit(t *testing.T, s *object.Store, tree types.ObjectID, parents ...types.CommitID) types.CommitID {
	t.Helper()
	c := &object.Commit{
		TreeHash: types.TreeID(tree),
		Parents:  parents,
		Author:   "Test <t@example.com>",
		Message:  "msg",
	}
	id, err := s.Write(c)
	if err != nil {
		t.Fatalf("write commit: %v", err)
	}
	return types.CommitID(id)
}

// A simple two-commit chain: c1 (blob a) -> c2 (blob a + blob b).
func buildChain(t *testing.T, s *object.Store) (c1, c2 types.CommitID) {
	t.Helper()
	a := writeBlob(t, s, "alpha")
	t1 := writeTree(t, s, object.TreeEntry{Mode: 0o100644, Hash: a, Name: "a.txt"})
	c1 = writeCommit(t, s, t1)

	b := writeBlob(t, s, "beta")
	t2 := writeTree(t, s,
		object.TreeEntry{Mode: 0o100644, Hash: a, Name: "a.txt"},
		object.TreeEntry{Mode: 0o100644, Hash: b, Name: "b.txt"},
	)
	c2 = writeCommit(t, s, t2, c1)
	return c1, c2
}

// tests -----------------------------------------------------------------

func TestEnumerateFullClone(t *testing.T) {
	s := newStore(t)
	_, c2 := buildChain(t, s)

	// haves empty → full closure of c2: 2 commits, 2 trees, 2 blobs = 6.
	ids, err := Enumerate(s, []types.CommitID{c2}, nil)
	if err != nil {
		t.Fatalf("enumerate: %v", err)
	}
	if len(ids) != 6 {
		t.Fatalf("full clone closure = %d objects, want 6", len(ids))
	}
}

func TestEnumerateIncremental(t *testing.T) {
	s := newStore(t)
	c1, c2 := buildChain(t, s)

	// haves = c1 → only c2's new objects: commit c2, tree t2, blob b = 3.
	ids, err := Enumerate(s, []types.CommitID{c2}, []types.CommitID{c1})
	if err != nil {
		t.Fatalf("enumerate: %v", err)
	}
	if len(ids) != 3 {
		t.Fatalf("incremental closure = %d objects, want 3 (c2, t2, b)", len(ids))
	}
}

func TestEnumerateDeterministic(t *testing.T) {
	s := newStore(t)
	_, c2 := buildChain(t, s)
	a, _ := Enumerate(s, []types.CommitID{c2}, nil)
	b, _ := Enumerate(s, []types.CommitID{c2}, nil)
	if len(a) != len(b) {
		t.Fatal("nondeterministic length")
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("nondeterministic order at %d", i)
		}
	}
}

func TestPackRoundTrip(t *testing.T) {
	src := newStore(t)
	_, c2 := buildChain(t, src)

	ids, err := Enumerate(src, []types.CommitID{c2}, nil)
	if err != nil {
		t.Fatalf("enumerate: %v", err)
	}

	var buf bytes.Buffer
	if err := WritePack(src, ids, &buf); err != nil {
		t.Fatalf("write pack: %v", err)
	}

	dst := newStore(t)
	res, err := ReadPack(dst, &buf)
	if err != nil {
		t.Fatalf("read pack: %v", err)
	}
	if res.ObjectsRead != 6 || res.ObjectsWritten != 6 {
		t.Fatalf("read=%d written=%d, want 6/6", res.ObjectsRead, res.ObjectsWritten)
	}

	// Destination must now resolve the full closure of c2.
	if _, err := dst.Read(types.ObjectID(c2)); err != nil {
		t.Fatalf("dst missing tip commit: %v", err)
	}
	got, err := Enumerate(dst, []types.CommitID{c2}, nil)
	if err != nil {
		t.Fatalf("re-enumerate on dst: %v", err)
	}
	if len(got) != 6 {
		t.Fatalf("dst closure = %d, want 6 (incomplete transfer)", len(got))
	}
}

func TestPackChecksumRejected(t *testing.T) {
	src := newStore(t)
	_, c2 := buildChain(t, src)
	ids, _ := Enumerate(src, []types.CommitID{c2}, nil)

	var buf bytes.Buffer
	if err := WritePack(src, ids, &buf); err != nil {
		t.Fatalf("write pack: %v", err)
	}
	corrupt := buf.Bytes()
	corrupt[len(corrupt)-1] ^= 0xff // flip a trailer bit

	dst := newStore(t)
	if _, err := ReadPack(dst, bytes.NewReader(corrupt)); err == nil {
		t.Fatal("corrupt pack accepted; expected checksum error")
	}
}

func TestPackIdempotent(t *testing.T) {
	src := newStore(t)
	_, c2 := buildChain(t, src)
	ids, _ := Enumerate(src, []types.CommitID{c2}, nil)

	var buf bytes.Buffer
	WritePack(src, ids, &buf)
	packBytes := buf.Bytes()

	dst := newStore(t)
	if _, err := ReadPack(dst, bytes.NewReader(packBytes)); err != nil {
		t.Fatalf("first ingest: %v", err)
	}
	// Second ingest of the same pack must succeed (idempotent writes).
	if _, err := ReadPack(dst, bytes.NewReader(packBytes)); err != nil {
		t.Fatalf("second ingest should be idempotent: %v", err)
	}
}
