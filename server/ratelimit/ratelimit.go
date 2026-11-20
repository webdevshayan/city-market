// Package ratelimit protects public endpoints with a per-IP token bucket.
// Buckets live in a bounded, janitor-swept map. On Vercel this limiter is
// per instance (instances are ephemeral), which blunts abuse but is not a
// global guarantee — a durable cross-instance limit needs shared state and
// arrives with the database/queue phase if traffic demands it.
package ratelimit

import (
	"math"
	"net/http"
	"strings"
	"sync"
	"time"
)

type bucket struct {
	tokens   float64
	lastSeen time.Time
}

// Limiter is a token-bucket rate limiter keyed by client IP.
type Limiter struct {
	mu      sync.Mutex
	rate    float64 // tokens added per second
	burst   float64 // bucket capacity (and initial fill)
	buckets map[string]*bucket
	maxKeys int
	now     func() time.Time // injectable clock for tests
}

// New returns a limiter allowing bursts up to burst requests, refilled at
// ratePerSec requests per second, per client IP.
func New(ratePerSec float64, burst int) *Limiter {
	return &Limiter{
		rate:    ratePerSec,
		burst:   float64(burst),
		buckets: make(map[string]*bucket),
		maxKeys: 10_000,
		now:     time.Now,
	}
}

// keyFor prefers the first X-Forwarded-For hop (Vercel fronts TLS, so the
// socket peer is the edge), falling back to the remote address.
func keyFor(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.IndexByte(xff, ','); i >= 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	return r.RemoteAddr
}

// Middleware returns 429 with Retry-After while the caller's bucket is empty.
func (l *Limiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !l.allow(keyFor(r)) {
			w.Header().Set("Retry-After", "1")
			http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (l *Limiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	b, ok := l.buckets[key]
	if !ok {
		if len(l.buckets) >= l.maxKeys {
			l.sweepLocked(now)
			if len(l.buckets) >= l.maxKeys {
				return false // refuse brand-new clients only when truly full
			}
		}
		b = &bucket{tokens: l.burst, lastSeen: now}
		l.buckets[key] = b
	}
	elapsed := now.Sub(b.lastSeen).Seconds()
	if elapsed > 0 {
		b.tokens = math.Min(l.burst, b.tokens+elapsed*l.rate)
	}
	b.lastSeen = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// sweepLocked drops buckets idle for more than an hour, bounding memory.
func (l *Limiter) sweepLocked(now time.Time) {
	cutoff := now.Add(-time.Hour)
	for k, b := range l.buckets {
		if b.lastSeen.Before(cutoff) {
			delete(l.buckets, k)
		}
	}
}
