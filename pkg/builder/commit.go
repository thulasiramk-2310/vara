package builder

import (
	"time"

	"github.com/thulasiramk-2310/vara/pkg/object"
	"github.com/thulasiramk-2310/vara/pkg/types"
)

// BuildCommit constructs a Commit object and writes it to the object store.
func BuildCommit(store *object.Store, treeID types.TreeID, parents []types.CommitID, author, message string) (types.CommitID, error) {
	commit := &object.Commit{
		TreeHash:  treeID,
		Parents:   parents,
		Author:    author,
		Message:   message,
		Timestamp: time.Now().Unix(),
	}

	id, err := store.Write(commit)
	if err != nil {
		return types.CommitID{}, err
	}

	return types.CommitID(id), nil
}
