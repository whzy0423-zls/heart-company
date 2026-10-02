package siteconfig

import (
	"path/filepath"
	"testing"
)

func TestMiniappCoursesEnabledDefaultsOnAndPreservesDisabledCatalog(t *testing.T) {
	if !MiniappCoursesEnabled(SiteConfig{}) {
		t.Fatal("legacy configuration must keep course enrollment enabled")
	}
	for _, enabled := range []bool{false, true} {
		cfg := validConfig()
		cfg.Home["miniappCourses"] = map[string]any{
			"enabled": enabled,
			"items":   []any{map[string]any{"id": "c1", "title": "九型成长课", "enabled": true, "priceCents": 100}},
		}
		path := filepath.Join(t.TempDir(), "site-config.json")
		if err := Write(path, cfg); err != nil {
			t.Fatal(err)
		}
		stored, err := Read(path)
		if err != nil {
			t.Fatal(err)
		}
		if MiniappCoursesEnabled(stored) != enabled {
			t.Fatalf("switch changed during normalization/write/read: want %v", enabled)
		}
		items, err := MiniappCourses(stored)
		if err != nil || len(items) != 1 || items[0].Title != "九型成长课" {
			t.Fatalf("switch discarded historical catalog: %+v, %v", items, err)
		}
	}
}

func TestMiniappCoursesSwitchRejectsNonBooleanConfig(t *testing.T) {
	cfg := validConfig()
	cfg.Home["miniappCourses"] = map[string]any{"enabled": "false", "items": []any{}}
	if err := Validate(cfg); err == nil {
		t.Fatal("course switch accepts non-boolean values")
	}
}
