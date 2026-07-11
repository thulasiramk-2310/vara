package authz

import (
	"fmt"
	"log"
)

// Denied is an authorization failure. The server maps it to HTTP 403
// (RFC-0018 §8.2). It names the attempted action and resource but never the
// subject's other capabilities (no capability disclosure, §11).
type Denied struct {
	Subject  string
	Action   Capability
	Resource string
}

func (d *Denied) Error() string {
	return fmt.Sprintf("subject %q lacks capability %q on repository %q", d.Subject, d.Action, d.Resource)
}

// Enforcer evaluates authorization decisions against a policy store and audits
// every decision (RFC-0018 §11). It is the only component that reads policy
// (A9); it reads no repository state (A5).
type Enforcer struct {
	store *Store
	log   *log.Logger
}

// NewEnforcer builds an Enforcer over a policy store. logger may be nil to
// disable audit logging.
func NewEnforcer(store *Store, logger *log.Logger) *Enforcer {
	return &Enforcer{store: store, log: logger}
}

// Authorize returns nil if subject holds cap on repo, or a *Denied otherwise.
// The decision is a pure function of identity + policy (A5); a policy that
// failed to load resolves to deny (fail closed, A4).
func (e *Enforcer) Authorize(subject string, cap Capability, repo string) error {
	p, err := e.store.PolicyFor(repo)
	if err != nil && e.log != nil {
		e.log.Printf("authz: policy load error for repository %q: %v", repo, err)
	}
	allow := p.Allows(subject, cap)
	e.audit(subject, cap, repo, allow)
	if !allow {
		return &Denied{Subject: subject, Action: cap, Resource: repo}
	}
	return nil
}

func (e *Enforcer) audit(subject string, cap Capability, repo string, allow bool) {
	if e.log == nil {
		return
	}
	decision := "DENY"
	if allow {
		decision = "ALLOW"
	}
	e.log.Printf("%s subject=%s action=%s resource=%s", decision, subject, cap, repo)
}
