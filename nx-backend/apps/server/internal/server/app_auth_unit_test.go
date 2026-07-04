package server

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
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
			name:   "sms login alias reaches verify sms handler",
			method: http.MethodPost,
			path:   "/api/app/auth/sms/login",
			body:   `{"phone":"13800000000","code":"123"}`,
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
