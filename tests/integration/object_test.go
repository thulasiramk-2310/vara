package integration

import (
	"testing"
	"time"

	"github.com/thulasiramk-2310/vara/pkg/object"
	"github.com/thulasiramk-2310/vara/pkg/types"
)

func TestObjectRoundTrip(t *testing.T) {
	tempDir := t.TempDir()
	store := object.NewStore(tempDir)

	// 1. Test Blob
	blob := object.NewBlob([]byte("hello world"))
	h, err := store.Write(blob)
	if err != nil {
		t.Fatalf("Failed to write blob: %v", err)
	}

	loadedObj, err := store.Read(h)
	if err != nil {
		t.Fatalf("Failed to read blob: %v", err)
	}
	loadedBlob, ok := loadedObj.(*object.Blob)
	if !ok {
		t.Fatalf("Expected Blob, got %T", loadedObj)
	}
	if string(loadedBlob.Data) != "hello world" {
		t.Fatalf("Blob data mismatch, expected 'hello world', got %q", string(loadedBlob.Data))
	}

	// 2. Test Tree
	tree := object.NewTree([]object.TreeEntry{
		{Mode: 100644, Hash: h, Name: "file.txt"},
	})
	th, err := store.Write(tree)
	if err != nil {
		t.Fatalf("Failed to write tree: %v", err)
	}

	loadedTreeObj, err := store.Read(th)
	if err != nil {
		t.Fatalf("Failed to read tree: %v", err)
	}
	loadedTree, ok := loadedTreeObj.(*object.Tree)
	if !ok {
		t.Fatalf("Expected Tree, got %T", loadedTreeObj)
	}
	if len(loadedTree.Entries) != 1 || loadedTree.Entries[0].Name != "file.txt" {
		t.Fatalf("Tree entries mismatch")
	}

	// 3. Test Commit
	commit := &object.Commit{
		TreeHash:  types.TreeID(th),
		Parents:   []types.CommitID{},
		Author:    "Test Author",
		Message:   "Initial commit",
		Timestamp: time.Now().Unix(),
	}
	ch, err := store.Write(commit)
	if err != nil {
		t.Fatalf("Failed to write commit: %v", err)
	}

	loadedCommitObj, err := store.Read(ch)
	if err != nil {
		t.Fatalf("Failed to read commit: %v", err)
	}
	loadedCommit, ok := loadedCommitObj.(*object.Commit)
	if !ok {
		t.Fatalf("Expected Commit, got %T", loadedCommitObj)
	}
	if loadedCommit.Author != "Test Author" || loadedCommit.Message != "Initial commit" {
		t.Fatalf("Commit mismatch")
	}
}
