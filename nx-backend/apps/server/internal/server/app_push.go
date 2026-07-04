package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"nine-xing/nx-backend/apps/server/internal/httpx"
)

// --- App 端：设备推送令牌注册/注销 ---

func (s *Server) appPushRegister(w http.ResponseWriter, r *http.Request) {
	userInfo, ok := appUserFromContext(r)
	if !ok {
		httpx.Fail(w, http.StatusUnauthorized, "未登录")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
	var body struct {
		RegistrationID string `json:"registrationId"`
		Platform       string `json:"platform"`
		DeviceInfo     string `json:"deviceInfo"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.RegistrationID == "" {
		httpx.Fail(w, http.StatusBadRequest, "registrationId 不能为空")
		return
	}
	body.RegistrationID = strings.TrimSpace(body.RegistrationID)
	body.Platform = strings.TrimSpace(body.Platform)
	body.DeviceInfo = strings.TrimSpace(body.DeviceInfo)
	if body.RegistrationID == "" {
		httpx.Fail(w, http.StatusBadRequest, "registrationId 不能为空")
		return
	}
	if body.Platform == "" {
		body.Platform = "android"
	}

	if err := s.pushStore.RegisterDevice(r.Context(), userInfo.ID, body.RegistrationID, body.Platform, body.DeviceInfo); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "注册失败")
		return
	}
	httpx.OK(w, nil)
}

func (s *Server) appPushUnregister(w http.ResponseWriter, r *http.Request) {
	userInfo, ok := appUserFromContext(r)
	if !ok {
		httpx.Fail(w, http.StatusUnauthorized, "未登录")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 2<<10)
	var body struct {
		RegistrationID string `json:"registrationId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.RegistrationID == "" {
		httpx.Fail(w, http.StatusBadRequest, "registrationId 不能为空")
		return
	}
	body.RegistrationID = strings.TrimSpace(body.RegistrationID)
	if body.RegistrationID == "" {
		httpx.Fail(w, http.StatusBadRequest, "registrationId 不能为空")
		return
	}

	if err := s.pushStore.UnregisterDevice(r.Context(), userInfo.ID, body.RegistrationID); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "注销失败")
		return
	}
	httpx.OK(w, nil)
}
