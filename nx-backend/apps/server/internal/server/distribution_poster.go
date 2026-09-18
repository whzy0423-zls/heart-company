package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"nine-xing/nx-backend/apps/server/internal/httpx"
)

const distributionPosterKey = "distribution_poster"
const defaultPosterLandingURL = "https://xn--9iq9az5uo8fz16d.com/app"

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
	return posterTemplate{ID: "default", Name: "默认海报", Enabled: true, SortOrder: 0, LandingURL: defaultPosterLandingURL, QRSize: 176, QRX: 62, QRY: 1010, InviteX: 286, InviteY: 1100, InviteWidth: 350, InviteFontSize: 26}
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
		if template.QRSize < 132 || template.QRSize > 220 {
			return errors.New("二维码尺寸应为 132–220")
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
		if template.Enabled {
			items = append(items, template)
		}
	}
	return items
}

func (s *Server) appPosterTemplates(ctx context.Context) []posterTemplate {
	if s.db == nil {
		return nil
	}
	var raw []byte
	if err := s.db.QueryRowContext(ctx, "SELECT config FROM site_configs WHERE key=$1", distributionPosterKey).Scan(&raw); err != nil {
		return nil
	}
	var cfg posterConfig
	if json.Unmarshal(raw, &cfg) != nil {
		return nil
	}
	return activePosterTemplates(cfg)
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
		cfg := defaultPosterConfig()
		var raw []byte
		err := s.db.QueryRowContext(r.Context(), "SELECT config FROM site_configs WHERE key=$1", distributionPosterKey).Scan(&raw)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			httpx.Fail(w, 500, "读取海报配置失败")
			return
		}
		if err == nil && json.Unmarshal(raw, &cfg) != nil {
			httpx.Fail(w, 500, "海报配置格式错误")
			return
		}
		cfg = normalizePosterConfig(cfg)
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
