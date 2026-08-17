package server

import (
	"context"
	"database/sql"
	"time"
)

type dbRateLimiter struct {
	db     *sql.DB
	scope  string
	limit  int
	window time.Duration
}

func newDBRateLimiter(db *sql.DB, scope string, limit int, window time.Duration) *dbRateLimiter {
	if limit <= 0 {
		limit = 1
	}
	if window <= 0 {
		window = time.Minute
	}
	return &dbRateLimiter{db: db, scope: scope, limit: limit, window: window}
}

func (l *dbRateLimiter) Allow(ctx context.Context, key string, now time.Time) bool {
	allowed, err := l.allow(ctx, key, now)
	return err == nil && allowed
}

func (l *dbRateLimiter) allow(ctx context.Context, key string, now time.Time) (bool, error) {
	if l == nil || l.db == nil || key == "" {
		return true, nil
	}
	expires := now.Add(l.window)
	var count int
	err := l.db.QueryRowContext(ctx, `
		WITH expired AS (
		  DELETE FROM request_rate_limits
		  WHERE scope=$1 AND key<>$2 AND expires_at <= $4
		)
		INSERT INTO request_rate_limits(scope, key, count, expires_at, update_time)
		VALUES($1,$2,1,$3,$4)
		ON CONFLICT(scope, key) DO UPDATE SET
		  count = CASE WHEN request_rate_limits.expires_at <= $4 THEN 1 ELSE request_rate_limits.count + 1 END,
		  expires_at = CASE WHEN request_rate_limits.expires_at <= $4 THEN EXCLUDED.expires_at ELSE request_rate_limits.expires_at END,
		  update_time = $4
		RETURNING count`, l.scope, key, expires, now).Scan(&count)
	if err != nil {
		return false, err
	}
	return count <= l.limit, nil
}
