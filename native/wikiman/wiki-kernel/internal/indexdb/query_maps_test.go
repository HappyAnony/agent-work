//go:build fts5

package indexdb

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestQueryMapsOriginalLimitsUnionConcatAndOwnedFailures(t *testing.T) {
	ctx := context.Background()
	bundle, e := OpenMemory(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer bundle.Close()
	db, _ := bundle.Database(Main)
	if _, e = db.Exec("CREATE TABLE selected_query (id TEXT, value INTEGER); INSERT INTO selected_query VALUES ('one',1),('two',2),('three',3)"); e != nil {
		t.Fatal(e)
	}
	for _, tc := range []struct {
		stmt  string
		count int
	}{
		{"SELECT * FROM selected_query ORDER BY value", 2},
		{"SELECT * FROM selected_query ORDER BY value LIMIT 3", 3},
		{"SELECT id || '-suffix' AS joined FROM selected_query ORDER BY value", 2},
		{"SELECT id FROM selected_query WHERE value=1 UNION SELECT id FROM selected_query WHERE value=2", 2},
		{"SELECT id AS renamed, value FROM selected_query WHERE value=99", 0},
	} {
		out, e := QueryMaps(ctx, db, tc.stmt, 2, false)
		if e != nil || len(out) != tc.count {
			t.Fatal(tc.stmt, out, e)
		}
	}
	if _, e = QueryMaps(ctx, db, "DELETE FROM selected_query", 2, false); e == nil {
		t.Fatal("reader acquired derived writer")
	}
	if _, e = QueryMaps(ctx, db, "UPDATE selected_query SET value=8 WHERE id='one'", 2, true); e != nil {
		t.Fatal(e)
	}
	out, e := QueryMaps(ctx, db, "SELECT value FROM selected_query WHERE id='one'", 2, false)
	if e != nil || out[0]["value"] != int64(8) {
		t.Fatal(out, e)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if out, e = QueryMaps(cancelled, db, "SELECT * FROM selected_query", 2, false); !errors.Is(e, context.Canceled) || out != nil {
		t.Fatal(out, e)
	}
	short, stop := context.WithTimeout(ctx, time.Millisecond)
	defer stop()
	out, e = QueryMaps(short, db, "WITH RECURSIVE seq(n) AS (VALUES(0) UNION ALL SELECT n+1 FROM seq WHERE n<10000000) SELECT n FROM seq LIMIT 10000000", 2, false)
	if !errors.Is(e, context.DeadlineExceeded) || out != nil {
		t.Fatal("canceled SQLite returned partial success", len(out), e)
	}
}
