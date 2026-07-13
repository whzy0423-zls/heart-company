package llm

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestSSEReaderParsesFraming(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		reader io.Reader
		want   []sseEvent
	}{
		{
			name:   "LF frames and multiple events per read",
			reader: strings.NewReader("event: first\ndata: one\n\nevent: second\ndata: two\n\n"),
			want: []sseEvent{
				{Event: "first", Data: "one"},
				{Event: "second", Data: "two"},
			},
		},
		{
			name:   "CRLF frames",
			reader: strings.NewReader("event: delta\r\ndata: hello\r\n\r\n"),
			want:   []sseEvent{{Event: "delta", Data: "hello"}},
		},
		{
			name: "delimiter split across reads",
			reader: &chunkReader{chunks: [][]byte{
				[]byte("event: delta\nda"),
				[]byte("ta: split\n"),
				[]byte("\nevent: done\n"),
				[]byte("data: ok\n\n"),
			}},
			want: []sseEvent{
				{Event: "delta", Data: "split"},
				{Event: "done", Data: "ok"},
			},
		},
		{
			name:   "comments are ignored",
			reader: strings.NewReader(": connected\n\n: ping\nevent: delta\ndata: visible\n\n"),
			want:   []sseEvent{{Event: "delta", Data: "visible"}},
		},
		{
			name:   "multi-line data preserves newlines",
			reader: strings.NewReader("event: content_block_delta\ndata: first\ndata: second\ndata:\n\n"),
			want:   []sseEvent{{Event: "content_block_delta", Data: "first\nsecond\n"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var got []sseEvent
			err := readSSE(context.Background(), tt.reader, func(event sseEvent) error {
				got = append(got, event)
				return nil
			})
			if err != nil {
				t.Fatalf("readSSE returned error: %v", err)
			}
			if fmt.Sprint(got) != fmt.Sprint(tt.want) {
				t.Fatalf("events mismatch\n got: %#v\nwant: %#v", got, tt.want)
			}
		})
	}
}

func TestSSEReaderAcceptsEventExactlyAtOneMiBBoundary(t *testing.T) {
	t.Parallel()

	const prefix = "data: "
	payload := strings.Repeat("x", maxSSEEventBytes-len(prefix)-1)
	stream := prefix + payload + "\n\n"

	var got sseEvent
	err := readSSE(context.Background(), strings.NewReader(stream), func(event sseEvent) error {
		got = event
		return nil
	})
	if err != nil {
		t.Fatalf("exactly 1 MiB event should be accepted: %v", err)
	}
	if got.Data != payload {
		t.Fatalf("payload length mismatch: got %d want %d", len(got.Data), len(payload))
	}
}

func TestSSEReaderRejectsEventOverOneMiBBoundary(t *testing.T) {
	t.Parallel()

	const prefix = "data: "
	payload := strings.Repeat("x", maxSSEEventBytes-len(prefix))
	stream := prefix + payload + "\n\n"

	err := readSSE(context.Background(), strings.NewReader(stream), func(sseEvent) error {
		t.Fatal("oversized event must not be emitted")
		return nil
	})
	if !errors.Is(err, ErrSSEEventTooLarge) {
		t.Fatalf("expected ErrSSEEventTooLarge, got %v", err)
	}
}

func TestSSEReaderReturnsConsumerErrorUnchanged(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("consumer stopped")
	err := readSSE(context.Background(), strings.NewReader("data: first\n\ndata: second\n\n"), func(sseEvent) error {
		return wantErr
	})
	if err != wantErr {
		t.Fatalf("consumer error should be returned unchanged: got %v want %v", err, wantErr)
	}
}

func TestSSEReaderReturnsContextCancellationUnchanged(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	reader, writer := io.Pipe()
	defer writer.Close()

	result := make(chan error, 1)
	go func() {
		result <- readSSE(ctx, reader, func(sseEvent) error { return nil })
	}()

	cancel()
	select {
	case err := <-result:
		if err != context.Canceled {
			t.Fatalf("context cancellation should be returned unchanged: got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("readSSE did not stop promptly after cancellation")
	}
}

func TestSSEReaderEmitsIncrementally(t *testing.T) {
	t.Parallel()

	reader, writer := io.Pipe()
	defer reader.Close()
	firstEmitted := make(chan struct{})
	releaseSecond := make(chan struct{})
	result := make(chan error, 1)

	go func() {
		defer writer.Close()
		_, _ = io.WriteString(writer, "event: delta\ndata: first\n\n")
		<-releaseSecond
		_, _ = io.WriteString(writer, "event: delta\ndata: second\n\n")
	}()

	var mu sync.Mutex
	var got []sseEvent
	go func() {
		result <- readSSE(context.Background(), reader, func(event sseEvent) error {
			mu.Lock()
			got = append(got, event)
			count := len(got)
			mu.Unlock()
			if count == 1 {
				close(firstEmitted)
			}
			return nil
		})
	}()

	select {
	case <-firstEmitted:
	case err := <-result:
		t.Fatalf("reader completed before the second event was released: %v", err)
	case <-time.After(time.Second):
		t.Fatal("first event was buffered instead of emitted incrementally")
	}

	close(releaseSecond)
	if err := <-result; err != nil {
		t.Fatalf("readSSE returned error: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if fmt.Sprint(got) != fmt.Sprint([]sseEvent{{Event: "delta", Data: "first"}, {Event: "delta", Data: "second"}}) {
		t.Fatalf("unexpected events: %#v", got)
	}
}

type chunkReader struct {
	chunks [][]byte
}

func (r *chunkReader) Read(p []byte) (int, error) {
	if len(r.chunks) == 0 {
		return 0, io.EOF
	}
	chunk := r.chunks[0]
	r.chunks = r.chunks[1:]
	n := copy(p, chunk)
	if n < len(chunk) {
		r.chunks = append([][]byte{chunk[n:]}, r.chunks...)
	}
	return n, nil
}
