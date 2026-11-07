//go:build fts5

package workspace

import (
	"context"
	"errors"
	"strings"
	"testing"
	"wiki-kernel/internal/blockview"
)

func TestNativeEmbedsActualOwnersOrderHeadingModesAndReadOnlySQL(t *testing.T) {
	ctx := context.Background()
	root := positionSource(t)
	before := digest(t, root)
	m := open(t, root, positionOptions(root))
	for _, page := range []string{pageA, pageB} {
		l := getDocLease(t, m, page)
		queueGetDoc(t, l)
		if e := l.FlushNativeMainQueue(ctx); e != nil {
			t.Fatal(e)
		}
		blocks, e := l.NativeEmbeds(ctx, blockview.EmbedRequest{IncludeIDs: []string{getDocDefinition, blockID}, Breadcrumb: true})
		if e != nil || len(blocks) != 2 || blocks[0].Block.ID != getDocDefinition || blocks[1].Block.ID != blockID {
			t.Fatal(page, blocks, e)
		}
		owner, foreign := "OwnerTermA", "OwnerTermB"
		if page == pageB {
			owner, foreign = foreign, owner
		}
		if !strings.Contains(blocks[0].Block.Markdown, owner) || strings.Contains(blocks[0].Block.Markdown, foreign) || !strings.Contains(blocks[0].Block.HPath, "Notebook & "+page) {
			t.Fatal(page, blocks[0])
		}
		for _, mode := range []int{0, 1, 2} {
			blocks, e = l.NativeEmbeds(ctx, blockview.EmbedRequest{IncludeIDs: []string{positionH1}, HeadingMode: mode, Breadcrumb: true})
			if e != nil || len(blocks) != 1 {
				t.Fatal(mode, blocks, e)
			}
			if mode == 1 && blocks[0].Block.Markdown != "Heading & "+page {
				t.Fatal("heading-only", blocks[0])
			}
			if mode == 2 && strings.Contains(blocks[0].Block.Markdown, "Heading & "+page) {
				t.Fatal("heading retained", blocks[0])
			}
		}
		blocks, e = l.NativeEmbeds(ctx, blockview.EmbedRequest{Query: true, SQL: "SELECT * FROM blocks WHERE id='" + blockID + "'", ExcludeIDs: []string{blockID}})
		if e != nil || blocks == nil || len(blocks) != 0 {
			t.Fatal(blocks, e)
		}
		if _, e = l.NativeEmbeds(ctx, blockview.EmbedRequest{Query: true, SQL: "DELETE FROM blocks"}); e == nil {
			t.Fatal("write SQL admitted")
		}
		canceled, cancel := context.WithCancel(ctx)
		cancel()
		if _, e = l.NativeEmbeds(canceled, blockview.EmbedRequest{}); !errors.Is(e, context.Canceled) {
			t.Fatal(e)
		}
	}
	if digest(t, root) != before {
		t.Fatal("embed renderer changed source")
	}
}
