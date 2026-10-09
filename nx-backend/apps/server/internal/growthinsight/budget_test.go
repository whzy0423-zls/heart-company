package growthinsight

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"
)

func budgetStore(t *testing.T, db *sql.DB, daily, user int) *Store {
	t.Helper()
	s, err := NewStoreWithBudgetLimits(db, BudgetLimits{DailyAttempts: daily, UserDailyAttempts: user})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestBudgetDefaultsAndInvalidConfiguration(t *testing.T) {
	if NewStore(nil).budgetLimits != (BudgetLimits{DailyAttempts: 100, UserDailyAttempts: 3}) {
		t.Fatal("unsafe budget defaults")
	}
	for _, limits := range []BudgetLimits{{}, {DailyAttempts: 0, UserDailyAttempts: 3}, {DailyAttempts: 100, UserDailyAttempts: 0}, {DailyAttempts: -1, UserDailyAttempts: 3}} {
		if _, err := NewStoreWithBudgetLimits(nil, limits); !errors.Is(err, ErrInvalid) {
			t.Fatalf("invalid budget accepted %+v: %v", limits, err)
		}
	}
}

func TestDailyBudgetCountsFailuresAndManualRetries(t *testing.T) {
	_, db, now := fixture(t)
	s := budgetStore(t, db, 100, 2)
	ctx := context.Background()
	if _, err := s.SetConsent(ctx, 1, true); err != nil {
		t.Fatal(err)
	}
	calls := 0
	boom := errors.New("provider fixture failure")
	complete := func(context.Context, string) (string, error) { calls++; return "", boom }
	for i := 0; i < 2; i++ {
		at := now.Add(time.Duration(i) * time.Minute)
		if err := s.enqueue(ctx, 1, at, true); err != nil {
			t.Fatal(err)
		}
		if err := s.Tick(ctx, complete, at); !errors.Is(err, boom) {
			t.Fatalf("expected attempt failure: %v", err)
		}
	}
	if err := s.enqueue(ctx, 1, now.Add(2*time.Minute), true); err != nil {
		t.Fatal(err)
	}
	if err := s.Tick(ctx, complete, now.Add(2*time.Minute)); !errors.Is(err, ErrBudgetExhausted) {
		t.Fatalf("budget bypassed: %v", err)
	}
	if calls != 2 {
		t.Fatalf("model calls=%d", calls)
	}
	var count, attempts int
	var due time.Time
	var code, status string
	if err := db.QueryRow(`SELECT count(*) FROM app_growth_insight_attempts WHERE app_user_id=1`).Scan(&count); err != nil || count != 2 {
		t.Fatalf("attempt ledger=%d err=%v", count, err)
	}
	if err := db.QueryRow(`SELECT due_at,error_code,status,attempts FROM app_growth_insight_jobs WHERE app_user_id=1`).Scan(&due, &code, &status, &attempts); err != nil {
		t.Fatal(err)
	}
	if !due.Equal(time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)) || code != "user_daily_budget_exhausted" || status != "pending" || attempts != 0 {
		t.Fatalf("budget deferral due=%s code=%s status=%s attempts=%d", due, code, status, attempts)
	}
	if err := s.enqueue(ctx, 1, now.Add(3*time.Minute), true); err != nil {
		t.Fatal(err)
	}
	if err := s.Tick(ctx, complete, now.Add(3*time.Minute)); err != nil || calls != 2 {
		t.Fatalf("admin retry reset deferral: calls=%d err=%v", calls, err)
	}
	if err := db.QueryRow(`SELECT error_code FROM app_growth_insight_jobs WHERE app_user_id=1`).Scan(&code); err != nil || code != "user_daily_budget_exhausted" {
		t.Fatalf("lost budget waiting reason: %s %v", code, err)
	}
}

func TestBudgetSurvivesConsentWithdrawalAndResetsAtUTCMidnight(t *testing.T) {
	_, db, now := fixture(t)
	s := budgetStore(t, db, 100, 1)
	ctx := context.Background()
	if _, err := s.SetConsent(ctx, 1, true); err != nil {
		t.Fatal(err)
	}
	calls := 0
	boom := errors.New("fixture failure")
	complete := func(context.Context, string) (string, error) { calls++; return "", boom }
	if err := s.Tick(ctx, complete, now); !errors.Is(err, boom) {
		t.Fatal(err)
	}
	if _, err := s.SetConsent(ctx, 1, false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetConsent(ctx, 1, true); err != nil {
		t.Fatal(err)
	}
	if err := s.Tick(ctx, complete, now.Add(time.Hour)); !errors.Is(err, ErrBudgetExhausted) || calls != 1 {
		t.Fatalf("withdrawal refunded attempt: %d %v", calls, err)
	}
	midnight := time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)
	if err := s.Tick(ctx, complete, midnight.Add(-time.Microsecond)); err != nil || calls != 1 {
		t.Fatalf("budget reset before UTC midnight: %d %v", calls, err)
	}
	if err := s.Tick(ctx, complete, midnight); !errors.Is(err, boom) || calls != 2 {
		t.Fatalf("UTC day did not reset budget: %d %v", calls, err)
	}
}

func budgetJobs(t *testing.T, s *Store, db *sql.DB, now time.Time) (job, job) {
	t.Helper()
	ctx := context.Background()
	if _, err := db.Exec(`INSERT INTO app_chat_messages(session_id,role,content,create_time) VALUES(2,'user','Another owner statement',$1)`, now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{1, 2} {
		if _, err := s.SetConsent(ctx, id, true); err != nil {
			t.Fatal(err)
		}
		if err := s.enqueue(ctx, id, now, false); err != nil {
			t.Fatal(err)
		}
	}
	j1, err := s.claim(ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	j2, err := s.claim(ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	return j1, j2
}

func TestGlobalBudgetReservationIsAtomicAcrossStores(t *testing.T) {
	_, db, now := fixture(t)
	a := budgetStore(t, db, 1, 3)
	b := budgetStore(t, db, 1, 3)
	ctx := context.Background()
	j1, j2 := budgetJobs(t, a, db, now)
	errs := make(chan error, 2)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i, j := range []job{j1, j2} {
		wg.Add(1)
		go func(i int, j job) {
			defer wg.Done()
			<-start
			s := a
			if i == 1 {
				s = b
			}
			errs <- s.reserveAttempt(ctx, j, now)
		}(i, j)
	}
	close(start)
	wg.Wait()
	close(errs)
	succeeded, exhausted := 0, 0
	for err := range errs {
		if err == nil {
			succeeded++
		} else if errors.Is(err, ErrBudgetExhausted) {
			exhausted++
		} else {
			t.Fatal(err)
		}
	}
	if succeeded != 1 || exhausted != 1 {
		t.Fatalf("global race success=%d exhausted=%d", succeeded, exhausted)
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM app_growth_insight_attempts`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("global overspend: %d %v", count, err)
	}
}

func TestBudgetReservationIsIdempotentAndRequiresLiveConsent(t *testing.T) {
	_, db, now := fixture(t)
	s := budgetStore(t, db, 1, 1)
	ctx := context.Background()
	j, _ := budgetJobs(t, s, db, now)
	for i := 0; i < 2; i++ {
		if err := s.reserveAttempt(ctx, j, now); err != nil {
			t.Fatalf("idempotent reservation: %v", err)
		}
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM app_growth_insight_attempts`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("double charge=%d %v", count, err)
	}
	if _, err := s.SetConsent(ctx, j.userID, false); err != nil {
		t.Fatal(err)
	}
	if err := s.reserveAttempt(ctx, j, now); !errors.Is(err, ErrStale) {
		t.Fatalf("opted-out reservation accepted: %v", err)
	}
}

func TestAccountDeletionDoesNotRefundGlobalBudget(t *testing.T) {
	_, db, now := fixture(t)
	s := budgetStore(t, db, 1, 3)
	ctx := context.Background()
	j1, j2 := budgetJobs(t, s, db, now)
	if err := s.reserveAttempt(ctx, j1, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM app_users WHERE id=$1`, j1.userID); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM app_growth_insight_attempts WHERE app_user_id IS NULL`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("global ledger erased on deletion: %d %v", count, err)
	}
	if err := s.reserveAttempt(ctx, j2, now); !errors.Is(err, ErrBudgetExhausted) {
		t.Fatalf("account deletion refunded budget: %v", err)
	}
}

func TestBudgetSoftAccountDeletionRemovesOwnerWithoutRefundingGlobalBudget(t *testing.T) {
	_, db, now := fixture(t)
	s := budgetStore(t, db, 1, 3)
	ctx := context.Background()
	j1, j2 := budgetJobs(t, s, db, now)
	if err := s.reserveAttempt(ctx, j1, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`ALTER TABLE app_users ADD COLUMN phone TEXT NOT NULL DEFAULT ''`); err != nil {
		t.Fatal(err)
	}
	// The existing account deletion endpoint anonymizes the phone and disables the row.
	if _, err := db.Exec(`UPDATE app_users SET status='disabled',phone='deleted-'||id::text||'-fixture' WHERE id=$1`, j1.userID); err != nil {
		t.Fatal(err)
	}
	var anonymous, linked int
	if err := db.QueryRow(`SELECT count(*) FILTER(WHERE app_user_id IS NULL),count(*) FILTER(WHERE app_user_id=$1) FROM app_growth_insight_attempts`, j1.userID).Scan(&anonymous, &linked); err != nil || anonymous != 1 || linked != 0 {
		t.Fatalf("soft deletion kept budget owner: anonymous=%d linked=%d err=%v", anonymous, linked, err)
	}
	if err := s.reserveAttempt(ctx, j2, now); !errors.Is(err, ErrBudgetExhausted) {
		t.Fatalf("soft account deletion refunded global budget: %v", err)
	}
}

func TestBudgetOrdinaryAccountDisableKeepsUserDailyCharge(t *testing.T) {
	_, db, now := fixture(t)
	s := budgetStore(t, db, 100, 1)
	ctx := context.Background()
	j, _ := budgetJobs(t, s, db, now)
	if err := s.reserveAttempt(ctx, j, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE app_users SET status='disabled' WHERE id=$1`, j.userID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE app_users SET status='active' WHERE id=$1`, j.userID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetConsent(ctx, j.userID, true); err != nil {
		t.Fatal(err)
	}
	if err := s.enqueue(ctx, j.userID, now, true); err != nil {
		t.Fatal(err)
	}
	j, err := s.claim(ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.reserveAttempt(ctx, j, now); !errors.Is(err, ErrBudgetExhausted) {
		t.Fatalf("ordinary disable refunded user budget: %v", err)
	}
}

func TestBudgetCleanupRetainsSevenDaysAndFutureRows(t *testing.T) {
	_, db, now := fixture(t)
	s := budgetStore(t, db, 100, 3)
	ctx := context.Background()
	j, _ := budgetJobs(t, s, db, now)
	for _, day := range []string{"2026-10-02", "2026-10-03", "2026-10-09", "2026-10-10"} {
		if _, err := db.Exec(`INSERT INTO app_growth_insight_attempts(claim_token,budget_day,reserved_at) VALUES($1,$2,$3)`, day, day, now); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.reserveAttempt(ctx, j, now); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM app_growth_insight_attempts WHERE claim_token='2026-10-02'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("expired ledger retained=%d %v", count, err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM app_growth_insight_attempts WHERE claim_token IN ('2026-10-03','2026-10-09','2026-10-10')`).Scan(&count); err != nil || count != 3 {
		t.Fatalf("current/future ledger deleted=%d %v", count, err)
	}
}

func TestInsufficientEvidenceDoesNotSpendModelBudget(t *testing.T) {
	_, db, now := fixture(t)
	s := budgetStore(t, db, 1, 1)
	ctx := context.Background()
	if _, err := db.Exec(`DELETE FROM app_chat_messages WHERE session_id=1 AND role='user'`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetConsent(ctx, 1, true); err != nil {
		t.Fatal(err)
	}
	if err := s.Tick(ctx, func(context.Context, string) (string, error) { t.Fatal("unexpected model attempt"); return "", nil }, now); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM app_growth_insight_attempts`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("insufficient input spent budget=%d %v", count, err)
	}
}

func TestBudgetReservationChargesDayAfterElapsedMidnight(t *testing.T) {
	_, db, _ := fixture(t)
	s := budgetStore(t, db, 1, 1)
	ctx := context.Background()
	now := time.Date(2026, 10, 9, 23, 59, 0, 0, time.UTC)
	j, _ := budgetJobs(t, s, db, now)
	j.claimedAt = time.Now().Add(-2 * time.Minute)
	if _, err := db.Exec(`INSERT INTO app_growth_insight_attempts(claim_token,budget_day,reserved_at) VALUES('previous-day','2026-10-09',$1)`, now); err != nil {
		t.Fatal(err)
	}
	if err := s.reserveAttempt(ctx, j, now); err != nil {
		t.Fatalf("elapsed midnight still charged previous day: %v", err)
	}
	var day string
	var reserved time.Time
	if err := db.QueryRow(`SELECT budget_day::text,reserved_at FROM app_growth_insight_attempts WHERE claim_token=$1`, j.token).Scan(&day, &reserved); err != nil {
		t.Fatal(err)
	}
	if day != "2026-10-10" || reserved.Before(time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("reservation used stale clock: %s %s", day, reserved)
	}
}

func TestBudgetCleanupRunsWithoutPendingWork(t *testing.T) {
	s, db, now := fixture(t)
	if _, err := db.Exec(`INSERT INTO app_growth_insight_attempts(claim_token,budget_day,reserved_at) VALUES('expired','2026-10-02',$1),('current','2026-10-09',$1),('future','2026-10-10',$1)`, now); err != nil {
		t.Fatal(err)
	}
	if err := s.Tick(context.Background(), func(context.Context, string) (string, error) { t.Fatal("unexpected model call"); return "", nil }, now); err != nil {
		t.Fatal(err)
	}
	var expired, retained int
	if err := db.QueryRow(`SELECT count(*) FILTER(WHERE claim_token='expired'),count(*) FILTER(WHERE claim_token IN ('current','future')) FROM app_growth_insight_attempts`).Scan(&expired, &retained); err != nil || expired != 0 || retained != 2 {
		t.Fatalf("idle cleanup expired=%d retained=%d err=%v", expired, retained, err)
	}
}

func TestBudgetCountsProviderTimeoutAndKeepsPublishedReport(t *testing.T) {
	_, db, now := fixture(t)
	s := budgetStore(t, db, 100, 1)
	ctx := context.Background()
	first := enableAndRun(t, s, now)
	if _, err := db.Exec(`INSERT INTO app_chat_messages(session_id,role,content,create_time) VALUES(1,'user','A new attempt',$1)`, now.Add(24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	calls := 0
	complete := func(context.Context, string) (string, error) { calls++; return "", context.DeadlineExceeded }
	if err := s.Tick(ctx, complete, now.Add(25*time.Hour)); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("provider timeout missing: %v", err)
	}
	if err := s.enqueue(ctx, 1, now.Add(26*time.Hour), true); err != nil {
		t.Fatal(err)
	}
	if err := s.Tick(ctx, complete, now.Add(26*time.Hour)); !errors.Is(err, ErrBudgetExhausted) || calls != 1 {
		t.Fatalf("timeout was refunded: calls=%d err=%v", calls, err)
	}
	view, err := s.View(ctx, 1, 1)
	if err != nil || view.ReportID != first.ReportID || len(view.Actions) != len(first.Actions) {
		t.Fatalf("budget exhaustion replaced published report: %+v %v", view, err)
	}
}
