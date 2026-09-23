package server

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"time"

	"nine-xing/nx-backend/apps/server/internal/voice"
	"nine-xing/nx-backend/apps/server/internal/xinzhili"
)

const voiceBroadcastQueueSize = 8

// defaultVoiceBroadcastDrainTimeout covers the usual one-to-three sentence
// answer while remaining bounded by the Bailian HTTP client's request budget.
// Text deltas and persistence complete before this terminal drain starts.
const defaultVoiceBroadcastDrainTimeout = 30 * time.Second

func (s *Server) voiceBroadcastTerminalDrainTimeout() time.Duration {
	if s != nil && s.voiceBroadcastDrainTimeout > 0 {
		return s.voiceBroadcastDrainTimeout
	}
	return defaultVoiceBroadcastDrainTimeout
}

func voiceBroadcastTerminalErrorCode(err, requestErr error) string {
	if err == nil {
		return ""
	}
	if requestErr != nil && (errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)) {
		return ""
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, xinzhili.ErrTTSTimeout) {
		return "synthesis_timeout"
	}
	return "synthesis_failed"
}

// voiceBroadcastSegment is an in-process segment. Audio is converted to a
// base64 string only at the HTTP/WebSocket boundary, never persisted.
type voiceBroadcastSegment struct {
	ReplyID    string
	SegmentSeq uint32
	MIME       string
	Audio      []byte
	Text       string
	Final      bool
}

type voiceBroadcastSegmentPayload struct {
	ReplyID     string `json:"replyId"`
	SegmentSeq  uint32 `json:"segmentSeq"`
	MIME        string `json:"mime"`
	ByteLength  int    `json:"byteLength"`
	AudioBase64 string `json:"audioBase64"`
	Final       bool   `json:"final"`
}

func (s voiceBroadcastSegment) ssePayload() voiceBroadcastSegmentPayload {
	return voiceBroadcastSegmentPayload{
		ReplyID: s.ReplyID, SegmentSeq: s.SegmentSeq, MIME: s.MIME,
		ByteLength: len(s.Audio), AudioBase64: base64.StdEncoding.EncodeToString(s.Audio), Final: s.Final,
	}
}

type voiceBroadcastErrorPayload struct {
	ReplyID string `json:"replyId"`
	Code    string `json:"code"`
}

type voiceBroadcastStream struct {
	ctx       context.Context
	runCtx    context.Context
	cancel    context.CancelFunc
	replyID   string
	provider  voiceBroadcastSynthesizer
	emit      func(voiceBroadcastSegment) error
	chunker   *voice.SentenceChunker
	jobs      chan string
	done      chan struct{}
	closeOnce sync.Once
	jobsMu    sync.Mutex
	mu        sync.Mutex
	closed    bool
	firstErr  error
}

// voiceBroadcastCloseHandle owns the one asynchronous Close call for a
// stream. Callers can wait briefly for the normal final segment, then abort
// the optional channel without blocking the text response.
type voiceBroadcastCloseHandle struct {
	stream *voiceBroadcastStream
	done   chan error
}

func startVoiceBroadcastClose(stream *voiceBroadcastStream) *voiceBroadcastCloseHandle {
	if stream == nil {
		return nil
	}
	handle := &voiceBroadcastCloseHandle{stream: stream, done: make(chan error, 1)}
	go func() { handle.done <- stream.Close() }()
	return handle
}

func (h *voiceBroadcastCloseHandle) await(timeout time.Duration) (error, bool) {
	if h == nil {
		return nil, true
	}
	if timeout <= 0 {
		select {
		case err := <-h.done:
			return err, true
		default:
			return context.DeadlineExceeded, false
		}
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case err := <-h.done:
		return err, true
	case <-timer.C:
		// Close may still be waiting on a provider that does not honor its
		// context immediately. Cancel and detach the emitter so the caller can
		// close its event channel without a late send or a blocked wait.
		h.abort()
		return context.DeadlineExceeded, false
	}
}

func (h *voiceBroadcastCloseHandle) abort() {
	if h == nil || h.stream == nil {
		return
	}
	h.stream.Cancel()
	h.stream.detachEmit()
}

func newVoiceBroadcastStream(ctx context.Context, replyID string, provider voiceBroadcastSynthesizer, emit func(voiceBroadcastSegment) error) *voiceBroadcastStream {
	if ctx == nil {
		ctx = context.Background()
	}
	streamCtx, cancel := context.WithCancel(ctx)
	stream := &voiceBroadcastStream{
		ctx: ctx, runCtx: streamCtx, cancel: cancel, replyID: strings.TrimSpace(replyID), provider: provider,
		emit: emit, chunker: voice.NewSentenceChunker(96), jobs: make(chan string, voiceBroadcastQueueSize), done: make(chan struct{}),
	}
	if stream.replyID == "" {
		stream.replyID = newVoiceBroadcastReplyID()
	}
	go stream.run(streamCtx)
	return stream
}

func newVoiceBroadcastReplyID() string {
	var bytes [12]byte
	if _, err := rand.Read(bytes[:]); err == nil {
		return hex.EncodeToString(bytes[:])
	}
	return "voice-reply"
}

func (s *voiceBroadcastStream) Push(delta string) error {
	if s == nil || strings.TrimSpace(delta) == "" {
		return nil
	}
	s.jobsMu.Lock()
	defer s.jobsMu.Unlock()
	s.mu.Lock()
	if s.closed {
		err := s.firstErr
		s.mu.Unlock()
		if err != nil {
			return err
		}
		return errors.New("voice broadcast stream is closed")
	}
	chunks := s.chunker.Push(delta)
	s.mu.Unlock()
	return s.enqueue(chunks)
}

func (s *voiceBroadcastStream) enqueue(chunks []string) error {
	for _, chunk := range chunks {
		select {
		case s.jobs <- chunk:
			// Keep voice generation asynchronous from the text producer. The
			// provider may be slower than the model, so a bounded queue must not
			// turn Push into backpressure on the text stream.
		case <-s.runCtx.Done():
			return s.runCtx.Err()
		default:
			// Prefer the newest sentence when the optional voice queue is full.
			// Dropping an older queued sentence is preferable to blocking text
			// delivery; the terminal Close call can still enqueue its final text.
			select {
			case <-s.jobs:
			default:
			}
			select {
			case s.jobs <- chunk:
			case <-s.runCtx.Done():
				return s.runCtx.Err()
			default:
				// The worker may have won the race between the two non-blocking
				// operations. Dropping this chunk still preserves the no-blocking
				// contract.
			}
		}
	}
	return nil
}

// Close flushes the final unterminated sentence and marks the last emitted
// segment with final=true. It is safe to call more than once.
func (s *voiceBroadcastStream) Close() error {
	if s == nil {
		return nil
	}
	s.jobsMu.Lock()
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		s.jobsMu.Unlock()
		return s.Wait()
	}
	chunks := s.chunker.Flush()
	s.closed = true
	s.mu.Unlock()
	if err := s.enqueue(chunks); err != nil && !errors.Is(err, context.Canceled) {
		s.recordError(err)
	}
	s.closeOnce.Do(func() { close(s.jobs) })
	s.jobsMu.Unlock()
	return s.Wait()
}

func (s *voiceBroadcastStream) Cancel() {
	if s == nil {
		return
	}
	s.cancel()
}

func (s *voiceBroadcastStream) Wait() error {
	if s == nil {
		return nil
	}
	<-s.done
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.firstErr != nil {
		return s.firstErr
	}
	if err := s.runCtx.Err(); err != nil {
		return err
	}
	return nil
}

func (s *voiceBroadcastStream) recordError(err error) {
	if err == nil {
		return
	}
	s.mu.Lock()
	if s.firstErr == nil {
		s.firstErr = err
	}
	s.mu.Unlock()
}

// detachEmit prevents a late provider completion from writing to a pipeline
// event channel whose owner has already emitted its terminal text event. The
// emitter call itself is serialized with this mutation below.
func (s *voiceBroadcastStream) detachEmit() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.emit = nil
	s.mu.Unlock()
}

func (s *voiceBroadcastStream) run(ctx context.Context) {
	defer close(s.done)
	var pending *voiceBroadcastSegment
	var sequence uint32
	for {
		select {
		case <-ctx.Done():
			return
		case text, ok := <-s.jobs:
			if !ok {
				if pending != nil && ctx.Err() == nil {
					pending.Final = true
					if err := s.emitSegment(*pending); err != nil {
						s.recordError(err)
					}
				}
				return
			}
			if s.provider == nil {
				s.recordError(errors.New("voice broadcast provider unavailable"))
				s.cancel()
				return
			}
			audio, mimeType, err := s.provider.Synthesize(ctx, text)
			if err != nil {
				if ctx.Err() == nil {
					s.recordError(err)
					s.cancel()
				}
				return
			}
			if len(audio) == 0 {
				s.recordError(errors.New("voice broadcast provider returned empty audio"))
				s.cancel()
				return
			}
			segment := voiceBroadcastSegment{ReplyID: s.replyID, SegmentSeq: sequence, MIME: mimeType, Audio: append([]byte(nil), audio...), Text: text}
			sequence++
			if pending != nil {
				if err := s.emitSegment(*pending); err != nil {
					s.recordError(err)
					s.cancel()
					return
				}
			}
			pending = &segment
		}
	}
}

func (s *voiceBroadcastStream) emitSegment(segment voiceBroadcastSegment) error {
	if s == nil {
		return nil
	}
	// Hold the same mutex used by detachEmit while invoking the callback. A
	// timeout can therefore either detach before this call starts or wait for
	// the in-flight callback to observe cancellation; it can never race a
	// closed events channel.
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.emit == nil {
		return nil
	}
	select {
	case <-s.runCtx.Done():
		return s.runCtx.Err()
	default:
	}
	return s.emit(segment)
}
