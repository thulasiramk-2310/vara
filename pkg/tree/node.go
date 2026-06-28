package tree

import (
	"strings"

	"github.com/thulasiramk-2310/vara/pkg/index"
	"github.com/thulasiramk-2310/vara/pkg/types"
)

// Node represents an in-memory directory or file in a tree hierarchy.
type Node struct {
	IsBlob   bool
	BlobID   types.ObjectID
	Mode     uint32
	Children map[string]*Node
}

// BuildFromIndex constructs a hierarchical tree representation from a linear index.
// This normalizes paths and builds the directory structure.
func BuildFromIndex(idx *index.Index) *Node {
	root := &Node{
		Children: make(map[string]*Node),
	}

	for _, e := range idx.Entries {
		if e.State == index.StateDeleted {
			continue // Do not include deleted files
		}

		parts := strings.Split(e.Path, "/")
		current := root
		for i, part := range parts {
			if i == len(parts)-1 {
				// Leaf node (Blob)
				current.Children[part] = &Node{
					IsBlob: true,
					BlobID: types.ObjectID(e.ObjectID),
					Mode:   0o100644, // Default regular file mode
				}
			} else {
				// Intermediate node (Tree)
				if current.Children[part] == nil {
					current.Children[part] = &Node{
						Children: make(map[string]*Node),
					}
				}
				current = current.Children[part]
			}
		}
	}

	return root
}
