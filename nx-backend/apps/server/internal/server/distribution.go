package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"nine-xing/nx-backend/apps/server/internal/httpx"
)

type distributionAgentResponse struct {
	ID                    int64  `json:"id"`
	AppUserID             int64  `json:"appUserId"`
	AgentCode             string `json:"agentCode"`
	Level                 int    `json:"level"`
	ParentAgentID         int64  `json:"parentAgentId"`
	RootAgentID           int64  `json:"rootAgentId"`
	Path                  string `json:"path"`
	Status                string `json:"status"`
	DirectUserCount       int64  `json:"directUserCount"`
	SecondLevelAgentCount int64  `json:"secondLevelAgentCount"`
	ThirdLevelAgentCount  int64  `json:"thirdLevelAgentCount"`
}

type distributionAnalyticsSummary struct {
	TotalAgents             int64 `json:"totalAgents"`
	ActiveAgents            int64 `json:"activeAgents"`
	PausedAgents            int64 `json:"pausedAgents"`
	TotalOrderAmount        int64 `json:"totalOrderAmount"`
	TotalCommissionAmount   int64 `json:"totalCommissionAmount"`
	PendingCommissionAmount int64 `json:"pendingCommissionAmount"`
	SettledCommissionAmount int64 `json:"settledCommissionAmount"`
	CommissionRecordCount   int64 `json:"commissionRecordCount"`
}

type distributionAnalyticsTrendItem struct {
	Date             string `json:"date"`
	OrderAmount      int64  `json:"orderAmount"`
	CommissionAmount int64  `json:"commissionAmount"`
	CommissionCount  int64  `json:"commissionCount"`
}

type distributionAnalyticsAgentRanking struct {
	AgentID                 int64  `json:"agentId"`
	AgentCode               string `json:"agentCode"`
	AppUserID               int64  `json:"appUserId"`
	Status                  string `json:"status"`
	DirectUserCount         int64  `json:"directUserCount"`
	ChildAgentCount         int64  `json:"childAgentCount"`
	OrderCount              int64  `json:"orderCount"`
	OrderAmount             int64  `json:"orderAmount"`
	CommissionAmount        int64  `json:"commissionAmount"`
	PendingCommissionAmount int64  `json:"pendingCommissionAmount"`
}

func (s *Server) adminDistributionAnalytics(w http.ResponseWriter, r *http.Request) {
	var summary distributionAnalyticsSummary
	if err := s.db.QueryRowContext(r.Context(), `
		SELECT COUNT(*),
		       COUNT(*) FILTER (WHERE status='active'),
		       COUNT(*) FILTER (WHERE status='paused')
		FROM distribution_agents`).Scan(&summary.TotalAgents, &summary.ActiveAgents, &summary.PausedAgents); err != nil {
		httpx.Fail(w, 500, err.Error())
		return
	}
	if err := s.db.QueryRowContext(r.Context(), `
		SELECT COALESCE(SUM(CASE WHEN order_row=1 THEN order_amount ELSE 0 END),0),
		       COALESCE(SUM(commission_amount),0), COUNT(*)
		FROM (SELECT *, row_number() OVER(PARTITION BY order_id ORDER BY id) AS order_row
		 FROM distribution_commission_records WHERE status<>'reversed') c`).Scan(&summary.TotalOrderAmount, &summary.TotalCommissionAmount, &summary.CommissionRecordCount); err != nil {
		httpx.Fail(w, 500, err.Error())
		return
	}
	if err := s.db.QueryRowContext(r.Context(), `SELECT COALESCE(SUM(commission_amount),0) FROM distribution_commission_records WHERE status='pending'`).Scan(&summary.PendingCommissionAmount); err != nil {
		httpx.Fail(w, 500, err.Error())
		return
	}
	if err := s.db.QueryRowContext(r.Context(), `SELECT COALESCE(SUM(commission_amount),0) FROM distribution_commission_records WHERE status='settled'`).Scan(&summary.SettledCommissionAmount); err != nil {
		httpx.Fail(w, 500, err.Error())
		return
	}

	trend, err := generateDistributionTrend(r.Context(), s.db, 30)
	if err != nil {
		httpx.Fail(w, 500, err.Error())
		return
	}
	rankings, err := s.distributionAgentRankings(r.Context())
	if err != nil {
		httpx.Fail(w, 500, err.Error())
		return
	}
	httpx.OK(w, map[string]any{
		"summary":       summary,
		"trend":         trend,
		"agentRankings": rankings,
	})
}

func generateDistributionTrend(ctx context.Context, db *sql.DB, days int) ([]distributionAnalyticsTrendItem, error) {
	if days <= 0 {
		days = 30
	}
	start := time.Now().AddDate(0, 0, -days+1)
	items := make([]distributionAnalyticsTrendItem, 0, days)
	byDate := map[string]int{}
	for i := 0; i < days; i++ {
		date := start.AddDate(0, 0, i).Format("2006-01-02")
		items = append(items, distributionAnalyticsTrendItem{Date: date})
		byDate[date] = len(items) - 1
	}
	rows, err := db.QueryContext(ctx, `
		SELECT to_char(created_at::date,'YYYY-MM-DD') AS day,
		       COALESCE(SUM(CASE WHEN order_row=1 THEN order_amount ELSE 0 END),0),
		       COALESCE(SUM(commission_amount),0), COUNT(*)
		FROM (SELECT *,row_number() OVER(PARTITION BY order_id ORDER BY id) AS order_row
		 FROM distribution_commission_records WHERE created_at::date >= CURRENT_DATE - ($1::int - 1)
		 AND status<>'reversed') c
		GROUP BY day
		ORDER BY day`, days)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		var row distributionAnalyticsTrendItem
		if err := rows.Scan(&key, &row.OrderAmount, &row.CommissionAmount, &row.CommissionCount); err != nil {
			return nil, err
		}
		if index, ok := byDate[key]; ok {
			items[index].OrderAmount = row.OrderAmount
			items[index].CommissionAmount = row.CommissionAmount
			items[index].CommissionCount = row.CommissionCount
		}
	}
	return items, rows.Err()
}

func (s *Server) distributionAgentRankings(ctx context.Context) ([]distributionAnalyticsAgentRanking, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT a.id,
		       a.agent_code,
		       a.app_user_id,
		       a.status,
		       (SELECT COUNT(*) FROM distribution_user_relations rel WHERE rel.direct_agent_id=a.id) AS direct_user_count,
		       (SELECT COUNT(*) FROM distribution_agents child WHERE child.parent_agent_id=a.id) AS child_agent_count,
		       COUNT(c.id) AS order_count,
		       COALESCE(SUM(c.order_amount),0) AS order_amount,
		       COALESCE(SUM(c.commission_amount),0) AS commission_amount,
		       COALESCE(SUM(CASE WHEN c.status='pending' THEN c.commission_amount ELSE 0 END),0) AS pending_commission_amount
		FROM distribution_agents a
		LEFT JOIN distribution_commission_records c ON c.agent_id=a.id AND c.status<>'reversed'
		GROUP BY a.id,a.agent_code,a.app_user_id,a.status
		ORDER BY commission_amount DESC, order_count DESC, a.id DESC
		LIMIT 20`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []distributionAnalyticsAgentRanking{}
	for rows.Next() {
		var item distributionAnalyticsAgentRanking
		if err := rows.Scan(&item.AgentID, &item.AgentCode, &item.AppUserID, &item.Status, &item.DirectUserCount, &item.ChildAgentCount, &item.OrderCount, &item.OrderAmount, &item.CommissionAmount, &item.PendingCommissionAmount); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Server) currentDistributionAgent(ctx context.Context, appUserID int64) (distributionAgentResponse, error) {
	var current distributionAgentResponse
	err := s.db.QueryRowContext(ctx, `SELECT id,app_user_id,agent_code,level,COALESCE(parent_agent_id,0),root_agent_id,agent_path,status FROM distribution_agents WHERE app_user_id=$1 AND status='active'`, appUserID).Scan(&current.ID, &current.AppUserID, &current.AgentCode, &current.Level, &current.ParentAgentID, &current.RootAgentID, &current.Path, &current.Status)
	return current, err
}

func (s *Server) agentDistributionProfile(w http.ResponseWriter, r *http.Request) {
	current, err := s.currentDistributionAgent(r.Context(), userFromRequest(r).ID)
	if err == sql.ErrNoRows {
		httpx.Fail(w, http.StatusForbidden, "not an active agent")
		return
	}
	if err != nil {
		httpx.Fail(w, 500, err.Error())
		return
	}
	httpx.OK(w, current)
}

func (s *Server) agentDistributionAnalytics(w http.ResponseWriter, r *http.Request) {
	current, err := s.currentDistributionAgent(r.Context(), userFromRequest(r).ID)
	if err == sql.ErrNoRows {
		httpx.Fail(w, http.StatusForbidden, "not an active agent")
		return
	}
	if err != nil {
		httpx.Fail(w, 500, err.Error())
		return
	}
	start, end := distributionAnalyticsRange(r.URL.Query())
	summary, err := s.scopedDistributionAnalyticsSummary(r.Context(), current, start, end)
	if err != nil {
		httpx.Fail(w, 500, err.Error())
		return
	}
	trend, err := s.scopedDistributionTrend(r.Context(), current, start, end)
	if err != nil {
		httpx.Fail(w, 500, err.Error())
		return
	}
	users, err := s.scopedDistributionUsers(r.Context(), current, start, end)
	if err != nil {
		httpx.Fail(w, 500, err.Error())
		return
	}
	orders, err := s.scopedDistributionOrders(r.Context(), current, start, end)
	if err != nil {
		httpx.Fail(w, 500, err.Error())
		return
	}
	httpx.OK(w, map[string]any{"summary": summary, "trend": trend, "users": users, "orders": orders})
}

func distributionAnalyticsRange(q url.Values) (time.Time, time.Time) {
	now := time.Now()
	loc := now.Location()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	start := today.AddDate(0, 0, -6)
	end := today.Add(24*time.Hour - time.Nanosecond)
	if parsed, err := time.ParseInLocation("2006-01-02", q.Get("endDate"), loc); err == nil {
		end = parsed.Add(24*time.Hour - time.Nanosecond)
	}
	if parsed, err := time.ParseInLocation("2006-01-02", q.Get("startDate"), loc); err == nil {
		start = parsed
	}
	return start, end
}

func (s *Server) scopedDistributionAnalyticsSummary(ctx context.Context, current distributionAgentResponse, start, end time.Time) (distributionAnalyticsSummary, error) {
	var summary distributionAnalyticsSummary
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*),
		       COUNT(*) FILTER (WHERE agent.status='active'),
		       COUNT(*) FILTER (WHERE agent.status='paused')
		FROM distribution_agents agent
		WHERE agent.agent_path LIKE $1 || '%'`, current.Path).Scan(&summary.TotalAgents, &summary.ActiveAgents, &summary.PausedAgents); err != nil {
		return summary, err
	}
	if err := s.db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(CASE WHEN c.order_row=1 THEN c.order_amount ELSE 0 END),0),
		       COALESCE(SUM(c.commission_amount),0), COUNT(*)
		FROM (SELECT records.*,row_number() OVER(PARTITION BY records.order_id ORDER BY records.id) AS order_row
		 FROM distribution_commission_records records JOIN distribution_agents agent ON agent.id=records.agent_id
		 WHERE agent.agent_path LIKE $1 || '%' AND records.status<>'reversed'
		 AND records.created_at BETWEEN $2 AND $3) c`, current.Path, start, end).Scan(&summary.TotalOrderAmount, &summary.TotalCommissionAmount, &summary.CommissionRecordCount); err != nil {
		return summary, err
	}
	if err := s.db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(c.commission_amount),0)
		FROM distribution_commission_records c
		JOIN distribution_agents agent ON agent.id=c.agent_id
		WHERE agent.agent_path LIKE $1 || '%'
		  AND c.status='pending'
		  AND c.created_at BETWEEN $2 AND $3`, current.Path, start, end).Scan(&summary.PendingCommissionAmount); err != nil {
		return summary, err
	}
	if err := s.db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(c.commission_amount),0)
		FROM distribution_commission_records c
		JOIN distribution_agents agent ON agent.id=c.agent_id
		WHERE agent.agent_path LIKE $1 || '%'
		  AND c.status='settled'
		  AND c.created_at BETWEEN $2 AND $3`, current.Path, start, end).Scan(&summary.SettledCommissionAmount); err != nil {
		return summary, err
	}
	return summary, nil
}

func (s *Server) scopedDistributionTrend(ctx context.Context, current distributionAgentResponse, start, end time.Time) ([]distributionAnalyticsTrendItem, error) {
	items := []distributionAnalyticsTrendItem{}
	byDate := map[string]int{}
	for day := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, start.Location()); !day.After(end); day = day.AddDate(0, 0, 1) {
		key := day.Format("2006-01-02")
		items = append(items, distributionAnalyticsTrendItem{Date: key})
		byDate[key] = len(items) - 1
		if len(items) >= 370 {
			break
		}
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT to_char(c.created_at::date,'YYYY-MM-DD') AS day,
		       COALESCE(SUM(CASE WHEN c.order_row=1 THEN c.order_amount ELSE 0 END),0),
		       COALESCE(SUM(c.commission_amount),0), COUNT(*)
		FROM (SELECT records.*,row_number() OVER(PARTITION BY records.order_id ORDER BY records.id) AS order_row
		 FROM distribution_commission_records records JOIN distribution_agents agent ON agent.id=records.agent_id
		 WHERE agent.agent_path LIKE $1 || '%' AND records.status<>'reversed'
		 AND records.created_at BETWEEN $2 AND $3) c
		GROUP BY day
		ORDER BY day`, current.Path, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		var row distributionAnalyticsTrendItem
		if err := rows.Scan(&key, &row.OrderAmount, &row.CommissionAmount, &row.CommissionCount); err != nil {
			return nil, err
		}
		if index, ok := byDate[key]; ok {
			items[index].OrderAmount = row.OrderAmount
			items[index].CommissionAmount = row.CommissionAmount
			items[index].CommissionCount = row.CommissionCount
		}
	}
	return items, rows.Err()
}

func (s *Server) scopedDistributionUsers(ctx context.Context, current distributionAgentResponse, start, end time.Time) ([]map[string]any, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT u.id,u.nickname,u.member_level,r.direct_agent_id,r.bound_at,COALESCE(SUM(o.amount),0),COUNT(o.id)
		FROM distribution_user_relations r
		JOIN distribution_agents agent ON agent.id=r.direct_agent_id
		JOIN app_users u ON u.id=r.app_user_id
		LEFT JOIN app_orders o ON o.app_user_id=u.id AND o.paid_at BETWEEN $2 AND $3
		WHERE agent.agent_path LIKE $1 || '%'
		GROUP BY u.id,u.nickname,u.member_level,r.direct_agent_id,r.bound_at
		ORDER BY r.bound_at DESC
		LIMIT 200`, current.Path, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, directAgentID, amount, orderCount int64
		var nickname, memberLevel string
		var boundAt time.Time
		if err := rows.Scan(&id, &nickname, &memberLevel, &directAgentID, &boundAt, &amount, &orderCount); err != nil {
			return nil, err
		}
		items = append(items, map[string]any{"id": id, "nickname": nickname, "memberLevel": memberLevel, "directAgentId": directAgentID, "boundAt": boundAt.Format(time.RFC3339), "orderAmount": amount, "orderCount": orderCount})
	}
	return items, rows.Err()
}

func (s *Server) scopedDistributionOrders(ctx context.Context, current distributionAgentResponse, start, end time.Time) ([]map[string]any, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT o.id,o.out_trade_no,o.app_user_id,o.amount,o.status,o.paid_at
		FROM app_orders o
		JOIN distribution_user_relations rel ON rel.app_user_id=o.app_user_id
		JOIN distribution_agents agent ON agent.id=rel.direct_agent_id
		WHERE agent.agent_path LIKE $1 || '%'
		  AND o.paid_at BETWEEN $2 AND $3
		ORDER BY o.paid_at DESC NULLS LAST,o.id DESC
		LIMIT 200`, current.Path, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, appUserID, amount int64
		var outTradeNo, status string
		var paidAt sql.NullTime
		if err := rows.Scan(&id, &outTradeNo, &appUserID, &amount, &status, &paidAt); err != nil {
			return nil, err
		}
		paid := ""
		if paidAt.Valid {
			paid = paidAt.Time.Format(time.RFC3339)
		}
		items = append(items, map[string]any{"id": id, "outTradeNo": outTradeNo, "appUserId": appUserID, "amount": amount, "status": status, "paidAt": paid})
	}
	return items, rows.Err()
}

func (s *Server) agentDistributionAgentsRouter(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		s.agentDistributionCreateChild(w, r)
		return
	}
	s.agentDistributionAgents(w, r)
}

func (s *Server) agentDistributionAgents(w http.ResponseWriter, r *http.Request) {
	current, err := s.currentDistributionAgent(r.Context(), userFromRequest(r).ID)
	if err == sql.ErrNoRows {
		httpx.Fail(w, http.StatusForbidden, "not an active agent")
		return
	}
	if err != nil {
		httpx.Fail(w, 500, err.Error())
		return
	}
	rows, err := s.db.QueryContext(r.Context(), `
		SELECT agent.id,agent.app_user_id,agent.agent_code,agent.level,COALESCE(agent.parent_agent_id,0),agent.root_agent_id,agent.agent_path,agent.status,
		       (SELECT COUNT(*) FROM distribution_user_relations rel WHERE rel.direct_agent_id=agent.id) AS direct_user_count,
		       (SELECT COUNT(*) FROM distribution_agents child WHERE child.parent_agent_id=agent.id AND child.level=agent.level+1) AS second_level_agent_count,
		       0 AS third_level_agent_count
		FROM distribution_agents agent
		WHERE agent.parent_agent_id=$1
		ORDER BY agent.id DESC`, current.ID)
	if err != nil {
		httpx.Fail(w, 500, err.Error())
		return
	}
	defer rows.Close()
	items := []distributionAgentResponse{}
	for rows.Next() {
		var item distributionAgentResponse
		if err := rows.Scan(&item.ID, &item.AppUserID, &item.AgentCode, &item.Level, &item.ParentAgentID, &item.RootAgentID, &item.Path, &item.Status, &item.DirectUserCount, &item.SecondLevelAgentCount, &item.ThirdLevelAgentCount); err != nil {
			httpx.Fail(w, 500, err.Error())
			return
		}
		items = append(items, item)
	}
	httpx.OK(w, map[string]any{"items": items, "total": len(items), "current": current})
}

func (s *Server) agentDistributionCreateChild(w http.ResponseWriter, r *http.Request) {
	current, err := s.currentDistributionAgent(r.Context(), userFromRequest(r).ID)
	if err == sql.ErrNoRows {
		httpx.Fail(w, http.StatusForbidden, "not an active agent")
		return
	}
	if err != nil {
		httpx.Fail(w, 500, err.Error())
		return
	}
	if current.Level >= 3 {
		httpx.Fail(w, http.StatusForbidden, "agent cannot create child")
		return
	}
	var in struct {
		AppUserID int64 `json:"appUserId"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil || in.AppUserID <= 0 {
		httpx.Fail(w, 400, "invalid appUserId")
		return
	}
	var relationAgentID int64
	err = s.db.QueryRowContext(r.Context(), `SELECT direct_agent_id FROM distribution_user_relations WHERE app_user_id=$1`, in.AppUserID).Scan(&relationAgentID)
	if err == sql.ErrNoRows || relationAgentID != current.ID {
		httpx.Fail(w, http.StatusForbidden, "user was not directly invited by this agent")
		return
	}
	if err != nil {
		httpx.Fail(w, 500, err.Error())
		return
	}
	var out distributionAgentResponse
	err = s.db.QueryRowContext(r.Context(), `INSERT INTO distribution_agents(id,app_user_id,agent_code,level,parent_agent_id,root_agent_id,agent_path,status) VALUES(nextval('distribution_agents_id_seq'),$1::bigint,'A'||$1::text||'-'||currval('distribution_agents_id_seq'),$2,$3,$4,$5||currval('distribution_agents_id_seq')||'/','active') RETURNING id,app_user_id,agent_code,level,COALESCE(parent_agent_id,0),root_agent_id,agent_path,status`, in.AppUserID, current.Level+1, current.ID, current.RootAgentID, current.Path).Scan(&out.ID, &out.AppUserID, &out.AgentCode, &out.Level, &out.ParentAgentID, &out.RootAgentID, &out.Path, &out.Status)
	if err != nil {
		httpx.Fail(w, http.StatusConflict, err.Error())
		return
	}
	httpx.OK(w, out)
}

func (s *Server) adminDistributionAgentCreate(w http.ResponseWriter, r *http.Request) {
	var in struct {
		AppUserID int64 `json:"appUserId"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil || in.AppUserID <= 0 {
		httpx.Fail(w, http.StatusBadRequest, "invalid appUserId")
		return
	}
	code := "A" + strconv.FormatInt(in.AppUserID, 10)
	var appUserExists bool
	if err := s.db.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM app_users WHERE id=$1)`, in.AppUserID).Scan(&appUserExists); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !appUserExists {
		httpx.Fail(w, http.StatusNotFound, "app user not found")
		return
	}
	var existingAgentID int64
	err := s.db.QueryRowContext(r.Context(), `SELECT id FROM distribution_agents WHERE app_user_id=$1 OR lower(agent_code)=lower($2) LIMIT 1`, in.AppUserID, code).Scan(&existingAgentID)
	if err == nil {
		httpx.Fail(w, http.StatusConflict, "user is already an agent or agentCode already exists")
		return
	}
	if err != sql.ErrNoRows {
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback()
	createdBy := sql.NullInt64{Int64: userFromRequest(r).ID, Valid: userFromRequest(r).ID > 0}
	var out distributionAgentResponse
	err = tx.QueryRowContext(r.Context(), `INSERT INTO distribution_agents(id,app_user_id,agent_code,level,root_agent_id,agent_path,status,created_by) VALUES(nextval('distribution_agents_id_seq'),$1,$2,1,currval('distribution_agents_id_seq'),'/'||currval('distribution_agents_id_seq')||'/','active',$3) RETURNING id,app_user_id,agent_code,level,COALESCE(parent_agent_id,0),root_agent_id,agent_path,status`, in.AppUserID, code, createdBy).Scan(&out.ID, &out.AppUserID, &out.AgentCode, &out.Level, &out.ParentAgentID, &out.RootAgentID, &out.Path, &out.Status)
	if err != nil {
		httpx.Fail(w, http.StatusConflict, err.Error())
		return
	}
	if _, err = tx.ExecContext(r.Context(), `UPDATE distribution_agents SET root_agent_id=id,agent_path='/'||id||'/' WHERE id=$1`, out.ID); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err = tx.Commit(); err != nil {
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
	// Bound dates use the same China calendar displayed by the mobile app.
	var users, today, month, team int64
	err = s.db.QueryRowContext(r.Context(), `SELECT
  count(*) FILTER (WHERE rel.direct_agent_id=$1),
  count(*) FILTER (WHERE rel.direct_agent_id=$1 AND rel.bound_at >= (date_trunc('day',now() AT TIME ZONE 'Asia/Shanghai') AT TIME ZONE 'Asia/Shanghai')),
  count(*) FILTER (WHERE rel.direct_agent_id=$1 AND rel.bound_at >= (date_trunc('month',now() AT TIME ZONE 'Asia/Shanghai') AT TIME ZONE 'Asia/Shanghai')),
  count(*)
 FROM distribution_user_relations rel JOIN distribution_agents direct ON direct.id=rel.direct_agent_id
 WHERE direct.agent_path LIKE $2||'%'`, a.ID, a.Path).Scan(&users, &today, &month, &team)
	if err != nil {
		httpx.Fail(w, 500, "distribution user metrics failed")
		return
	}
	var orders, commission, pending, settled, available int64
	err = s.db.QueryRowContext(r.Context(), `SELECT count(*),
  COALESCE(sum(c.commission_amount) FILTER(WHERE c.status<>'reversed'),0),
  COALESCE(sum(c.commission_amount) FILTER(WHERE c.status='pending'),0),
  COALESCE(sum(c.commission_amount) FILTER(WHERE c.status='settled'),0),
  COALESCE(sum(c.commission_amount) FILTER(WHERE c.status='pending' AND c.commission_amount>0
   AND NOT EXISTS(SELECT 1 FROM distribution_settlement_items i JOIN distribution_settlements st ON st.id=i.settlement_id
    WHERE i.commission_id=c.id AND st.status IN ('draft','approved','pending','paid'))),0)
 FROM distribution_commission_records c WHERE c.agent_id=$1`, a.ID).Scan(&orders, &commission, &pending, &settled, &available)
	if err != nil {
		httpx.Fail(w, 500, "distribution commission metrics failed")
		return
	}
	httpx.OK(w, map[string]any{"isAgent": true, "agent": a, "directUsers": users, "todayInvites": today, "monthInvites": month, "teamUserCount": team,
		"commissionRecords": orders, "commissionAmount": commission, "pendingCommission": pending, "settledCommission": settled, "settleableCommission": available})
}

func (s *Server) appDistributionBind(w http.ResponseWriter, r *http.Request) {
	if _, ok := appUserFromContext(r); !ok {
		httpx.Fail(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	httpx.Fail(w, http.StatusForbidden, "邀请码仅支持注册时填写，注册后不支持补绑")
}

// The INSERT result, not a preflight existence check, determines whether this
// SMS sign-in created the account. Existing users never gain a new referral.
func createSMSUserWithDistributionInvite(ctx context.Context, db *sql.DB, phone, code string) error {
	code = strings.TrimSpace(code)
	if code == "" {
		return nil
	}
	if db == nil {
		return fmt.Errorf("distribution database unavailable")
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var agentID int64
	if err = tx.QueryRowContext(ctx, `SELECT id FROM distribution_agents WHERE lower(agent_code)=lower($1) AND status='active' FOR SHARE`, code).Scan(&agentID); err != nil {
		return err
	}
	var userID int64
	err = tx.QueryRowContext(ctx, `INSERT INTO app_users(phone) VALUES($1) ON CONFLICT(phone) DO NOTHING RETURNING id`, phone).Scan(&userID)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO distribution_user_relations(app_user_id,direct_agent_id) VALUES($1,$2)`, userID, agentID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO distribution_invite_events(agent_id,app_user_id,agent_code,event_type) VALUES($1,$2,$3,'register')`, agentID, userID, code); err != nil {
		return err
	}
	return tx.Commit()
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
	err = s.db.QueryRowContext(r.Context(), `INSERT INTO distribution_agents(id,app_user_id,agent_code,level,parent_agent_id,root_agent_id,agent_path,status) VALUES(nextval('distribution_agents_id_seq'),$1::bigint,'A'||$1::text||'-'||currval('distribution_agents_id_seq'),$2,$3,$4,$5||currval('distribution_agents_id_seq')||'/','active') RETURNING id,app_user_id,agent_code,level,COALESCE(parent_agent_id,0),root_agent_id,agent_path,status`, in.AppUserID, parent.Level+1, parent.ID, parent.RootAgentID, parent.Path).Scan(&out.ID, &out.AppUserID, &out.AgentCode, &out.Level, &out.ParentAgentID, &out.RootAgentID, &out.Path, &out.Status)
	if err != nil {
		httpx.Fail(w, http.StatusConflict, err.Error())
		return
	}
	httpx.OK(w, out)
}

func (s *Server) adminDistributionAgents(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(), `
		SELECT a.id,a.app_user_id,a.agent_code,a.level,COALESCE(a.parent_agent_id,0),a.root_agent_id,a.agent_path,a.status,
		       (SELECT COUNT(*) FROM distribution_user_relations rel WHERE rel.direct_agent_id=a.id) AS direct_user_count,
		       (SELECT COUNT(*) FROM distribution_agents child WHERE child.root_agent_id=a.id AND child.level=2) AS second_level_agent_count,
		       (SELECT COUNT(*) FROM distribution_agents child WHERE child.root_agent_id=a.id AND child.level=3) AS third_level_agent_count
		FROM distribution_agents a
		ORDER BY a.id DESC LIMIT 200`)
	if err != nil {
		httpx.Fail(w, 500, err.Error())
		return
	}
	defer rows.Close()
	items := []distributionAgentResponse{}
	for rows.Next() {
		var a distributionAgentResponse
		if err := rows.Scan(&a.ID, &a.AppUserID, &a.AgentCode, &a.Level, &a.ParentAgentID, &a.RootAgentID, &a.Path, &a.Status, &a.DirectUserCount, &a.SecondLevelAgentCount, &a.ThirdLevelAgentCount); err != nil {
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
	if json.NewDecoder(r.Body).Decode(&in) != nil || (in.Status != "active" && in.Status != "paused") {
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
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		httpx.Fail(w, 500, err.Error())
		return
	}
	defer tx.Rollback()
	if err = lockDistributionLedger(r.Context(), tx); err != nil {
		httpx.Fail(w, 500, "distribution ledger lock failed")
		return
	}
	res, err := tx.ExecContext(r.Context(), `UPDATE distribution_commission_records SET status='reversed',reversal_reason=$2,updated_at=now() WHERE id=$1 AND status='pending'`, id, strings.TrimSpace(in.Reason))
	if err != nil {
		httpx.Fail(w, 500, err.Error())
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		httpx.Fail(w, 404, "commission not found or already reversed")
		return
	}
	if err = tx.Commit(); err != nil {
		httpx.Fail(w, 500, err.Error())
		return
	}
	httpx.OK(w, map[string]any{"reversed": true, "reason": in.Reason})
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

func validateDistributionAgentCode(ctx context.Context, db *sql.DB, code string) (bool, error) {
	code = strings.TrimSpace(code)
	if db == nil || code == "" {
		return false, nil
	}
	var status string
	err := db.QueryRowContext(ctx, `SELECT status FROM distribution_agents WHERE lower(agent_code)=lower($1)`, code).Scan(&status)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return status == "active", nil
}

func bindDistributionAgent(ctx context.Context, db *sql.DB, userID int64, code string) {
	code = strings.TrimSpace(code)
	if db == nil || userID <= 0 || code == "" {
		return
	}
	_, _ = db.ExecContext(ctx, `INSERT INTO distribution_user_relations(app_user_id,direct_agent_id) SELECT $1,id FROM distribution_agents WHERE lower(agent_code)=lower($2) AND status='active' AND app_user_id<>$1 AND NOT EXISTS (SELECT 1 FROM distribution_agents existing WHERE existing.app_user_id=$1) ON CONFLICT(app_user_id) DO NOTHING`, userID, code)
}

func (s *Server) adminDistributionSettlements(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(), `SELECT id,agent_id,period_start,period_end,amount,status,created_at FROM distribution_settlements ORDER BY id DESC LIMIT 200`)
	if err != nil {
		httpx.Fail(w, 500, err.Error())
		return
	}
	defer rows.Close()
	type item struct {
		ID          int64  `json:"id"`
		AgentID     int64  `json:"agentId"`
		Amount      int64  `json:"amount"`
		PeriodStart string `json:"periodStart"`
		PeriodEnd   string `json:"periodEnd"`
		Status      string `json:"status"`
		CreatedAt   string `json:"createdAt"`
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
	if len(in.Rates) != 3 {
		httpx.Fail(w, 400, "rates for levels 1, 2 and 3 are required")
		return
	}
	var totalRate int64
	for level := 1; level <= 3; level++ {
		rate, exists := in.Rates[level]
		if !exists || rate < 0 || rate > 10000 {
			httpx.Fail(w, 400, "invalid rate")
			return
		}
		totalRate += rate
	}
	if totalRate > 10000 {
		httpx.Fail(w, 400, "commission rates cannot exceed 10000 bps")
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
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/admin/distribution/rules/"), "/")
	if len(parts) != 2 || parts[1] != "activate" {
		httpx.Fail(w, 404, "rule action not found")
		return
	}
	id, err := strconv.ParseInt(parts[0], 10, 64)
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
	if _, err = tx.ExecContext(r.Context(), `LOCK TABLE distribution_commission_rules IN SHARE ROW EXCLUSIVE MODE`); err != nil {
		httpx.Fail(w, 500, err.Error())
		return
	}
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
	rows, err := s.db.QueryContext(r.Context(), `
		SELECT r.id,
		       r.version,
		       r.status,
		       r.created_at,
		       COALESCE(r.activated_at,r.created_at),
		       COALESCE(jsonb_object_agg(i.agent_level::text,i.rate_bps ORDER BY i.agent_level) FILTER (WHERE i.id IS NOT NULL),'{}'::jsonb),
		       COALESCE(sum(i.rate_bps),0)
		  FROM distribution_commission_rules r
		  LEFT JOIN distribution_commission_rule_items i ON i.rule_id=r.id
		 GROUP BY r.id
		 ORDER BY r.version DESC`)
	if err != nil {
		httpx.Fail(w, 500, err.Error())
		return
	}
	defer rows.Close()
	type item struct {
		ID           int64         `json:"id"`
		Version      int64         `json:"version"`
		Status       string        `json:"status"`
		CreatedAt    string        `json:"createdAt"`
		ActivatedAt  string        `json:"activatedAt"`
		Rates        map[int]int64 `json:"rates"`
		TotalRateBPS int64         `json:"totalRateBps"`
	}
	out := []item{}
	for rows.Next() {
		var x item
		var a, b time.Time
		var ratesRaw []byte
		if err := rows.Scan(&x.ID, &x.Version, &x.Status, &a, &b, &ratesRaw, &x.TotalRateBPS); err != nil {
			httpx.Fail(w, 500, err.Error())
			return
		}
		x.CreatedAt = a.Format(time.RFC3339)
		x.ActivatedAt = b.Format(time.RFC3339)
		x.Rates = map[int]int64{1: 0, 2: 0, 3: 0}
		var rates map[int]int64
		if len(ratesRaw) > 0 && json.Unmarshal(ratesRaw, &rates) == nil {
			for level := 1; level <= 3; level++ {
				x.Rates[level] = rates[level]
			}
		}
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

func (s *Server) appDistributionProfile(w http.ResponseWriter, r *http.Request) {
	u, ok := appUserFromContext(r)
	if !ok {
		httpx.Fail(w, 401, "unauthorized")
		return
	}
	var a distributionAgentResponse
	if err := s.db.QueryRowContext(r.Context(), `SELECT id,app_user_id,agent_code,level,COALESCE(parent_agent_id,0),root_agent_id,agent_path,status FROM distribution_agents WHERE app_user_id=$1`, u.ID).Scan(&a.ID, &a.AppUserID, &a.AgentCode, &a.Level, &a.ParentAgentID, &a.RootAgentID, &a.Path, &a.Status); err == sql.ErrNoRows {
		httpx.Fail(w, 404, "not an agent")
		return
	} else if err != nil {
		httpx.Fail(w, 500, err.Error())
		return
	}
	httpx.OK(w, map[string]any{"agent": a, "inviteUrl": "/invite?agent=" + url.QueryEscape(a.AgentCode)})
}

func (s *Server) adminDistributionUsers(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(), `SELECT u.id,u.nickname,u.member_level,r.direct_agent_id,r.bound_at FROM distribution_user_relations r JOIN app_users u ON u.id=r.app_user_id ORDER BY r.bound_at DESC LIMIT 500`)
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

func (s *Server) adminDistributionOrders(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(), `SELECT o.id,o.out_trade_no,o.app_user_id,o.amount,o.status,o.paid_at FROM app_orders o JOIN distribution_user_relations rel ON rel.app_user_id=o.app_user_id ORDER BY o.id DESC LIMIT 500`)
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
