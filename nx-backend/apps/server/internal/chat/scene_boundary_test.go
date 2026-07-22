package chat

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRegularChatStoreRejectsHiddenSceneSessionAndMessages(t *testing.T) {
	store := newSceneBoundaryStore(t)

	if _, err := store.GetSession(context.Background(), 7, 42); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetSession error = %v, want ErrNotFound", err)
	}
	messages, err := store.ListMessages(context.Background(), 42)
	if err != nil {
		t.Fatalf("ListMessages returned error: %v", err)
	}
	if len(messages) != 0 {
		t.Fatalf("ListMessages returned hidden messages: %+v", messages)
	}
}

func TestRegularChatStoreDoesNotEnumerateHiddenSceneMessages(t *testing.T) {
	store := newSceneBoundaryStore(t)

	search, err := store.SearchMessages(context.Background(), 7, 9, "秘密")
	if err != nil {
		t.Fatalf("SearchMessages returned error: %v", err)
	}
	if len(search) != 1 || search[0].ID != 101 || search[0].Content != "普通聊天秘密" {
		t.Fatalf("SearchMessages = %+v, want only regular chat result", search)
	}

	favorites, err := store.ListFavorites(context.Background(), 7, 9)
	if err != nil {
		t.Fatalf("ListFavorites returned error: %v", err)
	}
	if len(favorites) != 1 || favorites[0].ID != 101 || favorites[0].Content != "普通聊天秘密" {
		t.Fatalf("ListFavorites = %+v, want only regular chat favorite", favorites)
	}
}

func TestRegularChatStoreRejectsHiddenSceneMessageMutation(t *testing.T) {
	store := newSceneBoundaryStore(t)

	if err := store.SetFeedback(context.Background(), 7, 102, "helpful"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("SetFeedback error = %v, want ErrNotFound", err)
	}
	if _, err := store.ToggleFavorite(context.Background(), 7, 102); !errors.Is(err, ErrNotFound) {
		t.Fatalf("ToggleFavorite error = %v, want ErrNotFound", err)
	}
}

func TestRegularChatStoreRejectsHiddenSceneVoiceData(t *testing.T) {
	store := newSceneBoundaryStore(t)

	if _, err := store.GetVoiceAudioAssetID(context.Background(), 7, 103); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetVoiceAudioAssetID error = %v, want ErrNotFound", err)
	}
	if _, err := store.GetVoiceTranscript(context.Background(), 7, 103); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetVoiceTranscript error = %v, want ErrNotFound", err)
	}
}

const sceneBoundaryDriverName = "chat_scene_boundary_test"

var registerSceneBoundaryDriverOnce sync.Once

func newSceneBoundaryStore(t *testing.T) *Store {
	t.Helper()
	registerSceneBoundaryDriverOnce.Do(func() {
		sql.Register(sceneBoundaryDriverName, sceneBoundaryDriver{})
	})
	database, err := sql.Open(sceneBoundaryDriverName, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return NewStore(database)
}

type sceneBoundaryDriver struct{}

func (sceneBoundaryDriver) Open(string) (driver.Conn, error) { return sceneBoundaryConn{}, nil }

type sceneBoundaryConn struct{}

func (sceneBoundaryConn) Prepare(string) (driver.Stmt, error) { return nil, driver.ErrSkip }
func (sceneBoundaryConn) Close() error                        { return nil }
func (sceneBoundaryConn) Begin() (driver.Tx, error)           { return nil, driver.ErrSkip }

func (sceneBoundaryConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	normalized := strings.Join(strings.Fields(query), " ")
	now := time.Now()
	switch normalized {
	case "SELECT id, app_user_id, card_id, title, updated_at, create_time FROM app_chat_sessions WHERE id = $1 AND app_user_id = $2 AND scene = 'chat'":
		if !namedValuesEqual(args, int64(42), int64(7)) {
			return nil, errors.New("unexpected GetSession arguments")
		}
		return &sceneBoundaryRows{columns: sessionColumns()}, nil
	case "SELECT m.id, m.session_id, m.role, m.content, m.sources, m.favorite, m.feedback, m.message_type, m.audio_asset_id, m.audio_duration_ms, m.transcript, m.create_time FROM app_chat_messages m JOIN app_chat_sessions s ON s.id = m.session_id WHERE m.session_id = $1 AND s.scene = 'chat' ORDER BY m.create_time, m.id":
		if !namedValuesEqual(args, int64(42)) {
			return nil, errors.New("unexpected ListMessages arguments")
		}
		return &sceneBoundaryRows{columns: messageColumns()}, nil
	case "SELECT m.id, m.session_id, s.card_id, m.role, m.content, m.sources, m.favorite, m.create_time FROM app_chat_messages m JOIN app_chat_sessions s ON s.id = m.session_id WHERE s.app_user_id = $1 AND s.scene = 'chat' AND ($2 = 0 OR s.card_id = $2) AND m.content ILIKE '%' || $3 || '%' ORDER BY m.create_time DESC, m.id DESC LIMIT 100":
		if !namedValuesEqual(args, int64(7), int64(9), "秘密") {
			return nil, errors.New("unexpected SearchMessages arguments")
		}
		return &sceneBoundaryRows{
			columns: []string{"id", "session_id", "card_id", "role", "content", "sources", "favorite", "create_time"},
			values:  [][]driver.Value{{int64(101), int64(41), int64(9), "assistant", "普通聊天秘密", []byte("[]"), true, now}},
		}, nil
	case "SELECT m.id, m.session_id, s.card_id, m.content, m.sources, m.create_time FROM app_chat_messages m JOIN app_chat_sessions s ON s.id = m.session_id WHERE s.app_user_id = $1 AND s.scene = 'chat' AND m.favorite = true AND ($2 = 0 OR s.card_id = $2) ORDER BY m.create_time DESC, m.id DESC":
		if !namedValuesEqual(args, int64(7), int64(9)) {
			return nil, errors.New("unexpected ListFavorites arguments")
		}
		return &sceneBoundaryRows{
			columns: []string{"id", "session_id", "card_id", "content", "sources", "create_time"},
			values:  [][]driver.Value{{int64(101), int64(41), int64(9), "普通聊天秘密", []byte("[]"), now}},
		}, nil
	case "UPDATE app_chat_messages m SET favorite = NOT m.favorite FROM app_chat_sessions s WHERE m.id = $1 AND m.session_id = s.id AND s.app_user_id = $2 AND s.scene = 'chat' AND m.role = 'assistant' RETURNING m.favorite":
		if !namedValuesEqual(args, int64(102), int64(7)) {
			return nil, errors.New("unexpected ToggleFavorite arguments")
		}
		return &sceneBoundaryRows{columns: []string{"favorite"}}, nil
	case "SELECT m.audio_asset_id FROM app_chat_messages m JOIN app_chat_sessions s ON s.id = m.session_id WHERE m.id = $1 AND s.app_user_id = $2 AND s.scene = 'chat' AND m.role = 'user' AND m.message_type = 'voice' AND m.audio_asset_id IS NOT NULL":
		if !namedValuesEqual(args, int64(103), int64(7)) {
			return nil, errors.New("unexpected GetVoiceAudioAssetID arguments")
		}
		return &sceneBoundaryRows{columns: []string{"audio_asset_id"}}, nil
	case "SELECT m.transcript FROM app_chat_messages m JOIN app_chat_sessions s ON s.id = m.session_id WHERE m.id = $1 AND s.app_user_id = $2 AND s.scene = 'chat' AND m.role = 'user' AND m.message_type = 'voice'":
		if !namedValuesEqual(args, int64(103), int64(7)) {
			return nil, errors.New("unexpected GetVoiceTranscript arguments")
		}
		return &sceneBoundaryRows{columns: []string{"transcript"}}, nil
	default:
		return nil, fmt.Errorf("query is missing the exact regular-chat scene boundary: %s", normalized)
	}
}

func (sceneBoundaryConn) ExecContext(_ context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	normalized := strings.Join(strings.Fields(query), " ")
	if normalized != "UPDATE app_chat_messages m SET feedback = $3 FROM app_chat_sessions s WHERE m.id = $1 AND m.session_id = s.id AND s.app_user_id = $2 AND s.scene = 'chat' AND m.role = 'assistant'" {
		return nil, fmt.Errorf("feedback query is missing the exact regular-chat scene boundary: %s", normalized)
	}
	if !namedValuesEqual(args, int64(102), int64(7), "helpful") {
		return nil, errors.New("unexpected SetFeedback arguments")
	}
	return driver.RowsAffected(0), nil
}

func namedValuesEqual(args []driver.NamedValue, values ...any) bool {
	if len(args) != len(values) {
		return false
	}
	for i, value := range values {
		if args[i].Value != value {
			return false
		}
	}
	return true
}

func sessionColumns() []string {
	return []string{"id", "app_user_id", "card_id", "title", "updated_at", "create_time"}
}

func messageColumns() []string {
	return []string{"id", "session_id", "role", "content", "sources", "favorite", "feedback", "message_type", "audio_asset_id", "audio_duration_ms", "transcript", "create_time"}
}

type sceneBoundaryRows struct {
	columns []string
	values  [][]driver.Value
	index   int
}

func (r *sceneBoundaryRows) Columns() []string { return r.columns }
func (r *sceneBoundaryRows) Close() error      { return nil }
func (r *sceneBoundaryRows) Next(dest []driver.Value) error {
	if r.index >= len(r.values) {
		return io.EOF
	}
	copy(dest, r.values[r.index])
	r.index++
	return nil
}
