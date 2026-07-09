package profilecalibration

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"io"
	"strings"
	"testing"
	"time"
)

func TestDailyQuizPushStatsScansDailyCounts(t *testing.T) {
	registerAdminPushDriver()
	db, err := sql.Open(adminPushDriverName, "stats")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	stats, err := NewStore(db).DailyQuizPushStats(context.Background(), "2026-07-09")
	if err != nil {
		t.Fatalf("stats: %v", err)
	}

	if stats.Date != "2026-07-09" || !stats.Pushed {
		t.Fatalf("expected date and pushed flag, got %+v", stats)
	}
	if stats.EligibleUsers != 12 || stats.PushedUsers != 9 || stats.AnsweredUsers != 4 || stats.CompletedUsers != 2 || stats.TotalAnswers != 17 || stats.PendingReassessmentReports != 3 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
}

func TestListDailyQuizPushRecordsScansBatchRows(t *testing.T) {
	registerAdminPushDriver()
	db, err := sql.Open(adminPushDriverName, "records")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	items, total, err := NewStore(db).ListDailyQuizPushRecords(context.Background(), "2026-07-09", 1, 20)
	if err != nil {
		t.Fatalf("records: %v", err)
	}

	if total != 1 || len(items) != 1 {
		t.Fatalf("expected one record, total=%d items=%+v", total, items)
	}
	item := items[0]
	if item.BatchID != 88 || item.AppUserID != 7 || item.CardID != 123 || !item.Pushed || !item.Completed {
		t.Fatalf("unexpected record IDs/status: %+v", item)
	}
	if item.Phone != "13800000000" || item.Nickname != "测试用户" || item.CardName != "本人人格卡" {
		t.Fatalf("unexpected user/card labels: %+v", item)
	}
	if item.PushSentAt != "2026/07/09 09:00:00" || item.CompletedAt != "2026/07/09 09:05:00" || item.QuizDate != "2026-07-09" {
		t.Fatalf("unexpected formatted dates: %+v", item)
	}
}

const adminPushDriverName = "profile_calibration_admin_push_test"

var adminPushDriverRegistered bool

func registerAdminPushDriver() {
	if adminPushDriverRegistered {
		return
	}
	sql.Register(adminPushDriverName, adminPushDriver{})
	adminPushDriverRegistered = true
}

type adminPushDriver struct{}

func (adminPushDriver) Open(string) (driver.Conn, error) {
	return adminPushConn{}, nil
}

type adminPushConn struct{}

func (adminPushConn) Prepare(string) (driver.Stmt, error) { return nil, nil }
func (adminPushConn) Close() error                        { return nil }
func (adminPushConn) Begin() (driver.Tx, error)           { return nil, nil }

func (adminPushConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	switch {
	case strings.Contains(query, "eligible_users") && strings.Contains(query, "pending_reassessment_reports"):
		return &adminPushRows{
			columns: []string{"eligible_users", "pushed_users", "answered_users", "completed_users", "total_answers", "pending_reassessment_reports"},
			values:  [][]driver.Value{{int64(12), int64(9), int64(4), int64(2), int64(17), int64(3)}},
		}, nil
	case strings.Contains(query, "COUNT(*)") && strings.Contains(query, "app_daily_quiz_batches"):
		return &adminPushRows{
			columns: []string{"total"},
			values:  [][]driver.Value{{int64(1)}},
		}, nil
	case strings.Contains(query, "FROM app_daily_quiz_batches b"):
		return &adminPushRows{
			columns: []string{
				"app_user_id", "phone", "nickname", "card_id", "card_name", "quiz_date", "batch_id",
				"pushed", "push_sent_at", "answered_count", "completed", "completed_at",
			},
			values: [][]driver.Value{{
				int64(7),
				"13800000000",
				"测试用户",
				int64(123),
				"本人人格卡",
				time.Date(2026, 7, 9, 0, 0, 0, 0, time.UTC),
				int64(88),
				true,
				time.Date(2026, 7, 9, 9, 0, 0, 0, time.UTC),
				int64(5),
				true,
				time.Date(2026, 7, 9, 9, 5, 0, 0, time.UTC),
			}},
		}, nil
	default:
		return &adminPushRows{}, nil
	}
}

type adminPushRows struct {
	columns []string
	values  [][]driver.Value
	index   int
}

func (r *adminPushRows) Columns() []string { return r.columns }
func (r *adminPushRows) Close() error      { return nil }

func (r *adminPushRows) Next(dest []driver.Value) error {
	if r.index >= len(r.values) {
		return io.EOF
	}
	copy(dest, r.values[r.index])
	r.index++
	return nil
}
