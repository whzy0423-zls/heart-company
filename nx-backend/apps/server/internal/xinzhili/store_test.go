package xinzhili

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestModePreferenceFirstWriteAndCAS(t *testing.T) {
	database, userID, _, cleanup := openStoreFixture(t)
	defer cleanup()
	store := NewStore(database)

	created, err := store.UpdateMode(context.Background(), userID, ModeComfort, 0)
	if err != nil {
		t.Fatal(err)
	}
	if created.UserID != userID || created.Requested != ModeComfort || created.Revision != 1 {
		t.Fatalf("created=%+v", created)
	}
	updated, err := store.UpdateMode(context.Background(), userID, ModeArgument, 1)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Requested != ModeArgument || updated.Revision != 2 {
		t.Fatalf("updated=%+v", updated)
	}
	got, found, err := store.ReadMode(context.Background(), userID)
	if err != nil || !found || got != updated {
		t.Fatalf("read found=%v err=%v got=%+v", found, err, got)
	}
}

func TestModePreferenceConflictDoesNotChangeUpdateTime(t *testing.T) {
	database, userID, _, cleanup := openStoreFixture(t)
	defer cleanup()
	store := NewStore(database)
	if _, err := store.UpdateMode(context.Background(), userID, ModeNormal, 0); err != nil {
		t.Fatal(err)
	}
	var before time.Time
	if err := database.QueryRow(`SELECT update_time FROM app_xinzhili_mode_preferences WHERE app_user_id=$1`, userID).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateMode(context.Background(), userID, ModeComfort, 0); !errors.Is(err, ErrModePreferenceConflict) {
		t.Fatalf("err=%v", err)
	}
	var requested string
	var revision int64
	var after time.Time
	if err := database.QueryRow(`SELECT requested_mode, revision, update_time FROM app_xinzhili_mode_preferences WHERE app_user_id=$1`, userID).Scan(&requested, &revision, &after); err != nil {
		t.Fatal(err)
	}
	if requested != string(ModeNormal) || revision != 1 || !after.Equal(before) {
		t.Fatalf("requested=%s revision=%d before=%s after=%s", requested, revision, before, after)
	}
}

func TestModePreferenceConcurrentFirstWriteHasSingleWinner(t *testing.T) {
	database, userID, _, cleanup := openStoreFixture(t)
	defer cleanup()
	store := NewStore(database)
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for _, mode := range []Mode{ModeArgument, ModeComfort} {
		wg.Add(1)
		go func(mode Mode) {
			defer wg.Done()
			<-start
			_, err := store.UpdateMode(context.Background(), userID, mode, 0)
			errs <- err
		}(mode)
	}
	close(start)
	wg.Wait()
	close(errs)
	var successes, conflicts int
	for err := range errs {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrModePreferenceConflict):
			conflicts++
		default:
			t.Fatalf("unexpected err=%v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("successes=%d conflicts=%d", successes, conflicts)
	}
}

func TestModePreferenceConcurrentCASUpdateHasSingleWinner(t *testing.T) {
	database, userID, _, cleanup := openStoreFixture(t)
	defer cleanup()
	store := NewStore(database)
	if _, err := store.UpdateMode(context.Background(), userID, ModeNormal, 0); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for _, mode := range []Mode{ModeArgument, ModeComfort} {
		wg.Add(1)
		go func(mode Mode) {
			defer wg.Done()
			<-start
			_, err := store.UpdateMode(context.Background(), userID, mode, 1)
			errs <- err
		}(mode)
	}
	close(start)
	wg.Wait()
	close(errs)
	var successes, conflicts int
	for err := range errs {
		if err == nil {
			successes++
		} else if errors.Is(err, ErrModePreferenceConflict) {
			conflicts++
		} else {
			t.Fatalf("unexpected err=%v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("successes=%d conflicts=%d", successes, conflicts)
	}
}

func TestDeliveryAllowsFixedTransitionsAndIdempotentAck(t *testing.T) {
	database, _, sessionID, cleanup := openStoreFixture(t)
	defer cleanup()
	messageID := insertAssistantDelivery(t, database, sessionID, "你好，慢慢说。")
	store := NewStore(database)

	steps := []struct {
		status DeliveryStatus
		text   string
	}{
		{DeliverySynthesizing, ""},
		{DeliverySent, ""},
		{DeliverySent, "你好，"},
		{DeliveryUnconfirmed, "你好，"},
		{DeliveryPlayed, "你好，慢慢说。"},
		{DeliveryPlayed, "你好，慢慢说。"},
	}
	for _, step := range steps {
		got, err := store.UpdateDelivery(context.Background(), messageID, step.status, step.text)
		if err != nil {
			t.Fatalf("status=%s text=%q: %v", step.status, step.text, err)
		}
		if got.Status != step.status || got.DeliveredText != step.text {
			t.Fatalf("got=%+v", got)
		}
	}
}

func TestDeliveryAllowsTTSFailureFromGeneratedOrSynthesizing(t *testing.T) {
	for _, synthesizeFirst := range []bool{false, true} {
		t.Run(fmt.Sprintf("synthesizeFirst=%v", synthesizeFirst), func(t *testing.T) {
			database, _, sessionID, cleanup := openStoreFixture(t)
			defer cleanup()
			messageID := insertAssistantDelivery(t, database, sessionID, "回答")
			store := NewStore(database)
			if synthesizeFirst {
				if _, err := store.UpdateDelivery(context.Background(), messageID, DeliverySynthesizing, ""); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := store.UpdateDelivery(context.Background(), messageID, DeliveryTTSFailed, ""); err != nil {
				t.Fatal(err)
			}
			if _, err := store.UpdateDelivery(context.Background(), messageID, DeliverySynthesizing, ""); !errors.Is(err, ErrInvalidDeliveryTransition) {
				t.Fatalf("tts_failed must be terminal, err=%v", err)
			}
		})
	}
}

func TestDeliveryConcurrentDuplicateAckIsIdempotent(t *testing.T) {
	database, _, sessionID, cleanup := openStoreFixture(t)
	defer cleanup()
	messageID := insertAssistantDelivery(t, database, sessionID, "完整回答")
	store := NewStore(database)
	for _, status := range []DeliveryStatus{DeliverySynthesizing, DeliverySent} {
		if _, err := store.UpdateDelivery(context.Background(), messageID, status, ""); err != nil {
			t.Fatal(err)
		}
	}
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := store.UpdateDelivery(context.Background(), messageID, DeliveryPlayed, "完整回答")
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("duplicate ack err=%v", err)
		}
	}
}

func TestDeliveryOnlySentMayAdvanceTextWithoutChangingStatus(t *testing.T) {
	for _, status := range []DeliveryStatus{DeliveryGenerated, DeliverySynthesizing, DeliveryTTSFailed, DeliveryPlayed, DeliveryInterrupted} {
		t.Run(string(status), func(t *testing.T) {
			database, _, sessionID, cleanup := openStoreFixture(t)
			defer cleanup()
			messageID := insertAssistantDelivery(t, database, sessionID, "完整回答")
			store := NewStore(database)
			switch status {
			case DeliverySynthesizing:
				_, _ = store.UpdateDelivery(context.Background(), messageID, DeliverySynthesizing, "")
			case DeliveryTTSFailed:
				_, _ = store.UpdateDelivery(context.Background(), messageID, DeliveryTTSFailed, "")
			case DeliveryPlayed, DeliveryInterrupted:
				_, _ = store.UpdateDelivery(context.Background(), messageID, DeliverySynthesizing, "")
				_, _ = store.UpdateDelivery(context.Background(), messageID, DeliverySent, "")
				_, _ = store.UpdateDelivery(context.Background(), messageID, status, "")
			}
			if _, err := store.UpdateDelivery(context.Background(), messageID, status, "完整"); !errors.Is(err, ErrInvalidDeliveryTransition) {
				t.Fatalf("same-state text growth err=%v", err)
			}
		})
	}

	database, _, sessionID, cleanup := openStoreFixture(t)
	defer cleanup()
	messageID := insertAssistantDelivery(t, database, sessionID, "完整回答")
	store := NewStore(database)
	if _, err := store.UpdateDelivery(context.Background(), messageID, DeliverySynthesizing, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateDelivery(context.Background(), messageID, DeliverySent, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateDelivery(context.Background(), messageID, DeliverySent, "完整"); err != nil {
		t.Fatalf("sent prefix ack err=%v", err)
	}
}

func TestDeliveryTerminalTransitionCannotAdvanceText(t *testing.T) {
	database, _, sessionID, cleanup := openStoreFixture(t)
	defer cleanup()
	messageID := insertAssistantDelivery(t, database, sessionID, "完整回答")
	if _, err := NewStore(database).UpdateDelivery(context.Background(), messageID, DeliveryTTSFailed, "完整"); !errors.Is(err, ErrInvalidDeliveryTransition) {
		t.Fatalf("tts_failed err=%v", err)
	}
}

func TestDeliveryInterruptedCapturesFinalConfirmedPrefixOnce(t *testing.T) {
	database, _, sessionID, cleanup := openStoreFixture(t)
	defer cleanup()
	messageID := insertAssistantDelivery(t, database, sessionID, "完整回答")
	store := NewStore(database)
	for _, status := range []DeliveryStatus{DeliverySynthesizing, DeliverySent} {
		if _, err := store.UpdateDelivery(context.Background(), messageID, status, ""); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.UpdateDelivery(context.Background(), messageID, DeliveryInterrupted, "完整"); err != nil {
		t.Fatalf("capture final interrupted prefix: %v", err)
	}
	if _, err := store.UpdateDelivery(context.Background(), messageID, DeliveryInterrupted, "完整回答"); !errors.Is(err, ErrInvalidDeliveryTransition) {
		t.Fatalf("interrupted terminal growth err=%v", err)
	}
}

func TestDeliveryRejectsIllegalTransitionsAndInvalidPrefixes(t *testing.T) {
	t.Run("skip generated to sent", func(t *testing.T) {
		database, _, sessionID, cleanup := openStoreFixture(t)
		defer cleanup()
		messageID := insertAssistantDelivery(t, database, sessionID, "完整回答")
		if _, err := NewStore(database).UpdateDelivery(context.Background(), messageID, DeliverySent, ""); !errors.Is(err, ErrInvalidDeliveryTransition) {
			t.Fatalf("err=%v", err)
		}
	})
	t.Run("played is terminal", func(t *testing.T) {
		database, _, sessionID, cleanup := openStoreFixture(t)
		defer cleanup()
		messageID := insertAssistantDelivery(t, database, sessionID, "完整回答")
		store := NewStore(database)
		steps := []struct {
			status DeliveryStatus
			text   string
		}{{DeliverySynthesizing, ""}, {DeliverySent, ""}, {DeliveryPlayed, "完整回答"}}
		for _, step := range steps {
			if _, err := store.UpdateDelivery(context.Background(), messageID, step.status, step.text); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := store.UpdateDelivery(context.Background(), messageID, DeliverySent, "完整回答"); !errors.Is(err, ErrInvalidDeliveryTransition) {
			t.Fatalf("err=%v", err)
		}
	})
	t.Run("interrupted cannot become played", func(t *testing.T) {
		database, _, sessionID, cleanup := openStoreFixture(t)
		defer cleanup()
		messageID := insertAssistantDelivery(t, database, sessionID, "完整回答")
		store := NewStore(database)
		for _, status := range []DeliveryStatus{DeliverySynthesizing, DeliverySent, DeliveryInterrupted} {
			if _, err := store.UpdateDelivery(context.Background(), messageID, status, ""); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := store.UpdateDelivery(context.Background(), messageID, DeliveryPlayed, ""); !errors.Is(err, ErrInvalidDeliveryTransition) {
			t.Fatalf("err=%v", err)
		}
	})

	database, _, sessionID, cleanup := openStoreFixture(t)
	defer cleanup()
	messageID := insertAssistantDelivery(t, database, sessionID, "你好世界")
	store := NewStore(database)
	if _, err := store.UpdateDelivery(context.Background(), messageID, DeliverySynthesizing, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateDelivery(context.Background(), messageID, DeliverySent, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateDelivery(context.Background(), messageID, DeliverySent, "你好"); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []string{"你", "世界", "你好世界!"} {
		if _, err := store.UpdateDelivery(context.Background(), messageID, DeliverySent, invalid); !errors.Is(err, ErrInvalidDeliveredText) {
			t.Fatalf("text=%q err=%v", invalid, err)
		}
	}
}

func TestDeliveryRejectsOrdinaryChatAndUserMessages(t *testing.T) {
	database, _, xinzhiliSessionID, cleanup := openStoreFixture(t)
	defer cleanup()
	store := NewStore(database)
	var ordinarySessionID, ordinaryAssistantID, userMessageID int64
	if err := database.QueryRow(`INSERT INTO app_chat_sessions(app_user_id, card_id, scene) SELECT app_user_id, card_id, 'chat' FROM app_chat_sessions WHERE id=$1 RETURNING id`, xinzhiliSessionID).Scan(&ordinarySessionID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRow(`INSERT INTO app_chat_messages(session_id, role, content, delivery_status) VALUES ($1,'assistant','普通回答','generated') RETURNING id`, ordinarySessionID).Scan(&ordinaryAssistantID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRow(`INSERT INTO app_chat_messages(session_id, role, content, delivery_status) VALUES ($1,'user','用户说话','generated') RETURNING id`, xinzhiliSessionID).Scan(&userMessageID); err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{ordinaryAssistantID, userMessageID} {
		if _, err := store.UpdateDelivery(context.Background(), id, DeliverySynthesizing, ""); !errors.Is(err, ErrDeliveryNotFound) {
			t.Fatalf("message=%d err=%v", id, err)
		}
	}
}

func insertAssistantDelivery(t *testing.T, database *sql.DB, sessionID int64, content string) int64 {
	t.Helper()
	var id int64
	if err := database.QueryRow(`INSERT INTO app_chat_messages(session_id, role, content, delivery_status, delivered_text, xinzhili_mode) VALUES ($1,'assistant',$2,'generated','','normal') RETURNING id`, sessionID, content).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func openStoreFixture(t *testing.T) (*sql.DB, int64, int64, func()) {
	t.Helper()
	dsn := stringsTrim(os.Getenv("TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run xinzhili store tests")
	}
	if !containsTestLoopback(dsn) {
		t.Fatal("TEST_DATABASE_URL must be a loopback isolated test database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	adminDB, err := sql.Open("pgx", dsn)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	schemaName := fmt.Sprintf("task2_xin_store_%d", time.Now().UnixNano())
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
	if _, err := database.ExecContext(ctx, task2StoreFixtureSchema); err != nil {
		t.Fatalf("create isolated fixture schema: %v", err)
	}
	var userID, cardID, sessionID int64
	phone := fmt.Sprintf("task2-store-%d", time.Now().UnixNano())
	if err := database.QueryRowContext(ctx, `INSERT INTO app_users(phone) VALUES ($1) RETURNING id`, phone).Scan(&userID); err != nil {
		database.Close()
		cancel()
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `INSERT INTO app_user_cards(app_user_id, card_type, name, relation, enneagram, wing, profile, status) VALUES ($1,'primary','测试卡','self',1,2,'{}','active') RETURNING id`, userID).Scan(&cardID); err != nil {
		database.Close()
		cancel()
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `INSERT INTO app_chat_sessions(app_user_id, card_id, scene) VALUES ($1,$2,'xinzhili_voice') RETURNING id`, userID, cardID).Scan(&sessionID); err != nil {
		database.Close()
		cancel()
		t.Fatal(err)
	}
	cleanup := func() {
		_ = database.Close()
		_, _ = adminDB.Exec(`DROP SCHEMA IF EXISTS ` + schemaName + ` CASCADE`)
		_ = adminDB.Close()
		cancel()
	}
	return database, userID, sessionID, cleanup
}

const task2StoreFixtureSchema = `
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
);
CREATE TABLE app_xinzhili_mode_preferences(
  app_user_id BIGINT PRIMARY KEY REFERENCES app_users(id) ON DELETE CASCADE,
  requested_mode TEXT NOT NULL CHECK (requested_mode IN ('normal','argument','comfort','deep_listening')),
  revision BIGINT NOT NULL CHECK (revision > 0), update_time TIMESTAMPTZ NOT NULL DEFAULT now()
);`

func stringsTrim(value string) string {
	for len(value) > 0 && (value[0] == ' ' || value[0] == '\n' || value[0] == '\t' || value[0] == '\r') {
		value = value[1:]
	}
	for len(value) > 0 {
		last := value[len(value)-1]
		if last != ' ' && last != '\n' && last != '\t' && last != '\r' {
			break
		}
		value = value[:len(value)-1]
	}
	return value
}

func containsTestLoopback(dsn string) bool {
	lower := strings.ToLower(dsn)
	return strings.Contains(lower, "test") && (strings.Contains(lower, "127.0.0.1") || strings.Contains(lower, "localhost"))
}
