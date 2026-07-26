package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestEnsureVideoSubmissionRecoveryRetriesAfterFailure(t *testing.T) {
	var calls atomic.Int32
	s := &Server{
		videoSubmissionRecovery: func(context.Context) (int64, error) {
			if calls.Add(1) == 1 {
				return 0, errors.New("database unavailable")
			}
			return 1, nil
		},
	}
	if err := s.ensureVideoSubmissionRecovery(context.Background()); err == nil {
		t.Fatal("first recovery attempt must fail")
	}
	if s.videoSubmissionRecoveryReady.Load() {
		t.Fatal("failed recovery must not open the submission gate")
	}
	if err := s.ensureVideoSubmissionRecovery(context.Background()); err != nil {
		t.Fatalf("second recovery attempt: %v", err)
	}
	if !s.videoSubmissionRecoveryReady.Load() {
		t.Fatal("successful recovery did not open the submission gate")
	}
	if err := s.ensureVideoSubmissionRecovery(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Fatalf("recovery calls = %d, want 2", calls.Load())
	}
}

func TestEnsureVideoSubmissionRecoverySerializesConcurrentAttempts(t *testing.T) {
	var calls atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})
	s := &Server{
		videoSubmissionRecovery: func(context.Context) (int64, error) {
			if calls.Add(1) == 1 {
				close(started)
			}
			<-release
			return 1, nil
		},
	}

	const workers = 12
	errorsCh := make(chan error, workers)
	var group sync.WaitGroup
	for range workers {
		group.Add(1)
		go func() {
			defer group.Done()
			errorsCh <- s.ensureVideoSubmissionRecovery(context.Background())
		}()
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("recovery did not start")
	}
	close(release)
	group.Wait()
	close(errorsCh)
	for err := range errorsCh {
		if err != nil {
			t.Fatalf("concurrent recovery: %v", err)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("concurrent recovery calls = %d, want 1", calls.Load())
	}
}

func TestVideoWorkflowReturnsServiceUnavailableWhileRecoveryFails(t *testing.T) {
	s := &Server{
		videoSubmissionRecovery: func(context.Context) (int64, error) {
			return 0, errors.New("database unavailable")
		},
	}
	requestKey := "11111111-1111-4111-8111-111111111111"
	jsonBody := func(value any) *bytes.Reader {
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		return bytes.NewReader(raw)
	}
	tests := []struct {
		name    string
		handler http.HandlerFunc
		request *http.Request
	}{
		{name: "workflow", handler: s.videoWorkflowGet, request: httptest.NewRequest(http.MethodGet, "/api/video/projects-workflow/1", nil)},
		{name: "submission status", handler: s.videoWorkflowSubmissionStatus, request: httptest.NewRequest(http.MethodGet, "/api/video/generation-submissions/1", nil)},
		{name: "safe generate", handler: s.videoWorkflowGenerate, request: httptest.NewRequest(http.MethodPost, "/api/video/shots-generate-safe/1", jsonBody(map[string]string{"requestKey": requestKey}))},
		{name: "legacy generate", handler: s.generateVideoShot, request: httptest.NewRequest(http.MethodPost, "/api/video/shots-generate/1", jsonBody(map[string]string{"requestKey": requestKey}))},
		{name: "safe batch", handler: s.videoWorkflowBatchGenerate, request: httptest.NewRequest(http.MethodPost, "/api/video/projects-batch-generate-safe/1", jsonBody(map[string]any{"items": []map[string]string{{"requestKey": requestKey, "shotId": "1"}}}))},
		{name: "legacy batch", handler: s.batchGenerateShots, request: httptest.NewRequest(http.MethodPost, "/api/video/projects-batch-generate/1", jsonBody(map[string]any{"items": []map[string]string{{"requestKey": requestKey, "shotId": "1"}}}))},
		{name: "reconcile", handler: s.videoWorkflowReconcile, request: httptest.NewRequest(http.MethodPost, "/api/video/generation-submissions/reconcile/"+requestKey, jsonBody(map[string]string{"taskId": "task-1"}))},
		{name: "generic project generate", handler: s.generateVideo, request: httptest.NewRequest(http.MethodPost, "/api/video/generate", jsonBody(map[string]string{"prompt": "test", "requestKey": requestKey, "shotId": "1"}))},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			testCase.handler(recorder, testCase.request)
			if recorder.Code != http.StatusServiceUnavailable {
				t.Fatalf("status = %d, want 503: %s", recorder.Code, recorder.Body.String())
			}
			if !strings.Contains(recorder.Body.String(), "视频任务恢复中，请稍后重试") {
				t.Fatalf("missing recovery response: %s", recorder.Body.String())
			}
		})
	}
}
