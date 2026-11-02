package commands

import (
	"strings"
	"testing"
)

func TestCheckServeSafety(t *testing.T) {
	policy := ServeConfig{PolicyDir: "/srv/policy"}
	open := ServeConfig{}
	explicit := ServeConfig{AllowAnonymousAll: true}

	cases := []struct {
		name    string
		addr    string
		cfg     ServeConfig
		allowed bool
	}{
		{"policy on every interface", ":8080", policy, true},
		{"policy on a public address", "0.0.0.0:8080", policy, true},
		{"no policy on the default address", ":8080", open, false},
		{"no policy on all interfaces", "0.0.0.0:8080", open, false},
		{"no policy on all IPv6 interfaces", "[::]:8080", open, false},
		{"no policy on a LAN address", "192.168.1.20:8080", open, false},
		{"no policy on a hostname", "hub.example:8080", open, false},
		{"no policy on localhost", "localhost:8080", open, true},
		{"no policy on 127.0.0.1", "127.0.0.1:8080", open, true},
		{"no policy on another loopback address", "127.1.2.3:9000", open, true},
		{"no policy on IPv6 loopback", "[::1]:8080", open, true},
		{"explicit opt-in on all interfaces", ":8080", explicit, true},
	}
	for _, c := range cases {
		err := checkServeSafety(c.addr, c.cfg)
		if (err == nil) != c.allowed {
			t.Errorf("%s: allowed=%v, err=%v", c.name, c.allowed, err)
		}
		if err != nil && !strings.Contains(err.Error(), "--policy") {
			t.Errorf("%s: the refusal should say how to fix it, got %q", c.name, err)
		}
	}
}
