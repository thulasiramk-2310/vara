package identity

import (
	"crypto/subtle"
	"maps"
)

// AnonymousSource is the "authentication disabled" source: every request
// resolves to the anonymous identity, regardless of any credential presented.
// It is the default for a server that enforces no identity (RFC-0016 behavior).
type AnonymousSource struct{}

func (AnonymousSource) Authenticate(_ *Credential) (Identity, error) {
	return Anonymous, nil
}

// BasicSource validates HTTP Basic credentials against a fixed user→secret map.
// It requires a credential: a missing one is an authentication failure (401),
// which is how a server demands identity for every request.
type BasicSource struct {
	creds map[string]string // username -> secret
}

// NewBasicSource builds a BasicSource from a username→secret map.
func NewBasicSource(creds map[string]string) *BasicSource {
	return &BasicSource{creds: maps.Clone(creds)}
}

func (s *BasicSource) Authenticate(cred *Credential) (Identity, error) {
	if cred == nil {
		return Identity{}, &AuthError{Reason: "credentials required"}
	}
	if cred.Scheme != SchemeBasic {
		return Identity{}, &AuthError{Reason: "unsupported authentication scheme for this server"}
	}
	want, known := s.creds[cred.User]
	// Constant-time compare, and compare even when the user is unknown, to avoid
	// leaking user existence through timing (RFC-0017 §11).
	match := subtle.ConstantTimeCompare([]byte(cred.Secret), []byte(want)) == 1
	if !known || !match {
		return Identity{}, &AuthError{Reason: "invalid credentials"}
	}
	return Identity{ID: cred.User, Method: MethodBasic}, nil
}

// BearerSource validates opaque bearer tokens against a fixed token→subject map.
// It requires a credential.
type BearerSource struct {
	tokens map[string]string // token -> subject
}

// NewBearerSource builds a BearerSource from a token→subject map.
func NewBearerSource(tokens map[string]string) *BearerSource {
	return &BearerSource{tokens: maps.Clone(tokens)}
}

func (s *BearerSource) Authenticate(cred *Credential) (Identity, error) {
	if cred == nil {
		return Identity{}, &AuthError{Reason: "credentials required"}
	}
	if cred.Scheme != SchemeBearer {
		return Identity{}, &AuthError{Reason: "unsupported authentication scheme for this server"}
	}
	subject, ok := s.tokens[cred.Token]
	if !ok {
		return Identity{}, &AuthError{Reason: "invalid token"}
	}
	return Identity{ID: subject, Method: MethodBearer}, nil
}

// Multi tries each source in turn for a presented credential and returns the
// first success. A nil credential resolves to anonymous iff AllowAnonymous is
// set; otherwise it is an authentication failure. This composes Basic + Bearer
// (+ optional anonymous) on one server.
type Multi struct {
	Sources        []IdentitySource
	AllowAnonymous bool
}

func (m *Multi) Authenticate(cred *Credential) (Identity, error) {
	if cred == nil {
		if m.AllowAnonymous {
			return Anonymous, nil
		}
		return Identity{}, &AuthError{Reason: "credentials required"}
	}
	var last error = &AuthError{Reason: "no configured method accepted the credential"}
	for _, src := range m.Sources {
		id, err := src.Authenticate(cred)
		if err == nil {
			return id, nil
		}
		last = err
	}
	return Identity{}, last
}
