package miniapp

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/businessmessage"
	"nine-xing/nx-backend/apps/server/internal/dbtx"
)

type userServiceFakeBeginner struct {
	tx    *userServiceFakeTx
	err   error
	calls int
	ctx   context.Context
}

func (f *userServiceFakeBeginner) BeginTx(ctx context.Context, _ *sql.TxOptions) (dbtx.Tx, error) {
	f.calls++
	f.ctx = ctx
	if f.err != nil {
		return nil, f.err
	}
	return f.tx, nil
}

type userServiceFakeTx struct {
	commitErr     error
	commitCalls   int
	rollbackCalls int
}

func (f *userServiceFakeTx) ExecContext(context.Context, string, ...any) (sql.Result, error) {
	return nil, errors.New("unexpected ExecContext")
}

func (f *userServiceFakeTx) QueryContext(context.Context, string, ...any) (*sql.Rows, error) {
	return nil, errors.New("unexpected QueryContext")
}

func (f *userServiceFakeTx) QueryRowContext(context.Context, string, ...any) *sql.Row {
	return &sql.Row{}
}

func (f *userServiceFakeTx) Commit() error {
	f.commitCalls++
	return f.commitErr
}

func (f *userServiceFakeTx) Rollback() error {
	f.rollbackCalls++
	return nil
}

type userServiceFakeUsers struct {
	id          int64
	created     bool
	upsertErr   error
	user        User
	getErr      error
	upsertCalls int
	getCalls    int
	gotQ        dbtx.DBTX
	getQ        dbtx.DBTX
	gotCtx      context.Context
	getCtx      context.Context
	openid      string
	unionid     string
	channel     string
	scene       string
	waitForDone bool
}

func (f *userServiceFakeUsers) UpsertByOpenIDWithDBTX(ctx context.Context, q dbtx.DBTX, openid, unionid, channel, scene string) (int64, bool, error) {
	f.upsertCalls++
	f.gotCtx = ctx
	f.gotQ = q
	f.openid = openid
	f.unionid = unionid
	f.channel = channel
	f.scene = scene
	if f.waitForDone {
		<-ctx.Done()
		return 0, false, ctx.Err()
	}
	return f.id, f.created, f.upsertErr
}

func (f *userServiceFakeUsers) GetUserWithDBTX(ctx context.Context, q dbtx.DBTX, id int64) (User, error) {
	f.getCalls++
	f.getCtx = ctx
	f.getQ = q
	if id != f.id {
		return User{}, errors.New("unexpected user id")
	}
	return f.user, f.getErr
}

type userServiceFakeMessages struct {
	created bool
	err     error
	calls   int
	gotQ    dbtx.DBTX
	gotCtx  context.Context
	event   businessmessage.Event
}

func (f *userServiceFakeMessages) Create(ctx context.Context, q dbtx.DBTX, event businessmessage.Event) (bool, error) {
	f.calls++
	f.gotCtx = ctx
	f.gotQ = q
	f.event = event
	return f.created, f.err
}

func TestMiniappUserFirstLoginCreatesMessageInSameTransaction(t *testing.T) {
	tx := &userServiceFakeTx{}
	users := &userServiceFakeUsers{
		id:      42,
		created: true,
		user:    User{ID: "42", Nickname: " 小芯 "},
	}
	messages := &userServiceFakeMessages{created: true}
	beginner := &userServiceFakeBeginner{tx: tx}
	started := time.Now()

	id, err := NewService(beginner, users, messages).UpsertUser(context.Background(), " openid-secret ", " unionid-secret ", " 广告投放 ", " 首页二维码 ")

	if err != nil {
		t.Fatalf("UpsertUser() error = %v", err)
	}
	if id != 42 {
		t.Fatalf("UpsertUser() id = %d, want 42", id)
	}
	if users.gotQ != tx || users.getQ != tx || messages.gotQ != tx {
		t.Fatal("upsert, user read, and message must share one transaction")
	}
	if beginner.ctx != users.gotCtx || beginner.ctx != users.getCtx || beginner.ctx != messages.gotCtx {
		t.Fatal("begin, user writes, user read, and message must share one operation context")
	}
	deadline, ok := beginner.ctx.Deadline()
	if !ok {
		t.Fatal("miniapp user operation context must have a deadline")
	}
	if remaining := deadline.Sub(started); remaining < 9*time.Second || remaining > 11*time.Second {
		t.Fatalf("operation deadline remaining = %v, want about 10s", remaining)
	}
	if users.openid != "openid-secret" || users.unionid != "unionid-secret" || users.channel != "广告投放" || users.scene != "首页二维码" {
		t.Fatalf("service input was not trimmed: %+v", users)
	}
	wantEvent := businessmessage.MiniappUserCreated("42", "小芯")
	if messages.event != wantEvent {
		t.Fatalf("message event = %+v, want %+v", messages.event, wantEvent)
	}
	if strings.Contains(messages.event.Title, "openid-secret") || strings.Contains(messages.event.Content, "openid-secret") ||
		strings.Contains(messages.event.Title, "unionid-secret") || strings.Contains(messages.event.Content, "unionid-secret") {
		t.Fatalf("message leaked identity: %+v", messages.event)
	}
	if tx.commitCalls != 1 || tx.rollbackCalls != 1 {
		t.Fatalf("commit=%d rollback=%d, want 1 and deferred 1", tx.commitCalls, tx.rollbackCalls)
	}
}

func TestMiniappUserFirstLoginUsesSafeFallbackName(t *testing.T) {
	tx := &userServiceFakeTx{}
	users := &userServiceFakeUsers{id: 9, created: true, user: User{ID: "9", Nickname: "昵称-openid-secret"}}
	messages := &userServiceFakeMessages{created: true}

	_, err := NewService(&userServiceFakeBeginner{tx: tx}, users, messages).UpsertUser(context.Background(), "openid-secret", "unionid-secret", "", "")

	if err != nil {
		t.Fatalf("UpsertUser() error = %v", err)
	}
	if messages.event.Content != "微信用户9首次进入小程序" {
		t.Fatalf("message content = %q, want safe id fallback", messages.event.Content)
	}
	if strings.Contains(messages.event.Content, "openid-secret") || strings.Contains(messages.event.Content, "unionid-secret") {
		t.Fatalf("message leaked identity: %+v", messages.event)
	}
}

func TestMiniappUserRepeatLoginDoesNotCreateMessageOrReadUser(t *testing.T) {
	tx := &userServiceFakeTx{}
	users := &userServiceFakeUsers{id: 42, created: false}
	messages := &userServiceFakeMessages{}

	id, err := NewService(&userServiceFakeBeginner{tx: tx}, users, messages).UpsertUser(context.Background(), "openid", "unionid", "channel", "scene")

	if err != nil || id != 42 {
		t.Fatalf("UpsertUser() = (%d, %v), want (42, nil)", id, err)
	}
	if users.getCalls != 0 || messages.calls != 0 {
		t.Fatalf("repeat login must skip user read/message: get=%d message=%d", users.getCalls, messages.calls)
	}
	if tx.commitCalls != 1 || tx.rollbackCalls != 1 {
		t.Fatalf("commit=%d rollback=%d, want 1 and deferred 1", tx.commitCalls, tx.rollbackCalls)
	}
}

func TestMiniappUserMessageFailureRollsBackWithoutCommit(t *testing.T) {
	wantErr := errors.New("message unavailable")
	tx := &userServiceFakeTx{}
	users := &userServiceFakeUsers{id: 42, created: true, user: User{ID: "42"}}
	messages := &userServiceFakeMessages{err: wantErr}

	_, err := NewService(&userServiceFakeBeginner{tx: tx}, users, messages).UpsertUser(context.Background(), "openid", "", "", "")

	if !errors.Is(err, wantErr) || !strings.Contains(err.Error(), "create miniapp user message") {
		t.Fatalf("expected wrapped message error, got %v", err)
	}
	if tx.commitCalls != 0 || tx.rollbackCalls != 1 {
		t.Fatalf("commit=%d rollback=%d, want 0 and 1", tx.commitCalls, tx.rollbackCalls)
	}
}

func TestMiniappUserCommitFailureReturnsError(t *testing.T) {
	wantErr := errors.New("commit unavailable")
	tx := &userServiceFakeTx{commitErr: wantErr}
	users := &userServiceFakeUsers{id: 42, created: false}

	_, err := NewService(&userServiceFakeBeginner{tx: tx}, users, &userServiceFakeMessages{}).UpsertUser(context.Background(), "openid", "", "", "")

	if !errors.Is(err, wantErr) || !strings.Contains(err.Error(), "commit miniapp user transaction") {
		t.Fatalf("expected wrapped commit error, got %v", err)
	}
	if tx.commitCalls != 1 || tx.rollbackCalls != 1 {
		t.Fatalf("commit=%d rollback=%d, want 1 and deferred 1", tx.commitCalls, tx.rollbackCalls)
	}
}

func TestMiniappUserCanceledContextRollsBackWithoutCommit(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	tx := &userServiceFakeTx{}
	users := &userServiceFakeUsers{waitForDone: true}

	_, err := NewService(&userServiceFakeBeginner{tx: tx}, users, &userServiceFakeMessages{}).UpsertUser(ctx, "openid", "", "", "")

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled, got %v", err)
	}
	if tx.commitCalls != 0 || tx.rollbackCalls != 1 {
		t.Fatalf("commit=%d rollback=%d, want 0 and 1", tx.commitCalls, tx.rollbackCalls)
	}
}

func TestMiniappUserRejectsUnconfiguredService(t *testing.T) {
	tests := []struct {
		name    string
		service *Service
	}{
		{name: "nil receiver", service: nil},
		{name: "missing beginner", service: NewService(nil, &userServiceFakeUsers{}, &userServiceFakeMessages{})},
		{name: "missing users", service: NewService(&userServiceFakeBeginner{tx: &userServiceFakeTx{}}, nil, &userServiceFakeMessages{})},
		{name: "missing messages", service: NewService(&userServiceFakeBeginner{tx: &userServiceFakeTx{}}, &userServiceFakeUsers{}, nil)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.service.UpsertUser(context.Background(), "openid", "", "", "")
			if !errors.Is(err, ErrServiceNotConfigured) {
				t.Fatalf("expected ErrServiceNotConfigured, got %v", err)
			}
		})
	}
}

func TestMiniappUserRejectsInvalidSourceBeforeStartingTransaction(t *testing.T) {
	tests := []struct {
		name    string
		openid  string
		unionid string
		channel string
		scene   string
		wantErr error
	}{
		{name: "openid unsafe character", openid: "openid.with-dot", wantErr: ErrInvalidOpenID},
		{name: "unionid newline", openid: "openid", unionid: "union\nid", wantErr: ErrInvalidUserSource},
		{name: "channel nul", openid: "openid", channel: "来源\x00参数", wantErr: ErrInvalidUserSource},
		{name: "scene del", openid: "openid", scene: "场景\x7f参数", wantErr: ErrInvalidUserSource},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			beginner := &userServiceFakeBeginner{tx: &userServiceFakeTx{}}
			users := &userServiceFakeUsers{id: 1}
			messages := &userServiceFakeMessages{}

			_, err := NewService(beginner, users, messages).UpsertUser(context.Background(), tt.openid, tt.unionid, tt.channel, tt.scene)

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if beginner.calls != 0 || users.upsertCalls != 0 || messages.calls != 0 {
				t.Fatalf("invalid source must stop before transaction: begin=%d upsert=%d message=%d", beginner.calls, users.upsertCalls, messages.calls)
			}
		})
	}
}

func TestMiniappUserStoreRejectsInvalidInputBeforeQuery(t *testing.T) {
	store := &Store{}
	tests := []struct {
		name    string
		openid  string
		unionid string
		channel string
		scene   string
		wantErr error
	}{
		{name: "nil query target", openid: "openid", wantErr: ErrNilDBTX},
		{name: "empty openid", openid: " \n\t ", wantErr: ErrInvalidOpenID},
		{name: "long openid", openid: strings.Repeat("o", maxOpenIDRunes+1), wantErr: ErrInvalidOpenID},
		{name: "long unionid", openid: "openid", unionid: strings.Repeat("u", maxUnionIDRunes+1), wantErr: ErrInvalidUserSource},
		{name: "long channel", openid: "openid", channel: strings.Repeat("c", maxChannelRunes+1), wantErr: ErrInvalidUserSource},
		{name: "long scene", openid: "openid", scene: strings.Repeat("s", maxSceneRunes+1), wantErr: ErrInvalidUserSource},
		{name: "openid nul", openid: "open\x00id", wantErr: ErrInvalidOpenID},
		{name: "openid newline", openid: "open\nid", wantErr: ErrInvalidOpenID},
		{name: "openid del", openid: "open\x7fid", wantErr: ErrInvalidOpenID},
		{name: "openid unsafe character", openid: "openid.with-dot", wantErr: ErrInvalidOpenID},
		{name: "unionid control", openid: "openid", unionid: "union\nid", wantErr: ErrInvalidUserSource},
		{name: "unionid unsafe character", openid: "openid", unionid: "union.id", wantErr: ErrInvalidUserSource},
		{name: "channel control", openid: "openid", channel: "中文\x00来源", wantErr: ErrInvalidUserSource},
		{name: "scene control", openid: "openid", scene: "中文\x7f场景", wantErr: ErrInvalidUserSource},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var q dbtx.DBTX = &userServiceFakeTx{}
			if errors.Is(tt.wantErr, ErrNilDBTX) {
				q = nil
			}
			_, _, err := store.UpsertByOpenIDWithDBTX(context.Background(), q, tt.openid, tt.unionid, tt.channel, tt.scene)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}
