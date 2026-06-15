package httpapi

import (
	"sync"
	"time"
)

// rateLimiter is a simple fixed-window limiter keyed by a string (e.g. client
// IP), used to throttle login attempts.
type rateLimiter struct {
	mu       sync.Mutex
	hits     map[string]*window
	limit    int
	interval time.Duration
}

type window struct {
	count int
	reset time.Time
}

func newRateLimiter(limit int, interval time.Duration) *rateLimiter {
	return &rateLimiter{hits: map[string]*window{}, limit: limit, interval: interval}
}

// Allow reports whether an action for key is within the limit, recording it.
func (rl *rateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	now := time.Now()
	w := rl.hits[key]
	if w == nil || now.After(w.reset) {
		rl.hits[key] = &window{count: 1, reset: now.Add(rl.interval)}
		return true
	}
	if w.count >= rl.limit {
		return false
	}
	w.count++
	return true
}
