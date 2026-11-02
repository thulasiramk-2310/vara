// Package commands implements VARA-RFC-0012.
package commands

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/thulasiramk-2310/vara/internal/conflict"
	"github.com/thulasiramk-2310/vara/internal/mergestate"
	"github.com/thulasiramk-2310/vara/internal/worktree"
	"github.com/thulasiramk-2310/vara/pkg/index"
	"github.com/thulasiramk-2310/vara/pkg/object"
	"github.com/thulasiramk-2310/vara/pkg/scanner"
	"github.com/thulasiramk-2310/vara/pkg/types"
)

// RunAddFromCwd is what the command line runs: the arguments are typed relative to the
// directory the user is standing in, so they are turned into repository-relative pathspecs
// first ("." from a subfolder means that folder). A path outside the repository is an error.
func RunAddFromCwd(ctx *Context, args []string) error {
	if len(args) == 0 {
		return RunAdd(ctx, args)
	}
	specs, err := ResolvePathspecs(ctx.Repository.RootDir, args)
	if err != nil {
		return err
	}
	return RunAdd(ctx, specs)
}

// RunAdd stages files matching args into the index (RFC-0012 §2). The pathspecs are
// relative to the repository root; the command line resolves them from the current
// directory with RunAddFromCwd.
//
// "." means all modified/untracked files.
// Specific paths are matched by prefix: "vara add src/" stages everything under src/.
// A pathspec that matches no file at all is an error, and nothing is staged then.
func RunAdd(ctx *Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("nothing specified, nothing added\n\nusage: vara add <pathspec>...\n       vara add .")
	}

	specs := make([]string, len(args))
	for i, a := range args {
		specs[i] = filepath.ToSlash(a)
	}
	addAll := false
	for _, spec := range specs {
		if spec == "." {
			addAll = true
		}
	}

	wt, err := worktree.New(ctx.Repository.RootDir)
	if err != nil {
		return fmt.Errorf("failed to init worktree: %v", err)
	}

	s := scanner.New(ctx.Index)
	res, err := s.Scan(wt)
	if err != nil {
		return fmt.Errorf("scan failed: %v", err)
	}

	// Every name must refer to something real (changed or not) before anything is staged,
	// so a typo is an error and a mixed list never half-applies.
	for i, spec := range specs {
		if spec == "." {
			continue
		}
		matched := false
		for path := range res.Files {
			if matchesAny(path, []string{spec}) {
				matched = true
				break
			}
		}
		if !matched {
			return fmt.Errorf("pathspec '%s' did not match any files (a typo, or ignored by .varaignore)", args[i])
		}
	}

	// Collect candidates (modified or untracked), filtered by the specs.
	var toAdd []string
	for path, st := range res.Files {
		if st != scanner.StatusModified && st != scanner.StatusUntracked {
			continue
		}
		if addAll || matchesAny(path, specs) {
			toAdd = append(toAdd, path)
		}
	}

	if len(toAdd) == 0 {
		return nil
	}

	store := object.NewStore(ctx.Repository.VaraDir)

	// markerFree records staged paths whose content has no conflict markers, used
	// below to mark a conflicted path resolved when the user resolves it by hand
	// and `vara add`s it (the Git-style "add means resolved") — but never while
	// markers remain, so a half-edited conflict cannot be committed.
	markerFree := map[string]bool{}

	for _, path := range toAdd {
		absPath := filepath.Join(ctx.Repository.RootDir, filepath.FromSlash(path))
		f, err := os.Open(absPath)
		if err != nil {
			return fmt.Errorf("add %s: %w", path, err)
		}
		content, err := io.ReadAll(f)
		f.Close()
		if err != nil {
			return fmt.Errorf("add %s: %w", path, err)
		}

		blob := object.NewBlob(content)
		id, err := store.Write(blob)
		if err != nil {
			return fmt.Errorf("add %s: %w", path, err)
		}
		markerFree[path] = !conflict.HasMarkers(content)

		info, _ := os.Stat(absPath)
		fp := uint64(0)
		if info != nil {
			fp = uint64(info.ModTime().UnixNano())
		}

		found := false
		for i := range ctx.Index.Entries {
			if ctx.Index.Entries[i].Path == path {
				ctx.Index.Entries[i].ObjectID = types.BlobID(id)
				ctx.Index.Entries[i].Fingerprint = fp
				ctx.Index.Entries[i].State = index.StateModified
				found = true
				break
			}
		}
		if !found {
			ctx.Index.Entries = append(ctx.Index.Entries, index.Entry{
				Fingerprint: fp,
				ObjectID:    types.BlobID(id),
				State:       index.StateAdded,
				Path:        path,
			})
		}
	}

	indexPath := filepath.Join(ctx.Repository.VaraDir, "index")
	data, err := ctx.Index.Serialize()
	if err != nil {
		return err
	}
	tmpPath := indexPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, indexPath); err != nil {
		return err
	}

	// Git-style "add means resolved": if a merge is in progress, mark any staged
	// conflict path resolved — but only when its staged content is marker-free, so
	// a still-conflicted file can never be marked resolved by accident.
	return markResolvedByAdd(ctx.Repository.VaraDir, markerFree)
}

// ResolvePathspecs turns pathspecs typed relative to the current directory into pathspecs
// relative to the repository root ("." alone meaning the whole repository). A pathspec that
// points outside the repository is an error.
func ResolvePathspecs(root string, args []string) ([]string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	// Compare real paths so a symlinked temp or home directory does not look like "outside".
	if r, err := filepath.EvalSymlinks(root); err == nil {
		root = r
	}
	if c, err := filepath.EvalSymlinks(cwd); err == nil {
		cwd = c
	}
	specs := make([]string, 0, len(args))
	for _, arg := range args {
		abs := arg
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(cwd, abs)
		}
		abs = filepath.Clean(abs)
		if r, err := filepath.EvalSymlinks(filepath.Dir(abs)); err == nil {
			abs = filepath.Join(r, filepath.Base(abs))
		}
		rel, err := filepath.Rel(root, abs)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return nil, fmt.Errorf("'%s' is outside the repository", arg)
		}
		specs = append(specs, filepath.ToSlash(rel))
	}
	return specs, nil
}

// markResolvedByAdd flips the sidecar Resolved flag for staged, marker-free
// conflict paths. It is a no-op when no conflict sidecar exists.
func markResolvedByAdd(varaDir string, markerFree map[string]bool) error {
	st, ok, err := mergestate.Read(varaDir)
	if err != nil || !ok {
		return nil // no merge in progress (or unreadable) — nothing to mark
	}
	changed := false
	for path, clean := range markerFree {
		if !clean {
			continue
		}
		if _, isConflict := st.Lookup(path); isConflict {
			if st.SetResolved(path, true) {
				changed = true
			}
		}
	}
	if !changed {
		return nil
	}
	return mergestate.Write(varaDir, st)
}

// matchesAny returns true if path matches any of the given pathspecs.
// A pathspec of "src/" matches anything with that prefix.
// A pathspec of "foo.go" matches exactly "foo.go".
func matchesAny(path string, specs []string) bool {
	for _, spec := range specs {
		// Normalize to forward slashes
		spec = filepath.ToSlash(spec)
		if spec == path {
			return true
		}
		// Directory prefix: "src" matches "src/foo.go"
		if strings.HasPrefix(path, strings.TrimSuffix(spec, "/")+"/") {
			return true
		}
	}
	return false
}
