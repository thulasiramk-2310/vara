package config

import (
	"reflect"
	"testing"
)

func TestEntriesListsASectionInFileOrder(t *testing.T) {
	c, err := Parse("[merge]\nstrategy = manual\n[merge \"rule\"]\n*.lock = theirs\nCHANGELOG.md = Union\ndocs/** = ours\n")
	if err != nil {
		t.Fatal(err)
	}
	got := c.Entries("merge", "rule")
	want := []Entry{{"*.lock", "theirs"}, {"changelog.md", "Union"}, {"docs/**", "ours"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Entries = %#v, want %#v", got, want)
	}
	if g := c.Entries("merge", ""); len(g) != 1 || g[0].Key != "strategy" {
		t.Fatalf("the plain [merge] section must be separate from [merge \"rule\"]: %#v", g)
	}
	if c.Entries("nope", "") != nil {
		t.Fatal("a missing section lists nothing")
	}
}
