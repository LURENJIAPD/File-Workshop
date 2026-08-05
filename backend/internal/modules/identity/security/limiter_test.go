package security

import (
	"testing"
	"time"
)

func TestIPLimiterRejectsBurstAboveTenRequests(t *testing.T) {
	limiter := NewIPLimiter()
	now := time.Date(2026, 8, 5, 8, 0, 0, 0, time.UTC)
	for index := 0; index < 10; index++ {
		if allowed, _ := limiter.Allow("127.0.0.1", now); !allowed {
			t.Fatalf("request %d unexpectedly rejected", index+1)
		}
	}
	if allowed, retryAfter := limiter.Allow("127.0.0.1", now); allowed || retryAfter <= 0 {
		t.Fatalf("eleventh request = allowed %v, retryAfter %v", allowed, retryAfter)
	}
}
