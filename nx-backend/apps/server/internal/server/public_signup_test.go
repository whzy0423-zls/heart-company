package server

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"nine-xing/nx-backend/apps/server/internal/businessmessage"
	"nine-xing/nx-backend/apps/server/internal/dbtx"
	"nine-xing/nx-backend/apps/server/internal/signup"
)

type publicSignupTx struct {
	commitErr error
}

func (t *publicSignupTx) ExecContext(context.Context, string, ...any) (sql.Result, error) {
	return nil, errors.New("unexpected ExecContext")
}

func (t *publicSignupTx) QueryContext(context.Context, string, ...any) (*sql.Rows, error) {
	return nil, errors.New("unexpected QueryContext")
}

func (t *publicSignupTx) QueryRowContext(context.Context, string, ...any) *sql.Row { return &sql.Row{} }
func (t *publicSignupTx) Commit() error                                            { return t.commitErr }
func (t *publicSignupTx) Rollback() error                                          { return nil }

type publicSignupBeginner struct{ tx *publicSignupTx }

func (b publicSignupBeginner) BeginTx(context.Context, *sql.TxOptions) (dbtx.Tx, error) {
	return b.tx, nil
}

type publicSignupLeadWriter struct {
	lead        signup.Lead
	gotPlatform string
}

func (w *publicSignupLeadWriter) CreateWithDBTX(_ context.Context, _ dbtx.DBTX, _ signup.LeadInput, _ *http.Request, platform string) (signup.Lead, error) {
	w.gotPlatform = platform
	return w.lead, nil
}

type publicSignupMessageWriter struct{}

func (publicSignupMessageWriter) Create(context.Context, dbtx.DBTX, businessmessage.Event) (bool, error) {
	return true, nil
}

func TestPublicSignupBroadcastsOnlyAfterServiceCommit(t *testing.T) {
	tests := []struct {
		name          string
		commitErr     error
		wantStatus    int
		wantBroadcast bool
	}{
		{name: "committed", wantStatus: http.StatusOK, wantBroadcast: true},
		{name: "commit failed", commitErr: errors.New("commit failed"), wantStatus: http.StatusBadRequest, wantBroadcast: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lead := signup.Lead{ID: "42", Name: "张三", Contact: "13812345678", ContactType: signup.ContactTypePhone, SourcePlatform: "website"}
			leadWriter := &publicSignupLeadWriter{lead: lead}
			service := signup.NewService(publicSignupBeginner{tx: &publicSignupTx{commitErr: tt.commitErr}}, leadWriter, publicSignupMessageWriter{})
			s := &Server{
				signupService:     service,
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
			if leadWriter.gotPlatform != "website" {
				t.Fatalf("service source platform = %q, want website", leadWriter.gotPlatform)
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
