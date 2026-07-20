package server

import (
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"nine-xing/nx-backend/apps/server/internal/auditlog"
	"nine-xing/nx-backend/apps/server/internal/httpx"
)

type adminAppChatMessage struct {
	ID         int64           `json:"id"`
	SessionID  int64           `json:"sessionId"`
	AppUserID  int64           `json:"appUserId"`
	Phone      string          `json:"phone"`
	Nickname   string          `json:"nickname"`
	CardID     int64           `json:"cardId"`
	CardName   string          `json:"cardName"`
	Role       string          `json:"role"`
	Content    string          `json:"content"`
	Sources    json.RawMessage `json:"sources"`
	Favorite   bool            `json:"favorite"`
	Feedback   string          `json:"feedback"`
	CreateTime string          `json:"createTime"`
}

type adminAppMemory struct {
	ID         int64  `json:"id"`
	AppUserID  int64  `json:"appUserId"`
	Phone      string `json:"phone"`
	Nickname   string `json:"nickname"`
	CardID     int64  `json:"cardId"`
	CardName   string `json:"cardName"`
	Content    string `json:"content"`
	Status     string `json:"status"`
	SourceTime string `json:"sourceTime"`
	CreateTime string `json:"createTime"`
	UpdateTime string `json:"updateTime"`
}

func (s *Server) adminAppOrderGrant(w http.ResponseWriter, r *http.Request) {
	id, ok := parseTrailingIntID(w, r, "/api/app-orders/", "/grant")
	if !ok {
		return
	}
	var body struct {
		ActivationAt string `json:"activationAt"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4<<10)).Decode(&body); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "请选择会员生效时间")
		return
	}
	activationAt, err := time.Parse(time.RFC3339, strings.TrimSpace(body.ActivationAt))
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, "会员生效时间格式不正确")
		return
	}

	tx, err := s.db.BeginTx(r.Context(), &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback()
	var before adminAppOrder
	if err := tx.QueryRowContext(r.Context(), `
		SELECT id, out_trade_no, app_user_id, product_id, title, amount, status, transaction_id
		FROM app_orders WHERE id=$1 FOR UPDATE`, id).Scan(
		&before.ID, &before.OutTradeNo, &before.AppUserID, &before.ProductID,
		&before.Title, &before.Amount, &before.Status, &before.TransactionID,
	); err != nil {
		httpx.Fail(w, http.StatusNotFound, "order not found")
		return
	}
	if before.Status == "paid" {
		var level string
		var startedAt, expiresAt sql.NullTime
		if err := tx.QueryRowContext(r.Context(), `SELECT member_level, member_started_at, member_expires_at FROM app_users WHERE id=$1`, before.AppUserID).Scan(&level, &startedAt, &expiresAt); err != nil {
			httpx.Fail(w, http.StatusInternalServerError, err.Error())
			return
		}
		if err := tx.Commit(); err != nil {
			httpx.Fail(w, http.StatusInternalServerError, err.Error())
			return
		}
		httpx.OK(w, adminMembershipGrantResp{OrderID: id, PlanCode: level, StartedAt: formatNullableTime(startedAt), ExpiresAt: formatNullableTime(expiresAt), AlreadyGranted: true})
		return
	}
	if _, err := membershipDurationDays(before.ProductID); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "订单商品不是可开通的会员套餐")
		return
	}
	var currentStartedAt, currentExpiresAt sql.NullTime
	if err := tx.QueryRowContext(r.Context(), `SELECT member_started_at, member_expires_at FROM app_users WHERE id=$1 FOR UPDATE`, before.AppUserID).Scan(&currentStartedAt, &currentExpiresAt); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	var currentExpiry *time.Time
	if currentExpiresAt.Valid {
		currentExpiry = &currentExpiresAt.Time
	}
	period, err := calculateMembershipPeriod(before.ProductID, activationAt, currentExpiry)
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	startedAt := period.Start
	if currentExpiresAt.Valid && currentExpiresAt.Time.After(activationAt) && currentStartedAt.Valid {
		startedAt = currentStartedAt.Time
	}
	if _, err := tx.ExecContext(r.Context(), `
		UPDATE app_users
		SET member_level=$1, member_started_at=$2, member_expires_at=$3, update_time=now()
		WHERE id=$4`, before.ProductID, startedAt, period.Expires, before.AppUserID); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	if _, err := tx.ExecContext(r.Context(), `
		UPDATE app_orders
		SET status='paid', paid_at=COALESCE(paid_at, now()), activation_at=$2,
		    membership_expires_at=$3, update_time=now()
		WHERE id=$1`, id, activationAt, period.Expires); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := tx.Commit(); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	after := adminMembershipGrantResp{OrderID: id, PlanCode: before.ProductID, StartedAt: startedAt.Format(time.RFC3339), ExpiresAt: period.Expires.Format(time.RFC3339)}
	s.recordAdminAudit(r, auditlog.Entry{Action: "app_order.grant", TargetType: "app_order", TargetID: strconv.FormatInt(id, 10), Before: before, After: after, Summary: "确认收款并开通 App 会员"})
	httpx.OK(w, after)
}

type adminMembershipGrantResp struct {
	OrderID        int64  `json:"orderId"`
	PlanCode       string `json:"planCode"`
	StartedAt      string `json:"startedAt"`
	ExpiresAt      string `json:"expiresAt"`
	AlreadyGranted bool   `json:"alreadyGranted"`
}

func formatNullableTime(value sql.NullTime) string {
	if !value.Valid {
		return ""
	}
	return value.Time.Format(time.RFC3339)
}

func (s *Server) adminAppChatMessages(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, pageSize := appOrderPagination(q)
	where := []string{"1=1"}
	args := []any{}
	add := func(clause string, value any) {
		args = append(args, value)
		where = append(where, strings.ReplaceAll(clause, "?", "$"+strconv.Itoa(len(args))))
	}
	if role := strings.TrimSpace(q.Get("role")); role != "" {
		add("m.role = ?", role)
	}
	if feedback := strings.TrimSpace(q.Get("feedback")); feedback != "" {
		add("m.feedback = ?", feedback)
	}
	if keyword := strings.TrimSpace(q.Get("keyword")); keyword != "" {
		args = append(args, "%"+keyword+"%", "%"+keyword+"%", "%"+keyword+"%")
		n := len(args)
		where = append(where, "(m.content ILIKE $"+strconv.Itoa(n-2)+" OR u.phone ILIKE $"+strconv.Itoa(n-1)+" OR u.nickname ILIKE $"+strconv.Itoa(n)+")")
	}
	whereSQL := strings.Join(where, " AND ")
	var total int
	if err := s.db.QueryRowContext(r.Context(), `SELECT count(*) FROM app_chat_messages m JOIN app_chat_sessions cs ON cs.id=m.session_id JOIN app_users u ON u.id=cs.app_user_id LEFT JOIN app_user_cards c ON c.id=cs.card_id WHERE `+whereSQL, args...).Scan(&total); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	listArgs := append(append([]any{}, args...), pageSize, (page-1)*pageSize)
	rows, err := s.db.QueryContext(r.Context(), `
		SELECT m.id, m.session_id, cs.app_user_id, COALESCE(u.phone,''), COALESCE(u.nickname,''), cs.card_id, COALESCE(c.name,''), m.role, m.content, m.sources, m.favorite, m.feedback,
		       to_char(m.create_time AT TIME ZONE 'Asia/Shanghai', 'YYYY/MM/DD HH24:MI:SS')
		FROM app_chat_messages m JOIN app_chat_sessions cs ON cs.id=m.session_id JOIN app_users u ON u.id=cs.app_user_id LEFT JOIN app_user_cards c ON c.id=cs.card_id
		WHERE `+whereSQL+` ORDER BY m.create_time DESC, m.id DESC LIMIT $`+strconv.Itoa(len(args)+1)+` OFFSET $`+strconv.Itoa(len(args)+2), listArgs...)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	items := []adminAppChatMessage{}
	for rows.Next() {
		var item adminAppChatMessage
		if err := rows.Scan(&item.ID, &item.SessionID, &item.AppUserID, &item.Phone, &item.Nickname, &item.CardID, &item.CardName, &item.Role, &item.Content, &item.Sources, &item.Favorite, &item.Feedback, &item.CreateTime); err != nil {
			httpx.Fail(w, http.StatusInternalServerError, err.Error())
			return
		}
		if len(item.Sources) == 0 {
			item.Sources = json.RawMessage(`[]`)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.OK(w, map[string]any{"items": items, "total": total})
}

func (s *Server) adminAppMemories(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, pageSize := appOrderPagination(q)
	where := []string{"1=1"}
	args := []any{}
	add := func(clause string, value any) {
		args = append(args, value)
		where = append(where, strings.ReplaceAll(clause, "?", "$"+strconv.Itoa(len(args))))
	}
	if status := strings.TrimSpace(q.Get("status")); status != "" {
		add("m.status = ?", status)
	}
	if keyword := strings.TrimSpace(q.Get("keyword")); keyword != "" {
		args = append(args, "%"+keyword+"%", "%"+keyword+"%", "%"+keyword+"%")
		n := len(args)
		where = append(where, "(m.content ILIKE $"+strconv.Itoa(n-2)+" OR u.phone ILIKE $"+strconv.Itoa(n-1)+" OR u.nickname ILIKE $"+strconv.Itoa(n)+")")
	}
	whereSQL := strings.Join(where, " AND ")
	var total int
	if err := s.db.QueryRowContext(r.Context(), `SELECT count(*) FROM app_memories m JOIN app_users u ON u.id=m.app_user_id LEFT JOIN app_user_cards c ON c.id=m.card_id WHERE `+whereSQL, args...).Scan(&total); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	listArgs := append(append([]any{}, args...), pageSize, (page-1)*pageSize)
	rows, err := s.db.QueryContext(r.Context(), `
		SELECT m.id, m.app_user_id, COALESCE(u.phone,''), COALESCE(u.nickname,''), m.card_id, COALESCE(c.name,''), m.content, m.status,
		       COALESCE(to_char(m.source_time AT TIME ZONE 'Asia/Shanghai', 'YYYY/MM/DD HH24:MI:SS'), ''),
		       to_char(m.create_time AT TIME ZONE 'Asia/Shanghai', 'YYYY/MM/DD HH24:MI:SS'),
		       to_char(m.update_time AT TIME ZONE 'Asia/Shanghai', 'YYYY/MM/DD HH24:MI:SS')
		FROM app_memories m JOIN app_users u ON u.id=m.app_user_id LEFT JOIN app_user_cards c ON c.id=m.card_id
		WHERE `+whereSQL+` ORDER BY m.update_time DESC, m.id DESC LIMIT $`+strconv.Itoa(len(args)+1)+` OFFSET $`+strconv.Itoa(len(args)+2), listArgs...)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	items := []adminAppMemory{}
	for rows.Next() {
		var item adminAppMemory
		if err := rows.Scan(&item.ID, &item.AppUserID, &item.Phone, &item.Nickname, &item.CardID, &item.CardName, &item.Content, &item.Status, &item.SourceTime, &item.CreateTime, &item.UpdateTime); err != nil {
			httpx.Fail(w, http.StatusInternalServerError, err.Error())
			return
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.OK(w, map[string]any{"items": items, "total": total})
}

func (s *Server) adminAppMemoryStatus(w http.ResponseWriter, r *http.Request) {
	id, ok := parseTrailingIntID(w, r, "/api/app-memories/", "/status")
	if !ok {
		return
	}
	var body struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid request body")
		return
	}
	body.Status = strings.TrimSpace(body.Status)
	if body.Status != "active" && body.Status != "disabled" {
		httpx.Fail(w, http.StatusBadRequest, "invalid status")
		return
	}
	var beforeStatus string
	_ = s.db.QueryRowContext(r.Context(), `SELECT status FROM app_memories WHERE id=$1`, id).Scan(&beforeStatus)
	result, err := s.db.ExecContext(r.Context(), `UPDATE app_memories SET status=$1, update_time=now() WHERE id=$2`, body.Status, id)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	if n, _ := result.RowsAffected(); n == 0 {
		httpx.Fail(w, http.StatusNotFound, "memory not found")
		return
	}
	s.recordAdminAudit(r, auditlog.Entry{Action: "app_memory.status", TargetType: "app_memory", TargetID: strconv.FormatInt(id, 10), Before: map[string]any{"status": beforeStatus}, After: map[string]any{"status": body.Status}, Summary: "更新 App 私库记忆状态"})
	httpx.OK(w, true)
}

func parseTrailingIntID(w http.ResponseWriter, r *http.Request, prefix, suffix string) (int64, bool) {
	text := strings.TrimPrefix(r.URL.Path, prefix)
	text = strings.TrimSuffix(text, suffix)
	text = strings.Trim(text, "/")
	id, err := strconv.ParseInt(text, 10, 64)
	if err != nil || id <= 0 {
		httpx.Fail(w, http.StatusBadRequest, "invalid id")
		return 0, false
	}
	return id, true
}

func appOrderPagination(query url.Values) (int, int) {
	page := 1
	if n, err := strconv.Atoi(strings.TrimSpace(query.Get("page"))); err == nil && n > 0 {
		page = n
	}
	pageSize := 20
	if n, err := strconv.Atoi(strings.TrimSpace(query.Get("pageSize"))); err == nil && n > 0 {
		pageSize = n
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize
}

func formatAdminTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("2006/01/02 15:04:05")
}
