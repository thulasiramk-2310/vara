package identity

import (
	"encoding/base64"
	"testing"
)

func basicHeader(user, secret string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+secret))
}

func TestParseHeaderAnonymous(t *testing.T) {
	cred, err := ParseHeader("")
	if err != nil || cred != nil {
		t.Fatalf("empty header should be anonymous (nil,nil), got cred=%v err=%v", cred, err)
	}
}

func TestParseHeaderBasic(t *testing.T) {
	cred, err := ParseHeader(basicHeader("alice", "s3cret"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cred.Scheme != SchemeBasic || cred.User != "alice" || cred.Secret != "s3cret" {
		t.Fatalf("bad parse: %+v", cred)
	}
}

func TestParseHeaderBearer(t *testing.T) {
	cred, err := ParseHeader("Bearer tok-123")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cred.Scheme != SchemeBearer || cred.Token != "tok-123" {
		t.Fatalf("bad parse: %+v", cred)
	}
}

func TestParseHeaderMalformed(t *testing.T) {
	cases := []string{
		"Basic",                 // no space
		"Basic !!!notbase64!!!", // bad base64
		"Basic " + base64.StdEncoding.EncodeToString([]byte("nocolon")), // no user:secret
		"Bearer ",    // empty token
		"Digest abc", // unsupported scheme
	}
	for _, h := range cases {
		if _, err := ParseHeader(h); err == nil {
			t.Fatalf("expected parse error for %q", h)
		}
	}
}

// TestParsePrecedesAuthenticate proves the RFC-0017 §6.2 separation: a malformed
// header is rejected by ParseHeader and never produces a Credential, so a source
// (here a spy) is never consulted.
func TestParsePrecedesAuthenticate(t *testing.T) {
	spy := &spySource{}
	cred, err := ParseHeader("Digest whatever")
	if err == nil {
		t.Fatal("malformed header should fail to parse")
	}
	// The server would stop here; simulate that it does not call the source.
	if cred != nil {
		_, _ = spy.Authenticate(cred)
	}
	if spy.calls != 0 {
		t.Fatalf("identity source consulted %d times for a malformed header; want 0", spy.calls)
	}
}

type spySource struct{ calls int }

func (s *spySource) Authenticate(*Credential) (Identity, error) {
	s.calls++
	return Anonymous, nil
}

func TestAnonymousSource(t *testing.T) {
	id, err := AnonymousSource{}.Authenticate(nil)
	if err != nil || id != Anonymous {
		t.Fatalf("anonymous source: id=%v err=%v", id, err)
	}
	// Ignores any presented credential — auth disabled.
	id, err = AnonymousSource{}.Authenticate(&Credential{Scheme: SchemeBearer, Token: "x"})
	if err != nil || id != Anonymous {
		t.Fatalf("anonymous source should ignore credentials: id=%v err=%v", id, err)
	}
}

func TestBasicSource(t *testing.T) {
	src := NewBasicSource(map[string]string{"alice": "s3cret"})

	id, err := src.Authenticate(&Credential{Scheme: SchemeBasic, User: "alice", Secret: "s3cret"})
	if err != nil || id.ID != "alice" || id.Method != MethodBasic {
		t.Fatalf("valid basic: id=%v err=%v", id, err)
	}
	if _, err := src.Authenticate(&Credential{Scheme: SchemeBasic, User: "alice", Secret: "wrong"}); err == nil {
		t.Fatal("wrong password should fail")
	}
	if _, err := src.Authenticate(&Credential{Scheme: SchemeBasic, User: "nobody", Secret: "x"}); err == nil {
		t.Fatal("unknown user should fail")
	}
	if _, err := src.Authenticate(nil); err == nil {
		t.Fatal("missing credential should fail (401)")
	}
	if _, err := src.Authenticate(&Credential{Scheme: SchemeBearer, Token: "x"}); err == nil {
		t.Fatal("wrong scheme should fail")
	}
}

func TestBearerSource(t *testing.T) {
	src := NewBearerSource(map[string]string{"tok-123": "bob"})

	id, err := src.Authenticate(&Credential{Scheme: SchemeBearer, Token: "tok-123"})
	if err != nil || id.ID != "bob" || id.Method != MethodBearer {
		t.Fatalf("valid bearer: id=%v err=%v", id, err)
	}
	if _, err := src.Authenticate(&Credential{Scheme: SchemeBearer, Token: "nope"}); err == nil {
		t.Fatal("invalid token should fail")
	}
	if _, err := src.Authenticate(nil); err == nil {
		t.Fatal("missing credential should fail")
	}
}

func TestMultiSource(t *testing.T) {
	m := &Multi{
		Sources: []IdentitySource{
			NewBasicSource(map[string]string{"alice": "pw"}),
			NewBearerSource(map[string]string{"tok": "bob"}),
		},
		AllowAnonymous: true,
	}
	if id, err := m.Authenticate(nil); err != nil || id != Anonymous {
		t.Fatalf("nil cred with AllowAnonymous should be anonymous: %v %v", id, err)
	}
	if id, _ := m.Authenticate(&Credential{Scheme: SchemeBasic, User: "alice", Secret: "pw"}); id.ID != "alice" {
		t.Fatalf("basic via multi: %v", id)
	}
	if id, _ := m.Authenticate(&Credential{Scheme: SchemeBearer, Token: "tok"}); id.ID != "bob" {
		t.Fatalf("bearer via multi: %v", id)
	}
	if _, err := m.Authenticate(&Credential{Scheme: SchemeBearer, Token: "bad"}); err == nil {
		t.Fatal("bad credential should fail across all sources")
	}

	// Without AllowAnonymous, a missing credential is rejected.
	m2 := &Multi{Sources: m.Sources, AllowAnonymous: false}
	if _, err := m2.Authenticate(nil); err == nil {
		t.Fatal("nil cred without AllowAnonymous should fail")
	}
}
