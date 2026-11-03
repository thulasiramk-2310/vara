package commands

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// A server policy folder as the hub leaves it: an admin who made the repository "demo".
func sharePolicy(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("demo.json", `{"version":1,"subjects":{"admin":["read","create-ref","push","force-push","delete-ref","delete-repo","rename-repo","admin"]}}`)
	write("notes.json", `{"version":1,"subjects":{"admin":["read","admin"]}}`)
	write("_server.json", `{"version":1,"subjects":{"admin":["create-repo","list-repos","manage-accounts"]}}`)
	return dir
}

func caps(t *testing.T, dir, file, who string) []string {
	t.Helper()
	p, err := loadPolicyShape(filepath.Join(dir, file))
	if err != nil {
		t.Fatal(err)
	}
	c := append([]string(nil), p.Subjects[who]...)
	sort.Strings(c)
	return c
}

func TestShareWriteGrantsPushAndTheServerLevelListing(t *testing.T) {
	dir := sharePolicy(t)
	msg, err := Share(ShareOptions{PolicyDir: dir, Repo: "demo", User: "bob", Level: ShareWrite})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(msg, "push") || !strings.Contains(msg, "Hub") {
		t.Fatalf("message should say what Bob can do: %q", msg)
	}
	if got := caps(t, dir, "demo.json", "bob"); !reflect.DeepEqual(got, []string{"create-ref", "push", "read"}) {
		t.Fatalf("repo caps = %v", got)
	}
	// the second edit the Hub's repository list needs, made in the same step
	if got := caps(t, dir, "_server.json", "bob"); !reflect.DeepEqual(got, []string{"list-repos"}) {
		t.Fatalf("server caps = %v", got)
	}
	// nobody else changed
	if got := caps(t, dir, "_server.json", "admin"); len(got) != 3 {
		t.Fatalf("admin's server caps were disturbed: %v", got)
	}
}

func TestShareReadGivesOnlyRead(t *testing.T) {
	dir := sharePolicy(t)
	if _, err := Share(ShareOptions{PolicyDir: dir, Repo: "demo", User: "carol", Level: ShareRead}); err != nil {
		t.Fatal(err)
	}
	if got := caps(t, dir, "demo.json", "carol"); !reflect.DeepEqual(got, []string{"read"}) {
		t.Fatalf("caps = %v", got)
	}
}

func TestShareNeverGivesDangerousCapabilities(t *testing.T) {
	dir := sharePolicy(t)
	if _, err := Share(ShareOptions{PolicyDir: dir, Repo: "demo", User: "bob", Level: ShareWrite}); err != nil {
		t.Fatal(err)
	}
	for _, c := range caps(t, dir, "demo.json", "bob") {
		for _, bad := range []string{"force-push", "delete-ref", "delete-repo", "rename-repo", "admin"} {
			if c == bad {
				t.Fatalf("a shared person was given %q", c)
			}
		}
	}
}

func TestShareCanMoveSomeoneBetweenReadAndWrite(t *testing.T) {
	dir := sharePolicy(t)
	Share(ShareOptions{PolicyDir: dir, Repo: "demo", User: "bob", Level: ShareWrite})
	Share(ShareOptions{PolicyDir: dir, Repo: "demo", User: "bob", Level: ShareRead})
	if got := caps(t, dir, "demo.json", "bob"); !reflect.DeepEqual(got, []string{"read"}) {
		t.Fatalf("after downgrade: %v", got)
	}
}

func TestShareRefusesToChangeAnOwner(t *testing.T) {
	dir := sharePolicy(t)
	before, _ := os.ReadFile(filepath.Join(dir, "demo.json"))
	_, err := Share(ShareOptions{PolicyDir: dir, Repo: "demo", User: "admin", Level: ShareRead})
	if err == nil || !strings.Contains(err.Error(), "owner") {
		t.Fatalf("demoting the owner must be refused, got %v", err)
	}
	_, err = Share(ShareOptions{PolicyDir: dir, Repo: "demo", User: "admin", Level: ShareNone})
	if err == nil {
		t.Fatal("removing the owner must be refused")
	}
	after, _ := os.ReadFile(filepath.Join(dir, "demo.json"))
	if string(before) != string(after) {
		t.Fatal("a refused change must leave the policy untouched")
	}
}

func TestShareNoneRemovesTheRepoGrantAndKeepsListingWhileAnotherRepoIsShared(t *testing.T) {
	dir := sharePolicy(t)
	Share(ShareOptions{PolicyDir: dir, Repo: "demo", User: "bob", Level: ShareWrite})
	Share(ShareOptions{PolicyDir: dir, Repo: "notes", User: "bob", Level: ShareRead})
	if _, err := Share(ShareOptions{PolicyDir: dir, Repo: "demo", User: "bob", Level: ShareNone}); err != nil {
		t.Fatal(err)
	}
	if got := caps(t, dir, "demo.json", "bob"); len(got) != 0 {
		t.Fatalf("bob still has access to demo: %v", got)
	}
	if got := caps(t, dir, "_server.json", "bob"); !reflect.DeepEqual(got, []string{"list-repos"}) {
		t.Fatalf("bob still has notes, so he keeps the listing: %v", got)
	}
	// removing the last share takes the listing away too
	Share(ShareOptions{PolicyDir: dir, Repo: "notes", User: "bob", Level: ShareNone})
	if got := caps(t, dir, "_server.json", "bob"); len(got) != 0 {
		t.Fatalf("nothing is shared with bob any more, but he can still list: %v", got)
	}
}

func TestShareNoneKeepsAnAdminsListing(t *testing.T) {
	dir := sharePolicy(t)
	// an account that can create repositories keeps list-repos even after a share is removed
	if err := os.WriteFile(filepath.Join(dir, "_server.json"),
		[]byte(`{"version":1,"subjects":{"admin":["create-repo","list-repos","manage-accounts"],"dana":["create-repo","list-repos"]}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	Share(ShareOptions{PolicyDir: dir, Repo: "demo", User: "dana", Level: ShareRead})
	Share(ShareOptions{PolicyDir: dir, Repo: "demo", User: "dana", Level: ShareNone})
	if got := caps(t, dir, "_server.json", "dana"); !reflect.DeepEqual(got, []string{"create-repo", "list-repos"}) {
		t.Fatalf("dana's own server permissions were taken away: %v", got)
	}
}

func TestShareRejectsUnsafeNamesAndMissingRepos(t *testing.T) {
	dir := sharePolicy(t)
	for _, repo := range []string{"../etc/passwd", "_server", "", "a/b", ".hidden", "demo.json/../x"} {
		if _, err := Share(ShareOptions{PolicyDir: dir, Repo: repo, User: "bob", Level: ShareRead}); err == nil {
			t.Fatalf("repo name %q must be refused", repo)
		}
	}
	for _, user := range []string{"", "bo b", "../x", "a/b"} {
		if _, err := Share(ShareOptions{PolicyDir: dir, Repo: "demo", User: user, Level: ShareRead}); err == nil {
			t.Fatalf("user name %q must be refused", user)
		}
	}
	if _, err := Share(ShareOptions{PolicyDir: dir, Repo: "ghost", User: "bob", Level: ShareRead}); err == nil || !strings.Contains(err.Error(), "ghost") {
		t.Fatalf("a missing repository must be named in the error: %v", err)
	}
	if _, err := Share(ShareOptions{PolicyDir: "", Repo: "demo", User: "bob", Level: ShareRead}); err == nil {
		t.Fatal("no policy folder must be refused")
	}
	// nothing was created by the refused attempts
	if _, err := os.Stat(filepath.Join(dir, "ghost.json")); err == nil {
		t.Fatal("a refused share created a policy file")
	}
}

func TestShareChecksTheAccountExistsWhenAskedTo(t *testing.T) {
	dir := sharePolicy(t)
	accounts := t.TempDir()
	if err := os.MkdirAll(filepath.Join(accounts, "accounts"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := Share(ShareOptions{PolicyDir: dir, AccountsDir: accounts, Repo: "demo", User: "bob", Level: ShareRead})
	if err == nil || !strings.Contains(err.Error(), "no account named") {
		t.Fatalf("an unknown account must be refused with a hint: %v", err)
	}
	if err := os.WriteFile(filepath.Join(accounts, "accounts", "bob.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Share(ShareOptions{PolicyDir: dir, AccountsDir: accounts, Repo: "demo", User: "bob", Level: ShareRead}); err != nil {
		t.Fatalf("an existing account must be accepted: %v", err)
	}
}

func TestShareRefusesAMalformedPolicyInsteadOfOverwritingIt(t *testing.T) {
	dir := sharePolicy(t)
	bad := filepath.Join(dir, "demo.json")
	if err := os.WriteFile(bad, []byte("{ not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Share(ShareOptions{PolicyDir: dir, Repo: "demo", User: "bob", Level: ShareRead}); err == nil {
		t.Fatal("a malformed policy must stop the change")
	}
	if data, _ := os.ReadFile(bad); string(data) != "{ not json" {
		t.Fatal("a malformed policy was overwritten")
	}
}

func TestShareListNamesEveryoneInPlainWords(t *testing.T) {
	dir := sharePolicy(t)
	Share(ShareOptions{PolicyDir: dir, Repo: "demo", User: "bob", Level: ShareWrite})
	Share(ShareOptions{PolicyDir: dir, Repo: "demo", User: "carol", Level: ShareRead})
	out, err := ShareList(dir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"admin", "owner", "bob", "write", "carol", "read"} {
		if !strings.Contains(out, want) {
			t.Fatalf("list is missing %q:\n%s", want, out)
		}
	}
	all, err := ShareList(dir, "")
	if err != nil || !strings.Contains(all, "notes") || !strings.Contains(all, "demo") {
		t.Fatalf("listing every repository: %v\n%s", err, all)
	}
	if strings.Contains(all, "_server") {
		t.Fatal("the server policy is not a repository")
	}
}

func TestRunShareParsesTheCommandLine(t *testing.T) {
	dir := sharePolicy(t)
	if err := RunShare([]string{"--policy", dir, "demo", "bob", "--write"}); err != nil {
		t.Fatal(err)
	}
	if got := caps(t, dir, "demo.json", "bob"); len(got) != 3 {
		t.Fatalf("--write gave %v", got)
	}
	// no level means read
	if err := RunShare([]string{"--policy", dir, "demo", "carol"}); err != nil {
		t.Fatal(err)
	}
	if got := caps(t, dir, "demo.json", "carol"); !reflect.DeepEqual(got, []string{"read"}) {
		t.Fatalf("default level gave %v", got)
	}
	for _, bad := range [][]string{
		{"--policy", dir, "demo", "bob", "--read", "--write"},
		{"--policy", dir, "demo", "bob", "extra"},
		{"--policy", dir, "--nope"},
		{"--policy", dir, "demo", "--write"}, // a level without a person
		{"--policy"},
	} {
		if err := RunShare(bad); err == nil {
			t.Fatalf("%v must be refused", bad)
		}
	}
}
