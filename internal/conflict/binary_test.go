package conflict

import "testing"

func TestIsBinary(t *testing.T) {
	cases := []struct {
		name string
		data []byte
		want bool
	}{
		{"empty", nil, false},
		{"plain text", []byte("hello\nworld\n"), false},
		{"nul byte", []byte("abc\x00def"), true},
		{"nul at start", []byte("\x00"), true},
		{"utf8 no nul", []byte("café ☕ résumé\n"), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := IsBinary(c.data); got != c.want {
				t.Fatalf("IsBinary(%q) = %v, want %v", c.data, got, c.want)
			}
		})
	}

	// A NUL beyond the sniff window is not detected (matches Git's bounded sniff).
	big := make([]byte, binarySniffLen+10)
	for i := range big {
		big[i] = 'x'
	}
	big[binarySniffLen+5] = 0
	if IsBinary(big) {
		t.Fatal("NUL past the sniff window should not be flagged binary")
	}
}
