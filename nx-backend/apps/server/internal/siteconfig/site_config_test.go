package siteconfig

import "testing"

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
		{ID: "1", Name: "完美型", Avatar: "/assets/avatars/1.png"},
	}
	return config
}
