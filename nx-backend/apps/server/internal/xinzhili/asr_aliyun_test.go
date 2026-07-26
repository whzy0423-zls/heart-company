package xinzhili

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

const testASRKey = "test-asr-secret"

type fixedASRClock struct{ now time.Time }

func (c fixedASRClock) Now() time.Time { return c.now }

type trackingReadDeadlineConn struct {
	*websocket.Conn
	mu                sync.Mutex
	readDeadlineCalls int
}

func (c *trackingReadDeadlineConn) SetReadDeadline(deadline time.Time) error {
	c.mu.Lock()
	c.readDeadlineCalls++
	c.mu.Unlock()
	return c.Conn.SetReadDeadline(deadline)
}

func (c *trackingReadDeadlineConn) calls() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.readDeadlineCalls
}

type asrWireMessage struct {
	Header struct {
		Action       string `json:"action"`
		Event        string `json:"event"`
		TaskID       string `json:"task_id"`
		ErrorCode    string `json:"error_code"`
		ErrorMessage string `json:"error_message"`
	} `json:"header"`
	Payload struct {
		TaskGroup  string `json:"task_group"`
		Task       string `json:"task"`
		Function   string `json:"function"`
		Model      string `json:"model"`
		Parameters struct {
			Format     string `json:"format"`
			SampleRate int    `json:"sample_rate"`
			Channels   *int   `json:"channels"`
		} `json:"parameters"`
	} `json:"payload"`
}

type fakeASRServer struct {
	t      *testing.T
	server *httptest.Server
	url    string
	done   chan struct{}
	errors chan error
	onConn func(*fakeASRPeer)
}

type fakeASRPeer struct {
	conn    *websocket.Conn
	request *http.Request
	errors  chan<- error
}

func (p *fakeASRPeer) reportf(format string, args ...any) {
	select {
	case p.errors <- fmt.Errorf(format, args...):
	default:
	}
}

func (p *fakeASRPeer) receiveRunTask() (asrWireMessage, bool) {
	messageType, raw, err := p.conn.ReadMessage()
	if err != nil {
		p.reportf("read run-task: %v", err)
		return asrWireMessage{}, false
	}
	if messageType != websocket.TextMessage {
		p.reportf("run-task message type=%d", messageType)
		return asrWireMessage{}, false
	}
	var message asrWireMessage
	if err := json.Unmarshal(raw, &message); err != nil {
		p.reportf("decode run-task: %v", err)
		return asrWireMessage{}, false
	}
	return message, true
}

func (p *fakeASRPeer) writeEvent(taskID, event string, payload any) bool {
	message := map[string]any{
		"header":  map[string]any{"task_id": taskID, "event": event},
		"payload": payload,
	}
	if err := p.conn.WriteJSON(message); err != nil {
		p.reportf("write %s: %v", event, err)
		return false
	}
	return true
}

func receiveRunTask(peer *fakeASRPeer) asrWireMessage {
	message, _ := peer.receiveRunTask()
	return message
}

func writeASREvent(peer *fakeASRPeer, taskID, event string, payload any) {
	peer.writeEvent(taskID, event, payload)
}

func newFakeASRServer(t *testing.T, onConn func(*fakeASRPeer)) *fakeASRServer {
	t.Helper()
	f := &fakeASRServer{t: t, done: make(chan struct{}), errors: make(chan error, 64), onConn: onConn}
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			f.errors <- fmt.Errorf("upgrade: %w", err)
			return
		}
		defer close(f.done)
		defer conn.Close()
		onConn(&fakeASRPeer{conn: conn, request: r, errors: f.errors})
	}))
	f.url = "ws" + strings.TrimPrefix(f.server.URL, "http")
	t.Cleanup(func() {
		f.server.CloseClientConnections()
		f.server.Close()
		select {
		case <-f.done:
		case <-time.After(time.Second):
			t.Error("fake ASR server handler did not exit")
		}
		for {
			select {
			case err := <-f.errors:
				t.Error(err)
			default:
				return
			}
		}
	})
	return f
}

func testASRConfig(endpoint string) RealtimeASRConfig {
	return RealtimeASRConfig{
		Provider: RealtimeASRProvider,
		Endpoint: endpoint,
		APIKey:   testASRKey,
		Region:   "cn-beijing",
		Model:    RealtimeASRModel,
	}
}

func testASRFactory(clock ASRClock, timeouts AliyunASRTimeouts) *AliyunASRFactory {
	return NewAliyunASRFactory(AliyunASROptions{Clock: clock, Timeouts: timeouts})
}

func readASREvent(t *testing.T, events <-chan ASREvent) ASREvent {
	t.Helper()
	select {
	case event, ok := <-events:
		if !ok {
			t.Fatal("events closed unexpectedly")
		}
		return event
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for normalized ASR event")
		return ASREvent{}
	}
}

func waitASREventsClosed(t *testing.T, events <-chan ASREvent) {
	t.Helper()
	select {
	case _, ok := <-events:
		if ok {
			t.Fatal("expected events channel to close")
		}
	case <-time.After(time.Second):
		t.Fatal("events channel did not close")
	}
}

func drainASREventsUntilClosed(t *testing.T, events <-chan ASREvent) {
	t.Helper()
	deadline := time.NewTimer(time.Second)
	defer deadline.Stop()
	for {
		select {
		case _, ok := <-events:
			if !ok {
				return
			}
		case <-deadline.C:
			t.Fatal("events channel did not close while draining")
		}
	}
}

func TestASRLifecycleAuthenticatesAndKeepsTaskIDAcrossStrictSequence(t *testing.T) {
	pcm := []byte{0x01, 0x02, 0x03, 0x04}
	server := newFakeASRServer(t, func(peer *fakeASRPeer) {
		conn, request := peer.conn, peer.request
		if got := request.Header.Get("Authorization"); got != "Bearer "+testASRKey {
			peer.reportf("Authorization=%q", got)
		}
		run := receiveRunTask(peer)
		if run.Header.Action != "run-task" {
			peer.reportf("action=%q", run.Header.Action)
		}
		if !isStandardUUID(run.Header.TaskID) {
			peer.reportf("task_id=%q is not a standard UUID", run.Header.TaskID)
		}
		if run.Payload.TaskGroup != "audio" || run.Payload.Task != "asr" || run.Payload.Function != "recognition" {
			peer.reportf("unexpected task tuple: %+v", run.Payload)
		}
		if run.Payload.Model != RealtimeASRModel {
			peer.reportf("model=%q", run.Payload.Model)
		}
		if run.Payload.Parameters.Format != "pcm" || run.Payload.Parameters.SampleRate != 16000 || run.Payload.Parameters.Channels != nil {
			peer.reportf("parameters=%+v", run.Payload.Parameters)
		}
		writeASREvent(peer, run.Header.TaskID, "task-started", map[string]any{})

		messageType, gotPCM, err := conn.ReadMessage()
		if err != nil {
			peer.reportf("read PCM: %v", err)
			return
		}
		if messageType != websocket.BinaryMessage || string(gotPCM) != string(pcm) {
			peer.reportf("PCM type=%d bytes=%v", messageType, gotPCM)
		}

		finish := receiveRunTask(peer)
		if finish.Header.Action != "finish-task" || finish.Header.TaskID != run.Header.TaskID {
			peer.reportf("finish=%+v run task_id=%q", finish.Header, run.Header.TaskID)
		}
		writeASREvent(peer, run.Header.TaskID, "task-finished", map[string]any{})
	})

	factory := testASRFactory(fixedASRClock{now: time.Unix(123, 0)}, AliyunASRTimeouts{})
	session, err := factory.Open(context.Background(), testASRConfig(server.url))
	if err != nil {
		t.Fatal(err)
	}
	if err := session.WritePCM(context.Background(), pcm); err != nil {
		t.Fatal(err)
	}
	if err := session.FinishInput(context.Background()); err != nil {
		t.Fatal(err)
	}
	event := readASREvent(t, session.Events())
	if event.Kind != ASREventTaskFinished || event.TaskID == "" || !event.At.Equal(time.Unix(123, 0)) {
		t.Fatalf("event=%+v", event)
	}
	waitASREventsClosed(t, session.Events())
	if err := session.Err(); err != nil {
		t.Fatalf("normal finish Err=%v", err)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestASROpenWaitsForMatchingTaskStartedAndIgnoresOtherTasks(t *testing.T) {
	release := make(chan struct{})
	otherTaskStarted := make(chan struct{})
	server := newFakeASRServer(t, func(peer *fakeASRPeer) {
		run := receiveRunTask(peer)
		writeASREvent(peer, "00000000-0000-4000-8000-000000000000", "task-started", map[string]any{})
		close(otherTaskStarted)
		<-release
		writeASREvent(peer, run.Header.TaskID, "task-started", map[string]any{})
		_, _, _ = peer.conn.ReadMessage()
	})

	type result struct {
		session ASRSession
		err     error
	}
	resultCh := make(chan result, 1)
	go func() {
		session, err := testASRFactory(nil, AliyunASRTimeouts{FirstEvent: time.Second}).Open(context.Background(), testASRConfig(server.url))
		resultCh <- result{session: session, err: err}
	}()
	<-otherTaskStarted
	select {
	case got := <-resultCh:
		if got.session != nil {
			_ = got.session.Close()
		}
		t.Fatalf("Open returned before matching task-started: %v", got.err)
	default:
	}
	close(release)
	got := <-resultCh
	if got.err != nil {
		t.Fatal(got.err)
	}
	if err := got.session.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestASRNormalizesPartialFinalAndSpeechActivity(t *testing.T) {
	clock := fixedASRClock{now: time.Unix(456, 0)}
	server := newFakeASRServer(t, func(peer *fakeASRPeer) {
		run := receiveRunTask(peer)
		writeASREvent(peer, run.Header.TaskID, "task-started", map[string]any{})
		writeASREvent(peer, "00000000-0000-4000-8000-000000000000", "result-generated", map[string]any{
			"output": map[string]any{"sentence": map[string]any{"text": "必须忽略", "sentence_end": true}},
		})
		writeASREvent(peer, run.Header.TaskID, "result-generated", map[string]any{
			"output": map[string]any{"sentence": map[string]any{"text": "你好", "sentence_end": false, "begin_time": 10}},
		})
		writeASREvent(peer, run.Header.TaskID, "result-generated", map[string]any{
			"output": map[string]any{"sentence": map[string]any{"text": "你好呀", "sentence_end": true, "begin_time": 10}},
		})
		writeASREvent(peer, run.Header.TaskID, "task-finished", map[string]any{})
	})

	session, err := testASRFactory(clock, AliyunASRTimeouts{}).Open(context.Background(), testASRConfig(server.url))
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	want := []ASREvent{
		{Kind: ASREventSpeechStarted, TaskID: sessionTaskID(session), At: clock.now},
		{Kind: ASREventPartial, Partial: "你好", Stable: false, TaskID: sessionTaskID(session), At: clock.now},
		{Kind: ASREventFinal, Final: "你好呀", Stable: true, TaskID: sessionTaskID(session), At: clock.now},
		{Kind: ASREventTaskFinished, TaskID: sessionTaskID(session), At: clock.now},
	}
	for i, expected := range want {
		got := readASREvent(t, session.Events())
		if got != expected {
			t.Fatalf("event[%d]=%+v want=%+v", i, got, expected)
		}
	}
	waitASREventsClosed(t, session.Events())
}

func TestASRRejectsEmptyPCMAndWritesAfterFinishWhileFinishIsIdempotent(t *testing.T) {
	finishCount := make(chan int, 1)
	server := newFakeASRServer(t, func(peer *fakeASRPeer) {
		conn := peer.conn
		run := receiveRunTask(peer)
		writeASREvent(peer, run.Header.TaskID, "task-started", map[string]any{})
		count := 0
		for {
			_ = conn.SetReadDeadline(time.Now().Add(150 * time.Millisecond))
			_, raw, err := conn.ReadMessage()
			if err != nil {
				break
			}
			var message asrWireMessage
			if json.Unmarshal(raw, &message) == nil && message.Header.Action == "finish-task" {
				count++
			}
		}
		finishCount <- count
	})
	session, err := testASRFactory(nil, AliyunASRTimeouts{Close: 80 * time.Millisecond}).Open(context.Background(), testASRConfig(server.url))
	if err != nil {
		t.Fatal(err)
	}
	if err := session.WritePCM(context.Background(), nil); !errors.Is(err, ErrASREmptyPCM) {
		t.Fatalf("empty PCM err=%v", err)
	}
	if err := session.FinishInput(context.Background()); !errors.Is(err, ErrASRTimeout) {
		t.Fatalf("first FinishInput err=%v", err)
	}
	if err := session.FinishInput(context.Background()); !errors.Is(err, ErrASRTimeout) {
		t.Fatalf("second FinishInput should share timeout: %v", err)
	}
	if err := session.WritePCM(context.Background(), []byte{1}); !errors.Is(err, ErrASRInputFinished) && !errors.Is(err, ErrASRClosed) {
		t.Fatalf("write after finish err=%v", err)
	}
	waitASREventsClosed(t, session.Events())
	if !errors.Is(session.Err(), ErrASRTimeout) {
		t.Fatalf("Err=%v want close timeout", session.Err())
	}
	if got := <-finishCount; got != 1 {
		t.Fatalf("finish-task count=%d", got)
	}
}

func TestASRConcurrentFinishInputSharesTheFirstSendResult(t *testing.T) {
	finishReceived := make(chan struct{})
	server := newFakeASRServer(t, func(peer *fakeASRPeer) {
		conn := peer.conn
		run := receiveRunTask(peer)
		writeASREvent(peer, run.Header.TaskID, "task-started", map[string]any{})
		_, _, _ = conn.ReadMessage()
		close(finishReceived)
		_, _, _ = conn.ReadMessage()
	})
	factory := testASRFactory(nil, AliyunASRTimeouts{Close: 80 * time.Millisecond})
	var tracked *trackingReadDeadlineConn
	factory.wrapConn = func(conn *websocket.Conn) asrWebSocketConn {
		tracked = &trackingReadDeadlineConn{Conn: conn}
		return tracked
	}
	session, err := factory.Open(context.Background(), testASRConfig(server.url))
	if err != nil {
		t.Fatal(err)
	}
	readDeadlineCallsAfterOpen := tracked.calls()
	firstResult := make(chan error, 1)
	go func() { firstResult <- session.FinishInput(context.Background()) }()
	<-finishReceived
	secondCtx, cancelSecond := context.WithCancel(context.Background())
	cancelSecond()
	secondResult := make(chan error, 1)
	go func() { secondResult <- session.FinishInput(secondCtx) }()
	if err := <-firstResult; !errors.Is(err, ErrASRTimeout) {
		t.Fatalf("first err=%v", err)
	}
	if err := <-secondResult; !errors.Is(err, ErrASRTimeout) {
		t.Fatalf("second err=%v", err)
	}
	if got := tracked.calls(); got != readDeadlineCallsAfterOpen {
		t.Fatalf("FinishInput changed read deadline from business goroutine: before=%d after=%d", readDeadlineCallsAfterOpen, got)
	}
	waitASREventsClosed(t, session.Events())
	if !errors.Is(session.Err(), ErrASRTimeout) {
		t.Fatalf("session Err=%v", session.Err())
	}
}

func TestASRTaskFailedClosesEventsWithStableError(t *testing.T) {
	server := newFakeASRServer(t, func(peer *fakeASRPeer) {
		conn := peer.conn
		run := receiveRunTask(peer)
		if err := conn.WriteJSON(map[string]any{
			"header": map[string]any{
				"task_id": run.Header.TaskID, "event": "task-started",
			},
		}); err != nil {
			peer.reportf("write task-started: %v", err)
			return
		}
		if err := conn.WriteJSON(map[string]any{
			"header": map[string]any{
				"task_id": run.Header.TaskID, "event": "task-failed",
				"error_code": "InvalidParameter", "error_message": "bad config",
			},
		}); err != nil {
			peer.reportf("write task-failed: %v", err)
		}
	})
	session, err := testASRFactory(nil, AliyunASRTimeouts{}).Open(context.Background(), testASRConfig(server.url))
	if err != nil {
		t.Fatal(err)
	}
	waitASREventsClosed(t, session.Events())
	var upstream *ASRUpstreamError
	if !errors.As(session.Err(), &upstream) || upstream.Code != "InvalidParameter" {
		t.Fatalf("Err=%#v", session.Err())
	}
	originalErr := session.Err()
	if err := session.WritePCM(context.Background(), []byte{1, 2}); !errors.Is(err, ErrASRClosed) {
		t.Fatalf("WritePCM after terminal failure err=%v", err)
	}
	if err := session.FinishInput(context.Background()); !errors.Is(err, ErrASRClosed) {
		t.Fatalf("FinishInput after terminal failure err=%v", err)
	}
	if session.Err() != originalErr {
		t.Fatalf("terminal error changed from %v to %v", originalErr, session.Err())
	}
}

func TestASRNormalTaskFinishedAtomicallyClosesSession(t *testing.T) {
	server := newFakeASRServer(t, func(peer *fakeASRPeer) {
		conn := peer.conn
		run := receiveRunTask(peer)
		writeASREvent(peer, run.Header.TaskID, "task-started", map[string]any{})
		_, _, _ = conn.ReadMessage()
		writeASREvent(peer, run.Header.TaskID, "task-finished", map[string]any{})
	})
	session, err := testASRFactory(nil, AliyunASRTimeouts{}).Open(context.Background(), testASRConfig(server.url))
	if err != nil {
		t.Fatal(err)
	}
	if err := session.FinishInput(context.Background()); err != nil {
		t.Fatal(err)
	}
	drainASREventsUntilClosed(t, session.Events())
	concrete := session.(*aliyunASRSession)
	concrete.stateMu.RLock()
	closed := concrete.closed
	concrete.stateMu.RUnlock()
	if !closed {
		t.Fatal("normal task-finished did not atomically mark session closed")
	}
	if err := session.WritePCM(context.Background(), []byte{1}); !errors.Is(err, ErrASRClosed) && !errors.Is(err, ErrASRInputFinished) {
		t.Fatalf("WritePCM after normal finish err=%v", err)
	}
	if err := session.FinishInput(context.Background()); err != nil {
		t.Fatalf("repeated FinishInput err=%v", err)
	}
}

func TestASRRejectsOversizedUpstreamMessagesAndTranscripts(t *testing.T) {
	tests := []struct {
		name string
		text string
	}{
		{name: "websocket message", text: strings.Repeat("x", (1<<20)+1)},
		{name: "sentence text", text: strings.Repeat("中", (64<<10)+1)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newFakeASRServer(t, func(peer *fakeASRPeer) {
				conn := peer.conn
				run := receiveRunTask(peer)
				writeASREvent(peer, run.Header.TaskID, "task-started", map[string]any{})
				_ = conn.WriteJSON(map[string]any{
					"header": map[string]any{"task_id": run.Header.TaskID, "event": "result-generated"},
					"payload": map[string]any{
						"output": map[string]any{"sentence": map[string]any{
							"text": tt.text, "sentence_end": false, "begin_time": 1,
						}},
					},
				})
			})
			session, err := testASRFactory(nil, AliyunASRTimeouts{}).Open(context.Background(), testASRConfig(server.url))
			if err != nil {
				t.Fatal(err)
			}
			drainASREventsUntilClosed(t, session.Events())
			if !errors.Is(session.Err(), ErrASRMessageTooLarge) || !errors.Is(session.Err(), ErrASRProtocol) {
				t.Fatalf("Err=%v", session.Err())
			}
		})
	}
}

func TestASRBackpressureCannotBlockReaderOrFinishForever(t *testing.T) {
	server := newFakeASRServer(t, func(peer *fakeASRPeer) {
		conn := peer.conn
		run := receiveRunTask(peer)
		writeASREvent(peer, run.Header.TaskID, "task-started", map[string]any{})
		_, _, _ = conn.ReadMessage()
		for i := 0; i < asrEventBuffer+20; i++ {
			err := conn.WriteJSON(map[string]any{
				"header": map[string]any{"task_id": run.Header.TaskID, "event": "result-generated"},
				"payload": map[string]any{"output": map[string]any{"sentence": map[string]any{
					"text": "partial", "sentence_end": false, "begin_time": i,
				}}},
			})
			if err != nil {
				return
			}
		}
		_ = conn.WriteJSON(map[string]any{
			"header":  map[string]any{"task_id": run.Header.TaskID, "event": "task-finished"},
			"payload": map[string]any{},
		})
	})
	session, err := testASRFactory(nil, AliyunASRTimeouts{
		Close: 500 * time.Millisecond, EventDelivery: 40 * time.Millisecond,
	}).Open(context.Background(), testASRConfig(server.url))
	if err != nil {
		t.Fatal(err)
	}
	if err := session.FinishInput(context.Background()); !errors.Is(err, ErrASRBackpressure) {
		t.Fatalf("FinishInput err=%v", err)
	}
	drainASREventsUntilClosed(t, session.Events())
	if !errors.Is(session.Err(), ErrASRBackpressure) {
		t.Fatalf("session Err=%v", session.Err())
	}
}

func TestASROpenFailsOnTaskFailedMalformedFirstEventAndTimeout(t *testing.T) {
	tests := []struct {
		name    string
		handler func(*fakeASRPeer, string)
		want    error
	}{
		{
			name: "task failed",
			handler: func(peer *fakeASRPeer, taskID string) {
				if err := peer.conn.WriteJSON(map[string]any{"header": map[string]any{
					"task_id": taskID, "event": "task-failed", "error_code": "Unauthorized", "error_message": "denied",
				}}); err != nil {
					peer.reportf("write task-failed: %v", err)
				}
			},
			want: ErrASRUpstream,
		},
		{
			name: "malformed first event",
			handler: func(peer *fakeASRPeer, _ string) {
				if err := peer.conn.WriteMessage(websocket.TextMessage, []byte(`{"header":`)); err != nil {
					peer.reportf("write malformed event: %v", err)
				}
			},
			want: ErrASRProtocol,
		},
		{
			name: "first event timeout",
			handler: func(peer *fakeASRPeer, _ string) {
				_, _, _ = peer.conn.ReadMessage()
			},
			want: ErrASRTimeout,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newFakeASRServer(t, func(peer *fakeASRPeer) {
				run := receiveRunTask(peer)
				tt.handler(peer, run.Header.TaskID)
			})
			_, err := testASRFactory(nil, AliyunASRTimeouts{FirstEvent: 50 * time.Millisecond}).Open(context.Background(), testASRConfig(server.url))
			if !errors.Is(err, tt.want) {
				t.Fatalf("err=%v want errors.Is %v", err, tt.want)
			}
		})
	}
}

func TestASROpenCancellationInterruptsWaitingForTaskStarted(t *testing.T) {
	runTaskReceived := make(chan struct{})
	server := newFakeASRServer(t, func(peer *fakeASRPeer) {
		conn := peer.conn
		_ = receiveRunTask(peer)
		close(runTaskReceived)
		_, _, _ = conn.ReadMessage()
	})
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := testASRFactory(nil, AliyunASRTimeouts{FirstEvent: time.Second}).Open(ctx, testASRConfig(server.url))
		result <- err
	}()
	<-runTaskReceived
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err=%v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Open did not stop after context cancellation")
	}
}

func TestASRReportsUnexpectedDisconnectAndContextCancellation(t *testing.T) {
	t.Run("disconnect", func(t *testing.T) {
		server := newFakeASRServer(t, func(peer *fakeASRPeer) {
			run := receiveRunTask(peer)
			writeASREvent(peer, run.Header.TaskID, "task-started", map[string]any{})
		})
		session, err := testASRFactory(nil, AliyunASRTimeouts{}).Open(context.Background(), testASRConfig(server.url))
		if err != nil {
			t.Fatal(err)
		}
		waitASREventsClosed(t, session.Events())
		if session.Err() == nil {
			t.Fatal("unexpected disconnect must be reported")
		}
	})

	t.Run("context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		server := newFakeASRServer(t, func(peer *fakeASRPeer) {
			conn := peer.conn
			run := receiveRunTask(peer)
			writeASREvent(peer, run.Header.TaskID, "task-started", map[string]any{})
			_, _, _ = conn.ReadMessage()
		})
		session, err := testASRFactory(nil, AliyunASRTimeouts{}).Open(ctx, testASRConfig(server.url))
		if err != nil {
			t.Fatal(err)
		}
		cancel()
		waitASREventsClosed(t, session.Events())
		if !errors.Is(session.Err(), context.Canceled) {
			t.Fatalf("Err=%v", session.Err())
		}
	})
}

func TestASRCloseIsPromptConcurrentSafeAndLeavesNoReader(t *testing.T) {
	serverReady := make(chan struct{})
	server := newFakeASRServer(t, func(peer *fakeASRPeer) {
		conn := peer.conn
		run := receiveRunTask(peer)
		writeASREvent(peer, run.Header.TaskID, "task-started", map[string]any{})
		close(serverReady)
		_, _, _ = conn.ReadMessage()
	})
	baseline := runtime.NumGoroutine()
	session, err := testASRFactory(nil, AliyunASRTimeouts{Write: 100 * time.Millisecond, Close: 100 * time.Millisecond}).Open(context.Background(), testASRConfig(server.url))
	if err != nil {
		t.Fatal(err)
	}
	<-serverReady
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = session.Close()
			_ = session.Err()
		}()
	}
	wg.Wait()
	waitASREventsClosed(t, session.Events())
	deadline := time.Now().Add(time.Second)
	for runtime.NumGoroutine() > baseline+3 && time.Now().Before(deadline) {
		runtime.Gosched()
		time.Sleep(5 * time.Millisecond)
	}
	if runtime.NumGoroutine() > baseline+3 {
		t.Fatalf("reader goroutine appears leaked: before=%d after=%d", baseline, runtime.NumGoroutine())
	}
}

func TestASRHandshakeAndWriteTimeoutsAreBounded(t *testing.T) {
	t.Run("handshake", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			time.Sleep(180 * time.Millisecond)
		}))
		defer server.Close()
		_, err := testASRFactory(nil, AliyunASRTimeouts{Handshake: 40 * time.Millisecond}).Open(context.Background(), testASRConfig("ws"+strings.TrimPrefix(server.URL, "http")))
		if !errors.Is(err, ErrASRTimeout) {
			t.Fatalf("handshake err=%v", err)
		}
	})

	t.Run("write", func(t *testing.T) {
		server := newFakeASRServer(t, func(peer *fakeASRPeer) {
			run := receiveRunTask(peer)
			writeASREvent(peer, run.Header.TaskID, "task-started", map[string]any{})
			time.Sleep(250 * time.Millisecond)
		})
		session, err := testASRFactory(nil, AliyunASRTimeouts{Write: 25 * time.Millisecond}).Open(context.Background(), testASRConfig(server.url))
		if err != nil {
			t.Fatal(err)
		}
		defer session.Close()
		largePCM := make([]byte, 32<<20)
		result := make(chan error, 1)
		go func() { result <- session.WritePCM(context.Background(), largePCM) }()
		select {
		case err = <-result:
			if err == nil {
				t.Fatal("expected bounded write error")
			}
		case <-time.After(time.Second):
			t.Fatal("WritePCM did not respect configured timeout")
		}
	})
}

func TestASROpenRejectsInvalidRuntimeConfigurationWithoutDialing(t *testing.T) {
	factory := testASRFactory(nil, AliyunASRTimeouts{})
	tests := []struct {
		name   string
		mutate func(*RealtimeASRConfig)
	}{
		{"empty api key", func(cfg *RealtimeASRConfig) { cfg.APIKey = "" }},
		{"wrong provider", func(cfg *RealtimeASRConfig) { cfg.Provider = "other" }},
		{"wrong model", func(cfg *RealtimeASRConfig) { cfg.Model = "other" }},
		{"bad endpoint", func(cfg *RealtimeASRConfig) { cfg.Endpoint = "http://127.0.0.1" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := testASRConfig("ws://127.0.0.1:1")
			tt.mutate(&cfg)
			if _, err := factory.Open(context.Background(), cfg); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func isStandardUUID(value string) bool {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' {
		return false
	}
	for i, r := range value {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			continue
		}
		if !strings.ContainsRune("0123456789abcdef", r) {
			return false
		}
	}
	return value[14] == '4' && strings.ContainsRune("89ab", rune(value[19]))
}

func sessionTaskID(session ASRSession) string {
	return session.(*aliyunASRSession).taskID
}
