package classroom

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"io"
	"strings"
	"testing"
	"time"
)

func TestSeriesEntitlementCoversFutureLessonsButNotMovedOutLessons(t *testing.T) {
	seriesID := int64(41)
	grant := Entitlement{WXUserID: 7, SeriesID: &seriesID, Source: EntitlementPurchase}

	future := Content{ID: 52, SeriesID: &seriesID, AccessLevel: AccessInherit}
	if !EntitlementCoversContent(grant, future) {
		t.Fatal("a series purchase must cover lessons added after purchase")
	}

	otherSeries := int64(99)
	moved := future
	moved.SeriesID = &otherSeries
	if EntitlementCoversContent(grant, moved) {
		t.Fatal("a lesson moved out of the purchased series must no longer be covered")
	}
}

func TestContentEntitlementAndActiveMembershipAccess(t *testing.T) {
	contentID := int64(52)
	grant := Entitlement{WXUserID: 7, ContentID: &contentID, Source: EntitlementManual}
	content := Content{ID: contentID, AccessLevel: AccessPaid}
	if !EntitlementCoversContent(grant, content) {
		t.Fatal("manual content grant must cover its lesson")
	}
	if !CanAccessWithEntitlements(AccessMember, true, true, false) {
		t.Fatal("active membership must satisfy member access")
	}
	if CanAccessWithEntitlements(AccessPaid, true, true, false) {
		t.Fatal("membership must not silently unlock individually paid lessons")
	}
	if !CanAccessWithEntitlements(AccessPaid, true, false, true) {
		t.Fatal("target entitlement must unlock paid lesson")
	}
	if CanAccessWithEntitlements(AccessLogin, false, false, false) {
		t.Fatal("login-gated lesson requires an authenticated user")
	}
}

func TestManualGrantRequiresManualSourceAndCreator(t *testing.T) {
	contentID := int64(52)
	creatorID := int64(3)
	valid := Entitlement{WXUserID: 7, ContentID: &contentID, Source: EntitlementManual, CreatedBy: &creatorID}
	if err := ValidateManualGrant(valid); err != nil {
		t.Fatalf("valid manual grant rejected: %v", err)
	}
	invalid := valid
	invalid.Source = EntitlementPurchase
	if err := ValidateManualGrant(invalid); err == nil {
		t.Fatal("manual grant must not masquerade as a purchase")
	}
	invalid = valid
	invalid.CreatedBy = nil
	if err := ValidateManualGrant(invalid); err == nil {
		t.Fatal("manual grant requires an operator for audit")
	}
}

func TestGrantManualPersistsEntitlementAndAuditTogether(t *testing.T) {
	state := &entitlementTestState{}
	db := openEntitlementTestDB(t, state)
	store := NewStore(db)
	contentID, creatorID := int64(52), int64(3)
	grant := Entitlement{WXUserID: 7, ContentID: &contentID, Source: EntitlementManual, CreatedBy: &creatorID}
	created, err := store.GrantManual(context.Background(), grant, "客服补发")
	if err != nil || created.ID != 91 {
		t.Fatalf("manual grant failed: created=%+v err=%v", created, err)
	}
	if state.entitlementInserts != 1 || state.auditInserts != 1 || state.commits != 1 {
		t.Fatalf("entitlement and audit must commit together: %+v", state)
	}
}

func TestRevokeEntitlementPersistsReasonedAudit(t *testing.T) {
	state := &entitlementTestState{}
	db := openEntitlementTestDB(t, state)
	store := NewStore(db)
	changed, err := store.RevokeEntitlement(context.Background(), 91, 3, "误发撤销")
	if err != nil || !changed {
		t.Fatalf("revoke failed: changed=%v err=%v", changed, err)
	}
	if state.revokeUpdates != 1 || state.auditInserts != 1 || state.commits != 1 {
		t.Fatalf("revoke and audit must commit together: %+v", state)
	}
}

type entitlementTestState struct{ entitlementInserts, revokeUpdates, auditInserts, commits int }

type entitlementTestDriver struct{ state *entitlementTestState }
type entitlementTestConn struct{ state *entitlementTestState }
type entitlementTestTx struct{ state *entitlementTestState }
type entitlementTestRows struct {
	kind string
	sent bool
}

func openEntitlementTestDB(t *testing.T, state *entitlementTestState) *sql.DB {
	t.Helper()
	name := "classroom_entitlement_" + time.Now().Format("150405.000000000")
	sql.Register(name, entitlementTestDriver{state: state})
	db, err := sql.Open(name, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func (d entitlementTestDriver) Open(string) (driver.Conn, error) {
	return &entitlementTestConn{state: d.state}, nil
}
func (c *entitlementTestConn) Prepare(string) (driver.Stmt, error) { return nil, driver.ErrSkip }
func (c *entitlementTestConn) Close() error                        { return nil }
func (c *entitlementTestConn) Begin() (driver.Tx, error) {
	return &entitlementTestTx{state: c.state}, nil
}
func (c *entitlementTestConn) BeginTx(context.Context, driver.TxOptions) (driver.Tx, error) {
	return c.Begin()
}
func (c *entitlementTestConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	if strings.Contains(query, "INSERT INTO classroom_entitlements") {
		c.state.entitlementInserts++
		return &entitlementTestRows{kind: "insert"}, nil
	}
	if strings.Contains(query, "FROM classroom_entitlements") {
		return &entitlementTestRows{kind: "select"}, nil
	}
	return &entitlementTestRows{kind: "empty"}, nil
}
func (c *entitlementTestConn) ExecContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Result, error) {
	if strings.Contains(query, "UPDATE classroom_entitlements") {
		c.state.revokeUpdates++
	}
	if strings.Contains(query, "INSERT INTO admin_operation_logs") {
		c.state.auditInserts++
	}
	return driver.RowsAffected(1), nil
}
func (tx *entitlementTestTx) Commit() error   { tx.state.commits++; return nil }
func (tx *entitlementTestTx) Rollback() error { return nil }
func (r *entitlementTestRows) Columns() []string {
	if r.kind == "select" {
		return []string{"wx_user_id", "series_id", "content_id", "revoked_at"}
	}
	return []string{"id", "created_at", "updated_at"}
}
func (r *entitlementTestRows) Close() error { return nil }
func (r *entitlementTestRows) Next(dest []driver.Value) error {
	if r.sent {
		return io.EOF
	}
	if r.kind == "empty" {
		return io.EOF
	}
	if r.kind == "select" {
		dest[0], dest[1], dest[2], dest[3] = int64(7), nil, int64(52), nil
		r.sent = true
		return nil
	}
	now := time.Now()
	dest[0], dest[1], dest[2] = int64(91), now, now
	r.sent = true
	return nil
}
