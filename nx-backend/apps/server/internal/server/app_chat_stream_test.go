package server

import (
	"bytes"
	"strings"
	"testing"
)

func TestWriteAppChatSSEWritesDeltaEventAndFlushes(t *testing.T) {
	var body bytes.Buffer
	flusher := &appChatStreamTestFlusher{}

	if err := writeAppChatSSE(&body, flusher, "delta", map[string]string{"content": "hello"}); err != nil {
		t.Fatalf("writeAppChatSSE returned error: %v", err)
	}

	got := body.String()
	if !strings.Contains(got, "event: delta\n") {
		t.Fatalf("SSE output missing delta event line: %q", got)
	}
	if !strings.Contains(got, `data: {"content":"hello"}`+"\n\n") {
		t.Fatalf("SSE output missing JSON data line: %q", got)
	}
	if flusher.flushes != 1 {
		t.Fatalf("Flush called %d times, want 1", flusher.flushes)
	}
}

func TestAppChatSessionIDFromPathParsesStreamAndAskSuffixes(t *testing.T) {
	id, ok := appChatSessionIDFromPath("/api/app/chat/sessions/1/ask/stream", "/ask/stream")
	if !ok || id != 1 {
		t.Fatalf("stream path parsed as id=%d ok=%v, want id=1 ok=true", id, ok)
	}

	id, ok = appChatSessionIDFromPath("/api/app/chat/sessions/2/ask", "/ask")
	if !ok || id != 2 {
		t.Fatalf("ask path parsed as id=%d ok=%v, want id=2 ok=true", id, ok)
	}
}

func TestAppChatSessionIDFromPathRejectsInvalidPaths(t *testing.T) {
	invalidPaths := []string{
		"/api/app/chat/sessions/not-a-number/ask/stream",
		"/api/app/chat/sessions/0/ask/stream",
		"1/ask/stream",
		"/api/app/chat/session/1/ask/stream",
		"/api/app/chat/sessions/1/ask/stream/extra",
	}

	for _, path := range invalidPaths {
		if id, ok := appChatSessionIDFromPath(path, "/ask/stream"); ok {
			t.Fatalf("appChatSessionIDFromPath(%q) = id=%d ok=true, want ok=false", path, id)
		}
	}
}

type appChatStreamTestFlusher struct {
	flushes int
}

func (f *appChatStreamTestFlusher) Flush() {
	f.flushes++
}
