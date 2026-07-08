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

// bucketIdleTTL is how long a bucket may sit unused before it is eligible for
// eviction. Keeps the buckets map bounded under a high cardinality of keys
// (rotating API keys, many client IPs).
const bucketIdleTTL = 10 * time.Minute

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

	// Opportunistic eviction: clear a few stale buckets every so often so the
	// map does not grow without bound under high-cardinality keys.
	if len(l.buckets) > 256 {
		l.evictStaleLocked(now)
	}

	if b.tokens >= 1 {
		b.tokens -= 1
		return true
	}
	return false
}

// evictStaleLocked removes buckets that have been idle longer than
// bucketIdleTTL. Caller must hold l.mu.
func (l *Limiter) evictStaleLocked(now time.Time) {
	cutoff := now.Add(-bucketIdleTTL)
	for k, b := range l.buckets {
		if b.last.Before(cutoff) {
			delete(l.buckets, k)
		}
	}
}
