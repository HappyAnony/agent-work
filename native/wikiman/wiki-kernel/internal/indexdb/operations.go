package indexdb

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

var ErrNoDatabase = errors.New("index_database_required")

// These are the common connection/transaction entry points used by both the
// original SQL facades and contextual services. A database is always explicit;
// no global lookup, process exit, queue, configuration or source write occurs.
func Query(ctx context.Context, db *sql.DB, statement string, args ...interface{}) (*sql.Rows, error) {
	if db == nil {
		return nil, ErrNoDatabase
	}
	if strings.TrimSpace(statement) == "" {
		return nil, errors.New("index_statement_required")
	}
	return db.QueryContext(ctx, statement, args...)
}

func QueryRow(ctx context.Context, db *sql.DB, statement string, args ...interface{}) (*sql.Row, error) {
	if db == nil {
		return nil, ErrNoDatabase
	}
	if strings.TrimSpace(statement) == "" {
		return nil, errors.New("index_statement_required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return db.QueryRowContext(ctx, statement, args...), nil
}

func Begin(ctx context.Context, db *sql.DB) (*sql.Tx, error) {
	if db == nil {
		return nil, ErrNoDatabase
	}
	return db.BeginTx(ctx, nil)
}

func Commit(tx *sql.Tx) error {
	if tx == nil {
		return errors.New("tx is nil")
	}
	return tx.Commit()
}

// Transaction is for disposable indexes, not authoritative source transactions
// or save receipts. Errors/cancellation/panic roll back before ownership ends.
func Transaction(ctx context.Context, db *sql.DB, fn func(*sql.Tx) error) (err error) {
	if fn == nil {
		return errors.New("index_transaction_handler_required")
	}
	tx, err := Begin(ctx, db)
	if err != nil {
		return err
	}
	defer func() {
		rollback := tx.Rollback()
		if rollback != nil && !errors.Is(rollback, sql.ErrTxDone) {
			err = errors.Join(err, rollback)
		}
	}()
	if err = fn(tx); err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	return Commit(tx)
}

func (b *Bundle) Transaction(ctx context.Context, kind Kind, fn func(*sql.Tx) error) error {
	db, err := b.Database(kind)
	if err != nil {
		return err
	}
	return Transaction(ctx, db, fn)
}
