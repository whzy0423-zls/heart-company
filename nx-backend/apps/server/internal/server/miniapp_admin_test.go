package server

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"nine-xing/nx-backend/apps/server/internal/miniapp"
)

type fakeMiniappAdminReader struct {
	list       miniapp.AdminUserPage
	detail     miniapp.AdminUserDetail
	listErr    error
	detailErr  error
	listOpts   miniapp.AdminListOptions
	detailOpts miniapp.AdminDetailOptions
	detailID   int64
}

func (f *fakeMiniappAdminReader) ListUsers(_ context.Context, opts miniapp.AdminListOptions) (miniapp.AdminUserPage, error) {
	f.listOpts = opts
	return f.list, f.listErr
}

func (f *fakeMiniappAdminReader) GetUserDetail(_ context.Context, id int64, opts miniapp.AdminDetailOptions) (miniapp.AdminUserDetail, error) {
	f.detailID, f.detailOpts = id, opts
	return f.detail, f.detailErr
}

func TestMiniappUsersListParsesOptionsAndNeverLeaksSensitiveFields(t *testing.T) {
	reader := &fakeMiniappAdminReader{list: miniapp.AdminUserPage{Items: []miniapp.AdminUser{{ID: "7", Nickname: "小芯", Phone: "138****5678"}}, Total: 1}}
	s := &Server{miniappAdmin: reader}
	r := httptest.NewRequest(http.MethodGet, "/api/miniapp/users?page=2&pageSize=30&keyword=138123&channel=wechat", nil)
	w := httptest.NewRecorder()
	s.miniappUsers(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if reader.listOpts != (miniapp.AdminListOptions{Page: 2, PageSize: 30, Keyword: "138123", Channel: "wechat"}) {
		t.Fatalf("options=%+v", reader.listOpts)
	}
	if strings.Contains(strings.ToLower(w.Body.String()), "openid") || strings.Contains(w.Body.String(), "13812345678") {
		t.Fatalf("sensitive data leaked: %s", w.Body.String())
	}
}

func TestMiniappUsersDetailParsesIndependentPagination(t *testing.T) {
	reader := &fakeMiniappAdminReader{detail: miniapp.AdminUserDetail{User: miniapp.AdminUser{ID: "9223372036854775807"}}}
	s := &Server{miniappAdmin: reader}
	r := httptest.NewRequest(http.MethodGet, "/api/miniapp/users/9223372036854775807?testPage=2&testPageSize=3&bookingPage=4&bookingPageSize=5", nil)
	w := httptest.NewRecorder()
	s.miniappUserByID(w, r)
	if w.Code != http.StatusOK || reader.detailID != int64(9223372036854775807) {
		t.Fatalf("status=%d id=%d body=%s", w.Code, reader.detailID, w.Body.String())
	}
	want := miniapp.AdminDetailOptions{TestPage: 2, TestPageSize: 3, BookingPage: 4, BookingPageSize: 5}
	if reader.detailOpts != want {
		t.Fatalf("options=%+v want=%+v", reader.detailOpts, want)
	}
}

func TestMiniappUsersRejectInvalidAndOverflowParameters(t *testing.T) {
	tests := []string{
		"/api/miniapp/users?page=-1",
		"/api/miniapp/users?pageSize=101",
		"/api/miniapp/users/0",
		"/api/miniapp/users/9223372036854775808",
		"/api/miniapp/users/1?bookingPageSize=101",
	}
	for _, path := range tests {
		t.Run(path, func(t *testing.T) {
			s := &Server{miniappAdmin: &fakeMiniappAdminReader{}}
			r := httptest.NewRequest(http.MethodGet, path, nil)
			w := httptest.NewRecorder()
			if strings.TrimSuffix(path, "/") == "/api/miniapp/users" || strings.Contains(path, "/api/miniapp/users?") {
				s.miniappUsers(w, r)
			} else {
				s.miniappUserByID(w, r)
			}
			if w.Code != http.StatusBadRequest {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
		})
	}
}

func TestMiniappUsersMapsNotFoundAndDatabaseErrors(t *testing.T) {
	tests := []struct {
		err  error
		want int
	}{
		{err: sql.ErrNoRows, want: http.StatusNotFound},
		{err: errors.New("database unavailable"), want: http.StatusInternalServerError},
	}
	for _, tt := range tests {
		s := &Server{miniappAdmin: &fakeMiniappAdminReader{detailErr: tt.err}}
		w := httptest.NewRecorder()
		s.miniappUserByID(w, httptest.NewRequest(http.MethodGet, "/api/miniapp/users/7", nil))
		if w.Code != tt.want {
			t.Fatalf("err=%v status=%d body=%s", tt.err, w.Code, w.Body.String())
		}
	}
}

func TestMiniappUsersRoutesRequireCustomerMiniappPermission(t *testing.T) {
	source := readServerSourceForTest(t)
	wants := []string{
		`s.mux.HandleFunc("/api/miniapp/users", s.method(http.MethodGet, s.requirePermission("Customer:Miniapp:List", s.miniappUsers)))`,
		`s.mux.HandleFunc("/api/miniapp/users/", s.method(http.MethodGet, s.requirePermission("Customer:Miniapp:List", s.miniappUserByID)))`,
	}
	for _, want := range wants {
		if !strings.Contains(source, want) {
			t.Fatalf("missing protected route %q", want)
		}
	}
}

func readServerSourceForTest(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
