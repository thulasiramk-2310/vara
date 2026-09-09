package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/thulasiramk-2310/vara/internal/conflict"
	"github.com/thulasiramk-2310/vara/internal/locking"
	mergeengine "github.com/thulasiramk-2310/vara/internal/merge"
	"github.com/thulasiramk-2310/vara/internal/mergestate"
	"github.com/thulasiramk-2310/vara/internal/smartmerge"
	"github.com/thulasiramk-2310/vara/internal/transaction"
	"github.com/thulasiramk-2310/vara/pkg/builder"
	"github.com/thulasiramk-2310/vara/pkg/graph"
	"github.com/thulasiramk-2310/vara/pkg/graphindex"
	"github.com/thulasiramk-2310/vara/pkg/index"
	"github.com/thulasiramk-2310/vara/pkg/object"
	"github.com/thulasiramk-2310/vara/pkg/recovery"
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
		// Re-decide every engine-flagged conflict above the frozen engine, from the
		// stored base/ours/theirs blobs, using the base-aware (order-independent)
		// renderer. This (a) fixes the engine's ours/theirs order-dependence on
		// adjacent edits, (b) auto-settles disjoint edits the engine over-reports,
		// (c) re-renders genuine conflicts in the configured marker style (zdiff3),
		// and (d) records the content-bearing sidecar for the paths that truly
		// remain conflicted. It also mutates the working tree + index for content
		// paths so what the user sees matches what commit will gate on.
		style := configuredMarkerStyle(ctx)
		state, allSettled, refErr := refineConflicts(ctx, store, ourCommitID, theirCommitID, ourLabel, theirLabel, out.Conflicts, style)
		if refErr != nil {
			// Fallback: leave the engine's markers in place and record a best-effort
			// (record-only) sidecar, preserving the prior behavior.
			fmt.Fprintf(os.Stderr, "vara: warning: could not refine conflicts (%v); leaving engine markers\n", refErr)
			if err := writeIndex(ctx); err != nil {
				return "", fmt.Errorf("merge: write index: %w", err)
			}
			if err := writeConflictSidecar(ctx, store, ourCommitID, theirCommitID, ourLabel, theirLabel, out.Conflicts); err != nil {
				fmt.Fprintf(os.Stderr, "vara: warning: could not record conflict details: %v\n", err)
			}
			if err := txn.Commit(); err != nil {
				return "", err
			}
			return conflictSummary(out.Conflicts), nil
		}

		if allSettled {
			// Every conflict auto-resolved and nothing structural remains — complete
			// the merge as a normal two-parent commit (clearing the MERGE_HEAD the
			// engine wrote). This is what makes disjoint-adjacent edits "just work".
			mergeCommitID, err := concludeMerge(ctx, store, resolver, txn, ourCommitID, theirCommitID, theirLabel, true)
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("Auto-merged all conflicts.\nMerge made by 'recursive'.\n[%s] Merge branch '%s'\n",
				mergeCommitID.String()[:7], theirLabel), nil
		}

		if err := writeIndex(ctx); err != nil {
			return "", fmt.Errorf("merge: write index: %w", err)
		}
		if err := mergestate.Write(ctx.Repository.VaraDir, state); err != nil {
			fmt.Fprintf(os.Stderr, "vara: warning: could not record conflict details: %v\n", err)
		}
		if err := txn.Commit(); err != nil {
			return "", err
		}
		return conflictSummary(state.Paths()), nil

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
		mergeCommitID, err := concludeMerge(ctx, store, resolver, txn, ourCommitID, theirCommitID, theirLabel, false)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("Merge made by 'recursive'.\n[%s] Merge branch '%s'\n",
			mergeCommitID.String()[:7], theirLabel), nil
	}
}

// concludeMerge builds the two-parent merge commit from the current index and
// advances HEAD, inside the caller's transaction. When clearMergeHead is set (the
// auto-resolved-conflict path), it also clears MERGE_HEAD and the conflict sidecar
// the engine left behind. It is shared by the clean-merge (Success) path and the
// all-conflicts-auto-resolved path so both seal history identically.
func concludeMerge(ctx *Context, store *object.Store, resolver *refs.FSResolver, txn *transaction.Transaction, our, their types.CommitID, theirLabel string, clearMergeHead bool) (types.CommitID, error) {
	if err := txn.SetState(transaction.StateVerify); err != nil {
		return types.CommitID{}, fmt.Errorf("merge: journal: %w", err)
	}
	treeID, err := builder.BuildTree(ctx.Index, store)
	if err != nil {
		return types.CommitID{}, fmt.Errorf("merge: build tree: %w", err)
	}
	author, err := commitAuthor(ctx.Repository.VaraDir)
	if err != nil {
		return types.CommitID{}, err
	}
	mergeCommitID, err := builder.BuildCommit(
		store,
		types.TreeID(treeID),
		[]types.CommitID{our, their},
		author,
		fmt.Sprintf("Merge branch '%s'", theirLabel),
	)
	if err != nil {
		return types.CommitID{}, fmt.Errorf("merge: build commit: %w", err)
	}
	if err := writeIndex(ctx); err != nil {
		return types.CommitID{}, fmt.Errorf("merge: write index: %w", err)
	}
	if clearMergeHead {
		if err := recovery.ClearMergeHead(ctx.Repository.VaraDir); err != nil {
			return types.CommitID{}, fmt.Errorf("merge: clear MERGE_HEAD: %w", err)
		}
		_ = mergestate.Clear(ctx.Repository.VaraDir)
	}
	if err := advanceRef(ctx, resolver, mergeCommitID); err != nil {
		return types.CommitID{}, fmt.Errorf("merge: advance ref: %w", err)
	}
	txn.TrackRef("HEAD", mergeCommitID.String())
	appendReflog(ctx, our, mergeCommitID,
		fmt.Sprintf("merge %s: Merge made by 'recursive'", theirLabel))
	if err := txn.Commit(); err != nil {
		return types.CommitID{}, err
	}
	graphindex.Invalidate(ctx.Repository.VaraDir)
	return mergeCommitID, nil
}

// conflictSummary renders the "Automatic merge failed" message listing the
// still-unresolved conflicted paths (sorted).
func conflictSummary(paths []string) string {
	sorted := append([]string(nil), paths...)
	sort.Strings(sorted)
	var sb strings.Builder
	sb.WriteString("Automatic merge failed; fix conflicts and then commit.\n")
	sb.WriteString("Conflicts:\n")
	for _, c := range sorted {
		sb.WriteString("\t" + c + "\n")
	}
	return sb.String()
}

// configuredMarkerStyle resolves the conflict marker style: the repo config
// 'merge.conflictStyle' (merge|diff3|zdiff3) if set and valid, else zdiff3.
func configuredMarkerStyle(ctx *Context) conflict.MarkerStyle {
	if cfg, err := loadConfig(ctx.Repository.VaraDir); err == nil {
		if v, ok := cfg.Get("merge", "", "conflictStyle"); ok {
			if s, ok := conflict.ParseMarkerStyle(v); ok {
				return s
			}
		}
	}
	return conflict.StyleZdiff3
}

// writeConflictSidecar records the three sides of every conflicted path into
// .vara/CONFLICTS. It recomputes the merge base with the same public engine API
// the merge used, then reads each path out of the base / ours / theirs trees. An
// empty blob ID means that side does not contain the path.
func writeConflictSidecar(ctx *Context, store *object.Store, ourCommit, theirCommit types.CommitID, ourLabel, theirLabel string, conflicts []string) error {
	varaDir := ctx.Repository.VaraDir
	baseCommit, err := graph.MergeBase(store, ourCommit, theirCommit)
	if err != nil {
		return fmt.Errorf("find merge base: %w", err)
	}

	sideBlobs := func(c types.CommitID) (map[string]types.BlobID, error) {
		tree, err := commitTree(store, c)
		if err != nil {
			return nil, err
		}
		return treeBlobs(store, tree)
	}
	baseMap, err := sideBlobs(baseCommit)
	if err != nil {
		return err
	}
	ourMap, err := sideBlobs(ourCommit)
	if err != nil {
		return err
	}
	theirMap, err := sideBlobs(theirCommit)
	if err != nil {
		return err
	}

	sideOf := func(m map[string]types.BlobID, p string) mergestate.Side {
		if id, ok := m[p]; ok {
			return mergestate.Side{Present: true, Blob: id.String()}
		}
		return mergestate.Side{Present: false}
	}

	st := &mergestate.State{
		Version:    mergestate.Version,
		OurLabel:   ourLabel,
		TheirLabel: theirLabel,
	}
	for _, p := range conflicts {
		ours := sideOf(ourMap, p)
		theirs := sideOf(theirMap, p)
		// Both sides present → content conflict (edit/edit or add/add), resolvable
		// by re-rendering the sides. Exactly one side present → modify/delete, a
		// structural conflict that only an explicit ours/theirs choice can settle.
		kind := mergestate.KindContent
		if ours.Present != theirs.Present {
			kind = mergestate.KindModifyDelete
		}
		st.Entries = append(st.Entries, mergestate.Entry{
			Path:   p,
			Kind:   kind,
			Base:   sideOf(baseMap, p),
			Ours:   ours,
			Theirs: theirs,
		})
	}

	// MergeTouched = every path the merge changed relative to pre-merge HEAD
	// (ourMap is the HEAD tree, since ourCommit is HEAD). This is the symmetric
	// difference between the post-merge index and HEAD: cleanly-merged files,
	// merge-added files, and merge-deleted files. Abort uses it to distinguish a
	// merge-authored change from one the user makes afterward.
	st.MergeTouched = mergeTouchedPaths(ctx.Index, ourMap)

	return mergestate.Write(varaDir, st)
}

// refineConflicts re-decides every engine-flagged conflict above the frozen engine.
// For each content path it re-renders from the stored base/ours/theirs blobs with
// the base-aware, order-independent renderer in the configured style, writing the
// result to the working tree and index. It returns the sidecar State holding only
// the paths that genuinely remain conflicted, and allSettled=true when nothing does
// (every content conflict auto-resolved and no modify/delete remains), in which case
// the caller finalizes a clean merge commit. modify/delete paths are structural and
// carried through unchanged; binary content is never line-merged (ours is kept and
// the conflict recorded for an explicit ours/theirs choice).
func refineConflicts(ctx *Context, store *object.Store, ourCommit, theirCommit types.CommitID, ourLabel, theirLabel string, conflicts []string, style conflict.MarkerStyle) (*mergestate.State, bool, error) {
	baseCommit, err := graph.MergeBase(store, ourCommit, theirCommit)
	if err != nil {
		return nil, false, fmt.Errorf("find merge base: %w", err)
	}
	sideBlobs := func(c types.CommitID) (map[string]types.BlobID, error) {
		tree, err := commitTree(store, c)
		if err != nil {
			return nil, err
		}
		return treeBlobs(store, tree)
	}
	baseMap, err := sideBlobs(baseCommit)
	if err != nil {
		return nil, false, err
	}
	ourMap, err := sideBlobs(ourCommit)
	if err != nil {
		return nil, false, err
	}
	theirMap, err := sideBlobs(theirCommit)
	if err != nil {
		return nil, false, err
	}

	sideOf := func(m map[string]types.BlobID, p string) mergestate.Side {
		if id, ok := m[p]; ok {
			return mergestate.Side{Present: true, Blob: id.String()}
		}
		return mergestate.Side{Present: false}
	}
	bytesOf := func(s mergestate.Side) ([]byte, error) {
		if !s.Present {
			return nil, nil
		}
		id, ok := parseBlobID(s.Blob)
		if !ok {
			return nil, fmt.Errorf("bad blob id %q", s.Blob)
		}
		return blobContent(store, id)
	}

	st := &mergestate.State{Version: mergestate.Version, OurLabel: ourLabel, TheirLabel: theirLabel}
	cfg, _ := loadConfig(ctx.Repository.VaraDir) // nil on error → structural drivers stay off
	for _, p := range conflicts {
		base := sideOf(baseMap, p)
		ours := sideOf(ourMap, p)
		theirs := sideOf(theirMap, p)

		if ours.Present != theirs.Present {
			// modify/delete — structural; leave the engine's worktree (ours kept).
			st.Entries = append(st.Entries, mergestate.Entry{Path: p, Kind: mergestate.KindModifyDelete, Base: base, Ours: ours, Theirs: theirs})
			continue
		}

		bb, err := bytesOf(base)
		if err != nil {
			return nil, false, err
		}
		ob, err := bytesOf(ours)
		if err != nil {
			return nil, false, err
		}
		tb, err := bytesOf(theirs)
		if err != nil {
			return nil, false, err
		}

		if conflict.IsBinary(bb) || conflict.IsBinary(ob) || conflict.IsBinary(tb) {
			// Never line-merge binary — keep ours in the worktree, record unresolved.
			if err := restageResolved(ctx, store, p, ob); err != nil {
				return nil, false, err
			}
			st.Entries = append(st.Entries, mergestate.Entry{Path: p, Kind: mergestate.KindContent, Base: base, Ours: ours, Theirs: theirs})
			continue
		}

		// Structural (semantic) merge, opt-in per type (RFC-0025). Engages only for
		// registered types the repo enabled; a clean structural merge auto-resolves
		// what the line merge false-conflicted on. On parse failure or a genuine
		// same-leaf conflict it falls through to the line merge below.
		if driver := smartmerge.SelectDriver(p, cfg); driver != "" {
			r := smartmerge.Merge(driver, bb, ob, tb, ourLabel, theirLabel)
			if r.ParseOK && r.Conflicts == 0 {
				if err := restageResolved(ctx, store, p, r.Merged); err != nil {
					return nil, false, err
				}
				continue // clean structural auto-resolve — not a conflict
			}
			if r.ParseOK && r.Rendered {
				// Genuine same-key divergence: per-key markers in the worktree,
				// recorded unresolved so commit still gates on it.
				if err := restageResolved(ctx, store, p, r.Merged); err != nil {
					return nil, false, err
				}
				st.Entries = append(st.Entries, mergestate.Entry{Path: p, Kind: mergestate.KindContent, Base: base, Ours: ours, Theirs: theirs})
				continue
			}
			// parse failed, or a conflict with no structural renderer (YAML) →
			// fall through to the line merge below.
		}

		merged, stats := conflict.Render(bb, ob, tb, ourLabel, theirLabel, style)
		if err := restageResolved(ctx, store, p, merged); err != nil {
			return nil, false, err
		}
		if stats.Remaining == 0 {
			continue // fully auto-resolved — not a conflict
		}
		st.Entries = append(st.Entries, mergestate.Entry{Path: p, Kind: mergestate.KindContent, Base: base, Ours: ours, Theirs: theirs})
	}

	st.MergeTouched = mergeTouchedPaths(ctx.Index, ourMap)
	return st, len(st.Entries) == 0, nil
}

// mergeTouchedPaths returns the paths where the post-merge index differs from the
// pre-merge HEAD tree.
func mergeTouchedPaths(idx *index.Index, headMap map[string]types.BlobID) []string {
	idxMap := map[string]types.BlobID{}
	for _, e := range idx.Entries {
		if e.State == index.StateDeleted {
			continue
		}
		idxMap[e.Path] = e.ObjectID
	}
	touched := map[string]bool{}
	for p, id := range idxMap {
		if h, ok := headMap[p]; !ok || h != id {
			touched[p] = true
		}
	}
	for p := range headMap {
		if _, ok := idxMap[p]; !ok {
			touched[p] = true
		}
	}
	out := make([]string, 0, len(touched))
	for p := range touched {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
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
	rm.Append("HEAD", oldID, newID, reflogActor(ctx.Repository.VaraDir), message)
}
