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
	Code                string   `json:"code"`
	Name                string   `json:"name"`
	Subtitle            string   `json:"subtitle"`
	PriceCents          int      `json:"priceCents"`
	OriginalPriceCents  int      `json:"originalPriceCents"`
	Badge               string   `json:"badge"`
	Features            []string `json:"features"`
	Enabled             bool     `json:"enabled"`
	SortOrder           int      `json:"sortOrder"`
	DurationDays        int      `json:"durationDays"`
	DailyChatLimit      int      `json:"dailyChatLimit"`
	StoryMonthlyLimit   int      `json:"storyMonthlyLimit"`
	CardLimit           int      `json:"cardLimit"`
	DeepChatEnabled     bool     `json:"deepChatEnabled"`
	CompanionEnabled    bool     `json:"companionEnabled"`
	MemberPosterEnabled bool     `json:"memberPosterEnabled"`
}

func defaultAppPlans() []appPlanConfig {
	return []appPlanConfig{
		{Code: "free", Name: "免费版", Subtitle: "每日基础陪伴", Features: []string{"每日 5 轮基础对话", "首次 1 篇人生故事", "最多 1 张人物卡", "经典海报"}, Enabled: true, DailyChatLimit: 5, StoryMonthlyLimit: 1, CardLimit: 1},
		{Code: "vip_month", Name: "月卡会员", Subtitle: "灵活体验完整成长陪伴", PriceCents: 2900, Badge: "灵活", Features: []string{"深度对话与专业陪伴", "每月 3 篇人生故事", "最多 5 张人物卡", "2 款会员海报"}, Enabled: true, SortOrder: 10, DurationDays: 30, DailyChatLimit: -1, StoryMonthlyLimit: 3, CardLimit: 5, DeepChatEnabled: true, CompanionEnabled: true, MemberPosterEnabled: true},
		{Code: "vip_quarter", Name: "季卡会员", Subtitle: "约 ¥26.3/月，适合持续成长", PriceCents: 7900, OriginalPriceCents: 8700, Badge: "推荐", Features: []string{"深度对话与专业陪伴", "每月 5 篇人生故事", "最多 8 张人物卡", "2 款会员海报"}, Enabled: true, SortOrder: 20, DurationDays: 90, DailyChatLimit: -1, StoryMonthlyLimit: 5, CardLimit: 8, DeepChatEnabled: true, CompanionEnabled: true, MemberPosterEnabled: true},
		{Code: "vip_year", Name: "年卡会员", Subtitle: "约 ¥16.6/月，适合长期自我探索", PriceCents: 19900, OriginalPriceCents: 34800, Badge: "最划算", Features: []string{"深度对话与专业陪伴", "每月 12 篇人生故事", "最多 20 张人物卡", "2 款会员海报"}, Enabled: true, SortOrder: 30, DurationDays: 365, DailyChatLimit: -1, StoryMonthlyLimit: 12, CardLimit: 20, DeepChatEnabled: true, CompanionEnabled: true, MemberPosterEnabled: true},
	}
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
	switch normalizeAppPlanCode(code) {
	case "free", "vip_month", "vip_quarter", "vip_year":
		return true
	default:
		return false
	}
}

func validateAppPlan(plan appPlanConfig) error {
	plan.Code = strings.ToLower(strings.TrimSpace(plan.Code))
	if !supportedAppPlanCode(plan.Code) || normalizeAppPlanCode(plan.Code) != plan.Code {
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
	if plan.DailyChatLimit < -1 || plan.StoryMonthlyLimit < 0 || plan.CardLimit < 0 {
		return errors.New("套餐额度不合法")
	}
	if len(plan.Features) > 8 {
		return errors.New("套餐权益最多 8 项")
	}
	return nil
}

func defaultAppPlan(code string) appPlanConfig {
	code = normalizeAppPlanCode(code)
	for _, plan := range defaultAppPlans() {
		if plan.Code == code {
			return plan
		}
	}
	return appPlanConfig{Code: code, Name: "会员版", Subtitle: "完整成长陪伴", Enabled: true, DurationDays: 30, DailyChatLimit: -1, StoryMonthlyLimit: 3, CardLimit: 5, DeepChatEnabled: true, CompanionEnabled: true, MemberPosterEnabled: true}
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
		plans = append(plans, plan)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(plans) == 0 {
		return nil, sql.ErrNoRows
	}
	return plans, nil
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
	return defaultAppPlan(code)
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
	before := s.appPlan(r.Context(), code)
	row := s.db.QueryRowContext(r.Context(), `UPDATE app_plans SET
		name=$2,subtitle=$3,price_cents=$4,original_price_cents=$5,badge=$6,features=$7::jsonb,
		enabled=$8,sort_order=$9,duration_days=$10,daily_chat_limit=$11,story_monthly_limit=$12,
		card_limit=$13,deep_chat_enabled=$14,companion_enabled=$15,member_poster_enabled=$16,update_time=now()
		WHERE code=$1 RETURNING `+appPlanColumns,
		code, input.Name, input.Subtitle, input.PriceCents, input.OriginalPriceCents, input.Badge, features,
		input.Enabled, input.SortOrder, input.DurationDays, input.DailyChatLimit, input.StoryMonthlyLimit,
		input.CardLimit, input.DeepChatEnabled, input.CompanionEnabled, input.MemberPosterEnabled)
	after, err := scanAppPlan(row)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.Fail(w, http.StatusNotFound, "套餐不存在")
		return
	}
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "套餐配置保存失败")
		return
	}
	s.recordAdminAudit(r, auditlog.Entry{Action: "app_plan.update", TargetType: "app_plan", TargetID: code, Before: before, After: after, Summary: "更新 App 套餐配置"})
	httpx.OK(w, after)
}
