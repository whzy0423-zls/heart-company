package profilecalibration

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
)

func TestListDailyReminderCandidatesFiltersEligiblePrimaryCards(t *testing.T) {
	profileCalibrationQueryRecorder.reset()
	database, err := sql.Open(profileCalibrationPushDriverName, "daily-reminders")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })

	_, err = NewStore(database).ListDailyReminderCandidates(context.Background(), "2026-07-09", 100)
	if err != nil {
		t.Fatalf("list daily reminder candidates: %v", err)
	}

	query := profileCalibrationQueryRecorder.query()
	for _, want := range []string{
		"c.card_type = 'primary'",
		"c.status = 'active'",
		"u.status = 'active'",
		"c.create_time < $1::date",
		"EXISTS (SELECT 1 FROM app_device_tokens",
		"NOT EXISTS (",
		"app_reassessment_jobs",
		"status IN ('pending','generating','generated')",
		"app_daily_quiz_batches",
		"push_sent_at IS NOT NULL",
		"completed = true",
		"answered_count >= 5",
	} {
		if !strings.Contains(query, want) {
			t.Fatalf("expected daily reminder candidate query to contain %q, query:\n%s", want, query)
		}
	}
}

func TestClaimBatchPushUsesConditionalUpdateBeforeSending(t *testing.T) {
	profileCalibrationQueryRecorder.reset()
	database, err := sql.Open(profileCalibrationPushDriverName, "claim-batch")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })

	claimed, err := NewStore(database).ClaimBatchPush(context.Background(), 88)
	if err != nil {
		t.Fatalf("claim batch push: %v", err)
	}
	if !claimed {
		t.Fatal("recording driver reports one affected row, expected claimed=true")
	}
	query := profileCalibrationQueryRecorder.query()
	for _, want := range []string{"UPDATE app_daily_quiz_batches", "push_claimed_at=now()", "push_sent_at IS NULL", "push_claimed_at IS NULL"} {
		if !strings.Contains(query, want) {
			t.Fatalf("expected batch claim query to contain %q, query:\n%s", want, query)
		}
	}
}

func TestClaimReassessmentPushUsesConditionalUpdateBeforeSending(t *testing.T) {
	profileCalibrationQueryRecorder.reset()
	database, err := sql.Open(profileCalibrationPushDriverName, "claim-reassessment")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })

	claimed, err := NewStore(database).ClaimReassessmentPush(context.Background(), 66)
	if err != nil {
		t.Fatalf("claim reassessment push: %v", err)
	}
	if !claimed {
		t.Fatal("recording driver reports one affected row, expected claimed=true")
	}
	query := profileCalibrationQueryRecorder.query()
	for _, want := range []string{"UPDATE app_reassessment_jobs", "push_claimed_at=now()", "push_sent_at IS NULL", "push_claimed_at IS NULL"} {
		if !strings.Contains(query, want) {
			t.Fatalf("expected reassessment claim query to contain %q, query:\n%s", want, query)
		}
	}
}

func TestMarkBatchPushSentUpdatesDailyQuizBatch(t *testing.T) {
	profileCalibrationQueryRecorder.reset()
	database, err := sql.Open(profileCalibrationPushDriverName, "mark-batch")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })

	if err := NewStore(database).MarkBatchPushSent(context.Background(), 88); err != nil {
		t.Fatalf("mark batch push sent: %v", err)
	}

	query := profileCalibrationQueryRecorder.query()
	if !strings.Contains(query, "UPDATE app_daily_quiz_batches") || !strings.Contains(query, "push_sent_at=COALESCE(push_sent_at,now())") {
		t.Fatalf("expected batch push_sent_at update, query:\n%s", query)
	}
}

func TestListGeneratedReassessmentPushCandidatesSelectsUnpushedReports(t *testing.T) {
	profileCalibrationQueryRecorder.reset()
	database, err := sql.Open(profileCalibrationPushDriverName, "reassessment-reminders")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })

	_, err = NewStore(database).ListGeneratedReassessmentPushCandidates(context.Background(), 100)
	if err != nil {
		t.Fatalf("list reassessment push candidates: %v", err)
	}

	query := profileCalibrationQueryRecorder.query()
	for _, want := range []string{
		"FROM app_reassessment_jobs j",
		"j.status = 'generated'",
		"j.push_sent_at IS NULL",
		"EXISTS (SELECT 1 FROM app_device_tokens",
		"ORDER BY j.create_time ASC, j.id ASC",
	} {
		if !strings.Contains(query, want) {
			t.Fatalf("expected reassessment candidate query to contain %q, query:\n%s", want, query)
		}
	}
}

func TestMarkReassessmentPushSentUpdatesJob(t *testing.T) {
	profileCalibrationQueryRecorder.reset()
	database, err := sql.Open(profileCalibrationPushDriverName, "mark-reassessment")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })

	if err := NewStore(database).MarkReassessmentPushSent(context.Background(), 66); err != nil {
		t.Fatalf("mark reassessment push sent: %v", err)
	}

	query := profileCalibrationQueryRecorder.query()
	if !strings.Contains(query, "UPDATE app_reassessment_jobs") || !strings.Contains(query, "push_sent_at=COALESCE(push_sent_at,now())") {
		t.Fatalf("expected reassessment push_sent_at update, query:\n%s", query)
	}
}

func TestReassessmentTriggerEnforcesFourteenDayCooldown(t *testing.T) {
	source := readStoreSource(t)
	for _, want := range []string{
		"INTERVAL '14 days'",
		"status IN ('accepted','rejected','expired')",
		"update_time > now() - INTERVAL '14 days'",
	} {
		if !strings.Contains(source, want) {
			t.Fatalf("expected reassessment trigger to enforce 14-day cooldown with %q", want)
		}
	}
}

func readStoreSource(t *testing.T) string {
	t.Helper()
	content, err := os.ReadFile("store.go")
	if err != nil {
		t.Fatalf("read store.go: %v", err)
	}
	return string(content)
}

var profileCalibrationQueryRecorder = &profileCalibrationRecordingQuery{}

const profileCalibrationPushDriverName = "profile_calibration_push_test"

func init() {
	sql.Register(profileCalibrationPushDriverName, profileCalibrationPushDriver{})
}

type profileCalibrationRecordingQuery struct {
	mu    sync.Mutex
	value string
}

func (r *profileCalibrationRecordingQuery) reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.value = ""
}

func (r *profileCalibrationRecordingQuery) set(query string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.value = query
}

func (r *profileCalibrationRecordingQuery) query() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.value
}

type profileCalibrationPushDriver struct{}

func (profileCalibrationPushDriver) Open(string) (driver.Conn, error) {
	return profileCalibrationPushConn{}, nil
}

type profileCalibrationPushConn struct{}

func (profileCalibrationPushConn) Prepare(string) (driver.Stmt, error) { return nil, nil }
func (profileCalibrationPushConn) Close() error                        { return nil }
func (profileCalibrationPushConn) Begin() (driver.Tx, error)           { return nil, nil }

func (profileCalibrationPushConn) ExecContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Result, error) {
	profileCalibrationQueryRecorder.set(query)
	return driver.RowsAffected(1), nil
}

func (profileCalibrationPushConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	profileCalibrationQueryRecorder.set(query)
	return profileCalibrationEmptyRows{}, nil
}

type profileCalibrationEmptyRows struct{}

func (profileCalibrationEmptyRows) Columns() []string         { return nil }
func (profileCalibrationEmptyRows) Close() error              { return nil }
func (profileCalibrationEmptyRows) Next([]driver.Value) error { return io.EOF }

var _ driver.ExecerContext = profileCalibrationPushConn{}
var _ driver.QueryerContext = profileCalibrationPushConn{}
