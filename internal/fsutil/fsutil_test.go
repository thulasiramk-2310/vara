package fsutil

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteFileCreatesWithContentAndMode(t *testing.T) {
	dir := t.TempDir()
	final := filepath.Join(dir, "ref")
	if err := WriteFile(final+".lock", final, []byte("abc\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(final)
	if err != nil || string(b) != "abc\n" {
		t.Fatalf("content %q, %v", b, err)
	}
	// The mode must be what an ordinary create with the same permission gives (the umask applies).
	probe := filepath.Join(dir, "probe")
	if err := os.WriteFile(probe, nil, 0o640); err != nil {
		t.Fatal(err)
	}
	want, _ := os.Stat(probe)
	if got, _ := os.Stat(final); got.Mode().Perm() != want.Mode().Perm() {
		t.Fatalf("mode = %v, want %v", got.Mode().Perm(), want.Mode().Perm())
	}
}

func TestWriteFileReplacesAnExistingFile(t *testing.T) {
	dir := t.TempDir()
	final := filepath.Join(dir, "ref")
	_ = os.WriteFile(final, []byte("old\n"), 0o644)
	if err := WriteFile(final+".tmp", final, []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(final); string(b) != "new\n" {
		t.Fatalf("got %q", b)
	}
}

func TestWriteFileLeavesNoTemporaryFile(t *testing.T) {
	dir := t.TempDir()
	final := filepath.Join(dir, "ref")
	if err := WriteFile(final+".tmp", final, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Fatalf("temporary file left behind: %s", e.Name())
		}
	}
}

// The new content must be on disk before the rename makes it the real file; otherwise a power cut
// can leave the real name pointing at an empty file (seen on XFS). The directory entry is flushed
// after the rename so the rename itself survives.
func TestDataIsFlushedBeforeTheRenameAndTheDirectoryAfter(t *testing.T) {
	dir := t.TempDir()
	final := filepath.Join(dir, "ref")
	tmp := final + ".tmp"
	_ = os.WriteFile(final, []byte("old\n"), 0o644)

	var steps []string
	oldSyncFile, oldSyncDir := syncFile, syncDir
	t.Cleanup(func() { syncFile, syncDir = oldSyncFile, oldSyncDir })
	syncFile = func(f *os.File) error {
		b, _ := os.ReadFile(f.Name())
		old, _ := os.ReadFile(final)
		if string(b) != "new\n" || string(old) != "old\n" {
			t.Errorf("at the data flush: temp=%q final=%q (final must still be the old content)", b, old)
		}
		steps = append(steps, "flush-data")
		return f.Sync()
	}
	syncDir = func(d string) error {
		if b, _ := os.ReadFile(final); string(b) != "new\n" {
			t.Errorf("at the directory flush the rename should already have happened, final=%q", b)
		}
		if _, err := os.Stat(tmp); err == nil {
			t.Errorf("the temporary file should be gone at the directory flush")
		}
		steps = append(steps, "flush-dir")
		return nil
	}
	if err := WriteFile(tmp, final, []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if strings.Join(steps, ",") != "flush-data,flush-dir" {
		t.Fatalf("steps = %v", steps)
	}
}

// A failure must leave the old file alone and no temporary file behind.
func TestAFailedRenameKeepsTheOldStateAndCleansUp(t *testing.T) {
	dir := t.TempDir()
	final := filepath.Join(dir, "ref")
	if err := os.MkdirAll(filepath.Join(final, "inner"), 0o755); err != nil { // a non-empty directory cannot be replaced by a file
		t.Fatal(err)
	}
	tmp := final + ".tmp"
	if err := WriteFile(tmp, final, []byte("x"), 0o644); err == nil {
		t.Fatal("want an error when the destination cannot be replaced")
	}
	if _, err := os.Stat(tmp); err == nil {
		t.Fatal("the temporary file must be removed after a failure")
	}
	if fi, err := os.Stat(final); err != nil || !fi.IsDir() {
		t.Fatal("the destination must be untouched")
	}
}

func TestWriteFileIntoAMissingDirectoryFails(t *testing.T) {
	final := filepath.Join(t.TempDir(), "no", "such", "dir", "ref")
	if err := WriteFile(final+".tmp", final, []byte("x"), 0o644); err == nil {
		t.Fatal("want an error")
	}
}
