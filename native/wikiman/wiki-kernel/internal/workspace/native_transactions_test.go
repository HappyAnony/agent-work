//go:build fts5

package workspace

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"wiki-kernel/internal/indexdb"
)

func TestOriginalIndexTransactionsHavePageOwnership(t *testing.T) {
	ctx := context.Background()
	root := fixture(t)
	before := digest(t, root)
	m := open(t, root, Options{})
	a, err := m.Acquire(ctx, storeID, pageA)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Release()
	b, err := m.Acquire(ctx, storeID, pageB)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Release()
	tables := map[indexdb.Kind]string{indexdb.Main: "blocks", indexdb.History: "histories_fts_case_insensitive", indexdb.Assets: "asset_contents_fts_case_insensitive", indexdb.BlockTree: "blocktrees"}
	for kind, table := range tables {
		if err = a.WithNativeTransaction(ctx, kind, func(tx *sql.Tx) error {
			_, err := tx.Exec("INSERT INTO "+table+"(id) VALUES(?)", "same-id")
			return err
		}); err != nil {
			t.Fatal(kind, err)
		}
		rollback := errors.New("synthetic index rollback")
		if err = a.WithNativeTransaction(ctx, kind, func(tx *sql.Tx) error {
			if _, err := tx.Exec("INSERT INTO "+table+"(id) VALUES(?)", "rollback-id"); err != nil {
				return err
			}
			return rollback
		}); !errors.Is(err, rollback) {
			t.Fatal(err)
		}
		for lease, want := range map[*Lease]int{a: 1, b: 0} {
			if err = lease.WithNativeTransaction(ctx, kind, func(tx *sql.Tx) error {
				var count int
				err := tx.QueryRow("SELECT count(*) FROM " + table).Scan(&count)
				if err == nil && count != want {
					t.Fatal("cross-page transaction or partial rollback", kind, count, want)
				}
				return err
			}); err != nil {
				t.Fatal(err)
			}
		}
	}
	if digest(t, root) != before {
		t.Fatal("derived transaction wrote authoritative source")
	}
}
