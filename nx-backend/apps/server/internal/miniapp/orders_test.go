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
	existing       bool
	existingAmount int
	existingTitle  string
	insertCount    int
	lockCount      int
	closeCount     int
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
func (c *orderTestConn) Begin() (driver.Tx, error)           { return orderTestTx{}, nil }
func (c *orderTestConn) BeginTx(context.Context, driver.TxOptions) (driver.Tx, error) {
	return orderTestTx{}, nil
}

func (c *orderTestConn) ExecContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Result, error) {
	if strings.Contains(query, "pg_advisory_xact_lock") {
		c.state.lockCount++
	}
	if strings.Contains(query, "UPDATE orders SET status='closed'") {
		c.state.closeCount++
		c.state.existing = false
	}
	return driver.RowsAffected(1), nil
}

func (c *orderTestConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	switch {
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

type orderTestTx struct{}

func (orderTestTx) Commit() error   { return nil }
func (orderTestTx) Rollback() error { return nil }

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
var _ driver.Tx = orderTestTx{}
var _ driver.Rows = (*orderTestRows)(nil)
