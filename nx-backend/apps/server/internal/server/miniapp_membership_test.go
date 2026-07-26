package server

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/config"
	"nine-xing/nx-backend/apps/server/internal/miniapp"
	"nine-xing/nx-backend/apps/server/internal/wxpay"
)

func TestMiniappMembershipValidity(t *testing.T) {
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	future := now.Add(24 * time.Hour)
	past := now.Add(-time.Second)
	tests := []struct {
		name    string
		level   int
		expires *time.Time
		want    bool
	}{
		{name: "free", level: 0, want: false},
		{name: "legacy lifetime member", level: 1, want: true},
		{name: "dated active member", level: 1, expires: &future, want: true},
		{name: "expired member", level: 1, expires: &past, want: false},
		{name: "refund or revoke wins over remaining date", level: 0, expires: &future, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := miniapp.IsMembershipActive(tt.level, tt.expires, now); got != tt.want {
				t.Fatalf("IsMembershipActive() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWxPayCallbackAcceptsMemberOrder(t *testing.T) {
	env := config.Env{}
	env.WxPay.MchID = "merchant"
	env.WxPay.AppID = "miniapp"
	err := validateWxPayCallbackAgainstOrder(env, wxpay.CallbackResult{
		OutTradeNo: "member-order", MchID: "merchant", AppID: "miniapp", AmountTotal: 9900,
	}, paymentOrderSnapshot{Amount: 9900, Product: "member"})
	if err != nil {
		t.Fatalf("member payment callback should reach miniapp.MarkOrderPaid: %v", err)
	}
}

func TestPayNotifyMemberContract(t *testing.T) {
	for _, tt := range []struct {
		name           string
		durationDays   int64
		wantCode       int
		wantReply      string
		wantPaid       bool
		wantMembership bool
	}{
		{name: "success commits order and membership", durationDays: 30, wantCode: http.StatusOK, wantReply: `"code":"SUCCESS"`, wantPaid: true, wantMembership: true},
		{name: "missing duration fails and rolls back", durationDays: 0, wantCode: http.StatusInternalServerError, wantReply: `"code":"FAIL"`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			state := &memberNotifyDBState{durationDays: tt.durationDays, status: "pending"}
			db := newMemberNotifyDB(t, state)
			s := &Server{
				env:     config.Env{WxPay: config.WxPayConfig{MchID: "merchant", AppID: "miniapp"}},
				pay:     mustWxPayClient(config.Env{WxPay: config.WxPayConfig{Dev: true}}),
				miniapp: miniapp.NewStore(db),
				payNotifyParser: func(http.Header, []byte) (wxpay.CallbackResult, error) {
					return wxpay.CallbackResult{Success: true, OutTradeNo: "member-order", TransactionID: "wx-tx", MchID: "merchant", AppID: "miniapp", AmountTotal: 9900}, nil
				},
			}
			recorder := httptest.NewRecorder()
			s.payNotify(recorder, httptest.NewRequest(http.MethodPost, "/api/pay/notify", strings.NewReader(`{}`)))
			if recorder.Code != tt.wantCode || !strings.Contains(recorder.Body.String(), tt.wantReply) {
				t.Fatalf("unexpected response code=%d body=%s", recorder.Code, recorder.Body.String())
			}
			if (state.status == "paid") != tt.wantPaid || state.membershipGranted != tt.wantMembership {
				t.Fatalf("unexpected persisted state: status=%s membership=%v", state.status, state.membershipGranted)
			}
		})
	}
}

const memberNotifyDriverName = "miniapp_member_notify_test"

var (
	memberNotifyRegister sync.Once
	memberNotifyStates   sync.Map
	memberNotifySeq      atomic.Int64
)

type memberNotifyDBState struct {
	durationDays      int64
	status            string
	pendingPaid       bool
	pendingMembership bool
	membershipGranted bool
}

func newMemberNotifyDB(t *testing.T, state *memberNotifyDBState) *sql.DB {
	t.Helper()
	memberNotifyRegister.Do(func() { sql.Register(memberNotifyDriverName, memberNotifyDriver{}) })
	key := strconv.FormatInt(memberNotifySeq.Add(1), 10)
	memberNotifyStates.Store(key, state)
	t.Cleanup(func() { memberNotifyStates.Delete(key) })
	db, err := sql.Open(memberNotifyDriverName, key)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

type memberNotifyDriver struct{}

func (memberNotifyDriver) Open(name string) (driver.Conn, error) {
	v, _ := memberNotifyStates.Load(name)
	return &memberNotifyConn{state: v.(*memberNotifyDBState)}, nil
}

type memberNotifyConn struct{ state *memberNotifyDBState }

func (c *memberNotifyConn) Prepare(string) (driver.Stmt, error) { return nil, driver.ErrSkip }
func (c *memberNotifyConn) Close() error                        { return nil }
func (c *memberNotifyConn) Begin() (driver.Tx, error) {
	return c.BeginTx(context.Background(), driver.TxOptions{})
}
func (c *memberNotifyConn) BeginTx(context.Context, driver.TxOptions) (driver.Tx, error) {
	return &memberNotifyTx{state: c.state}, nil
}
func (c *memberNotifyConn) CheckNamedValue(*driver.NamedValue) error { return nil }
func (c *memberNotifyConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	switch {
	case strings.Contains(query, "SELECT out_trade_no, product, amount, status"):
		return &memberNotifyRows{columns: []string{"out_trade_no", "product", "amount", "status"}, values: [][]driver.Value{{"member-order", "member", int64(9900), c.state.status}}}, nil
	case strings.Contains(query, "FROM orders WHERE out_trade_no=$1 FOR UPDATE"):
		return &memberNotifyRows{columns: []string{"id", "wx_user_id", "ref_id", "product", "status"}, values: [][]driver.Value{{int64(1), int64(7), c.state.durationDays, "member", c.state.status}}}, nil
	default:
		return &memberNotifyRows{}, nil
	}
}
func (c *memberNotifyConn) ExecContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Result, error) {
	if strings.Contains(query, "UPDATE orders SET status='paid'") {
		c.state.pendingPaid = true
	}
	if strings.Contains(query, "UPDATE wx_users") && strings.Contains(query, "member_expires_at") {
		c.state.pendingMembership = true
	}
	return driver.RowsAffected(1), nil
}

type memberNotifyTx struct{ state *memberNotifyDBState }

func (tx *memberNotifyTx) Commit() error {
	if tx.state.pendingPaid {
		tx.state.status = "paid"
	}
	if tx.state.pendingMembership {
		tx.state.membershipGranted = true
	}
	return nil
}
func (tx *memberNotifyTx) Rollback() error {
	tx.state.pendingPaid = false
	tx.state.pendingMembership = false
	return nil
}

type memberNotifyRows struct {
	columns []string
	values  [][]driver.Value
	index   int
}

func (r *memberNotifyRows) Columns() []string { return r.columns }
func (r *memberNotifyRows) Close() error      { return nil }
func (r *memberNotifyRows) Next(dest []driver.Value) error {
	if r.index >= len(r.values) {
		return io.EOF
	}
	copy(dest, r.values[r.index])
	r.index++
	return nil
}

var _ driver.Conn = (*memberNotifyConn)(nil)
var _ driver.ConnBeginTx = (*memberNotifyConn)(nil)
var _ driver.QueryerContext = (*memberNotifyConn)(nil)
var _ driver.ExecerContext = (*memberNotifyConn)(nil)
