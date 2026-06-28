// Package hash implements VARA-RFC-0002.
//
// RFC:
// VARA-RFC-0002 Object Format
//
// Responsibilities:
// - SHA-256 generation for VARA objects.
//
// This package MUST NOT:
// - Access refs, objects, or the index on disk.
package hash

import (
	"crypto/sha256"

	"github.com/thulasiramk-2310/vara/pkg/types"
)

// Zero is the zero-value hash.
var Zero types.BaseID

// Compute calculates the SHA-256 hash of the given data.
func Compute(data []byte) types.BaseID {
	return sha256.Sum256(data)
}
