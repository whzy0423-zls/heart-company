package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

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
	err := s.db.QueryRowContext(r.Context(), `INSERT INTO distribution_agents(id,app_user_id,agent_code,level,root_agent_id,agent_path,status) VALUES(nextval('distribution_agents_id_seq'),$1,$2,1,currval('distribution_agents_id_seq'),'/'||currval('distribution_agents_id_seq')||'/','active') RETURNING id,app_user_id,agent_code,level,COALESCE(parent_agent_id,0),root_agent_id,agent_path,status`, in.AppUserID, code).Scan(&out.ID, &out.AppUserID, &out.AgentCode, &out.Level, &out.ParentAgentID, &out.RootAgentID, &out.Path, &out.Status)
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

func (s *Server) appDistributionBind(w http.ResponseWriter, r *http.Request) {
	u, ok := appUserFromContext(r)
	if !ok {
		httpx.Fail(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var in struct {
		AgentCode string `json:"agentCode"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil || strings.TrimSpace(in.AgentCode) == "" {
		httpx.Fail(w, http.StatusBadRequest, "agentCode is required")
		return
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		httpx.Fail(w, 500, err.Error())
		return
	}
	defer tx.Rollback()
	var agentID, agentUserID int64
	var status string
	err = tx.QueryRowContext(r.Context(), `SELECT id,app_user_id,status FROM distribution_agents WHERE lower(agent_code)=lower($1) FOR UPDATE`, strings.TrimSpace(in.AgentCode)).Scan(&agentID, &agentUserID, &status)
	if err == sql.ErrNoRows {
		httpx.Fail(w, http.StatusNotFound, "agent not found")
		return
	}
	if err != nil {
		httpx.Fail(w, 500, err.Error())
		return
	}
	if status != "active" {
		httpx.Fail(w, http.StatusConflict, "agent is inactive")
		return
	}
	if agentUserID == u.ID {
		httpx.Fail(w, http.StatusBadRequest, "cannot bind self")
		return
	}
	_, err = tx.ExecContext(r.Context(), `INSERT INTO distribution_user_relations(app_user_id,direct_agent_id) VALUES($1,$2)`, u.ID, agentID)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate") || strings.Contains(err.Error(), "unique") {
			httpx.Fail(w, http.StatusConflict, "user already bound")
			return
		}
		httpx.Fail(w, 500, err.Error())
		return
	}
	if err = tx.Commit(); err != nil {
		httpx.Fail(w, 500, err.Error())
		return
	}
	httpx.OK(w, map[string]any{"bound": true, "agentId": agentID})
}

func (s *Server) appDistributionCreateChild(w http.ResponseWriter, r *http.Request) {
	u, ok := appUserFromContext(r)
	if !ok {
		httpx.Fail(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var in struct {
		AppUserID int64 `json:"appUserId"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil || in.AppUserID <= 0 {
		httpx.Fail(w, 400, "invalid appUserId")
		return
	}
	var parent distributionAgentResponse
	err := s.db.QueryRowContext(r.Context(), `SELECT id,app_user_id,agent_code,level,COALESCE(parent_agent_id,0),root_agent_id,agent_path,status FROM distribution_agents WHERE app_user_id=$1`, u.ID).Scan(&parent.ID, &parent.AppUserID, &parent.AgentCode, &parent.Level, &parent.ParentAgentID, &parent.RootAgentID, &parent.Path, &parent.Status)
	if err == sql.ErrNoRows {
		httpx.Fail(w, 403, "not an agent")
		return
	}
	if err != nil {
		httpx.Fail(w, 500, err.Error())
		return
	}
	if parent.Status != "active" || parent.Level >= 3 {
		httpx.Fail(w, 403, "agent cannot create child")
		return
	}
	var relationAgentID int64
	err = s.db.QueryRowContext(r.Context(), `SELECT direct_agent_id FROM distribution_user_relations WHERE app_user_id=$1`, in.AppUserID).Scan(&relationAgentID)
	if err == sql.ErrNoRows || relationAgentID != parent.ID {
		httpx.Fail(w, 403, "user was not directly invited by this agent")
		return
	}
	var out distributionAgentResponse
	err = s.db.QueryRowContext(r.Context(), `INSERT INTO distribution_agents(id,app_user_id,agent_code,level,parent_agent_id,root_agent_id,agent_path,status) VALUES(nextval('distribution_agents_id_seq'),$1,'A'||$1||'-'||currval('distribution_agents_id_seq'),$2,$3,$4,$5||currval('distribution_agents_id_seq')||'/','active') RETURNING id,app_user_id,agent_code,level,COALESCE(parent_agent_id,0),root_agent_id,agent_path,status`, in.AppUserID, parent.Level+1, parent.ID, parent.RootAgentID, parent.Path).Scan(&out.ID, &out.AppUserID, &out.AgentCode, &out.Level, &out.ParentAgentID, &out.RootAgentID, &out.Path)
	if err != nil {
		httpx.Fail(w, http.StatusConflict, err.Error())
		return
	}
	httpx.OK(w, out)
}

func (s *Server) adminDistributionAgents(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(), `SELECT id,app_user_id,agent_code,level,COALESCE(parent_agent_id,0),root_agent_id,agent_path,status FROM distribution_agents ORDER BY id DESC LIMIT 200`)
	if err != nil {
		httpx.Fail(w, 500, err.Error())
		return
	}
	defer rows.Close()
	items := []distributionAgentResponse{}
	for rows.Next() {
		var a distributionAgentResponse
		if err := rows.Scan(&a.ID, &a.AppUserID, &a.AgentCode, &a.Level, &a.ParentAgentID, &a.RootAgentID, &a.Path, &a.Status); err != nil {
			httpx.Fail(w, 500, err.Error())
			return
		}
		items = append(items, a)
	}
	httpx.OK(w, map[string]any{"items": items, "total": len(items)})
}

func (s *Server) adminDistributionAgentStatus(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(strings.TrimPrefix(r.URL.Path, "/api/admin/distribution/agents/"), 10, 64)
	if err != nil || id <= 0 {
		httpx.Fail(w, 400, "invalid id")
		return
	}
	var in struct {
		Status string `json:"status"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil || (in.Status != "active" && in.Status != "paused" && in.Status != "disabled") {
		httpx.Fail(w, 400, "invalid status")
		return
	}
	res, err := s.db.ExecContext(r.Context(), `UPDATE distribution_agents SET status=$2,updated_at=now() WHERE id=$1`, id, in.Status)
	if err != nil {
		httpx.Fail(w, 500, err.Error())
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		httpx.Fail(w, 404, "agent not found")
		return
	}
	httpx.OK(w, map[string]any{"updated": true})
}

func (s *Server) appDistributionCommissions(w http.ResponseWriter, r *http.Request) {
	u, ok := appUserFromContext(r)
	if !ok {
		httpx.Fail(w, 401, "unauthorized")
		return
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT c.id,c.order_id,c.agent_level,c.order_amount,c.rate_bps,c.commission_amount,c.rule_version,c.status,c.created_at FROM distribution_commission_records c JOIN distribution_agents a ON a.id=c.agent_id WHERE a.app_user_id=$1 ORDER BY c.id DESC LIMIT 200`, u.ID)
	if err != nil {
		httpx.Fail(w, 500, err.Error())
		return
	}
	defer rows.Close()
	type item struct {
		ID, OrderID                  int64
		Level                        int
		OrderAmount, RateBPS, Amount int64
		RuleVersion                  int64
		Status                       string
		CreatedAt                    string
	}
	out := []item{}
	for rows.Next() {
		var x item
		var tm time.Time
		if err := rows.Scan(&x.ID, &x.OrderID, &x.Level, &x.OrderAmount, &x.RateBPS, &x.Amount, &x.RuleVersion, &x.Status, &tm); err != nil {
			httpx.Fail(w, 500, err.Error())
			return
		}
		x.CreatedAt = tm.Format(time.RFC3339)
		out = append(out, x)
	}
	httpx.OK(w, map[string]any{"items": out, "total": len(out)})
}

func (s *Server) adminDistributionAgentsRouter(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		s.adminDistributionAgentCreate(w, r)
		return
	}
	s.adminDistributionAgents(w, r)
}

func (s *Server) adminDistributionCommissions(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(), `SELECT id,order_id,agent_id,app_user_id,agent_level,order_amount,rate_bps,commission_amount,rule_version,status,created_at FROM distribution_commission_records ORDER BY id DESC LIMIT 500`)
	if err != nil {
		httpx.Fail(w, 500, err.Error())
		return
	}
	defer rows.Close()
	type row struct {
		ID, OrderID, AgentID, AppUserID           int64
		Level                                     int
		OrderAmount, RateBPS, Amount, RuleVersion int64
		Status                                    string
		CreatedAt                                 string
	}
	out := []row{}
	for rows.Next() {
		var x row
		var tm time.Time
		if err := rows.Scan(&x.ID, &x.OrderID, &x.AgentID, &x.AppUserID, &x.Level, &x.OrderAmount, &x.RateBPS, &x.Amount, &x.RuleVersion, &x.Status, &tm); err != nil {
			httpx.Fail(w, 500, err.Error())
			return
		}
		x.CreatedAt = tm.Format(time.RFC3339)
		out = append(out, x)
	}
	httpx.OK(w, map[string]any{"items": out, "total": len(out)})
}

func (s *Server) adminDistributionCommissionReverse(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(strings.TrimPrefix(r.URL.Path, "/api/admin/distribution/commissions/"), 10, 64)
	if err != nil || id <= 0 {
		httpx.Fail(w, 400, "invalid id")
		return
	}
	var in struct {
		Reason string `json:"reason"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil || strings.TrimSpace(in.Reason) == "" {
		httpx.Fail(w, 400, "reason is required")
		return
	}
	res, err := s.db.ExecContext(r.Context(), `UPDATE distribution_commission_records SET status='reversed',updated_at=now() WHERE id=$1 AND status<>'reversed'`, id)
	if err != nil {
		httpx.Fail(w, 500, err.Error())
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		httpx.Fail(w, 404, "commission not found or already reversed")
		return
	}
	httpx.OK(w, map[string]any{"reversed": true, "reason": in.Reason})
}

func (s *Server) adminDistributionSettlementPreview(w http.ResponseWriter, r *http.Request) {
	var in struct {
		AgentID int64  `json:"agentId"`
		Start   string `json:"start"`
		End     string `json:"end"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil || in.AgentID <= 0 || strings.TrimSpace(in.Start) == "" || strings.TrimSpace(in.End) == "" {
		httpx.Fail(w, 400, "agentId, start and end are required")
		return
	}
	var amount int64
	var count int
	err := s.db.QueryRowContext(r.Context(), `SELECT COALESCE(sum(commission_amount),0),count(*) FROM distribution_commission_records WHERE agent_id=$1 AND status='pending' AND created_at >= $2::date AND created_at < ($3::date + INTERVAL '1 day')`, in.AgentID, in.Start, in.End).Scan(&amount, &count)
	if err != nil {
		httpx.Fail(w, 500, err.Error())
		return
	}
	httpx.OK(w, map[string]any{"agentId": in.AgentID, "amount": amount, "count": count, "start": in.Start, "end": in.End})
}

func (s *Server) adminDistributionSettlementCreate(w http.ResponseWriter, r *http.Request) {
	var in struct {
		AgentID int64  `json:"agentId"`
		Start   string `json:"start"`
		End     string `json:"end"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil || in.AgentID <= 0 || strings.TrimSpace(in.Start) == "" || strings.TrimSpace(in.End) == "" {
		httpx.Fail(w, 400, "agentId, start and end are required")
		return
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		httpx.Fail(w, 500, err.Error())
		return
	}
	defer tx.Rollback()
	var id, amount int64
	var count int
	err = tx.QueryRowContext(r.Context(), `WITH picked AS (SELECT COALESCE(sum(commission_amount),0) amount,count(*) count FROM distribution_commission_records WHERE agent_id=$1 AND status='pending' AND created_at >= $2::date AND created_at < ($3::date + INTERVAL '1 day')) INSERT INTO distribution_settlements(agent_id,period_start,period_end,amount,status) SELECT $1,$2::date,$3::date,amount,'draft' FROM picked WHERE count>0 RETURNING id,amount,(SELECT count FROM picked)`, in.AgentID, in.Start, in.End).Scan(&id, &amount, &count)
	if err == sql.ErrNoRows {
		httpx.Fail(w, 409, "no settleable commissions")
		return
	}
	if err != nil {
		httpx.Fail(w, 409, err.Error())
		return
	}
	_, err = tx.ExecContext(r.Context(), `INSERT INTO distribution_settlement_items(settlement_id,commission_id,amount) SELECT $1,id,commission_amount FROM distribution_commission_records WHERE agent_id=$2 AND status='pending' AND created_at >= $3::date AND created_at < ($4::date + INTERVAL '1 day')`, id, in.AgentID, in.Start, in.End)
	if err != nil {
		httpx.Fail(w, 500, err.Error())
		return
	}
	if _, err = tx.ExecContext(r.Context(), `UPDATE distribution_commission_records SET status='pending',updated_at=now() WHERE agent_id=$1 AND status='pending' AND created_at >= $2::date AND created_at < ($3::date + INTERVAL '1 day')`, in.AgentID, in.Start, in.End); err != nil {
		httpx.Fail(w, 500, err.Error())
		return
	}
	if err = tx.Commit(); err != nil {
		httpx.Fail(w, 500, err.Error())
		return
	}
	httpx.OK(w, map[string]any{"settlementId": id, "amount": amount, "count": count})
}

func (s *Server) adminDistributionSettlementAction(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/admin/distribution/settlements/"), "/"), "/")
	if len(parts) != 2 {
		httpx.Fail(w, 400, "invalid settlement path")
		return
	}
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || id <= 0 {
		httpx.Fail(w, 400, "invalid id")
		return
	}
	action := parts[1]
	next := map[string]string{"approve": "approved", "paid": "paid", "reject": "rejected", "cancel": "canceled"}[action]
	if next == "" {
		httpx.Fail(w, 400, "invalid action")
		return
	}
	var clause string
	switch next {
	case "approved":
		clause = "status='draft'"
	case "paid":
		clause = "status='approved'"
	case "rejected":
		clause = "status='draft'"
	case "canceled":
		clause = "status IN ('draft','approved')"
	}
	q := `UPDATE distribution_settlements SET status=$1,paid_at=CASE WHEN $1='paid' THEN COALESCE(paid_at,now()) ELSE paid_at END WHERE id=$2 AND ` + clause
	res, err := s.db.ExecContext(r.Context(), q, next, id)
	if err != nil {
		httpx.Fail(w, 500, err.Error())
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		httpx.Fail(w, 409, "invalid settlement state or not found")
		return
	}
	if next == "paid" {
		if _, err = s.db.ExecContext(r.Context(), `UPDATE distribution_commission_records SET status='settled', updated_at=now() WHERE id IN (SELECT commission_id FROM distribution_settlement_items WHERE settlement_id=$1) AND status='pending'`, id); err != nil {
			httpx.Fail(w, 500, err.Error())
			return
		}
	}
	httpx.OK(w, map[string]any{"updated": true, "status": next})
}

func (s *Server) publicDistributionInvite(w http.ResponseWriter, r *http.Request) {
	code := strings.TrimSpace(r.URL.Query().Get("agent"))
	if code == "" {
		httpx.Fail(w, 400, "agent is required")
		return
	}
	var id int64
	var status string
	err := s.db.QueryRowContext(r.Context(), `SELECT id,status FROM distribution_agents WHERE lower(agent_code)=lower($1)`, code).Scan(&id, &status)
	if err == sql.ErrNoRows {
		httpx.Fail(w, 404, "agent not found")
		return
	}
	if err != nil {
		httpx.Fail(w, 500, err.Error())
		return
	}
	_, _ = s.db.ExecContext(r.Context(), `INSERT INTO distribution_invite_events(agent_id,agent_code,event_type) VALUES($1,$2,'click')`, id, code)
	httpx.OK(w, map[string]any{"valid": status == "active", "agentCode": code})
}

func bindDistributionAgent(ctx context.Context, db *sql.DB, userID int64, code string) {
	code = strings.TrimSpace(code)
	if db == nil || userID <= 0 || code == "" {
		return
	}
	_, _ = db.ExecContext(ctx, `INSERT INTO distribution_user_relations(app_user_id,direct_agent_id) SELECT $1,id FROM distribution_agents WHERE lower(agent_code)=lower($2) AND status='active' ON CONFLICT(app_user_id) DO NOTHING`, userID, code)
}

func (s *Server) adminDistributionSettlements(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(), `SELECT id,agent_id,period_start,period_end,amount,status,created_at FROM distribution_settlements ORDER BY id DESC LIMIT 200`)
	if err != nil {
		httpx.Fail(w, 500, err.Error())
		return
	}
	defer rows.Close()
	type item struct {
		ID, AgentID, Amount                       int64
		PeriodStart, PeriodEnd, Status, CreatedAt string
	}
	out := []item{}
	for rows.Next() {
		var x item
		var a, b time.Time
		var c time.Time
		if err := rows.Scan(&x.ID, &x.AgentID, &a, &b, &x.Amount, &x.Status, &c); err != nil {
			httpx.Fail(w, 500, err.Error())
			return
		}
		x.PeriodStart = a.Format("2006-01-02")
		x.PeriodEnd = b.Format("2006-01-02")
		x.CreatedAt = c.Format(time.RFC3339)
		out = append(out, x)
	}
	httpx.OK(w, map[string]any{"items": out, "total": len(out)})
}

func (s *Server) adminDistributionSettlementsRouter(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		s.adminDistributionSettlementCreate(w, r)
		return
	}
	s.adminDistributionSettlements(w, r)
}

func (s *Server) appDistributionUsers(w http.ResponseWriter, r *http.Request) {
	u, ok := appUserFromContext(r)
	if !ok {
		httpx.Fail(w, 401, "unauthorized")
		return
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT u.id,u.nickname,u.member_level,r.direct_agent_id,r.bound_at FROM distribution_user_relations r JOIN distribution_agents direct ON direct.id=r.direct_agent_id JOIN distribution_agents me ON direct.agent_path LIKE me.agent_path || '%' JOIN app_users u ON u.id=r.app_user_id WHERE me.app_user_id=$1 ORDER BY r.bound_at DESC LIMIT 200`, u.ID)
	if err != nil {
		httpx.Fail(w, 500, err.Error())
		return
	}
	defer rows.Close()
	type item struct {
		ID                    int64
		Nickname, MemberLevel string
		DirectAgentID         int64
		BoundAt               string
	}
	out := []item{}
	for rows.Next() {
		var x item
		var tm time.Time
		if err := rows.Scan(&x.ID, &x.Nickname, &x.MemberLevel, &x.DirectAgentID, &tm); err != nil {
			httpx.Fail(w, 500, err.Error())
			return
		}
		x.BoundAt = tm.Format(time.RFC3339)
		out = append(out, x)
	}
	httpx.OK(w, map[string]any{"items": out, "total": len(out)})
}

func (s *Server) appDistributionAgents(w http.ResponseWriter, r *http.Request) {
	u, ok := appUserFromContext(r)
	if !ok {
		httpx.Fail(w, 401, "unauthorized")
		return
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT child.id,child.app_user_id,child.agent_code,child.level,COALESCE(child.parent_agent_id,0),child.root_agent_id,child.agent_path,child.status FROM distribution_agents parent JOIN distribution_agents child ON child.parent_agent_id=parent.id WHERE parent.app_user_id=$1 ORDER BY child.id DESC`, u.ID)
	if err != nil {
		httpx.Fail(w, 500, err.Error())
		return
	}
	defer rows.Close()
	out := []distributionAgentResponse{}
	for rows.Next() {
		var a distributionAgentResponse
		if err := rows.Scan(&a.ID, &a.AppUserID, &a.AgentCode, &a.Level, &a.ParentAgentID, &a.RootAgentID, &a.Path, &a.Status); err != nil {
			httpx.Fail(w, 500, err.Error())
			return
		}
		out = append(out, a)
	}
	httpx.OK(w, map[string]any{"items": out, "total": len(out)})
}

func (s *Server) appDistributionOrders(w http.ResponseWriter, r *http.Request) {
	u, ok := appUserFromContext(r)
	if !ok {
		httpx.Fail(w, 401, "unauthorized")
		return
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT o.id,o.out_trade_no,o.app_user_id,o.amount,o.status,o.paid_at FROM app_orders o JOIN distribution_user_relations rel ON rel.app_user_id=o.app_user_id JOIN distribution_agents direct ON direct.id=rel.direct_agent_id JOIN distribution_agents me ON direct.agent_path LIKE me.agent_path||'%' WHERE me.app_user_id=$1 ORDER BY o.id DESC LIMIT 200`, u.ID)
	if err != nil {
		httpx.Fail(w, 500, err.Error())
		return
	}
	defer rows.Close()
	type item struct {
		ID, AppUserID, Amount      int64
		OutTradeNo, Status, PaidAt string
	}
	out := []item{}
	for rows.Next() {
		var x item
		var tm sql.NullTime
		if err := rows.Scan(&x.ID, &x.OutTradeNo, &x.AppUserID, &x.Amount, &x.Status, &tm); err != nil {
			httpx.Fail(w, 500, err.Error())
			return
		}
		if tm.Valid {
			x.PaidAt = tm.Time.Format(time.RFC3339)
		}
		out = append(out, x)
	}
	httpx.OK(w, map[string]any{"items": out, "total": len(out)})
}

func (s *Server) adminDistributionRuleCreate(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name  string        `json:"name"`
		Rates map[int]int64 `json:"rates"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil || strings.TrimSpace(in.Name) == "" {
		httpx.Fail(w, 400, "name is required")
		return
	}
	if len(in.Rates) == 0 {
		httpx.Fail(w, 400, "rates are required")
		return
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		httpx.Fail(w, 500, err.Error())
		return
	}
	defer tx.Rollback()
	var ruleID, version int64
	if err = tx.QueryRowContext(r.Context(), `INSERT INTO distribution_commission_rules(version,status) SELECT COALESCE(max(version),0)+1,'draft' FROM distribution_commission_rules RETURNING id,version`).Scan(&ruleID, &version); err != nil {
		httpx.Fail(w, 500, err.Error())
		return
	}
	for level, rate := range in.Rates {
		if level < 1 || level > 3 || rate < 0 || rate > 10000 {
			httpx.Fail(w, 400, "invalid rate")
			return
		}
		if _, err = tx.ExecContext(r.Context(), `INSERT INTO distribution_commission_rule_items(rule_id,agent_level,rate_bps) VALUES($1,$2,$3)`, ruleID, level, rate); err != nil {
			httpx.Fail(w, 500, err.Error())
			return
		}
	}
	if err = tx.Commit(); err != nil {
		httpx.Fail(w, 500, err.Error())
		return
	}
	httpx.OK(w, map[string]any{"id": ruleID, "version": version, "name": in.Name, "status": "draft"})
}

func (s *Server) adminDistributionRuleActivate(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(strings.TrimPrefix(r.URL.Path, "/api/admin/distribution/rules/"), 10, 64)
	if err != nil || id <= 0 {
		httpx.Fail(w, 400, "invalid id")
		return
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		httpx.Fail(w, 500, err.Error())
		return
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(r.Context(), `UPDATE distribution_commission_rules SET status='archived' WHERE status='active'`); err != nil {
		httpx.Fail(w, 500, err.Error())
		return
	}
	res, err := tx.ExecContext(r.Context(), `UPDATE distribution_commission_rules SET status='active',activated_at=now() WHERE id=$1 AND status='draft'`, id)
	if err != nil {
		httpx.Fail(w, 500, err.Error())
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		httpx.Fail(w, 409, "rule not found or not draft")
		return
	}
	if err = tx.Commit(); err != nil {
		httpx.Fail(w, 500, err.Error())
		return
	}
	httpx.OK(w, map[string]any{"activated": true, "id": id})
}

func (s *Server) adminDistributionRules(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(), `SELECT id,version,status,created_at,COALESCE(activated_at,created_at) FROM distribution_commission_rules ORDER BY version DESC`)
	if err != nil {
		httpx.Fail(w, 500, err.Error())
		return
	}
	defer rows.Close()
	type item struct {
		ID, Version                    int64
		Status, CreatedAt, ActivatedAt string
	}
	out := []item{}
	for rows.Next() {
		var x item
		var a, b time.Time
		if err := rows.Scan(&x.ID, &x.Version, &x.Status, &a, &b); err != nil {
			httpx.Fail(w, 500, err.Error())
			return
		}
		x.CreatedAt = a.Format(time.RFC3339)
		x.ActivatedAt = b.Format(time.RFC3339)
		out = append(out, x)
	}
	httpx.OK(w, map[string]any{"items": out, "total": len(out)})
}

func (s *Server) adminDistributionRulesRouter(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		s.adminDistributionRuleCreate(w, r)
		return
	}
	s.adminDistributionRules(w, r)
}
