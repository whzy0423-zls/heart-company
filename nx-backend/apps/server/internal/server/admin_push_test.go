package server

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"nine-xing/nx-backend/apps/server/internal/push"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestAdminPushSendRejectsOversizedBody(t *testing.T) {
	body := `{"title":"` + strings.Repeat("a", 9000) + `","content":"test"}`
	req := httptest.NewRequest(http.MethodPost, "/api/push/send", strings.NewReader(body))
	res := httptest.NewRecorder()

	s := &Server{}
	s.adminPushSend(res, req)

	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected oversized push body to be rejected, got %d", res.Code)
	}
}

func TestAdminPushSendRejectsUnknownMemberLevel(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/push/send", strings.NewReader(`{
		"title":"会员推送",
		"content":"test",
		"targetType":"level",
		"targetValue":"gold"
	}`))
	res := httptest.NewRecorder()

	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("expected unknown member level to return 400, panicked: %v", recovered)
		}
	}()

	s := &Server{}
	s.adminPushSend(res, req)

	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected unknown member level to be rejected, got %d body=%s", res.Code, res.Body.String())
	}
}

func TestAdminPushSendReturnsBeforePusherFinishesAndWorkerMarksSuccess(t *testing.T) {
	fixture := newAdminPushAsyncFixture([]adminPushAsyncDevice{{id: 1, registrationID: "reg-1"}})
	database := openAdminPushAsyncDB(t, fixture)
	blocking := newBlockingPusher()
	s := &Server{pushStore: push.NewStore(database, blocking)}
	req := httptest.NewRequest(http.MethodPost, "/api/push/send", strings.NewReader(`{
		"title":"异步推送",
		"content":"不要等待 JPush",
		"deepLink":"/daily"
	}`))
	res := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		s.adminPushSend(res, req)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(150 * time.Millisecond):
		blocking.release()
		<-done
		t.Fatalf("send handler waited for pusher to finish")
	}

	if res.Code != http.StatusOK {
		blocking.release()
		t.Fatalf("expected 200, got %d body=%s", res.Code, res.Body.String())
	}
	body := decodeAdminPushSendResponse(t, res.Body.Bytes())
	if body.Data.RecordID != fixture.recordID {
		blocking.release()
		t.Fatalf("expected recordId %d, got %d body=%s", fixture.recordID, body.Data.RecordID, res.Body.String())
	}
	if body.Data.Status != "pending" && body.Data.Status != "sending" {
		blocking.release()
		t.Fatalf("expected response status pending/sending, got %q body=%s", body.Data.Status, res.Body.String())
	}
	if body.Data.Message == "" {
		blocking.release()
		t.Fatalf("expected response message, body=%s", res.Body.String())
	}
	created := fixture.createdRecord()
	if created.title != "异步推送" || created.content != "不要等待 JPush" || created.deepLink != "/daily" {
		blocking.release()
		t.Fatalf("unexpected created record: %+v", created)
	}

	select {
	case <-blocking.called:
	case <-time.After(time.Second):
		blocking.release()
		t.Fatal("expected worker to call pusher")
	}
	blocking.release()

	updates := fixture.waitForStatuses(t, "sending", "success")
	success := updates[len(updates)-1]
	if success.sentCount != 1 || success.errorMessage != "" {
		t.Fatalf("expected success update with sentCount=1 and empty error, got %+v", success)
	}
}

func TestAdminPushSendWorkerMarksFailedWhenPusherErrors(t *testing.T) {
	fixture := newAdminPushAsyncFixture([]adminPushAsyncDevice{{id: 1, registrationID: "reg-1"}})
	database := openAdminPushAsyncDB(t, fixture)
	s := &Server{pushStore: push.NewStore(database, erringPusher{err: errors.New("jpush boom")})}
	req := httptest.NewRequest(http.MethodPost, "/api/push/send", strings.NewReader(`{
		"title":"失败推送",
		"content":"写入失败原因"
	}`))
	res := httptest.NewRecorder()

	s.adminPushSend(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected async send request to return 200, got %d body=%s", res.Code, res.Body.String())
	}
	body := decodeAdminPushSendResponse(t, res.Body.Bytes())
	if body.Data.RecordID != fixture.recordID {
		t.Fatalf("expected recordId %d, got %d", fixture.recordID, body.Data.RecordID)
	}

	updates := fixture.waitForStatuses(t, "sending", "failed")
	failed := updates[len(updates)-1]
	if failed.sentCount != 0 || !strings.Contains(failed.errorMessage, "jpush boom") {
		t.Fatalf("expected failed update with fail reason, got %+v", failed)
	}
}

func TestAdminPushSendWorkerMarksFailedWhenNoAudienceWithoutCallingPusher(t *testing.T) {
	fixture := newAdminPushAsyncFixture(nil)
	database := openAdminPushAsyncDB(t, fixture)
	pusher := newFailOnCallPusher()
	s := &Server{pushStore: push.NewStore(database, pusher)}
	req := httptest.NewRequest(http.MethodPost, "/api/push/send", strings.NewReader(`{
		"title":"空受众",
		"content":"没有设备"
	}`))
	res := httptest.NewRecorder()

	s.adminPushSend(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected async send request to return 200, got %d body=%s", res.Code, res.Body.String())
	}
	body := decodeAdminPushSendResponse(t, res.Body.Bytes())
	if body.Data.RecordID != fixture.recordID {
		t.Fatalf("expected recordId %d, got %d", fixture.recordID, body.Data.RecordID)
	}

	updates := fixture.waitForStatuses(t, "sending", "failed")
	failed := updates[len(updates)-1]
	if failed.sentCount != 0 || failed.errorMessage != "无推送目标" {
		t.Fatalf("expected no-audience failed update, got %+v", failed)
	}
	select {
	case <-pusher.called:
		t.Fatal("pusher must not be called when no registration IDs exist")
	default:
	}
}

func TestAdminPushSendWorkerPersistsFailureAfterWorkerContextTimeout(t *testing.T) {
	fixture := newAdminPushAsyncFixture([]adminPushAsyncDevice{{id: 1, registrationID: "reg-1"}})
	fixture.failCanceledExec = true
	database := openAdminPushAsyncDB(t, fixture)
	s := &Server{
		pushStore:       push.NewStore(database, waitForContextDonePusher{}),
		pushSendTimeout: 5 * time.Millisecond,
	}

	s.runAdminPushSendTask(adminPushSendTask{
		recordID:   fixture.recordID,
		title:      "超时推送",
		content:    "最终状态仍要落库",
		targetType: "all",
		audit:      adminPushAuditMeta{operatorName: "admin"},
	})

	updates := fixture.updatesSnapshot()
	if len(updates) == 0 {
		t.Fatal("expected status updates")
	}
	final := updates[len(updates)-1]
	if final.status != "failed" || !strings.Contains(final.errorMessage, "context deadline exceeded") {
		t.Fatalf("expected final failed timeout status to persist, got updates=%+v", updates)
	}
}

func TestAdminPushRecoveryIntervalRunsBeforeSendingTimeoutExpires(t *testing.T) {
	s := &Server{pushSendTimeout: 10 * time.Second}

	interval := s.adminPushRecoveryInterval()

	if interval <= 0 || interval >= s.adminPushSendTimeout() {
		t.Fatalf("expected recovery interval to run before timeout expires, got interval=%s timeout=%s", interval, s.adminPushSendTimeout())
	}
}

func TestAdminPushAudienceCountRejectsUnknownMemberLevel(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/push/audience-count?targetType=level&targetValue=gold", nil)
	res := httptest.NewRecorder()

	s := &Server{}
	s.adminPushAudienceCount(res, req)

	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected unknown member level to be rejected, got %d body=%s", res.Code, res.Body.String())
	}
}

func TestAdminPushAudienceCountReturnsNormalizedTargetDeviceAndUserCount(t *testing.T) {
	database, err := sql.Open("admin_push_count_test", "")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	s := &Server{pushStore: push.NewStore(database, push.NoopPusher{})}
	req := httptest.NewRequest(http.MethodGet, "/api/push/audience-count?targetType=level&targetValue=vip", nil)
	res := httptest.NewRecorder()

	s.adminPushAudienceCount(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", res.Code, res.Body.String())
	}
	var body struct {
		Data struct {
			DeviceCount int64  `json:"deviceCount"`
			TargetType  string `json:"targetType"`
			TargetValue string `json:"targetValue"`
			UserCount   int64  `json:"userCount"`
		} `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Data.DeviceCount != 0 || body.Data.UserCount != 0 {
		t.Fatalf("expected zero counts from fixture, got %+v", body.Data)
	}
	if body.Data.TargetType != "level" || body.Data.TargetValue != "vip" {
		t.Fatalf("expected normalized target echo, got %+v", body.Data)
	}
}

func TestAdminPushAudienceCountReturnsServiceUnavailableWhenStoreMissing(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/push/audience-count", nil)
	res := httptest.NewRecorder()

	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("expected missing push store to return 500, panicked: %v", recovered)
		}
	}()

	s := &Server{}
	s.adminPushAudienceCount(res, req)

	if res.Code != http.StatusInternalServerError {
		t.Fatalf("expected missing push store to return 500, got %d body=%s", res.Code, res.Body.String())
	}
}

func init() {
	sql.Register("admin_push_count_test", adminPushCountDriver{})
}

type adminPushCountDriver struct{}

func (adminPushCountDriver) Open(string) (driver.Conn, error) {
	return adminPushCountConn{}, nil
}

type adminPushCountConn struct{}

func (adminPushCountConn) Prepare(string) (driver.Stmt, error) {
	return nil, nil
}

func (adminPushCountConn) Close() error {
	return nil
}

func (adminPushCountConn) Begin() (driver.Tx, error) {
	return nil, nil
}

func (adminPushCountConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	return &adminPushCountRows{columns: []string{"device_count", "user_count"}, values: []driver.Value{int64(0), int64(0)}}, nil
}

type adminPushCountRows struct {
	columns []string
	values  []driver.Value
	done    bool
}

func (r *adminPushCountRows) Columns() []string { return r.columns }

func (r *adminPushCountRows) Close() error { return nil }

func (r *adminPushCountRows) Next(dest []driver.Value) error {
	if r.done {
		return io.EOF
	}
	copy(dest, r.values)
	r.done = true
	return nil
}

func init() {
	sql.Register("push_store_test", serverPushStoreTestDriver{})
}

type serverPushStoreTestDriver struct{}

func (serverPushStoreTestDriver) Open(string) (driver.Conn, error) {
	return serverPushStoreTestConn{}, nil
}

type serverPushStoreTestConn struct{}

func (serverPushStoreTestConn) Prepare(string) (driver.Stmt, error) { return nil, nil }
func (serverPushStoreTestConn) Close() error                        { return nil }
func (serverPushStoreTestConn) Begin() (driver.Tx, error)           { return nil, nil }
func (serverPushStoreTestConn) ExecContext(context.Context, string, []driver.NamedValue) (driver.Result, error) {
	return driver.RowsAffected(1), nil
}
func (serverPushStoreTestConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	if strings.Contains(query, "COUNT(DISTINCT dt.registration_id)") {
		return &serverPushStoreOneRow{columns: []string{"device_count", "user_count"}, values: []driver.Value{int64(0), int64(0)}}, nil
	}
	return serverPushStoreEmptyRows{}, nil
}

type serverPushStoreEmptyRows struct{}

func (serverPushStoreEmptyRows) Columns() []string         { return nil }
func (serverPushStoreEmptyRows) Close() error              { return nil }
func (serverPushStoreEmptyRows) Next([]driver.Value) error { return io.EOF }

type serverPushStoreOneRow struct {
	columns []string
	values  []driver.Value
	done    bool
}

func (r *serverPushStoreOneRow) Columns() []string { return r.columns }
func (r *serverPushStoreOneRow) Close() error      { return nil }
func (r *serverPushStoreOneRow) Next(dest []driver.Value) error {
	if r.done {
		return io.EOF
	}
	copy(dest, r.values)
	r.done = true
	return nil
}

var _ driver.ExecerContext = serverPushStoreTestConn{}
var _ driver.QueryerContext = serverPushStoreTestConn{}

type adminPushSendResponse struct {
	Code int `json:"code"`
	Data struct {
		RecordID int64  `json:"recordId"`
		Status   string `json:"status"`
		Message  string `json:"message"`
	} `json:"data"`
}

func decodeAdminPushSendResponse(t *testing.T, raw []byte) adminPushSendResponse {
	t.Helper()
	var body adminPushSendResponse
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode response: %v body=%s", err, string(raw))
	}
	if body.Code != 0 {
		t.Fatalf("expected code=0, got %+v body=%s", body, string(raw))
	}
	return body
}

type blockingPusher struct {
	called      chan struct{}
	released    chan struct{}
	calledOnce  sync.Once
	releaseOnce sync.Once
}

func newBlockingPusher() *blockingPusher {
	return &blockingPusher{called: make(chan struct{}), released: make(chan struct{})}
}

func (p *blockingPusher) Push(ctx context.Context, registrationIDs []string, _ push.Message) (push.PushResult, error) {
	p.calledOnce.Do(func() { close(p.called) })
	select {
	case <-p.released:
		return push.PushResult{MsgID: "msg-1", Sent: len(registrationIDs)}, nil
	case <-ctx.Done():
		return push.PushResult{}, ctx.Err()
	}
}

func (p *blockingPusher) release() {
	p.releaseOnce.Do(func() { close(p.released) })
}

type erringPusher struct {
	err error
}

func (p erringPusher) Push(context.Context, []string, push.Message) (push.PushResult, error) {
	return push.PushResult{}, p.err
}

type failOnCallPusher struct {
	called chan struct{}
	once   sync.Once
}

func newFailOnCallPusher() *failOnCallPusher {
	return &failOnCallPusher{called: make(chan struct{})}
}

func (p *failOnCallPusher) Push(context.Context, []string, push.Message) (push.PushResult, error) {
	p.once.Do(func() { close(p.called) })
	return push.PushResult{}, errors.New("unexpected pusher call")
}

type waitForContextDonePusher struct{}

func (waitForContextDonePusher) Push(ctx context.Context, _ []string, _ push.Message) (push.PushResult, error) {
	<-ctx.Done()
	return push.PushResult{}, ctx.Err()
}

type adminPushAsyncDevice struct {
	id             int64
	registrationID string
}

type adminPushAsyncCreated struct {
	title       string
	content     string
	targetType  string
	targetValue string
	deepLink    string
	operator    string
}

type adminPushAsyncUpdate struct {
	id           int64
	status       string
	sentCount    int
	errorMessage string
}

type adminPushAsyncFixture struct {
	mu               sync.Mutex
	recordID         int64
	devices          []adminPushAsyncDevice
	created          []adminPushAsyncCreated
	updates          []adminPushAsyncUpdate
	updateC          chan adminPushAsyncUpdate
	failCanceledExec bool
}

func newAdminPushAsyncFixture(devices []adminPushAsyncDevice) *adminPushAsyncFixture {
	return &adminPushAsyncFixture{
		recordID: 101,
		devices:  append([]adminPushAsyncDevice(nil), devices...),
		updateC:  make(chan adminPushAsyncUpdate, 10),
	}
}

func (f *adminPushAsyncFixture) addCreated(args []driver.NamedValue) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.created = append(f.created, adminPushAsyncCreated{
		title:       namedString(args, 0),
		content:     namedString(args, 1),
		targetType:  namedString(args, 2),
		targetValue: namedString(args, 3),
		deepLink:    namedString(args, 4),
		operator:    namedString(args, 5),
	})
}

func (f *adminPushAsyncFixture) createdRecord() adminPushAsyncCreated {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.created) == 0 {
		return adminPushAsyncCreated{}
	}
	return f.created[len(f.created)-1]
}

func (f *adminPushAsyncFixture) addUpdate(args []driver.NamedValue) {
	update := adminPushAsyncUpdate{
		id:           namedInt64(args, 0),
		status:       namedString(args, 1),
		sentCount:    int(namedInt64(args, 2)),
		errorMessage: namedString(args, 3),
	}
	f.mu.Lock()
	f.updates = append(f.updates, update)
	f.mu.Unlock()
	select {
	case f.updateC <- update:
	default:
	}
}

func (f *adminPushAsyncFixture) waitForStatuses(t *testing.T, statuses ...string) []adminPushAsyncUpdate {
	t.Helper()
	deadline := time.After(2 * time.Second)
	seen := make([]adminPushAsyncUpdate, 0, len(statuses))
	for len(seen) < len(statuses) {
		select {
		case update := <-f.updateC:
			if update.status == statuses[len(seen)] {
				seen = append(seen, update)
			}
		case <-deadline:
			f.mu.Lock()
			all := append([]adminPushAsyncUpdate(nil), f.updates...)
			f.mu.Unlock()
			t.Fatalf("timed out waiting for statuses %v after seeing %v", statuses, all)
		}
	}
	return seen
}

func (f *adminPushAsyncFixture) updatesSnapshot() []adminPushAsyncUpdate {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]adminPushAsyncUpdate(nil), f.updates...)
}

func (f *adminPushAsyncFixture) deviceRowsAfter(lastID int64, limit int64) [][]driver.Value {
	f.mu.Lock()
	defer f.mu.Unlock()
	rows := [][]driver.Value{}
	for _, device := range f.devices {
		if device.id <= lastID {
			continue
		}
		rows = append(rows, []driver.Value{device.id, device.registrationID})
		if limit > 0 && int64(len(rows)) >= limit {
			break
		}
	}
	return rows
}

var adminPushAsyncFixtures sync.Map

func init() {
	sql.Register("admin_push_async_test", adminPushAsyncDriver{})
}

func openAdminPushAsyncDB(t *testing.T, fixture *adminPushAsyncFixture) *sql.DB {
	t.Helper()
	dsn := strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	adminPushAsyncFixtures.Store(dsn, fixture)
	t.Cleanup(func() { adminPushAsyncFixtures.Delete(dsn) })
	database, err := sql.Open("admin_push_async_test", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database
}

type adminPushAsyncDriver struct{}

func (adminPushAsyncDriver) Open(name string) (driver.Conn, error) {
	value, ok := adminPushAsyncFixtures.Load(name)
	if !ok {
		return nil, fmt.Errorf("unknown admin push async fixture %q", name)
	}
	return &adminPushAsyncConn{fixture: value.(*adminPushAsyncFixture)}, nil
}

type adminPushAsyncConn struct {
	fixture *adminPushAsyncFixture
}

func (c *adminPushAsyncConn) Prepare(string) (driver.Stmt, error) {
	return nil, nil
}

func (c *adminPushAsyncConn) Close() error {
	return nil
}

func (c *adminPushAsyncConn) Begin() (driver.Tx, error) {
	return nil, nil
}

func (c *adminPushAsyncConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	if c.fixture.failCanceledExec && ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if strings.Contains(query, "UPDATE push_notifications") {
		c.fixture.addUpdate(args)
	}
	return driver.RowsAffected(1), nil
}

func (c *adminPushAsyncConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if strings.Contains(query, "INSERT INTO push_notifications") {
		c.fixture.addCreated(args)
		return &adminPushAsyncRows{
			columns: []string{"id"},
			values:  [][]driver.Value{{c.fixture.recordID}},
		}, nil
	}
	if strings.Contains(query, "FROM app_device_tokens") {
		lastID := namedInt64(args, 0)
		limit := namedInt64(args, 1)
		if strings.Contains(query, "u.member_level = $1") {
			lastID = namedInt64(args, 1)
			limit = namedInt64(args, 2)
		}
		return &adminPushAsyncRows{
			columns: []string{"id", "registration_id"},
			values:  c.fixture.deviceRowsAfter(lastID, limit),
		}, nil
	}
	return &adminPushAsyncRows{}, nil
}

type adminPushAsyncRows struct {
	columns []string
	values  [][]driver.Value
	index   int
}

func (r *adminPushAsyncRows) Columns() []string {
	return r.columns
}

func (r *adminPushAsyncRows) Close() error {
	return nil
}

func (r *adminPushAsyncRows) Next(dest []driver.Value) error {
	if r.index >= len(r.values) {
		return io.EOF
	}
	copy(dest, r.values[r.index])
	r.index++
	return nil
}

func namedString(args []driver.NamedValue, index int) string {
	if index >= len(args) {
		return ""
	}
	value, _ := args[index].Value.(string)
	return value
}

func namedInt64(args []driver.NamedValue, index int) int64 {
	if index >= len(args) {
		return 0
	}
	switch value := args[index].Value.(type) {
	case int64:
		return value
	case int:
		return int64(value)
	default:
		return 0
	}
}

var _ driver.ExecerContext = (*adminPushAsyncConn)(nil)
var _ driver.QueryerContext = (*adminPushAsyncConn)(nil)
