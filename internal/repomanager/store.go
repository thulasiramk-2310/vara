package repomanager

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// The metadata store is the on-disk home of the "what is this?" record, one JSON
// file per repository keyed by immutable ID: <meta-dir>/<id>.json (RFC-0019
// §5.2). Keying by id (not name) is what lets a repository keep one metadata
// record across a rename. These are pure file helpers; the Manager owns the
// in-memory index and all locking.

// writeMeta atomically writes one metadata record (temp file + rename), so a
// concurrent scanner never observes a half-written record.
func writeMeta(dir string, m *Metadata) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	final := filepath.Join(dir, m.ID+".json")
	tmp := final + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, final); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// removeMeta deletes one metadata record. A missing file is not an error, so a
// resumed delete (RFC-0019 §5.6) is idempotent.
func removeMeta(dir, id string) error {
	err := os.Remove(filepath.Join(dir, id+".json"))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// scanMeta reads every metadata record under dir. Temp files (mid-write) are
// skipped. A malformed record is a hard error: the store must not silently drop
// a repository it cannot parse.
func scanMeta(dir string) ([]*Metadata, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []*Metadata
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		var m Metadata
		if err := json.Unmarshal(data, &m); err != nil {
			return nil, fmt.Errorf("metadata %s: %w", name, err)
		}
		out = append(out, &m)
	}
	return out, nil
}
