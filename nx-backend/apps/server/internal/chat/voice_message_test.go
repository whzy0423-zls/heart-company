package chat

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestVoiceMessageJSONHidesTranscriptAndAssetID(t *testing.T) {
	message := Message{
		ID:              7,
		Role:            "user",
		MessageType:     "voice",
		AudioAssetID:    88,
		AudioDurationMs: 3200,
		AudioURL:        "/chat/messages/7/audio",
		Transcript:      "这是后台隐藏的转写",
	}

	raw, err := json.Marshal(message)
	if err != nil {
		t.Fatal(err)
	}
	encoded := string(raw)
	for _, hidden := range []string{"这是后台隐藏的转写", "audioAssetId", "transcript"} {
		if strings.Contains(encoded, hidden) {
			t.Fatalf("voice message JSON leaked %q: %s", hidden, encoded)
		}
	}
	for _, visible := range []string{`"messageType":"voice"`, `"audioDurationMs":3200`, `"audioUrl":"/chat/messages/7/audio"`} {
		if !strings.Contains(encoded, visible) {
			t.Fatalf("voice message JSON missing %q: %s", visible, encoded)
		}
	}
}

func TestVoiceMessageEffectiveContentUsesHiddenTranscript(t *testing.T) {
	message := Message{Role: "user", MessageType: "voice", Content: "", Transcript: "孩子最近不愿意沟通"}
	if got := message.EffectiveContent(); got != "孩子最近不愿意沟通" {
		t.Fatalf("EffectiveContent() = %q", got)
	}

	text := Message{Role: "user", MessageType: "text", Content: "普通文字", Transcript: "不应使用"}
	if got := text.EffectiveContent(); got != "普通文字" {
		t.Fatalf("text EffectiveContent() = %q", got)
	}
}
