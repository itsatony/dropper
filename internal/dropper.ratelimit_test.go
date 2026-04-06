package dropper

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRateLimiter_AllowUnderLimit verifies that all requests are allowed when
// the count stays at or below the configured limit.
func TestRateLimiter_AllowUnderLimit(t *testing.T) {
	rl := NewRateLimiter(DefaultRateLimitLogin, RateLimitWindow)
	ip := "192.0.2.1"

	for i := range DefaultRateLimitLogin {
		require.True(t, rl.Allow(ip), "call %d should be allowed", i+1)
	}
}

// TestRateLimiter_BlockOverLimit verifies that the (limit+1)th request within
// the window is denied.
func TestRateLimiter_BlockOverLimit(t *testing.T) {
	rl := NewRateLimiter(DefaultRateLimitLogin, RateLimitWindow)
	ip := "192.0.2.2"

	for i := range DefaultRateLimitLogin {
		assert.True(t, rl.Allow(ip), "call %d should be allowed", i+1)
	}

	assert.False(t, rl.Allow(ip), "call %d should be denied", DefaultRateLimitLogin+1)
}

// TestRateLimiter_DifferentIPsIndependent verifies that rate limits for
// distinct IP addresses are tracked independently.
func TestRateLimiter_DifferentIPsIndependent(t *testing.T) {
	rl := NewRateLimiter(DefaultRateLimitLogin, RateLimitWindow)
	ipA := "1.2.3.4"
	ipB := "5.6.7.8"

	for i := range DefaultRateLimitLogin {
		assert.True(t, rl.Allow(ipA), "ipA call %d should be allowed", i+1)
		assert.True(t, rl.Allow(ipB), "ipB call %d should be allowed", i+1)
	}
}

// TestRateLimiter_WindowExpiry verifies that requests made after the window
// has fully elapsed are permitted again.
func TestRateLimiter_WindowExpiry(t *testing.T) {
	const limit = 3
	const window = 50 * time.Millisecond

	rl := NewRateLimiter(limit, window)
	ip := "192.0.2.3"

	for i := range limit {
		require.True(t, rl.Allow(ip), "call %d should be allowed", i+1)
	}

	assert.False(t, rl.Allow(ip), "call %d should be denied (at limit)", limit+1)

	time.Sleep(100 * time.Millisecond)

	assert.True(t, rl.Allow(ip), "call after window expiry should be allowed")
}

// TestRateLimiter_SlidingWindow verifies the sliding-window semantics: only
// entries within the trailing window count toward the limit.
func TestRateLimiter_SlidingWindow(t *testing.T) {
	const limit = 5
	const window = 100 * time.Millisecond

	rl := NewRateLimiter(limit, window)
	ip := "192.0.2.4"

	// First batch: 3 calls at t≈0.
	for i := range 3 {
		require.True(t, rl.Allow(ip), "first-batch call %d should be allowed", i+1)
	}

	// Sleep so the first batch is now 60 ms old (still within the 100 ms window).
	time.Sleep(60 * time.Millisecond)

	// Second batch: 2 more calls (total in-window = 5 — at the limit).
	for i := range 2 {
		require.True(t, rl.Allow(ip), "second-batch call %d should be allowed", i+1)
	}

	// Sixth call must be denied — window still holds all 5 entries.
	assert.False(t, rl.Allow(ip), "sixth call should be denied (window full)")

	// Sleep another 50 ms: total elapsed ≈ 110 ms. The first-batch entries
	// (at t≈0) are now older than 100 ms and fall outside the window. Only the
	// second-batch entries (at t≈60 ms, now ≈50 ms old) remain — count = 2.
	time.Sleep(50 * time.Millisecond)

	assert.True(t, rl.Allow(ip), "call after first batch expires should be allowed")
}

// TestRateLimiter_Concurrent verifies thread safety under concurrent load and
// that exactly `limit` requests are allowed when many goroutines race.
func TestRateLimiter_Concurrent(t *testing.T) {
	const limit = 5
	const goroutines = 50

	rl := NewRateLimiter(limit, RateLimitWindow)
	ip := "192.0.2.5"

	var allowed atomic.Int64
	var wg sync.WaitGroup

	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			if rl.Allow(ip) {
				allowed.Add(1)
			}
		}()
	}

	wg.Wait()

	assert.Equal(t, int64(limit), allowed.Load(),
		"exactly %d requests should be allowed under concurrent load", limit)
}

// TestRateLimiter_CleanupEmptyIP verifies that entries for an IP are fully
// cleaned up after the window expires, allowing the IP to start fresh.
func TestRateLimiter_CleanupEmptyIP(t *testing.T) {
	const limit = 2
	const window = 50 * time.Millisecond

	rl := NewRateLimiter(limit, window)
	ip := "192.0.2.6"

	require.True(t, rl.Allow(ip), "first call should be allowed")
	require.True(t, rl.Allow(ip), "second call should be allowed")

	// Let the window expire so all stored timestamps become stale.
	time.Sleep(100 * time.Millisecond)

	// A new call should succeed: stale entries are pruned and the map key is
	// deleted, so the IP is treated as fresh.
	assert.True(t, rl.Allow(ip), "call after window expiry should be allowed (entries cleaned up)")

	rl.mu.Lock()
	_, exists := rl.windows[ip]
	rl.mu.Unlock()

	// After the successful call the map entry should exist again (one entry),
	// not be deleted. The important invariant is that the previous stale entries
	// did not prevent the new call from succeeding.
	assert.True(t, exists, "map entry should exist after a successful post-expiry call")
}
