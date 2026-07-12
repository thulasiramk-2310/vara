package identity

import "time"

// CredentialKind distinguishes how a bearer credential was minted (RFC-0020
// §5.2/§5.3). It is a STORE detail only; it never reaches Identity (S12), so
// authorization and the transport cannot tell a session from a token.
type CredentialKind string

const (
	KindSession CredentialKind = "session"
	KindToken   CredentialKind = "token"
)

// BearerCredential is a stored session or API token. The secret itself is never
// stored — only SecretHash (RFC-0020 §6.2). Sessions carry an absolute ExpiresAt
// and an idle timeout; tokens carry neither (long-lived until revoked).
type BearerCredential struct {
	ID          string         `json:"id"`
	Kind        CredentialKind `json:"kind"`
	Subject     string         `json:"subject"`
	Name        string         `json:"name,omitempty"`
	SecretHash  string         `json:"secret_hash"`
	CreatedAt   time.Time      `json:"created_at"`
	ExpiresAt   time.Time      `json:"expires_at,omitempty"` // zero = never (tokens)
	IdleSeconds int64          `json:"idle_seconds,omitempty"`
	LastUsedAt  time.Time      `json:"last_used_at,omitempty"`
	Revoked     bool           `json:"revoked"`
}

// valid reports whether the credential authenticates at time now: not revoked,
// not past its absolute expiry, and (for a session) not idle beyond its window.
// Expiry is evaluated here, at resolution time, so it is honored even without a
// background sweep (RFC-0020 §5.2, S9).
func (c *BearerCredential) valid(now time.Time) bool {
	if c.Revoked {
		return false
	}
	if !c.ExpiresAt.IsZero() && now.After(c.ExpiresAt) {
		return false
	}
	if c.IdleSeconds > 0 && !c.LastUsedAt.IsZero() {
		if now.After(c.LastUsedAt.Add(time.Duration(c.IdleSeconds) * time.Second)) {
			return false
		}
	}
	return true
}
