package miniapp

import (
	"context"
	"database/sql"
	"database/sql/driver"
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

func TestCreateOrReusePendingOrderClosesMismatchedPendingOrderAndCreatesNew(t *testing.T) {
	state := &orderTestState{
		existing:       true,
		existingAmount: 1,
		existingTitle:  "旧报告",
	}
	store := newOrderTestStore(t, state)

	order, err := store.CreateOrReusePendingOrder(context.Background(), 7, "new-order", "report", 42, "报告", 990)
	if err != nil {
		t.Fatal(err)
	}
	if order.OutTradeNo != "new-order" {
		t.Fatalf("expected mismatched pending order to be replaced, got %q", order.OutTradeNo)
	}
	if state.closeCount != 1 {
		t.Fatalf("expected mismatched pending order to be closed, got %d", state.closeCount)
	}
	if state.insertCount != 1 {
		t.Fatalf("expected one replacement insert, got %d", state.insertCount)
	}
	if state.lockCount != 1 {
		t.Fatalf("expected advisory lock before lookup, got %d", state.lockCount)
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

func TestRevokeAllMembershipClearsAuthoritativeFields(t *testing.T) {
	state := &orderTestState{}
	store := newOrderTestStore(t, state)
	if err := store.RevokeAllMembership(context.Background(), 7); err != nil {
		t.Fatal(err)
	}
	if !state.membershipRevoked {
		t.Fatal("expected revoke to clear wx_users membership fields")
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
	commitCount                       int
	rollbackCount                     int
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
	}
	if strings.Contains(query, "UPDATE orders SET status='closed'") {
		c.state.closeCount++
		c.state.existing = false
	}
	if strings.Contains(query, "UPDATE orders SET status='paid'") {
		c.state.orderPaidCount++
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
	}
	return driver.RowsAffected(1), nil
}

func (c *orderTestConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	switch {
	case strings.Contains(query, "FROM orders WHERE out_trade_no=$1 FOR UPDATE"):
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
