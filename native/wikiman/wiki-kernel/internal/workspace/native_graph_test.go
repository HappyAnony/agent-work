//go:build fts5

package workspace

import (
	"context"
	"errors"
	"strings"
	"testing"
	"wiki-kernel/conf"
	"wiki-kernel/internal/indexdb"
)

func TestNativeGraphOriginalAssemblyOwnerIsolationAndQueryFailure(t *testing.T) {
	root := infoSource(t)
	options := lifecycleOptions(root)
	old := options.Configuration
	options.Configuration = func(store, page string) (conf.AppData, error) {
		v, e := old(store, page)
		if e != nil {
			return v, e
		}
		v.Graph = conf.NewGraph()
		v.Graph.Local.Paragraph = true
		v.Graph.Global.Paragraph = true
		v.Graph.Local.DailyNote = true
		v.Graph.Global.DailyNote = true
		return v, nil
	}
	m := open(t, root, options)
	before := digest(t, root)
	ctx := context.Background()
	for _, page := range []string{pageA, pageB} {
		l := getDocLease(t, m, page)
		queueGetDoc(t, l)
		if e := l.FlushNativeMainQueue(ctx); e != nil {
			t.Fatal(e)
		}
		for _, id := range []string{"", docID} {
			got, e := l.NativeGraph(ctx, id, "")
			if e != nil || len(got.Nodes) == 0 {
				t.Fatal(got, e)
			}
			found := false
			for _, node := range got.Nodes {
				if node.ID == docID {
					found = true
					if !strings.Contains(node.Label, page) {
						t.Fatal("foreign graph title", node)
					}
				}
			}
			if !found {
				t.Fatal("root missing", got)
			}
		}
		if e := l.WithNativeIndexes(ctx, func(b *indexdb.Bundle) error {
			db, e := b.Database(indexdb.Main)
			if e != nil {
				return e
			}
			_, e = db.Exec("DROP TABLE refs")
			return e
		}); e != nil {
			t.Fatal(e)
		}
		if got, e := l.NativeGraph(ctx, "", ""); e == nil || got.Nodes != nil {
			t.Fatal("query failure became empty success", got, e)
		}
		canceled, cancel := context.WithCancel(ctx)
		cancel()
		if _, e := l.NativeGraph(canceled, "", ""); !errors.Is(e, context.Canceled) {
			t.Fatal(e)
		}
	}
	if digest(t, root) != before {
		t.Fatal("graph changed source")
	}
}
