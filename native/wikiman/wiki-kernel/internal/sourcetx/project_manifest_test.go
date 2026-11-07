package sourcetx

import (
	"context"
	"database/sql"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"wiki-kernel/internal/wikistore"
)

func projectionTestTransaction(t *testing.T, rootPath string) (*sql.DB, *sql.Tx, *os.Root, *wikistore.TreeSnapshot) {
	t.Helper()
	ctx := context.Background()
	tree, err := (wikistore.Scanner{}).Scan(ctx, rootPath)
	if err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { root.Close() })
	db, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "projection.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(schema + `INSERT INTO documents VALUES('baseline','page','box','doc','hash');`); err != nil {
		t.Fatal(err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	return db, tx, root, tree
}

func requireProjectionRollback(t *testing.T, db *sql.DB, tx *sql.Tx) {
	t.Helper()
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM documents WHERE path='baseline'`).Scan(&count); err != nil || count != 1 {
		t.Fatal("failed projection changed published database", count, err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM documents`).Scan(&count); err != nil || count != 1 {
		t.Fatal("failed projection published partial document rows", count, err)
	}
}

func TestProjectionManifestFailuresRollBackContentChanges(t *testing.T) {
	t.Run("incomplete enumeration", func(t *testing.T) {
		rootPath := fixture(t)
		db, tx, root, tree := projectionTestTransaction(t, rootPath)
		base := "pages/" + p2 + "/data"
		calls := 0
		incomplete := errors.New("incomplete enumeration")
		op := contentManifestIO{readDir: func(root *os.Root, name string) ([]fs.DirEntry, error) {
			if name == base {
				calls++
				if calls == 2 {
					return nil, incomplete
				}
			}
			return fs.ReadDir(root.FS(), name)
		}}
		err := updateContentWithIO(context.Background(), tx, root, tree, true, op)
		if !errors.Is(err, incomplete) {
			t.Fatal(err)
		}
		requireProjectionRollback(t, db, tx)
	})

	t.Run("same metadata replacement", func(t *testing.T) {
		rootPath := fixture(t)
		db, tx, root, tree := projectionTestTransaction(t, rootPath)
		target := filepath.Join(rootPath, filepath.FromSlash(docPath))
		original, err := os.Stat(target)
		if err != nil {
			t.Fatal(err)
		}
		calls := 0
		op := contentManifestIO{lstat: func(root *os.Root, name string) (fs.FileInfo, error) {
			if name == docPath {
				calls++
				if calls == 5 {
					raw, err := os.ReadFile(target)
					if err != nil {
						t.Fatal(err)
					}
					for i := range raw {
						raw[i] = ' '
					}
					tmp := target + ".replacement"
					if err := os.WriteFile(tmp, raw, original.Mode()); err != nil {
						t.Fatal(err)
					}
					if err := os.Chtimes(tmp, original.ModTime(), original.ModTime()); err != nil {
						t.Fatal(err)
					}
					if err := os.Rename(tmp, target); err != nil {
						t.Fatal(err)
					}
				}
			}
			return root.Lstat(name)
		}}
		err = updateContentWithIO(context.Background(), tx, root, tree, true, op)
		if !errors.Is(err, errProjectionSourceChanged) {
			t.Fatal(err)
		}
		requireProjectionRollback(t, db, tx)
	})
}
