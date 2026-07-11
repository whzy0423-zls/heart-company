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

func TestListRecentMessagesFiltersInvalidTailBeforeLimiting(t *testing.T) {
	registerRecentMessagesDriver()
	database, err := sql.Open(recentMessagesDriverName, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })

	messages, err := NewStore(database).ListRecentMessages(context.Background(), 42, 12)
	if err != nil {
		t.Fatalf("ListRecentMessages returned error: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}
	if messages[0].Role != "user" || messages[0].Content != "我女儿今年八岁" {
		t.Fatalf("expected oldest selected message first, got %+v", messages[0])
	}
	if messages[1].Role != "assistant" || messages[1].Content != "我记住了" {
		t.Fatalf("expected newest selected message last, got %+v", messages[1])
	}
}

const recentMessagesDriverName = "chat_recent_messages_test"

var registerRecentMessagesDriverOnce sync.Once

func registerRecentMessagesDriver() {
	registerRecentMessagesDriverOnce.Do(func() {
		sql.Register(recentMessagesDriverName, recentMessagesDriver{})
	})
}

type recentMessagesDriver struct{}

func (recentMessagesDriver) Open(string) (driver.Conn, error) {
	return recentMessagesConn{}, nil
}

type recentMessagesConn struct{}

func (recentMessagesConn) Prepare(string) (driver.Stmt, error) { return nil, driver.ErrSkip }
func (recentMessagesConn) Close() error                        { return nil }
func (recentMessagesConn) Begin() (driver.Tx, error)           { return nil, driver.ErrSkip }

func (recentMessagesConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if !strings.Contains(query, "FROM app_chat_messages") ||
		!strings.Contains(query, "session_id = $1") ||
		!strings.Contains(query, "role IN ('user', 'assistant')") ||
		!strings.Contains(query, "btrim(content) <> ''") ||
		!strings.Contains(query, "ORDER BY create_time DESC, id DESC") ||
		!strings.Contains(query, "LIMIT $2") {
		return nil, errors.New("recent messages query must be scoped, limited, and newest first")
	}
	if len(args) != 2 || args[0].Value != int64(42) || args[1].Value != int64(12) {
		return nil, errors.New("unexpected recent messages query arguments")
	}
	now := time.Now()
	return &recentMessagesRows{values: [][]driver.Value{
		{int64(2), int64(42), "assistant", "我记住了", []byte("[]"), false, "", now.Add(time.Second)},
		{int64(1), int64(42), "user", "我女儿今年八岁", []byte("[]"), false, "", now},
	}}, nil
}

type recentMessagesRows struct {
	values [][]driver.Value
	index  int
}

func (r *recentMessagesRows) Columns() []string {
	return []string{"id", "session_id", "role", "content", "sources", "favorite", "feedback", "create_time"}
}
func (r *recentMessagesRows) Close() error { return nil }
func (r *recentMessagesRows) Next(dest []driver.Value) error {
	if r.index >= len(r.values) {
		return io.EOF
	}
	copy(dest, r.values[r.index])
	r.index++
	return nil
}
