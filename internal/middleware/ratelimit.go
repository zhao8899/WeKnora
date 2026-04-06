package middleware

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

const (
	// Default API rate limit: 100 requests per 60-second sliding window per tenant.
	defaultAPIRateWindow      = 60 * time.Second
	defaultAPIRateMaxRequests = 100
)

// rateLimitLuaScript is a sliding-window rate limiter implemented as an atomic
// Lua script on a Redis Sorted Set. Identical logic to internal/im/ratelimit.go.
var rateLimitLuaScript = redis.NewScript(`
local key     = KEYS[1]
local now     = tonumber(ARGV[1])
local window  = tonumber(ARGV[2])
local maxReq  = tonumber(ARGV[3])
local member  = ARGV[4]

redis.call('ZREMRANGEBYSCORE', key, 0, now - window)
local count = redis.call('ZCARD', key)
if count < maxReq then
    redis.call('ZADD', key, now, member)
    redis.call('PEXPIRE', key, window + 1000)
    return 1
end
return 0
`)

// APIRateLimit returns a Gin middleware that applies per-tenant sliding-window
// rate limiting to all API requests. Uses Redis when available, local fallback
// when not.
func APIRateLimit(redisClient *redis.Client) gin.HandlerFunc {
	window := defaultAPIRateWindow
	maxReq := defaultAPIRateMaxRequests

	hostname, _ := os.Hostname()
	instanceID := fmt.Sprintf("%s-%d", hostname, os.Getpid())

	local := &localLimiter{
		entries:    make(map[string][]time.Time),
		window:     window,
		maxReq:     maxReq,
		lastClean:  time.Now(),
		cleanEvery: 2 * time.Minute,
	}

	return func(c *gin.Context) {
		// Build rate-limit key: prefer tenant ID, fall back to client IP
		key := "api_rl:" + c.ClientIP()
		if tenantID, ok := c.Get("tenantID"); ok {
			key = fmt.Sprintf("api_rl:t:%v", tenantID)
		}

		allowed := false
		if redisClient != nil {
			nowMs := time.Now().UnixMilli()
			member := fmt.Sprintf("%s:%d", instanceID, nowMs)
			result, err := rateLimitLuaScript.Run(
				context.Background(), redisClient,
				[]string{key},
				nowMs, window.Milliseconds(), maxReq, member,
			).Int()
			if err == nil {
				allowed = result == 1
			} else {
				// Redis unavailable — fall back to local
				allowed = local.allow(key)
			}
		} else {
			allowed = local.allow(key)
		}

		if !allowed {
			c.Header("Retry-After", "60")
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error":   "rate_limit_exceeded",
				"message": "Too many requests. Please try again later.",
			})
			return
		}

		c.Next()
	}
}

// localLimiter is a simple in-memory sliding-window rate limiter used when
// Redis is unavailable.
type localLimiter struct {
	mu         sync.Mutex
	entries    map[string][]time.Time
	window     time.Duration
	maxReq     int
	lastClean  time.Time
	cleanEvery time.Duration
}

func (l *localLimiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()

	// Periodic cleanup of expired keys
	if now.Sub(l.lastClean) > l.cleanEvery {
		cutoff := now.Add(-l.window)
		for k, ts := range l.entries {
			var valid []time.Time
			for _, t := range ts {
				if t.After(cutoff) {
					valid = append(valid, t)
				}
			}
			if len(valid) == 0 {
				delete(l.entries, k)
			} else {
				l.entries[k] = valid
			}
		}
		l.lastClean = now
	}

	cutoff := now.Add(-l.window)
	ts := l.entries[key]

	// Prune expired entries
	start := 0
	for start < len(ts) && !ts[start].After(cutoff) {
		start++
	}
	ts = ts[start:]

	if len(ts) >= l.maxReq {
		l.entries[key] = ts
		return false
	}

	l.entries[key] = append(ts, now)
	return true
}
