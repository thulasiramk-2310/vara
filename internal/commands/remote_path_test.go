package commands

import (
	"path/filepath"
	"testing"
)

func realPath(t *testing.T, p string) string {
	t.Helper()
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return p
}

func originURL(t *testing.T, repoRoot, name string) string {
	t.Helper()
	cfg, err := loadConfig(filepath.Join(repoRoot, ".vara"))
	if err != nil {
		t.Fatal(err)
	}
	r, ok := cfg.Remote(name)
	if !ok {
		t.Fatalf("no remote %q", name)
	}
	return r.URL
}

// A relative path means "relative to where I was when I typed it". Stored as typed, it points
// somewhere else the next time the user runs pull from another folder.
func TestCloneByRelativePathStoresAnAbsoluteRemote(t *testing.T) {
	base := realPath(t, t.TempDir())
	server := filepath.Join(base, "server")
	initRepo(t, server)
	setMain(t, server, commitInRepo(t, server, "a.txt", "hello"))

	chdir(t, base)
	if _, err := RunClone("server", "clone"); err != nil {
		t.Fatalf("clone: %v", err)
	}
	got := originURL(t, filepath.Join(base, "clone"), "origin")
	if !filepath.IsAbs(got) || realPath(t, got) != server {
		t.Fatalf("origin should be the absolute path %s, got %q", server, got)
	}

	// and it works from a different folder
	chdir(t, t.TempDir())
	if _, err := RunFetch(ctxFor(t, filepath.Join(base, "clone")), "origin"); err != nil {
		t.Fatalf("fetch from another directory: %v", err)
	}
}

func TestRemoteAddStoresAnAbsolutePathForALocalRepository(t *testing.T) {
	base := realPath(t, t.TempDir())
	server := filepath.Join(base, "server")
	initRepo(t, server)
	work := filepath.Join(base, "work")
	initRepo(t, work)

	chdir(t, base)
	if _, err := RunRemote(ctxFor(t, work), []string{"add", "up", "server"}); err != nil {
		t.Fatal(err)
	}
	if got := originURL(t, work, "up"); !filepath.IsAbs(got) || realPath(t, got) != server {
		t.Fatalf("want %s, got %q", server, got)
	}
}

func TestRemoteAddLeavesRealURLsAsTyped(t *testing.T) {
	work := filepath.Join(t.TempDir(), "work")
	initRepo(t, work)
	for _, u := range []string{"http://example.com/proj", "https://hub.example.org/a/b", "file:///srv/repos/x"} {
		name := "r" + filepath.Base(u)
		if _, err := RunRemote(ctxFor(t, work), []string{"add", name, u}); err != nil {
			t.Fatal(err)
		}
		if got := originURL(t, work, name); got != u {
			t.Fatalf("a URL must be stored exactly as typed: want %q, got %q", u, got)
		}
	}
}

// Repositories cloned by an earlier version hold a relative path. It means "relative to the
// repository", so it keeps working from any folder.
func TestAnOldRelativeRemoteResolvesFromTheRepositoryRoot(t *testing.T) {
	base := realPath(t, t.TempDir())
	server := filepath.Join(base, "server")
	initRepo(t, server)
	setMain(t, server, commitInRepo(t, server, "a.txt", "hello"))
	clone := filepath.Join(base, "clone")
	if _, err := RunClone(server, clone); err != nil {
		t.Fatal(err)
	}
	cfg, _ := loadConfig(filepath.Join(clone, ".vara"))
	cfg.RemoveRemote("origin")
	cfg.AddRemote("origin", "../server") // as an old version stored it
	if err := saveConfig(filepath.Join(clone, ".vara"), cfg); err != nil {
		t.Fatal(err)
	}

	chdir(t, t.TempDir())
	if _, err := RunFetch(ctxFor(t, clone), "origin"); err != nil {
		t.Fatalf("an old relative remote should resolve from the repository root: %v", err)
	}
}
