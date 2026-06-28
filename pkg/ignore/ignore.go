// Package ignore implements VARA Ignore Subset v1 parsing and matching.
//
// Responsibilities:
// - Parse .varaignore files.
// - Evaluate path matches against deterministic v1 rules:
//   - Comments (#)
//   - Blank lines
//   - *.ext
//   - directory/
//   - exact-name
package ignore

import (
	"bufio"
	"os"
	"strings"
)

// Matcher evaluates paths against a set of compiled rules.
type Matcher struct {
	rules []string
}

// Load reads and parses a .varaignore file.
func Load(path string) (*Matcher, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Matcher{}, nil
		}
		return nil, err
	}
	defer f.Close()

	var rules []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		rules = append(rules, line)
	}
	return &Matcher{rules: rules}, scanner.Err()
}

// Ignore checks if a given forward-slash relative path matches any rule.
func (m *Matcher) Ignore(relPath string) bool {
	// The .vara directory is ALWAYS ignored.
	if relPath == ".vara" || strings.HasPrefix(relPath, ".vara/") {
		return true
	}

	for _, rule := range m.rules {
		if matchRule(rule, relPath) {
			return true
		}
	}
	return false
}

// matchRule applies the Subset v1 logic.
func matchRule(rule, path string) bool {
	// 1. *.ext
	if strings.HasPrefix(rule, "*.") {
		ext := strings.TrimPrefix(rule, "*")
		return strings.HasSuffix(path, ext)
	}

	// 2. directory/
	if strings.HasSuffix(rule, "/") {
		dir := strings.TrimSuffix(rule, "/")
		// Match exact directory name, or anything underneath it
		if path == dir || strings.HasPrefix(path, dir+"/") {
			return true
		}
		// Also match if the directory appears anywhere in the path (e.g. node_modules/)
		if strings.Contains(path, "/"+dir+"/") || strings.HasSuffix(path, "/"+dir) {
			return true
		}
		// Also match if it starts with the directory name and slash
		if strings.HasPrefix(path, dir+"/") {
			return true
		}
		return false
	}

	// 3. Exact filename match (e.g. "build")
	if path == rule || strings.HasSuffix(path, "/"+rule) || strings.HasPrefix(path, rule+"/") {
		return true
	}

	return false
}
