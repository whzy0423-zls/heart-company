package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"nine-xing/nx-backend/apps/server/internal/appnotification"
	"nine-xing/nx-backend/apps/server/internal/auth"
	"nine-xing/nx-backend/apps/server/internal/httpx"
)

type fakeAppNotificationService struct {
	listUserID, listPage, listPageSize int
	markUserID, markID                 int64
	markAllUserID                      int64
	found                              bool
	createdUsers                       []createdUserNotification
	createdAudiences                   []createdAudienceNotification
}

type createdUserNotification struct {
	userID                             int64
	kind, title, content, link, source string
}

type createdAudienceNotification struct {
	targetType, targetValue            string
	kind, title, content, link, source string
}

func (f *fakeAppNotificationService) CreateForUser(_ context.Context, userID int64, kind, title, content, link, source string) (int64, error) {
	f.createdUsers = append(f.createdUsers, createdUserNotification{userID, kind, title, content, link, source})
	return 1, nil
}

func (f *fakeAppNotificationService) CreateForAudience(_ context.Context, targetType, targetValue, kind, title, content, link, source string) (int64, error) {
	f.createdAudiences = append(f.createdAudiences, createdAudienceNotification{targetType, targetValue, kind, title, content, link, source})
	return 1, nil
}

func (f *fakeAppNotificationService) List(_ context.Context, userID int64, page, pageSize int) ([]appnotification.Notification, int, int, error) {
	f.listUserID, f.listPage, f.listPageSize = int(userID), page, pageSize
	return []appnotification.Notification{{ID: 9, Title: "今日成长", Read: false}}, 1, 1, nil
}

func (f *fakeAppNotificationService) UnreadCount(_ context.Context, userID int64) (int, error) {
	return int(userID), nil
}

func (f *fakeAppNotificationService) MarkRead(_ context.Context, userID, notificationID int64) (bool, error) {
	f.markUserID, f.markID = userID, notificationID
	return f.found, nil
}

func (f *fakeAppNotificationService) MarkAllRead(_ context.Context, userID int64) (int64, error) {
	f.markAllUserID = userID
	return 3, nil
}

func TestAppNotificationListUsesAuthenticatedUserAndPagination(t *testing.T) {
	service := &fakeAppNotificationService{}
	s := &Server{appNotifications: service}
	req := httptest.NewRequest(http.MethodGet, "/api/app/notifications?page=2&pageSize=5", nil)
	req = req.WithContext(contextWithAppUser(req.Context(), auth.UserInfo{ID: 7}))
	rec := httptest.NewRecorder()

	s.appNotificationList(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if service.listUserID != 7 || service.listPage != 2 || service.listPageSize != 5 {
		t.Fatalf("unexpected list call user=%d page=%d pageSize=%d", service.listUserID, service.listPage, service.listPageSize)
	}
	var response httpx.Response
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	data, ok := response.Data.(map[string]any)
	if !ok || data["unreadCount"] != float64(1) || data["total"] != float64(1) {
		t.Fatalf("unexpected response data %#v", response.Data)
	}
}

func TestAppNotificationMarkReadEnforcesOwnershipResult(t *testing.T) {
	service := &fakeAppNotificationService{found: false}
	s := &Server{appNotifications: service}
	req := httptest.NewRequest(http.MethodPost, "/api/app/notifications/99/read", nil)
	req = req.WithContext(contextWithAppUser(req.Context(), auth.UserInfo{ID: 7}))
	rec := httptest.NewRecorder()

	s.appNotificationAction(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if service.markUserID != 7 || service.markID != 99 {
		t.Fatalf("unexpected mark call user=%d id=%d", service.markUserID, service.markID)
	}
}

func TestAppNotificationMarkAllRead(t *testing.T) {
	service := &fakeAppNotificationService{}
	s := &Server{appNotifications: service}
	req := httptest.NewRequest(http.MethodPost, "/api/app/notifications/read-all", nil)
	req = req.WithContext(contextWithAppUser(req.Context(), auth.UserInfo{ID: 12}))
	rec := httptest.NewRecorder()

	s.appNotificationMarkAllRead(rec, req)

	if rec.Code != http.StatusOK || service.markAllUserID != 12 {
		t.Fatalf("status=%d user=%d body=%s", rec.Code, service.markAllUserID, rec.Body.String())
	}
}

func TestAdminPushPersistsAudienceInbox(t *testing.T) {
	service := &fakeAppNotificationService{}
	s := &Server{appNotifications: service}
	task := adminPushSendTask{
		recordID: 42, title: "系统公告", content: "新的成长内容已上线",
		targetType: "level", targetValue: "pro", deepLink: "/tasks",
	}

	if err := s.persistAdminPushInbox(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	if len(service.createdAudiences) != 1 {
		t.Fatalf("created audiences=%d", len(service.createdAudiences))
	}
	created := service.createdAudiences[0]
	if created.targetType != "level" || created.targetValue != "pro" || created.source != "admin-push:42" || created.link != "/tasks" {
		t.Fatalf("unexpected notification %+v", created)
	}
}
