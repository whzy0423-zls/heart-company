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

const (
	appAgentDiscountAudienceSelf    = "agent_self"
	appAgentDiscountAudienceInvited = "invited_user"
	appAgentDiscountPercentOff      = "percent_off"
	appAgentDiscountAmountOff       = "amount_off"
)

var (
	errAppAgentDiscountInvalidCode         = errors.New("一级代理邀请码不存在或已失效")
	errAppAgentDiscountSelfInvite          = errors.New("不能使用自己的邀请码")
	errAppAgentDiscountAttributionConflict = errors.New("已有其他代理邀请关系，不能使用此邀请码")
)

type appAgentDiscountRule struct {
	ProductID string `json:"productId"`
	Audience  string `json:"audience"`
	Mode      string `json:"mode"`
	Value     int    `json:"value"`
	Enabled   bool   `json:"enabled"`
}

type appAgentDiscountQuote struct {
	ProductID      string `json:"productId"`
	BasePriceCents int    `json:"basePriceCents"`
	DiscountCents  int    `json:"discountCents"`
	PayableCents   int    `json:"payableCents"`
	Audience       string `json:"audience"`
	AgentID        int64  `json:"agentId"`
	AgentCode      string `json:"agentCode"`
	RuleMode       string `json:"ruleMode"`
	RuleValue      int    `json:"ruleValue"`
}

type appAgentDiscountAudience struct {
	Audience  string
	AgentID   int64
	AgentCode string
}

func validateAppAgentDiscountRule(rule appAgentDiscountRule) error {
	switch rule.ProductID {
	case "vip_month", "vip_quarter", "vip_year", "svip_month", "svip_quarter", "svip_year":
	default:
		return errors.New("请选择有效的付费套餐")
	}
	if rule.Audience != appAgentDiscountAudienceSelf && rule.Audience != appAgentDiscountAudienceInvited {
		return errors.New("优惠对象不合法")
	}
	switch rule.Mode {
	case appAgentDiscountPercentOff:
		if rule.Value < 0 || rule.Value >= 10000 || (rule.Enabled && rule.Value == 0) {
			return errors.New("百分比减免必须在 0% 到 100% 之间")
		}
	case appAgentDiscountAmountOff:
		if rule.Value < 0 || (rule.Enabled && rule.Value == 0) {
			return errors.New("固定减免金额必须大于 0 分")
		}
	default:
		return errors.New("优惠方式不合法")
	}
	return nil
}

func calculateAppAgentDiscount(basePriceCents int, mode string, value int) (discountCents, payableCents int) {
	if basePriceCents <= 0 || value <= 0 {
		return 0, basePriceCents
	}
	switch mode {
	case appAgentDiscountPercentOff:
		discountCents = int((int64(basePriceCents)*int64(value) + 5000) / 10000)
	case appAgentDiscountAmountOff:
		discountCents = value
	default:
		return 0, basePriceCents
	}
	if discountCents >= basePriceCents {
		discountCents = basePriceCents - 1
	}
	return discountCents, basePriceCents - discountCents
}

func (s *Server) loadAppAgentDiscountRules(ctx context.Context) ([]appAgentDiscountRule, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT product_id,audience,mode,value,enabled FROM app_agent_discount_rules ORDER BY product_id,audience`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	rules := []appAgentDiscountRule{}
	for rows.Next() {
		var rule appAgentDiscountRule
		if err := rows.Scan(&rule.ProductID, &rule.Audience, &rule.Mode, &rule.Value, &rule.Enabled); err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	return rules, rows.Err()
}

func (s *Server) adminAppAgentDiscounts(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		rules, err := s.loadAppAgentDiscountRules(r.Context())
		if err != nil {
			httpx.Fail(w, http.StatusInternalServerError, "读取代理优惠配置失败")
			return
		}
		httpx.OK(w, map[string]any{"rules": rules})
		return
	}
	if r.Method != http.MethodPut {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var input struct {
		Rules []appAgentDiscountRule `json:"rules"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 32<<10)).Decode(&input); err != nil || input.Rules == nil || len(input.Rules) > 12 {
		httpx.Fail(w, http.StatusBadRequest, "优惠配置格式不正确")
		return
	}
	seen := map[string]bool{}
	for i := range input.Rules {
		rule := &input.Rules[i]
		rule.ProductID = strings.TrimSpace(rule.ProductID)
		rule.Audience = strings.TrimSpace(rule.Audience)
		rule.Mode = strings.TrimSpace(rule.Mode)
		if err := validateAppAgentDiscountRule(*rule); err != nil {
			httpx.Fail(w, http.StatusBadRequest, err.Error())
			return
		}
		key := rule.ProductID + "/" + rule.Audience
		if seen[key] {
			httpx.Fail(w, http.StatusBadRequest, "同一套餐和优惠对象不能重复配置")
			return
		}
		seen[key] = true
	}
	before, err := s.loadAppAgentDiscountRules(r.Context())
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "读取代理优惠配置失败")
		return
	}
	tx, err := s.db.BeginTx(r.Context(), &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "保存代理优惠配置失败")
		return
	}
	defer tx.Rollback()
	for _, rule := range input.Rules {
		if !rule.Enabled || rule.Mode != appAgentDiscountAmountOff {
			continue
		}
		var price int
		if err := tx.QueryRowContext(r.Context(), `SELECT price_cents FROM app_plans WHERE code=$1`, rule.ProductID).Scan(&price); err != nil || price <= rule.Value {
			httpx.Fail(w, http.StatusBadRequest, "固定减免必须低于当前套餐价格")
			return
		}
	}
	if _, err := tx.ExecContext(r.Context(), `DELETE FROM app_agent_discount_rules`); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "保存代理优惠配置失败")
		return
	}
	for _, rule := range input.Rules {
		if _, err := tx.ExecContext(r.Context(), `INSERT INTO app_agent_discount_rules(product_id,audience,mode,value,enabled) VALUES($1,$2,$3,$4,$5)`, rule.ProductID, rule.Audience, rule.Mode, rule.Value, rule.Enabled); err != nil {
			httpx.Fail(w, http.StatusInternalServerError, "保存代理优惠配置失败")
			return
		}
	}
	if err := tx.Commit(); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "保存代理优惠配置失败")
		return
	}
	after, err := s.loadAppAgentDiscountRules(r.Context())
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "读取代理优惠配置失败")
		return
	}
	s.recordAdminAudit(r, auditlog.Entry{Action: "app_agent_discount.update", TargetType: "app_agent_discount", TargetID: "global", Before: before, After: after, Summary: "更新一级代理会员优惠"})
	httpx.OK(w, map[string]any{"rules": after})
}

func (s *Server) resolveAppAgentDiscountAudience(ctx context.Context, userID int64, rawCode string) (appAgentDiscountAudience, error) {
	code := strings.TrimSpace(rawCode)
	if len(code) > 80 {
		return appAgentDiscountAudience{}, errAppAgentDiscountInvalidCode
	}
	var ownID int64
	var ownLevel int
	var ownStatus, ownCode string
	err := s.db.QueryRowContext(ctx, `SELECT id,level,status,agent_code FROM distribution_agents WHERE app_user_id=$1`, userID).Scan(&ownID, &ownLevel, &ownStatus, &ownCode)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return appAgentDiscountAudience{}, err
	}
	var directRootID int64
	var directRootLevel int
	var directRootStatus, directRootCode string
	err = s.db.QueryRowContext(ctx, `SELECT root.id,root.level,root.status,root.agent_code FROM distribution_user_relations r JOIN distribution_agents direct ON direct.id=r.direct_agent_id JOIN distribution_agents root ON root.id=direct.root_agent_id WHERE r.app_user_id=$1`, userID).Scan(&directRootID, &directRootLevel, &directRootStatus, &directRootCode)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return appAgentDiscountAudience{}, err
	}
	if code == "" {
		if ownLevel == 1 && ownStatus == "active" {
			return appAgentDiscountAudience{Audience: appAgentDiscountAudienceSelf, AgentID: ownID, AgentCode: ownCode}, nil
		}
		if directRootLevel == 1 && directRootStatus == "active" {
			return appAgentDiscountAudience{Audience: appAgentDiscountAudienceInvited, AgentID: directRootID, AgentCode: directRootCode}, nil
		}
		return appAgentDiscountAudience{}, nil
	}
	var selected appAgentDiscountAudience
	var selectedUserID int64
	err = s.db.QueryRowContext(ctx, `SELECT id,app_user_id,agent_code FROM distribution_agents WHERE lower(agent_code)=lower($1) AND level=1 AND status='active'`, code).Scan(&selected.AgentID, &selectedUserID, &selected.AgentCode)
	if errors.Is(err, sql.ErrNoRows) {
		return appAgentDiscountAudience{}, errAppAgentDiscountInvalidCode
	}
	if err != nil {
		return appAgentDiscountAudience{}, err
	}
	if selectedUserID == userID {
		return appAgentDiscountAudience{}, errAppAgentDiscountSelfInvite
	}
	if (ownLevel == 1 && ownID != selected.AgentID) || (directRootID != 0 && directRootID != selected.AgentID) {
		return appAgentDiscountAudience{}, errAppAgentDiscountAttributionConflict
	}
	selected.Audience = appAgentDiscountAudienceInvited
	return selected, nil
}

func (s *Server) quoteAppAgentDiscount(ctx context.Context, userID int64, productID, agentCode string, basePriceCents int) (appAgentDiscountQuote, error) {
	audience, err := s.resolveAppAgentDiscountAudience(ctx, userID, agentCode)
	if err != nil {
		return appAgentDiscountQuote{}, err
	}
	return s.quoteAppAgentDiscountForAudience(ctx, productID, basePriceCents, audience)
}

func (s *Server) quoteAppAgentDiscountForAudience(ctx context.Context, productID string, basePriceCents int, audience appAgentDiscountAudience) (appAgentDiscountQuote, error) {
	quote := appAgentDiscountQuote{
		ProductID: productID, BasePriceCents: basePriceCents, PayableCents: basePriceCents,
		Audience: audience.Audience, AgentID: audience.AgentID, AgentCode: audience.AgentCode,
	}
	if audience.Audience == "" || basePriceCents <= 0 {
		return quote, nil
	}
	err := s.db.QueryRowContext(ctx, `SELECT mode,value FROM app_agent_discount_rules WHERE product_id=$1 AND audience=$2 AND enabled=true`, productID, audience.Audience).Scan(&quote.RuleMode, &quote.RuleValue)
	if errors.Is(err, sql.ErrNoRows) {
		return quote, nil
	}
	if err != nil {
		return appAgentDiscountQuote{}, err
	}
	if err := validateAppAgentDiscountRule(appAgentDiscountRule{ProductID: productID, Audience: audience.Audience, Mode: quote.RuleMode, Value: quote.RuleValue, Enabled: true}); err != nil {
		return appAgentDiscountQuote{}, fmt.Errorf("invalid stored agent discount rule: %w", err)
	}
	quote.DiscountCents, quote.PayableCents = calculateAppAgentDiscount(basePriceCents, quote.RuleMode, quote.RuleValue)
	return quote, nil
}

func (s *Server) appBillingDiscountQuote(w http.ResponseWriter, r *http.Request) {
	user, ok := appUserFromContext(r)
	if !ok {
		httpx.Fail(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var input struct {
		AgentCode string `json:"agentCode"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4<<10)).Decode(&input); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "邀请码格式不正确")
		return
	}
	audience, err := s.resolveAppAgentDiscountAudience(r.Context(), user.ID, input.AgentCode)
	if err != nil {
		switch {
		case errors.Is(err, errAppAgentDiscountInvalidCode), errors.Is(err, errAppAgentDiscountSelfInvite):
			httpx.Fail(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, errAppAgentDiscountAttributionConflict):
			httpx.Fail(w, http.StatusConflict, err.Error())
		default:
			httpx.Fail(w, http.StatusInternalServerError, "读取代理优惠失败")
		}
		return
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT code,price_cents FROM app_plans WHERE enabled=true AND price_cents>0 AND code IN ('vip_month','vip_quarter','vip_year','svip_month','svip_quarter','svip_year') ORDER BY CASE code WHEN 'vip_month' THEN 1 WHEN 'vip_quarter' THEN 2 WHEN 'vip_year' THEN 3 WHEN 'svip_month' THEN 4 WHEN 'svip_quarter' THEN 5 WHEN 'svip_year' THEN 6 END`)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "读取套餐价格失败")
		return
	}
	products := []struct {
		ID         string
		PriceCents int
	}{}
	for rows.Next() {
		var product struct {
			ID         string
			PriceCents int
		}
		if err := rows.Scan(&product.ID, &product.PriceCents); err != nil {
			rows.Close()
			httpx.Fail(w, http.StatusInternalServerError, "读取套餐价格失败")
			return
		}
		products = append(products, product)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		httpx.Fail(w, http.StatusInternalServerError, "读取套餐价格失败")
		return
	}
	rows.Close()
	prices := make([]appAgentDiscountQuote, 0, len(products))
	for _, product := range products {
		quote, err := s.quoteAppAgentDiscountForAudience(r.Context(), product.ID, product.PriceCents, audience)
		if err != nil {
			httpx.Fail(w, http.StatusInternalServerError, "计算代理优惠失败")
			return
		}
		prices = append(prices, quote)
	}
	httpx.OK(w, map[string]any{"audience": audience.Audience, "agentCode": audience.AgentCode, "prices": prices})
}
