package integration

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/thulasiramk-2310/vara/internal/commands"
	"github.com/thulasiramk-2310/vara/pkg/recovery"
)

// scripted is a Session whose "person" types the given answers, one per line.
func scripted(answers string, edits ...string) (*commands.Session, *bytes.Buffer) {
	out := &bytes.Buffer{}
	n := 0
	return &commands.Session{
		In:  strings.NewReader(answers),
		Out: out,
		Edit: func(initial string) (string, error) {
			if n >= len(edits) {
				return initial, nil // saved unchanged: still has markers
			}
			n++
			return edits[n-1], nil
		},
	}, out
}

const tenLines = "l1\nl2\nl3\nl4\nl5\nl6\nl7\nl8\nl9\nl10\n"

// twoHunks conflicts on lines 2 and 9, far enough apart to be two hunks.
func twoHunks(t *testing.T) (*commands.Context, string) {
	t.Helper()
	ctx, dir := policyRepo(t, map[string]edit{
		"f.txt": {
			base:    tenLines,
			main:    strings.Replace(strings.Replace(tenLines, "l2", "MAIN2", 1), "l9", "MAIN9", 1),
			feature: strings.Replace(strings.Replace(tenLines, "l2", "FEAT2", 1), "l9", "FEAT9", 1),
		},
	})
	if _, err := commands.RunMerge(ctx, "feature"); err != nil {
		t.Fatalf("merge: %v", err)
	}
	if !recovery.MergeInProgress(ctx.Repository.VaraDir) {
		t.Fatal("expected a conflicted merge")
	}
	return ctx, dir
}

func TestInteractiveChoosesPerHunkAndCompletes(t *testing.T) {
	ctx, dir := twoHunks(t)
	sess, out := scripted("o\nt\n")
	res, err := commands.RunResolveInteractive(ctx, commands.ResolveArgs{}, sess)
	if err != nil {
		t.Fatalf("resolve -i: %v", err)
	}
	want := "l1\nMAIN2\nl3\nl4\nl5\nl6\nl7\nl8\nFEAT9\nl10\n"
	if got := readFile(dir, "f.txt"); got != want {
		t.Fatalf("one hunk each way:\n got %q\nwant %q", got, want)
	}
	if !strings.Contains(res, "resolved  f.txt (2 hunks, chosen by you)") {
		t.Fatalf("summary: %q", res)
	}
	// The conflict was shown with both sides and the base.
	for _, s := range []string{"conflict 1 of 2", "MAIN2", "FEAT2", "base (where both started)", "l2"} {
		if !strings.Contains(out.String(), s) {
			t.Errorf("the prompt should show %q:\n%s", s, out.String())
		}
	}
	if o, err := commands.RunMergeContinue(ctx, ""); err != nil || !strings.Contains(o, "Merge complete") {
		t.Fatalf("continue after interactive: %v %q", err, o)
	}
}

func TestInteractiveBothAndTheirsFirst(t *testing.T) {
	ctx, dir := twoHunks(t)
	sess, _ := scripted("b\nr\n")
	if _, err := commands.RunResolveInteractive(ctx, commands.ResolveArgs{}, sess); err != nil {
		t.Fatal(err)
	}
	want := "l1\nMAIN2\nFEAT2\nl3\nl4\nl5\nl6\nl7\nl8\nFEAT9\nMAIN9\nl10\n"
	if got := readFile(dir, "f.txt"); got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestInteractiveSkipLeavesThatConflictAndBlocksCompletion(t *testing.T) {
	ctx, dir := twoHunks(t)
	sess, _ := scripted("s\nt\n")
	res, err := commands.RunResolveInteractive(ctx, commands.ResolveArgs{}, sess)
	if err != nil {
		t.Fatal(err)
	}
	got := readFile(dir, "f.txt")
	if !strings.Contains(got, "<<<<<<<") || !strings.Contains(got, "MAIN2") || !strings.Contains(got, "FEAT2") {
		t.Fatalf("the skipped hunk must keep its markers: %q", got)
	}
	if !strings.Contains(got, "FEAT9") || strings.Contains(got, "MAIN9") {
		t.Fatalf("the decided hunk must be settled: %q", got)
	}
	if !strings.Contains(res, "partial   f.txt (1 of 2 decided)") || !strings.Contains(res, "Still to resolve") {
		t.Fatalf("summary: %q", res)
	}
	if _, err := commands.RunMergeContinue(ctx, ""); err == nil {
		t.Fatal("--continue must refuse while a conflict is skipped")
	}
}

func TestInteractiveQuitKeepsProgressAndAnotherRunCarriesOn(t *testing.T) {
	ctx, dir := twoHunks(t)
	sess, _ := scripted("o\nq\n")
	res, err := commands.RunResolveInteractive(ctx, commands.ResolveArgs{}, sess)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res, "partial   f.txt (1 of 2 decided)") {
		t.Fatalf("summary after quit: %q", res)
	}
	first := readFile(dir, "f.txt")
	if !strings.Contains(first, "MAIN2") || strings.Contains(first, "FEAT2") || !strings.Contains(first, "<<<<<<<") {
		t.Fatalf("quitting must keep the first decision and leave the rest: %q", first)
	}
	// A second session picks up the file, and only the remaining conflict is asked.
	sess2, out2 := scripted("t\n")
	if _, err := commands.RunResolveInteractive(ctx, commands.ResolveArgs{}, sess2); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out2.String(), "conflict 2 of 2") || !strings.Contains(out2.String(), "conflict 1 of 1") {
		t.Fatalf("the second run should ask only about what is left:\n%s", out2.String())
	}
	if got := readFile(dir, "f.txt"); got != "l1\nMAIN2\nl3\nl4\nl5\nl6\nl7\nl8\nFEAT9\nl10\n" {
		t.Fatalf("after both sessions: %q", got)
	}
}

func TestInteractiveEndOfInputIsQuittingNotAnAnswer(t *testing.T) {
	ctx, dir := twoHunks(t)
	before := readFile(dir, "f.txt")
	sess, _ := scripted("") // the terminal closes immediately
	if _, err := commands.RunResolveInteractive(ctx, commands.ResolveArgs{}, sess); err != nil {
		t.Fatal(err)
	}
	if readFile(dir, "f.txt") != before {
		t.Fatal("no input must change nothing")
	}
	if !recovery.MergeInProgress(ctx.Repository.VaraDir) {
		t.Fatal("the merge must stay open")
	}
}

func TestInteractiveBadAnswersAreExplainedThenAccepted(t *testing.T) {
	ctx, dir := twoHunks(t)
	sess, out := scripted("x\n7\nmaybe\nOURS\no\nt\n")
	if _, err := commands.RunResolveInteractive(ctx, commands.ResolveArgs{}, sess); err != nil {
		t.Fatal(err)
	}
	if strings.Count(out.String(), "Please answer one of") != 4 {
		t.Fatalf("each bad answer should be explained:\n%s", out.String())
	}
	// "OURS" is not one of the accepted words; "o" then "t" decided the two hunks.
	if got := readFile(dir, "f.txt"); !strings.Contains(got, "MAIN2") || !strings.Contains(got, "FEAT9") {
		t.Fatalf("got %q", got)
	}
}

func TestInteractiveGarbageDoesNotLoopForever(t *testing.T) {
	ctx, dir := twoHunks(t)
	before := readFile(dir, "f.txt")
	sess, _ := scripted(strings.Repeat("zzz\n", 50))
	if _, err := commands.RunResolveInteractive(ctx, commands.ResolveArgs{}, sess); err != nil {
		t.Fatal(err)
	}
	if readFile(dir, "f.txt") != before {
		t.Fatal("garbage input must not decide anything")
	}
}

func TestInteractiveAllRemainingShortcut(t *testing.T) {
	ctx, dir := twoHunks(t)
	sess, out := scripted("T\n") // one keypress settles both hunks
	if _, err := commands.RunResolveInteractive(ctx, commands.ResolveArgs{}, sess); err != nil {
		t.Fatal(err)
	}
	if got := readFile(dir, "f.txt"); got != strings.Replace(strings.Replace(tenLines, "l2", "FEAT2", 1), "l9", "FEAT9", 1) {
		t.Fatalf("T should keep theirs everywhere: %q", got)
	}
	if strings.Contains(out.String(), "conflict 2 of 2") {
		t.Fatalf("the second conflict should not be asked about:\n%s", out.String())
	}
}

func TestInteractiveEditUsesYourText(t *testing.T) {
	ctx, dir := twoHunks(t)
	// First edit still has markers (refused and asked again), the second is clean.
	sess, out := scripted("e\ne\nt\n", "<<<<<<< x\nstill\n=======\nbroken\n>>>>>>> y\n", "MERGED2\n")
	if _, err := commands.RunResolveInteractive(ctx, commands.ResolveArgs{}, sess); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "still contains conflict markers") {
		t.Fatalf("an edit that keeps markers must be refused:\n%s", out.String())
	}
	if got := readFile(dir, "f.txt"); got != "l1\nMERGED2\nl3\nl4\nl5\nl6\nl7\nl8\nFEAT9\nl10\n" {
		t.Fatalf("got %q", got)
	}
}

func TestInteractiveBinaryAndModifyDelete(t *testing.T) {
	// Binary: pick a whole side.
	ctx, dir := policyRepo(t, map[string]edit{
		"asset.bin": {base: "\x00BASE\x00\x01", main: "\x00MAIN\x00\x05", feature: "\x00FEATURE\x00\x03"},
	})
	if _, err := commands.RunMerge(ctx, "feature"); err != nil {
		t.Fatal(err)
	}
	sess, _ := scripted("t\n")
	if _, err := commands.RunResolveInteractive(ctx, commands.ResolveArgs{}, sess); err != nil {
		t.Fatal(err)
	}
	if got := readFile(dir, "asset.bin"); got != "\x00FEATURE\x00\x03" {
		t.Fatalf("binary: %q", got)
	}

	// Modify/delete: ours modified, theirs deleted; take theirs = delete.
	ctx2, dir2 := makeModifyDeleteConflict(t)
	sess2, out2 := scripted("t\n")
	if _, err := commands.RunResolveInteractive(ctx2, commands.ResolveArgs{}, sess2); err != nil {
		t.Fatal(err)
	}
	if fileExists(dir2, "doc.txt") {
		t.Fatal("taking their side should delete the file")
	}
	if !strings.Contains(out2.String(), "modified by us, deleted by them") {
		t.Fatalf("the prompt should explain the conflict:\n%s", out2.String())
	}
}

func TestInteractiveOnlyTouchesTheFilesYouNameAndNothingWhenClean(t *testing.T) {
	ctx, dir := policyRepo(t, map[string]edit{
		"a.txt": {base: "x\n", main: "A-MAIN\n", feature: "A-FEAT\n"},
		"b.txt": {base: "x\n", main: "B-MAIN\n", feature: "B-FEAT\n"},
	})
	if _, err := commands.RunMerge(ctx, "feature"); err != nil {
		t.Fatal(err)
	}
	bBefore := readFile(dir, "b.txt")
	sess, _ := scripted("o\n")
	if _, err := commands.RunResolveInteractive(ctx, commands.ResolveArgs{Paths: []string{"a.txt"}}, sess); err != nil {
		t.Fatal(err)
	}
	if readFile(dir, "a.txt") != "A-MAIN\n" || readFile(dir, "b.txt") != bBefore {
		t.Fatal("only a.txt may change")
	}
	// Nothing left to ask about afterwards for a.txt.
	res, err := commands.RunResolveInteractive(ctx, commands.ResolveArgs{Paths: []string{"a.txt"}}, io2())
	if err != nil || !strings.Contains(strings.ToLower(res), "no conflict") {
		t.Fatalf("a settled file should report nothing to do: %v %q", err, res)
	}
}

func io2() *commands.Session { s, _ := scripted(""); return s }

func TestMergeInteractiveAfterRulesThenCompletes(t *testing.T) {
	ctx, dir := policyRepo(t, map[string]edit{
		"yarn.lock": {base: "v=BASE\n", main: "v=MAIN\n", feature: "v=FEATURE\n"},
		"a.go":      {base: "x := BASE\n", main: "x := MAIN\n", feature: "x := FEATURE\n"},
	})
	setMergeConfig(t, ctx, "", map[string]string{"*.lock": "theirs"})
	sess, out := scripted("t\ny\n") // theirs for a.go, then yes to completing
	res, err := commands.RunMergeWith(ctx, "feature", commands.MergeOptions{Interactive: sess})
	if err != nil {
		t.Fatalf("merge -i: %v", err)
	}
	if !strings.Contains(res, "Merge complete") || recovery.MergeInProgress(ctx.Repository.VaraDir) {
		t.Fatalf("the merge should be completed:\n%s", res)
	}
	if readFile(dir, "yarn.lock") != "v=FEATURE\n" || readFile(dir, "a.go") != "x := FEATURE\n" {
		t.Fatalf("lock=%q a.go=%q", readFile(dir, "yarn.lock"), readFile(dir, "a.go"))
	}
	// The rule settled the lock file without asking; only a.go was asked about.
	if strings.Contains(out.String(), "yarn.lock") && strings.Contains(out.String(), "conflict 1 of 1") && strings.Count(out.String(), "=== ") != 1 {
		t.Fatalf("only a.go should have been asked about:\n%s", out.String())
	}
}

func TestMergeInteractiveCanLeaveTheMergeOpen(t *testing.T) {
	ctx, _ := policyRepo(t, oneConflict)
	sess, _ := scripted("o\nn\n") // resolve, then decline to complete
	res, err := commands.RunMergeWith(ctx, "feature", commands.MergeOptions{Interactive: sess})
	if err != nil {
		t.Fatal(err)
	}
	if !recovery.MergeInProgress(ctx.Repository.VaraDir) || !strings.Contains(res, "left open") {
		t.Fatalf("declining must leave the merge open:\n%s", res)
	}
	if out, err := commands.RunMergeContinue(ctx, ""); err != nil || !strings.Contains(out, "Merge complete") {
		t.Fatalf("it must still be completable: %v %q", err, out)
	}
}

func TestInteractiveNeverCorruptsOnReadError(t *testing.T) {
	ctx, _ := twoHunks(t)
	sess := &commands.Session{In: io.NopCloser(strings.NewReader("")), Out: io.Discard}
	if _, err := commands.RunResolveInteractive(ctx, commands.ResolveArgs{}, sess); err != nil {
		t.Fatalf("an empty session must not error: %v", err)
	}
}
