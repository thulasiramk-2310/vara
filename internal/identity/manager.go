package identity

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

// AccountManager is the identity layer's account service (RFC-0020). It owns the
// account and credential stores and exposes the high-level operations the control
// plane drives (create/disable/delete/change-password, login/logout, token
// create/list/revoke), plus the two persistent IdentitySource adapters that plug
// into the RFC-0017 Multi source. It changes nothing about the Identity type or
// the pipeline — it only fills RFC-0017's interface (S1).
type AccountManager struct {
	accounts AccountStore
	creds    CredentialStore

	sessionAbsolute time.Duration
	sessionIdle     time.Duration
}

// ErrInvalidCredentials is the single, indistinguishable failure a login returns
// whether the username is absent, disabled, or the password is wrong — so a
// caller cannot enumerate accounts (RFC-0020 §12).
var ErrInvalidCredentials = errors.New("invalid credentials")

// ErrWeakPassword is returned when a password fails the minimal policy (§6.3).
var ErrWeakPassword = errors.New("password does not meet policy")

// NewAccountManager builds a file-backed manager rooted at dir (creating
// dir/accounts and dir/credentials). Session lifetimes use sane v1 defaults.
func NewAccountManager(dir string) (*AccountManager, error) {
	as, err := newFileAccountStore(fmt.Sprintf("%s/accounts", dir))
	if err != nil {
		return nil, err
	}
	cs, err := newFileCredentialStore(fmt.Sprintf("%s/credentials", dir))
	if err != nil {
		return nil, err
	}
	return &AccountManager{
		accounts:        as,
		creds:           cs,
		sessionAbsolute: 24 * time.Hour,
		sessionIdle:     2 * time.Hour,
	}, nil
}

// Account returns an account's record (without exposing the password hash to
// callers that only need the descriptor). Used by the control plane to render a
// descriptor; never returns a secret.
func (m *AccountManager) Account(username string) (*Account, error) {
	return m.accounts.Get(username)
}

// --- account administration ---------------------------------------------------

// CreateAccount creates an Active account with the given password.
func (m *AccountManager) CreateAccount(username, password string) error {
	if !ValidUsername(username) {
		return fmt.Errorf("%w: invalid username", ErrNoAccount)
	}
	if !ValidPassword(password) {
		return ErrWeakPassword
	}
	hash, err := HashPassword(password)
	if err != nil {
		return err
	}
	return m.accounts.Create(&Account{
		Username: username, PasswordHash: hash,
		State: AccountActive, CreatedAt: time.Now().UTC(),
	})
}

// DisableAccount disables an account and revokes all its credentials at once
// (RFC-0020 §5.1, §5.4): existing sessions/tokens stop resolving immediately.
func (m *AccountManager) DisableAccount(username string) error {
	a, err := m.accounts.Get(username)
	if err != nil {
		return err
	}
	a.State = AccountDisabled
	if err := m.accounts.Update(a); err != nil {
		return err
	}
	return m.creds.RevokeAllForSubject(username)
}

// DeleteAccount removes an account and cascades to all its credentials (§5.4).
func (m *AccountManager) DeleteAccount(username string) error {
	if _, err := m.accounts.Get(username); err != nil {
		return err
	}
	if err := m.creds.RevokeAllForSubject(username); err != nil {
		return err
	}
	return m.accounts.Delete(username)
}

// ChangePassword re-hashes an account's password (§6.1).
func (m *AccountManager) ChangePassword(username, newPassword string) error {
	if !ValidPassword(newPassword) {
		return ErrWeakPassword
	}
	a, err := m.accounts.Get(username)
	if err != nil {
		return err
	}
	hash, err := HashPassword(newPassword)
	if err != nil {
		return err
	}
	a.PasswordHash = hash
	return m.accounts.Update(a)
}

// --- sessions -----------------------------------------------------------------

// Login validates a username+password and mints a session, returning the session
// secret (shown once) and its absolute expiry. On any failure it returns
// ErrInvalidCredentials after spending KDF time, so timing does not reveal
// whether the account exists (§12).
func (m *AccountManager) Login(username, password string) (secret string, expiresAt time.Time, err error) {
	a, gerr := m.accounts.Get(username)
	if gerr != nil || a.State != AccountActive {
		SpendVerifyTime(password)
		return "", time.Time{}, ErrInvalidCredentials
	}
	match, needsRehash, verr := VerifyPassword(a.PasswordHash, password)
	if verr != nil || !match {
		return "", time.Time{}, ErrInvalidCredentials
	}
	if needsRehash {
		if nh, e := HashPassword(password); e == nil {
			a.PasswordHash = nh
			_ = m.accounts.Update(a)
		}
	}
	now := time.Now().UTC()
	plaintext, hash, err := NewSecret()
	if err != nil {
		return "", time.Time{}, err
	}
	exp := now.Add(m.sessionAbsolute)
	cred := &BearerCredential{
		ID: newCredID(), Kind: KindSession, Subject: username, SecretHash: hash,
		CreatedAt: now, ExpiresAt: exp, IdleSeconds: int64(m.sessionIdle / time.Second), LastUsedAt: now,
	}
	if err := m.creds.Put(cred); err != nil {
		return "", time.Time{}, err
	}
	return plaintext, exp, nil
}

// Logout revokes the session identified by the presented secret (idempotent).
func (m *AccountManager) Logout(secret string) error {
	return m.creds.DeleteBySecretHash(HashSecret(secret))
}

// --- API tokens ---------------------------------------------------------------

// TokenInfo is the metadata view of a token (never its secret), for listing.
type TokenInfo struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	CreatedAt  time.Time `json:"created_at"`
	LastUsedAt time.Time `json:"last_used_at,omitempty"`
}

// CreateToken mints a long-lived API token for subject, returning its id and the
// secret (shown once).
func (m *AccountManager) CreateToken(subject, name string) (id, secret string, err error) {
	plaintext, hash, err := NewSecret()
	if err != nil {
		return "", "", err
	}
	id = newCredID()
	cred := &BearerCredential{
		ID: id, Kind: KindToken, Subject: subject, Name: name,
		SecretHash: hash, CreatedAt: time.Now().UTC(),
	}
	if err := m.creds.Put(cred); err != nil {
		return "", "", err
	}
	return id, plaintext, nil
}

// ListTokens returns the subject's tokens as metadata only.
func (m *AccountManager) ListTokens(subject string) ([]TokenInfo, error) {
	creds, err := m.creds.ListBySubject(subject)
	if err != nil {
		return nil, err
	}
	var out []TokenInfo
	for _, c := range creds {
		if c.Kind == KindToken {
			out = append(out, TokenInfo{ID: c.ID, Name: c.Name, CreatedAt: c.CreatedAt, LastUsedAt: c.LastUsedAt})
		}
	}
	return out, nil
}

// RevokeToken revokes one of the subject's tokens by id.
func (m *AccountManager) RevokeToken(subject, id string) error {
	return m.creds.RevokeByID(subject, id)
}

// --- identity sources (RFC-0017 integration) ----------------------------------

// BasicSource returns an IdentitySource that authenticates username+password
// against the account store.
func (m *AccountManager) BasicSource() IdentitySource { return accountBasicSource{m} }

// BearerSource returns an IdentitySource that authenticates a bearer secret
// (session or token) against the credential store.
func (m *AccountManager) BearerSource() IdentitySource { return accountBearerSource{m} }

type accountBasicSource struct{ m *AccountManager }

func (s accountBasicSource) Authenticate(cred *Credential) (Identity, error) {
	if cred == nil {
		return Identity{}, &AuthError{Reason: "credentials required"}
	}
	if cred.Scheme != SchemeBasic {
		return Identity{}, &AuthError{Reason: "unsupported authentication scheme for this server"}
	}
	a, err := s.m.accounts.Get(cred.User)
	if err != nil || a.State != AccountActive {
		// Spend the same KDF time as a real verification so timing does not reveal
		// whether the account exists or is disabled (RFC-0020 §12).
		SpendVerifyTime(cred.Secret)
		return Identity{}, &AuthError{Reason: "invalid credentials"}
	}
	match, needsRehash, verr := VerifyPassword(a.PasswordHash, cred.Secret)
	if verr != nil || !match {
		return Identity{}, &AuthError{Reason: "invalid credentials"}
	}
	if needsRehash {
		if nh, e := HashPassword(cred.Secret); e == nil {
			a.PasswordHash = nh
			_ = s.m.accounts.Update(a)
		}
	}
	return Identity{ID: a.Username, Method: MethodBasic}, nil
}

type accountBearerSource struct{ m *AccountManager }

func (s accountBearerSource) Authenticate(cred *Credential) (Identity, error) {
	if cred == nil {
		return Identity{}, &AuthError{Reason: "credentials required"}
	}
	if cred.Scheme != SchemeBearer {
		return Identity{}, &AuthError{Reason: "unsupported authentication scheme for this server"}
	}
	hash := HashSecret(cred.Token)
	c, err := s.m.creds.GetBySecretHash(hash)
	if err != nil {
		return Identity{}, &AuthError{Reason: "invalid token"}
	}
	now := time.Now().UTC()
	if !c.valid(now) {
		return Identity{}, &AuthError{Reason: "invalid token"}
	}
	// The account must still exist and be Active — so disabling an account
	// immediately kills its tokens too (S6), without a sweep.
	a, err := s.m.accounts.Get(c.Subject)
	if err != nil || a.State != AccountActive {
		return Identity{}, &AuthError{Reason: "invalid token"}
	}
	_ = s.m.creds.Touch(hash, now)
	return Identity{ID: c.Subject, Method: MethodBearer}, nil
}

func newCredID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return "cred_" + hex.EncodeToString(b[:])
}
