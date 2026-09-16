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

// Serialize reservation/payout/refund mutations across processes. Payment
// transactions may already hold their order lock; this lock never takes one.
func lockDistributionLedger(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('nine-xing:distribution-ledger',0))`)
	return err
}

type distributionPeriod struct {
	AgentID int64  `json:"agentId"`
	Start   string `json:"start"`
	End     string `json:"end"`
}

func readDistributionPeriod(r *http.Request) (distributionPeriod, bool) {
	var in distributionPeriod
	if json.NewDecoder(r.Body).Decode(&in) != nil || in.AgentID <= 0 {
		return in, false
	}
	start, e1 := time.Parse("2006-01-02", in.Start)
	end, e2 := time.Parse("2006-01-02", in.End)
	return in, e1 == nil && e2 == nil && !end.Before(start)
}

const availableDistributionCommissions = `c.agent_id=$1 AND c.status='pending' AND c.commission_amount>0
 AND c.created_at >= $2::date AND c.created_at < ($3::date + INTERVAL '1 day')
 AND NOT EXISTS(SELECT 1 FROM distribution_settlement_items i JOIN distribution_settlements s ON s.id=i.settlement_id WHERE i.commission_id=c.id AND s.status IN ('draft','approved','pending','paid'))`

func (s *Server) adminDistributionSettlementPreview(w http.ResponseWriter, r *http.Request) {
	in, ok := readDistributionPeriod(r)
	if !ok {
		httpx.Fail(w, 400, "valid agentId and ordered YYYY-MM-DD dates are required")
		return
	}
	var amount int64
	var count int
	err := s.db.QueryRowContext(r.Context(), `SELECT COALESCE(sum(c.commission_amount),0),count(*) FROM distribution_commission_records c WHERE `+availableDistributionCommissions, in.AgentID, in.Start, in.End).Scan(&amount, &count)
	if err != nil {
		httpx.Fail(w, 500, "settlement preview failed")
		return
	}
	httpx.OK(w, map[string]any{"agentId": in.AgentID, "amount": amount, "count": count, "start": in.Start, "end": in.End})
}
func (s *Server) adminDistributionSettlementCreate(w http.ResponseWriter, r *http.Request) {
	in, ok := readDistributionPeriod(r)
	if !ok {
		httpx.Fail(w, 400, "valid agentId and ordered YYYY-MM-DD dates are required")
		return
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		httpx.Fail(w, 500, "settlement transaction failed")
		return
	}
	defer tx.Rollback()
	if err = lockDistributionLedger(r.Context(), tx); err != nil {
		httpx.Fail(w, 500, "settlement lock failed")
		return
	}
	// One statement snapshots picked rows, batch total, and reserved item amounts.
	var id, amount int64
	var count int
	err = tx.QueryRowContext(r.Context(), `WITH picked AS MATERIALIZED (
 SELECT c.id,c.commission_amount FROM distribution_commission_records c WHERE `+availableDistributionCommissions+` FOR UPDATE),
 batch AS (INSERT INTO distribution_settlements(agent_id,period_start,period_end,amount,status)
 SELECT $1,$2::date,$3::date,sum(commission_amount),'draft' FROM picked HAVING count(*)>0 RETURNING id,amount),
 items AS (INSERT INTO distribution_settlement_items(settlement_id,commission_id,amount) SELECT b.id,p.id,p.commission_amount FROM batch b CROSS JOIN picked p RETURNING id)
 SELECT b.id,b.amount,(SELECT count(*) FROM items) FROM batch b`, in.AgentID, in.Start, in.End).Scan(&id, &amount, &count)
	if err == sql.ErrNoRows {
		httpx.Fail(w, 409, "no settleable commissions")
		return
	}
	if err != nil {
		httpx.Fail(w, 409, "settlement reservation conflict")
		return
	}
	if err = tx.Commit(); err != nil {
		httpx.Fail(w, 500, "settlement commit failed")
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
	next := map[string]string{"approve": "approved", "paid": "paid", "reject": "rejected", "cancel": "cancelled"}[parts[1]]
	if next == "" {
		httpx.Fail(w, 400, "invalid action")
		return
	}
	var in struct {
		Reason           string `json:"reason"`
		PaymentReference string `json:"paymentReference"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&in)
	}
	in.PaymentReference = strings.TrimSpace(in.PaymentReference)
	in.Reason = strings.TrimSpace(in.Reason)
	if next == "paid" && (in.PaymentReference == "" || len(in.PaymentReference) > 200) {
		httpx.Fail(w, 400, "paymentReference is required (max 200 characters)")
		return
	}
	if (next == "rejected" || next == "cancelled") && in.Reason == "" {
		httpx.Fail(w, 400, "reason is required")
		return
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		httpx.Fail(w, 500, "settlement transaction failed")
		return
	}
	defer tx.Rollback()
	if err = lockDistributionLedger(r.Context(), tx); err != nil {
		httpx.Fail(w, 500, "settlement lock failed")
		return
	}
	var status string
	var amount int64
	err = tx.QueryRowContext(r.Context(), `SELECT status,amount FROM distribution_settlements WHERE id=$1 FOR UPDATE`, id).Scan(&status, &amount)
	if err == sql.ErrNoRows {
		httpx.Fail(w, 404, "settlement not found")
		return
	}
	if err != nil {
		httpx.Fail(w, 500, "settlement read failed")
		return
	}
	if !((status == "draft" && (next == "approved" || next == "rejected" || next == "cancelled")) || (status == "approved" && (next == "paid" || next == "cancelled"))) {
		httpx.Fail(w, 409, "invalid settlement state")
		return
	}
	if next == "approved" || next == "paid" {
		rows, e := tx.QueryContext(r.Context(), `SELECT c.status,c.commission_amount,i.amount FROM distribution_settlement_items i JOIN distribution_commission_records c ON c.id=i.commission_id WHERE i.settlement_id=$1 ORDER BY c.id FOR UPDATE OF c`, id)
		if e != nil {
			httpx.Fail(w, 500, "commission lock failed")
			return
		}
		var sum int64
		valid := true
		count := 0
		for rows.Next() {
			var st string
			var actual, snapshot int64
			if e = rows.Scan(&st, &actual, &snapshot); e != nil {
				break
			}
			count++
			sum += snapshot
			valid = valid && st == "pending" && actual == snapshot
		}
		if e == nil {
			e = rows.Err()
		}
		rows.Close()
		if e != nil {
			httpx.Fail(w, 500, "commission read failed")
			return
		}
		if !valid || count == 0 || sum != amount {
			httpx.Fail(w, 409, "commissions changed; cancel and recreate settlement")
			return
		}
	}
	if next == "paid" {
		if _, err = tx.ExecContext(r.Context(), `UPDATE distribution_commission_records SET status='settled',updated_at=now() WHERE id IN (SELECT commission_id FROM distribution_settlement_items WHERE settlement_id=$1)`, id); err != nil {
			httpx.Fail(w, 500, "commission payout failed")
			return
		}
	}
	if _, err = tx.ExecContext(r.Context(), `UPDATE distribution_settlements SET status=$2,payment_reference=CASE WHEN $2='paid' THEN $3 ELSE payment_reference END,action_reason=$4,paid_at=CASE WHEN $2='paid' THEN now() ELSE paid_at END WHERE id=$1`, id, next, in.PaymentReference, in.Reason); err != nil {
		httpx.Fail(w, 409, "settlement update conflict")
		return
	}
	if err = tx.Commit(); err != nil {
		httpx.Fail(w, 500, "settlement commit failed")
		return
	}
	httpx.OK(w, map[string]any{"updated": true, "status": next})
}
