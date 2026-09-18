package server

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"

	"nine-xing/nx-backend/apps/server/internal/httpx"
)

const distributionPosterKey = "distribution_poster"
const defaultPosterLandingURL = "https://xinzhili.cn/app"

type posterConfig struct {
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

func defaultPosterConfig() posterConfig {
	return posterConfig{LandingURL: defaultPosterLandingURL, QRSize: 176, QRX: 62, QRY: 1010, InviteX: 286, InviteY: 1100, InviteWidth: 350, InviteFontSize: 26}
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
	if !validPosterImage(cfg.TemplateURL) || !validPosterImage(cfg.QRImageURL) {
		return errors.New("请选择上传的图片")
	}
	if cfg.TemplateURL == "" {
		return errors.New("请先上传海报模板")
	}
	if cfg.QRSize < 132 || cfg.QRSize > 220 {
		return errors.New("二维码尺寸应为 132–220")
	}
	if cfg.QRX < 0 || cfg.QRY < 0 || cfg.QRX+cfg.QRSize > 720 || cfg.QRY+cfg.QRSize > 1280 {
		return errors.New("二维码位置超出海报范围")
	}
	if cfg.InviteWidth < 120 || cfg.InviteWidth > 600 || cfg.InviteFontSize < 16 || cfg.InviteFontSize > 64 || cfg.InviteX < 0 || cfg.InviteY < 0 || cfg.InviteX+cfg.InviteWidth > 720 || cfg.InviteY+cfg.InviteFontSize+12 > 1280 {
		return errors.New("邀请码位置或字号超出海报范围")
	}
	if utf8.RuneCountInString(cfg.Headline) > 32 || utf8.RuneCountInString(cfg.Subtitle) > 80 || utf8.RuneCountInString(cfg.CTA) > 24 {
		return errors.New("海报文字超出长度限制")
	}
	if cfg.LandingURL != "" {
		u, err := url.Parse(cfg.LandingURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || len(cfg.LandingURL) > 2048 {
			return errors.New("请输入有效的 HTTP/HTTPS 二维码链接")
		}
	}
	if cfg.QRImageURL == "" && cfg.LandingURL == "" {
		return errors.New("请上传二维码或设置二维码链接")
	}
	return nil
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
