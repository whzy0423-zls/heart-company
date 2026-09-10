package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"nine-xing/nx-backend/apps/server/internal/directmessage"
)

func TestDirectMessageRoutesAreRegistered(t *testing.T) {
	raw, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, route := range []string{"/api/app/direct/conversations", "/api/app/direct/messages/"} {
		if !strings.Contains(source, `s.mux.HandleFunc("`+route+`"`) {
			t.Fatalf("missing route %s", route)
		}
	}
}

func TestAppDirectMessageRouterRequiresAuthentication(t *testing.T) {
	server := &Server{}
	response := httptest.NewRecorder()
	server.appDirectMessageRouter(response, httptest.NewRequest(http.MethodGet, "/api/app/direct/conversations", nil))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthorized, got %d", response.Code)
	}
}

func TestMapDirectMessageErrorUsesStableStatuses(t *testing.T) {
	cases := []struct {
		err    error
		status int
	}{{directmessage.ErrCursorConflict, http.StatusBadRequest}, {directmessage.ErrPayloadConflict, http.StatusConflict}, {directmessage.ErrBlocked, http.StatusForbidden}, {directmessage.ErrRecallWindow, http.StatusUnprocessableEntity}, {errors.New("db"), http.StatusInternalServerError}}
	for _, tc := range cases {
		response := httptest.NewRecorder()
		mapDirectMessageError(response, tc.err)
		if response.Code != tc.status {
			t.Errorf("%v: expected %d got %d", tc.err, tc.status, response.Code)
		}
	}
}

func TestDirectReadEventIncludesReaderAndSequence(t *testing.T) {
	event := directReadEvent(12, 7, 31)
	if event["type"] != "read" {
		t.Fatalf("unexpected event type %#v", event)
	}
	data, _ := event["data"].(map[string]any)
	if data["conversationId"] != int64(12) || data["userId"] != int64(7) || data["sequence"] != int64(31) {
		t.Fatalf("unexpected read data %#v", data)
	}
}

func TestDirectMessageNotificationPayload(t *testing.T) {
	tests := []struct {
		name        string
		messageType string
		body        string
		wantContent string
	}{
		{name: "text", messageType: "text", body: "  最近怎么样？  ", wantContent: "最近怎么样？"},
		{name: "image", messageType: "image", wantContent: "[图片]"},
		{name: "video", messageType: "video", wantContent: "[视频]"},
		{name: "voice", messageType: "voice", wantContent: "[语音]"},
		{name: "sticker", messageType: "sticker", body: "😀", wantContent: "😀"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			message := directmessage.Message{ID: 99, ConversationID: 12, MessageType: tc.messageType, Body: tc.body}
			title, content, deepLink, source := directMessageNotificationPayload(message, "  小明  ")
			if title != "小明" || content != tc.wantContent || deepLink != "/direct/12" || source != "direct-message:99" {
				t.Fatalf("unexpected payload title=%q content=%q deepLink=%q source=%q", title, content, deepLink, source)
			}
		})
	}
}

func TestDirectMessageNotificationPayloadUsesSafeFallbacksAndRuneTruncation(t *testing.T) {
	message := directmessage.Message{ID: 7, ConversationID: 3, MessageType: "text", Body: strings.Repeat("九", 90)}
	title, content, _, _ := directMessageNotificationPayload(message, "")
	if title != "好友" {
		t.Fatalf("expected fallback title, got %q", title)
	}
	if got := len([]rune(content)); got != 81 || !strings.HasSuffix(content, "…") {
		t.Fatalf("expected 80-rune preview plus ellipsis, got %d runes: %q", got, content)
	}
}

func TestShouldNotifyDirectMessageOnlyOnceForRecipient(t *testing.T) {
	if !shouldNotifyDirectMessage(directmessage.Message{ID: 1, RecipientID: 9, WasCreated: true}) {
		t.Fatal("newly-created message with recipient should notify")
	}
	if shouldNotifyDirectMessage(directmessage.Message{ID: 1, RecipientID: 9, WasCreated: false}) {
		t.Fatal("idempotent retry must not notify twice")
	}
	if shouldNotifyDirectMessage(directmessage.Message{ID: 1, WasCreated: true}) {
		t.Fatal("message without recipient must not notify")
	}
}

func TestNotifyDirectMessageCreatesRecipientInboxItem(t *testing.T) {
	inbox := &fakeAppNotificationService{}
	server := &Server{appNotifications: inbox}
	message := directmessage.Message{
		ID:             41,
		ConversationID: 12,
		RecipientID:    9,
		WasCreated:     true,
		MessageType:    "image",
	}

	server.notifyDirectMessage(context.Background(), message, "小明")

	if len(inbox.createdUsers) != 1 {
		t.Fatalf("expected one inbox notification, got %d", len(inbox.createdUsers))
	}
	created := inbox.createdUsers[0]
	if created.userID != 9 || created.kind != "direct_message" || created.title != "小明" || created.content != "[图片]" || created.link != "/direct/12" || created.source != "direct-message:41" {
		t.Fatalf("unexpected inbox notification: %#v", created)
	}

	server.notifyDirectMessage(context.Background(), directmessage.Message{
		ID:          41,
		RecipientID: 9,
		WasCreated:  false,
	}, "小明")
	if len(inbox.createdUsers) != 1 {
		t.Fatalf("idempotent retry created another notification: %#v", inbox.createdUsers)
	}
}
