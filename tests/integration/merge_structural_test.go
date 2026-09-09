package integration

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/thulasiramk-2310/vara/internal/commands"
	"github.com/thulasiramk-2310/vara/pkg/config"
	"github.com/thulasiramk-2310/vara/pkg/recovery"
)

// setupJSONDivergence builds base {"a":1,"b":2} (compact, one line), then a
// feature branch that edits key "b" and main that edits key "a". Because both
// edits land on the same line, the LINE merger conflicts — the exact false
// conflict structural merge is meant to eliminate. Leaves the tree on main,
// having merged feature. Returns the merge output and repo dir.
func setupJSONDivergence(t *testing.T, enableStructural bool) (string, *commands.Context, string) {
	t.Helper()
	ctx, dir := setupRepo(t)
	writeFile(t, dir, "config.json", `{"a":1,"b":2}`+"\n")
	makeCommit(t, ctx, "base")

	if _, err := commands.RunBranch(ctx, "feature"); err != nil {
		t.Fatalf("branch: %v", err)
	}
	if _, err := commands.RunSwitch(ctx, "feature"); err != nil {
		t.Fatalf("switch feature: %v", err)
	}
	writeFile(t, dir, "config.json", `{"a":1,"b":20}`+"\n") // feature edits "b"
	makeCommit(t, ctx, "feature edits b")

	if _, err := commands.RunSwitch(ctx, "main"); err != nil {
		t.Fatalf("switch main: %v", err)
	}
	writeFile(t, dir, "config.json", `{"a":10,"b":2}`+"\n") // main edits "a"
	makeCommit(t, ctx, "main edits a")

	if enableStructural {
		cfg := config.New()
		cfg.Set("merge", "", "driver.json", "structured-json")
		if err := cfg.Save(filepath.Join(ctx.Repository.VaraDir, "config")); err != nil {
			t.Fatalf("write config: %v", err)
		}
	}

	out, err := commands.RunMerge(ctx, "feature")
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	return out, ctx, dir
}

// TestStructuralMergeAutoResolvesJSON: with the driver enabled, the merge that
// the line merger would conflict on instead auto-resolves to the combined object.
func TestStructuralMergeAutoResolvesJSON(t *testing.T) {
	out, ctx, dir := setupJSONDivergence(t, true)

	if !strings.Contains(out, "Auto-merged all conflicts") {
		t.Fatalf("expected a clean auto-merge, got: %q", out)
	}
	if recovery.MergeInProgress(ctx.Repository.VaraDir) {
		t.Fatal("merge should have concluded (no MERGE_HEAD)")
	}
	got := readFile(dir, "config.json")
	if strings.Contains(got, "<<<<<<<") {
		t.Fatalf("no conflict markers expected, got:\n%s", got)
	}
	var v any
	if err := json.Unmarshal([]byte(got), &v); err != nil {
		t.Fatalf("merged config.json is not valid JSON: %v\n%s", err, got)
	}
	want := map[string]any{"a": float64(10), "b": float64(20)}
	if !reflect.DeepEqual(v, want) {
		t.Fatalf("merged JSON = %v, want %v", v, want)
	}
}

// TestStructuralMergeAutoResolvesYAML: same win for YAML. Flow style ({a: 1, b: 2})
// keeps both keys on one line so the line merger conflicts; the structural YAML
// driver auto-resolves to the combined mapping (re-emitted in block style).
func TestStructuralMergeAutoResolvesYAML(t *testing.T) {
	ctx, dir := setupRepo(t)
	writeFile(t, dir, "conf.yaml", "{a: 1, b: 2}\n")
	makeCommit(t, ctx, "base")

	if _, err := commands.RunBranch(ctx, "feature"); err != nil {
		t.Fatalf("branch: %v", err)
	}
	if _, err := commands.RunSwitch(ctx, "feature"); err != nil {
		t.Fatalf("switch: %v", err)
	}
	writeFile(t, dir, "conf.yaml", "{a: 1, b: 20}\n")
	makeCommit(t, ctx, "feature edits b")

	if _, err := commands.RunSwitch(ctx, "main"); err != nil {
		t.Fatalf("switch main: %v", err)
	}
	writeFile(t, dir, "conf.yaml", "{a: 10, b: 2}\n")
	makeCommit(t, ctx, "main edits a")

	cfg := config.New()
	cfg.Set("merge", "", "driver.yaml", "structured-yaml")
	if err := cfg.Save(filepath.Join(ctx.Repository.VaraDir, "config")); err != nil {
		t.Fatalf("write config: %v", err)
	}

	out, err := commands.RunMerge(ctx, "feature")
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	if !strings.Contains(out, "Auto-merged all conflicts") {
		t.Fatalf("expected a clean auto-merge, got: %q", out)
	}
	got := readFile(dir, "conf.yaml")
	if strings.Contains(got, "<<<<<<<") {
		t.Fatalf("no markers expected, got:\n%s", got)
	}
	if !strings.Contains(got, "a: 10") || !strings.Contains(got, "b: 20") {
		t.Fatalf("expected combined a:10 / b:20, got:\n%s", got)
	}
}

// TestStructuralMergeOffStillConflicts: the control — without the driver enabled,
// the same divergence conflicts through the line merger, proving structural merge
// is genuinely what resolved it (and that it is opt-in).
func TestStructuralMergeOffStillConflicts(t *testing.T) {
	out, ctx, dir := setupJSONDivergence(t, false)

	if !strings.Contains(out, "Automatic merge failed") {
		t.Fatalf("without the driver, the line merger should conflict, got: %q", out)
	}
	if !recovery.MergeInProgress(ctx.Repository.VaraDir) {
		t.Fatal("expected an in-progress conflicted merge")
	}
	if !strings.Contains(readFile(dir, "config.json"), "<<<<<<<") {
		t.Fatal("expected conflict markers from the line merge")
	}
}
