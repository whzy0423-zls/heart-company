package server

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/config"
	"nine-xing/nx-backend/apps/server/internal/llm"
	"nine-xing/nx-backend/apps/server/internal/modelconfig"
	"nine-xing/nx-backend/apps/server/internal/storage"
	"nine-xing/nx-backend/apps/server/internal/uploadasset"
	"nine-xing/nx-backend/apps/server/internal/videoanalysis"
	"nine-xing/nx-backend/apps/server/internal/wxpay"
)

func TestNewPanicsForUnknownSMSProvider(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected sms sender config to panic for unknown provider")
		}
	}()

	mustSMSSender(config.SMSConfig{Provider: "unknown"})
}

func TestNewReturnsShutdownCapableHandler(t *testing.T) {
	handler := New(config.Env{JWTSecret: "test-secret"}, nil)
	shutdowner, ok := handler.(interface{ Shutdown() })
	if !ok {
		t.Fatalf("server.New returned %T without Shutdown capability", handler)
	}
	shutdowner.Shutdown()
}

func TestMustSMSSenderSupportsSpugProvider(t *testing.T) {
	sender := mustSMSSender(config.SMSConfig{
		Provider:           "spug",
		SpugAPIBase:        "https://push.spug.cc",
		SpugTemplateCode:   "tmpl123",
		SpugTemplateName:   "芯之力",
		SpugTimeoutSeconds: 5,
	})
	if sender == nil {
		t.Fatal("expected spug sender")
	}
}

func TestProductionWxPayIncompleteConfigDisablesPaymentWithoutPanic(t *testing.T) {
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("expected incomplete production wxpay config to disable payment without panic, got panic %v", recovered)
		}
	}()

	client := mustWxPayClient(config.Env{
		AppEnv: "production",
		WxPay:  config.WxPayConfig{Dev: false},
	})
	if client != nil {
		t.Fatal("expected incomplete production wxpay config to return nil client")
	}
}

func TestNewAllowsExplicitDevWxPay(t *testing.T) {
	client := mustWxPayClient(config.Env{WxPay: config.WxPayConfig{Dev: true}})
	if !client.DevMode() {
		t.Fatal("expected explicit dev wxpay to stay in dev mode")
	}
}

func TestWeChatMissingCredentialsOnlyFallbackOutsideProduction(t *testing.T) {
	productionClient := newWeChatClient(config.Env{AppEnv: "production"})
	if productionClient.DevMode() {
		t.Fatal("expected production wechat client to fail closed without dev fallback")
	}

	devClient := newWeChatClient(config.Env{AppEnv: "dev"})
	if !devClient.DevMode() {
		t.Fatal("expected non-production missing credentials to use dev fallback")
	}
}

func TestCreateReportOrderReturnsUnavailableWhenPaymentDisabled(t *testing.T) {
	s := &Server{}
	response := performRawUnit(http.HandlerFunc(s.createReportOrder), http.MethodPost, "/api/miniapp/report/order", `{"testRecordId":"1"}`)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected disabled payment to return 503, got %d body=%s", response.Code, response.Body.String())
	}
}

func TestPayNotifyReturnsUnavailableWhenPaymentDisabled(t *testing.T) {
	s := &Server{}
	response := performRawUnit(http.HandlerFunc(s.payNotify), http.MethodPost, "/api/pay/notify", `{}`)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected disabled payment callback to return 503, got %d body=%s", response.Code, response.Body.String())
	}
}

func TestValidateWxPayCallbackRejectsMismatchedOrderData(t *testing.T) {
	env := config.Env{WxPay: config.WxPayConfig{MchID: "mch-1", AppID: "wx-app-1"}}
	order := paymentOrderSnapshot{Product: "report", Amount: 990}

	for _, tc := range []struct {
		name   string
		result wxpay.CallbackResult
		want   string
	}{
		{
			name: "amount",
			result: wxpay.CallbackResult{
				AppID:       "wx-app-1",
				MchID:       "mch-1",
				AmountTotal: 1,
				OutTradeNo:  "rpt1",
				Success:     true,
			},
			want: "amount",
		},
		{
			name: "mchid",
			result: wxpay.CallbackResult{
				AppID:       "wx-app-1",
				MchID:       "wrong",
				AmountTotal: 990,
				OutTradeNo:  "rpt1",
				Success:     true,
			},
			want: "merchant",
		},
		{
			name: "appid",
			result: wxpay.CallbackResult{
				AppID:       "wrong",
				MchID:       "mch-1",
				AmountTotal: 990,
				OutTradeNo:  "rpt1",
				Success:     true,
			},
			want: "appid",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validateWxPayCallbackAgainstOrder(env, tc.result, order)
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), tc.want) {
				t.Fatalf("expected %s validation error, got %v", tc.want, err)
			}
		})
	}
}

func TestGenerateReportOutTradeNoIsUniqueUnderBurst(t *testing.T) {
	seen := map[string]struct{}{}
	for range 1000 {
		value, err := generateReportOutTradeNo(7, 42)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(value, "rpt7-42-") {
			t.Fatalf("unexpected out trade no prefix: %s", value)
		}
		if _, ok := seen[value]; ok {
			t.Fatalf("duplicate out trade no: %s", value)
		}
		seen[value] = struct{}{}
	}
}

func TestCORSAllowsAnyOriginOutsideProductionWhenNoAllowlist(t *testing.T) {
	s := &Server{env: config.Env{AppEnv: "dev"}}
	if !s.corsOriginAllowed("https://admin.localhost") {
		t.Fatal("expected dev CORS to allow arbitrary origin")
	}
}

func TestCORSRequiresAllowlistInProduction(t *testing.T) {
	s := &Server{env: config.Env{
		AppEnv: "production",
		CORSAllowedOrigins: []string{
			"https://admin.example.com",
		},
	}}
	if !s.corsOriginAllowed("https://admin.example.com/") {
		t.Fatal("expected configured production origin to be allowed")
	}
	if s.corsOriginAllowed("https://evil.example.com") {
		t.Fatal("expected unconfigured production origin to be rejected")
	}
}

func TestCORSPreflightAllowsPatch(t *testing.T) {
	s := &Server{env: config.Env{AppEnv: "dev"}}
	handler := s.withCORS(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	request := httptest.NewRequest(http.MethodOptions, "/api/app-users/1", nil)
	request.Header.Set("Origin", "https://admin.localhost")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("expected 204 preflight, got %d", response.Code)
	}
	if methods := response.Header().Get("Access-Control-Allow-Methods"); !strings.Contains(methods, "PATCH") {
		t.Fatalf("expected PATCH in CORS methods, got %q", methods)
	}
}

func TestPublicWriteEndpointsRateLimitByIP(t *testing.T) {
	s := &Server{
		publicAnalyticsIPLimiter: newStrRateLimiter(1, time.Minute),
	}

	first := performRawUnit(http.HandlerFunc(s.publicSiteVisit), http.MethodPost, "/api/public/site-visits", "{")
	if first.Code != http.StatusBadRequest {
		t.Fatalf("expected first malformed request to reach handler validation, got %d body=%s", first.Code, first.Body.String())
	}

	second := performRawUnit(http.HandlerFunc(s.publicSiteVisit), http.MethodPost, "/api/public/site-visits", "{")
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("expected repeated public write from same IP to be rate limited, got %d body=%s", second.Code, second.Body.String())
	}
}

func TestPublicBaseURLDoesNotUseRequestHostHeaders(t *testing.T) {
	s := &Server{}
	request := httptest.NewRequest(http.MethodPost, "/api/video/analysis", nil)
	request.Host = "evil.example.com"
	request.Header.Set("X-Forwarded-Host", "attacker.example.com")
	request.Header.Set("X-Forwarded-Proto", "https")

	if got := s.publicBaseURL(request); got != "" {
		t.Fatalf("expected empty public base URL without explicit config, got %q", got)
	}
}

func TestPublicBaseURLUsesExplicitConfig(t *testing.T) {
	s := &Server{env: config.Env{PublicBaseURL: "https://api.example.com/"}}
	request := httptest.NewRequest(http.MethodPost, "/api/video/analysis", nil)
	request.Host = "evil.example.com"
	request.Header.Set("X-Forwarded-Host", "attacker.example.com")

	if got := s.publicBaseURL(request); got != "https://api.example.com" {
		t.Fatalf("expected configured public base URL, got %q", got)
	}
}

func TestBackendLoginRateLimitTracksIPAndUsername(t *testing.T) {
	s := &Server{
		loginLimiter: newStrRateLimiter(2, time.Minute),
	}
	now := time.Unix(100, 0)

	if !s.allowLoginAttempt("Admin", "203.0.113.9", now) {
		t.Fatal("expected first login attempt to be allowed")
	}
	if !s.allowLoginAttempt(" admin ", "203.0.113.9", now.Add(time.Second)) {
		t.Fatal("expected second login attempt with normalized username to be allowed")
	}
	if s.allowLoginAttempt("admin", "203.0.113.9", now.Add(2*time.Second)) {
		t.Fatal("expected third login attempt for same IP and username to be limited")
	}
	if !s.allowLoginAttempt("other", "203.0.113.9", now.Add(3*time.Second)) {
		t.Fatal("expected different username to have a separate login window")
	}
}

// failingQueryDriver always returns a query error so DB limiter fallback can be tested.
var failingQueryDriverRegisterOnce sync.Once

type failingQueryDriver struct{}

type failingQueryConn struct{}

func (failingQueryDriver) Open(string) (driver.Conn, error) { return failingQueryConn{}, nil }
func (failingQueryConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("prepare unavailable")
}
func (failingQueryConn) Close() error              { return nil }
func (failingQueryConn) Begin() (driver.Tx, error) { return nil, errors.New("tx unavailable") }
func (failingQueryConn) QueryContext(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
	return nil, errors.New("db limiter unavailable")
}

var publicSiteAssetTestDriverRegisterOnce sync.Once

type publicSiteAssetTestDriver struct{}

type publicSiteAssetTestConn struct {
	config []byte
}

func (publicSiteAssetTestDriver) Open(config string) (driver.Conn, error) {
	return publicSiteAssetTestConn{config: []byte(config)}, nil
}
func (publicSiteAssetTestConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("prepare unavailable")
}
func (publicSiteAssetTestConn) Close() error              { return nil }
func (publicSiteAssetTestConn) Begin() (driver.Tx, error) { return nil, errors.New("tx unavailable") }
func (c publicSiteAssetTestConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if strings.Contains(query, "FROM site_configs") {
		return &publicSiteAssetTestRows{
			columns: []string{"config"},
			values:  []driver.Value{c.config},
		}, nil
	}
	if strings.Contains(query, "FROM upload_assets") {
		id, ok := args[0].Value.(int64)
		if !ok || (id != 42 && id != 43) {
			return nil, errors.New("unexpected upload asset id")
		}
		return &publicSiteAssetTestRows{
			columns: []string{"id", "key", "name", "content_type", "size", "data", "object_key", "object_url"},
			values:  []driver.Value{id, "upload-assets/" + strconv.FormatInt(id, 10), "carousel.png", "image/png", int64(len("carousel-image")), []byte("carousel-image"), "", ""},
		}, nil
	}
	return nil, errors.New("unexpected query: " + query)
}

type publicSiteAssetTestRows struct {
	columns []string
	values  []driver.Value
	done    bool
}

func (r *publicSiteAssetTestRows) Columns() []string { return r.columns }
func (r *publicSiteAssetTestRows) Close() error      { return nil }
func (r *publicSiteAssetTestRows) Next(dest []driver.Value) error {
	if r.done {
		return io.EOF
	}
	copy(dest, r.values)
	r.done = true
	return nil
}

func TestBackendLoginRateLimitFallsBackToMemoryWhenDBLimiterFails(t *testing.T) {
	driverName := "nine_xing_failing_limiter"
	failingQueryDriverRegisterOnce.Do(func() {
		sql.Register(driverName, failingQueryDriver{})
	})
	db, err := sql.Open(driverName, "")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	s := &Server{
		db:             db,
		loginLimiter:   newStrRateLimiter(2, time.Minute),
		loginDBLimiter: newDBRateLimiter(db, "admin_login", 2, time.Minute),
	}
	now := time.Unix(200, 0)

	if !s.allowLoginAttempt("admin", "203.0.113.10", now) {
		t.Fatal("expected first attempt to fall back to memory limiter and be allowed")
	}
	if !s.allowLoginAttempt("admin", "203.0.113.10", now.Add(time.Second)) {
		t.Fatal("expected second attempt to fall back to memory limiter and be allowed")
	}
	if s.allowLoginAttempt("admin", "203.0.113.10", now.Add(2*time.Second)) {
		t.Fatal("expected fallback memory limiter to block the third attempt")
	}
}

func TestPublicArticleAssetURLRewritesPrivatePreviewURL(t *testing.T) {
	if got := publicArticleAssetURL("/api/upload-assets/42"); got != "/api/public/article-assets/42" {
		t.Fatalf("expected public article asset URL, got %q", got)
	}
	if got := publicArticleAssetURL("/api/uploads/article/cover.png"); got != "/api/public/article-uploads/article/cover.png" {
		t.Fatalf("expected public article upload URL, got %q", got)
	}
	if got := publicArticleAssetURL("https://cdn.example.com/api/upload-assets/42"); got != "https://cdn.example.com/api/upload-assets/42" {
		t.Fatalf("expected external absolute upload-like URL to stay unchanged, got %q", got)
	}
	if got := publicArticleAssetURL("https://cdn.example.com/api/uploads/article/cover.png"); got != "https://cdn.example.com/api/uploads/article/cover.png" {
		t.Fatalf("expected external absolute local-upload-like URL to stay unchanged, got %q", got)
	}
	if got := publicArticleAssetURL("https://cdn.example.com/cover.png"); got != "https://cdn.example.com/cover.png" {
		t.Fatalf("expected public CDN URL to stay unchanged, got %q", got)
	}
}

func TestPublicArticleContentAssetURLsRewriteEmbeddedPrivateUploadURLs(t *testing.T) {
	content := strings.Join([]string{
		"![asset](/api/upload-assets/42)",
		"![local](/api/uploads/article/body.png)",
		"<img src=\"/api/upload-assets/43\">",
		"![external](https://cdn.example.com/api/upload-assets/44)",
		"![already](/api/public/article-assets/45)",
	}, "\n")

	got := publicArticleContentAssetURLs(content)

	for _, expected := range []string{
		"![asset](/api/public/article-assets/42)",
		"![local](/api/public/article-uploads/article/body.png)",
		"<img src=\"/api/public/article-assets/43\">",
		"![external](https://cdn.example.com/api/upload-assets/44)",
		"![already](/api/public/article-assets/45)",
	} {
		if !strings.Contains(got, expected) {
			t.Fatalf("expected rewritten article content to contain %q, got:\n%s", expected, got)
		}
	}
	if strings.Contains(got, "(/api/upload-assets/42)") || strings.Contains(got, "(/api/uploads/article/body.png)") || strings.Contains(got, `"/api/upload-assets/43"`) {
		t.Fatalf("expected private upload URLs to be rewritten, got:\n%s", got)
	}
}

func TestPublicArticleReferenceCacheIsBounded(t *testing.T) {
	previousCache := publicArticleReferenceCache
	t.Cleanup(func() {
		publicArticleReferenceCache = previousCache
	})
	publicArticleReferenceCache = newPublicArticleReferenceCacheStore()
	for i := 0; i < publicArticleReferenceCacheMaxEntries+50; i++ {
		key := "miss-random-key-" + strconv.Itoa(i)
		if _, err := cachedPublicArticleReference(key, func() (bool, error) {
			return false, nil
		}); err != nil {
			t.Fatalf("cache reference %s: %v", key, err)
		}
	}
	if got := publicArticleReferenceCache.Len(); got > publicArticleReferenceCacheMaxEntries {
		t.Fatalf("public article reference cache grew beyond limit: got %d want <= %d", got, publicArticleReferenceCacheMaxEntries)
	}
}

func TestClearPublicArticleReferenceCacheRemovesStaleEntries(t *testing.T) {
	previousCache := publicArticleReferenceCache
	t.Cleanup(func() {
		publicArticleReferenceCache = previousCache
	})
	publicArticleReferenceCache = newPublicArticleReferenceCacheStore()
	if _, err := cachedPublicArticleReference("asset:123", func() (bool, error) {
		return true, nil
	}); err != nil {
		t.Fatalf("cache reference: %v", err)
	}
	if got := publicArticleReferenceCache.Len(); got != 1 {
		t.Fatalf("expected one cached entry before clear, got %d", got)
	}

	clearPublicArticleReferenceCache()

	if got := publicArticleReferenceCache.Len(); got != 0 {
		t.Fatalf("expected article reference cache to be cleared after content mutation, got %d", got)
	}
}

func TestWritePublicUploadAssetForcesDownloadForUnsafeInlineMIME(t *testing.T) {
	recorder := httptest.NewRecorder()
	writePublicUploadAsset(recorder, uploadasset.Asset{
		ContentType: "text/html; charset=utf-8",
		Data:        []byte("<script>alert(1)</script>"),
	})

	if got := recorder.Header().Get("Content-Type"); got != "application/octet-stream" {
		t.Fatalf("unsafe inline content type should be normalized, got %q", got)
	}
	if got := recorder.Header().Get("Content-Disposition"); !strings.Contains(got, "attachment") {
		t.Fatalf("unsafe inline content should be served as attachment, got %q", got)
	}
}

func TestWritePublicUploadAssetAllowsSafeImageMIMEInline(t *testing.T) {
	recorder := httptest.NewRecorder()
	writePublicUploadAsset(recorder, uploadasset.Asset{
		ContentType: "image/png",
		Data:        []byte("png"),
	})

	if got := recorder.Header().Get("Content-Type"); got != "image/png" {
		t.Fatalf("safe image content type should be preserved, got %q", got)
	}
	if got := recorder.Header().Get("Content-Disposition"); got != "" {
		t.Fatalf("safe image should not force attachment, got %q", got)
	}
}

func TestServePublicLocalUploadForcesDownloadForUnsafeMIME(t *testing.T) {
	uploadDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(uploadDir, "logo.svg"), []byte(`<svg><script>alert(1)</script></svg>`), 0o644); err != nil {
		t.Fatal(err)
	}
	s := &Server{env: config.Env{UploadDir: uploadDir}}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/public/site-uploads/logo.svg", nil)

	s.servePublicLocalUpload(recorder, request, "logo.svg")

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected unsafe referenced local upload to be served as download, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if got := recorder.Header().Get("Content-Type"); got != "application/octet-stream" {
		t.Fatalf("unsafe local upload content type should be normalized, got %q", got)
	}
	if got := recorder.Header().Get("Content-Disposition"); !strings.Contains(got, "attachment") {
		t.Fatalf("unsafe local upload should be served as attachment, got %q", got)
	}
}

func TestPublicSiteAssetURLRewritesPrivateUploadURLs(t *testing.T) {
	if got := publicSiteAssetURL("/api/upload-assets/42"); got != "/api/public/site-assets/42" {
		t.Fatalf("expected public site asset URL, got %q", got)
	}
	if got := publicSiteAssetURL("/api/uploads/site/logo.png"); got != "/api/public/site-uploads/site/logo.png" {
		t.Fatalf("expected public site upload URL, got %q", got)
	}
	if got := publicSiteAssetURL("https://cdn.example.com/api/upload-assets/42"); got != "https://cdn.example.com/api/upload-assets/42" {
		t.Fatalf("expected external absolute upload-like URL to stay unchanged, got %q", got)
	}
	if got := publicSiteAssetURL("https://cdn.example.com/api/uploads/site/logo.png"); got != "https://cdn.example.com/api/uploads/site/logo.png" {
		t.Fatalf("expected external absolute local-upload-like URL to stay unchanged, got %q", got)
	}
	if got := publicSiteAssetURL("https://cdn.example.com/logo.png"); got != "https://cdn.example.com/logo.png" {
		t.Fatalf("expected public CDN URL to stay unchanged, got %q", got)
	}
}

func TestPublicAdminBrandingAssetURLRewritesPrivateUploadURLs(t *testing.T) {
	if got := publicAdminBrandingAssetURL("/api/upload-assets/7"); got != "/api/public/admin-branding-assets/7" {
		t.Fatalf("expected public branding asset URL, got %q", got)
	}
	if got := publicAdminBrandingAssetURL("/api/uploads/branding/logo.png"); got != "/api/public/admin-branding-uploads/branding/logo.png" {
		t.Fatalf("expected public branding upload URL, got %q", got)
	}
}

func TestPublicAssetReferenceChecksAcceptAlreadyPublicURLs(t *testing.T) {
	if !valueReferencesUploadAsset("/api/public/site-assets/42", 42) {
		t.Fatal("expected already-public site asset URL to count as an asset reference")
	}
	if !valueReferencesUploadAsset("/api/public/admin-branding-assets/42", 42) {
		t.Fatal("expected already-public branding asset URL to count as an asset reference")
	}
	if !valueReferencesLocalUpload("/api/public/site-uploads/site/logo.png", "/api/uploads/site/logo.png") {
		t.Fatal("expected already-public site upload URL to count as a local upload reference")
	}
	if !valueReferencesLocalUpload("/api/public/admin-branding-uploads/branding/logo.png", "/api/uploads/branding/logo.png") {
		t.Fatal("expected already-public branding upload URL to count as a local upload reference")
	}
}

func TestPublicSiteConfigRewritesAndServesReferencedLocalUpload(t *testing.T) {
	root := t.TempDir()
	uploadDir := filepath.Join(root, "uploads")
	if err := os.MkdirAll(filepath.Join(uploadDir, "site"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(uploadDir, "site", "logo.png"), []byte("logo"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(uploadDir, "site", "other.png"), []byte("other"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(uploadDir, "site", "customer-qr.png"), []byte("qr"), 0o644); err != nil {
		t.Fatal(err)
	}
	sitePath := filepath.Join(root, "site-config.json")
	if err := os.WriteFile(sitePath, []byte(`{
  "home": {"teacherTeaser": {"image": "/api/uploads/site/logo.png"}},
  "navigation": {
    "main": [{"label": "首页", "to": "/", "type": "route"}],
    "drawer": [{"label": "首页", "to": "/", "type": "route"}],
    "tabs": [{"label": "首页", "to": "/", "type": "route", "icon": "home", "match": "/"}]
  },
  "site": {"brandName": "九型芯之力", "copyright": "", "footerTagline": "", "logo": "/api/uploads/site/logo.png", "customerServiceQr": "/api/uploads/site/customer-qr.png"},
  "types": [{"id": "1", "name": "完美型", "description": "", "keywords": "", "avatar": "/assets/avatars/1.webp"}]
}`), 0o644); err != nil {
		t.Fatal(err)
	}
	handler := New(config.Env{JWTSecret: "test-secret", SiteConfig: sitePath, UploadDir: uploadDir}, nil)

	configResp := performRawUnit(handler, http.MethodGet, "/api/public/site-config", "")
	if configResp.Code != http.StatusOK {
		t.Fatalf("expected site config 200, got %d body=%s", configResp.Code, configResp.Body.String())
	}
	if body := configResp.Body.String(); !strings.Contains(body, "/api/public/site-uploads/site/logo.png") || !strings.Contains(body, "/api/public/site-uploads/site/customer-qr.png") || strings.Contains(body, "/api/uploads/site/") {
		t.Fatalf("expected public site config to rewrite local upload URLs, body=%s", body)
	}

	assetResp := performRawUnit(handler, http.MethodGet, "/api/public/site-uploads/site/logo.png", "")
	if assetResp.Code != http.StatusOK || assetResp.Body.String() != "logo" {
		t.Fatalf("expected referenced public upload 200, got %d body=%s", assetResp.Code, assetResp.Body.String())
	}
	qrResp := performRawUnit(handler, http.MethodGet, "/api/public/site-uploads/site/customer-qr.png", "")
	if qrResp.Code != http.StatusOK || qrResp.Body.String() != "qr" {
		t.Fatalf("expected referenced customer QR public upload 200, got %d body=%s", qrResp.Code, qrResp.Body.String())
	}
	proxyResp := performRawUnit(handler, http.MethodGet, "/api/public/customer-service-qr", "")
	if proxyResp.Code != http.StatusOK || proxyResp.Body.String() != "qr" {
		t.Fatalf("expected customer QR proxy 200, got %d body=%s", proxyResp.Code, proxyResp.Body.String())
	}
	unreferencedResp := performRawUnit(handler, http.MethodGet, "/api/public/site-uploads/site/other.png", "")
	if unreferencedResp.Code != http.StatusNotFound {
		t.Fatalf("expected unreferenced public upload 404, got %d", unreferencedResp.Code)
	}
}

func TestPublicSiteConfigExposesMiniappCarouselAssetThroughPublicURL(t *testing.T) {
	root := t.TempDir()
	sitePath := filepath.Join(root, "site-config.json")
	configBody := []byte(`{
  "home": {
    "miniappCarousel": {
      "items": [{"image": "/api/upload-assets/42"}]
    }
  },
  "navigation": {
    "main": [{"label": "首页", "to": "/", "type": "route"}],
    "drawer": [{"label": "首页", "to": "/", "type": "route"}],
    "tabs": [{"label": "首页", "to": "/", "type": "route", "icon": "home", "match": "/"}]
  },
  "site": {"brandName": "九型芯之力", "copyright": "", "footerTagline": "", "logo": "/assets/logo.svg"},
  "types": [{"id": "1", "name": "完美型", "description": "", "keywords": "", "avatar": "/assets/avatars/1.webp"}]
}`)
	if err := os.WriteFile(sitePath, configBody, 0o644); err != nil {
		t.Fatal(err)
	}

	publicSiteAssetTestDriverRegisterOnce.Do(func() {
		sql.Register("nine_xing_public_site_asset", publicSiteAssetTestDriver{})
	})
	database, err := sql.Open("nine_xing_public_site_asset", string(configBody))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })

	handler := New(config.Env{JWTSecret: "test-secret", SiteConfig: sitePath}, database)

	configResp := performRawUnit(handler, http.MethodGet, "/api/public/site-config", "")
	if configResp.Code != http.StatusOK {
		t.Fatalf("expected public site config 200, got %d body=%s", configResp.Code, configResp.Body.String())
	}
	if body := configResp.Body.String(); !strings.Contains(body, "/api/public/site-assets/42") || strings.Contains(body, "/api/upload-assets/42") {
		t.Fatalf("expected carousel image to use only the public asset URL, body=%s", body)
	}

	assetResp := performRawUnit(handler, http.MethodGet, "/api/public/site-assets/42", "")
	if assetResp.Code != http.StatusOK {
		t.Fatalf("expected referenced public carousel asset 200, got %d body=%s", assetResp.Code, assetResp.Body.String())
	}
	if got := assetResp.Body.String(); got != "carousel-image" {
		t.Fatalf("expected carousel image bytes, got %q", got)
	}
	if got := assetResp.Header().Get("Content-Type"); got != "image/png" {
		t.Fatalf("expected carousel image content type image/png, got %q", got)
	}

	unreferencedResp := performRawUnit(handler, http.MethodGet, "/api/public/site-assets/43", "")
	if unreferencedResp.Code != http.StatusNotFound {
		t.Fatalf("expected unreferenced asset to return 404, got %d body=%s", unreferencedResp.Code, unreferencedResp.Body.String())
	}
}

func TestPublicAdminBrandingRewritesAndServesReferencedLocalUpload(t *testing.T) {
	root := t.TempDir()
	uploadDir := filepath.Join(root, "uploads")
	if err := os.MkdirAll(filepath.Join(uploadDir, "branding"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(uploadDir, "branding", "logo.png"), []byte("brand"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(uploadDir, "branding", "other.png"), []byte("other"), 0o644); err != nil {
		t.Fatal(err)
	}
	brandingPath := filepath.Join(root, "admin-branding.json")
	if err := os.WriteFile(brandingPath, []byte(`{"name":"后台","logo":"/api/uploads/branding/logo.png","loadingText":""}`), 0o644); err != nil {
		t.Fatal(err)
	}
	handler := New(config.Env{JWTSecret: "test-secret", AdminConfig: brandingPath, UploadDir: uploadDir}, nil)

	brandingResp := performRawUnit(handler, http.MethodGet, "/api/public/admin-branding", "")
	if brandingResp.Code != http.StatusOK {
		t.Fatalf("expected admin branding 200, got %d body=%s", brandingResp.Code, brandingResp.Body.String())
	}
	if body := brandingResp.Body.String(); !strings.Contains(body, "/api/public/admin-branding-uploads/branding/logo.png") || strings.Contains(body, "/api/uploads/branding/logo.png") {
		t.Fatalf("expected public branding to rewrite local upload URL, body=%s", body)
	}

	assetResp := performRawUnit(handler, http.MethodGet, "/api/public/admin-branding-uploads/branding/logo.png", "")
	if assetResp.Code != http.StatusOK || assetResp.Body.String() != "brand" {
		t.Fatalf("expected referenced branding upload 200, got %d body=%s", assetResp.Code, assetResp.Body.String())
	}
	unreferencedResp := performRawUnit(handler, http.MethodGet, "/api/public/admin-branding-uploads/branding/other.png", "")
	if unreferencedResp.Code != http.StatusNotFound {
		t.Fatalf("expected unreferenced branding upload 404, got %d", unreferencedResp.Code)
	}
}

func performRawUnit(handler http.Handler, method, path, payload string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func TestValidateModelConfigBasesRejectsLocalAddresses(t *testing.T) {
	for _, apiBase := range []string{
		"http://127.0.0.1:8080",
		"http://localhost.:8080",
		"http://foo.localhost:8080",
	} {
		err := validateModelConfigBases(modelconfig.Config{
			Chat:     modelconfig.ChatConfig{APIBase: apiBase},
			Analysis: modelconfig.AnalysisConfig{APIBase: apiBase},
		})
		if err == nil {
			t.Fatalf("expected local model api base %q to be rejected", apiBase)
		}
	}
}

func TestValidateModelConfigBasesRejectsLocalAnalysisAddress(t *testing.T) {
	err := validateModelConfigBases(modelconfig.Config{
		Analysis: modelconfig.AnalysisConfig{APIBase: "http://127.0.0.1:8080"},
	})
	if err == nil {
		t.Fatal("expected local analysis api base to be rejected")
	}
}

func TestValidateModelConfigBasesAllowsPublicHTTPS(t *testing.T) {
	err := validateModelConfigBases(modelconfig.Config{
		Chat:      modelconfig.ChatConfig{APIBase: "https://api.minimaxi.com"},
		Image:     modelconfig.ImageConfig{APIBase: "https://image-gateway.example.com/v1"},
		Video:     modelconfig.VideoConfig{APIBase: "https://video-gateway.example.com/v1"},
		Analysis:  modelconfig.AnalysisConfig{APIBase: "https://api.minimaxi.com"},
		DailyQuiz: modelconfig.CompatibleModelConfig{APIBase: "https://quiz-gateway.example.com/v1"},
	})
	if err != nil {
		t.Fatalf("expected public model api bases to pass, got %v", err)
	}
}

func TestValidateModelConfigBasesRejectsLocalDailyQuizAddress(t *testing.T) {
	err := validateModelConfigBases(modelconfig.Config{
		DailyQuiz: modelconfig.CompatibleModelConfig{APIBase: "http://127.0.0.1:8080"},
	})
	if err == nil {
		t.Fatal("expected local daily quiz api base to be rejected")
	}
}

func TestBuildModelConfigViewIncludesDailyQuizConfig(t *testing.T) {
	admin := modelconfig.AdminModelConfig{
		Provider:       modelconfig.ProviderOpenAICompatible,
		APIBase:        "https://admin.example.com/v1",
		APIKey:         "admin-secret",
		Model:          "gpt-admin",
		TimeoutSeconds: 31,
	}
	dailyQuiz := modelconfig.CompatibleModelConfig{
		Provider:       modelconfig.ProviderAnthropicCompatible,
		APIBase:        "https://quiz.example.com/v1",
		APIKey:         "quiz-secret",
		Model:          "claude-quiz",
		TimeoutSeconds: 52,
	}

	view := buildModelConfigView(config.MiniMaxConfig{}, config.VideoConfig{}, config.ImageConfig{}, config.MiniMaxConfig{}, admin, dailyQuiz, modelconfig.Config{})

	if view.DailyQuiz.Provider != modelconfig.ProviderAnthropicCompatible || view.DailyQuiz.APIBase != "https://quiz.example.com/v1" || view.DailyQuiz.Model != "claude-quiz" || view.DailyQuiz.TimeoutSeconds != 52 {
		t.Fatalf("expected daily quiz model config in view, got %+v", view.DailyQuiz)
	}
	if !view.DailyQuiz.APIKeySet {
		t.Fatal("expected daily quiz apiKeySet to reflect configured secret")
	}
}

func TestBuildModelConfigViewIncludesCompatibleChatProviderWithoutGroupID(t *testing.T) {
	chat := config.MiniMaxConfig{
		Provider: modelconfig.ProviderAnthropicCompatible,
		APIBase:  "https://coding-play.codes",
		APIKey:   "chat-secret",
		Model:    "claude-sonnet-4-5",
		GroupID:  "legacy-group",
	}

	view := buildModelConfigView(chat, config.VideoConfig{}, config.ImageConfig{}, config.MiniMaxConfig{}, modelconfig.AdminModelConfig{}, modelconfig.CompatibleModelConfig{}, modelconfig.Config{})

	if view.Chat.Provider != modelconfig.ProviderAnthropicCompatible || view.Chat.APIBase != "https://coding-play.codes" || view.Chat.Model != "claude-sonnet-4-5" || !view.Chat.APIKeySet {
		t.Fatalf("unexpected compatible chat view: %+v", view.Chat)
	}
}

func TestNewChatGeneratorUsesCompatibleProtocolAdapter(t *testing.T) {
	generator := newChatGenerator(config.MiniMaxConfig{Provider: modelconfig.ProviderOpenAICompatible})
	if _, ok := generator.(*llm.CompatibleChatGenerator); !ok {
		t.Fatalf("chat generator type = %T", generator)
	}
}

func TestAnalysisVideoURLUsesSignedObjectURLWhenAvailable(t *testing.T) {
	signer := &recordingObjectSigner{url: "https://cdn.example.com/private.mp4?signature=ok"}
	s := &Server{uploader: signer}
	job := uploadasset.Asset{
		ObjectKey: "uploads/video/analysis/demo.mp4",
		ObjectURL: "https://cdn.example.com/uploads/video/analysis/demo.mp4",
	}

	got := s.analysisVideoURL(context.Background(), job)

	if got != signer.url {
		t.Fatalf("expected signed url, got %q", got)
	}
	if signer.objectKey != job.ObjectKey || signer.expires != 30*time.Minute {
		t.Fatalf("unexpected presign call: key=%q expires=%s", signer.objectKey, signer.expires)
	}
}

func TestAnalysisVideoURLFallsBackToObjectURLWithoutSigner(t *testing.T) {
	s := &Server{}
	job := uploadasset.Asset{
		ObjectKey: "uploads/video/analysis/demo.mp4",
		ObjectURL: "https://cdn.example.com/uploads/video/analysis/demo.mp4",
	}

	got := s.analysisVideoURL(context.Background(), job)

	if got != job.ObjectURL {
		t.Fatalf("expected object url fallback, got %q", got)
	}
}

func TestAnalysisJobVideoURLFallsBackToStoredURLWithoutAssetStore(t *testing.T) {
	s := &Server{}
	job := videoanalysis.Job{
		VideoAssetID: "12",
		VideoURL:     "https://cdn.example.com/uploads/video/analysis/demo.mp4",
	}

	got := s.analysisJobVideoURL(context.Background(), job)

	if got != job.VideoURL {
		t.Fatalf("expected stored video url fallback, got %q", got)
	}
}

func TestBackfillUploadAssetObjectURLReuploadsLegacyAsset(t *testing.T) {
	uploader := &recordingUploadResultUploader{
		result: storage.UploadResult{
			Key:         "upload/video/20260701/demo.png",
			URL:         "https://bucket.example.com/upload/video/20260701/demo.png",
			Name:        "demo.png",
			ContentType: "image/png",
			Size:        5,
		},
	}
	updater := &fakeUploadAssetObjectUpdater{}
	asset := uploadasset.Asset{
		ID:          34,
		Key:         "upload-assets/34",
		Name:        "demo.png",
		ContentType: "image/png",
		Data:        []byte("image"),
		ObjectKey:   "",
		ObjectURL:   "",
		Size:        5,
	}

	got, err := backfillUploadAssetObjectURL(context.Background(), updater, uploader, 34, asset)
	if err != nil {
		t.Fatalf("backfillUploadAssetObjectURL returned error: %v", err)
	}
	if got != uploader.result.URL {
		t.Fatalf("expected backfilled object url %q, got %q", uploader.result.URL, got)
	}
	if uploader.dir != "video/reference" || uploader.name != "demo.png" || uploader.content != "image" {
		t.Fatalf("unexpected upload input dir=%q name=%q content=%q", uploader.dir, uploader.name, uploader.content)
	}
	if updater.updatedID != 34 || updater.updatedKey != uploader.result.Key || updater.updatedURL != uploader.result.URL {
		t.Fatalf("expected upload asset object metadata to be backfilled, got id=%d key=%q url=%q", updater.updatedID, updater.updatedKey, updater.updatedURL)
	}
}

type recordingObjectSigner struct {
	expires   time.Duration
	objectKey string
	url       string
}

func (s *recordingObjectSigner) Upload(context.Context, storage.UploadInput) (storage.UploadResult, error) {
	return storage.UploadResult{}, nil
}

func (s *recordingObjectSigner) PresignGetURL(_ context.Context, objectKey string, expires time.Duration) (string, error) {
	s.objectKey = objectKey
	s.expires = expires
	return s.url, nil
}

type recordingUploadResultUploader struct {
	content string
	dir     string
	name    string
	result  storage.UploadResult
}

func (u *recordingUploadResultUploader) Upload(_ context.Context, input storage.UploadInput) (storage.UploadResult, error) {
	data, err := io.ReadAll(input.Reader)
	if err != nil {
		return storage.UploadResult{}, err
	}
	u.content = string(data)
	u.dir = input.Dir
	u.name = input.Filename
	return u.result, nil
}

type fakeUploadAssetObjectUpdater struct {
	updatedID  int64
	updatedKey string
	updatedURL string
}

func (s *fakeUploadAssetObjectUpdater) UpdateObjectMetadata(_ context.Context, id int64, objectKey string, objectURL string) error {
	s.updatedID = id
	s.updatedKey = objectKey
	s.updatedURL = objectURL
	return nil
}

func TestAuthorizePreservesTokenVersionForRefresh(t *testing.T) {
	source, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	start := strings.Index(text, "func (s *Server) authorizeAuthorization")
	if start < 0 {
		t.Fatal("authorizeAuthorization not found")
	}
	end := strings.Index(text[start:], "func mustSMSSender")
	if end < 0 {
		t.Fatal("authorizeAuthorization end not found")
	}
	body := text[start : start+end]
	if !strings.Contains(body, "TokenKind:    auth.TokenKindBackend") || !strings.Contains(body, "TokenVersion: currentVersion") {
		t.Fatalf("authorizeAuthorization must preserve TokenKind and TokenVersion in request context so refresh signs usable tokens; body=%s", body)
	}
}

func TestAdminBrandingPrivateEndpointRequiresPermission(t *testing.T) {
	source, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(source), `s.mux.HandleFunc("/api/admin-branding", s.requirePermission("System:Branding", s.adminBranding))`) {
		t.Fatal("/api/admin-branding private endpoint must require System:Branding; login screen should use /api/public/admin-branding")
	}
}

func TestCrossModuleHelperEndpointsAllowOwningPagePermissions(t *testing.T) {
	source, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	expectRoutes := []string{
		`s.mux.HandleFunc("/api/voice/profiles/list", s.method(http.MethodGet, s.requireAnyPermission([]string{"Voice:Profile:Manage", "Voice:Test:Manage"}, s.voiceProfiles)))`,
		`s.mux.HandleFunc("/api/voice/options", s.method(http.MethodGet, s.requireAnyPermission([]string{"Voice:Profile:Manage", "Voice:Test:Manage", "Voice:Content:Manage", "Reading:Article:Manage"}, s.voiceOptions)))`,
		`s.mux.HandleFunc("/api/video/analysis/list", s.method(http.MethodGet, s.requireAnyPermission([]string{"Video:Analysis:Manage", "Video:Storyboard:Manage"}, s.videoAnalysisList)))`,
		`s.mux.HandleFunc("/api/video/assets/polish-prompt", s.method(http.MethodPost, s.requireAnyPermission([]string{"Video:Asset:Manage", "Video:Generate:Manage"}, s.polishPrompt)))`,
	}
	for _, route := range expectRoutes {
		if !strings.Contains(text, route) {
			t.Fatalf("expected helper route permission to include owning page permission: %s", route)
		}
	}
}

func TestNewAdminHelperEndpointsAllowOwningPagePermissions(t *testing.T) {
	source, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	if !strings.Contains(text, `s.mux.HandleFunc("/api/site-config/build-status", s.method(http.MethodGet, s.requirePermission("Website:Write", s.siteBuildStatus)))`) {
		t.Fatal("build status exposes build logs and must require Website:Write")
	}
	expectRoutes := []string{
		`s.mux.HandleFunc("/api/quiz/cards", s.method(http.MethodGet, s.requireAnyPermission([]string{"Website:Read", "Customer:UserInsights:List"}, s.adminQuizCards)))`,
	}
	for _, route := range expectRoutes {
		if !strings.Contains(text, route) {
			t.Fatalf("expected page helper route permission: %s", route)
		}
	}
}

func TestLoginUsesAccessibleDefaultHomePath(t *testing.T) {
	source, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(source), `HomePath:  "/dashboard/analytics"`) {
		t.Fatal("login must not hard-code /dashboard/analytics for every role")
	}
	if !strings.Contains(string(source), "DefaultHomePathForUser") {
		t.Fatal("login should use system.DefaultHomePathForUser")
	}
}
