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

// RunCommit executes the `vara commit` pipeline.
func RunCommit(ctx *Context, message string) (types.CommitID, error) {
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
