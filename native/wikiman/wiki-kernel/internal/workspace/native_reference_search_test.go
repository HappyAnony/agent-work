//go:build fts5

package workspace

import (
	"context"
	"errors"
	"testing"
	"wiki-kernel/internal/blockview"
)

func TestNativeReferenceSearchActualOwnersNoPublication(t *testing.T) {
	ctx := context.Background()
	root := relationsSource(t)
	before := digest(t, root)
	m := open(t, root, positionOptions(root))
	a, b := getDocLease(t, m, pageA), getDocLease(t, m, pageB)
	for _, l := range []*Lease{a, b} {
		queueGetDoc(t, l)
		if e := l.FlushNativeMainQueue(ctx); e != nil {
			t.Fatal(e)
		}
		if ok, e := l.NativeCheckBlockRef(ctx, []string{blockID}); e != nil || !ok {
			t.Fatal(ok, e)
		}
		q := blockview.ReferenceSearchRequest{ID: positionRef, RootID: "other", Keyword: blockID, BeforeLen: 12, Database: true}
		out, e := l.SearchNativeReferences(ctx, q)
		if e != nil || len(out.Blocks) != 1 || out.Blocks[0].ID != blockID || !out.NewDoc {
			t.Fatal(out, e)
		}
		out.Blocks[0].Content = "mutated"
		again, e := l.SearchNativeReferences(ctx, q)
		if e != nil || again.Blocks[0].Content == "mutated" {
			t.Fatal(again, e)
		}
		q.ID = blockID
		out, e = l.SearchNativeReferences(ctx, q)
		if e != nil || len(out.Blocks) != 0 {
			t.Fatal("self filter", out, e)
		}
		canceled, cancel := context.WithCancel(ctx)
		cancel()
		if _, e = l.SearchNativeReferences(canceled, q); !errors.Is(e, context.Canceled) {
			t.Fatal(e)
		}
	}
	if digest(t, root) != before {
		t.Fatal("search published source")
	}
}
