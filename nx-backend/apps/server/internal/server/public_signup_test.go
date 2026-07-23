package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"nine-xing/nx-backend/apps/server/internal/signup"
)

type publicSignupCreator struct {
	lead signup.Lead
	err  error
}

func (c publicSignupCreator) CreateWebsiteSignup(_ context.Context, _ signup.LeadInput, _ *http.Request) (signup.Lead, error) {
	return c.lead, c.err
}

func TestPublicSignupBroadcastsOnlyAfterServiceCommit(t *testing.T) {
	tests := []struct {
		name          string
		serviceErr    error
		wantStatus    int
		wantBroadcast bool
	}{
		{name: "committed", wantStatus: http.StatusOK, wantBroadcast: true},
		{name: "commit failed", serviceErr: errors.New("commit failed"), wantStatus: http.StatusBadRequest, wantBroadcast: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lead := signup.Lead{ID: "42", Name: "张三", Contact: "13812345678", ContactType: signup.ContactTypePhone, SourcePlatform: "website"}
			s := &Server{
				signupService:     publicSignupCreator{lead: lead, err: tt.serviceErr},
				signupSubscribers: map[chan signup.Lead]struct{}{},
			}
			ch := make(chan signup.Lead, 1)
			s.addSignupSubscriber(ch)

			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/api/public/signups", strings.NewReader(`{"name":"张三","contactType":"phone","contact":"13812345678","sourcePlatform":"miniapp"}`))
			s.publicSignup(recorder, request)

			if recorder.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", recorder.Code, tt.wantStatus, recorder.Body.String())
			}
			select {
			case got := <-ch:
				if !tt.wantBroadcast {
					t.Fatalf("unexpected broadcast after failed commit: %+v", got)
				}
				if got.ID != lead.ID {
					t.Fatalf("broadcast lead = %+v, want %+v", got, lead)
				}
			default:
				if tt.wantBroadcast {
					t.Fatal("expected committed signup broadcast")
				}
			}
		})
	}
}
