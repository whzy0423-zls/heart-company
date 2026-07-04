package appuser

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestListInsightsReturnsAggregatedUserData(t *testing.T) {
	var seenQueries []string
	var seenArgs [][]driver.NamedValue
	database := openAppUserInsightsTestDB(t, func(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
		seenQueries = append(seenQueries, query)
		seenArgs = append(seenArgs, args)
		if strings.Contains(query, "SELECT count(*) FROM app_users") {
			return &appUserInsightsRows{
				columns: []string{"count"},
				values:  [][]driver.Value{{int64(1)}},
			}, nil
		}
		return &appUserInsightsRows{
			columns: []string{
				"id",
				"phone",
				"nickname",
				"avatar",
				"status",
				"member_level",
				"register_source",
				"last_login_at",
				"create_time",
				"update_time",
				"primary_type",
				"second_type",
				"wing_type",
				"gender",
				"latest_quiz_time",
				"profile",
				"score",
				"centers",
				"card_count",
				"memory_count",
				"latest_memory",
				"session_count",
				"message_count",
				"latest_chat_time",
				"compatibility_count",
				"latest_compatibility_summary",
			},
			values: [][]driver.Value{{
				int64(42),
				"13800000021",
				"测试客户",
				"",
				"active",
				"vip",
				"app_sms",
				time.Unix(100, 0),
				time.Unix(200, 0),
				time.Unix(300, 0),
				int64(5),
				int64(6),
				int64(4),
				"female",
				time.Unix(400, 0),
				[]byte(`{"summary":"理性且敏锐","traits":["观察"]}`),
				[]byte(`{"5":18,"6":12}`),
				[]byte(`[{"name":"脑","score":30}]`),
				int64(2),
				int64(3),
				"用户曾问：如何处理压力？",
				int64(4),
				int64(12),
				time.Unix(500, 0),
				int64(1),
				"彼此需要更多明确沟通",
			}},
		}, nil
	})

	result, err := NewStore(database).ListInsights(context.Background(), map[string]string{
		"keyword":     "测试",
		"memberLevel": "vip",
		"page":        "1",
		"pageSize":    "20",
		"status":      "active",
	})
	if err != nil {
		t.Fatalf("list insights: %v", err)
	}

	if result.Total != 1 || len(result.Items) != 1 {
		t.Fatalf("unexpected result page: %+v", result)
	}
	item := result.Items[0]
	if item.ID != 42 || item.Phone != "13800000021" || item.PrimaryType != 5 || item.SecondType != 6 || item.WingType != 4 {
		t.Fatalf("unexpected insight item: %+v", item)
	}
	if string(item.Profile) != `{"summary":"理性且敏锐","traits":["观察"]}` {
		t.Fatalf("unexpected profile json: %s", item.Profile)
	}
	if item.MemoryCount != 3 || item.SessionCount != 4 || item.MessageCount != 12 || item.CompatibilityCount != 1 {
		t.Fatalf("unexpected aggregate counts: %+v", item)
	}
	if item.LatestMemory != "用户曾问：如何处理压力？" || item.LatestCompatibilitySummary != "彼此需要更多明确沟通" {
		t.Fatalf("unexpected latest summaries: %+v", item)
	}

	if len(seenQueries) != 2 {
		t.Fatalf("expected count and list queries, got %d", len(seenQueries))
	}
	if !strings.Contains(seenQueries[1], "app_user_cards") ||
		!strings.Contains(seenQueries[1], "app_memories") ||
		!strings.Contains(seenQueries[1], "app_chat_messages") ||
		!strings.Contains(seenQueries[1], "app_compatibility_reports") {
		t.Fatalf("expected insights query to aggregate extracted app data, got %s", seenQueries[1])
	}
	if len(seenArgs[0]) != 3 || seenArgs[0][0].Value != "%测试%" || seenArgs[0][1].Value != "active" || seenArgs[0][2].Value != "vip" {
		t.Fatalf("unexpected filter args: %+v", seenArgs[0])
	}
}

var appUserInsightsDriverSeq atomic.Int64

func openAppUserInsightsTestDB(
	t *testing.T,
	query func(context.Context, string, []driver.NamedValue) (driver.Rows, error),
) *sql.DB {
	t.Helper()
	driverName := fmt.Sprintf("app_user_insights_test_%d", appUserInsightsDriverSeq.Add(1))
	sql.Register(driverName, appUserInsightsDriver{query: query})
	database, err := sql.Open(driverName, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database
}

type appUserInsightsDriver struct {
	query func(context.Context, string, []driver.NamedValue) (driver.Rows, error)
}

func (d appUserInsightsDriver) Open(string) (driver.Conn, error) {
	return appUserInsightsConn{query: d.query}, nil
}

type appUserInsightsConn struct {
	query func(context.Context, string, []driver.NamedValue) (driver.Rows, error)
}

func (appUserInsightsConn) Prepare(string) (driver.Stmt, error) { return nil, driver.ErrSkip }
func (appUserInsightsConn) Close() error                        { return nil }
func (appUserInsightsConn) Begin() (driver.Tx, error)           { return nil, driver.ErrSkip }

func (c appUserInsightsConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	return c.query(ctx, query, args)
}

type appUserInsightsRows struct {
	columns []string
	index   int
	values  [][]driver.Value
}

func (r *appUserInsightsRows) Columns() []string {
	return r.columns
}

func (r *appUserInsightsRows) Close() error {
	return nil
}

func (r *appUserInsightsRows) Next(dest []driver.Value) error {
	if r.index >= len(r.values) {
		return io.EOF
	}
	copy(dest, r.values[r.index])
	r.index++
	return nil
}
