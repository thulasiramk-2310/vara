// Package architecture holds mechanical checks that the layering rules the RFCs
// declare are actually enforced by the code, not merely documented.
package architecture

import (
	"os/exec"
	"strings"
	"testing"
)

const module = "github.com/thulasiramk-2310/vara"

// enginePackages are the engine and transport layers. Per RFC-0017 C1 and
// RFC-0018 A1, NONE of them may depend on the binding-layer identity/authz
// packages: authentication and authorization live strictly above the transport.
var enginePackages = []string{
	"pkg/object", "pkg/index", "pkg/graph", "pkg/refs", "pkg/hash",
	"pkg/compression", "pkg/transfer", "pkg/tree", "pkg/snapshot",
	"pkg/recovery", "pkg/graphindex", "pkg/verify", "pkg/gc",
	"internal/repository", "internal/transaction", "internal/locking",
	"internal/merge", "internal/undo", "internal/transport",
}

// forbidden are the binding-layer packages the engine must never import. The
// repository manager (RFC-0019) is a binding concern too (M1/M12): no engine or
// transport package may depend on it.
var forbidden = []string{
	module + "/internal/identity",
	module + "/internal/authz",
	module + "/internal/repomanager",
	module + "/internal/hub",
}

// TestEngineDoesNotImportBindingLayers verifies the C1/A1/M1 invariant
// mechanically: the transitive dependency set of every engine/transport package
// excludes the identity, authorization, and repository-management packages. If
// this fails, an engine package has started depending on a binding concern — the
// abstraction has leaked downward.
func TestEngineDoesNotImportBindingLayers(t *testing.T) {
	for _, pkg := range enginePackages {
		full := module + "/" + pkg
		out, err := exec.Command("go", "list", "-deps", full).CombinedOutput()
		if err != nil {
			t.Fatalf("go list -deps %s: %v\n%s", full, err, out)
		}
		deps := string(out)
		for _, bad := range forbidden {
			if strings.Contains(deps, bad) {
				t.Errorf("%s transitively imports %s — violates RFC-0017 C1 / RFC-0018 A1 / RFC-0019 M1", full, bad)
			}
		}
	}
}

// TestRepoManagerHasNoUpwardImports verifies RFC-0019 M12: the repository
// manager depends only downward. It must never import the server or the command
// layer that drive it — those sit above it in the hierarchy.
func TestRepoManagerHasNoUpwardImports(t *testing.T) {
	full := module + "/internal/repomanager"
	out, err := exec.Command("go", "list", "-deps", full).CombinedOutput()
	if err != nil {
		t.Fatalf("go list -deps %s: %v\n%s", full, err, out)
	}
	deps := string(out)
	for _, up := range []string{module + "/internal/server", module + "/internal/commands"} {
		if strings.Contains(deps, up) {
			t.Errorf("internal/repomanager transitively imports %s — violates RFC-0019 M12 (no upward imports)", up)
		}
	}
}

// TestHubProjectsEngineOnly verifies RFC-0021 H2/H8: the Hub projection layer
// reads the engine and nothing else. It must import no binding-layer package
// (server, commands, authz, identity, repomanager, protocol) — if it did, it
// would be assembling representations rather than purely projecting engine state.
func TestHubProjectsEngineOnly(t *testing.T) {
	full := module + "/internal/hub"
	out, err := exec.Command("go", "list", "-deps", full).CombinedOutput()
	if err != nil {
		t.Fatalf("go list -deps %s: %v\n%s", full, err, out)
	}
	deps := string(out)
	for _, bad := range []string{
		module + "/internal/server", module + "/internal/commands",
		module + "/internal/authz", module + "/internal/identity",
		module + "/internal/repomanager", module + "/internal/protocol",
	} {
		if strings.Contains(deps, bad) {
			t.Errorf("internal/hub imports %s — violates RFC-0021 H2/H8 (project the engine, don't reinterpret)", bad)
		}
	}
}

// TestDatastoreOwnershipSeparation verifies RFC-0020 S13: each subsystem owns
// exactly one datastore and never reaches into another's. authz owns policy,
// repomanager owns metadata, identity owns the credential store — so neither
// authz nor repomanager may import identity (the store-owning package), and
// identity may import neither of them.
func TestDatastoreOwnershipSeparation(t *testing.T) {
	checks := map[string][]string{
		module + "/internal/authz":       {module + "/internal/identity", module + "/internal/repomanager"},
		module + "/internal/repomanager": {module + "/internal/identity"},
		module + "/internal/identity":    {module + "/internal/authz", module + "/internal/repomanager"},
	}
	for pkg, forbid := range checks {
		out, err := exec.Command("go", "list", "-deps", pkg).CombinedOutput()
		if err != nil {
			t.Fatalf("go list -deps %s: %v\n%s", pkg, err, out)
		}
		deps := string(out)
		for _, bad := range forbid {
			if strings.Contains(deps, bad) {
				t.Errorf("%s imports %s — violates RFC-0020 S13 (one datastore per subsystem)", pkg, bad)
			}
		}
	}
}
