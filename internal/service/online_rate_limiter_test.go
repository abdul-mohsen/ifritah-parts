package service

import (
	"context"
	"testing"
	"time"
)

func TestOnlineRateLimiter_ZeroIntervalIsNoOp(t *testing.T) {
	l := NewOnlineRateLimiter(0)
	start := time.Now()
	for i := 0; i < 3; i++ {
		if err := l.Wait(context.Background()); err != nil {
			t.Fatalf("Wait err: %v", err)
		}
	}
	if elapsed := time.Since(start); elapsed > 20*time.Millisecond {
		t.Errorf("zero-interval limiter should not sleep; took %v", elapsed)
	}
}

func TestOnlineRateLimiter_EnforcesInterval(t *testing.T) {
	interval := 40 * time.Millisecond
	l := NewOnlineRateLimiter(interval)
	// First call: no wait (last is zero-time).
	start := time.Now()
	if err := l.Wait(context.Background()); err != nil {
		t.Fatalf("first Wait err: %v", err)
	}
	// Second call immediately: must sleep ~interval.
	if err := l.Wait(context.Background()); err != nil {
		t.Fatalf("second Wait err: %v", err)
	}
	elapsed := time.Since(start)
	if elapsed < interval-5*time.Millisecond {
		t.Errorf("expected >= %v elapsed between two calls, got %v", interval, elapsed)
	}
	// Sanity: shouldn't take more than 3x interval (allows for jitter).
	if elapsed > 3*interval {
		t.Errorf("expected < %v elapsed, got %v", 3*interval, elapsed)
	}
}

func TestOnlineRateLimiter_ContextCancelReturnsErr(t *testing.T) {
	l := NewOnlineRateLimiter(500 * time.Millisecond)
	// Prime: first call is free.
	_ = l.Wait(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	err := l.Wait(ctx)
	if err == nil {
		t.Fatalf("expected ctx.Err(), got nil")
	}
	if err != context.DeadlineExceeded && err != context.Canceled {
		t.Fatalf("expected context error, got %v", err)
	}
}

func TestOnlineRateLimiter_NilReceiverSafe(t *testing.T) {
	var l *OnlineRateLimiter // nil
	if err := l.Wait(context.Background()); err != nil {
		t.Errorf("nil limiter Wait should be no-op, got %v", err)
	}
}
