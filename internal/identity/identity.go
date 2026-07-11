// Package identity implements VARA-RFC-0017 (Identity & Authentication). It
// resolves a caller's credential into an Identity, or reports an authentication
// failure. It answers only "who is calling?" — never "may they?" (that is
// RFC-0018, package authz).
//
// Layer: a server-binding concern, ABOVE internal/transport. It imports no VARA
// engine package and reads no repository state (RFC-0017 C1, C6). The engine
// cannot import it, and it does not import the engine.
package identity

// IdentityMethod is the closed enumeration of ways an identity is established
// (RFC-0017 §5, §7.2). Active in v1: Anonymous, Basic, Bearer. The rest are
// reserved so their introduction is additive.
type IdentityMethod int

const (
	MethodAnonymous IdentityMethod = iota // no credential presented
	MethodBasic                           // HTTP Basic (RFC 7617)
	MethodBearer                          // Bearer token (RFC 6750)

	// Reserved — not accepted in v1 (RFC-0017 §5.2).
	MethodOAuth2
	MethodOIDC
	MethodMutualTLS
	MethodSSH
)

func (m IdentityMethod) String() string {
	switch m {
	case MethodAnonymous:
		return "anonymous"
	case MethodBasic:
		return "basic"
	case MethodBearer:
		return "bearer"
	case MethodOAuth2:
		return "oauth2"
	case MethodOIDC:
		return "oidc"
	case MethodMutualTLS:
		return "mtls"
	case MethodSSH:
		return "ssh"
	default:
		return "unknown"
	}
}

// Identity is the validated principal a credential resolves to (RFC-0017 §7.1).
// It is intentionally tiny: a stable ID and the method that produced it, and
// NOTHING permission-bearing (no roles, groups, or repository access — those are
// RFC-0018's data).
type Identity struct {
	ID     string
	Method IdentityMethod
}

// Anonymous is the reserved identity for a request that presents no credential
// (RFC-0017 §4). It is an ordinary identity, not a bypass: authorization
// (RFC-0018) governs it like any other subject.
var Anonymous = Identity{ID: "anonymous", Method: MethodAnonymous}
