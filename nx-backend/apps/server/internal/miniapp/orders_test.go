package miniapp

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestCreateOrReusePendingOrderReturnsExistingPendingOrder(t *testing.T) {
	state := &orderTestState{existing: true}
	store := newOrderTestStore(t, state)

	order, err := store.CreateOrReusePendingOrder(context.Background(), 7, "new-order", "report", 42, "报告", 990)
	if err != nil {
		t.Fatal(err)
	}
	if order.OutTradeNo != "existing-order" {
		t.Fatalf("expected existing pending order to be reused, got %q", order.OutTradeNo)
	}
	if state.insertCount != 0 {
		t.Fatalf("expected no insert for existing pending order, got %d", state.insertCount)
	}
	if state.lockCount != 1 {
		t.Fatalf("expected advisory lock before lookup, got %d", state.lockCount)
	}
}

func TestCreateOrReusePendingOrderCreatesWhenNoPendingOrder(t *testing.T) {
	state := &orderTestState{}
	store := newOrderTestStore(t, state)

	order, err := store.CreateOrReusePendingOrder(context.Background(), 7, "new-order", "report", 42, "报告", 990)
	if err != nil {
		t.Fatal(err)
	}
	if order.OutTradeNo != "new-order" {
		t.Fatalf("expected newly generated order number, got %q", order.OutTradeNo)
	}
	if state.insertCount != 1 {
		t.Fatalf("expected one insert, got %d", state.insertCount)
	}
	if state.lockCount != 1 {
		t.Fatalf("expected advisory lock before insert, got %d", state.lockCount)
	}
}

func TestCreateOrReusePendingOrderReturnsSnapshotConflictWithoutLocalClose(t *testing.T) {
	state := &orderTestState{
		existing:       true,
		existingAmount: 1,
		existingTitle:  "旧报告",
	}
	store := newOrderTestStore(t, state)

	order, err := store.CreateOrReusePendingOrder(context.Background(), 7, "new-order", "report", 42, "报告", 990)
	if !errors.Is(err, ErrPendingOrderSnapshotChanged) || order.OutTradeNo != "existing-order" {
		t.Fatalf("expected existing order snapshot conflict, order=%+v err=%v", order, err)
	}
	if state.closeCount != 0 || state.insertCount != 0 {
		t.Fatalf("database must not close before remote WeChat close succeeds: closes=%d inserts=%d", state.closeCount, state.insertCount)
	}
	if state.lockCount != 1 {
		t.Fatalf("expected advisory lock before lookup, got %d", state.lockCount)
	}
}

func TestReplacePendingOrderRechecksEntitlementUnderTargetLock(t *testing.T) {
	state := &orderTestState{existing: true, existingAmount: 2990, existingTitle: "基础系列", targetOwned: true}
	store := newOrderTestStore(t, state)
	_, err := store.ReplacePendingOrder(context.Background(), 7, "replacement", ProductClassroomSeries, 41, "基础系列", 3990, "existing-order")
	if !errors.Is(err, ErrOrderAlreadyOwned) {
		t.Fatalf("payment race must stop replacement after entitlement appears: %v", err)
	}
	if state.closeCount != 0 || state.insertCount != 0 || state.lockCount != 1 {
		t.Fatalf("replacement changed state after entitlement grant: %+v", state)
	}
}

func TestReplacePendingOrderClosesLocalOldOrderAfterRemoteSuccess(t *testing.T) {
	state := &orderTestState{paidProduct: ProductReport, paidRefID: 42, paidStatus: "pending"}
	store := newOrderTestStore(t, state)
	order, err := store.ReplacePendingOrder(context.Background(), 7, "replacement", ProductReport, 42, "报告", 1990, "existing-order")
	if err != nil || order.OutTradeNo != "replacement" {
		t.Fatalf("replacement failed: order=%+v err=%v", order, err)
	}
	if state.closeCount != 1 || state.insertCount != 1 || state.lockCount != 1 {
		t.Fatalf("local replacement did not commit expected transition: %+v", state)
	}
}

func TestCreatePendingClassroomOrderRechecksEntitlementUnderTargetLock(t *testing.T) {
	state := &orderTestState{targetOwned: true}
	store := newOrderTestStore(t, state)
	_, err := store.CreateOrReusePendingOrder(context.Background(), 7, "new-order", ProductClassroomContent, 52, "第一课", 990)
	if !errors.Is(err, ErrOrderAlreadyOwned) {
		t.Fatalf("creation must stop when payment granted access before target lock: %v", err)
	}
	if state.lockCount != 1 || state.insertCount != 0 {
		t.Fatalf("owned target created another payable order: %+v", state)
	}
}

func TestMarkOrderPaidStartsDatedMembership(t *testing.T) {
	state := &orderTestState{paidProduct: "member", paidRefID: 30}
	store := newOrderTestStore(t, state)

	changed, err := store.MarkOrderPaid(context.Background(), "member-order", "wx-transaction")
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected first callback to grant membership")
	}
	if state.membershipGrantCount != 1 || state.membershipDurationDays != 30 {
		t.Fatalf("expected one 30-day membership grant, got count=%d days=%d", state.membershipGrantCount, state.membershipDurationDays)
	}
	if !state.membershipRenewsFromCurrentExpiry {
		t.Fatal("expected membership renewal to extend from a current future expiry")
	}
}

func TestMarkOrderPaidDuplicateCallbackIsIdempotent(t *testing.T) {
	state := &orderTestState{paidProduct: "member", paidRefID: 30, paidStatus: "paid"}
	store := newOrderTestStore(t, state)

	changed, err := store.MarkOrderPaid(context.Background(), "member-order", "wx-transaction")
	if err != nil {
		t.Fatal(err)
	}
	if changed || state.membershipGrantCount != 0 {
		t.Fatalf("expected duplicate callback to be a no-op, changed=%v grants=%d", changed, state.membershipGrantCount)
	}
}

func TestMarkOrderPaidRejectsTerminalUnpaidOrders(t *testing.T) {
	for _, status := range []string{"refunded", "closed"} {
		t.Run(status, func(t *testing.T) {
			state := &orderTestState{paidProduct: "member", paidRefID: 30, paidStatus: status}
			store := newOrderTestStore(t, state)
			changed, err := store.MarkOrderPaid(context.Background(), "member-order", "late-wx-transaction")
			if err == nil || changed {
				t.Fatalf("terminal %s order must reject delayed callback, changed=%v err=%v", status, changed, err)
			}
			if state.orderPaidCount != 0 || state.membershipGrantCount != 0 || state.commitCount != 0 {
				t.Fatalf("terminal order changed state: paid=%d grants=%d commits=%d", state.orderPaidCount, state.membershipGrantCount, state.commitCount)
			}
		})
	}
}

func TestMarkOrderPaidRejectsMembershipWithoutDuration(t *testing.T) {
	state := &orderTestState{paidProduct: "member"}
	store := newOrderTestStore(t, state)

	if _, err := store.MarkOrderPaid(context.Background(), "member-order", "wx-transaction"); err == nil {
		t.Fatal("expected a new membership order without an explicit duration to fail")
	}
	if state.membershipGrantCount != 0 || state.commitCount != 0 || state.rollbackCount != 1 {
		t.Fatalf("invalid membership must rollback without commit, grants=%d commits=%d rollbacks=%d", state.membershipGrantCount, state.commitCount, state.rollbackCount)
	}
}

func TestMarkOrderPaidIssuesClassroomTargetEntitlementExactlyOnce(t *testing.T) {
	for _, tt := range []struct {
		product string
		refID   int64
	}{
		{product: ProductClassroomSeries, refID: 41},
		{product: ProductClassroomContent, refID: 52},
	} {
		t.Run(tt.product, func(t *testing.T) {
			state := &orderTestState{paidProduct: tt.product, paidRefID: tt.refID}
			store := newOrderTestStore(t, state)
			changed, err := store.MarkOrderPaid(context.Background(), "classroom-order", "wx-transaction")
			if err != nil || !changed {
				t.Fatalf("first callback changed=%v err=%v", changed, err)
			}
			if state.classroomGrantCount != 1 || state.classroomGrantProduct != tt.product || state.classroomGrantRefID != tt.refID {
				t.Fatalf("unexpected grant state: %+v", state)
			}
		})
	}
}

func TestMarkOrderPaidDuplicateClassroomCallbackIsIdempotent(t *testing.T) {
	state := &orderTestState{paidProduct: ProductClassroomContent, paidRefID: 52, paidStatus: "paid"}
	store := newOrderTestStore(t, state)
	changed, err := store.MarkOrderPaid(context.Background(), "classroom-order", "duplicate-wx-transaction")
	if err != nil || changed || state.classroomGrantCount != 0 || state.orderPaidCount != 0 {
		t.Fatalf("duplicate callback changed entitlement: changed=%v state=%+v err=%v", changed, state, err)
	}
}

func TestMarkOrderPaidAcceptsLateSuccessForLocallyClosedClassroomOrder(t *testing.T) {
	state := &orderTestState{paidProduct: ProductClassroomSeries, paidRefID: 41, paidStatus: "closed", siblingPending: "replacement-order"}
	store := newOrderTestStore(t, state)
	result, err := store.MarkOrderPaidDetailed(context.Background(), "old-order", "late-wx-transaction")
	if err != nil || !result.Changed || !result.LateSuccess {
		t.Fatalf("late real payment must be compensated, result=%+v err=%v", result, err)
	}
	if state.classroomGrantCount != 1 || state.siblingCloseCount != 0 || len(result.PendingToClose) != 1 || result.PendingToClose[0] != "replacement-order" {
		t.Fatalf("late payment must grant and return replacement for remote close: result=%+v state=%+v", result, state)
	}
	if err := store.ClosePendingOrders(context.Background(), result.PendingToClose); err != nil {
		t.Fatal(err)
	}
	if state.siblingCloseCount != 1 {
		t.Fatalf("local pending must close only after remote success: %+v", state)
	}
}

func TestMarkOrderPaidTakesTargetLockBeforeOrderRowLock(t *testing.T) {
	state := &orderTestState{paidProduct: ProductClassroomContent, paidRefID: 52}
	store := newOrderTestStore(t, state)
	if _, err := store.MarkOrderPaid(context.Background(), "classroom-order", "wx"); err != nil {
		t.Fatal(err)
	}
	if strings.Join(state.operationOrder, ",") != "target-lock,order-row-lock" {
		t.Fatalf("unexpected lock order: %v", state.operationOrder)
	}
}

func TestRefundClassroomOrderRevokesOnlyItsEntitlementAndWritesAudit(t *testing.T) {
	state := &orderTestState{paidProduct: ProductClassroomSeries, paidRefID: 41, paidStatus: "paid"}
	store := newOrderTestStore(t, state)
	changed, err := store.RefundClassroomOrder(context.Background(), "classroom-order", "user refund")
	if err != nil || !changed {
		t.Fatalf("refund changed=%v err=%v", changed, err)
	}
	if state.refundCount != 1 || state.classroomRevokeCount != 1 || state.auditCount != 1 {
		t.Fatalf("refund must revoke and audit in the transaction: %+v", state)
	}
}

func TestCreateOrReusePendingClassroomOrderKeepsSnapshotUntilPriceChanges(t *testing.T) {
	state := &orderTestState{existing: true, existingAmount: 2990, existingTitle: "基础系列"}
	store := newOrderTestStore(t, state)
	order, err := store.CreateOrReusePendingOrder(context.Background(), 7, "new", ProductClassroomSeries, 41, "基础系列", 2990)
	if err != nil || order.OutTradeNo != "existing-order" || state.closeCount != 0 {
		t.Fatalf("same snapshot should reuse pending order: order=%+v state=%+v err=%v", order, state, err)
	}
	order, err = store.CreateOrReusePendingOrder(context.Background(), 7, "replacement", ProductClassroomSeries, 41, "基础系列", 3990)
	if !errors.Is(err, ErrPendingOrderSnapshotChanged) || order.OutTradeNo != "existing-order" || state.closeCount != 0 {
		t.Fatalf("price change must await remote close: order=%+v state=%+v err=%v", order, state, err)
	}
}

func TestRevokeAllMembershipClearsAuthoritativeFields(t *testing.T) {
	state := &orderTestState{revokeRowsAffected: 1}
	store := newOrderTestStore(t, state)
	if err := store.RevokeAllMembership(context.Background(), 7); err != nil {
		t.Fatal(err)
	}
	if !state.membershipRevoked {
		t.Fatal("expected revoke to clear wx_users membership fields")
	}
}

func TestRevokeAllMembershipRejectsUnknownUser(t *testing.T) {
	state := &orderTestState{}
	store := newOrderTestStore(t, state)
	if err := store.RevokeAllMembership(context.Background(), 404); !errors.Is(err, ErrMembershipUserNotFound) {
		t.Fatalf("expected ErrMembershipUserNotFound, got %v", err)
	}
}

func newOrderTestStore(t *testing.T, state *orderTestState) *Store {
	t.Helper()
	registerOrderTestDriver()
	key := strconv.FormatInt(orderTestStateSeq.Add(1), 10)
	orderTestStates.Store(key, state)
	t.Cleanup(func() { orderTestStates.Delete(key) })
	db, err := sql.Open(orderTestDriverName, key)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return NewStore(db)
}

const orderTestDriverName = "miniapp_order_test"

var (
	registerOrderTestDriverOnce sync.Once
	orderTestStates             sync.Map
	orderTestStateSeq           atomic.Int64
)

func registerOrderTestDriver() {
	registerOrderTestDriverOnce.Do(func() {
		sql.Register(orderTestDriverName, orderTestDriver{})
	})
}

type orderTestState struct {
	existing                          bool
	existingAmount                    int
	existingTitle                     string
	insertCount                       int
	lockCount                         int
	closeCount                        int
	paidProduct                       string
	paidRefID                         int64
	paidStatus                        string
	orderPaidCount                    int
	membershipGrantCount              int
	membershipDurationDays            int
	membershipRenewsFromCurrentExpiry bool
	membershipRevoked                 bool
	revokeRowsAffected                int64
	commitCount                       int
	rollbackCount                     int
	classroomGrantCount               int
	classroomGrantProduct             string
	classroomGrantRefID               int64
	refundCount                       int
	classroomRevokeCount              int
	auditCount                        int
	targetOwned                       bool
	siblingPending                    string
	siblingCloseCount                 int
	operationOrder                    []string
}

type orderTestDriver struct{}

func (orderTestDriver) Open(name string) (driver.Conn, error) {
	value, _ := orderTestStates.Load(name)
	state, _ := value.(*orderTestState)
	return &orderTestConn{state: state}, nil
}

type orderTestConn struct {
	state *orderTestState
}

func (c *orderTestConn) Prepare(string) (driver.Stmt, error) { return nil, driver.ErrSkip }
func (c *orderTestConn) Close() error                        { return nil }
func (c *orderTestConn) Begin() (driver.Tx, error)           { return &orderTestTx{state: c.state}, nil }
func (c *orderTestConn) BeginTx(context.Context, driver.TxOptions) (driver.Tx, error) {
	return &orderTestTx{state: c.state}, nil
}

func (c *orderTestConn) ExecContext(_ context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	if strings.Contains(query, "pg_advisory_xact_lock") {
		c.state.lockCount++
		c.state.operationOrder = append(c.state.operationOrder, "target-lock")
	}
	if strings.Contains(query, "UPDATE orders SET status='closed'") {
		if strings.Contains(query, "out_trade_no IN") {
			c.state.siblingCloseCount++
		} else {
			c.state.closeCount++
			c.state.existing = false
		}
	}
	if strings.Contains(query, "UPDATE orders SET status='paid'") {
		c.state.orderPaidCount++
	}
	if strings.Contains(query, "INSERT INTO classroom_entitlements") {
		c.state.classroomGrantCount++
		if strings.Contains(query, "series_id") {
			c.state.classroomGrantProduct = ProductClassroomSeries
		} else {
			c.state.classroomGrantProduct = ProductClassroomContent
		}
		if len(args) > 1 {
			c.state.classroomGrantRefID, _ = args[1].Value.(int64)
		}
	}
	if strings.Contains(query, "UPDATE orders SET status='refunded'") {
		c.state.refundCount++
	}
	if strings.Contains(query, "UPDATE classroom_entitlements") && strings.Contains(query, "revoked_at") {
		c.state.classroomRevokeCount++
	}
	if strings.Contains(query, "INSERT INTO admin_operation_logs") {
		c.state.auditCount++
	}
	if strings.Contains(query, "UPDATE wx_users") && strings.Contains(query, "member_expires_at") {
		c.state.membershipGrantCount++
		c.state.membershipRenewsFromCurrentExpiry = strings.Contains(query, "GREATEST(COALESCE(member_expires_at, now()), now())")
		if len(args) > 1 {
			if days, ok := args[1].Value.(int64); ok {
				c.state.membershipDurationDays = int(days)
			}
		}
	}
	if strings.Contains(query, "member_level=0") && strings.Contains(query, "member_started_at=NULL") {
		c.state.membershipRevoked = true
		return driver.RowsAffected(c.state.revokeRowsAffected), nil
	}
	return driver.RowsAffected(1), nil
}

func (c *orderTestConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	switch {
	case strings.Contains(query, "SELECT wx_user_id,ref_id,product FROM orders"):
		product := c.state.paidProduct
		if product == "" {
			product = ProductReport
		}
		return &orderTestRows{columns: []string{"wx_user_id", "ref_id", "product"}, values: [][]driver.Value{{int64(7), c.state.paidRefID, product}}}, nil
	case strings.Contains(query, "FROM orders WHERE out_trade_no=$1 FOR UPDATE"):
		c.state.operationOrder = append(c.state.operationOrder, "order-row-lock")
		status := c.state.paidStatus
		if status == "" {
			status = "pending"
		}
		product := c.state.paidProduct
		if product == "" {
			product = "report"
		}
		return &orderTestRows{
			columns: []string{"id", "wx_user_id", "ref_id", "product", "status"},
			values:  [][]driver.Value{{int64(21), int64(7), c.state.paidRefID, product, status}},
		}, nil
	case strings.Contains(query, "SELECT EXISTS") && strings.Contains(query, "classroom_entitlements"):
		return &orderTestRows{columns: []string{"exists"}, values: [][]driver.Value{{c.state.targetOwned}}}, nil
	case strings.Contains(query, "SELECT out_trade_no FROM orders") && strings.Contains(query, "id<>$4"):
		if c.state.siblingPending == "" {
			return &orderTestRows{columns: []string{"out_trade_no"}}, nil
		}
		return &orderTestRows{columns: []string{"out_trade_no"}, values: [][]driver.Value{{c.state.siblingPending}}}, nil
	case strings.Contains(query, "WHERE wx_user_id=$1") && strings.Contains(query, "status='pending'"):
		if !c.state.existing {
			return &orderTestRows{columns: []string{"id", "out_trade_no", "product", "ref_id", "title", "amount", "status", "transaction_id", "create_time"}}, nil
		}
		title := c.state.existingTitle
		if title == "" {
			title = "报告"
		}
		amount := c.state.existingAmount
		if amount == 0 {
			amount = 990
		}
		return &orderTestRows{
			columns: []string{"id", "out_trade_no", "product", "ref_id", "title", "amount", "status", "transaction_id", "create_time"},
			values: [][]driver.Value{{
				int64(11), "existing-order", "report", "42", title, int64(amount), "pending", "", time.Unix(100, 0),
			}},
		}, nil
	case strings.Contains(query, "INSERT INTO orders"):
		c.state.insertCount++
		outTradeNo := args[0].Value
		return &orderTestRows{
			columns: []string{"id", "create_time"},
			values:  [][]driver.Value{{int64(12), time.Unix(200, 0), outTradeNo}},
		}, nil
	default:
		return &orderTestRows{}, nil
	}
}

func (c *orderTestConn) CheckNamedValue(*driver.NamedValue) error { return nil }

type orderTestTx struct {
	state     *orderTestState
	committed bool
}

func (tx *orderTestTx) Commit() error {
	tx.committed = true
	tx.state.commitCount++
	return nil
}
func (tx *orderTestTx) Rollback() error {
	if !tx.committed {
		tx.state.rollbackCount++
	}
	return nil
}

type orderTestRows struct {
	columns []string
	values  [][]driver.Value
	index   int
}

func (r *orderTestRows) Columns() []string { return r.columns }
func (r *orderTestRows) Close() error      { return nil }
func (r *orderTestRows) Next(dest []driver.Value) error {
	if r.index >= len(r.values) {
		return io.EOF
	}
	copy(dest, r.values[r.index])
	r.index++
	return nil
}

var _ driver.Conn = (*orderTestConn)(nil)
var _ driver.ConnBeginTx = (*orderTestConn)(nil)
var _ driver.ExecerContext = (*orderTestConn)(nil)
var _ driver.QueryerContext = (*orderTestConn)(nil)
var _ driver.NamedValueChecker = (*orderTestConn)(nil)
var _ driver.Tx = (*orderTestTx)(nil)
var _ driver.Rows = (*orderTestRows)(nil)
