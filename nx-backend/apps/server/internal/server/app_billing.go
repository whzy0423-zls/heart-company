package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"nine-xing/nx-backend/apps/server/internal/httpx"
	"nine-xing/nx-backend/apps/server/internal/xznpay"
)

type appEntitlementResp struct {
	PlanName                  string                             `json:"planName"`
	PlanCode                  string                             `json:"planCode"`
	PlanLevel                 string                             `json:"planLevel"`
	BillingCycle              string                             `json:"billingCycle"`
	Features                  []string                           `json:"features"`
	FeatureFlags              map[string]bool                    `json:"featureFlags,omitempty"`
	Quotas                    map[string]any                     `json:"quotas"`
	ResourceAccess            map[string][]appResourceAccessResp `json:"resourceAccess"`
	DowngradedAt              string                             `json:"downgradedAt,omitempty"`
	RetentionUntil            string                             `json:"retentionUntil,omitempty"`
	IsMember                  bool                               `json:"isMember"`
	ChatLimit                 int                                `json:"chatLimit"`
	ChatRemaining             int                                `json:"chatRemaining"`
	TrialChatRemaining        int                                `json:"trialChatRemaining"`
	TrialChatNearestExpiresAt string                             `json:"trialChatNearestExpiresAt,omitempty"`
	DeepReportRemaining       int                                `json:"deepReportRemaining"`
	StoryLimit                int                                `json:"storyLimit"`
	StoryRemaining            int                                `json:"storyRemaining"`
	CardLimit                 int                                `json:"cardLimit"`
	CardUsed                  int                                `json:"cardUsed"`
	StartedAt                 string                             `json:"startedAt,omitempty"`
	ExpiresAt                 string                             `json:"expiresAt,omitempty"`
	PendingOrder              *appOrderResp                      `json:"pendingOrder,omitempty"`
}

type appProductResp struct {
	ID                string   `json:"id"`
	PlanLevel         string   `json:"planLevel"`
	BillingCycle      string   `json:"billingCycle"`
	Title             string   `json:"title"`
	Subtitle          string   `json:"subtitle"`
	PriceText         string   `json:"priceText"`
	OriginalPriceText string   `json:"originalPriceText,omitempty"`
	Badge             string   `json:"badge,omitempty"`
	Features          []string `json:"features"`
	// PaymentChannels is the stable App contract. The provider-specific
	// channel code is deliberately hidden from clients; XZN's wxpay is exposed
	// as the product-level "wechat" option.
	PaymentChannels []appPaymentChannel `json:"paymentChannels,omitempty"`
	// PayChannels remains for older clients that only understand string codes.
	PayChannels         []string `json:"payChannels,omitempty"`
	Enabled             bool     `json:"enabled"`
	PayEnabled          bool     `json:"payEnabled"`
	ConfigurationStatus string   `json:"configurationStatus"`
	DisabledReason      string   `json:"disabledReason,omitempty"`
	PurchaseMode        string   `json:"purchaseMode"`
	DurationDays        int      `json:"durationDays"`
}

type appPaymentChannel struct {
	Code              string `json:"code"`
	Name              string `json:"name"`
	Enabled           bool   `json:"enabled"`
	UnavailableReason string `json:"unavailableReason,omitempty"`
}

const (
	appPurchaseModeCustomerService = "customer_service"
	appPurchaseModeXZN             = "xzn"
	appOrderPendingConfirmation    = "pending_confirmation"
	appCustomerServiceQRURL        = "/api/public/customer-service-qr"
	appPaymentProviderXZN          = "xzn"
)

func appCardLimit(memberLevel string) int {
	return appMembershipBenefits(memberLevel).CardLimit
}

func appPlanCode(memberLevel string) string {
	memberLevel = strings.ToLower(strings.TrimSpace(memberLevel))
	switch memberLevel {
	case "", "free":
		return "free"
	case "vip":
		return "vip_month"
	case "svip":
		// Keep S VIP as its own canonical plan. `vip_year` is the legacy
		// annual VIP SKU and carries the normalized VIP tier, not S VIP.
		return "svip"
	default:
		return memberLevel
	}
}

func cloneAppBoolMap(source map[string]bool) map[string]bool {
	if source == nil {
		return nil
	}
	result := make(map[string]bool, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func cloneAppIntMap(source map[string]int) map[string]int {
	if source == nil {
		return nil
	}
	result := make(map[string]int, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func appPlanName(planCode string) string {
	return appMembershipBenefits(planCode).PlanName
}

func (s *Server) appBillingEntitlements(w http.ResponseWriter, r *http.Request) {
	userInfo, ok := appUserFromContext(r)
	if !ok {
		httpx.Fail(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var memberLevel string
	var memberStartedAt sql.NullTime
	var memberExpiresAt sql.NullTime
	if err := s.db.QueryRowContext(r.Context(), `
		SELECT member_level, member_started_at, member_expires_at
		FROM app_users WHERE id=$1 AND status='active'`, userInfo.ID).Scan(
		&memberLevel, &memberStartedAt, &memberExpiresAt,
	); err != nil {
		httpx.Fail(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var cardUsed int
	_ = s.db.QueryRowContext(r.Context(),
		`SELECT count(*) FROM app_user_cards
		 WHERE app_user_id = $1 AND card_type='secondary' AND status='active'`,
		userInfo.ID).Scan(&cardUsed)
	var membershipExpiry *time.Time
	if memberExpiresAt.Valid {
		membershipExpiry = &memberExpiresAt.Time
	}
	planCode := appEffectivePlanCode(memberLevel, membershipExpiry, time.Now())
	resolvedPlan := resolveMembershipPlan(memberLevel, planCode, membershipExpiry, time.Now())
	resourceStates, _ := s.recomputeCardResourceAccess(r.Context(), userInfo.ID, resolvedPlan.PlanLevel)
	isMember := resolvedPlan.Active && resolvedPlan.PlanLevel != "free"
	startedAt := ""
	expiresAt := ""
	if isMember && memberStartedAt.Valid {
		startedAt = memberStartedAt.Time.Format(time.RFC3339)
	}
	if isMember && memberExpiresAt.Valid {
		expiresAt = memberExpiresAt.Time.Format(time.RFC3339)
	}
	plan := s.appPlan(r.Context(), planCode)
	// Canonical SVIP metadata is independent of the historical vip_year SKU;
	// keep the old SKU/name for VIP clients while exposing the new level fields.
	if resolvedPlan.PlanLevel == "svip" {
		canonical := defaultMembershipLevelPlan("svip")
		// Overlay every capability field used by this response. A disabled,
		// zero-priced seed row is intentionally valid for configuration, but it
		// must not make an already-active S VIP member look like a free user.
		plan.Code = canonical.Code
		plan.PlanLevel = canonical.PlanLevel
		plan.BillingCycle = canonical.BillingCycle
		plan.Name = canonical.Name
		plan.Features = append([]string(nil), canonical.Features...)
		plan.FeatureFlags = cloneAppBoolMap(canonical.FeatureFlags)
		plan.Limits = cloneAppIntMap(canonical.Limits)
		plan.DurationDays = canonical.DurationDays
		plan.DailyChatLimit = canonical.DailyChatLimit
		plan.StoryMonthlyLimit = canonical.StoryMonthlyLimit
		plan.CardLimit = canonical.CardLimit
		plan.DeepChatEnabled = canonical.DeepChatEnabled
		plan.CompanionEnabled = canonical.CompanionEnabled
		plan.MemberPosterEnabled = canonical.MemberPosterEnabled
	}
	storyQuota := s.lifeStoryQuotaForPlan(r.Context(), userInfo.ID, planCode)
	chatQuota := newAppChatQuotaSnapshot(plan.DailyChatLimit, 0, 0)
	if s.appChatQuota != nil {
		if snapshot, err := s.appChatQuota.Snapshot(r.Context(), userInfo.ID, plan.DailyChatLimit, time.Now()); err == nil {
			chatQuota = snapshot
		}
	}
	var pendingOrder *appOrderResp
	if order, found, err := s.findPendingAppOrder(r.Context(), userInfo.ID); err == nil && found {
		enriched, enrichErr := s.enrichCustomerServiceOrder(r.Context(), userInfo.ID, order)
		if enrichErr == nil {
			pendingOrder = &enriched
		}
	}
	cardActive, cardReadOnly, cardLocked := 0, 0, 0
	for _, state := range resourceStates {
		switch state.State {
		case resourceAccessActive:
			cardActive++
		case resourceAccessReadOnlyOverLimit:
			cardReadOnly++
		case resourceAccessLockedUpgrade:
			cardLocked++
		}
	}
	cardQuota := map[string]int{
		"limit":    membershipLevelCardLimit(resolvedPlan.PlanLevel),
		"used":     cardUsed,
		"active":   cardActive,
		"readOnly": cardReadOnly,
		"locked":   cardLocked,
	}
	quotas := map[string]any{
		"card":              cardQuota,
		"cards":             cardQuota,
		"cardLimit":         membershipLevelCardLimit(resolvedPlan.PlanLevel),
		"legacyCardLimit":   plan.CardLimit,
		"cardUsed":          cardUsed,
		"cardActive":        cardActive,
		"cardReadOnly":      cardReadOnly,
		"cardLocked":        cardLocked,
		"dailyChatLimit":    plan.DailyChatLimit,
		"storyMonthlyLimit": plan.StoryMonthlyLimit,
	}
	retentionUntil := ""
	for _, state := range resourceStates {
		if state.RetentionUntil != nil && (retentionUntil == "" || state.RetentionUntil.Format(time.RFC3339) > retentionUntil) {
			retentionUntil = state.RetentionUntil.Format(time.RFC3339)
		}
	}
	downgradedAt := ""
	if resolvedPlan.DowngradedAt != nil {
		downgradedAt = resolvedPlan.DowngradedAt.Format(time.RFC3339)
	}
	httpx.OK(w, appEntitlementResp{
		PlanName:                  plan.Name,
		PlanCode:                  planCode,
		PlanLevel:                 resolvedPlan.PlanLevel,
		BillingCycle:              resolvedPlan.BillingCycle,
		Features:                  append([]string(nil), plan.Features...),
		FeatureFlags:              plan.FeatureFlags,
		Quotas:                    quotas,
		ResourceAccess:            appResourceAccessForEntitlement(resourceStates),
		DowngradedAt:              downgradedAt,
		RetentionUntil:            retentionUntil,
		IsMember:                  isMember,
		ChatLimit:                 chatQuota.Limit,
		ChatRemaining:             chatQuota.Remaining,
		TrialChatRemaining:        chatQuota.TrialRemaining,
		TrialChatNearestExpiresAt: chatQuota.TrialNearestExpiresAt,
		DeepReportRemaining:       0,
		StoryLimit:                storyQuota.Limit,
		StoryRemaining:            storyQuota.Remaining,
		CardLimit:                 plan.CardLimit,
		CardUsed:                  cardUsed,
		StartedAt:                 startedAt,
		ExpiresAt:                 expiresAt,
		PendingOrder:              pendingOrder,
	})
}

func (s *Server) appBillingProducts(w http.ResponseWriter, r *http.Request) {
	mode, err := s.loadAppPaymentMode(r.Context())
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "读取支付模式失败")
		return
	}
	cfg, _ := s.loadXZNConfig(r.Context())
	plans, err := s.loadAppPlans(r.Context())
	if err != nil {
		plans = defaultAppPlans()
	}
	products := make([]appProductResp, 0, 3)
	for _, plan := range plans {
		if plan.Code == "free" {
			continue
		}
		products = append(products, appProductForPaymentMode(mode, cfg, appProductFromPlan(plan)))
	}
	httpx.OK(w, products)
}

func appProductFromPlan(plan appPlanConfig) appProductResp {
	plan = normalizeLoadedAppPlan(plan)
	return appProductResp{
		ID:                plan.Code,
		PlanLevel:         plan.PlanLevel,
		BillingCycle:      plan.BillingCycle,
		Title:             plan.Name,
		Subtitle:          plan.Subtitle,
		PriceText:         formatAppPlanPrice(plan.PriceCents),
		OriginalPriceText: formatAppPlanPrice(plan.OriginalPriceCents),
		Badge:             plan.Badge,
		Features:          append([]string(nil), plan.Features...),
		Enabled:           plan.Enabled,
		DurationDays:      plan.DurationDays,
	}
}

func formatAppPlanPrice(cents int) string {
	if cents <= 0 {
		return ""
	}
	if cents%100 == 0 {
		return fmt.Sprintf("¥%d", cents/100)
	}
	return fmt.Sprintf("¥%d.%02d", cents/100, cents%100)
}

func appProductForPaymentMode(mode string, cfg xznPaymentConfig, product appProductResp) appProductResp {
	if !product.Enabled {
		return appXZNProductWithStatus(product, "plan_disabled", "套餐已下架")
	}
	if mode != appPurchaseModeXZN {
		return appCustomerServiceProduct(product)
	}
	channels := appPaymentChannelsForConfig(cfg)
	product.PaymentChannels = channels
	product.PayChannels = make([]string, 0, len(channels))
	for _, channel := range channels {
		if channel.Enabled {
			product.PayChannels = append(product.PayChannels, channel.Code)
		}
	}
	if !xznCredentialsConfigured(cfg) {
		return appXZNProductWithStatus(product, "payment_not_configured", "在线支付尚未配置，请稍后重试")
	}
	if !cfg.Enabled {
		return appXZNProductWithStatus(product, "payment_disabled", "在线支付暂未开放")
	}
	if len(xznAvailableAppChannels(cfg)) == 0 {
		return appXZNProductWithStatus(product, "payment_channel_unavailable", "当前没有可用的支付渠道")
	}
	product.PayEnabled = true
	product.PurchaseMode = appPurchaseModeXZN
	product.ConfigurationStatus = "configured"
	product.DisabledReason = ""
	return product
}

func appCustomerServiceProduct(product appProductResp) appProductResp {
	product.PayEnabled = false
	product.PurchaseMode = appPurchaseModeCustomerService
	return product
}

func appXZNProductWithStatus(product appProductResp, status, reason string) appProductResp {
	product.PayEnabled = false
	product.PurchaseMode = appPurchaseModeXZN
	product.ConfigurationStatus = status
	product.DisabledReason = reason
	return product
}

type appOrderCreateReq struct {
	ProductID  string `json:"productId"`
	PayChannel string `json:"payChannel"`
}

type appOrderResp struct {
	OutTradeNo           string         `json:"outTradeNo"`
	ProductID            string         `json:"productId"`
	Title                string         `json:"title"`
	Amount               int            `json:"amount"`
	Status               string         `json:"status"`
	PayStatus            string         `json:"payStatus"`
	PayEnabled           bool           `json:"payEnabled"`
	ConfigurationStatus  string         `json:"configurationStatus"`
	DisabledReason       string         `json:"disabledReason,omitempty"`
	PaymentProvider      string         `json:"paymentProvider,omitempty"`
	PayChannel           string         `json:"payChannel,omitempty"`
	GatewayID            string         `json:"gatewayId,omitempty"`
	ProviderTradeNo      string         `json:"providerTradeNo,omitempty"`
	ProviderStatus       string         `json:"providerStatus,omitempty"`
	PayURL               string         `json:"payUrl,omitempty"`
	LastQueryAt          string         `json:"lastQueryAt,omitempty"`
	PaymentError         string         `json:"paymentError,omitempty"`
	Payment              map[string]any `json:"payment,omitempty"`
	PayParams            map[string]any `json:"payParams,omitempty"`
	Message              string         `json:"message"`
	PurchaseMode         string         `json:"purchaseMode"`
	CustomerServiceQRURL string         `json:"customerServiceQrUrl"`
	DurationDays         int            `json:"durationDays"`
	CurrentExpiresAt     string         `json:"currentExpiresAt,omitempty"`
	EstimatedExpiresAt   string         `json:"estimatedExpiresAt,omitempty"`
}

func appProductTitle(productID string) string {
	switch productID {
	case "vip_month":
		return "月卡会员"
	case "vip_quarter":
		return "季卡会员"
	case "vip_year":
		return "年卡会员"
	case "svip":
		return "S VIP"
	default:
		return ""
	}
}

func appProductAmount(productID string) int {
	switch productID {
	case "vip_month":
		return 2900
	case "vip_quarter":
		return 7900
	case "vip_year":
		return 19900
	case "svip":
		// S VIP is seeded unpublished and therefore has no fallback price.
		// A configured plan is priced from app_plans; this helper only serves
		// legacy callers that need a deterministic amount.
		return 0
	default:
		return 0
	}
}

func normalizeAppPayChannel(channel string) string {
	switch strings.ToLower(strings.TrimSpace(channel)) {
	case "alipay", "ali_pay", "ali":
		return "alipay"
	case "wxpay", "wechat", "weixin", "wechatpay":
		return "wxpay"
	default:
		return ""
	}
}

func displayAppPayChannel(channel string) string {
	switch normalizeAppPayChannel(channel) {
	case "alipay":
		return "alipay"
	case "wxpay":
		return "wechat"
	default:
		return ""
	}
}

func xznCredentialsConfigured(cfg xznPaymentConfig) bool {
	return strings.TrimSpace(cfg.PID) != "" && strings.TrimSpace(cfg.Secret) != "" && strings.TrimSpace(cfg.NotifyURL) != ""
}

func xznAvailableAppChannels(cfg xznPaymentConfig) []string {
	if !xznCredentialsConfigured(cfg) || !cfg.Enabled {
		return nil
	}
	channels := make([]string, 0, 2)
	if cfg.AlipayEnabled && strings.TrimSpace(cfg.AlipayGatewayID) != "" {
		channels = append(channels, "alipay")
	}
	if cfg.WechatEnabled && strings.TrimSpace(cfg.WechatGatewayID) != "" && !isXZNAppBlockedWechatGateway(cfg.WechatGatewayID) {
		channels = append(channels, "wxpay")
	}
	return channels
}

func appPaymentChannelsForConfig(cfg xznPaymentConfig) []appPaymentChannel {
	if !xznCredentialsConfigured(cfg) || !cfg.Enabled {
		return nil
	}
	channels := make([]appPaymentChannel, 0, 2)
	alipay := appPaymentChannel{Code: "alipay", Name: "支付宝", Enabled: cfg.AlipayEnabled}
	if !cfg.AlipayEnabled {
		alipay.UnavailableReason = "支付宝支付暂未开放"
	} else if strings.TrimSpace(cfg.AlipayGatewayID) == "" {
		alipay.Enabled = false
		alipay.UnavailableReason = "支付宝 App 网关未配置"
	}
	channels = append(channels, alipay)

	wechat := appPaymentChannel{Code: "wechat", Name: "微信支付", Enabled: cfg.WechatEnabled}
	switch {
	case !cfg.WechatEnabled:
		wechat.UnavailableReason = "微信支付暂未开放"
	case strings.TrimSpace(cfg.WechatGatewayID) == "":
		wechat.Enabled = false
		wechat.UnavailableReason = "微信 App 网关未配置"
	case isXZNAppBlockedWechatGateway(cfg.WechatGatewayID):
		wechat.Enabled = false
		wechat.UnavailableReason = "当前微信网关不支持 App 支付"
	}
	channels = append(channels, wechat)
	return channels
}

func xznAppGateway(cfg xznPaymentConfig, channel string) (string, bool) {
	if !xznCredentialsConfigured(cfg) || !cfg.Enabled {
		return "", false
	}
	switch normalizeAppPayChannel(channel) {
	case "alipay":
		return strings.TrimSpace(cfg.AlipayGatewayID), cfg.AlipayEnabled && strings.TrimSpace(cfg.AlipayGatewayID) != ""
	case "wxpay":
		gateway := strings.TrimSpace(cfg.WechatGatewayID)
		return gateway, cfg.WechatEnabled && gateway != "" && !isXZNAppBlockedWechatGateway(gateway)
	default:
		return "", false
	}
}

func xznDefaultAppChannel(cfg xznPaymentConfig) (string, string, bool) {
	for _, channel := range []string{"alipay", "wxpay"} {
		if gateway, ok := xznAppGateway(cfg, channel); ok {
			return channel, gateway, true
		}
	}
	return "", "", false
}

func (s *Server) appBillingCreateOrder(w http.ResponseWriter, r *http.Request) {
	userInfo, ok := appUserFromContext(r)
	if !ok {
		httpx.Fail(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var body appOrderCreateReq
	if err := json.NewDecoder(io.LimitReader(r.Body, 4<<10)).Decode(&body); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid body")
		return
	}
	productID := strings.TrimSpace(body.ProductID)
	plan := s.appPlan(r.Context(), productID)
	title, amount, durationDays := plan.Name, plan.PriceCents, plan.DurationDays
	if productID == "free" || plan.Code != productID || !plan.Enabled || title == "" || amount <= 0 || durationDays <= 0 {
		httpx.Fail(w, http.StatusBadRequest, "invalid product")
		return
	}
	if existing, found, err := s.findPendingAppOrder(r.Context(), userInfo.ID); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "server error")
		return
	} else if found {
		enriched, enrichErr := s.enrichCustomerServiceOrder(r.Context(), userInfo.ID, existing)
		if enrichErr != nil {
			httpx.Fail(w, http.StatusInternalServerError, "server error")
			return
		}
		httpx.OK(w, enriched)
		return
	}
	mode, err := s.loadAppPaymentMode(r.Context())
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "读取支付模式失败")
		return
	}
	if mode == appPurchaseModeCustomerService {
		s.createCustomerServiceAppOrder(w, r, userInfo.ID, productID, title, amount, durationDays)
		return
	}
	cfg, _ := s.loadXZNConfig(r.Context())
	if strings.TrimSpace(body.PayChannel) == "" && cfg.Enabled && xznCredentialsConfigured(cfg) {
		httpx.Fail(w, http.StatusBadRequest, "请选择支付渠道")
		return
	}
	payChannel := normalizeAppPayChannel(body.PayChannel)
	gatewayID, online := "", false
	if payChannel == "" {
		payChannel, gatewayID, online = xznDefaultAppChannel(cfg)
	} else {
		gatewayID, online = xznAppGateway(cfg, payChannel)
	}
	if body.PayChannel != "" && !online {
		httpx.Fail(w, http.StatusServiceUnavailable, "所选支付渠道暂不可用")
		return
	}
	if !online {
		// New orders must never silently fall back to the retired customer-service
		// transfer path. Historical manual orders remain readable and grantable.
		httpx.Fail(w, http.StatusServiceUnavailable, "在线支付暂不可用，请稍后重试")
		return
	}
	outTradeNo := fmt.Sprintf("app%d-%s-%d", userInfo.ID, productID, time.Now().UnixNano())
	if online {
		if _, err := s.db.ExecContext(r.Context(), `
				INSERT INTO app_orders (out_trade_no, app_user_id, product_id, title, amount, duration_days, status, purchase_mode, payment_provider, pay_channel, gateway_id)
				VALUES ($1, $2, $3, $4, $5, $6, 'pending', 'xzn', 'xzn', $7, $8)`,
			outTradeNo, userInfo.ID, productID, title, amount, durationDays, payChannel, gatewayID); err != nil {
			if existing, found, findErr := s.findPendingAppOrder(r.Context(), userInfo.ID); findErr == nil && found {
				if enriched, enrichErr := s.enrichCustomerServiceOrder(r.Context(), userInfo.ID, existing); enrichErr == nil {
					httpx.OK(w, enriched)
					return
				}
			}
			httpx.Fail(w, http.StatusInternalServerError, "创建支付订单失败")
			return
		}
		client, _, clientErr := s.newXZNClient(r.Context())
		if clientErr != nil {
			_, _ = s.db.ExecContext(r.Context(), `UPDATE app_orders SET status='failed', payment_error=$2, update_time=now() WHERE out_trade_no=$1`, outTradeNo, "支付配置不可用")
			httpx.Fail(w, http.StatusServiceUnavailable, "在线支付暂不可用，请稍后重试")
			return
		}
		created, createErr := client.Create(r.Context(), xznpay.CreateRequest{
			OutTradeNo:  outTradeNo,
			TotalAmount: fmt.Sprintf("%d.%02d", amount/100, amount%100),
			Subject:     title,
			PaytypeCode: payChannel,
			ChannelID:   gatewayID,
			Attach:      strconv.FormatInt(userInfo.ID, 10),
			ClientIP:    s.clientIP(r),
			NotifyURL:   cfg.NotifyURL,
			ReturnURL:   xznOrderReturnURL(cfg.ReturnURL, outTradeNo),
		})
		if createErr != nil {
			_, _ = s.db.ExecContext(r.Context(), `UPDATE app_orders SET status='failed', payment_error=$2, update_time=now() WHERE out_trade_no=$1`, outTradeNo, createErr.Error())
			httpx.Fail(w, http.StatusBadGateway, "支付平台下单失败，请稍后重试")
			return
		}
		if _, err := s.db.ExecContext(r.Context(), `
			UPDATE app_orders
			SET provider_trade_no=$2, provider_status='WAIT_BUYER_PAY', pay_url=$3, update_time=now(), payment_error=''
			WHERE out_trade_no=$1`, outTradeNo, created.TradeNo, created.PayURL); err != nil {
			httpx.Fail(w, http.StatusInternalServerError, "保存支付订单失败")
			return
		}
		order, err := s.enrichCustomerServiceOrder(r.Context(), userInfo.ID, appOrderResp{
			OutTradeNo:      outTradeNo,
			ProductID:       productID,
			Title:           title,
			Amount:          amount,
			DurationDays:    durationDays,
			Status:          "pending",
			PaymentProvider: appPaymentProviderXZN,
			PayChannel:      payChannel,
			GatewayID:       gatewayID,
			ProviderTradeNo: created.TradeNo,
			ProviderStatus:  "WAIT_BUYER_PAY",
			PayURL:          created.PayURL,
		})
		if err != nil {
			httpx.Fail(w, http.StatusInternalServerError, "读取支付订单失败")
			return
		}
		httpx.OK(w, order)
		return
	}
	httpx.Fail(w, http.StatusServiceUnavailable, "支付暂不可用")
}

func (s *Server) createCustomerServiceAppOrder(w http.ResponseWriter, r *http.Request, appUserID int64, productID, title string, amount, durationDays int) {
	outTradeNo := fmt.Sprintf("app%d-%s-%d", appUserID, productID, time.Now().UnixNano())
	if _, err := s.db.ExecContext(r.Context(),
		`INSERT INTO app_orders (out_trade_no, app_user_id, product_id, title, amount, duration_days, status, purchase_mode, payment_provider)
			 VALUES ($1, $2, $3, $4, $5, $6, 'pending_confirmation', 'customer_service', 'manual')`,
		outTradeNo, appUserID, productID, title, amount, durationDays); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "server error")
		return
	}
	order, err := s.enrichCustomerServiceOrder(r.Context(), appUserID, appOrderResp{
		OutTradeNo:   outTradeNo,
		ProductID:    productID,
		Title:        title,
		Amount:       amount,
		DurationDays: durationDays,
		Status:       appOrderPendingConfirmation,
	})
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "server error")
		return
	}
	httpx.OK(w, order)
}

func resolveAppOrderPurchaseMode(storedMode, provider string) string {
	if mode, err := normalizeAppPaymentMode(storedMode); err == nil && strings.TrimSpace(storedMode) != "" {
		return mode
	}
	if strings.EqualFold(strings.TrimSpace(provider), appPaymentProviderXZN) {
		return appPurchaseModeXZN
	}
	return appPurchaseModeCustomerService
}

func (s *Server) appBillingOrderStatus(w http.ResponseWriter, r *http.Request) {
	userInfo, ok := appUserFromContext(r)
	if !ok {
		httpx.Fail(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	outTradeNo := strings.TrimSpace(r.URL.Query().Get("outTradeNo"))
	if outTradeNo == "" {
		httpx.Fail(w, http.StatusBadRequest, "outTradeNo required")
		return
	}
	var resp appOrderResp
	err := s.db.QueryRowContext(r.Context(),
		`SELECT out_trade_no, product_id, title, amount, status
		 FROM app_orders WHERE app_user_id = $1 AND out_trade_no = $2`,
		userInfo.ID, outTradeNo).Scan(&resp.OutTradeNo, &resp.ProductID, &resp.Title, &resp.Amount, &resp.Status)
	if err != nil {
		httpx.Fail(w, http.StatusNotFound, "order not found")
		return
	}
	s.loadAppOrderPaymentMeta(r.Context(), userInfo.ID, outTradeNo, &resp)
	if resp.PaymentProvider == appPaymentProviderXZN && (resp.Status == "pending" || resp.Status == "paying") {
		if _, reconcileErr := s.reconcileXZNAppOrder(r.Context(), outTradeNo); reconcileErr != nil {
			log.Printf("[XZNPAY] status reconciliation out_trade_no=%s: %v", outTradeNo, reconcileErr)
		}
		if refreshed, refreshErr := s.loadAppOrderByOutTradeNo(r.Context(), userInfo.ID, outTradeNo); refreshErr == nil {
			resp = refreshed
		}
	}
	enriched, err := s.enrichCustomerServiceOrder(r.Context(), userInfo.ID, resp)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "server error")
		return
	}
	httpx.OK(w, enriched)
}

func (s *Server) appBillingCancelOrder(w http.ResponseWriter, r *http.Request) {
	userInfo, ok := appUserFromContext(r)
	if !ok {
		httpx.Fail(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var body struct {
		OutTradeNo string `json:"outTradeNo"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4<<10)).Decode(&body); err != nil || strings.TrimSpace(body.OutTradeNo) == "" {
		httpx.Fail(w, http.StatusBadRequest, "outTradeNo required")
		return
	}
	body.OutTradeNo = strings.TrimSpace(body.OutTradeNo)

	tx, err := s.db.BeginTx(r.Context(), &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "server error")
		return
	}
	defer tx.Rollback()
	var orderID int64
	var status, provider string
	if err := tx.QueryRowContext(r.Context(), `SELECT id,status,COALESCE(payment_provider,'manual')
		FROM app_orders WHERE app_user_id=$1 AND out_trade_no=$2 FOR UPDATE`, userInfo.ID, body.OutTradeNo).Scan(&orderID, &status, &provider); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpx.Fail(w, http.StatusNotFound, "order not found")
			return
		}
		httpx.Fail(w, http.StatusInternalServerError, "server error")
		return
	}
	if provider != appPaymentProviderXZN {
		httpx.Fail(w, http.StatusConflict, "只有在线待支付订单可以取消")
		return
	}
	if status != "closed" {
		if status != "pending" && status != "paying" {
			httpx.Fail(w, http.StatusConflict, "当前订单状态不能取消")
			return
		}
		if _, err := tx.ExecContext(r.Context(), `UPDATE app_orders SET status='closed',update_time=now() WHERE id=$1`, orderID); err != nil {
			httpx.Fail(w, http.StatusInternalServerError, "server error")
			return
		}
	}
	if err := tx.Commit(); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "server error")
		return
	}
	order, err := s.loadAppOrderByOutTradeNo(r.Context(), userInfo.ID, body.OutTradeNo)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "server error")
		return
	}
	enriched, err := s.enrichOnlineOrder(r.Context(), userInfo.ID, order)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "server error")
		return
	}
	httpx.OK(w, enriched)
}

func (s *Server) findPendingAppOrder(ctx context.Context, appUserID int64) (appOrderResp, bool, error) {
	var resp appOrderResp
	err := s.db.QueryRowContext(ctx, `
		SELECT p.out_trade_no, p.product_id, p.title, p.amount, p.status
		FROM app_orders p
		WHERE p.app_user_id=$1 AND (p.status='pending_confirmation' OR (p.payment_provider='xzn' AND p.status IN ('pending','paying')))
		  AND NOT EXISTS (
			SELECT 1 FROM app_orders resolved
			WHERE resolved.app_user_id=p.app_user_id AND resolved.status='paid'
			  AND (resolved.create_time>p.create_time OR (resolved.create_time=p.create_time AND resolved.id>p.id))
		  )
		ORDER BY p.create_time DESC, p.id DESC LIMIT 1`, appUserID).Scan(
		&resp.OutTradeNo, &resp.ProductID, &resp.Title, &resp.Amount, &resp.Status,
	)
	if err == sql.ErrNoRows {
		return appOrderResp{}, false, nil
	}
	if err != nil {
		return appOrderResp{}, false, err
	}
	s.loadAppOrderPaymentMeta(ctx, appUserID, resp.OutTradeNo, &resp)
	return resp, true, nil
}

func (s *Server) enrichCustomerServiceOrder(ctx context.Context, appUserID int64, resp appOrderResp) (appOrderResp, error) {
	s.loadAppOrderPaymentMeta(ctx, appUserID, resp.OutTradeNo, &resp)
	if resp.PurchaseMode == appPurchaseModeXZN || resp.PaymentProvider == appPaymentProviderXZN {
		return s.enrichOnlineOrder(ctx, appUserID, resp)
	}
	resp = appCustomerServiceOrder(resp)
	if resp.DurationDays <= 0 {
		resp.DurationDays, _ = membershipDurationDays(resp.ProductID)
	}
	var currentExpiresAt sql.NullTime
	if err := s.db.QueryRowContext(ctx, `SELECT member_expires_at FROM app_users WHERE id=$1`, appUserID).Scan(&currentExpiresAt); err != nil {
		return appOrderResp{}, err
	}
	if currentExpiresAt.Valid {
		resp.CurrentExpiresAt = currentExpiresAt.Time.Format(time.RFC3339)
	}
	var currentExpiry *time.Time
	if currentExpiresAt.Valid {
		currentExpiry = &currentExpiresAt.Time
	}
	period, err := calculateMembershipPeriodDays(resp.DurationDays, time.Now(), currentExpiry)
	if err != nil {
		return appOrderResp{}, err
	}
	resp.EstimatedExpiresAt = period.Expires.Format(time.RFC3339)
	return resp, nil
}

func (s *Server) enrichOnlineOrder(ctx context.Context, appUserID int64, resp appOrderResp) (appOrderResp, error) {
	cfg, _ := s.loadXZNConfig(ctx)
	resp.PayChannel = displayAppPayChannel(resp.PayChannel)
	if resp.DurationDays <= 0 {
		resp.DurationDays, _ = membershipDurationDays(resp.ProductID)
	}
	resp.PayStatus = resp.ProviderStatus
	if resp.PayStatus == "" {
		resp.PayStatus = resp.Status
	}
	resp.PurchaseMode = appPurchaseModeXZN
	resp.CustomerServiceQRURL = ""
	resp.PayEnabled = resp.PayURL != "" && (resp.Status == "pending" || resp.Status == "paying")
	resp.ConfigurationStatus = "configured"
	if !xznCredentialsConfigured(cfg) || !cfg.Enabled {
		resp.PayEnabled = false
		resp.ConfigurationStatus = "payment_not_configured"
		resp.DisabledReason = "在线支付暂不可用"
	}
	if resp.PayURL != "" {
		resp.Payment = map[string]any{
			"type":       "web",
			"mode":       "h5",
			"channel":    resp.PayChannel,
			"payChannel": resp.PayChannel,
			"url":        resp.PayURL,
			"payUrl":     resp.PayURL,
			"returnUrl":  xznOrderReturnURL(cfg.ReturnURL, resp.OutTradeNo),
		}
	}
	switch {
	case resp.Status == "refunded":
		resp.Message = "订单已退款，会员权益已同步回退"
	case resp.Status == "closed":
		resp.Message = "订单已取消，可以重新选择支付方式"
	case strings.EqualFold(resp.ProviderStatus, "TRADE_SUCCESS"):
		resp.Message = "支付成功，会员已开通"
	case strings.EqualFold(resp.ProviderStatus, "TRADE_CLOSED"):
		resp.Message = "订单已关闭，未开通会员"
	case strings.EqualFold(resp.ProviderStatus, "TRADE_REFUND"):
		resp.Message = "订单已退款，会员权益已同步回退"
	case strings.EqualFold(resp.ProviderStatus, "TRADE_FREEZE"):
		resp.Message = "订单正在风控审核，请稍后查询"
	case strings.EqualFold(resp.ProviderStatus, "TRADE_UNFREEZE"):
		resp.Message = "订单已解除风控，请继续支付"
	case strings.EqualFold(resp.ProviderStatus, "WAIT_BUYER_PAY") || resp.ProviderStatus == "":
		resp.Message = "请在支付页面完成付款"
	default:
		resp.Message = "支付未完成，请稍后重试"
	}
	return s.enrichOrderDates(ctx, appUserID, resp)
}

func (s *Server) enrichOrderDates(ctx context.Context, appUserID int64, resp appOrderResp) (appOrderResp, error) {
	var currentExpiresAt sql.NullTime
	if err := s.db.QueryRowContext(ctx, `SELECT member_expires_at FROM app_users WHERE id=$1`, appUserID).Scan(&currentExpiresAt); err != nil {
		return appOrderResp{}, err
	}
	if currentExpiresAt.Valid {
		resp.CurrentExpiresAt = currentExpiresAt.Time.Format(time.RFC3339)
	}
	var currentExpiry *time.Time
	if currentExpiresAt.Valid {
		currentExpiry = &currentExpiresAt.Time
	}
	period, err := calculateMembershipPeriodDays(resp.DurationDays, time.Now(), currentExpiry)
	if err != nil {
		return appOrderResp{}, err
	}
	resp.EstimatedExpiresAt = period.Expires.Format(time.RFC3339)
	return resp, nil
}

func (s *Server) loadAppOrderPaymentMeta(ctx context.Context, appUserID int64, outTradeNo string, resp *appOrderResp) {
	if s == nil || s.db == nil || strings.TrimSpace(outTradeNo) == "" || resp == nil {
		return
	}
	var purchaseMode, provider, channel, gateway, tradeNo, providerStatus, payURL, paymentError string
	var lastQueryAt sql.NullTime
	err := s.db.QueryRowContext(ctx, `
		SELECT COALESCE(purchase_mode,''), COALESCE(payment_provider,''), COALESCE(pay_channel,''), COALESCE(gateway_id,''),
		       COALESCE(provider_trade_no,''), COALESCE(provider_status,''), COALESCE(pay_url,''),
		       COALESCE(payment_error,''), last_query_at
		FROM app_orders WHERE app_user_id=$1 AND out_trade_no=$2`, appUserID, outTradeNo).Scan(
		&purchaseMode, &provider, &channel, &gateway, &tradeNo, &providerStatus, &payURL, &paymentError, &lastQueryAt)
	if err != nil {
		return
	}
	resp.PurchaseMode = resolveAppOrderPurchaseMode(purchaseMode, provider)
	resp.PaymentProvider, resp.PayChannel, resp.GatewayID = provider, displayAppPayChannel(channel), gateway
	resp.ProviderTradeNo, resp.ProviderStatus, resp.PayURL, resp.PaymentError = tradeNo, providerStatus, payURL, paymentError
	if lastQueryAt.Valid {
		resp.LastQueryAt = lastQueryAt.Time.Format(time.RFC3339)
	}
	var durationDays int
	if err := s.db.QueryRowContext(ctx, `SELECT duration_days FROM app_orders WHERE app_user_id=$1 AND out_trade_no=$2`, appUserID, outTradeNo).Scan(&durationDays); err == nil && durationDays > 0 {
		resp.DurationDays = durationDays
	}
}

func (s *Server) loadAppOrderByOutTradeNo(ctx context.Context, appUserID int64, outTradeNo string) (appOrderResp, error) {
	var resp appOrderResp
	err := s.db.QueryRowContext(ctx, `SELECT out_trade_no, product_id, title, amount, status FROM app_orders WHERE app_user_id=$1 AND out_trade_no=$2`, appUserID, outTradeNo).
		Scan(&resp.OutTradeNo, &resp.ProductID, &resp.Title, &resp.Amount, &resp.Status)
	if err != nil {
		return appOrderResp{}, err
	}
	s.loadAppOrderPaymentMeta(ctx, appUserID, outTradeNo, &resp)
	return resp, nil
}

func appCustomerServiceOrder(resp appOrderResp) appOrderResp {
	resp.PayStatus = resp.Status
	resp.PayEnabled = false
	resp.PurchaseMode = appPurchaseModeCustomerService
	resp.CustomerServiceQRURL = appCustomerServiceQRURL
	if resp.Status == "paid" {
		resp.Message = "会员已由客服确认开通"
	} else if resp.Status == "refunded" {
		resp.Message = "订单已退款，会员权益已同步回退"
	} else {
		resp.Message = "请添加客服微信并提供手机号和订单号，转账后由客服确认开通"
	}
	return resp
}
