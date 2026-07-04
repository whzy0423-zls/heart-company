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

	"nine-xing/nx-backend/apps/server/internal/config"
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
	if got := publicArticleAssetURL("https://cdn.example.com/cover.png"); got != "https://cdn.example.com/cover.png" {
		t.Fatalf("expected public CDN URL to stay unchanged, got %q", got)
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
		Chat:     modelconfig.ChatConfig{APIBase: "https://api.minimaxi.com"},
		Image:    modelconfig.ImageConfig{APIBase: "https://image-gateway.example.com/v1"},
		Video:    modelconfig.VideoConfig{APIBase: "https://video-gateway.example.com/v1"},
		Analysis: modelconfig.AnalysisConfig{APIBase: "https://api.minimaxi.com"},
	})
	if err != nil {
		t.Fatalf("expected public model api bases to pass, got %v", err)
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
