package xinzhili

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/gorilla/websocket"
)

const (
	asrEventBuffer         = 64
	asrMaxJSONMessageBytes = 1 << 20
	asrMaxTranscriptBytes  = 64 << 10
	asrMaxTranscriptRunes  = 64 << 10
)

type ASRClock interface {
	Now() time.Time
}

type ASRWebSocketDialer interface {
	DialContext(ctx context.Context, urlStr string, requestHeader http.Header) (*websocket.Conn, *http.Response, error)
}

type asrWebSocketConn interface {
	Close() error
	ReadMessage() (messageType int, p []byte, err error)
	SetReadDeadline(t time.Time) error
	SetReadLimit(limit int64)
	SetWriteDeadline(t time.Time) error
	WriteControl(messageType int, data []byte, deadline time.Time) error
	WriteJSON(v any) error
	WriteMessage(messageType int, data []byte) error
}

type AliyunASRTimeouts struct {
	Handshake     time.Duration
	FirstEvent    time.Duration
	Write         time.Duration
	Close         time.Duration
	EventDelivery time.Duration
}

type AliyunASROptions struct {
	Dialer   ASRWebSocketDialer
	Clock    ASRClock
	Timeouts AliyunASRTimeouts
}

type AliyunASRFactory struct {
	dialer   ASRWebSocketDialer
	clock    ASRClock
	timeouts AliyunASRTimeouts
	wrapConn func(*websocket.Conn) asrWebSocketConn
}

type systemASRClock struct{}

func (systemASRClock) Now() time.Time { return time.Now() }

func NewAliyunASRFactory(options AliyunASROptions) *AliyunASRFactory {
	timeouts := options.Timeouts
	if timeouts.Handshake <= 0 {
		timeouts.Handshake = 10 * time.Second
	}
	if timeouts.FirstEvent <= 0 {
		timeouts.FirstEvent = 10 * time.Second
	}
	if timeouts.Write <= 0 {
		timeouts.Write = 5 * time.Second
	}
	if timeouts.Close <= 0 {
		timeouts.Close = 5 * time.Second
	}
	if timeouts.EventDelivery <= 0 {
		timeouts.EventDelivery = 250 * time.Millisecond
	}
	clock := options.Clock
	if clock == nil {
		clock = systemASRClock{}
	}
	dialer := options.Dialer
	if dialer == nil {
		copy := *websocket.DefaultDialer
		copy.HandshakeTimeout = timeouts.Handshake
		dialer = &copy
	}
	return &AliyunASRFactory{
		dialer: dialer, clock: clock, timeouts: timeouts,
		wrapConn: func(conn *websocket.Conn) asrWebSocketConn { return conn },
	}
}

func (f *AliyunASRFactory) Open(ctx context.Context, cfg RealtimeASRConfig) (ASRSession, error) {
	endpoint, err := validateASRRuntimeConfig(cfg)
	if err != nil {
		return nil, err
	}
	taskID, err := newASRTaskID()
	if err != nil {
		return nil, fmt.Errorf("生成实时语音识别任务 ID: %w", err)
	}

	dialCtx, cancelDial := context.WithTimeout(ctx, f.timeouts.Handshake)
	defer cancelDial()
	header := make(http.Header)
	header.Set("Authorization", "Bearer "+cfg.APIKey)
	rawConn, response, err := f.dialer.DialContext(dialCtx, endpoint, header)
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
	if err != nil {
		if errors.Is(dialCtx.Err(), context.DeadlineExceeded) {
			return nil, fmt.Errorf("%w: WebSocket 握手", ErrASRTimeout)
		}
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			return nil, fmt.Errorf("%w: WebSocket 握手: %w", ErrASRTimeout, err)
		}
		return nil, fmt.Errorf("连接实时语音识别上游: %w", err)
	}
	conn := f.wrapConn(rawConn)
	conn.SetReadLimit(asrMaxJSONMessageBytes)

	if err := writeASRJSON(ctx, conn, f.timeouts.Write, runTaskMessage(taskID)); err != nil {
		_ = conn.Close()
		return nil, err
	}
	waitDone := make(chan struct{})
	watcherDone := make(chan struct{})
	go func() {
		defer close(watcherDone)
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-waitDone:
		}
	}()
	err = waitForTaskStarted(ctx, conn, taskID, f.timeouts.FirstEvent)
	close(waitDone)
	<-watcherDone
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		_ = conn.Close()
		return nil, err
	}
	_ = conn.SetReadDeadline(time.Time{})

	sessionCtx, cancelSession := context.WithCancel(ctx)
	session := &aliyunASRSession{
		conn:       conn,
		clock:      f.clock,
		timeouts:   f.timeouts,
		taskID:     taskID,
		ctx:        sessionCtx,
		cancel:     cancelSession,
		events:     make(chan ASREvent, asrEventBuffer),
		done:       make(chan struct{}),
		finishDone: make(chan struct{}),
	}
	go session.readLoop()
	go session.watchContext()
	return session, nil
}

func validateASRRuntimeConfig(cfg RealtimeASRConfig) (string, error) {
	if strings.TrimSpace(cfg.Provider) != RealtimeASRProvider {
		return "", fmt.Errorf("实时 ASR provider 必须为 %s", RealtimeASRProvider)
	}
	if strings.TrimSpace(cfg.Model) != RealtimeASRModel {
		return "", fmt.Errorf("实时 ASR model 必须为 %s", RealtimeASRModel)
	}
	if strings.TrimSpace(cfg.APIKey) == "" {
		return "", errors.New("实时 ASR API Key 不能为空")
	}
	parsed, err := url.Parse(strings.TrimSpace(cfg.Endpoint))
	if err != nil || parsed.Host == "" {
		return "", errors.New("实时 ASR endpoint 无效")
	}
	switch parsed.Scheme {
	case "wss", "ws":
	case "https":
		parsed.Scheme = "wss"
	default:
		return "", errors.New("实时 ASR endpoint 必须使用 wss、ws 或 https")
	}
	return parsed.String(), nil
}

type aliyunASRSession struct {
	conn     asrWebSocketConn
	clock    ASRClock
	timeouts AliyunASRTimeouts
	taskID   string

	ctx    context.Context
	cancel context.CancelFunc
	events chan ASREvent
	done   chan struct{}

	writeMu        sync.Mutex
	stateMu        sync.RWMutex
	finished       bool
	finishDone     chan struct{}
	finishErr      error
	finishComplete bool
	terminal       bool
	closed         bool
	err            error
	speechActive   bool
	closeOnce      sync.Once
	finalizeOnce   sync.Once
}

func (s *aliyunASRSession) Events() <-chan ASREvent { return s.events }

func (s *aliyunASRSession) Err() error {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	return s.err
}

func (s *aliyunASRSession) WritePCM(ctx context.Context, pcm []byte) error {
	if len(pcm) == 0 {
		return ErrASREmptyPCM
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	s.stateMu.RLock()
	finished, closed := s.finished, s.closed
	s.stateMu.RUnlock()
	if closed {
		return ErrASRClosed
	}
	if finished {
		return ErrASRInputFinished
	}
	if err := setASRWriteDeadline(ctx, s.conn, s.timeouts.Write); err != nil {
		return err
	}
	if err := s.conn.WriteMessage(websocket.BinaryMessage, pcm); err != nil {
		mapped := mapASRWriteError(err)
		s.fail(mapped)
		return mapped
	}
	return nil
}

func (s *aliyunASRSession) FinishInput(ctx context.Context) error {
	s.stateMu.Lock()
	if s.finished {
		done := s.finishDone
		s.stateMu.Unlock()
		<-done
		return s.sharedFinishErr()
	}
	if s.closed {
		s.stateMu.Unlock()
		return ErrASRClosed
	}
	if err := ctx.Err(); err != nil {
		s.stateMu.Unlock()
		return err
	}
	s.finished = true
	s.stateMu.Unlock()

	s.writeMu.Lock()
	err := writeASRJSON(ctx, s.conn, s.timeouts.Write, finishTaskMessage(s.taskID))
	s.writeMu.Unlock()
	if err != nil {
		s.fail(err)
		<-s.finishDone
		return s.sharedFinishErr()
	}
	timer := time.NewTimer(s.timeouts.Close)
	defer timer.Stop()
	select {
	case <-s.finishDone:
	case <-ctx.Done():
		s.fail(ctx.Err())
		<-s.finishDone
	case <-timer.C:
		s.fail(fmt.Errorf("%w: 等待 task-finished", ErrASRTimeout))
		<-s.finishDone
	}
	return s.sharedFinishErr()
}

func (s *aliyunASRSession) Close() error {
	s.closeOnce.Do(func() {
		s.markTerminal(nil, ErrASRClosed)
		s.cancel()
		deadline := time.Now().Add(s.timeouts.Close)
		_ = s.conn.WriteControl(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), deadline)
		_ = s.conn.Close()
	})
	select {
	case <-s.done:
		return nil
	case <-time.After(s.timeouts.Close):
		return fmt.Errorf("%w: 关闭会话", ErrASRTimeout)
	}
}

func (s *aliyunASRSession) watchContext() {
	select {
	case <-s.ctx.Done():
		select {
		case <-s.done:
			return
		default:
		}
		s.stateMu.RLock()
		closed := s.closed
		s.stateMu.RUnlock()
		if !closed {
			s.fail(s.ctx.Err())
		}
	case <-s.done:
	}
}

func (s *aliyunASRSession) readLoop() {
	defer s.finalize()
	for {
		messageType, raw, err := s.conn.ReadMessage()
		if err != nil {
			s.handleReadError(err)
			return
		}
		if messageType != websocket.TextMessage {
			s.fail(fmt.Errorf("%w: 上游返回非文本控制消息", ErrASRProtocol))
			return
		}
		message, err := decodeASRServerMessage(raw)
		if err != nil {
			s.fail(err)
			return
		}
		if message.Header.TaskID != s.taskID {
			continue
		}
		switch message.Header.Event {
		case "result-generated":
			at := s.clock.Now()
			event, ok, normalizeErr := normalizeASRResult(message, at)
			if normalizeErr != nil {
				s.fail(normalizeErr)
				return
			}
			if ok {
				if !s.speechActive {
					s.emitNonCritical(ASREvent{Kind: ASREventSpeechStarted, TaskID: s.taskID, At: at})
					s.speechActive = true
				}
				if event.Kind == ASREventPartial {
					s.emitNonCritical(event)
				} else if !s.emitCritical(event) {
					return
				}
				if event.Kind == ASREventFinal {
					s.speechActive = false
				}
			}
		case "task-finished":
			if !s.emitCritical(ASREvent{Kind: ASREventTaskFinished, TaskID: s.taskID, At: s.clock.Now()}) {
				return
			}
			s.markTerminal(nil, nil)
			return
		case "task-failed":
			err := newASRUpstreamError(message)
			s.fail(err)
			return
		case "task-started":
			// A duplicate acknowledgement is harmless after Open has completed.
		default:
			// DashScope may add task-scoped informational events. Unknown events do
			// not alter the audio stream or the normalized consumer contract.
		}
	}
}

// Activity and partial events are advisory. When the bounded queue is full we
// drop the current advisory event so the sole WebSocket reader keeps moving.
func (s *aliyunASRSession) emitNonCritical(event ASREvent) {
	select {
	case s.events <- event:
	default:
	}
}

func (s *aliyunASRSession) emitCritical(event ASREvent) bool {
	timer := time.NewTimer(s.timeouts.EventDelivery)
	defer timer.Stop()
	select {
	case s.events <- event:
		return true
	case <-s.ctx.Done():
		return false
	case <-timer.C:
		s.fail(ErrASRBackpressure)
		return false
	}
}

func (s *aliyunASRSession) handleReadError(err error) {
	s.stateMu.RLock()
	terminal := s.terminal
	finished := s.finished
	s.stateMu.RUnlock()
	if terminal {
		return
	}
	if contextErr := s.ctx.Err(); contextErr != nil {
		s.fail(contextErr)
		return
	}
	var netErr net.Error
	if errors.Is(err, websocket.ErrReadLimit) {
		s.fail(ErrASRMessageTooLarge)
		return
	}
	if finished && errors.As(err, &netErr) && netErr.Timeout() {
		mapped := fmt.Errorf("%w: 等待 task-finished", ErrASRTimeout)
		s.fail(mapped)
		return
	}
	mapped := fmt.Errorf("%w: %v", ErrASRDisconnected, err)
	s.fail(mapped)
}

func (s *aliyunASRSession) fail(err error) {
	if s.markTerminal(err, err) {
		_ = s.conn.Close()
	}
}

func (s *aliyunASRSession) markTerminal(sessionErr, finishErr error) bool {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	if s.terminal {
		return false
	}
	s.terminal = true
	s.closed = true
	if sessionErr != nil && s.err == nil {
		s.err = sessionErr
	}
	if !s.finishComplete {
		s.finishComplete = true
		s.finishErr = finishErr
		close(s.finishDone)
	}
	return true
}

func (s *aliyunASRSession) sharedFinishErr() error {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	return s.finishErr
}

func (s *aliyunASRSession) finalize() {
	s.finalizeOnce.Do(func() {
		s.markTerminal(nil, ErrASRClosed)
		_ = s.conn.Close()
		close(s.events)
		close(s.done)
		s.cancel()
	})
}

type asrServerMessage struct {
	Header struct {
		TaskID       string `json:"task_id"`
		Event        string `json:"event"`
		ErrorCode    string `json:"error_code"`
		ErrorMessage string `json:"error_message"`
	} `json:"header"`
	Payload struct {
		Output struct {
			Sentence struct {
				Text        string `json:"text"`
				SentenceEnd bool   `json:"sentence_end"`
				BeginTime   int64  `json:"begin_time"`
			} `json:"sentence"`
		} `json:"output"`
	} `json:"payload"`
}

func decodeASRServerMessage(raw []byte) (asrServerMessage, error) {
	var message asrServerMessage
	if err := json.Unmarshal(raw, &message); err != nil {
		return asrServerMessage{}, fmt.Errorf("%w: 无法解析上游事件", ErrASRProtocol)
	}
	if message.Header.TaskID == "" || message.Header.Event == "" {
		return asrServerMessage{}, fmt.Errorf("%w: 上游事件缺少 header", ErrASRProtocol)
	}
	return message, nil
}

func waitForTaskStarted(ctx context.Context, conn asrWebSocketConn, taskID string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	if contextDeadline, ok := ctx.Deadline(); ok && contextDeadline.Before(deadline) {
		deadline = contextDeadline
	}
	if err := conn.SetReadDeadline(deadline); err != nil {
		return fmt.Errorf("设置实时语音识别首事件超时: %w", err)
	}
	for {
		messageType, raw, err := conn.ReadMessage()
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
			if errors.Is(err, websocket.ErrReadLimit) {
				return ErrASRMessageTooLarge
			}
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				return fmt.Errorf("%w: 等待 task-started", ErrASRTimeout)
			}
			return fmt.Errorf("%w: 等待 task-started: %v", ErrASRDisconnected, err)
		}
		if messageType != websocket.TextMessage {
			return fmt.Errorf("%w: task-started 前收到非文本消息", ErrASRProtocol)
		}
		message, err := decodeASRServerMessage(raw)
		if err != nil {
			return err
		}
		if message.Header.TaskID != taskID {
			continue
		}
		switch message.Header.Event {
		case "task-started":
			return nil
		case "task-failed":
			return newASRUpstreamError(message)
		default:
			return fmt.Errorf("%w: task-started 前收到 %s", ErrASRProtocol, message.Header.Event)
		}
	}
}

func normalizeASRResult(message asrServerMessage, at time.Time) (ASREvent, bool, error) {
	text := message.Payload.Output.Sentence.Text
	if text == "" {
		return ASREvent{}, false, nil
	}
	if len(text) > asrMaxTranscriptBytes || utf8.RuneCountInString(text) > asrMaxTranscriptRunes {
		return ASREvent{}, false, fmt.Errorf("%w: 单条识别文本超过限制", ErrASRMessageTooLarge)
	}
	if message.Payload.Output.Sentence.SentenceEnd {
		return ASREvent{Kind: ASREventFinal, Final: text, Stable: true, TaskID: message.Header.TaskID, At: at}, true, nil
	}
	return ASREvent{Kind: ASREventPartial, Partial: text, TaskID: message.Header.TaskID, At: at}, true, nil
}

func newASRUpstreamError(message asrServerMessage) error {
	return &ASRUpstreamError{Code: message.Header.ErrorCode, Message: message.Header.ErrorMessage}
}

func runTaskMessage(taskID string) any {
	return map[string]any{
		"header": map[string]any{
			"action": "run-task", "task_id": taskID, "streaming": "duplex",
		},
		"payload": map[string]any{
			"task_group": "audio", "task": "asr", "function": "recognition", "model": RealtimeASRModel,
			// DashScope exposes mono as an input-audio contract rather than a
			// run-task parameter. Callers must provide PCM16LE 16 kHz mono bytes.
			"parameters": map[string]any{"format": "pcm", "sample_rate": 16000},
			"input":      map[string]any{},
		},
	}
}

func finishTaskMessage(taskID string) any {
	return map[string]any{
		"header": map[string]any{
			"action": "finish-task", "task_id": taskID, "streaming": "duplex",
		},
		"payload": map[string]any{"input": map[string]any{}},
	}
}

func writeASRJSON(ctx context.Context, conn asrWebSocketConn, timeout time.Duration, value any) error {
	if err := setASRWriteDeadline(ctx, conn, timeout); err != nil {
		return err
	}
	if err := conn.WriteJSON(value); err != nil {
		return mapASRWriteError(err)
	}
	return nil
}

func setASRWriteDeadline(ctx context.Context, conn asrWebSocketConn, timeout time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	deadline := time.Now().Add(timeout)
	if contextDeadline, ok := ctx.Deadline(); ok && contextDeadline.Before(deadline) {
		deadline = contextDeadline
	}
	if err := conn.SetWriteDeadline(deadline); err != nil {
		return fmt.Errorf("设置实时语音识别写入超时: %w", err)
	}
	return nil
}

func mapASRWriteError(err error) error {
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return fmt.Errorf("%w: 写入上游", ErrASRTimeout)
	}
	return fmt.Errorf("%w: 写入上游: %v", ErrASRDisconnected, err)
}

func newASRTaskID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	raw[6] = (raw[6] & 0x0f) | 0x40
	raw[8] = (raw[8] & 0x3f) | 0x80
	var encoded [36]byte
	hex.Encode(encoded[0:8], raw[0:4])
	encoded[8] = '-'
	hex.Encode(encoded[9:13], raw[4:6])
	encoded[13] = '-'
	hex.Encode(encoded[14:18], raw[6:8])
	encoded[18] = '-'
	hex.Encode(encoded[19:23], raw[8:10])
	encoded[23] = '-'
	hex.Encode(encoded[24:36], raw[10:16])
	return string(encoded[:]), nil
}
