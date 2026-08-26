// Package commands — vara diff.
//
// Shows line-level changes as a unified diff: the working tree against the index
// (default), or the index against HEAD (--staged). It is conflict-aware: while a
// merge is in progress it first surfaces the unmerged paths from the sidecar and
// shows each as an ours-vs-theirs diff, since a plain working-tree diff of a file
// full of conflict markers is noise. The line-level diff reuses the base-aware
// LCS in internal/conflict (FormatUnified) — no engine change.
package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/thulasiramk-2310/vara/internal/conflict"
	"github.com/thulasiramk-2310/vara/internal/mergestate"
	"github.com/thulasiramk-2310/vara/pkg/index"
	"github.com/thulasiramk-2310/vara/pkg/object"
	"github.com/thulasiramk-2310/vara/pkg/recovery"
	"github.com/thulasiramk-2310/vara/pkg/refs"
	"github.com/thulasiramk-2310/vara/pkg/types"
)

// DiffArgs carries the parsed options for `vara diff`.
type DiffArgs struct {
	Staged bool     // compare index vs HEAD instead of working tree vs index
	Paths  []string // optional pathspecs to restrict to
}

// RunDiff executes `vara diff`.
func RunDiff(ctx *Context, da DiffArgs) (string, error) {
	store := object.NewStore(ctx.Repository.VaraDir)
	var sb strings.Builder

	// Conflict-aware preface: show unmerged paths (ours vs theirs) from the sidecar.
	unmerged := map[string]bool{}
	if recovery.MergeInProgress(ctx.Repository.VaraDir) {
		if st, ok, _ := mergestate.Read(ctx.Repository.VaraDir); ok {
			writeUnmergedDiff(&sb, store, st, da.Paths, unmerged)
		}
	}

	// Build the old→new content maps for the requested comparison.
	pairs, err := diffPairs(ctx, store, da.Staged)
	if err != nil {
		return "", err
	}

	paths := make([]string, 0, len(pairs))
	for p := range pairs {
		if unmerged[p] {
			continue // already shown in the conflict preface
		}
		if len(da.Paths) > 0 && !matchesAny(p, da.Paths) {
			continue
		}
		paths = append(paths, p)
	}
	sort.Strings(paths)

	for _, p := range paths {
		pr := pairs[p]
		d := conflict.FormatUnified(pr.old, pr.new, "a/"+p, "b/"+p)
		if len(d) == 0 {
			continue
		}
		sb.WriteString(diffHeader(p, pr))
		sb.Write(d)
	}

	if sb.Len() == 0 {
		return "", nil
	}
	return sb.String(), nil
}

// contentPair holds the two sides of a file-level comparison.
type contentPair struct {
	old, new         []byte
	oldMiss, newMiss bool // side does not contain the path (added/deleted)
}

// diffPairs builds path → (old, new) content for the requested comparison.
func diffPairs(ctx *Context, store *object.Store, staged bool) (map[string]contentPair, error) {
	pairs := map[string]contentPair{}

	idxBlobs := map[string]index.Entry{}
	for _, e := range ctx.Index.Entries {
		if e.State == index.StateDeleted {
			continue
		}
		idxBlobs[e.Path] = e
	}

	if staged {
		// index vs HEAD.
		headBlobs, _ := headBlobMap(ctx, store)
		for p, e := range idxBlobs {
			newC, err := blobContent(store, e.ObjectID)
			if err != nil {
				return nil, err
			}
			var oldC []byte
			miss := true
			if id, ok := headBlobs[p]; ok {
				oldC, err = blobContent(store, id)
				if err != nil {
					return nil, err
				}
				miss = false
			}
			pairs[p] = contentPair{old: oldC, new: newC, oldMiss: miss}
		}
		for p, id := range headBlobs {
			if _, ok := idxBlobs[p]; ok {
				continue
			}
			oldC, err := blobContent(store, id)
			if err != nil {
				return nil, err
			}
			pairs[p] = contentPair{old: oldC, new: nil, newMiss: true}
		}
		return pairs, nil
	}

	// working tree vs index.
	for p, e := range idxBlobs {
		oldC, err := blobContent(store, e.ObjectID)
		if err != nil {
			return nil, err
		}
		abs := filepath.Join(ctx.Repository.RootDir, filepath.FromSlash(p))
		newC, readErr := os.ReadFile(abs)
		if readErr != nil {
			pairs[p] = contentPair{old: oldC, new: nil, newMiss: true} // deleted on disk
			continue
		}
		pairs[p] = contentPair{old: oldC, new: newC}
	}
	return pairs, nil
}

// headBlobMap returns HEAD's path→blob map, or an empty map when HEAD is unborn.
func headBlobMap(ctx *Context, store *object.Store) (map[string]types.BlobID, error) {
	resolver := refs.NewFSResolver(ctx.Repository.VaraDir)
	headCommit, err := resolver.Resolve("HEAD")
	if err != nil {
		return map[string]types.BlobID{}, nil // unborn HEAD — everything is "new"
	}
	tree, err := commitTree(store, headCommit)
	if err != nil {
		return nil, err
	}
	return treeBlobs(store, tree)
}

// diffHeader renders the git-style file header line for a change.
func diffHeader(path string, pr contentPair) string {
	switch {
	case pr.oldMiss:
		return fmt.Sprintf("diff --vara a/%s b/%s\nnew file\n", path, path)
	case pr.newMiss:
		return fmt.Sprintf("diff --vara a/%s b/%s\ndeleted file\n", path, path)
	default:
		return fmt.Sprintf("diff --vara a/%s b/%s\n", path, path)
	}
}

// writeUnmergedDiff appends the conflict preface: each unmerged path as ours vs
// theirs, and records the paths shown so the normal diff skips them.
func writeUnmergedDiff(sb *strings.Builder, store *object.Store, st *mergestate.State, specs []string, shown map[string]bool) {
	var entries []mergestate.Entry
	for _, e := range st.Entries {
		if len(specs) > 0 && !matchesAny(e.Path, specs) {
			continue
		}
		entries = append(entries, e)
	}
	if len(entries) == 0 {
		return
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	sb.WriteString("Unmerged paths (ours vs theirs):\n")
	for _, e := range entries {
		shown[e.Path] = true
		ours := sideBytes(store, e.Ours)
		theirs := sideBytes(store, e.Theirs)
		fmt.Fprintf(sb, "* %s (%s)\n", e.Path, unmergedLabel(e))
		d := conflict.FormatUnified(ours, theirs, "ours/"+e.Path, "theirs/"+e.Path)
		if len(d) > 0 {
			sb.Write(d)
		}
	}
	sb.WriteString("\n")
}

// sideBytes returns a stored side's bytes, or nil when the side is absent.
func sideBytes(store *object.Store, s mergestate.Side) []byte {
	if !s.Present {
		return nil
	}
	id, ok := parseBlobID(s.Blob)
	if !ok {
		return nil
	}
	b, err := blobContent(store, id)
	if err != nil {
		return nil
	}
	return b
}

func unmergedLabel(e mergestate.Entry) string {
	if e.Kind == mergestate.KindModifyDelete {
		if e.Ours.Present && !e.Theirs.Present {
			return "deleted by them"
		}
		return "deleted by us"
	}
	if !e.Base.Present {
		return "both added"
	}
	return "both modified"
}
