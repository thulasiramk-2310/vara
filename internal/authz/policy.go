package authz

import (
	"encoding/json"
	"fmt"
	"os"
)

// Policy is an immutable, parsed authorization policy for one repository: a map
// of subject -> granted capabilities (RFC-0018 §7.1). Anonymous is an ordinary
// subject key.
type Policy struct {
	subjects map[string]map[Capability]bool
}

// DenyAll is the policy that grants nothing. It is returned when a configured
// policy store has no file for a repository, so the default is deny (RFC-0018
// A4). A nil *Policy also denies everything.
var DenyAll = &Policy{subjects: map[string]map[Capability]bool{}}

// Allows reports whether subject holds cap. Absence of an explicit grant is a
// denial (default deny, RFC-0018 A4).
func (p *Policy) Allows(subject string, cap Capability) bool {
	if p == nil {
		return false
	}
	caps, ok := p.subjects[subject]
	if !ok {
		return false
	}
	return caps[cap]
}

// policyFile is the on-disk JSON shape (RFC-0018 §7.1).
type policyFile struct {
	Version  int                 `json:"version"`
	Subjects map[string][]string `json:"subjects"`
}

// LoadPolicy reads and validates a policy file. An unknown capability token
// invalidates the whole policy (fail-fast, RFC-0018 §7.1): the returned error
// means the file MUST NOT be used, which — combined with default-deny — fails
// closed.
func LoadPolicy(path string) (*Policy, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var pf policyFile
	if err := json.Unmarshal(data, &pf); err != nil {
		return nil, fmt.Errorf("policy %s: %w", path, err)
	}
	p := &Policy{subjects: make(map[string]map[Capability]bool, len(pf.Subjects))}
	for subject, caps := range pf.Subjects {
		set := make(map[Capability]bool, len(caps))
		for _, c := range caps {
			cap := Capability(c)
			if !Known[cap] {
				return nil, fmt.Errorf("policy %s: unknown capability %q for subject %q", path, c, subject)
			}
			set[cap] = true
		}
		p.subjects[subject] = set
	}
	return p, nil
}
