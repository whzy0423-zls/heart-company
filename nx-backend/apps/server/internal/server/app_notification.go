package server

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"nine-xing/nx-backend/apps/server/internal/appnotification"
	"nine-xing/nx-backend/apps/server/internal/httpx"
)

type appNotificationService interface {
	CreateForUser(context.Context, int64, string, string, string, string, string) (int64, error)
	CreateForAudience(context.Context, string, string, string, string, string, string, string) (int64, error)
	List(context.Context, int64, int, int) ([]appnotification.Notification, int, int, error)
	UnreadCount(context.Context, int64) (int, error)
	MarkRead(context.Context, int64, int64) (bool, error)
	MarkAllRead(context.Context, int64) (int64, error)
}

func (s *Server) appNotificationList(w http.ResponseWriter, r *http.Request) {
	user, ok := appUserFromContext(r)
	if !ok {
		httpx.Fail(w, http.StatusUnauthorized, "未登录")
		return
	}
	page := positiveQueryInt(r, "page", 1)
	pageSize := positiveQueryInt(r, "pageSize", 20)
	if pageSize > 100 {
		pageSize = 20
	}
	items, total, unread, err := s.appNotifications.List(r.Context(), user.ID, page, pageSize)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "通知加载失败")
		return
	}
	httpx.OK(w, map[string]any{
		"items":       items,
		"page":        page,
		"pageSize":    pageSize,
		"total":       total,
		"unreadCount": unread,
	})
}

func (s *Server) appNotificationUnreadCount(w http.ResponseWriter, r *http.Request) {
	user, ok := appUserFromContext(r)
	if !ok {
		httpx.Fail(w, http.StatusUnauthorized, "未登录")
		return
	}
	count, err := s.appNotifications.UnreadCount(r.Context(), user.ID)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "未读通知加载失败")
		return
	}
	httpx.OK(w, map[string]any{"unreadCount": count})
}

func (s *Server) appNotificationAction(w http.ResponseWriter, r *http.Request) {
	user, ok := appUserFromContext(r)
	if !ok {
		httpx.Fail(w, http.StatusUnauthorized, "未登录")
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/app/notifications/")
	if !strings.HasSuffix(path, "/read") {
		httpx.Fail(w, http.StatusNotFound, "通知不存在")
		return
	}
	idText := strings.TrimSuffix(path, "/read")
	if strings.Contains(idText, "/") {
		httpx.Fail(w, http.StatusNotFound, "通知不存在")
		return
	}
	id, err := strconv.ParseInt(idText, 10, 64)
	if err != nil || id <= 0 {
		httpx.Fail(w, http.StatusBadRequest, "通知编号无效")
		return
	}
	found, err := s.appNotifications.MarkRead(r.Context(), user.ID, id)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "通知更新失败")
		return
	}
	if !found {
		httpx.Fail(w, http.StatusNotFound, "通知不存在")
		return
	}
	httpx.OK(w, nil)
}

func (s *Server) appNotificationMarkAllRead(w http.ResponseWriter, r *http.Request) {
	user, ok := appUserFromContext(r)
	if !ok {
		httpx.Fail(w, http.StatusUnauthorized, "未登录")
		return
	}
	updated, err := s.appNotifications.MarkAllRead(r.Context(), user.ID)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "通知更新失败")
		return
	}
	httpx.OK(w, map[string]any{"updated": updated})
}

func positiveQueryInt(r *http.Request, key string, fallback int) int {
	value, err := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get(key)))
	if err != nil || value < 1 {
		return fallback
	}
	return value
}
