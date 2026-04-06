package dropper

import (
	"sync"
	"time"
)

// RateLimiter tracks per-IP request rates using a sliding window.
type RateLimiter struct {
	mu      sync.Mutex
	windows map[string][]time.Time // key: IP address
	limit   int
	window  time.Duration
}

// NewRateLimiter creates a new RateLimiter with the given request limit and
// sliding window duration.
func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		windows: make(map[string][]time.Time),
		limit:   limit,
		window:  window,
	}
}

// Allow reports whether the given IP address is within the rate limit.
// Expired entries older than the window are pruned inline on every call.
func (rl *RateLimiter) Allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-rl.window)

	timestamps := rl.windows[ip]

	// Prune entries that have fallen outside the sliding window.
	valid := timestamps[:0]
	for _, t := range timestamps {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}

	if len(valid) == 0 {
		// Clean up the map entry entirely to prevent unbounded growth.
		delete(rl.windows, ip)
	}

	if len(valid) >= rl.limit {
		// Persist the pruned slice even when denying, so the next Allow call
		// does not need to re-prune the same stale entries.
		if len(valid) > 0 {
			rl.windows[ip] = valid
		}
		return false
	}

	rl.windows[ip] = append(valid, now)
	return true
}
