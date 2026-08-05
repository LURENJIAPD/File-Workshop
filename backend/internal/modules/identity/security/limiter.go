package security

import (
	"sync"
	"time"

	"golang.org/x/time/rate"
)

type ipEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

type IPLimiter struct {
	mu      sync.Mutex
	entries map[string]*ipEntry
	rate    rate.Limit
	burst   int
	ttl     time.Duration
}

func NewIPLimiter() *IPLimiter {
	return &IPLimiter{
		entries: make(map[string]*ipEntry),
		rate:    rate.Every(6 * time.Second),
		burst:   10,
		ttl:     time.Hour,
	}
}

func (l *IPLimiter) Allow(key string, now time.Time) (bool, time.Duration) {
	if key == "" {
		key = "unknown"
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	entry := l.entries[key]
	if entry == nil {
		entry = &ipEntry{limiter: rate.NewLimiter(l.rate, l.burst)}
		l.entries[key] = entry
	}
	entry.lastSeen = now
	allowed := entry.limiter.AllowN(now, 1)
	if len(l.entries) > 1024 {
		for entryKey, candidate := range l.entries {
			if now.Sub(candidate.lastSeen) > l.ttl {
				delete(l.entries, entryKey)
			}
		}
	}
	if allowed {
		return true, 0
	}
	return false, 6 * time.Second
}
