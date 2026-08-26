package commands

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/thulasiramk-2310/vara/internal/authz"
	"github.com/thulasiramk-2310/vara/internal/credstore"
	"github.com/thulasiramk-2310/vara/internal/devidentity"
	"github.com/thulasiramk-2310/vara/internal/identity"
	"github.com/thulasiramk-2310/vara/internal/repository"
	"github.com/thulasiramk-2310/vara/internal/server"
	"github.com/thulasiramk-2310/vara/internal/varahome"
	"github.com/thulasiramk-2310/vara/pkg/object"
	"github.com/thulasiramk-2310/vara/pkg/types"

	"net/http/httptest"
	"os"
)

// authHub starts an account-backed server (Basic+Bearer identity, RFC-0020
// control plane, RFC-0018 authorization) over a root the caller can pre-populate
// with repositories. It also isolates VARA_HOME so the credential store never
// touches the developer's real config.
func authHub(t *testing.T) (baseURL, root string, mgr *identity.AccountManager, policyRoot string) {
	t.Helper()
	t.Setenv(varahome.EnvHome, t.TempDir())
	root = t.TempDir()
	policyRoot = t.TempDir()
	var err error
	mgr, err = identity.NewAccountManager(t.TempDir())
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
		Accounts: mgr,
	}
	ts := httptest.NewServer(server.HandlerWithOptions(root, opts))
	t.Cleanup(ts.Close)
	return ts.URL, root, mgr, policyRoot
}

func putPolicy(t *testing.T, policyRoot, repo, body string) {
	t.Helper()
	if err := writeFileAtomic(filepath.Join(policyRoot, repo+".json"), []byte(body)); err != nil {
		t.Fatal(err)
	}
}

func writeFileAtomic(path string, data []byte) error {
	return atomicWriteFile(path, data, 0o644)
}

// seedServerRepo initialises repoName under root with one commit on main.
func seedServerRepo(t *testing.T, root, repoName string) types.CommitID {
	t.Helper()
	repoRoot := filepath.Join(root, repoName)
	initRepo(t, repoRoot)
	c1 := commitInRepo(t, repoRoot, "a.txt", "hello")
	setMain(t, repoRoot, c1)
	return c1
}

// TestEndToEndAuthenticatedWorkflow is the acceptance test from the task: set a
// commit identity, log in, clone/commit/push authenticated with no per-command
// flags, then log out and observe anonymous authorization behavior.
func TestEndToEndAuthenticatedWorkflow(t *testing.T) {
	baseURL, root, mgr, policyRoot := authHub(t)
	seedServerRepo(t, root, "test-repo")
	if err := mgr.CreateAccount("dev", "password1"); err != nil {
		t.Fatal(err)
	}
	putPolicy(t, policyRoot, "test-repo", `{"version":1,"subjects":{"dev":["read","create-ref","push"]}}`)

	// 1. Log in — the session is stored, not printed.
	if err := RunLogin([]string{baseURL, "dev"}, AuthConfig{Password: "password1"}); err != nil {
		t.Fatalf("login: %v", err)
	}
	store, _ := credstore.Load()
	cred, ok := store.Get(baseURL)
	if !ok || cred.Secret == "" || cred.Username != "dev" {
		t.Fatalf("login should have stored a credential for the origin: %+v ok=%v", cred, ok)
	}

	// 2. whoami uses the stored credential with no flags.
	if err := RunWhoami([]string{baseURL}, AuthConfig{}); err != nil {
		t.Fatalf("whoami after login: %v", err)
	}

	// 3. Clone with NO credential flags — auto-authenticated.
	dst := filepath.Join(t.TempDir(), "clone")
	if _, err := RunClone(baseURL+"/test-repo", dst); err != nil {
		t.Fatalf("authenticated clone: %v", err)
	}

	// 4. Configure a commit identity in the clone, commit, and confirm the
	//    author is the identity — the account ID never enters the commit object.
	if err := devidentity.SetRepo(filepath.Join(dst, repository.VaraDir),
		devidentity.Identity{Name: "Test Developer", Email: "test@example.com"}); err != nil {
		t.Fatal(err)
	}
	writeWorktree(t, dst, "a.txt", "hello\nlocal change")
	ctx := ctxFor(t, dst)
	if err := RunAdd(ctx, []string{"."}); err != nil {
		t.Fatalf("add: %v", err)
	}
	newCommit, err := RunCommit(ctx, "authenticated workflow")
	if err != nil {
		t.Fatalf("commit: %v", err)
	}
	assertCommitAuthor(t, dst, newCommit, "Test Developer <test@example.com>", "dev")

	// 5. Push with NO credential flags — auto-authenticated, authorized.
	ctx = ctxFor(t, dst)
	if _, err := RunPush(ctx, "origin", "main", false); err != nil {
		t.Fatalf("authenticated push: %v", err)
	}
	if got := resolveRef(t, filepath.Join(root, "test-repo"), "refs/heads/main"); got != newCommit {
		t.Fatalf("server did not advance to pushed commit")
	}

	// 6. Credentials never landed inside the repository.
	assertNoSecretInRepo(t, dst, cred.Secret)

	// 7. Log out — the stored credential is cleared.
	if err := RunLogout([]string{baseURL}, AuthConfig{}); err != nil {
		t.Fatalf("logout: %v", err)
	}
	store, _ = credstore.Load()
	if _, ok := store.Get(baseURL); ok {
		t.Fatal("logout should have removed the stored credential")
	}

	// 8. After logout the same push is anonymous → denied by policy.
	writeWorktree(t, dst, "a.txt", "hello\nlocal change\nmore")
	ctx = ctxFor(t, dst)
	_ = RunAdd(ctx, []string{"."})
	if _, err := RunCommit(ctx, "second change"); err != nil {
		t.Fatalf("commit: %v", err)
	}
	ctx = ctxFor(t, dst)
	if _, err := RunPush(ctx, "origin", "main", false); err == nil {
		t.Fatal("push after logout should fail (anonymous not authorized)")
	}
}

// TestOneLoginCoversMultipleRepos proves a single login authenticates clones of
// different repositories on the same server.
func TestOneLoginCoversMultipleRepos(t *testing.T) {
	baseURL, root, mgr, policyRoot := authHub(t)
	seedServerRepo(t, root, "repo-a")
	seedServerRepo(t, root, "repo-b")
	if err := mgr.CreateAccount("dev", "password1"); err != nil {
		t.Fatal(err)
	}
	putPolicy(t, policyRoot, "repo-a", `{"version":1,"subjects":{"dev":["read"]}}`)
	putPolicy(t, policyRoot, "repo-b", `{"version":1,"subjects":{"dev":["read"]}}`)

	if err := RunLogin([]string{baseURL, "dev"}, AuthConfig{Password: "password1"}); err != nil {
		t.Fatalf("login: %v", err)
	}
	for _, repo := range []string{"repo-a", "repo-b"} {
		dst := filepath.Join(t.TempDir(), repo)
		if _, err := RunClone(baseURL+"/"+repo, dst); err != nil {
			t.Fatalf("clone %s with one login: %v", repo, err)
		}
	}
}

// TestInvalidCredentialDoesNotDowngradeToAnonymous proves RFC-0017 §11: a stored
// credential the server rejects yields an authentication error with login
// guidance, never a silent anonymous success.
func TestInvalidCredentialDoesNotDowngradeToAnonymous(t *testing.T) {
	baseURL, root, mgr, policyRoot := authHub(t)
	seedServerRepo(t, root, "test-repo")
	if err := mgr.CreateAccount("dev", "password1"); err != nil {
		t.Fatal(err)
	}
	putPolicy(t, policyRoot, "test-repo", `{"version":1,"subjects":{"dev":["read"]}}`)

	// Store a bogus secret (as if the session had been revoked server-side).
	store, _ := credstore.Load()
	_ = store.Set(baseURL, credstore.Credential{Username: "dev", Secret: "not-a-real-token"})
	if err := store.Save(); err != nil {
		t.Fatal(err)
	}

	dst := filepath.Join(t.TempDir(), "clone")
	_, err := RunClone(baseURL+"/test-repo", dst)
	if err == nil {
		t.Fatal("clone with an invalid credential must fail, not fall back to anonymous")
	}
	if !strings.Contains(err.Error(), "vara login") {
		t.Fatalf("401 error should guide the user to log in, got: %v", err)
	}
}

// TestAuthorizationFailureIsDistinctFrom401 proves RFC-0018: a valid credential
// that lacks push permission yields an authorization error that does NOT tell
// the user to log in again.
func TestAuthorizationFailureIsDistinctFrom401(t *testing.T) {
	baseURL, root, mgr, policyRoot := authHub(t)
	seedServerRepo(t, root, "test-repo")
	if err := mgr.CreateAccount("dev", "password1"); err != nil {
		t.Fatal(err)
	}
	// dev may read (so clone works) but NOT push.
	putPolicy(t, policyRoot, "test-repo", `{"version":1,"subjects":{"dev":["read"]}}`)

	if err := RunLogin([]string{baseURL, "dev"}, AuthConfig{Password: "password1"}); err != nil {
		t.Fatalf("login: %v", err)
	}
	dst := filepath.Join(t.TempDir(), "clone")
	if _, err := RunClone(baseURL+"/test-repo", dst); err != nil {
		t.Fatalf("clone (read allowed): %v", err)
	}

	if err := devidentity.SetRepo(filepath.Join(dst, repository.VaraDir),
		devidentity.Identity{Name: "Dev", Email: "dev@example.com"}); err != nil {
		t.Fatal(err)
	}
	writeWorktree(t, dst, "a.txt", "hello\npush attempt")
	ctx := ctxFor(t, dst)
	_ = RunAdd(ctx, []string{"."})
	if _, err := RunCommit(ctx, "change"); err != nil {
		t.Fatalf("commit: %v", err)
	}
	ctx = ctxFor(t, dst)
	_, err := RunPush(ctx, "origin", "main", false)
	if err == nil {
		t.Fatal("push without push capability must fail")
	}
	msg := err.Error()
	if !strings.Contains(msg, "authoriz") {
		t.Fatalf("403 push error should describe an authorization problem, got: %v", err)
	}
	if strings.Contains(msg, "vara login") {
		t.Fatalf("403 must NOT tell the user to re-login (credential is valid): %v", err)
	}
}

// TestExpiredCredentialErrorsBeforeSending proves a locally-expired credential
// is not sent and is reported with re-login guidance.
func TestExpiredCredentialErrorsBeforeSending(t *testing.T) {
	baseURL, root, _, _ := authHub(t)
	seedServerRepo(t, root, "test-repo")

	store, _ := credstore.Load()
	_ = store.Set(baseURL, credstore.Credential{
		Username:  "dev",
		Secret:    "stale",
		ExpiresAt: "2000-01-01T00:00:00Z",
	})
	if err := store.Save(); err != nil {
		t.Fatal(err)
	}

	_, err := RunClone(baseURL+"/test-repo", filepath.Join(t.TempDir(), "clone"))
	if err == nil {
		t.Fatal("clone with an expired credential should fail")
	}
	if !strings.Contains(err.Error(), "expired") || !strings.Contains(err.Error(), "vara login") {
		t.Fatalf("expired-credential error should mention expiry and login, got: %v", err)
	}
}

// TestAnonymousCloneOfPublicRepoStillWorks proves RFC-0017 §13: with no stored
// credential and a policy permitting anonymous read, clone succeeds.
func TestAnonymousCloneOfPublicRepoStillWorks(t *testing.T) {
	baseURL, root, _, policyRoot := authHub(t)
	seedServerRepo(t, root, "public")
	putPolicy(t, policyRoot, "public", `{"version":1,"subjects":{"anonymous":["read"]}}`)

	dst := filepath.Join(t.TempDir(), "clone")
	if _, err := RunClone(baseURL+"/public", dst); err != nil {
		t.Fatalf("anonymous clone of a public repo should work: %v", err)
	}
}

// --- small helpers local to this file ---

func writeWorktree(t *testing.T, root, name, content string) {
	t.Helper()
	if err := writeFileAtomic(filepath.Join(root, name), []byte(content)); err != nil {
		t.Fatal(err)
	}
}

// assertCommitAuthor reads a commit object and checks its author is want and
// does not leak the account username.
func assertCommitAuthor(t *testing.T, root string, id types.CommitID, want, accountID string) {
	t.Helper()
	s := object.NewStore(filepath.Join(root, repository.VaraDir))
	obj, err := s.Read(types.ObjectID(id))
	if err != nil {
		t.Fatalf("read commit: %v", err)
	}
	c, ok := obj.(*object.Commit)
	if !ok {
		t.Fatalf("object %s is not a commit", id.String()[:7])
	}
	if c.Author != want {
		t.Fatalf("commit author = %q, want %q", c.Author, want)
	}
	if strings.Contains(c.Author, accountID) {
		t.Fatalf("account ID %q leaked into the commit author %q", accountID, c.Author)
	}
}

// assertNoSecretInRepo walks the repo directory and fails if the secret appears
// in any file — credentials must never be written under .vara/.
func assertNoSecretInRepo(t *testing.T, repoRoot, secret string) {
	t.Helper()
	if secret == "" {
		t.Fatal("secret is empty; test is not meaningful")
	}
	if found := grepDir(t, filepath.Join(repoRoot, repository.VaraDir), secret); found != "" {
		t.Fatalf("credential secret leaked into repository file: %s", found)
	}
}

// grepDir returns the path of the first file under dir whose contents contain
// needle, or "" if none do.
func grepDir(t *testing.T, dir, needle string) string {
	t.Helper()
	var hit string
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || hit != "" {
			return nil
		}
		data, rerr := os.ReadFile(path)
		if rerr == nil && strings.Contains(string(data), needle) {
			hit = path
		}
		return nil
	})
	return hit
}
