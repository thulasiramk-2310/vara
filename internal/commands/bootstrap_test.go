package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBootstrapCreatesAdminAndSeedsPolicy(t *testing.T) {
	acct := t.TempDir()
	pol := t.TempDir()
	args := []string{"create", "--accounts", acct, "--policy", pol, "--username", "admin", "--password", "adminpass"}

	if err := RunAccountBootstrap(args); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	// The account exists on disk.
	if _, err := os.Stat(filepath.Join(acct, "accounts", "admin.json")); err != nil {
		t.Fatalf("admin account not created: %v", err)
	}
	// _server.json grants the admin the server-scope admin capabilities.
	data, err := os.ReadFile(filepath.Join(pol, "_server.json"))
	if err != nil {
		t.Fatalf("read _server.json: %v", err)
	}
	for _, cap := range []string{"manage-accounts", "create-repo", "list-repos"} {
		if !strings.Contains(string(data), cap) {
			t.Fatalf("_server.json missing %q: %s", cap, data)
		}
	}
}

func TestBootstrapRefusesSecondUnlessForced(t *testing.T) {
	acct := t.TempDir()
	pol := t.TempDir()
	base := []string{"create", "--accounts", acct, "--policy", pol, "--username", "admin", "--password", "adminpass"}
	if err := RunAccountBootstrap(base); err != nil {
		t.Fatal(err)
	}
	// A second bootstrap (accounts now exist) is refused.
	second := []string{"create", "--accounts", acct, "--policy", pol, "--username", "admin2", "--password", "admin2pass"}
	if err := RunAccountBootstrap(second); err == nil {
		t.Fatal("second bootstrap should be refused without --force")
	}
	// With --force it succeeds, and the original admin's grant is preserved.
	forced := append(second, "--force")
	if err := RunAccountBootstrap(forced); err != nil {
		t.Fatalf("forced bootstrap: %v", err)
	}
	data, _ := os.ReadFile(filepath.Join(pol, "_server.json"))
	if !strings.Contains(string(data), `"admin"`) || !strings.Contains(string(data), `"admin2"`) {
		t.Fatalf("_server.json should grant both admins: %s", data)
	}
}

func TestBootstrapPreservesExistingPolicy(t *testing.T) {
	acct := t.TempDir()
	pol := t.TempDir()
	// Pre-existing policy for an unrelated subject.
	if err := os.WriteFile(filepath.Join(pol, "_server.json"),
		[]byte(`{"version":1,"subjects":{"alice":["create-repo"]}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	args := []string{"create", "--accounts", acct, "--policy", pol, "--username", "admin", "--password", "adminpass"}
	if err := RunAccountBootstrap(args); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(pol, "_server.json"))
	if !strings.Contains(string(data), "alice") {
		t.Fatalf("bootstrap clobbered an existing subject: %s", data)
	}
}

func TestBootstrapRequiresFlags(t *testing.T) {
	if err := RunAccountBootstrap([]string{"create", "--accounts", t.TempDir()}); err == nil {
		t.Fatal("missing --username/--password should error")
	}
	if err := RunAccountBootstrap([]string{"disable", "--accounts", t.TempDir()}); err == nil {
		t.Fatal("on-host bootstrap should only support 'create'")
	}
}
