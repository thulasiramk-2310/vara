package integration

import (
	"strings"
	"testing"

	"github.com/thulasiramk-2310/vara/internal/commands"
	"github.com/thulasiramk-2310/vara/internal/conflict"
	"github.com/thulasiramk-2310/vara/internal/mergestate"
)

// makeBinaryConflict sets up a binary file changed differently on both branches,
// producing a content conflict whose sides are binary.
func makeBinaryConflict(t *testing.T) (*commands.Context, string) {
	t.Helper()
	ctx, dir := setupRepo(t)

	writeFile(t, dir, "asset.bin", "\x00BASE\x00\x01\x02")
	makeCommit(t, ctx, "base")

	if _, err := commands.RunBranch(ctx, "feature"); err != nil {
		t.Fatalf("branch: %v", err)
	}
	if _, err := commands.RunSwitch(ctx, "feature"); err != nil {
		t.Fatalf("switch: %v", err)
	}
	writeFile(t, dir, "asset.bin", "\x00FEATURE\x00\x03\x04")
	makeCommit(t, ctx, "feature bin")

	if _, err := commands.RunSwitch(ctx, "main"); err != nil {
		t.Fatalf("switch main: %v", err)
	}
	writeFile(t, dir, "asset.bin", "\x00MAIN\x00\x05\x06")
	makeCommit(t, ctx, "main bin")

	if _, err := commands.RunMerge(ctx, "feature"); err != nil {
		t.Fatalf("merge: %v", err)
	}
	if _, ok, _ := mergestate.Read(ctx.Repository.VaraDir); !ok {
		t.Fatal("expected a conflict sidecar")
	}
	return ctx, dir
}

// TestBinaryConflictAutoRefuses: --auto (and --union) must NOT line-merge binary
// content; they leave it unresolved and tell the user to choose a side.
func TestBinaryConflictAutoRefuses(t *testing.T) {
	for _, strat := range []conflict.Strategy{conflict.Auto, conflict.Union} {
		t.Run(string(strat), func(t *testing.T) {
			ctx, _ := makeBinaryConflict(t)

			out, err := commands.RunResolve(ctx, commands.ResolveArgs{Strategy: strat})
			if err != nil {
				t.Fatalf("resolve %s: %v", strat, err)
			}
			if !strings.Contains(out, "need an explicit choice") || !strings.Contains(out, "binary") {
				t.Fatalf("%s should refuse the binary conflict, got: %q", strat, out)
			}
			// Commit must stay refused.
			ctx = reloadCtx(t, ctx)
			if _, err := commands.RunCommit(ctx, "premature"); err == nil {
				t.Fatal("commit must be refused while the binary conflict is unresolved")
			}
		})
	}
}

// TestBinaryConflictTheirsWritesWholeSide: --theirs writes the whole binary side
// verbatim (no markers, no corruption) and resolves.
func TestBinaryConflictTheirsWritesWholeSide(t *testing.T) {
	ctx, dir := makeBinaryConflict(t)

	if _, err := commands.RunResolve(ctx, commands.ResolveArgs{Strategy: conflict.Theirs}); err != nil {
		t.Fatalf("resolve --theirs: %v", err)
	}
	if got := readFile(dir, "asset.bin"); got != "\x00FEATURE\x00\x03\x04" {
		t.Fatalf("binary --theirs must write the whole side verbatim, got %q", got)
	}
	st, _, _ := mergestate.Read(ctx.Repository.VaraDir)
	if e, _ := st.Lookup("asset.bin"); !e.Resolved {
		t.Fatal("binary --theirs should mark the entry resolved")
	}
	ctx = reloadCtx(t, ctx)
	if _, err := commands.RunCommit(ctx, "merge binary"); err != nil {
		t.Fatalf("commit after resolve: %v", err)
	}
}

// TestStatusFlagsBinaryConflict: status labels a binary unmerged path as binary.
func TestStatusFlagsBinaryConflict(t *testing.T) {
	ctx, _ := makeBinaryConflict(t)

	ctx = reloadCtx(t, ctx)
	out, err := commands.RunStatus(ctx)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if !strings.Contains(out, "asset.bin") || !strings.Contains(out, "(binary)") {
		t.Fatalf("status should flag asset.bin as binary, got:\n%s", out)
	}
}
