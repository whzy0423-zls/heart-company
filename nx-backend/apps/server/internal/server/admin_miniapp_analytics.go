package server

import (
	"net/http"
	"strings"
	"time"

	"nine-xing/nx-backend/apps/server/internal/httpx"
)

type miniappAnalyticsSummary struct {
	PaidAmount    int     `json:"paidAmount"`
	PaidOrders    int     `json:"paidOrders"`
	TotalOrders   int     `json:"totalOrders"`
	PendingOrders int     `json:"pendingOrders"`
	ClosedOrders  int     `json:"closedOrders"`
	SuccessRate   float64 `json:"successRate"`
	AverageOrder  int     `json:"averageOrder"`
}

type miniappAnalyticsHour struct {
	Hour       int `json:"hour"`
	PaidAmount int `json:"paidAmount"`
	PaidOrders int `json:"paidOrders"`
}

type miniappAnalyticsStatus struct {
	Status string `json:"status"`
	Count  int    `json:"count"`
}

type miniappAnalyticsProduct struct {
	Product    string `json:"product"`
	Title      string `json:"title"`
	PaidAmount int    `json:"paidAmount"`
	PaidOrders int    `json:"paidOrders"`
}

type miniappAnalyticsRecentOrder struct {
	OutTradeNo    string `json:"outTradeNo"`
	Title         string `json:"title"`
	Amount        int    `json:"amount"`
	TransactionID string `json:"transactionId"`
	PaidAt        string `json:"paidAt"`
}

func parseMiniappAnalyticsDate(value string) (time.Time, error) {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return time.Time{}, err
	}
	if strings.TrimSpace(value) == "" {
		now := time.Now().In(location)
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, location), nil
	}
	return time.ParseInLocation("2006-01-02", strings.TrimSpace(value), location)
}

func (s *Server) adminMiniappAnalytics(w http.ResponseWriter, r *http.Request) {
	day, err := parseMiniappAnalyticsDate(r.URL.Query().Get("date"))
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, "日期格式应为 YYYY-MM-DD")
		return
	}
	dateText := day.Format("2006-01-02")

	var summary miniappAnalyticsSummary
	var paidCreatedOrders int
	if err := s.db.QueryRowContext(r.Context(), `
		SELECT count(*) FILTER (WHERE status='paid' AND paid_at >= ($1::timestamp AT TIME ZONE 'Asia/Shanghai')
		                                      AND paid_at < (($1::timestamp + interval '1 day') AT TIME ZONE 'Asia/Shanghai')),
		       COALESCE(sum(amount) FILTER (WHERE status='paid' AND paid_at >= ($1::timestamp AT TIME ZONE 'Asia/Shanghai')
		                                      AND paid_at < (($1::timestamp + interval '1 day') AT TIME ZONE 'Asia/Shanghai')), 0),
		       count(*) FILTER (WHERE create_time >= ($1::timestamp AT TIME ZONE 'Asia/Shanghai')
		                         AND create_time < (($1::timestamp + interval '1 day') AT TIME ZONE 'Asia/Shanghai')),
		       count(*) FILTER (WHERE status='paid' AND create_time >= ($1::timestamp AT TIME ZONE 'Asia/Shanghai')
		                                             AND create_time < (($1::timestamp + interval '1 day') AT TIME ZONE 'Asia/Shanghai')),
		       count(*) FILTER (WHERE status='pending' AND create_time >= ($1::timestamp AT TIME ZONE 'Asia/Shanghai')
		                                                AND create_time < (($1::timestamp + interval '1 day') AT TIME ZONE 'Asia/Shanghai')),
		       count(*) FILTER (WHERE status='closed' AND create_time >= ($1::timestamp AT TIME ZONE 'Asia/Shanghai')
		                                               AND create_time < (($1::timestamp + interval '1 day') AT TIME ZONE 'Asia/Shanghai'))
		FROM orders`, dateText).
		Scan(&summary.PaidOrders, &summary.PaidAmount, &summary.TotalOrders, &paidCreatedOrders, &summary.PendingOrders, &summary.ClosedOrders); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	if summary.TotalOrders > 0 {
		summary.SuccessRate = float64(paidCreatedOrders) / float64(summary.TotalOrders) * 100
	}
	if summary.PaidOrders > 0 {
		summary.AverageOrder = summary.PaidAmount / summary.PaidOrders
	}

	hours := make([]miniappAnalyticsHour, 0, 24)
	rows, err := s.db.QueryContext(r.Context(), `
		SELECT hours.hour,
		       COALESCE(sum(o.amount) FILTER (WHERE o.status='paid'), 0),
		       count(o.id) FILTER (WHERE o.status='paid')
		FROM generate_series(0, 23) AS hours(hour)
		LEFT JOIN orders o ON o.paid_at >= ($1::timestamp AT TIME ZONE 'Asia/Shanghai') + (hours.hour * interval '1 hour')
		                    AND o.paid_at < ($1::timestamp AT TIME ZONE 'Asia/Shanghai') + ((hours.hour + 1) * interval '1 hour')
		GROUP BY hours.hour ORDER BY hours.hour`, dateText)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	for rows.Next() {
		var point miniappAnalyticsHour
		if err := rows.Scan(&point.Hour, &point.PaidAmount, &point.PaidOrders); err != nil {
			rows.Close()
			httpx.Fail(w, http.StatusInternalServerError, err.Error())
			return
		}
		hours = append(hours, point)
	}
	rows.Close()

	statuses := []miniappAnalyticsStatus{}
	rows, err = s.db.QueryContext(r.Context(), `SELECT status, count(*) FROM orders WHERE create_time >= ($1::timestamp AT TIME ZONE 'Asia/Shanghai') AND create_time < (($1::timestamp + interval '1 day') AT TIME ZONE 'Asia/Shanghai') GROUP BY status ORDER BY count(*) DESC`, dateText)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	for rows.Next() {
		var item miniappAnalyticsStatus
		if err := rows.Scan(&item.Status, &item.Count); err != nil {
			rows.Close()
			httpx.Fail(w, http.StatusInternalServerError, err.Error())
			return
		}
		statuses = append(statuses, item)
	}
	rows.Close()

	products := []miniappAnalyticsProduct{}
	rows, err = s.db.QueryContext(r.Context(), `
		SELECT product, COALESCE(max(title), product), COALESCE(sum(amount), 0), count(*)
		FROM orders WHERE status='paid'
		  AND paid_at >= ($1::timestamp AT TIME ZONE 'Asia/Shanghai')
		  AND paid_at < (($1::timestamp + interval '1 day') AT TIME ZONE 'Asia/Shanghai')
		GROUP BY product ORDER BY sum(amount) DESC, count(*) DESC`, dateText)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	for rows.Next() {
		var item miniappAnalyticsProduct
		if err := rows.Scan(&item.Product, &item.Title, &item.PaidAmount, &item.PaidOrders); err != nil {
			rows.Close()
			httpx.Fail(w, http.StatusInternalServerError, err.Error())
			return
		}
		products = append(products, item)
	}
	rows.Close()

	recent := []miniappAnalyticsRecentOrder{}
	rows, err = s.db.QueryContext(r.Context(), `
		SELECT out_trade_no, title, amount, COALESCE(transaction_id, ''),
		       to_char(paid_at AT TIME ZONE 'Asia/Shanghai', 'HH24:MI:SS')
		FROM orders WHERE status='paid'
		  AND paid_at >= ($1::timestamp AT TIME ZONE 'Asia/Shanghai')
		  AND paid_at < (($1::timestamp + interval '1 day') AT TIME ZONE 'Asia/Shanghai')
		ORDER BY paid_at DESC NULLS LAST, id DESC LIMIT 10`, dateText)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	for rows.Next() {
		var item miniappAnalyticsRecentOrder
		if err := rows.Scan(&item.OutTradeNo, &item.Title, &item.Amount, &item.TransactionID, &item.PaidAt); err != nil {
			rows.Close()
			httpx.Fail(w, http.StatusInternalServerError, err.Error())
			return
		}
		recent = append(recent, item)
	}
	rows.Close()

	httpx.OK(w, map[string]any{
		"date": day.Format("2006-01-02"), "summary": summary, "hours": hours,
		"statuses": statuses, "products": products, "recentOrders": recent,
	})
}
