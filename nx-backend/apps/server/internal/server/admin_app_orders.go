package server

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"

	"nine-xing/nx-backend/apps/server/internal/httpx"
)

type adminAppOrder struct {
	ID                  int64  `json:"id"`
	OutTradeNo          string `json:"outTradeNo"`
	AppUserID           int64  `json:"appUserId"`
	Phone               string `json:"phone"`
	Nickname            string `json:"nickname"`
	MemberLevel         string `json:"memberLevel"`
	ProductID           string `json:"productId"`
	Title               string `json:"title"`
	Amount              int    `json:"amount"`
	Status              string `json:"status"`
	TransactionID       string `json:"transactionId"`
	PaymentProvider     string `json:"paymentProvider"`
	PayChannel          string `json:"payChannel"`
	GatewayID           string `json:"gatewayId"`
	ProviderTradeNo     string `json:"providerTradeNo"`
	ProviderStatus      string `json:"providerStatus"`
	PayURL              string `json:"payUrl"`
	LastQueryAt         string `json:"lastQueryAt"`
	PaymentError        string `json:"paymentError"`
	CreateTime          string `json:"createTime"`
	DurationDays        int    `json:"durationDays"`
	UpdateTime          string `json:"updateTime"`
	PaidAt              string `json:"paidAt"`
	MemberStartedAt     string `json:"memberStartedAt"`
	MemberExpiresAt     string `json:"memberExpiresAt"`
	RemainingDays       int    `json:"remainingDays"`
	ActivationAt        string `json:"activationAt"`
	MembershipExpiresAt string `json:"membershipExpiresAt"`
}

func (s *Server) adminAppOrders(w http.ResponseWriter, r *http.Request) {
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
	if productID := strings.TrimSpace(query.Get("productId")); productID != "" {
		add("o.product_id = ?", productID)
	}
	if keyword := strings.TrimSpace(query.Get("keyword")); keyword != "" {
		add("(o.out_trade_no ILIKE ? OR u.phone ILIKE ? OR u.nickname ILIKE ?)", "%"+keyword+"%")
		args = append(args, "%"+keyword+"%", "%"+keyword+"%")
		// 修正 add 只替换首个占位符的情况。
		where[len(where)-1] = "(o.out_trade_no ILIKE $" + strconv.Itoa(len(args)-2) + " OR u.phone ILIKE $" + strconv.Itoa(len(args)-1) + " OR u.nickname ILIKE $" + strconv.Itoa(len(args)) + ")"
	}
	whereSQL := strings.Join(where, " AND ")

	var total int
	if err := s.db.QueryRowContext(r.Context(), `SELECT count(*) FROM app_orders o LEFT JOIN app_users u ON u.id=o.app_user_id WHERE `+whereSQL, args...).Scan(&total); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	listArgs := append([]any{}, args...)
	listArgs = append(listArgs, pageSize, (page-1)*pageSize)
	rows, err := s.db.QueryContext(r.Context(), `
		SELECT o.id, o.out_trade_no, o.app_user_id, COALESCE(u.phone,''), COALESCE(u.nickname,''), COALESCE(u.member_level,''),
		       o.product_id, o.title, o.amount, o.status, COALESCE(o.transaction_id,''),
		       COALESCE(o.payment_provider,'manual'), COALESCE(o.pay_channel,''), COALESCE(o.gateway_id,''),
		       COALESCE(o.provider_trade_no,''), COALESCE(o.provider_status,''), COALESCE(o.pay_url,''),
		       COALESCE(to_char(o.last_query_at AT TIME ZONE 'Asia/Shanghai', 'YYYY/MM/DD HH24:MI:SS'), ''),
		       COALESCE(o.payment_error,''),
		       to_char(o.create_time AT TIME ZONE 'Asia/Shanghai', 'YYYY/MM/DD HH24:MI:SS'),
		       to_char(o.update_time AT TIME ZONE 'Asia/Shanghai', 'YYYY/MM/DD HH24:MI:SS'),
		       o.paid_at,
		       COALESCE(to_char(u.member_started_at AT TIME ZONE 'Asia/Shanghai', 'YYYY/MM/DD HH24:MI:SS'), ''),
		       COALESCE(to_char(u.member_expires_at AT TIME ZONE 'Asia/Shanghai', 'YYYY/MM/DD HH24:MI:SS'), ''),
		       CASE WHEN u.member_expires_at > now() THEN CEIL(EXTRACT(EPOCH FROM (u.member_expires_at-now()))/86400)::int ELSE 0 END,
		       COALESCE(to_char(o.activation_at AT TIME ZONE 'Asia/Shanghai', 'YYYY/MM/DD HH24:MI:SS'), ''),
		       COALESCE(to_char(o.membership_expires_at AT TIME ZONE 'Asia/Shanghai', 'YYYY/MM/DD HH24:MI:SS'), '')
		FROM app_orders o
		LEFT JOIN app_users u ON u.id=o.app_user_id
		WHERE `+whereSQL+`
		ORDER BY o.create_time DESC, o.id DESC
		LIMIT $`+strconv.Itoa(len(args)+1)+` OFFSET $`+strconv.Itoa(len(args)+2), listArgs...)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	items := []adminAppOrder{}
	for rows.Next() {
		var item adminAppOrder
		var paidAt sql.NullTime
		if err := rows.Scan(&item.ID, &item.OutTradeNo, &item.AppUserID, &item.Phone, &item.Nickname, &item.MemberLevel, &item.ProductID, &item.Title, &item.Amount, &item.Status, &item.TransactionID,
			&item.PaymentProvider, &item.PayChannel, &item.GatewayID, &item.ProviderTradeNo, &item.ProviderStatus, &item.PayURL, &item.LastQueryAt, &item.PaymentError,
			&item.CreateTime, &item.UpdateTime, &paidAt, &item.MemberStartedAt, &item.MemberExpiresAt, &item.RemainingDays, &item.ActivationAt, &item.MembershipExpiresAt); err != nil {
			httpx.Fail(w, http.StatusInternalServerError, err.Error())
			return
		}
		if paidAt.Valid {
			item.PaidAt = paidAt.Time.Format("2006/01/02 15:04:05")
		}
		item.DurationDays, _ = membershipDurationDays(item.ProductID)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.OK(w, map[string]any{"items": items, "total": total})
}
