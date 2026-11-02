package commands

import (
	"fmt"
	"os"
	"path"
	"sort"
	"strings"

	"github.com/thulasiramk-2310/vara/internal/conflict"
	"github.com/thulasiramk-2310/vara/internal/mergestate"
)

// MergeOptions says how a merge should treat the conflicts the engine could not
// settle by itself. The engine already combines edits that do not overlap; what
// is left are places where both sides changed the same lines, and only a person
// can say which side is right. These options let that person decide up front.
type MergeOptions struct {
	// Strategy, when set, applies to every conflict (vara merge --theirs).
	Strategy conflict.Strategy
	// Manual leaves every conflict for the user and ignores the merge.* settings
	// (vara merge --manual). The default, with no options and no config, is the
	// same: conflicts are left marked for 'vara resolve'.
	Manual bool
	// Interactive, when set, walks through whatever the choices above leave open,
	// asking about each conflict (vara merge --interactive).
	Interactive *Session
}

// Strategies a merge may apply on its own. "auto" is not one of them: the engine
// has already merged everything that can be merged without a decision.
func validMergeStrategy(s conflict.Strategy) bool {
	return s == conflict.Ours || s == conflict.Theirs || s == conflict.Union
}

// ParseMergeStrategy reads a strategy from a flag or a config value.
func ParseMergeStrategy(v string) (conflict.Strategy, error) {
	s := conflict.Strategy(strings.ToLower(strings.TrimSpace(v)))
	if !validMergeStrategy(s) {
		return "", fmt.Errorf("unknown merge strategy %q (want ours, theirs, union or manual)", v)
	}
	return s, nil
}

// mergePolicy is the effective rule set for one merge: the default strategy and
// the per-path rules from the repository config.
//
//	[merge]
//	    strategy = manual          # the default when absent
//	[merge "rule"]
//	    *.lock = theirs            # a pattern without "/" matches the file name
//	    CHANGELOG.md = union
//	    docs/** = ours             # a pattern with "/" matches the whole path; ** spans folders
//
// Patterns are case-insensitive, because config keys are stored lower-cased.
type mergePolicy struct {
	opts    MergeOptions
	def     conflict.Strategy
	rules   []mergeRule
	invalid []string // settings that were ignored, reported to the user
}

type mergeRule struct {
	pattern string
	strat   conflict.Strategy
}

func loadMergePolicy(ctx *Context, opts MergeOptions) mergePolicy {
	p := mergePolicy{opts: opts}
	if opts.Manual || opts.Strategy != "" {
		return p // the command line overrides the config
	}
	cfg, err := loadConfig(ctx.Repository.VaraDir)
	if err != nil {
		return p
	}
	if v, ok := cfg.Get("merge", "", "strategy"); ok && !strings.EqualFold(strings.TrimSpace(v), "manual") {
		if s, err := ParseMergeStrategy(v); err == nil {
			p.def = s
		} else {
			p.invalid = append(p.invalid, "merge.strategy = "+v)
		}
	}
	for _, e := range cfg.Entries("merge", "rule") {
		if strings.EqualFold(strings.TrimSpace(e.Value), "manual") {
			p.rules = append(p.rules, mergeRule{pattern: e.Key}) // an explicit "leave this to me"
			continue
		}
		s, err := ParseMergeStrategy(e.Value)
		if err != nil {
			p.invalid = append(p.invalid, fmt.Sprintf("merge.rule %s = %s", e.Key, e.Value))
			continue
		}
		p.rules = append(p.rules, mergeRule{pattern: e.Key, strat: s})
	}
	return p
}

// strategyFor picks how one conflicted path is handled: the command line first,
// then the first matching rule, then the default. "" means leave it to the user.
func (p mergePolicy) strategyFor(file string) conflict.Strategy {
	if p.opts.Manual {
		return ""
	}
	if p.opts.Strategy != "" {
		return p.opts.Strategy
	}
	for _, r := range p.rules {
		if matchMergePattern(r.pattern, file) {
			return r.strat
		}
	}
	return p.def
}

func (p mergePolicy) active() bool {
	return p.opts.Strategy != "" || p.def != "" || len(p.rules) > 0
}

// matchMergePattern reports whether a rule pattern covers a file path. A pattern
// without a slash is matched against the file's name, so "*.lock" covers
// "a/b/yarn.lock". A pattern with a slash is matched against the whole path, one
// folder at a time, and "**" stands for any number of folders.
func matchMergePattern(pattern, file string) bool {
	pattern = strings.ToLower(strings.Trim(pattern, "/"))
	file = strings.ToLower(file)
	if pattern == "" {
		return false
	}
	if !strings.Contains(pattern, "/") {
		ok, err := path.Match(pattern, path.Base(file))
		return err == nil && ok
	}
	return matchSegments(strings.Split(pattern, "/"), strings.Split(file, "/"))
}

func matchSegments(pat, seg []string) bool {
	for len(pat) > 0 {
		if pat[0] == "**" {
			if len(pat) == 1 {
				return len(seg) > 0 // a trailing ** covers everything beneath, not the folder itself
			}
			for i := 0; i <= len(seg); i++ {
				if matchSegments(pat[1:], seg[i:]) {
					return true
				}
			}
			return false
		}
		if len(seg) == 0 {
			return false
		}
		if ok, err := path.Match(pat[0], seg[0]); err != nil || !ok {
			return false
		}
		pat, seg = pat[1:], seg[1:]
	}
	return len(seg) == 0
}

// applyMergePolicy runs after a merge has left conflicts. It resolves, with the
// engine's own 'resolve' step, every conflict the policy covers; if that settles
// all of them it completes the merge, otherwise it reports what is still open.
// Nothing is hidden: the output says which side was kept for which files, and the
// merge can still be taken back with 'vara undo'.
func applyMergePolicy(ctx *Context, state *mergestate.State, opts MergeOptions) (string, error) {
	pol := loadMergePolicy(ctx, opts)
	var out strings.Builder
	for _, bad := range pol.invalid {
		fmt.Fprintf(&out, "warning: ignored %s (want ours, theirs, union or manual)\n", bad)
	}

	byStrategy := map[conflict.Strategy][]string{}
	var left []string
	for _, f := range state.Paths() {
		if s := pol.strategyFor(f); s != "" {
			byStrategy[s] = append(byStrategy[s], f)
		} else {
			left = append(left, f)
		}
	}
	if len(byStrategy) == 0 && opts.Interactive == nil {
		return out.String() + conflictSummary(state.Paths()), nil
	}

	order := []conflict.Strategy{conflict.Ours, conflict.Theirs, conflict.Union}
	for _, s := range order {
		files := byStrategy[s]
		if len(files) == 0 {
			continue
		}
		sort.Strings(files)
		fmt.Fprintf(&out, "Resolving by your %s choice (%s):\n", s, strings.Join(files, ", "))
		res, err := RunResolve(ctx, ResolveArgs{Strategy: s, Paths: files})
		if err != nil {
			// Leave the merge in progress and tell the user, so nothing is lost.
			fmt.Fprintf(os.Stderr, "vara: warning: could not apply %s to %s: %v\n", s, strings.Join(files, ", "), err)
			out.WriteString(conflictSummary(state.Paths()))
			return out.String(), nil
		}
		out.WriteString(res)
	}

	st, ok, err := mergestate.Read(ctx.Repository.VaraDir)
	if err != nil || !ok {
		return out.String(), err
	}
	remaining := st.UnresolvedPaths()
	if len(remaining) > 0 && opts.Interactive != nil {
		out.WriteString("\n")
		res, ierr := RunResolveInteractive(ctx, ResolveArgs{Paths: remaining}, opts.Interactive)
		out.WriteString(res)
		if ierr != nil {
			return out.String(), ierr
		}
		if st, ok, err = mergestate.Read(ctx.Repository.VaraDir); err != nil || !ok {
			return out.String(), err
		}
		remaining = st.UnresolvedPaths()
		if len(remaining) == 0 {
			s := opts.Interactive
			s.printf("Complete the merge now? [Y/n] ")
			if ans, got := s.readLine(); !got || !(ans == "" || strings.EqualFold(ans, "y") || strings.EqualFold(ans, "yes")) {
				out.WriteString("The merge is left open. Run 'vara merge --continue' to complete it, or 'vara merge --abort' to back out.\n")
				return out.String(), nil
			}
		}
	}
	if len(remaining) > 0 {
		out.WriteString(conflictSummary(remaining))
		return out.String(), nil
	}
	cont, err := RunMergeContinue(ctx, "")
	if err != nil {
		return out.String(), err
	}
	// The resolve step ends with "run 'vara commit' to complete the merge". That is
	// no longer true, because the merge is being completed right here.
	text := strings.ReplaceAll(out.String(), "All conflicts resolved. Run 'vara commit' to complete the merge.\n", "")
	text = strings.ReplaceAll(text, "All conflicts resolved. Run 'vara merge --continue' to complete the merge.\n", "")
	return text + cont, nil
}
