package xinzhili

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
)

func TestAliyunCosyVoiceTTSWebSocketProviderSendsTaskFlow(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer tts-key" {
			t.Fatalf("Authorization=%q", got)
		}
		if got := r.Header.Get("X-DashScope-WorkSpace"); got != "workspace-1" {
			t.Fatalf("workspace header=%q", got)
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("upgrade: %v", err)
		}
		defer conn.Close()

		run := readAliyunTTSTestMessage(t, conn)
		if run.Header.Action != "run-task" || run.Header.Streaming != "duplex" || run.Header.TaskID == "" {
			t.Fatalf("run header=%+v", run.Header)
		}
		if run.Payload.TaskGroup != "audio" || run.Payload.Task != "tts" || run.Payload.Function != "SpeechSynthesizer" || run.Payload.Model != "cosyvoice-v3.5-plus" {
			t.Fatalf("run payload=%+v", run.Payload)
		}
		if run.Payload.Parameters.TextType != "PlainText" || run.Payload.Parameters.Voice != "voice-id-1" || run.Payload.Parameters.Format != "mp3" {
			t.Fatalf("run params=%+v", run.Payload.Parameters)
		}
		if err := conn.WriteJSON(map[string]any{"header": map[string]any{"event": "task-started", "task_id": run.Header.TaskID}}); err != nil {
			t.Fatalf("task-started: %v", err)
		}

		cont := readAliyunTTSTestMessage(t, conn)
		if cont.Header.Action != "continue-task" || cont.Header.TaskID != run.Header.TaskID || cont.Payload.Input.Text != "好，我是七号，不需要担心" {
			t.Fatalf("continue=%+v", cont)
		}
		finish := readAliyunTTSTestMessage(t, conn)
		if finish.Header.Action != "finish-task" || finish.Header.TaskID != run.Header.TaskID {
			t.Fatalf("finish=%+v", finish.Header)
		}
		if err := conn.WriteMessage(websocket.BinaryMessage, testMP3()); err != nil {
			t.Fatalf("audio: %v", err)
		}
		if err := conn.WriteJSON(map[string]any{"header": map[string]any{"event": "task-finished", "task_id": run.Header.TaskID}}); err != nil {
			t.Fatalf("task-finished: %v", err)
		}
	}))
	defer server.Close()

	cfg := TTSConfig{Provider: TTSProviderAliyunCosyVoice, Endpoint: "ws" + strings.TrimPrefix(server.URL, "http"), APIKey: "tts-key", GroupID: "workspace-1", Model: "cosyvoice-v3.5-plus", Voice: "voice-id-1", Format: "mp3"}
	provider, err := (TTSProviderFactory{}).New(cfg)
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	audio, mime, err := provider.Synthesize(context.Background(), cfg, "OK，我是7号，不需要 worry")
	if err != nil {
		t.Fatalf("synthesize: %v", err)
	}
	if string(audio) != string(testMP3()) || mime != "audio/mpeg" {
		t.Fatalf("audio=%x mime=%q", audio, mime)
	}
}

func TestAliyunCosyVoiceTTSRejectsInvalidMP3Audio(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("upgrade: %v", err)
		}
		defer conn.Close()
		run := readAliyunTTSTestMessage(t, conn)
		if err := conn.WriteJSON(map[string]any{"header": map[string]any{"event": "task-started", "task_id": run.Header.TaskID}}); err != nil {
			t.Fatalf("task-started: %v", err)
		}
		_ = readAliyunTTSTestMessage(t, conn)
		_ = readAliyunTTSTestMessage(t, conn)
		if err := conn.WriteMessage(websocket.BinaryMessage, []byte("not mp3")); err != nil {
			t.Fatalf("audio: %v", err)
		}
		if err := conn.WriteJSON(map[string]any{"header": map[string]any{"event": "task-finished", "task_id": run.Header.TaskID}}); err != nil {
			t.Fatalf("task-finished: %v", err)
		}
	}))
	defer server.Close()

	cfg := TTSConfig{Provider: TTSProviderAliyunCosyVoice, Endpoint: "ws" + strings.TrimPrefix(server.URL, "http"), APIKey: "tts-key", GroupID: "workspace-1", Model: "cosyvoice-v3.5-plus", Voice: "voice-id-1", Format: "mp3"}
	provider, err := (TTSProviderFactory{}).New(cfg)
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	_, _, err = provider.Synthesize(context.Background(), cfg, "你好。")
	if err == nil || !strings.Contains(err.Error(), "音频格式无效") {
		t.Fatalf("err=%v", err)
	}
}

type aliyunTTSTestMessage struct {
	Header struct {
		Action    string `json:"action"`
		TaskID    string `json:"task_id"`
		Streaming string `json:"streaming"`
	} `json:"header"`
	Payload struct {
		TaskGroup  string `json:"task_group"`
		Task       string `json:"task"`
		Function   string `json:"function"`
		Model      string `json:"model"`
		Parameters struct {
			TextType string `json:"text_type"`
			Voice    string `json:"voice"`
			Format   string `json:"format"`
		} `json:"parameters"`
		Input struct {
			Text string `json:"text"`
		} `json:"input"`
	} `json:"payload"`
}

func readAliyunTTSTestMessage(t *testing.T, conn *websocket.Conn) aliyunTTSTestMessage {
	t.Helper()
	var message aliyunTTSTestMessage
	if err := conn.ReadJSON(&message); err != nil {
		t.Fatalf("read json: %v", err)
	}
	return message
}
