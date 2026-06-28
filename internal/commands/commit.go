package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/thulasiramk-2310/vara/pkg/builder"
	"github.com/thulasiramk-2310/vara/pkg/graphindex"
	"github.com/thulasiramk-2310/vara/pkg/index"
	"github.com/thulasiramk-2310/vara/pkg/object"
	"github.com/thulasiramk-2310/vara/pkg/types"
)

// RunCommit executes the `vara commit` pipeline.
func RunCommit(ctx *Context, message string) (types.CommitID, error) {
	if message == "" {
		return types.CommitID{}, fmt.Errorf("aborting commit due to empty commit message")
	}

	// 1. Build the Tree from the Index
	store := object.NewStore(ctx.Repository.VaraDir)
	treeID, err := builder.BuildTree(ctx.Index, store)
	if err != nil {
		return types.CommitID{}, fmt.Errorf("failed to build tree: %v", err)
	}

	// 2. Resolve parent commit from HEAD
	var parents []types.CommitID
	headPath := filepath.Join(ctx.Repository.VaraDir, "HEAD")
	headData, err := os.ReadFile(headPath)
	if err == nil && len(headData) > 0 {
		headContent := strings.TrimSpace(string(headData))
		if strings.HasPrefix(headContent, "ref: ") {
			refPath := strings.TrimPrefix(headContent, "ref: ")
			fullRefPath := filepath.Join(ctx.Repository.VaraDir, filepath.FromSlash(refPath))
			refData, err := os.ReadFile(fullRefPath)
			if err == nil && len(refData) >= 64 {
				// Hex encoded string of 32 bytes (64 chars)
				if p, err := types.ParseHex(strings.TrimSpace(string(refData))); err == nil {
					parents = append(parents, types.CommitID(p))
				}
			}
		} else if len(headContent) >= 64 {
			// Detached HEAD
			if p, err := types.ParseHex(strings.TrimSpace(headContent)); err == nil {
				parents = append(parents, types.CommitID(p))
			}
		}
	}

	// 3. Build the Commit
	author := "User <user@example.com>" // Hardcoded for now until config is supported
	commitID, err := builder.BuildCommit(store, types.TreeID(treeID), parents, author, message)
	if err != nil {
		return types.CommitID{}, fmt.Errorf("failed to build commit: %v", err)
	}

	// 4. Update HEAD or the branch it points to
	if err == nil && len(headData) > 0 {
		headContent := strings.TrimSpace(string(headData))
		if strings.HasPrefix(headContent, "ref: ") {
			refPath := strings.TrimPrefix(headContent, "ref: ")
			fullRefPath := filepath.Join(ctx.Repository.VaraDir, filepath.FromSlash(refPath))
			os.MkdirAll(filepath.Dir(fullRefPath), 0755)
			os.WriteFile(fullRefPath, []byte(commitID.String()+"\n"), 0644)
		} else {
			// Detached HEAD
			os.WriteFile(headPath, []byte(commitID.String()+"\n"), 0644)
		}
	} else {
		// By default, if HEAD is empty or doesn't exist, we assume it's ref: refs/heads/main
		refPath := "refs/heads/main"
		fullRefPath := filepath.Join(ctx.Repository.VaraDir, filepath.FromSlash(refPath))
		os.MkdirAll(filepath.Dir(fullRefPath), 0755)
		os.WriteFile(fullRefPath, []byte(commitID.String()+"\n"), 0644)
		os.WriteFile(headPath, []byte("ref: "+refPath+"\n"), 0644)
	}

	// 5. Update index states (StateAdded -> StateUnmodified, etc.)
	var newEntries []index.Entry
	for _, e := range ctx.Index.Entries {
		if e.State == index.StateDeleted {
			continue // Remove deleted files from the index
		}
		e.State = index.StateUnmodified
		newEntries = append(newEntries, e)
	}
	ctx.Index.Entries = newEntries

	indexPath := filepath.Join(ctx.Repository.VaraDir, "index")
	idxData, _ := ctx.Index.Serialize()
	os.WriteFile(indexPath, idxData, 0644)

	// Invalidate the graph index — next history traversal will rebuild it.
	graphindex.Invalidate(ctx.Repository.VaraDir)

	return commitID, nil
}
