package config

import (
	"path/filepath"
	"testing"
)

func TestParseAndGet(t *testing.T) {
	text := `
# a comment
[user]
    name = Thulasiram K
    email = t@example.com

[remote "origin"]
    url = /repos/project
    fetch = +refs/heads/*:refs/remotes/origin/*
`
	c, err := Parse(text)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if v, ok := c.Get("user", "", "name"); !ok || v != "Thulasiram K" {
		t.Fatalf("user.name = %q, %v", v, ok)
	}
	if v, ok := c.Get("user", "", "email"); !ok || v != "t@example.com" {
		t.Fatalf("user.email = %q, %v", v, ok)
	}
	r, ok := c.Remote("origin")
	if !ok {
		t.Fatal("origin remote missing")
	}
	if r.URL != "/repos/project" {
		t.Fatalf("origin url = %q", r.URL)
	}
}

func TestAddAndRoundTrip(t *testing.T) {
	c := New()
	c.AddRemote("origin", "/repos/upstream")
	c.Set("user", "", "name", "Alice")

	text := c.Serialize()
	c2, err := Parse(text)
	if err != nil {
		t.Fatalf("re-parse: %v\n%s", err, text)
	}
	r, ok := c2.Remote("origin")
	if !ok || r.URL != "/repos/upstream" {
		t.Fatalf("round-trip remote = %+v ok=%v", r, ok)
	}
	if r.Fetch != "+refs/heads/*:refs/remotes/origin/*" {
		t.Fatalf("default fetch not set: %q", r.Fetch)
	}
	if v, _ := c2.Get("user", "", "name"); v != "Alice" {
		t.Fatalf("round-trip user.name = %q", v)
	}
}

func TestRemoveRemote(t *testing.T) {
	c := New()
	c.AddRemote("origin", "/a")
	c.AddRemote("upstream", "/b")
	if !c.RemoveRemote("origin") {
		t.Fatal("RemoveRemote(origin) returned false")
	}
	if _, ok := c.Remote("origin"); ok {
		t.Fatal("origin still present after removal")
	}
	if got := c.Remotes(); len(got) != 1 || got[0].Name != "upstream" {
		t.Fatalf("remotes after removal = %+v", got)
	}
	if c.RemoveRemote("origin") {
		t.Fatal("second RemoveRemote(origin) should return false")
	}
}

func TestLoadMissingFile(t *testing.T) {
	c, err := Load(filepath.Join(t.TempDir(), "does-not-exist"))
	if err != nil {
		t.Fatalf("Load missing file should not error: %v", err)
	}
	if len(c.Remotes()) != 0 {
		t.Fatal("empty config should have no remotes")
	}
}

func TestSaveLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	c := New()
	c.AddRemote("origin", "/repos/x")
	if err := c.Save(path); err != nil {
		t.Fatalf("save: %v", err)
	}
	c2, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if r, ok := c2.Remote("origin"); !ok || r.URL != "/repos/x" {
		t.Fatalf("persisted remote = %+v ok=%v", r, ok)
	}
}
