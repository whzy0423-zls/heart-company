package miniapp

import (
	"context"
	"database/sql"
	"encoding/json"
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
	record      TestRecord
	insertErr   error
	updateErr   error
	insertCalls int
	updateCalls int
	insertQ     dbtx.DBTX
	updateQ     dbtx.DBTX
	insertCtx   context.Context
	updateCtx   context.Context
	insertUID   int64
	updateUID   int64
	input       TestRecordInput
	mainType    int
	waitInsert  bool
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

func (f *userServiceFakeUsers) InsertTestRecord(ctx context.Context, q dbtx.DBTX, userID int64, in TestRecordInput) (TestRecord, error) {
	f.insertCalls++
	f.insertCtx = ctx
	f.insertQ = q
	f.insertUID = userID
	f.input = in
	if f.waitInsert {
		<-ctx.Done()
		return TestRecord{}, ctx.Err()
	}
	return f.record, f.insertErr
}

func (f *userServiceFakeUsers) UpdateMainType(ctx context.Context, q dbtx.DBTX, userID int64, resultType int) error {
	f.updateCalls++
	f.updateCtx = ctx
	f.updateQ = q
	f.updateUID = userID
	f.mainType = resultType
	return f.updateErr
}

type userServiceFakeMessages struct {
	created bool
	err     error
	calls   int
	gotQ    dbtx.DBTX
	gotCtx  context.Context
	event   businessmessage.Event
}

func validTestRecordInput(resultType int) TestRecordInput {
	return TestRecordInput{
		ResultType: resultType,
		Score:      json.RawMessage(`{}`),
		Centers:    json.RawMessage(`[]`),
	}
}

func newTestRecordService(beginner dbtx.Beginner, store *userServiceFakeUsers, messages messageWriter) *Service {
	return NewService(beginner, store, messages, WithTestRecordWriter(store))
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

func TestMiniappTestRecordCreatesMessageAndUpdatesMainTypeInSameTransaction(t *testing.T) {
	tx := &userServiceFakeTx{}
	store := &userServiceFakeUsers{
		id:     42,
		user:   User{ID: "42", Nickname: " 小芯 ", MainType: 0},
		record: TestRecord{ID: "77", Gender: "female", ResultType: 9, SecondType: 1, Scores: json.RawMessage(`{"9":18}`), Centers: json.RawMessage(`[{"key":"gut","pct":80}]`), CreateTime: "2026/07/23 12:34:56"},
	}
	messages := &userServiceFakeMessages{created: true}
	beginner := &userServiceFakeBeginner{tx: tx}
	in := TestRecordInput{Gender: "female", ResultType: 9, SecondType: 1, Score: json.RawMessage(`{"9":18}`), Centers: json.RawMessage(`[{"key":"gut","pct":80}]`)}

	record, err := newTestRecordService(beginner, store, messages).SaveTestRecord(context.Background(), 42, in)

	if err != nil {
		t.Fatalf("SaveTestRecord() error = %v", err)
	}
	if record.ID != "77" {
		t.Fatalf("record = %+v, want id 77", record)
	}
	if store.insertQ != tx || store.updateQ != tx || store.getQ != tx || messages.gotQ != tx {
		t.Fatal("record insert, main type update, user read and message must share one transaction")
	}
	if beginner.ctx != store.insertCtx || beginner.ctx != store.updateCtx || beginner.ctx != store.getCtx || beginner.ctx != messages.gotCtx {
		t.Fatal("all test record operations must share one operation context")
	}
	deadline, ok := beginner.ctx.Deadline()
	if !ok || time.Until(deadline) < 9*time.Second || time.Until(deadline) > 11*time.Second {
		t.Fatalf("operation deadline = %v, want about 10 seconds", deadline)
	}
	if store.insertUID != 42 || store.updateUID != 42 || store.mainType != 9 {
		t.Fatalf("unexpected user/main type writes: insertUID=%d updateUID=%d mainType=%d", store.insertUID, store.updateUID, store.mainType)
	}
	if string(store.input.Scores) != `{"9":18}` || len(store.input.Score) != 0 {
		t.Fatalf("store input scores were not normalized: score=%s scores=%s", store.input.Score, store.input.Scores)
	}
	wantEvent := businessmessage.MiniappQuizSubmitted("77", "42", "微信用户42", 9)
	wantEvent.Content += "，提交时间：2026/07/23 12:34:56"
	if messages.event != wantEvent {
		t.Fatalf("message event = %+v, want %+v", messages.event, wantEvent)
	}
	if strings.Contains(messages.event.Content, "openid") || strings.Contains(messages.event.Content, "unionid") {
		t.Fatalf("message leaked identity: %+v", messages.event)
	}
	if tx.commitCalls != 1 || tx.rollbackCalls != 1 {
		t.Fatalf("commit=%d rollback=%d, want 1 and deferred 1", tx.commitCalls, tx.rollbackCalls)
	}
}

func TestMiniappTestRecordMessageNeverUsesNicknameThatMayContainWechatIdentity(t *testing.T) {
	for _, nickname := range []string{"openid-secret-value", "unionid-secret-value", "昵称-openid-secret-value"} {
		t.Run(nickname, func(t *testing.T) {
			tx := &userServiceFakeTx{}
			store := &userServiceFakeUsers{
				id:     42,
				user:   User{ID: "42", Nickname: nickname},
				record: TestRecord{ID: "77", ResultType: 9, CreateTime: "2026/07/23 12:34:56"},
			}
			messages := &userServiceFakeMessages{created: true}

			_, err := newTestRecordService(&userServiceFakeBeginner{tx: tx}, store, messages).SaveTestRecord(context.Background(), 42, validTestRecordInput(9))

			if err != nil {
				t.Fatalf("SaveTestRecord() error = %v", err)
			}
			if strings.Contains(messages.event.Content, nickname) || !strings.Contains(messages.event.Content, "微信用户42") {
				t.Fatalf("message did not use safe identifier: %+v", messages.event)
			}
		})
	}
}

func TestMiniappTestRecordFailuresRollbackWithoutCommit(t *testing.T) {
	wantErr := errors.New("dependency failed")
	tests := []struct {
		name       string
		configure  func(*userServiceFakeUsers, *userServiceFakeMessages)
		wantPrefix string
		wantUpdate int
		wantGet    int
		wantMsg    int
	}{
		{name: "insert", configure: func(store *userServiceFakeUsers, _ *userServiceFakeMessages) { store.insertErr = wantErr }, wantPrefix: "insert miniapp test record"},
		{name: "update main type", configure: func(store *userServiceFakeUsers, _ *userServiceFakeMessages) { store.updateErr = wantErr }, wantPrefix: "update miniapp main type", wantUpdate: 1},
		{name: "read user", configure: func(store *userServiceFakeUsers, _ *userServiceFakeMessages) { store.getErr = wantErr }, wantPrefix: "get miniapp test user", wantUpdate: 1, wantGet: 1},
		{name: "create message", configure: func(_ *userServiceFakeUsers, messages *userServiceFakeMessages) { messages.err = wantErr }, wantPrefix: "create miniapp test message", wantUpdate: 1, wantGet: 1, wantMsg: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx := &userServiceFakeTx{}
			store := &userServiceFakeUsers{id: 42, user: User{ID: "42", Nickname: "小芯"}, record: TestRecord{ID: "77", ResultType: 9}}
			messages := &userServiceFakeMessages{}
			tt.configure(store, messages)

			_, err := newTestRecordService(&userServiceFakeBeginner{tx: tx}, store, messages).SaveTestRecord(context.Background(), 42, validTestRecordInput(9))

			if !errors.Is(err, wantErr) || !strings.Contains(err.Error(), tt.wantPrefix) {
				t.Fatalf("error = %v, want wrapped %q", err, tt.wantPrefix)
			}
			if tx.commitCalls != 0 || tx.rollbackCalls != 1 {
				t.Fatalf("commit=%d rollback=%d, want 0 and 1", tx.commitCalls, tx.rollbackCalls)
			}
			if store.updateCalls != tt.wantUpdate || store.getCalls != tt.wantGet || messages.calls != tt.wantMsg {
				t.Fatalf("later calls update/get/message = %d/%d/%d, want %d/%d/%d", store.updateCalls, store.getCalls, messages.calls, tt.wantUpdate, tt.wantGet, tt.wantMsg)
			}
		})
	}
}

func TestMiniappTestRecordRepeatedSubmissionsCreateDistinctMessages(t *testing.T) {
	var events []businessmessage.Event
	for _, recordID := range []string{"77", "78"} {
		tx := &userServiceFakeTx{}
		store := &userServiceFakeUsers{id: 42, user: User{ID: "42", Nickname: "小芯"}, record: TestRecord{ID: recordID, ResultType: 9}}
		messages := &userServiceFakeMessages{created: true}

		if _, err := newTestRecordService(&userServiceFakeBeginner{tx: tx}, store, messages).SaveTestRecord(context.Background(), 42, validTestRecordInput(9)); err != nil {
			t.Fatalf("SaveTestRecord(%s) error = %v", recordID, err)
		}
		events = append(events, messages.event)
	}
	if events[0].BusinessID == events[1].BusinessID || events[0].BusinessID != "77" || events[1].BusinessID != "78" {
		t.Fatalf("message business ids = %q, %q, want distinct record ids", events[0].BusinessID, events[1].BusinessID)
	}
}

func TestMiniappTestRecordCommitFailureReturnsError(t *testing.T) {
	wantErr := errors.New("commit unavailable")
	tx := &userServiceFakeTx{commitErr: wantErr}
	store := &userServiceFakeUsers{id: 42, user: User{ID: "42"}, record: TestRecord{ID: "77", ResultType: 9}}

	_, err := newTestRecordService(&userServiceFakeBeginner{tx: tx}, store, &userServiceFakeMessages{}).SaveTestRecord(context.Background(), 42, validTestRecordInput(9))

	if !errors.Is(err, wantErr) || !strings.Contains(err.Error(), "commit miniapp test transaction") {
		t.Fatalf("error = %v, want wrapped commit error", err)
	}
	if tx.commitCalls != 1 || tx.rollbackCalls != 1 {
		t.Fatalf("commit=%d rollback=%d, want 1 and 1", tx.commitCalls, tx.rollbackCalls)
	}
}

func TestMiniappTestRecordCanceledContextRollsBack(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	tx := &userServiceFakeTx{}
	store := &userServiceFakeUsers{id: 42, waitInsert: true}

	_, err := newTestRecordService(&userServiceFakeBeginner{tx: tx}, store, &userServiceFakeMessages{}).SaveTestRecord(ctx, 42, validTestRecordInput(9))

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context canceled", err)
	}
	if tx.commitCalls != 0 || tx.rollbackCalls != 1 {
		t.Fatalf("commit=%d rollback=%d, want 0 and 1", tx.commitCalls, tx.rollbackCalls)
	}
}

func TestMiniappTestRecordRejectsInvalidInputBeforeTransaction(t *testing.T) {
	valid := validTestRecordInput(1)
	tests := []struct {
		name string
		uid  int64
		in   TestRecordInput
	}{
		{name: "invalid user", uid: 0, in: valid},
		{name: "invalid result", uid: 1, in: TestRecordInput{ResultType: 10, Score: json.RawMessage(`{}`), Centers: json.RawMessage(`[]`)}},
		{name: "invalid second", uid: 1, in: TestRecordInput{ResultType: 1, SecondType: -1, Score: json.RawMessage(`{}`), Centers: json.RawMessage(`[]`)}},
		{name: "same result and second", uid: 1, in: TestRecordInput{ResultType: 1, SecondType: 1, Score: json.RawMessage(`{}`), Centers: json.RawMessage(`[]`)}},
		{name: "missing score", uid: 1, in: TestRecordInput{ResultType: 1, Centers: json.RawMessage(`[]`)}},
		{name: "null score", uid: 1, in: TestRecordInput{ResultType: 1, Score: json.RawMessage(`null`), Centers: json.RawMessage(`[]`)}},
		{name: "array score", uid: 1, in: TestRecordInput{ResultType: 1, Score: json.RawMessage(`[]`), Centers: json.RawMessage(`[]`)}},
		{name: "invalid score json", uid: 1, in: TestRecordInput{ResultType: 1, Score: json.RawMessage(`{`), Centers: json.RawMessage(`[]`)}},
		{name: "conflicting score aliases", uid: 1, in: TestRecordInput{ResultType: 1, Score: json.RawMessage(`{"1":1}`), Scores: json.RawMessage(`{"1":2}`), Centers: json.RawMessage(`[]`)}},
		{name: "missing centers", uid: 1, in: TestRecordInput{ResultType: 1, Score: json.RawMessage(`{}`)}},
		{name: "null centers", uid: 1, in: TestRecordInput{ResultType: 1, Score: json.RawMessage(`{}`), Centers: json.RawMessage(`null`)}},
		{name: "object centers", uid: 1, in: TestRecordInput{ResultType: 1, Score: json.RawMessage(`{}`), Centers: json.RawMessage(`{}`)}},
		{name: "invalid centers json", uid: 1, in: TestRecordInput{ResultType: 1, Score: json.RawMessage(`{}`), Centers: json.RawMessage(`[`)}},
		{name: "score too large", uid: 1, in: TestRecordInput{ResultType: 1, Score: json.RawMessage(`{"value":"` + strings.Repeat("x", maxTestRecordJSONBytes) + `"}`), Centers: json.RawMessage(`[]`)}},
		{name: "centers too large", uid: 1, in: TestRecordInput{ResultType: 1, Score: json.RawMessage(`{}`), Centers: json.RawMessage(`["` + strings.Repeat("x", maxTestRecordJSONBytes) + `"]`)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			beginner := &userServiceFakeBeginner{tx: &userServiceFakeTx{}}
			store := &userServiceFakeUsers{id: tt.uid}

			_, err := newTestRecordService(beginner, store, &userServiceFakeMessages{}).SaveTestRecord(context.Background(), tt.uid, tt.in)

			if !errors.Is(err, ErrInvalidTestRecord) {
				t.Fatalf("error = %v, want ErrInvalidTestRecord", err)
			}
			if beginner.calls != 0 || store.insertCalls != 0 {
				t.Fatalf("invalid input reached transaction: begin=%d insert=%d", beginner.calls, store.insertCalls)
			}
		})
	}
}

func TestMiniappTestRecordStoreRejectsNilQueryTarget(t *testing.T) {
	store := &Store{}
	_, err := store.InsertTestRecord(context.Background(), nil, 1, validTestRecordInput(1))
	if !errors.Is(err, ErrNilDBTX) {
		t.Fatalf("InsertTestRecord() error = %v, want ErrNilDBTX", err)
	}
	if err := store.UpdateMainType(context.Background(), nil, 1, 1); !errors.Is(err, ErrNilDBTX) {
		t.Fatalf("UpdateMainType() error = %v, want ErrNilDBTX", err)
	}
}

func TestMiniappTestRecordRejectsUnconfiguredService(t *testing.T) {
	tests := []struct {
		name    string
		service *Service
	}{
		{name: "nil receiver", service: nil},
		{name: "missing beginner", service: NewService(nil, &userServiceFakeUsers{}, &userServiceFakeMessages{}, WithTestRecordWriter(&userServiceFakeUsers{}))},
		{name: "missing users", service: NewService(&userServiceFakeBeginner{tx: &userServiceFakeTx{}}, nil, &userServiceFakeMessages{}, WithTestRecordWriter(&userServiceFakeUsers{}))},
		{name: "missing test writer option", service: NewService(&userServiceFakeBeginner{tx: &userServiceFakeTx{}}, &userServiceFakeUsers{}, &userServiceFakeMessages{})},
		{name: "missing messages", service: NewService(&userServiceFakeBeginner{tx: &userServiceFakeTx{}}, &userServiceFakeUsers{}, nil, WithTestRecordWriter(&userServiceFakeUsers{}))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.service.SaveTestRecord(context.Background(), 1, validTestRecordInput(1))
			if !errors.Is(err, ErrServiceNotConfigured) {
				t.Fatalf("error = %v, want ErrServiceNotConfigured", err)
			}
		})
	}
}

func TestNormalizeScoresPrefersClientScoreAndSupportsLegacyScores(t *testing.T) {
	tests := []struct {
		name   string
		score  json.RawMessage
		scores json.RawMessage
		want   string
	}{
		{name: "client score", score: json.RawMessage(`{"9":18}`), want: `{"9":18}`},
		{name: "legacy scores", scores: json.RawMessage(`{"8":12}`), want: `{"8":12}`},
		{name: "matching aliases", score: json.RawMessage(`{"1":1,"2":2}`), scores: json.RawMessage(`{"2":2,"1":1}`), want: `{"1":1,"2":2}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeScores(tt.score, tt.scores)
			if err != nil || string(got) != tt.want {
				t.Fatalf("NormalizeScores() = %s, %v; want %s, nil", got, err, tt.want)
			}
		})
	}
}

func TestNormalizeScoresRejectsDifferentLargeIntegerAliases(t *testing.T) {
	_, err := NormalizeScores(
		json.RawMessage(`{"1":9007199254740992}`),
		json.RawMessage(`{"1":9007199254740993}`),
	)
	if !errors.Is(err, ErrInvalidTestRecord) {
		t.Fatalf("NormalizeScores() error = %v, want ErrInvalidTestRecord", err)
	}
}

type loginOnlyUsers struct{ inner *userServiceFakeUsers }

func (f loginOnlyUsers) UpsertByOpenIDWithDBTX(ctx context.Context, q dbtx.DBTX, openid, unionid, channel, scene string) (int64, bool, error) {
	return f.inner.UpsertByOpenIDWithDBTX(ctx, q, openid, unionid, channel, scene)
}

func (f loginOnlyUsers) GetUserWithDBTX(ctx context.Context, q dbtx.DBTX, id int64) (User, error) {
	return f.inner.GetUserWithDBTX(ctx, q, id)
}

func TestMiniappUserDoesNotRequireTestRecordDependency(t *testing.T) {
	tx := &userServiceFakeTx{}
	users := &userServiceFakeUsers{id: 42, created: false}

	id, err := NewService(&userServiceFakeBeginner{tx: tx}, loginOnlyUsers{inner: users}, &userServiceFakeMessages{}).UpsertUser(context.Background(), "openid", "", "", "")

	if err != nil || id != 42 {
		t.Fatalf("UpsertUser() = (%d, %v), want (42, nil)", id, err)
	}
}
