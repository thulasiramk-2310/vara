// Package transfer implements VARA-RFC-0014 §7 (Object Enumeration) and §8
// (Transfer Format). It computes the set of objects one repository must send
// to another and encodes/decodes that set as a VPCK stream.
//
// RFC:
// VARA-RFC-0014 Remote Protocol
//
// Responsibilities:
//   - Enumerate the object closure of a set of commits, minus a boundary.
//   - Encode objects as a framed loose-object stream (VPCK).
//   - Decode a VPCK stream into an object store with per-object verification.
//
// This package MUST NOT:
//   - Perform any I/O with a peer (that is the transport layer's job).
//   - Update references.
package transfer

import (
	"fmt"
	"sort"

	"github.com/thulasiramk-2310/vara/pkg/object"
	"github.com/thulasiramk-2310/vara/pkg/types"
)

// modeSubtree is the tree-entry mode marking a subdirectory (see pkg/builder).
const modeSubtree = 0o040000

// Enumerate returns the object IDs reachable from wants but not reachable from
// haves, per RFC-0014 §7. The result is sorted by hex ID so the produced pack
// is deterministic for a given (wants, haves).
func Enumerate(store *object.Store, wants, haves []types.CommitID) ([]types.ObjectID, error) {
	// Step 1: boundary — every commit reachable from a have. These are commits
	// the receiver already has, so the commit walk stops at them.
	boundary := map[types.CommitID]bool{}

	// haveObjs — trees/blobs the receiver already has. Because objects are
	// content-addressed, if a have tip's tree is present on the receiver then
	// its entire closure is too: skipping at the tree level is complete. We seed
	// this from the have TIPS' trees only (O(current tree), not O(history)),
	// which catches every currently-tracked unchanged file — the common case.
	haveObjs := map[types.ObjectID]bool{}

	for _, h := range haves {
		if err := walkCommits(store, h, boundary, nil); err != nil {
			// A have we cannot read (e.g. a client commit the remote lacks) is
			// treated as unknown: skip it. The receiver just gets more objects.
			continue
		}
		if obj, err := store.Read(types.ObjectID(h)); err == nil {
			if commit, ok := obj.(*object.Commit); ok {
				// Best-effort: ignore errors so a partial have never aborts.
				_ = addTreeClosure(store, types.ObjectID(commit.TreeHash), haveObjs)
			}
		}
	}

	// Step 2: commit walk from wants, stopping at the boundary.
	sendCommits := map[types.CommitID]bool{}
	for _, w := range wants {
		if boundary[w] {
			continue
		}
		if err := walkCommits(store, w, sendCommits, boundary); err != nil {
			return nil, err
		}
	}

	// Step 3: tree/blob closure of every commit we are sending, minus anything
	// the receiver already has (haveObjs).
	objs := map[types.ObjectID]bool{}
	for c := range sendCommits {
		objs[types.ObjectID(c)] = true
		obj, err := store.Read(types.ObjectID(c))
		if err != nil {
			return nil, fmt.Errorf("read commit %s: %w", short(c), err)
		}
		commit, ok := obj.(*object.Commit)
		if !ok {
			return nil, fmt.Errorf("%s is not a commit", short(c))
		}
		if err := addTreeClosureExcluding(store, types.ObjectID(commit.TreeHash), objs, haveObjs); err != nil {
			return nil, err
		}
	}

	out := make([]types.ObjectID, 0, len(objs))
	for id := range objs {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].String() < out[j].String() })
	return out, nil
}

// walkCommits does a BFS over commit ancestry starting at start, recording
// every visited commit in seen. If stop is non-nil, commits in stop are
// neither recorded nor traversed through.
func walkCommits(store *object.Store, start types.CommitID, seen map[types.CommitID]bool, stop map[types.CommitID]bool) error {
	if stop != nil && stop[start] {
		return nil
	}
	if seen[start] {
		return nil
	}
	queue := []types.CommitID{start}
	seen[start] = true
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		obj, err := store.Read(types.ObjectID(cur))
		if err != nil {
			return fmt.Errorf("read commit %s: %w", short(cur), err)
		}
		commit, ok := obj.(*object.Commit)
		if !ok {
			return fmt.Errorf("%s is not a commit", short(cur))
		}
		for _, p := range commit.Parents {
			if stop != nil && stop[p] {
				continue
			}
			if !seen[p] {
				seen[p] = true
				queue = append(queue, p)
			}
		}
	}
	return nil
}

// addTreeClosure adds a tree and every object it transitively references
// (subtrees and blobs) to objs.
func addTreeClosure(store *object.Store, treeID types.ObjectID, objs map[types.ObjectID]bool) error {
	return addTreeClosureExcluding(store, treeID, objs, nil)
}

// addTreeClosureExcluding is addTreeClosure with a set of objects the receiver
// already has. Because a tree ID uniquely determines its entire subtree, a tree
// present in exclude implies its whole closure is present, so we prune there.
func addTreeClosureExcluding(store *object.Store, treeID types.ObjectID, objs, exclude map[types.ObjectID]bool) error {
	if exclude[treeID] || objs[treeID] {
		return nil
	}
	objs[treeID] = true
	obj, err := store.Read(treeID)
	if err != nil {
		return fmt.Errorf("read tree %s: %w", treeID.String()[:7], err)
	}
	tree, ok := obj.(*object.Tree)
	if !ok {
		return fmt.Errorf("%s is not a tree", treeID.String()[:7])
	}
	for _, e := range tree.Entries {
		if e.Mode == modeSubtree {
			if err := addTreeClosureExcluding(store, e.Hash, objs, exclude); err != nil {
				return err
			}
			continue
		}
		if !exclude[e.Hash] {
			objs[e.Hash] = true // blob leaf
		}
	}
	return nil
}

func short(c types.CommitID) string {
	s := c.String()
	if len(s) > 7 {
		return s[:7]
	}
	return s
}
