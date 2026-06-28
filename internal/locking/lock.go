// Package locking implements VARA-RFC-0006 §2.
//
// RFC:
// VARA-RFC-0006 Locking and Transactions
//
// Responsibilities:
// - Provide OS-level exclusive file locking for the repository.
// - Enforce the lock hierarchy: Repository → Refs → Index → Objects.
// - Support exponential backoff with a configurable timeout (default 30s).
//
// This package MUST NOT:
// - Import commands, transaction, repository, or any higher-layer package.
package locking

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	varaerrors "github.com/thulasiramk-2310/vara/internal/errors"
)

// Named lock files relative to VaraDir (RFC-0006 §2).
// Locks MUST be acquired in this exact order to prevent deadlocks:
// Repository → Refs → Index → Objects.
const (
	NameRepository = "locks/repository.lock"
	NameRefs       = "locks/refs.lock"
	NameIndex      = "locks/index.lock"
	NameObjects    = "locks/objects.lock"
)

const (
	defaultTimeout = 30 * time.Second
	initialBackoff = 10 * time.Millisecond
	maxBackoff     = 500 * time.Millisecond
)

// LockFile represents an acquired OS-level lock file.
// The caller must call Release when the locked region is done.
type LockFile struct {
	path string
	file *os.File
}

// Acquire attempts to acquire a named lock within the given vara directory,
// using exponential backoff up to the default 30-second timeout.
func Acquire(varaDir, name string) (*LockFile, error) {
	return AcquireWithTimeout(varaDir, name, defaultTimeout)
}

// AcquireWithTimeout attempts to acquire a lock with a specific timeout.
// Uses O_EXCL|O_CREATE for atomic, race-free lock acquisition on NTFS and ext4.
func AcquireWithTimeout(varaDir, name string, timeout time.Duration) (*LockFile, error) {
	lockPath := filepath.Join(varaDir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(lockPath), 0755); err != nil {
		return nil, fmt.Errorf("locking: create lock dir: %w", err)
	}

	deadline := time.Now().Add(timeout)
	backoff := initialBackoff

	for {
		// O_EXCL|O_CREATE is atomic on NTFS and POSIX: succeeds only if the file
		// does not exist, preventing two processes from acquiring simultaneously.
		f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err == nil {
			// Record our PID so crash-recovery tooling can identify the owner.
			fmt.Fprintf(f, "%d\n", os.Getpid())
			f.Sync()
			return &LockFile{path: lockPath, file: f}, nil
		}

		if !os.IsExist(err) {
			return nil, fmt.Errorf("locking: unexpected error acquiring %s: %w", name, err)
		}

		// Lock is held by another process.
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("%w: %s held by another process", varaerrors.ErrLockTimeout, name)
		}

		time.Sleep(backoff)
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

// Release releases the lock and removes the lock file.
// It is safe to call Release multiple times.
func (l *LockFile) Release() error {
	if l.file != nil {
		l.file.Close()
		l.file = nil
	}
	err := os.Remove(l.path)
	if os.IsNotExist(err) {
		return nil // Already released.
	}
	return err
}

// Path returns the absolute path of the lock file.
func (l *LockFile) Path() string {
	return l.path
}
