package authz

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// WritePolicy atomically writes a policy file granting each subject its
// capabilities (RFC-0018 §7.1 shape). It is how a higher layer SEEDS a policy —
// e.g. the Repository Manager seeding a new repository's owner (RFC-0019 §7.3) —
// without itself knowing the on-disk format: authz remains the only package that
// reads or writes policy (RFC-0018 A9).
//
// Every capability is validated against the closed set before writing, so a
// seed can never produce a file that would later fail LoadPolicy's fail-fast and
// strand the repository at DenyAll.
func WritePolicy(path string, subjects map[string][]Capability) error {
	pf := policyFile{Version: 1, Subjects: make(map[string][]string, len(subjects))}
	for subject, caps := range subjects {
		out := make([]string, 0, len(caps))
		for _, c := range caps {
			if !Known[c] {
				return fmt.Errorf("write policy %s: unknown capability %q for subject %q", path, c, subject)
			}
			out = append(out, string(c))
		}
		pf.Subjects[subject] = out
	}
	data, err := json.MarshalIndent(pf, "", "  ")
	if err != nil {
		return err
	}
	// Write to a temp file in the same directory, then rename, so a reader never
	// observes a half-written policy (same atomic-rename discipline as the store).
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// PolicyPath returns the on-disk path of a repository's policy file within a
// policy root (RFC-0018 §7). Exposed so a seeder can locate the file it must
// write/move/remove without duplicating the naming rule.
func PolicyPath(root, repo string) string {
	return filepath.Join(root, repo+".json")
}
