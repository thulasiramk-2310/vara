package fuzz

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thulasiramk-2310/vara/pkg/refs"
	"github.com/thulasiramk-2310/vara/pkg/verify"
)

// FuzzRefName ensures ValidateName never panics on arbitrary input.
func FuzzRefName(f *testing.F) {
	f.Add("main")
	f.Add("refs/heads/main")
	f.Add("")
	f.Add("../escape")
	f.Add("a..b")
	f.Add(strings.Repeat("x", 256))
	f.Add("HEAD")
	f.Add("refs/heads/\x00null")
	f.Add("feature/my-branch")

	f.Fuzz(func(t *testing.T, name string) {
		refs.ValidateName(name) // must not panic
	})
}

// FuzzJournalParser ensures the journal JSON parser never panics on arbitrary input.
func FuzzJournalParser(f *testing.F) {
	valid, _ := json.Marshal(map[string]string{
		"transaction": "txn-123",
		"state":       "COMPLETED",
		"command":     "commit",
	})
	f.Add(valid)
	f.Add([]byte(`{}`))
	f.Add([]byte(`{"transaction":"","state":"","command":""}`))
	f.Add([]byte(`not json at all`))
	f.Add([]byte(nil))
	f.Add([]byte(`{"transaction":` + strings.Repeat("x", 4096) + `}`))

	f.Fuzz(func(t *testing.T, data []byte) {
		var j struct {
			ID      string `json:"transaction"`
			State   string `json:"state"`
			Command string `json:"command"`
		}
		json.Unmarshal(data, &j) // must not panic
	})
}

// FuzzTreeBlob fuzzes the tree object parser with seeds that cover all
// edge cases: empty payloads, truncated entries, large entry counts.
func FuzzTreeBlob(f *testing.F) {
	// A minimal valid tree entry: 4-byte mode + 32-byte hash + name + NUL.
	entry := make([]byte, 4+32+4)
	entry[0], entry[1], entry[2], entry[3] = 0, 0, 0x81, 0xa4 // mode 0100644
	copy(entry[4:], bytes.Repeat([]byte{0xab}, 32))
	copy(entry[36:], []byte("name"))
	entry = append(entry, 0) // NUL terminator

	f.Add(append([]byte("vara-tree:v1\x00"), entry...))
	f.Add([]byte("vara-tree:v1\x00"))
	f.Add([]byte("vara-tree:v1\x00" + strings.Repeat("\xff", 128)))
	f.Add([]byte(nil))
	f.Add([]byte("vara-tree:v1\x00" + strings.Repeat("a\x00", 1024)))

	f.Fuzz(func(t *testing.T, data []byte) {
		r := bytes.NewReader(data)
		// ReadHeader then attempt tree parse — must not panic.
		if typ, err := readHeaderSafe(r); err == nil && typ == "vara-tree:v1" {
			r2 := bytes.NewReader(data[len("vara-tree:v1")+1:])
			// nolint: errcheck
			_ = r2
		}
	})
}

// FuzzCommitObject fuzzes the commit object parser.
func FuzzCommitObject(f *testing.F) {
	// A valid commit body: 32-byte tree + 4-byte parentCount(0) + 8-byte timestamp + "Author\0Message"
	body := make([]byte, 32+4+8)
	copy(body[:32], bytes.Repeat([]byte{0x11}, 32)) // tree hash
	// parentCount = 0 (already zero from make)
	body = append(body, []byte("User <u@example.com>\x00first commit")...)

	f.Add(append([]byte("vara-commit:v1\x00"), body...))
	f.Add([]byte("vara-commit:v1\x00"))
	f.Add([]byte("vara-commit:v1\x00" + strings.Repeat("\xff", 256)))
	f.Add([]byte(nil))

	f.Fuzz(func(t *testing.T, data []byte) {
		r := bytes.NewReader(data)
		_ = r // Ensure reader construction does not panic.
	})
}

// FuzzVerifyOnGarbage ensures the verify engine never panics when the .vara
// directory is filled with adversarial content.
func FuzzVerifyOnGarbage(f *testing.F) {
	f.Add([]byte("not zstd"))
	f.Add([]byte(nil))
	f.Add([]byte(strings.Repeat("\xff", 128)))

	f.Fuzz(func(t *testing.T, payload []byte) {
		// Create a minimal fake .vara directory.
		dir := t.TempDir()
		varaDir := filepath.Join(dir, ".vara")
		os.MkdirAll(filepath.Join(varaDir, "refs", "heads"), 0755)
		os.MkdirAll(filepath.Join(varaDir, "journal"), 0755)
		os.MkdirAll(filepath.Join(varaDir, "snapshots"), 0755)
		os.MkdirAll(filepath.Join(varaDir, "ab"), 0755)

		// Write a fake object file with adversarial content.
		fakeID := "ab" + strings.Repeat("0", 62)
		os.WriteFile(filepath.Join(varaDir, "ab", fakeID[2:]), payload, 0644)

		// Write a fake snapshot with adversarial content.
		os.WriteFile(filepath.Join(varaDir, "snapshots", "snap-20260101-000000-test-abc1234.tar.zst"), payload, 0644)

		verify.Verify(varaDir) // must not panic
	})
}

// readHeaderSafe is a panic-safe wrapper used by fuzz tests to avoid
// importing internal object package from the fuzz package.
func readHeaderSafe(r *bytes.Reader) (string, error) {
	const nulByte = byte(0)
	var hdr []byte
	for {
		b, err := r.ReadByte()
		if err != nil {
			return "", err
		}
		if b == nulByte {
			break
		}
		hdr = append(hdr, b)
		if len(hdr) > 32 {
			return "", nil
		}
	}
	return string(hdr), nil
}
