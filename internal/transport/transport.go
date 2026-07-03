// Package transport implements VARA-RFC-0014 §6 (Transport Abstraction). It
// exposes a network-independent interface for exchanging refs and objects with
// a peer repository, plus a local-filesystem implementation.
//
// RFC:
// VARA-RFC-0014 Remote Protocol
//
// Layer: sits above internal/repository (it opens repositories) and below
// internal/commands (which drives clone/fetch/pull/push). It orchestrates
// pkg/transfer, pkg/refs, pkg/graph, and pkg/object.
package transport

import (
	"io"

	"github.com/thulasiramk-2310/vara/pkg/types"
)

// RefAdvertisement is one reference the peer holds.
type RefAdvertisement struct {
	Name   string
	Target types.CommitID
}

// RefUpdate requests moving Name from Old to New. Old is the value the caller
// believes the peer currently holds (compare-and-swap); a zero Old means the
// caller expects the reference not to exist yet. Force permits a
// non-fast-forward update.
type RefUpdate struct {
	Name  string
	Old   types.CommitID
	New   types.CommitID
	Force bool
}

// RefUpdateResult reports the outcome of one RefUpdate.
type RefUpdateResult struct {
	Name   string
	OK     bool
	Reason string
}

// Transport is the peer-independent protocol surface (RFC-0014 §6).
type Transport interface {
	// ListRefs advertises the peer's references.
	ListRefs() ([]RefAdvertisement, error)

	// HeadTarget returns the branch ref name the peer's HEAD points at
	// (e.g. "refs/heads/main"), used by clone to pick the default branch.
	HeadTarget() (string, error)

	// FetchPack returns a VPCK stream containing the closure of wants minus
	// everything reachable from haves.
	FetchPack(wants, haves []types.CommitID) (io.ReadCloser, error)

	// ReceivePack ingests a VPCK stream into the peer, then applies the given
	// reference updates, each guarded by a compare-and-swap on its Old value
	// and a fast-forward check (unless Force).
	ReceivePack(pack io.Reader, updates []RefUpdate) ([]RefUpdateResult, error)

	// Close releases any resources held by the transport.
	Close() error
}

// zeroCommit is the sentinel "no such reference" value.
var zeroCommit types.CommitID
