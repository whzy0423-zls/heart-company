package server

import (
	"database/sql"
	"log"
	"net/http"
	"strconv"
	"strings"

	"nine-xing/nx-backend/apps/server/internal/httpx"
)

type adminMiniappOrder struct {
	ID            int64  `json:"id"`
	OutTradeNo    string `json:"outTradeNo"`
	WxUserID      int64  `json:"wxUserId"`
	Phone         string `json:"phone"`
	Nickname      string `json:"nickname"`
	Product       string `json:"product"`
	Title         string `json:"title"`
	Amount        int    `json:"amount"`
	Status        string `json:"status"`
	TransactionID string `json:"transactionId"`
	CreateTime    string `json:"createTime"`
	UpdateTime    string `json:"updateTime"`
	PaidAt        string `json:"paidAt"`
}

func (s *Server) adminMiniappOrders(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	page, pageSize := appOrderPagination(query)
	where := []string{"1=1"}
	args := []any{}
	add := func(clause string, value any) {
		args = append(args, value)
		where = append(where, strings.ReplaceAll(clause, "?", "$"+strconv.Itoa(len(args))))
	}
	if status := strings.TrimSpace(query.Get("status")); status != "" {
		add("o.status = ?", status)
	}
	if product := strings.TrimSpace(query.Get("product")); product != "" {
		add("o.product = ?", product)
	}
	if keyword := strings.TrimSpace(query.Get("keyword")); keyword != "" {
		value := "%" + keyword + "%"
		args = append(args, value, value, value)
		base := len(args) - 2
		where = append(where, "(o.out_trade_no ILIKE $"+strconv.Itoa(base)+" OR u.phone ILIKE $"+strconv.Itoa(base+1)+" OR u.nickname ILIKE $"+strconv.Itoa(base+2)+")")
	}
	whereSQL := strings.Join(where, " AND ")
	var total int
	if err := s.db.QueryRowContext(r.Context(), `SELECT count(*) FROM orders o LEFT JOIN wx_users u ON u.id=o.wx_user_id WHERE `+whereSQL, args...).Scan(&total); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	listArgs := append([]any{}, args...)
	listArgs = append(listArgs, pageSize, (page-1)*pageSize)
	rows, err := s.db.QueryContext(r.Context(), `
		SELECT o.id, o.out_trade_no, o.wx_user_id, COALESCE(u.phone,''), COALESCE(u.nickname,''),
		       o.product, o.title, o.amount, o.status, COALESCE(o.transaction_id,''),
		       to_char(o.create_time AT TIME ZONE 'Asia/Shanghai', 'YYYY/MM/DD HH24:MI:SS'),
		       to_char(o.update_time AT TIME ZONE 'Asia/Shanghai', 'YYYY/MM/DD HH24:MI:SS'),
		       COALESCE(to_char(o.paid_at AT TIME ZONE 'Asia/Shanghai', 'YYYY/MM/DD HH24:MI:SS'), '')
		FROM orders o LEFT JOIN wx_users u ON u.id=o.wx_user_id
		WHERE `+whereSQL+` ORDER BY o.create_time DESC, o.id DESC
		LIMIT $`+strconv.Itoa(len(args)+1)+` OFFSET $`+strconv.Itoa(len(args)+2), listArgs...)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	items := []adminMiniappOrder{}
	for rows.Next() {
		var item adminMiniappOrder
		var paidAt sql.NullString
		if err := rows.Scan(&item.ID, &item.OutTradeNo, &item.WxUserID, &item.Phone, &item.Nickname, &item.Product, &item.Title, &item.Amount, &item.Status, &item.TransactionID, &item.CreateTime, &item.UpdateTime, &paidAt); err != nil {
			httpx.Fail(w, http.StatusInternalServerError, err.Error())
			return
		}
		if paidAt.Valid {
			item.PaidAt = paidAt.String
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	var all, paid, pending, paidAmount int
	if err := s.db.QueryRowContext(r.Context(), `
		SELECT count(*),
		       count(*) FILTER (WHERE status='paid'),
		       count(*) FILTER (WHERE status='pending'),
		       COALESCE(sum(amount) FILTER (WHERE status='paid'), 0)
		FROM orders`).Scan(&all, &paid, &pending, &paidAmount); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.OK(w, map[string]any{
		"items":   items,
		"total":   total,
		"summary": map[string]int{"total": all, "paid": paid, "pending": pending, "paidAmount": paidAmount},
	})
}

func miniappOrderReconcileAction(tradeState string) string {
	switch strings.ToUpper(strings.TrimSpace(tradeState)) {
	case "SUCCESS":
		return "paid"
	case "CLOSED", "REVOKED", "PAYERROR":
		return "closed"
	default:
		return ""
	}
}

func (s *Server) adminMiniappOrdersReconcile(w http.ResponseWriter, r *http.Request) {
	if s.pay == nil {
		httpx.Fail(w, http.StatusServiceUnavailable, "微信支付服务未配置")
		return
	}
	rows, err := s.db.QueryContext(r.Context(), `
		SELECT out_trade_no, product, amount
		FROM orders
		WHERE status='pending'
		ORDER BY create_time DESC, id DESC
		LIMIT 50`)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	type pendingOrder struct {
		OutTradeNo string
		Product    string
		Amount     int
	}
	pending := make([]pendingOrder, 0, 20)
	for rows.Next() {
		var order pendingOrder
		if err := rows.Scan(&order.OutTradeNo, &order.Product, &order.Amount); err != nil {
			_ = rows.Close()
			httpx.Fail(w, http.StatusInternalServerError, err.Error())
			return
		}
		pending = append(pending, order)
	}
	if err := rows.Close(); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}

	checked, paid, closed, failed := 0, 0, 0, 0
	for _, order := range pending {
		result, queryErr := s.pay.QueryOrder(r.Context(), order.OutTradeNo)
		if queryErr != nil {
			log.Printf("wxpay reconcile query failed: out_trade_no=%s err=%v", order.OutTradeNo, queryErr)
			failed++
			continue
		}
		checked++
		log.Printf("wxpay reconcile queried: out_trade_no=%s trade_state=%s transaction_id=%s amount=%d appid=%s mchid=%s", order.OutTradeNo, result.TradeState, result.TransactionID, result.AmountTotal, result.AppID, result.MchID)
		switch miniappOrderReconcileAction(result.TradeState) {
		case "paid":
			if err := validateWxPayCallbackAgainstOrder(s.env, result, paymentOrderSnapshot{Amount: order.Amount, Product: order.Product}); err != nil {
				log.Printf("wxpay reconcile validation failed: out_trade_no=%s product=%s amount=%d err=%v", order.OutTradeNo, order.Product, order.Amount, err)
				failed++
				continue
			}
			apply, err := s.miniapp.MarkOrderPaidDetailed(r.Context(), order.OutTradeNo, result.TransactionID)
			if err != nil {
				log.Printf("wxpay reconcile mark paid failed: out_trade_no=%s transaction_id=%s err=%v", order.OutTradeNo, result.TransactionID, err)
				failed++
				continue
			}
			if err = closeCompetingPendingOrders(r.Context(), s.pay, apply.PendingToClose); err != nil {
				log.Printf("wxpay reconcile close remote competing orders failed: out_trade_no=%s err=%v", order.OutTradeNo, err)
				failed++
				continue
			}
			if err = s.miniapp.ClosePendingOrders(r.Context(), apply.PendingToClose); err != nil {
				log.Printf("wxpay reconcile close local competing orders failed: out_trade_no=%s err=%v", order.OutTradeNo, err)
				failed++
				continue
			}
			if apply.Changed {
				paid++
			}
		case "closed":
			result, err := s.db.ExecContext(r.Context(), `UPDATE orders SET status='closed', update_time=now() WHERE out_trade_no=$1 AND status='pending'`, order.OutTradeNo)
			if err != nil {
				failed++
				continue
			}
			if affected, _ := result.RowsAffected(); affected > 0 {
				closed++
			}
		}
	}
	httpx.OK(w, map[string]any{"checked": checked, "paid": paid, "closed": closed, "failed": failed})
}
