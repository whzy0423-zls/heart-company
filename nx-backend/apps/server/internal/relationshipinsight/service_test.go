package relationshipinsight

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

func TestAnalyzeMarksSmallSamplesPreliminaryAndKeepsScoresBounded(t *testing.T) {
	now := time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)
	report := analyze(1, 2, 9, []messageSample{
		{SenderID: 1, Type: "text", Body: "谢谢你理解我", SequenceNo: 1, CreatedAt: now},
		{SenderID: 2, Type: "text", Body: "没关系，我听你", SequenceNo: 2, CreatedAt: now.Add(time.Minute)},
	})
	if report.ObservationLevel != "preliminary" {
		t.Fatalf("observation level = %q", report.ObservationLevel)
	}
	if !strings.Contains(report.Summary, "不代表感情结论") {
		t.Fatalf("summary must contain the signal disclaimer: %q", report.Summary)
	}
	for name, metric := range report.Metrics {
		if metric.Score < 0 || metric.Score > 100 || metric.Confidence < 0 || metric.Confidence > 100 {
			t.Fatalf("%s out of range: %+v", name, metric)
		}
	}
}

func TestAnalyzeMarksTwentyMessagesAcrossThreeDaysStable(t *testing.T) {
	start := time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)
	messages := make([]messageSample, 20)
	for index := range messages {
		messages[index] = messageSample{
			SenderID:   int64(1 + index%2),
			Type:       "text",
			Body:       "正常交流",
			SequenceNo: int64(index + 1),
			CreatedAt:  start.Add(time.Duration(index%3)*24*time.Hour + time.Duration(index)*time.Minute),
		}
	}
	if got := analyze(1, 2, 9, messages).ObservationLevel; got != "stable" {
		t.Fatalf("observation level = %q", got)
	}
}

func TestGenerateRejectsNonParticipantBeforeMembershipLookup(t *testing.T) {
	database := openInsightDB(t, func(query string, _ []driver.NamedValue) (driver.Rows, error) {
		if strings.Contains(query, "FROM direct_conversations") {
			return insightRows([]string{"user_low_id", "user_high_id"}), nil
		}
		t.Fatalf("unexpected query after participant rejection: %s", query)
		return nil, errors.New("unexpected query")
	})
	_, err := NewService(database).Generate(context.Background(), 1, 99)
	if !errors.Is(err, ErrNotParticipant) {
		t.Fatalf("error = %v", err)
	}
}

func TestGenerateRequiresActiveVIP(t *testing.T) {
	database := openInsightDB(t, func(query string, _ []driver.NamedValue) (driver.Rows, error) {
		switch {
		case strings.Contains(query, "FROM direct_conversations"):
			return insightRows([]string{"user_low_id", "user_high_id"}, int64(1), int64(2)), nil
		case strings.Contains(query, "SELECT member_level"):
			return insightRows([]string{"member_level", "member_expires_at"}, "free", nil), nil
		default:
			t.Fatalf("unexpected query: %s", query)
			return nil, errors.New("unexpected query")
		}
	})
	_, err := NewService(database).Generate(context.Background(), 1, 9)
	if !errors.Is(err, ErrVIPRequired) {
		t.Fatalf("error = %v", err)
	}
}

func TestGenerateRejectsEmptyConversation(t *testing.T) {
	database := openInsightDB(t, func(query string, _ []driver.NamedValue) (driver.Rows, error) {
		switch {
		case strings.Contains(query, "FROM direct_conversations"):
			return insightRows([]string{"user_low_id", "user_high_id"}, int64(1), int64(2)), nil
		case strings.Contains(query, "SELECT member_level"):
			return insightRows([]string{"member_level", "member_expires_at"}, "vip", time.Now().Add(time.Hour)), nil
		case strings.Contains(query, "FROM direct_messages"):
			return insightRows([]string{"sender_id", "message_type", "body", "sequence_no", "created_at"}), nil
		default:
			t.Fatalf("unexpected query: %s", query)
			return nil, errors.New("unexpected query")
		}
	})
	_, err := NewService(database).Generate(context.Background(), 1, 9)
	if !errors.Is(err, ErrNoMessages) {
		t.Fatalf("error = %v", err)
	}
}

func TestVisiblePersonalityRequiresFriendsVisibility(t *testing.T) {
	for _, test := range []struct {
		name       string
		visibility string
		wantType   bool
	}{
		{name: "published", visibility: "friends", wantType: true},
		{name: "hidden", visibility: "private", wantType: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			database := openInsightDB(t, func(query string, _ []driver.NamedValue) (driver.Rows, error) {
				if !strings.Contains(query, "personality_visibility") {
					t.Fatalf("unexpected query: %s", query)
				}
				return insightRows([]string{"personality_visibility", "enneagram"}, test.visibility, int64(5)), nil
			})
			personality, reference, err := NewService(database).visiblePersonality(context.Background(), 2)
			if err != nil {
				t.Fatal(err)
			}
			if (personality != nil) != test.wantType || (reference != nil) != test.wantType {
				t.Fatalf("personality=%v reference=%v", personality, reference)
			}
		})
	}
}

func TestGetRedactsStoredPersonalityAfterPeerHidesType(t *testing.T) {
	queries := 0
	database := openInsightDB(t, func(query string, _ []driver.NamedValue) (driver.Rows, error) {
		queries++
		switch {
		case strings.Contains(query, "FROM relationship_insights"):
			return insightRows(
				[]string{"id", "conversation_id", "peer_id", "from_sequence", "to_sequence", "message_count", "status", "observation_level", "personality_type_snapshot", "metrics", "summary", "personality_reference", "suggestions", "created_at"},
				int64(7), int64(9), int64(2), int64(1), int64(20), int64(20), "completed", "stable", int64(5), []byte(`{"temperature":{"score":80,"confidence":90,"trend":"up","evidence":"x"}}`), "摘要", []byte(`{"label":"5号"}`), []byte(`["建议"]`), time.Now(),
			), nil
		case strings.Contains(query, "personality_visibility"):
			return insightRows([]string{"personality_visibility", "enneagram"}, "private", int64(5)), nil
		default:
			t.Fatalf("unexpected query: %s", query)
			return nil, errors.New("unexpected query")
		}
	})
	report, err := NewService(database).Get(context.Background(), 1, 7)
	if err != nil {
		t.Fatal(err)
	}
	if report.PersonalityTypeSnapshot != nil || report.PersonalityReference != nil {
		t.Fatalf("hidden personality leaked: %+v", report)
	}
	if queries != 2 {
		t.Fatalf("queries = %d, want 2", queries)
	}
}

type insightQuery func(string, []driver.NamedValue) (driver.Rows, error)

type insightConnector struct{ query insightQuery }

func (c insightConnector) Connect(context.Context) (driver.Conn, error) {
	return insightConn{query: c.query}, nil
}
func (c insightConnector) Driver() driver.Driver { return insightDriver{} }

type insightDriver struct{}

func (insightDriver) Open(string) (driver.Conn, error) { return nil, errors.New("use connector") }

type insightConn struct{ query insightQuery }

func (c insightConn) Prepare(string) (driver.Stmt, error) { return nil, errors.New("not supported") }
func (c insightConn) Close() error                        { return nil }
func (c insightConn) Begin() (driver.Tx, error)           { return nil, errors.New("not supported") }
func (c insightConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	return c.query(query, args)
}

type insightRowSet struct {
	columns []string
	values  [][]driver.Value
	index   int
}

func insightRows(columns []string, values ...driver.Value) driver.Rows {
	rows := [][]driver.Value{}
	if len(values) > 0 {
		rows = append(rows, values)
	}
	return &insightRowSet{columns: columns, values: rows}
}
func (r *insightRowSet) Columns() []string { return r.columns }
func (r *insightRowSet) Close() error      { return nil }
func (r *insightRowSet) Next(dest []driver.Value) error {
	if r.index >= len(r.values) {
		return io.EOF
	}
	copy(dest, r.values[r.index])
	r.index++
	return nil
}

func openInsightDB(t *testing.T, query insightQuery) *sql.DB {
	t.Helper()
	database := sql.OpenDB(insightConnector{query: query})
	t.Cleanup(func() { _ = database.Close() })
	return database
}
