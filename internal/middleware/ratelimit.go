// Package middleware provides Gin middleware for the parts-engine server.
package middleware

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// bucket is a per-client token bucket.
type bucket struct {
	tokens    float64
	lastRefil time.Time
	mu        sync.Mutex
}

// RateLimiter is a per-IP token-bucket rate limiter.
// It uses only the standard library — no external dependency.
//
// Each unique client IP gets an independent bucket.
// Buckets are lazily evicted when they have been idle for cleanupAfter.
type RateLimiter struct {
	rate         float64       // tokens per second
	burst        float64       // maximum tokens (burst capacity)
	cleanupAfter time.Duration // idle duration before eviction
	clients      sync.Map      // IP → *bucket
	stop         chan struct{}
}

// NewRateLimiter creates a limiter with:
//   - ratePerMin requests per minute sustained rate
//   - burst maximum burst of requests
//
// The cleanup goroutine runs every minute to evict idle buckets.
func NewRateLimiter(ratePerMin, burst int) *RateLimiter {
	rl := &RateLimiter{
		rate:         float64(ratePerMin) / 60.0,
		burst:        float64(burst),
		cleanupAfter: 5 * time.Minute,
		stop:         make(chan struct{}),
	}
	go rl.cleanupLoop()
	return rl
}

// Stop terminates the background cleanup goroutine.
func (rl *RateLimiter) Stop() {
	close(rl.stop)
}

// Allow returns true when the client represented by key may proceed.
func (rl *RateLimiter) Allow(key string) bool {
	now := time.Now()
	raw, _ := rl.clients.LoadOrStore(key, &bucket{
		tokens:    rl.burst,
		lastRefil: now,
	})
	b := raw.(*bucket)

	b.mu.Lock()
	defer b.mu.Unlock()

	elapsed := now.Sub(b.lastRefil).Seconds()
	b.tokens += elapsed * rl.rate
	if b.tokens > rl.burst {
		b.tokens = rl.burst
	}
	b.lastRefil = now

	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// Middleware returns a Gin handler that enforces the rate limit per client IP.
// On exceed it returns 429 with a JSON body containing the retry_after hint.
func (rl *RateLimiter) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		if !rl.Allow(ip) {
			retryAfter := int(1.0/rl.rate) + 1
			c.Header("Retry-After", fmt.Sprintf("%d", retryAfter))
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error":       "rate limit exceeded",
				"hint":        "Search is limited to avoid overloading the parts database. Please slow down.",
				"retry_after": retryAfter,
			})
			return
		}
		c.Next()
	}
}

func (rl *RateLimiter) cleanupLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-rl.stop:
			return
		case now := <-ticker.C:
			rl.clients.Range(func(key, val interface{}) bool {
				b := val.(*bucket)
				b.mu.Lock()
				idle := now.Sub(b.lastRefil)
				b.mu.Unlock()
				if idle > rl.cleanupAfter {
					rl.clients.Delete(key)
				}
				return true
			})
		}
	}
}
