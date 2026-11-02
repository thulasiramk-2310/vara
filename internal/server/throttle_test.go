package server

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time          { return c.t }
func (c *fakeClock) advance(d time.Duration) { c.t = c.t.Add(d) }

func TestLimiterLocksAfterTheFreeFailuresAndDoublesTheWait(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1_000_000, 0)}
	l := newFailureLimiter(3, 10*time.Second, 80*time.Second, time.Hour, clk.now)
	for i := 0; i < 2; i++ {
		l.fail("k")
		if w := l.wait("k"); w != 0 {
			t.Fatalf("failure %d must still be free, got a wait of %v", i+1, w)
		}
	}
	l.fail("k") // the third
	if w := l.wait("k"); w != 10*time.Second {
		t.Fatalf("after the free failures want a 10s wait, got %v", w)
	}
	clk.advance(10 * time.Second)
	if w := l.wait("k"); w != 0 {
		t.Fatalf("the lock must expire, got %v", w)
	}
	for _, want := range []time.Duration{20 * time.Second, 40 * time.Second, 80 * time.Second, 80 * time.Second} {
		l.fail("k")
		if w := l.wait("k"); w != want {
			t.Fatalf("want a wait of %v, got %v", want, w)
		}
		clk.advance(want)
	}
}

func TestLimiterKeysAreIndependentAndSuccessClears(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1_000_000, 0)}
	l := newFailureLimiter(2, 10*time.Second, time.Minute, time.Hour, clk.now)
	l.fail("a")
	l.fail("a")
	if l.wait("a") == 0 {
		t.Fatal("a should be locked")
	}
	if l.wait("b") != 0 {
		t.Fatal("b must be unaffected by a")
	}
	l.clear("a")
	if l.wait("a") != 0 {
		t.Fatal("a success must clear the lock")
	}
}

func TestLimiterForgetsOldFailures(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1_000_000, 0)}
	l := newFailureLimiter(3, 10*time.Second, time.Minute, time.Hour, clk.now)
	l.fail("k")
	l.fail("k")
	clk.advance(2 * time.Hour)
	l.fail("k") // counts as the first again, not the third
	if w := l.wait("k"); w != 0 {
		t.Fatalf("failures from two hours ago must not count, got a wait of %v", w)
	}
}

func login(t *testing.T, base, user, pass string) *http.Response {
	t.Helper()
	body := fmt.Sprintf(`{"username":%q,"password":%q}`, user, pass)
	resp, err := http.Post(base+"/_vara/sessions", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp
}

// Guessing a password must get slower, not stay free.
func TestLoginIsThrottledAfterRepeatedFailures(t *testing.T) {
	ts, mgr, _ := newAccountHub(t)
	_ = mgr.CreateAccount("alice", "password1")
	for i := 0; i < 5; i++ {
		if st := login(t, ts.URL, "alice", "wrong").StatusCode; st != http.StatusUnauthorized {
			t.Fatalf("failure %d = %d, want 401", i+1, st)
		}
	}
	resp := login(t, ts.URL, "alice", "password1") // even the right password waits
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("after 5 failures want 429, got %d", resp.StatusCode)
	}
	if n, err := strconv.Atoi(resp.Header.Get("Retry-After")); err != nil || n <= 0 {
		t.Fatalf("a throttled response must say when to retry, got %q", resp.Header.Get("Retry-After"))
	}
}

func TestAThrottledAccountDoesNotLockOutOthers(t *testing.T) {
	ts, mgr, _ := newAccountHub(t)
	_ = mgr.CreateAccount("alice", "password1")
	_ = mgr.CreateAccount("bob", "password2")
	for i := 0; i < 6; i++ {
		login(t, ts.URL, "alice", "wrong")
	}
	if st := login(t, ts.URL, "bob", "password2").StatusCode; st != http.StatusCreated {
		t.Fatalf("bob must still log in, got %d", st)
	}
}

func TestASuccessfulLoginClearsTheFailureCount(t *testing.T) {
	ts, mgr, _ := newAccountHub(t)
	_ = mgr.CreateAccount("alice", "password1")
	for round := 0; round < 3; round++ {
		for i := 0; i < 4; i++ {
			login(t, ts.URL, "alice", "wrong")
		}
		if st := login(t, ts.URL, "alice", "password1").StatusCode; st != http.StatusCreated {
			t.Fatalf("round %d: a real user who mistyped a few times must get in, got %d", round, st)
		}
	}
}

// One address trying many different names is guessing too.
func TestPasswordSprayingFromOneAddressIsThrottled(t *testing.T) {
	ts, mgr, _ := newAccountHub(t)
	_ = mgr.CreateAccount("fresh", "password9")
	for i := 0; i < 25; i++ {
		login(t, ts.URL, fmt.Sprintf("user%d", i), "wrong")
	}
	if st := login(t, ts.URL, "fresh", "password9").StatusCode; st != http.StatusTooManyRequests {
		t.Fatalf("after spraying 25 names from one address want 429 for a new name, got %d", st)
	}
}

func TestBasicAuthGuessingIsThrottledToo(t *testing.T) {
	ts, mgr, _ := newAccountHub(t)
	_ = mgr.CreateAccount("alice", "password1")
	get := func(pass string) int {
		req, _ := http.NewRequest(http.MethodGet, ts.URL+"/nosuch/info/refs", nil)
		req.SetBasicAuth("alice", pass)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	for i := 0; i < 5; i++ {
		if st := get("wrong"); st != http.StatusUnauthorized {
			t.Fatalf("failure %d = %d, want 401", i+1, st)
		}
	}
	if st := get("password1"); st != http.StatusTooManyRequests {
		t.Fatalf("Basic guessing must be throttled like login, got %d", st)
	}
}
