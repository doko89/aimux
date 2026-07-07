package middleware

import (
	"sync"
	"time"
)

// Limiter is an in-memory token-bucket rate limiter keyed by client identity.
type Limiter struct {
	rpm     float64
	burst   float64
	mu      sync.Mutex
	buckets map[string]*bucket
}

type bucket struct {
	tokens float64
	last   time.Time
}

// NewLimiter creates a limiter allowing rpm requests per minute with the given
// burst capacity.
func NewLimiter(rpm, burst int) *Limiter {
	b := float64(burst)
	if b <= 0 {
		b = float64(rpm)
	}
	return &Limiter{
		rpm:     float64(rpm),
		burst:   b,
		buckets: make(map[string]*bucket),
	}
}

// Allow reports whether a request from the given key is permitted.
func (l *Limiter) Allow(key string) bool {
	if l.rpm <= 0 {
		return true
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	b, ok := l.buckets[key]
	if !ok {
		b = &bucket{tokens: l.burst, last: now}
		l.buckets[key] = b
	}

	elapsed := now.Sub(b.last).Seconds()
	refill := (l.rpm / 60.0) * elapsed
	b.tokens += refill
	if b.tokens > l.burst {
		b.tokens = l.burst
	}
	b.last = now

	if b.tokens >= 1 {
		b.tokens -= 1
		return true
	}
	return false
}
