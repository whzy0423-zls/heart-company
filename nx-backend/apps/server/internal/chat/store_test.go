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
	if len(messages) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(messages))
	}
	if messages[0].Role != "user" || messages[0].Content != "我女儿今年八岁" {
		t.Fatalf("expected oldest selected message first, got %+v", messages[0])
	}
	if messages[1].MessageType != "voice" || messages[1].EffectiveContent() != "她最近不愿意沟通" || messages[1].AudioURL != "/api/app/chat/messages/3/audio" {
		t.Fatalf("expected hidden voice transcript in the middle, got %+v", messages[1])
	}
	if messages[2].Role != "assistant" || messages[2].Content != "我记住了" {
		t.Fatalf("expected newest selected message last, got %+v", messages[1])
	}
}

func TestXinzhiliSceneSessionIsExcludedFromRegularSessionList(t *testing.T) {
	registerSceneSessionDriver()
	database, err := sql.Open(sceneSessionDriverName, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })

	store := NewStore(database)
	session, err := store.GetOrCreateSceneSession(context.Background(), 7, 9, "xinzhili_voice")
	if err != nil {
		t.Fatalf("GetOrCreateSceneSession returned error: %v", err)
	}
	if session.ID != 42 || session.AppUserID != 7 || session.CardID != 9 {
		t.Fatalf("unexpected session: %+v", session)
	}

	sessions, err := store.ListSessions(context.Background(), 7)
	if err != nil {
		t.Fatalf("ListSessions returned error: %v", err)
	}
	if len(sessions) != 1 || sessions[0].ID != 43 {
		t.Fatalf("regular session list = %+v, want only chat session 43", sessions)
	}
	for _, listed := range sessions {
		if listed.ID == session.ID {
			t.Fatalf("xinzhili scene session %d leaked into regular session list", session.ID)
		}
	}
}

const sceneSessionDriverName = "chat_scene_session_test"

var registerSceneSessionDriverOnce sync.Once

func registerSceneSessionDriver() {
	registerSceneSessionDriverOnce.Do(func() {
		sql.Register(sceneSessionDriverName, sceneSessionDriver{})
	})
}

type sceneSessionDriver struct{}

func (sceneSessionDriver) Open(string) (driver.Conn, error) { return sceneSessionConn{}, nil }

type sceneSessionConn struct{}

func (sceneSessionConn) Prepare(string) (driver.Stmt, error) { return nil, driver.ErrSkip }
func (sceneSessionConn) Close() error                        { return nil }
func (sceneSessionConn) Begin() (driver.Tx, error)           { return nil, driver.ErrSkip }
func (sceneSessionConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	now := time.Now()
	switch {
	case strings.Contains(query, "FROM app_chat_sessions") && strings.Contains(query, "scene = $3"):
		if len(args) != 3 || args[0].Value != int64(7) || args[1].Value != int64(9) || args[2].Value != "xinzhili_voice" {
			return nil, errors.New("unexpected scene session arguments")
		}
		return &singleSessionRows{values: []driver.Value{int64(42), int64(7), int64(9), "", now, now}}, nil
	case strings.Contains(query, "FROM app_chat_sessions") && strings.Contains(query, "scene = 'chat'"):
		if len(args) != 1 || args[0].Value != int64(7) {
			return nil, errors.New("unexpected regular session list arguments")
		}
		return &singleSessionRows{values: []driver.Value{int64(43), int64(7), int64(9), "普通聊天", now, now}}, nil
	default:
		return nil, errors.New("session query must keep xinzhili and regular chat scenes isolated")
	}
}

type singleSessionRows struct {
	values []driver.Value
	done   bool
}

func (r *singleSessionRows) Columns() []string {
	return []string{"id", "app_user_id", "card_id", "title", "updated_at", "create_time"}
}
func (r *singleSessionRows) Close() error { return nil }
func (r *singleSessionRows) Next(dest []driver.Value) error {
	if r.done {
		return io.EOF
	}
	copy(dest, r.values)
	r.done = true
	return nil
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
		!strings.Contains(query, "btrim(transcript) <> ''") ||
		!strings.Contains(query, "ORDER BY create_time DESC, id DESC") ||
		!strings.Contains(query, "LIMIT $2") {
		return nil, errors.New("recent messages query must be scoped, limited, and newest first")
	}
	if len(args) != 2 || args[0].Value != int64(42) || args[1].Value != int64(12) {
		return nil, errors.New("unexpected recent messages query arguments")
	}
	now := time.Now()
	return &recentMessagesRows{values: [][]driver.Value{
		{int64(2), int64(42), "assistant", "我记住了", []byte("[]"), false, "", "text", nil, int64(0), "", now.Add(2 * time.Second)},
		{int64(3), int64(42), "user", "", []byte("[]"), false, "", "voice", int64(88), int64(3200), "她最近不愿意沟通", now.Add(time.Second)},
		{int64(1), int64(42), "user", "我女儿今年八岁", []byte("[]"), false, "", "text", nil, int64(0), "", now},
	}}, nil
}

type recentMessagesRows struct {
	values [][]driver.Value
	index  int
}

func (r *recentMessagesRows) Columns() []string {
	return []string{"id", "session_id", "role", "content", "sources", "favorite", "feedback", "message_type", "audio_asset_id", "audio_duration_ms", "transcript", "create_time"}
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
