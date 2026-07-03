package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/thulasiramk-2310/vara/internal/locking"
	mergeengine "github.com/thulasiramk-2310/vara/internal/merge"
	"github.com/thulasiramk-2310/vara/internal/transaction"
	"github.com/thulasiramk-2310/vara/pkg/builder"
	"github.com/thulasiramk-2310/vara/pkg/graphindex"
	"github.com/thulasiramk-2310/vara/pkg/object"
	"github.com/thulasiramk-2310/vara/pkg/reflog"
	"github.com/thulasiramk-2310/vara/pkg/refs"
	"github.com/thulasiramk-2310/vara/pkg/snapshot"
	"github.com/thulasiramk-2310/vara/pkg/types"
)

// RunMerge executes `vara merge <branch>` (RFC-0012 §2, RFC-0008 §4).
//
// Pipeline:
//  1. Validate branch name and resolve both commits.
//  2. Snapshot working directory (RFC-0009 §5).
//  3. Begin transaction (Refs + Index locks).
//  4. Execute merge engine (fast-forward or three-way).
//  5. On Success/FastForward: create merge commit, update refs, write index.
//  6. On Conflict: write index with markers, leave MERGE_HEAD, notify user.
func RunMerge(ctx *Context, branchName string) (string, error) {
	if branchName == "" {
		return "", fmt.Errorf("branch name required")
	}
	if err := refs.ValidateName(branchName); err != nil {
		return "", err
	}

	resolver := refs.NewFSResolver(ctx.Repository.VaraDir)
	theirRef := "refs/heads/" + branchName

	theirCommitID, err := resolver.Resolve(theirRef)
	if err != nil {
		return "", fmt.Errorf("branch '%s' not found", branchName)
	}

	return MergeIntoHEAD(ctx, theirCommitID, branchName)
}

// MergeIntoHEAD merges an arbitrary commit into the current HEAD using the same
// snapshot → transaction → merge-engine pipeline as `vara merge`. It is shared
// by `vara merge <branch>` (theirLabel = branch name) and `vara pull`
// (theirLabel = "<remote>/<branch>"), since the merge engine is commit-based.
func MergeIntoHEAD(ctx *Context, theirCommitID types.CommitID, theirLabel string) (string, error) {
	resolver := refs.NewFSResolver(ctx.Repository.VaraDir)

	ourCommitID, err := resolver.Resolve("HEAD")
	if err != nil {
		return "", fmt.Errorf("merge: HEAD is not a valid commit; nothing to merge into")
	}

	// Snapshot before any mutation.
	if _, snapErr := snapshot.Create(
		ctx.Repository.VaraDir, ctx.Repository.RootDir,
		"merge", ourCommitID.String(),
	); snapErr != nil {
		fmt.Fprintf(os.Stderr, "vara: warning: snapshot failed: %v\n", snapErr)
	}

	txn, err := transaction.Begin(ctx.Repository.VaraDir, "merge",
		locking.NameRefs, locking.NameIndex)
	if err != nil {
		return "", fmt.Errorf("merge: %w", err)
	}
	defer txn.Rollback()

	if err := txn.SetState(transaction.StateExecute); err != nil {
		return "", fmt.Errorf("merge: journal: %w", err)
	}

	store := object.NewStore(ctx.Repository.VaraDir)

	// Determine ours label from HEAD symbolic ref.
	ourLabel := "HEAD"
	if currentRef, err := resolver.ResolveSymbolic("HEAD"); err == nil {
		ourLabel = strings.TrimPrefix(currentRef, "refs/heads/")
	}

	out, err := mergeengine.Merge(mergeengine.Inputs{
		Store:       store,
		Index:       ctx.Index,
		VaraDir:     ctx.Repository.VaraDir,
		RootDir:     ctx.Repository.RootDir,
		OurCommit:   ourCommitID,
		TheirCommit: theirCommitID,
		OurLabel:    ourLabel,
		TheirLabel:  theirLabel,
	})
	if err != nil {
		return "", fmt.Errorf("merge: %w", err)
	}

	switch out.Result {
	case mergeengine.Conflict:
		// Write conflict-marked index to disk; leave MERGE_HEAD written by engine.
		if err := writeIndex(ctx); err != nil {
			return "", fmt.Errorf("merge: write index: %w", err)
		}
		if err := txn.Commit(); err != nil {
			return "", err
		}
		var sb strings.Builder
		sb.WriteString("Automatic merge failed; fix conflicts and then commit.\n")
		sb.WriteString("Conflicts:\n")
		for _, c := range out.Conflicts {
			sb.WriteString("\t" + c + "\n")
		}
		return sb.String(), nil

	case mergeengine.FastForward:
		// Update the current branch ref to point to their commit.
		if err := writeIndex(ctx); err != nil {
			return "", fmt.Errorf("merge: write index: %w", err)
		}
		if err := txn.SetState(transaction.StateVerify); err != nil {
			return "", fmt.Errorf("merge: journal: %w", err)
		}
		if err := advanceRef(ctx, resolver, out.NewCommitID); err != nil {
			return "", fmt.Errorf("merge: advance ref: %w", err)
		}
		txn.TrackRef("HEAD", out.NewCommitID.String())
		appendReflog(ctx, ourCommitID, out.NewCommitID,
			fmt.Sprintf("merge %s: Fast-forward", theirLabel))
		if err := txn.Commit(); err != nil {
			return "", err
		}
		graphindex.Invalidate(ctx.Repository.VaraDir)
		return fmt.Sprintf("Fast-forward\nUpdated to %s\n", out.NewCommitID.String()[:7]), nil

	default: // Success — true three-way merge, no conflicts.
		if err := txn.SetState(transaction.StateVerify); err != nil {
			return "", fmt.Errorf("merge: journal: %w", err)
		}

		// Create the merge commit.
		treeID, err := builder.BuildTree(ctx.Index, store)
		if err != nil {
			return "", fmt.Errorf("merge: build tree: %w", err)
		}
		mergeCommitID, err := builder.BuildCommit(
			store,
			types.TreeID(treeID),
			[]types.CommitID{ourCommitID, theirCommitID},
			"User <user@example.com>",
			fmt.Sprintf("Merge branch '%s'", theirLabel),
		)
		if err != nil {
			return "", fmt.Errorf("merge: build commit: %w", err)
		}

		if err := writeIndex(ctx); err != nil {
			return "", fmt.Errorf("merge: write index: %w", err)
		}
		if err := advanceRef(ctx, resolver, mergeCommitID); err != nil {
			return "", fmt.Errorf("merge: advance ref: %w", err)
		}
		txn.TrackRef("HEAD", mergeCommitID.String())
		appendReflog(ctx, ourCommitID, mergeCommitID,
			fmt.Sprintf("merge %s: Merge made by 'recursive'", theirLabel))
		if err := txn.Commit(); err != nil {
			return "", err
		}
		graphindex.Invalidate(ctx.Repository.VaraDir)
		return fmt.Sprintf("Merge made by 'recursive'.\n[%s] Merge branch '%s'\n",
			mergeCommitID.String()[:7], theirLabel), nil
	}
}

func writeIndex(ctx *Context) error {
	data, err := ctx.Index.Serialize()
	if err != nil {
		return err
	}
	indexPath := filepath.Join(ctx.Repository.VaraDir, "index")
	return atomicWriteFile(indexPath, data, 0644)
}

// advanceRef updates the current branch (or detached HEAD) to point to newID.
func advanceRef(ctx *Context, resolver *refs.FSResolver, newID types.CommitID) error {
	currentRef, err := resolver.ResolveSymbolic("HEAD")
	if err != nil {
		// Detached HEAD: update HEAD directly.
		headPath := filepath.Join(ctx.Repository.VaraDir, "HEAD")
		return atomicWriteFile(headPath, []byte(newID.String()+"\n"), 0644)
	}
	return resolver.Update(currentRef, newID)
}

func appendReflog(ctx *Context, oldID, newID types.CommitID, message string) {
	rm := reflog.NewManager(ctx.Repository.VaraDir)
	rm.Append("HEAD", oldID, newID, "User <user@example.com>", message)
}
