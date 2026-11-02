package commands

import (
	"testing"

	"github.com/thulasiramk-2310/vara/internal/conflict"
)

func TestMatchMergePattern(t *testing.T) {
	cases := []struct {
		pattern, file string
		want          bool
	}{
		// no slash: matches the file name, at any depth
		{"*.lock", "yarn.lock", true},
		{"*.lock", "web/yarn.lock", true},
		{"*.lock", "web/yarn.lock.bak", false},
		{"CHANGELOG.md", "CHANGELOG.md", true},
		{"CHANGELOG.md", "docs/changelog.md", true}, // case-insensitive
		{"CHANGELOG.md", "docs/CHANGELOG.md.old", false},
		{"go.*", "go.sum", true},
		// with a slash: anchored to the whole path, one folder at a time
		{"docs/*.md", "docs/a.md", true},
		{"docs/*.md", "docs/sub/a.md", false},
		{"docs/*.md", "other/docs/a.md", false},
		{"docs/**", "docs/a.md", true},
		{"docs/**", "docs/sub/deep/a.md", true},
		{"docs/**", "docs", false},
		{"src/**/gen.go", "src/gen.go", true},
		{"src/**/gen.go", "src/a/b/gen.go", true},
		{"src/**/gen.go", "src/a/b/other.go", false},
		{"**/vendor/**", "x/vendor/y/z.go", true},
		{"/docs/**", "docs/a.md", true}, // a leading slash is tolerated
		// things that must never match
		{"", "a.txt", false},
		{"[", "a.txt", false}, // a malformed pattern matches nothing instead of panicking
	}
	for _, c := range cases {
		if got := matchMergePattern(c.pattern, c.file); got != c.want {
			t.Errorf("matchMergePattern(%q, %q) = %v, want %v", c.pattern, c.file, got, c.want)
		}
	}
}

func TestParseMergeStrategy(t *testing.T) {
	for _, ok := range []string{"ours", "theirs", "union", " Theirs ", "UNION"} {
		if _, err := ParseMergeStrategy(ok); err != nil {
			t.Errorf("%q should be accepted: %v", ok, err)
		}
	}
	// "auto" is not a merge-time choice: the engine has already merged what it can.
	for _, bad := range []string{"", "auto", "manual", "mine", "ours,theirs"} {
		if _, err := ParseMergeStrategy(bad); err == nil {
			t.Errorf("%q should be rejected", bad)
		}
	}
}

func TestStrategyPrecedence(t *testing.T) {
	rules := []mergeRule{
		{pattern: "*.lock", strat: conflict.Theirs},
		{pattern: "docs/**", strat: conflict.Union},
		{pattern: "keep.txt"}, // an explicit "leave this one to me"
	}
	p := mergePolicy{def: conflict.Ours, rules: rules}
	want := map[string]conflict.Strategy{
		"yarn.lock":      conflict.Theirs, // a rule beats the default
		"docs/a.md":      conflict.Union,
		"src/main.go":    conflict.Ours, // no rule: the default
		"keep.txt":       "",            // an explicit manual rule beats the default
		"sub/keep.txt":   "",
		"docs/yarn.lock": conflict.Theirs, // the first matching rule wins
	}
	for f, w := range want {
		if got := p.strategyFor(f); got != w {
			t.Errorf("strategyFor(%q) = %q, want %q", f, got, w)
		}
	}

	// The command line overrides rules and the default.
	forced := mergePolicy{opts: MergeOptions{Strategy: conflict.Theirs}, def: conflict.Ours, rules: rules}
	for _, f := range []string{"yarn.lock", "docs/a.md", "keep.txt", "src/main.go"} {
		if got := forced.strategyFor(f); got != conflict.Theirs {
			t.Errorf("--theirs: strategyFor(%q) = %q", f, got)
		}
	}
	// --manual ignores everything.
	manual := mergePolicy{opts: MergeOptions{Manual: true}, def: conflict.Ours, rules: rules}
	if got := manual.strategyFor("yarn.lock"); got != "" {
		t.Errorf("--manual: strategyFor = %q, want none", got)
	}
	// Nothing configured: nothing is decided for the user.
	if (mergePolicy{}).active() || (mergePolicy{}).strategyFor("a") != "" {
		t.Error("an empty policy must not decide anything")
	}
}
