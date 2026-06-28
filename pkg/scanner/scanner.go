// Package scanner implements differential analysis for VARA repositories.
//
// Responsibilities:
// - Compare the working directory against the index.
// - Compare the index against the object store (HEAD).
// - Compute FileStatus (Added, Modified, Deleted, Untracked).
//
// This package MUST NOT:
// - Execute CLI commands or import the commands layer.
// - Directly mutate the index or object store on disk.
package scanner

import (
	"io"
	"os"

	"github.com/thulasiramk-2310/vara/pkg/index"
	"github.com/thulasiramk-2310/vara/pkg/object"
	"github.com/thulasiramk-2310/vara/pkg/types"
)

// FileStatus represents the differential state of a file.
type FileStatus string

const (
	StatusUntracked FileStatus = "Untracked"
	StatusModified  FileStatus = "Modified"
	StatusDeleted   FileStatus = "Deleted"
	StatusAdded     FileStatus = "Added"
	StatusClean     FileStatus = "Clean"
)

// Result contains the output of a repository scan.
type Result struct {
	Files map[string]FileStatus
}

// Worktree defines the interface for enumerating repository files.
type Worktree interface {
	Walk(fn func(relPath string, info os.FileInfo) error) error
	Open(relPath string) (io.ReadCloser, error)
}

// Scanner compares filesystem state with the index.
type Scanner struct {
	Idx *index.Index
}

// New initializes a Scanner.
func New(idx *index.Index) *Scanner {
	return &Scanner{
		Idx: idx,
	}
}

// Scan performs a differential analysis against the provided Worktree.
func (s *Scanner) Scan(wt Worktree) (*Result, error) {
	res := &Result{
		Files: make(map[string]FileStatus),
	}

	// 1. Build map of index entries
	indexMap := make(map[string]index.Entry)
	if s.Idx != nil {
		for _, e := range s.Idx.Entries {
			indexMap[e.Path] = e
		}
	}

	// 2. Walk the working directory using the abstract Worktree
	err := wt.Walk(func(relPath string, info os.FileInfo) error {
		idxEntry, exists := indexMap[relPath]

		if !exists {
			res.Files[relPath] = StatusUntracked
		} else {
			fingerprint := uint64(info.ModTime().UnixNano())

			if fingerprint != idxEntry.Fingerprint {
				// ModTime differs — file is definitely changed.
				res.Files[relPath] = StatusModified
			} else {
				// ModTime matches — verify content hash to guard against
				// sub-resolution writes (two writes within the same filesystem
				// timestamp tick get the same ModTime).
				h, err := hashFile(wt, relPath)
				if err != nil {
					return err
				}
				if h == idxEntry.ObjectID {
					res.Files[relPath] = StatusClean
				} else {
					res.Files[relPath] = StatusModified
				}
			}
			// Mark as processed
			delete(indexMap, relPath)
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	// 3. Any files left in indexMap but not on disk are Deleted
	for path := range indexMap {
		res.Files[path] = StatusDeleted
	}

	return res, nil
}

// hashFile computes the VARA blob ID for a file using object.BlobHash, which
// is the single authoritative hash definition (matches what store.Write records).
func hashFile(wt Worktree, relPath string) (types.BlobID, error) {
	var id types.BlobID
	f, err := wt.Open(relPath)
	if err != nil {
		return id, err
	}
	defer f.Close()

	content, err := io.ReadAll(f)
	if err != nil {
		return id, err
	}

	id = object.BlobHash(content)
	return id, nil
}
