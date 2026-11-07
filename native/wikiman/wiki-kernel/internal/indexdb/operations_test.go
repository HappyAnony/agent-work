package indexdb

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
)

func TestExplicitTransactionCommitRollbackCancelAndPanic(t *testing.T) {
	ctx := context.Background()
	db, err := OpenFile(ctx, filepath.Join(t.TempDir(), "synthetic.db"), 2)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec(`CREATE TABLE entries(id TEXT PRIMARY KEY,value TEXT)`); err != nil {
		t.Fatal(err)
	}
	insert := func(id string) func(*sql.Tx) error {
		return func(tx *sql.Tx) error {
			_, err := tx.Exec(`INSERT INTO entries VALUES(?,?)`, id, "fixture")
			return err
		}
	}
	if err = Transaction(ctx, db, insert("committed")); err != nil {
		t.Fatal(err)
	}
	expected := errors.New("synthetic rollback")
	if err = Transaction(ctx, db, func(tx *sql.Tx) error {
		if err := insert("rollback")(tx); err != nil {
			return err
		}
		return expected
	}); !errors.Is(err, expected) {
		t.Fatal(err)
	}
	cancelCtx, cancel := context.WithCancel(ctx)
	if err = Transaction(cancelCtx, db, func(tx *sql.Tx) error {
		if err := insert("cancelled")(tx); err != nil {
			return err
		}
		cancel()
		return nil
	}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	func() {
		defer func() {
			if recover() == nil {
				t.Error("panic swallowed")
			}
		}()
		_ = Transaction(ctx, db, func(tx *sql.Tx) error {
			if err := insert("panic")(tx); err != nil {
				return err
			}
			panic("synthetic")
		})
	}()
	row, err := QueryRow(ctx, db, `SELECT count(*) FROM entries`)
	if err != nil {
		t.Fatal(err)
	}
	var count int
	if err = row.Scan(&count); err != nil || count != 1 {
		t.Fatal(count, err)
	}
	rows, err := Query(ctx, db, `SELECT id FROM entries`)
	if err != nil {
		t.Fatal(err)
	}
	rows.Close()
}

func TestExplicitDatabaseFailuresReturnErrors(t *testing.T) {
	ctx := context.Background()
	if _, err := Begin(ctx, nil); !errors.Is(err, ErrNoDatabase) {
		t.Fatal(err)
	}
	if _, err := Query(ctx, nil, "SELECT 1"); !errors.Is(err, ErrNoDatabase) {
		t.Fatal(err)
	}
	if _, err := QueryRow(ctx, nil, "SELECT 1"); !errors.Is(err, ErrNoDatabase) {
		t.Fatal(err)
	}
	if err := Commit(nil); err == nil {
		t.Fatal("nil transaction accepted")
	}
	if err := Transaction(ctx, nil, nil); err == nil {
		t.Fatal("missing callback accepted")
	}
}
