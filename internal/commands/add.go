// Package commands implements VARA-RFC-0012.
package commands

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/thulasiramk-2310/vara/internal/worktree"
	"github.com/thulasiramk-2310/vara/pkg/index"
	"github.com/thulasiramk-2310/vara/pkg/object"
	"github.com/thulasiramk-2310/vara/pkg/scanner"
	"github.com/thulasiramk-2310/vara/pkg/types"
)

// RunAdd stages files matching args into the index (RFC-0012 §2).
//
// "." or no path restriction means all modified/untracked files.
// Specific paths are matched by prefix: "vara add src/" stages everything under src/.
func RunAdd(ctx *Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("nothing specified, nothing added\n\nusage: vara add <pathspec>...\n       vara add .")
	}

	addAll := len(args) == 1 && args[0] == "."

	wt, err := worktree.New(ctx.Repository.RootDir)
	if err != nil {
		return fmt.Errorf("failed to init worktree: %v", err)
	}

	s := scanner.New(ctx.Index)
	res, err := s.Scan(wt)
	if err != nil {
		return fmt.Errorf("scan failed: %v", err)
	}

	// Collect candidates (modified or untracked), filtered by args.
	var toAdd []string
	for path, st := range res.Files {
		if st != scanner.StatusModified && st != scanner.StatusUntracked {
			continue
		}
		if addAll || matchesAny(path, args) {
			toAdd = append(toAdd, path)
		}
	}

	if len(toAdd) == 0 {
		return nil
	}

	store := object.NewStore(ctx.Repository.VaraDir)

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
	return os.Rename(tmpPath, indexPath)
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
