package siteconfig

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"
)

func TestWriteReadPreservesMiniappHomeAndExistingOSSCarousel(t *testing.T) {
	config := validConfig()
	carousel := map[string]any{
		"autoplay": true,
		"interval": 4000,
		"items": []any{map[string]any{
			"enabled": true,
			"image":   "https://nine-xing.oss-cn-hangzhou.aliyuncs.com/miniapp/carousel-1.webp",
		}},
	}
	miniappHome := map[string]any{
		"brand": map[string]any{
			"enabled": true,
			"name":    "九型芯之力",
			"tagline": "看见动机，找到成长方向",
		},
		"hero": map[string]any{
			"enabled":     true,
			"kicker":      "老师导学",
			"title":       "读懂自己",
			"description": "从核心动机出发",
			"buttonText":  "开始人格测试",
		},
		"entriesSection": map[string]any{
			"enabled": true,
			"items": []any{map[string]any{
				"key": "test", "enabled": true, "title": "人格测试",
				"description": "找到你的核心动机", "icon": "compass", "theme": "blue",
			}},
		},
		"growth": map[string]any{
			"enabled":     true,
			"eyebrow":     "老师陪伴",
			"title":       "把测试发现带进课程练习",
			"description": "让理解沉淀为行动",
		},
	}
	config.Home["miniappCarousel"] = carousel
	config.Home["miniappHome"] = miniappHome

	path := filepath.Join(t.TempDir(), "site-config.json")
	if err := Write(path, config); err != nil {
		t.Fatal(err)
	}
	got, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}

	for _, key := range []string{"miniappCarousel", "miniappHome"} {
		wantJSON, err := json.Marshal(config.Home[key])
		if err != nil {
			t.Fatal(err)
		}
		gotJSON, err := json.Marshal(got.Home[key])
		if err != nil {
			t.Fatal(err)
		}
		var wantValue, gotValue any
		if err := json.Unmarshal(wantJSON, &wantValue); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(gotJSON, &gotValue); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(gotValue, wantValue) {
			t.Fatalf("expected home.%s to survive write/read: got=%s want=%s", key, gotJSON, wantJSON)
		}
	}

	gotCarousel := got.Home["miniappCarousel"].(map[string]any)
	gotItems := gotCarousel["items"].([]any)
	gotImage := gotItems[0].(map[string]any)["image"]
	if gotImage != "https://nine-xing.oss-cn-hangzhou.aliyuncs.com/miniapp/carousel-1.webp" {
		t.Fatalf("expected OSS carousel URL to remain unchanged, got %v", gotImage)
	}
}

func TestValidateRejectsUnsafeURLFields(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*SiteConfig)
	}{
		{
			name: "navigation javascript link",
			mutate: func(config *SiteConfig) {
				config.Navigation.Main[0].To = "javascript:alert(1)"
			},
		},
		{
			name: "site logo data url",
			mutate: func(config *SiteConfig) {
				config.Site.Logo = "data:image/svg+xml,<svg onload=alert(1)>"
			},
		},
		{
			name: "site logo protocol relative url",
			mutate: func(config *SiteConfig) {
				config.Site.Logo = "//evil.example/logo.svg"
			},
		},
		{
			name: "customer service qr data url",
			mutate: func(config *SiteConfig) {
				config.Site.CustomerServiceQr = "data:image/svg+xml,<svg onload=alert(1)>"
			},
		},
		{
			name: "nested home action javascript link",
			mutate: func(config *SiteConfig) {
				config.Home["hero"] = map[string]any{
					"actions": []any{
						map[string]any{
							"label": "bad",
							"to":    "java\nscript:alert(1)",
						},
					},
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := validConfig()
			tt.mutate(&config)
			if err := Validate(config); err == nil {
				t.Fatal("expected unsafe URL field to be rejected")
			}
		})
	}
}

func TestValidateAllowsSafeURLFields(t *testing.T) {
	config := validConfig()
	config.Navigation.Main = append(config.Navigation.Main,
		NavItem{Label: "anchor", To: "#signup", Type: "anchor"},
		NavItem{Label: "route", To: "/courses", Type: "route"},
		NavItem{Label: "external", To: "https://example.com/courses", Type: "link"},
	)
	config.Home["hero"] = map[string]any{
		"actions": []any{
			map[string]any{"label": "signup", "to": "/#signup"},
			map[string]any{"label": "phone", "href": "tel:13800000000"},
		},
	}
	config.Home["teacherTeaser"] = map[string]any{
		"image":    "/assets/teacher.jpg",
		"buttonTo": "/teacher",
	}
	config.Site.CustomerServiceQr = "https://cdn.example.com/customer-service.png"

	if err := Validate(config); err != nil {
		t.Fatalf("expected safe URL fields to pass, got %v", err)
	}
}

func validConfig() SiteConfig {
	var config SiteConfig
	config.Site.BrandName = "九型芯之力"
	config.Site.Logo = "/assets/logo.svg"
	config.Navigation.Main = []NavItem{
		{Label: "首页", To: "/", Type: "route"},
	}
	config.Navigation.Drawer = []NavItem{
		{Label: "首页", To: "/", Type: "route"},
	}
	config.Navigation.Tabs = []TabItem{
		{NavItem: NavItem{Label: "首页", To: "/", Type: "route"}, Icon: "home", Match: "/"},
	}
	config.Home = map[string]any{
		"enterprise": map[string]any{
			"buttonHref": "#signup",
		},
	}
	config.Types = []struct {
		Avatar      string `json:"avatar"`
		Description string `json:"description"`
		ID          string `json:"id"`
		Keywords    string `json:"keywords"`
		Name        string `json:"name"`
	}{
		{ID: "1", Name: "完美型", Avatar: "/assets/avatars/1.webp"},
	}
	return config
}

func TestSiteFieldMissingDistinguishesMissingAndEmpty(t *testing.T) {
	if !siteFieldMissing([]byte(`{"site":{"logo":"/logo.svg"}}`), "customerServiceQr") {
		t.Fatal("expected missing customerServiceQr to be detected")
	}
	if siteFieldMissing([]byte(`{"site":{"customerServiceQr":""}}`), "customerServiceQr") {
		t.Fatal("explicit empty customerServiceQr should not be treated as missing")
	}
}
