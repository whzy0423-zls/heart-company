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
	xinzhiliProtocolUpdateMessage = "语音服务正在更新，请稍后重试"
)

var xinzhiliUpgrader = websocket.Upgrader{
	ReadBufferSize:    16 << 10,
	WriteBufferSize:   16 << 10,
	EnableCompression: true,
}

var errXinzhiliModeDisabled = errors.New("xinzhili mode is disabled")

type xinzhiliModePreferenceStore interface {
	ReadMode(context.Context, int64) (xinzhili.ModePreference, bool, error)
	UpdateMode(context.Context, int64, xinzhili.Mode, int64) (xinzhili.ModePreference, error)
}

type xinzhiliModeSnapshot = xinzhili.ModeSnapshot

type xinzhiliRealtimeConn struct {
	server         *Server
	ws             *websocket.Conn
	userID         int64
	sink           *xinzhiliWSSink
	sess           xinzhili.TurnSession
	mu             sync.Mutex
	closed         bool
	sessionID      string
	configVersion  int64
	generation     uint32
	cardID         int64
	conversationID int64
	requestedMode  xinzhili.Mode
	pendingMode    xinzhili.Mode
	effectiveMode  xinzhili.Mode
	modeRevision   int64
	modeStore      xinzhiliModePreferenceStore
	readConfig     func(context.Context) (xinzhili.Config, bool, error)
	turnKey        uint64
	turnMu         sync.Mutex
	turns          map[uint64]string
}

type xinzhiliWSSink struct {
	conn *xinzhiliRealtimeConn
	mu   sync.Mutex
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
	c := &xinzhiliRealtimeConn{server: s, ws: ws, userID: user.ID, turns: make(map[uint64]string), modeStore: xinzhili.NewStore(s.db)}
	c.sink = &xinzhiliWSSink{conn: c}
	s.replaceXinzhiliLease(c)
	defer s.releaseXinzhiliLease(c)
	defer c.close(websocket.CloseNormalClosure, "")
	c.readLoop(r.Context())
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
		code := "invalid_envelope"
		var pe *xinzhili.ProtocolError
		if errors.As(err, &pe) {
			code = pe.Code
		}
		logXinzhiliProtocolError(data, err)
		c.sendError(ctx, code, xinzhiliProtocolUpdateMessage, true, false)
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
			_ = c.sess.Cancel(ctx, *e.TurnID)
		}
	case xinzhili.EventPlaybackInterrupt:
		if c.sess != nil && e.TurnID != nil {
			_ = c.sess.Interrupt(ctx, *e.TurnID)
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
	cfg, found, err := c.currentConfig(ctx)
	if err != nil || !found || !cfg.Enabled {
		c.sendError(ctx, "xinzhili_not_configured", "请先配置芯之力会话模型后重试", false, true)
		return
	}
	modeSnapshot, err := c.loadModeSnapshot(ctx, cfg)
	if err != nil {
		c.sendError(ctx, "mode_preference_unavailable", "模式偏好读取失败", true, true)
		return
	}
	deps, err := c.server.newXinzhiliRealtimeDependencies(cfg, c.sink)
	if err != nil {
		c.sendError(ctx, "xinzhili_dependencies_unavailable", "芯之力服务暂不可用", true, true)
		return
	}
	c.sess = xinzhili.NewSession(deps)
	c.configVersion = cfg.Version
	readyPayload := map[string]any{"sessionId": c.sessionID, "cardId": p.CardID, "conversationId": p.ConversationID}
	mergeXinzhiliModeSnapshot(readyPayload, modeSnapshot)
	c.sendControl(ctx, xinzhili.EventSessionReady, readyPayload, nil, nil)
}

func (c *xinzhiliRealtimeConn) startTurn(ctx context.Context, e xinzhili.Envelope) {
	if c.sess == nil || e.TurnID == nil {
		c.sendError(ctx, "session_not_ready", "会话尚未就绪", true, false)
		return
	}
	var p struct {
		TurnKey uint64 `json:"turnKey"`
	}
	if json.Unmarshal(e.Payload, &p) != nil {
		c.sendError(ctx, "invalid_turn", "轮次参数无效", false, false)
		return
	}
	cfg, found, err := c.currentConfig(ctx)
	if err != nil || !found || !cfg.Enabled {
		c.sendError(ctx, "xinzhili_not_configured", "请先配置芯之力会话模型后重试", false, true)
		return
	}
	c.mu.Lock()
	mode := c.pendingMode
	if !modeEnabled(cfg.EnabledModes, mode) {
		mode = xinzhili.ModeNormal
		c.requestedMode = mode
		c.pendingMode = mode
	}
	cardID, conversationID := c.cardID, c.conversationID
	c.mu.Unlock()
	in := xinzhili.StartTurnInput{UserID: c.userID, CardID: cardID, ConversationID: conversationID, TurnID: *e.TurnID, Mode: mode, ASRConfig: cfg.RealtimeASR, TTSConfig: cfg.TTS, Timing: cfg.Timing, CommonPrompt: cfg.CommonPrompt, ModePrompt: cfg.ModePrompts[mode], KnowledgeTopK: 6, KnowledgeMinScore: 0.2, TheoryTopK: 6, TheoryMinScore: 0.2}
	if err := c.sess.StartTurn(ctx, in); err != nil {
		c.sendError(ctx, "turn_start_failed", "无法开始当前轮次", true, false)
		return
	}
	c.mu.Lock()
	c.effectiveMode = mode
	c.mu.Unlock()
	_ = c.sendControl(ctx, xinzhili.EventModeChanged, c.modeSnapshot(cfg), nil, nil)
	c.turnMu.Lock()
	c.turns[p.TurnKey] = *e.TurnID
	c.turnMu.Unlock()
	c.mu.Lock()
	c.turnKey = p.TurnKey
	c.mu.Unlock()
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
	c.turnMu.Lock()
	turnID := c.turns[f.TurnKey]
	c.turnMu.Unlock()
	if turnID == "" {
		c.sendError(ctx, "unknown_turn", "音频帧对应的轮次不存在", false, false)
		return
	}
	if err := c.sess.PushPCM(ctx, xinzhili.PCMFrame{TurnID: turnID, Data: f.Payload}); err != nil {
		c.sendError(ctx, "audio_frame_rejected", "音频帧未被接收", true, false)
	}
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
	cfg, found, err := c.currentConfig(ctx)
	if err != nil || !found || !cfg.Enabled {
		c.sendError(ctx, "xinzhili_not_configured", "请先配置芯之力会话模型后重试", false, false)
		return
	}
	snapshot, err := c.persistModeChange(ctx, cfg, p.Mode, p.ExpectedRevision)
	if errors.Is(err, errXinzhiliModeDisabled) {
		c.sendError(ctx, "mode_not_enabled", "当前模式尚未启用", false, false)
		return
	}
	if errors.Is(err, xinzhili.ErrModePreferenceConflict) {
		refreshed, refreshErr := c.refreshModeSnapshot(ctx, cfg)
		if refreshErr != nil {
			c.sendError(ctx, "mode_change_failed", "模式切换失败", true, false)
			return
		}
		c.sendErrorWithModeSnapshot(ctx, "mode_revision_conflict", "模式状态已更新，请同步后重试", true, false, refreshed)
		return
	}
	if err != nil {
		c.sendError(ctx, "mode_change_failed", "模式切换失败", true, false)
		return
	}
	c.sendControl(ctx, xinzhili.EventModeChanged, snapshot, nil, nil)
}

func (c *xinzhiliRealtimeConn) currentConfig(ctx context.Context) (xinzhili.Config, bool, error) {
	if c.readConfig != nil {
		return c.readConfig(ctx)
	}
	return xinzhili.ReadConfig(ctx, c.server.db)
}

func (c *xinzhiliRealtimeConn) loadModeSnapshot(ctx context.Context, cfg xinzhili.Config) (xinzhiliModeSnapshot, error) {
	preference, found, err := c.modeStore.ReadMode(ctx, c.userID)
	if err != nil {
		return xinzhiliModeSnapshot{}, err
	}
	mode, revision := xinzhili.ModeNormal, int64(0)
	if found {
		revision = preference.Revision
	}
	if !modeEnabled(cfg.EnabledModes, mode) {
		mode = xinzhili.ModeNormal
	}
	c.mu.Lock()
	c.requestedMode, c.pendingMode, c.effectiveMode, c.modeRevision = mode, mode, mode, revision
	c.mu.Unlock()
	return c.modeSnapshot(cfg), nil
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

func (c *xinzhiliRealtimeConn) refreshModeSnapshot(ctx context.Context, cfg xinzhili.Config) (xinzhiliModeSnapshot, error) {
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
	c.mu.Lock()
	c.requestedMode = mode
	c.pendingMode = mode
	c.modeRevision = revision
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

func mergeXinzhiliModeSnapshot(dst map[string]any, snapshot xinzhiliModeSnapshot) {
	dst["enabledModes"] = snapshot.EnabledModes
	dst["requestedMode"] = snapshot.RequestedMode
	dst["pendingMode"] = snapshot.PendingMode
	dst["effectiveMode"] = snapshot.EffectiveMode
	dst["revision"] = snapshot.Revision
	dst["configVersion"] = snapshot.ConfigVersion
}

func (c *xinzhiliRealtimeConn) sendControl(ctx context.Context, typ xinzhili.EventType, payload any, turnID *string, turnSeq *uint64) error {
	b, _ := json.Marshal(payloadOrEmpty(payload))
	sid := c.sessionID
	seq := uint64(time.Now().UnixNano())
	e := xinzhili.Envelope{ProtocolVersion: xinzhili.ProtocolVersion, Type: typ, SessionID: &sid, SessionSeq: &seq, TurnID: turnID, TurnSeq: turnSeq, ConfigVersion: c.configVersion, TimestampMs: time.Now().UnixMilli(), Payload: b}
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
func (c *xinzhiliRealtimeConn) sendErrorWithModeSnapshot(ctx context.Context, code, msg string, retryable, fatal bool, snapshot xinzhiliModeSnapshot) {
	p := xinzhili.ErrorPayload{Code: code, Message: msg, Retryable: retryable, Fatal: fatal, ModeSnapshot: &snapshot}
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
	c.mu.Unlock()
	if sess != nil {
		_ = sess.Close()
	}
	_ = c.ws.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(code, text), time.Now().Add(time.Second))
	_ = c.ws.Close()
}

func (s *xinzhiliWSSink) SendControl(ctx context.Context, event xinzhili.Envelope) error {
	if event.Generation == 0 {
		event.Generation = s.conn.generation
	}
	if event.SessionID == nil {
		sid := s.conn.sessionID
		event.SessionID = &sid
	}
	if event.SessionSeq == nil {
		seq := uint64(time.Now().UnixNano())
		event.SessionSeq = &seq
	}
	if event.ConfigVersion == 0 {
		event.ConfigVersion = s.conn.configVersion
	}
	if event.TimestampMs == 0 {
		event.TimestampMs = time.Now().UnixMilli()
	}
	return s.write(ctx, func() error {
		b, err := xinzhili.EncodeEnvelope(event, xinzhili.DirectionServer, true)
		if err != nil {
			return err
		}
		return s.conn.ws.WriteMessage(websocket.TextMessage, b)
	})
}
func (s *xinzhiliWSSink) SendAudio(ctx context.Context, seg xinzhili.AudioSegment) error {
	return s.write(ctx, func() error {
		s.conn.mu.Lock()
		turnKey := s.conn.turnKey
		s.conn.mu.Unlock()
		b, err := xinzhili.EncodeBinaryFrame(xinzhili.BinaryFrame{FrameType: xinzhili.FrameTypeAssistantMP3, Flags: xinzhili.FlagStart | xinzhili.FlagEnd, Generation: s.conn.generation, TurnKey: turnKey, SegmentSeq: seg.Seq, Payload: seg.Audio})
		if err != nil {
			return err
		}
		return s.conn.ws.WriteMessage(websocket.BinaryMessage, b)
	})
}
func (s *xinzhiliWSSink) write(ctx context.Context, fn func() error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
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
