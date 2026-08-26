package commands

import (
	"fmt"

	"github.com/thulasiramk-2310/vara/pkg/object"
	"github.com/thulasiramk-2310/vara/pkg/types"
)

// commitTree returns the tree ID recorded by a commit.
func commitTree(store *object.Store, commitID types.CommitID) (types.TreeID, error) {
	obj, err := store.Read(types.ObjectID(commitID))
	if err != nil {
		return types.TreeID{}, fmt.Errorf("read commit %s: %w", commitID.String()[:7], err)
	}
	c, ok := obj.(*object.Commit)
	if !ok {
		return types.TreeID{}, fmt.Errorf("object %s is not a commit", commitID.String()[:7])
	}
	return c.TreeHash, nil
}

// treeBlobs walks a tree recursively and returns a path → blob-ID map for every
// file it contains (directories are descended, not recorded). Read-only.
func treeBlobs(store *object.Store, treeID types.TreeID) (map[string]types.BlobID, error) {
	out := make(map[string]types.BlobID)
	if err := walkTreeBlobs(store, types.ObjectID(treeID), "", out); err != nil {
		return nil, err
	}
	return out, nil
}

func walkTreeBlobs(store *object.Store, objID types.ObjectID, prefix string, out map[string]types.BlobID) error {
	obj, err := store.Read(objID)
	if err != nil {
		return fmt.Errorf("read object %s: %w", objID.String()[:7], err)
	}
	tree, ok := obj.(*object.Tree)
	if !ok {
		return fmt.Errorf("object %s is not a tree", objID.String()[:7])
	}
	for _, e := range tree.Entries {
		relPath := prefix + e.Name
		if e.Mode == 0o040000 { // directory
			if err := walkTreeBlobs(store, e.Hash, relPath+"/", out); err != nil {
				return err
			}
			continue
		}
		out[relPath] = types.BlobID(e.Hash)
	}
	return nil
}

// blobContent reads a blob's bytes by ID.
func blobContent(store *object.Store, id types.BlobID) ([]byte, error) {
	obj, err := store.Read(types.ObjectID(id))
	if err != nil {
		return nil, fmt.Errorf("read blob %s: %w", id.String()[:7], err)
	}
	blob, ok := obj.(*object.Blob)
	if !ok {
		return nil, fmt.Errorf("object %s is not a blob", id.String()[:7])
	}
	return blob.Data, nil
}

// parseBlobID parses a hex blob ID; the bool is false for an empty or invalid
// string (an empty side of a conflict is stored as "").
func parseBlobID(hexID string) (types.BlobID, bool) {
	if hexID == "" {
		return types.BlobID{}, false
	}
	id, err := types.ParseHex(hexID)
	if err != nil {
		return types.BlobID{}, false
	}
	return types.BlobID(id), true
}
