package chat

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestConversationContextStoreReadsStateAndMessagesAfterWatermark(t *testing.T) {
	database := openConversationContextTestDB(t)
	store := NewStore(database)

	state, err := store.GetConversationState(context.Background(), 42)
	if err != nil {
		t.Fatalf("GetConversationState returned error: %v", err)
	}
	if state.Summary != "已有摘要" || state.SummaryThroughMessageID != 8 {
		t.Fatalf("unexpected state: %+v", state)
	}

	messages, err := store.ListMessagesAfter(context.Background(), 42, 8)
	if err != nil {
		t.Fatalf("ListMessagesAfter returned error: %v", err)
	}
	if len(messages) != 3 || messages[0].ID != 9 || messages[1].ID != 10 || messages[2].EffectiveContent() != "语音里的问题" {
		t.Fatalf("unexpected messages: %+v", messages)
	}
}

func TestConversationContextStoreUpdatesSummaryWithExpectedWatermark(t *testing.T) {
	database := openConversationContextTestDB(t)
	updated, err := NewStore(database).UpdateConversationSummary(context.Background(), 42, 8, "新摘要", 13)
	if err != nil {
		t.Fatalf("UpdateConversationSummary returned error: %v", err)
	}
	if !updated {
		t.Fatal("expected summary update to succeed")
	}
}

const conversationContextDriverName = "chat_conversation_context_test"

var registerConversationContextDriverOnce sync.Once

func openConversationContextTestDB(t *testing.T) *sql.DB {
	t.Helper()
	registerConversationContextDriverOnce.Do(func() {
		sql.Register(conversationContextDriverName, conversationContextDriver{})
	})
	database, err := sql.Open(conversationContextDriverName, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database
}

type conversationContextDriver struct{}

func (conversationContextDriver) Open(string) (driver.Conn, error) {
	return conversationContextConn{}, nil
}

type conversationContextConn struct{}

func (conversationContextConn) Prepare(string) (driver.Stmt, error) { return nil, driver.ErrSkip }
func (conversationContextConn) Close() error                        { return nil }
func (conversationContextConn) Begin() (driver.Tx, error)           { return nil, driver.ErrSkip }

func (conversationContextConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if strings.Contains(query, "FROM app_chat_sessions") {
		if !strings.Contains(query, "context_summary") || len(args) != 1 || args[0].Value != int64(42) {
			return nil, errors.New("invalid conversation state query")
		}
		return &conversationContextRows{columns: []string{"context_summary", "context_summary_through_message_id"}, values: [][]driver.Value{{"已有摘要", int64(8)}}}, nil
	}
	if strings.Contains(query, "FROM app_chat_messages") {
		if !strings.Contains(query, "session_id = $1") || !strings.Contains(query, "id > $2") || !strings.Contains(query, "ORDER BY id") || len(args) != 2 || args[0].Value != int64(42) || args[1].Value != int64(8) {
			return nil, errors.New("invalid messages-after-watermark query")
		}
		now := time.Now()
		return &conversationContextRows{columns: []string{"id", "session_id", "role", "content", "sources", "favorite", "feedback", "message_type", "audio_asset_id", "audio_duration_ms", "transcript", "create_time"}, values: [][]driver.Value{
			{int64(9), int64(42), "user", "新问题", []byte("[]"), false, "", "text", nil, int64(0), "", now},
			{int64(10), int64(42), "assistant", "新回答", []byte("[]"), false, "", "text", nil, int64(0), "", now.Add(time.Second)},
			{int64(11), int64(42), "user", "", []byte("[]"), false, "", "voice", int64(88), int64(2600), "语音里的问题", now.Add(2 * time.Second)},
		}}, nil
	}
	return nil, errors.New("unexpected query")
}

func (conversationContextConn) ExecContext(_ context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	if !strings.Contains(query, "UPDATE app_chat_sessions") ||
		!strings.Contains(query, "context_summary_through_message_id = $3") ||
		!strings.Contains(query, "context_summary_through_message_id = $4") ||
		len(args) != 4 || args[0].Value != int64(42) || args[1].Value != "新摘要" || args[2].Value != int64(13) || args[3].Value != int64(8) {
		return nil, errors.New("invalid conditional summary update")
	}
	return driver.RowsAffected(1), nil
}

type conversationContextRows struct {
	columns []string
	values  [][]driver.Value
	index   int
}

func (r *conversationContextRows) Columns() []string { return r.columns }
func (r *conversationContextRows) Close() error      { return nil }
func (r *conversationContextRows) Next(dest []driver.Value) error {
	if r.index >= len(r.values) {
		return io.EOF
	}
	copy(dest, r.values[r.index])
	r.index++
	return nil
}
