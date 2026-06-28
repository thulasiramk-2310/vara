// Package recovery implements VARA-RFC-0009 §2–4.
//
// RFC:
// VARA-RFC-0009 Undo and Recovery
//
// Responsibilities:
// - Inspect the Write-Ahead Journal for pending/failed transactions.
// - Inspect the reflog for previous HEAD positions.
// - Inspect available workspace snapshots.
// - Execute the Recovery Decision Tree (RFC-0009 §3).
//
// This package MUST NOT:
// - Import commands, transaction, locking, or any higher-layer package.
// - Mutate references or the working directory directly (that is undo's job).
package recovery

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/thulasiramk-2310/vara/pkg/reflog"
	"github.com/thulasiramk-2310/vara/pkg/snapshot"
	"github.com/thulasiramk-2310/vara/pkg/types"
)

// JournalState mirrors the on-disk transaction state strings.
type JournalState string

const (
	StateBegin    JournalState = "BEGIN"
	StateExecute  JournalState = "EXECUTE"
	StateVerify   JournalState = "VERIFY"
	StateCommit   JournalState = "COMMIT"
	StateComplete JournalState = "COMPLETED"
	StateFailed   JournalState = "FAILED"
)

// PendingTransaction represents an unfinished journal entry.
type PendingTransaction struct {
	ID      string
	State   JournalState
	Command string
	Path    string // Absolute path to the journal file.
}

// ReflogEntry is a re-export of reflog.Entry for callers that only import recovery.
type ReflogEntry = reflog.Entry

// ScanJournal returns all journal entries whose state is not COMPLETED,
// sorted oldest-first by filename (which encodes nanosecond timestamps).
func ScanJournal(varaDir string) ([]PendingTransaction, error) {
	journalDir := filepath.Join(varaDir, "journal")
	entries, err := os.ReadDir(journalDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("recovery: read journal dir: %w", err)
	}

	var pending []PendingTransaction
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		path := filepath.Join(journalDir, e.Name())
		txn, err := readJournalEntry(path)
		if err != nil {
			continue // Skip corrupt entries.
		}
		if txn.State != StateComplete {
			pending = append(pending, PendingTransaction{
				ID:      txn.ID,
				State:   txn.State,
				Command: txn.Command,
				Path:    path,
			})
		}
	}
	return pending, nil
}

// HeadHistory reads the reflog for HEAD and returns entries newest-first.
// Returns nil if no reflog exists yet.
func HeadHistory(varaDir string) ([]ReflogEntry, error) {
	rm := reflog.NewManager(varaDir)
	entries, err := rm.Read("HEAD")
	if err != nil {
		return nil, fmt.Errorf("recovery: read HEAD reflog: %w", err)
	}
	// Reverse so that the most recent entry is first.
	for i, j := 0, len(entries)-1; i < j; i, j = i+1, j-1 {
		entries[i], entries[j] = entries[j], entries[i]
	}
	return entries, nil
}

// PreviousHEAD returns the commit ID that HEAD pointed to before the last
// recorded reflog entry. Returns ErrNoHistory if the reflog is empty or
// there is no previous entry.
func PreviousHEAD(varaDir string) (types.CommitID, error) {
	entries, err := HeadHistory(varaDir)
	if err != nil {
		return types.CommitID{}, err
	}
	if len(entries) == 0 {
		return types.CommitID{}, ErrNoHistory
	}
	prev := entries[0].OldID
	if prev == (types.CommitID{}) {
		return types.CommitID{}, ErrNoHistory
	}
	return prev, nil
}

// ListSnapshots returns snapshot filenames in .vara/snapshots/, newest-first.
func ListSnapshots(varaDir string) ([]string, error) {
	names, err := snapshot.List(varaDir)
	if err != nil {
		return nil, err
	}
	// Filenames encode timestamp; reverse alphabetical = newest first.
	for i, j := 0, len(names)-1; i < j; i, j = i+1, j-1 {
		names[i], names[j] = names[j], names[i]
	}
	return names, nil
}

// MergeInProgress returns true when .vara/MERGE_HEAD exists, indicating a
// suspended three-way merge.
func MergeInProgress(varaDir string) bool {
	_, err := os.Stat(filepath.Join(varaDir, "MERGE_HEAD"))
	return err == nil
}

// ClearMergeHead deletes .vara/MERGE_HEAD, used when aborting a merge.
func ClearMergeHead(varaDir string) error {
	err := os.Remove(filepath.Join(varaDir, "MERGE_HEAD"))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// journalFile is the minimal shape of a journal JSON entry.
type journalFile struct {
	ID      string       `json:"transaction"`
	State   JournalState `json:"state"`
	Command string       `json:"command"`
}

func readJournalEntry(path string) (journalFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return journalFile{}, err
	}
	var jf journalFile
	if err := json.Unmarshal(data, &jf); err != nil {
		return journalFile{}, fmt.Errorf("recovery: parse %s: %w", filepath.Base(path), err)
	}
	return jf, nil
}

// ErrNoHistory is returned when there is no prior HEAD state to restore.
var ErrNoHistory = fmt.Errorf("recovery: no previous HEAD state recorded in reflog")
