//go:build fts5

package workspace

import (
	"context"
	"errors"
	"strings"
	"testing"
	"wiki-kernel/internal/docquery"
)

func TestNativeBlockContentActualOwnersAndReadOnly(t *testing.T) {
	ctx := context.Background()
	root := relationsSource(t)
	before := digest(t, root)
	m := open(t, root, positionOptions(root))
	a, b := getDocLease(t, m, pageA), getDocLease(t, m, pageB)
	for _, l := range []*Lease{a, b} {
		outline, e := l.NativeOutline(ctx, docID, false)
		if e != nil || len(outline) == 0 {
			t.Fatal(outline, e)
		}
		for _, embed := range []bool{false, true} {
			dom, e := l.NativeBlockDOMs(ctx, []string{blockID}, embed)
			if e != nil || len(dom) != 1 || !strings.Contains(dom[blockID], blockID) {
				t.Fatal(dom, e)
			}
		}
		out, e := l.NativeReadBlocks(ctx, docquery.BlocksRequest{Mode: "block", ID: blockID})
		if e != nil || out.Block == nil || out.Block.ID != blockID {
			t.Fatal(out, e)
		}
		md, e := l.NativeReadBlocks(ctx, docquery.BlocksRequest{Mode: "md", ID: blockID})
		if e != nil || !strings.Contains(md.Text, out.Block.Content) {
			t.Fatal(md, e)
		}
		children, e := l.NativeReadBlocks(ctx, docquery.BlocksRequest{Mode: "children", ID: docID})
		if e != nil || len(children.Children) == 0 {
			t.Fatal(children, e)
		}
		ctx, cancel := context.WithCancel(ctx)
		cancel()
		if _, e = l.NativeReadBlocks(ctx, docquery.BlocksRequest{Mode: "block", ID: blockID}); !errors.Is(e, context.Canceled) {
			t.Fatal(e)
		}
	}
	aa, e := a.NativeReadBlocks(ctx, docquery.BlocksRequest{Mode: "block", ID: blockID})
	if e != nil {
		t.Fatal(e)
	}
	bb, e := b.NativeReadBlocks(ctx, docquery.BlocksRequest{Mode: "block", ID: blockID})
	if e != nil {
		t.Fatal(e)
	}
	if aa.Block.ParentID == bb.Block.ParentID {
		t.Fatal("owners selected same parent", aa.Block, bb.Block)
	}
	if digest(t, root) != before {
		t.Fatal("read published source")
	}
}
