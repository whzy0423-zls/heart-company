package server

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	appdb "nine-xing/nx-backend/apps/server/internal/db"
)

func TestAppChatQuotaDateUsesAsiaShanghai(t *testing.T) {
	instant := time.Date(2026, 9, 14, 16, 30, 0, 0, time.UTC)
	if got := appChatQuotaDate(instant); got != "2026-09-15" {
		t.Fatalf("appChatQuotaDate() = %q, want 2026-09-15", got)
	}
}

func TestNewAppChatQuotaSnapshot(t *testing.T) {
	tests := []struct {
		limit, reserved, consumed int
		want                      appChatQuotaSnapshot
	}{
		{-1, 0, 0, appChatQuotaSnapshot{Limit: -1, Remaining: -1, TotalRemaining: -1}},
		{5, 1, 2, appChatQuotaSnapshot{Limit: 5, Reserved: 1, Consumed: 2, Remaining: 2, TotalRemaining: 2}},
		{5, 3, 4, appChatQuotaSnapshot{Limit: 5, Reserved: 3, Consumed: 4, Remaining: 0}},
	}
	for _, tt := range tests {
		if got := newAppChatQuotaSnapshot(tt.limit, tt.reserved, tt.consumed); got != tt.want {
			t.Errorf("snapshot(%d,%d,%d) = %#v, want %#v", tt.limit, tt.reserved, tt.consumed, got, tt.want)
		}
	}
}

func TestAppChatQuotaSnapshotKeepsDailyAndTrialBalancesSeparate(t *testing.T) {
	got := newAppChatQuotaSnapshotWithTrial(5, 1, 4, 7, "2026-09-17T12:00:00+08:00")
	if got.Remaining != 0 {
		t.Fatalf("daily remaining = %d, want 0", got.Remaining)
	}
	if got.TrialRemaining != 7 || got.TotalRemaining != 7 {
		t.Fatalf("unexpected trial totals: %+v", got)
	}
	if got.TrialNearestExpiresAt != "2026-09-17T12:00:00+08:00" {
		t.Fatalf("unexpected nearest expiry: %+v", got)
	}
}

func TestAppChatQuotaSnapshotKeepsMemberUnlimitedWithoutSpendingTrial(t *testing.T) {
	got := newAppChatQuotaSnapshotWithTrial(-1, 0, 0, 9, "2026-09-17T12:00:00+08:00")
	if got.Remaining != -1 || got.TotalRemaining != -1 || got.TrialRemaining != 9 {
		t.Fatalf("unexpected member quota snapshot: %+v", got)
	}
}

func TestAppChatQuotaExhaustedErrorIsStable(t *testing.T) {
	if !errors.Is(errAppChatQuotaExhausted, errAppChatQuotaExhausted) {
		t.Fatal("quota exhaustion must remain comparable with errors.Is")
	}
}

func TestDatabaseAppChatQuotaConsumesDailyBeforeEarliestTrialGrant(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run chat quota integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	database, err := appdb.Open(ctx, dsn, "trial-credit-admin", "test-password-123")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	var userID int64
	phone := fmt.Sprintf("199%08d", time.Now().UnixNano()%100000000)
	if err := database.QueryRowContext(ctx, `INSERT INTO app_users(phone) VALUES($1) RETURNING id`, phone).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	defer database.ExecContext(context.Background(), `DELETE FROM app_users WHERE id=$1`, userID)

	now := time.Now()
	var firstGrantID int64
	if err := database.QueryRowContext(ctx, `INSERT INTO app_chat_trial_credit_grants
		(app_user_id,amount,remaining,reason,expires_at,idempotency_key)
		VALUES($1,2,2,'早到期',$2,$3) RETURNING id`, userID, now.Add(24*time.Hour), fmt.Sprintf("trial-first-%d", userID)).Scan(&firstGrantID); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `INSERT INTO app_chat_trial_credit_grants
		(app_user_id,amount,remaining,reason,expires_at,idempotency_key)
		VALUES($1,3,3,'晚到期',$2,$3)`, userID, now.Add(48*time.Hour), fmt.Sprintf("trial-second-%d", userID)); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `INSERT INTO app_chat_trial_credit_grants
		(app_user_id,amount,remaining,reason,expires_at,status,idempotency_key)
		VALUES($1,2,2,'已过期',$2,'active',$3),($1,2,2,'已撤销',$4,'revoked',$5)`,
		userID, now.Add(-time.Hour), fmt.Sprintf("trial-expired-%d", userID), now.Add(72*time.Hour), fmt.Sprintf("trial-revoked-%d", userID)); err != nil {
		t.Fatal(err)
	}

	manager := newDatabaseAppChatQuotaManager(database)
	dailyKey, _, err := manager.Reserve(ctx, userID, 1, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Commit(ctx, dailyKey); err != nil {
		t.Fatal(err)
	}
	trialKey, snapshot, err := manager.Reserve(ctx, userID, 1, now)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Remaining != 0 || snapshot.TrialRemaining != 4 {
		t.Fatalf("unexpected post-reserve snapshot: %+v", snapshot)
	}
	if _, err := manager.Commit(ctx, trialKey); err != nil {
		t.Fatal(err)
	}
	var remaining, reserved int
	var status string
	if err := database.QueryRowContext(ctx, `SELECT remaining,reserved,status FROM app_chat_trial_credit_grants WHERE id=$1`, firstGrantID).Scan(&remaining, &reserved, &status); err != nil {
		t.Fatal(err)
	}
	if remaining != 1 || reserved != 0 || status != "active" {
		t.Fatalf("earliest grant settlement remaining=%d reserved=%d status=%s", remaining, reserved, status)
	}

	releaseKey, _, err := manager.Reserve(ctx, userID, 1, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Release(ctx, releaseKey); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `SELECT remaining,reserved FROM app_chat_trial_credit_grants WHERE id=$1`, firstGrantID).Scan(&remaining, &reserved); err != nil {
		t.Fatal(err)
	}
	if remaining != 1 || reserved != 0 {
		t.Fatalf("released reservation changed grant remaining=%d reserved=%d", remaining, reserved)
	}

	memberKey, memberSnapshot, err := manager.Reserve(ctx, userID, -1, now)
	if err != nil || memberKey != "" || memberSnapshot.Remaining != -1 {
		t.Fatalf("member reserve should bypass credits: key=%q snapshot=%+v err=%v", memberKey, memberSnapshot, err)
	}
}

func TestDatabaseAppChatQuotaReleasesStaleTrialReservationAcrossQuotaDates(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run chat quota integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	database, err := appdb.Open(ctx, dsn, "trial-credit-admin", "test-password-123")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	var userID int64
	phone := fmt.Sprintf("197%08d", time.Now().UnixNano()%100000000)
	if err := database.QueryRowContext(ctx, `INSERT INTO app_users(phone) VALUES($1) RETURNING id`, phone).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	defer database.ExecContext(context.Background(), `DELETE FROM app_users WHERE id=$1`, userID)

	now := time.Now()
	previousDate := appChatQuotaDate(now.Add(-24 * time.Hour))
	if _, err := database.ExecContext(ctx, `INSERT INTO app_chat_daily_quotas(app_user_id,quota_date,quota_limit)
		VALUES($1,$2::date,0)`, userID, previousDate); err != nil {
		t.Fatal(err)
	}
	var grantID int64
	if err := database.QueryRowContext(ctx, `INSERT INTO app_chat_trial_credit_grants
		(app_user_id,amount,remaining,reserved,reason,expires_at,idempotency_key)
		VALUES($1,1,1,1,'跨天超时预留',$2,$3) RETURNING id`, userID, now.Add(24*time.Hour), fmt.Sprintf("trial-stale-%d", userID)).Scan(&grantID); err != nil {
		t.Fatal(err)
	}
	staleKey := fmt.Sprintf("stale-trial-%d", userID)
	if _, err := database.ExecContext(ctx, `INSERT INTO app_chat_quota_reservations
		(reservation_key,app_user_id,quota_date,source,trial_grant_id,status,create_time)
		VALUES($1,$2,$3::date,'trial',$4,'reserved',now()-interval '11 minutes')`, staleKey, userID, previousDate, grantID); err != nil {
		t.Fatal(err)
	}

	manager := newDatabaseAppChatQuotaManager(database)
	key, _, err := manager.Reserve(ctx, userID, 0, now)
	if err != nil {
		t.Fatalf("reserve after cross-date stale reservation: %v", err)
	}
	if key == "" {
		t.Fatal("expected released trial credit to become reservable")
	}
	var staleStatus string
	if err := database.QueryRowContext(ctx, `SELECT status FROM app_chat_quota_reservations WHERE reservation_key=$1`, staleKey).Scan(&staleStatus); err != nil {
		t.Fatal(err)
	}
	if staleStatus != "released" {
		t.Fatalf("stale reservation status=%q, want released", staleStatus)
	}
	if _, err := manager.Release(ctx, key); err != nil {
		t.Fatal(err)
	}
	var remaining, reserved int
	if err := database.QueryRowContext(ctx, `SELECT remaining,reserved FROM app_chat_trial_credit_grants WHERE id=$1`, grantID).Scan(&remaining, &reserved); err != nil {
		t.Fatal(err)
	}
	if remaining != 1 || reserved != 0 {
		t.Fatalf("released cross-date grant remaining=%d reserved=%d", remaining, reserved)
	}
}

type recordingAppChatQuotaManager struct {
	mu           sync.Mutex
	reserveErr   error
	reserveCalls int
	commitCalls  int
	releaseCalls int
	consumed     int
	released     int
	trial        int
	trialExpiry  string
	committed    map[string]bool
}

func (m *recordingAppChatQuotaManager) Reserve(_ context.Context, _ int64, limit int, _ time.Time) (string, appChatQuotaSnapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.reserveCalls++
	if m.reserveErr != nil {
		return "", newAppChatQuotaSnapshot(limit, 0, 0), m.reserveErr
	}
	if m.committed == nil {
		m.committed = make(map[string]bool)
	}
	key := fmt.Sprintf("quota-%d", m.reserveCalls)
	return key, newAppChatQuotaSnapshot(limit, 1, m.consumed), nil
}

func (m *recordingAppChatQuotaManager) Commit(_ context.Context, key string) (appChatQuotaSnapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.commitCalls++
	if !m.committed[key] {
		m.committed[key] = true
		m.consumed++
	}
	return newAppChatQuotaSnapshot(5, 0, m.consumed), nil
}

func (m *recordingAppChatQuotaManager) Release(_ context.Context, key string) (appChatQuotaSnapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.releaseCalls++
	if !m.committed[key] {
		m.released++
		m.committed[key] = true
	}
	return newAppChatQuotaSnapshot(5, 0, m.consumed), nil
}

func (m *recordingAppChatQuotaManager) Snapshot(_ context.Context, _ int64, limit int, _ time.Time) (appChatQuotaSnapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return newAppChatQuotaSnapshotWithTrial(limit, 0, m.consumed, m.trial, m.trialExpiry), nil
}

func (m *recordingAppChatQuotaManager) counts() (reserve, commit, release, consumed, released int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.reserveCalls, m.commitCalls, m.releaseCalls, m.consumed, m.released
}
