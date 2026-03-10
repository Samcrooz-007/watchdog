package ratelimit

import (
	"sync"
	"time"
)

// Implements a simple token bucket rate limiter
type TokenBucket struct {
	mu       sync.Mutex
	tokens   int
	capacity int
	refill   int
	interval time.Duration
	lastFill time.Time
}

// Creates a rate limiter with specified capacity and refill rate capacity
func NewTokenBucket(capacity, refillPerInterval int, interval time.Duration) *TokenBucket {
	return &TokenBucket{
		tokens:   capacity,
		capacity: capacity,
		refill:   refillPerInterval,
		interval: interval,
		lastFill: time.Now(),
	}
}

// Allow checks if a request should be allowed
// Uses monotonic time via time.Since() to prevent clock skew issues
func (tb *TokenBucket) Allow() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	// Refill tokens based on elapsed time using monotonic clock
	// time.Since() uses monotonic readings when available, unaffected by wall clock changes
	elapsed := time.Since(tb.lastFill)
	if elapsed >= tb.interval {
		periods := int(elapsed / tb.interval)
		tb.tokens += periods * tb.refill
		if tb.tokens > tb.capacity {
			tb.tokens = tb.capacity
		}
		// Advance lastFill by exact periods to prevent drift
		tb.lastFill = tb.lastFill.Add(time.Duration(periods) * tb.interval)
	}

	// Check if we have tokens available
	if tb.tokens > 0 {
		tb.tokens--
		return true
	}

	return false
}
