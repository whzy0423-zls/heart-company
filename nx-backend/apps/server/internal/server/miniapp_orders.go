package server

import (
	"net/http"
	"strconv"
	"strings"

	"nine-xing/nx-backend/apps/server/internal/httpx"
	"nine-xing/nx-backend/apps/server/internal/siteconfig"
)

type miniappOrderItem struct {
	ID         string `json:"id"`
	OutTradeNo string `json:"outTradeNo"`
	Product    string `json:"product"`
	RefID      string `json:"refId"`
	Title      string `json:"title"`
	Amount     int    `json:"amount"`
	Status     string `json:"status"`
	CreateTime string `json:"createTime"`
	PaidAt     string `json:"paidAt"`
	BookingID  string `json:"bookingId"`
	CourseID   string `json:"courseId"`
	Cover      string `json:"cover"`
}

func (s *Server) miniappOrders(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	page, pageSize := appOrderPagination(query)
	status := strings.TrimSpace(query.Get("status"))
	switch status {
	case "", "pending", "paid", "closed", "refunded":
	default:
		httpx.Fail(w, http.StatusBadRequest, "invalid order status")
		return
	}
	args := []any{userFromRequest(r).ID}
	where := "o.wx_user_id=$1"
	if status != "" {
		args = append(args, status)
		where += " AND o.status=$2"
	}
	var total int
	if err := s.db.QueryRowContext(r.Context(), `SELECT count(*) FROM orders o WHERE `+where, args...).Scan(&total); err != nil {
		httpx.Fail(w, 500, "读取订单失败")
		return
	}
	args = append(args, pageSize, (page-1)*pageSize)
	rows, err := s.db.QueryContext(r.Context(), `SELECT o.id::text,o.out_trade_no,o.product,o.ref_id::text,o.title,o.amount,o.status,
 to_char(o.create_time AT TIME ZONE 'Asia/Shanghai','YYYY/MM/DD HH24:MI:SS'),COALESCE(to_char(o.paid_at AT TIME ZONE 'Asia/Shanghai','YYYY/MM/DD HH24:MI:SS'),''),COALESCE(b.id::text,''),COALESCE(b.course_id,'')
 FROM orders o LEFT JOIN bookings b ON o.product='course_booking' AND o.ref_id=b.id AND o.wx_user_id=b.wx_user_id
 WHERE `+where+` ORDER BY o.create_time DESC,o.id DESC LIMIT $`+strconv.Itoa(len(args)-1)+` OFFSET $`+strconv.Itoa(len(args)), args...)
	if err != nil {
		httpx.Fail(w, 500, "读取订单失败")
		return
	}
	defer rows.Close()
	items := []miniappOrderItem{}
	for rows.Next() {
		var item miniappOrderItem
		if err := rows.Scan(&item.ID, &item.OutTradeNo, &item.Product, &item.RefID, &item.Title, &item.Amount, &item.Status, &item.CreateTime, &item.PaidAt, &item.BookingID, &item.CourseID); err != nil {
			httpx.Fail(w, 500, "读取订单失败")
			return
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		httpx.Fail(w, 500, "读取订单失败")
		return
	}
	if err := rows.Close(); err != nil {
		httpx.Fail(w, 500, "读取订单失败")
		return
	}
	// Historical titles and amounts always come from immutable order snapshots.
	// Current covers are optional: deleting a catalog entry never hides an order.
	if len(items) > 0 {
		if courses, err := siteconfig.MiniappCoursesFromStore(r.Context(), s.db, s.env.SiteConfig); err == nil {
			covers := map[string]string{}
			for _, course := range courses {
				covers[course.ID] = course.Cover
			}
			for i := range items {
				items[i].Cover = covers[items[i].CourseID]
			}
		}
	}
	httpx.OK(w, map[string]any{"items": items, "total": total, "page": page, "pageSize": pageSize})
}
