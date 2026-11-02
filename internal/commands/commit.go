package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/thulasiramk-2310/vara/internal/conflict"
	"github.com/thulasiramk-2310/vara/internal/devidentity"
	"github.com/thulasiramk-2310/vara/internal/mergestate"
	"github.com/thulasiramk-2310/vara/pkg/builder"
	"github.com/thulasiramk-2310/vara/pkg/graphindex"
	"github.com/thulasiramk-2310/vara/pkg/index"
	"github.com/thulasiramk-2310/vara/pkg/object"
	"github.com/thulasiramk-2310/vara/pkg/recovery"
	"github.com/thulasiramk-2310/vara/pkg/refs"
	"github.com/thulasiramk-2310/vara/pkg/types"
)

// commitAuthor resolves the identity to record as a commit's author/committer
// (RFC-0017). A missing identity is a hard, actionable error — VARA never
// invents a placeholder author.
func commitAuthor(varaDir string) (string, error) {
	id, _, err := devidentity.Resolve(varaDir)
	if err != nil {
		if _, ok := err.(devidentity.ErrNoIdentity); ok {
			return "", fmt.Errorf("no VARA identity configured.\n\n" +
				"Configure one with:\n\n" +
				"  vara identity set --default \\\n" +
				"    --name \"Your Name\" \\\n" +
				"    --email \"you@example.com\"")
		}
		return "", err
	}
	return id.String(), nil
}

// reflogActor resolves the identity for a local reflog entry. Unlike a commit
// author, the reflog is local-only metadata, so a missing identity must not
// fail an offline operation (switch/branch): it falls back to a neutral,
// non-fabricated marker rather than an error or a fake email.
func reflogActor(varaDir string) string {
	if id, ok := devidentity.BestEffort(varaDir); ok {
		return id.String()
	}
	return "unknown <unknown>"
}

// CommitOptions tunes `vara commit`.
type CommitOptions struct {
	// AllowEmpty records a commit even when nothing differs from the parent.
	AllowEmpty bool
}

// RunCommit executes the `vara commit` pipeline. It refuses to record a commit that changes
// nothing; use RunCommitWith and AllowEmpty for a deliberate empty commit.
func RunCommit(ctx *Context, message string) (types.CommitID, error) {
	return RunCommitWith(ctx, message, CommitOptions{})
}

// RunCommitWith is RunCommit with options.
func RunCommitWith(ctx *Context, message string, opts CommitOptions) (types.CommitID, error) {
	if message == "" {
		return types.CommitID{}, fmt.Errorf("aborting commit due to empty commit message")
	}

	// A merge in progress must be fully resolved before it can be committed —
	// refuse while any staged file still carries conflict markers, so a merge is
	// never sealed with unresolved <<<<<<< markers baked into history.
	merging := recovery.MergeInProgress(ctx.Repository.VaraDir)
	if merging {
		if unresolved := unresolvedConflicts(ctx); len(unresolved) > 0 {
			return types.CommitID{}, fmt.Errorf(
				"cannot commit: unresolved conflicts in:\n\t%s\n\n"+
					"Fix them with 'vara resolve' (or edit by hand), then commit.",
				strings.Join(unresolved, "\n\t"))
		}
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

	// 2b. If a merge is in progress, MERGE_HEAD is the second parent — this is
	// what turns the follow-up commit into a real merge commit rather than a
	// single-parent commit that silently drops the merge relationship.
	if merging {
		if p, ok := readMergeHead(ctx.Repository.VaraDir); ok {
			parents = append(parents, p)
		}
	}

	// 2c. Nothing new to record is almost always a slip (a second Enter, a forgotten add), and
	// would only add a duplicate commit. A merge commit is the exception: it is meaningful even
	// when its tree equals its first parent's.
	if !merging && !opts.AllowEmpty {
		same := false
		if len(parents) == 0 {
			same = len(ctx.Index.Entries) == 0
		} else if pt, perr := commitTree(store, parents[0]); perr == nil {
			same = pt == types.TreeID(treeID)
		}
		if same {
			return types.CommitID{}, fmt.Errorf("nothing to commit; use 'vara add' first, or --allow-empty to record an empty commit")
		}
	}

	// 3. Build the Commit. The author is the resolved developer identity
	// (RFC-0017): repository identity, else user default, else a hard error —
	// never a fabricated placeholder, and the account ID never enters the commit.
	author, err := commitAuthor(ctx.Repository.VaraDir)
	if err != nil {
		return types.CommitID{}, err
	}
	commitID, err := builder.BuildCommit(store, types.TreeID(treeID), parents, author, message)
	if err != nil {
		return types.CommitID{}, fmt.Errorf("failed to build commit: %v", err)
	}

	// 4. Move HEAD's branch (or HEAD itself when detached) to the new commit, through the
	// reference updater: lock file, flush, atomic rename (RFC-0004 §4). A failure is an error,
	// never a success message over a branch that did not move.
	resolver := refs.NewFSResolver(ctx.Repository.VaraDir)
	if len(headData) > 0 {
		// Update follows a symbolic HEAD to its branch, and writes HEAD directly when detached.
		if uerr := resolver.Update("HEAD", commitID); uerr != nil {
			return types.CommitID{}, fmt.Errorf("commit %s was recorded but the branch could not be updated: %w",
				commitID.String()[:7], uerr)
		}
	} else {
		// No HEAD yet: start on main.
		const mainRef = "refs/heads/main"
		if uerr := resolver.Update(mainRef, commitID); uerr != nil {
			return types.CommitID{}, fmt.Errorf("commit %s was recorded but %s could not be created: %w",
				commitID.String()[:7], mainRef, uerr)
		}
		if uerr := resolver.Symbolic("HEAD", mainRef); uerr != nil {
			return types.CommitID{}, fmt.Errorf("commit %s was recorded but HEAD could not be set: %w",
				commitID.String()[:7], uerr)
		}
	}

	// 4b. Record the commit in the HEAD reflog, which undo reads (RFC-0009 §6).
	var oldID types.CommitID
	if len(parents) > 0 {
		oldID = parents[0]
	}
	subject, _, _ := strings.Cut(message, "\n")
	appendReflog(ctx, oldID, commitID, "commit: "+subject)

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
	idxData, ierr := ctx.Index.Serialize()
	if ierr != nil {
		return types.CommitID{}, fmt.Errorf("commit %s was recorded but the index could not be saved: %w", commitID.String()[:7], ierr)
	}
	if werr := atomicWriteFile(indexPath, idxData, 0644); werr != nil {
		return types.CommitID{}, fmt.Errorf("commit %s was recorded but the index could not be saved: %w", commitID.String()[:7], werr)
	}

	// Invalidate the graph index — next history traversal will rebuild it.
	graphindex.Invalidate(ctx.Repository.VaraDir)

	// The merge is now sealed into a two-parent commit; clear both MERGE_HEAD and
	// the conflict sidecar so the repository is no longer "merging".
	if merging {
		_ = recovery.ClearMergeHead(ctx.Repository.VaraDir)
		_ = mergestate.Clear(ctx.Repository.VaraDir)
	}

	return commitID, nil
}

// readMergeHead reads .vara/MERGE_HEAD and returns the recorded commit ID.
func readMergeHead(varaDir string) (types.CommitID, bool) {
	data, err := os.ReadFile(filepath.Join(varaDir, "MERGE_HEAD"))
	if err != nil {
		return types.CommitID{}, false
	}
	p, err := types.ParseHex(strings.TrimSpace(string(data)))
	if err != nil {
		return types.CommitID{}, false
	}
	return types.CommitID(p), true
}

// unresolvedConflicts returns the merge-involved files that are not yet resolved,
// so a merge commit can be refused until they are.
//
// The conflict sidecar (.vara/CONFLICTS) is the source of truth: an entry's
// Resolved flag, set by `vara resolve`, is authoritative. Marker scanning cannot
// stand in for it — modify/delete and add/add conflicts carry no markers in the
// working tree, so a scan would wrongly report them resolved and let the commit
// silently bake in the "ours" side. When no sidecar exists (a merge begun by an
// older binary), it falls back to scanning tracked files for markers so the
// historical guard still holds for content conflicts.
func unresolvedConflicts(ctx *Context) []string {
	if st, ok, err := mergestate.Read(ctx.Repository.VaraDir); err == nil && ok {
		out := st.UnresolvedPaths()
		sort.Strings(out)
		return out
	}

	var out []string
	seen := map[string]bool{}
	for _, e := range ctx.Index.Entries {
		if e.State == index.StateDeleted || seen[e.Path] {
			continue
		}
		seen[e.Path] = true
		abs := filepath.Join(ctx.Repository.RootDir, filepath.FromSlash(e.Path))
		data, err := os.ReadFile(abs)
		if err != nil {
			continue
		}
		if conflict.HasMarkers(data) {
			out = append(out, e.Path)
		}
	}
	sort.Strings(out)
	return out
}
