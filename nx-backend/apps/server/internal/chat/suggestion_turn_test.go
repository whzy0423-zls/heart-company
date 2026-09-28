package chat

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestGetSuggestionTurnValidatesOwnerSceneAndSessionInSingleRead(t *testing.T) {
	for _, tt := range []struct {
		scene                  string
		user, session, message int64
		found                  bool
	}{
		{"chat", 7, 42, 99, true}, {"enneagram_9", 7, 42, 99, true}, {"skill_chat", 7, 42, 99, true}, {"xinzhili_voice", 7, 0, 99, true},
		{"chat", 8, 42, 99, false}, {"chat", 7, 43, 99, false}, {"chat", 7, 42, 100, false}, {"another_hidden_scene", 7, 42, 99, false},
	} {
		t.Run(tt.scene, func(t *testing.T) {
			name := "suggestion_turn_" + strings.ReplaceAll(t.Name(), "/", "_")
			sql.Register(name, suggestionTurnDriver{})
			db, err := sql.Open(name, tt.scene)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			turn, err := NewStore(db).GetSuggestionTurn(context.Background(), tt.user, tt.session, tt.message, tt.scene)
			if tt.found {
				if err != nil || turn.SessionID != 42 || turn.Question != "保存的语音转写" || turn.Answer != "保存的实际回答" {
					t.Fatalf("turn=%+v err=%v", turn, err)
				}
			} else if !errors.Is(err, ErrNotFound) {
				t.Fatalf("expected not found got %+v %v", turn, err)
			}
		})
	}
}

func TestGetSuggestionTurnPostgresKeepsChatAndHiddenScenesSeparate(t *testing.T) {
	db, userID, cardID, cleanup := openChatSceneFixture(t)
	defer cleanup()
	store := NewStore(db)
	ctx := context.Background()
	for _, scene := range []string{"chat", "enneagram_9", "xinzhili_voice"} {
		session, err := store.GetOrCreateSceneSession(ctx, userID, cardID, scene)
		if err != nil {
			t.Fatal(err)
		}
		messageID, err := store.SavePair(ctx, session.ID, "这一轮的问题", "这一轮的实际回答", nil)
		if err != nil {
			t.Fatal(err)
		}
		turn, err := store.GetSuggestionTurn(ctx, userID, session.ID, messageID, scene)
		if err != nil || turn.Question != "这一轮的问题" || turn.Answer != "这一轮的实际回答" {
			t.Fatalf("scene=%s turn=%+v err=%v", scene, turn, err)
		}
		for _, lookup := range []struct {
			user, session int64
			scene         string
		}{
			{userID + 1, session.ID, scene}, {userID, session.ID + 100, scene}, {userID, session.ID, "enneagram_1"},
		} {
			_, err := store.GetSuggestionTurn(ctx, lookup.user, lookup.session, messageID, lookup.scene)
			if !errors.Is(err, ErrNotFound) {
				t.Fatalf("cross-scene lookup=%+v err=%v", lookup, err)
			}
		}
	}
}

func TestGetSuggestionTurnPostgresChecksSkillAvailability(t *testing.T) {
	db, userID, cardID, cleanup := openChatSceneFixture(t)
	defer cleanup()
	ctx := context.Background()
	store := NewStore(db)
	_, err := db.ExecContext(ctx, `ALTER TABLE app_chat_sessions ADD COLUMN skill_version_id BIGINT;
	 CREATE TABLE app_skill_libraries(id BIGINT PRIMARY KEY,status TEXT NOT NULL);
	 CREATE TABLE app_skill_categories(id BIGINT PRIMARY KEY,library_id BIGINT,status TEXT NOT NULL);
	 CREATE TABLE app_skills(id BIGINT PRIMARY KEY,category_id BIGINT,status TEXT NOT NULL);
	 CREATE TABLE app_skill_versions(id BIGINT PRIMARY KEY,skill_id BIGINT,theory_release_id BIGINT,status TEXT NOT NULL);
	 INSERT INTO app_skill_libraries VALUES(1,'enabled');INSERT INTO app_skill_categories VALUES(2,1,'enabled');
	 INSERT INTO app_skills VALUES(3,2,'enabled');INSERT INTO app_skill_versions VALUES(4,3,5,'published');`)
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.GetOrCreateSceneSession(ctx, userID, cardID, "skill_chat")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, `UPDATE app_chat_sessions SET skill_version_id=4 WHERE id=$1`, session.ID); err != nil {
		t.Fatal(err)
	}
	messageID, err := store.SavePair(ctx, session.ID, "这个技能怎么练习？", "先从一次具体练习开始。", nil)
	if err != nil {
		t.Fatal(err)
	}
	turn, err := store.GetSuggestionTurn(ctx, userID, session.ID, messageID, "skill_chat")
	if err != nil || turn.Question != "这个技能怎么练习？" {
		t.Fatalf("turn=%+v err=%v", turn, err)
	}
	if _, err = db.ExecContext(ctx, `UPDATE app_skills SET status='disabled' WHERE id=3`); err != nil {
		t.Fatal(err)
	}
	if _, err = store.GetSuggestionTurn(ctx, userID, session.ID, messageID, "skill_chat"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("disabled skill should be hidden: %v", err)
	}
}

func TestGetSuggestionTurnPostgresAssociatesRealtimeAndRejectsPartial(t *testing.T) {
	db, userID, cardID, cleanup := openChatSceneFixture(t)
	defer cleanup()
	ctx := context.Background()
	store := NewStore(db)
	session, err := store.GetOrCreateSceneSession(ctx, userID, cardID, "xinzhili_voice")
	if err != nil {
		t.Fatal(err)
	}
	firstUserID, err := store.SaveSceneUserText(ctx, session.ID, "第一轮实际问题", "normal")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.SaveSceneUserText(ctx, session.ID, "并发的另一轮问题", "normal"); err != nil {
		t.Fatal(err)
	}
	assistantID, err := store.CreateSceneAssistantForUser(ctx, session.ID, firstUserID, "第一轮完整回答。", "normal")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.GetSuggestionTurn(ctx, userID, session.ID, assistantID, "xinzhili_voice"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("partial generation must not be used, err=%v", err)
	}
	if err = store.CompleteSceneAssistant(ctx, assistantID, "第一轮完整回答。", nil); err != nil {
		t.Fatal(err)
	}
	if err = store.AcknowledgeSceneAssistant(ctx, assistantID, "第一轮", false); err != nil {
		t.Fatal(err)
	}
	turn, err := store.GetSuggestionTurn(ctx, userID, session.ID, assistantID, "xinzhili_voice")
	if err != nil || turn.Question != "第一轮实际问题" || turn.Answer != "第一轮完整回答。" {
		t.Fatalf("wrong concurrent association: turn=%+v err=%v", turn, err)
	}
	var status string
	if err = db.QueryRow(`SELECT delivery_status FROM app_chat_messages WHERE id=$1`, assistantID).Scan(&status); err != nil || status != "generated" {
		t.Fatalf("generation marker lost after partial ack status=%s err=%v", status, err)
	}
	if err = store.AcknowledgeSceneAssistant(ctx, assistantID, "第一轮完整回答。", true); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT delivery_status FROM app_chat_messages WHERE id=$1`, assistantID).Scan(&status); err != nil || status != "played" {
		t.Fatalf("complete playback status=%s err=%v", status, err)
	}
}

type suggestionTurnDriver struct{}
type suggestionTurnConn struct{ scene string }

func (suggestionTurnDriver) Open(name string) (driver.Conn, error) {
	return suggestionTurnConn{scene: name}, nil
}
func (suggestionTurnConn) Prepare(string) (driver.Stmt, error) { return nil, driver.ErrSkip }
func (suggestionTurnConn) Close() error                        { return nil }
func (suggestionTurnConn) Begin() (driver.Tx, error)           { return nil, driver.ErrSkip }
func (c suggestionTurnConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	for _, required := range []string{"s.app_user_id = $1", "m.id = $2", "s.id = $3", "s.scene = $4", "m.role = 'assistant'", "u.role = 'user'", "u.transcript", "u.create_time = m.create_time"} {
		if !strings.Contains(query, required) {
			return nil, errors.New("missing isolation predicate: " + required)
		}
	}
	if !strings.Contains(query, "u.id = m.reply_to_message_id") || !strings.Contains(query, "m.reply_to_message_id IS NULL") || !strings.Contains(query, "m.delivery_status IN ('generated','played')") {
		return nil, errors.New("missing completed explicit-turn association")
	}
	found := args[0].Value == int64(7) && args[1].Value == int64(99) && (args[2].Value == int64(42) || args[2].Value == int64(0)) && args[3].Value == c.scene
	return &suggestionTurnRows{found: found}, nil
}

type suggestionTurnRows struct{ found, read bool }

func (*suggestionTurnRows) Columns() []string { return []string{"session_id", "question", "answer"} }
func (*suggestionTurnRows) Close() error      { return nil }
func (r *suggestionTurnRows) Next(dest []driver.Value) error {
	if !r.found || r.read {
		return io.EOF
	}
	r.read = true
	copy(dest, []driver.Value{int64(42), "保存的语音转写", "保存的实际回答"})
	return nil
}
