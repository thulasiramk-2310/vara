package devidentity

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/thulasiramk-2310/vara/internal/varahome"
)

// isolate points VARA_HOME at a temp dir and returns a fresh repo .vara dir, so
// identity resolution never touches the developer's real config.
func isolate(t *testing.T) (varaDir string) {
	t.Helper()
	t.Setenv(varahome.EnvHome, t.TempDir())
	varaDir = filepath.Join(t.TempDir(), ".vara")
	if err := os.MkdirAll(varaDir, 0o755); err != nil {
		t.Fatal(err)
	}
	return varaDir
}

func TestResolveRepositoryOverridesDefault(t *testing.T) {
	varaDir := isolate(t)
	if err := SetDefault(Identity{Name: "Default User", Email: "default@example.com"}); err != nil {
		t.Fatal(err)
	}
	if err := SetRepo(varaDir, Identity{Name: "Repo User", Email: "repo@example.com"}); err != nil {
		t.Fatal(err)
	}
	id, scope, err := Resolve(varaDir)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if scope != ScopeRepository {
		t.Fatalf("scope = %s, want Repository", scope)
	}
	if id.Email != "repo@example.com" {
		t.Fatalf("repo identity must override default; got %s", id)
	}
}

func TestResolveFallsBackToDefault(t *testing.T) {
	varaDir := isolate(t)
	if err := SetDefault(Identity{Name: "Default User", Email: "default@example.com"}); err != nil {
		t.Fatal(err)
	}
	id, scope, err := Resolve(varaDir)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if scope != ScopeDefault {
		t.Fatalf("scope = %s, want User default", scope)
	}
	if id.Name != "Default User" {
		t.Fatalf("got %s", id)
	}
}

func TestResolveMissingIsErrNoIdentity(t *testing.T) {
	varaDir := isolate(t)
	_, _, err := Resolve(varaDir)
	if err == nil {
		t.Fatal("expected an error when no identity is configured")
	}
	if _, ok := err.(ErrNoIdentity); !ok {
		t.Fatalf("want ErrNoIdentity, got %T: %v", err, err)
	}
}

func TestStringFormat(t *testing.T) {
	got := Identity{Name: "Ada Lovelace", Email: "ada@example.com"}.String()
	if got != "Ada Lovelace <ada@example.com>" {
		t.Fatalf("String() = %q", got)
	}
}

func TestValidateRejectsBadInput(t *testing.T) {
	varaDir := isolate(t)
	cases := []Identity{
		{Name: "", Email: "a@b.com"},
		{Name: "A", Email: ""},
		{Name: "A", Email: "not-an-email"},
		{Name: "A\nB", Email: "a@b.com"},
	}
	for _, c := range cases {
		if err := SetRepo(varaDir, c); err == nil {
			t.Fatalf("SetRepo(%+v) should have failed validation", c)
		}
	}
}

// TestHalfWrittenFileTreatedAsAbsent proves a file missing name or email does
// not silently produce a partial author.
func TestHalfWrittenFileTreatedAsAbsent(t *testing.T) {
	varaDir := isolate(t)
	if err := os.WriteFile(RepoPath(varaDir), []byte("name = Only Name\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, err := Resolve(varaDir)
	if _, ok := err.(ErrNoIdentity); !ok {
		t.Fatalf("partial identity file should resolve to ErrNoIdentity, got %v", err)
	}
}

// TestBestEffortNeverFails confirms the reflog helper degrades gracefully when
// no identity is configured, so offline switch/branch never break.
func TestBestEffortNeverFails(t *testing.T) {
	varaDir := isolate(t)
	if _, ok := BestEffort(varaDir); ok {
		t.Fatal("BestEffort should report not-ok when nothing is configured")
	}
	if err := SetRepo(varaDir, Identity{Name: "A", Email: "a@b.com"}); err != nil {
		t.Fatal(err)
	}
	if id, ok := BestEffort(varaDir); !ok || id.Email != "a@b.com" {
		t.Fatalf("BestEffort after set = %v ok=%v", id, ok)
	}
}
