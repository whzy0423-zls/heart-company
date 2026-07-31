package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"nine-xing/nx-backend/apps/server/internal/bailianconfig"
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

type recordingXinzhiliTurnSession struct {
	pcm          []xinzhili.PCMFrame
	starts       []xinzhili.StartTurnInput
	pushErr      error
	pushErrs     []error
	startEntered chan struct{}
	startRelease <-chan struct{}
}

func (s *recordingXinzhiliTurnSession) StartTurn(_ context.Context, input xinzhili.StartTurnInput) error {
	s.starts = append(s.starts, input)
	if s.startEntered != nil {
		close(s.startEntered)
	}
	if s.startRelease != nil {
		<-s.startRelease
	}
	return nil
}
func (s *recordingXinzhiliTurnSession) PushPCM(_ context.Context, frame xinzhili.PCMFrame) error {
	s.pcm = append(s.pcm, frame)
	if len(s.pushErrs) > 0 {
		err := s.pushErrs[0]
		s.pushErrs = s.pushErrs[1:]
		return err
	}
	return s.pushErr
}
func (s *recordingXinzhiliTurnSession) HandlePlaybackAck(context.Context, xinzhili.PlaybackAck) error {
	return nil
}
func (s *recordingXinzhiliTurnSession) Interrupt(context.Context, string) error { return nil }
func (s *recordingXinzhiliTurnSession) Cancel(context.Context, string) error    { return nil }
func (s *recordingXinzhiliTurnSession) Close() error                            { return nil }

func newXinzhiliWebsocketPair(t *testing.T) (*websocket.Conn, *websocket.Conn) {
	t.Helper()
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
	t.Cleanup(h.Close)
	client, _, err := websocket.DefaultDialer.Dial("ws"+h.URL[len("http"):], nil)
	if err != nil {
		t.Fatal(err)
	}
	server := <-serverConn
	t.Cleanup(func() {
		_ = client.Close()
		_ = server.Close()
	})
	return server, client
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

func TestXinzhiliRuntimeCredentialRefreshesSharedBailianKeyForEveryTurn(t *testing.T) {
	serverWS, _ := newXinzhiliWebsocketPair(t)
	cfg := validBailianXinzhiliModelConfigForHandler()
	cfg.Version = 8
	configStore := &fakeXinzhiliModelConfigStore{config: cfg, found: true}
	credentials := &memoryBailianCredentialStore{
		cfg: bailianconfig.Config{Version: 1, APIKey: "sk-shared-turn-1"}, found: true,
	}
	session := &recordingXinzhiliTurnSession{}
	c := &xinzhiliRealtimeConn{
		server: &Server{
			xinzhiliModelConfig: configStore,
			bailianCredentials:  credentials,
		},
		ws: serverWS, sess: session, userID: 17, sessionID: "xz-runtime",
		pendingMode: xinzhili.ModeNormal, turns: make(map[uint64]string), audioSeq: make(map[uint64]uint32),
	}
	c.sink = &xinzhiliWSSink{conn: c}

	startXinzhiliRuntimeCredentialTurn(t, c, "turn-1", 101)
	credentials.mu.Lock()
	credentials.cfg = bailianconfig.Config{Version: 2, APIKey: "sk-shared-turn-2"}
	credentials.mu.Unlock()
	startXinzhiliRuntimeCredentialTurn(t, c, "turn-2", 102)

	if len(session.starts) != 2 {
		t.Fatalf("started turns=%d want=2", len(session.starts))
	}
	for i, want := range []string{"sk-shared-turn-1", "sk-shared-turn-2"} {
		got := session.starts[i]
		if got.ASRConfig.APIKey != want || got.TTSConfig.APIKey != want {
			t.Fatalf("turn %d runtime keys ASR=%q TTS=%q want=%q", i+1, got.ASRConfig.APIKey, got.TTSConfig.APIKey, want)
		}
	}
	if configStore.config.RealtimeASR.APIKey != "" || configStore.config.TTS.APIKey != "" {
		t.Fatalf("runtime key was written back to persisted config: %+v", configStore.config)
	}
}

func TestXinzhiliRuntimeCredentialNeverOverwritesMiniMaxPrivateTTSKey(t *testing.T) {
	serverWS, _ := newXinzhiliWebsocketPair(t)
	cfg := validXinzhiliModelConfigForHandler()
	cfg.Version = 9
	cfg.RealtimeASR.APIKey = ""
	cfg.TTS = xinzhili.TTSConfig{
		Provider: xinzhili.TTSProviderMiniMax,
		Endpoint: "https://api.minimax.chat/v1/t2a_v2",
		APIKey:   "minimax-private",
		GroupID:  "minimax-group",
		Model:    "speech-02-hd",
		Voice:    "minimax-voice",
		Format:   "mp3",
	}
	configStore := &fakeXinzhiliModelConfigStore{config: cfg, found: true}
	session := &recordingXinzhiliTurnSession{}
	c := &xinzhiliRealtimeConn{
		server: &Server{
			xinzhiliModelConfig: configStore,
			bailianCredentials: &memoryBailianCredentialStore{
				cfg: bailianconfig.Config{Version: 6, APIKey: "sk-shared-asr"}, found: true,
			},
		},
		ws: serverWS, sess: session, userID: 18, sessionID: "xz-minimax",
		pendingMode: xinzhili.ModeNormal, turns: make(map[uint64]string), audioSeq: make(map[uint64]uint32),
	}
	c.sink = &xinzhiliWSSink{conn: c}

	startXinzhiliRuntimeCredentialTurn(t, c, "turn-minimax", 201)
	if len(session.starts) != 1 {
		t.Fatalf("started turns=%d want=1", len(session.starts))
	}
	got := session.starts[0]
	if got.ASRConfig.APIKey != "sk-shared-asr" || got.TTSConfig.APIKey != "minimax-private" {
		t.Fatalf("runtime credential separation failed: ASR=%q TTS=%q", got.ASRConfig.APIKey, got.TTSConfig.APIKey)
	}
}

func TestXinzhiliStartTurnAcceptsSignedDartTurnKey(t *testing.T) {
	serverWS, _ := newXinzhiliWebsocketPair(t)
	cfg := validXinzhiliModelConfigForHandler()
	cfg.Version = 9
	configStore := &fakeXinzhiliModelConfigStore{config: cfg, found: true}
	session := &recordingXinzhiliTurnSession{}
	c := &xinzhiliRealtimeConn{
		server: &Server{xinzhiliModelConfig: configStore},
		ws:     serverWS, sess: session, userID: 18, sessionID: "xz-signed-turn-key",
		pendingMode: xinzhili.ModeNormal, turns: make(map[uint64]string), audioSeq: make(map[uint64]uint32),
	}
	c.sink = &xinzhiliWSSink{conn: c}

	turnID := "turn-negative-0"
	c.startTurn(context.Background(), xinzhili.Envelope{
		TurnID:  &turnID,
		Payload: json.RawMessage(`{"turnKey":-907766470923855312}`),
	})

	if len(session.starts) != 1 {
		t.Fatalf("started turns=%d want=1", len(session.starts))
	}
	want := xinzhili.TurnKey(turnID)
	if c.turnKey != want || c.turns[want] != turnID {
		t.Fatalf("turn state key=%d turns=%v want=%d", c.turnKey, c.turns, want)
	}
	if session.starts[0].TurnKey != want {
		t.Fatalf("session turn key=%d want=%d", session.starts[0].TurnKey, want)
	}
}

func TestXinzhiliRuntimeCredentialRejectsNonOfficialASREndpoint(t *testing.T) {
	for _, endpoint := range []string{
		"wss://asr.example.com/api-ws/v1/inference",
		"wss://dashscope.aliyuncs.com@evil.test/api-ws/v1/inference",
		"wss://dashscope.aliyuncs.com:8443/api-ws/v1/inference",
		"wss://dashscope.aliyuncs.com.evil.test/api-ws/v1/inference",
		"wss://dashscope.aliyuncs.com/api-ws/v1/other",
	} {
		cfg := validBailianXinzhiliModelConfigForHandler()
		cfg.RealtimeASR.Endpoint = endpoint
		s := &Server{bailianCredentials: &memoryBailianCredentialStore{
			cfg: bailianconfig.Config{Version: 4, APIKey: "sk-shared"}, found: true,
		}}
		runtime, err := s.withXinzhiliRuntimeCredentials(context.Background(), cfg)
		if err == nil {
			t.Fatalf("endpoint %q received shared runtime credentials: %+v", endpoint, runtime)
		}
		if runtime.RealtimeASR.APIKey != "" || runtime.TTS.APIKey != "" {
			t.Fatalf("endpoint %q leaked shared credential: %+v", endpoint, runtime)
		}
	}
}

func TestXinzhiliRuntimeCredentialTreatsCustomNativeBailianAsPrivateTTS(t *testing.T) {
	cfg := validBailianXinzhiliModelConfigForHandler()
	cfg.TTS.Provider = xinzhili.TTSProviderBailian
	cfg.TTS.Endpoint = "https://bailian-proxy.example/api/v1"
	cfg.TTS.APIKey = ""
	s := &Server{bailianCredentials: &memoryBailianCredentialStore{
		cfg: bailianconfig.Config{Version: 5, APIKey: "sk-shared"}, found: true,
	}}
	if runtime, err := s.withXinzhiliRuntimeCredentials(context.Background(), cfg); err == nil {
		t.Fatalf("custom native Bailian without private key received shared credential: %+v", runtime)
	}

	cfg.TTS.APIKey = "proxy-private-key"
	runtime, err := s.withXinzhiliRuntimeCredentials(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.RealtimeASR.APIKey != "sk-shared" || runtime.TTS.APIKey != "proxy-private-key" {
		t.Fatalf("runtime credentials ASR=%q TTS=%q", runtime.RealtimeASR.APIKey, runtime.TTS.APIKey)
	}
}

func startXinzhiliRuntimeCredentialTurn(t *testing.T, c *xinzhiliRealtimeConn, turnID string, turnKey uint64) {
	t.Helper()
	c.startTurn(context.Background(), xinzhili.Envelope{
		TurnID:  &turnID,
		Payload: json.RawMessage(fmt.Sprintf(`{"turnKey":%d}`, turnKey)),
	})
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

func TestXinzhiliConfigChangedKeepsActiveEffectiveModeUntilNextTurnStarts(t *testing.T) {
	serverWS, clientWS := newXinzhiliWebsocketPair(t)
	cfg := validXinzhiliModelConfigForHandler()
	cfg.Version = 12
	cfg.EnabledModes = []xinzhili.Mode{xinzhili.ModeNormal}
	store := &fakeXinzhiliModelConfigStore{config: cfg, found: true}
	session := &recordingXinzhiliTurnSession{}
	c := &xinzhiliRealtimeConn{
		server: &Server{xinzhiliModelConfig: store}, ws: serverWS, sess: session,
		userID: 7, sessionID: "xz-transition", generation: 3, configVersion: 11,
		requestedMode: xinzhili.ModeArgument, pendingMode: xinzhili.ModeArgument,
		effectiveMode: xinzhili.ModeArgument, modeRevision: 5, turnKey: 71,
		turns: map[uint64]string{71: "turn-old"}, audioSeq: map[uint64]uint32{},
	}
	c.sink = &xinzhiliWSSink{conn: c}

	if err := c.applyXinzhiliConfigChanged(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	if c.requestedMode != xinzhili.ModeNormal || c.pendingMode != xinzhili.ModeNormal || c.effectiveMode != xinzhili.ModeArgument {
		t.Fatalf("active transition modes requested=%s pending=%s effective=%s", c.requestedMode, c.pendingMode, c.effectiveMode)
	}
	readXinzhiliConfigChanged(t, clientWS, xinzhili.ModeArgument, 12)

	turnID := "turn-new"
	c.startTurn(context.Background(), xinzhili.Envelope{TurnID: &turnID, Payload: json.RawMessage(`{"turnKey":72}`)})
	if len(session.starts) != 1 || session.starts[0].Mode != xinzhili.ModeNormal {
		t.Fatalf("starts=%+v", session.starts)
	}
	if c.effectiveMode != xinzhili.ModeNormal {
		t.Fatalf("effective mode=%s want normal after successful next turn", c.effectiveMode)
	}
	readXinzhiliConfigChanged(t, clientWS, xinzhili.ModeNormal, 12)
}

func TestXinzhiliStartTurnRefreshesMissedHigherConfigVersionBeforeStarting(t *testing.T) {
	serverWS, clientWS := newXinzhiliWebsocketPair(t)
	cfg := validXinzhiliModelConfigForHandler()
	cfg.Version = 22
	cfg.EnabledModes = []xinzhili.Mode{xinzhili.ModeNormal}
	store := &fakeXinzhiliModelConfigStore{config: cfg, found: true}
	session := &recordingXinzhiliTurnSession{}
	c := &xinzhiliRealtimeConn{
		server: &Server{xinzhiliModelConfig: store}, ws: serverWS, sess: session,
		userID: 8, sessionID: "xz-version-fallback", generation: 4, configVersion: 21,
		requestedMode: xinzhili.ModeArgument, pendingMode: xinzhili.ModeArgument,
		effectiveMode: xinzhili.ModeArgument, modeRevision: 2,
		turns: map[uint64]string{}, audioSeq: map[uint64]uint32{},
	}
	c.sink = &xinzhiliWSSink{conn: c}
	turnID := "turn-fallback"
	c.startTurn(context.Background(), xinzhili.Envelope{TurnID: &turnID, Payload: json.RawMessage(`{"turnKey":91}`)})

	readXinzhiliConfigChanged(t, clientWS, xinzhili.ModeNormal, 22)
	if c.configVersion != 22 || len(session.starts) != 1 || session.starts[0].Mode != xinzhili.ModeNormal {
		t.Fatalf("version=%d starts=%+v", c.configVersion, session.starts)
	}
}

func TestXinzhiliStartTurnDoesNotOverwriteNewConfigStateWhenSaveInterleaves(t *testing.T) {
	serverWS, clientWS := newXinzhiliWebsocketPair(t)
	oldConfig := validXinzhiliModelConfigForHandler()
	oldConfig.Version = 31
	oldConfig.EnabledModes = []xinzhili.Mode{xinzhili.ModeNormal, xinzhili.ModeArgument}
	store := &fakeXinzhiliModelConfigStore{config: oldConfig, found: true}
	startEntered := make(chan struct{})
	startRelease := make(chan struct{})
	session := &recordingXinzhiliTurnSession{startEntered: startEntered, startRelease: startRelease}
	c := &xinzhiliRealtimeConn{
		server: &Server{xinzhiliModelConfig: store}, ws: serverWS, sess: session,
		userID: 10, sessionID: "xz-config-interleave", generation: 5, configVersion: 31,
		requestedMode: xinzhili.ModeArgument, pendingMode: xinzhili.ModeArgument,
		effectiveMode: xinzhili.ModeArgument, modeRevision: 4,
		turns: map[uint64]string{}, audioSeq: map[uint64]uint32{},
	}
	c.sink = &xinzhiliWSSink{conn: c}

	turnID := "turn-old-config"
	startDone := make(chan struct{})
	go func() {
		defer close(startDone)
		c.startTurn(context.Background(), xinzhili.Envelope{TurnID: &turnID, Payload: json.RawMessage(`{"turnKey":101}`)})
	}()
	<-startEntered

	newConfig := oldConfig
	newConfig.Version = 32
	newConfig.EnabledModes = []xinzhili.Mode{xinzhili.ModeNormal}
	applyDone := make(chan error, 1)
	go func() { applyDone <- c.applyXinzhiliConfigChanged(context.Background(), newConfig) }()

	// Force the newer config to finish applying while the old StartTurn is
	// blocked, then let that stale StartTurn finish its state commit.
	applyErr := <-applyDone
	close(startRelease)
	<-startDone
	if applyErr != nil {
		t.Fatal(applyErr)
	}

	if c.configVersion != 32 || c.requestedMode != xinzhili.ModeNormal || c.pendingMode != xinzhili.ModeNormal || c.effectiveMode != xinzhili.ModeArgument {
		t.Fatalf("version=%d requested=%s pending=%s effective=%s", c.configVersion, c.requestedMode, c.pendingMode, c.effectiveMode)
	}
	if len(session.starts) != 1 || session.starts[0].Mode != xinzhili.ModeArgument {
		t.Fatalf("active turn snapshot=%+v", session.starts)
	}
	readXinzhiliConfigChanged(t, clientWS, xinzhili.ModeNormal, 32)
	readXinzhiliConfigChanged(t, clientWS, xinzhili.ModeArgument, 32)
}

func readXinzhiliConfigChanged(t *testing.T, client *websocket.Conn, effective xinzhili.Mode, version int64) {
	t.Helper()
	_ = client.SetReadDeadline(time.Now().Add(time.Second))
	kind, data, err := client.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	if kind != websocket.TextMessage {
		t.Fatalf("frame kind=%d want text", kind)
	}
	var envelope xinzhili.Envelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		t.Fatal(err)
	}
	var snapshot xinzhiliModeSnapshot
	if err := json.Unmarshal(envelope.Payload, &snapshot); err != nil {
		t.Fatal(err)
	}
	if envelope.Type != xinzhili.EventConfigChanged || envelope.ConfigVersion != version ||
		snapshot.ConfigVersion != version || snapshot.RequestedMode != xinzhili.ModeNormal ||
		snapshot.PendingMode != xinzhili.ModeNormal || snapshot.EffectiveMode != effective {
		t.Fatalf("event=%s envelopeVersion=%d snapshot=%+v body=%s", envelope.Type, envelope.ConfigVersion, snapshot, data)
	}
}

func TestXinzhiliWSSinkSendAudioUsesSegmentTurnKeyWhenConnectionAdvances(t *testing.T) {
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
	oldTurnKey := xinzhili.TurnKey("turn-1")
	rc := &xinzhiliRealtimeConn{ws: conn, sessionID: "xz-test", generation: 7, turnKey: xinzhili.TurnKey("turn-2")}
	sink := &xinzhiliWSSink{conn: rc}
	if err := sink.SendAudio(context.Background(), xinzhili.AudioSegment{TurnKey: oldTurnKey, Seq: 3, Audio: []byte("mp3")}); err != nil {
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
	if frame.Generation != 7 || frame.FrameType != xinzhili.FrameTypeAssistantMP3 || frame.TurnKey != oldTurnKey || frame.SegmentSeq != 3 || string(frame.Payload) != "mp3" {
		t.Fatalf("unexpected frame: %+v", frame)
	}
}

func TestXinzhiliWSSinkSendControlUsesConnectionGenerationAndMonotonicSequence(t *testing.T) {
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
	for _, kind := range []xinzhili.EventType{xinzhili.EventSessionReady, xinzhili.EventConfigChanged} {
		if err := sink.SendControl(context.Background(), xinzhili.Envelope{
			ProtocolVersion: xinzhili.ProtocolVersion,
			Type:            kind,
			Payload:         json.RawMessage(`{}`),
		}); err != nil {
			t.Fatalf("SendControl(%s): %v", kind, err)
		}
	}
	_ = client.SetReadDeadline(time.Now().Add(time.Second))
	for wantSeq := uint64(0); wantSeq < 2; wantSeq++ {
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
		if envelope.Generation != 9 || envelope.SessionSeq == nil || *envelope.SessionSeq != wantSeq {
			t.Fatalf("envelope generation=%d sessionSeq=%v, want generation=9 sessionSeq=%d", envelope.Generation, envelope.SessionSeq, wantSeq)
		}
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
	err = (&xinzhiliWSSink{conn: rc}).SendAudio(context.Background(), xinzhili.AudioSegment{TurnKey: 1, Audio: []byte("x")})
	if err == nil {
		t.Fatal("SendAudio returned nil after connection close")
	}
}

func TestXinzhiliBinaryRejectsMismatchedGeneration(t *testing.T) {
	serverWS, _ := newXinzhiliWebsocketPair(t)
	session := &recordingXinzhiliTurnSession{}
	c := &xinzhiliRealtimeConn{
		ws: serverWS, sess: session, generation: 7, sessionID: "xz-test",
		turns: map[uint64]string{42: "turn-1"},
	}
	c.sink = &xinzhiliWSSink{conn: c}
	data, err := xinzhili.EncodeBinaryFrame(xinzhili.BinaryFrame{
		FrameType: xinzhili.FrameTypeInputPCM, Flags: xinzhili.FlagStart,
		Generation: 6, TurnKey: 42, AudioSeq: 0, Payload: []byte{1, 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	c.handleBinary(context.Background(), data)
	if len(session.pcm) != 0 {
		t.Fatalf("mismatched generation forwarded %d PCM frames", len(session.pcm))
	}
}

func TestXinzhiliBinaryDropsDuplicateAudioSequence(t *testing.T) {
	serverWS, _ := newXinzhiliWebsocketPair(t)
	session := &recordingXinzhiliTurnSession{}
	c := &xinzhiliRealtimeConn{
		ws: serverWS, sess: session, generation: 7, sessionID: "xz-test",
		turns: map[uint64]string{42: "turn-1"},
	}
	c.sink = &xinzhiliWSSink{conn: c}
	data, err := xinzhili.EncodeBinaryFrame(xinzhili.BinaryFrame{
		FrameType: xinzhili.FrameTypeInputPCM, Flags: xinzhili.FlagStart,
		Generation: 7, TurnKey: 42, AudioSeq: 0, Payload: []byte{1, 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	c.handleBinary(context.Background(), data)
	c.handleBinary(context.Background(), data)
	if len(session.pcm) != 1 {
		t.Fatalf("duplicate audio sequence forwarded %d PCM frames", len(session.pcm))
	}
}

func TestXinzhiliBinaryConsumesTailFramesAfterASRInputFinishedThenClosed(t *testing.T) {
	serverWS, clientWS := newXinzhiliWebsocketPair(t)
	session := &recordingXinzhiliTurnSession{pushErrs: []error{xinzhili.ErrASRInputFinished, xinzhili.ErrASRClosed}}
	c := &xinzhiliRealtimeConn{
		ws: serverWS, sess: session, generation: 7, sessionID: "xz-test",
		turns: map[uint64]string{42: "turn-1"}, audioSeq: map[uint64]uint32{42: 0},
	}
	c.sink = &xinzhiliWSSink{conn: c}
	data, err := xinzhili.EncodeBinaryFrame(xinzhili.BinaryFrame{
		FrameType: xinzhili.FrameTypeInputPCM, Flags: xinzhili.FlagStart,
		Generation: 7, TurnKey: 42, AudioSeq: 0, Payload: []byte{1, 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	c.handleBinary(context.Background(), data)
	data, err = xinzhili.EncodeBinaryFrame(xinzhili.BinaryFrame{
		FrameType:  xinzhili.FrameTypeInputPCM,
		Generation: 7, TurnKey: 42, AudioSeq: 1, Payload: []byte{3, 4},
	})
	if err != nil {
		t.Fatal(err)
	}
	c.handleBinary(context.Background(), data)
	if got := c.audioSeq[42]; got != 2 {
		t.Fatalf("audio sequence=%d want=2", got)
	}
	_ = clientWS.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
	if _, _, err := clientWS.ReadMessage(); err == nil {
		t.Fatal("ASR finished/closed tail frames emitted an error event")
	}
}

func TestXinzhiliBinaryReportsUnexpectedPCMRejection(t *testing.T) {
	serverWS, clientWS := newXinzhiliWebsocketPair(t)
	session := &recordingXinzhiliTurnSession{pushErr: errors.New("write failed")}
	c := &xinzhiliRealtimeConn{
		ws: serverWS, sess: session, generation: 7, sessionID: "xz-test",
		turns: map[uint64]string{42: "turn-1"}, audioSeq: map[uint64]uint32{42: 0},
	}
	c.sink = &xinzhiliWSSink{conn: c}
	data, err := xinzhili.EncodeBinaryFrame(xinzhili.BinaryFrame{
		FrameType: xinzhili.FrameTypeInputPCM, Flags: xinzhili.FlagStart,
		Generation: 7, TurnKey: 42, AudioSeq: 0, Payload: []byte{1, 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	c.handleBinary(context.Background(), data)
	_ = clientWS.SetReadDeadline(time.Now().Add(time.Second))
	_, raw, err := clientWS.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := xinzhili.DecodeEnvelope(raw, xinzhili.DirectionServer, true)
	if err != nil {
		t.Fatal(err)
	}
	var payload xinzhili.ErrorPayload
	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Code != "audio_frame_rejected" {
		t.Fatalf("error code=%q", payload.Code)
	}
	if got := c.audioSeq[42]; got != 0 {
		t.Fatalf("rejected audio sequence=%d want=0", got)
	}
}

func TestXinzhiliControlDropsDuplicateClientSequence(t *testing.T) {
	serverWS, clientWS := newXinzhiliWebsocketPair(t)
	c := &xinzhiliRealtimeConn{
		ws: serverWS, sess: &recordingXinzhiliTurnSession{}, generation: 7,
		sessionID: "xz-test", turns: make(map[uint64]string),
	}
	c.sink = &xinzhiliWSSink{conn: c}
	sessionSeq := uint64(0)
	envelope := xinzhili.Envelope{
		ProtocolVersion: xinzhili.ProtocolVersion,
		Type:            xinzhili.EventSessionPing,
		SessionID:       &c.sessionID,
		Generation:      7,
		SessionSeq:      &sessionSeq,
		TimestampMs:     time.Now().UnixMilli(),
		Payload:         json.RawMessage(`{}`),
	}
	data, err := xinzhili.EncodeEnvelope(envelope, xinzhili.DirectionClient, true)
	if err != nil {
		t.Fatal(err)
	}
	c.handleEnvelope(context.Background(), data)
	c.handleEnvelope(context.Background(), data)

	_ = clientWS.SetReadDeadline(time.Now().Add(time.Second))
	if _, _, err := clientWS.ReadMessage(); err != nil {
		t.Fatalf("first pong: %v", err)
	}
	_ = clientWS.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
	if _, _, err := clientWS.ReadMessage(); err == nil {
		t.Fatal("duplicate client sessionSeq produced a second pong")
	}
}

func TestXinzhiliPreReadySequenceErrorUsesClientGeneration(t *testing.T) {
	serverWS, clientWS := newXinzhiliWebsocketPair(t)
	c := &xinzhiliRealtimeConn{ws: serverWS, turns: make(map[uint64]string)}
	c.sink = &xinzhiliWSSink{conn: c}
	sessionSeq := uint64(1)
	start := xinzhili.Envelope{
		ProtocolVersion: xinzhili.ProtocolVersion,
		Type:            xinzhili.EventSessionStart, Generation: 7, SessionSeq: &sessionSeq,
		TimestampMs: 1, Payload: json.RawMessage(`{"cardId":1,"conversationId":0}`),
	}
	data, err := xinzhili.EncodeEnvelope(start, xinzhili.DirectionClient, false)
	if err != nil {
		t.Fatal(err)
	}
	c.handleEnvelope(context.Background(), data)

	_ = clientWS.SetReadDeadline(time.Now().Add(time.Second))
	_, response, err := clientWS.ReadMessage()
	if err != nil {
		t.Fatalf("read protocol error: %v", err)
	}
	envelope, err := xinzhili.DecodeEnvelope(response, xinzhili.DirectionServer, false)
	if err != nil {
		t.Fatal(err)
	}
	if envelope.Generation != 7 {
		t.Fatalf("error generation=%d want=7", envelope.Generation)
	}
	var payload xinzhili.ErrorPayload
	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Code != "control_sequence_gap" {
		t.Fatalf("error code=%q", payload.Code)
	}
}

func TestXinzhiliPreReadyDecodeErrorsReturnEncodableSessionErrors(t *testing.T) {
	tests := []struct {
		name           string
		request        string
		wantCode       string
		wantGeneration uint32
	}{
		{
			name:           "malformed json",
			request:        `{`,
			wantCode:       xinzhili.ProtocolErrorInvalidEnvelope,
			wantGeneration: 0,
		},
		{
			name:           "unsupported version",
			request:        `{"protocolVersion":"xinzhili.voice.v0","type":"session.start","sessionId":null,"generation":7,"turnId":null,"sessionSeq":0,"turnSeq":null,"configVersion":0,"timestampMs":1,"payload":{"cardId":1,"conversationId":0}}`,
			wantCode:       xinzhili.ProtocolErrorUnsupportedVersion,
			wantGeneration: 7,
		},
		{
			name:           "invalid event direction",
			request:        `{"protocolVersion":"xinzhili.voice.v1","type":"session.ready","sessionId":"client-session","generation":8,"turnId":null,"sessionSeq":0,"turnSeq":null,"configVersion":0,"timestampMs":1,"payload":{}}`,
			wantCode:       xinzhili.ProtocolErrorInvalidEventDirection,
			wantGeneration: 8,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			serverWS, clientWS := newXinzhiliWebsocketPair(t)
			c := &xinzhiliRealtimeConn{ws: serverWS, turns: make(map[uint64]string)}
			c.sink = &xinzhiliWSSink{conn: c}

			c.handleEnvelope(context.Background(), []byte(tt.request))

			_ = clientWS.SetReadDeadline(time.Now().Add(time.Second))
			kind, response, err := clientWS.ReadMessage()
			if err != nil {
				t.Fatalf("read pre-ready protocol error: %v", err)
			}
			if kind != websocket.TextMessage {
				t.Fatalf("frame kind=%d want text", kind)
			}
			envelope, err := xinzhili.DecodeEnvelope(response, xinzhili.DirectionServer, false)
			if err != nil {
				t.Fatalf("decode pre-ready protocol error: %v; response=%s", err, response)
			}
			if envelope.Type != xinzhili.EventError || envelope.TurnID != nil || envelope.TurnSeq != nil {
				t.Fatalf("pre-ready error is not session-level: %+v", envelope)
			}
			if envelope.SessionID == nil || *envelope.SessionID == "" {
				t.Fatalf("pre-ready error has invalid sessionId: %+v", envelope.SessionID)
			}
			if envelope.Generation != tt.wantGeneration {
				t.Fatalf("generation=%d want=%d", envelope.Generation, tt.wantGeneration)
			}
			var payload xinzhili.ErrorPayload
			if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
				t.Fatal(err)
			}
			if payload.Code != tt.wantCode {
				t.Fatalf("error code=%q want=%q", payload.Code, tt.wantCode)
			}
		})
	}
}

func TestXinzhiliTurnCancelReleasesTurnSequenceState(t *testing.T) {
	turnID := "turn-cancel"
	turnKey := xinzhili.TurnKey(turnID)
	guard := xinzhili.NewSequenceGuard(7)
	startSessionSeq, startTurnSeq := uint64(0), uint64(0)
	start := xinzhili.Envelope{
		ProtocolVersion: xinzhili.ProtocolVersion,
		Type:            xinzhili.EventTurnStart, SessionID: stringPtr("xz-test"), Generation: 7,
		TurnID: &turnID, SessionSeq: &startSessionSeq, TurnSeq: &startTurnSeq,
		TimestampMs: 1, Payload: json.RawMessage(fmt.Sprintf(`{"turnKey":%d}`, turnKey)),
	}
	if disposition, err := guard.Observe(xinzhili.DirectionClient, start); err != nil || disposition != xinzhili.SequenceAccept {
		t.Fatalf("prime sequence guard: disposition=%v err=%v", disposition, err)
	}
	if err := guard.RegisterActiveTurn(turnID, turnKey); err != nil {
		t.Fatal(err)
	}

	c := &xinzhiliRealtimeConn{
		sess: &recordingXinzhiliTurnSession{}, generation: 7, sessionID: "xz-test",
		sequence: guard, turnKey: turnKey,
		turns: map[uint64]string{turnKey: turnID}, audioSeq: map[uint64]uint32{turnKey: 3},
	}
	cancelSessionSeq, cancelTurnSeq := uint64(1), uint64(1)
	cancel := xinzhili.Envelope{
		ProtocolVersion: xinzhili.ProtocolVersion,
		Type:            xinzhili.EventTurnCancel, SessionID: &c.sessionID, Generation: 7,
		TurnID: &turnID, SessionSeq: &cancelSessionSeq, TurnSeq: &cancelTurnSeq,
		TimestampMs: 2, Payload: json.RawMessage(`{}`),
	}
	data, err := xinzhili.EncodeEnvelope(cancel, xinzhili.DirectionClient, true)
	if err != nil {
		t.Fatal(err)
	}
	c.handleEnvelope(context.Background(), data)
	if len(c.turns) != 0 || len(c.audioSeq) != 0 || c.turnKey != 0 {
		t.Fatalf("cancel retained turn state: turns=%v audioSeq=%v turnKey=%d", c.turns, c.audioSeq, c.turnKey)
	}
}

func stringPtr(value string) *string { return &value }
