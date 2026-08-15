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
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/appuser"
	"nine-xing/nx-backend/apps/server/internal/config"
	serverdb "nine-xing/nx-backend/apps/server/internal/db"
	"nine-xing/nx-backend/apps/server/internal/realip"
	"nine-xing/nx-backend/apps/server/internal/testutil"
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

func TestSMSVerifyIPLimiterRunsBeforePhoneLimiter(t *testing.T) {
	now := time.Unix(150, 0)
	ip := "203.0.113.9"
	existingPhone := "13800000007"
	newPhone := "13800000008"
	phoneLimiter := newStrRateLimiter(5, time.Minute)
	ipLimiter := newStrRateLimiter(1, time.Minute)
	if !phoneLimiter.Allow(existingPhone, now) {
		t.Fatal("expected existing phone setup attempt to be allowed")
	}
	if !ipLimiter.Allow(ip, now) {
		t.Fatal("expected IP setup attempt to be allowed")
	}
	s := &Server{
		smsVerifyPhoneLimiter: phoneLimiter,
		smsVerifyIPLimiter:    ipLimiter,
	}

	if s.allowSMSVerifyAttempt(existingPhone, ip, now) {
		t.Fatal("expected exhausted IP limiter to reject existing phone")
	}
	if s.allowSMSVerifyAttempt(newPhone, ip, now) {
		t.Fatal("expected exhausted IP limiter to reject new phone")
	}

	if got, ok := rateLimiterCount(phoneLimiter, existingPhone); !ok || got != 1 {
		t.Fatalf("expected existing phone limiter count to remain 1, got count=%d exists=%t", got, ok)
	}
	if _, ok := rateLimiterCount(phoneLimiter, newPhone); ok {
		t.Fatal("expected rejected IP not to create a new phone limiter key")
	}
}

func TestAppSendSMSIPLimiterRunsBeforePhoneLimiter(t *testing.T) {
	now := time.Now()
	ip := "203.0.113.10"
	existingPhone := "13800000009"
	newPhone := "13800000010"
	phoneLimiter := newStrRateLimiter(5, time.Minute)
	ipLimiter := newStrRateLimiter(1, time.Minute)
	if !phoneLimiter.Allow(existingPhone, now) {
		t.Fatal("expected existing phone setup attempt to be allowed")
	}
	if !ipLimiter.Allow(ip, now) {
		t.Fatal("expected IP setup attempt to be allowed")
	}
	s := &Server{
		smsPhoneLimiter: phoneLimiter,
		smsIPLimiter:    ipLimiter,
	}

	for _, phone := range []string{existingPhone, newPhone} {
		request := httptest.NewRequest(http.MethodPost, "/api/app/auth/send-sms", strings.NewReader(`{"phone":"`+phone+`"}`))
		request.RemoteAddr = ip + ":1234"
		response := httptest.NewRecorder()

		s.appSendSMS(response, request)

		if response.Code != http.StatusTooManyRequests {
			t.Fatalf("expected exhausted IP limiter to return 429 for %s, got %d body=%s", phone, response.Code, response.Body.String())
		}
	}

	if got, ok := rateLimiterCount(phoneLimiter, existingPhone); !ok || got != 1 {
		t.Fatalf("expected existing phone limiter count to remain 1, got count=%d exists=%t", got, ok)
	}
	if _, ok := rateLimiterCount(phoneLimiter, newPhone); ok {
		t.Fatal("expected rejected IP not to create a new phone limiter key")
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

func TestAppSendSMSDoesNotUseDevCodeOutsideDevOrTest(t *testing.T) {
	registerAppAuthUnitTestDriver()

	for _, appEnv := range []string{" staging ", " production "} {
		t.Run(strings.TrimSpace(appEnv), func(t *testing.T) {
			db, err := sql.Open(appAuthUnitTestDriverName, "")
			if err != nil {
				t.Fatal(err)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}

			s := &Server{
				env:             config.Env{AppEnv: appEnv},
				appUsers:        appuser.NewStore(db),
				smsPhoneLimiter: newStrRateLimiter(100, time.Minute),
				smsIPLimiter:    newStrRateLimiter(100, time.Minute),
			}
			request := httptest.NewRequest(http.MethodPost, "/api/app/auth/sms", strings.NewReader(`{"phone":"13800000000"}`))
			response := httptest.NewRecorder()

			s.appSendSMS(response, request)

			if response.Code != http.StatusServiceUnavailable {
				t.Fatalf("expected %s without SMS provider to fail closed before storage, got %d body=%s", strings.TrimSpace(appEnv), response.Code, response.Body.String())
			}
			if strings.Contains(response.Body.String(), "devCode") {
				t.Fatalf("expected %s response not to contain devCode, got %s", strings.TrimSpace(appEnv), response.Body.String())
			}
		})
	}
}

func TestAppSendSMSDevelopmentLogDoesNotContainDevCode(t *testing.T) {
	s := newAppAuthPhoneValidationTestServer(t)
	s.env = config.Env{AppEnv: "development"}
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

func TestWriteAppSessionDoesNotWriteWhenRefreshTokenPersistenceFails(t *testing.T) {
	registerAppAuthUnitTestDriver()
	db, err := sql.Open(appAuthUnitTestDriverName, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	s := &Server{
		env:      config.Env{JWTSecret: "task-6-test-secret"},
		appUsers: appuser.NewStore(db),
	}
	response := &writeTrackingResponseWriter{header: make(http.Header)}
	request := httptest.NewRequest(http.MethodPost, "/api/app/auth/login", nil)

	err = s.writeAppSession(response, request, appuser.User{ID: 42, Phone: "13800000011", Status: "active"}, "test-device")

	if err == nil {
		t.Fatal("expected refresh token persistence error")
	}
	if response.wrote {
		t.Fatalf("expected session writer to return the persistence error without writing a response, got status=%d body=%s", response.status, response.body.String())
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

func TestAppPasswordHandlersAllowUnknownFieldsInFirstJSONValue(t *testing.T) {
	requests := []struct {
		name string
		path string
		body string
	}{
		{
			name: "registration continues to sms limiter",
			path: "/api/app/auth/register",
			body: `{"nickname":"心之力用户","account":"xinuser","password":"secret1","phone":"13800000000","code":"123456","futureField":{"enabled":true}}`,
		},
		{
			name: "login continues to password limiter",
			path: "/api/app/auth/login",
			body: `{"account":"xinuser","password":"secret1","futureField":{"enabled":true}}`,
		},
	}

	for _, request := range requests {
		t.Run(request.name, func(t *testing.T) {
			server := newAppPasswordTrailingJSONTestServer(t)
			req := httptest.NewRequest(http.MethodPost, request.path, strings.NewReader(request.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			server.mux.ServeHTTP(rec, req)

			if rec.Code != http.StatusTooManyRequests {
				t.Fatalf("expected unknown field to be accepted and reach limiter with 429, got %d body=%s", rec.Code, rec.Body.String())
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

func TestAppPasswordHandlersRejectUnavailableAuthDependencies(t *testing.T) {
	registerAppAuthUnitTestDriver()
	db, err := sql.Open(appAuthUnitTestDriverName, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	handlers := []struct {
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
	dependencies := []struct {
		name      string
		configure func(*Server)
	}{
		{
			name: "missing app user store",
			configure: func(server *Server) {
				server.db = db
			},
		},
		{
			name: "missing database",
			configure: func(server *Server) {
				server.appUsers = appuser.NewStore(db)
			},
		},
	}

	for _, handler := range handlers {
		for _, dependency := range dependencies {
			t.Run(handler.name+"/"+dependency.name, func(t *testing.T) {
				server := &Server{mux: http.NewServeMux()}
				dependency.configure(server)
				server.routes()

				rec, recovered := serveAppAuthRequest(server, handler.path, handler.body)
				if recovered != nil {
					t.Fatalf("expected unavailable dependency to return 503 instead of panicking: %v", recovered)
				}
				if rec.Code != http.StatusServiceUnavailable {
					t.Fatalf("expected unavailable dependency to return 503, got %d body=%s", rec.Code, rec.Body.String())
				}
				if !strings.Contains(rec.Body.String(), "认证服务不可用") {
					t.Fatalf("expected unavailable auth service message, got body=%s", rec.Body.String())
				}
			})
		}
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

func TestAppPasswordLoginIPLimiterRunsBeforeAccountLimiter(t *testing.T) {
	now := time.Unix(250, 0)
	ip := "203.0.113.11"
	existingAccount := "existinguser"
	newAccount := "newuser"
	accountLimiter := newStrRateLimiter(5, time.Minute)
	ipLimiter := newStrRateLimiter(1, time.Minute)
	if !accountLimiter.Allow(existingAccount, now) {
		t.Fatal("expected existing account setup attempt to be allowed")
	}
	if !ipLimiter.Allow(ip, now) {
		t.Fatal("expected IP setup attempt to be allowed")
	}
	s := &Server{
		appPasswordAccountLimiter: accountLimiter,
		appPasswordIPLimiter:      ipLimiter,
	}

	if s.allowAppPasswordLoginAttempt(existingAccount, ip, now) {
		t.Fatal("expected exhausted IP limiter to reject existing account")
	}
	if s.allowAppPasswordLoginAttempt(newAccount, ip, now) {
		t.Fatal("expected exhausted IP limiter to reject new account")
	}

	if got, ok := rateLimiterCount(accountLimiter, existingAccount); !ok || got != 1 {
		t.Fatalf("expected existing account limiter count to remain 1, got count=%d exists=%t", got, ok)
	}
	if _, ok := rateLimiterCount(accountLimiter, newAccount); ok {
		t.Fatal("expected rejected IP not to create a new account limiter key")
	}
}

func TestAppPasswordLoginAttemptLimiterUsesDatabaseWithoutTouchingMemory(t *testing.T) {
	database := openAppPasswordLoginLimiterTestDatabase(t)
	now := time.Now()
	account := " XinUser "
	accountKey := "xinuser"
	ip := " 203.0.113.20 "
	ipKey := "203.0.113.20"
	accountLimiter := newStrRateLimiter(1, time.Minute)
	ipLimiter := newStrRateLimiter(1, time.Minute)
	if !accountLimiter.Allow(accountKey, now) {
		t.Fatal("expected account memory limiter setup attempt to be allowed")
	}
	if !ipLimiter.Allow(ipKey, now) {
		t.Fatal("expected IP memory limiter setup attempt to be allowed")
	}
	s := &Server{
		appPasswordAccountLimiter:   accountLimiter,
		appPasswordIPLimiter:        ipLimiter,
		appPasswordAccountDBLimiter: newDBRateLimiter(database, "app_password_account", 5, time.Minute),
		appPasswordIPDBLimiter:      newDBRateLimiter(database, "app_password_ip", 30, time.Minute),
	}

	if !s.allowAppPasswordLoginAttempt(account, ip, now) {
		t.Fatal("expected healthy database limiters to bypass exhausted memory limiters")
	}
	if got, ok := rateLimiterCount(accountLimiter, accountKey); !ok || got != 1 {
		t.Fatalf("expected account memory limiter count to remain 1, got count=%d exists=%t", got, ok)
	}
	if got, ok := rateLimiterCount(ipLimiter, ipKey); !ok || got != 1 {
		t.Fatalf("expected IP memory limiter count to remain 1, got count=%d exists=%t", got, ok)
	}
	assertDBRateLimiterCount(t, database, "app_password_account", "account:xinuser", 1)
	assertDBRateLimiterCount(t, database, "app_password_ip", "ip:203.0.113.20", 1)
}

func TestAppPasswordLoginAttemptLimiterFallsBackToMemoryWhenDatabaseFails(t *testing.T) {
	database := openFailingAppPasswordLoginLimiterDatabase(t)
	now := time.Unix(300, 0)

	t.Run("account", func(t *testing.T) {
		accountLimiter := newStrRateLimiter(1, time.Minute)
		s := &Server{
			appPasswordAccountLimiter:   accountLimiter,
			appPasswordIPLimiter:        newStrRateLimiter(10, time.Minute),
			appPasswordAccountDBLimiter: newDBRateLimiter(database, "app_password_account", 5, time.Minute),
		}

		if !s.allowAppPasswordLoginAttempt(" XinUser ", "203.0.113.21", now) {
			t.Fatal("expected first account attempt to fall back to memory and be allowed")
		}
		if s.allowAppPasswordLoginAttempt("xinuser", "203.0.113.22", now.Add(time.Second)) {
			t.Fatal("expected account memory fallback to reject the second attempt")
		}
	})

	t.Run("ip", func(t *testing.T) {
		accountLimiter := newStrRateLimiter(10, time.Minute)
		ipLimiter := newStrRateLimiter(1, time.Minute)
		s := &Server{
			appPasswordAccountLimiter: accountLimiter,
			appPasswordIPLimiter:      ipLimiter,
			appPasswordIPDBLimiter:    newDBRateLimiter(database, "app_password_ip", 30, time.Minute),
		}

		if !s.allowAppPasswordLoginAttempt("firstuser", " 203.0.113.23 ", now) {
			t.Fatal("expected first IP attempt to fall back to memory and be allowed")
		}
		if s.allowAppPasswordLoginAttempt("seconduser", "203.0.113.23", now.Add(time.Second)) {
			t.Fatal("expected IP memory fallback to reject the second attempt")
		}
		if _, ok := rateLimiterCount(accountLimiter, "seconduser"); ok {
			t.Fatal("expected rejected IP fallback not to create an account memory key")
		}
	})
}

func openAppPasswordLoginLimiterTestDatabase(t *testing.T) *sql.DB {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run app password limiter integration tests")
	}
	if err := testutil.ValidateIsolatedPostgresDSN(dsn); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	database, err := serverdb.Open(ctx, dsn, "admin", "123456")
	if err != nil {
		t.Fatalf("open app password limiter database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	clearAppPasswordLoginLimiterScopes(t, database)
	t.Cleanup(func() { clearAppPasswordLoginLimiterScopes(t, database) })
	return database
}

func openFailingAppPasswordLoginLimiterDatabase(t *testing.T) *sql.DB {
	t.Helper()
	const driverName = "nine_xing_failing_limiter"
	failingQueryDriverRegisterOnce.Do(func() {
		sql.Register(driverName, failingQueryDriver{})
	})
	database, err := sql.Open(driverName, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database
}

func clearAppPasswordLoginLimiterScopes(t *testing.T, database *sql.DB) {
	t.Helper()
	if _, err := database.Exec(`DELETE FROM request_rate_limits WHERE scope IN ('app_password_account', 'app_password_ip')`); err != nil {
		t.Fatalf("clear app password login limiter scopes: %v", err)
	}
}

func assertDBRateLimiterCount(t *testing.T, database *sql.DB, scope, key string, want int) {
	t.Helper()
	var got int
	if err := database.QueryRow(`SELECT count FROM request_rate_limits WHERE scope=$1 AND key=$2`, scope, key).Scan(&got); err != nil {
		t.Fatalf("read rate limiter %s/%s: %v", scope, key, err)
	}
	if got != want {
		t.Fatalf("rate limiter %s/%s count=%d want=%d", scope, key, got, want)
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

type writeTrackingResponseWriter struct {
	header http.Header
	status int
	body   bytes.Buffer
	wrote  bool
}

func (w *writeTrackingResponseWriter) Header() http.Header {
	return w.header
}

func (w *writeTrackingResponseWriter) WriteHeader(status int) {
	w.status = status
	w.wrote = true
}

func (w *writeTrackingResponseWriter) Write(body []byte) (int, error) {
	w.wrote = true
	return w.body.Write(body)
}

func rateLimiterCount(limiter *strRateLimiter, key string) (int, bool) {
	limiter.mu.Lock()
	defer limiter.mu.Unlock()
	window, ok := limiter.keys[key]
	return window.count, ok
}

func serveAppAuthRequest(server *Server, path, body string) (rec *httptest.ResponseRecorder, recovered any) {
	rec = httptest.NewRecorder()
	defer func() {
		recovered = recover()
	}()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	server.mux.ServeHTTP(rec, req)
	return rec, nil
}

func newRouteOnlyServer() *Server {
	server := &Server{mux: http.NewServeMux()}
	server.routes()
	return server
}
