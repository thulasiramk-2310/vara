// Package commands — vara resolve (the automatic conflict fixer).
//
// After a conflicted `vara merge`/`vara pull`, .vara/MERGE_HEAD records the merge
// in progress and .vara/CONFLICTS (the content-bearing sidecar) records, per
// conflicted path, its Kind, the base/ours/theirs sides (with presence), and a
// Resolved flag. `vara resolve` is driven entirely by that sidecar — never by a
// working-tree marker scan, which is lossy for content conflicts and absent for
// structural (modify/delete) ones.
//
//   - Content conflicts (edit/edit, add/add): the three sides are re-rendered
//     from the stored blobs via the public pkg/diff.ThreeWayMerge and settled by
//     strategy. Reconstructing from blobs makes resolve idempotent and
//     re-runnable — `--theirs` then `--ours` recovers the original side.
//   - Modify/delete conflicts: "combine a file with its absence" is meaningless,
//     so --union and --auto refuse them; only an explicit --ours/--theirs decides,
//     honoring a deletion by removing the file rather than inventing an empty one.
//
// Resolving a path sets its Resolved flag; `vara commit` refuses while any entry
// is unresolved. resolve only ever touches recorded conflict paths, so a tracked
// file that merely contains marker-shaped text is left alone.
package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/thulasiramk-2310/vara/internal/conflict"
	"github.com/thulasiramk-2310/vara/internal/mergestate"
	"github.com/thulasiramk-2310/vara/internal/smartmerge"
	"github.com/thulasiramk-2310/vara/pkg/config"
	"github.com/thulasiramk-2310/vara/pkg/diff"
	"github.com/thulasiramk-2310/vara/pkg/index"
	"github.com/thulasiramk-2310/vara/pkg/object"
	"github.com/thulasiramk-2310/vara/pkg/recovery"
	"github.com/thulasiramk-2310/vara/pkg/types"
)

// ResolveArgs carries the parsed options for `vara resolve`.
type ResolveArgs struct {
	Strategy conflict.Strategy    // ours | theirs | union | auto
	List     bool                 // only list conflicted files, change nothing
	Paths    []string             // optional pathspecs to restrict to
	Style    conflict.MarkerStyle // marker style for auto's leftover conflicts
	StyleSet bool                 // whether Style was set explicitly (else use config)
}

// RunResolve executes `vara resolve`. It returns human-readable output.
func RunResolve(ctx *Context, ra ResolveArgs) (string, error) {
	if !ra.StyleSet {
		ra.Style = configuredMarkerStyle(ctx)
	}
	st, hasSidecar, err := mergestate.Read(ctx.Repository.VaraDir)
	if err != nil {
		return "", fmt.Errorf("resolve: read conflict state: %w", err)
	}
	if hasSidecar {
		return resolveFromSidecar(ctx, ra, st)
	}
	// Legacy fallback: a merge begun by an older binary left MERGE_HEAD but no
	// sidecar. Fall back to the marker scan, restricted to files that actually
	// contain markers (the historical behavior).
	return resolveLegacy(ctx, ra)
}

// resolveFromSidecar is the primary path: the conflicted set, the three sides,
// and the resolved state all come from .vara/CONFLICTS.
func resolveFromSidecar(ctx *Context, ra ResolveArgs, st *mergestate.State) (string, error) {
	entries := st.Entries
	if len(ra.Paths) > 0 {
		entries = filterEntriesByPathspecs(entries, ra.Paths)
	}
	if len(entries) == 0 {
		return noConflictsMessage(ctx), nil
	}

	store := object.NewStore(ctx.Repository.VaraDir)
	cfg, _ := loadConfig(ctx.Repository.VaraDir) // nil on error → structural drivers stay off

	if ra.List {
		return listConflicts(store, st, entries)
	}

	strat := ra.Strategy
	if strat == "" {
		strat = conflict.Auto
	}

	var sb strings.Builder
	resolvedNow := 0
	var needsChoice []string     // modify/delete that union/auto can't settle
	var stillConflicted []string // content that auto left with genuine conflicts

	for _, e := range entries {
		switch e.Kind {
		case mergestate.KindModifyDelete:
			side, ok := chosenPresentSide(e, strat)
			if !ok {
				// union/auto cannot settle a modify/delete — leave it untouched.
				needsChoice = append(needsChoice, describeModifyDelete(e))
				continue
			}
			if err := applyModifyDeleteChoice(ctx, store, e, side); err != nil {
				return "", fmt.Errorf("resolve %s: %w", e.Path, err)
			}
			st.SetResolved(e.Path, true)
			resolvedNow++
			sb.WriteString(fmt.Sprintf("\tresolved  %s (modify/delete, %s)\n", e.Path, sideName(side)))

		default: // KindContent
			// Binary content can't be line-merged without corruption: only an
			// explicit ours/theirs choice (a whole-side write) is meaningful.
			bin, err := entryIsBinary(store, e)
			if err != nil {
				return "", fmt.Errorf("resolve %s: %w", e.Path, err)
			}
			if bin {
				side, ok := chosenPresentSide(e, strat)
				if !ok {
					needsChoice = append(needsChoice, describeBinary(e))
					continue
				}
				content, err := sideContent(store, side)
				if err != nil {
					return "", fmt.Errorf("resolve %s: %w", e.Path, err)
				}
				if err := restageResolved(ctx, store, e.Path, content); err != nil {
					return "", err
				}
				st.SetResolved(e.Path, true)
				resolvedNow++
				sb.WriteString(fmt.Sprintf("\tresolved  %s (binary, %s)\n", e.Path, strat))
				continue
			}

			out, stats, err := resolveContent(store, st, e, strat, ra.Style, cfg)
			if err != nil {
				return "", fmt.Errorf("resolve %s: %w", e.Path, err)
			}
			if err := restageResolved(ctx, store, e.Path, out); err != nil {
				return "", err
			}
			if stats.Remaining == 0 {
				st.SetResolved(e.Path, true)
				resolvedNow++
				if stats.Total == 0 {
					// auto combined disjoint edits with no ambiguous hunk.
					sb.WriteString(fmt.Sprintf("\tresolved  %s (clean merge, %s)\n", e.Path, strat))
				} else {
					sb.WriteString(fmt.Sprintf("\tresolved  %s (%d hunk%s, %s)\n", e.Path, stats.Total, plural(stats.Total), strat))
				}
			} else {
				st.SetResolved(e.Path, false)
				stillConflicted = append(stillConflicted, e.Path)
				sb.WriteString(fmt.Sprintf("\tpartial   %s (%d of %d resolved)\n", e.Path, stats.Resolved, stats.Total))
			}
		}
	}

	if err := writeIndex(ctx); err != nil {
		return "", fmt.Errorf("resolve: write index: %w", err)
	}
	if err := mergestate.Write(ctx.Repository.VaraDir, st); err != nil {
		return "", fmt.Errorf("resolve: update conflict state: %w", err)
	}

	return sidecarSummary(st, strat, resolvedNow, sb.String(), stillConflicted, needsChoice), nil
}

// chosenPresentSide returns the side a strategy selects for a modify/delete
// conflict, and false if the strategy is not an explicit ours/theirs choice
// (union/auto refuse modify/delete).
func chosenPresentSide(e mergestate.Entry, strat conflict.Strategy) (mergestate.Side, bool) {
	switch strat {
	case conflict.Ours:
		return e.Ours, true
	case conflict.Theirs:
		return e.Theirs, true
	default:
		return mergestate.Side{}, false
	}
}

// applyModifyDeleteChoice materializes the chosen side of a modify/delete
// conflict: write its blob when present, or delete the file when the chosen side
// deleted it (never invent an empty file).
func applyModifyDeleteChoice(ctx *Context, store *object.Store, e mergestate.Entry, side mergestate.Side) error {
	if !side.Present {
		return applyDeletion(ctx, e.Path)
	}
	id, ok := parseBlobID(side.Blob)
	if !ok {
		return fmt.Errorf("corrupt conflict record: bad blob id %q", side.Blob)
	}
	content, err := blobContent(store, id)
	if err != nil {
		return err
	}
	return restageResolved(ctx, store, e.Path, content)
}

// applyDeletion removes a path from the working tree and marks its index entry
// deleted so the next commit drops it from the tree.
func applyDeletion(ctx *Context, path string) error {
	abs := filepath.Join(ctx.Repository.RootDir, filepath.FromSlash(path))
	if err := os.Remove(abs); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("resolve: delete %s: %w", path, err)
	}
	for i := range ctx.Index.Entries {
		if ctx.Index.Entries[i].Path == path {
			ctx.Index.Entries[i].State = index.StateDeleted
			return nil
		}
	}
	return nil
}

// resolveContent settles a content conflict from the three stored sides.
//   - ours/theirs write that side verbatim.
//   - auto runs the base-aware three-way merge (conflict.MergeThreeWay), which
//     settles disjoint edits the engine's coarser merge lumps into one conflict
//     and leaves genuine overlaps (and hunk-level modify/delete) as markers.
//   - union re-renders the engine's conflict and keeps both sides of each hunk.
func resolveContent(store *object.Store, st *mergestate.State, e mergestate.Entry, strat conflict.Strategy, style conflict.MarkerStyle, cfg *config.Config) ([]byte, conflict.Stats, error) {
	base, err := sideContent(store, e.Base)
	if err != nil {
		return nil, conflict.Stats{}, err
	}
	ours, err := sideContent(store, e.Ours)
	if err != nil {
		return nil, conflict.Stats{}, err
	}
	theirs, err := sideContent(store, e.Theirs)
	if err != nil {
		return nil, conflict.Stats{}, err
	}
	ourLabel, theirLabel := conflictLabels(st)

	switch strat {
	case conflict.Ours:
		total := conflict.Count(renderSides(base, ours, theirs, ourLabel, theirLabel))
		return ours, conflict.Stats{Total: total, Resolved: total}, nil
	case conflict.Theirs:
		total := conflict.Count(renderSides(base, ours, theirs, ourLabel, theirLabel))
		return theirs, conflict.Stats{Total: total, Resolved: total}, nil
	case conflict.Auto:
		// A structural driver (opt-in per type) settles independent edits and, for
		// JSON, renders leftover divergences as per-key markers — matching what a
		// fresh `vara merge` produces, so resolve doesn't revert to line markers.
		if driver := smartmerge.SelectDriver(e.Path, cfg); driver != "" {
			r := smartmerge.Merge(driver, base, ours, theirs, ourLabel, theirLabel)
			if r.ParseOK && (r.Conflicts == 0 || r.Rendered) {
				return r.Merged, conflict.Stats{Total: r.Conflicts, Remaining: r.Conflicts}, nil
			}
		}
		// Otherwise the base-aware line merge settles disjoint edits and re-renders
		// leftover conflicts in the configured style.
		out, stats := conflict.Render(base, ours, theirs, ourLabel, theirLabel, style)
		return out, stats, nil
	default: // union
		return conflict.Resolve(renderSides(base, ours, theirs, ourLabel, theirLabel), strat)
	}
}

// renderConflict reconstructs the merge-engine's conflict rendering for one path
// from the stored blobs, so resolve never depends on the working-tree bytes.
func renderConflict(store *object.Store, st *mergestate.State, e mergestate.Entry) ([]byte, error) {
	base, err := sideContent(store, e.Base)
	if err != nil {
		return nil, err
	}
	ours, err := sideContent(store, e.Ours)
	if err != nil {
		return nil, err
	}
	theirs, err := sideContent(store, e.Theirs)
	if err != nil {
		return nil, err
	}
	ourLabel, theirLabel := conflictLabels(st)
	return renderSides(base, ours, theirs, ourLabel, theirLabel), nil
}

// renderSides produces the engine's marker rendering for three sides. pkg/diff
// .ThreeWayMerge handles an empty base (both sides added the path) by emitting a
// single whole-file conflict block, so this is uniform across content conflicts.
func renderSides(base, ours, theirs []byte, ourLabel, theirLabel string) []byte {
	merged, _ := diff.ThreeWayMerge(base, ours, theirs, ourLabel, theirLabel)
	return merged
}

// conflictLabels returns the merge's side labels, defaulting when unset.
func conflictLabels(st *mergestate.State) (string, string) {
	ourLabel, theirLabel := st.OurLabel, st.TheirLabel
	if ourLabel == "" {
		ourLabel = "ours"
	}
	if theirLabel == "" {
		theirLabel = "theirs"
	}
	return ourLabel, theirLabel
}

// sideContent returns a side's stored bytes, or nil when the side is absent
// (the path does not exist on that side of the merge).
func sideContent(store *object.Store, side mergestate.Side) ([]byte, error) {
	if !side.Present {
		return nil, nil
	}
	id, ok := parseBlobID(side.Blob)
	if !ok {
		return nil, fmt.Errorf("corrupt conflict record: bad blob id %q", side.Blob)
	}
	return blobContent(store, id)
}

func listConflicts(store *object.Store, st *mergestate.State, entries []mergestate.Entry) (string, error) {
	var sb strings.Builder
	sb.WriteString("Conflicted files:\n")
	for _, e := range entries {
		mark := " "
		if e.Resolved {
			mark = "✓"
		}
		switch e.Kind {
		case mergestate.KindModifyDelete:
			sb.WriteString(fmt.Sprintf("\t%s %s (%s)\n", mark, e.Path, describeModifyDelete(e)))
		default:
			render, err := renderConflict(store, st, e)
			if err != nil {
				return "", err
			}
			hunks := conflict.Count(render)
			sb.WriteString(fmt.Sprintf("\t%s %s (%d conflict%s)\n", mark, e.Path, hunks, plural(hunks)))
		}
	}
	return sb.String(), nil
}

// entryIsBinary reports whether any stored side of a content conflict is binary,
// in which case a line merge would corrupt the file.
func entryIsBinary(store *object.Store, e mergestate.Entry) (bool, error) {
	for _, s := range []mergestate.Side{e.Base, e.Ours, e.Theirs} {
		b, err := sideContent(store, s)
		if err != nil {
			return false, err
		}
		if conflict.IsBinary(b) {
			return true, nil
		}
	}
	return false, nil
}

// describeBinary renders a human-readable summary of a binary content conflict.
func describeBinary(e mergestate.Entry) string {
	return fmt.Sprintf("binary conflict: %s changed on both sides", e.Path)
}

// describeModifyDelete renders a human-readable summary of which side deleted.
func describeModifyDelete(e mergestate.Entry) string {
	if e.Ours.Present && !e.Theirs.Present {
		return "modify/delete: modified by us, deleted by them"
	}
	return "modify/delete: deleted by us, modified by them"
}

func sideName(s mergestate.Side) string {
	if s.Present {
		return "kept"
	}
	return "deleted"
}

func filterEntriesByPathspecs(entries []mergestate.Entry, specs []string) []mergestate.Entry {
	var out []mergestate.Entry
	for _, e := range entries {
		if matchesAny(e.Path, specs) {
			out = append(out, e)
		}
	}
	return out
}

func sidecarSummary(st *mergestate.State, strat conflict.Strategy, resolvedNow int, detail string, stillConflicted, needsChoice []string) string {
	var head strings.Builder
	head.WriteString(fmt.Sprintf("Resolved %d file%s using strategy '%s'.\n", resolvedNow, plural(resolvedNow), strat))
	head.WriteString(detail)
	if len(needsChoice) > 0 {
		head.WriteString(fmt.Sprintf("\n%d conflict(s) need an explicit choice ('%s' cannot decide):\n", len(needsChoice), strat))
		for _, d := range needsChoice {
			head.WriteString("\t" + d + "\n")
		}
		head.WriteString("Re-run with --ours (keep our version) or --theirs (take their version).\n")
	}
	if len(stillConflicted) > 0 {
		head.WriteString(fmt.Sprintf("\n%d file(s) still have conflicts 'auto' could not settle:\n", len(stillConflicted)))
		for _, p := range stillConflicted {
			head.WriteString("\t" + p + "\n")
		}
		head.WriteString("Re-run with --ours, --theirs, or --union to force a choice, or edit them by hand.\n")
	}
	if st.AllResolved() {
		head.WriteString("\nAll conflicts resolved. Run 'vara commit' to complete the merge.\n")
	}
	return head.String()
}

// --- legacy marker-scan fallback (no sidecar present) ---------------------

func resolveLegacy(ctx *Context, ra ResolveArgs) (string, error) {
	conflicted, err := findConflictedFiles(ctx)
	if err != nil {
		return "", err
	}
	if len(ra.Paths) > 0 {
		conflicted = filterByPathspecs(conflicted, ra.Paths)
	}
	if len(conflicted) == 0 {
		return noConflictsMessage(ctx), nil
	}

	if ra.List {
		var sb strings.Builder
		sb.WriteString("Conflicted files:\n")
		for _, c := range conflicted {
			sb.WriteString(fmt.Sprintf("\t%s (%d conflict%s)\n", c.path, c.hunks, plural(c.hunks)))
		}
		return sb.String(), nil
	}

	strat := ra.Strategy
	if strat == "" {
		strat = conflict.Auto
	}

	store := object.NewStore(ctx.Repository.VaraDir)
	var sb strings.Builder
	fullyResolved := 0
	var stillConflicted []string
	for _, c := range conflicted {
		out, stats, err := conflict.Resolve(c.data, strat)
		if err != nil {
			return "", fmt.Errorf("resolve %s: %w", c.path, err)
		}
		if err := restageResolved(ctx, store, c.path, out); err != nil {
			return "", err
		}
		if stats.Remaining == 0 {
			fullyResolved++
			sb.WriteString(fmt.Sprintf("\tresolved  %s (%d hunk%s, %s)\n", c.path, stats.Total, plural(stats.Total), strat))
		} else {
			stillConflicted = append(stillConflicted, c.path)
			sb.WriteString(fmt.Sprintf("\tpartial   %s (%d of %d resolved)\n", c.path, stats.Resolved, stats.Total))
		}
	}
	if err := writeIndex(ctx); err != nil {
		return "", fmt.Errorf("resolve: write index: %w", err)
	}

	var head strings.Builder
	head.WriteString(fmt.Sprintf("Resolved %d file%s using strategy '%s'.\n", fullyResolved, plural(fullyResolved), strat))
	head.WriteString(sb.String())
	if len(stillConflicted) > 0 {
		head.WriteString(fmt.Sprintf("\n%d file(s) still have conflicts 'auto' could not settle:\n", len(stillConflicted)))
		for _, p := range stillConflicted {
			head.WriteString("\t" + p + "\n")
		}
		head.WriteString("Re-run with --ours, --theirs, or --union to force a choice, or edit them by hand.\n")
	} else if recovery.MergeInProgress(ctx.Repository.VaraDir) {
		head.WriteString("\nAll conflicts resolved. Run 'vara commit' to complete the merge.\n")
	}
	return head.String(), nil
}

// conflictedFile bundles a tracked file that still carries conflict markers.
type conflictedFile struct {
	path  string
	data  []byte
	hunks int
}

// findConflictedFiles scans the tracked (indexed) files for conflict markers.
func findConflictedFiles(ctx *Context) ([]conflictedFile, error) {
	seen := map[string]bool{}
	var out []conflictedFile
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
			out = append(out, conflictedFile{path: e.Path, data: data, hunks: conflict.Count(data)})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].path < out[j].path })
	return out, nil
}

func filterByPathspecs(files []conflictedFile, specs []string) []conflictedFile {
	var out []conflictedFile
	for _, f := range files {
		if matchesAny(f.path, specs) {
			out = append(out, f)
		}
	}
	return out
}

// --- shared helpers -------------------------------------------------------

func noConflictsMessage(ctx *Context) string {
	if recovery.MergeInProgress(ctx.Repository.VaraDir) {
		return "No conflicts remaining. Run 'vara commit' to complete the merge.\n"
	}
	return "No conflicted files.\n"
}

// restageResolved writes the resolved bytes to the working tree and updates the
// file's index entry to the new blob, mirroring `vara add` for one file.
func restageResolved(ctx *Context, store *object.Store, path string, data []byte) error {
	abs := filepath.Join(ctx.Repository.RootDir, filepath.FromSlash(path))
	if err := os.WriteFile(abs, data, 0644); err != nil {
		return fmt.Errorf("resolve: write %s: %w", path, err)
	}
	id, err := store.Write(object.NewBlob(data))
	if err != nil {
		return fmt.Errorf("resolve: store %s: %w", path, err)
	}
	fp := uint64(0)
	if info, statErr := os.Stat(abs); statErr == nil {
		fp = uint64(info.ModTime().UnixNano())
	}
	for i := range ctx.Index.Entries {
		if ctx.Index.Entries[i].Path == path {
			ctx.Index.Entries[i].ObjectID = types.BlobID(id)
			ctx.Index.Entries[i].Fingerprint = fp
			ctx.Index.Entries[i].State = index.StateModified
			return nil
		}
	}
	ctx.Index.Entries = append(ctx.Index.Entries, index.Entry{
		Fingerprint: fp,
		ObjectID:    types.BlobID(id),
		State:       index.StateAdded,
		Path:        path,
	})
	return nil
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
