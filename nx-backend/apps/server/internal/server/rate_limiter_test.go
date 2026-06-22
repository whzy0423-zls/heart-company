package server

import (
	"testing"
	"time"
)

func TestFixedWindowRateLimiterAllowsWithinLimit(t *testing.T) {
	now := time.Unix(100, 0)
	limiter := newFixedWindowRateLimiter(2, time.Minute)

	if !limiter.Allow(1, now) {
		t.Fatal("expected first request to be allowed")
	}
	if !limiter.Allow(1, now.Add(10*time.Second)) {
		t.Fatal("expected second request to be allowed")
	}
	if limiter.Allow(1, now.Add(20*time.Second)) {
		t.Fatal("expected third request in same window to be rejected")
	}
}

func TestFixedWindowRateLimiterResetsAfterWindow(t *testing.T) {
	now := time.Unix(100, 0)
	limiter := newFixedWindowRateLimiter(1, time.Minute)

	if !limiter.Allow(1, now) {
		t.Fatal("expected first request to be allowed")
	}
	if limiter.Allow(1, now.Add(30*time.Second)) {
		t.Fatal("expected request in same window to be rejected")
	}
	if !limiter.Allow(1, now.Add(time.Minute+time.Second)) {
		t.Fatal("expected request after window to be allowed")
	}
}

func TestFixedWindowRateLimiterTracksUsersSeparately(t *testing.T) {
	now := time.Unix(100, 0)
	limiter := newFixedWindowRateLimiter(1, time.Minute)

	if !limiter.Allow(1, now) {
		t.Fatal("expected user 1 first request allowed")
	}
	if !limiter.Allow(2, now) {
		t.Fatal("expected user 2 first request allowed")
	}
	if limiter.Allow(1, now) {
		t.Fatal("expected user 1 second request rejected")
	}
}
