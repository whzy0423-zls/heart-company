package signup

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/businessmessage"
	"nine-xing/nx-backend/apps/server/internal/dbtx"
)

type fakeBeginner struct {
	tx    *fakeTx
	err   error
	calls int
	ctx   context.Context
}

func (f *fakeBeginner) BeginTx(ctx context.Context, _ *sql.TxOptions) (dbtx.Tx, error) {
	f.calls++
	f.ctx = ctx
	if f.err != nil {
		return nil, f.err
	}
	return f.tx, nil
}

type fakeTx struct {
	commitErr     error
	commitCalls   int
	rollbackCalls int
}

func (f *fakeTx) ExecContext(context.Context, string, ...any) (sql.Result, error) {
	return nil, errors.New("unexpected ExecContext")
}

func (f *fakeTx) QueryContext(context.Context, string, ...any) (*sql.Rows, error) {
	return nil, errors.New("unexpected QueryContext")
}

func (f *fakeTx) QueryRowContext(context.Context, string, ...any) *sql.Row { return &sql.Row{} }

func (f *fakeTx) Commit() error {
	f.commitCalls++
	return f.commitErr
}

func (f *fakeTx) Rollback() error {
	f.rollbackCalls++
	return nil
}

type fakeLeadWriter struct {
	lead        Lead
	err         error
	calls       int
	gotQ        dbtx.DBTX
	gotInput    LeadInput
	gotRequest  *http.Request
	gotPlatform string
	gotCtx      context.Context
	waitForDone bool
}

func (f *fakeLeadWriter) CreateWithDBTX(ctx context.Context, q dbtx.DBTX, input LeadInput, r *http.Request, platform string) (Lead, error) {
	f.calls++
	f.gotCtx = ctx
	f.gotQ = q
	f.gotInput = input
	f.gotRequest = r
	f.gotPlatform = platform
	if f.waitForDone {
		<-ctx.Done()
		return Lead{}, ctx.Err()
	}
	return f.lead, f.err
}

type fakeMessageWriter struct {
	created bool
	err     error
	calls   int
	gotQ    dbtx.DBTX
	event   businessmessage.Event
	gotCtx  context.Context
}

func (f *fakeMessageWriter) Create(ctx context.Context, q dbtx.DBTX, event businessmessage.Event) (bool, error) {
	f.calls++
	f.gotCtx = ctx
	f.gotQ = q
	f.event = event
	return f.created, f.err
}

func TestWebsiteSignupBeginFailureStopsWrites(t *testing.T) {
	beginErr := errors.New("begin unavailable")
	beginner := &fakeBeginner{err: beginErr}
	leads := &fakeLeadWriter{}
	messages := &fakeMessageWriter{}

	_, err := NewService(beginner, leads, messages).CreateWebsiteSignup(context.Background(), LeadInput{Name: "张三"}, nil)

	if !errors.Is(err, beginErr) || !strings.Contains(err.Error(), "begin website signup transaction") {
		t.Fatalf("expected wrapped begin error, got %v", err)
	}
	if leads.calls != 0 || messages.calls != 0 {
		t.Fatalf("writes must not run after begin failure: leads=%d messages=%d", leads.calls, messages.calls)
	}
}

func TestWebsiteSignupLeadFailureRollsBack(t *testing.T) {
	leadErr := errors.New("lead invalid")
	tx := &fakeTx{}
	leads := &fakeLeadWriter{err: leadErr}
	messages := &fakeMessageWriter{}

	_, err := NewService(&fakeBeginner{tx: tx}, leads, messages).CreateWebsiteSignup(context.Background(), LeadInput{Name: "张三"}, nil)

	if !errors.Is(err, leadErr) || !strings.Contains(err.Error(), "create website signup") {
		t.Fatalf("expected wrapped lead error, got %v", err)
	}
	if tx.rollbackCalls != 1 || tx.commitCalls != 0 {
		t.Fatalf("expected one rollback and no commit, got rollback=%d commit=%d", tx.rollbackCalls, tx.commitCalls)
	}
	if messages.calls != 0 {
		t.Fatalf("message must not be written after lead failure, calls=%d", messages.calls)
	}
}

func TestWebsiteSignupMessageFailureRollsBackWithoutCommit(t *testing.T) {
	messageErr := errors.New("message unavailable")
	tx := &fakeTx{}
	leads := &fakeLeadWriter{lead: Lead{ID: "42", Name: "张三", ContactType: ContactTypePhone, Contact: "13812345678"}}
	messages := &fakeMessageWriter{err: messageErr}

	_, err := NewService(&fakeBeginner{tx: tx}, leads, messages).CreateWebsiteSignup(context.Background(), LeadInput{Name: "张三"}, nil)

	if !errors.Is(err, messageErr) || !strings.Contains(err.Error(), "create website signup message") {
		t.Fatalf("expected wrapped message error, got %v", err)
	}
	if tx.rollbackCalls != 1 || tx.commitCalls != 0 {
		t.Fatalf("expected one rollback and no commit, got rollback=%d commit=%d", tx.rollbackCalls, tx.commitCalls)
	}
}

func TestWebsiteSignupCommitFailureReturnsError(t *testing.T) {
	commitErr := errors.New("commit unavailable")
	tx := &fakeTx{commitErr: commitErr}
	leads := &fakeLeadWriter{lead: Lead{ID: "42", Name: "张三", ContactType: ContactTypePhone, Contact: "13812345678"}}
	messages := &fakeMessageWriter{created: true}

	_, err := NewService(&fakeBeginner{tx: tx}, leads, messages).CreateWebsiteSignup(context.Background(), LeadInput{Name: "张三"}, nil)

	if !errors.Is(err, commitErr) || !strings.Contains(err.Error(), "commit website signup transaction") {
		t.Fatalf("expected wrapped commit error, got %v", err)
	}
	if tx.commitCalls != 1 {
		t.Fatalf("expected exactly one commit attempt, got %d", tx.commitCalls)
	}
}

func TestWebsiteSignupSuccessUsesWebsiteSourceAndCommittedMessage(t *testing.T) {
	tx := &fakeTx{}
	request, _ := http.NewRequest(http.MethodPost, "/api/public/signups", nil)
	input := LeadInput{Name: "张三", Contact: "13812345678", ContactType: ContactTypePhone}
	want := Lead{ID: "42", Name: "张三", ContactType: ContactTypePhone, Contact: "13812345678", SourcePlatform: "website"}
	leads := &fakeLeadWriter{lead: want}
	messages := &fakeMessageWriter{created: true}

	beginner := &fakeBeginner{tx: tx}
	started := time.Now()
	got, err := NewService(beginner, leads, messages).CreateWebsiteSignup(context.Background(), input, request)

	if err != nil {
		t.Fatalf("CreateWebsiteSignup() error = %v", err)
	}
	if got != want {
		t.Fatalf("CreateWebsiteSignup() = %+v, want %+v", got, want)
	}
	if leads.gotQ != tx || messages.gotQ != tx {
		t.Fatal("lead and message must share the same transaction")
	}
	if beginner.ctx != leads.gotCtx || beginner.ctx != messages.gotCtx {
		t.Fatal("begin, lead, and message must receive the same operation context")
	}
	deadline, ok := beginner.ctx.Deadline()
	if !ok {
		t.Fatal("website signup operation context must have a deadline")
	}
	remaining := deadline.Sub(started)
	if remaining < 9*time.Second || remaining > 11*time.Second {
		t.Fatalf("website signup deadline remaining = %v, want about 10s", remaining)
	}
	if leads.gotPlatform != "website" {
		t.Fatalf("source platform = %q, want website", leads.gotPlatform)
	}
	if messages.event.BusinessID != "42" || messages.event.TargetPath != "/customer/signups?leadId=42&open=detail" {
		t.Fatalf("unexpected message target: %+v", messages.event)
	}
	if messages.event.Content != "张三提交了官网报名，手机号：138****5678" {
		t.Fatalf("message content = %q", messages.event.Content)
	}
	if tx.commitCalls != 1 || tx.rollbackCalls != 1 {
		t.Fatalf("expected one commit and deferred rollback, got commit=%d rollback=%d", tx.commitCalls, tx.rollbackCalls)
	}
}

func TestWebsiteSignupCanceledContextRollsBackWithoutCommit(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	cancel()
	tx := &fakeTx{}
	leads := &fakeLeadWriter{waitForDone: true}
	messages := &fakeMessageWriter{}

	_, err := NewService(&fakeBeginner{tx: tx}, leads, messages).CreateWebsiteSignup(parent, LeadInput{Name: "张三"}, nil)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled, got %v", err)
	}
	if tx.rollbackCalls != 1 || tx.commitCalls != 0 {
		t.Fatalf("expected rollback without commit, got rollback=%d commit=%d", tx.rollbackCalls, tx.commitCalls)
	}
	if messages.calls != 0 {
		t.Fatalf("message must not run after cancellation, calls=%d", messages.calls)
	}
}

func TestWebsiteSignupDeadlineExceededRollsBackWithoutCommit(t *testing.T) {
	parent, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	tx := &fakeTx{}
	leads := &fakeLeadWriter{waitForDone: true}

	_, err := NewService(&fakeBeginner{tx: tx}, leads, &fakeMessageWriter{}).CreateWebsiteSignup(parent, LeadInput{Name: "张三"}, nil)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
	if tx.rollbackCalls != 1 || tx.commitCalls != 0 {
		t.Fatalf("expected rollback without commit, got rollback=%d commit=%d", tx.rollbackCalls, tx.commitCalls)
	}
}

func TestWebsiteSignupRejectsUnconfiguredService(t *testing.T) {
	tests := []struct {
		name    string
		service *Service
	}{
		{name: "nil receiver", service: nil},
		{name: "missing beginner", service: NewService(nil, &fakeLeadWriter{}, &fakeMessageWriter{})},
		{name: "missing leads", service: NewService(&fakeBeginner{tx: &fakeTx{}}, nil, &fakeMessageWriter{})},
		{name: "missing messages", service: NewService(&fakeBeginner{tx: &fakeTx{}}, &fakeLeadWriter{}, nil)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.service.CreateWebsiteSignup(context.Background(), LeadInput{}, nil)
			if !errors.Is(err, ErrServiceNotConfigured) {
				t.Fatalf("expected ErrServiceNotConfigured, got %v", err)
			}
		})
	}
}
