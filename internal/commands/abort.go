// Package commands — vara merge --abort.
//
// Abort restores the repository to its pre-merge state. Because VARA does not
// advance HEAD during a conflicted merge, the pre-merge tracked state is exactly
// the current HEAD's tree (see docs/merge-state.md). Abort checks out that tree,
// discards the merge's conflict staging, and removes MERGE_HEAD and the conflict
// sidecar.
//
// It refuses — naming the files — if it would clobber edits the user made, after
// the conflict appeared, to files the merge never touched, so unrelated work is
// never silently discarded.
package commands

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/thulasiramk-2310/vara/internal/locking"
	"github.com/thulasiramk-2310/vara/internal/mergestate"
	"github.com/thulasiramk-2310/vara/internal/transaction"
	"github.com/thulasiramk-2310/vara/pkg/index"
	"github.com/thulasiramk-2310/vara/pkg/object"
	"github.com/thulasiramk-2310/vara/pkg/recovery"
	"github.com/thulasiramk-2310/vara/pkg/refs"
	"github.com/thulasiramk-2310/vara/pkg/snapshot"
)

// RunMergeAbort executes `vara merge --abort`.
func RunMergeAbort(ctx *Context) (string, error) {
	varaDir := ctx.Repository.VaraDir
	if !recovery.MergeInProgress(varaDir) {
		return "", fmt.Errorf("no merge in progress; nothing to abort")
	}

	store := object.NewStore(varaDir)
	resolver := refs.NewFSResolver(varaDir)

	headCommit, err := resolver.Resolve("HEAD")
	if err != nil {
		return "", fmt.Errorf("merge --abort: HEAD is not a valid commit: %w", err)
	}
	headTree, err := commitTree(store, headCommit)
	if err != nil {
		return "", fmt.Errorf("merge --abort: %w", err)
	}
	headBlobs, err := treeBlobs(store, headTree)
	if err != nil {
		return "", fmt.Errorf("merge --abort: read HEAD tree: %w", err)
	}

	// Paths present in the current index (whatever state it is now in).
	idxPaths := map[string]bool{}
	for _, e := range ctx.Index.Entries {
		if e.State == index.StateDeleted {
			continue
		}
		idxPaths[e.Path] = true
	}

	// "Merge-involved" must be judged from what the merge did at conflict time —
	// recorded in the sidecar (conflicts + MergeTouched) — NOT from the current
	// index, which a later `vara resolve`/`vara add`/`vara rm` has mutated. Using
	// the current index would let a user's own post-conflict deletion masquerade as
	// a merge-authored change and be silently restored by abort.
	st, hasSidecar, err := mergestate.Read(varaDir)
	if err != nil {
		return "", fmt.Errorf("merge --abort: read conflict state: %w", err)
	}
	involved := func(p string) bool {
		if hasSidecar {
			return st.MergeInvolved(p)
		}
		return false
	}

	// Clobber guard: for a path the merge did NOT touch, any post-conflict user
	// change — an edit, a deletion, or a brand-new file — must not be silently
	// discarded by the restore-to-HEAD. Refuse and name every such path.
	var clobber []string
	allPaths := map[string]bool{}
	for p := range headBlobs {
		allPaths[p] = true
	}
	for p := range idxPaths {
		allPaths[p] = true
	}
	for p := range allPaths {
		if involved(p) {
			continue
		}
		headID, inHead := headBlobs[p]
		abs := filepath.Join(ctx.Repository.RootDir, filepath.FromSlash(p))
		disk, readErr := os.ReadFile(abs)
		onDisk := readErr == nil

		switch {
		case inHead && !onDisk:
			// User deleted a tracked file the merge never touched; restore would
			// resurrect it, discarding their deletion.
			clobber = append(clobber, p)
		case inHead && onDisk:
			headContent, err := blobContent(store, headID)
			if err != nil {
				return "", fmt.Errorf("merge --abort: read HEAD blob for %s: %w", p, err)
			}
			if !bytes.Equal(disk, headContent) {
				clobber = append(clobber, p) // user edited it after the conflict
			}
		case !inHead && onDisk:
			// A new file the user added post-conflict; restore-to-HEAD would delete it.
			clobber = append(clobber, p)
		}
	}
	if len(clobber) > 0 {
		sort.Strings(clobber)
		return "", fmt.Errorf(
			"cannot abort: these files have local changes not part of the merge:\n\t%s\n\n"+
				"Commit, stash, or discard them first, then run 'vara merge --abort'.",
			strings.Join(clobber, "\n\t"))
	}

	// Snapshot before mutation (non-fatal on failure, matching switch/merge).
	if _, snapErr := snapshot.Create(varaDir, ctx.Repository.RootDir, "merge-abort", headCommit.String()); snapErr != nil {
		fmt.Fprintf(os.Stderr, "vara: warning: snapshot failed: %v\n", snapErr)
	}

	txn, err := transaction.Begin(varaDir, "merge-abort", locking.NameRefs, locking.NameIndex)
	if err != nil {
		return "", fmt.Errorf("merge --abort: %w", err)
	}
	defer txn.Rollback()
	if err := txn.SetState(transaction.StateExecute); err != nil {
		return "", fmt.Errorf("merge --abort: journal: %w", err)
	}

	// Restore the working tree + index to HEAD's tree.
	oldPaths := collectIndexPaths(ctx.Index)
	newIdx := index.New()
	if err := checkoutTree(store, headTree, ctx.Repository.RootDir, newIdx); err != nil {
		return "", fmt.Errorf("merge --abort: checkout HEAD: %w", err)
	}
	newPaths := collectIndexPaths(newIdx)
	for p := range oldPaths {
		if !newPaths[p] {
			os.Remove(filepath.Join(ctx.Repository.RootDir, filepath.FromSlash(p)))
		}
	}

	ctx.Index = newIdx
	if err := writeIndex(ctx); err != nil {
		return "", fmt.Errorf("merge --abort: write index: %w", err)
	}

	if err := txn.SetState(transaction.StateVerify); err != nil {
		return "", fmt.Errorf("merge --abort: journal: %w", err)
	}

	// Clear the merge-in-progress markers.
	if err := recovery.ClearMergeHead(varaDir); err != nil {
		return "", fmt.Errorf("merge --abort: clear MERGE_HEAD: %w", err)
	}
	if err := mergestate.Clear(varaDir); err != nil {
		return "", fmt.Errorf("merge --abort: clear conflict sidecar: %w", err)
	}

	if err := txn.Commit(); err != nil {
		return "", err
	}

	return fmt.Sprintf("Merge aborted. Restored to %s.\n", headCommit.String()[:7]), nil
}
