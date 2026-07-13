package llm

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
)

const maxSSEEventBytes = 1 << 20

// ErrSSEEventTooLarge is returned before an event larger than one MiB can be
// delivered to a provider-specific consumer.
var ErrSSEEventTooLarge = errors.New("SSE event exceeds size limit")

type sseEvent struct {
	Event string
	Data  string
}

type sseEmitter func(event sseEvent) error

// readSSE incrementally parses provider-native SSE framing. It deliberately
// leaves event names and data uninterpreted so each adapter can handle its own
// JSON protocol.
func readSSE(ctx context.Context, stream io.Reader, emit sseEmitter) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	stopCancellationWatch := make(chan struct{})
	defer close(stopCancellationWatch)
	if closer, ok := stream.(io.Closer); ok {
		go func() {
			select {
			case <-ctx.Done():
				_ = closer.Close()
			case <-stopCancellationWatch:
			}
		}()
	}

	reader := bufio.NewReaderSize(stream, 32*1024)
	eventName := ""
	eventBytes := 0
	hasData := false
	var eventData strings.Builder

	dispatch := func() error {
		defer func() {
			eventName = ""
			eventBytes = 0
			hasData = false
			eventData.Reset()
		}()
		if !hasData {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if emit == nil {
			return nil
		}
		return emit(sseEvent{Event: eventName, Data: eventData.String()})
	}

	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		rawLine, readErr := readBoundedSSELine(reader)
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			if err := ctx.Err(); err != nil {
				return err
			}
			return readErr
		}

		if len(rawLine) > 0 {
			terminated := rawLine[len(rawLine)-1] == '\n'
			line := bytes.TrimSuffix(rawLine, []byte("\n"))
			line = bytes.TrimSuffix(line, []byte("\r"))
			if len(line) == 0 {
				if err := dispatch(); err != nil {
					return err
				}
			} else {
				logicalBytes := len(line)
				if terminated {
					logicalBytes++
				}
				eventBytes += logicalBytes
				if eventBytes > maxSSEEventBytes {
					return fmt.Errorf("%w: maximum %d bytes", ErrSSEEventTooLarge, maxSSEEventBytes)
				}

				if line[0] != ':' {
					field, value, found := bytes.Cut(line, []byte(":"))
					if !found {
						value = nil
					}
					value = bytes.TrimPrefix(value, []byte(" "))
					switch string(field) {
					case "event":
						eventName = string(value)
					case "data":
						if hasData {
							eventData.WriteByte('\n')
						}
						hasData = true
						eventData.Write(value)
					}
				}
			}
		}

		if errors.Is(readErr, io.EOF) {
			if err := dispatch(); err != nil {
				return err
			}
			return nil
		}
	}
}

func readBoundedSSELine(reader *bufio.Reader) ([]byte, error) {
	line := make([]byte, 0, 32*1024)
	for {
		fragment, err := reader.ReadSlice('\n')
		if len(line)+len(fragment) > maxSSEEventBytes+2 {
			return nil, fmt.Errorf("%w: maximum %d bytes", ErrSSEEventTooLarge, maxSSEEventBytes)
		}
		line = append(line, fragment...)
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		return line, err
	}
}
