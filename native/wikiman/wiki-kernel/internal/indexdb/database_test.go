package indexdb

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestExplicitOpenFileAndFailures(t *testing.T) {
	ctx := context.Background()
	name := filepath.Join(t.TempDir(), "index ?&#.db")
	db, err := OpenFile(ctx, name, 2)
	if err != nil {
		t.Fatal(err)
	}
	var matches bool
	if err = db.QueryRowContext(ctx, `SELECT regexp('^wiki','wiki index')`).Scan(&matches); err != nil || !matches {
		t.Fatal(matches, err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(name); err != nil {
		t.Fatal(err)
	}
	if _, err = OpenFile(ctx, "", 2); err == nil {
		t.Fatal("missing explicit path accepted")
	}
	if _, err = OpenFile(ctx, t.TempDir(), 2); err == nil {
		t.Fatal("directory accepted as db")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	missing := filepath.Join(t.TempDir(), "cancelled.db")
	if _, err = OpenFile(cancelled, missing, 2); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err = os.Stat(missing); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("cancelled open created a file", err)
	}
}

func TestExplicitRelativeFileResolvesWithoutChangingCallerDirectory(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	db, err := OpenFile(context.Background(), "relative ?&#.db", 1)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = os.Stat(filepath.Join(root, "relative ?&#.db")); err != nil {
		t.Fatal(err)
	}
	if got, err := os.Getwd(); err != nil || got != root {
		t.Fatal("factory changed cwd", got, err)
	}
}

func TestFailureClosesAlreadyOpenedHandles(t *testing.T) {
	var handles []*sql.DB
	want := errors.New("synthetic setup failure")
	_, err := openMemory(context.Background(), func(_ context.Context, db *sql.DB, kind Kind, _ string) error {
		handles = append(handles, db)
		if kind == Assets {
			return want
		}
		return nil
	})
	if !errors.Is(err, want) || len(handles) != 3 {
		t.Fatal(err, len(handles))
	}
	for _, db := range handles {
		if db.Stats().OpenConnections != 0 || db.Ping() == nil {
			t.Fatal("failed setup leaked DB")
		}
	}
}

func TestSchemaDefinitionsAreDetached(t *testing.T) {
	for _, kind := range []Kind{Main, History, Assets, BlockTree} {
		a, err := Statements(kind)
		if err != nil || len(a) == 0 {
			t.Fatal(kind, err)
		}
		a[0] = "mutated"
		b, _ := Statements(kind)
		if b[0] == a[0] {
			t.Fatal("mutable global schema")
		}
	}
}
