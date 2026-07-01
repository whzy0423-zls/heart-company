package server

import (
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
}

type appProductResp struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Subtitle  string   `json:"subtitle"`
	PriceText string   `json:"priceText"`
	Badge     string   `json:"badge,omitempty"`
	Features  []string `json:"features"`
	Enabled   bool     `json:"enabled"`
}

func appCardLimit(memberLevel string) int {
	if memberLevel == "" || memberLevel == "free" {
		return 1
	}
	return 5
}

func appPlanName(memberLevel string) string {
	if memberLevel == "" || memberLevel == "free" {
		return "免费版"
	}
	return "会员版"
}

func (s *Server) appBillingEntitlements(w http.ResponseWriter, r *http.Request) {
	userInfo, ok := appUserFromContext(r)
	if !ok {
		httpx.Fail(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	user, err := s.appUsers.FindByID(r.Context(), userInfo.ID)
	if err != nil {
		httpx.Fail(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var cardUsed int
	_ = s.db.QueryRowContext(r.Context(),
		`SELECT count(*) FROM app_user_cards
		 WHERE app_user_id = $1 AND card_type='secondary' AND status='active'`,
		userInfo.ID).Scan(&cardUsed)
	httpx.OK(w, appEntitlementResp{
		PlanName:            appPlanName(user.MemberLevel),
		PlanCode:            user.MemberLevel,
		IsMember:            user.MemberLevel != "" && user.MemberLevel != "free",
		ChatRemaining:       0,
		DeepReportRemaining: 0,
		CardLimit:           appCardLimit(user.MemberLevel),
		CardUsed:            cardUsed,
	})
}

func (s *Server) appBillingProducts(w http.ResponseWriter, r *http.Request) {
	httpx.OK(w, []appProductResp{
		{
			ID:        "vip_month",
			Title:     "月卡会员",
			Subtitle:  "适合轻度陪伴与日常问答",
			PriceText: "待开放",
			Badge:     "推荐",
			Features:  []string{"更多问答额度", "最多 5 张人物卡", "成长练习完整记录"},
			Enabled:   false,
		},
		{
			ID:        "vip_quarter",
			Title:     "季卡会员",
			Subtitle:  "适合持续成长陪伴",
			PriceText: "待开放",
			Features:  []string{"月卡全部权益", "更长会员有效期", "后续周报优先体验"},
			Enabled:   false,
		},
		{
			ID:        "vip_year",
			Title:     "年卡会员",
			Subtitle:  "适合长期自我探索",
			PriceText: "待开放",
			Badge:     "省心",
			Features:  []string{"全年会员权益", "深度报告权益", "新功能优先体验"},
			Enabled:   false,
		},
		{
			ID:        "deep_report",
			Title:     "深度报告",
			Subtitle:  "解锁更完整的人格分析",
			PriceText: "待开放",
			Features:  []string{"压力点分析", "关系模式分析", "成长建议整理"},
			Enabled:   false,
		},
	})
}

type appOrderCreateReq struct {
	ProductID string `json:"productId"`
}

type appOrderResp struct {
	OutTradeNo string         `json:"outTradeNo"`
	ProductID  string         `json:"productId"`
	Title      string         `json:"title"`
	Amount     int            `json:"amount"`
	Status     string         `json:"status"`
	PayStatus  string         `json:"payStatus"`
	PayParams  map[string]any `json:"payParams,omitempty"`
	Message    string         `json:"message"`
}

func appProductTitle(productID string) string {
	switch productID {
	case "vip_month":
		return "月卡会员"
	case "vip_quarter":
		return "季卡会员"
	case "vip_year":
		return "年卡会员"
	case "deep_report":
		return "深度报告"
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
	case "deep_report":
		return 990
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
		 VALUES ($1, $2, $3, $4, $5, 'pending')`,
		outTradeNo, userInfo.ID, productID, title, amount); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "server error")
		return
	}
	httpx.OK(w, appOrderResp{
		OutTradeNo: outTradeNo,
		ProductID:  productID,
		Title:      title,
		Amount:     amount,
		Status:     "pending",
		PayStatus:  "not_configured",
		Message:    "App 支付 SDK 与商户参数尚未配置，订单已创建但不会标记为支付成功",
	})
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
	resp.PayStatus = resp.Status
	resp.Message = "支付结果以后端订单状态为准"
	httpx.OK(w, resp)
}
