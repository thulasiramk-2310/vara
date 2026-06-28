// Package transaction implements VARA-RFC-0006 §3–4.
//
// RFC:
// VARA-RFC-0006 Locking and Transactions
//
// Responsibilities:
// - Manage the Write-Ahead Journal in .vara/journal/.
// - Coordinate lock acquisition and release per the RFC-0006 hierarchy.
// - Expose the six-phase transaction lifecycle: BEGIN, EXECUTE, VERIFY,
//   COMMIT, COMPLETE, FAILED.
//
// This package MUST NOT:
// - Import commands or repository packages.
package transaction

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/thulasiramk-2310/vara/internal/locking"
)

// State represents the current phase of a transaction (RFC-0006 §4.1).
type State string

const (
	StateBegin    State = "BEGIN"
	StateExecute  State = "EXECUTE"
	StateVerify   State = "VERIFY"
	StateCommit   State = "COMMIT"
	StateComplete State = "COMPLETED"
	StateFailed   State = "FAILED"
)

// journal is the on-disk representation written to .vara/journal/<id>.json.
type journal struct {
	ID      string            `json:"transaction"`
	State   State             `json:"state"`
	Command string            `json:"command"`
	Objects []string          `json:"objects,omitempty"`
	Refs    map[string]string `json:"refs,omitempty"`
}

// Transaction represents an active repository transaction with acquired locks
// and a live journal entry.
type Transaction struct {
	id          string
	varaDir     string
	j           journal
	journalPath string
	locks       []*locking.LockFile
	committed   bool
}

// Begin starts a new transaction, acquiring the specified locks in order
// (callers must pass lock names in hierarchy order per RFC-0006 §2:
// Repository → Refs → Index → Objects).
func Begin(varaDir, command string, lockNames ...string) (*Transaction, error) {
	var acquired []*locking.LockFile
	for _, name := range lockNames {
		l, err := locking.Acquire(varaDir, name)
		if err != nil {
			for _, held := range acquired {
				held.Release()
			}
			return nil, fmt.Errorf("transaction: %w", err)
		}
		acquired = append(acquired, l)
	}

	journalDir := filepath.Join(varaDir, "journal")
	if err := os.MkdirAll(journalDir, 0755); err != nil {
		for _, l := range acquired {
			l.Release()
		}
		return nil, fmt.Errorf("transaction: journal dir: %w", err)
	}

	id := fmt.Sprintf("txn-%d", time.Now().UnixNano())
	t := &Transaction{
		id:          id,
		varaDir:     varaDir,
		journalPath: filepath.Join(journalDir, id+".json"),
		locks:       acquired,
		j: journal{
			ID:      id,
			State:   StateBegin,
			Command: command,
			Refs:    make(map[string]string),
		},
	}

	if err := t.flush(); err != nil {
		for _, l := range acquired {
			l.Release()
		}
		return nil, fmt.Errorf("transaction: journal write: %w", err)
	}

	return t, nil
}

// SetState advances the transaction to a new phase and persists the journal.
func (t *Transaction) SetState(s State) error {
	t.j.State = s
	return t.flush()
}

// TrackObject records an object ID in the journal for crash recovery.
func (t *Transaction) TrackObject(id string) {
	t.j.Objects = append(t.j.Objects, id)
}

// TrackRef records a reference update in the journal (name → new commit hex).
func (t *Transaction) TrackRef(name, commitHex string) {
	t.j.Refs[name] = commitHex
}

// Commit marks the transaction as successfully completed, persists the journal,
// and releases all locks.
func (t *Transaction) Commit() error {
	t.j.State = StateComplete
	err := t.flush()
	t.releaseLocks()
	t.committed = true
	return err
}

// Rollback marks the transaction as failed and releases all locks.
// It is a no-op after a successful Commit.
func (t *Transaction) Rollback() {
	if t.committed {
		return
	}
	t.j.State = StateFailed
	t.flush() // Best-effort; ignore error on rollback.
	t.releaseLocks()
	t.committed = true // Prevent double-release.
}

func (t *Transaction) releaseLocks() {
	for _, l := range t.locks {
		l.Release()
	}
	t.locks = nil
}

func (t *Transaction) flush() error {
	data, err := json.MarshalIndent(t.j, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(t.journalPath, data, 0644)
}
