package integration

import (
	"bytes"
	"testing"
	"time"

	"github.com/thulasiramk-2310/vara/pkg/object"
	"github.com/thulasiramk-2310/vara/pkg/types"
)

func TestObjectExhaustive(t *testing.T) {
	tempDir := t.TempDir()
	store := object.NewStore(tempDir)

	t.Run("Empty Blob", func(t *testing.T) {
		b := object.NewBlob([]byte{})
		h, err := store.Write(b)
		if err != nil {
			t.Fatal(err)
		}
		loaded, _ := store.Read(h)
		if len(loaded.(*object.Blob).Data) != 0 {
			t.Fatal("expected empty blob")
		}
	})

	t.Run("Large Blob 10MB", func(t *testing.T) {
		data := bytes.Repeat([]byte("a"), 10*1024*1024)
		b := object.NewBlob(data)
		h, err := store.Write(b)
		if err != nil {
			t.Fatal(err)
		}
		loaded, _ := store.Read(h)
		if len(loaded.(*object.Blob).Data) != 10*1024*1024 {
			t.Fatal("length mismatch")
		}
	})

	t.Run("UTF-8 Filenames", func(t *testing.T) {
		b := object.NewBlob([]byte("content"))
		h, _ := store.Write(b)
		tree := object.NewTree([]object.TreeEntry{
			{Mode: 100644, Hash: h, Name: "こんにちは.txt"},
		})
		th, _ := store.Write(tree)
		loaded, _ := store.Read(th)
		if loaded.(*object.Tree).Entries[0].Name != "こんにちは.txt" {
			t.Fatal("UTF-8 name mismatch")
		}
	})

	t.Run("Multi-parent commit", func(t *testing.T) {
		commit := &object.Commit{
			TreeHash: types.TreeID{},
			Parents: []types.CommitID{
				{}, {},
			},
			Author:    "Merge Bot",
			Message:   "Merge branch",
			Timestamp: time.Now().Unix(),
		}
		ch, _ := store.Write(commit)
		loaded, _ := store.Read(ch)
		if len(loaded.(*object.Commit).Parents) != 2 {
			t.Fatal("Parents mismatch")
		}
	})
}
