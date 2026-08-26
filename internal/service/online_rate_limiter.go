package service

// M8 per-source rate limiter using a shared token bucket.
//
// Each OnlineSource declares its minimum interval via RateLimit(). The
// dispatcher constructs one OnlineRateLimiter per source name and calls
// Wait() before each outbound request. This lets us:
//
//   * Cap eBay Motors calls at their 5,000/day free-tier limit
//   * Send at most 1 request per 2 seconds to schema.org public sites
//   * Never exceed 1 request per 5 seconds to smaller reference sites
//   * Prevent one over-eager source from starving the others by holding
//     the ctx budget hostage — Wait() respects ctx.Done()
//
// This is a POLL-based limiter, not a leaky bucket: each Wait() blocks
// until at least `interval` has elapsed since the previous Wait() for the
// same source. Simpler than a golang.org/x/time/rate token bucket, and we
// don't need burst capacity here because every online lookup is
// user-triggered (not scheduled).

import (
	"context"
	"sync"
	"time"
)

// OnlineRateLimiter enforces a minimum interval between Wait() calls per
// source name. Safe for concurrent use.
type OnlineRateLimiter struct {
	interval time.Duration
	mu       sync.Mutex
	last     time.Time
}

// NewOnlineRateLimiter returns a limiter that enforces `interval` between
// consecutive Wait() returns. An interval of 0 disables rate-limiting
// entirely (Wait returns immediately without touching state).
func NewOnlineRateLimiter(interval time.Duration) *OnlineRateLimiter {
	return &OnlineRateLimiter{interval: interval}
}

// Wait blocks until either the required interval has elapsed since the
// last Wait() return or ctx is cancelled. Returns ctx.Err() on cancel;
// otherwise nil.
func (l *OnlineRateLimiter) Wait(ctx context.Context) error {
	if l == nil || l.interval <= 0 {
		return nil
	}

	l.mu.Lock()
	elapsed := time.Since(l.last)
	remaining := l.interval - elapsed
	if remaining <= 0 {
		l.last = time.Now()
		l.mu.Unlock()
		return nil
	}
	l.mu.Unlock()

	// Sleep with ctx-cancellation support.
	timer := time.NewTimer(remaining)
	defer timer.Stop()
	select {
	case <-timer.C:
		l.mu.Lock()
		l.last = time.Now()
		l.mu.Unlock()
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
