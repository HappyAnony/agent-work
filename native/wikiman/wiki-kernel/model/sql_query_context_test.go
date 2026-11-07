//go:build fts5

package model

import (
	"context"
	"errors"
	"testing"
	"wiki-kernel/av"
)

func TestSQLQueryActualOwnerReadOnlyAndCancellation(t *testing.T) {
	c, _, _, _ := deepAVFixture(t, av.LayoutTypeTable)
	out, e := QuerySQLContext(context.Background(), "SELECT 'owned' AS value")
	if e != nil || len(out) != 1 || out[0]["value"] != "owned" {
		t.Fatal(out, e)
	}
	c.r.owner.persistenceReadOnly = true
	if out, e = QuerySQLContext(context.Background(), "SELECT 'read-only' AS value"); e != nil || len(out) != 1 {
		t.Fatal(out, e)
	}
	if _, e = QuerySQLContext(context.Background(), "DELETE FROM blocks"); e == nil {
		t.Fatal("readonly admitted SQL mutation")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if out, e = QuerySQLContext(ctx, "SELECT 1"); !errors.Is(e, context.Canceled) || out != nil {
		t.Fatal(out, e)
	}
	if e = c.r.owner.CloseCloudRepo(context.Background()); e != nil {
		t.Fatal(e)
	}
	if _, e = QuerySQLContext(context.Background(), "SELECT 1"); e == nil {
		t.Fatal("closed owner admitted SQL")
	}
}
