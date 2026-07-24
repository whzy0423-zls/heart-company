package server

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"nine-xing/nx-backend/apps/server/internal/httpx"
	"nine-xing/nx-backend/apps/server/internal/miniapp"
)

type miniappAdminReader interface {
	ListUsers(context.Context, miniapp.AdminListOptions) (miniapp.AdminUserPage, error)
	GetUserDetail(context.Context, int64, miniapp.AdminDetailOptions) (miniapp.AdminUserDetail, error)
}

func (s *Server) miniappUsers(w http.ResponseWriter, r *http.Request) {
	page, err := parseAdminPage(r, "page", "pageSize")
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid pagination")
		return
	}
	result, err := s.miniappAdmin.ListUsers(r.Context(), miniapp.AdminListOptions{
		Page: page.Page, PageSize: page.PageSize,
		Keyword: strings.TrimSpace(r.URL.Query().Get("keyword")),
		Channel: strings.TrimSpace(r.URL.Query().Get("channel")),
	})
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "query miniapp users failed")
		return
	}
	httpx.OK(w, result)
}

func (s *Server) miniappUserByID(w http.ResponseWriter, r *http.Request) {
	idText := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/api/miniapp/users/"))
	id, err := strconv.ParseInt(idText, 10, 64)
	if err != nil || id <= 0 || strings.Contains(idText, "/") {
		httpx.Fail(w, http.StatusBadRequest, "invalid id")
		return
	}
	testPage, err := parseAdminPage(r, "testPage", "testPageSize")
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid pagination")
		return
	}
	bookingPage, err := parseAdminPage(r, "bookingPage", "bookingPageSize")
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid pagination")
		return
	}
	result, err := s.miniappAdmin.GetUserDetail(r.Context(), id, miniapp.AdminDetailOptions{
		TestPage: testPage.Page, TestPageSize: testPage.PageSize,
		BookingPage: bookingPage.Page, BookingPageSize: bookingPage.PageSize,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpx.Fail(w, http.StatusNotFound, "miniapp user not found")
			return
		}
		httpx.Fail(w, http.StatusInternalServerError, "query miniapp user failed")
		return
	}
	httpx.OK(w, result)
}

func parseAdminPage(r *http.Request, pageKey, pageSizeKey string) (miniapp.AdminPagination, error) {
	parse := func(key string) (int, error) {
		value, exists := r.URL.Query()[key]
		if !exists {
			return 0, nil
		}
		if len(value) != 1 || strings.TrimSpace(value[0]) == "" {
			return 0, miniapp.ErrInvalidAdminPagination
		}
		parsed, err := strconv.Atoi(value[0])
		if err != nil || parsed <= 0 {
			return 0, miniapp.ErrInvalidAdminPagination
		}
		return parsed, nil
	}
	page, err := parse(pageKey)
	if err != nil {
		return miniapp.AdminPagination{}, err
	}
	pageSize, err := parse(pageSizeKey)
	if err != nil {
		return miniapp.AdminPagination{}, err
	}
	return miniapp.NormalizeAdminPagination(page, pageSize)
}
