package repomanager

import (
	"crypto/rand"
	"encoding/hex"
)

// newID mints an opaque, immutable repository identifier (RFC-0019 §4, §5.3):
// the prefix "repo_" plus 128 bits of randomness. It never changes for the life
// of the repository, including across rename (M11), so higher layers (issues,
// pull requests, permalinks) can reference a repository by an id that a rename
// can never invalidate.
func newID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return "repo_" + hex.EncodeToString(b[:])
}
