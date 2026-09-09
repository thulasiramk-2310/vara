package smartmerge

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/thulasiramk-2310/vara/pkg/config"
)

func decode(t *testing.T, b []byte) any {
	t.Helper()
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, b)
	}
	return v
}

func mergeJSON(base, ours, theirs string) Result {
	return MergeJSON([]byte(base), []byte(ours), []byte(theirs), "main", "feature")
}

// --- clean structural merges ------------------------------------------------

func TestMergeJSONIndependentKeys(t *testing.T) {
	r := mergeJSON(`{"a":1,"b":2}`, `{"a":10,"b":2}`, `{"a":1,"b":20}`)
	if !r.ParseOK || r.Conflicts != 0 {
		t.Fatalf("expected a clean merge, got %+v", r)
	}
	want := map[string]any{"a": float64(10), "b": float64(20)}
	if got := decode(t, r.Merged); !reflect.DeepEqual(got, want) {
		t.Fatalf("merged = %v, want %v", got, want)
	}
}

func TestMergeJSONNestedIndependent(t *testing.T) {
	r := mergeJSON(`{"cfg":{"a":1,"b":2}}`, `{"cfg":{"a":9,"b":2}}`, `{"cfg":{"a":1,"b":8}}`)
	if !r.ParseOK || r.Conflicts != 0 {
		t.Fatalf("expected clean, got %+v", r)
	}
	want := map[string]any{"cfg": map[string]any{"a": float64(9), "b": float64(8)}}
	if got := decode(t, r.Merged); !reflect.DeepEqual(got, want) {
		t.Fatalf("merged = %v, want %v", got, want)
	}
}

func TestMergeJSONAddDistinctKeys(t *testing.T) {
	r := mergeJSON(`{"a":1}`, `{"a":1,"x":true}`, `{"a":1,"y":false}`)
	if r.Conflicts != 0 {
		t.Fatalf("adding distinct keys should be clean, got %+v", r)
	}
	want := map[string]any{"a": float64(1), "x": true, "y": false}
	if got := decode(t, r.Merged); !reflect.DeepEqual(got, want) {
		t.Fatalf("merged = %v, want %v", got, want)
	}
}

// --- per-key conflict rendering ---------------------------------------------

func TestMergeJSONPerKeyConflictMarkers(t *testing.T) {
	// "a" resolves cleanly (only ours changed); "b" diverges → per-key markers.
	r := mergeJSON(`{"a":1,"b":2}`, `{"a":10,"b":9}`, `{"a":1,"b":8}`)
	if !r.ParseOK || r.Conflicts != 1 || !r.Rendered {
		t.Fatalf("expected one rendered conflict, got %+v", r)
	}
	out := string(r.Merged)
	// Clean key merged in place.
	if !strings.Contains(out, `"a": 10`) {
		t.Fatalf("clean key 'a' should be merged to 10:\n%s", out)
	}
	// Diverging key wrapped in markers pointing at "b", with all three sides.
	for _, want := range []string{`"b":`, "<<<<<<< main", "9", "||||||| base", "2", "=======", "8", ">>>>>>> feature"} {
		if !strings.Contains(out, want) {
			t.Fatalf("conflict rendering missing %q:\n%s", want, out)
		}
	}
	// It must NOT be valid JSON (it carries markers) — sanity that markers exist.
	if !strings.Contains(out, "<<<<<<<") {
		t.Fatalf("expected conflict markers:\n%s", out)
	}
}

func TestMergeJSONModifyDeleteKeyRenders(t *testing.T) {
	// ours modifies "a"; theirs deletes it → conflict, theirs side empty.
	r := mergeJSON(`{"a":1}`, `{"a":2}`, `{}`)
	if !r.ParseOK || r.Conflicts != 1 || !r.Rendered {
		t.Fatalf("modify/delete must render a conflict, got %+v", r)
	}
	out := string(r.Merged)
	if !strings.Contains(out, "<<<<<<< main") || !strings.Contains(out, "2") {
		t.Fatalf("expected ours side with value 2:\n%s", out)
	}
	// theirs deleted → the =======..>>>>>>> section has no value line between them.
	if !strings.Contains(out, "=======\n>>>>>>> feature") {
		t.Fatalf("deleted (theirs) side should be empty:\n%s", out)
	}
}

func TestMergeJSONMalformedFallsBack(t *testing.T) {
	if r := mergeJSON(`{`, `{"a":1}`, `{"a":2}`); r.ParseOK {
		t.Fatalf("unparseable input must report ParseOK=false, got %+v", r)
	}
}

func TestMergeJSONOrderIndependent(t *testing.T) {
	m1 := mergeJSON(`{"a":1,"b":2}`, `{"a":10,"b":2}`, `{"a":1,"b":20}`)
	m2 := mergeJSON(`{"a":1,"b":2}`, `{"a":1,"b":20}`, `{"a":10,"b":2}`)
	if m1.Conflicts != 0 || m2.Conflicts != 0 {
		t.Fatal("both directions should be clean")
	}
	if string(m1.Merged) != string(m2.Merged) {
		t.Fatalf("structural merge must be order-independent:\n%s\nvs\n%s", m1.Merged, m2.Merged)
	}
}

// --- YAML -------------------------------------------------------------------

func mergeYAML(base, ours, theirs string) Result {
	return MergeYAML([]byte(base), []byte(ours), []byte(theirs), "main", "feature")
}

func TestMergeYAMLIndependentKeys(t *testing.T) {
	r := mergeYAML("a: 1\nb: 2\n", "a: 10\nb: 2\n", "a: 1\nb: 20\n")
	if !r.ParseOK || r.Conflicts != 0 {
		t.Fatalf("expected a clean structural YAML merge, got %+v", r)
	}
	var v any
	if err := yaml.Unmarshal(r.Merged, &v); err != nil {
		t.Fatalf("merged output is not valid YAML: %v\n%s", err, r.Merged)
	}
	want := map[string]any{"a": 10, "b": 20}
	if !reflect.DeepEqual(v, want) {
		t.Fatalf("merged = %#v, want %#v", v, want)
	}
}

func TestMergeYAMLPerKeyConflict(t *testing.T) {
	// "a" resolves cleanly; "b" diverges → per-key YAML markers, clean key kept.
	r := mergeYAML("a: 1\nb: 2\n", "a: 10\nb: 9\n", "a: 1\nb: 8\n")
	if !r.ParseOK || r.Conflicts != 1 || !r.Rendered {
		t.Fatalf("expected one rendered YAML conflict, got %+v", r)
	}
	out := string(r.Merged)
	if !strings.Contains(out, "a: 10") {
		t.Fatalf("clean key 'a' should be merged to 10:\n%s", out)
	}
	for _, want := range []string{"b:", "<<<<<<< main", "||||||| base", "=======", ">>>>>>> feature"} {
		if !strings.Contains(out, want) {
			t.Fatalf("YAML conflict rendering missing %q:\n%s", want, out)
		}
	}
}

func TestMergeYAMLMalformedFallsBack(t *testing.T) {
	if r := mergeYAML("a:\n\tb: 1\n", "a: 1\n", "a: 2\n"); r.ParseOK {
		t.Fatalf("unparseable YAML must report ParseOK=false, got %+v", r)
	}
}

// --- driver selection -------------------------------------------------------

func TestSelectDriver(t *testing.T) {
	if d := SelectDriver("x.json", nil); d != "" {
		t.Fatal("nil config must disable structural merge")
	}
	cfg := config.New()
	if d := SelectDriver("x.json", cfg); d != "" {
		t.Fatal("unset config must default to line merge (opt-in)")
	}
	cfg.Set("merge", "", "driver.json", "structured-json")
	if d := SelectDriver("x.json", cfg); d != "structured-json" {
		t.Fatalf("enabled JSON config must select structured-json, got %q", d)
	}
	if d := SelectDriver("dir/Config.JSON", cfg); d != "structured-json" {
		t.Fatal("extension match must be case-insensitive")
	}
	if d := SelectDriver("notes.txt", cfg); d != "" {
		t.Fatal("non-json paths must stay on the line merge")
	}
	cfg.Set("merge", "", "driver.yaml", "structured-yaml")
	if d := SelectDriver("k8s.yaml", cfg); d != "structured-yaml" {
		t.Fatalf("enabled YAML config must select structured-yaml, got %q", d)
	}
	if d := SelectDriver("k8s.yml", cfg); d != "structured-yaml" {
		t.Fatal(".yml must also select the yaml driver")
	}
	cfg.Set("merge", "", "driver.json", "line")
	if d := SelectDriver("x.json", cfg); d != "" {
		t.Fatal("an explicit non-structural value must disable it")
	}
}
