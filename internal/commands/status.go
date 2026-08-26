package commands

import (
	"fmt"
	"sort"
	"strings"

	"github.com/thulasiramk-2310/vara/internal/mergestate"
	"github.com/thulasiramk-2310/vara/internal/status"
	"github.com/thulasiramk-2310/vara/internal/worktree"
	"github.com/thulasiramk-2310/vara/pkg/object"
	"github.com/thulasiramk-2310/vara/pkg/recovery"
	"github.com/thulasiramk-2310/vara/pkg/refs"
	"github.com/thulasiramk-2310/vara/pkg/scanner"
)

// RunStatus executes the `vara status` pipeline.
func RunStatus(ctx *Context) (string, error) {
	wt, err := worktree.New(ctx.Repository.RootDir)
	if err != nil {
		return "", fmt.Errorf("failed to init worktree: %v", err)
	}

	s := scanner.New(ctx.Index)
	res, err := s.Scan(wt)
	if err != nil {
		return "", fmt.Errorf("scan failed: %v", err)
	}

	sr := status.FromScanner(res)

	// Overlay in-progress-merge state from the conflict sidecar. It is the
	// authoritative source of conflicted/resolved paths: marker-less modify/delete
	// and add/add conflicts leave the worktree looking merely modified or clean, so
	// the scanner alone cannot surface them. Paths the merge owns are moved out of
	// the ordinary buckets so they are reported once, under the merge sections.
	ms, owned, err := buildMergeStatus(ctx)
	if err != nil {
		return "", err
	}
	if ms != nil {
		sr.Merge = ms
		stripPaths(sr, owned)
	}

	branch := currentBranch(ctx)
	return status.FormatLong(sr, branch), nil
}

// buildMergeStatus reads the conflict sidecar and returns the merge overlay for
// status plus the set of paths the merge owns (to strip from other buckets).
// Returns (nil, nil, nil) when no merge is in progress.
func buildMergeStatus(ctx *Context) (*status.MergeStatus, map[string]bool, error) {
	st, ok, err := mergestate.Read(ctx.Repository.VaraDir)
	if err != nil {
		return nil, nil, fmt.Errorf("status: read conflict state: %w", err)
	}
	owned := map[string]bool{}

	if ok {
		store := object.NewStore(ctx.Repository.VaraDir)
		ms := &status.MergeStatus{InProgress: true}
		for _, e := range st.Entries {
			owned[e.Path] = true
			if e.Resolved {
				ms.Resolved = append(ms.Resolved, e.Path)
				continue
			}
			line := conflictLine(e)
			// Flag binary content conflicts — they can't be line-merged, so the
			// user must pick a whole side.
			if e.Kind == mergestate.KindContent {
				if bin, _ := entryIsBinary(store, e); bin {
					line.Long += " (binary)"
				}
			}
			ms.Unmerged = append(ms.Unmerged, line)
		}
		sort.Strings(ms.Resolved)
		sort.Slice(ms.Unmerged, func(i, j int) bool { return ms.Unmerged[i].Path < ms.Unmerged[j].Path })
		return ms, owned, nil
	}

	// Legacy: MERGE_HEAD exists but no sidecar (a merge begun by an older binary).
	// Surface at least that a merge is in progress, listing marker-bearing tracked
	// files as both-modified — the same restricted scan `vara resolve` falls back to.
	if recovery.MergeInProgress(ctx.Repository.VaraDir) {
		files, err := findConflictedFiles(ctx)
		if err != nil {
			return nil, nil, err
		}
		ms := &status.MergeStatus{InProgress: true}
		for _, f := range files {
			owned[f.path] = true
			ms.Unmerged = append(ms.Unmerged, status.ConflictLine{Path: f.path, Long: "both modified", Short: "UU"})
		}
		return ms, owned, nil
	}

	return nil, owned, nil
}

// conflictLine classifies a sidecar entry into a git-style unmerged display line.
func conflictLine(e mergestate.Entry) status.ConflictLine {
	if e.Kind == mergestate.KindModifyDelete {
		if e.Ours.Present && !e.Theirs.Present {
			return status.ConflictLine{Path: e.Path, Long: "deleted by them", Short: "UD"}
		}
		return status.ConflictLine{Path: e.Path, Long: "deleted by us", Short: "DU"}
	}
	// Content conflict: no base means both sides added the path.
	if !e.Base.Present {
		return status.ConflictLine{Path: e.Path, Long: "both added", Short: "AA"}
	}
	return status.ConflictLine{Path: e.Path, Long: "both modified", Short: "UU"}
}

// stripPaths removes merge-owned paths from the ordinary status buckets so they
// are only reported once, under the merge sections.
func stripPaths(sr *status.StatusResult, owned map[string]bool) {
	filter := func(in []string) []string {
		out := in[:0:0]
		for _, p := range in {
			if !owned[p] {
				out = append(out, p)
			}
		}
		return out
	}
	sr.Clean = filter(sr.Clean)
	sr.Modified = filter(sr.Modified)
	sr.Staged = filter(sr.Staged)
	sr.Deleted = filter(sr.Deleted)
	sr.Untracked = filter(sr.Untracked)
}

// currentBranch returns the name of the current branch, or empty string if
// HEAD is detached or the ref cannot be read.
func currentBranch(ctx *Context) string {
	resolver := refs.NewFSResolver(ctx.Repository.VaraDir)
	target, err := resolver.ResolveSymbolic("HEAD")
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(target, "refs/heads/")
}
