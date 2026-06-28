package builder

import (
	"github.com/thulasiramk-2310/vara/pkg/index"
	"github.com/thulasiramk-2310/vara/pkg/object"
	"github.com/thulasiramk-2310/vara/pkg/tree"
	"github.com/thulasiramk-2310/vara/pkg/types"
)

// BuildTree converts the linear Index into a hierarchy of Tree objects,
// writes them to the store, and returns the root TreeID.
func BuildTree(idx *index.Index, store *object.Store) (types.ObjectID, error) {
	root := tree.BuildFromIndex(idx)
	// Perform a post-order traversal to hash and store trees
	return writeNode(root, store)
}

func writeNode(n *tree.Node, store *object.Store) (types.ObjectID, error) {
	if n.IsBlob {
		return n.BlobID, nil
	}

	var entries []object.TreeEntry
	for name, child := range n.Children {
		hash, err := writeNode(child, store)
		if err != nil {
			return types.ObjectID{}, err
		}

		mode := uint32(0o040000) // Directory mode by default
		if child.IsBlob {
			mode = child.Mode
		}

		entries = append(entries, object.TreeEntry{
			Mode: mode,
			Name: name,
			Hash: hash,
		})
	}

	tree := object.NewTree(entries)
	id, err := store.Write(tree)
	if err != nil {
		return types.ObjectID{}, err
	}

	return id, nil
}
