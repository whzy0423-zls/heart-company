package server

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"sync"

	"nine-xing/nx-backend/apps/server/internal/voice"
)

const voiceBroadcastQueueSize = 8

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
		case <-s.runCtx.Done():
			return s.runCtx.Err()
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
	defer s.jobsMu.Unlock()
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return s.Wait()
	}
	chunks := s.chunker.Flush()
	s.closed = true
	s.mu.Unlock()
	if err := s.enqueue(chunks); err != nil && !errors.Is(err, context.Canceled) {
		s.recordError(err)
	}
	s.closeOnce.Do(func() { close(s.jobs) })
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
