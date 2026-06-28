package integration

import (
	"testing"

	"github.com/thulasiramk-2310/vara/pkg/builder"
	"github.com/thulasiramk-2310/vara/pkg/index"
	"github.com/thulasiramk-2310/vara/pkg/object"
	"github.com/thulasiramk-2310/vara/pkg/types"
)

func TestBuilder_TreeAndCommit(t *testing.T) {
	tmpDir := t.TempDir()
	store := object.NewStore(tmpDir)

	// Create a couple of blobs directly in the store to represent files
	b1 := object.NewBlob([]byte("main content"))
	b1ID, _ := store.Write(b1)
	
	b2 := object.NewBlob([]byte("utils content"))
	b2ID, _ := store.Write(b2)
	
	b3 := object.NewBlob([]byte("readme content"))
	b3ID, _ := store.Write(b3)

	// Create an index referencing these blobs
	idx := index.New()
	idx.Entries = append(idx.Entries, index.Entry{
		Path:     "src/main.go",
		ObjectID: types.BlobID(b1ID),
		State:    index.StateAdded,
	})
	idx.Entries = append(idx.Entries, index.Entry{
		Path:     "src/utils.go",
		ObjectID: types.BlobID(b2ID),
		State:    index.StateModified,
	})
	idx.Entries = append(idx.Entries, index.Entry{
		Path:     "README.md",
		ObjectID: types.BlobID(b3ID),
		State:    index.StateAdded,
	})

	// 1. Build the Tree
	rootTreeID, err := builder.BuildTree(idx, store)
	if err != nil {
		t.Fatalf("BuildTree failed: %v", err)
	}

	// Verify the root tree was written
	obj, err := store.Read(types.ObjectID(rootTreeID))
	if err != nil {
		t.Fatalf("Failed to read root tree: %v", err)
	}
	if obj.Type() != object.TypeTree {
		t.Fatalf("Expected TypeTree, got %v", obj.Type())
	}
	rootTree := obj.(*object.Tree)
	if len(rootTree.Entries) != 2 { // "README.md" and "src"
		t.Fatalf("Expected 2 entries in root tree, got %d", len(rootTree.Entries))
	}
	
	// Check if "src" is a tree
	var srcTreeID types.ObjectID
	for _, e := range rootTree.Entries {
		if e.Name == "src" {
			srcTreeID = e.Hash
		}
	}
	if srcTreeID == (types.ObjectID{}) {
		t.Fatalf("Could not find 'src' directory in root tree")
	}

	// Read 'src' tree
	srcObj, err := store.Read(srcTreeID)
	if err != nil {
		t.Fatalf("Failed to read src tree: %v", err)
	}
	srcTree := srcObj.(*object.Tree)
	if len(srcTree.Entries) != 2 { // main.go and utils.go
		t.Fatalf("Expected 2 entries in src tree, got %d", len(srcTree.Entries))
	}

	// 2. Build the Commit
	author := "Alice <alice@example.com>"
	msg := "Initial commit"
	var parents []types.CommitID

	commitID, err := builder.BuildCommit(store, types.TreeID(rootTreeID), parents, author, msg)
	if err != nil {
		t.Fatalf("BuildCommit failed: %v", err)
	}

	// Verify Commit
	cObj, err := store.Read(types.ObjectID(commitID))
	if err != nil {
		t.Fatalf("Failed to read commit: %v", err)
	}
	if cObj.Type() != object.TypeCommit {
		t.Fatalf("Expected TypeCommit, got %v", cObj.Type())
	}
	commit := cObj.(*object.Commit)
	
	if commit.Author != author {
		t.Errorf("Expected author %q, got %q", author, commit.Author)
	}
	if commit.Message != msg {
		t.Errorf("Expected message %q, got %q", msg, commit.Message)
	}
	if commit.TreeHash != types.TreeID(rootTreeID) {
		t.Errorf("Expected tree hash %v, got %v", rootTreeID, commit.TreeHash)
	}
}
