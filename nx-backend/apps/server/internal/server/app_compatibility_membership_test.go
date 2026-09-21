package server

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"io"
	"strings"
	"sync"
	"testing"

	"nine-xing/nx-backend/apps/server/internal/appuser"
	"nine-xing/nx-backend/apps/server/internal/compatibility"
)

func TestCompatibilityReportAccessRedactsWhenEitherSourceCardIsRetainedReadOnly(t *testing.T) {
	registerCompatibilityMembershipDriver()
	db, err := sql.Open(compatibilityMembershipDriverName, "")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	server := &Server{db: db, appUsers: appuser.NewStore(db)}
	access, err := server.compatibilityReportAccess(context.Background(), 7, 1, 4)
	if err != nil {
		t.Fatal(err)
	}
	if access.State != resourceAccessReadOnlyOverLimit || access.RequiredPlanLevel != "svip" || !access.UpgradeRequired {
		t.Fatalf("overflow source card should lock historical report: %+v", access)
	}

	report := appCompatibilityReport{
		CardAID: 1, CardBID: 4, Summary: "历史摘要", IsFull: true,
		Dynamics: "private dynamics", Highlights: []string{"private"},
		Scores: compatibilityScoresForTest(),
	}
	applyAppCompatibilityAccess(&report, access)
	if report.Summary != "历史摘要" || report.IsFull || report.Dynamics != "" || len(report.Highlights) != 0 {
		t.Fatalf("locked report content was not redacted: %+v", report)
	}
}

func TestCompatibilityReportAccessPropagatesVipSourceRequirement(t *testing.T) {
	registerCompatibilityMembershipDriver()
	db, err := sql.Open(compatibilityMembershipDriverName, "vip_required")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	server := &Server{db: db, appUsers: appuser.NewStore(db)}
	access, err := server.compatibilityReportAccess(context.Background(), 7, 1, 4)
	if err != nil {
		t.Fatal(err)
	}
	if access.State != resourceAccessReadOnlyOverLimit || access.RequiredPlanLevel != "vip" || !access.UpgradeRequired {
		t.Fatalf("VIP source card requirement was not preserved: %+v", access)
	}
}

// Keep the test independent from the concrete score representation's field
// names while still ensuring a non-zero analysis payload is cleared.
func compatibilityScoresForTest() (scores compatibility.Scores) {
	scores.Resonance = 80
	return scores
}

const compatibilityMembershipDriverName = "app_compatibility_membership_test"

var registerCompatibilityMembershipDriverOnce sync.Once

func registerCompatibilityMembershipDriver() {
	registerCompatibilityMembershipDriverOnce.Do(func() {
		sql.Register(compatibilityMembershipDriverName, compatibilityMembershipDriver{})
	})
}

type compatibilityMembershipDriver struct{}

func (compatibilityMembershipDriver) Open(name string) (driver.Conn, error) {
	return compatibilityMembershipConn{requiredPlanLevel: name}, nil
}

type compatibilityMembershipConn struct {
	requiredPlanLevel string
}

func (compatibilityMembershipConn) Prepare(string) (driver.Stmt, error) { return nil, driver.ErrSkip }
func (compatibilityMembershipConn) Close() error                        { return nil }
func (compatibilityMembershipConn) Begin() (driver.Tx, error) {
	return compatibilityMembershipTx{}, nil
}

func (c compatibilityMembershipConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	switch {
	case strings.Contains(query, "FROM app_users") && strings.Contains(query, "member_level"):
		level := "vip"
		if c.requiredPlanLevel == "vip_required" {
			level = "free"
		}
		return newCompatibilityMembershipRows([]string{"member_level", "member_expires_at"}, [][]driver.Value{{level, nil}}), nil
	case strings.Contains(query, "SELECT card_type,status"):
		cardID, _ := args[0].Value.(int64)
		if cardID == 1 {
			return newCompatibilityMembershipRows([]string{"card_type", "status"}, [][]driver.Value{{"primary", "active"}}), nil
		}
		return newCompatibilityMembershipRows([]string{"card_type", "status"}, [][]driver.Value{{"secondary", "active"}}), nil
	case strings.Contains(query, "FROM app_membership_resource_access"):
		required := "svip"
		if c.requiredPlanLevel == "vip_required" {
			required = "vip"
		}
		return newCompatibilityMembershipRows(
			[]string{"resource_id", "state", "required_plan_level", "priority_rank", "reason", "retention_until"},
			[][]driver.Value{{int64(4), resourceAccessReadOnlyOverLimit, required, int64(4), "over limit", nil}},
		), nil
	default:
		return nil, driver.ErrSkip
	}
}

type compatibilityMembershipTx struct{}

func (compatibilityMembershipTx) Commit() error   { return nil }
func (compatibilityMembershipTx) Rollback() error { return nil }

type compatibilityMembershipRows struct {
	columns []string
	values  [][]driver.Value
	index   int
}

func newCompatibilityMembershipRows(columns []string, values [][]driver.Value) driver.Rows {
	return &compatibilityMembershipRows{columns: columns, values: values}
}

func (r *compatibilityMembershipRows) Columns() []string { return r.columns }
func (r *compatibilityMembershipRows) Close() error      { return nil }
func (r *compatibilityMembershipRows) Next(dest []driver.Value) error {
	if r.index >= len(r.values) {
		return io.EOF
	}
	copy(dest, r.values[r.index])
	r.index++
	return nil
}
