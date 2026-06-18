package siteconfig

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
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
		BrandName     string `json:"brandName"`
		Copyright     string `json:"copyright"`
		FooterTagline string `json:"footerTagline"`
		Logo          string `json:"logo"`
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

func Validate(config SiteConfig) error {
	if config.Site.BrandName == "" {
		return errors.New("site.brandName is required")
	}
	if config.Site.Logo == "" {
		return errors.New("site.logo is required")
	}
	if len(config.Navigation.Main) == 0 {
		return errors.New("navigation.main is required")
	}
	if len(config.Types) == 0 {
		return errors.New("types is required")
	}
	for _, item := range config.Types {
		if item.ID == "" || item.Name == "" {
			return errors.New("type id and name are required")
		}
	}
	return nil
}
