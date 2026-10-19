package undo

import (
	"archive/tar"
	"os"
	"path/filepath"
	"testing"

	"github.com/klauspost/compress/zstd"
)

func writeArchive(t *testing.T, path string, files map[string]string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	enc, _ := zstd.NewWriter(f)
	tw := tar.NewWriter(enc)
	for name, body := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Size: int64(len(body)), Mode: 0o644, Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		tw.Write([]byte(body))
	}
	tw.Close()
	enc.Close()
}

func TestRestoreSnapshotWritesFilesInsideTheRepository(t *testing.T) {
	root, snap := t.TempDir(), filepath.Join(t.TempDir(), "s.tar.zst")
	writeArchive(t, snap, map[string]string{"a.txt": "A", "dir/b.txt": "B"})
	n, err := RestoreSnapshot(root, snap)
	if err != nil || n != 2 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if b, _ := os.ReadFile(filepath.Join(root, "dir/b.txt")); string(b) != "B" {
		t.Fatalf("dir/b.txt = %q", b)
	}
}

func TestRestoreSnapshotRefusesPathsOutsideTheRepository(t *testing.T) {
	outer := t.TempDir()
	root := filepath.Join(outer, "repo")
	os.Mkdir(root, 0o755)
	snap := filepath.Join(t.TempDir(), "evil.tar.zst")
	writeArchive(t, snap, map[string]string{"../escaped.txt": "X"})
	if _, err := RestoreSnapshot(root, snap); err == nil {
		t.Fatal("an entry escaping the repository must be refused")
	}
	if _, err := os.Stat(filepath.Join(outer, "escaped.txt")); err == nil {
		t.Fatal("the escaping file was written")
	}
}
