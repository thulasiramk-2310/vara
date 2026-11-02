package commands

import (
	"net/http/httptest"
	"testing"

	"github.com/thulasiramk-2310/vara/internal/authz"
	"github.com/thulasiramk-2310/vara/internal/identity"
	"github.com/thulasiramk-2310/vara/internal/repomanager"
	"github.com/thulasiramk-2310/vara/internal/server"
	"github.com/thulasiramk-2310/vara/internal/varahome"
)

// controlHub starts an account-backed server with the repository control plane on, and an
// account "dev" that holds the server-level capabilities. VARA_HOME is isolated.
func controlHub(t *testing.T) (baseURL string, mgr *identity.AccountManager, policyRoot string) {
	t.Helper()
	t.Setenv(varahome.EnvHome, t.TempDir())
	root, policyRoot := t.TempDir(), t.TempDir()
	var err error
	mgr, err = identity.NewAccountManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	rm, err := repomanager.New(root, policyRoot, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	opts := server.Options{
		Identity: &identity.Multi{
			Sources:        []identity.IdentitySource{mgr.BasicSource(), mgr.BearerSource()},
			AllowAnonymous: true,
		},
		Authz:    authz.NewEnforcer(authz.NewStore(policyRoot), nil),
		Methods:  []string{"auth-basic", "auth-bearer"},
		Manager:  rm,
		Accounts: mgr,
	}
	ts := httptest.NewServer(server.HandlerWithOptions(root, opts))
	t.Cleanup(ts.Close)
	if err := mgr.CreateAccount("dev", "password1"); err != nil {
		t.Fatal(err)
	}
	putPolicy(t, policyRoot, "_server", `{"version":1,"subjects":{"dev":["create-repo","list-repos","manage-accounts"]}}`)
	return ts.URL, mgr, policyRoot
}

// After "vara login" nothing should need a password on the command line: it lands in shell
// history and in the process list.
func TestRepoCommandsUseTheStoredSession(t *testing.T) {
	base, _, _ := controlHub(t)
	if err := RunLogin([]string{base, "dev"}, AuthConfig{Password: "password1"}); err != nil {
		t.Fatal(err)
	}
	if err := RunRepo([]string{"create", base, "proj"}, RepoConfig{}); err != nil {
		t.Fatalf("repo create after login, with no flags: %v", err)
	}
	if err := RunRepo([]string{"list", base}, RepoConfig{}); err != nil {
		t.Fatalf("repo list after login, with no flags: %v", err)
	}
}

func TestTokenCommandsUseTheStoredSession(t *testing.T) {
	base, _, _ := controlHub(t)
	if err := RunLogin([]string{base, "dev"}, AuthConfig{Password: "password1"}); err != nil {
		t.Fatal(err)
	}
	if err := RunToken([]string{"create", base, "ci"}, AuthConfig{}); err != nil {
		t.Fatalf("token create after login, with no flags: %v", err)
	}
	if err := RunToken([]string{"list", base}, AuthConfig{}); err != nil {
		t.Fatalf("token list after login, with no flags: %v", err)
	}
}

func TestAccountCommandsUseTheStoredSession(t *testing.T) {
	base, mgr, _ := controlHub(t)
	if err := RunLogin([]string{base, "dev"}, AuthConfig{Password: "password1"}); err != nil {
		t.Fatal(err)
	}
	if err := RunAccount([]string{"create", base, "bob"}, AuthConfig{Password: "password22"}); err != nil {
		t.Fatalf("account create after login, with no flags: %v", err)
	}
	if _, _, err := mgr.Login("bob", "password22"); err != nil {
		t.Fatalf("the new account should exist and accept its password: %v", err)
	}
}

// An explicit credential on the command line still wins over the saved session.
func TestAnExplicitCredentialWinsOverTheStoredSession(t *testing.T) {
	base, mgr, _ := controlHub(t)
	if err := mgr.CreateAccount("nobody", "password3"); err != nil {
		t.Fatal(err)
	}
	if err := RunLogin([]string{base, "dev"}, AuthConfig{Password: "password1"}); err != nil {
		t.Fatal(err)
	}
	// "nobody" holds no server capabilities, so listing as them must be refused even though the
	// saved session (dev) could do it.
	if err := RunRepo([]string{"list", base}, RepoConfig{Basic: "nobody:password3"}); err == nil {
		t.Fatal("an explicit --basic for an account without list-repos must be used, and refused")
	}
}

func TestWithoutAnyCredentialStillFailsClearly(t *testing.T) {
	base, _, _ := controlHub(t)
	if err := RunRepo([]string{"list", base}, RepoConfig{}); err == nil {
		t.Fatal("no login and no flags must still be refused")
	}
}
