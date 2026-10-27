package httpx

import (
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// RateLimiter is a per-key token bucket with idle-entry eviction.
type RateLimiter struct {
	mu      sync.Mutex
	limit   rate.Limit
	burst   int
	entries map[string]*limiterEntry
}

type limiterEntry struct {
	lim  *rate.Limiter
	seen time.Time
}

func NewRateLimiter(every time.Duration, burst int) *RateLimiter {
	return &RateLimiter{
		limit:   rate.Every(every),
		burst:   burst,
		entries: map[string]*limiterEntry{},
	}
}

func (l *RateLimiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	e, ok := l.entries[key]
	if !ok {
		e = &limiterEntry{lim: rate.NewLimiter(l.limit, l.burst)}
		l.entries[key] = e
	}
	e.seen = time.Now()
	return e.lim.Allow()
}

// Sweep drops keys idle for longer than maxIdle.
func (l *RateLimiter) Sweep(maxIdle time.Duration) {
	cutoff := time.Now().Add(-maxIdle)
	l.mu.Lock()
	defer l.mu.Unlock()
	for k, e := range l.entries {
		if e.seen.Before(cutoff) {
			delete(l.entries, k)
		}
	}
}
