package api

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/yangjj-iso/skill-cloud/server/internal/auth"
)

// RateLimitConfig caps how many requests a single API key may make in a
// rolling window. Zero values disable rate-limiting.
type RateLimitConfig struct {
	RequestsPerMinute int
}

// DefaultRateLimit is the platform-wide default applied when the operator
// hasn't explicitly configured one. 60 req/min is generous enough for
// interactive agents while still rejecting obvious abuse.
var DefaultRateLimit = RateLimitConfig{RequestsPerMinute: 60}

// rateLimiter is a per-key sliding-window counter. It is intentionally
// in-memory: replacing it with Redis when we run multi-replica is a
// straight swap of this struct.
type rateLimiter struct {
	mu    sync.Mutex
	limit int
	hits  map[string][]time.Time
}

func newRateLimiter(limit int) *rateLimiter {
	return &rateLimiter{limit: limit, hits: map[string][]time.Time{}}
}

// allow records a request for key at `now`. It returns (allowed,
// remaining, resetAt) where resetAt is when the oldest in-window hit
// will fall out of the window.
func (rl *rateLimiter) allow(key string, now time.Time) (bool, int, time.Time) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	cutoff := now.Add(-time.Minute)
	window := rl.hits[key]
	// Drop entries older than the window. The slice is append-only and
	// time-ordered, so a single forward scan finds the trim point.
	keep := 0
	for keep < len(window) && window[keep].Before(cutoff) {
		keep++
	}
	window = window[keep:]

	if len(window) >= rl.limit {
		oldest := window[0]
		rl.hits[key] = window
		return false, 0, oldest.Add(time.Minute)
	}

	window = append(window, now)
	rl.hits[key] = window
	remaining := rl.limit - len(window)
	reset := window[0].Add(time.Minute)
	return true, remaining, reset
}

// rateLimitMiddleware enforces RateLimitConfig per API key (or per
// caller IP when no API key is present, i.e. in dev mode).
func rateLimitMiddleware(cfg RateLimitConfig) gin.HandlerFunc {
	if cfg.RequestsPerMinute <= 0 {
		return func(c *gin.Context) { c.Next() }
	}
	rl := newRateLimiter(cfg.RequestsPerMinute)
	return func(c *gin.Context) {
		key := rateLimitKey(c)
		allowed, remaining, reset := rl.allow(key, time.Now())
		c.Header("X-RateLimit-Limit", strconv.Itoa(cfg.RequestsPerMinute))
		c.Header("X-RateLimit-Remaining", strconv.Itoa(remaining))
		c.Header("X-RateLimit-Reset", strconv.FormatInt(reset.Unix(), 10))
		if !allowed {
			c.Header("Retry-After", strconv.Itoa(int(time.Until(reset).Seconds())+1))
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "rate limit exceeded"})
			return
		}
		c.Next()
	}
}

// rateLimitKey prefers the authenticated API key as the bucket
// identifier; it falls back to the caller's IP so unauthenticated /
// dev-mode traffic is still rate-limited per source.
func rateLimitKey(c *gin.Context) string {
	if p, ok := auth.PrincipalFromContext(c.Request.Context()); ok && p.APIKeyID != uuid.Nil {
		return "key:" + p.APIKeyID.String()
	}
	return "ip:" + callerIPFromContext(c)
}
