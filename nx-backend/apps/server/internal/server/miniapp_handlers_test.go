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
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/auth"
	"nine-xing/nx-backend/apps/server/internal/config"
	"nine-xing/nx-backend/apps/server/internal/miniapp"
	"nine-xing/nx-backend/apps/server/internal/rag"
	"nine-xing/nx-backend/apps/server/internal/signup"
	"nine-xing/nx-backend/apps/server/internal/wechat"
)

func TestMergeAppRAGDocumentsIncludesKnowledgeStore(t *testing.T) {
	docs := mergeAppRAGDocuments(
		[]rag.Document{{ID: "type-1", Title: "1号", Content: "原则"}},
		[]rag.Document{{ID: "kb-8", Title: "课程答疑", Content: "课程安排"}},
	)
	if len(docs) != 2 {
		t.Fatalf("expected site and knowledge documents, got %+v", docs)
	}
	if docs[0].ID != "type-1" || docs[1].ID != "kb-8" {
		t.Fatalf("unexpected document order: %+v", docs)
	}
}

func TestMiniappSiteRAGDocumentsExcludeKnowledgeStore(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "site-config.json")
	if err := os.WriteFile(configPath, []byte(miniappRAGTestConfig), 0o644); err != nil {
		t.Fatal(err)
	}

	server := &Server{
		env:      config.Env{SiteConfig: configPath},
		ragCache: newMiniappRAGCache(time.Minute),
		ragDocs: &fakeRAGDocumentStore{
			enabledDocs: []rag.Document{{ID: "kb-8", Title: "后台知识库", Content: "公共知识库内容"}},
		},
	}

	docs, err := server.miniappRAGDocuments(context.Background())
	if err != nil {
		t.Fatalf("miniappRAGDocuments returned error: %v", err)
	}
	foundSite := false
	foundKnowledge := false
	for _, doc := range docs {
		switch doc.ID {
		case "type-1":
			foundSite = true
		case "kb-8":
			foundKnowledge = true
		}
	}
	if !foundSite || foundKnowledge {
		t.Fatalf("expected site document and no public knowledge documents, foundSite=%v foundKnowledge=%v docs=%+v", foundSite, foundKnowledge, docs)
	}
}

const miniappRAGTestConfig = `{
  "site": {
    "brandName": "九型星球",
    "logo": "/logo.png"
  },
  "navigation": {
    "main": [{"label": "首页", "to": "/"}]
  },
  "home": {},
  "types": [
    {
      "id": "1",
      "name": "完美型",
      "avatar": "/type-1.png",
      "description": "完美型重视原则。",
      "keywords": "原则 自律 改进"
    }
  ]
}`

func TestAppKnowledgeDocumentsAreSearchable(t *testing.T) {
	service := rag.NewService(mergeAppRAGDocuments(
		[]rag.Document{{ID: "type-1", Title: "1号 完美型", Content: "完美型重视原则。", Tags: []string{"完美型"}}},
		[]rag.Document{{ID: "kb-8", Title: "企业沟通课", Content: "企业沟通课适合团队冲突复盘和管理者沟通训练。", Tags: []string{"企业", "沟通"}}},
	))

	answer, err := service.Ask(nil, rag.AskInput{Question: "企业沟通课适合什么场景？"})
	if err != nil {
		t.Fatalf("Ask returned error: %v", err)
	}
	if len(answer.Sources) == 0 || answer.Sources[0].ID != "kb-8" {
		t.Fatalf("expected knowledge document source, got %+v", answer.Sources)
	}
}

type wxLoginUserServiceFake struct {
	id      int64
	err     error
	calls   int
	openid  string
	unionid string
	channel string
	scene   string
}

func (f *wxLoginUserServiceFake) UpsertUser(_ context.Context, openid, unionid, channel, scene string) (int64, error) {
	f.calls++
	f.openid = openid
	f.unionid = unionid
	f.channel = channel
	f.scene = scene
	return f.id, f.err
}

var wxLoginReadDriverOnce sync.Once

type wxLoginReadDriver struct{}
type wxLoginReadConn struct{}
type wxLoginReadRows struct {
	done bool
}

func (wxLoginReadDriver) Open(string) (driver.Conn, error) { return wxLoginReadConn{}, nil }
func (wxLoginReadConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("prepare unavailable")
}
func (wxLoginReadConn) Close() error              { return nil }
func (wxLoginReadConn) Begin() (driver.Tx, error) { return nil, errors.New("transactions unavailable") }
func (wxLoginReadConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	if !strings.Contains(query, "SELECT id, nickname, avatar") {
		return nil, errors.New("unexpected non-read miniapp query")
	}
	return &wxLoginReadRows{}, nil
}
func (*wxLoginReadRows) Columns() []string {
	return []string{"id", "nickname", "avatar", "phone", "gender", "main_type", "member_level", "create_time"}
}
func (*wxLoginReadRows) Close() error { return nil }
func (r *wxLoginReadRows) Next(dest []driver.Value) error {
	if r.done {
		return io.EOF
	}
	r.done = true
	dest[0] = int64(42)
	dest[1] = "小芯"
	dest[2] = "https://example.com/avatar.png"
	dest[3] = ""
	dest[4] = ""
	dest[5] = int64(0)
	dest[6] = int64(0)
	dest[7] = time.Unix(1_700_000_000, 0)
	return nil
}

func openWxLoginReadDB(t *testing.T) *sql.DB {
	t.Helper()
	const driverName = "nine_xing_wx_login_read"
	wxLoginReadDriverOnce.Do(func() { sql.Register(driverName, wxLoginReadDriver{}) })
	database, err := sql.Open(driverName, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database
}

func TestWxLoginUsesMiniappUserServiceAndKeepsResponseCompatible(t *testing.T) {
	service := &wxLoginUserServiceFake{id: 42}
	s := &Server{
		env:            config.Env{JWTSecret: "test-secret"},
		miniapp:        miniapp.NewStore(openWxLoginReadDB(t)),
		miniappService: service,
		wx:             wechat.NewClient("", "", true),
	}

	response := performRawUnit(http.HandlerFunc(s.wxLogin), http.MethodPost, "/api/wx/login", `{"code":"login-code","channel":"campaign","scene":"1001"}`)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
	if service.calls != 1 || !strings.HasPrefix(service.openid, "dev_") || service.unionid != "" || service.channel != "campaign" || service.scene != "1001" {
		t.Fatalf("unexpected service call: %+v", service)
	}
	var envelope struct {
		Data struct {
			AccessToken string       `json:"accessToken"`
			User        miniapp.User `json:"user"`
			DevMode     bool         `json:"devMode"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Data.AccessToken == "" || envelope.Data.User.ID != "42" || envelope.Data.User.Nickname != "小芯" || !envelope.Data.DevMode {
		t.Fatalf("incompatible login response: %+v", envelope.Data)
	}
}

func TestWxLoginStopsWhenMiniappUserTransactionFails(t *testing.T) {
	service := &wxLoginUserServiceFake{err: errors.New("transaction failed")}
	s := &Server{
		env:            config.Env{JWTSecret: "test-secret"},
		miniapp:        miniapp.NewStore(openWxLoginReadDB(t)),
		miniappService: service,
		wx:             wechat.NewClient("", "", true),
	}

	response := performRawUnit(http.HandlerFunc(s.wxLogin), http.MethodPost, "/api/wx/login", `{"code":"login-code"}`)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
	if service.calls != 1 {
		t.Fatalf("service calls = %d, want 1", service.calls)
	}
}

func TestWxLoginReturnsBadRequestForInvalidChannelOrScene(t *testing.T) {
	tests := []struct {
		name    string
		payload string
	}{
		{name: "invalid channel", payload: `{"code":"login-code","channel":"bad\u0000channel","scene":"1001"}`},
		{name: "invalid scene", payload: `{"code":"login-code","channel":"campaign","scene":"bad\u007fscene"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := &wxLoginUserServiceFake{err: miniapp.ErrInvalidUserSource}
			s := &Server{
				env:            config.Env{JWTSecret: "test-secret"},
				miniapp:        miniapp.NewStore(openWxLoginReadDB(t)),
				miniappService: service,
				wx:             wechat.NewClient("", "", true),
			}

			response := performRawUnit(http.HandlerFunc(s.wxLogin), http.MethodPost, "/api/wx/login", tt.payload)

			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
			}
		})
	}
}

type wxProfileUpdateState struct {
	nickname string
	avatar   string
	userID   int64
}

type wxProfileUpdateDriver struct{ state *wxProfileUpdateState }
type wxProfileUpdateConn struct{ state *wxProfileUpdateState }

func (d wxProfileUpdateDriver) Open(string) (driver.Conn, error) {
	return &wxProfileUpdateConn{state: d.state}, nil
}
func (*wxProfileUpdateConn) Prepare(string) (driver.Stmt, error) { return nil, driver.ErrSkip }
func (*wxProfileUpdateConn) Close() error                        { return nil }
func (*wxProfileUpdateConn) Begin() (driver.Tx, error)           { return nil, driver.ErrSkip }
func (c *wxProfileUpdateConn) ExecContext(_ context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	if !strings.Contains(query, "UPDATE wx_users SET nickname=$1, avatar=$2 WHERE id=$3") || len(args) != 3 {
		return nil, fmt.Errorf("unexpected profile update: %s %+v", query, args)
	}
	c.state.nickname = fmt.Sprint(args[0].Value)
	c.state.avatar = fmt.Sprint(args[1].Value)
	c.state.userID, _ = args[2].Value.(int64)
	return driver.RowsAffected(1), nil
}
func (c *wxProfileUpdateConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	if !strings.Contains(query, "SELECT id, nickname, avatar") {
		return nil, fmt.Errorf("unexpected profile query: %s", query)
	}
	return &classroomRows{cols: []string{"id", "nickname", "avatar", "phone", "gender", "main_type", "member_level", "create_time"}, values: [][]driver.Value{{int64(42), c.state.nickname, c.state.avatar, "", "", int64(1), int64(0), time.Unix(1_700_000_000, 0)}}}, nil
}

func TestWxUserInfoPutPersistsProfileAndReturnsUpdatedDTO(t *testing.T) {
	state := &wxProfileUpdateState{nickname: "旧昵称"}
	driverName := fmt.Sprintf("wx-profile-update-%d", time.Now().UnixNano())
	sql.Register(driverName, wxProfileUpdateDriver{state: state})
	database, err := sql.Open(driverName, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	s := &Server{miniapp: miniapp.NewStore(database)}
	request := httptest.NewRequest(http.MethodPut, "/api/wx/userinfo", strings.NewReader(`{"nickname":"新昵称","avatar":"https://avatar.example/new.png"}`))
	request = request.WithContext(withUser(request.Context(), auth.UserInfo{ID: 42, Roles: []string{miniappRole}}))
	response := httptest.NewRecorder()
	s.wxUserInfo(response, request)
	if response.Code != http.StatusOK || state.userID != 42 || state.nickname != "新昵称" || state.avatar != "https://avatar.example/new.png" {
		t.Fatalf("profile update status=%d state=%+v body=%s", response.Code, state, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"nickname":"新昵称"`) || !strings.Contains(response.Body.String(), `"avatar":"https://avatar.example/new.png"`) {
		t.Fatalf("updated profile DTO missing: %s", response.Body.String())
	}
}

type miniappTestRecorderFake struct {
	record miniapp.TestRecord
	err    error
	calls  int
	uid    int64
	input  miniapp.TestRecordInput
}

func (f *miniappTestRecorderFake) SaveTestRecord(_ context.Context, uid int64, in miniapp.TestRecordInput) (miniapp.TestRecord, error) {
	f.calls++
	f.uid = uid
	f.input = in
	return f.record, f.err
}

func performMiniappTestRecordPost(s *Server, payload string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/api/miniapp/test-records", strings.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	request = request.WithContext(withUser(request.Context(), auth.UserInfo{ID: 42, Roles: []string{miniappRole}}))
	response := httptest.NewRecorder()
	s.miniappTestRecords(response, request)
	return response
}

func TestMiniappTestRecordsPostUsesTransactionalServiceAndKeepsResponseCompatible(t *testing.T) {
	service := &miniappTestRecorderFake{record: miniapp.TestRecord{
		ID:         "77",
		Gender:     "female",
		ResultType: 9,
		SecondType: 1,
		Scores:     json.RawMessage(`{"9":18}`),
		Centers:    json.RawMessage(`[]`),
		CreateTime: "2026/07/23 12:00:00",
	}}
	s := &Server{miniappTestService: service}

	response := performMiniappTestRecordPost(s, `{"gender":"female","resultType":9,"secondType":1,"score":{"9":18},"centers":[]}`)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
	if service.calls != 1 || service.uid != 42 || service.input.ResultType != 9 || service.input.SecondType != 1 || string(service.input.Score) != `{"9":18}` || len(service.input.Scores) != 0 {
		t.Fatalf("unexpected service call: %+v", service)
	}
	var envelope struct {
		Data miniapp.TestRecord `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Data.ID != "77" || envelope.Data.ResultType != 9 {
		t.Fatalf("incompatible response: %+v", envelope.Data)
	}
}

func TestMiniappTestRecordsPostMapsValidationAndInternalErrors(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantBody   string
		secret     string
	}{
		{name: "validation", err: fmt.Errorf("unsafe validation details: %w", miniapp.ErrInvalidTestRecord), wantStatus: http.StatusBadRequest, wantBody: "测评数据格式不正确", secret: "unsafe validation details"},
		{name: "internal sentinel", err: miniapp.ErrServiceNotConfigured, wantStatus: http.StatusInternalServerError, wantBody: "测评提交失败，请稍后重试", secret: miniapp.ErrServiceNotConfigured.Error()},
		{name: "database error", err: errors.New("database password=super-secret unavailable"), wantStatus: http.StatusInternalServerError, wantBody: "测评提交失败，请稍后重试", secret: "super-secret"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := &miniappTestRecorderFake{err: tt.err}
			response := performMiniappTestRecordPost(&Server{miniappTestService: service}, `{"resultType":9,"score":{},"centers":[]}`)
			if response.Code != tt.wantStatus {
				t.Fatalf("status = %d body=%s, want %d", response.Code, response.Body.String(), tt.wantStatus)
			}
			if !strings.Contains(response.Body.String(), tt.wantBody) || strings.Contains(response.Body.String(), tt.secret) {
				t.Fatalf("unsafe response body = %s, want %q without %q", response.Body.String(), tt.wantBody, tt.secret)
			}
		})
	}
}

func TestMiniappTestRecordsPostRejectsMalformedTrailingAndOversizedJSON(t *testing.T) {
	tests := []struct {
		name    string
		payload string
	}{
		{name: "malformed", payload: `{"resultType":`},
		{name: "trailing", payload: `{"resultType":9} trailing-garbage`},
		{name: "multiple values", payload: `{"resultType":9}{"resultType":8}`},
		{name: "oversized", payload: `{"resultType":9,"score":{"value":"` + strings.Repeat("x", 132*1024) + `"},"centers":[]}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := &miniappTestRecorderFake{record: miniapp.TestRecord{ID: "77", ResultType: 9}}
			response := performMiniappTestRecordPost(&Server{miniappTestService: service}, tt.payload)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d body=%s, want 400", response.Code, response.Body.String())
			}
			if service.calls != 0 {
				t.Fatalf("invalid JSON reached service %d times", service.calls)
			}
		})
	}
}

type miniappBookingCreatorFake struct {
	result  miniapp.BookingResult
	err     error
	calls   int
	uid     int64
	input   miniapp.BookingInput
	request *http.Request
}

func (f *miniappBookingCreatorFake) CreateBooking(_ context.Context, uid int64, input miniapp.BookingInput, request *http.Request) (miniapp.BookingResult, error) {
	f.calls++
	f.uid, f.input, f.request = uid, input, request
	return f.result, f.err
}

func performMiniappBookingPost(s *Server, payload string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/api/miniapp/bookings", strings.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	request = request.WithContext(withUser(request.Context(), auth.UserInfo{ID: 42, Roles: []string{miniappRole}}))
	response := httptest.NewRecorder()
	s.miniappBookings(response, request)
	return response
}

func TestMiniappBookingsPostUsesTransactionalServiceBroadcastsAfterSuccessAndReturnsBooking(t *testing.T) {
	lead := signup.Lead{ID: "91", Name: "张三", SourcePlatform: "miniapp"}
	booking := miniapp.Booking{ID: "73", SignupID: "91", Kind: "consult", ContactName: "张三", Phone: "13812345678", Status: "pending"}
	service := &miniappBookingCreatorFake{result: miniapp.BookingResult{Booking: booking, Lead: lead}}
	subscriber := make(chan signup.Lead, 1)
	s := &Server{
		miniappBookingService: service,
		signupSubscribers:     map[chan signup.Lead]struct{}{subscriber: {}},
	}

	response := performMiniappBookingPost(s, `{"kind":"consult","contactName":"张三","phone":"13812345678"}`)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
	if service.calls != 1 || service.uid != 42 || service.input.ContactName != "张三" || service.request == nil {
		t.Fatalf("unexpected service call: %+v", service)
	}
	select {
	case got := <-subscriber:
		if got.ID != lead.ID {
			t.Fatalf("broadcast lead = %+v, want %+v", got, lead)
		}
	default:
		t.Fatal("successful committed booking was not broadcast")
	}
	var envelope struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Data["id"] != "73" || envelope.Data["signupId"] != "91" || envelope.Data["contactName"] != "张三" {
		t.Fatalf("incompatible booking response = %+v", envelope.Data)
	}
	if _, exists := envelope.Data["lead"]; exists {
		t.Fatalf("response leaked internal lead result: %+v", envelope.Data)
	}
	if _, exists := envelope.Data["booking"]; exists {
		t.Fatalf("response nested booking result unexpectedly: %+v", envelope.Data)
	}
}

func TestMiniappBookingsPostDoesNotBroadcastOnFailureAndMapsErrorsSafely(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantBody   string
		secret     string
	}{
		{name: "validation", err: fmt.Errorf("unsafe details: %w", miniapp.ErrInvalidBooking), wantStatus: http.StatusBadRequest, wantBody: "预约信息格式不正确", secret: "unsafe details"},
		{name: "internal", err: errors.New("database password=super-secret unavailable"), wantStatus: http.StatusInternalServerError, wantBody: "预约提交失败，请稍后重试", secret: "super-secret"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := &miniappBookingCreatorFake{err: tt.err}
			subscriber := make(chan signup.Lead, 1)
			s := &Server{miniappBookingService: service, signupSubscribers: map[chan signup.Lead]struct{}{subscriber: {}}}

			response := performMiniappBookingPost(s, `{"contactName":"张三","phone":"13812345678"}`)

			if response.Code != tt.wantStatus {
				t.Fatalf("status = %d body=%s, want %d", response.Code, response.Body.String(), tt.wantStatus)
			}
			if !strings.Contains(response.Body.String(), tt.wantBody) || strings.Contains(response.Body.String(), tt.secret) {
				t.Fatalf("unsafe response body = %s", response.Body.String())
			}
			select {
			case got := <-subscriber:
				t.Fatalf("failed booking broadcast lead: %+v", got)
			default:
			}
		})
	}
}

func TestMiniappBookingsPostRejectsMalformedTrailingAndOversizedJSON(t *testing.T) {
	tests := []struct {
		name    string
		payload string
	}{
		{name: "malformed", payload: `{"contactName":`},
		{name: "trailing", payload: `{"contactName":"张三"} trailing`},
		{name: "multiple values", payload: `{"contactName":"张三"}{"phone":"13812345678"}`},
		{name: "oversized", payload: `{"contactName":"张三","phone":"13812345678","message":"` + strings.Repeat("x", 17*1024) + `"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := &miniappBookingCreatorFake{}
			response := performMiniappBookingPost(&Server{miniappBookingService: service}, tt.payload)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d body=%s, want 400", response.Code, response.Body.String())
			}
			if service.calls != 0 {
				t.Fatalf("invalid JSON reached service %d times", service.calls)
			}
		})
	}
}
