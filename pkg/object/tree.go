package object

import (
	"bytes"
	"encoding/binary"
	"sort"

	"github.com/thulasiramk-2310/vara/internal/errors"
	"github.com/thulasiramk-2310/vara/pkg/types"
)

// TreeEntry represents a single file or subdirectory in a tree.
type TreeEntry struct {
	Mode uint32
	Hash types.ObjectID // Can be BlobID or TreeID
	Name string
}

// Tree represents a directory structure.
type Tree struct {
	Entries []TreeEntry
}

// NewTree creates a new Tree and sorts its entries lexicographically.
func NewTree(entries []TreeEntry) *Tree {
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name < entries[j].Name
	})
	return &Tree{Entries: entries}
}

// Type returns TypeTree.
func (t *Tree) Type() ObjectType {
	return TypeTree
}

// Serialize converts the tree to its raw on-disk byte format.
func (t *Tree) Serialize() []byte {
	var buf bytes.Buffer
	WriteHeader(&buf, TypeTree)
	for _, e := range t.Entries {
		binary.Write(&buf, binary.BigEndian, e.Mode)
		buf.Write(e.Hash[:])
		buf.WriteString(e.Name)
		buf.WriteByte(0)
	}
	return buf.Bytes()
}

// DeserializeTree reads a tree from the reader.
func DeserializeTree(r *bytes.Reader) (*Tree, error) {
	var entries []TreeEntry
	for r.Len() > 0 {
		var mode uint32
		if err := binary.Read(r, binary.BigEndian, &mode); err != nil {
			return nil, errors.ErrInvalidObject
		}
		var h types.ObjectID
		if n, err := r.Read(h[:]); err != nil || n != 32 {
			return nil, errors.ErrInvalidObject
		}
		nameBytes, err := readUntilNul(r)
		if err != nil {
			return nil, errors.ErrInvalidObject
		}
		entries = append(entries, TreeEntry{
			Mode: mode,
			Hash: h,
			Name: string(nameBytes),
		})
	}
	return &Tree{Entries: entries}, nil
}
