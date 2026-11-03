package fsutil

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// On Windows a reader that has the file open makes the rename fail for a moment. This
// stands in for that on any platform: the first renames fail, a later one succeeds.
func TestRenameIsRetriedWhileTheFileIsBusy(t *testing.T) {
	oldFn, oldRetries := renameFn, renameRetries
	defer func() { renameFn, renameRetries = oldFn, oldRetries }()
	renameRetries = 10
	calls := 0
	renameFn = func(a, b string) error {
		calls++
		if calls <= 3 {
			return errors.New("The process cannot access the file because it is being used by another process.")
		}
		return os.Rename(a, b)
	}
	dir := t.TempDir()
	final := filepath.Join(dir, "state.json")
	if err := WriteFile(final+".tmp", final, []byte("new"), 0o644); err != nil {
		t.Fatalf("a busy file should be retried until it is free: %v", err)
	}
	if calls != 4 {
		t.Fatalf("expected 3 failed attempts and one success, got %d calls", calls)
	}
	if got, _ := os.ReadFile(final); string(got) != "new" {
		t.Fatalf("content = %q", got)
	}
}

func TestRenameGivesUpAndLeavesNoTempFile(t *testing.T) {
	oldFn, oldRetries := renameFn, renameRetries
	defer func() { renameFn, renameRetries = oldFn, oldRetries }()
	renameRetries = 3
	calls := 0
	renameFn = func(a, b string) error { calls++; return errors.New("busy for ever") }
	dir := t.TempDir()
	final := filepath.Join(dir, "state.json")
	if err := os.WriteFile(final, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := WriteFile(final+".tmp", final, []byte("new"), 0o644)
	if err == nil || calls != 4 {
		t.Fatalf("should fail after 1 try + 3 retries, got err=%v calls=%d", err, calls)
	}
	if got, _ := os.ReadFile(final); string(got) != "old" {
		t.Fatalf("a failed write must leave the old content, got %q", got)
	}
	if _, statErr := os.Stat(final + ".tmp"); !os.IsNotExist(statErr) {
		t.Fatal("the temporary file must be removed after a failure")
	}
}

// With no retries (every platform but Windows) one failure is final, with no waiting.
func TestRenameDoesNotRetryWhenRetriesAreOff(t *testing.T) {
	oldFn, oldRetries := renameFn, renameRetries
	defer func() { renameFn, renameRetries = oldFn, oldRetries }()
	renameRetries = 0
	calls := 0
	renameFn = func(a, b string) error { calls++; return errors.New("nope") }
	dir := t.TempDir()
	if err := WriteFile(filepath.Join(dir, "x.tmp"), filepath.Join(dir, "x"), []byte("d"), 0o644); err == nil || calls != 1 {
		t.Fatalf("err=%v calls=%d, want one failed attempt", err, calls)
	}
}
