package integration

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/thulasiramk-2310/vara/internal/commands"
	"github.com/thulasiramk-2310/vara/internal/conflict"
	"github.com/thulasiramk-2310/vara/pkg/config"
	"github.com/thulasiramk-2310/vara/pkg/recovery"
	"github.com/thulasiramk-2310/vara/pkg/types"
)

// edit is one file's three versions: the common base, and what each branch made of it.
type edit struct{ base, main, feature string }

// policyRepo builds a repository on `main` with a `feature` branch that both
// changed the given files, ready to merge. Each file that differs on both sides
// from the base in the same place is a genuine conflict.
func policyRepo(t *testing.T, files map[string]edit) (*commands.Context, string) {
	t.Helper()
	ctx, dir := setupRepo(t)
	for name, e := range files {
		writeFile(t, dir, name, e.base)
	}
	makeCommit(t, ctx, "base")
	if _, err := commands.RunBranch(ctx, "feature"); err != nil {
		t.Fatalf("branch: %v", err)
	}
	if _, err := commands.RunSwitch(ctx, "feature"); err != nil {
		t.Fatalf("switch feature: %v", err)
	}
	for name, e := range files {
		writeFile(t, dir, name, e.feature)
	}
	makeCommit(t, ctx, "feature change")
	if _, err := commands.RunSwitch(ctx, "main"); err != nil {
		t.Fatalf("switch main: %v", err)
	}
	for name, e := range files {
		writeFile(t, dir, name, e.main)
	}
	makeCommit(t, ctx, "main change")
	return ctx, dir
}

// setMergeConfig writes [merge] strategy and [merge "rule"] entries into the repo config.
func setMergeConfig(t *testing.T, ctx *commands.Context, def string, rules map[string]string) {
	t.Helper()
	path := filepath.Join(ctx.Repository.VaraDir, "config")
	cfg, err := config.Load(path)
	if err != nil {
		cfg = config.New()
	}
	if def != "" {
		cfg.Set("merge", "", "strategy", def)
	}
	for pat, strat := range rules {
		cfg.Set("merge", "rule", pat, strat)
	}
	if err := cfg.Save(path); err != nil {
		t.Fatalf("save config: %v", err)
	}
}

func lines(a, b, c string) string { return "line1\n" + b + "\nline3\n" + c }

var oneConflict = map[string]edit{
	"shared.txt": {base: "line1\nBASE\nline3\n", main: "line1\nMAIN\nline3\n", feature: "line1\nFEATURE\nline3\n"},
}

func assertMergeCompleted(t *testing.T, ctx *commands.Context, out string) {
	t.Helper()
	if recovery.MergeInProgress(ctx.Repository.VaraDir) {
		t.Fatalf("merge should be complete, but is still in progress:\n%s", out)
	}
	if !strings.Contains(out, "Merge complete") {
		t.Fatalf("output should report the completed merge:\n%s", out)
	}
}

func TestMergeTheirsCompletesInOneStep(t *testing.T) {
	ctx, dir := policyRepo(t, oneConflict)
	out, err := commands.RunMergeWith(ctx, "feature", commands.MergeOptions{Strategy: conflict.Theirs})
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	assertMergeCompleted(t, ctx, out)
	got := readFile(dir, "shared.txt")
	if got != "line1\nFEATURE\nline3\n" || strings.Contains(got, "<<<<<<<") {
		t.Fatalf("--theirs should keep the other branch's side, got %q", got)
	}
	// It must say what it did, naming the file and the side kept.
	if !strings.Contains(out, "shared.txt") || !strings.Contains(out, "theirs") {
		t.Fatalf("the output must name the file and the side kept:\n%s", out)
	}
}

func TestMergeOursKeepsOurSide(t *testing.T) {
	ctx, dir := policyRepo(t, oneConflict)
	out, err := commands.RunMergeWith(ctx, "feature", commands.MergeOptions{Strategy: conflict.Ours})
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	assertMergeCompleted(t, ctx, out)
	if got := readFile(dir, "shared.txt"); got != "line1\nMAIN\nline3\n" {
		t.Fatalf("--ours should keep our side, got %q", got)
	}
}

func TestMergeUnionKeepsBothSides(t *testing.T) {
	ctx, dir := policyRepo(t, oneConflict)
	out, err := commands.RunMergeWith(ctx, "feature", commands.MergeOptions{Strategy: conflict.Union})
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	assertMergeCompleted(t, ctx, out)
	if got := readFile(dir, "shared.txt"); got != "line1\nMAIN\nFEATURE\nline3\n" {
		t.Fatalf("--union should keep both sides, ours first, got %q", got)
	}
}

func TestMergeWithoutAChoiceStillLeavesConflictsForYou(t *testing.T) {
	ctx, dir := policyRepo(t, oneConflict)
	out, err := commands.RunMerge(ctx, "feature")
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	if !strings.Contains(out, "Automatic merge failed") || !recovery.MergeInProgress(ctx.Repository.VaraDir) {
		t.Fatalf("with no choice the behaviour must be unchanged (conflicts left):\n%s", out)
	}
	if !strings.Contains(readFile(dir, "shared.txt"), "<<<<<<<") {
		t.Fatal("markers should be left for the user")
	}
}

func TestMergeConfigDefaultApplies(t *testing.T) {
	ctx, dir := policyRepo(t, oneConflict)
	setMergeConfig(t, ctx, "theirs", nil)
	out, err := commands.RunMerge(ctx, "feature") // no flag at all
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	assertMergeCompleted(t, ctx, out)
	if got := readFile(dir, "shared.txt"); got != "line1\nFEATURE\nline3\n" {
		t.Fatalf("merge.strategy = theirs should apply, got %q", got)
	}
}

func TestMergeRulesChooseFileByFile(t *testing.T) {
	ctx, dir := policyRepo(t, map[string]edit{
		"yarn.lock":      {base: "v=BASE\n", main: "v=MAIN\n", feature: "v=FEATURE\n"},
		"CHANGELOG.md":   {base: "# log\nBASE\n", main: "# log\nMAIN\n", feature: "# log\nFEATURE\n"},
		"src/handler.go": {base: "x := BASE\n", main: "x := MAIN\n", feature: "x := FEATURE\n"},
	})
	setMergeConfig(t, ctx, "", map[string]string{"*.lock": "theirs", "CHANGELOG.md": "union"})

	out, err := commands.RunMerge(ctx, "feature")
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	if got := readFile(dir, "yarn.lock"); got != "v=FEATURE\n" {
		t.Errorf("yarn.lock should take theirs by rule, got %q", got)
	}
	if got := readFile(dir, "CHANGELOG.md"); got != "# log\nMAIN\nFEATURE\n" {
		t.Errorf("CHANGELOG.md should keep both by rule, got %q", got)
	}
	// The file with no rule is still yours to decide, and the merge stays open.
	if got := readFile(dir, "src/handler.go"); !strings.Contains(got, "<<<<<<<") {
		t.Errorf("a file with no rule must keep its conflict markers, got %q", got)
	}
	if !recovery.MergeInProgress(ctx.Repository.VaraDir) {
		t.Fatal("the merge must stay open while one conflict is unresolved")
	}
	if !strings.Contains(out, "src/handler.go") {
		t.Fatalf("the output must list what is still open:\n%s", out)
	}
	if _, err := commands.RunMergeContinue(ctx, ""); err == nil {
		t.Fatal("--continue must refuse while a conflict remains")
	}

	// Finish by hand, exactly as before this feature existed.
	if _, err := commands.RunResolve(ctx, commands.ResolveArgs{Strategy: conflict.Ours, Paths: []string{"src/handler.go"}}); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if out, err := commands.RunMergeContinue(ctx, ""); err != nil || !strings.Contains(out, "Merge complete") {
		t.Fatalf("continue: %v %q", err, out)
	}
	if got := readFile(dir, "src/handler.go"); got != "x := MAIN\n" {
		t.Fatalf("hand-resolved file: %q", got)
	}
}

func TestMergeFlagBeatsRuleAndRuleBeatsDefault(t *testing.T) {
	files := map[string]edit{
		"a.lock": {base: "v=BASE\n", main: "v=MAIN\n", feature: "v=FEATURE\n"},
		"b.txt":  {base: "v=BASE\n", main: "v=MAIN\n", feature: "v=FEATURE\n"},
	}
	// Rule beats default: a.lock is theirs by rule, b.txt falls to the default (ours).
	ctx, dir := policyRepo(t, files)
	setMergeConfig(t, ctx, "ours", map[string]string{"*.lock": "theirs"})
	out, err := commands.RunMerge(ctx, "feature")
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	assertMergeCompleted(t, ctx, out)
	if readFile(dir, "a.lock") != "v=FEATURE\n" || readFile(dir, "b.txt") != "v=MAIN\n" {
		t.Fatalf("rule/default: a.lock=%q b.txt=%q", readFile(dir, "a.lock"), readFile(dir, "b.txt"))
	}

	// Flag beats rule: --ours wins over the *.lock = theirs rule.
	ctx2, dir2 := policyRepo(t, files)
	setMergeConfig(t, ctx2, "", map[string]string{"*.lock": "theirs"})
	if _, err := commands.RunMergeWith(ctx2, "feature", commands.MergeOptions{Strategy: conflict.Ours}); err != nil {
		t.Fatalf("merge: %v", err)
	}
	if readFile(dir2, "a.lock") != "v=MAIN\n" {
		t.Fatalf("--ours should beat the rule, got %q", readFile(dir2, "a.lock"))
	}
}

func TestMergeManualIgnoresConfig(t *testing.T) {
	ctx, dir := policyRepo(t, oneConflict)
	setMergeConfig(t, ctx, "theirs", nil)
	out, err := commands.RunMergeWith(ctx, "feature", commands.MergeOptions{Manual: true})
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	if !strings.Contains(out, "Automatic merge failed") || !strings.Contains(readFile(dir, "shared.txt"), "<<<<<<<") {
		t.Fatalf("--manual must leave the conflict for the user:\n%s", out)
	}
}

func TestMergeBadConfigIsReportedNotObeyed(t *testing.T) {
	ctx, dir := policyRepo(t, oneConflict)
	setMergeConfig(t, ctx, "banana", map[string]string{"*.txt": "auto"})
	out, err := commands.RunMerge(ctx, "feature")
	if err != nil {
		t.Fatalf("a bad setting must not break the merge: %v", err)
	}
	if !strings.Contains(out, "ignored merge.strategy = banana") || !strings.Contains(out, "ignored merge.rule") {
		t.Fatalf("bad settings must be reported:\n%s", out)
	}
	if !strings.Contains(readFile(dir, "shared.txt"), "<<<<<<<") {
		t.Fatal("with only invalid settings the conflict must be left for the user")
	}
}

func TestMergeBinaryConflictFollowsTheChoice(t *testing.T) {
	ctx, dir := policyRepo(t, map[string]edit{
		"asset.bin": {base: "\x00BASE\x00\x01", main: "\x00MAIN\x00\x05", feature: "\x00FEATURE\x00\x03"},
	})
	out, err := commands.RunMergeWith(ctx, "feature", commands.MergeOptions{Strategy: conflict.Theirs})
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	assertMergeCompleted(t, ctx, out)
	if got := readFile(dir, "asset.bin"); got != "\x00FEATURE\x00\x03" {
		t.Fatalf("binary file should take their whole version, got %q", got)
	}
}

func TestMergeBinaryCannotBeUnioned(t *testing.T) {
	ctx, _ := policyRepo(t, map[string]edit{
		"asset.bin": {base: "\x00BASE\x00\x01", main: "\x00MAIN\x00\x05", feature: "\x00FEATURE\x00\x03"},
	})
	out, err := commands.RunMergeWith(ctx, "feature", commands.MergeOptions{Strategy: conflict.Union})
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	// Two binary files cannot be concatenated, so this must not complete silently.
	if !recovery.MergeInProgress(ctx.Repository.VaraDir) {
		t.Fatalf("--union on a binary conflict must leave it for the user:\n%s", out)
	}
}

func TestAutoResolvedMergeCanBeTakenBack(t *testing.T) {
	ctx, dir := policyRepo(t, oneConflict)
	before := readFile(dir, "shared.txt") // our side, before the merge
	out, err := commands.RunMergeWith(ctx, "feature", commands.MergeOptions{Strategy: conflict.Theirs})
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	assertMergeCompleted(t, ctx, out)
	if _, err := commands.RunUndo(ctx, true); err != nil {
		t.Fatalf("undo: %v", err)
	}
	if got := readFile(dir, "shared.txt"); got != before {
		t.Fatalf("undo should bring back our side %q, got %q", before, got)
	}
	_ = types.CommitID{}
}

func TestMergeThatNeedsNoChoiceIsUnaffected(t *testing.T) {
	// Disjoint edits merge cleanly on their own; the options must not interfere.
	ctx, dir := policyRepo(t, map[string]edit{
		"f.txt": {base: "a\nb\nc\nd\ne\nf\ng\n", main: "A\nb\nc\nd\ne\nf\ng\n", feature: "a\nb\nc\nd\ne\nf\nG\n"},
	})
	out, err := commands.RunMergeWith(ctx, "feature", commands.MergeOptions{Strategy: conflict.Theirs})
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	if recovery.MergeInProgress(ctx.Repository.VaraDir) {
		t.Fatalf("a clean merge must complete:\n%s", out)
	}
	if got := readFile(dir, "f.txt"); got != "A\nb\nc\nd\ne\nf\nG\n" {
		t.Fatalf("both edits must survive even with --theirs, got %q", got)
	}
}
