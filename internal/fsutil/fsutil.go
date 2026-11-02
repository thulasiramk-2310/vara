// Package fsutil writes the repository's state files so that a crash or a power cut never leaves
// half a file behind (VARA-RFC-0004 §4 steps 2-4, VARA-RFC-0006: atomic rename).
//
// The order is what makes it safe: the new content goes to a temporary file, is flushed to disk,
// and only then is renamed over the real file; the directory is flushed last so the rename itself
// survives. Without the first flush, a power cut after the rename can leave the real file empty on
// filesystems that delay data writes (XFS, ext4 in some modes). It depends on the standard library
// only, so every layer may use it.
package fsutil

import (
	"os"
	"path/filepath"
)

// syncFile and syncDir are variables so tests can check the order of the steps.
var (
	syncFile = func(f *os.File) error { return f.Sync() }
	syncDir  = func(dir string) error {
		d, err := os.Open(dir)
		if err != nil {
			return err
		}
		defer d.Close()
		return d.Sync()
	}
)

// WriteFile atomically replaces final with data. tmp is the temporary name to write first; it must
// be in the same directory as final so the rename cannot cross a filesystem. On any failure the old
// file is untouched and tmp is removed.
func WriteFile(tmp, final string, data []byte, perm os.FileMode) error {
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := syncFile(f); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, final); err != nil {
		os.Remove(tmp)
		return err
	}
	// The data is safe and the rename is done. A directory flush is best effort: some platforms
	// (Windows) cannot open a directory, and the rename has already taken effect either way.
	_ = syncDir(filepath.Dir(final))
	return nil
}
