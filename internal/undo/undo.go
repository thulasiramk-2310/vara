// Package undo implements VARA-RFC-0009 §6.
//
// RFC:
// VARA-RFC-0009 Undo and Recovery
//
// Responsibilities:
// - Execute the three-layer recovery model: Journal → Reflog → Snapshot.
// - Coordinate with pkg/recovery for inspection.
// - Apply the reflog rollback and workspace snapshot restore.
//
// This package MUST NOT:
// - Import commands or any higher-layer package.
package undo

import (
	"archive/tar"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/klauspost/compress/zstd"
	"github.com/thulasiramk-2310/vara/internal/locking"
	"github.com/thulasiramk-2310/vara/internal/transaction"
	"github.com/thulasiramk-2310/vara/pkg/index"
	"github.com/thulasiramk-2310/vara/pkg/object"
	"github.com/thulasiramk-2310/vara/pkg/recovery"
	"github.com/thulasiramk-2310/vara/pkg/refs"
	"github.com/thulasiramk-2310/vara/pkg/types"
)

// Result describes what the undo operation did.
type Result struct {
	Layer   int    // 1 = Journal, 2 = Reflog, 3 = Snapshot.
	Message string // Human-readable description.
}

// Inputs bundles everything the undo engine needs.
type Inputs struct {
	VaraDir string
	RootDir string
	Index   *index.Index
	Store   *object.Store
}

// Undo executes the Recovery Decision Tree (RFC-0009 §3):
//
//	Layer 1 — Journal crash recovery.
//	Layer 2 — Reflog command undo.
//	Layer 3 — Workspace snapshot restore (if nothing else applies).
func Undo(in Inputs) (Result, error) {
	// Layer 1: pending journal entry → crash recovery.
	pending, err := recovery.ScanJournal(in.VaraDir)
	if err != nil {
		return Result{}, err
	}
	if len(pending) > 0 {
		return undoJournal(in, pending[0])
	}

	// Layer 2: reflog rollback.
	prevID, err := recovery.PreviousHEAD(in.VaraDir)
	if err == nil {
		return undoReflog(in, prevID)
	}
	if err != recovery.ErrNoHistory {
		return Result{}, err
	}

	// Layer 3: snapshot restore.
	snaps, err := recovery.ListSnapshots(in.VaraDir)
	if err != nil {
		return Result{}, err
	}
	if len(snaps) > 0 {
		return undoSnapshot(in, snaps[0])
	}

	return Result{}, fmt.Errorf("nothing to undo (no pending transactions, reflog, or snapshots)")
}

// undoJournal marks a stuck transaction as FAILED so the repository is no
// longer considered "recovery pending". The objects and refs written during the
// failed transaction are already immutable blobs; refs were not committed.
func undoJournal(in Inputs, txn recovery.PendingTransaction) (Result, error) {
	// Simply mark the journal entry as FAILED — the lock files will have been
	// released when the process died, so no locks need clearing here.
	data, err := os.ReadFile(txn.Path)
	if err != nil {
		return Result{}, fmt.Errorf("read journal: %w", err)
	}
	updated := strings.ReplaceAll(string(data), `"`+string(txn.State)+`"`, `"FAILED"`)
	if err := os.WriteFile(txn.Path, []byte(updated), 0644); err != nil {
		return Result{}, fmt.Errorf("update journal: %w", err)
	}
	return Result{
		Layer:   1,
		Message: fmt.Sprintf("Crash recovery: marked transaction %s as FAILED.", txn.ID),
	}, nil
}

// undoReflog resets HEAD (and the current branch ref) to prevID, then
// checks out that commit's tree (RFC-0009 §6).
func undoReflog(in Inputs, prevID types.CommitID) (Result, error) {
	txn, err := transaction.Begin(in.VaraDir, "undo", locking.NameRefs, locking.NameIndex)
	if err != nil {
		return Result{}, fmt.Errorf("%w", err)
	}
	defer txn.Rollback()

	resolver := refs.NewFSResolver(in.VaraDir)

	// Find the current branch ref.
	currentRef, symErr := resolver.ResolveSymbolic("HEAD")
	if symErr == nil {
		// Update the branch pointer.
		if err := resolver.Update(currentRef, prevID); err != nil {
			return Result{}, fmt.Errorf("update ref: %w", err)
		}
	} else {
		// Detached HEAD — update HEAD directly.
		headPath := filepath.Join(in.VaraDir, "HEAD")
		if err := atomicWrite(headPath, prevID.String()+"\n"); err != nil {
			return Result{}, fmt.Errorf("update HEAD: %w", err)
		}
	}

	// Checkout the previous commit's tree into a fresh index.
	in.Index.Entries = nil
	obj, err := in.Store.Read(types.ObjectID(prevID))
	if err != nil {
		return Result{}, fmt.Errorf("read previous commit: %w", err)
	}
	prevCommit, ok := obj.(*object.Commit)
	if !ok {
		return Result{}, fmt.Errorf("previous HEAD is not a commit")
	}
	if err := checkoutTree(in.Store, prevCommit.TreeHash, in.RootDir, in.Index); err != nil {
		return Result{}, fmt.Errorf("checkout: %w", err)
	}

	// Persist index.
	idxData, err := in.Index.Serialize()
	if err != nil {
		return Result{}, err
	}
	if err := atomicWrite(filepath.Join(in.VaraDir, "index"), string(idxData)); err != nil {
		return Result{}, err
	}

	// Clear MERGE_HEAD if we were in a suspended merge.
	recovery.ClearMergeHead(in.VaraDir)

	if err := txn.Commit(); err != nil {
		return Result{}, err
	}
	return Result{
		Layer:   2,
		Message: fmt.Sprintf("Undone: HEAD reset to %s.", prevID.String()[:7]),
	}, nil
}

// undoSnapshot restores the working directory from the most recent snapshot.
func undoSnapshot(in Inputs, snapName string) (Result, error) {
	if _, err := RestoreSnapshot(in.RootDir, filepath.Join(in.VaraDir, "snapshots", snapName)); err != nil {
		return Result{}, err
	}
	return Result{
		Layer:   3,
		Message: fmt.Sprintf("Workspace restored from snapshot: %s.", snapName),
	}, nil
}

// RestoreSnapshot writes the files in the snapshot archive at snapPath into
// rootDir and returns how many it wrote. Files the archive does not contain are
// left alone. An entry that would land outside rootDir, or that is not a plain
// file or directory, is refused before anything is written.
func RestoreSnapshot(rootDir, snapPath string) (int, error) {
	rootAbs, err := filepath.Abs(rootDir)
	if err != nil {
		return 0, err
	}
	type entry struct {
		path string
		dir  bool
		data []byte
	}
	var entries []entry

	f, err := os.Open(snapPath)
	if err != nil {
		return 0, fmt.Errorf("open snapshot: %w", err)
	}
	defer f.Close()
	dec, err := zstd.NewReader(f)
	if err != nil {
		return 0, fmt.Errorf("zstd decoder: %w", err)
	}
	defer dec.Close()
	tr := tar.NewReader(dec)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return 0, fmt.Errorf("read tar: %w", err)
		}
		target := filepath.Join(rootAbs, filepath.FromSlash(hdr.Name))
		if target != rootAbs && !strings.HasPrefix(target, rootAbs+string(filepath.Separator)) {
			return 0, fmt.Errorf("snapshot entry %q points outside the repository", hdr.Name)
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			entries = append(entries, entry{path: target, dir: true})
		case tar.TypeReg, '\x00': // pkg/snapshot leaves Typeflag unset: an old-style regular file
			data, err := io.ReadAll(tr)
			if err != nil {
				return 0, fmt.Errorf("extract %s: %w", hdr.Name, err)
			}
			entries = append(entries, entry{path: target, data: data})
		default:
			return 0, fmt.Errorf("snapshot entry %q is not a plain file", hdr.Name)
		}
	}

	written := 0
	for _, e := range entries {
		if e.dir {
			if err := os.MkdirAll(e.path, 0755); err != nil {
				return written, err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(e.path), 0755); err != nil {
			return written, err
		}
		if err := os.WriteFile(e.path, e.data, 0644); err != nil {
			return written, fmt.Errorf("write %s: %w", e.path, err)
		}
		written++
	}
	return written, nil
}

// checkoutTree mirrors the one in internal/merge to avoid import cycles.
func checkoutTree(store *object.Store, treeID types.TreeID, rootDir string, idx *index.Index) error {
	return checkoutTreeEntries(store, types.ObjectID(treeID), rootDir, "", idx)
}

func checkoutTreeEntries(store *object.Store, objID types.ObjectID, rootDir, prefix string, idx *index.Index) error {
	obj, err := store.Read(objID)
	if err != nil {
		return err
	}
	tree, ok := obj.(*object.Tree)
	if !ok {
		return fmt.Errorf("%s is not a tree", objID.String()[:7])
	}
	for _, entry := range tree.Entries {
		relPath := prefix + entry.Name
		absPath := filepath.Join(rootDir, filepath.FromSlash(relPath))
		if entry.Mode == 0o040000 {
			os.MkdirAll(absPath, 0755)
			if err := checkoutTreeEntries(store, entry.Hash, rootDir, relPath+"/", idx); err != nil {
				return err
			}
			continue
		}
		blobObj, err := store.Read(entry.Hash)
		if err != nil {
			return err
		}
		blob, ok := blobObj.(*object.Blob)
		if !ok {
			return fmt.Errorf("%s is not a blob", entry.Hash.String()[:7])
		}
		os.MkdirAll(filepath.Dir(absPath), 0755)
		if err := os.WriteFile(absPath, blob.Data, 0644); err != nil {
			return err
		}
		idx.Entries = append(idx.Entries, index.Entry{
			ObjectID: types.BlobID(entry.Hash),
			State:    index.StateUnmodified,
			Path:     relPath,
		})
	}
	return nil
}

func atomicWrite(path, content string) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), 0644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}
