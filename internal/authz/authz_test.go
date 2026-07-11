package authz

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writePolicy(t *testing.T, dir, repo, body string) string {
	t.Helper()
	path := filepath.Join(dir, repo+".json")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write policy: %v", err)
	}
	return path
}

func TestRequiredForUpdate(t *testing.T) {
	cases := []struct {
		oldZero, newZero, force bool
		want                    Capability
	}{
		{true, false, false, CapCreateRef}, // new ref
		{false, false, false, CapPush},     // fast-forward existing
		{false, false, true, CapForcePush}, // forced
		{false, true, false, CapDeleteRef}, // deletion
		{true, false, true, CapForcePush},  // force on a new ref: force wins over create? no — old=zero
	}
	// Note: newZero and oldZero are mutually exclusive in practice; deletion is
	// checked first, then create, then force.
	for i, c := range cases {
		if i == 4 {
			// old=zero, force=true → create-ref path is not reached because force
			// is only checked after create; verify actual precedence:
			if got := RequiredForUpdate(true, false, true); got != CapCreateRef {
				t.Fatalf("old=zero,force=true → %s, want create-ref (create precedes force)", got)
			}
			continue
		}
		if got := RequiredForUpdate(c.oldZero, c.newZero, c.force); got != c.want {
			t.Fatalf("case %d: got %s want %s", i, got, c.want)
		}
	}
}

func TestDefaultDeny(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	e := NewEnforcer(store, nil)

	// No policy file for "repo" → deny everything (default deny, A4).
	if err := e.Authorize("alice", CapRead, "repo"); err == nil {
		t.Fatal("no policy file should deny read")
	}
	// Unlisted subject → denied even when a file exists for others.
	writePolicy(t, dir, "repo2", `{"version":1,"subjects":{"alice":["read"]}}`)
	if err := e.Authorize("bob", CapRead, "repo2"); err == nil {
		t.Fatal("unlisted subject should be denied")
	}
}

func TestAllowAndDeny(t *testing.T) {
	dir := t.TempDir()
	writePolicy(t, dir, "repo", `{
		"version": 1,
		"subjects": {
			"anonymous": ["read"],
			"alice": ["read", "push"],
			"bob": ["read", "push", "force-push", "delete-ref"]
		}
	}`)
	e := NewEnforcer(NewStore(dir), nil)

	// anonymous may read, not push.
	if err := e.Authorize("anonymous", CapRead, "repo"); err != nil {
		t.Fatalf("anonymous read denied: %v", err)
	}
	if err := e.Authorize("anonymous", CapPush, "repo"); err == nil {
		t.Fatal("anonymous push should be denied")
	}
	// alice may push but not force-push (push ≠ force-push).
	if err := e.Authorize("alice", CapPush, "repo"); err != nil {
		t.Fatalf("alice push denied: %v", err)
	}
	if err := e.Authorize("alice", CapForcePush, "repo"); err == nil {
		t.Fatal("alice force-push should be denied")
	}
	// bob may force-push and delete.
	if err := e.Authorize("bob", CapForcePush, "repo"); err != nil {
		t.Fatalf("bob force-push denied: %v", err)
	}
}

func TestUnknownCapabilityFailsClosed(t *testing.T) {
	dir := t.TempDir()
	writePolicy(t, dir, "repo", `{"version":1,"subjects":{"alice":["read","teleport"]}}`)
	store := NewStore(dir)

	// Loading returns an error (fail-fast), and the enforcer denies (fail closed).
	if _, err := store.PolicyFor("repo"); err == nil {
		t.Fatal("unknown capability should invalidate the policy on load")
	}
	e := NewEnforcer(store, nil)
	if err := e.Authorize("alice", CapRead, "repo"); err == nil {
		t.Fatal("invalid policy must fail closed (deny), even a valid-looking read")
	}
}

func TestAtomicReloadKeepsOldPolicyOnBrokenEdit(t *testing.T) {
	dir := t.TempDir()
	path := writePolicy(t, dir, "repo", `{"version":1,"subjects":{"alice":["read"]}}`)
	store := NewStore(dir)

	if err := (NewEnforcer(store, nil)).Authorize("alice", CapRead, "repo"); err != nil {
		t.Fatalf("initial policy should allow alice read: %v", err)
	}

	// Overwrite with a broken policy and bump mtime so a reload is attempted.
	if err := os.WriteFile(path, []byte(`{"version":1,"subjects":{"alice":["nonsense"]}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(2 * time.Second)
	_ = os.Chtimes(path, future, future)

	// The broken edit must NOT replace the working policy: alice still reads.
	p, err := store.PolicyFor("repo")
	if err == nil {
		t.Fatal("broken reload should report an error")
	}
	if !p.Allows("alice", CapRead) {
		t.Fatal("broken edit replaced the working policy; alice lost read access")
	}
}

func TestValidReloadIsVisible(t *testing.T) {
	dir := t.TempDir()
	path := writePolicy(t, dir, "repo", `{"version":1,"subjects":{"alice":["read"]}}`)
	store := NewStore(dir)

	if _, err := store.PolicyFor("repo"); err != nil {
		t.Fatalf("initial load: %v", err)
	}
	// Grant push; bump mtime.
	if err := os.WriteFile(path, []byte(`{"version":1,"subjects":{"alice":["read","push"]}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(2 * time.Second)
	_ = os.Chtimes(path, future, future)

	p, err := store.PolicyFor("repo")
	if err != nil {
		t.Fatalf("valid reload: %v", err)
	}
	if !p.Allows("alice", CapPush) {
		t.Fatal("valid reload not visible: alice should now have push")
	}
}
