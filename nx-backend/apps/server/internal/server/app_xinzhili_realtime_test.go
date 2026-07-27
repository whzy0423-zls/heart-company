package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"nine-xing/nx-backend/apps/server/internal/xinzhili"
)

func TestXinzhiliProtocolFailureIsSanitized(t *testing.T) {
	originalWriter, originalFlags := log.Writer(), log.Flags()
	var logs bytes.Buffer
	log.SetOutput(&logs)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(originalWriter)
		log.SetFlags(originalFlags)
	})

	data := []byte(`{"protocolVersion":"xinzhili.voice.v1","type":"session.ready","timestampMs":1,"accessToken":"TOKEN_SECRET","payload":{"appBuild":106,"clientCapabilities":["generation"],"transcript":"PRIVATE_TEXT"}}`)
	_, protocolErr := xinzhili.DecodeEnvelope(data, xinzhili.DirectionClient, false)
	logXinzhiliProtocolError(data, protocolErr)

	got := logs.String()
	for _, required := range []string{"protocol_version=\"xinzhili.voice.v1\"", "event_type=\"session.ready\"", "error_code=\"invalid_event_direction\"", "app_build=106", "client_capabilities=\"generation\""} {
		if !strings.Contains(got, required) {
			t.Errorf("log missing %q: %s", required, got)
		}
	}
	for _, secret := range []string{"TOKEN_SECRET", "PRIVATE_TEXT"} {
		if strings.Contains(got, secret) {
			t.Errorf("log leaked %q: %s", secret, got)
		}
	}
	if xinzhiliProtocolUpdateMessage != "语音服务正在更新，请稍后重试" {
		t.Fatalf("unexpected user message %q", xinzhiliProtocolUpdateMessage)
	}
}

type fakeXinzhiliModeStore struct {
	preference xinzhili.ModePreference
	found      bool
	readErr    error
	updateErr  error
	updates    []xinzhili.ModePreference
}

func (s *fakeXinzhiliModeStore) ReadMode(context.Context, int64) (xinzhili.ModePreference, bool, error) {
	return s.preference, s.found, s.readErr
}

func (s *fakeXinzhiliModeStore) UpdateMode(_ context.Context, userID int64, mode xinzhili.Mode, expectedRevision int64) (xinzhili.ModePreference, error) {
	if s.updateErr != nil {
		return xinzhili.ModePreference{}, s.updateErr
	}
	p := xinzhili.ModePreference{UserID: userID, Requested: mode, Revision: expectedRevision + 1}
	s.updates = append(s.updates, p)
	return p, nil
}

func TestXinzhiliModeSnapshotFallsBackWhenStoredModeIsDisabled(t *testing.T) {
	store := &fakeXinzhiliModeStore{found: true, preference: xinzhili.ModePreference{UserID: 7, Requested: xinzhili.ModeArgument, Revision: 4}}
	c := &xinzhiliRealtimeConn{userID: 7, modeStore: store}

	snapshot, err := c.loadModeSnapshot(context.Background(), xinzhili.Config{Version: 12, EnabledModes: []xinzhili.Mode{xinzhili.ModeNormal, xinzhili.ModeComfort}})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.RequestedMode != xinzhili.ModeNormal || snapshot.PendingMode != xinzhili.ModeNormal || snapshot.EffectiveMode != xinzhili.ModeNormal {
		t.Fatalf("disabled stored mode was not normalized: %+v", snapshot)
	}
	if snapshot.Revision != 4 || snapshot.ConfigVersion != 12 {
		t.Fatalf("version fields = %+v", snapshot)
	}
	if len(snapshot.EnabledModes) != 2 || snapshot.EnabledModes[1] != xinzhili.ModeComfort {
		t.Fatalf("enabled modes = %#v", snapshot.EnabledModes)
	}
}

func TestXinzhiliChangeModeRejectsDisabledMode(t *testing.T) {
	store := &fakeXinzhiliModeStore{}
	c := &xinzhiliRealtimeConn{userID: 7, modeStore: store, requestedMode: xinzhili.ModeNormal, effectiveMode: xinzhili.ModeNormal, modeRevision: 2}

	_, err := c.persistModeChange(context.Background(), xinzhili.Config{Version: 3, EnabledModes: []xinzhili.Mode{xinzhili.ModeNormal}}, xinzhili.ModeArgument, 2)
	if !errors.Is(err, errXinzhiliModeDisabled) {
		t.Fatalf("error = %v, want disabled mode", err)
	}
	if len(store.updates) != 0 {
		t.Fatalf("disabled mode reached store: %+v", store.updates)
	}
}

func TestXinzhiliChangeModePersistsPendingWithoutChangingEffectiveMode(t *testing.T) {
	store := &fakeXinzhiliModeStore{}
	c := &xinzhiliRealtimeConn{userID: 7, modeStore: store, requestedMode: xinzhili.ModeNormal, pendingMode: xinzhili.ModeNormal, effectiveMode: xinzhili.ModeNormal, modeRevision: 2}
	cfg := xinzhili.Config{Version: 9, EnabledModes: []xinzhili.Mode{xinzhili.ModeNormal, xinzhili.ModeComfort}}

	snapshot, err := c.persistModeChange(context.Background(), cfg, xinzhili.ModeComfort, 2)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.RequestedMode != xinzhili.ModeComfort || snapshot.PendingMode != xinzhili.ModeComfort || snapshot.EffectiveMode != xinzhili.ModeNormal {
		t.Fatalf("unexpected mode snapshot: %+v", snapshot)
	}
	if snapshot.Revision != 3 || snapshot.ConfigVersion != 9 {
		t.Fatalf("unexpected versions: %+v", snapshot)
	}
}

func TestXinzhiliModeSnapshotJSONContract(t *testing.T) {
	payload, err := json.Marshal(xinzhiliModeSnapshot{
		EnabledModes:  []xinzhili.Mode{xinzhili.ModeNormal, xinzhili.ModeDeepListening},
		RequestedMode: xinzhili.ModeDeepListening,
		PendingMode:   xinzhili.ModeDeepListening,
		EffectiveMode: xinzhili.ModeNormal,
		Revision:      5,
		ConfigVersion: 11,
	})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"enabledModes", "requestedMode", "pendingMode", "effectiveMode", "revision", "configVersion"} {
		if _, ok := got[field]; !ok {
			t.Fatalf("missing wire field %q in %s", field, payload)
		}
	}
}

func TestXinzhiliWSSinkSendAudioUsesConnectionGeneration(t *testing.T) {
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
	rc := &xinzhiliRealtimeConn{ws: conn, sessionID: "xz-test", generation: 7, turnKey: xinzhili.TurnKey("turn-1")}
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
	if frame.Generation != 7 || frame.FrameType != xinzhili.FrameTypeAssistantMP3 || frame.TurnKey != rc.turnKey || frame.SegmentSeq != 3 || string(frame.Payload) != "mp3" {
		t.Fatalf("unexpected frame: %+v", frame)
	}
}

func TestXinzhiliWSSinkSendControlUsesConnectionGeneration(t *testing.T) {
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
	rc := &xinzhiliRealtimeConn{ws: conn, sessionID: "xz-test", generation: 9}
	sink := &xinzhiliWSSink{conn: rc}
	if err := sink.SendControl(context.Background(), xinzhili.Envelope{
		ProtocolVersion: xinzhili.ProtocolVersion,
		Type:            xinzhili.EventSessionReady,
		Payload:         json.RawMessage(`{}`),
	}); err != nil {
		t.Fatalf("SendControl: %v", err)
	}
	_ = client.SetReadDeadline(time.Now().Add(time.Second))
	kind, data, err := client.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	if kind != websocket.TextMessage {
		t.Fatalf("frame kind = %d, want text", kind)
	}
	envelope, err := xinzhili.DecodeEnvelope(data, xinzhili.DirectionServer, true)
	if err != nil {
		t.Fatal(err)
	}
	if envelope.Generation != 9 {
		t.Fatalf("generation = %d, want 9", envelope.Generation)
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
