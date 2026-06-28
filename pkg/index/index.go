// Package index implements VARA-RFC-0005.
//
// RFC:
// VARA-RFC-0005 Index (The Staging Area)
//
// Responsibilities:
// - Read and write the `.vara/index` binary file.
// - Verify index integrity (SHA-256 trailer).
// - Maintain Racy VARA mitigation (fingerprints).
//
// This package MUST NOT:
// - Provide CLI logic.
// - Import any commands layer packages.
package index

import (
	"bytes"
	"encoding/binary"

	"github.com/thulasiramk-2310/vara/internal/errors"
	"github.com/thulasiramk-2310/vara/pkg/types"
)

const (
	Signature = "VARA"
	Version   = 1
)

// EntryState defines the state of a file in the index.
type EntryState uint8

const (
	StateUnmodified EntryState = 0
	StateAdded      EntryState = 1
	StateModified   EntryState = 2
	StateDeleted    EntryState = 3
)

// Entry represents a single file tracked by the index.
type Entry struct {
	Fingerprint uint64       // Stat-based modification detection
	ObjectID    types.BlobID // Hash of the blob in the object store
	State       EntryState
	Path        string // Forward-slash separated path
}

// Index represents the deserialized state of `.vara/index`.
type Index struct {
	Entries []Entry
}

// New creates an empty index.
func New() *Index {
	return &Index{Entries: make([]Entry, 0)}
}

// Serialize converts the Index to its binary on-disk format.
func (idx *Index) Serialize() ([]byte, error) {
	var buf bytes.Buffer

	// 1. Header (4 bytes signature, 4 bytes version, 4 bytes entry count)
	buf.WriteString(Signature)
	if err := binary.Write(&buf, binary.BigEndian, uint32(Version)); err != nil {
		return nil, err
	}
	if err := binary.Write(&buf, binary.BigEndian, uint32(len(idx.Entries))); err != nil {
		return nil, err
	}

	// 2. Entries
	for _, e := range idx.Entries {
		if err := binary.Write(&buf, binary.BigEndian, e.Fingerprint); err != nil {
			return nil, err
		}
		if _, err := buf.Write(e.ObjectID[:]); err != nil {
			return nil, err
		}
		if err := binary.Write(&buf, binary.BigEndian, e.State); err != nil {
			return nil, err
		}

		// NUL-terminated path string
		if _, err := buf.WriteString(e.Path); err != nil {
			return nil, err
		}
		if err := buf.WriteByte(0); err != nil {
			return nil, err
		}
	}

	// 3. Extension Block (Empty for now)
	// Not written yet, we can add this later as a 0-length extension list

	return buf.Bytes(), nil
}

// Deserialize parses the binary data into an Index.
func Deserialize(data []byte) (*Index, error) {
	r := bytes.NewReader(data)

	// Read Signature
	sig := make([]byte, 4)
	if _, err := r.Read(sig); err != nil || string(sig) != Signature {
		return nil, errors.ErrCorruptIndex
	}

	// Read Version
	var version uint32
	if err := binary.Read(r, binary.BigEndian, &version); err != nil || version != Version {
		return nil, errors.ErrCorruptIndex
	}

	// Read Entry Count
	var count uint32
	if err := binary.Read(r, binary.BigEndian, &count); err != nil {
		return nil, errors.ErrCorruptIndex
	}

	entries := make([]Entry, count)
	for i := uint32(0); i < count; i++ {
		if err := binary.Read(r, binary.BigEndian, &entries[i].Fingerprint); err != nil {
			return nil, errors.ErrCorruptIndex
		}

		if n, err := r.Read(entries[i].ObjectID[:]); err != nil || n != 32 {
			return nil, errors.ErrCorruptIndex
		}

		if err := binary.Read(r, binary.BigEndian, &entries[i].State); err != nil {
			return nil, errors.ErrCorruptIndex
		}

		pathBytes, err := readUntilNul(r)
		if err != nil {
			return nil, errors.ErrCorruptIndex
		}
		entries[i].Path = string(pathBytes)
	}

	return &Index{Entries: entries}, nil
}

func readUntilNul(r *bytes.Reader) ([]byte, error) {
	var buf bytes.Buffer
	for {
		b, err := r.ReadByte()
		if err != nil {
			return nil, err
		}
		if b == 0 {
			break
		}
		buf.WriteByte(b)
	}
	return buf.Bytes(), nil
}
