package siteconfig

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type NavItem struct {
	Label string `json:"label"`
	To    string `json:"to"`
	Type  string `json:"type"`
}

type TabItem struct {
	NavItem
	Icon  string `json:"icon"`
	Match string `json:"match"`
}

type SiteConfig struct {
	Home       map[string]any `json:"home"`
	Navigation struct {
		Drawer []NavItem `json:"drawer"`
		Main   []NavItem `json:"main"`
		Tabs   []TabItem `json:"tabs"`
	} `json:"navigation"`
	Site struct {
		BrandName         string `json:"brandName"`
		Copyright         string `json:"copyright"`
		FooterTagline     string `json:"footerTagline"`
		Logo              string `json:"logo"`
		CustomerServiceQr string `json:"customerServiceQr"`
	} `json:"site"`
	Types []struct {
		Avatar      string `json:"avatar"`
		Description string `json:"description"`
		ID          string `json:"id"`
		Keywords    string `json:"keywords"`
		Name        string `json:"name"`
	} `json:"types"`
}

const defaultConfigKey = "default"

func Read(path string) (SiteConfig, error) {
	file, err := os.ReadFile(path)
	if err != nil {
		return SiteConfig{}, err
	}

	var config SiteConfig
	if err := json.Unmarshal(file, &config); err != nil {
		return SiteConfig{}, err
	}
	if err := Validate(config); err != nil {
		return SiteConfig{}, err
	}
	return config, nil
}

func ReadStore(ctx context.Context, db *sql.DB, path string) (SiteConfig, error) {
	if db == nil {
		return Read(path)
	}

	c, cancel := context.WithTimeout(ctxOrBackground(ctx), 10*time.Second)
	defer cancel()

	var raw []byte
	err := db.QueryRowContext(c, `SELECT config FROM site_configs WHERE key=$1`, defaultConfigKey).Scan(&raw)
	if err == nil {
		var config SiteConfig
		if err := json.Unmarshal(raw, &config); err != nil {
			return SiteConfig{}, err
		}
		if siteFieldMissing(raw, "customerServiceQr") {
			if defaults, err := Read(path); err == nil {
				config.Site.CustomerServiceQr = defaults.Site.CustomerServiceQr
			}
		}
		if err := Validate(config); err != nil {
			return SiteConfig{}, err
		}
		return config, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return SiteConfig{}, err
	}

	config, err := Read(path)
	if err != nil {
		return SiteConfig{}, err
	}
	if err := UpsertStore(c, db, config); err != nil {
		return SiteConfig{}, err
	}
	return config, nil
}

func Write(path string, config SiteConfig) error {
	if err := Validate(config); err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	body, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')

	tempPath := path + ".tmp"
	if err := os.WriteFile(tempPath, body, 0o644); err != nil {
		return err
	}
	return os.Rename(tempPath, path)
}

func WriteStore(ctx context.Context, db *sql.DB, path string, config SiteConfig) error {
	if err := Validate(config); err != nil {
		return err
	}
	if db != nil {
		c, cancel := context.WithTimeout(ctxOrBackground(ctx), 10*time.Second)
		defer cancel()
		if err := UpsertStore(c, db, config); err != nil {
			return err
		}
	}
	return Write(path, config)
}

func UpsertStore(ctx context.Context, db *sql.DB, config SiteConfig) error {
	body, err := json.Marshal(config)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx,
		`INSERT INTO site_configs (key, config, update_time)
		 VALUES ($1, $2::jsonb, now())
		 ON CONFLICT (key) DO UPDATE SET config=EXCLUDED.config, update_time=now()`,
		defaultConfigKey, string(body),
	)
	return err
}

func ctxOrBackground(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func siteFieldMissing(raw []byte, field string) bool {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(raw, &payload); err != nil {
		return false
	}
	var site map[string]json.RawMessage
	if err := json.Unmarshal(payload["site"], &site); err != nil {
		return false
	}
	_, ok := site[field]
	return !ok
}

func Validate(config SiteConfig) error {
	if config.Site.BrandName == "" {
		return errors.New("site.brandName is required")
	}
	if config.Site.Logo == "" {
		return errors.New("site.logo is required")
	}
	if err := validateURLField("site.logo", config.Site.Logo, urlKindMedia); err != nil {
		return err
	}
	if err := validateURLField("site.customerServiceQr", config.Site.CustomerServiceQr, urlKindMedia); err != nil {
		return err
	}
	if len(config.Navigation.Main) == 0 {
		return errors.New("navigation.main is required")
	}
	for i, item := range config.Navigation.Main {
		if err := validateURLField(fmt.Sprintf("navigation.main[%d].to", i), item.To, urlKindLink); err != nil {
			return err
		}
	}
	for i, item := range config.Navigation.Drawer {
		if err := validateURLField(fmt.Sprintf("navigation.drawer[%d].to", i), item.To, urlKindLink); err != nil {
			return err
		}
	}
	for i, item := range config.Navigation.Tabs {
		if err := validateURLField(fmt.Sprintf("navigation.tabs[%d].to", i), item.To, urlKindLink); err != nil {
			return err
		}
	}
	if len(config.Types) == 0 {
		return errors.New("types is required")
	}
	for i, item := range config.Types {
		if item.ID == "" || item.Name == "" {
			return errors.New("type id and name are required")
		}
		if err := validateURLField(fmt.Sprintf("types[%d].avatar", i), item.Avatar, urlKindMedia); err != nil {
			return err
		}
	}
	if err := validateDynamicURLFields("home", config.Home); err != nil {
		return err
	}
	return nil
}

type urlKind int

const (
	urlKindLink urlKind = iota
	urlKindMedia
)

func validateDynamicURLFields(path string, value any) error {
	switch node := value.(type) {
	case map[string]any:
		for key, child := range node {
			childPath := path + "." + key
			if text, ok := child.(string); ok && urlFieldKind(key) != nil {
				if err := validateURLField(childPath, text, *urlFieldKind(key)); err != nil {
					return err
				}
			}
			if err := validateDynamicURLFields(childPath, child); err != nil {
				return err
			}
		}
	case []any:
		for i, child := range node {
			if err := validateDynamicURLFields(fmt.Sprintf("%s[%d]", path, i), child); err != nil {
				return err
			}
		}
	}
	return nil
}

func urlFieldKind(key string) *urlKind {
	normalized := strings.ToLower(strings.TrimSpace(key))
	media := urlKindMedia
	link := urlKindLink
	switch {
	case normalized == "logo" ||
		normalized == "avatar" ||
		normalized == "image" ||
		normalized == "fallbackimage" ||
		normalized == "poster" ||
		normalized == "cover":
		return &media
	case normalized == "to" ||
		normalized == "href" ||
		normalized == "url" ||
		normalized == "buttonto" ||
		normalized == "buttonhref" ||
		strings.HasSuffix(normalized, "href"):
		return &link
	default:
		return nil
	}
}

func validateURLField(path, value string, kind urlKind) error {
	raw := strings.TrimSpace(value)
	if raw == "" {
		return nil
	}
	if strings.HasPrefix(raw, "//") {
		return fmt.Errorf("%s must not be protocol-relative", path)
	}
	if hasControl(raw) {
		return fmt.Errorf("%s contains control characters", path)
	}
	scheme := urlScheme(raw)
	if scheme == "" {
		return nil
	}
	switch scheme {
	case "http", "https":
		return nil
	case "mailto", "tel":
		if kind == urlKindLink {
			return nil
		}
	}
	return fmt.Errorf("%s has unsupported URL scheme %q", path, scheme)
}

func hasControl(value string) bool {
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}

func urlScheme(value string) string {
	for i, r := range value {
		switch r {
		case ':':
			if i == 0 {
				return ""
			}
			candidate := value[:i]
			for _, ch := range candidate {
				if !(ch >= 'a' && ch <= 'z') && !(ch >= 'A' && ch <= 'Z') && !(ch >= '0' && ch <= '9') && ch != '+' && ch != '-' && ch != '.' {
					return ""
				}
			}
			return strings.ToLower(candidate)
		case '/', '#', '?':
			return ""
		}
	}
	return ""
}
