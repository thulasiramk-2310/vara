package smartmerge

import (
	"encoding/json"
	"reflect"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/thulasiramk-2310/vara/pkg/config"
)

// decode re-parses merged bytes so assertions compare structure, not whitespace.
func decode(t *testing.T, b []byte) any {
	t.Helper()
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatalf("merged output is not valid JSON: %v\n%s", err, b)
	}
	return v
}

func TestMergeJSONIndependentKeys(t *testing.T) {
	// The headline case: compact JSON the line merger conflicts on, because both
	// edits are on the same line — but they touch different keys.
	base := []byte(`{"a":1,"b":2}`)
	ours := []byte(`{"a":10,"b":2}`)
	theirs := []byte(`{"a":1,"b":20}`)

	merged, clean, parseOK := MergeJSON(base, ours, theirs)
	if !parseOK || !clean {
		t.Fatalf("expected a clean structural merge, got clean=%v parseOK=%v", clean, parseOK)
	}
	want := map[string]any{"a": float64(10), "b": float64(20)}
	if got := decode(t, merged); !reflect.DeepEqual(got, want) {
		t.Fatalf("merged = %v, want %v", got, want)
	}
}

func TestMergeJSONSameKeyConflicts(t *testing.T) {
	base := []byte(`{"a":1}`)
	ours := []byte(`{"a":10}`)
	theirs := []byte(`{"a":20}`)
	_, clean, parseOK := MergeJSON(base, ours, theirs)
	if !parseOK {
		t.Fatal("expected parseOK")
	}
	if clean {
		t.Fatal("same key changed differently must NOT be clean (falls back to line merge)")
	}
}

func TestMergeJSONOneSideEqualsBase(t *testing.T) {
	base := []byte(`{"a":1,"b":2}`)
	ours := []byte(`{"a":1,"b":2}`) // unchanged
	theirs := []byte(`{"a":1,"b":99}`)
	merged, clean, parseOK := MergeJSON(base, ours, theirs)
	if !parseOK || !clean {
		t.Fatalf("clean=%v parseOK=%v", clean, parseOK)
	}
	want := map[string]any{"a": float64(1), "b": float64(99)}
	if got := decode(t, merged); !reflect.DeepEqual(got, want) {
		t.Fatalf("merged = %v, want %v", got, want)
	}
}

func TestMergeJSONAddDistinctKeys(t *testing.T) {
	base := []byte(`{"a":1}`)
	ours := []byte(`{"a":1,"x":true}`)
	theirs := []byte(`{"a":1,"y":false}`)
	merged, clean, _ := MergeJSON(base, ours, theirs)
	if !clean {
		t.Fatal("adding distinct keys should merge clean")
	}
	want := map[string]any{"a": float64(1), "x": true, "y": false}
	if got := decode(t, merged); !reflect.DeepEqual(got, want) {
		t.Fatalf("merged = %v, want %v", got, want)
	}
}

func TestMergeJSONModifyDeleteKeyConflicts(t *testing.T) {
	base := []byte(`{"a":1}`)
	ours := []byte(`{"a":2}`) // modified
	theirs := []byte(`{}`)    // deleted
	_, clean, parseOK := MergeJSON(base, ours, theirs)
	if !parseOK || clean {
		t.Fatalf("modify/delete of a key must conflict, got clean=%v parseOK=%v", clean, parseOK)
	}
}

func TestMergeJSONNestedIndependent(t *testing.T) {
	base := []byte(`{"cfg":{"a":1,"b":2}}`)
	ours := []byte(`{"cfg":{"a":9,"b":2}}`)
	theirs := []byte(`{"cfg":{"a":1,"b":8}}`)
	merged, clean, _ := MergeJSON(base, ours, theirs)
	if !clean {
		t.Fatal("independent nested-key edits should merge clean")
	}
	want := map[string]any{"cfg": map[string]any{"a": float64(9), "b": float64(8)}}
	if got := decode(t, merged); !reflect.DeepEqual(got, want) {
		t.Fatalf("merged = %v, want %v", got, want)
	}
}

func TestMergeJSONMalformedFallsBack(t *testing.T) {
	_, _, parseOK := MergeJSON([]byte(`{`), []byte(`{"a":1}`), []byte(`{"a":2}`))
	if parseOK {
		t.Fatal("unparseable input must report parseOK=false so the caller falls back to line merge")
	}
}

// TestMergeJSONOrderIndependent: swapping ours/theirs yields identical bytes.
func TestMergeJSONOrderIndependent(t *testing.T) {
	base := []byte(`{"a":1,"b":2}`)
	x := []byte(`{"a":10,"b":2}`)
	y := []byte(`{"a":1,"b":20}`)
	m1, c1, _ := MergeJSON(base, x, y)
	m2, c2, _ := MergeJSON(base, y, x)
	if !c1 || !c2 {
		t.Fatal("both directions should be clean")
	}
	if string(m1) != string(m2) {
		t.Fatalf("structural merge must be order-independent:\n%s\nvs\n%s", m1, m2)
	}
}

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

func TestMergeYAMLIndependentKeys(t *testing.T) {
	// Same headline case as JSON: independent keys the line merger would clash on.
	base := []byte("a: 1\nb: 2\n")
	ours := []byte("a: 10\nb: 2\n")
	theirs := []byte("a: 1\nb: 20\n")
	merged, clean, parseOK := MergeYAML(base, ours, theirs)
	if !parseOK || !clean {
		t.Fatalf("expected a clean structural YAML merge, got clean=%v parseOK=%v", clean, parseOK)
	}
	var v any
	if err := yaml.Unmarshal(merged, &v); err != nil {
		t.Fatalf("merged output is not valid YAML: %v\n%s", err, merged)
	}
	want := map[string]any{"a": 10, "b": 20} // yaml.v3 decodes ints as int
	if !reflect.DeepEqual(v, want) {
		t.Fatalf("merged = %#v, want %#v", v, want)
	}
}

func TestMergeYAMLNestedIndependent(t *testing.T) {
	base := []byte("cfg:\n  a: 1\n  b: 2\n")
	ours := []byte("cfg:\n  a: 9\n  b: 2\n")
	theirs := []byte("cfg:\n  a: 1\n  b: 8\n")
	merged, clean, _ := MergeYAML(base, ours, theirs)
	if !clean {
		t.Fatal("independent nested YAML edits should merge clean")
	}
	var v any
	if err := yaml.Unmarshal(merged, &v); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"cfg": map[string]any{"a": 9, "b": 8}}
	if !reflect.DeepEqual(v, want) {
		t.Fatalf("merged = %#v, want %#v", v, want)
	}
}

func TestMergeYAMLSameKeyConflicts(t *testing.T) {
	_, clean, parseOK := MergeYAML([]byte("a: 1\n"), []byte("a: 10\n"), []byte("a: 20\n"))
	if !parseOK || clean {
		t.Fatalf("same-key clash must conflict, got clean=%v parseOK=%v", clean, parseOK)
	}
}

func TestMergeYAMLMalformedFallsBack(t *testing.T) {
	// A tab in indentation is invalid YAML.
	_, _, parseOK := MergeYAML([]byte("a:\n\tb: 1\n"), []byte("a: 1\n"), []byte("a: 2\n"))
	if parseOK {
		t.Fatal("unparseable YAML must report parseOK=false so the caller falls back")
	}
}
