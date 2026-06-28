// Package diff implements VARA-RFC-0008 §4 (tree and content diffing).
//
// RFC:
// VARA-RFC-0008 Merge Algorithm
//
// Responsibilities:
// - Compute file-level differences between two tree objects (DiffTrees).
// - Compute line-level three-way content merge (ThreeWayMerge).
// - Provide the FileDiff type consumed by the merge engine.
//
// This package MUST NOT:
// - Import refs, commands, transaction, locking, or any higher-layer package.
package diff

import (
	"fmt"
	"sort"

	"github.com/thulasiramk-2310/vara/pkg/object"
	"github.com/thulasiramk-2310/vara/pkg/types"
)

// ChangeType describes how a file changed between two tree states.
type ChangeType uint8

const (
	Added    ChangeType = iota // Present in new tree only.
	Deleted                    // Present in old tree only.
	Modified                   // Present in both, different content.
)

func (c ChangeType) String() string {
	switch c {
	case Added:
		return "A"
	case Deleted:
		return "D"
	case Modified:
		return "M"
	default:
		return " "
	}
}

// FileDiff represents a single file-level change between two trees.
type FileDiff struct {
	Type    ChangeType
	OldPath string
	NewPath string
	OldBlob types.ObjectID // Zero for Added.
	NewBlob types.ObjectID // Zero for Deleted.
}

// Path returns the canonical path for display.
func (f FileDiff) Path() string {
	if f.NewPath != "" {
		return f.NewPath
	}
	return f.OldPath
}

// DiffTrees computes file-level differences between two tree objects.
// Pass a zero types.ObjectID to represent an empty tree
// (e.g. when diffing against the initial commit).
func DiffTrees(store *object.Store, oldTreeID, newTreeID types.ObjectID) ([]FileDiff, error) {
	oldFiles := make(map[string]types.ObjectID)
	newFiles := make(map[string]types.ObjectID)

	if oldTreeID != (types.ObjectID{}) {
		if err := collectFiles(store, oldTreeID, "", oldFiles); err != nil {
			return nil, fmt.Errorf("diff: collect old tree: %w", err)
		}
	}
	if newTreeID != (types.ObjectID{}) {
		if err := collectFiles(store, newTreeID, "", newFiles); err != nil {
			return nil, fmt.Errorf("diff: collect new tree: %w", err)
		}
	}

	var diffs []FileDiff

	for path, oldBlob := range oldFiles {
		if newBlob, ok := newFiles[path]; !ok {
			diffs = append(diffs, FileDiff{Type: Deleted, OldPath: path, OldBlob: oldBlob})
		} else if oldBlob != newBlob {
			diffs = append(diffs, FileDiff{
				Type:    Modified,
				OldPath: path, NewPath: path,
				OldBlob: oldBlob, NewBlob: newBlob,
			})
		}
	}

	for path, newBlob := range newFiles {
		if _, ok := oldFiles[path]; !ok {
			diffs = append(diffs, FileDiff{Type: Added, NewPath: path, NewBlob: newBlob})
		}
	}

	sort.Slice(diffs, func(i, j int) bool {
		return diffs[i].Path() < diffs[j].Path()
	})
	return diffs, nil
}

// ThreeWayMerge merges file content from base, ours, and theirs using the
// diff3 algorithm (RFC-0008 §4.2). It returns the merged bytes and a boolean
// indicating whether any conflict markers were introduced.
// Conflict regions are marked:
//
//	<<<<<<< <ourLabel>
//	... ours ...
//	=======
//	... theirs ...
//	>>>>>>> <theirLabel>
func ThreeWayMerge(base, ours, theirs []byte, ourLabel, theirLabel string) ([]byte, bool) {
	baseLines := splitLines(base)
	ourLines := splitLines(ours)
	theirLines := splitLines(theirs)

	editsBO := myersDiff(baseLines, ourLines)
	editsBT := myersDiff(baseLines, theirLines)

	ourBlocks := extractChangeBlocks(editsBO)
	theirBlocks := extractChangeBlocks(editsBT)

	merged, hasConflict := mergeChangeBlocks(baseLines, ourBlocks, theirBlocks, ourLabel, theirLabel)

	var result []byte
	for _, line := range merged {
		result = append(result, line...)
	}
	return result, hasConflict
}

// collectFiles recursively populates path→blobID for all files under a tree object.
func collectFiles(store *object.Store, treeObjID types.ObjectID, prefix string, out map[string]types.ObjectID) error {
	obj, err := store.Read(treeObjID)
	if err != nil {
		return err
	}
	tree, ok := obj.(*object.Tree)
	if !ok {
		return fmt.Errorf("object %s is not a tree", treeObjID.String()[:7])
	}
	for _, e := range tree.Entries {
		path := prefix + e.Name
		if e.Mode == 0o040000 { // Directory.
			if err := collectFiles(store, e.Hash, path+"/", out); err != nil {
				return err
			}
		} else {
			out[path] = e.Hash
		}
	}
	return nil
}

// splitLines splits content into a slice of line strings, preserving each
// line's trailing newline character. A final line without '\n' is included as-is.
func splitLines(data []byte) []string {
	if len(data) == 0 {
		return nil
	}
	var lines []string
	start := 0
	for i, b := range data {
		if b == '\n' {
			lines = append(lines, string(data[start:i+1]))
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, string(data[start:]))
	}
	return lines
}
