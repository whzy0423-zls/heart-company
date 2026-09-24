package chat

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"

	"nine-xing/nx-backend/apps/server/internal/testdb"
)

func TestEnneagramDialogueIsolation(t *testing.T) {
	database, _ := testdb.OpenEnvIsolatedSchema(t, "enneagram_dialogue")
	schema, err := os.ReadFile("../db/schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(string(schema)); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO app_users(id,phone) VALUES(88001,'dialogue-fixture'); INSERT INTO app_user_cards(id,app_user_id,card_type,name,enneagram) VALUES(88001,88001,'primary','fixture',1)`); err != nil {
		t.Fatal(err)
	}
	store := NewStore(database)
	regular := context.Background()
	one := WithEnneagramType(regular, 1)
	two := WithEnneagramType(regular, 2)
	main, err := store.GetOrCreateSession(regular, 88001, 88001)
	if err != nil {
		t.Fatal(err)
	}
	a, err := store.GetOrCreateSession(one, 88001, 88001)
	if err != nil {
		t.Fatal(err)
	}
	b, err := store.GetOrCreateSession(two, 88001, 88001)
	if err != nil {
		t.Fatal(err)
	}
	again, err := store.GetOrCreateSession(one, 88001, 88001)
	if err != nil {
		t.Fatal(err)
	}
	if a.ID == b.ID || a.ID == main.ID || again.ID != a.ID {
		t.Fatalf("sessions not isolated: %d %d %d %d", main.ID, a.ID, b.ID, again.ID)
	}
	for _, ctx := range []context.Context{regular, two} {
		if _, err := store.GetSession(ctx, 88001, a.ID); !errors.Is(err, ErrNotFound) {
			t.Fatalf("foreign scene readable: %v", err)
		}
	}
	if _, err := store.GetSession(one, 88002, a.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign user readable: %v", err)
	}
	if _, err := store.SavePair(one, a.ID, "你好", "你好，一起理清这件事。", []byte("[]")); err != nil {
		t.Fatal(err)
	}
	messages, err := store.ListMessages(one, a.ID)
	if err != nil || len(messages) != 2 {
		t.Fatalf("messages=%v err=%v", messages, err)
	}
	hidden, err := store.ListMessages(two, a.ID)
	if err != nil || len(hidden) != 0 {
		t.Fatalf("messages leaked=%v err=%v", hidden, err)
	}
	sessions, err := store.ListSessions(regular, 88001)
	if err != nil || len(sessions) != 1 || sessions[0].ID != main.ID {
		t.Fatalf("main list leaked: %v %v", sessions, err)
	}
}

func TestEnneagramDialogueMessageBoundariesAndVoiceURL(t *testing.T) {
	database, _ := testdb.OpenEnvIsolatedSchema(t, "enneagram_messages")
	schema, err := os.ReadFile("../db/schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(string(schema)); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO app_users(id,phone) VALUES(88001,'dialogue-fixture'); INSERT INTO app_user_cards(id,app_user_id,card_type,name,enneagram) VALUES(88001,88001,'primary','fixture',1); INSERT INTO upload_assets(id,key,data) VALUES(88001,'voice-fixture','audio'::bytea)`); err != nil {
		t.Fatal(err)
	}
	store := NewStore(database)
	ctx := WithEnneagramType(context.Background(), 8)
	session, err := store.GetOrCreateSession(ctx, 88001, 88001)
	if err != nil {
		t.Fatal(err)
	}
	voiceID, answerID, err := store.SaveVoicePair(ctx, session.ID, 88001, 2200, "语音转写内容", "角色八的回答", nil)
	if err != nil {
		t.Fatal(err)
	}
	if favorite, err := store.ToggleFavorite(ctx, 88001, answerID); err != nil || !favorite {
		t.Fatalf("favorite=%v err=%v", favorite, err)
	}
	if err := store.SetFeedback(ctx, 88001, answerID, "helpful"); err != nil {
		t.Fatal(err)
	}
	for _, role := range []int{0, 1, 2, 3, 4, 5, 6, 7, 9} {
		other := WithEnneagramType(context.Background(), role)
		if _, err := store.GetMessageSession(other, 88001, answerID); !errors.Is(err, ErrNotFound) {
			t.Fatalf("role %d message lookup leaked: %v", role, err)
		}
		if _, err := store.ToggleFavorite(other, 88001, answerID); !errors.Is(err, ErrNotFound) {
			t.Fatalf("role %d changed favorite: %v", role, err)
		}
		if err := store.SetFeedback(other, 88001, answerID, "not_helpful"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("role %d changed feedback: %v", role, err)
		}
		if _, err := store.GetVoiceAudioAssetID(other, 88001, voiceID); !errors.Is(err, ErrNotFound) {
			t.Fatalf("role %d audio leaked: %v", role, err)
		}
		if _, err := store.GetVoiceTranscript(other, 88001, voiceID); !errors.Is(err, ErrNotFound) {
			t.Fatalf("role %d transcript leaked: %v", role, err)
		}
		if favorites, err := store.ListFavorites(other, 88001, 0); err != nil || len(favorites) != 0 {
			t.Fatalf("role %d favorite list leaked: %v %v", role, favorites, err)
		}
		if found, err := store.SearchMessages(other, 88001, 0, "角色八"); err != nil || len(found) != 0 {
			t.Fatalf("role %d search leaked: %v %v", role, found, err)
		}
	}
	if assetID, err := store.GetVoiceAudioAssetID(ctx, 88001, voiceID); err != nil || assetID != 88001 {
		t.Fatalf("owned audio=%d err=%v", assetID, err)
	}
	if transcript, err := store.GetVoiceTranscript(ctx, 88001, voiceID); err != nil || transcript != "语音转写内容" {
		t.Fatalf("owned transcript=%q err=%v", transcript, err)
	}
	if messages, err := store.ListMessages(ctx, session.ID); err != nil || len(messages) != 2 || messages[0].AudioURL != fmt.Sprintf("/api/app/enneagram/8/chat/messages/%d/audio", voiceID) {
		t.Fatalf("scoped voice messages=%+v err=%v", messages, err)
	}
}
