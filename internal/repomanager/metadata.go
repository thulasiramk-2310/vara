// Package repomanager implements VARA-RFC-0019 (Repository Management &
// Ownership): the server-side control plane that brings repositories into
// existence, removes them, renames them, lists them, and records what each one
// is. It orchestrates three artifacts — content (via repository.Init), policy
// (via internal/authz), and metadata (owned here) — and never reimplements
// repository layout (RFC-0003) or reads repository content (RFC-0019 M1–M3).
//
// Layer: above internal/repository and internal/authz, beside internal/transport,
// below internal/server. It has NO upward imports (M12) and the engine never
// imports it.
package repomanager

import "time"

// State is a repository's lifecycle position (RFC-0019 §5.4). Creating and
// Deleting are on-disk tombstones: never served, never listed as live.
type State string

const (
	StateCreating State = "creating"
	StateActive   State = "active"
	StateArchived State = "archived"
	StateDeleting State = "deleting"
)

// live reports whether a repository in this state is a real, addressable
// repository (Active or Archived) as opposed to a tombstone (Creating/Deleting).
func (s State) live() bool { return s == StateActive || s == StateArchived }

// Visibility is the closed enum of RFC-0019 §6.2. v1 treats every repository as
// private for authorization; public semantics are reserved.
type Visibility string

const (
	VisibilityPrivate Visibility = "private"
	VisibilityPublic  Visibility = "public"
)

func (v Visibility) valid() bool { return v == VisibilityPrivate || v == VisibilityPublic }

// Metadata is the repository's "what is this?" record (RFC-0019 §6.1). It is the
// only ID-addressed artifact and carries NO capability grants — those live only
// in policy (M9). It is also the descriptor the control plane returns.
type Metadata struct {
	ID          string     `json:"id"`   // immutable (M11)
	Name        string     `json:"name"` // mutable (rename)
	Owner       string     `json:"owner"`
	Visibility  Visibility `json:"visibility"`
	State       State      `json:"state"`
	Description string     `json:"description"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// clone returns a copy so callers never hold a pointer into the manager's cache.
func (m *Metadata) clone() *Metadata {
	c := *m
	return &c
}
