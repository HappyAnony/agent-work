//go:build fts5

package model

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"wiki-kernel/conf"
)

func TestGraphOwnedActualAssemblyReadOnlyFailureAndNoSourceRepair(t *testing.T) {
	c, r, id, file, before := referenceModelFixture(t)
	c.Export = conf.NewExport()
	c.Graph = conf.NewGraph()
	c.Graph.Local.Paragraph = true
	c.Graph.Global.Paragraph = true
	c.Graph.Local.DailyNote = true
	c.Graph.Global.DailyNote = true
	ctx := context.Background()
	c.persistenceReadOnly = true
	for _, local := range []bool{false, true} {
		var key string
		if local {
			key = id
		}
		got, e := GraphContext(ctx, key, "", "", nil)
		if e != nil || len(got.Nodes) == 0 {
			t.Fatal(got, e)
		}
		found := false
		for _, node := range got.Nodes {
			if node.ID == id {
				found = true
			}
		}
		if !found {
			t.Fatal("definition missing", got)
		}
	}
	if raw, e := os.ReadFile(file); e != nil || string(raw) != string(before) {
		t.Fatal("graph read migrated source", e)
	}
	value, _ := json.Marshal(conf.NewGlobalGraph())
	if _, e := SetGraphSettingContext(ctx, "graphGlobal", value); !errors.Is(e, ErrSettingReadOnly) {
		t.Fatal(e)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, e := BuildGraphContext(canceled, ""); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if _, e := r.main.DB.Exec("DROP TABLE refs"); e != nil {
		t.Fatal(e)
	}
	if got, e := BuildGraphContext(ctx, ""); e == nil || got.Nodes != nil {
		t.Fatal("query failure became successful empty graph", got, e)
	}
}
