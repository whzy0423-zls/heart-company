package chat

import (
	"context"
	"fmt"
)

type enneagramTypeKey struct{}

// WithEnneagramType scopes existing chat operations to a server-selected role.
func WithEnneagramType(ctx context.Context, mainType int) context.Context {
	return context.WithValue(ctx, enneagramTypeKey{}, mainType)
}

func EnneagramType(ctx context.Context) int {
	value, _ := ctx.Value(enneagramTypeKey{}).(int)
	if value < 1 || value > 9 {
		return 0
	}
	return value
}

func publicChatScene(ctx context.Context) string {
	if mainType := EnneagramType(ctx); mainType > 0 {
		return fmt.Sprintf("enneagram_%d", mainType)
	}
	return "chat"
}

// The scene is derived only from a bounded integer, never request SQL text.
func publicChatSceneSQL(ctx context.Context) string {
	return "'" + publicChatScene(ctx) + "'"
}

func VoiceAudioURL(ctx context.Context, messageID int64) string {
	if mainType := EnneagramType(ctx); mainType > 0 {
		return fmt.Sprintf("/api/app/enneagram/%d/chat/messages/%d/audio", mainType, messageID)
	}
	return fmt.Sprintf("/api/app/chat/messages/%d/audio", messageID)
}
