package server

import (
	"net/http"

	"nine-xing/nx-backend/apps/server/internal/httpx"
	"nine-xing/nx-backend/apps/server/internal/xinzhili"
)

func (s *Server) appXinzhiliModeSnapshot(w http.ResponseWriter, r *http.Request) {
	user, ok := appUserFromContext(r)
	if !ok || user.ID <= 0 {
		httpx.Fail(w, http.StatusUnauthorized, "未登录")
		return
	}
	cfg, found, err := s.readXinzhiliRealtimeConfig(r.Context())
	if err != nil || !found || !cfg.Enabled {
		httpx.Fail(w, http.StatusServiceUnavailable, "芯之力尚未配置")
		return
	}
	store := s.xinzhiliModeStore
	if store == nil {
		store = xinzhili.NewStore(s.db)
	}
	conn := &xinzhiliRealtimeConn{userID: user.ID, modeStore: store}
	snapshot, err := conn.readModeSnapshot(r.Context(), cfg)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "读取芯之力模式失败")
		return
	}
	httpx.OK(w, snapshot)
}
