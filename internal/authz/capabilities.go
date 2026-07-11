// Package authz implements VARA-RFC-0018 (Authorization & Repository Policy). It
// evaluates (subject, capability, repository) -> allow | deny from a
// server-managed policy. It answers only "may this subject do this?" — never
// "who are they?" (RFC-0017) and never how the repository stores data (engine).
//
// Layer: a server-binding concern, ABOVE internal/transport. It imports no VARA
// engine or transport package and reads NO repository state — decisions are a
// pure function of identity + policy (RFC-0018 A1, A5). Capability determination
// takes plain booleans (§8.1) rather than transport types, so authz stays
// independent of the transport.
package authz

// Capability is a member of the closed, operation-centric vocabulary
// (RFC-0018 §5.1). A capability exists only if it corresponds to an observable
// transport operation, which is why client verbs clone/fetch/pull (wire-
// indistinguishable) collapse to `read`.
type Capability string

const (
	CapRead      Capability = "read"       // GET /info/refs, POST /fetch
	CapCreateRef Capability = "create-ref" // receive: new ref (old = zero)
	CapPush      Capability = "push"       // receive: fast-forward existing ref
	CapForcePush Capability = "force-push" // receive: force = true
	CapDeleteRef Capability = "delete-ref" // receive: deletion (new = zero, reserved)
)

// Known is the complete closed set of v1 capabilities. A policy file naming any
// token outside this set is invalid (fail-fast, RFC-0018 §7.1).
var Known = map[Capability]bool{
	CapRead:      true,
	CapCreateRef: true,
	CapPush:      true,
	CapForcePush: true,
	CapDeleteRef: true,
}

// RequiredForUpdate maps a ref update to the capability it requires, from the
// update's shape ALONE — never from repository state (RFC-0018 §8.1, A5).
// oldZero: the ref does not yet exist; newZero: a deletion; force: declared
// intent to rewrite history.
func RequiredForUpdate(oldZero, newZero, force bool) Capability {
	switch {
	case newZero:
		return CapDeleteRef
	case oldZero:
		return CapCreateRef
	case force:
		return CapForcePush
	default:
		return CapPush
	}
}
