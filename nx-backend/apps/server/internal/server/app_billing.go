package server

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"nine-xing/nx-backend/apps/server/internal/httpx"
)

type appEntitlementResp struct {
	PlanName            string `json:"planName"`
	PlanCode            string `json:"planCode"`
	IsMember            bool   `json:"isMember"`
	ChatRemaining       int    `json:"chatRemaining"`
	DeepReportRemaining int    `json:"deepReportRemaining"`
	CardLimit           int    `json:"cardLimit"`
	CardUsed            int    `json:"cardUsed"`
	StartedAt           string `json:"startedAt,omitempty"`
	ExpiresAt           string `json:"expiresAt,omitempty"`
}

type appProductResp struct {
	ID                  string   `json:"id"`
	Title               string   `json:"title"`
	Subtitle            string   `json:"subtitle"`
	PriceText           string   `json:"priceText"`
	Badge               string   `json:"badge,omitempty"`
	Features            []string `json:"features"`
	Enabled             bool     `json:"enabled"`
	PayEnabled          bool     `json:"payEnabled"`
	ConfigurationStatus string   `json:"configurationStatus"`
	DisabledReason      string   `json:"disabledReason,omitempty"`
	PurchaseMode        string   `json:"purchaseMode"`
}

const (
	appPurchaseModeCustomerService = "customer_service"
	appOrderPendingConfirmation    = "pending_confirmation"
	appCustomerServiceQRURL        = "/api/public/customer-service-qr"
)

func appCardLimit(memberLevel string) int {
	if memberLevel == "" || memberLevel == "free" {
		return 1
	}
	return 5
}

func appPlanCode(memberLevel string) string {
	switch memberLevel {
	case "", "free":
		return "free"
	case "vip":
		return "vip_month"
	case "svip":
		return "vip_year"
	default:
		return memberLevel
	}
}

func appPlanName(planCode string) string {
	switch planCode {
	case "free":
		return "免费版"
	case "vip_month":
		return "月卡会员"
	case "vip_quarter":
		return "季卡会员"
	case "vip_year":
		return "年卡会员"
	default:
		return "会员版"
	}
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
	planCode := appPlanCode(memberLevel)
	legacyWithoutExpiry := (memberLevel == "vip" || memberLevel == "svip") && !memberExpiresAt.Valid
	isMember := planCode != "free" && (legacyWithoutExpiry || (memberExpiresAt.Valid && memberExpiresAt.Time.After(time.Now())))
	if !isMember {
		planCode = "free"
	}
	startedAt := ""
	expiresAt := ""
	if isMember && memberStartedAt.Valid {
		startedAt = memberStartedAt.Time.Format(time.RFC3339)
	}
	if isMember && memberExpiresAt.Valid {
		expiresAt = memberExpiresAt.Time.Format(time.RFC3339)
	}
	httpx.OK(w, appEntitlementResp{
		PlanName:            appPlanName(planCode),
		PlanCode:            planCode,
		IsMember:            isMember,
		ChatRemaining:       0,
		DeepReportRemaining: 0,
		CardLimit:           appCardLimit(planCode),
		CardUsed:            cardUsed,
		StartedAt:           startedAt,
		ExpiresAt:           expiresAt,
	})
}

func (s *Server) appBillingProducts(w http.ResponseWriter, r *http.Request) {
	httpx.OK(w, []appProductResp{
		appCustomerServiceProduct(appProductResp{
			ID:        "vip_month",
			Title:     "月卡会员",
			Subtitle:  "适合轻度陪伴与日常问答",
			PriceText: "¥29",
			Badge:     "推荐",
			Features:  []string{"更多问答额度", "最多 5 张人物卡", "成长练习完整记录"},
			Enabled:   true,
		}),
		appCustomerServiceProduct(appProductResp{
			ID:        "vip_quarter",
			Title:     "季卡会员",
			Subtitle:  "适合持续成长陪伴",
			PriceText: "¥79",
			Features:  []string{"月卡全部权益", "更长会员有效期", "后续周报优先体验"},
			Enabled:   true,
		}),
		appCustomerServiceProduct(appProductResp{
			ID:        "vip_year",
			Title:     "年卡会员",
			Subtitle:  "适合长期自我探索",
			PriceText: "¥199",
			Badge:     "省心",
			Features:  []string{"全年会员权益", "长期成长画像", "会员专属海报模板"},
			Enabled:   true,
		}),
	})
}

func appCustomerServiceProduct(product appProductResp) appProductResp {
	product.PayEnabled = false
	product.PurchaseMode = appPurchaseModeCustomerService
	return product
}

type appOrderCreateReq struct {
	ProductID string `json:"productId"`
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
	PayParams            map[string]any `json:"payParams,omitempty"`
	Message              string         `json:"message"`
	PurchaseMode         string         `json:"purchaseMode"`
	CustomerServiceQRURL string         `json:"customerServiceQrUrl"`
}

func appProductTitle(productID string) string {
	switch productID {
	case "vip_month":
		return "月卡会员"
	case "vip_quarter":
		return "季卡会员"
	case "vip_year":
		return "年卡会员"
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
	default:
		return 0
	}
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
	title := appProductTitle(productID)
	amount := appProductAmount(productID)
	if title == "" || amount <= 0 {
		httpx.Fail(w, http.StatusBadRequest, "invalid product")
		return
	}
	outTradeNo := fmt.Sprintf("app%d-%s-%d", userInfo.ID, productID, time.Now().UnixNano())
	if _, err := s.db.ExecContext(r.Context(),
		`INSERT INTO app_orders (out_trade_no, app_user_id, product_id, title, amount, status)
		 VALUES ($1, $2, $3, $4, $5, 'pending_confirmation')`,
		outTradeNo, userInfo.ID, productID, title, amount); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "server error")
		return
	}
	httpx.OK(w, appCustomerServiceOrder(appOrderResp{
		OutTradeNo: outTradeNo,
		ProductID:  productID,
		Title:      title,
		Amount:     amount,
		Status:     appOrderPendingConfirmation,
	}))
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
	httpx.OK(w, appCustomerServiceOrder(resp))
}

func appCustomerServiceOrder(resp appOrderResp) appOrderResp {
	resp.PayStatus = resp.Status
	resp.PayEnabled = false
	resp.PurchaseMode = appPurchaseModeCustomerService
	resp.CustomerServiceQRURL = appCustomerServiceQRURL
	if resp.Status == "paid" {
		resp.Message = "会员已由客服确认开通"
	} else {
		resp.Message = "请添加客服微信并提供手机号和订单号，转账后由客服确认开通"
	}
	return resp
}
