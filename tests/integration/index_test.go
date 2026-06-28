package integration

import (
	"bytes"
	"testing"

	"github.com/thulasiramk-2310/vara/pkg/index"
	"github.com/thulasiramk-2310/vara/pkg/types"
)

func TestIndexRoundTrip(t *testing.T) {
	idx := index.New()
	
	// Add test entries
	var id1, id2 types.BlobID
	id1[0] = 1
	id2[0] = 2

	idx.Entries = append(idx.Entries, index.Entry{
		Fingerprint: 123456789,
		ObjectID:    id1,
		State:       index.StateAdded,
		Path:        "foo/bar.txt",
	})
	
	idx.Entries = append(idx.Entries, index.Entry{
		Fingerprint: 987654321,
		ObjectID:    id2,
		State:       index.StateModified,
		Path:        "baz.go",
	})

	// Serialize
	data, err := idx.Serialize()
	if err != nil {
		t.Fatalf("Serialize failed: %v", err)
	}

	// Deserialize
	loaded, err := index.Deserialize(data)
	if err != nil {
		t.Fatalf("Deserialize failed: %v", err)
	}

	// Verify
	if len(loaded.Entries) != 2 {
		t.Fatalf("Expected 2 entries, got %d", len(loaded.Entries))
	}

	if loaded.Entries[0].Path != "foo/bar.txt" || loaded.Entries[0].State != index.StateAdded {
		t.Errorf("Entry 0 mismatch")
	}

	if loaded.Entries[1].Path != "baz.go" || loaded.Entries[1].State != index.StateModified {
		t.Errorf("Entry 1 mismatch")
	}
}

func TestIndexCorruptHeader(t *testing.T) {
	// Invalid signature
	data := []byte("INVALID\x00\x00\x00\x01")
	_, err := index.Deserialize(data)
	if err == nil {
		t.Error("Expected error for invalid signature")
	}

	// Valid signature, invalid version
	var buf bytes.Buffer
	buf.WriteString("VARA")
	buf.Write([]byte{0, 0, 0, 99}) // Version 99
	_, err = index.Deserialize(buf.Bytes())
	if err == nil {
		t.Error("Expected error for invalid version")
	}
}
