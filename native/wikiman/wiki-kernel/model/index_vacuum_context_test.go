//go:build fts5

package model

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	legacysql "wiki-kernel/sql"
)

func seedVacuum(t *testing.T, db *sql.DB) {
	t.Helper()
	if _, e := db.Exec("CREATE TABLE vacuum_fixture (id INTEGER, data BLOB)"); e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 16; i++ {
		if _, e := db.Exec("INSERT INTO vacuum_fixture VALUES (?,zeroblob(16384))", i); e != nil {
			t.Fatal(e)
		}
	}
	if _, e := db.Exec("DELETE FROM vacuum_fixture WHERE id>0"); e != nil {
		t.Fatal(e)
	}
	var free int
	if e := db.QueryRow("PRAGMA freelist_count").Scan(&free); e != nil || free == 0 {
		t.Fatal(free, e)
	}
}
func TestIndexVacuumActualOwnedDatabasesReadonlyAndErrors(t *testing.T) {
	c, _, _, _, _ := referenceModelFixture(t)
	dbs := []*sql.DB{legacysql.CaptureLegacyMainQueue().DB, legacysql.CaptureLegacyHistoryQueue().DB, legacysql.CaptureLegacyAssetQueue().DB}
	for _, db := range dbs {
		seedVacuum(t, db)
	}
	c.persistenceReadOnly = true
	if e := VacuumDataIndexContext(context.Background()); e == nil {
		t.Fatal("readonly indexes vacuumed")
	}
	c.persistenceReadOnly = false
	if e := VacuumDataIndexContext(context.Background()); e != nil {
		t.Fatal(e)
	}
	for _, db := range dbs {
		var count, free int
		if e := db.QueryRow("SELECT count(*) FROM vacuum_fixture").Scan(&count); e != nil || count != 1 {
			t.Fatal(count, e)
		}
		if e := db.QueryRow("PRAGMA freelist_count").Scan(&free); e != nil || free != 0 {
			t.Fatal("vacuum did not run", free, e)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if e := VacuumDataIndexContext(ctx); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if e := dbs[2].Close(); e != nil {
		t.Fatal(e)
	}
	if e := VacuumDataIndexContext(context.Background()); e == nil {
		t.Fatal("closed selected database swallowed")
	}
}
