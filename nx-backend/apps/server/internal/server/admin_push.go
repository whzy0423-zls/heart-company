package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"nine-xing/nx-backend/apps/server/internal/auditlog"
	"nine-xing/nx-backend/apps/server/internal/httpx"
	"nine-xing/nx-backend/apps/server/internal/push"
)

// --- Admin 端：推送管理 ---

func (s *Server) adminPushList(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))

	items, total, err := s.pushStore.ListPushHistory(r.Context(), page, pageSize)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "查询失败")
		return
	}
	httpx.OK(w, map[string]interface{}{
		"items": items,
		"total": total,
	})
}

func (s *Server) adminPushSend(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
	var body struct {
		Title       string `json:"title"`
		Content     string `json:"content"`
		TargetType  string `json:"targetType"`
		TargetValue string `json:"targetValue"`
		DeepLink    string `json:"deepLink"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	if body.Title == "" || body.Content == "" {
		httpx.Fail(w, http.StatusBadRequest, "标题和内容不能为空")
		return
	}
	body.Title = strings.TrimSpace(body.Title)
	body.Content = strings.TrimSpace(body.Content)
	body.TargetType = strings.TrimSpace(body.TargetType)
	body.TargetValue = strings.TrimSpace(body.TargetValue)
	body.DeepLink = strings.TrimSpace(body.DeepLink)
	if body.Title == "" || body.Content == "" {
		httpx.Fail(w, http.StatusBadRequest, "标题和内容不能为空")
		return
	}
	targetType, targetValue, msg := normalizePushTarget(body.TargetType, body.TargetValue)
	if msg != "" {
		httpx.Fail(w, http.StatusBadRequest, msg)
		return
	}
	body.TargetType = targetType
	body.TargetValue = targetValue

	user := userFromRequest(r)
	operator := strings.TrimSpace(user.RealName)
	if operator == "" {
		operator = strings.TrimSpace(user.Username)
	}
	if operator == "" {
		operator = "admin"
	}

	recordID, err := s.pushStore.CreatePushRecord(r.Context(), body.Title, body.Content, body.TargetType, body.TargetValue, body.DeepLink, operator)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "创建推送记录失败")
		return
	}

	sent := 0
	msgIDs := []string{}
	pushErr := s.pushStore.ForEachRegistrationIDBatch(r.Context(), body.TargetType, body.TargetValue, 1000, func(regIDs []string) error {
		result, err := s.pushStore.Pusher().Push(r.Context(), regIDs, push.Message{
			Title:    body.Title,
			Content:  body.Content,
			DeepLink: body.DeepLink,
		})
		if err != nil {
			return err
		}
		sent += result.Sent
		if result.MsgID != "" {
			msgIDs = append(msgIDs, result.MsgID)
		}
		return nil
	})

	if pushErr != nil {
		_ = s.pushStore.UpdatePushStatus(r.Context(), recordID, "failed", sent, pushErr.Error())
		s.recordAdminAudit(r, auditlog.Entry{
			Action:     "push.send",
			TargetType: "push_notification",
			TargetID:   strconv.FormatInt(recordID, 10),
			After:      pushAuditSnapshot(body.Title, body.Content, body.TargetType, body.TargetValue, body.DeepLink, sent, "failed", pushErr.Error()),
			Summary:    "发送 App 推送失败",
		})
		httpx.Fail(w, http.StatusInternalServerError, "推送发送失败: "+pushErr.Error())
		return
	}

	if sent == 0 {
		_ = s.pushStore.UpdatePushStatus(r.Context(), recordID, "success", 0, "无推送目标")
		s.recordAdminAudit(r, auditlog.Entry{
			Action:     "push.send",
			TargetType: "push_notification",
			TargetID:   strconv.FormatInt(recordID, 10),
			After:      pushAuditSnapshot(body.Title, body.Content, body.TargetType, body.TargetValue, body.DeepLink, 0, "success", "无推送目标"),
			Summary:    "发送 App 推送：无推送目标",
		})
		httpx.OK(w, map[string]interface{}{"sent": 0, "message": "无推送目标"})
		return
	}

	_ = s.pushStore.UpdatePushStatus(r.Context(), recordID, "success", sent, "")
	s.recordAdminAudit(r, auditlog.Entry{
		Action:     "push.send",
		TargetType: "push_notification",
		TargetID:   strconv.FormatInt(recordID, 10),
		After:      pushAuditSnapshot(body.Title, body.Content, body.TargetType, body.TargetValue, body.DeepLink, sent, "success", ""),
		Summary:    "发送 App 推送成功",
	})
	httpx.OK(w, map[string]interface{}{
		"sent":  sent,
		"msgId": strings.Join(msgIDs, ","),
	})
}

func (s *Server) adminPushAudienceCount(w http.ResponseWriter, r *http.Request) {
	targetType, targetValue, msg := normalizePushTarget(r.URL.Query().Get("targetType"), r.URL.Query().Get("targetValue"))
	if msg != "" {
		httpx.Fail(w, http.StatusBadRequest, msg)
		return
	}
	deviceCount, userCount, err := s.pushStore.CountAudience(r.Context(), targetType, targetValue)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "查询失败")
		return
	}
	httpx.OK(w, map[string]int64{
		"deviceCount": deviceCount,
		"userCount":   userCount,
	})
}

func normalizePushTarget(targetType, targetValue string) (string, string, string) {
	targetType = strings.TrimSpace(targetType)
	targetValue = strings.TrimSpace(targetValue)
	if targetType == "" {
		targetType = "all"
	}
	if targetType != "all" && targetType != "level" {
		return targetType, targetValue, "不支持的推送目标"
	}
	if targetType == "level" && targetValue == "" {
		return targetType, targetValue, "会员等级不能为空"
	}
	if targetType == "level" && !validPushMemberLevel(targetValue) {
		return targetType, targetValue, "不支持的会员等级"
	}
	if targetType == "all" {
		targetValue = ""
	}
	return targetType, targetValue, ""
}

func validPushMemberLevel(value string) bool {
	switch value {
	case "free", "vip", "svip":
		return true
	default:
		return false
	}
}

func pushAuditSnapshot(title, content, targetType, targetValue, deepLink string, sent int, status string, errMsg string) map[string]any {
	return map[string]any{
		"title":       title,
		"content":     content,
		"targetType":  targetType,
		"targetValue": targetValue,
		"deepLink":    deepLink,
		"sent":        sent,
		"status":      status,
		"error":       errMsg,
	}
}
