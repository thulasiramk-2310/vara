// Package path provides OS-independent path normalization and canonicalization.
//
// Responsibilities:
// - Slash normalization (e.g. Windows "\" to "/").
// - Computing repository-relative paths.
package path

import (
	"path/filepath"
)

// ToRepoRelative normalizes a path to be forward-slash separated and relative to base.
// Base is expected to be the absolute path of the repository root.
// Target is the absolute or relative path to a file.
func ToRepoRelative(base, target string) (string, error) {
	if !filepath.IsAbs(target) {
		// If it's already relative, we just need to ensure it's relative to the base if we have one.
		// However, to keep it simple, we assume the inputs are correctly resolvable.
		absTarget, err := filepath.Abs(target)
		if err != nil {
			return "", err
		}
		target = absTarget
	}
	
	if !filepath.IsAbs(base) {
		absBase, err := filepath.Abs(base)
		if err != nil {
			return "", err
		}
		base = absBase
	}

	rel, err := filepath.Rel(base, target)
	if err != nil {
		return "", err
	}
	
	return filepath.ToSlash(rel), nil
}
