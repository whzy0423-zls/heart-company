package server

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"nine-xing/nx-backend/apps/server/internal/auditlog"
	"nine-xing/nx-backend/apps/server/internal/httpx"
)

type appTrialCreditGrantInput struct {
	Amount         int    `json:"amount"`
	Reason         string `json:"reason"`
	ExpiresAt      string `json:"expiresAt"`
	IdempotencyKey string `json:"idempotencyKey"`
}

type normalizedAppTrialCreditGrantInput struct {
	Amount         int
	Reason         string
	ExpiresAt      time.Time
	IdempotencyKey string
}

type adminAppTrialCreditGrant struct {
	ID             int64  `json:"id"`
	AppUserID      int64  `json:"appUserId"`
	Amount         int    `json:"amount"`
	Remaining      int    `json:"remaining"`
	Reserved       int    `json:"reserved"`
	Reason         string `json:"reason"`
	ExpiresAt      string `json:"expiresAt"`
	Status         string `json:"status"`
	OperatorID     int64  `json:"operatorId,omitempty"`
	OperatorName   string `json:"operatorName,omitempty"`
	RevokedBy      int64  `json:"revokedBy,omitempty"`
	RevokedByName  string `json:"revokedByName,omitempty"`
	RevokedAt      string `json:"revokedAt,omitempty"`
	CreateTime     string `json:"createTime"`
	UpdateTime     string `json:"updateTime"`
	AlreadyGranted bool   `json:"alreadyGranted,omitempty"`
	RevokedAmount  int    `json:"revokedAmount,omitempty"`
}

type adminAppTrialCreditList struct {
	TrialChatRemaining        int                        `json:"trialChatRemaining"`
	TrialChatNearestExpiresAt string                     `json:"trialChatNearestExpiresAt,omitempty"`
	Items                     []adminAppTrialCreditGrant `json:"items"`
}

func normalizeAppTrialCreditGrantInput(input appTrialCreditGrantInput, now time.Time) (normalizedAppTrialCreditGrantInput, error) {
	input.Reason = strings.TrimSpace(input.Reason)
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	if input.Amount < 1 || input.Amount > 1000 {
		return normalizedAppTrialCreditGrantInput{}, errors.New("赠送次数必须在 1 到 1000 之间")
	}
	if input.Reason == "" || utf8.RuneCountInString(input.Reason) > 200 {
		return normalizedAppTrialCreditGrantInput{}, errors.New("赠送原因必填且不能超过 200 字")
	}
	if input.IdempotencyKey == "" || len(input.IdempotencyKey) > 128 {
		return normalizedAppTrialCreditGrantInput{}, errors.New("幂等标识不正确")
	}
	expiresAt := now.Add(72 * time.Hour)
	if strings.TrimSpace(input.ExpiresAt) != "" {
		parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(input.ExpiresAt))
		if err != nil {
			return normalizedAppTrialCreditGrantInput{}, errors.New("到期时间格式不正确")
		}
		expiresAt = parsed
	}
	if !expiresAt.After(now) {
		return normalizedAppTrialCreditGrantInput{}, errors.New("到期时间必须晚于当前时间")
	}
	return normalizedAppTrialCreditGrantInput{
		Amount:         input.Amount,
		Reason:         input.Reason,
		ExpiresAt:      expiresAt,
		IdempotencyKey: input.IdempotencyKey,
	}, nil
}

func parseAppTrialCreditPath(path string) (userID, grantID int64, action string, err error) {
	relative := strings.Trim(strings.TrimPrefix(path, "/api/app-users/"), "/")
	parts := strings.Split(relative, "/")
	if len(parts) < 2 || parts[1] != "trial-chat-credits" {
		return 0, 0, "", errors.New("invalid trial credit path")
	}
	userID, err = strconv.ParseInt(parts[0], 10, 64)
	if err != nil || userID <= 0 {
		return 0, 0, "", errors.New("invalid app user id")
	}
	if len(parts) == 2 {
		return userID, 0, "collection", nil
	}
	if len(parts) != 4 || parts[3] != "revoke" {
		return 0, 0, "", errors.New("invalid trial credit action")
	}
	grantID, err = strconv.ParseInt(parts[2], 10, 64)
	if err != nil || grantID <= 0 {
		return 0, 0, "", errors.New("invalid trial credit id")
	}
	return userID, grantID, "revoke", nil
}

func (s *Server) adminAppTrialCredits(w http.ResponseWriter, r *http.Request) {
	userID, grantID, action, err := parseAppTrialCreditPath(r.URL.Path)
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	if action == "revoke" {
		if r.Method != http.MethodPost {
			httpx.Fail(w, http.StatusMethodNotAllowed, "Method Not Allowed")
			return
		}
		s.adminAppTrialCreditRevoke(w, r, userID, grantID)
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.adminAppTrialCreditList(w, r, userID)
	case http.MethodPost:
		s.adminAppTrialCreditGrant(w, r, userID)
	default:
		httpx.Fail(w, http.StatusMethodNotAllowed, "Method Not Allowed")
	}
}

func (s *Server) adminAppTrialCreditGrant(w http.ResponseWriter, r *http.Request, userID int64) {
	var body appTrialCreditGrantInput
	if err := json.NewDecoder(io.LimitReader(r.Body, 8<<10)).Decode(&body); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "请求内容不正确")
		return
	}
	now := time.Now()
	input, err := normalizeAppTrialCreditGrantInput(body, now)
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	operatorID := userFromRequest(r).ID
	tx, err := s.db.BeginTx(r.Context(), &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "赠送试用额度失败")
		return
	}
	defer tx.Rollback()
	var userStatus string
	if err := tx.QueryRowContext(r.Context(), `SELECT status FROM app_users WHERE id=$1 FOR UPDATE`, userID).Scan(&userStatus); errors.Is(err, sql.ErrNoRows) {
		httpx.Fail(w, http.StatusNotFound, "App 用户不存在")
		return
	} else if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "读取 App 用户失败")
		return
	}
	if userStatus != "active" {
		httpx.Fail(w, http.StatusConflict, "已禁用用户不能赠送试用额度")
		return
	}
	operatorValue := any(nil)
	if operatorID > 0 {
		operatorValue = operatorID
	}
	grant := adminAppTrialCreditGrant{}
	var expiresAt, createTime, updateTime time.Time
	err = tx.QueryRowContext(r.Context(), `INSERT INTO app_chat_trial_credit_grants
		(app_user_id,amount,remaining,reserved,reason,expires_at,status,operator_id,idempotency_key)
		VALUES($1,$2,$2,0,$3,$4,'active',$5,$6)
		ON CONFLICT(idempotency_key) DO NOTHING
		RETURNING id,app_user_id,amount,remaining,reserved,reason,expires_at,status,COALESCE(operator_id,0),create_time,update_time`,
		userID, input.Amount, input.Reason, input.ExpiresAt, operatorValue, input.IdempotencyKey,
	).Scan(&grant.ID, &grant.AppUserID, &grant.Amount, &grant.Remaining, &grant.Reserved, &grant.Reason, &expiresAt, &grant.Status, &grant.OperatorID, &createTime, &updateTime)
	if errors.Is(err, sql.ErrNoRows) {
		err = tx.QueryRowContext(r.Context(), `SELECT id,app_user_id,amount,remaining,reserved,reason,expires_at,status,COALESCE(operator_id,0),create_time,update_time
			FROM app_chat_trial_credit_grants WHERE idempotency_key=$1`, input.IdempotencyKey).Scan(
			&grant.ID, &grant.AppUserID, &grant.Amount, &grant.Remaining, &grant.Reserved, &grant.Reason, &expiresAt, &grant.Status, &grant.OperatorID, &createTime, &updateTime,
		)
		expiryMismatch := strings.TrimSpace(body.ExpiresAt) != "" && !expiresAt.Equal(input.ExpiresAt)
		if err == nil && (grant.AppUserID != userID || grant.Amount != input.Amount || grant.Reason != input.Reason || expiryMismatch) {
			httpx.Fail(w, http.StatusConflict, "该提交标识已用于其他赠送请求")
			return
		}
		grant.AlreadyGranted = err == nil
	}
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "赠送试用额度失败")
		return
	}
	if err := tx.Commit(); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "赠送试用额度失败")
		return
	}
	grant.ExpiresAt = expiresAt.Format(time.RFC3339)
	grant.CreateTime = createTime.Format(time.RFC3339)
	grant.UpdateTime = updateTime.Format(time.RFC3339)
	if !grant.AlreadyGranted {
		s.recordAdminAudit(r, auditlog.Entry{Action: "app_trial_credit.grant", TargetType: "app_user", TargetID: strconv.FormatInt(userID, 10), After: grant, Summary: "赠送 App 聊天试用额度"})
	}
	httpx.OK(w, grant)
}

func (s *Server) adminAppTrialCreditList(w http.ResponseWriter, r *http.Request, userID int64) {
	var exists bool
	if err := s.db.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM app_users WHERE id=$1)`, userID).Scan(&exists); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "读取 App 用户失败")
		return
	}
	if !exists {
		httpx.Fail(w, http.StatusNotFound, "App 用户不存在")
		return
	}
	now := time.Now()
	result := adminAppTrialCreditList{Items: []adminAppTrialCreditGrant{}}
	var nearest sql.NullTime
	if err := s.db.QueryRowContext(r.Context(), `SELECT COALESCE(SUM(remaining-reserved),0),MIN(expires_at)
		FROM app_chat_trial_credit_grants WHERE app_user_id=$1 AND status='active' AND expires_at>$2 AND remaining>reserved`, userID, now).Scan(&result.TrialChatRemaining, &nearest); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "读取试用额度失败")
		return
	}
	if nearest.Valid {
		result.TrialChatNearestExpiresAt = nearest.Time.Format(time.RFC3339)
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT g.id,g.app_user_id,g.amount,g.remaining,g.reserved,g.reason,g.expires_at,
		CASE WHEN g.status='active' AND g.expires_at<=$2 THEN 'expired' ELSE g.status END,
		COALESCE(g.operator_id,0),COALESCE(op.nickname,op.username,''),COALESCE(g.revoked_by,0),COALESCE(rev.nickname,rev.username,''),
		g.revoked_at,g.create_time,g.update_time
		FROM app_chat_trial_credit_grants g
		LEFT JOIN users op ON op.id=g.operator_id LEFT JOIN users rev ON rev.id=g.revoked_by
		WHERE g.app_user_id=$1 ORDER BY g.create_time DESC,g.id DESC LIMIT 100`, userID, now)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "读取试用额度记录失败")
		return
	}
	defer rows.Close()
	for rows.Next() {
		var item adminAppTrialCreditGrant
		var expiresAt, createTime, updateTime time.Time
		var revokedAt sql.NullTime
		if err := rows.Scan(&item.ID, &item.AppUserID, &item.Amount, &item.Remaining, &item.Reserved, &item.Reason, &expiresAt,
			&item.Status, &item.OperatorID, &item.OperatorName, &item.RevokedBy, &item.RevokedByName, &revokedAt, &createTime, &updateTime); err != nil {
			httpx.Fail(w, http.StatusInternalServerError, "读取试用额度记录失败")
			return
		}
		item.ExpiresAt = expiresAt.Format(time.RFC3339)
		item.CreateTime = createTime.Format(time.RFC3339)
		item.UpdateTime = updateTime.Format(time.RFC3339)
		if revokedAt.Valid {
			item.RevokedAt = revokedAt.Time.Format(time.RFC3339)
		}
		result.Items = append(result.Items, item)
	}
	if err := rows.Err(); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "读取试用额度记录失败")
		return
	}
	httpx.OK(w, result)
}

func (s *Server) adminAppTrialCreditRevoke(w http.ResponseWriter, r *http.Request, userID, grantID int64) {
	operatorID := userFromRequest(r).ID
	tx, err := s.db.BeginTx(r.Context(), &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "撤销试用额度失败")
		return
	}
	defer tx.Rollback()
	var item adminAppTrialCreditGrant
	var expiresAt, createTime, updateTime time.Time
	if err := tx.QueryRowContext(r.Context(), `SELECT id,app_user_id,amount,remaining,reserved,reason,expires_at,status,COALESCE(operator_id,0),create_time,update_time
		FROM app_chat_trial_credit_grants WHERE id=$1 AND app_user_id=$2 FOR UPDATE`, grantID, userID).Scan(
		&item.ID, &item.AppUserID, &item.Amount, &item.Remaining, &item.Reserved, &item.Reason, &expiresAt, &item.Status, &item.OperatorID, &createTime, &updateTime,
	); errors.Is(err, sql.ErrNoRows) {
		httpx.Fail(w, http.StatusNotFound, "试用额度记录不存在")
		return
	} else if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "读取试用额度记录失败")
		return
	}
	if item.Status != "active" || !expiresAt.After(time.Now()) {
		httpx.Fail(w, http.StatusConflict, "该试用额度已失效，不能撤销")
		return
	}
	item.RevokedAmount = max(item.Remaining-item.Reserved, 0)
	operatorValue := any(nil)
	if operatorID > 0 {
		operatorValue = operatorID
	}
	if _, err := tx.ExecContext(r.Context(), `UPDATE app_chat_trial_credit_grants
		SET remaining=reserved,status='revoked',revoked_by=$2,revoked_at=now(),update_time=now() WHERE id=$1`, grantID, operatorValue); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "撤销试用额度失败")
		return
	}
	if err := tx.Commit(); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "撤销试用额度失败")
		return
	}
	item.Remaining = item.Reserved
	item.Status = "revoked"
	item.RevokedBy = operatorID
	item.RevokedAt = time.Now().Format(time.RFC3339)
	item.ExpiresAt = expiresAt.Format(time.RFC3339)
	item.CreateTime = createTime.Format(time.RFC3339)
	item.UpdateTime = time.Now().Format(time.RFC3339)
	s.recordAdminAudit(r, auditlog.Entry{Action: "app_trial_credit.revoke", TargetType: "app_trial_credit", TargetID: strconv.FormatInt(grantID, 10), Before: map[string]any{"remaining": item.Remaining + item.RevokedAmount, "reserved": item.Reserved}, After: item, Summary: "撤销 App 聊天试用额度"})
	httpx.OK(w, item)
}

func appTrialCreditRoute(path string) bool {
	return strings.Contains(path, "/trial-chat-credits")
}

func appTrialCreditPermission(method string) (string, error) {
	switch method {
	case http.MethodGet:
		return "Customer:App:List", nil
	case http.MethodPost:
		return "Customer:AppTrialCredit:Grant", nil
	default:
		return "", fmt.Errorf("unsupported method %s", method)
	}
}
