package api

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRateLimiterAllowsUnderLimit(t *testing.T) {
	rl := newRateLimiter(3)
	now := time.Unix(1_700_000_000, 0)
	for i := 0; i < 3; i++ {
		allowed, _, _ := rl.allow("k", now.Add(time.Duration(i)*time.Second))
		assert.True(t, allowed, "request %d under limit must pass", i)
	}
	denied, remaining, _ := rl.allow("k", now.Add(3*time.Second))
	assert.False(t, denied)
	assert.Equal(t, 0, remaining)
}

func TestRateLimiterSweepDropsExpiredKeys(t *testing.T) {
	// Repro of the leak: in a long-running server every distinct
	// caller (key, IP) ever observed leaves an entry in the map.
	// After sweeping, only keys with hits inside the rolling window
	// remain.
	rl := newRateLimiter(60)
	old := time.Unix(1_700_000_000, 0)
	for i := 0; i < 500; i++ {
		rl.allow("key:"+string(rune('a'+i%26))+string(rune('0'+i%10)), old.Add(time.Duration(i)*time.Millisecond))
	}
	assert.Greater(t, rl.size(), 0, "limiter should retain keys before sweep")

	// Advance time well past the 1-minute window and sweep.
	rl.sweep(old.Add(2 * time.Minute))
	assert.Equal(t, 0, rl.size(), "sweep must reclaim every expired key")
}

func TestRateLimiterTrimmingPreservesActiveKeys(t *testing.T) {
	rl := newRateLimiter(10)
	now := time.Unix(1_700_000_000, 0)
	// One key in the window, one with stale hits only.
	rl.allow("active", now)
	rl.allow("stale", now.Add(-90*time.Second))

	rl.sweep(now)

	assert.Equal(t, 1, rl.size(), "active key kept, stale key dropped")
	allowed, _, _ := rl.allow("active", now.Add(time.Second))
	assert.True(t, allowed)
}
