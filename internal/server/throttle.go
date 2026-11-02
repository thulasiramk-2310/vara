package server

import (
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/thulasiramk-2310/vara/internal/protocol"
)

// failureLimiter slows password guessing. It counts consecutive failures per key; once a key has
// had `free` of them it is refused for a wait that starts at `base` and doubles with every further
// failure, up to `max`. A success clears the key, and failures older than `forget` stop counting.
//
// State is in memory only: a restart forgives everyone, which keeps this off the disk and is fine
// for slowing guessing down. It is not a substitute for strong passwords.
type failureLimiter struct {
	mu                sync.Mutex
	now               func() time.Time
	free              int
	base, max, forget time.Duration
	entries           map[string]*failure
	lastSweep         time.Time
}

type failure struct {
	count int
	last  time.Time
	until time.Time
}

func newFailureLimiter(free int, base, max, forget time.Duration, now func() time.Time) *failureLimiter {
	if now == nil {
		now = time.Now
	}
	return &failureLimiter{now: now, free: free, base: base, max: max, forget: forget, entries: map[string]*failure{}}
}

// wait reports how long key must still wait; zero means the attempt may go ahead.
func (l *failureLimiter) wait(key string) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	f := l.entries[key]
	if f == nil {
		return 0
	}
	if d := f.until.Sub(l.now()); d > 0 {
		return d
	}
	return 0
}

// fail records one failure for key.
func (l *failureLimiter) fail(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.sweep(now)
	f := l.entries[key]
	if f == nil || now.Sub(f.last) > l.forget {
		f = &failure{}
		l.entries[key] = f
	}
	f.count++
	f.last = now
	if f.count >= l.free {
		d := l.base
		for i := l.free; i < f.count && d < l.max; i++ {
			d *= 2
		}
		if d > l.max {
			d = l.max
		}
		f.until = now.Add(d)
	}
}

// clear forgets key (after a successful login).
func (l *failureLimiter) clear(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.entries, key)
}

// sweep drops entries nobody has touched for `forget`, at most once a minute, so a stream of
// random names cannot grow the table without bound. Callers hold l.mu.
func (l *failureLimiter) sweep(now time.Time) {
	if now.Sub(l.lastSweep) < time.Minute {
		return
	}
	l.lastSweep = now
	for k, f := range l.entries {
		if now.Sub(f.last) > l.forget {
			delete(l.entries, k)
		}
	}
}

// guessGuard is the server's pair of limiters: one per (address, account) so a person who mistypes
// is slowed alone, and one per address so trying many different accounts from one place is slowed
// too. Addresses are the connection's own; X-Forwarded-For is not trusted, because anyone can send it.
type guessGuard struct {
	pair *failureLimiter
	addr *failureLimiter
}

func newGuessGuard() *guessGuard {
	return &guessGuard{
		pair: newFailureLimiter(5, 30*time.Second, 15*time.Minute, time.Hour, nil),
		addr: newFailureLimiter(20, 30*time.Second, 15*time.Minute, time.Hour, nil),
	}
}

func clientAddr(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func pairKey(r *http.Request, user string) string {
	return clientAddr(r) + "|" + strings.ToLower(user)
}

// throttled answers 429 with a Retry-After when this address or this (address, account) pair has
// to wait, and reports whether it did.
func (g *guessGuard) throttled(w http.ResponseWriter, r *http.Request, user string) bool {
	wait := g.pair.wait(pairKey(r, user))
	if a := g.addr.wait(clientAddr(r)); a > wait {
		wait = a
	}
	if wait <= 0 {
		return false
	}
	secs := int((wait + time.Second - 1) / time.Second)
	w.Header().Set("Retry-After", strconv.Itoa(secs))
	writeError(w, http.StatusTooManyRequests, protocol.CodeRateLimited,
		"too many failed sign-in attempts; try again in "+strconv.Itoa(secs)+" seconds")
	return true
}

func (g *guessGuard) failed(r *http.Request, user string) {
	g.pair.fail(pairKey(r, user))
	g.addr.fail(clientAddr(r))
}

func (g *guessGuard) succeeded(r *http.Request, user string) {
	g.pair.clear(pairKey(r, user))
}
