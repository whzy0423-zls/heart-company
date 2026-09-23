package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"nine-xing/nx-backend/apps/server/internal/httpx"
)

const distributionPosterKey = "distribution_poster"
const defaultPosterLandingURL = "https://xn--9iq9az5uo8fz16d.com/app"

// The admin upload endpoint requires a backend JWT. The app distribution
// surface uses an app JWT, so poster assets are exposed through a dedicated
// route after the URL has been checked against the active poster config.
const (
	distributionPosterAssetPrefix  = "/api/app/distribution/poster-assets/"
	distributionPosterUploadPrefix = "/api/app/distribution/poster-uploads/"
)

type posterTemplate struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Enabled        bool   `json:"enabled"`
	SortOrder      int    `json:"sortOrder"`
	TemplateURL    string `json:"templateUrl"`
	Headline       string `json:"headline"`
	Subtitle       string `json:"subtitle"`
	CTA            string `json:"cta"`
	LandingURL     string `json:"landingUrl"`
	QRImageURL     string `json:"qrImageUrl"`
	QRSize         int    `json:"qrSize"`
	QRX            int    `json:"qrX"`
	QRY            int    `json:"qrY"`
	InviteX        int    `json:"inviteX"`
	InviteY        int    `json:"inviteY"`
	InviteWidth    int    `json:"inviteWidth"`
	InviteFontSize int    `json:"inviteFontSize"`
	ShowQRCode     bool   `json:"showQrCode"`
	ShowInviteCode bool   `json:"showInviteCode"`
	Status         string `json:"distributionStatus"`
}

type posterConfig struct {
	Templates []posterTemplate `json:"templates,omitempty"`
	// Legacy fields remain accepted and returned for older admin clients.
	posterTemplate
}

func defaultPosterConfig() posterConfig {
	template := defaultPosterTemplate()
	return posterConfig{Templates: []posterTemplate{template}, posterTemplate: template}
}

func defaultPosterTemplate() posterTemplate {
	return posterTemplate{ID: "default", Name: "默认海报", Enabled: true, SortOrder: 0, LandingURL: defaultPosterLandingURL, QRSize: 176, QRX: 62, QRY: 1010, InviteX: 286, InviteY: 1100, InviteWidth: 350, InviteFontSize: 26, ShowQRCode: true, ShowInviteCode: true, Status: "dispatched"}
}

func normalizePosterConfig(cfg posterConfig) posterConfig {
	if len(cfg.Templates) > 0 {
		cfg.Templates = append([]posterTemplate(nil), cfg.Templates...)
	}
	if len(cfg.Templates) == 0 && (cfg.TemplateURL != "" || cfg.LandingURL != "" || cfg.QRImageURL != "") {
		legacy := cfg.posterTemplate
		if legacy.ID == "" {
			legacy.ID = "default"
		}
		if legacy.Name == "" {
			legacy.Name = "默认海报"
		}
		legacy.Enabled = true
		cfg.Templates = []posterTemplate{legacy}
	}
	if len(cfg.Templates) == 1 && cfg.Templates[0].TemplateURL == "" && cfg.TemplateURL != "" {
		legacy := cfg.posterTemplate
		if legacy.ID == "" {
			legacy.ID = cfg.Templates[0].ID
		}
		if legacy.Name == "" {
			legacy.Name = cfg.Templates[0].Name
		}
		legacy.Enabled = true
		cfg.Templates[0] = legacy
	}
	if len(cfg.Templates) == 0 {
		cfg = defaultPosterConfig()
	}
	for i := range cfg.Templates {
		if cfg.Templates[i].ID == "" {
			cfg.Templates[i].ID = fmt.Sprintf("poster-%d", time.Now().UnixNano()+int64(i))
		}
		if cfg.Templates[i].Name == "" {
			cfg.Templates[i].Name = fmt.Sprintf("海报模板 %d", i+1)
		}
		if cfg.Templates[i].Status == "" {
			// Legacy configs predate visibility flags and therefore decode both
			// booleans as false. Keep their published appearance unchanged.
			if !cfg.Templates[i].ShowQRCode && !cfg.Templates[i].ShowInviteCode {
				cfg.Templates[i].ShowQRCode = true
				cfg.Templates[i].ShowInviteCode = true
			}
			cfg.Templates[i].Status = "dispatched"
		}
	}
	sort.SliceStable(cfg.Templates, func(i, j int) bool { return cfg.Templates[i].SortOrder < cfg.Templates[j].SortOrder })
	cfg.posterTemplate = cfg.Templates[0]
	return cfg
}

func validPosterImage(value string) bool {
	if value == "" {
		return true
	}
	// Persisted uploads are served with authentication; never fetch arbitrary URLs.
	for _, prefix := range []string{"/api/upload-assets/", "/api/uploads/"} {
		if strings.HasPrefix(value, prefix) && !strings.ContainsAny(value, "?#\\") && !strings.Contains(value, "..") {
			return len(value) > len(prefix) && len(value) <= 512
		}
	}
	return false
}

func validatePosterConfig(cfg posterConfig) error {
	cfg = normalizePosterConfig(cfg)
	if len(cfg.Templates) == 0 {
		return errors.New("请至少保留一个海报模板")
	}
	seen := map[string]bool{}
	active := 0
	for _, template := range cfg.Templates {
		if template.ID == "" || seen[template.ID] {
			return errors.New("海报模板 ID 不能为空且不能重复")
		}
		seen[template.ID] = true
		if template.Enabled {
			active++
		}
		if !validPosterImage(template.TemplateURL) || !validPosterImage(template.QRImageURL) {
			return errors.New("请选择上传的图片")
		}
		if template.TemplateURL == "" {
			return errors.New("请先上传海报模板")
		}
		if template.QRSize < 100 || template.QRSize > 220 {
			return errors.New("二维码尺寸应为 100–220")
		}
		if template.QRX < 0 || template.QRY < 0 || template.QRX+template.QRSize > 720 || template.QRY+template.QRSize > 1280 {
			return errors.New("二维码位置超出海报范围")
		}
		if template.InviteWidth < 120 || template.InviteWidth > 600 || template.InviteFontSize < 16 || template.InviteFontSize > 64 || template.InviteX < 0 || template.InviteY < 0 || template.InviteX+template.InviteWidth > 720 || template.InviteY+template.InviteFontSize+12 > 1280 {
			return errors.New("邀请码位置或字号超出海报范围")
		}
		if utf8.RuneCountInString(template.Headline) > 32 || utf8.RuneCountInString(template.Subtitle) > 80 || utf8.RuneCountInString(template.CTA) > 24 {
			return errors.New("海报文字超出长度限制")
		}
		if template.LandingURL != "" {
			u, err := url.Parse(template.LandingURL)
			if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || len(template.LandingURL) > 2048 {
				return errors.New("请输入有效的 HTTP/HTTPS 二维码链接")
			}
		}
		if template.QRImageURL == "" && template.LandingURL == "" {
			return errors.New("请上传二维码或设置二维码链接")
		}
	}
	if active == 0 {
		return errors.New("请至少启用一个海报模板")
	}
	return nil
}

func activePosterTemplates(cfg posterConfig) []posterTemplate {
	cfg = normalizePosterConfig(cfg)
	items := make([]posterTemplate, 0, len(cfg.Templates))
	for _, template := range cfg.Templates {
		if template.Enabled && strings.EqualFold(template.Status, "dispatched") {
			items = append(items, template)
		}
	}
	return items
}

func appDistributionPosterAssetURL(raw string) string {
	return publicConfigAssetURL(raw, distributionPosterAssetPrefix, distributionPosterUploadPrefix)
}

func posterConfigReferencesUploadAsset(cfg posterConfig, id int64) bool {
	if id <= 0 {
		return false
	}
	for _, template := range activePosterTemplates(cfg) {
		if valueReferencesUploadAsset(template.TemplateURL, id) ||
			valueReferencesUploadAsset(template.QRImageURL, id) {
			return true
		}
	}
	return false
}

func posterConfigReferencesLocalUpload(cfg posterConfig, privateURL string) bool {
	privateURL = strings.TrimSpace(privateURL)
	if privateURL == "" {
		return false
	}
	for _, template := range activePosterTemplates(cfg) {
		if valueReferencesLocalUpload(template.TemplateURL, privateURL) ||
			valueReferencesLocalUpload(template.QRImageURL, privateURL) {
			return true
		}
	}
	return false
}

func posterImageContentType(raw string) (string, bool) {
	contentType := strings.TrimSpace(raw)
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil || !strings.HasPrefix(strings.ToLower(mediaType), "image/") {
		return "", false
	}
	return contentType, true
}

func (s *Server) readDistributionPosterConfig(ctx context.Context) (posterConfig, error) {
	cfg := defaultPosterConfig()
	if s == nil || s.db == nil {
		return cfg, nil
	}
	var raw []byte
	err := s.db.QueryRowContext(ctx, "SELECT config FROM site_configs WHERE key=$1", distributionPosterKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return cfg, err
	}
	return normalizePosterConfig(cfg), nil
}

func (s *Server) appPosterTemplates(ctx context.Context) []posterTemplate {
	cfg, err := s.readDistributionPosterConfig(ctx)
	if err != nil {
		return nil
	}
	items := activePosterTemplates(cfg)
	for i := range items {
		items[i].TemplateURL = appDistributionPosterAssetURL(items[i].TemplateURL)
		items[i].QRImageURL = appDistributionPosterAssetURL(items[i].QRImageURL)
	}
	return items
}

// appDistributionPosterAsset serves only image upload assets referenced by an
// enabled distribution poster template. It intentionally does not expose the
// general upload preview endpoint to app tokens.
func (s *Server) appDistributionPosterAsset(w http.ResponseWriter, r *http.Request) {
	idText := strings.TrimPrefix(r.URL.Path, distributionPosterAssetPrefix)
	id, err := strconv.ParseInt(strings.Trim(idText, "/"), 10, 64)
	if err != nil || id <= 0 || s == nil || s.uploads == nil {
		http.NotFound(w, r)
		return
	}
	cfg, err := s.readDistributionPosterConfig(r.Context())
	if err != nil || !posterConfigReferencesUploadAsset(cfg, id) {
		http.NotFound(w, r)
		return
	}
	asset, err := s.uploads.Find(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	contentType, ok := posterImageContentType(asset.ContentType)
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(asset.Data)))
	w.Header().Set("Cache-Control", "private, max-age=300")
	w.Header().Set("Vary", "Authorization")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(asset.Data)
}

// appDistributionPosterUpload serves legacy local files referenced by an
// enabled poster template. New uploads normally use upload_assets, but older
// configurations may still point at /api/uploads/ paths.
func (s *Server) appDistributionPosterUpload(w http.ResponseWriter, r *http.Request) {
	rel := publicUploadRelativePath(r.URL.Path, distributionPosterUploadPrefix)
	if rel == "" || s == nil {
		http.NotFound(w, r)
		return
	}
	privateURL := "/api/uploads/" + rel
	cfg, err := s.readDistributionPosterConfig(r.Context())
	if err != nil || !posterConfigReferencesLocalUpload(cfg, privateURL) {
		http.NotFound(w, r)
		return
	}
	s.servePublicLocalUpload(w, r, rel)
}

func (s *Server) distributionPosterConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPut {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if s.db == nil {
		httpx.Fail(w, 503, "海报配置存储暂不可用")
		return
	}
	if r.Method == http.MethodGet {
		cfg, err := s.readDistributionPosterConfig(r.Context())
		if err != nil {
			httpx.Fail(w, 500, "读取海报配置失败")
			return
		}
		httpx.OK(w, cfg)
		return
	}
	cfg := defaultPosterConfig()
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&cfg) != nil {
		httpx.Fail(w, 400, "海报配置格式错误")
		return
	}
	cfg = normalizePosterConfig(cfg)
	if err := validatePosterConfig(cfg); err != nil {
		httpx.Fail(w, 400, err.Error())
		return
	}
	raw, _ := json.Marshal(cfg)
	_, err := s.db.ExecContext(r.Context(), `INSERT INTO site_configs (key,config,update_time) VALUES ($1,$2::jsonb,now()) ON CONFLICT (key) DO UPDATE SET config=EXCLUDED.config,update_time=now()`, distributionPosterKey, string(raw))
	if err != nil {
		httpx.Fail(w, 500, "保存海报配置失败")
		return
	}
	httpx.OK(w, cfg)
}

// distributionPosterTemplateAction changes visibility without requiring the
// admin client to round-trip the whole canvas configuration.
func (s *Server) distributionPosterTemplateAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if s.db == nil {
		httpx.Fail(w, 503, "海报配置存储暂不可用")
		return
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/distribution-poster-config/templates/"), "/")
	if len(parts) != 2 || parts[0] == "" {
		httpx.Fail(w, 400, "海报模板路径错误")
		return
	}
	status := map[string]string{"publish": "published", "dispatch": "dispatched", "withdraw": "draft"}[parts[1]]
	if status == "" {
		httpx.Fail(w, 400, "不支持的海报模板操作")
		return
	}
	cfg, err := s.readDistributionPosterConfig(r.Context())
	if err != nil {
		httpx.Fail(w, 500, "读取海报配置失败")
		return
	}
	found := false
	for i := range cfg.Templates {
		if cfg.Templates[i].ID == parts[0] {
			cfg.Templates[i].Status = status
			found = true
			break
		}
	}
	if !found {
		httpx.Fail(w, 404, "海报模板不存在")
		return
	}
	cfg = normalizePosterConfig(cfg)
	raw, _ := json.Marshal(cfg)
	if _, err := s.db.ExecContext(r.Context(), `INSERT INTO site_configs (key,config,update_time) VALUES ($1,$2::jsonb,now()) ON CONFLICT (key) DO UPDATE SET config=EXCLUDED.config,update_time=now()`, distributionPosterKey, string(raw)); err != nil {
		httpx.Fail(w, 500, "保存海报状态失败")
		return
	}
	httpx.OK(w, cfg)
}
