package repomanager_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/thulasiramk-2310/vara/internal/repomanager"
)

// testDirs returns three fresh directories: the repository root, the policy
// root, and the metadata dir (the last is created by New).
func testDirs(t *testing.T) (reposRoot, policyRoot, metaDir string) {
	t.Helper()
	root := t.TempDir()
	reposRoot = filepath.Join(root, "repos")
	policyRoot = filepath.Join(root, "policy")
	metaDir = filepath.Join(root, "meta")
	for _, d := range []string{reposRoot, policyRoot} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return
}

func newManager(t *testing.T) (*repomanager.Manager, string, string, string) {
	t.Helper()
	reposRoot, policyRoot, metaDir := testDirs(t)
	m, err := repomanager.New(reposRoot, policyRoot, metaDir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return m, reposRoot, policyRoot, metaDir
}

func TestLifecycleRoundTrip(t *testing.T) {
	m, reposRoot, policyRoot, metaDir := newManager(t)

	md, err := m.Create("website", "alice", "", "my site")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !strings.HasPrefix(md.ID, "repo_") {
		t.Fatalf("id = %q, want repo_ prefix", md.ID)
	}
	if md.State != repomanager.StateActive || md.Owner != "alice" || md.Visibility != repomanager.VisibilityPrivate {
		t.Fatalf("descriptor wrong: %+v", md)
	}
	// All three artifacts exist.
	if _, err := os.Stat(filepath.Join(reposRoot, "website", ".vara")); err != nil {
		t.Fatalf("content not created: %v", err)
	}
	if _, err := os.Stat(filepath.Join(policyRoot, "website.json")); err != nil {
		t.Fatalf("policy not seeded: %v", err)
	}
	if _, err := os.Stat(filepath.Join(metaDir, md.ID+".json")); err != nil {
		t.Fatalf("metadata not written: %v", err)
	}

	// Get + List.
	if got, err := m.Get("website"); err != nil || got.ID != md.ID {
		t.Fatalf("Get: %v (%+v)", err, got)
	}
	if list, _ := m.List(); len(list) != 1 || list[0].Name != "website" {
		t.Fatalf("List = %+v, want [website]", list)
	}
	if !m.Servable("website") {
		t.Fatalf("Active repo should be servable")
	}

	// Delete removes all three artifacts.
	if err := m.Delete("website"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := m.Get("website"); !errors.Is(err, repomanager.ErrNotFound) {
		t.Fatalf("Get after delete = %v, want ErrNotFound", err)
	}
	if m.Servable("website") {
		t.Fatalf("deleted repo must not be servable")
	}
	for _, p := range []string{
		filepath.Join(reposRoot, "website"),
		filepath.Join(policyRoot, "website.json"),
		filepath.Join(metaDir, md.ID+".json"),
	} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Fatalf("artifact %s survived delete (err=%v)", p, err)
		}
	}
}

func TestCreateConflictAndDeleteAbsent(t *testing.T) {
	m, _, _, _ := newManager(t)
	if _, err := m.Create("dup", "alice", "", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Create("dup", "bob", "", ""); !errors.Is(err, repomanager.ErrExists) {
		t.Fatalf("re-create = %v, want ErrExists", err)
	}
	if err := m.Delete("ghost"); !errors.Is(err, repomanager.ErrNotFound) {
		t.Fatalf("delete absent = %v, want ErrNotFound", err)
	}
}

func TestRenamePreservesID(t *testing.T) {
	m, reposRoot, policyRoot, _ := newManager(t)
	created, err := m.Create("old", "alice", "", "")
	if err != nil {
		t.Fatal(err)
	}
	renamed, err := m.Rename("old", "new")
	if err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if renamed.ID != created.ID {
		t.Fatalf("id changed across rename: %s -> %s", created.ID, renamed.ID)
	}
	if renamed.Name != "new" {
		t.Fatalf("name = %q, want new", renamed.Name)
	}
	// Name-addressed artifacts moved.
	if _, err := os.Stat(filepath.Join(reposRoot, "new", ".vara")); err != nil {
		t.Fatalf("content not moved: %v", err)
	}
	if _, err := os.Stat(filepath.Join(policyRoot, "new.json")); err != nil {
		t.Fatalf("policy not moved: %v", err)
	}
	if _, err := os.Stat(filepath.Join(reposRoot, "old")); !os.IsNotExist(err) {
		t.Fatalf("old content survived rename")
	}
	// Old name is free, new name resolves.
	if _, err := m.Get("old"); !errors.Is(err, repomanager.ErrNotFound) {
		t.Fatalf("old name still resolves: %v", err)
	}
	if got, err := m.Get("new"); err != nil || got.ID != created.ID {
		t.Fatalf("new name Get: %v", err)
	}
	// Rename onto a taken name conflicts.
	if _, err := m.Create("other", "alice", "", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Rename("new", "other"); !errors.Is(err, repomanager.ErrExists) {
		t.Fatalf("rename-onto-taken = %v, want ErrExists", err)
	}
}

func TestNameValidation(t *testing.T) {
	m, _, _, _ := newManager(t)
	for _, bad := range []string{"", ".", "..", "_vara", "_server", ".git", ".vara", "a/b", `a\b`, "a..b", "sp ace", "emoji😀"} {
		if _, err := m.Create(bad, "alice", "", ""); !errors.Is(err, repomanager.ErrInvalidName) {
			t.Fatalf("Create(%q) = %v, want ErrInvalidName", bad, err)
		}
	}
	for _, good := range []string{"a", "my-repo", "my_repo", "v1.0", "A-Z_09"} {
		if !repomanager.ValidName(good) {
			t.Fatalf("ValidName(%q) = false, want true", good)
		}
	}
}

// TestCreateAtomicRollback proves M6: if the policy seed fails, the whole create
// rolls back — no content, no metadata, no live repository.
func TestCreateAtomicRollback(t *testing.T) {
	reposRoot, _, metaDir := testDirs(t)
	// Point the policy root at a path whose parent does not exist, so seeding the
	// policy file fails at the write step.
	badPolicy := filepath.Join(t.TempDir(), "does", "not", "exist")
	m, err := repomanager.New(reposRoot, badPolicy, metaDir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Create("doomed", "alice", "", ""); err == nil {
		t.Fatal("Create should fail when policy seed fails")
	}
	// Nothing survives.
	if _, err := m.Get("doomed"); !errors.Is(err, repomanager.ErrNotFound) {
		t.Fatalf("Get after failed create = %v, want ErrNotFound", err)
	}
	if list, _ := m.List(); len(list) != 0 {
		t.Fatalf("List after failed create = %+v, want empty", list)
	}
	if _, err := os.Stat(filepath.Join(reposRoot, "doomed")); !os.IsNotExist(err) {
		t.Fatalf("content survived a rolled-back create")
	}
	entries, _ := os.ReadDir(metaDir)
	if len(entries) != 0 {
		t.Fatalf("metadata survived a rolled-back create: %d files", len(entries))
	}
}

// TestTombstoneReclaim proves a crashed create (a Creating tombstone left on
// disk) is not live, is not served, and is reclaimed by a fresh create.
func TestTombstoneReclaim(t *testing.T) {
	m, _, _, metaDir := newManager(t)

	// Simulate a crash mid-create: a Creating metadata record with no content.
	tomb := repomanager.Metadata{
		ID: "repo_deadbeef", Name: "half", Owner: "alice",
		Visibility: repomanager.VisibilityPrivate, State: repomanager.StateCreating,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	data, _ := json.Marshal(tomb)
	if err := os.WriteFile(filepath.Join(metaDir, tomb.ID+".json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	// A tombstone is neither served nor listed.
	if m.Servable("half") {
		t.Fatalf("Creating tombstone must not be servable")
	}
	if list, _ := m.List(); len(list) != 0 {
		t.Fatalf("tombstone appeared in List: %+v", list)
	}

	// A fresh create of the same name reclaims it and succeeds with a NEW id.
	md, err := m.Create("half", "bob", "", "")
	if err != nil {
		t.Fatalf("reclaim create: %v", err)
	}
	if md.ID == tomb.ID {
		t.Fatalf("reclaim reused the tombstone id")
	}
	if md.State != repomanager.StateActive {
		t.Fatalf("reclaimed repo state = %s, want active", md.State)
	}
	if _, err := os.Stat(filepath.Join(metaDir, tomb.ID+".json")); !os.IsNotExist(err) {
		t.Fatalf("old tombstone metadata not removed")
	}
}

func TestListStableOrder(t *testing.T) {
	m, _, _, _ := newManager(t)
	for _, n := range []string{"charlie", "alice", "bravo"} {
		if _, err := m.Create(n, "o", "", ""); err != nil {
			t.Fatal(err)
		}
	}
	want := []string{"alice", "bravo", "charlie"}
	for iter := 0; iter < 3; iter++ {
		list, _ := m.List()
		if len(list) != len(want) {
			t.Fatalf("len = %d, want %d", len(list), len(want))
		}
		for i, md := range list {
			if md.Name != want[i] {
				t.Fatalf("order[%d] = %q, want %q", i, md.Name, want[i])
			}
		}
	}
}

// TestArtifactsNeverMix proves M9: the policy file holds only capabilities, and
// the metadata file holds only descriptive fields — never the reverse.
func TestArtifactsNeverMix(t *testing.T) {
	m, _, policyRoot, metaDir := newManager(t)
	md, err := m.Create("sep", "alice", repomanager.VisibilityPublic, "desc")
	if err != nil {
		t.Fatal(err)
	}

	policy, err := os.ReadFile(filepath.Join(policyRoot, "sep.json"))
	if err != nil {
		t.Fatal(err)
	}
	ps := string(policy)
	// Policy grants the owner capabilities...
	for _, cap := range []string{"read", "push", "delete-repo", "rename-repo", "admin"} {
		if !strings.Contains(ps, cap) {
			t.Fatalf("policy missing owner capability %q: %s", cap, ps)
		}
	}
	// ...and carries no descriptive metadata fields.
	for _, field := range []string{"visibility", "created_at", "\"state\"", "description"} {
		if strings.Contains(ps, field) {
			t.Fatalf("policy leaked metadata field %q: %s", field, ps)
		}
	}

	meta, err := os.ReadFile(filepath.Join(metaDir, md.ID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	ms := string(meta)
	// Metadata is descriptive and carries no capability grant.
	for _, field := range []string{"\"id\"", "visibility", "created_at"} {
		if !strings.Contains(ms, field) {
			t.Fatalf("metadata missing field %q: %s", field, ms)
		}
	}
	for _, cap := range []string{"create-ref", "force-push", "delete-repo", "\"admin\""} {
		if strings.Contains(ms, cap) {
			t.Fatalf("metadata leaked capability %q: %s", cap, ms)
		}
	}
}
