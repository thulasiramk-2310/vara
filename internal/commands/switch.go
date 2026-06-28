// Package commands implements VARA-RFC-0012.
//
// This file implements the `vara switch` command (RFC-0012 §2).
// Depends on: RFC-0004 (refs), RFC-0005 (index), RFC-0006 (locking/transaction),
//             RFC-0007 (commit graph), RFC-0009 (snapshots).
package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/thulasiramk-2310/vara/internal/locking"
	"github.com/thulasiramk-2310/vara/internal/transaction"
	"github.com/thulasiramk-2310/vara/pkg/index"
	"github.com/thulasiramk-2310/vara/pkg/object"
	"github.com/thulasiramk-2310/vara/pkg/reflog"
	"github.com/thulasiramk-2310/vara/pkg/refs"
	"github.com/thulasiramk-2310/vara/pkg/snapshot"
	"github.com/thulasiramk-2310/vara/pkg/types"
)

// RunSwitch executes the `vara switch <branch>` command.
//
// Pipeline (RFC-0012 §2):
//  1. Validate branch name (RFC-0004 §3).
//  2. Resolve target branch → commit ID.
//  3. Check if HEAD already points to the branch.
//  4. Snapshot working directory (RFC-0009 §5).
//  5. Begin transaction (RFC-0006 §4) acquiring Refs + Index locks.
//  6. Checkout target tree to working directory.
//  7. Remove stale tracked files not present in target tree.
//  8. Write new index atomically.
//  9. Update HEAD atomically (RFC-0004 §4).
//  10. Append to reflog.
//  11. Commit transaction.
func RunSwitch(ctx *Context, targetBranch string) (string, error) {
	if targetBranch == "" {
		return "", fmt.Errorf("branch name required")
	}

	// 1. Validate branch name (RFC-0004 §3).
	if err := refs.ValidateName(targetBranch); err != nil {
		return "", err
	}

	resolver := refs.NewFSResolver(ctx.Repository.VaraDir)
	targetRef := "refs/heads/" + targetBranch

	// 2. Resolve target branch to a commit.
	targetCommitID, err := resolver.Resolve(targetRef)
	if err != nil {
		return "", fmt.Errorf("branch '%s' not found", targetBranch)
	}

	// 3. Already on this branch?
	if currentRef, symErr := resolver.ResolveSymbolic("HEAD"); symErr == nil && currentRef == targetRef {
		return fmt.Sprintf("Already on '%s'\n", targetBranch), nil
	}

	// 4. Capture current HEAD state for snapshot naming and reflog.
	var currentCommitID types.CommitID
	currentCommitHex := strings.Repeat("0", 64)
	currentBranchName := "HEAD"

	if cid, cidErr := resolver.Resolve("HEAD"); cidErr == nil {
		currentCommitID = cid
		currentCommitHex = cid.String()
	}
	if currentRef, symErr := resolver.ResolveSymbolic("HEAD"); symErr == nil {
		currentBranchName = strings.TrimPrefix(currentRef, "refs/heads/")
	}

	// 5. Snapshot the working directory before any mutation (RFC-0009 §5).
	// Non-fatal: warn and proceed so a disk-full condition doesn't block switching.
	if _, snapErr := snapshot.Create(
		ctx.Repository.VaraDir, ctx.Repository.RootDir,
		"switch", currentCommitHex,
	); snapErr != nil {
		fmt.Fprintf(os.Stderr, "vara: warning: snapshot failed: %v\n", snapErr)
	}

	// 6. Begin transaction — Refs lock before Index lock per RFC-0006 §2 hierarchy.
	txn, err := transaction.Begin(ctx.Repository.VaraDir, "switch",
		locking.NameRefs, locking.NameIndex)
	if err != nil {
		return "", fmt.Errorf("switch: %w", err)
	}
	defer txn.Rollback() // No-op after Commit().

	if err := txn.SetState(transaction.StateExecute); err != nil {
		return "", fmt.Errorf("switch: journal: %w", err)
	}

	// 7. Load the target commit object.
	store := object.NewStore(ctx.Repository.VaraDir)
	targetCommitObj, err := store.Read(types.ObjectID(targetCommitID))
	if err != nil {
		return "", fmt.Errorf("switch: read target commit: %w", err)
	}
	targetCommit, ok := targetCommitObj.(*object.Commit)
	if !ok {
		return "", fmt.Errorf("switch: target object is not a commit")
	}

	// 8. Checkout target tree; build the new index.
	oldPaths := collectIndexPaths(ctx.Index)
	newIdx := index.New()
	if err := checkoutTree(store, targetCommit.TreeHash, ctx.Repository.RootDir, newIdx); err != nil {
		return "", fmt.Errorf("switch: checkout: %w", err)
	}

	// 9. Remove files tracked by the current index but absent in the new tree.
	newPaths := collectIndexPaths(newIdx)
	for path := range oldPaths {
		if !newPaths[path] {
			absPath := filepath.Join(ctx.Repository.RootDir, filepath.FromSlash(path))
			os.Remove(absPath) // Best-effort; ignore ENOENT.
		}
	}

	// 10. Write the new index atomically (RFC-0006 §4 EXECUTE step).
	ctx.Index = newIdx
	idxData, err := newIdx.Serialize()
	if err != nil {
		return "", fmt.Errorf("switch: serialize index: %w", err)
	}
	indexPath := filepath.Join(ctx.Repository.VaraDir, "index")
	if err := atomicWriteFile(indexPath, idxData, 0644); err != nil {
		return "", fmt.Errorf("switch: write index: %w", err)
	}

	if err := txn.SetState(transaction.StateVerify); err != nil {
		return "", fmt.Errorf("switch: journal: %w", err)
	}

	// 11. Update HEAD atomically (RFC-0004 §4 steps 2–4).
	headPath := filepath.Join(ctx.Repository.VaraDir, "HEAD")
	headContent := fmt.Sprintf("ref: %s\n", targetRef)
	if err := atomicWriteFile(headPath, []byte(headContent), 0644); err != nil {
		return "", fmt.Errorf("switch: update HEAD: %w", err)
	}
	txn.TrackRef("HEAD", targetCommitID.String())

	// 12. Append a reflog entry for HEAD (RFC-0004 §4 step 6).
	rm := reflog.NewManager(ctx.Repository.VaraDir)
	rm.Append("HEAD", currentCommitID, targetCommitID, "User <user@example.com>",
		fmt.Sprintf("switch: moving from %s to %s", currentBranchName, targetBranch))

	if err := txn.SetState(transaction.StateCommit); err != nil {
		return "", fmt.Errorf("switch: journal: %w", err)
	}
	if err := txn.Commit(); err != nil {
		return "", fmt.Errorf("switch: commit transaction: %w", err)
	}

	return fmt.Sprintf("Switched to branch '%s'\n", targetBranch), nil
}

// checkoutTree recursively checks out a tree object into rootDir,
// appending one index.Entry per blob to newIdx.
func checkoutTree(store *object.Store, treeID types.TreeID, rootDir string, newIdx *index.Index) error {
	return checkoutTreeEntries(store, types.ObjectID(treeID), rootDir, "", newIdx)
}

func checkoutTreeEntries(store *object.Store, objID types.ObjectID, rootDir, prefix string, newIdx *index.Index) error {
	obj, err := store.Read(objID)
	if err != nil {
		return fmt.Errorf("read object %s: %w", objID.String()[:7], err)
	}
	tree, ok := obj.(*object.Tree)
	if !ok {
		return fmt.Errorf("object %s is not a tree", objID.String()[:7])
	}

	for _, entry := range tree.Entries {
		relPath := prefix + entry.Name
		absPath := filepath.Join(rootDir, filepath.FromSlash(relPath))

		if entry.Mode == 0o040000 { // Directory.
			if err := os.MkdirAll(absPath, 0755); err != nil {
				return err
			}
			if err := checkoutTreeEntries(store, entry.Hash, rootDir, relPath+"/", newIdx); err != nil {
				return err
			}
			continue
		}

		// Blob: write content to working directory.
		blobObj, err := store.Read(entry.Hash)
		if err != nil {
			return fmt.Errorf("read blob %s: %w", entry.Hash.String()[:7], err)
		}
		blob, ok := blobObj.(*object.Blob)
		if !ok {
			return fmt.Errorf("object %s is not a blob", entry.Hash.String()[:7])
		}
		if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
			return err
		}
		if err := os.WriteFile(absPath, blob.Data, 0644); err != nil {
			return fmt.Errorf("write %s: %w", relPath, err)
		}

		info, _ := os.Stat(absPath)
		fp := uint64(0)
		if info != nil {
			fp = uint64(info.ModTime().UnixNano())
		}
		newIdx.Entries = append(newIdx.Entries, index.Entry{
			Fingerprint: fp,
			ObjectID:    types.BlobID(entry.Hash),
			State:       index.StateUnmodified,
			Path:        relPath,
		})
	}
	return nil
}

// collectIndexPaths returns a set of paths tracked by idx.
func collectIndexPaths(idx *index.Index) map[string]bool {
	paths := make(map[string]bool, len(idx.Entries))
	for _, e := range idx.Entries {
		paths[e.Path] = true
	}
	return paths
}

// atomicWriteFile writes content to path using temp-file + rename for crash safety.
func atomicWriteFile(path string, content []byte, perm os.FileMode) error {
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, content, perm); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return err
	}
	return nil
}
