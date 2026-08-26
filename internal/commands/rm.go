// Package commands — vara rm.
//
// `vara rm <pathspec>...` removes tracked files from the working tree and stages
// the deletion (the index entry becomes StateDeleted, so the next commit drops it
// from the tree). It is the counterpart to `vara add` for removals: without it
// the CLI cannot record a deletion at all, which in turn makes the deletion side
// of a modify/delete conflict unreachable.
//
// During a merge, removing a conflicted path is a valid resolution — the user has
// chosen the deletion — so rm marks that path resolved in the conflict sidecar,
// giving modify/delete a hand-resolution exit that does not depend on
// `vara resolve`.
package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/thulasiramk-2310/vara/internal/mergestate"
	"github.com/thulasiramk-2310/vara/pkg/index"
)

// RunRm executes `vara rm`. It returns human-readable output.
func RunRm(ctx *Context, paths []string) (string, error) {
	if len(paths) == 0 {
		return "", fmt.Errorf("nothing specified\n\nusage: vara rm <pathspec>...")
	}

	// Select tracked (non-deleted) index entries matching the pathspecs.
	matched := map[string]bool{}
	for _, e := range ctx.Index.Entries {
		if e.State == index.StateDeleted {
			continue
		}
		if matchesAny(e.Path, paths) {
			matched[e.Path] = true
		}
	}
	if len(matched) == 0 {
		return "", fmt.Errorf("pathspec did not match any tracked files: %s", strings.Join(paths, ", "))
	}

	// Remove each file from the working tree and mark its index entry deleted.
	for i := range ctx.Index.Entries {
		p := ctx.Index.Entries[i].Path
		if !matched[p] {
			continue
		}
		abs := filepath.Join(ctx.Repository.RootDir, filepath.FromSlash(p))
		if err := os.Remove(abs); err != nil && !os.IsNotExist(err) {
			return "", fmt.Errorf("rm %s: %w", p, err)
		}
		ctx.Index.Entries[i].State = index.StateDeleted
	}

	if err := writeIndex(ctx); err != nil {
		return "", fmt.Errorf("rm: write index: %w", err)
	}

	removed := make([]string, 0, len(matched))
	for p := range matched {
		removed = append(removed, p)
	}
	sort.Strings(removed)

	// Staging a deletion of a conflicted path resolves it (the user chose to
	// delete). No marker check is needed — a deletion is a definitive choice.
	if err := markRemovedResolved(ctx.Repository.VaraDir, removed); err != nil {
		return "", err
	}

	var sb strings.Builder
	for _, p := range removed {
		sb.WriteString(fmt.Sprintf("rm '%s'\n", p))
	}
	return sb.String(), nil
}

// markRemovedResolved flips the sidecar Resolved flag for any removed path that
// is a recorded conflict. No-op when no conflict sidecar exists.
func markRemovedResolved(varaDir string, removed []string) error {
	st, ok, err := mergestate.Read(varaDir)
	if err != nil || !ok {
		return nil
	}
	changed := false
	for _, p := range removed {
		if _, isConflict := st.Lookup(p); isConflict {
			if st.SetResolved(p, true) {
				changed = true
			}
		}
	}
	if !changed {
		return nil
	}
	return mergestate.Write(varaDir, st)
}
