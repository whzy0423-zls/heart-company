package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/auth"
	"nine-xing/nx-backend/apps/server/internal/problemfollowup"
)

type fakeProblemFollowupStore struct {
	jobs                        []*problemfollowup.Job
	beginCalls                  int
	beginErr                    error
	loadErr                     error
	saves, failures, deliveries int
	delivery                    *problemfollowup.Delivery
}

func (f *fakeProblemFollowupStore) BeginTurn(_ context.Context, userID, sessionID int64, _ time.Time) (problemfollowup.Turn, error) {
	f.beginCalls++
	if userID != 7 || sessionID != 42 {
		return problemfollowup.Turn{}, errors.New("wrong turn owner")
	}
	return problemfollowup.Turn{}, f.beginErr
}
func (f *fakeProblemFollowupStore) Claim(context.Context, time.Time) (*problemfollowup.Job, error) {
	if len(f.jobs) == 0 {
		return nil, nil
	}
	job := f.jobs[0]
	f.jobs = f.jobs[1:]
	return job, nil
}
func (f *fakeProblemFollowupStore) LoadContext(context.Context, problemfollowup.Job) (problemfollowup.Input, error) {
	return problemfollowup.Input{}, f.loadErr
}
func (f *fakeProblemFollowupStore) SaveDecision(context.Context, problemfollowup.Job, problemfollowup.Decision, time.Time) error {
	f.saves++
	return nil
}
func (f *fakeProblemFollowupStore) Fail(context.Context, problemfollowup.Job, time.Time) error {
	f.failures++
	return nil
}
func (f *fakeProblemFollowupStore) Deliver(context.Context, problemfollowup.Job, time.Time) (*problemfollowup.Delivery, error) {
	f.deliveries++
	return f.delivery, nil
}

func TestProblemFollowupReceivesNewTextBeforeAnswerAndRejectsActivityFailure(t *testing.T) {
	for _, path := range []string{"/api/app/chat/sessions/42/ask", "/api/app/chat/sessions/42/ask/stream"} {
		for _, fail := range []bool{false, true} {
			t.Run(path+"/failure="+map[bool]string{false: "false", true: "true"}[fail], func(t *testing.T) {
				chatStore := newFakeAppChatStreamStore()
				s := newAppChatStreamServer(chatStore, &hygieneAppChatGenerator{
					answer:       "先完成一个十分钟的小任务。",
					streamChunks: []string{"先完成一个十分钟的小任务。"},
				})
				jobs := &fakeProblemFollowupStore{}
				if fail {
					jobs.beginErr = errors.New("activity unavailable")
				}
				s.problemFollowups = jobs
				request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"question":"最近总是拖延，怎么办？"}`))
				request = request.WithContext(contextWithAppUser(request.Context(), auth.UserInfo{ID: 7}))
				response := httptest.NewRecorder()
				s.appChatRouter(response, request)
				if jobs.beginCalls != 1 {
					t.Fatalf("activity calls=%d, want 1", jobs.beginCalls)
				}
				want := http.StatusOK
				if fail {
					want = http.StatusServiceUnavailable
				}
				if response.Code != want {
					t.Fatalf("status=%d want=%d body=%s", response.Code, want, response.Body.String())
				}
			})
		}
	}
}

func TestProblemFollowupInvalidTextDoesNotCancelExistingTask(t *testing.T) {
	for _, path := range []string{"/api/app/chat/sessions/42/ask", "/api/app/chat/sessions/42/ask/stream"} {
		s := newAppChatStreamServer(newFakeAppChatStreamStore(), successfulAppChatGenerator("答"))
		jobs := &fakeProblemFollowupStore{}
		s.problemFollowups = jobs
		request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"question":""}`))
		request = request.WithContext(contextWithAppUser(request.Context(), auth.UserInfo{ID: 7}))
		response := httptest.NewRecorder()
		s.appChatRouter(response, request)
		if response.Code != http.StatusBadRequest || jobs.beginCalls != 0 {
			t.Fatalf("status=%d calls=%d", response.Code, jobs.beginCalls)
		}
	}
}

func TestProblemFollowupClassificationDoesNotSendBeforeDue(t *testing.T) {
	jobs := &fakeProblemFollowupStore{jobs: []*problemfollowup.Job{{ID: 1, Status: "classifying"}}}
	calls := 0
	s := &Server{problemFollowups: jobs, problemFollowupEvaluate: func(ctx context.Context, _ problemfollowup.Input) (problemfollowup.Decision, error) {
		calls++
		if _, ok := ctx.Deadline(); !ok {
			t.Error("classification must have bounded deadline")
		}
		return problemfollowup.Decision{ShouldFollowUp: true, ProblemSummary: "拖延", Message: "刚才提到的拖延问题，现在好一些了吗？"}, nil
	}}
	if err := s.runProblemFollowupOnce(context.Background(), time.Now()); err != nil {
		t.Fatal(err)
	}
	if calls != 1 || jobs.saves != 1 || jobs.deliveries != 0 || jobs.failures != 0 {
		t.Fatalf("calls=%d jobs=%+v", calls, jobs)
	}
}

func TestProblemFollowupStaleOrFailedClassifierNeverSends(t *testing.T) {
	for _, stale := range []bool{false, true} {
		t.Run(map[bool]string{false: "model_failure", true: "stale"}[stale], func(t *testing.T) {
			jobs := &fakeProblemFollowupStore{jobs: []*problemfollowup.Job{{ID: 1, Status: "classifying"}}}
			if stale {
				jobs.loadErr = problemfollowup.ErrStale
			}
			s := &Server{problemFollowups: jobs, problemFollowupEvaluate: func(context.Context, problemfollowup.Input) (problemfollowup.Decision, error) {
				if stale {
					t.Fatal("stale task invoked model")
				}
				return problemfollowup.Decision{}, errors.New("model unavailable")
			}}
			_ = s.runProblemFollowupOnce(context.Background(), time.Now())
			wantFailures := 1
			if stale {
				wantFailures = 0
			}
			if jobs.saves != 0 || jobs.deliveries != 0 || jobs.failures != wantFailures {
				t.Fatalf("jobs=%+v", jobs)
			}
		})
	}
}

func TestProblemFollowupPushFailureDoesNotRequeueCommittedMessage(t *testing.T) {
	jobs := &fakeProblemFollowupStore{jobs: []*problemfollowup.Job{{ID: 1, Status: "delivering"}}, delivery: &problemfollowup.Delivery{AppUserID: 7, SessionID: 42, DeepLink: "/chat/42"}}
	pushed := 0
	s := &Server{problemFollowups: jobs, problemFollowupPush: func(_ context.Context, d problemfollowup.Delivery) error {
		pushed++
		if jobs.deliveries != 1 || d.DeepLink != "/chat/42" {
			t.Fatal("push preceded durable delivery")
		}
		return errors.New("push unavailable")
	}}
	if err := s.runProblemFollowupOnce(context.Background(), time.Now()); err != nil {
		t.Fatal(err)
	}
	if pushed != 1 || jobs.failures != 0 {
		t.Fatalf("pushes=%d retries=%d", pushed, jobs.failures)
	}
	if err := s.runProblemFollowupOnce(context.Background(), time.Now()); err != nil {
		t.Fatal(err)
	}
	if pushed != 1 {
		t.Fatalf("duplicate push %d", pushed)
	}
}

func TestProblemFollowupCancelledDeliveryDoesNotPush(t *testing.T) {
	jobs := &fakeProblemFollowupStore{jobs: []*problemfollowup.Job{{ID: 1, Status: "delivering"}}}
	s := &Server{problemFollowups: jobs, problemFollowupPush: func(context.Context, problemfollowup.Delivery) error { t.Fatal("cancelled task pushed"); return nil }}
	if err := s.runProblemFollowupOnce(context.Background(), time.Now()); err != nil {
		t.Fatal(err)
	}
	if jobs.deliveries != 1 || jobs.failures != 0 {
		t.Fatalf("jobs=%+v", jobs)
	}
}

func TestProblemFollowupVoiceActivityPrecedesASRAndRejectsInvalidUpload(t *testing.T) {
	for _, tt := range []struct {
		name, duration        string
		beginErr              error
		wantCalls, wantStatus int
	}{
		{name: "accepted voice cancels even when recognition unavailable", duration: "1200", wantCalls: 1, wantStatus: http.StatusServiceUnavailable},
		{name: "invalid voice does not cancel", duration: "200", wantCalls: 0, wantStatus: http.StatusBadRequest},
		{name: "activity failure refuses acceptance", duration: "1200", beginErr: errors.New("activity write failed"), wantCalls: 1, wantStatus: http.StatusServiceUnavailable},
	} {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeVoiceChatStore{fakeAppChatStreamStore: newFakeAppChatStreamStore()}
			s := newVoiceChatTestServer(store, nil)
			jobs := &fakeProblemFollowupStore{beginErr: tt.beginErr}
			s.problemFollowups = jobs
			body, contentType := voiceChatMultipartBody(t, "voice.aac", "audio/aac", "audio", tt.duration)
			req := httptest.NewRequest(http.MethodPost, "/api/app/chat/sessions/42/voice", body)
			req.Header.Set("Content-Type", contentType)
			req = req.WithContext(contextWithAppUser(req.Context(), auth.UserInfo{ID: 7}))
			w := httptest.NewRecorder()
			s.appChatRouter(w, req)
			if w.Code != tt.wantStatus || jobs.beginCalls != tt.wantCalls {
				t.Fatalf("status=%d calls=%d body=%s", w.Code, jobs.beginCalls, w.Body.String())
			}
			if tt.beginErr != nil && !strings.Contains(w.Body.String(), "消息暂未提交") {
				t.Fatalf("activity failure was ignored: %s", w.Body.String())
			}
		})
	}
}
