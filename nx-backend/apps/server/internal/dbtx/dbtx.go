package dbtx

import (
	"context"
	"database/sql"
	"errors"
)

var ErrNilDB = errors.New("dbtx: database is nil")

// DBTX is the common query surface implemented by both sql.DB and sql.Tx.
type DBTX interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// Tx is the transaction surface needed by domain services.
type Tx interface {
	DBTX
	Commit() error
	Rollback() error
}

// Beginner allows transaction boundaries to be replaced in unit tests.
type Beginner interface {
	BeginTx(context.Context, *sql.TxOptions) (Tx, error)
}

// SQLBeginner adapts sql.DB to Beginner.
type SQLBeginner struct {
	DB *sql.DB
}

func (b SQLBeginner) BeginTx(ctx context.Context, opts *sql.TxOptions) (Tx, error) {
	if b.DB == nil {
		return nil, ErrNilDB
	}
	return b.DB.BeginTx(ctx, opts)
}
