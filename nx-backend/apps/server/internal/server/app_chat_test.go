package server

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
)

func TestAppChatMemoriesForPromptLoadsRecentActiveMemoriesScopedToCard(t *testing.T) {
	registerAppChatMemoryTestDriver()
	database, err := sql.Open(appChatMemoryTestDriverName, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })

	s := &Server{db: database}
	memories, err := s.appChatMemoriesForPrompt(context.Background(), 7, 11, 6)
	if err != nil {
		t.Fatalf("appChatMemoriesForPrompt returned error: %v", err)
	}

	want := []string{"用户曾问：如何处理职场压力？", "用户曾问：如何改善亲密关系？"}
	if strings.Join(memories, "\n") != strings.Join(want, "\n") {
		t.Fatalf("unexpected memories: %#v", memories)
	}
}

const appChatMemoryTestDriverName = "app_chat_memory_test"

var registerAppChatMemoryTestDriverOnce sync.Once

func registerAppChatMemoryTestDriver() {
	registerAppChatMemoryTestDriverOnce.Do(func() {
		sql.Register(appChatMemoryTestDriverName, appChatMemoryTestDriver{})
	})
}

type appChatMemoryTestDriver struct{}

func (appChatMemoryTestDriver) Open(string) (driver.Conn, error) {
	return appChatMemoryTestConn{}, nil
}

type appChatMemoryTestConn struct{}

func (appChatMemoryTestConn) Prepare(string) (driver.Stmt, error) { return nil, driver.ErrSkip }
func (appChatMemoryTestConn) Close() error                        { return nil }
func (appChatMemoryTestConn) Begin() (driver.Tx, error)           { return appQuizTestTx{}, nil }

func (appChatMemoryTestConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if !strings.Contains(query, "FROM app_memories") ||
		!strings.Contains(query, "app_user_id = $1") ||
		!strings.Contains(query, "card_id = $2") ||
		!strings.Contains(query, "status = 'active'") ||
		!strings.Contains(query, "ORDER BY update_time DESC, id DESC") ||
		!strings.Contains(query, "LIMIT $3") {
		return nil, errors.New("memory query is not scoped to active user/card memories")
	}
	if len(args) != 3 ||
		asInt64(args[0].Value) != 7 ||
		asInt64(args[1].Value) != 11 ||
		asInt64(args[2].Value) != 6 {
		return nil, errors.New("unexpected memory query arguments")
	}
	return &appChatMemoryRows{
		values: [][]driver.Value{
			{"用户曾问：如何处理职场压力？"},
			{"  "},
			{"用户曾问：如何改善亲密关系？"},
		},
	}, nil
}

func asInt64(value any) int64 {
	switch typed := value.(type) {
	case int:
		return int64(typed)
	case int64:
		return typed
	default:
		return 0
	}
}

type appChatMemoryRows struct {
	values [][]driver.Value
	index  int
}

func (r *appChatMemoryRows) Columns() []string {
	return []string{"content"}
}

func (r *appChatMemoryRows) Close() error {
	return nil
}

func (r *appChatMemoryRows) Next(dest []driver.Value) error {
	if r.index >= len(r.values) {
		return io.EOF
	}
	copy(dest, r.values[r.index])
	r.index++
	return nil
}
