package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"nine-xing/nx-backend/apps/server/internal/auditlog"
	"nine-xing/nx-backend/apps/server/internal/httpx"
	"nine-xing/nx-backend/apps/server/internal/push"
)

// --- Admin 端：推送管理 ---

const defaultAdminPushSendTimeout = 10 * time.Minute

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
	if s == nil || s.pushStore == nil {
		httpx.Fail(w, http.StatusInternalServerError, "推送服务不可用")
		return
	}

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

	task := adminPushSendTask{
		recordID:    recordID,
		title:       body.Title,
		content:     body.Content,
		targetType:  body.TargetType,
		targetValue: body.TargetValue,
		deepLink:    body.DeepLink,
		audit: adminPushAuditMeta{
			operatorID:   user.ID,
			operatorName: operator,
			ip:           s.clientIP(r),
			userAgent:    r.UserAgent(),
		},
	}
	s.recordAdminPushSendAudit(r.Context(), task, 0, "pending", "", "创建 App 推送任务")
	go s.runAdminPushSendTask(task)

	httpx.OK(w, map[string]interface{}{
		"recordId": recordID,
		"status":   "pending",
		"message":  "推送任务已创建，后台发送中",
	})
}

type adminPushAuditMeta struct {
	operatorID   int64
	operatorName string
	ip           string
	userAgent    string
}

type adminPushSendTask struct {
	recordID    int64
	title       string
	content     string
	targetType  string
	targetValue string
	deepLink    string
	audit       adminPushAuditMeta
}

func (s *Server) runAdminPushSendTask(task adminPushSendTask) {
	if s == nil || s.pushStore == nil {
		return
	}
	if s.pushSendSlots != nil {
		s.pushSendSlots <- struct{}{}
		defer func() { <-s.pushSendSlots }()
	}

	ctx, cancel := context.WithTimeout(context.Background(), s.adminPushSendTimeout())
	defer cancel()

	sent := 0
	defer func() {
		if recovered := recover(); recovered != nil {
			errMsg := fmt.Sprintf("push worker panic: %v", recovered)
			s.finishAdminPushSendTask(task, sent, "failed", errMsg, "发送 App 推送失败")
		}
	}()

	claimed, err := s.pushStore.ClaimPendingPushTask(ctx, task.recordID)
	if err != nil {
		log.Printf("push task claim failed id=%d: %v", task.recordID, err)
		return
	}
	if !claimed {
		log.Printf("push task skipped because it is no longer pending id=%d", task.recordID)
		return
	}

	pushErr := s.pushStore.ForEachRegistrationIDBatch(ctx, task.targetType, task.targetValue, 1000, func(regIDs []string) error {
		pusher := s.pushStore.Pusher()
		if pusher == nil {
			return errors.New("push sender is not configured")
		}
		result, err := pusher.Push(ctx, regIDs, push.Message{
			Title:    task.title,
			Content:  task.content,
			DeepLink: task.deepLink,
		})
		if err != nil {
			return err
		}
		sent += result.Sent
		return nil
	})

	status := "success"
	errMsg := ""
	summary := "发送 App 推送成功"
	if pushErr != nil {
		status = "failed"
		errMsg = pushErr.Error()
		summary = "发送 App 推送失败"
	} else if sent == 0 {
		status = "failed"
		errMsg = "无推送目标"
		summary = "发送 App 推送：无推送目标"
	}

	s.finishAdminPushSendTask(task, sent, status, errMsg, summary)
}

func (s *Server) adminPushSendTimeout() time.Duration {
	if s != nil && s.pushSendTimeout > 0 {
		return s.pushSendTimeout
	}
	return defaultAdminPushSendTimeout
}

func (s *Server) adminPushRecoveryInterval() time.Duration {
	if s != nil && s.pushRecoveryInterval > 0 {
		return s.pushRecoveryInterval
	}
	timeout := s.adminPushSendTimeout()
	interval := timeout / 2
	if interval <= 0 {
		return time.Minute
	}
	if interval > time.Minute {
		return time.Minute
	}
	if interval < time.Second {
		return time.Second
	}
	return interval
}

func (s *Server) finishAdminPushSendTask(task adminPushSendTask, sent int, status, errMsg, summary string) {
	if s == nil || s.pushStore == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := s.pushStore.UpdatePushStatus(ctx, task.recordID, status, sent, errMsg); err != nil {
		log.Printf("push task update final status failed id=%d status=%s: %v", task.recordID, status, err)
	}
	s.recordAdminPushSendAudit(ctx, task, sent, status, errMsg, summary)
}

func (s *Server) recordAdminPushSendAudit(ctx context.Context, task adminPushSendTask, sent int, status, errMsg, summary string) {
	if s == nil || s.auditLogs == nil {
		return
	}
	operatorName := strings.TrimSpace(task.audit.operatorName)
	if operatorName == "" {
		operatorName = "unknown"
	}
	entry := auditlog.Entry{
		OperatorID:   task.audit.operatorID,
		OperatorName: operatorName,
		Action:       "push.send",
		TargetType:   "push_notification",
		TargetID:     strconv.FormatInt(task.recordID, 10),
		IP:           task.audit.ip,
		UserAgent:    task.audit.userAgent,
		After:        pushAuditSnapshot(task.title, task.content, task.targetType, task.targetValue, task.deepLink, sent, status, errMsg),
		Summary:      summary,
	}
	if err := s.auditLogs.Record(ctx, entry); err != nil {
		log.Printf("admin audit log failed action=%s target=%s/%s: %v", entry.Action, entry.TargetType, entry.TargetID, err)
	}
}

func (s *Server) recoverPushAsyncTasks() {
	if s == nil || s.pushStore == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cutoff := time.Now().Add(-s.adminPushSendTimeout())
	if err := s.pushStore.MarkInterruptedPushTasksBefore(ctx, "服务重启，推送发送状态中断，请重新发送", cutoff); err != nil {
		log.Printf("mark interrupted push async tasks failed: %v", err)
	}
	for {
		tasks, err := s.pushStore.ListRecoverablePushTasks(ctx, 100)
		if err != nil {
			log.Printf("recover push async tasks failed: %v", err)
			return
		}
		if len(tasks) == 0 {
			return
		}
		for _, item := range tasks {
			task := adminPushSendTask{
				recordID:    item.ID,
				title:       item.Title,
				content:     item.Content,
				targetType:  item.TargetType,
				targetValue: item.TargetValue,
				deepLink:    item.DeepLink,
				audit: adminPushAuditMeta{
					operatorName: strings.TrimSpace(item.Operator),
				},
			}
			s.runAdminPushSendTask(task)
		}
	}
}

func (s *Server) runPushAsyncRecoveryLoop(ctx context.Context) {
	s.recoverPushAsyncTasks()
	ticker := time.NewTicker(s.adminPushRecoveryInterval())
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.recoverPushAsyncTasks()
		}
	}
}

func (s *Server) adminPushAudienceCount(w http.ResponseWriter, r *http.Request) {
	targetType, targetValue, msg := normalizePushTarget(r.URL.Query().Get("targetType"), r.URL.Query().Get("targetValue"))
	if msg != "" {
		httpx.Fail(w, http.StatusBadRequest, msg)
		return
	}
	if s == nil || s.pushStore == nil {
		httpx.Fail(w, http.StatusInternalServerError, "推送服务不可用")
		return
	}
	deviceCount, userCount, err := s.pushStore.CountAudience(r.Context(), targetType, targetValue)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "查询失败")
		return
	}
	httpx.OK(w, map[string]interface{}{
		"deviceCount": deviceCount,
		"targetType":  targetType,
		"targetValue": targetValue,
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
