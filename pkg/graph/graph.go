// Package graph implements VARA-RFC-0007.
//
// RFC:
// VARA-RFC-0007 Commit Graph and Traversal
//
// Responsibilities:
// - BFS/DFS commit graph traversal (Walker).
// - Merge base (Lowest Common Ancestor) algorithm (RFC-0007 §6.1).
// - Generation number computation for graph algorithms.
//
// This package MUST NOT:
// - Import refs, commands, transaction, locking, or any higher-layer package.
package graph

import (
	"fmt"

	varaerrors "github.com/thulasiramk-2310/vara/internal/errors"
	"github.com/thulasiramk-2310/vara/pkg/object"
	"github.com/thulasiramk-2310/vara/pkg/types"
)

// Walker allows traversing commit history from a set of starting tips.
type Walker struct {
	store   *object.Store
	queue   []types.CommitID
	visited map[types.CommitID]bool
}

// NewWalker creates a new commit graph walker starting from the given tips.
func NewWalker(store *object.Store, tips ...types.CommitID) *Walker {
	w := &Walker{
		store:   store,
		queue:   make([]types.CommitID, 0, len(tips)),
		visited: make(map[types.CommitID]bool),
	}
	
	for _, tip := range tips {
		if !w.visited[tip] {
			w.queue = append(w.queue, tip)
			w.visited[tip] = true
		}
	}
	return w
}

// Next returns the next commit in a simplified BFS traversal.
// Returns an empty CommitID when no more commits are available.
func (w *Walker) Next() (*object.Commit, types.CommitID, error) {
	if len(w.queue) == 0 {
		return nil, types.CommitID{}, nil
	}

	// Dequeue
	currID := w.queue[0]
	w.queue = w.queue[1:]

	obj, err := w.store.Read(types.ObjectID(currID))
	if err != nil {
		return nil, types.CommitID{}, fmt.Errorf("failed to read commit %s: %v", currID.String(), err)
	}

	if obj.Type() != object.TypeCommit {
		return nil, types.CommitID{}, fmt.Errorf("object %s is not a commit", currID.String())
	}

	commit := obj.(*object.Commit)

	// Enqueue parents
	for _, parentID := range commit.Parents {
		if !w.visited[parentID] {
			w.queue = append(w.queue, parentID)
			w.visited[parentID] = true
		}
	}

	return commit, currID, nil
}

// MergeBase returns the best common ancestor of commits a and b
// (RFC-0007 §6.1: "nearest common ancestor — the one with the highest
// Generation Number").
//
// Algorithm:
//  1. Collect all ancestors of a, computing each commit's generation number.
//  2. Find the first ancestor of b (via BFS) that is also an ancestor of a.
//     Because BFS traverses by increasing distance from b, the first match is
//     the most recent common ancestor.
//
// Returns varaerrors.ErrRefNotFound when the two commits share no common ancestor.
func MergeBase(store *object.Store, a, b types.CommitID) (types.CommitID, error) {
	if a == b {
		return a, nil
	}

	// Step 1: collect full ancestor set of a (including a itself).
	ancestorsA := make(map[types.CommitID]bool)
	if err := collectAncestors(store, a, ancestorsA); err != nil {
		return types.CommitID{}, fmt.Errorf("graph.MergeBase: %w", err)
	}

	// Step 2: BFS from b; first hit in ancestorsA is the nearest common ancestor.
	queue := []types.CommitID{b}
	visited := make(map[types.CommitID]bool)
	visited[b] = true

	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]

		if ancestorsA[curr] {
			return curr, nil
		}

		obj, err := store.Read(types.ObjectID(curr))
		if err != nil {
			return types.CommitID{}, fmt.Errorf("graph.MergeBase: read %s: %w", curr.String()[:7], err)
		}
		commit, ok := obj.(*object.Commit)
		if !ok {
			return types.CommitID{}, fmt.Errorf("graph.MergeBase: %s is not a commit", curr.String()[:7])
		}
		for _, parent := range commit.Parents {
			if !visited[parent] {
				visited[parent] = true
				queue = append(queue, parent)
			}
		}
	}

	return types.CommitID{}, varaerrors.ErrRefNotFound
}

// Generation computes the generation number of a commit (RFC-0007 §4):
// root = 0, child = max(parent generations) + 1.
// Results are memoised in the provided cache.
func Generation(store *object.Store, id types.CommitID, cache map[types.CommitID]int) (int, error) {
	if g, ok := cache[id]; ok {
		return g, nil
	}
	obj, err := store.Read(types.ObjectID(id))
	if err != nil {
		return 0, err
	}
	commit, ok := obj.(*object.Commit)
	if !ok {
		return 0, fmt.Errorf("graph.Generation: %s is not a commit", id.String()[:7])
	}
	if len(commit.Parents) == 0 {
		cache[id] = 0
		return 0, nil
	}
	maxParent := 0
	for _, p := range commit.Parents {
		g, err := Generation(store, p, cache)
		if err != nil {
			return 0, err
		}
		if g > maxParent {
			maxParent = g
		}
	}
	gen := maxParent + 1
	cache[id] = gen
	return gen, nil
}

// collectAncestors performs BFS from start, adding every reachable commit
// (including start itself) to ancestors.
func collectAncestors(store *object.Store, start types.CommitID, ancestors map[types.CommitID]bool) error {
	queue := []types.CommitID{start}
	ancestors[start] = true

	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]

		obj, err := store.Read(types.ObjectID(curr))
		if err != nil {
			return fmt.Errorf("read %s: %w", curr.String()[:7], err)
		}
		commit, ok := obj.(*object.Commit)
		if !ok {
			return fmt.Errorf("%s is not a commit", curr.String()[:7])
		}
		for _, parent := range commit.Parents {
			if !ancestors[parent] {
				ancestors[parent] = true
				queue = append(queue, parent)
			}
		}
	}
	return nil
}
