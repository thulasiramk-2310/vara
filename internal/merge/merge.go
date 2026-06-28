// Package merge implements VARA-RFC-0008.
//
// RFC:
// VARA-RFC-0008 Merge Algorithm
//
// Responsibilities:
// - Fast-Forward merge detection (RFC-0008 §4.1).
// - Three-way merge execution (RFC-0008 §4.2).
// - Conflict classification and marker insertion (RFC-0008 §5).
// - Write MERGE_HEAD on conflict suspension (RFC-0008 §4.2 step 4).
//
// This package MUST NOT:
// - Import commands, transaction, locking, or any higher-layer package.
// - Perform reference resolution (callers must pass resolved commit IDs).
package merge

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/thulasiramk-2310/vara/pkg/diff"
	"github.com/thulasiramk-2310/vara/pkg/graph"
	"github.com/thulasiramk-2310/vara/pkg/index"
	"github.com/thulasiramk-2310/vara/pkg/object"
	"github.com/thulasiramk-2310/vara/pkg/types"
)

// Result is the outcome of a merge operation (RFC-0008 §3).
type Result uint8

const (
	Success     Result = iota // Merge completed automatically.
	Conflict                  // Suspended pending user conflict resolution.
	FastForward               // Current branch was simply advanced.
)

// Output carries the result of a Merge call.
type Output struct {
	Result      Result
	NewCommitID types.CommitID // Set on Success or FastForward.
	Conflicts   []string       // File paths with unresolved conflicts.
}

// Inputs bundles the data the merge engine needs without importing commands.
type Inputs struct {
	Store       *object.Store
	Index       *index.Index
	VaraDir     string
	RootDir     string
	OurCommit   types.CommitID // HEAD at the time of merge.
	TheirCommit types.CommitID // Tip of the branch being merged in.
	OurLabel    string         // Used in conflict markers (e.g. "HEAD").
	TheirLabel  string         // Used in conflict markers (e.g. branch name).
}

// fileChange records how a single file changed on one side of the merge.
type fileChange struct {
	changeType diff.ChangeType
	blobID     types.ObjectID
}

// Merge executes the merge algorithm (RFC-0008 §4).
// The caller is responsible for the surrounding transaction and ref updates.
func Merge(in Inputs) (Output, error) {
	base, err := graph.MergeBase(in.Store, in.OurCommit, in.TheirCommit)
	if err != nil {
		return Output{}, fmt.Errorf("merge: find merge base: %w", err)
	}

	// Fast-forward: base == our commit → their tip is a direct descendant.
	if base == in.OurCommit {
		if err := fastForward(in); err != nil {
			return Output{}, fmt.Errorf("merge: fast-forward: %w", err)
		}
		return Output{Result: FastForward, NewCommitID: in.TheirCommit}, nil
	}

	// Already up-to-date: base == their commit → we already contain their work.
	if base == in.TheirCommit {
		return Output{Result: Success, NewCommitID: in.OurCommit}, nil
	}

	return threeWay(in, base)
}

// fastForward updates the working directory and index to match TheirCommit.
func fastForward(in Inputs) error {
	theirCommit, err := readCommit(in.Store, in.TheirCommit)
	if err != nil {
		return err
	}
	return checkoutTree(in.Store, theirCommit.TreeHash, in.RootDir, in.Index)
}

// threeWay performs the recursive three-way merge (RFC-0008 §4.2).
func threeWay(in Inputs, base types.CommitID) (Output, error) {
	baseCommit, err := readCommit(in.Store, base)
	if err != nil {
		return Output{}, err
	}
	ourCommit, err := readCommit(in.Store, in.OurCommit)
	if err != nil {
		return Output{}, err
	}
	theirCommit, err := readCommit(in.Store, in.TheirCommit)
	if err != nil {
		return Output{}, err
	}

	baseTreeID := types.ObjectID(baseCommit.TreeHash)
	ourTreeID := types.ObjectID(ourCommit.TreeHash)
	theirTreeID := types.ObjectID(theirCommit.TreeHash)

	diffOurs, err := diff.DiffTrees(in.Store, baseTreeID, ourTreeID)
	if err != nil {
		return Output{}, fmt.Errorf("merge: diff base→ours: %w", err)
	}
	diffTheirs, err := diff.DiffTrees(in.Store, baseTreeID, theirTreeID)
	if err != nil {
		return Output{}, fmt.Errorf("merge: diff base→theirs: %w", err)
	}

	ourChanges := make(map[string]fileChange)
	for _, d := range diffOurs {
		ourChanges[d.Path()] = fileChange{d.Type, d.NewBlob}
	}
	theirChanges := make(map[string]fileChange)
	for _, d := range diffTheirs {
		theirChanges[d.Path()] = fileChange{d.Type, d.NewBlob}
	}

	allPaths := make(map[string]bool)
	for p := range ourChanges {
		allPaths[p] = true
	}
	for p := range theirChanges {
		allPaths[p] = true
	}

	var conflicts []string

	for path := range allPaths {
		oc, ourChanged := ourChanges[path]
		tc, theirChanged := theirChanges[path]

		switch {
		case ourChanged && !theirChanged:
			if err := applyChange(in, path, oc); err != nil {
				return Output{}, err
			}

		case !ourChanged && theirChanged:
			if err := applyChange(in, path, tc); err != nil {
				return Output{}, err
			}

		case oc.blobID == tc.blobID:
			// Both sides made the identical change.
			if err := applyChange(in, path, oc); err != nil {
				return Output{}, err
			}

		case oc.changeType == diff.Modified && tc.changeType == diff.Modified:
			// Both modified: attempt line-level three-way merge.
			hasConflict, err := mergeFileContent(in, path, baseCommit.TreeHash, oc.blobID, tc.blobID)
			if err != nil {
				return Output{}, err
			}
			if hasConflict {
				conflicts = append(conflicts, path)
			}

		default:
			// Structural conflict (delete/modify, etc.) — RFC-0008 §5.
			conflicts = append(conflicts, path)
		}
	}

	if len(conflicts) > 0 {
		// Write MERGE_HEAD so subsequent commands know a merge is in progress.
		mergeHeadPath := filepath.Join(in.VaraDir, "MERGE_HEAD")
		os.WriteFile(mergeHeadPath, []byte(in.TheirCommit.String()+"\n"), 0644)
		return Output{Result: Conflict, Conflicts: conflicts}, nil
	}

	return Output{Result: Success}, nil
}

// mergeFileContent runs line-level three-way merge on one modified file.
// Returns true when the merged file contains conflict markers.
func mergeFileContent(
	in Inputs,
	path string,
	baseTreeID types.TreeID,
	ourBlobID, theirBlobID types.ObjectID,
) (bool, error) {
	baseBlobID, _ := findBlobInTree(in.Store, baseTreeID, path)

	baseContent := readBlobData(in.Store, baseBlobID)
	ourContent := readBlobData(in.Store, ourBlobID)
	theirContent := readBlobData(in.Store, theirBlobID)

	merged, hasConflict := diff.ThreeWayMerge(baseContent, ourContent, theirContent, in.OurLabel, in.TheirLabel)

	absPath := filepath.Join(in.RootDir, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
		return false, err
	}
	if err := os.WriteFile(absPath, merged, 0644); err != nil {
		return false, fmt.Errorf("merge: write merged %s: %w", path, err)
	}

	blob := object.NewBlob(merged)
	blobID, err := in.Store.Write(blob)
	if err != nil {
		return false, fmt.Errorf("merge: store merged blob: %w", err)
	}
	updateIndex(in.Index, path, types.BlobID(blobID))

	return hasConflict, nil
}

// applyChange applies a single-sided file change to the working directory
// and index.
func applyChange(in Inputs, path string, fc fileChange) error {
	absPath := filepath.Join(in.RootDir, filepath.FromSlash(path))
	if fc.changeType == diff.Deleted {
		os.Remove(absPath)
		removeFromIndex(in.Index, path)
		return nil
	}
	// Added or Modified.
	content := readBlobData(in.Store, fc.blobID)
	if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(absPath, content, 0644); err != nil {
		return fmt.Errorf("merge: apply %s: %w", path, err)
	}
	updateIndex(in.Index, path, types.BlobID(fc.blobID))
	return nil
}

// checkoutTree recursively checks out a tree into rootDir and updates idx.
func checkoutTree(store *object.Store, treeID types.TreeID, rootDir string, idx *index.Index) error {
	return checkoutTreeEntries(store, types.ObjectID(treeID), rootDir, "", idx)
}

func checkoutTreeEntries(store *object.Store, objID types.ObjectID, rootDir, prefix string, idx *index.Index) error {
	obj, err := store.Read(objID)
	if err != nil {
		return err
	}
	tree, ok := obj.(*object.Tree)
	if !ok {
		return fmt.Errorf("merge: %s is not a tree", objID.String()[:7])
	}
	for _, entry := range tree.Entries {
		relPath := prefix + entry.Name
		absPath := filepath.Join(rootDir, filepath.FromSlash(relPath))
		if entry.Mode == 0o040000 {
			os.MkdirAll(absPath, 0755)
			if err := checkoutTreeEntries(store, entry.Hash, rootDir, relPath+"/", idx); err != nil {
				return err
			}
			continue
		}
		blob, err := readBlobObj(store, entry.Hash)
		if err != nil {
			return err
		}
		os.MkdirAll(filepath.Dir(absPath), 0755)
		if err := os.WriteFile(absPath, blob.Data, 0644); err != nil {
			return err
		}
		updateIndex(idx, relPath, types.BlobID(entry.Hash))
	}
	return nil
}

// findBlobInTree walks a tree to find the blob ID at path.
func findBlobInTree(store *object.Store, treeID types.TreeID, path string) (types.ObjectID, error) {
	files := make(map[string]types.ObjectID)
	if err := collectTreeFiles(store, types.ObjectID(treeID), "", files); err != nil {
		return types.ObjectID{}, err
	}
	id, ok := files[path]
	if !ok {
		return types.ObjectID{}, fmt.Errorf("path %s not in tree", path)
	}
	return id, nil
}

func collectTreeFiles(store *object.Store, treeObjID types.ObjectID, prefix string, out map[string]types.ObjectID) error {
	obj, err := store.Read(treeObjID)
	if err != nil {
		return err
	}
	tree, ok := obj.(*object.Tree)
	if !ok {
		return fmt.Errorf("%s is not a tree", treeObjID.String()[:7])
	}
	for _, e := range tree.Entries {
		p := prefix + e.Name
		if e.Mode == 0o040000 {
			collectTreeFiles(store, e.Hash, p+"/", out)
		} else {
			out[p] = e.Hash
		}
	}
	return nil
}

func readCommit(store *object.Store, id types.CommitID) (*object.Commit, error) {
	obj, err := store.Read(types.ObjectID(id))
	if err != nil {
		return nil, err
	}
	c, ok := obj.(*object.Commit)
	if !ok {
		return nil, fmt.Errorf("%s is not a commit", id.String()[:7])
	}
	return c, nil
}

func readBlobData(store *object.Store, id types.ObjectID) []byte {
	if id == (types.ObjectID{}) {
		return nil
	}
	obj, err := store.Read(id)
	if err != nil {
		return nil
	}
	b, ok := obj.(*object.Blob)
	if !ok {
		return nil
	}
	return b.Data
}

func readBlobObj(store *object.Store, id types.ObjectID) (*object.Blob, error) {
	obj, err := store.Read(id)
	if err != nil {
		return nil, err
	}
	b, ok := obj.(*object.Blob)
	if !ok {
		return nil, fmt.Errorf("%s is not a blob", id.String()[:7])
	}
	return b, nil
}

func updateIndex(idx *index.Index, path string, blobID types.BlobID) {
	for i := range idx.Entries {
		if idx.Entries[i].Path == path {
			idx.Entries[i].ObjectID = blobID
			idx.Entries[i].State = index.StateModified
			return
		}
	}
	idx.Entries = append(idx.Entries, index.Entry{
		ObjectID: blobID,
		State:    index.StateAdded,
		Path:     path,
	})
}

func removeFromIndex(idx *index.Index, path string) {
	out := idx.Entries[:0]
	for _, e := range idx.Entries {
		if e.Path != path {
			out = append(out, e)
		}
	}
	idx.Entries = out
}
