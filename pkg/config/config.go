// Package config implements a minimal subset of VARA-RFC-0010 (Configuration
// System): reading and writing the INI-format `.vara/config` file.
//
// RFC:
// VARA-RFC-0010 Configuration System
// VARA-RFC-0014 Remote Protocol (§4 Remote Configuration)
//
// Responsibilities:
//   - Parse and serialize the INI config format, including subsectioned
//     headers of the form [remote "origin"].
//   - Provide typed accessors for remotes (used by the remote protocol).
//
// This package MUST NOT:
//   - Resolve the configuration cascade (env/flags/global/system). Only the
//     repository-local file is handled for now; the cascade is future work.
//   - Import any higher layer (commands, transport, repository).
package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Section is one INI section: a name, an optional subname (the quoted part of
// a header like [remote "origin"]), and its ordered key/value pairs.
type Section struct {
	Name    string
	Subname string
	keys    map[string]string
	order   []string
}

func (s *Section) get(key string) (string, bool) {
	v, ok := s.keys[strings.ToLower(key)]
	return v, ok
}

func (s *Section) set(key, value string) {
	lk := strings.ToLower(key)
	if _, exists := s.keys[lk]; !exists {
		s.order = append(s.order, lk)
	}
	s.keys[lk] = value
}

// Config is an ordered collection of INI sections.
type Config struct {
	sections []*Section
}

// New returns an empty configuration.
func New() *Config { return &Config{} }

func sectionKey(name, subname string) string {
	return strings.ToLower(name) + "\x00" + subname
}

func (c *Config) find(name, subname string) *Section {
	target := sectionKey(name, subname)
	for _, s := range c.sections {
		if sectionKey(s.Name, s.Subname) == target {
			return s
		}
	}
	return nil
}

func (c *Config) findOrCreate(name, subname string) *Section {
	if s := c.find(name, subname); s != nil {
		return s
	}
	s := &Section{Name: name, Subname: subname, keys: map[string]string{}}
	c.sections = append(c.sections, s)
	return s
}

// Get returns the value for section/subname/key and whether it was present.
func (c *Config) Get(name, subname, key string) (string, bool) {
	s := c.find(name, subname)
	if s == nil {
		return "", false
	}
	return s.get(key)
}

// Set assigns a value, creating the section if necessary.
func (c *Config) Set(name, subname, key, value string) {
	c.findOrCreate(name, subname).set(key, value)
}

// RemoveSection deletes a section and all its keys. It is a no-op if absent.
func (c *Config) RemoveSection(name, subname string) {
	target := sectionKey(name, subname)
	out := c.sections[:0]
	for _, s := range c.sections {
		if sectionKey(s.Name, s.Subname) != target {
			out = append(out, s)
		}
	}
	c.sections = out
}

// Load reads a config file. A missing file yields an empty config, not an
// error — an uninitialized repository simply has no configured values.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return New(), nil
		}
		return nil, err
	}
	return Parse(string(data))
}

// Parse reads INI text into a Config.
func Parse(text string) (*Config, error) {
	c := New()
	var cur *Section
	sc := bufio.NewScanner(strings.NewReader(text))
	line := 0
	for sc.Scan() {
		line++
		raw := strings.TrimSpace(sc.Text())
		if raw == "" || raw[0] == '#' || raw[0] == ';' {
			continue
		}
		if raw[0] == '[' {
			name, subname, err := parseHeader(raw)
			if err != nil {
				return nil, fmt.Errorf("config line %d: %w", line, err)
			}
			cur = c.findOrCreate(name, subname)
			continue
		}
		eq := strings.IndexByte(raw, '=')
		if eq < 0 {
			return nil, fmt.Errorf("config line %d: expected key = value", line)
		}
		if cur == nil {
			return nil, fmt.Errorf("config line %d: key outside any section", line)
		}
		key := strings.TrimSpace(raw[:eq])
		val := strings.TrimSpace(raw[eq+1:])
		cur.set(key, val)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return c, nil
}

// parseHeader parses "[name]" or `[name "subname"]`.
func parseHeader(raw string) (name, subname string, err error) {
	if raw[len(raw)-1] != ']' {
		return "", "", fmt.Errorf("unterminated section header %q", raw)
	}
	inner := strings.TrimSpace(raw[1 : len(raw)-1])
	if q := strings.IndexByte(inner, '"'); q >= 0 {
		name = strings.TrimSpace(inner[:q])
		rest := inner[q+1:]
		end := strings.IndexByte(rest, '"')
		if end < 0 {
			return "", "", fmt.Errorf("unterminated subsection name in %q", raw)
		}
		subname = rest[:end]
		if name == "" {
			return "", "", fmt.Errorf("empty section name in %q", raw)
		}
		return name, subname, nil
	}
	if inner == "" {
		return "", "", fmt.Errorf("empty section header %q", raw)
	}
	return inner, "", nil
}

// Serialize renders the config back to INI text.
func (c *Config) Serialize() string {
	var b strings.Builder
	for i, s := range c.sections {
		if i > 0 {
			b.WriteByte('\n')
		}
		if s.Subname != "" {
			fmt.Fprintf(&b, "[%s \"%s\"]\n", s.Name, s.Subname)
		} else {
			fmt.Fprintf(&b, "[%s]\n", s.Name)
		}
		for _, k := range s.order {
			fmt.Fprintf(&b, "\t%s = %s\n", k, s.keys[k])
		}
	}
	return b.String()
}

// Save writes the config atomically (temp file + rename).
func (c *Config) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(c.Serialize()), 0644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// --- Remote helpers (RFC-0014 §4) ---

// DefaultFetchRefspec is assumed when a remote has no explicit fetch refspec.
const DefaultFetchRefspec = "+refs/heads/*:refs/remotes/%s/*"

// Remote describes one configured remote.
type Remote struct {
	Name  string
	URL   string
	Fetch string
}

// Remote returns the named remote, if configured.
func (c *Config) Remote(name string) (Remote, bool) {
	s := c.find("remote", name)
	if s == nil {
		return Remote{}, false
	}
	url, ok := s.get("url")
	if !ok {
		return Remote{}, false
	}
	fetch, ok := s.get("fetch")
	if !ok {
		fetch = fmt.Sprintf(DefaultFetchRefspec, name)
	}
	return Remote{Name: name, URL: url, Fetch: fetch}, true
}

// Remotes returns all configured remotes, sorted by name.
func (c *Config) Remotes() []Remote {
	var out []Remote
	for _, s := range c.sections {
		if strings.ToLower(s.Name) != "remote" || s.Subname == "" {
			continue
		}
		if r, ok := c.Remote(s.Subname); ok {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// AddRemote registers a remote with the default fetch refspec.
func (c *Config) AddRemote(name, url string) {
	c.Set("remote", name, "url", url)
	c.Set("remote", name, "fetch", fmt.Sprintf(DefaultFetchRefspec, name))
}

// RemoveRemote deletes a remote. Returns false if it did not exist.
func (c *Config) RemoveRemote(name string) bool {
	if c.find("remote", name) == nil {
		return false
	}
	c.RemoveSection("remote", name)
	return true
}
