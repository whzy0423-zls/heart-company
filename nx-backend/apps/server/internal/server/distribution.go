package server

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"nine-xing/nx-backend/apps/server/internal/httpx"
)

type distributionAgentResponse struct {
	ID            int64  `json:"id"`
	AppUserID     int64  `json:"appUserId"`
	AgentCode     string `json:"agentCode"`
	Level         int    `json:"level"`
	ParentAgentID int64  `json:"parentAgentId"`
	RootAgentID   int64  `json:"rootAgentId"`
	Path          string `json:"path"`
	Status        string `json:"status"`
}

func (s *Server) adminDistributionAgentCreate(w http.ResponseWriter, r *http.Request) {
	var in struct {
		AppUserID int64  `json:"appUserId"`
		AgentCode string `json:"agentCode"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil || in.AppUserID <= 0 {
		httpx.Fail(w, http.StatusBadRequest, "invalid appUserId")
		return
	}
	code := strings.TrimSpace(in.AgentCode)
	if code == "" {
		code = "A" + strconv.FormatInt(in.AppUserID, 10)
	}
	var out distributionAgentResponse
	err := s.db.QueryRowContext(r.Context(), `INSERT INTO distribution_agents(app_user_id,agent_code,level,root_agent_id,agent_path,status) VALUES($1,$2,1,0,'/','active') RETURNING id,app_user_id,agent_code,level,COALESCE(parent_agent_id,0),root_agent_id,agent_path,status`, in.AppUserID, code).Scan(&out.ID, &out.AppUserID, &out.AgentCode, &out.Level, &out.ParentAgentID, &out.RootAgentID, &out.Path, &out.Status)
	if err != nil {
		httpx.Fail(w, http.StatusConflict, err.Error())
		return
	}
	if _, err = s.db.ExecContext(r.Context(), `UPDATE distribution_agents SET root_agent_id=id,agent_path='/'||id||'/' WHERE id=$1`, out.ID); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	out.RootAgentID, out.Path = out.ID, "/"+strconv.FormatInt(out.ID, 10)+"/"
	httpx.OK(w, out)
}

func (s *Server) appDistributionOverview(w http.ResponseWriter, r *http.Request) {
	u, ok := appUserFromContext(r)
	if !ok {
		httpx.Fail(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var a distributionAgentResponse
	err := s.db.QueryRowContext(r.Context(), `SELECT id,app_user_id,agent_code,level,COALESCE(parent_agent_id,0),root_agent_id,agent_path,status FROM distribution_agents WHERE app_user_id=$1`, u.ID).Scan(&a.ID, &a.AppUserID, &a.AgentCode, &a.Level, &a.ParentAgentID, &a.RootAgentID, &a.Path, &a.Status)
	if err == sql.ErrNoRows {
		httpx.OK(w, map[string]any{"isAgent": false})
		return
	}
	if err != nil {
		httpx.Fail(w, 500, err.Error())
		return
	}
	var users, orders int64
	var commission int64
	_ = s.db.QueryRowContext(r.Context(), `SELECT count(*) FROM distribution_user_relations WHERE direct_agent_id=$1`, a.ID).Scan(&users)
	_ = s.db.QueryRowContext(r.Context(), `SELECT count(*) FROM distribution_commission_records WHERE agent_id=$1`, a.ID).Scan(&orders)
	_ = s.db.QueryRowContext(r.Context(), `SELECT COALESCE(sum(commission_amount),0) FROM distribution_commission_records WHERE agent_id=$1 AND status<>'reversed'`, a.ID).Scan(&commission)
	httpx.OK(w, map[string]any{"isAgent": true, "agent": a, "directUsers": users, "commissionRecords": orders, "commissionAmount": commission})
}
