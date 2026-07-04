package gc

import (
	"path/filepath"
	"testing"

	"github.com/thulasiramk-2310/vara/internal/repository"
	"github.com/thulasiramk-2310/vara/pkg/object"
	"github.com/thulasiramk-2310/vara/pkg/refs"
	"github.com/thulasiramk-2310/vara/pkg/types"
)

func setup(t *testing.T) (varaDir string, store *object.Store) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "repo")
	if _, err := repository.Init(root); err != nil {
		t.Fatalf("init: %v", err)
	}
	varaDir = filepath.Join(root, repository.VaraDir)
	return varaDir, object.NewStore(varaDir)
}

// commit builds a one-file commit and points main at it.
func commit(t *testing.T, varaDir string, store *object.Store, content string) types.CommitID {
	t.Helper()
	blob, _ := store.Write(object.NewBlob([]byte(content)))
	tree, _ := store.Write(object.NewTree([]object.TreeEntry{
		{Mode: 0o100644, Hash: blob, Name: "f.txt"},
	}))
	id, err := store.Write(&object.Commit{
		TreeHash: types.TreeID(tree),
		Author:   "T <t@e.com>",
		Message:  content,
	})
	if err != nil {
		t.Fatalf("commit: %v", err)
	}
	cid := types.CommitID(id)
	if err := refs.NewFSResolver(varaDir).Update("refs/heads/main", cid); err != nil {
		t.Fatalf("set main: %v", err)
	}
	return cid
}

func TestCollectRemovesOrphan(t *testing.T) {
	varaDir, store := setup(t)
	commit(t, varaDir, store, "reachable content")

	// An unreferenced blob: on disk, not in any tree/commit.
	orphan, err := store.Write(object.NewBlob([]byte("orphan garbage")))
	if err != nil {
		t.Fatalf("write orphan: %v", err)
	}

	// Dry run: reports the orphan, deletes nothing.
	dry, err := Collect(varaDir, false)
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if dry.Removed != 1 {
		t.Fatalf("dry run Removed = %d, want 1", dry.Removed)
	}
	if _, err := store.Read(orphan); err != nil {
		t.Fatal("dry run must not delete the orphan")
	}

	// Real run: deletes exactly the orphan.
	got, err := Collect(varaDir, true)
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if got.Removed != 1 {
		t.Fatalf("Removed = %d, want 1", got.Removed)
	}
	if _, err := store.Read(orphan); err == nil {
		t.Fatal("orphan should be gone after gc")
	}
}

func TestCollectPreservesReachable(t *testing.T) {
	varaDir, store := setup(t)
	c := commit(t, varaDir, store, "keep me")

	if _, err := Collect(varaDir, true); err != nil {
		t.Fatalf("collect: %v", err)
	}
	// The commit and its whole closure must survive.
	if _, err := store.Read(types.ObjectID(c)); err != nil {
		t.Fatalf("gc deleted a reachable commit: %v", err)
	}
	res, _ := Collect(varaDir, true)
	if res.Removed != 0 {
		t.Fatalf("second gc removed %d objects; store should be clean", res.Removed)
	}
}

func TestCollectEmptyRepo(t *testing.T) {
	varaDir, _ := setup(t)
	res, err := Collect(varaDir, true)
	if err != nil {
		t.Fatalf("gc on empty repo: %v", err)
	}
	if res.Removed != 0 || res.Scanned != 0 {
		t.Fatalf("empty repo gc: %+v", res)
	}
}
