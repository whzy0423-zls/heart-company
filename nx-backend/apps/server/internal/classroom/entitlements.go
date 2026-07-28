package classroom

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// EntitlementCoversContent resolves grants against the lesson's current
// placement. A series grant therefore includes lessons added later, while a
// lesson moved to another series stops inheriting the old grant.
func EntitlementCoversContent(grant Entitlement, content Content) bool {
	if grant.RevokedAt != nil {
		return false
	}
	if grant.ContentID != nil {
		return *grant.ContentID == content.ID
	}
	return grant.SeriesID != nil && content.SeriesID != nil && *grant.SeriesID == *content.SeriesID
}

// RevokeEntitlement records both the revocation and its reasoned audit in one
// transaction. Repeating the operation is idempotent.
func (s *Store) RevokeEntitlement(ctx context.Context, entitlementID, operatorID int64, reason string) (bool, error) {
	if entitlementID <= 0 || operatorID <= 0 || strings.TrimSpace(reason) == "" {
		return false, errors.New("entitlement, operator and reason are required")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	var wxUserID int64
	var seriesID, contentID sql.NullInt64
	var revokedAt *time.Time
	if err = tx.QueryRowContext(ctx, `SELECT wx_user_id,series_id,content_id,revoked_at FROM classroom_entitlements WHERE id=$1 FOR UPDATE`, entitlementID).
		Scan(&wxUserID, &seriesID, &contentID, &revokedAt); err != nil {
		return false, err
	}
	if revokedAt != nil {
		return false, nil
	}
	if _, err = tx.ExecContext(ctx, `UPDATE classroom_entitlements SET revoked_at=now(),updated_at=now() WHERE id=$1 AND revoked_at IS NULL`, entitlementID); err != nil {
		return false, err
	}
	targetType, targetID := "content", contentID.Int64
	if seriesID.Valid {
		targetType, targetID = "series", seriesID.Int64
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO admin_operation_logs
		(operator_id,operator_name,action,target_type,target_id,before_data,after_data,summary)
		VALUES ($1,'admin','classroom_entitlement_revoke',$2,$3,$4::jsonb,$5::jsonb,$6)`,
		operatorID, "classroom_"+targetType, strconv.FormatInt(targetID, 10),
		fmt.Sprintf(`{"entitlementId":%d,"wxUserId":%d,"revoked":false}`, entitlementID, wxUserID),
		fmt.Sprintf(`{"entitlementId":%d,"wxUserId":%d,"revoked":true}`, entitlementID, wxUserID), strings.TrimSpace(reason),
	); err != nil {
		return false, err
	}
	if err = tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

// CanAccessWithEntitlements keeps membership and one-off purchase semantics
// separate: membership only satisfies member-gated content.
func CanAccessWithEntitlements(access AccessLevel, loggedIn, member, entitled bool) bool {
	switch access {
	case AccessPublic:
		return true
	case AccessLogin:
		return loggedIn
	case AccessMember:
		return member
	case AccessPaid:
		return entitled
	default:
		return false
	}
}

func ValidateManualGrant(grant Entitlement) error {
	if err := grant.Validate(); err != nil {
		return err
	}
	if grant.Source != EntitlementManual {
		return errors.New("manual grant must use manual source")
	}
	if grant.CreatedBy == nil || *grant.CreatedBy <= 0 {
		return errors.New("manual grant operator is required")
	}
	return nil
}

// GrantManual persists the grant and its operator audit in one transaction.
func (s *Store) GrantManual(ctx context.Context, grant Entitlement, summary string) (Entitlement, error) {
	if err := ValidateManualGrant(grant); err != nil {
		return Entitlement{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Entitlement{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if err = tx.QueryRowContext(ctx, `INSERT INTO classroom_entitlements
		(wx_user_id,series_id,content_id,order_id,source,expires_at,revoked_at,created_by)
		VALUES ($1,$2,$3,NULL,'manual',$4,NULL,$5) RETURNING id,created_at,updated_at`,
		grant.WXUserID, grant.SeriesID, grant.ContentID, grant.ExpiresAt, grant.CreatedBy,
	).Scan(&grant.ID, &grant.CreatedAt, &grant.UpdatedAt); err != nil {
		return Entitlement{}, fmt.Errorf("create manual classroom entitlement: %w", err)
	}
	targetType, targetID := "content", int64(0)
	if grant.SeriesID != nil {
		targetType, targetID = "series", *grant.SeriesID
	} else {
		targetID = *grant.ContentID
	}
	summary = strings.TrimSpace(summary)
	if summary == "" {
		summary = "manual classroom entitlement granted"
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO admin_operation_logs
		(operator_id,operator_name,action,target_type,target_id,before_data,after_data,summary)
		VALUES ($1,'admin','classroom_entitlement_grant',$2,$3,'{}'::jsonb,$4::jsonb,$5)`,
		*grant.CreatedBy, "classroom_"+targetType, strconv.FormatInt(targetID, 10),
		fmt.Sprintf(`{"entitlementId":%d,"wxUserId":%d,"source":"manual"}`, grant.ID, grant.WXUserID), summary,
	); err != nil {
		return Entitlement{}, fmt.Errorf("audit manual classroom entitlement: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return Entitlement{}, err
	}
	return grant, nil
}
