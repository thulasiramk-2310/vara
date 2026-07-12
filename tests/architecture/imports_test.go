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
