// Package snapshot implements VARA-RFC-0009 §5.
//
// RFC:
// VARA-RFC-0009 Undo and Recovery
//
// Responsibilities:
// - Create Zstandard-compressed tar archives of the working directory.
// - Store snapshots in .vara/snapshots/ before any mutating command.
// - List available snapshots for recovery.
//
// This package MUST NOT:
// - Import commands, transaction, locking, or any higher-layer package.
package snapshot

import (
	"archive/tar"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/klauspost/compress/zstd"
)

// Create archives the working directory (excluding .vara/) into a
// snap-YYYYMMDD-HHMMSS-<command>-<commit7>.tar.zst file inside .vara/snapshots/.
// commitHex is the current HEAD commit hash (64 hex chars) or a placeholder like
// strings.Repeat("0", 64) for an empty repository.
func Create(varaDir, rootDir, commandName, commitHex string) (string, error) {
	varaAbs, err := filepath.Abs(varaDir)
	if err != nil {
		return "", fmt.Errorf("snapshot: resolve vara dir: %w", err)
	}
	rootAbs, err := filepath.Abs(rootDir)
	if err != nil {
		return "", fmt.Errorf("snapshot: resolve root dir: %w", err)
	}

	snapshotsDir := filepath.Join(varaAbs, "snapshots")
	if err := os.MkdirAll(snapshotsDir, 0755); err != nil {
		return "", fmt.Errorf("snapshot: create snapshots dir: %w", err)
	}

	shortCommit := commitHex
	if len(shortCommit) > 7 {
		shortCommit = shortCommit[:7]
	}

	timestamp := time.Now().Format("20060102-150405")
	filename := fmt.Sprintf("snap-%s-%s-%s.tar.zst", timestamp, commandName, shortCommit)
	snapPath := filepath.Join(snapshotsDir, filename)

	f, err := os.Create(snapPath)
	if err != nil {
		return "", fmt.Errorf("snapshot: create archive file: %w", err)
	}

	enc, err := zstd.NewWriter(f)
	if err != nil {
		f.Close()
		os.Remove(snapPath)
		return "", fmt.Errorf("snapshot: init zstd encoder: %w", err)
	}

	tw := tar.NewWriter(enc)
	varaPrefix := varaAbs + string(filepath.Separator)

	walkErr := filepath.Walk(rootAbs, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		absPath, aerr := filepath.Abs(path)
		if aerr != nil {
			return aerr
		}

		// Skip the .vara directory entirely.
		if absPath == varaAbs || strings.HasPrefix(absPath, varaPrefix) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if info.IsDir() {
			return nil // Directories are implicit in tar archives.
		}

		rel, rerr := filepath.Rel(rootAbs, absPath)
		if rerr != nil {
			return rerr
		}

		hdr := &tar.Header{
			Name:    filepath.ToSlash(rel),
			Size:    info.Size(),
			Mode:    int64(info.Mode()),
			ModTime: info.ModTime(),
		}

		if werr := tw.WriteHeader(hdr); werr != nil {
			return fmt.Errorf("snapshot: write header for %s: %w", rel, werr)
		}

		file, ferr := os.Open(absPath)
		if ferr != nil {
			return fmt.Errorf("snapshot: open %s: %w", rel, ferr)
		}
		_, copyErr := io.Copy(tw, file)
		file.Close()
		return copyErr
	})

	twErr := tw.Close()
	encErr := enc.Close()
	fErr := f.Close()

	if walkErr != nil {
		os.Remove(snapPath)
		return "", fmt.Errorf("snapshot: walk: %w", walkErr)
	}
	if twErr != nil {
		os.Remove(snapPath)
		return "", fmt.Errorf("snapshot: finalize tar: %w", twErr)
	}
	if encErr != nil {
		os.Remove(snapPath)
		return "", fmt.Errorf("snapshot: finalize zstd: %w", encErr)
	}
	if fErr != nil {
		os.Remove(snapPath)
		return "", fmt.Errorf("snapshot: close archive: %w", fErr)
	}

	return snapPath, nil
}

// List returns snapshot filenames stored in .vara/snapshots/, sorted by filename
// (which encodes the creation timestamp). Returns nil if no snapshots exist.
func List(varaDir string) ([]string, error) {
	snapshotsDir := filepath.Join(varaDir, "snapshots")
	entries, err := os.ReadDir(snapshotsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("snapshot: read dir: %w", err)
	}

	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".tar.zst") {
			names = append(names, e.Name())
		}
	}
	return names, nil
}
