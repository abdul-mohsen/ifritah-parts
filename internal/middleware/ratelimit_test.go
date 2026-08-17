package middleware

import (
	"testing"
	"time"
)

// TestRateLimiter_AllowsUnderLimit verifies requests within quota pass.
func TestRateLimiter_AllowsUnderLimit(t *testing.T) {
	rl := NewRateLimiter(60, 5) // 1 req/sec, burst 5
	defer rl.Stop()
	for i := 0; i < 5; i++ {
		if !rl.Allow("client1") {
			t.Fatalf("request %d rejected before burst exhausted", i+1)
		}
	}
}

// TestRateLimiter_BlocksAtLimit verifies the burst cap is enforced.
func TestRateLimiter_BlocksAtLimit(t *testing.T) {
	rl := NewRateLimiter(60, 3) // burst=3
	defer rl.Stop()
	for i := 0; i < 3; i++ {
		rl.Allow("clientA") // exhaust burst
	}
	if rl.Allow("clientA") {
		t.Error("expected 4th request to be blocked after burst exhausted")
	}
}

// TestRateLimiter_RefillsOverTime verifies tokens accumulate after a wait.
func TestRateLimiter_RefillsOverTime(t *testing.T) {
	rl := NewRateLimiter(60, 1) // 1 req/sec, burst=1
	defer rl.Stop()

	rl.Allow("clientB") // exhaust burst
	if rl.Allow("clientB") {
		t.Fatal("second immediate request should be blocked")
	}

	// Wait >1 second for 1 token to refill
	time.Sleep(1100 * time.Millisecond)
	if !rl.Allow("clientB") {
		t.Error("request after 1s refill should be allowed")
	}
}

// TestRateLimiter_IsolatesClients verifies different IPs have independent buckets.
func TestRateLimiter_IsolatesClients(t *testing.T) {
	rl := NewRateLimiter(60, 2) // burst=2
	defer rl.Stop()

	// Exhaust client1
	rl.Allow("client1")
	rl.Allow("client1")
	if rl.Allow("client1") {
		t.Error("client1 should be blocked")
	}

	// client2 should still have its own fresh bucket
	if !rl.Allow("client2") {
		t.Error("client2 should be allowed (independent bucket)")
	}
}
