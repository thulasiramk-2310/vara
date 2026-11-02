package commands

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/thulasiramk-2310/vara/internal/conflict"
	"github.com/thulasiramk-2310/vara/internal/mergestate"
	"github.com/thulasiramk-2310/vara/pkg/object"
)

// Session is the terminal an interactive resolution talks to. The command line
// uses the real one; tests hand in scripted input and a buffer.
type Session struct {
	In  io.Reader
	Out io.Writer
	// Edit opens text for the person to change and returns what they saved.
	Edit func(initial string) (string, error)

	reader *bufio.Reader
}

// NewTerminalSession is the session for a real terminal: standard input and
// output, and the user's editor ($VISUAL, then $EDITOR, then vi) for "edit".
func NewTerminalSession() *Session {
	return &Session{In: os.Stdin, Out: os.Stdout, Edit: editInEditor}
}

// IsTerminal reports whether f is an interactive terminal.
func IsTerminal(f *os.File) bool {
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

func (s *Session) printf(format string, a ...any) { fmt.Fprintf(s.Out, format, a...) }

// readLine returns the next line of input without its newline. ok is false when
// the input has ended, which callers treat as "quit", never as an answer.
func (s *Session) readLine() (string, bool) {
	if s.reader == nil {
		s.reader = bufio.NewReader(s.In)
	}
	line, err := s.reader.ReadString('\n')
	if err != nil && line == "" {
		return "", false
	}
	return strings.TrimSpace(line), true
}

// editInEditor writes initial to a private temporary file, opens the user's
// editor on it, and returns the saved text. The file is always removed.
func editInEditor(initial string) (string, error) {
	f, err := os.CreateTemp("", "vara-conflict-*.txt")
	if err != nil {
		return "", err
	}
	defer os.Remove(f.Name())
	if _, err := f.WriteString(initial); err != nil {
		f.Close()
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	editor := os.Getenv("VISUAL")
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor == "" {
		editor = "vi"
	}
	// The editor setting can carry arguments ("code --wait"), so the shell
	// splits it; the file name is passed separately and is never parsed.
	cmd := exec.Command("sh", "-c", editor+` "$1"`, "sh", f.Name())
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("editor %q failed: %w", editor, err)
	}
	out, err := os.ReadFile(f.Name())
	return string(out), err
}

const interactiveHelp = `  o   keep OUR side of this conflict
  t   keep THEIR side
  b   keep both, ours first          (r  both, theirs first)
  e   edit it yourself in your editor
  s   skip: leave this conflict for later
  O   keep ours for this and every remaining conflict in this file
  T   keep theirs for this and every remaining conflict in this file
  q   quit now; what you have already decided is kept
  ?   show this help
`

// RunResolveInteractive walks through the unresolved conflicts and asks, hunk by
// hunk, what to keep. Everything decided is saved as it goes, so quitting (or the
// terminal closing) loses nothing, and a later run carries on with what is left.
// It changes only the files it is asked about, and uses the same re-stage and
// conflict record as 'vara resolve', so 'vara merge --continue' works afterwards.
func RunResolveInteractive(ctx *Context, ra ResolveArgs, s *Session) (string, error) {
	st, hasSidecar, err := mergestate.Read(ctx.Repository.VaraDir)
	if err != nil {
		return "", fmt.Errorf("resolve: read conflict state: %w", err)
	}
	if !hasSidecar {
		return "", fmt.Errorf("resolve: no conflict record to work from; start the merge again with this version of vara")
	}
	var entries []mergestate.Entry
	for _, e := range st.Entries {
		if !e.Resolved {
			entries = append(entries, e)
		}
	}
	if len(ra.Paths) > 0 {
		entries = filterEntriesByPathspecs(entries, ra.Paths)
	}
	if len(entries) == 0 {
		return noConflictsMessage(ctx), nil
	}

	store := object.NewStore(ctx.Repository.VaraDir)
	run := &interactiveRun{ctx: ctx, store: store, st: st, s: s, style: configuredMarkerStyle(ctx)}
	var sb strings.Builder

	for i, e := range entries {
		s.printf("\n=== %s  (file %d of %d)\n", e.Path, i+1, len(entries))
		var line string
		var err error
		switch {
		case e.Kind == mergestate.KindModifyDelete:
			line, err = run.modifyDelete(e)
		default:
			bin, berr := entryIsBinary(store, e)
			if berr != nil {
				return sb.String(), fmt.Errorf("resolve %s: %w", e.Path, berr)
			}
			if bin {
				line, err = run.binary(e)
			} else {
				line, err = run.text(e)
			}
		}
		if err != nil {
			return sb.String(), fmt.Errorf("resolve %s: %w", e.Path, err)
		}
		sb.WriteString(line)
		// Save after every file so nothing is lost if the session ends abruptly.
		if err := writeIndex(ctx); err != nil {
			return sb.String(), fmt.Errorf("resolve: write index: %w", err)
		}
		if err := mergestate.Write(ctx.Repository.VaraDir, st); err != nil {
			return sb.String(), fmt.Errorf("resolve: update conflict state: %w", err)
		}
		if run.quit {
			break
		}
	}

	s.printf("\n")
	out := "Interactive resolution:\n" + sb.String()
	if remaining := st.UnresolvedPaths(); len(remaining) > 0 {
		out += "Still to resolve:\n\t" + strings.Join(remaining, "\n\t") + "\nRun the same command again to carry on.\n"
	} else {
		out += "All conflicts resolved. Run 'vara merge --continue' to complete the merge.\n"
	}
	return out, nil
}

type interactiveRun struct {
	ctx   *Context
	store *object.Store
	st    *mergestate.State
	s     *Session
	style conflict.MarkerStyle
	quit  bool
}

// ask prints prompt and returns the answer, one of valid (single letters, plus
// the capitals O and T where offered). Unknown answers are explained and asked
// again; five in a row, or the end of the input, count as quitting.
func (r *interactiveRun) ask(prompt, valid string) string {
	for tries := 0; tries < 5; tries++ {
		r.s.printf("%s", prompt)
		line, ok := r.s.readLine()
		if !ok {
			r.s.printf("\n")
			return "q"
		}
		switch line {
		case "ours":
			line = "o"
		case "theirs":
			line = "t"
		case "both":
			line = "b"
		case "edit":
			line = "e"
		case "skip":
			line = "s"
		case "quit":
			line = "q"
		}
		if line == "?" {
			r.s.printf("%s", interactiveHelp)
			continue
		}
		if len(line) == 1 && strings.Contains(valid, line) {
			return line
		}
		if len(line) == 1 && line != "O" && line != "T" && strings.Contains(valid, strings.ToLower(line)) {
			return strings.ToLower(line)
		}
		r.s.printf("Please answer one of: %s (or ? for help)\n", spaced(valid))
	}
	return "q"
}

func spaced(valid string) string {
	parts := make([]string, 0, len(valid))
	for _, c := range valid {
		parts = append(parts, string(c))
	}
	return strings.Join(parts, " ")
}

// text walks the conflict hunks of one text file.
func (r *interactiveRun) text(e mergestate.Entry) (string, error) {
	abs := filepath.Join(r.ctx.Repository.RootDir, filepath.FromSlash(e.Path))
	data, err := os.ReadFile(abs)
	if err != nil || !conflict.HasMarkers(data) {
		// No markers to work from (removed by hand, say): rebuild them from the
		// three stored sides, the same way 'vara resolve' does.
		base, berr := sideContent(r.store, e.Base)
		ours, oerr := sideContent(r.store, e.Ours)
		theirs, terr := sideContent(r.store, e.Theirs)
		if err := errors.Join(berr, oerr, terr); err != nil {
			return "", err
		}
		ourLabel, theirLabel := conflictLabels(r.st)
		data, _ = conflict.Render(base, ours, theirs, ourLabel, theirLabel, r.style)
	}

	segs, perr := conflict.Parse(data)
	if perr != nil {
		r.s.printf("Cannot read the conflict markers in this file (%v).\nResolve it by hand, then 'vara add' it.\n", perr)
		return fmt.Sprintf("\tskipped   %s (unreadable markers)\n", e.Path), nil
	}
	total := conflict.HunkCount(segs)
	if total == 0 {
		if err := restageResolved(r.ctx, r.store, e.Path, data); err != nil {
			return "", err
		}
		r.st.SetResolved(e.Path, true)
		return fmt.Sprintf("\tresolved  %s (no conflict left to decide)\n", e.Path), nil
	}

	decisions := make([]conflict.Decision, total)
	bulk := conflict.PickSkip
	k := 0
	for si, seg := range segs {
		if seg.Hunk == nil {
			continue
		}
		h := seg.Hunk
		if r.quit {
			break
		}
		if bulk != conflict.PickSkip {
			decisions[k].Pick = bulk
			k++
			continue
		}
		r.show(e.Path, segs, si, k+1, total)
		for {
			valid := "otbresqOT"
			a := r.ask("Keep [o]urs, [t]heirs, [b]oth, [e]dit, [s]kip, [q]uit, [?] > ", valid)
			switch a {
			case "o":
				decisions[k].Pick = conflict.PickOurs
			case "t":
				decisions[k].Pick = conflict.PickTheirs
			case "b":
				decisions[k].Pick = conflict.PickBoth
			case "r":
				decisions[k].Pick = conflict.PickBothTheirsFirst
			case "O":
				decisions[k].Pick, bulk = conflict.PickOurs, conflict.PickOurs
			case "T":
				decisions[k].Pick, bulk = conflict.PickTheirs, conflict.PickTheirs
			case "s":
				// leave it
			case "q":
				r.quit = true
			case "e":
				custom, ok := r.edit(h, si == len(segs)-1)
				if !ok {
					continue // ask again about this same conflict
				}
				decisions[k].Pick, decisions[k].Custom = conflict.PickCustom, custom
			}
			break
		}
		k++
	}

	decided := 0
	for _, d := range decisions {
		if d.Pick != conflict.PickSkip {
			decided++
		}
	}
	if decided == 0 {
		return fmt.Sprintf("\tskipped   %s (nothing decided)\n", e.Path), nil
	}
	out, left := conflict.Assemble(segs, decisions)
	if err := restageResolved(r.ctx, r.store, e.Path, out); err != nil {
		return "", err
	}
	if left == 0 {
		r.st.SetResolved(e.Path, true)
		return fmt.Sprintf("\tresolved  %s (%d hunk%s, chosen by you)\n", e.Path, total, plural(total)), nil
	}
	r.st.SetResolved(e.Path, false)
	return fmt.Sprintf("\tpartial   %s (%d of %d decided)\n", e.Path, total-left, total), nil
}

// show prints one hunk with a few lines of the surrounding text for context.
func (r *interactiveRun) show(path string, segs []conflict.Segment, at, n, total int) {
	const context = 3
	h := segs[at].Hunk
	r.s.printf("\n--- %s: conflict %d of %d\n", path, n, total)
	if at > 0 && segs[at-1].Text != nil {
		before := segs[at-1].Text
		if len(before) > context {
			before = before[len(before)-context:]
		}
		for _, l := range before {
			r.s.printf("    %s\n", strings.TrimRight(l, "\r\n"))
		}
	}
	block := func(title string, lines []string) {
		r.s.printf("  %s:\n", title)
		if len(lines) == 0 {
			r.s.printf("  | (nothing)\n")
		}
		for _, l := range lines {
			r.s.printf("  | %s\n", strings.TrimRight(l, "\r\n"))
		}
	}
	block("ours ("+orDefault(h.OurLabel, "yours")+")", h.Ours)
	if h.HasBase {
		block("base (where both started)", h.Base)
	}
	block("theirs ("+orDefault(h.TheirLabel, "theirs")+")", h.Theirs)
	if at+1 < len(segs) && segs[at+1].Text != nil {
		after := segs[at+1].Text
		if len(after) > context {
			after = after[:context]
		}
		for _, l := range after {
			r.s.printf("    %s\n", strings.TrimRight(l, "\r\n"))
		}
	}
}

func orDefault(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

// edit lets the person write the resolution of one hunk. The editor opens on the
// hunk with its markers; the answer is refused while any marker is left in it.
func (r *interactiveRun) edit(h *conflict.Hunk, lastSegment bool) ([]string, bool) {
	if r.s.Edit == nil {
		r.s.printf("No editor is available in this session.\n")
		return nil, false
	}
	text, err := r.s.Edit(strings.Join(h.Raw, ""))
	if err != nil {
		r.s.printf("Could not edit: %v\n", err)
		return nil, false
	}
	if segs, perr := conflict.Parse([]byte(text)); perr != nil || conflict.HunkCount(segs) > 0 {
		r.s.printf("That text still contains conflict markers. Remove them, or choose another option.\n")
		return nil, false
	}
	if text != "" && !strings.HasSuffix(text, "\n") && !lastSegment {
		text += "\n" // the lines that follow must not fuse onto this one
	}
	lines := strings.SplitAfter(text, "\n")
	if n := len(lines); n > 0 && lines[n-1] == "" {
		lines = lines[:n-1]
	}
	return lines, true
}

// binary asks which whole version of a binary file to keep.
func (r *interactiveRun) binary(e mergestate.Entry) (string, error) {
	r.s.printf("%s\nA binary file cannot be merged line by line; keep one whole version.\n", describeBinary(e))
	a := r.ask("Keep [o]urs, [t]heirs, [s]kip, [q]uit > ", "otsq")
	switch a {
	case "q":
		r.quit = true
		fallthrough
	case "s":
		return fmt.Sprintf("\tskipped   %s (binary)\n", e.Path), nil
	}
	side := e.Ours
	name := "ours"
	if a == "t" {
		side, name = e.Theirs, "theirs"
	}
	content, err := sideContent(r.store, side)
	if err != nil {
		return "", err
	}
	if err := restageResolved(r.ctx, r.store, e.Path, content); err != nil {
		return "", err
	}
	r.st.SetResolved(e.Path, true)
	return fmt.Sprintf("\tresolved  %s (binary, %s)\n", e.Path, name), nil
}

// modifyDelete asks whether to keep a file that one side changed and the other deleted.
func (r *interactiveRun) modifyDelete(e mergestate.Entry) (string, error) {
	r.s.printf("%s\n", describeModifyDelete(e))
	r.s.printf("  ours:   %s\n  theirs: %s\n", sideName(e.Ours), sideName(e.Theirs))
	a := r.ask("Take [o]urs, take [t]heirs, [s]kip, [q]uit > ", "otsq")
	switch a {
	case "q":
		r.quit = true
		fallthrough
	case "s":
		return fmt.Sprintf("\tskipped   %s (modify/delete)\n", e.Path), nil
	}
	side, name := e.Ours, "ours"
	if a == "t" {
		side, name = e.Theirs, "theirs"
	}
	if err := applyModifyDeleteChoice(r.ctx, r.store, e, side); err != nil {
		return "", err
	}
	r.st.SetResolved(e.Path, true)
	return fmt.Sprintf("\tresolved  %s (modify/delete, %s: %s)\n", e.Path, name, sideName(side)), nil
}
