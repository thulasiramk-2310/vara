package identity

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// --- password & secret handling (S3/S4) --------------------------------------

func TestPasswordHashing(t *testing.T) {
	pw := "correct horse battery staple"
	h, err := HashPassword(pw)
	if err != nil {
		t.Fatal(err)
	}
	// Not plaintext, not a bare fast hash — a self-describing argon2id PHC string.
	if strings.Contains(h, pw) {
		t.Fatalf("hash leaks plaintext: %s", h)
	}
	if !strings.HasPrefix(h, "$argon2id$") {
		t.Fatalf("not an argon2id PHC string: %s", h)
	}
	ok, rehash, err := VerifyPassword(h, pw)
	if err != nil || !ok {
		t.Fatalf("verify right password: ok=%v err=%v", ok, err)
	}
	if rehash {
		t.Fatalf("default-cost hash should not need rehash")
	}
	if ok, _, _ := VerifyPassword(h, "wrong"); ok {
		t.Fatalf("verify accepted a wrong password")
	}
	// Per-password salt: same password hashes to different strings.
	h2, _ := HashPassword(pw)
	if h == h2 {
		t.Fatalf("identical hashes for same password — missing per-password salt")
	}
}

func TestSecretHashing(t *testing.T) {
	p, h, err := NewSecret()
	if err != nil {
		t.Fatal(err)
	}
	if HashSecret(p) != h {
		t.Fatalf("stored hash does not match hash of plaintext")
	}
	if strings.Contains(h, p) {
		t.Fatalf("stored hash contains the plaintext secret")
	}
	p2, _, _ := NewSecret()
	if p == p2 {
		t.Fatalf("two minted secrets collided")
	}
}

// --- credential validity / expiry (S9) ---------------------------------------

func TestBearerValidity(t *testing.T) {
	now := time.Now()
	cases := map[string]struct {
		c    BearerCredential
		want bool
	}{
		"fresh token (no expiry)": {BearerCredential{}, true},
		"revoked":                 {BearerCredential{Revoked: true}, false},
		"absolute-expired":        {BearerCredential{ExpiresAt: now.Add(-time.Minute)}, false},
		"absolute-future":         {BearerCredential{ExpiresAt: now.Add(time.Hour)}, true},
		"idle-expired":            {BearerCredential{IdleSeconds: 60, LastUsedAt: now.Add(-2 * time.Minute)}, false},
		"idle-fresh":              {BearerCredential{IdleSeconds: 3600, LastUsedAt: now.Add(-time.Minute)}, true},
	}
	for name, tc := range cases {
		if got := tc.c.valid(now); got != tc.want {
			t.Errorf("%s: valid = %v, want %v", name, got, tc.want)
		}
	}
}

// --- account manager ---------------------------------------------------------

func mgr(t *testing.T) *AccountManager {
	t.Helper()
	m, err := NewAccountManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestLoginAndEnumerationResistance(t *testing.T) {
	m := mgr(t)
	if err := m.CreateAccount("alice", "password1"); err != nil {
		t.Fatal(err)
	}

	// Wrong password and absent user return the SAME error (no enumeration).
	if _, _, err := m.Login("alice", "nope"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("wrong password err = %v, want ErrInvalidCredentials", err)
	}
	if _, _, err := m.Login("ghost", "whatever"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("absent user err = %v, want ErrInvalidCredentials", err)
	}

	// Success mints a session that authenticates as bearer.
	secret, exp, err := m.Login("alice", "password1")
	if err != nil || secret == "" || !exp.After(time.Now()) {
		t.Fatalf("login: secret=%q exp=%v err=%v", secret, exp, err)
	}
	id, err := m.BearerSource().Authenticate(&Credential{Scheme: SchemeBearer, Token: secret})
	if err != nil || id.ID != "alice" || id.Method != MethodBearer {
		t.Fatalf("session bearer auth: id=%+v err=%v", id, err)
	}

	// Logout revokes it immediately (S6).
	if err := m.Logout(secret); err != nil {
		t.Fatal(err)
	}
	if _, err := m.BearerSource().Authenticate(&Credential{Scheme: SchemeBearer, Token: secret}); err == nil {
		t.Fatalf("logged-out session still authenticates")
	}
}

func TestPasswordPolicy(t *testing.T) {
	m := mgr(t)
	if err := m.CreateAccount("a", "short"); !errors.Is(err, ErrWeakPassword) {
		t.Fatalf("short password err = %v, want ErrWeakPassword", err)
	}
	// A long passphrase is accepted and not truncated (§6.3).
	long := strings.Repeat("correct horse ", 40) // ~560 bytes
	if err := m.CreateAccount("bob", long); err != nil {
		t.Fatalf("long passphrase rejected: %v", err)
	}
	if _, _, err := m.Login("bob", long); err != nil {
		t.Fatalf("long passphrase login failed: %v", err)
	}
}

func TestBasicSourceRejectsWrongAndAbsent(t *testing.T) {
	m := mgr(t)
	_ = m.CreateAccount("alice", "password1")
	src := m.BasicSource()
	if id, err := src.Authenticate(&Credential{Scheme: SchemeBasic, User: "alice", Secret: "password1"}); err != nil || id.ID != "alice" || id.Method != MethodBasic {
		t.Fatalf("valid basic: id=%+v err=%v", id, err)
	}
	if _, err := src.Authenticate(&Credential{Scheme: SchemeBasic, User: "alice", Secret: "wrong"}); err == nil {
		t.Fatalf("wrong password accepted")
	}
	if _, err := src.Authenticate(&Credential{Scheme: SchemeBasic, User: "ghost", Secret: "x"}); err == nil {
		t.Fatalf("absent account accepted")
	}
}

// TestDisableRevokesAllCredentials proves S6: disabling an account immediately
// kills its sessions and tokens, and its password login.
func TestDisableRevokesAllCredentials(t *testing.T) {
	m := mgr(t)
	_ = m.CreateAccount("alice", "password1")
	_, tokenSecret, err := m.CreateToken("alice", "ci")
	if err != nil {
		t.Fatal(err)
	}
	sessSecret, _, _ := m.Login("alice", "password1")

	// Both authenticate before disable.
	for _, s := range []string{tokenSecret, sessSecret} {
		if _, err := m.BearerSource().Authenticate(&Credential{Scheme: SchemeBearer, Token: s}); err != nil {
			t.Fatalf("credential should authenticate pre-disable: %v", err)
		}
	}

	if err := m.DisableAccount("alice"); err != nil {
		t.Fatal(err)
	}

	// Neither authenticates after disable; password login fails too.
	for _, s := range []string{tokenSecret, sessSecret} {
		if _, err := m.BearerSource().Authenticate(&Credential{Scheme: SchemeBearer, Token: s}); err == nil {
			t.Fatalf("credential still authenticates after account disabled")
		}
	}
	if _, _, err := m.Login("alice", "password1"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("disabled account login = %v, want ErrInvalidCredentials", err)
	}
}

func TestTokenLifecycle(t *testing.T) {
	m := mgr(t)
	_ = m.CreateAccount("alice", "password1")
	id, secret, err := m.CreateToken("alice", "ci")
	if err != nil {
		t.Fatal(err)
	}
	// Listing shows metadata, never the secret.
	infos, _ := m.ListTokens("alice")
	if len(infos) != 1 || infos[0].ID != id || infos[0].Name != "ci" {
		t.Fatalf("ListTokens = %+v", infos)
	}
	// Authenticates, then revoked → fails.
	if _, err := m.BearerSource().Authenticate(&Credential{Scheme: SchemeBearer, Token: secret}); err != nil {
		t.Fatalf("token should authenticate: %v", err)
	}
	if err := m.RevokeToken("alice", id); err != nil {
		t.Fatal(err)
	}
	if _, err := m.BearerSource().Authenticate(&Credential{Scheme: SchemeBearer, Token: secret}); err == nil {
		t.Fatalf("revoked token still authenticates")
	}
	if err := m.RevokeToken("alice", id); !errors.Is(err, ErrNoCredential) {
		t.Fatalf("re-revoke = %v, want ErrNoCredential", err)
	}
}
