package ratelimit

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func newTestLimiter(rate float64, burst int, now *fakeClock) *Limiter {
	l := New(rate, burst)
	l.now = now.Now
	return l
}

// fakeClock is a manually advanced clock so refill behaviour is exact.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

func TestBurstExhaustionAndRefill(t *testing.T) {
	now := &fakeClock{t: time.Unix(0, 0)}
	l := newTestLimiter(1, 3, now) // burst 3, refill 1/s

	for i := 0; i < 3; i++ {
		if !l.allow("1.2.3.4") {
			t.Fatalf("request %d of the burst must pass", i+1)
		}
	}
	if l.allow("1.2.3.4") {
		t.Fatal("4th immediate request must be denied (burst exhausted)")
	}

	now.Advance(2 * time.Second) // two tokens refilled
	if !l.allow("1.2.3.4") || !l.allow("1.2.3.4") {
		t.Fatal("refilled requests must pass")
	}
	if l.allow("1.2.3.4") {
		t.Fatal("requests beyond refill must be denied")
	}
}

func TestClientsAreIndependent(t *testing.T) {
	now := &fakeClock{t: time.Unix(0, 0)}
	l := newTestLimiter(1, 1, now)

	if !l.allow("a") {
		t.Fatal("first client must pass")
	}
	if !l.allow("b") {
		t.Fatal("second client must pass — buckets are per-IP")
	}
	if l.allow("a") || l.allow("b") {
		t.Fatal("both buckets must deplete independently")
	}
}

func TestMiddlewareReturns429WithRetryAfter(t *testing.T) {
	l := New(0.001, 1)
	handler := l.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	srv := httptest.NewServer(handler)
	defer srv.Close()

	resp1, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp1.Body.Close()
	if resp1.StatusCode != http.StatusOK {
		t.Fatalf("first request = %d, want 200", resp1.StatusCode)
	}

	resp2, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("second request = %d, want 429", resp2.StatusCode)
	}
	if resp2.Header.Get("Retry-After") == "" {
		t.Fatal("429 must carry Retry-After")
	}
}

func TestKeyPrefersFirstForwardedForHop(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "10.0.0.1:5555"
	r.Header.Set("X-Forwarded-For", "203.0.113.7, 10.0.0.9")
	if got := keyFor(r); got != "203.0.113.7" {
		t.Fatalf("key = %q, want first XFF hop", got)
	}
	r2 := httptest.NewRequest("GET", "/", nil)
	r2.RemoteAddr = "10.0.0.1:5555"
	if got := keyFor(r2); got != "10.0.0.1:5555" {
		t.Fatalf("key = %q, want RemoteAddr when no XFF", got)
	}
}

func TestBucketMapIsBounded(t *testing.T) {
	now := &fakeClock{t: time.Unix(0, 0)}
	l := newTestLimiter(1, 1, now)
	l.maxKeys = 5

	// Six fresh idle buckets: the sweep can't help (nothing stale), so the
	// sixth client is refused rather than growing the map past the cap.
	for i := 0; i < 5; i++ {
		l.allow(string(rune('a' + i)))
	}
	if l.allow("f") {
		t.Fatal("map must refuse new clients at capacity with nothing to sweep")
	}

	// Age everything out, then new clients are admitted again.
	now.Advance(2 * time.Hour)
	if !l.allow("f") {
		t.Fatal("after sweeping stale buckets a new client must be admitted")
	}
}
