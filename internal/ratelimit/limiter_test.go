package ratelimit

import (
	"testing"
	"time"
)

func TestTokenBucket_Allow(t *testing.T) {
	// Create bucket with 10 tokens, refill 10 per second
	tb := NewTokenBucket(10, 10, time.Second)

	// Should allow first 10 requests
	for i := 0; i < 10; i++ {
		if !tb.Allow() {
			t.Errorf("request %d should be allowed", i)
		}
	}

	// 11th request should be denied (no tokens left)
	if tb.Allow() {
		t.Error("request 11 should be denied (bucket empty)")
	}
}

func TestTokenBucket_Refill(t *testing.T) {
	// Create bucket with 5 tokens, refill 5 per 100ms
	tb := NewTokenBucket(5, 5, 100*time.Millisecond)

	// Consume all tokens
	for range 5 {
		tb.Allow()
	}

	// Should be denied
	if tb.Allow() {
		t.Error("should be denied before refill")
	}

	// Wait for refill
	time.Sleep(150 * time.Millisecond)

	// Should be allowed again
	if !tb.Allow() {
		t.Error("should be allowed after refill")
	}
}

func TestTokenBucket_NoDrift(t *testing.T) {
	// Test that lastFill advances by exact periods with no drift
	tb := NewTokenBucket(50, 10, 100*time.Millisecond)

	// Consume most tokens to avoid hitting capacity
	for range 45 {
		tb.Allow()
	}

	// Wait for refill and establish baseline
	time.Sleep(120 * time.Millisecond)
	tb.Allow() // Triggers refill

	// Record baseline
	tb.mu.Lock()
	baseline := tb.lastFill
	tb.mu.Unlock()

	// Wait for exactly 10 refill periods (1 second)
	time.Sleep(time.Second + 10*time.Millisecond)

	// Trigger refill calculation
	tb.Allow()

	// Verify lastFill advanced by exactly 10 periods
	tb.mu.Lock()
	expectedFill := baseline.Add(10 * 100 * time.Millisecond)
	actualFill := tb.lastFill
	tb.mu.Unlock()

	// Time should match exactly (within 1ms for timing jitter)
	drift := actualFill.Sub(expectedFill)
	if drift < 0 {
		drift = -drift
	}
	if drift > time.Millisecond {
		t.Errorf(
			"time drift detected: expected %v, got %v (drift: %v)",
			expectedFill,
			actualFill,
			drift,
		)
	}
}

func TestTokenBucket_MultipleRefills(t *testing.T) {
	// Create bucket with 10 tokens, refill 5 per 50ms
	tb := NewTokenBucket(10, 5, 50*time.Millisecond)

	// Consume all tokens
	for range 10 {
		tb.Allow()
	}

	// Wait for 2 refill periods (should add 10 tokens)
	time.Sleep(120 * time.Millisecond)

	// Should be able to consume 10 tokens (capped at capacity)
	allowed := 0
	for range 15 {
		if tb.Allow() {
			allowed++
		}
	}

	if allowed != 10 {
		t.Errorf("expected 10 tokens after refill, got %d", allowed)
	}
}
