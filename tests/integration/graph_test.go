package integration

import (
	"testing"
	"time"

	"github.com/thulasiramk-2310/vara/pkg/graph"
	"github.com/thulasiramk-2310/vara/pkg/object"
	"github.com/thulasiramk-2310/vara/pkg/types"
)

func TestWalker_BFS(t *testing.T) {
	tmpDir := t.TempDir()
	store := object.NewStore(tmpDir)

	// Create a dummy tree
	tree := object.NewTree(nil)
	treeID, _ := store.Write(tree)

	// C1 (root)
	c1 := &object.Commit{
		TreeHash:  types.TreeID(treeID),
		Author:    "A",
		Message:   "C1",
		Timestamp: time.Now().Unix(),
	}
	c1ID, _ := store.Write(c1)

	// C2 (child of C1)
	c2 := &object.Commit{
		TreeHash:  types.TreeID(treeID),
		Parents:   []types.CommitID{types.CommitID(c1ID)},
		Author:    "A",
		Message:   "C2",
		Timestamp: time.Now().Unix(),
	}
	c2ID, _ := store.Write(c2)

	// C3 (child of C2)
	c3 := &object.Commit{
		TreeHash:  types.TreeID(treeID),
		Parents:   []types.CommitID{types.CommitID(c2ID)},
		Author:    "A",
		Message:   "C3",
		Timestamp: time.Now().Unix(),
	}
	c3ID, _ := store.Write(c3)

	// Walk from C3
	w := graph.NewWalker(store, types.CommitID(c3ID))

	// Expected order: C3, C2, C1
	expected := []string{"C3", "C2", "C1"}
	var got []string

	for {
		c, _, err := w.Next()
		if err != nil {
			t.Fatalf("Walk error: %v", err)
		}
		if c == nil {
			break
		}
		got = append(got, c.Message)
	}

	if len(got) != len(expected) {
		t.Fatalf("Expected %d commits, got %d", len(expected), len(got))
	}

	for i, msg := range expected {
		if got[i] != msg {
			t.Errorf("Expected commit message %q at index %d, got %q", msg, i, got[i])
		}
	}
}
