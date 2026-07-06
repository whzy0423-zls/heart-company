package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"nine-xing/nx-backend/apps/server/internal/config"
	"nine-xing/nx-backend/apps/server/internal/httpx"
	"nine-xing/nx-backend/apps/server/internal/rag"
	"nine-xing/nx-backend/apps/server/internal/wxpay"
)

// mustWxPayClient 用 env 构造支付客户端；生产配置不全时禁用支付功能，避免阻断后台整体启动。
func mustWxPayClient(env config.Env) *wxpay.Client {
	isProduction := config.IsProduction(env.AppEnv)
	wxpayDev := env.WxPay.Dev || (!isProduction && !wxPayConfigComplete(env.WxPay))
	if isProduction && !env.WxPay.Dev && !wxPayConfigComplete(env.WxPay) {
		log.Print("[WXPAY] payment disabled: production wxpay config is incomplete")
		return nil
	}
	client, err := wxpay.NewClient(wxpay.Config{
		MchID:            env.WxPay.MchID,
		AppID:            env.WxPay.AppID,
		APIv3Key:         env.WxPay.APIv3Key,
		SerialNo:         env.WxPay.SerialNo,
		PrivateKeyPath:   env.WxPay.PrivateKeyPath,
		PlatformCertPath: env.WxPay.PlatformCertPath,
		NotifyURL:        env.WxPay.NotifyURL,
		Dev:              wxpayDev,
	})
	if err != nil {
		panic("wxpay init: " + err.Error())
	}
	return client
}

func wxPayConfigComplete(cfg config.WxPayConfig) bool {
	return strings.TrimSpace(cfg.MchID) != "" &&
		strings.TrimSpace(cfg.AppID) != "" &&
		strings.TrimSpace(cfg.APIv3Key) != "" &&
		strings.TrimSpace(cfg.SerialNo) != "" &&
		strings.TrimSpace(cfg.PrivateKeyPath) != "" &&
		strings.TrimSpace(cfg.PlatformCertPath) != "" &&
		strings.TrimSpace(cfg.NotifyURL) != ""
}

type paymentOrderSnapshot struct {
	Amount  int
	Product string
}

func generateReportOutTradeNo(uid, recordID int64) (string, error) {
	var suffix [4]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return "", err
	}
	return fmt.Sprintf("rpt%d-%d-%d-%s", uid, recordID, time.Now().UnixNano(), hex.EncodeToString(suffix[:])), nil
}

func validateWxPayCallbackAgainstOrder(env config.Env, result wxpay.CallbackResult, order paymentOrderSnapshot) error {
	if strings.TrimSpace(result.OutTradeNo) == "" {
		return errors.New("wxpay callback missing out_trade_no")
	}
	if strings.TrimSpace(result.MchID) != strings.TrimSpace(env.WxPay.MchID) {
		return errors.New("wxpay merchant mismatch")
	}
	if strings.TrimSpace(result.AppID) != strings.TrimSpace(env.WxPay.AppID) {
		return errors.New("wxpay appid mismatch")
	}
	if result.AmountTotal <= 0 || result.AmountTotal != order.Amount {
		return fmt.Errorf("wxpay amount mismatch: callback=%d order=%d", result.AmountTotal, order.Amount)
	}
	if order.Product != "report" {
		return fmt.Errorf("unsupported payment product: %s", order.Product)
	}
	return nil
}

// reportOrderRequest 下单请求：解锁某条测试记录的深度报告。
type reportOrderRequest struct {
	TestRecordID string `json:"testRecordId"`
}

// createReportOrder 为深度报告下单，返回小程序拉起支付所需参数。
func (s *Server) createReportOrder(w http.ResponseWriter, r *http.Request) {
	if s.pay == nil {
		httpx.Fail(w, http.StatusServiceUnavailable, "payment service is not configured")
		return
	}
	uid := userFromRequest(r).ID
	var body reportOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}
	recordID, err := strconv.ParseInt(body.TestRecordID, 10, 64)
	if err != nil || recordID <= 0 {
		httpx.Fail(w, http.StatusBadRequest, "testRecordId is required")
		return
	}

	ctx := r.Context()
	// 校验记录归属
	owner, err := s.miniapp.TestRecordOwner(ctx, recordID)
	if err != nil {
		httpx.Fail(w, http.StatusNotFound, "测试记录不存在")
		return
	}
	if owner != uid {
		httpx.Fail(w, http.StatusForbidden, "无权为该记录下单")
		return
	}
	// 已解锁则不再重复下单
	unlocked, err := s.miniapp.IsReportUnlocked(ctx, uid, recordID)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	if unlocked {
		httpx.Fail(w, http.StatusConflict, "该报告已解锁")
		return
	}

	openID, err := s.miniapp.OpenIDByUserID(ctx, uid)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}

	price := s.env.WxPay.ReportPriceCents
	outTradeNo, err := generateReportOutTradeNo(uid, recordID)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "create order failed")
		return
	}
	order, err := s.miniapp.CreateOrReusePendingOrder(ctx, uid, outTradeNo, "report", recordID, "九型深度报告", price)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	outTradeNo = order.OutTradeNo

	prepay, err := s.pay.Prepay(ctx, outTradeNo, openID, "九型芯之力·深度报告", price)
	if err != nil {
		httpx.Fail(w, http.StatusBadGateway, "下单失败："+err.Error())
		return
	}
	httpx.OK(w, map[string]any{
		"outTradeNo": outTradeNo,
		"amount":     price,
		"payParams":  prepay,
	})
}

// devPayReportOrder 仅用于本地/测试环境模拟小程序报告支付成功。
// 真实微信回调仍走 /api/pay/notify，且不会接受未签名明文。
func (s *Server) devPayReportOrder(w http.ResponseWriter, r *http.Request) {
	if config.IsProduction(s.env.AppEnv) || s.pay == nil || !s.pay.DevMode() {
		httpx.Fail(w, http.StatusNotFound, "Not Found")
		return
	}
	user := userFromRequest(r)
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 4*1024))
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, "read body failed")
		return
	}
	result, err := s.pay.ParseDevCallback(raw)
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	if !result.Success {
		httpx.OK(w, map[string]any{"paid": false})
		return
	}
	_, ownerID, _, product, status, err := s.miniapp.OrderByOutTradeNo(r.Context(), result.OutTradeNo)
	if err != nil {
		httpx.Fail(w, http.StatusNotFound, "order not found")
		return
	}
	if product != "report" {
		httpx.Fail(w, http.StatusBadRequest, "unsupported dev payment product")
		return
	}
	if ownerID != user.ID {
		httpx.Fail(w, http.StatusForbidden, "Forbidden")
		return
	}
	if status == "paid" {
		httpx.OK(w, map[string]any{"paid": true})
		return
	}
	transactionID := strings.TrimSpace(result.TransactionID)
	if transactionID == "" {
		transactionID = "dev-" + result.OutTradeNo
	}
	if _, err := s.miniapp.MarkOrderPaid(r.Context(), result.OutTradeNo, transactionID); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.OK(w, map[string]any{"paid": true})
}

// payNotify 接收真实微信支付回调，落账并发放权益。
func (s *Server) payNotify(w http.ResponseWriter, r *http.Request) {
	if s.pay == nil {
		httpx.Fail(w, http.StatusServiceUnavailable, "payment service is not configured")
		return
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 64*1024))
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, "read body failed")
		return
	}
	result, err := s.pay.ParseCallbackWithHeaders(r.Header, raw)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"code": "FAIL", "message": err.Error()})
		return
	}
	if !result.Success {
		// 非成功状态：回执 200 表示已接收，不发放权益。
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"code": "SUCCESS", "message": "OK"})
		return
	}
	order, err := s.miniapp.PaymentOrderSnapshot(r.Context(), result.OutTradeNo)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"code": "FAIL", "message": err.Error()})
		return
	}
	if err := validateWxPayCallbackAgainstOrder(s.env, result, paymentOrderSnapshot{
		Amount:  order.Amount,
		Product: order.Product,
	}); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"code": "FAIL", "message": err.Error()})
		return
	}
	if _, err := s.miniapp.MarkOrderPaid(r.Context(), result.OutTradeNo, result.TransactionID); err != nil {
		// 落账失败要返回非 SUCCESS，微信会重试
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"code": "FAIL", "message": err.Error()})
		return
	}
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"code": "SUCCESS", "message": "OK"})
}

// reportStatus 查询某测试记录的解锁状态。
func (s *Server) reportStatus(w http.ResponseWriter, r *http.Request) {
	uid := userFromRequest(r).ID
	recordID, err := strconv.ParseInt(r.URL.Query().Get("testRecordId"), 10, 64)
	if err != nil || recordID <= 0 {
		httpx.Fail(w, http.StatusBadRequest, "testRecordId is required")
		return
	}
	unlocked, err := s.miniapp.IsReportUnlocked(r.Context(), uid, recordID)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.OK(w, map[string]any{"unlocked": unlocked, "priceCents": s.env.WxPay.ReportPriceCents})
}

// reportContent 返回深度报告正文，仅在已解锁时生成（基于 RAG/LLM）。
func (s *Server) reportContent(w http.ResponseWriter, r *http.Request) {
	uid := userFromRequest(r).ID
	recordID, err := strconv.ParseInt(r.URL.Query().Get("testRecordId"), 10, 64)
	if err != nil || recordID <= 0 {
		httpx.Fail(w, http.StatusBadRequest, "testRecordId is required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), s.chatTimeout)
	defer cancel()

	unlocked, err := s.miniapp.IsReportUnlocked(ctx, uid, recordID)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !unlocked {
		httpx.Fail(w, http.StatusPaymentRequired, "报告未解锁")
		return
	}

	user, err := s.miniapp.GetUser(ctx, uid)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	question := fmt.Sprintf(
		"请基于九型人格知识，为主型 %d 号的用户生成一份结构化的深度性格报告，包含：性格画像、核心动机与恐惧、优势、成长盲点、人际关系建议、职业发展建议。语气专业而温暖。",
		user.MainType,
	)
	docs, err := s.miniappRAGDocuments(ctx)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	service := rag.NewService(docs, rag.WithGenerator(s.generator()))
	answer, err := service.Ask(ctx, rag.AskInput{
		Question: question,
		UserProfile: rag.UserProfile{
			Nickname: user.Nickname,
			MainType: user.MainType,
		},
	})
	if err != nil {
		httpx.Fail(w, http.StatusBadGateway, err.Error())
		return
	}
	httpx.OK(w, answer)
}
