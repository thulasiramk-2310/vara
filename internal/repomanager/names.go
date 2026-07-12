package repomanager

import "strings"

// ValidName reports whether name is a legal repository name (RFC-0019 §10).
// Names are validated identically wherever a repository name is minted, so the
// control plane and data plane agree. The rules, and why each exists:
//
//   - one path segment, filesystem-safe charset [A-Za-z0-9._-] — so a name is a
//     valid directory on every supported OS and a valid <name>.json policy file;
//   - not "." / ".." / empty — no traversal, no current/parent directory;
//   - must not begin with "_" — reserves the control-plane prefix (_vara) and the
//     server policy key (_server), making /_vara/... and _server.json unspoofable;
//   - must not begin with "." — reserves .git / .vara and any dotfile-looking
//     name, so a repository can never shadow an engine or tooling directory.
//
// The "/" separator is rejected by the charset rule; keeping it reserved is what
// lets a future namespace scheme (owner/repo) be a pure extension rather than a
// migration (RFC-0019 §10, §14).
func ValidName(name string) bool {
	if name == "" || name == "." || name == ".." {
		return false
	}
	if name[0] == '_' || name[0] == '.' {
		return false
	}
	// Reject any ".." run, matching the data-plane traversal check, so a name can
	// never be interpreted as a parent-directory reference on any layer.
	if strings.Contains(name, "..") {
		return false
	}
	for _, r := range name {
		switch {
		case r >= 'A' && r <= 'Z',
			r >= 'a' && r <= 'z',
			r >= '0' && r <= '9',
			r == '.', r == '_', r == '-':
		default:
			return false
		}
	}
	return true
}
