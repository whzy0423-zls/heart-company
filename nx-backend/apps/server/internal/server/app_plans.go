package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"nine-xing/nx-backend/apps/server/internal/auditlog"
	"nine-xing/nx-backend/apps/server/internal/httpx"
)

type appPlanConfig struct {
	Code                string          `json:"code"`
	PlanLevel           string          `json:"planLevel"`
	BillingCycle        string          `json:"billingCycle"`
	Name                string          `json:"name"`
	Subtitle            string          `json:"subtitle"`
	PriceCents          int             `json:"priceCents"`
	OriginalPriceCents  int             `json:"originalPriceCents"`
	Badge               string          `json:"badge"`
	Features            []string        `json:"features"`
	FeatureFlags        map[string]bool `json:"featureFlags,omitempty"`
	Limits              map[string]int  `json:"limits,omitempty"`
	Enabled             bool            `json:"enabled"`
	SortOrder           int             `json:"sortOrder"`
	DurationDays        int             `json:"durationDays"`
	DailyChatLimit      int             `json:"dailyChatLimit"`
	StoryMonthlyLimit   int             `json:"storyMonthlyLimit"`
	CardLimit           int             `json:"cardLimit"`
	DeepChatEnabled     bool            `json:"deepChatEnabled"`
	CompanionEnabled    bool            `json:"companionEnabled"`
	MemberPosterEnabled bool            `json:"memberPosterEnabled"`
}

func defaultAppPlans() []appPlanConfig {
	return []appPlanConfig{
		{Code: "free", PlanLevel: "free", BillingCycle: "none", Name: "免费版", Subtitle: "每日基础陪伴", Features: []string{"每日 5 轮基础对话", "首次 1 篇人生故事", "最多 1 张人物卡", "经典海报"}, Enabled: true, DailyChatLimit: 5, StoryMonthlyLimit: 1, CardLimit: 1, FeatureFlags: map[string]bool{"deepChat": false, "companion": false, "memberPoster": false}, Limits: map[string]int{"cardLimit": 1, "dailyChatLimit": 5, "storyMonthlyLimit": 1}},
		{Code: "vip_month", PlanLevel: "vip", BillingCycle: "month", Name: "月卡会员", Subtitle: "灵活体验完整成长陪伴", PriceCents: 2900, Badge: "灵活", Features: []string{"深度对话与专业陪伴", "每月 3 篇人生故事", "最多 3 张人物卡", "2 款会员海报"}, Enabled: true, SortOrder: 10, DurationDays: 30, DailyChatLimit: -1, StoryMonthlyLimit: 3, CardLimit: 3, DeepChatEnabled: true, CompanionEnabled: true, MemberPosterEnabled: true, FeatureFlags: map[string]bool{"deepChat": true, "companion": true, "memberPoster": true}, Limits: map[string]int{"cardLimit": 3, "dailyChatLimit": -1, "storyMonthlyLimit": 3}},
		{Code: "vip_quarter", PlanLevel: "vip", BillingCycle: "quarter", Name: "季卡会员", Subtitle: "约 ¥26.3/月，适合持续成长", PriceCents: 7900, OriginalPriceCents: 8700, Badge: "推荐", Features: []string{"深度对话与专业陪伴", "每月 5 篇人生故事", "最多 3 张人物卡", "2 款会员海报"}, Enabled: true, SortOrder: 20, DurationDays: 90, DailyChatLimit: -1, StoryMonthlyLimit: 5, CardLimit: 3, DeepChatEnabled: true, CompanionEnabled: true, MemberPosterEnabled: true, FeatureFlags: map[string]bool{"deepChat": true, "companion": true, "memberPoster": true}, Limits: map[string]int{"cardLimit": 3, "dailyChatLimit": -1, "storyMonthlyLimit": 5}},
		{Code: "vip_year", PlanLevel: "vip", BillingCycle: "year", Name: "年卡会员", Subtitle: "约 ¥16.6/月，适合长期自我探索", PriceCents: 19900, OriginalPriceCents: 34800, Badge: "最划算", Features: []string{"深度对话与专业陪伴", "每月 12 篇人生故事", "最多 3 张人物卡", "2 款会员海报"}, Enabled: true, SortOrder: 30, DurationDays: 365, DailyChatLimit: -1, StoryMonthlyLimit: 12, CardLimit: 3, DeepChatEnabled: true, CompanionEnabled: true, MemberPosterEnabled: true, FeatureFlags: map[string]bool{"deepChat": true, "companion": true, "memberPoster": true}, Limits: map[string]int{"cardLimit": 3, "dailyChatLimit": -1, "storyMonthlyLimit": 12}},
		// S VIP is intentionally disabled until an administrator sets its price
		// and commercial terms. The canonical level and quotas are still exposed
		// so clients and migrations can distinguish it from the legacy annual VIP.
		{Code: "svip", PlanLevel: "svip", BillingCycle: "year", Name: "S VIP", Subtitle: "深度陪伴与优先权益", Features: []string{"深度对话与专业陪伴", "每月 12 篇人生故事", "最多 10 张人物卡", "全部高级内容"}, Enabled: false, SortOrder: 40, DurationDays: 365, DailyChatLimit: -1, StoryMonthlyLimit: 12, CardLimit: 10, DeepChatEnabled: true, CompanionEnabled: true, MemberPosterEnabled: true, FeatureFlags: map[string]bool{"deepChat": true, "companion": true, "memberPoster": true}, Limits: map[string]int{"cardLimit": 10, "dailyChatLimit": -1, "storyMonthlyLimit": 12}},
	}
}

// defaultMembershipLevelPlan is the canonical level policy. The legacy SKU
// plans above intentionally keep their historical flat columns for old
// clients; the ledger and new entitlement contract use this level policy.
func defaultMembershipLevelPlan(level string) appPlanConfig {
	level = normalizeMembershipLevel(level)
	switch level {
	case "vip":
		return appPlanConfig{Code: "vip", PlanLevel: "vip", BillingCycle: "month", Name: "VIP", Features: []string{"深度对话与专业陪伴", "每月 3 篇人生故事", "最多 3 张人物卡"}, Enabled: true, DurationDays: 30, DailyChatLimit: -1, StoryMonthlyLimit: 3, CardLimit: 3, DeepChatEnabled: true, CompanionEnabled: true, MemberPosterEnabled: true, FeatureFlags: map[string]bool{"deepChat": true, "companion": true, "memberPoster": true}, Limits: map[string]int{"cardLimit": 3, "dailyChatLimit": -1, "storyMonthlyLimit": 3}}
	case "svip":
		return appPlanConfig{Code: "svip", PlanLevel: "svip", BillingCycle: "year", Name: "S VIP", Features: []string{"深度对话与专业陪伴", "每月 12 篇人生故事", "最多 10 张人物卡"}, Enabled: true, DurationDays: 365, DailyChatLimit: -1, StoryMonthlyLimit: 12, CardLimit: 10, DeepChatEnabled: true, CompanionEnabled: true, MemberPosterEnabled: true, FeatureFlags: map[string]bool{"deepChat": true, "companion": true, "memberPoster": true}, Limits: map[string]int{"cardLimit": 10, "dailyChatLimit": -1, "storyMonthlyLimit": 12}}
	default:
		return appPlanConfig{Code: "free", PlanLevel: "free", BillingCycle: "none", Name: "免费版", Enabled: true, DailyChatLimit: 5, StoryMonthlyLimit: 1, CardLimit: 1, FeatureFlags: map[string]bool{"deepChat": false, "companion": false, "memberPoster": false}, Limits: map[string]int{"cardLimit": 1, "dailyChatLimit": 5, "storyMonthlyLimit": 1}}
	}
}

func normalizeMembershipLevel(level string) string {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "vip", "vip_month", "vip_quarter", "vip_year":
		return "vip"
	case "svip":
		return "svip"
	default:
		return "free"
	}
}

func normalizeBillingCycle(level, sku string) string {
	level = strings.ToLower(strings.TrimSpace(level))
	sku = strings.ToLower(strings.TrimSpace(sku))
	switch sku {
	case "vip_month":
		return "month"
	case "vip_quarter":
		return "quarter"
	case "vip_year":
		return "year"
	case "svip":
		return "year"
	}
	if level == "free" || level == "" {
		return "none"
	}
	if level == "svip" {
		return "year"
	}
	return "month"
}

func normalizeAppPlanCode(code string) string {
	switch strings.ToLower(strings.TrimSpace(code)) {
	case "", "free":
		return "free"
	case "vip":
		return "vip_month"
	case "svip":
		return "vip_year"
	default:
		return strings.ToLower(strings.TrimSpace(code))
	}
}

func supportedAppPlanCode(code string) bool {
	switch strings.ToLower(strings.TrimSpace(code)) {
	case "free", "vip", "svip", "vip_month", "vip_quarter", "vip_year":
		return true
	default:
		return false
	}
}

func validateAppPlan(plan appPlanConfig) error {
	plan.Code = strings.ToLower(strings.TrimSpace(plan.Code))
	plan.PlanLevel = normalizeMembershipLevel(plan.PlanLevel)
	if plan.BillingCycle == "" {
		plan.BillingCycle = normalizeBillingCycle(plan.PlanLevel, plan.Code)
	}
	if !supportedAppPlanCode(plan.Code) || (normalizeAppPlanCode(plan.Code) != plan.Code && plan.Code != "vip" && plan.Code != "svip") {
		return errors.New("套餐代码不受支持")
	}
	if strings.TrimSpace(plan.Name) == "" || len([]rune(plan.Name)) > 40 {
		return errors.New("套餐名称不能为空且不能超过 40 个字")
	}
	if plan.PriceCents < 0 || plan.OriginalPriceCents < 0 {
		return errors.New("套餐价格不能为负数")
	}
	if plan.Code != "free" && (plan.PriceCents <= 0 || plan.DurationDays <= 0) {
		return errors.New("付费套餐价格和有效天数必须大于 0")
	}
	if plan.Code == "free" && (plan.PriceCents != 0 || plan.DurationDays != 0) {
		return errors.New("免费套餐价格和有效天数必须为 0")
	}
	if plan.PlanLevel == "free" && plan.BillingCycle != "none" {
		return errors.New("免费套餐周期必须为 none")
	}
	if plan.PlanLevel != "free" && plan.BillingCycle == "none" {
		return errors.New("付费套餐必须配置购买周期")
	}
	if plan.DailyChatLimit < -1 || plan.StoryMonthlyLimit < 0 || plan.CardLimit < 0 {
		return errors.New("套餐额度不合法")
	}
	if len(plan.Features) > 8 {
		return errors.New("套餐权益最多 8 项")
	}
	return nil
}

func defaultAppPlan(code string) appPlanConfig {
	if strings.EqualFold(strings.TrimSpace(code), "svip") {
		plan := defaultMembershipLevelPlan("svip")
		plan.Enabled = false
		return plan
	}
	code = normalizeAppPlanCode(code)
	for _, plan := range defaultAppPlans() {
		if plan.Code == code {
			return plan
		}
	}
	if code != "free" && code != "vip_month" && code != "vip_quarter" && code != "vip_year" && code != "vip" && code != "svip" {
		return appPlanConfig{Code: code, PlanLevel: "vip", BillingCycle: "month", Name: "会员版", Subtitle: "完整成长陪伴", Enabled: true, DurationDays: 30, DailyChatLimit: -1, StoryMonthlyLimit: 3, CardLimit: 5, DeepChatEnabled: true, CompanionEnabled: true, MemberPosterEnabled: true, FeatureFlags: map[string]bool{"deepChat": true, "companion": true, "memberPoster": true}, Limits: map[string]int{"cardLimit": 5, "dailyChatLimit": -1, "storyMonthlyLimit": 3}}
	}
	plan := defaultMembershipLevelPlan(normalizeMembershipLevel(code))
	plan.Code = code
	plan.BillingCycle = normalizeBillingCycle(plan.PlanLevel, code)
	return plan
}

func scanAppPlan(scanner interface{ Scan(...any) error }) (appPlanConfig, error) {
	var plan appPlanConfig
	var features []byte
	err := scanner.Scan(&plan.Code, &plan.Name, &plan.Subtitle, &plan.PriceCents, &plan.OriginalPriceCents, &plan.Badge, &features, &plan.Enabled, &plan.SortOrder, &plan.DurationDays, &plan.DailyChatLimit, &plan.StoryMonthlyLimit, &plan.CardLimit, &plan.DeepChatEnabled, &plan.CompanionEnabled, &plan.MemberPosterEnabled)
	if err != nil {
		return appPlanConfig{}, err
	}
	if err := json.Unmarshal(features, &plan.Features); err != nil {
		return appPlanConfig{}, err
	}
	plan.PlanLevel = normalizeMembershipLevel(plan.Code)
	plan.BillingCycle = normalizeBillingCycle(plan.PlanLevel, plan.Code)
	plan.FeatureFlags = map[string]bool{"deepChat": plan.DeepChatEnabled, "companion": plan.CompanionEnabled, "memberPoster": plan.MemberPosterEnabled}
	plan.Limits = map[string]int{"cardLimit": plan.CardLimit, "dailyChatLimit": plan.DailyChatLimit, "storyMonthlyLimit": plan.StoryMonthlyLimit}
	return plan, validateAppPlan(plan)
}

const appPlanColumns = `code,name,subtitle,price_cents,original_price_cents,badge,features,enabled,sort_order,duration_days,daily_chat_limit,story_monthly_limit,card_limit,deep_chat_enabled,companion_enabled,member_poster_enabled`

func (s *Server) loadAppPlans(ctx context.Context) ([]appPlanConfig, error) {
	if s == nil || s.db == nil {
		return defaultAppPlans(), nil
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+appPlanColumns+` FROM app_plans ORDER BY sort_order,code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	plans := make([]appPlanConfig, 0, 4)
	for rows.Next() {
		plan, err := scanAppPlan(rows)
		if err != nil {
			return nil, err
		}
		if err := s.loadAppPlanCapabilities(ctx, &plan); err != nil {
			// Old database drivers/schema versions may not have the additive
			// columns yet; the normalized defaults above remain valid.
			if !errors.Is(err, sql.ErrNoRows) {
				plan = normalizeLoadedAppPlan(plan)
			}
		}
		plans = append(plans, normalizeLoadedAppPlan(plan))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(plans) == 0 {
		return nil, sql.ErrNoRows
	}
	return plans, nil
}

func normalizeLoadedAppPlan(plan appPlanConfig) appPlanConfig {
	// The level is authoritative for access. Older rows may have been created
	// before plan_level existed and therefore carry the column default `free`;
	// infer the level from the SKU in that case rather than granting a wrong
	// quota.
	if plan.PlanLevel == "" || (plan.PlanLevel == "free" && plan.Code != "free") {
		plan.PlanLevel = normalizeMembershipLevel(plan.Code)
	} else {
		plan.PlanLevel = normalizeMembershipLevel(plan.PlanLevel)
	}
	if plan.BillingCycle == "" {
		plan.BillingCycle = normalizeBillingCycle(plan.PlanLevel, plan.Code)
	}
	if plan.FeatureFlags == nil {
		plan.FeatureFlags = map[string]bool{"deepChat": plan.DeepChatEnabled, "companion": plan.CompanionEnabled, "memberPoster": plan.MemberPosterEnabled}
	}
	if plan.Limits == nil {
		plan.Limits = map[string]int{"cardLimit": plan.CardLimit, "dailyChatLimit": plan.DailyChatLimit, "storyMonthlyLimit": plan.StoryMonthlyLimit}
	}
	// Card capacity is a tier entitlement, not a billing-cycle entitlement.
	// Keep the legacy flat column for old clients, but make both runtime values
	// agree with the canonical policy (free 1, VIP 3, S VIP 10).
	canonical := defaultMembershipLevelPlan(plan.PlanLevel)
	plan.CardLimit = canonical.CardLimit
	plan.Limits["cardLimit"] = canonical.CardLimit
	plan.Features = normalizeCardFeatureCopy(plan.Features, canonical.CardLimit)
	return plan
}

func normalizeCardFeatureCopy(features []string, cardLimit int) []string {
	if len(features) == 0 {
		return features
	}
	needle := "人物卡"
	result := append([]string(nil), features...)
	for i, feature := range result {
		if !strings.Contains(feature, needle) {
			continue
		}
		prefix := strings.TrimSpace(feature[:strings.Index(feature, needle)])
		if prefix == "" {
			result[i] = fmt.Sprintf("最多 %d 张%s", cardLimit, needle)
		} else {
			result[i] = fmt.Sprintf("%s最多 %d 张%s", prefix, cardLimit, needle)
		}
	}
	return result
}

func (s *Server) loadAppPlanCapabilities(ctx context.Context, plan *appPlanConfig) error {
	if s == nil || s.db == nil || plan == nil {
		return nil
	}
	var level, cycle string
	var flagsRaw, limitsRaw []byte
	err := s.db.QueryRowContext(ctx, `SELECT plan_level,billing_cycle,feature_flags,limits FROM app_plans WHERE code=$1`, plan.Code).
		Scan(&level, &cycle, &flagsRaw, &limitsRaw)
	if err != nil {
		return err
	}
	plan.PlanLevel = normalizeMembershipLevel(level)
	if cycle == "none" || cycle == "month" || cycle == "quarter" || cycle == "year" {
		plan.BillingCycle = cycle
	} else {
		plan.BillingCycle = normalizeBillingCycle(plan.PlanLevel, plan.Code)
	}
	if len(flagsRaw) > 0 {
		_ = json.Unmarshal(flagsRaw, &plan.FeatureFlags)
	}
	if len(limitsRaw) > 0 {
		_ = json.Unmarshal(limitsRaw, &plan.Limits)
	}
	return nil
}

func (s *Server) appPlan(ctx context.Context, code string) appPlanConfig {
	code = normalizeAppPlanCode(code)
	plans, err := s.loadAppPlans(ctx)
	if err == nil {
		for _, plan := range plans {
			if plan.Code == code {
				return plan
			}
		}
	}
	return normalizeLoadedAppPlan(defaultAppPlan(code))
}

func encodeAppPlanFeatures(features []string) ([]byte, error) {
	clean := make([]string, 0, len(features))
	for _, feature := range features {
		if value := strings.TrimSpace(feature); value != "" {
			clean = append(clean, value)
		}
	}
	if len(clean) > 8 {
		return nil, fmt.Errorf("套餐权益最多 8 项")
	}
	return json.Marshal(clean)
}

func (s *Server) adminAppPlans(w http.ResponseWriter, r *http.Request) {
	plans, err := s.loadAppPlans(r.Context())
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "套餐配置读取失败")
		return
	}
	httpx.OK(w, plans)
}

func (s *Server) adminAppPlanUpdate(w http.ResponseWriter, r *http.Request) {
	code := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/api/admin/app-plans/"))
	if strings.Contains(code, "/") || !supportedAppPlanCode(code) || normalizeAppPlanCode(code) != code {
		httpx.Fail(w, http.StatusBadRequest, "套餐代码不合法")
		return
	}
	var input appPlanConfig
	if err := json.NewDecoder(io.LimitReader(r.Body, 32<<10)).Decode(&input); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "套餐配置格式不正确")
		return
	}
	input.Code = code
	input.PlanLevel = normalizeMembershipLevel(input.PlanLevel)
	input.BillingCycle = normalizeBillingCycle(input.PlanLevel, input.Code)
	input.Name = strings.TrimSpace(input.Name)
	input.Subtitle = strings.TrimSpace(input.Subtitle)
	input.Badge = strings.TrimSpace(input.Badge)
	features, err := encodeAppPlanFeatures(input.Features)
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := json.Unmarshal(features, &input.Features); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "套餐权益格式不正确")
		return
	}
	if err := validateAppPlan(input); err != nil {
		httpx.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	if input.FeatureFlags == nil {
		input.FeatureFlags = map[string]bool{"deepChat": input.DeepChatEnabled, "companion": input.CompanionEnabled, "memberPoster": input.MemberPosterEnabled}
	}
	if input.Limits == nil {
		input.Limits = map[string]int{"cardLimit": input.CardLimit, "dailyChatLimit": input.DailyChatLimit, "storyMonthlyLimit": input.StoryMonthlyLimit}
	}
	featureFlags, err := json.Marshal(input.FeatureFlags)
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, "功能开关格式不正确")
		return
	}
	limits, err := json.Marshal(input.Limits)
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, "扩展额度格式不正确")
		return
	}
	before := s.appPlan(r.Context(), code)
	row := s.db.QueryRowContext(r.Context(), `UPDATE app_plans SET
		name=$2,subtitle=$3,price_cents=$4,original_price_cents=$5,badge=$6,features=$7::jsonb,
		enabled=$8,sort_order=$9,duration_days=$10,daily_chat_limit=$11,story_monthly_limit=$12,
		card_limit=$13,deep_chat_enabled=$14,companion_enabled=$15,member_poster_enabled=$16,
		plan_level=$17,billing_cycle=$18,feature_flags=$19::jsonb,limits=$20::jsonb,update_time=now()
		WHERE code=$1 RETURNING `+appPlanColumns,
		code, input.Name, input.Subtitle, input.PriceCents, input.OriginalPriceCents, input.Badge, features,
		input.Enabled, input.SortOrder, input.DurationDays, input.DailyChatLimit, input.StoryMonthlyLimit,
		input.CardLimit, input.DeepChatEnabled, input.CompanionEnabled, input.MemberPosterEnabled,
		input.PlanLevel, input.BillingCycle, featureFlags, limits)
	after, err := scanAppPlan(row)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.Fail(w, http.StatusNotFound, "套餐不存在")
		return
	}
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "套餐配置保存失败")
		return
	}
	after.PlanLevel = input.PlanLevel
	after.BillingCycle = input.BillingCycle
	after.FeatureFlags = input.FeatureFlags
	after.Limits = input.Limits
	s.recordAdminAudit(r, auditlog.Entry{Action: "app_plan.update", TargetType: "app_plan", TargetID: code, Before: before, After: after, Summary: "更新 App 套餐配置"})
	httpx.OK(w, after)
}
