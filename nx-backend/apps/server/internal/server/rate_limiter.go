package server

import (
	"sync"
	"time"
)

type fixedWindowRateLimiter struct {
	limit  int
	mu     sync.Mutex
	users  map[int64]rateWindow
	window time.Duration
}

type rateWindow struct {
	count     int
	expiresAt time.Time
}

func newFixedWindowRateLimiter(limit int, window time.Duration) *fixedWindowRateLimiter {
	if limit <= 0 {
		limit = 12
	}
	if window <= 0 {
		window = time.Minute
	}
	return &fixedWindowRateLimiter{
		limit:  limit,
		users:  map[int64]rateWindow{},
		window: window,
	}
}

func (l *fixedWindowRateLimiter) Allow(userID int64, now time.Time) bool {
	if l == nil || userID <= 0 {
		return true
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	current := l.users[userID]
	if current.expiresAt.IsZero() || !now.Before(current.expiresAt) {
		l.users[userID] = rateWindow{count: 1, expiresAt: now.Add(l.window)}
		l.pruneLocked(now)
		return true
	}
	if current.count >= l.limit {
		return false
	}
	current.count++
	l.users[userID] = current
	return true
}

func (l *fixedWindowRateLimiter) pruneLocked(now time.Time) {
	for userID, item := range l.users {
		if !item.expiresAt.IsZero() && !now.Before(item.expiresAt) {
			delete(l.users, userID)
		}
	}
}

type strRateLimiter struct {
	limit, maxKeys int
	mu             sync.Mutex
	keys           map[string]rateWindow
	window         time.Duration
	nextPrune      time.Time
}

func newStrRateLimiter(limit int, window time.Duration) *strRateLimiter {
	return newBoundedStrRateLimiter(limit, window, 10_000)
}
func newBoundedStrRateLimiter(limit int, window time.Duration, maxKeys int) *strRateLimiter {
	if limit <= 0 {
		limit = 1
	}
	if window <= 0 {
		window = time.Minute
	}
	if maxKeys <= 0 {
		maxKeys = 10_000
	}
	return &strRateLimiter{
		limit:   limit,
		maxKeys: maxKeys,
		keys:    map[string]rateWindow{},
		window:  window,
	}
}

func (l *strRateLimiter) Allow(key string, now time.Time) bool {
	if l == nil || key == "" {
		return true
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	current := l.keys[key]
	if current.expiresAt.IsZero() || !now.Before(current.expiresAt) {
		if current.expiresAt.IsZero() && (l.nextPrune.IsZero() || !now.Before(l.nextPrune)) {
			l.pruneStrLocked(now)
			l.nextPrune = now.Add(min(l.window, time.Minute))
		}
		if current.expiresAt.IsZero() && len(l.keys) >= l.maxKeys {
			return false
		}
		l.keys[key] = rateWindow{count: 1, expiresAt: now.Add(l.window)}
		return true
	}
	if current.count >= l.limit {
		return false
	}
	current.count++
	l.keys[key] = current
	return true
}

func (l *strRateLimiter) pruneStrLocked(now time.Time) {
	for key, item := range l.keys {
		if !item.expiresAt.IsZero() && !now.Before(item.expiresAt) {
			delete(l.keys, key)
		}
	}
}
