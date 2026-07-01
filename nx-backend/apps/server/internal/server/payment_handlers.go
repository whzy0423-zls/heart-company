package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"nine-xing/nx-backend/apps/server/internal/config"
	"nine-xing/nx-backend/apps/server/internal/httpx"
	"nine-xing/nx-backend/apps/server/internal/rag"
	"nine-xing/nx-backend/apps/server/internal/wxpay"
)

// mustWxPayClient 用 env 构造支付客户端；生产配置不全时启动失败，避免误入模拟支付。
func mustWxPayClient(env config.Env) *wxpay.Client {
	wxpayDev := env.WxPay.Dev || (env.AppEnv != "production" && !wxPayConfigComplete(env.WxPay))
	client, err := wxpay.NewClient(wxpay.Config{
		MchID:          env.WxPay.MchID,
		AppID:          env.WxPay.AppID,
		APIv3Key:       env.WxPay.APIv3Key,
		SerialNo:       env.WxPay.SerialNo,
		PrivateKeyPath: env.WxPay.PrivateKeyPath,
		NotifyURL:      env.WxPay.NotifyURL,
		Dev:            wxpayDev,
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
		strings.TrimSpace(cfg.NotifyURL) != ""
}

// reportOrderRequest 下单请求：解锁某条测试记录的深度报告。
type reportOrderRequest struct {
	TestRecordID string `json:"testRecordId"`
}

// createReportOrder 为深度报告下单，返回小程序拉起支付所需参数。
func (s *Server) createReportOrder(w http.ResponseWriter, r *http.Request) {
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
	outTradeNo := fmt.Sprintf("rpt%d-%d-%d", uid, recordID, time.Now().Unix())
	if _, err := s.miniapp.CreateOrder(ctx, uid, outTradeNo, "report", recordID, "九型深度报告", price); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}

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

// payNotify 接收微信支付回调（或 dev 模拟回调），落账并发放权益。
// dev 模式下可直接 POST {"out_trade_no":"..."} 模拟支付成功。
func (s *Server) payNotify(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 64*1024))
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, "read body failed")
		return
	}
	result, err := s.pay.ParseCallback(raw)
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
	docs, err := s.retrieveDocsForQuery(ctx, question, 8)
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
