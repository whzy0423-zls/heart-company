package server

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"nine-xing/nx-backend/apps/server/internal/appuser"
)

func TestEnsureCardWritableAllowsActivePrimaryWithoutLedgerLookup(t *testing.T) {
	registerPrimaryCardAccessDriver()
	primaryCardAccessQueries.Store(0)
	db, err := sql.Open(primaryCardAccessDriverName, "")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	server := &Server{db: db, appUsers: appuser.NewStore(db)}
	if err := server.ensureCardWritable(context.Background(), 7, 1); err != nil {
		t.Fatalf("active primary card should remain writable: %v", err)
	}
	if got := primaryCardAccessQueries.Load(); got != 1 {
		t.Fatalf("expected only the primary-card lookup, got %d queries", got)
	}
}

const primaryCardAccessDriverName = "membership_primary_card_access_test"

var (
	registerPrimaryCardAccessDriverOnce sync.Once
	primaryCardAccessQueries            atomic.Int64
)

func registerPrimaryCardAccessDriver() {
	registerPrimaryCardAccessDriverOnce.Do(func() {
		sql.Register(primaryCardAccessDriverName, primaryCardAccessDriver{})
	})
}

type primaryCardAccessDriver struct{}

func (primaryCardAccessDriver) Open(string) (driver.Conn, error) {
	return primaryCardAccessConn{}, nil
}

type primaryCardAccessConn struct{}

func (primaryCardAccessConn) Prepare(string) (driver.Stmt, error) { return nil, driver.ErrSkip }
func (primaryCardAccessConn) Close() error                        { return nil }
func (primaryCardAccessConn) Begin() (driver.Tx, error)           { return primaryCardAccessTx{}, nil }

func (primaryCardAccessConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	primaryCardAccessQueries.Add(1)
	if strings.Contains(query, "SELECT card_type,status") {
		return &primaryCardAccessRows{
			columns: []string{"card_type", "status"},
			values:  [][]driver.Value{{"primary", "active"}},
		}, nil
	}
	return nil, driver.ErrSkip
}

type primaryCardAccessTx struct{}

func (primaryCardAccessTx) Commit() error   { return nil }
func (primaryCardAccessTx) Rollback() error { return nil }

type primaryCardAccessRows struct {
	columns []string
	values  [][]driver.Value
	index   int
}

func (r *primaryCardAccessRows) Columns() []string { return r.columns }
func (r *primaryCardAccessRows) Close() error      { return nil }
func (r *primaryCardAccessRows) Next(dest []driver.Value) error {
	if r.index >= len(r.values) {
		return io.EOF
	}
	copy(dest, r.values[r.index])
	r.index++
	return nil
}

const pgxErrorDriverName = "membership_pgx_error_test"

var registerPgxErrorDriverOnce sync.Once

func registerPgxErrorDriver() {
	registerPgxErrorDriverOnce.Do(func() {
		sql.Register(pgxErrorDriverName, pgxErrorDriver{})
	})
}

// pgxErrorDriver intentionally has "pgx" in its concrete type name. This
// keeps membershipLegacyDriverError from classifying a generic timeout as an
// old fixture-schema error, which is the production fail-open case we need to
// exercise.
type pgxErrorDriver struct{}

func (pgxErrorDriver) Open(string) (driver.Conn, error) { return pgxErrorConn{}, nil }

type pgxErrorConn struct{}

func (pgxErrorConn) Prepare(string) (driver.Stmt, error) { return nil, driver.ErrSkip }
func (pgxErrorConn) Close() error                        { return nil }
func (pgxErrorConn) Begin() (driver.Tx, error)           { return pgxErrorTx{}, nil }

func (pgxErrorConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	switch {
	case strings.Contains(query, "SELECT member_level,member_expires_at"):
		return &membershipErrorRows{
			columns: []string{"member_level", "member_expires_at"},
			values:  [][]driver.Value{{"free", nil}},
		}, nil
	case strings.Contains(query, "SELECT card_type,status"):
		return &membershipErrorRows{
			columns: []string{"card_type", "status"},
			values:  [][]driver.Value{{"secondary", "active"}},
		}, nil
	default:
		return nil, errors.New("database timeout while reading membership resource access")
	}
}

type pgxErrorTx struct{}

func (pgxErrorTx) Commit() error   { return nil }
func (pgxErrorTx) Rollback() error { return nil }

type membershipErrorRows struct {
	columns []string
	values  [][]driver.Value
	index   int
}

func (r *membershipErrorRows) Columns() []string { return r.columns }
func (r *membershipErrorRows) Close() error      { return nil }
func (r *membershipErrorRows) Next(dest []driver.Value) error {
	if r.index >= len(r.values) {
		return io.EOF
	}
	copy(dest, r.values[r.index])
	r.index++
	return nil
}

func TestCardMembershipMetadataFailsClosedOnProductionLedgerError(t *testing.T) {
	registerPgxErrorDriver()
	db, err := sql.Open(pgxErrorDriverName, "")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	server := &Server{db: db}
	metadata, accessErr := server.cardMembershipResourceMetadata(context.Background(), 7, 42, "history")
	if accessErr == nil {
		t.Fatal("expected membership access error for a production-style ledger timeout")
	}
	if metadata.State == resourceAccessActive || metadata.UpgradeRequired {
		t.Fatalf("generic ledger error must not return active/upgrade metadata: %+v", metadata)
	}
}

func TestMembershipLegacyDriverErrorRequiresExplicitFixtureMarker(t *testing.T) {
	registerPrimaryCardAccessDriver()
	db, err := sql.Open(primaryCardAccessDriverName, "")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if membershipLegacyDriverError(db, errors.New("database timeout while reading membership resource access")) {
		t.Fatal("arbitrary fixture-driver errors must remain fail-closed")
	}
	if !membershipLegacyDriverError(db, errors.New("unexpected query: app_membership_resource_access")) {
		t.Fatal("explicit unsupported-query fixture error should use legacy compatibility")
	}
}
