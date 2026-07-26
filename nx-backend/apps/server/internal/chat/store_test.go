package chat

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestSceneSessionsDoNotReuseOrdinaryChatAndLegacyReadersHideThem(t *testing.T) {
	database, userID, cardID, cleanup := openChatSceneFixture(t)
	defer cleanup()
	store := NewStore(database)

	ordinary, err := store.GetOrCreateSession(context.Background(), userID, cardID)
	if err != nil {
		t.Fatal(err)
	}
	voice, err := store.GetOrCreateSceneSession(context.Background(), userID, cardID, "xinzhili_voice")
	if err != nil {
		t.Fatal(err)
	}
	voiceAgain, err := store.GetOrCreateSceneSession(context.Background(), userID, cardID, "xinzhili_voice")
	if err != nil {
		t.Fatal(err)
	}
	if ordinary.ID == voice.ID || voiceAgain.ID != voice.ID {
		t.Fatalf("ordinary=%d voice=%d voiceAgain=%d", ordinary.ID, voice.ID, voiceAgain.ID)
	}

	sessions, err := store.ListSessions(context.Background(), userID)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 || sessions[0].ID != ordinary.ID {
		t.Fatalf("legacy ListSessions leaked scene session: %+v", sessions)
	}
	if _, err := store.GetSession(context.Background(), userID, voice.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("legacy GetSession err=%v", err)
	}
}

func TestDeliveryContextUsesConfirmedAssistantTextOnly(t *testing.T) {
	database, userID, cardID, cleanup := openChatSceneFixture(t)
	defer cleanup()
	store := NewStore(database)
	voice, err := store.GetOrCreateSceneSession(context.Background(), userID, cardID, "xinzhili_voice")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO app_chat_messages(session_id, role, content) VALUES ($1,'user','我今天很难过')`, voice.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO app_chat_messages(session_id, role, content, delivery_status, delivered_text, xinzhili_mode) VALUES ($1,'assistant','已播放部分但未确认余下回答','sent','已播放部分','comfort')`, voice.ID); err != nil {
		t.Fatal(err)
	}

	recent, err := store.ListRecentMessages(context.Background(), voice.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	after, err := store.ListMessagesAfter(context.Background(), voice.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(recent) != 2 || recent[0].Content != "我今天很难过" || recent[1].Content != "已播放部分" {
		t.Fatalf("recent context=%+v", recent)
	}
	if len(after) != 2 || after[0].Content != "我今天很难过" || after[1].Content != "已播放部分" {
		t.Fatalf("incremental context=%+v", after)
	}
}

func TestDeliveryContextStopsBeforeMutableAssistantUntilLatePlayed(t *testing.T) {
	database, userID, cardID, cleanup := openChatSceneFixture(t)
	defer cleanup()
	store := NewStore(database)
	voice, err := store.GetOrCreateSceneSession(context.Background(), userID, cardID, "xinzhili_voice")
	if err != nil {
		t.Fatal(err)
	}
	var firstID, mutableID int64
	if err := database.QueryRow(`INSERT INTO app_chat_messages(session_id, role, content) VALUES ($1,'user','第一条') RETURNING id`, voice.ID).Scan(&firstID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRow(`INSERT INTO app_chat_messages(session_id, role, content, delivery_status, delivered_text, xinzhili_mode) VALUES ($1,'assistant','迟到确认的完整回答','unconfirmed','迟到确认','normal') RETURNING id`, voice.ID).Scan(&mutableID); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 25; i++ {
		if _, err := database.Exec(`INSERT INTO app_chat_messages(session_id, role, content) VALUES ($1,'user',$2)`, voice.ID, fmt.Sprintf("后续消息%d", i+1)); err != nil {
			t.Fatal(err)
		}
	}
	var lastID int64
	if err := database.QueryRow(`SELECT max(id) FROM app_chat_messages WHERE session_id=$1`, voice.ID).Scan(&lastID); err != nil {
		t.Fatal(err)
	}

	before, err := store.ListMessagesAfter(context.Background(), voice.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != 27 || before[0].ID != firstID || before[1].ID != mutableID || before[1].Content != "迟到确认" {
		t.Fatalf("current context must retain messages around mutable assistant, got %+v", before)
	}
	if updated, err := store.UpdateConversationSummary(context.Background(), voice.ID, 0, "不安全摘要", lastID); err != nil || updated {
		t.Fatalf("unsafe watermark updated=%v err=%v", updated, err)
	}

	if _, err := database.Exec(`UPDATE app_chat_messages SET delivery_status='played', delivered_text=content WHERE id=$1`, mutableID); err != nil {
		t.Fatal(err)
	}
	after, err := store.ListMessagesAfter(context.Background(), voice.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 27 || after[1].ID != mutableID || after[1].Content != "迟到确认的完整回答" {
		t.Fatalf("late played answer must return on next incremental read, got len=%d messages=%+v", len(after), after)
	}
	if updated, err := store.UpdateConversationSummary(context.Background(), voice.ID, 0, "安全摘要", lastID); err != nil || !updated {
		t.Fatalf("safe watermark updated=%v err=%v", updated, err)
	}
}

func TestDeliveryContextKeepsMessagesAfterPermanentlyUnconfirmedAssistant(t *testing.T) {
	database, userID, cardID, cleanup := openChatSceneFixture(t)
	defer cleanup()
	store := NewStore(database)
	voice, err := store.GetOrCreateSceneSession(context.Background(), userID, cardID, "xinzhili_voice")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO app_chat_messages(session_id, role, content) VALUES ($1,'user','屏障前用户消息')`, voice.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO app_chat_messages(session_id, role, content, delivery_status, delivered_text, xinzhili_mode) VALUES ($1,'assistant','永久未确认回答','unconfirmed','','normal')`, voice.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO app_chat_messages(session_id, role, content) VALUES ($1,'user','屏障后用户消息')`, voice.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO app_chat_messages(session_id, role, content, delivery_status, delivered_text, xinzhili_mode) VALUES ($1,'assistant','屏障后完整回答','played','屏障后完整回答','normal')`, voice.ID); err != nil {
		t.Fatal(err)
	}
	var lastID int64
	if err := database.QueryRow(`SELECT max(id) FROM app_chat_messages WHERE session_id=$1`, voice.ID).Scan(&lastID); err != nil {
		t.Fatal(err)
	}
	messages, err := store.ListMessagesAfter(context.Background(), voice.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 4 || messages[1].Content != "" || messages[2].Content != "屏障后用户消息" || messages[3].Content != "屏障后完整回答" {
		t.Fatalf("current context must keep messages after mutable assistant: %+v", messages)
	}
	if updated, err := store.UpdateConversationSummary(context.Background(), voice.ID, 0, "不可越过", lastID); err != nil || updated {
		t.Fatalf("watermark crossed unconfirmed updated=%v err=%v", updated, err)
	}
}

func TestDeliverySummaryWatermarkCannotMoveBackward(t *testing.T) {
	database, userID, cardID, cleanup := openChatSceneFixture(t)
	defer cleanup()
	session, err := NewStore(database).GetOrCreateSession(context.Background(), userID, cardID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`UPDATE app_chat_sessions SET context_summary='原摘要', context_summary_through_message_id=5 WHERE id=$1`, session.ID); err != nil {
		t.Fatal(err)
	}
	updated, err := NewStore(database).UpdateConversationSummary(context.Background(), session.ID, 5, "倒退摘要", 4)
	if err != nil || updated {
		t.Fatalf("backward watermark updated=%v err=%v", updated, err)
	}
	state, err := NewStore(database).GetConversationState(context.Background(), session.ID)
	if err != nil || state.Summary != "原摘要" || state.SummaryThroughMessageID != 5 {
		t.Fatalf("state changed after backward watermark: %+v err=%v", state, err)
	}
}

func TestDeliveryOrdinaryChatWritesKeepMetadataNull(t *testing.T) {
	database, userID, cardID, cleanup := openChatSceneFixture(t)
	defer cleanup()
	store := NewStore(database)
	session, err := store.GetOrCreateSession(context.Background(), userID, cardID)
	if err != nil {
		t.Fatal(err)
	}
	assistantID, err := store.SavePair(context.Background(), session.ID, "问题", "回答", nil)
	if err != nil {
		t.Fatal(err)
	}
	var status, delivered, mode sql.NullString
	if err := database.QueryRow(`SELECT delivery_status, delivered_text, xinzhili_mode FROM app_chat_messages WHERE id=$1`, assistantID).Scan(&status, &delivered, &mode); err != nil {
		t.Fatal(err)
	}
	if status.Valid || delivered.Valid || mode.Valid {
		t.Fatalf("ordinary chat metadata status=%+v delivered=%+v mode=%+v", status, delivered, mode)
	}
}

func TestSceneHiddenMessagesAreRejectedByOrdinaryChatFeatures(t *testing.T) {
	database, userID, cardID, cleanup := openChatSceneFixture(t)
	defer cleanup()
	store := NewStore(database)
	voice, err := store.GetOrCreateSceneSession(context.Background(), userID, cardID, "xinzhili_voice")
	if err != nil {
		t.Fatal(err)
	}
	var assistantID, userVoiceID int64
	if err := database.QueryRow(`INSERT INTO app_chat_messages(session_id, role, content, favorite, delivery_status, delivered_text, xinzhili_mode) VALUES ($1,'assistant','芯之力隐藏秘密',true,'played','芯之力隐藏秘密','normal') RETURNING id`, voice.ID).Scan(&assistantID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRow(`INSERT INTO app_chat_messages(session_id, role, content, message_type, transcript) VALUES ($1,'user','','voice','芯之力隐藏转写') RETURNING id`, voice.ID).Scan(&userVoiceID); err != nil {
		t.Fatal(err)
	}
	messages, err := store.ListMessages(context.Background(), voice.ID)
	if err != nil || len(messages) != 0 {
		t.Fatalf("ListMessages leaked hidden scene messages=%+v err=%v", messages, err)
	}
	if err := store.SetFeedback(context.Background(), userID, assistantID, "helpful"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("SetFeedback err=%v", err)
	}
	if _, err := store.ToggleFavorite(context.Background(), userID, assistantID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("ToggleFavorite err=%v", err)
	}
	favorites, err := store.ListFavorites(context.Background(), userID, 0)
	if err != nil || len(favorites) != 0 {
		t.Fatalf("favorites=%+v err=%v", favorites, err)
	}
	results, err := store.SearchMessages(context.Background(), userID, 0, "隐藏秘密")
	if err != nil || len(results) != 0 {
		t.Fatalf("search=%+v err=%v", results, err)
	}
	if _, err := store.GetVoiceTranscript(context.Background(), userID, userVoiceID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetVoiceTranscript err=%v", err)
	}
}

func TestConversationIsolationRequiresMatchingUserCardSceneAndConversation(t *testing.T) {
	database, userID, cardID, cleanup := openChatSceneFixture(t)
	defer cleanup()
	store := NewStore(database)
	ctx := context.Background()

	voice, err := store.ResolveSceneSession(ctx, userID, cardID, "xinzhili_voice", 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ResolveSceneSession(ctx, userID, cardID, "xinzhili_voice", voice.ID); err != nil {
		t.Fatalf("matching conversation rejected: %v", err)
	}
	if _, err := store.ResolveSceneSession(ctx, userID, cardID, "chat", voice.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("scene mismatch err=%v, want ErrNotFound", err)
	}

	var otherCardID int64
	if err := database.QueryRow(`INSERT INTO app_user_cards(app_user_id, card_type, name, relation, enneagram, wing, profile, status) VALUES ($1,'secondary','另一张卡','friend',2,1,'{}','active') RETURNING id`, userID).Scan(&otherCardID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ResolveSceneSession(ctx, userID, otherCardID, "xinzhili_voice", voice.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("card mismatch err=%v, want ErrNotFound", err)
	}
}

func TestDeliveryPersistenceUsesAcknowledgedExactPrefixes(t *testing.T) {
	database, userID, cardID, cleanup := openChatSceneFixture(t)
	defer cleanup()
	store := NewStore(database)
	ctx := context.Background()

	session, err := store.ResolveSceneSession(ctx, userID, cardID, "xinzhili_voice", 0)
	if err != nil {
		t.Fatal(err)
	}
	userMessageID, err := store.SaveSceneUserText(ctx, session.ID, "我想慢下来", "comfort")
	if err != nil || userMessageID == 0 {
		t.Fatalf("SaveSceneUserText id=%d err=%v", userMessageID, err)
	}
	assistantID, err := store.CreateSceneAssistant(ctx, session.ID, "先呼吸。再感受脚底。", "comfort")
	if err != nil || assistantID == 0 {
		t.Fatalf("CreateSceneAssistant id=%d err=%v", assistantID, err)
	}
	if err := store.AcknowledgeSceneAssistant(ctx, assistantID, "先呼吸。", false); err != nil {
		t.Fatal(err)
	}
	var intermediateStatus string
	if err := database.QueryRow(`SELECT delivery_status FROM app_chat_messages WHERE id=$1`, assistantID).Scan(&intermediateStatus); err != nil || intermediateStatus != "sent" {
		t.Fatalf("first ack status=%q err=%v", intermediateStatus, err)
	}
	if err := store.AcknowledgeSceneAssistant(ctx, assistantID, "先呼吸。再感受脚底。", false); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRow(`SELECT delivery_status FROM app_chat_messages WHERE id=$1`, assistantID).Scan(&intermediateStatus); err != nil || intermediateStatus != "sent" {
		t.Fatalf("pre-completion final ack status=%q err=%v", intermediateStatus, err)
	}
	if err := store.CompleteSceneAssistant(ctx, assistantID, "先呼吸。再感受脚底。", json.RawMessage(`[{"id":"theory:1"}]`)); err != nil {
		t.Fatal(err)
	}
	if err := store.AcknowledgeSceneAssistant(ctx, assistantID, "先呼吸。再感受脚底。", true); err != nil {
		t.Fatal(err)
	}
	if err := store.AcknowledgeSceneAssistant(ctx, assistantID, "先呼吸。再感受脚底。", true); err != nil {
		t.Fatalf("duplicate final ack: %v", err)
	}
	if err := store.AcknowledgeSceneAssistant(ctx, assistantID, "先呼吸。再感受脚底。", false); err != nil {
		t.Fatalf("duplicate non-complete ack after played: %v", err)
	}
	if err := store.AcknowledgeSceneAssistant(ctx, assistantID, "先呼吸。", false); err == nil {
		t.Fatal("backward ack must be rejected")
	}

	messages, err := store.ListRecentMessages(ctx, session.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 || messages[0].Content != "我想慢下来" || messages[1].Content != "先呼吸。再感受脚底。" {
		t.Fatalf("unexpected delivered context: %+v", messages)
	}
	var status, delivered string
	var sources []byte
	if err := database.QueryRow(`SELECT delivery_status, delivered_text, sources FROM app_chat_messages WHERE id=$1`, assistantID).Scan(&status, &delivered, &sources); err != nil {
		t.Fatal(err)
	}
	if status != "played" || delivered != "先呼吸。再感受脚底。" || !strings.Contains(string(sources), "theory:1") {
		t.Fatalf("status=%q delivered=%q sources=%s", status, delivered, sources)
	}
}

func TestDeliveryAcknowledgementCanAdvanceBeforeGenerationCompletes(t *testing.T) {
	database, userID, cardID, cleanup := openChatSceneFixture(t)
	defer cleanup()
	store := NewStore(database)
	ctx := context.Background()

	session, err := store.ResolveSceneSession(ctx, userID, cardID, "xinzhili_voice", 0)
	if err != nil {
		t.Fatal(err)
	}
	assistantID, err := store.CreateSceneAssistant(ctx, session.ID, "先呼吸。", "comfort")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AcknowledgeSceneAssistant(ctx, assistantID, "先呼吸。", false); err != nil {
		t.Fatal(err)
	}
	if err := store.AcknowledgeSceneAssistant(ctx, assistantID, "先呼吸。再感受脚底。", false); err != nil {
		t.Fatalf("acknowledge streamed prefix before completion: %v", err)
	}

	var content, delivered string
	if err := database.QueryRow(`SELECT content, delivered_text FROM app_chat_messages WHERE id=$1`, assistantID).Scan(&content, &delivered); err != nil {
		t.Fatal(err)
	}
	if content != delivered || delivered != "先呼吸。再感受脚底。" {
		t.Fatalf("content=%q delivered=%q", content, delivered)
	}
}

func openChatSceneFixture(t *testing.T) (*sql.DB, int64, int64, func()) {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run chat scene integration tests")
	}
	lowerDSN := strings.ToLower(dsn)
	if !strings.Contains(lowerDSN, "test") || (!strings.Contains(lowerDSN, "127.0.0.1") && !strings.Contains(lowerDSN, "localhost")) {
		t.Fatal("TEST_DATABASE_URL must be a loopback isolated test database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	adminDB, err := sql.Open("pgx", dsn)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	schemaName := fmt.Sprintf("task2_chat_store_%d", time.Now().UnixNano())
	if _, err := adminDB.ExecContext(ctx, `CREATE SCHEMA `+schemaName); err != nil {
		_ = adminDB.Close()
		cancel()
		t.Fatal(err)
	}
	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	query.Set("search_path", schemaName+",public")
	parsed.RawQuery = query.Encode()
	database, err := sql.Open("pgx", parsed.String())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, task2ChatFixtureSchema); err != nil {
		t.Fatalf("create isolated fixture schema: %v", err)
	}
	var userID, cardID int64
	if err := database.QueryRowContext(ctx, `INSERT INTO app_users(phone) VALUES ($1) RETURNING id`, fmt.Sprintf("task2-chat-%d", time.Now().UnixNano())).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `INSERT INTO app_user_cards(app_user_id, card_type, name, relation, enneagram, wing, profile, status) VALUES ($1,'primary','测试卡','self',1,2,'{}','active') RETURNING id`, userID).Scan(&cardID); err != nil {
		t.Fatal(err)
	}
	cleanup := func() {
		_ = database.Close()
		_, _ = adminDB.Exec(`DROP SCHEMA IF EXISTS ` + schemaName + ` CASCADE`)
		_ = adminDB.Close()
		cancel()
	}
	return database, userID, cardID, cleanup
}

const task2ChatFixtureSchema = `
CREATE TABLE app_users(id BIGSERIAL PRIMARY KEY, phone TEXT NOT NULL DEFAULT '');
CREATE TABLE app_user_cards(
  id BIGSERIAL PRIMARY KEY,
  app_user_id BIGINT NOT NULL REFERENCES app_users(id) ON DELETE CASCADE,
  card_type TEXT NOT NULL DEFAULT 'primary', name TEXT NOT NULL DEFAULT '',
  relation TEXT NOT NULL DEFAULT '', enneagram INT NOT NULL DEFAULT 0,
  wing INT NOT NULL DEFAULT 0, profile JSONB NOT NULL DEFAULT '{}', status TEXT NOT NULL DEFAULT 'active'
);
CREATE TABLE app_chat_sessions(
  id BIGSERIAL PRIMARY KEY,
  app_user_id BIGINT NOT NULL REFERENCES app_users(id) ON DELETE CASCADE,
  card_id BIGINT NOT NULL REFERENCES app_user_cards(id) ON DELETE CASCADE,
  title TEXT NOT NULL DEFAULT '', scene TEXT NOT NULL DEFAULT 'chat',
  context_summary TEXT NOT NULL DEFAULT '', context_summary_through_message_id BIGINT NOT NULL DEFAULT 0,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(), create_time TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE app_chat_messages(
  id BIGSERIAL PRIMARY KEY,
  session_id BIGINT NOT NULL REFERENCES app_chat_sessions(id) ON DELETE CASCADE,
  role TEXT NOT NULL, content TEXT NOT NULL DEFAULT '', sources JSONB NOT NULL DEFAULT '[]',
  favorite BOOLEAN NOT NULL DEFAULT false, feedback TEXT NOT NULL DEFAULT '',
  message_type TEXT NOT NULL DEFAULT 'text', audio_asset_id BIGINT,
  audio_duration_ms INTEGER NOT NULL DEFAULT 0, transcript TEXT NOT NULL DEFAULT '',
  delivery_status TEXT, delivered_text TEXT, xinzhili_mode TEXT,
  create_time TIMESTAMPTZ NOT NULL DEFAULT now()
);`

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

func TestStorePostgresKeepsXinzhiliSceneAndTextMessagesHidden(t *testing.T) {
	database := openChatBoundaryTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	suffix := time.Now().UnixNano()
	var userID, cardID int64
	if err := database.QueryRowContext(ctx,
		`INSERT INTO app_users (phone) VALUES ($1) RETURNING id`,
		fmt.Sprintf("chat-boundary-%d", suffix),
	).Scan(&userID); err != nil {
		t.Fatalf("create app user: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		_, _ = database.ExecContext(cleanupCtx, `DELETE FROM app_users WHERE id = $1`, userID)
	})
	if err := database.QueryRowContext(ctx,
		`INSERT INTO app_user_cards (app_user_id, card_type, name) VALUES ($1, 'primary', '测试卡片') RETURNING id`,
		userID,
	).Scan(&cardID); err != nil {
		t.Fatalf("create app user card: %v", err)
	}

	store := NewStore(database)
	regular, err := store.GetOrCreateSession(ctx, userID, cardID)
	if err != nil {
		t.Fatalf("GetOrCreateSession: %v", err)
	}
	xinzhili, err := store.GetOrCreateSceneSession(ctx, userID, cardID, "xinzhili_voice")
	if err != nil {
		t.Fatalf("GetOrCreateSceneSession: %v", err)
	}
	if regular.ID == xinzhili.ID {
		t.Fatalf("chat and xinzhili scenes shared session %d", regular.ID)
	}

	againRegular, err := store.GetOrCreateSceneSession(ctx, userID, cardID, "chat")
	if err != nil {
		t.Fatalf("resume chat scene: %v", err)
	}
	againXinzhili, err := store.GetOrCreateSceneSession(ctx, userID, cardID, "xinzhili_voice")
	if err != nil {
		t.Fatalf("resume xinzhili scene: %v", err)
	}
	if againRegular.ID != regular.ID || againXinzhili.ID != xinzhili.ID {
		t.Fatalf("scene resume crossed boundaries: chat=%d/%d xinzhili=%d/%d", regular.ID, againRegular.ID, xinzhili.ID, againXinzhili.ID)
	}

	sessions, err := store.ListSessions(ctx, userID)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) != 1 || sessions[0].ID != regular.ID {
		t.Fatalf("regular session list = %+v, want only chat session %d", sessions, regular.ID)
	}

	sources := json.RawMessage(`[{"id":"kb-1","title":"知识"}]`)
	hiddenAssistantID, err := store.SavePair(ctx, xinzhili.ID, "我最近总是着急", "先停一下。", sources)
	if err != nil {
		t.Fatalf("SavePair: %v", err)
	}
	if _, err := store.SavePair(ctx, regular.ID, "普通聊天问题", "普通聊天回答", nil); err != nil {
		t.Fatalf("SavePair regular chat: %v", err)
	}

	if _, err := store.GetSession(ctx, userID, xinzhili.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetSession hidden scene error = %v, want ErrNotFound", err)
	}
	hiddenMessages, err := store.ListMessages(ctx, xinzhili.ID)
	if err != nil {
		t.Fatalf("ListMessages hidden scene: %v", err)
	}
	if len(hiddenMessages) != 0 {
		t.Fatalf("ListMessages exposed hidden scene messages: %+v", hiddenMessages)
	}
	searchResults, err := store.SearchMessages(ctx, userID, cardID, "着急")
	if err != nil {
		t.Fatalf("SearchMessages hidden text: %v", err)
	}
	if len(searchResults) != 0 {
		t.Fatalf("SearchMessages exposed hidden text: %+v", searchResults)
	}
	if err := store.SetFeedback(ctx, userID, hiddenAssistantID, "helpful"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("SetFeedback hidden message error = %v, want ErrNotFound", err)
	}
	if _, err := store.ToggleFavorite(ctx, userID, hiddenAssistantID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("ToggleFavorite hidden message error = %v, want ErrNotFound", err)
	}
	if _, err := database.ExecContext(ctx, `UPDATE app_chat_messages SET favorite = true WHERE id = $1`, hiddenAssistantID); err != nil {
		t.Fatalf("mark hidden message favorite: %v", err)
	}
	favorites, err := store.ListFavorites(ctx, userID, cardID)
	if err != nil {
		t.Fatalf("ListFavorites hidden message: %v", err)
	}
	for _, favorite := range favorites {
		if favorite.ID == hiddenAssistantID {
			t.Fatalf("ListFavorites exposed hidden message: %+v", favorite)
		}
	}

	rows, err := database.QueryContext(ctx,
		`SELECT role, content, sources, message_type, audio_asset_id, audio_duration_ms, transcript
		 FROM app_chat_messages WHERE session_id = $1 ORDER BY id`,
		xinzhili.ID,
	)
	if err != nil {
		t.Fatalf("read saved messages: %v", err)
	}
	defer rows.Close()

	type storedMessage struct {
		role, content, messageType, transcript string
		sources                                json.RawMessage
		audioAssetID                           sql.NullInt64
		audioDurationMs                        int
	}
	var messages []storedMessage
	for rows.Next() {
		var message storedMessage
		if err := rows.Scan(&message.role, &message.content, &message.sources, &message.messageType, &message.audioAssetID, &message.audioDurationMs, &message.transcript); err != nil {
			t.Fatalf("scan saved message: %v", err)
		}
		messages = append(messages, message)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate saved messages: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("saved messages = %+v, want text pair", messages)
	}
	wantContents := []string{"我最近总是着急", "先停一下。"}
	wantRoles := []string{"user", "assistant"}
	for i, message := range messages {
		if message.role != wantRoles[i] || message.content != wantContents[i] || message.messageType != "text" {
			t.Fatalf("saved message %d = %+v", i, message)
		}
		if message.audioAssetID.Valid || message.audioDurationMs != 0 || message.transcript != "" {
			t.Fatalf("saved text message %d contains voice columns: %+v", i, message)
		}
	}
	if string(messages[0].sources) != "[]" {
		t.Fatalf("user sources = %s, want []", messages[0].sources)
	}
	var gotSources, wantSources any
	if err := json.Unmarshal(messages[1].sources, &gotSources); err != nil {
		t.Fatalf("decode assistant sources: %v", err)
	}
	if err := json.Unmarshal(sources, &wantSources); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotSources, wantSources) {
		t.Fatalf("assistant sources = %s, want %s", messages[1].sources, sources)
	}

	var hiddenVoiceMessageID int64
	if err := database.QueryRowContext(ctx,
		`INSERT INTO app_chat_messages (session_id, role, content, message_type, transcript)
		 VALUES ($1, 'user', '', 'voice', '隐藏语音转写') RETURNING id`,
		xinzhili.ID,
	).Scan(&hiddenVoiceMessageID); err != nil {
		t.Fatalf("insert hidden voice message: %v", err)
	}
	if _, err := store.GetVoiceTranscript(ctx, userID, hiddenVoiceMessageID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetVoiceTranscript hidden message error = %v, want ErrNotFound", err)
	}
}

func openChatBoundaryTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run chat boundary PostgreSQL integration tests")
	}
	if err := testutil.ValidateIsolatedPostgresDSN(dsn); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	database, err := db.Open(ctx, dsn, "chat_boundary_test", "123456")
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database
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
func (sceneSessionConn) Begin() (driver.Tx, error)           { return sceneSessionNoopTx{}, nil }
func (sceneSessionConn) BeginTx(context.Context, driver.TxOptions) (driver.Tx, error) {
	return sceneSessionNoopTx{}, nil
}
func (sceneSessionConn) ExecContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Result, error) {
	if !strings.Contains(query, "pg_advisory_xact_lock") {
		return nil, fmt.Errorf("unexpected session exec: %s", query)
	}
	return driver.RowsAffected(1), nil
}
func (sceneSessionConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	now := time.Now()
	normalized := strings.Join(strings.Fields(query), " ")
	switch normalized {
	case "SELECT id, app_user_id, card_id, title, updated_at, create_time FROM app_chat_sessions WHERE app_user_id = $1 AND card_id = $2 AND scene = $3 ORDER BY updated_at DESC, id DESC LIMIT 1":
		if len(args) != 3 || args[0].Value != int64(7) || args[1].Value != int64(9) || args[2].Value != "xinzhili_voice" {
			return nil, errors.New("unexpected scene session arguments")
		}
		return &singleSessionRows{values: []driver.Value{int64(42), int64(7), int64(9), "", now, now}}, nil
	case "SELECT id, app_user_id, card_id, title, updated_at, create_time FROM app_chat_sessions WHERE app_user_id = $1 AND scene = 'chat' ORDER BY updated_at DESC":
		if len(args) != 1 || args[0].Value != int64(7) {
			return nil, errors.New("unexpected regular session list arguments")
		}
		return &singleSessionRows{values: []driver.Value{int64(43), int64(7), int64(9), "普通聊天", now, now}}, nil
	default:
		return nil, fmt.Errorf("unexpected session query: %s", normalized)
	}
}

type sceneSessionNoopTx struct{}

func (sceneSessionNoopTx) Commit() error   { return nil }
func (sceneSessionNoopTx) Rollback() error { return nil }

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
