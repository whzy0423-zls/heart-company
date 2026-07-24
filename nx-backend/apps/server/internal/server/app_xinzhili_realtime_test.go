package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"nine-xing/nx-backend/apps/server/internal/xinzhili"
)

func TestXinzhiliWSSinkSendAudioUsesBinaryFrame(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	serverConn := make(chan *websocket.Conn, 1)
	h := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		serverConn <- conn
	}))
	defer h.Close()
	client, _, err := websocket.DefaultDialer.Dial("ws"+h.URL[len("http"):], nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	conn := <-serverConn
	defer conn.Close()
	rc := &xinzhiliRealtimeConn{ws: conn, sessionID: "xz-test", turnKey: xinzhili.TurnKey("turn-1")}
	sink := &xinzhiliWSSink{conn: rc}
	if err := sink.SendAudio(context.Background(), xinzhili.AudioSegment{Seq: 3, Audio: []byte("mp3")}); err != nil {
		t.Fatalf("SendAudio: %v", err)
	}
	_ = client.SetReadDeadline(time.Now().Add(time.Second))
	kind, data, err := client.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	if kind != websocket.BinaryMessage {
		t.Fatalf("frame kind = %d, want binary", kind)
	}
	frame, err := xinzhili.DecodeBinaryFrame(data)
	if err != nil {
		t.Fatal(err)
	}
	if frame.FrameType != xinzhili.FrameTypeAssistantMP3 || frame.TurnKey != rc.turnKey || frame.SegmentSeq != 3 || string(frame.Payload) != "mp3" {
		t.Fatalf("unexpected frame: %+v", frame)
	}
}

func TestXinzhiliWSSinkReturnsWriteError(t *testing.T) {
	ws := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}).Upgrade(w, r, nil)
		if err == nil {
			_ = conn.Close()
		}
	}))
	defer ws.Close()
	client, _, err := websocket.DefaultDialer.Dial("ws"+ws.URL[len("http"):], nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	conn := client
	_ = conn.Close()
	rc := &xinzhiliRealtimeConn{ws: conn}
	err = (&xinzhiliWSSink{conn: rc}).SendAudio(context.Background(), xinzhili.AudioSegment{Audio: []byte("x")})
	if err == nil {
		t.Fatal("SendAudio returned nil after connection close")
	}
}
