package identity

import (
	"encoding/base64"
	"strings"
)

// Scheme is the authentication scheme parsed from an Authorization header.
type Scheme int

const (
	SchemeBasic Scheme = iota
	SchemeBearer
)

// Credential is a parsed, well-formed authentication credential (RFC-0017 §6.2).
// It is produced by ParseHeader and consumed by an IdentitySource. Parsing and
// authentication are distinct steps: a Credential only ever exists once the
// header syntax is valid, so an IdentitySource never sees malformed input.
type Credential struct {
	Scheme Scheme
	User   string // Basic: the username
	Secret string // Basic: the password
	Token  string // Bearer: the token
}

// AuthError is an authentication failure. The server maps it to HTTP 401
// (RFC-0017 §8.2). Its message is safe to return; it never contains the
// credential value.
type AuthError struct{ Reason string }

func (e *AuthError) Error() string { return e.Reason }

// IdentitySource validates a credential and returns the resolved identity
// (RFC-0017 §7.3). A nil credential means "anonymous". It returns an error ONLY
// for a presented-but-invalid credential; it MUST NOT return an error for a
// valid credential that merely lacks permissions (that is RFC-0018), and it MUST
// NOT read repository state (RFC-0017 C6).
type IdentitySource interface {
	Authenticate(cred *Credential) (Identity, error)
}

// ParseHeader parses an Authorization header value into a Credential
// (RFC-0017 §6.2). An empty header yields (nil, nil) — the anonymous credential.
// A syntactically invalid header yields an AuthError and MUST NOT reach an
// IdentitySource: parsing precedes authentication.
func ParseHeader(header string) (*Credential, error) {
	if header == "" {
		return nil, nil // anonymous
	}
	scheme, rest, ok := strings.Cut(header, " ")
	if !ok {
		return nil, &AuthError{Reason: "malformed Authorization header"}
	}
	rest = strings.TrimSpace(rest)
	switch strings.ToLower(scheme) {
	case "basic":
		raw, err := base64.StdEncoding.DecodeString(rest)
		if err != nil {
			return nil, &AuthError{Reason: "invalid base64 in Basic credential"}
		}
		user, secret, ok := strings.Cut(string(raw), ":")
		if !ok {
			return nil, &AuthError{Reason: "malformed Basic credential (expected user:secret)"}
		}
		return &Credential{Scheme: SchemeBasic, User: user, Secret: secret}, nil
	case "bearer":
		if rest == "" {
			return nil, &AuthError{Reason: "empty bearer token"}
		}
		return &Credential{Scheme: SchemeBearer, Token: rest}, nil
	default:
		return nil, &AuthError{Reason: "unsupported authentication scheme " + scheme}
	}
}
