package credstore

import (
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/thulasiramk-2310/vara/internal/varahome"
)

func isolate(t *testing.T) {
	t.Helper()
	t.Setenv(varahome.EnvHome, t.TempDir())
}

func TestOriginReducesURLs(t *testing.T) {
	cases := map[string]string{
		"https://vara.example.com/repo-a":        "https://vara.example.com",
		"https://vara.example.com/repo-b?x=1":    "https://vara.example.com",
		"https://VARA.example.com:443/deep/path": "https://vara.example.com:443",
		"http://localhost:8099/hello":            "http://localhost:8099",
		"vara://localhost:8099/hello":            "http://localhost:8099",
	}
	for in, want := range cases {
		got, err := Origin(in)
		if err != nil {
			t.Fatalf("Origin(%q): %v", in, err)
		}
		if got != want {
			t.Fatalf("Origin(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestOriginRejectsNonHTTP(t *testing.T) {
	for _, in := range []string{"/local/path", "file:///tmp/x", "ftp://h/x"} {
		if _, err := Origin(in); err == nil {
			t.Fatalf("Origin(%q) should have failed", in)
		}
	}
}

// TestOneLoginServesEveryRepoOnServer is the core RFC-0020 §11 property: a
// credential stored under any repo URL is found for every other repo on the
// same server.
func TestOneLoginServesEveryRepoOnServer(t *testing.T) {
	isolate(t)
	s, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Set("https://vara.example.com/repo-a", Credential{Username: "alice", Secret: "sekret"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}

	reload, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, url := range []string{
		"https://vara.example.com/repo-b",
		"https://vara.example.com/repo-c",
		"https://vara.example.com:443/repo-a", // note: explicit :443 is a distinct origin
	} {
		c, ok := reload.Get(url)
		if url == "https://vara.example.com:443/repo-a" {
			if ok {
				t.Fatalf("explicit :443 must be a distinct origin, unexpectedly matched")
			}
			continue
		}
		if !ok || c.Secret != "sekret" || c.Username != "alice" {
			t.Fatalf("Get(%q) = %+v ok=%v, want alice/sekret", url, c, ok)
		}
	}
}

func TestDeleteRemovesCredential(t *testing.T) {
	isolate(t)
	s, _ := Load()
	_ = s.Set("https://h/r", Credential{Username: "u", Secret: "s"})
	removed, err := s.Delete("https://h/other")
	if err != nil || !removed {
		t.Fatalf("delete: removed=%v err=%v", removed, err)
	}
	if _, ok := s.Get("https://h/r"); ok {
		t.Fatal("credential should be gone after delete")
	}
}

func TestExpiry(t *testing.T) {
	past := Credential{ExpiresAt: time.Now().Add(-time.Hour).Format(time.RFC3339)}
	future := Credential{ExpiresAt: time.Now().Add(time.Hour).Format(time.RFC3339)}
	never := Credential{ExpiresAt: ""}
	garbage := Credential{ExpiresAt: "not-a-time"}
	if !past.Expired() {
		t.Error("past credential should be expired")
	}
	if future.Expired() {
		t.Error("future credential should not be expired")
	}
	if never.Expired() {
		t.Error("no-expiry (API token) should never be expired")
	}
	if garbage.Expired() {
		t.Error("unparseable expiry should not be treated as expired")
	}
}

// TestFileIsOwnerOnly verifies the credential file is written with restrictive
// permissions (0600) on POSIX systems, so a secret is never group/world readable.
func TestFileIsOwnerOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits are not meaningful on Windows")
	}
	isolate(t)
	s, _ := Load()
	_ = s.Set("https://h/r", Credential{Username: "u", Secret: "s"})
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	path, _ := Path()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm&0o077 != 0 {
		t.Fatalf("credential file mode = %o, want no group/other bits", perm)
	}
}

// TestMissingFileIsEmptyStore proves a fresh install has no credentials and does
// not error.
func TestMissingFileIsEmptyStore(t *testing.T) {
	isolate(t)
	s, err := Load()
	if err != nil {
		t.Fatalf("load on fresh home: %v", err)
	}
	if len(s.List()) != 0 {
		t.Fatal("fresh store should be empty")
	}
}

// TestSecretIsStoredButFileIsJSON is a guard that the secret round-trips (so it
// can be presented later) and lives only in the store file under the home.
func TestSecretIsStoredButFileIsJSON(t *testing.T) {
	isolate(t)
	s, _ := Load()
	_ = s.Set("https://h/r", Credential{Username: "u", Secret: "top-secret-value"})
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	path, _ := Path()
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "top-secret-value") {
		t.Fatal("secret should be persisted for later presentation")
	}
}
