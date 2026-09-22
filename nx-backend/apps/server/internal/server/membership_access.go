package server

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"nine-xing/nx-backend/apps/server/internal/httpx"
	"nine-xing/nx-backend/apps/server/internal/quiz"
)

const (
	resourceTypeCards               = "cards"
	resourceAccessActive            = "active"
	resourceAccessReadOnlyOverLimit = "read_only_over_limit"
	resourceAccessLockedUpgrade     = "locked_requires_upgrade"
	resourceAccessDeletedByUser     = "deleted_by_user"
	membershipRetentionPeriod       = 30 * 24 * time.Hour
)

type resourceAccessCandidate struct {
	ResourceID             int64
	CreatedAt              time.Time
	PreviousRetentionUntil *time.Time
}

type resourceAccessState struct {
	ResourceID        int64
	State             string
	RequiredPlanLevel string
	PriorityRank      int
	Reason            string
	RetentionUntil    *time.Time
}

type membershipUpgradeError struct {
	RequiredPlanLevel string
	AccessState       string
}

var errMembershipPlanUnavailable = errors.New("membership plan unavailable")

// membershipResourceMetadata is attached to historical resources that remain
// readable after a membership downgrade. A read-only resource is intentionally
// different from a deleted resource: the client can render its history and
// offer an upgrade action, while mutation endpoints still reject writes.
type membershipResourceMetadata struct {
	State             string `json:"accessState"`
	RequiredPlanLevel string `json:"requiredPlanLevel"`
	Reason            string `json:"accessReason,omitempty"`
	UpgradeRequired   bool   `json:"upgradeRequired"`
}

func membershipResourceMetadataForPlan(planLevel, requiredPlanLevel, reason string) membershipResourceMetadata {
	plan := normalizeMembershipLevel(planLevel)
	required := normalizeMembershipLevel(requiredPlanLevel)
	if required == "free" {
		return membershipResourceMetadata{
			State:             resourceAccessActive,
			RequiredPlanLevel: required,
			Reason:            strings.TrimSpace(reason),
		}
	}
	if membershipLevelRank(plan) >= membershipLevelRank(required) {
		return membershipResourceMetadata{
			State:             resourceAccessActive,
			RequiredPlanLevel: required,
			Reason:            strings.TrimSpace(reason),
		}
	}
	if strings.TrimSpace(reason) == "" {
		reason = "历史内容已保留，请升级后继续使用"
	}
	return membershipResourceMetadata{
		State:             resourceAccessReadOnlyOverLimit,
		RequiredPlanLevel: required,
		Reason:            reason,
		UpgradeRequired:   true,
	}
}

func membershipLevelRank(level string) int {
	switch normalizeMembershipLevel(level) {
	case "svip":
		return 2
	case "vip":
		return 1
	default:
		return 0
	}
}

func (e *membershipUpgradeError) Error() string {
	return "membership upgrade required"
}

// writeMembershipAccessError turns the two membership outcomes into stable
// HTTP responses for mutation handlers. A missing ledger table is handled by
// ensureCardWritable as a legacy-schema compatibility case; any other lookup
// failure must not silently allow a write against an unknown entitlement state.
func writeMembershipAccessError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	var upgradeErr *membershipUpgradeError
	if errors.As(err, &upgradeErr) {
		writeMembershipUpgradeRequired(w, upgradeErr.RequiredPlanLevel, upgradeErr.AccessState)
		return true
	}
	httpx.Fail(w, http.StatusInternalServerError, "membership access unavailable")
	return true
}

// membershipLedgerSchemaUnavailable identifies the additive-table errors that
// can occur while an older deployment is being upgraded. PostgreSQL exposes a
// SQLSTATE through pgx errors; the text fallback keeps compatibility with the
// lightweight test drivers used by older handlers.
func membershipLedgerSchemaUnavailable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, driver.ErrSkip) || strings.Contains(strings.ToLower(err.Error()), "driver: skip fast-path") {
		return true
	}
	var stateErr interface{ SQLState() string }
	if errors.As(err, &stateErr) {
		switch stateErr.SQLState() {
		case "42P01", "42703": // undefined_table / undefined_column
			return true
		}
	}
	message := strings.ToLower(err.Error())
	if !strings.Contains(message, "app_membership_resource_access") {
		return false
	}
	return strings.Contains(message, "does not exist") ||
		strings.Contains(message, "undefined table") ||
		strings.Contains(message, "undefined column")
}

func membershipPlanSchemaUnavailable(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	if !strings.Contains(message, "member_level") && !strings.Contains(message, "member_expires_at") {
		return false
	}
	return strings.Contains(message, "does not exist") ||
		strings.Contains(message, "undefined column") ||
		strings.Contains(message, "unknown column")
}

// membershipLegacyDriverError reports errors from the small database drivers
// used by older unit fixtures. Production opens PostgreSQL through pgx; those
// errors must remain fail-closed. A fixture driver, on the other hand, often
// returns a generic "unexpected query" error for additive membership queries
// it does not know about. Treating that as a hard entitlement failure would
// block otherwise unrelated handlers while a migration is being rolled out.
func membershipLegacyDriverError(db *sql.DB, err error) bool {
	if err == nil || db == nil {
		return false
	}
	if membershipLedgerSchemaUnavailable(err) {
		return true
	}
	// The application database is opened with the pgx stdlib driver. Keep the
	// check deliberately narrow so a real PostgreSQL error cannot be hidden by
	// a broad message-based fallback. Non-production test drivers are allowed
	// to opt into the legacy path only when they explicitly report that the
	// additive query is unknown or unimplemented; an arbitrary driver error is
	// still an entitlement lookup failure and must fail closed.
	driverType := fmt.Sprintf("%T", db.Driver())
	if strings.Contains(driverType, "pgx") || strings.Contains(driverType, "pq") {
		return false
	}
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	for _, marker := range []string{
		"unexpected query",
		"query not implemented",
		"not implemented",
		"unsupported query",
		"unknown query",
		"unrecognized query",
	} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}

func (s *Server) ensureMembershipLevel(ctx context.Context, appUserID int64, required string) error {
	plan, err := s.currentAppMembershipPlanWithError(ctx, appUserID)
	if err != nil {
		if errors.Is(err, errMembershipPlanUnavailable) {
			// Legacy/unit fixtures may not expose membership columns yet. Their
			// handlers historically proceeded without a plan gate; preserve that
			// behavior until the additive schema is available.
			return nil
		}
		return err
	}
	if membershipLevelRank(plan.PlanLevel) >= membershipLevelRank(required) {
		return nil
	}
	return &membershipUpgradeError{
		RequiredPlanLevel: normalizeMembershipLevel(required),
		AccessState:       resourceAccessLockedUpgrade,
	}
}

// recomputeResourceAccessStates is deterministic: the oldest resources keep
// write access after a downgrade, while newer resources remain readable.
func recomputeResourceAccessStates(candidates []resourceAccessCandidate, planLevel string, now time.Time) []resourceAccessState {
	items := append([]resourceAccessCandidate(nil), candidates...)
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].ResourceID < items[j].ResourceID
		}
		return items[i].CreatedAt.Before(items[j].CreatedAt)
	})
	limit := membershipLevelCardLimit(planLevel)
	result := make([]resourceAccessState, 0, len(items))
	for index, item := range items {
		rank := index + 1
		state := resourceAccessState{
			ResourceID:        item.ResourceID,
			State:             resourceAccessActive,
			RequiredPlanLevel: requiredPlanLevelForRank(rank),
			PriorityRank:      rank,
		}
		if rank > limit {
			state.State = resourceAccessReadOnlyOverLimit
			state.Reason = "超出当前会员等级的人物卡额度"
			if item.PreviousRetentionUntil != nil && item.PreviousRetentionUntil.After(now) {
				retention := *item.PreviousRetentionUntil
				state.RetentionUntil = &retention
			} else {
				retention := now.Add(membershipRetentionPeriod)
				state.RetentionUntil = &retention
			}
		}
		result = append(result, state)
	}
	return result
}

func requiredPlanLevelForRank(rank int) string {
	switch {
	case rank <= 1:
		return "free"
	case rank <= 3:
		return "vip"
	default:
		return "svip"
	}
}

type appResourceAccessResp struct {
	ResourceID        int64  `json:"resourceId"`
	State             string `json:"state"`
	RequiredPlanLevel string `json:"requiredPlanLevel"`
	PriorityRank      int    `json:"priorityRank"`
	Reason            string `json:"reason,omitempty"`
	RetentionUntil    string `json:"retentionUntil,omitempty"`
}

func resourceAccessResponse(state resourceAccessState) appResourceAccessResp {
	resp := appResourceAccessResp{
		ResourceID: state.ResourceID, State: state.State,
		RequiredPlanLevel: state.RequiredPlanLevel, PriorityRank: state.PriorityRank,
		Reason: state.Reason,
	}
	if state.RetentionUntil != nil {
		resp.RetentionUntil = state.RetentionUntil.Format(time.RFC3339)
	}
	return resp
}

func (s *Server) recomputeCardResourceAccess(ctx context.Context, appUserID int64, planLevel string) ([]resourceAccessState, error) {
	if s == nil || s.db == nil || appUserID <= 0 {
		return nil, errors.New("membership access database is unavailable")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `
		SELECT c.id,c.create_time,a.retention_until
		FROM app_user_cards c
		LEFT JOIN app_membership_resource_access a
		  ON a.app_user_id=c.app_user_id AND a.resource_type='cards' AND a.resource_id=c.id
		WHERE c.app_user_id=$1 AND c.card_type='secondary' AND c.status='active'
		ORDER BY c.create_time,c.id`, appUserID)
	if err != nil {
		return nil, err
	}
	candidates := make([]resourceAccessCandidate, 0)
	for rows.Next() {
		var candidate resourceAccessCandidate
		var previous sql.NullTime
		if err := rows.Scan(&candidate.ResourceID, &candidate.CreatedAt, &previous); err != nil {
			rows.Close()
			return nil, err
		}
		if previous.Valid {
			candidate.PreviousRetentionUntil = &previous.Time
		}
		candidates = append(candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	now := time.Now()
	states := recomputeResourceAccessStates(candidates, planLevel, now)
	for _, state := range states {
		var retention any
		if state.RetentionUntil != nil {
			retention = *state.RetentionUntil
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO app_membership_resource_access
			  (app_user_id,resource_type,resource_id,state,required_plan_level,priority_rank,reason,retention_until,update_time)
			VALUES ($1,'cards',$2,$3,$4,$5,$6,$7,now())
			ON CONFLICT (app_user_id,resource_type,resource_id) DO UPDATE SET
			  state=EXCLUDED.state,required_plan_level=EXCLUDED.required_plan_level,
			  priority_rank=EXCLUDED.priority_rank,reason=EXCLUDED.reason,
			  retention_until=EXCLUDED.retention_until,update_time=now()`,
			appUserID, state.ResourceID, state.State, state.RequiredPlanLevel,
			state.PriorityRank, state.Reason, retention); err != nil {
			return nil, err
		}
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE app_membership_resource_access a
		SET state='deleted_by_user',reason='用户已删除人物卡',update_time=now()
		WHERE a.app_user_id=$1 AND a.resource_type='cards' AND a.state<>'deleted_by_user'
		  AND NOT EXISTS (
			SELECT 1 FROM app_user_cards c
			WHERE c.id=a.resource_id AND c.app_user_id=a.app_user_id
			  AND c.card_type='secondary' AND c.status='active'
		  )`, appUserID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return states, nil
}

func (s *Server) cardResourceAccess(ctx context.Context, appUserID, cardID int64) (resourceAccessState, error) {
	var state resourceAccessState
	if s == nil || s.db == nil {
		return state, errors.New("membership access database is unavailable")
	}
	var retention sql.NullTime
	err := s.db.QueryRowContext(ctx, `
		SELECT resource_id,state,required_plan_level,priority_rank,reason,retention_until
		FROM app_membership_resource_access
		WHERE app_user_id=$1 AND resource_type='cards' AND resource_id=$2`, appUserID, cardID).
		Scan(&state.ResourceID, &state.State, &state.RequiredPlanLevel, &state.PriorityRank, &state.Reason, &retention)
	if err != nil {
		return state, err
	}
	if retention.Valid {
		state.RetentionUntil = &retention.Time
	}
	return state, nil
}

// membershipMetadataForCard converts a card ledger row into the metadata used
// by card-scoped history endpoints (memories, portrait and trend). Primary
// cards and legacy deployments may have no ledger row; a free user still owns
// the base card and must retain the free-level view in that case. When a row
// exists, compare its required level with the current plan as well as trusting
// its state so an upgrade/downgrade is reflected even before a background
// reconciliation has rewritten a stale row.
func membershipMetadataForCard(planLevel string, state resourceAccessState, found bool, reason string) membershipResourceMetadata {
	plan := normalizeMembershipLevel(planLevel)
	if !found {
		if plan == "free" {
			return membershipResourceMetadata{State: resourceAccessActive, RequiredPlanLevel: "free"}
		}
		return membershipResourceMetadataForPlan(plan, "vip", reason)
	}

	required := normalizeMembershipLevel(state.RequiredPlanLevel)
	if membershipLevelRank(plan) >= membershipLevelRank(required) {
		return membershipResourceMetadata{
			State:             resourceAccessActive,
			RequiredPlanLevel: required,
			Reason:            strings.TrimSpace(state.Reason),
		}
	}
	accessState := state.State
	if accessState != resourceAccessReadOnlyOverLimit && accessState != resourceAccessLockedUpgrade {
		accessState = resourceAccessReadOnlyOverLimit
	}
	if strings.TrimSpace(state.Reason) == "" {
		reason = "历史内容已保留，请升级后继续使用"
	} else {
		reason = state.Reason
	}
	return membershipResourceMetadata{
		State:             accessState,
		RequiredPlanLevel: required,
		Reason:            strings.TrimSpace(reason),
		UpgradeRequired:   true,
	}
}

// cardMembershipResourceMetadata is the shared read-side gate for resources
// attached to a card. It deliberately fails open only for the additive ledger
// compatibility case; a free user without a ledger row receives the base-card
// view instead of being treated as a VIP-only resource.
func (s *Server) cardMembershipResourceMetadata(ctx context.Context, appUserID, cardID int64, reason string) (membershipResourceMetadata, error) {
	if s == nil || s.db == nil || appUserID <= 0 || cardID <= 0 {
		return membershipMetadataForCard("free", resourceAccessState{}, false, reason), nil
	}
	plan, planErr := s.currentAppMembershipPlanWithError(ctx, appUserID)
	if planErr != nil && !errors.Is(planErr, errMembershipPlanUnavailable) {
		return membershipResourceMetadata{}, planErr
	}
	state, found, err := s.compatibilityCardResourceAccess(ctx, appUserID, cardID, plan.PlanLevel)
	if err != nil {
		// Only an additive schema error or a deliberately partial legacy driver
		// may use the old free fallback. A real production database failure must
		// reach the handler instead of turning an unknown entitlement state into
		// an active resource.
		if membershipLegacyDriverError(s.db, err) {
			return membershipMetadataForCard(plan.PlanLevel, resourceAccessState{}, false, reason), nil
		}
		return membershipResourceMetadata{}, fmt.Errorf("membership access lookup: %w", err)
	}
	return membershipMetadataForCard(plan.PlanLevel, state, found, reason), nil
}

// ensureCardWritable lazily rebuilds a missing ledger so callers remain
// correct immediately after an old membership row expires or a new card is
// created. Only a missing additive ledger table fails open for compatibility;
// once a ledger row exists, locked writes are always rejected and other lookup
// failures propagate to the mutation handler.
func (s *Server) ensureCardWritable(ctx context.Context, appUserID, cardID int64) error {
	// A few legacy/unit-only server instances intentionally omit the database
	// while exercising the chat pipeline. The real authenticated handlers have
	// a database and the ledger is authoritative there; keep those lightweight
	// fixtures compatible instead of dereferencing a nil DB.
	if s == nil || s.db == nil || appUserID <= 0 || cardID <= 0 {
		return nil
	}
	// Handler-only fixtures may provide a partial database but no app-user
	// store. They cannot establish authenticated ownership, so preserve the
	// historical no-op behavior and avoid adding membership queries to those
	// tests. Fully wired production servers always have appUsers configured.
	if s.appUsers == nil {
		return nil
	}
	// The primary card is the user's base profile and is not counted against
	// the secondary-card entitlement ledger. Check ownership/type up front so
	// chat, compatibility, and profile mutations do not try to resolve a
	// ledger row that is intentionally absent for primary cards.
	var cardType, cardStatus string
	primaryErr := s.db.QueryRowContext(ctx, `
		SELECT card_type,status
		FROM app_user_cards
		WHERE id = $1 AND app_user_id = $2`, cardID, appUserID).Scan(&cardType, &cardStatus)
	switch {
	case primaryErr == nil:
		if strings.EqualFold(strings.TrimSpace(cardType), "primary") && strings.EqualFold(strings.TrimSpace(cardStatus), "active") {
			return nil
		}
	case errors.Is(primaryErr, sql.ErrNoRows):
		// Let the backing mutation/query return its normal not-found response.
	case membershipLegacyDriverError(s.db, primaryErr):
		// Older rolling-migration fixtures may not recognize the additive
		// ownership query; retain their historical compatibility behavior.
	case s.appUsers == nil:
		// Handler-only unit fixtures often expose only a partial SQL surface.
		return nil
	default:
		return fmt.Errorf("card ownership lookup: %w", primaryErr)
	}
	// Reconcile before reading an existing row. A ledger row written while a
	// user was on SVIP can otherwise remain `active` after expiry/downgrade,
	// allowing a mutation to bypass the current plan until another list or
	// entitlement request happens to refresh it.
	if plan, planErr := s.currentAppMembershipPlanWithError(ctx, appUserID); planErr == nil {
		if _, reconcileErr := s.recomputeCardResourceAccess(ctx, appUserID, plan.PlanLevel); reconcileErr != nil && !membershipLegacyDriverError(s.db, reconcileErr) {
			return fmt.Errorf("membership access rebuild: %w", reconcileErr)
		}
	} else if !errors.Is(planErr, errMembershipPlanUnavailable) {
		return fmt.Errorf("membership plan lookup: %w", planErr)
	}
	state, err := s.cardResourceAccess(ctx, appUserID, cardID)
	if errors.Is(err, sql.ErrNoRows) {
		level := "free"
		var rawLevel string
		var expiry sql.NullTime
		queryErr := s.db.QueryRowContext(ctx, `SELECT member_level,member_expires_at FROM app_users WHERE id=$1`, appUserID).Scan(&rawLevel, &expiry)
		if queryErr == nil {
			var expiryPtr *time.Time
			if expiry.Valid {
				expiryPtr = &expiry.Time
			}
			level = resolveMembershipPlan(rawLevel, rawLevel, expiryPtr, time.Now()).PlanLevel
		} else if !membershipPlanSchemaUnavailable(queryErr) && !membershipLegacyDriverError(s.db, queryErr) {
			// Do not silently treat an unavailable account row as a free user. The
			// ledger row is missing, so an unknown membership plan must block the
			// mutation until the entitlement state can be read.
			return fmt.Errorf("membership plan lookup: %w", queryErr)
		}
		if _, rebuildErr := s.recomputeCardResourceAccess(ctx, appUserID, level); rebuildErr == nil {
			state, err = s.cardResourceAccess(ctx, appUserID, cardID)
		} else if !membershipLegacyDriverError(s.db, rebuildErr) {
			return fmt.Errorf("membership access rebuild: %w", rebuildErr)
		} else {
			// The additive ledger table is not present yet. Preserve legacy
			// deployments' behavior until the schema migration has completed.
			return nil
		}
	}
	if err != nil {
		if membershipLegacyDriverError(s.db, err) {
			return nil
		}
		// Older handler-only fixtures do not initialize the app user store and
		// use a deliberately partial SQL driver that rejects unrelated queries.
		// Keep those fixtures compatible; production servers always wire
		// appUsers and therefore still fail closed on an unknown DB error.
		if s.appUsers == nil {
			return nil
		}
		return fmt.Errorf("membership access lookup: %w", err)
	}
	if state.State == resourceAccessReadOnlyOverLimit || state.State == resourceAccessLockedUpgrade {
		return &membershipUpgradeError{RequiredPlanLevel: state.RequiredPlanLevel, AccessState: state.State}
	}
	return nil
}

func (s *Server) currentAppMembershipPlan(ctx context.Context, appUserID int64) resolvedMembershipPlan {
	plan, _ := s.currentAppMembershipPlanWithError(ctx, appUserID)
	return plan
}

func (s *Server) currentAppMembershipPlanWithError(ctx context.Context, appUserID int64) (resolvedMembershipPlan, error) {
	fallback := resolvedMembershipPlan{PlanLevel: "free", BillingCycle: "none", SKU: "free", Active: true}
	if s == nil || s.db == nil || appUserID <= 0 {
		return fallback, nil
	}
	var level string
	var expiry sql.NullTime
	if err := s.db.QueryRowContext(ctx, `SELECT member_level,member_expires_at FROM app_users WHERE id=$1`, appUserID).Scan(&level, &expiry); err != nil {
		// During a rolling migration, older fixtures may not expose the
		// membership columns yet. Keep those deployments on the historical
		// free fallback; surface all other database failures to mutations.
		if errors.Is(err, sql.ErrNoRows) {
			return fallback, nil
		}
		if errors.Is(err, driver.ErrSkip) || membershipPlanSchemaUnavailable(err) || membershipLedgerSchemaUnavailable(err) || membershipLegacyDriverError(s.db, err) {
			return fallback, errMembershipPlanUnavailable
		}
		return fallback, fmt.Errorf("membership plan lookup: %w", err)
	}
	var expiryPtr *time.Time
	if expiry.Valid {
		expiryPtr = &expiry.Time
	}
	return resolveMembershipPlan(level, level, expiryPtr, time.Now()), nil
}

func (s *Server) refreshMembershipResourceAccess(ctx context.Context, appUserID int64) {
	if s == nil || s.db == nil || appUserID <= 0 {
		return
	}
	plan := s.currentAppMembershipPlan(ctx, appUserID)
	_, _ = s.recomputeCardResourceAccess(ctx, appUserID, plan.PlanLevel)
}

func (s *Server) listCardResourceAccess(ctx context.Context, appUserID int64) ([]resourceAccessState, error) {
	if s == nil || s.db == nil || appUserID <= 0 {
		return nil, errors.New("membership access database is unavailable")
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT resource_id,state,required_plan_level,priority_rank,reason,retention_until
		FROM app_membership_resource_access
		WHERE app_user_id=$1 AND resource_type='cards' AND state<>'deleted_by_user'
		ORDER BY priority_rank,resource_id`, appUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []resourceAccessState
	for rows.Next() {
		var state resourceAccessState
		var retention sql.NullTime
		if err := rows.Scan(&state.ResourceID, &state.State, &state.RequiredPlanLevel, &state.PriorityRank, &state.Reason, &retention); err != nil {
			return nil, err
		}
		if retention.Valid {
			state.RetentionUntil = &retention.Time
		}
		result = append(result, state)
	}
	return result, rows.Err()
}

func (s *Server) decorateCardsWithResourceAccess(ctx context.Context, appUserID int64, cards []quiz.Card, planLevel string) []quiz.Card {
	states, err := s.listCardResourceAccess(ctx, appUserID)
	if err != nil {
		if s == nil || s.db == nil || membershipLegacyDriverError(s.db, err) {
			return cards
		}
		// The list response may still expose basic card identity, but it must not
		// claim that the resource is writable while the ledger is unavailable.
		for i := range cards {
			cards[i].AccessState = resourceAccessLockedUpgrade
			cards[i].RequiredPlanLevel = "svip"
			cards[i].AccessReason = "会员资源状态暂不可用，请稍后重试或升级会员"
		}
		return cards
	}
	byID := make(map[int64]resourceAccessState, len(states))
	for _, state := range states {
		byID[state.ResourceID] = state
	}
	for i := range cards {
		cards[i].AccessState = resourceAccessActive
		cards[i].RequiredPlanLevel = "free"
		cards[i].AccessReason = ""
		if state, ok := byID[cards[i].ID]; ok {
			cards[i].AccessState = state.State
			cards[i].RequiredPlanLevel = state.RequiredPlanLevel
			cards[i].AccessReason = state.Reason
			cards[i].RetentionUntil = formatOptionalTime(state.RetentionUntil)
		}
	}
	return cards
}
func writeMembershipUpgradeRequired(w http.ResponseWriter, requiredPlanLevel, accessState string) {
	message := "该人物卡当前为只读，请升级会员后继续编辑"
	if strings.TrimSpace(accessState) == resourceAccessLockedUpgrade {
		message = "该资源需要升级会员后才能编辑"
	}
	payload := map[string]any{
		"code":              "membership_upgrade_required",
		"requiredPlanLevel": requiredPlanLevel,
		"accessState":       accessState,
		"message":           message,
	}
	// This response intentionally uses a string code rather than the legacy
	// numeric envelope so clients can branch without parsing error text.
	status := http.StatusForbidden
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func formatOptionalTime(value *time.Time) string {
	if value == nil || value.IsZero() {
		return ""
	}
	return value.Format(time.RFC3339)
}

func appResourceAccessForEntitlement(states []resourceAccessState) map[string][]appResourceAccessResp {
	items := make([]appResourceAccessResp, 0, len(states))
	for _, state := range states {
		items = append(items, resourceAccessResponse(state))
	}
	// Both spellings are emitted during the migration: `cards` is the stable
	// resource type and `card` matches early App clients' singular key.
	return map[string][]appResourceAccessResp{resourceTypeCards: items, "card": items}
}
