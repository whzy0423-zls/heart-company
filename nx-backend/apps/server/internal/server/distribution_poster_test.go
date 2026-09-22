package server

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"nine-xing/nx-backend/apps/server/internal/config"
	"nine-xing/nx-backend/apps/server/internal/uploadasset"
)

func TestAppDistributionPosterAssetURLRewritesPrivateUploads(t *testing.T) {
	if got := appDistributionPosterAssetURL("/api/upload-assets/23"); got != distributionPosterAssetPrefix+"23" {
		t.Fatalf("asset URL=%q, want %q", got, distributionPosterAssetPrefix+"23")
	}
	if got := appDistributionPosterAssetURL("/api/uploads/posters/default.png"); got != distributionPosterUploadPrefix+"posters/default.png" {
		t.Fatalf("local upload URL=%q, want %q", got, distributionPosterUploadPrefix+"posters/default.png")
	}
	if got := appDistributionPosterAssetURL("https://cdn.example.com/poster.png"); got != "https://cdn.example.com/poster.png" {
		t.Fatalf("external URL=%q", got)
	}
}

func TestPosterConfigReferencesOnlyEnabledTemplates(t *testing.T) {
	active := defaultPosterTemplate()
	active.TemplateURL = "/api/upload-assets/23"
	active.QRImageURL = "/api/uploads/posters/active-qr.png"
	disabled := active
	disabled.ID = "disabled"
	disabled.Enabled = false
	disabled.TemplateURL = "/api/upload-assets/99"
	disabled.QRImageURL = "/api/uploads/posters/disabled-qr.png"
	cfg := posterConfig{Templates: []posterTemplate{active, disabled}}

	if !posterConfigReferencesUploadAsset(cfg, 23) {
		t.Fatal("active template asset should be exposed")
	}
	if posterConfigReferencesUploadAsset(cfg, 99) {
		t.Fatal("disabled template asset must stay private")
	}
	if !posterConfigReferencesLocalUpload(cfg, "/api/uploads/posters/active-qr.png") {
		t.Fatal("active template local upload should be exposed")
	}
	if posterConfigReferencesLocalUpload(cfg, "/api/uploads/posters/disabled-qr.png") {
		t.Fatal("disabled template local upload must stay private")
	}
}

func TestPosterConfigValidation(t *testing.T) {
	base := defaultPosterConfig()
	base.TemplateURL = "/api/upload-assets/1"
	base.LandingURL = "https://example.com/fixed"
	if err := validatePosterConfig(base); err != nil {
		t.Fatal(err)
	}
	cases := map[string]func(*posterConfig){
		"QR out of bounds":     func(c *posterConfig) { c.QRX = 719 },
		"invite out of bounds": func(c *posterConfig) { c.InviteY = 1279 },
		"invalid font":         func(c *posterConfig) { c.InviteFontSize = 100 },
		"missing template":     func(c *posterConfig) { c.TemplateURL = "" },
		"missing QR":           func(c *posterConfig) { c.LandingURL = "" },
		"script QR":            func(c *posterConfig) { c.LandingURL = "javascript:alert(1)" },
		"untrusted image":      func(c *posterConfig) { c.TemplateURL = "https://example.com/image.png" },
		"traversal":            func(c *posterConfig) { c.TemplateURL = "/api/uploads/../secret" },
		"oversize QR":          func(c *posterConfig) { c.QRSize = 221 },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			c := base
			mutate(&c)
			if validatePosterConfig(c) == nil {
				t.Fatal("accepted invalid configuration")
			}
		})
	}
	t.Run("fixed image without URL", func(t *testing.T) {
		c := base
		c.LandingURL = ""
		c.QRImageURL = "/api/upload-assets/2"
		if err := validatePosterConfig(c); err != nil {
			t.Fatal(err)
		}
	})
}

func TestPosterImageContentType(t *testing.T) {
	for _, tc := range []struct {
		name  string
		raw   string
		want  string
		valid bool
	}{
		{name: "png", raw: "image/png", want: "image/png", valid: true},
		{name: "with parameters", raw: "image/jpeg; charset=binary", want: "image/jpeg; charset=binary", valid: true},
		{name: "svg is still an image", raw: "image/svg+xml", want: "image/svg+xml", valid: true},
		{name: "video", raw: "video/mp4"},
		{name: "malformed", raw: "image/"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := posterImageContentType(tc.raw)
			if ok != tc.valid || got != tc.want {
				t.Fatalf("posterImageContentType(%q)=(%q,%v), want (%q,%v)", tc.raw, got, ok, tc.want, tc.valid)
			}
		})
	}
}

func TestAppDistributionPosterAssetRouteRequiresAppAuth(t *testing.T) {
	const secret = "distribution-poster-route-secret"
	s := &Server{env: config.Env{JWTSecret: secret}, mux: http.NewServeMux()}
	s.routes()
	response := httptest.NewRecorder()
	s.mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, distributionPosterAssetPrefix+"1", nil))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("poster asset route status=%d, want %d; body=%s", response.Code, http.StatusUnauthorized, response.Body.String())
	}
	response = httptest.NewRecorder()
	s.mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, distributionPosterUploadPrefix+"posters/default.png", nil))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("poster upload route status=%d, want %d; body=%s", response.Code, http.StatusUnauthorized, response.Body.String())
	}
}

func TestAppDistributionPosterAssetServesOnlyReferencedActiveImage(t *testing.T) {
	cfg := posterConfig{Templates: []posterTemplate{
		{ID: "active", Enabled: true, TemplateURL: "/api/upload-assets/42"},
		{ID: "disabled", Enabled: false, TemplateURL: "/api/upload-assets/43"},
	}}
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	publicSiteAssetTestDriverRegisterOnce.Do(func() {
		sql.Register("nine_xing_public_site_asset", publicSiteAssetTestDriver{})
	})
	db, err := sql.Open("nine_xing_public_site_asset", string(raw))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	s := &Server{db: db, uploads: uploadasset.NewStore(db)}
	response := httptest.NewRecorder()
	s.appDistributionPosterAsset(response, httptest.NewRequest(http.MethodGet, distributionPosterAssetPrefix+"42", nil))
	if response.Code != http.StatusOK || response.Body.String() != "carousel-image" {
		t.Fatalf("active asset status=%d body=%q", response.Code, response.Body.String())
	}
	if got := response.Header().Get("Content-Type"); got != "image/png" {
		t.Fatalf("active asset content type=%q", got)
	}

	response = httptest.NewRecorder()
	s.appDistributionPosterAsset(response, httptest.NewRequest(http.MethodGet, distributionPosterAssetPrefix+"43", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("disabled asset status=%d body=%q", response.Code, response.Body.String())
	}
}

func TestAppDistributionPosterUploadServesOnlyReferencedLocalImage(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "posters"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "posters", "active.png"), []byte("active-local"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "posters", "other.png"), []byte("other-local"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := posterConfig{Templates: []posterTemplate{
		{ID: "active", Enabled: true, TemplateURL: "/api/uploads/posters/active.png"},
	}}
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	publicSiteAssetTestDriverRegisterOnce.Do(func() {
		sql.Register("nine_xing_public_site_asset", publicSiteAssetTestDriver{})
	})
	db, err := sql.Open("nine_xing_public_site_asset", string(raw))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	s := &Server{db: db, env: config.Env{UploadDir: root}}
	response := httptest.NewRecorder()
	s.appDistributionPosterUpload(response, httptest.NewRequest(http.MethodGet, distributionPosterUploadPrefix+"posters/active.png", nil))
	if response.Code != http.StatusOK || response.Body.String() != "active-local" {
		t.Fatalf("active local status=%d body=%q", response.Code, response.Body.String())
	}

	response = httptest.NewRecorder()
	s.appDistributionPosterUpload(response, httptest.NewRequest(http.MethodGet, distributionPosterUploadPrefix+"posters/other.png", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("unreferenced local status=%d body=%q", response.Code, response.Body.String())
	}
}
