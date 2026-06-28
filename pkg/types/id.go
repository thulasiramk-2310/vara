// Package types defines strongly typed primitives for VARA.
//
// Responsibilities:
// - Prevent accidental misuse by enforcing type safety (e.g., mixing CommitID and TreeID).
//
// This package MUST NOT:
// - Depend on any other VARA package.
package types

import "encoding/hex"

// BaseID represents a raw 32-byte hash.
type BaseID [32]byte

// String returns the 64-character hex representation.
func (id BaseID) String() string {
	return hex.EncodeToString(id[:])
}

// ParseHex converts a 64-character hex string into a BaseID.
func ParseHex(s string) (BaseID, error) {
	var id BaseID
	if len(s) != 64 {
		return id, hex.ErrLength
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		return id, err
	}
	copy(id[:], b)
	return id, nil
}

// Strong types for specific objects
type ObjectID BaseID

func (id ObjectID) String() string { return BaseID(id).String() }

type CommitID BaseID

func (id CommitID) String() string { return BaseID(id).String() }

type TreeID BaseID

func (id TreeID) String() string { return BaseID(id).String() }

type BlobID BaseID

func (id BlobID) String() string { return BaseID(id).String() }

// Generation number for the commit graph topological sorting.
type Generation uint64
