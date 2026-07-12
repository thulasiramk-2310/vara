package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/thulasiramk-2310/vara/internal/authz"
	"github.com/thulasiramk-2310/vara/internal/identity"
)

// newAccountHub starts a server with an AccountManager wired both as the identity
// source (Basic+Bearer) and as the RFC-0020 control plane, sharing one policy
// root with the authorization enforcer.
func newAccountHub(t *testing.T) (*httptest.Server, *identity.AccountManager, string) {
	t.Helper()
	root := t.TempDir()
	policyRoot := t.TempDir()
	mgr, err := identity.NewAccountManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	opts := Options{
		Identity: &identity.Multi{
			Sources:        []identity.IdentitySource{mgr.BasicSource(), mgr.BearerSource()},
			AllowAnonymous: true,
		},
		Authz:    authz.NewEnforcer(authz.NewStore(policyRoot), nil),
		Methods:  []string{"auth-basic", "auth-bearer"},
		Accounts: mgr,
	}
	ts := httptest.NewServer(HandlerWithOptions(root, opts))
	t.Cleanup(ts.Close)
	return ts, mgr, policyRoot
}

func bearer(tok string) string { return "Bearer " + tok }

// TestAccountControlPlane exercises the full account/session/token surface with
// authorization: admin creates bob, bob logs in, uses the session, mints a token,
// and cannot administer accounts.
func TestAccountControlPlane(t *testing.T) {
	ts, mgr, policyRoot := newAccountHub(t)

	// Seed an admin account on the host, and grant it manage-accounts.
	if err := mgr.CreateAccount("admin", "adminpass"); err != nil {
		t.Fatal(err)
	}
	putPolicy(t, policyRoot, "_server", `{"version":1,"subjects":{"admin":["manage-accounts"]}}`)

	// admin creates bob.
	if st, body := cpReq(t, http.MethodPost, ts.URL+"/_vara/accounts", basic("admin", "adminpass"), `{"username":"bob","password":"hunter2pw"}`); st != http.StatusCreated {
		t.Fatalf("admin create bob = %d %s, want 201", st, body)
	}

	// A non-admin (anonymous) cannot create accounts → 403.
	if st, body := cpReq(t, http.MethodPost, ts.URL+"/_vara/accounts", "", `{"username":"eve","password":"eviltoken"}`); st != http.StatusForbidden {
		t.Fatalf("anonymous create = %d %s, want 403", st, body)
	}

	// bob logs in.
	st, body := cpReq(t, http.MethodPost, ts.URL+"/_vara/sessions", "", `{"username":"bob","password":"hunter2pw"}`)
	if st != http.StatusCreated {
		t.Fatalf("bob login = %d %s, want 201", st, body)
	}
	var login struct {
		Secret string `json:"secret"`
	}
	if err := json.Unmarshal([]byte(body), &login); err != nil || login.Secret == "" {
		t.Fatalf("login body missing secret: %s", body)
	}

	// bob uses the session (bearer) to list tokens → 200.
	if st, body := cpReq(t, http.MethodGet, ts.URL+"/_vara/tokens", bearer(login.Secret), ""); st != http.StatusOK {
		t.Fatalf("bob list tokens via session = %d %s, want 200", st, body)
	}

	// bob mints an API token.
	st, body = cpReq(t, http.MethodPost, ts.URL+"/_vara/tokens", bearer(login.Secret), `{"name":"ci"}`)
	if st != http.StatusCreated || !strings.Contains(body, `"secret"`) {
		t.Fatalf("bob create token = %d %s, want 201 with secret", st, body)
	}

	// bob cannot administer accounts (no manage-accounts) → 403.
	if st, _ := cpReq(t, http.MethodPost, ts.URL+"/_vara/accounts", basic("bob", "hunter2pw"), `{"username":"x","password":"password1"}`); st != http.StatusForbidden {
		t.Fatalf("bob create account want 403, got %d", st)
	}
}

// TestLoginFailureIs401 proves a bad login is 401 (never 403, never a hint).
func TestLoginFailureIs401(t *testing.T) {
	ts, mgr, _ := newAccountHub(t)
	_ = mgr.CreateAccount("alice", "password1")

	// Wrong password.
	if st, body := cpReq(t, http.MethodPost, ts.URL+"/_vara/sessions", "", `{"username":"alice","password":"wrong"}`); st != http.StatusUnauthorized {
		t.Fatalf("wrong password login = %d %s, want 401", st, body)
	}
	// Absent user — identical response.
	if st, _ := cpReq(t, http.MethodPost, ts.URL+"/_vara/sessions", "", `{"username":"ghost","password":"wrong"}`); st != http.StatusUnauthorized {
		t.Fatalf("absent user login want 401, got %d", st)
	}
}

// TestSelfPasswordChange proves an account can change its own password without
// manage-accounts, and that the change takes effect.
func TestSelfPasswordChange(t *testing.T) {
	ts, mgr, _ := newAccountHub(t)
	_ = mgr.CreateAccount("carol", "oldpassword")

	// carol changes her own password (self, no manage-accounts) → 204.
	if st, body := cpReq(t, http.MethodPut, ts.URL+"/_vara/accounts/carol/password", basic("carol", "oldpassword"), `{"password":"newpassword"}`); st != http.StatusNoContent {
		t.Fatalf("self password change = %d %s, want 204", st, body)
	}
	// Old password no longer logs in; new one does.
	if st, _ := cpReq(t, http.MethodPost, ts.URL+"/_vara/sessions", "", `{"username":"carol","password":"oldpassword"}`); st != http.StatusUnauthorized {
		t.Fatalf("old password still works, got %d", st)
	}
	if st, _ := cpReq(t, http.MethodPost, ts.URL+"/_vara/sessions", "", `{"username":"carol","password":"newpassword"}`); st != http.StatusCreated {
		t.Fatalf("new password login want 201, got %d", st)
	}

	// carol cannot change SOMEONE ELSE's password without manage-accounts → 403.
	_ = mgr.CreateAccount("dave", "davepassword")
	if st, _ := cpReq(t, http.MethodPut, ts.URL+"/_vara/accounts/dave/password", basic("carol", "newpassword"), `{"password":"hacked12"}`); st != http.StatusForbidden {
		t.Fatalf("cross-account password change want 403, got %d", st)
	}
}

// TestSessionRevocationOverHTTP proves S6 end-to-end: logout invalidates the
// session on the very next request.
func TestSessionRevocationOverHTTP(t *testing.T) {
	ts, mgr, _ := newAccountHub(t)
	_ = mgr.CreateAccount("alice", "password1")

	_, body := cpReq(t, http.MethodPost, ts.URL+"/_vara/sessions", "", `{"username":"alice","password":"password1"}`)
	var login struct {
		Secret string `json:"secret"`
	}
	_ = json.Unmarshal([]byte(body), &login)

	// Works before logout.
	if st, _ := cpReq(t, http.MethodGet, ts.URL+"/_vara/tokens", bearer(login.Secret), ""); st != http.StatusOK {
		t.Fatalf("session should work pre-logout, got %d", st)
	}
	// Logout.
	if st, _ := cpReq(t, http.MethodDelete, ts.URL+"/_vara/sessions/current", bearer(login.Secret), ""); st != http.StatusNoContent {
		t.Fatalf("logout want 204, got %d", st)
	}
	// Immediately invalid.
	if st, _ := cpReq(t, http.MethodGet, ts.URL+"/_vara/tokens", bearer(login.Secret), ""); st != http.StatusUnauthorized {
		t.Fatalf("revoked session still works, got %d", st)
	}
}
