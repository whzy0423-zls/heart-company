package server

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/appuser"
	"nine-xing/nx-backend/apps/server/internal/config"
	"nine-xing/nx-backend/apps/server/internal/realip"
)

type failingRandReader struct{}

func (f failingRandReader) Read([]byte) (int, error) {
	return 0, errors.New("rand failed")
}

func TestGenerateSMSCodeReturnsErrorWhenRandomReaderFails(t *testing.T) {
	code, err := generateSMSCodeFromReader(failingRandReader{})
	if err == nil {
		t.Fatal("expected random reader error")
	}
	if code != "" {
		t.Fatalf("expected no fallback code, got %q", code)
	}
}

func TestSMSCodeExpiryIsTenMinutes(t *testing.T) {
	if smsCodeExpiry != 10*time.Minute {
		t.Fatalf("expected sms code expiry 10 minutes, got %s", smsCodeExpiry)
	}
}

func TestSMSVerifyAttemptLimiterBlocksRepeatedAttempts(t *testing.T) {
	s := &Server{
		smsVerifyPhoneLimiter: newStrRateLimiter(5, time.Minute),
		smsVerifyIPLimiter:    newStrRateLimiter(10, time.Minute),
	}
	now := time.Unix(100, 0)

	for i := 0; i < 5; i++ {
		if !s.allowSMSVerifyAttempt("13800000007", "127.0.0.1", now) {
			t.Fatalf("attempt %d should be allowed", i+1)
		}
	}
	if s.allowSMSVerifyAttempt("13800000007", "127.0.0.1", now) {
		t.Fatal("expected repeated verify attempts for one phone to be rate limited")
	}
}

func TestAppAuthRejectsInvalidMainlandPhoneFormats(t *testing.T) {
	s := newAppAuthPhoneValidationTestServer(t)

	tests := []struct {
		name    string
		handler http.HandlerFunc
		payload string
	}{
		{
			name:    "send rejects non-digits",
			handler: s.appSendSMS,
			payload: `{"phone":"1380000abcd"}`,
		},
		{
			name:    "send rejects invalid mainland prefix",
			handler: s.appSendSMS,
			payload: `{"phone":"12800000000"}`,
		},
		{
			name:    "verify rejects non-digits",
			handler: s.appVerifySMS,
			payload: `{"phone":"1380000abcd","code":"123456"}`,
		},
		{
			name:    "verify rejects invalid mainland prefix",
			handler: s.appVerifySMS,
			payload: `{"phone":"12800000000","code":"123456"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/api/app/auth/sms", strings.NewReader(tt.payload))
			response := httptest.NewRecorder()

			tt.handler(response, request)

			if response.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d body=%s", response.Code, response.Body.String())
			}
		})
	}
}

func TestAppSendSMSDoesNotUseDevCodeInProduction(t *testing.T) {
	s := newAppAuthPhoneValidationTestServer(t)
	s.env = config.Env{AppEnv: "production"}
	request := httptest.NewRequest(http.MethodPost, "/api/app/auth/sms", strings.NewReader(`{"phone":"13800000000"}`))
	response := httptest.NewRecorder()

	s.appSendSMS(response, request)

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected production without SMS provider to fail closed, got %d body=%s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "devCode") {
		t.Fatalf("expected production response not to contain devCode, got %s", response.Body.String())
	}
}

func TestAppSendSMSDevelopmentLogDoesNotContainDevCode(t *testing.T) {
	s := newAppAuthPhoneValidationTestServer(t)
	phone := "13912345678"

	var logOutput bytes.Buffer
	previousWriter := log.Writer()
	previousFlags := log.Flags()
	previousPrefix := log.Prefix()
	log.SetOutput(&logOutput)
	log.SetFlags(0)
	log.SetPrefix("")
	defer func() {
		log.SetOutput(previousWriter)
		log.SetFlags(previousFlags)
		log.SetPrefix(previousPrefix)
	}()

	request := httptest.NewRequest(http.MethodPost, "/api/app/auth/send-sms", strings.NewReader(`{"phone":"`+phone+`"}`))
	response := httptest.NewRecorder()

	s.appSendSMS(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", response.Code, response.Body.String())
	}
	var payload struct {
		Data struct {
			DevCode string `json:"devCode"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Data.DevCode == "" {
		t.Fatal("expected devCode response for local development")
	}
	if !strings.Contains(logOutput.String(), "[SMS-DEV]") {
		t.Fatalf("expected development SMS log marker, got %q", logOutput.String())
	}
	if strings.Contains(logOutput.String(), payload.Data.DevCode) {
		t.Fatalf("expected development log not to contain devCode, got %q", logOutput.String())
	}
}

func TestClientIPIgnoresForwardedHeadersFromUntrustedRemote(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodPost, "/api/app/auth/send-sms", nil)
	req.RemoteAddr = "203.0.113.8:12345"
	req.Header.Set("X-Forwarded-For", "198.51.100.9")
	req.Header.Set("X-Real-Ip", "198.51.100.10")

	if got := s.clientIP(req); got != "203.0.113.8" {
		t.Fatalf("expected untrusted direct remote IP, got %q", got)
	}
}

func TestClientIPIgnoresForwardedHeadersFromPrivateProxyByDefault(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodPost, "/api/app/auth/send-sms", nil)
	req.RemoteAddr = "10.0.0.12:12345"
	req.Header.Set("X-Forwarded-For", "198.51.100.9, 10.0.0.12")

	if got := s.clientIP(req); got != "10.0.0.12" {
		t.Fatalf("expected private proxy headers to be ignored unless explicitly trusted, got %q", got)
	}
}

func TestClientIPTrustsForwardedHeadersFromConfiguredProxy(t *testing.T) {
	trustedProxyCIDRs, err := realip.ParseTrustedProxyCIDRs([]string{"10.0.0.0/24"})
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{trustedProxyCIDRs: trustedProxyCIDRs}
	req := httptest.NewRequest(http.MethodPost, "/api/app/auth/send-sms", nil)
	req.RemoteAddr = "10.0.0.12:12345"
	req.Header.Set("X-Forwarded-For", "198.51.100.9, 10.0.0.12")

	if got := s.clientIP(req); got != "198.51.100.9" {
		t.Fatalf("expected forwarded client IP from trusted proxy, got %q", got)
	}
}

func newAppAuthPhoneValidationTestServer(t *testing.T) *Server {
	t.Helper()
	registerAppAuthUnitTestDriver()
	db, err := sql.Open(appAuthUnitTestDriverName, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	return &Server{
		appUsers:              appuser.NewStore(db),
		smsPhoneLimiter:       newStrRateLimiter(100, time.Minute),
		smsIPLimiter:          newStrRateLimiter(100, time.Minute),
		smsVerifyPhoneLimiter: newStrRateLimiter(100, time.Minute),
		smsVerifyIPLimiter:    newStrRateLimiter(100, time.Minute),
	}
}

const appAuthUnitTestDriverName = "app_auth_unit_test"

var registerAppAuthUnitTestDriverOnce sync.Once

func registerAppAuthUnitTestDriver() {
	registerAppAuthUnitTestDriverOnce.Do(func() {
		sql.Register(appAuthUnitTestDriverName, appAuthUnitTestDriver{})
	})
}

type appAuthUnitTestDriver struct{}

func (appAuthUnitTestDriver) Open(string) (driver.Conn, error) {
	return appAuthUnitTestConn{}, nil
}

type appAuthUnitTestConn struct{}

func (appAuthUnitTestConn) Prepare(string) (driver.Stmt, error) { return nil, driver.ErrSkip }
func (appAuthUnitTestConn) Close() error                        { return nil }
func (appAuthUnitTestConn) Begin() (driver.Tx, error)           { return appAuthUnitTestTx{}, nil }

func (appAuthUnitTestConn) BeginTx(context.Context, driver.TxOptions) (driver.Tx, error) {
	return appAuthUnitTestTx{}, nil
}

func (appAuthUnitTestConn) ExecContext(context.Context, string, []driver.NamedValue) (driver.Result, error) {
	return driver.RowsAffected(1), nil
}

func (appAuthUnitTestConn) QueryContext(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
	return appAuthUnitTestRows{}, nil
}

type appAuthUnitTestTx struct{}

func (appAuthUnitTestTx) Commit() error   { return nil }
func (appAuthUnitTestTx) Rollback() error { return nil }

type appAuthUnitTestRows struct{}

func (appAuthUnitTestRows) Columns() []string {
	return []string{"id"}
}

func (appAuthUnitTestRows) Close() error {
	return nil
}

func (appAuthUnitTestRows) Next([]driver.Value) error {
	return io.EOF
}

func TestAppAuthCompatibilityAliasRoutes(t *testing.T) {
	server := newRouteOnlyServer()

	tests := []struct {
		name   string
		method string
		path   string
		body   string
		want   int
	}{
		{
			name:   "sms send alias reaches send sms handler",
			method: http.MethodPost,
			path:   "/api/app/auth/sms/send",
			body:   `{"phone":"1380000"}`,
			want:   http.StatusBadRequest,
		},
		{
			name:   "legacy sms endpoint reaches send sms handler",
			method: http.MethodPost,
			path:   "/api/app/auth/sms",
			body:   `{"phone":"1380000"}`,
			want:   http.StatusBadRequest,
		},
		{
			name:   "sms login alias reaches verify sms handler",
			method: http.MethodPost,
			path:   "/api/app/auth/sms/login",
			body:   `{"phone":"13800000000","code":"123"}`,
			want:   http.StatusBadRequest,
		},
		{
			name:   "password registration route reaches handler",
			method: http.MethodPost,
			path:   "/api/app/auth/register",
			body:   `{`,
			want:   http.StatusBadRequest,
		},
		{
			name:   "password login route reaches handler",
			method: http.MethodPost,
			path:   "/api/app/auth/login",
			body:   `{`,
			want:   http.StatusBadRequest,
		},
		{
			name:   "me alias reaches app auth guard",
			method: http.MethodGet,
			path:   "/api/app/me",
			want:   http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			server.mux.ServeHTTP(rec, req)

			if rec.Code != tt.want {
				t.Fatalf("expected status %d, got %d body=%s", tt.want, rec.Code, rec.Body.String())
			}
		})
	}
}

func TestAppPasswordRegistrationValidationRejectsBeforeStore(t *testing.T) {
	server := newRouteOnlyServer()
	longPassword := strings.Repeat("a", 73)

	tests := []struct {
		name        string
		payload     string
		wantMessage string
	}{
		{
			name:        "invalid account",
			payload:     `{"nickname":"心之力用户","account":"1bad","password":"secret1","phone":"13800000000","code":"123456"}`,
			wantMessage: "用户名格式不正确",
		},
		{
			name:        "password shorter than six bytes",
			payload:     `{"nickname":"心之力用户","account":"xinuser","password":"12345","phone":"13800000000","code":"123456"}`,
			wantMessage: "密码格式不正确",
		},
		{
			name:        "password longer than seventy two bytes",
			payload:     `{"nickname":"心之力用户","account":"xinuser","password":"` + longPassword + `","phone":"13800000000","code":"123456"}`,
			wantMessage: "密码格式不正确",
		},
		{
			name:        "invalid nickname",
			payload:     `{"nickname":"   ","account":"xinuser","password":"secret1","phone":"13800000000","code":"123456"}`,
			wantMessage: "昵称格式不正确",
		},
		{
			name:        "invalid mainland phone",
			payload:     `{"nickname":"心之力用户","account":"xinuser","password":"secret1","phone":"12800000000","code":"123456"}`,
			wantMessage: "手机号格式不正确",
		},
		{
			name:        "code is not six digits",
			payload:     `{"nickname":"心之力用户","account":"xinuser","password":"secret1","phone":"13800000000","code":"12345"}`,
			wantMessage: "验证码格式不正确",
		},
		{
			name:        "code contains non digits",
			payload:     `{"nickname":"心之力用户","account":"xinuser","password":"secret1","phone":"13800000000","code":"12345a"}`,
			wantMessage: "验证码格式不正确",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/app/auth/register", strings.NewReader(tt.payload))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			server.mux.ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), tt.wantMessage) {
				t.Fatalf("expected message %q, got body=%s", tt.wantMessage, rec.Body.String())
			}
		})
	}
}

func TestAppPasswordHandlersRejectTrailingJSON(t *testing.T) {
	requests := []struct {
		name string
		path string
		body string
	}{
		{
			name: "registration",
			path: "/api/app/auth/register",
			body: `{"nickname":"心之力用户","account":"xinuser","password":"secret1","phone":"13800000000","code":"123456"}`,
		},
		{
			name: "login",
			path: "/api/app/auth/login",
			body: `{"account":"xinuser","password":"secret1"}`,
		},
	}
	suffixes := []struct {
		name  string
		value string
	}{
		{name: "garbage", value: "garbage"},
		{name: "second json object", value: ` {"extra":true}`},
		{name: "body limit exceeded by trailing whitespace", value: strings.Repeat(" ", appPasswordAuthBodyLimit+1)},
	}

	for _, request := range requests {
		for _, suffix := range suffixes {
			t.Run(request.name+"/"+suffix.name, func(t *testing.T) {
				server := newAppPasswordTrailingJSONTestServer(t)
				req := httptest.NewRequest(http.MethodPost, request.path, strings.NewReader(request.body+suffix.value))
				req.Header.Set("Content-Type", "application/json")
				rec := httptest.NewRecorder()

				server.mux.ServeHTTP(rec, req)

				if rec.Code != http.StatusBadRequest {
					t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
				}
			})
		}
	}
}

func TestAppPasswordHandlersAllowTrailingWhitespace(t *testing.T) {
	requests := []struct {
		name string
		path string
		body string
	}{
		{
			name: "registration continues to sms limiter",
			path: "/api/app/auth/register",
			body: `{"nickname":"心之力用户","account":"xinuser","password":"secret1","phone":"13800000000","code":"123456"}`,
		},
		{
			name: "login continues to password limiter",
			path: "/api/app/auth/login",
			body: `{"account":"xinuser","password":"secret1"}`,
		},
	}

	for _, request := range requests {
		t.Run(request.name, func(t *testing.T) {
			server := newAppPasswordTrailingJSONTestServer(t)
			req := httptest.NewRequest(http.MethodPost, request.path, strings.NewReader(request.body+" \n\t"))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			server.mux.ServeHTTP(rec, req)

			if rec.Code != http.StatusTooManyRequests {
				t.Fatalf("expected trailing whitespace to reach limiter with 429, got %d body=%s", rec.Code, rec.Body.String())
			}
		})
	}
}

func newAppPasswordTrailingJSONTestServer(t *testing.T) *Server {
	t.Helper()
	now := time.Now()
	smsLimiter := newStrRateLimiter(1, time.Minute)
	if !smsLimiter.Allow("13800000000", now) {
		t.Fatal("expected SMS limiter setup attempt to be allowed")
	}
	accountLimiter := newStrRateLimiter(1, time.Minute)
	if !accountLimiter.Allow("xinuser", now) {
		t.Fatal("expected password limiter setup attempt to be allowed")
	}
	server := &Server{
		mux:                       http.NewServeMux(),
		smsVerifyPhoneLimiter:     smsLimiter,
		appPasswordAccountLimiter: accountLimiter,
	}
	server.routes()
	return server
}

func TestAppPasswordLoginValidationRejectsBeforeStore(t *testing.T) {
	server := newRouteOnlyServer()

	tests := []struct {
		name    string
		payload string
	}{
		{
			name:    "empty identifier",
			payload: `{"account":"   ","password":"secret1"}`,
		},
		{
			name:    "empty password",
			payload: `{"account":"xinuser","password":""}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/app/auth/login", strings.NewReader(tt.payload))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			server.mux.ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), "请输入账号和密码") {
				t.Fatalf("expected missing credentials message, got body=%s", rec.Body.String())
			}
		})
	}
}

func TestAppPasswordRegistrationErrorResponses(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		wantStatus  int
		wantMessage string
	}{
		{name: "invalid account", err: appuser.ErrInvalidAccount, wantStatus: http.StatusBadRequest, wantMessage: "用户名格式不正确"},
		{name: "invalid password", err: appuser.ErrInvalidPassword, wantStatus: http.StatusBadRequest, wantMessage: "密码格式不正确"},
		{name: "invalid nickname", err: appuser.ErrInvalidNickname, wantStatus: http.StatusBadRequest, wantMessage: "昵称格式不正确"},
		{name: "invalid sms code", err: appuser.ErrInvalidSMSCode, wantStatus: http.StatusUnauthorized, wantMessage: "验证码错误或已过期"},
		{name: "disabled user", err: appuser.ErrUserDisabled, wantStatus: http.StatusForbidden, wantMessage: "账号已被禁用"},
		{name: "account taken", err: appuser.ErrAccountTaken, wantStatus: http.StatusConflict, wantMessage: "用户名已存在"},
		{name: "phone registered", err: appuser.ErrPhoneAlreadyRegistered, wantStatus: http.StatusConflict, wantMessage: "该手机号已注册"},
		{name: "wrapped account error", err: fmt.Errorf("register: %w", appuser.ErrAccountTaken), wantStatus: http.StatusConflict, wantMessage: "用户名已存在"},
		{name: "internal error", err: errors.New("database unavailable"), wantStatus: http.StatusInternalServerError, wantMessage: "注册失败"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, message := appPasswordRegistrationErrorResponse(tt.err)
			if status != tt.wantStatus || message != tt.wantMessage {
				t.Fatalf("expected (%d, %q), got (%d, %q)", tt.wantStatus, tt.wantMessage, status, message)
			}
		})
	}
}

func TestAppPasswordLoginErrorResponses(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		wantStatus  int
		wantMessage string
	}{
		{name: "invalid credentials", err: appuser.ErrInvalidCredentials, wantStatus: http.StatusUnauthorized, wantMessage: "账号或密码错误"},
		{name: "wrapped invalid credentials", err: fmt.Errorf("login: %w", appuser.ErrInvalidCredentials), wantStatus: http.StatusUnauthorized, wantMessage: "账号或密码错误"},
		{name: "disabled user", err: appuser.ErrUserDisabled, wantStatus: http.StatusForbidden, wantMessage: "账号已被禁用"},
		{name: "internal error", err: errors.New("database unavailable"), wantStatus: http.StatusInternalServerError, wantMessage: "登录失败"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, message := appPasswordLoginErrorResponse(tt.err)
			if status != tt.wantStatus || message != tt.wantMessage {
				t.Fatalf("expected (%d, %q), got (%d, %q)", tt.wantStatus, tt.wantMessage, status, message)
			}
		})
	}
}

func TestAppPasswordLoginAttemptLimiterNormalizesKeysAndFailsOpenWhenNil(t *testing.T) {
	now := time.Unix(200, 0)

	if !(&Server{}).allowAppPasswordLoginAttempt(" XinUser ", " 127.0.0.1 ", now) {
		t.Fatal("expected nil password login limiters to fail open")
	}

	accountLimited := &Server{
		appPasswordAccountLimiter: newStrRateLimiter(1, time.Minute),
		appPasswordIPLimiter:      newStrRateLimiter(10, time.Minute),
	}
	if !accountLimited.allowAppPasswordLoginAttempt(" XinUser ", "127.0.0.1", now) {
		t.Fatal("expected first normalized account attempt to be allowed")
	}
	if accountLimited.allowAppPasswordLoginAttempt("xinuser", "127.0.0.2", now) {
		t.Fatal("expected account limiter to use lowercase trimmed identifier")
	}

	ipLimited := &Server{
		appPasswordAccountLimiter: newStrRateLimiter(10, time.Minute),
		appPasswordIPLimiter:      newStrRateLimiter(1, time.Minute),
	}
	if !ipLimited.allowAppPasswordLoginAttempt("firstuser", " 127.0.0.1 ", now) {
		t.Fatal("expected first normalized IP attempt to be allowed")
	}
	if ipLimited.allowAppPasswordLoginAttempt("seconduser", "127.0.0.1", now) {
		t.Fatal("expected IP limiter to use trimmed IP")
	}
}

func TestAppPasswordLoginRouteAppliesLimiterBeforeStore(t *testing.T) {
	now := time.Now()
	accountLimiter := newStrRateLimiter(1, time.Minute)
	if !accountLimiter.Allow("xinuser", now) {
		t.Fatal("expected limiter setup attempt to be allowed")
	}
	server := &Server{
		mux:                       http.NewServeMux(),
		appPasswordAccountLimiter: accountLimiter,
	}
	server.routes()

	req := httptest.NewRequest(http.MethodPost, "/api/app/auth/login", strings.NewReader(`{"account":" XinUser ","password":"secret1"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 before store access, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAppUsersAdminUpdateRouteAllowsWriteMethodsThroughAuth(t *testing.T) {
	server := newRouteOnlyServer()

	for _, method := range []string{http.MethodPut, http.MethodPatch} {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/api/app-users/42", strings.NewReader(`{"status":"disabled","memberLevel":"vip"}`))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			server.mux.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("expected write route to reach auth guard with 401, got %d body=%s", rec.Code, rec.Body.String())
			}
		})
	}
}

func newRouteOnlyServer() *Server {
	server := &Server{mux: http.NewServeMux()}
	server.routes()
	return server
}
