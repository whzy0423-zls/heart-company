package siteconfig

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

const MaxCoursePriceCents = 99_999_900

type MiniappCourse struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Subtitle    string   `json:"subtitle"`
	Description string   `json:"description"`
	Cover       string   `json:"cover"`
	Badge       string   `json:"badge"`
	Format      string   `json:"format"`
	Duration    string   `json:"duration"`
	Schedule    string   `json:"schedule"`
	Location    string   `json:"location"`
	Bullets     []string `json:"bullets"`
	Outline     []string `json:"outline"`
	Notice      string   `json:"notice"`
	Enabled     bool     `json:"enabled"`
	PriceCents  int      `json:"priceCents"`
	PaymentMode string   `json:"paymentMode"`
}

type MiniappCoursesConfig struct {
	Items []MiniappCourse `json:"items"`
}

var courseIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,79}$`)

// EnsureMiniappCourses migrates legacy display cards only when the new key is absent.
// An explicitly empty catalog is intentional and must never repopulate itself.
func EnsureMiniappCourses(config *SiteConfig) {
	if config.Home == nil {
		config.Home = map[string]any{}
	}
	if value, exists := config.Home["miniappCourses"]; exists {
		// Price is the single source of truth, including legacy saved modes.
		// Keep malformed prices intact so validation still rejects them.
		raw, err := json.Marshal(value)
		var catalog MiniappCoursesConfig
		if err == nil && json.Unmarshal(raw, &catalog) == nil && catalog.Items != nil {
			// Patch only the derived field, keeping extension fields owned by
			// other editors intact on both reads and subsequent writes.
			var normalized map[string]any
			_ = json.Unmarshal(raw, &normalized)
			items, ok := normalized["items"].([]any)
			if ok {
				for i, course := range catalog.Items {
					if item, ok := items[i].(map[string]any); ok {
						item["paymentMode"] = "consult"
						if course.PriceCents > 0 {
							item["paymentMode"] = "paid"
						}
					}
				}
				config.Home["miniappCourses"] = normalized
			}
		}
		return
	}
	raw, _ := json.Marshal(config.Home["courses"])
	var legacy struct {
		Items []MiniappCourse `json:"items"`
	}
	_ = json.Unmarshal(raw, &legacy)
	items := make([]MiniappCourse, 0, len(legacy.Items))
	used := map[string]int{}
	for _, item := range legacy.Items {
		item.Title = strings.TrimSpace(item.Title)
		if item.Title == "" {
			continue
		}
		hash := sha256.Sum256([]byte(item.Title))
		base := fmt.Sprintf("course-%x", hash[:6])
		used[base]++
		item.ID = base
		if used[base] > 1 {
			item.ID = fmt.Sprintf("%s-%d", base, used[base])
		}
		item.Enabled, item.PriceCents, item.PaymentMode = true, 0, "consult"
		if item.Bullets == nil {
			item.Bullets = []string{}
		}
		if item.Outline == nil {
			item.Outline = []string{}
		}
		items = append(items, item)
	}
	// Keep dynamic configuration represented as maps/slices for existing editors.
	encoded, _ := json.Marshal(MiniappCoursesConfig{Items: items})
	var value map[string]any
	_ = json.Unmarshal(encoded, &value)
	config.Home["miniappCourses"] = value
}

func MiniappCourses(config SiteConfig) ([]MiniappCourse, error) {
	EnsureMiniappCourses(&config)
	raw, err := json.Marshal(config.Home["miniappCourses"])
	if err != nil {
		return nil, err
	}
	var catalog MiniappCoursesConfig
	if err = json.Unmarshal(raw, &catalog); err != nil || catalog.Items == nil {
		return nil, fmt.Errorf("home.miniappCourses.items must be an array")
	}
	if len(catalog.Items) > 200 {
		return nil, fmt.Errorf("at most 200 miniapp courses are allowed")
	}
	seen := map[string]bool{}
	seenTitles := map[string]bool{}
	for _, item := range catalog.Items {
		if !courseIDPattern.MatchString(item.ID) || seen[item.ID] {
			return nil, fmt.Errorf("course id must be unique and contain 1-80 letters, numbers, underscores or hyphens")
		}
		seen[item.ID] = true
		title := strings.TrimSpace(item.Title)
		if title == "" || len([]rune(title)) > 120 {
			return nil, fmt.Errorf("course title is required (max 120 characters)")
		}
		titleKey := strings.ToLower(title)
		if seenTitles[titleKey] {
			return nil, fmt.Errorf("course title must be unique")
		}
		seenTitles[titleKey] = true
		if item.PaymentMode != "consult" && item.PaymentMode != "paid" {
			return nil, fmt.Errorf("course paymentMode must be consult or paid")
		}
		if item.PriceCents < 0 || item.PriceCents > MaxCoursePriceCents {
			return nil, fmt.Errorf("invalid course priceCents")
		}
		if err := validateURLField("home.miniappCourses.cover", item.Cover, urlKindMedia); err != nil {
			return nil, err
		}
	}
	return catalog.Items, nil
}

func MiniappCoursesFromStore(ctx context.Context, db *sql.DB, path string) ([]MiniappCourse, error) {
	cfg, err := ReadStore(ctx, db, path)
	if err != nil {
		return nil, err
	}
	return MiniappCourses(cfg)
}
