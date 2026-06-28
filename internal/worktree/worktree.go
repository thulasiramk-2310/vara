// Package worktree isolates OS-specific filesystem iteration from the core protocol engines.
//
// Responsibilities:
// - Enumerate files traversing a directory.
// - Filter results via pkg/ignore.
// - Canonicalize paths via pkg/path.
package worktree

import (
	"io"
	"os"
	"path/filepath"

	"github.com/thulasiramk-2310/vara/pkg/ignore"
	varapath "github.com/thulasiramk-2310/vara/pkg/path"
)

// Worktree encapsulates a physical filesystem checkout.
type Worktree struct {
	RootDir string
	Matcher *ignore.Matcher
}

// New initializes a Worktree parsing `.varaignore` at the root if it exists.
func New(rootDir string) (*Worktree, error) {
	absRoot, err := filepath.Abs(rootDir)
	if err != nil {
		return nil, err
	}

	ignorePath := filepath.Join(absRoot, ".varaignore")
	matcher, err := ignore.Load(ignorePath)
	if err != nil {
		return nil, err
	}

	return &Worktree{
		RootDir: absRoot,
		Matcher: matcher,
	}, nil
}

// Walk enumerates all non-ignored files in the worktree.
// The relPath provided to the callback is guaranteed to be forward-slash separated
// and relative to RootDir.
func (w *Worktree) Walk(fn func(relPath string, info os.FileInfo) error) error {
	return filepath.Walk(w.RootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		rel, err := varapath.ToRepoRelative(w.RootDir, path)
		if err != nil {
			return err
		}

		// Don't process the root directory itself as a file/entry
		if rel == "." {
			return nil
		}

		// Filter against VARA Ignore Subset v1
		if w.Matcher.Ignore(rel) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil // skip file
		}

		// We only emit files, we don't track empty directories yet
		if info.IsDir() {
			return nil
		}

		return fn(rel, info)
	})
}

// Open opens a file identified by a repository-relative forward-slash path.
func (w *Worktree) Open(relPath string) (io.ReadCloser, error) {
	absPath := filepath.Join(w.RootDir, filepath.FromSlash(relPath))
	return os.Open(absPath)
}
