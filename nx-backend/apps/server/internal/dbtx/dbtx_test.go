package dbtx

import (
	"context"
	"database/sql"
	"errors"
	"testing"
)

var (
	_ DBTX = (*sql.DB)(nil)
	_ DBTX = (*sql.Tx)(nil)
	_ Tx   = (*sql.Tx)(nil)
)

type fakeTx struct {
	committed  bool
	rolledBack bool
}

func (*fakeTx) ExecContext(context.Context, string, ...any) (sql.Result, error) {
	return nil, nil
}

func (*fakeTx) QueryContext(context.Context, string, ...any) (*sql.Rows, error) {
	return nil, nil
}

func (*fakeTx) QueryRowContext(context.Context, string, ...any) *sql.Row {
	return nil
}

func (tx *fakeTx) Commit() error {
	tx.committed = true
	return nil
}

func (tx *fakeTx) Rollback() error {
	tx.rolledBack = true
	return nil
}

type fakeBeginner struct {
	tx Tx
}

func (b fakeBeginner) BeginTx(context.Context, *sql.TxOptions) (Tx, error) {
	return b.tx, nil
}

func TestInterfacesAcceptReplaceableTransactionBoundary(t *testing.T) {
	tx := &fakeTx{}
	var beginner Beginner = fakeBeginner{tx: tx}

	got, err := beginner.BeginTx(context.Background(), &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatalf("BeginTx() error = %v", err)
	}
	if got != tx {
		t.Fatalf("BeginTx() = %T, want original fake transaction", got)
	}
	if err := got.Commit(); err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	if !tx.committed {
		t.Fatal("Commit() did not reach fake transaction")
	}
}

func TestSQLBeginnerRejectsNilDatabase(t *testing.T) {
	_, err := (SQLBeginner{}).BeginTx(context.Background(), nil)
	if err == nil {
		t.Fatal("BeginTx() error = nil, want nil database error")
	}
	if !errors.Is(err, ErrNilDB) {
		t.Fatalf("BeginTx() error = %v, want ErrNilDB", err)
	}
}
