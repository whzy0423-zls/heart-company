package knowledgeclient

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

func (c *Client) StreamAnswer(ctx context.Context, request AnswerRequest) (<-chan StreamEvent, <-chan error) {
	events := make(chan StreamEvent)
	errs := make(chan error, 1)
	go func() {
		defer close(events)
		defer close(errs)
		req, err := c.newJSONRequest(ctx, http.MethodPost, "/internal/v1/answer/stream", request)
		if err != nil {
			errs <- err
			return
		}
		req.Header.Set("Accept", "text/event-stream")
		response, err := c.httpClient.Do(req)
		if err != nil {
			errs <- err
			return
		}
		defer response.Body.Close()
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			errs <- &ResponseError{StatusCode: response.StatusCode, Body: response.Status}
			return
		}

		scanner := bufio.NewScanner(response.Body)
		scanner.Buffer(make([]byte, 64*1024), int(c.maxResponseBytes))
		var eventType string
		var data strings.Builder
		done := false
		dispatch := func() bool {
			if eventType == "" {
				data.Reset()
				return true
			}
			raw := json.RawMessage(strings.TrimSpace(data.String()))
			if eventType == "error" {
				var streamErr StreamError
				if err := json.Unmarshal(raw, &streamErr); err != nil {
					errs <- fmt.Errorf("decode stream error: %w", err)
				} else {
					errs <- &streamErr
				}
				return false
			}
			select {
			case events <- StreamEvent{Type: eventType, Data: raw}:
			case <-ctx.Done():
				errs <- ctx.Err()
				return false
			}
			if eventType == "done" {
				done = true
			}
			eventType = ""
			data.Reset()
			return true
		}

		for scanner.Scan() {
			line := scanner.Text()
			switch {
			case line == "":
				if !dispatch() {
					return
				}
			case strings.HasPrefix(line, "event:"):
				eventType = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			case strings.HasPrefix(line, "data:"):
				if data.Len() > 0 {
					data.WriteByte('\n')
				}
				data.WriteString(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
			}
		}
		if err := scanner.Err(); err != nil {
			errs <- err
			return
		}
		if eventType != "" && !dispatch() {
			return
		}
		if !done {
			errs <- ErrStreamInterrupted
		}
	}()
	return events, errs
}
