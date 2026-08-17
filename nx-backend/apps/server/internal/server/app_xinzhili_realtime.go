package server

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	appconfig "nine-xing/nx-backend/apps/server/internal/config"
	"nine-xing/nx-backend/apps/server/internal/xinzhili"
)

const (
	xinzhiliMaxMessageBytes       = 1 << 20
	xinzhiliIdleTimeout           = 2 * time.Minute
	xinzhiliWriteTimeout          = 2 * time.Second
	xinzhiliBroadcastTimeout      = 3 * time.Second
	xinzhiliProtocolUpdateMessage = "语音服务正在更新，请稍后重试"
)

var xinzhiliUpgrader = websocket.Upgrader{
	ReadBufferSize:    16 << 10,
	WriteBufferSize:   16 << 10,
	EnableCompression: true,
}

var errXinzhiliModeDisabled = errors.New("xinzhili mode is disabled")
var errXinzhiliConfigSuperseded = errors.New("xinzhili config event superseded")

type xinzhiliModePreferenceStore interface {
	ReadMode(context.Context, int64) (xinzhili.ModePreference, bool, error)
	UpdateMode(context.Context, int64, xinzhili.Mode, int64) (xinzhili.ModePreference, error)
}

type xinzhiliModeSnapshot struct {
	EnabledModes  []xinzhili.Mode `json:"enabledModes"`
	RequestedMode xinzhili.Mode   `json:"requestedMode"`
	PendingMode   xinzhili.Mode   `json:"pendingMode"`
	EffectiveMode xinzhili.Mode   `json:"effectiveMode"`
	Revision      int64           `json:"revision"`
	ConfigVersion int64           `json:"configVersion"`
}

type xinzhiliRealtimeConn struct {
	server         *Server
	ws             *websocket.Conn
	userID         int64
	sink           *xinzhiliWSSink
	sess           xinzhili.TurnSession
	sessionEpoch   uint64
	configStateMu  sync.Mutex
	mu             sync.Mutex
	closed         bool
	sessionID      string
	configVersion  int64
	enabledModes   []xinzhili.Mode
	generation     uint32
	cardID         int64
	conversationID int64
	requestedMode  xinzhili.Mode
	pendingMode    xinzhili.Mode
	effectiveMode  xinzhili.Mode
	modeRevision   int64
	modeStore      xinzhiliModePreferenceStore
	newSession     func(xinzhili.Config, xinzhili.SessionSink) (xinzhili.TurnSession, error)
	sequence       *xinzhili.SequenceGuard
	turnKey        uint64
	turnMu         sync.Mutex
	turns          map[uint64]string
	audioSeq       map[uint64]uint32
	audioInputDone map[uint64]bool
}

type xinzhiliWSSink struct {
	conn           *xinzhiliRealtimeConn
	mu             sync.Mutex
	nextSessionSeq uint64
}

func (s *Server) xinzhiliRealtime(w http.ResponseWriter, r *http.Request) {
	user, ok := appUserFromContext(r)
	if !ok || user.ID <= 0 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	origin := r.Header.Get("Origin")
	if origin != "" && !s.corsOriginAllowed(origin) {
		http.Error(w, "origin_not_allowed", http.StatusForbidden)
		return
	}
	if appconfig.IsProduction(s.env.AppEnv) && r.TLS == nil && strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "http") {
		http.Error(w, "tls_required", http.StatusUpgradeRequired)
		return
	}
	if !s.reserveXinzhiliRealtime(user.ID) {
		http.Error(w, "realtime_capacity_exceeded", http.StatusServiceUnavailable)
		return
	}
	reserved := true
	defer func() {
		if reserved {
			s.releaseXinzhiliRealtimeReservation()
		}
	}()
	upgrader := xinzhiliUpgrader
	upgrader.CheckOrigin = func(req *http.Request) bool {
		o := req.Header.Get("Origin")
		return o == "" || s.corsOriginAllowed(o)
	}
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	ws.SetReadLimit(xinzhiliMaxMessageBytes)
	modeStore := s.xinzhiliModeStore
	if modeStore == nil {
		modeStore = xinzhili.NewStore(s.db)
	}
	c := &xinzhiliRealtimeConn{server: s, ws: ws, userID: user.ID, turns: make(map[uint64]string), audioSeq: make(map[uint64]uint32), modeStore: modeStore}
	c.sink = &xinzhiliWSSink{conn: c}
	s.replaceXinzhiliLease(c)
	s.releaseXinzhiliRealtimeReservation()
	reserved = false
	if s.metrics != nil {
		s.metrics.RealtimeOpened()
		defer s.metrics.RealtimeClosed()
	}
	defer s.releaseXinzhiliLease(c)
	defer c.close(websocket.CloseNormalClosure, "")
	c.readLoop(r.Context())
}

func (s *Server) reserveXinzhiliRealtime(userID int64) bool {
	s.xinzhiliLeaseMu.Lock()
	defer s.xinzhiliLeaseMu.Unlock()
	_, alreadyConnected := s.xinzhiliLeases[userID]
	if s.xinzhiliMaxConnections > 0 && len(s.xinzhiliLeases)+s.xinzhiliPendingConnections >= s.xinzhiliMaxConnections && !alreadyConnected {
		return false
	}
	s.xinzhiliPendingConnections++
	return true
}

func (s *Server) releaseXinzhiliRealtimeReservation() {
	s.xinzhiliLeaseMu.Lock()
	if s.xinzhiliPendingConnections > 0 {
		s.xinzhiliPendingConnections--
	}
	s.xinzhiliLeaseMu.Unlock()
}

func (s *Server) replaceXinzhiliLease(c *xinzhiliRealtimeConn) {
	s.xinzhiliLeaseMu.Lock()
	old := s.xinzhiliLeases[c.userID]
	s.xinzhiliLeases[c.userID] = c
	s.xinzhiliLeaseMu.Unlock()
	if old != nil && old != c {
		old.close(websocket.ClosePolicyViolation, "replaced")
	}
}

func (s *Server) releaseXinzhiliLease(c *xinzhiliRealtimeConn) {
	s.xinzhiliLeaseMu.Lock()
	if s.xinzhiliLeases[c.userID] == c {
		delete(s.xinzhiliLeases, c.userID)
	}
	s.xinzhiliLeaseMu.Unlock()
}

func (s *Server) broadcastXinzhiliConfigChanged(ctx context.Context, cfg xinzhili.Config) {
	s.xinzhiliLeaseMu.Lock()
	connections := make([]*xinzhiliRealtimeConn, 0, len(s.xinzhiliLeases))
	for _, connection := range s.xinzhiliLeases {
		connections = append(connections, connection)
	}
	s.xinzhiliLeaseMu.Unlock()

	var wg sync.WaitGroup
	for _, connection := range connections {
		connection := connection
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := connection.applyXinzhiliConfigChanged(ctx, cfg); err != nil {
				log.Printf("xinzhili config_changed delivery failed user_id=%d config_version=%d err=%v", connection.userID, cfg.Version, err)
			}
		}()
	}
	wg.Wait()
}

func (s *Server) scheduleXinzhiliConfigChanged(cfg xinzhili.Config) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), xinzhiliBroadcastTimeout)
		defer cancel()
		s.broadcastXinzhiliConfigChanged(ctx, cfg)
	}()
}

func (c *xinzhiliRealtimeConn) readLoop(ctx context.Context) {
	_ = c.ws.SetReadDeadline(time.Now().Add(xinzhiliIdleTimeout))
	for {
		kind, data, err := c.ws.ReadMessage()
		if err != nil {
			return
		}
		_ = c.ws.SetReadDeadline(time.Now().Add(xinzhiliIdleTimeout))
		switch kind {
		case websocket.TextMessage:
			c.handleEnvelope(ctx, data)
		case websocket.BinaryMessage:
			c.handleBinary(ctx, data)
		default:
			c.sendError(ctx, "unsupported_frame", "不支持的 WebSocket 帧类型", true, false)
		}
	}
}

func (c *xinzhiliRealtimeConn) handleEnvelope(ctx context.Context, data []byte) {
	ready := c.sess != nil
	e, err := xinzhili.DecodeEnvelope(data, xinzhili.DirectionClient, ready)
	if err != nil {
		c.ensurePreReadyProtocolIdentity(data)
		code := "invalid_envelope"
		var pe *xinzhili.ProtocolError
		if errors.As(err, &pe) {
			code = pe.Code
		}
		logXinzhiliProtocolError(data, err)
		c.sendError(ctx, code, xinzhiliProtocolUpdateMessage, true, false)
		return
	}
	c.mu.Lock()
	if c.sequence == nil {
		c.generation = e.Generation
		if c.sessionID == "" {
			c.sessionID = randomSessionID()
		}
		c.sequence = xinzhili.NewSequenceGuard(e.Generation)
	}
	sequence := c.sequence
	c.mu.Unlock()
	disposition, err := sequence.Observe(xinzhili.DirectionClient, e)
	if err != nil {
		code := "control_sequence_gap"
		if errors.Is(err, xinzhili.ErrGenerationMismatch) {
			code = "generation_mismatch"
		}
		c.sendError(ctx, code, xinzhiliProtocolUpdateMessage, true, false)
		return
	}
	if disposition == xinzhili.SequenceDrop {
		return
	}
	switch e.Type {
	case xinzhili.EventSessionStart:
		c.startSession(ctx, e)
	case xinzhili.EventSessionPing:
		c.sendControl(ctx, xinzhili.EventSessionPong, nil, nil, nil)
	case xinzhili.EventSessionStop:
		c.close(websocket.CloseNormalClosure, "")
	case xinzhili.EventModeChange:
		c.changeMode(ctx, e)
	case xinzhili.EventTurnStart:
		c.startTurn(ctx, e)
	case xinzhili.EventTurnCancel:
		if c.sess != nil && e.TurnID != nil {
			if err := c.sess.Cancel(ctx, *e.TurnID); err == nil {
				c.releaseTurn(*e.TurnID, true)
			}
		}
	case xinzhili.EventPlaybackInterrupt:
		if c.sess != nil && e.TurnID != nil {
			if err := c.sess.Interrupt(ctx, *e.TurnID); err == nil {
				c.releaseTurn(*e.TurnID, true)
			}
		}
	case xinzhili.EventAssistantPlaybackAck:
		if c.sess != nil && e.TurnID != nil {
			var p struct {
				SegmentSeq uint32 `json:"segmentSeq"`
			}
			if json.Unmarshal(e.Payload, &p) == nil {
				_ = c.sess.HandlePlaybackAck(ctx, xinzhili.PlaybackAck{TurnID: *e.TurnID, SegmentSeq: p.SegmentSeq})
			}
		}
	}
}

func (c *xinzhiliRealtimeConn) ensurePreReadyProtocolIdentity(data []byte) {
	var wire struct {
		Generation uint32 `json:"generation"`
	}
	_ = json.Unmarshal(data, &wire)

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.sessionID == "" {
		c.sessionID = randomSessionID()
	}
	if c.sequence == nil {
		c.generation = wire.Generation
		c.sequence = xinzhili.NewSequenceGuard(wire.Generation)
	}
}

func logXinzhiliProtocolError(data []byte, err error) {
	var wire struct {
		ProtocolVersion string          `json:"protocolVersion"`
		Type            string          `json:"type"`
		Payload         json.RawMessage `json:"payload"`
	}
	_ = json.Unmarshal(data, &wire)
	var client struct {
		AppBuild           int      `json:"appBuild"`
		ClientCapabilities []string `json:"clientCapabilities"`
	}
	if len(wire.Payload) > 0 {
		_ = json.Unmarshal(wire.Payload, &client)
	}
	code, field, reason := "invalid_envelope", "", "invalid protocol envelope"
	var protocolErr *xinzhili.ProtocolError
	if errors.As(err, &protocolErr) {
		code, field = protocolErr.Code, protocolErr.Field
		if protocolErr.Reason != "" {
			reason = protocolErr.Reason
		}
	}
	log.Printf(
		"xinzhili_protocol_error protocol_version=%q event_type=%q error_code=%q failing_field=%q reason=%q app_build=%d client_capabilities=%q",
		wire.ProtocolVersion,
		wire.Type,
		code,
		field,
		reason,
		client.AppBuild,
		strings.Join(client.ClientCapabilities, ","),
	)
}

func (c *xinzhiliRealtimeConn) startSession(ctx context.Context, e xinzhili.Envelope) {
	var p struct {
		CardID         int64 `json:"cardId"`
		ConversationID int64 `json:"conversationId"`
	}
	_ = json.Unmarshal(e.Payload, &p)
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	if c.sess != nil {
		c.mu.Unlock()
		c.sendError(ctx, "session_already_started", "会话已经启动", false, true)
		return
	}
	c.sessionID = randomSessionID()
	c.generation = e.Generation
	c.cardID = p.CardID
	c.conversationID = p.ConversationID
	c.mu.Unlock()
	for {
		cfg, found, err := c.server.readXinzhiliRealtimeConfig(ctx)
		if err != nil || !found || !cfg.Enabled {
			c.sendError(ctx, "xinzhili_not_configured", "请先配置芯之力会话模型后重试", false, true)
			return
		}
		modeSnapshot, err := c.readModeSnapshot(ctx, cfg)
		if err != nil {
			c.sendError(ctx, "mode_preference_unavailable", "模式偏好读取失败", true, true)
			return
		}
		candidate, err := c.createSession(cfg)
		if err != nil {
			c.sendError(ctx, "xinzhili_dependencies_unavailable", "芯之力服务暂不可用", true, true)
			return
		}

		c.configStateMu.Lock()
		c.mu.Lock()
		stale := c.configVersion > cfg.Version
		closed := c.closed
		var candidateEpoch uint64
		if !stale && !closed {
			c.sessionEpoch++
			candidateEpoch = c.sessionEpoch
			c.sess = candidate
			c.configVersion = cfg.Version
			c.enabledModes = append([]xinzhili.Mode(nil), cfg.EnabledModes...)
			c.requestedMode = modeSnapshot.RequestedMode
			c.pendingMode = modeSnapshot.PendingMode
			c.effectiveMode = modeSnapshot.EffectiveMode
			c.modeRevision = modeSnapshot.Revision
		}
		c.mu.Unlock()
		c.configStateMu.Unlock()
		if stale {
			_ = candidate.Close()
			continue
		}
		if closed {
			_ = candidate.Close()
			return
		}

		readyPayload := map[string]any{"sessionId": c.sessionID, "cardId": p.CardID, "conversationId": p.ConversationID}
		mergeXinzhiliModeSnapshot(readyPayload, modeSnapshot)
		if err := c.sendControlAtConfigVersion(ctx, xinzhili.EventSessionReady, readyPayload, nil, nil, cfg.Version); err != nil {
			if c.releaseCandidateSession(candidateEpoch) {
				_ = candidate.Close()
			}
			if errors.Is(err, errXinzhiliConfigSuperseded) {
				continue
			}
			return
		}
		return
	}
}

func (c *xinzhiliRealtimeConn) releaseCandidateSession(epoch uint64) bool {
	c.configStateMu.Lock()
	defer c.configStateMu.Unlock()
	c.mu.Lock()
	defer c.mu.Unlock()
	if epoch == 0 || c.sessionEpoch != epoch || c.sess == nil {
		return false
	}
	c.sess = nil
	c.sessionEpoch++
	return true
}

func (c *xinzhiliRealtimeConn) createSession(cfg xinzhili.Config) (xinzhili.TurnSession, error) {
	if c.newSession != nil {
		return c.newSession(cfg, c.sink)
	}
	deps, err := c.server.newXinzhiliRealtimeDependencies(cfg, c.sink)
	if err != nil {
		return nil, err
	}
	return xinzhili.NewSession(deps), nil
}

func (c *xinzhiliRealtimeConn) startTurn(ctx context.Context, e xinzhili.Envelope) {
	if c.sess == nil || e.TurnID == nil {
		c.sendError(ctx, "session_not_ready", "会话尚未就绪", true, false)
		return
	}
	turnKey, err := xinzhili.DecodeTurnStartKey(e.Payload)
	if err != nil {
		c.sendError(ctx, "invalid_turn", "轮次参数无效", false, false)
		return
	}
	var cfg xinzhili.Config
	for {
		var found bool
		cfg, found, err = c.server.readXinzhiliRealtimeConfig(ctx)
		if err != nil || !found || !cfg.Enabled {
			c.sendError(ctx, "xinzhili_not_configured", "请先配置芯之力会话模型后重试", false, true)
			return
		}
		c.mu.Lock()
		currentConfigVersion := c.configVersion
		c.mu.Unlock()
		if cfg.Version > currentConfigVersion {
			if err := c.applyXinzhiliConfigChanged(ctx, cfg); err != nil {
				return
			}
		}
		cfg, err = c.server.withXinzhiliRuntimeCredentials(ctx, cfg)
		if err != nil {
			c.sendError(ctx, "xinzhili_credentials_unavailable", "芯之力语音凭证暂不可用", true, false)
			return
		}

		c.configStateMu.Lock()
		c.mu.Lock()
		currentConfigVersion = c.configVersion
		if currentConfigVersion <= cfg.Version {
			mode := c.pendingMode
			if !modeEnabled(cfg.EnabledModes, mode) {
				mode = xinzhili.ModeNormal
			}
			cardID, conversationID := c.cardID, c.conversationID
			previousTurnKey := c.turnKey
			sequence := c.sequence
			c.mu.Unlock()
			c.configStateMu.Unlock()

			c.startTurnWithConfig(ctx, e, turnKey, cfg, mode, cardID, conversationID, previousTurnKey, sequence)
			return
		}
		c.mu.Unlock()
		c.configStateMu.Unlock()
	}
}

func (c *xinzhiliRealtimeConn) startTurnWithConfig(ctx context.Context, e xinzhili.Envelope, turnKey uint64, cfg xinzhili.Config, mode xinzhili.Mode, cardID, conversationID int64, previousTurnKey uint64, sequence *xinzhili.SequenceGuard) {
	c.turnMu.Lock()
	previousTurnID := c.turns[previousTurnKey]
	c.turnMu.Unlock()
	if previousTurnID != "" && previousTurnID != *e.TurnID {
		c.releaseTurn(previousTurnID, true)
	}
	if sequence != nil {
		if err := sequence.RegisterActiveTurn(*e.TurnID, turnKey); err != nil {
			c.sendError(ctx, "turn_key_collision", "轮次标识冲突，请重新开始", true, false)
			return
		}
	}
	in := xinzhili.StartTurnInput{UserID: c.userID, CardID: cardID, ConversationID: conversationID, TurnID: *e.TurnID, TurnKey: turnKey, Mode: mode, ASRConfig: cfg.RealtimeASR, TTSConfig: cfg.TTS, Timing: cfg.Timing, CommonPrompt: cfg.CommonPrompt, ModePrompt: cfg.ModePrompts[mode], KnowledgeTopK: 6, KnowledgeMinScore: 0.2, TheoryTopK: 6, TheoryMinScore: 0.2}
	if err := c.sess.StartTurn(ctx, in); err != nil {
		if sequence != nil {
			sequence.ReleaseActiveTurn(*e.TurnID)
		}
		c.sendError(ctx, "turn_start_failed", "无法开始当前轮次", true, false)
		return
	}

	c.configStateMu.Lock()
	c.turnMu.Lock()
	c.turns[turnKey] = *e.TurnID
	if c.audioSeq == nil {
		c.audioSeq = make(map[uint64]uint32)
	}
	c.audioSeq[turnKey] = 0
	if c.audioInputDone != nil {
		delete(c.audioInputDone, turnKey)
	}
	c.turnMu.Unlock()

	c.mu.Lock()
	previousEffectiveMode := c.effectiveMode
	c.effectiveMode = mode
	if c.configVersion <= cfg.Version {
		c.pendingMode = mode
		c.requestedMode = mode
		c.enabledModes = append([]xinzhili.Mode(nil), cfg.EnabledModes...)
	}
	authoritativeModes := c.enabledModes
	if len(authoritativeModes) == 0 {
		authoritativeModes = append([]xinzhili.Mode(nil), cfg.EnabledModes...)
	}
	c.turnKey = turnKey
	modeTransitionCompleted := previousEffectiveMode != mode &&
		(c.configVersion > cfg.Version || !modeEnabled(authoritativeModes, previousEffectiveMode))
	snapshot := xinzhiliModeSnapshot{
		EnabledModes:  append([]xinzhili.Mode(nil), authoritativeModes...),
		RequestedMode: c.requestedMode, PendingMode: c.pendingMode,
		EffectiveMode: c.effectiveMode, Revision: c.modeRevision, ConfigVersion: c.configVersion,
	}
	c.mu.Unlock()
	c.configStateMu.Unlock()
	if modeTransitionCompleted {
		_ = c.sendControl(ctx, xinzhili.EventConfigChanged, snapshot, nil, nil)
	}
}

func (c *xinzhiliRealtimeConn) releaseTurn(turnID string, terminal bool) {
	var releasedKeys []uint64
	c.turnMu.Lock()
	for turnKey, activeTurnID := range c.turns {
		if activeTurnID != turnID {
			continue
		}
		delete(c.turns, turnKey)
		delete(c.audioSeq, turnKey)
		delete(c.audioInputDone, turnKey)
		releasedKeys = append(releasedKeys, turnKey)
	}
	c.turnMu.Unlock()

	c.mu.Lock()
	for _, turnKey := range releasedKeys {
		if c.turnKey == turnKey {
			c.turnKey = 0
		}
	}
	sequence := c.sequence
	c.mu.Unlock()
	if sequence == nil {
		return
	}
	if terminal {
		sequence.MarkTerminal(turnID)
		return
	}
	sequence.ReleaseActiveTurn(turnID)
}

func (c *xinzhiliRealtimeConn) handleBinary(ctx context.Context, data []byte) {
	if c.sess == nil {
		c.sendError(ctx, "session_not_ready", "会话尚未就绪", true, false)
		return
	}
	f, err := xinzhili.DecodeBinaryFrame(data)
	if err != nil || f.FrameType != xinzhili.FrameTypeInputPCM {
		c.sendError(ctx, "invalid_audio_frame", "音频帧无效", true, false)
		return
	}
	if f.Generation != c.generation {
		c.sendError(ctx, "generation_mismatch", "音频连接版本不一致，请重新连接", true, false)
		return
	}
	c.turnMu.Lock()
	turnID := c.turns[f.TurnKey]
	expectedAudioSeq := c.audioSeq[f.TurnKey]
	c.turnMu.Unlock()
	if turnID == "" {
		c.sendError(ctx, "unknown_turn", "音频帧对应的轮次不存在", false, false)
		return
	}
	if f.AudioSeq < expectedAudioSeq {
		return
	}
	if f.AudioSeq > expectedAudioSeq {
		c.sendError(ctx, "audio_sequence_gap", "音频帧序号不连续，请重新说一次", true, false)
		return
	}
	if err := c.sess.PushPCM(ctx, xinzhili.PCMFrame{TurnID: turnID, Data: f.Payload}); err != nil {
		// FinishInput closes Paraformer's audio input before the final
		// task-finished event reaches the App. PCM already queued on the phone
		// during that short window is an expected tail, not a broken turn. Once
		// that barrier is observed, the ASR may close before the last queued PCM
		// arrives, so ErrASRClosed remains an expected tail for the same turn.
		// Consume its sequence so following queued frames remain monotonic.
		if !c.consumeASRTail(f.TurnKey, err) {
			log.Printf("xinzhili pcm rejected turn=%q turn_key=%d audio_seq=%d payload_bytes=%d err=%T:%v", turnID, f.TurnKey, f.AudioSeq, len(f.Payload), err, err)
			c.sendError(ctx, "audio_frame_rejected", "音频帧未被接收", true, false)
			return
		}
	}
	c.turnMu.Lock()
	if c.audioSeq == nil {
		c.audioSeq = make(map[uint64]uint32)
	}
	c.audioSeq[f.TurnKey] = expectedAudioSeq + 1
	c.turnMu.Unlock()
}

func (c *xinzhiliRealtimeConn) consumeASRTail(turnKey uint64, err error) bool {
	c.turnMu.Lock()
	defer c.turnMu.Unlock()
	if errors.Is(err, xinzhili.ErrASRInputFinished) {
		if c.audioInputDone == nil {
			c.audioInputDone = make(map[uint64]bool)
		}
		c.audioInputDone[turnKey] = true
		return true
	}
	return errors.Is(err, xinzhili.ErrASRClosed) && c.audioInputDone[turnKey]
}

func (c *xinzhiliRealtimeConn) changeMode(ctx context.Context, e xinzhili.Envelope) {
	var p struct {
		Mode             xinzhili.Mode `json:"mode"`
		ExpectedRevision int64         `json:"expectedRevision"`
	}
	if json.Unmarshal(e.Payload, &p) != nil || !knownXinzhiliMode(p.Mode) || p.ExpectedRevision < 0 {
		c.sendError(ctx, "invalid_mode", "模式无效", false, false)
		return
	}
	cfg, found, err := c.server.readXinzhiliRealtimeConfig(ctx)
	if err != nil || !found || !cfg.Enabled {
		c.sendError(ctx, "xinzhili_not_configured", "请先配置芯之力会话模型后重试", false, false)
		return
	}
	snapshot, err := c.persistModeChangeAuthoritative(ctx, cfg, p.Mode, p.ExpectedRevision)
	if errors.Is(err, errXinzhiliModeDisabled) {
		c.sendError(ctx, "mode_not_enabled", "当前模式尚未启用", false, false)
		return
	}
	if errors.Is(err, xinzhili.ErrModePreferenceConflict) {
		c.sendError(ctx, "mode_revision_conflict", "模式状态已更新，请同步后重试", true, false)
		return
	}
	if err != nil {
		c.sendError(ctx, "mode_change_failed", "模式切换失败", true, false)
		return
	}
	_ = c.sendControlAtConfigVersion(ctx, xinzhili.EventModeChanged, snapshot, nil, nil, snapshot.ConfigVersion)
}

func (s *Server) readXinzhiliRealtimeConfig(ctx context.Context) (xinzhili.Config, bool, error) {
	if s.xinzhiliModelConfig != nil {
		return s.xinzhiliModelConfig.Read(ctx)
	}
	return xinzhili.ReadConfig(ctx, s.db)
}

// withXinzhiliRuntimeCredentials returns a per-turn copy. Shared credentials
// are never written back to xinzhili_model_config, and private TTS credentials
// are deliberately left untouched.
func (s *Server) withXinzhiliRuntimeCredentials(ctx context.Context, cfg xinzhili.Config) (xinzhili.Config, error) {
	if !xinzhili.IsOfficialDashScopeRealtimeASREndpoint(cfg.RealtimeASR.Endpoint) {
		return xinzhili.Config{}, errors.New("realtime ASR endpoint is not official DashScope")
	}
	usesSharedTTS := xinzhili.TTSUsesBailianCredentials(cfg.TTS)
	if !usesSharedTTS && strings.TrimSpace(cfg.TTS.APIKey) == "" {
		return xinzhili.Config{}, errors.New("private TTS credential is empty")
	}
	resolved, err := s.resolveBailianCredentialsForConfig(ctx, cfg, true)
	if err != nil {
		return xinzhili.Config{}, err
	}
	key := strings.TrimSpace(resolved.APIKey)
	if key == "" {
		return xinzhili.Config{}, errors.New("shared Bailian credential is empty")
	}
	runtime := cfg
	runtime.RealtimeASR.APIKey = key
	if usesSharedTTS {
		runtime.TTS.APIKey = key
	}
	return runtime, nil
}

func (c *xinzhiliRealtimeConn) loadModeSnapshot(ctx context.Context, cfg xinzhili.Config) (xinzhiliModeSnapshot, error) {
	snapshot, err := c.readModeSnapshot(ctx, cfg)
	if err != nil {
		return xinzhiliModeSnapshot{}, err
	}
	c.mu.Lock()
	c.requestedMode, c.pendingMode, c.effectiveMode, c.modeRevision = snapshot.RequestedMode, snapshot.PendingMode, snapshot.EffectiveMode, snapshot.Revision
	c.mu.Unlock()
	return snapshot, nil
}

func (c *xinzhiliRealtimeConn) readModeSnapshot(ctx context.Context, cfg xinzhili.Config) (xinzhiliModeSnapshot, error) {
	preference, found, err := c.modeStore.ReadMode(ctx, c.userID)
	if err != nil {
		return xinzhiliModeSnapshot{}, err
	}
	mode, revision := xinzhili.ModeNormal, int64(0)
	if found {
		mode, revision = preference.Requested, preference.Revision
	}
	if !modeEnabled(cfg.EnabledModes, mode) {
		mode = xinzhili.ModeNormal
	}
	return xinzhiliModeSnapshot{
		EnabledModes: append([]xinzhili.Mode(nil), cfg.EnabledModes...), RequestedMode: mode,
		PendingMode: mode, EffectiveMode: mode, Revision: revision, ConfigVersion: cfg.Version,
	}, nil
}

func (c *xinzhiliRealtimeConn) persistModeChangeAuthoritative(ctx context.Context, cfg xinzhili.Config, mode xinzhili.Mode, expectedRevision int64) (xinzhiliModeSnapshot, error) {
	if !modeEnabled(cfg.EnabledModes, mode) {
		return xinzhiliModeSnapshot{}, errXinzhiliModeDisabled
	}
	preference, err := c.modeStore.UpdateMode(ctx, c.userID, mode, expectedRevision)
	if err != nil {
		return xinzhiliModeSnapshot{}, err
	}
	for {
		latest, found, err := c.server.readXinzhiliRealtimeConfig(ctx)
		if err != nil || !found || !latest.Enabled {
			return xinzhiliModeSnapshot{}, errors.New("xinzhili config unavailable")
		}
		c.mu.Lock()
		currentVersion := c.configVersion
		c.mu.Unlock()
		if latest.Version > currentVersion {
			// State application is authoritative even when the best-effort
			// config_changed write fails on this connection.
			_ = c.applyXinzhiliConfigChanged(ctx, latest)
		}
		if !modeEnabled(latest.EnabledModes, mode) {
			fallback := fallbackXinzhiliMode(latest.EnabledModes)
			if preference.Requested != fallback {
				preference, err = c.modeStore.UpdateMode(ctx, c.userID, fallback, preference.Revision)
				if err != nil {
					return xinzhiliModeSnapshot{}, err
				}
			}
			if _, committed := c.commitModePreference(latest, preference); !committed {
				continue
			}
			return xinzhiliModeSnapshot{}, errXinzhiliModeDisabled
		}
		if preference.Requested != mode {
			preference, err = c.modeStore.UpdateMode(ctx, c.userID, mode, preference.Revision)
			if err != nil {
				return xinzhiliModeSnapshot{}, err
			}
			continue
		}
		if snapshot, committed := c.commitModePreference(latest, preference); committed {
			return snapshot, nil
		}
	}
}

func (c *xinzhiliRealtimeConn) commitModePreference(cfg xinzhili.Config, preference xinzhili.ModePreference) (xinzhiliModeSnapshot, bool) {
	c.configStateMu.Lock()
	defer c.configStateMu.Unlock()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.configVersion != cfg.Version {
		return xinzhiliModeSnapshot{}, false
	}
	c.requestedMode = preference.Requested
	c.pendingMode = preference.Requested
	c.modeRevision = preference.Revision
	return xinzhiliModeSnapshot{
		EnabledModes: append([]xinzhili.Mode(nil), c.enabledModes...), RequestedMode: c.requestedMode,
		PendingMode: c.pendingMode, EffectiveMode: c.effectiveMode, Revision: c.modeRevision, ConfigVersion: c.configVersion,
	}, true
}

func fallbackXinzhiliMode(enabled []xinzhili.Mode) xinzhili.Mode {
	fallback := xinzhili.ModeNormal
	if !modeEnabled(enabled, fallback) && len(enabled) > 0 {
		fallback = enabled[0]
	}
	return fallback
}

func (c *xinzhiliRealtimeConn) persistModeChange(ctx context.Context, cfg xinzhili.Config, mode xinzhili.Mode, expectedRevision int64) (xinzhiliModeSnapshot, error) {
	if !modeEnabled(cfg.EnabledModes, mode) {
		return xinzhiliModeSnapshot{}, errXinzhiliModeDisabled
	}
	preference, err := c.modeStore.UpdateMode(ctx, c.userID, mode, expectedRevision)
	if err != nil {
		return xinzhiliModeSnapshot{}, err
	}
	c.mu.Lock()
	c.requestedMode = preference.Requested
	c.pendingMode = preference.Requested
	c.modeRevision = preference.Revision
	c.mu.Unlock()
	return c.modeSnapshot(cfg), nil
}

func (c *xinzhiliRealtimeConn) modeSnapshot(cfg xinzhili.Config) xinzhiliModeSnapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	return xinzhiliModeSnapshot{
		EnabledModes: append([]xinzhili.Mode(nil), cfg.EnabledModes...), RequestedMode: c.requestedMode,
		PendingMode: c.pendingMode, EffectiveMode: c.effectiveMode, Revision: c.modeRevision, ConfigVersion: cfg.Version,
	}
}

func (c *xinzhiliRealtimeConn) applyXinzhiliConfigChanged(ctx context.Context, cfg xinzhili.Config) error {
	c.configStateMu.Lock()
	c.turnMu.Lock()
	hasActiveTurn := len(c.turns) > 0
	c.turnMu.Unlock()

	c.mu.Lock()
	if cfg.Version <= c.configVersion {
		c.mu.Unlock()
		c.configStateMu.Unlock()
		return nil
	}
	fallback := fallbackXinzhiliMode(cfg.EnabledModes)
	if !modeEnabled(cfg.EnabledModes, c.requestedMode) {
		c.requestedMode = fallback
	}
	if !modeEnabled(cfg.EnabledModes, c.pendingMode) {
		c.pendingMode = fallback
	}
	if !hasActiveTurn && !modeEnabled(cfg.EnabledModes, c.effectiveMode) {
		c.effectiveMode = fallback
	}
	c.configVersion = cfg.Version
	c.enabledModes = append([]xinzhili.Mode(nil), cfg.EnabledModes...)
	snapshot := xinzhiliModeSnapshot{
		EnabledModes:  append([]xinzhili.Mode(nil), cfg.EnabledModes...),
		RequestedMode: c.requestedMode, PendingMode: c.pendingMode,
		EffectiveMode: c.effectiveMode, Revision: c.modeRevision, ConfigVersion: cfg.Version,
	}
	c.mu.Unlock()
	c.configStateMu.Unlock()

	return c.sendControlAtConfigVersion(ctx, xinzhili.EventConfigChanged, snapshot, nil, nil, cfg.Version)
}

func mergeXinzhiliModeSnapshot(dst map[string]any, snapshot xinzhiliModeSnapshot) {
	dst["enabledModes"] = snapshot.EnabledModes
	dst["requestedMode"] = snapshot.RequestedMode
	dst["pendingMode"] = snapshot.PendingMode
	dst["effectiveMode"] = snapshot.EffectiveMode
	dst["revision"] = snapshot.Revision
	dst["configVersion"] = snapshot.ConfigVersion
}

func (c *xinzhiliRealtimeConn) sendControl(ctx context.Context, typ xinzhili.EventType, payload any, turnID *string, turnSeq *uint64) error {
	c.mu.Lock()
	configVersion := c.configVersion
	c.mu.Unlock()
	return c.sendControlAtConfigVersion(ctx, typ, payload, turnID, turnSeq, configVersion)
}

func (c *xinzhiliRealtimeConn) sendControlAtConfigVersion(ctx context.Context, typ xinzhili.EventType, payload any, turnID *string, turnSeq *uint64, configVersion int64) error {
	b, _ := json.Marshal(payloadOrEmpty(payload))
	c.mu.Lock()
	sid := c.sessionID
	c.mu.Unlock()
	e := xinzhili.Envelope{ProtocolVersion: xinzhili.ProtocolVersion, Type: typ, SessionID: &sid, TurnID: turnID, TurnSeq: turnSeq, ConfigVersion: configVersion, TimestampMs: time.Now().UnixMilli(), Payload: b}
	return c.sink.SendControl(ctx, e)
}
func payloadOrEmpty(v any) any {
	if v == nil {
		return map[string]any{}
	}
	return v
}
func (c *xinzhiliRealtimeConn) sendError(ctx context.Context, code, msg string, retryable, fatal bool) {
	p := xinzhili.ErrorPayload{Code: code, Message: msg, Retryable: retryable, Fatal: fatal}
	_ = c.sendControl(ctx, xinzhili.EventError, p, nil, nil)
}
func (c *xinzhiliRealtimeConn) close(code int, text string) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	sess := c.sess
	c.sess = nil
	c.sessionEpoch++
	c.mu.Unlock()
	if sess != nil {
		_ = sess.Close()
	}
	_ = c.ws.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(code, text), time.Now().Add(time.Second))
	_ = c.ws.Close()
}

func (s *xinzhiliWSSink) SendControl(ctx context.Context, event xinzhili.Envelope) error {
	return s.write(ctx, func() error {
		if event.Type == xinzhili.EventSessionReady || event.Type == xinzhili.EventModeChanged || event.Type == xinzhili.EventConfigChanged {
			s.conn.mu.Lock()
			currentConfigVersion := s.conn.configVersion
			s.conn.mu.Unlock()
			if event.ConfigVersion < currentConfigVersion && event.Type == xinzhili.EventConfigChanged {
				return nil
			}
			if event.ConfigVersion < currentConfigVersion {
				return errXinzhiliConfigSuperseded
			}
		}
		if event.Generation == 0 {
			event.Generation = s.conn.generation
		}
		if event.SessionID == nil {
			sid := s.conn.sessionID
			event.SessionID = &sid
		}
		seq := s.nextSessionSeq
		s.nextSessionSeq++
		event.SessionSeq = &seq
		if event.ConfigVersion == 0 {
			s.conn.mu.Lock()
			event.ConfigVersion = s.conn.configVersion
			s.conn.mu.Unlock()
		}
		if event.TimestampMs == 0 {
			event.TimestampMs = time.Now().UnixMilli()
		}
		s.conn.mu.Lock()
		sequence := s.conn.sequence
		s.conn.mu.Unlock()
		if sequence != nil {
			disposition, err := sequence.Observe(xinzhili.DirectionServer, event)
			if err != nil {
				return err
			}
			if disposition == xinzhili.SequenceDrop {
				return nil
			}
		}
		b, err := xinzhili.EncodeEnvelope(event, xinzhili.DirectionServer, true)
		if err != nil {
			return err
		}
		return s.conn.ws.WriteMessage(websocket.TextMessage, b)
	})
}
func (s *xinzhiliWSSink) SendAudio(ctx context.Context, seg xinzhili.AudioSegment) error {
	return s.write(ctx, func() error {
		if seg.TurnKey == 0 {
			return errors.New("xinzhili: audio segment turn key missing")
		}
		b, err := xinzhili.EncodeBinaryFrame(xinzhili.BinaryFrame{FrameType: xinzhili.FrameTypeAssistantMP3, Flags: xinzhili.FlagStart | xinzhili.FlagEnd, Generation: s.conn.generation, TurnKey: seg.TurnKey, SegmentSeq: seg.Seq, Payload: seg.Audio})
		if err != nil {
			return err
		}
		return s.conn.ws.WriteMessage(websocket.BinaryMessage, b)
	})
}
func (s *xinzhiliWSSink) write(ctx context.Context, fn func() error) error {
	for !s.mu.TryLock() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Millisecond):
		}
	}
	defer s.mu.Unlock()
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	deadline := time.Now().Add(xinzhiliWriteTimeout)
	if contextDeadline, ok := ctx.Deadline(); ok && contextDeadline.Before(deadline) {
		deadline = contextDeadline
	}
	if err := s.conn.ws.SetWriteDeadline(deadline); err != nil {
		return err
	}
	return fn()
}

func randomSessionID() string { var b [12]byte; _, _ = rand.Read(b[:]); return "xz-" + stringHex(b[:]) }
func stringHex(b []byte) string {
	const h = "0123456789abcdef"
	out := make([]byte, len(b)*2)
	for i, v := range b {
		out[i*2] = h[v>>4]
		out[i*2+1] = h[v&15]
	}
	return string(out)
}
func modeEnabled(m []xinzhili.Mode, v xinzhili.Mode) bool {
	for _, x := range m {
		if x == v {
			return true
		}
	}
	return false
}
func knownXinzhiliMode(m xinzhili.Mode) bool {
	return m == xinzhili.ModeNormal || m == xinzhili.ModeArgument || m == xinzhili.ModeComfort || m == xinzhili.ModeDeepListening
}
