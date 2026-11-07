package model

import (
	"context"
	"errors"
	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"reflect"
	"testing"
	"wiki-kernel/internal/blockview"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/workspace"
)

func attachBookmarkRead(t *testing.T, h *bookmarkHarness, l *workspace.Lease) *bookmarkReadEnvironment {
	t.Helper()
	if err := l.WithNativeIndexes(h.ctx, func(b *indexdb.Bundle) error {
		db, e := b.Database(indexdb.Main)
		if e != nil {
			return e
		}
		if _, e = db.ExecContext(h.ctx, "DELETE FROM blocks WHERE id=?", "20261026140000-deleted"); e != nil {
			return e
		}
		_, e = db.ExecContext(h.ctx, "UPDATE blocks SET type='p', ial='{: ' || ial || '}'")
		return e
	}); err != nil {
		t.Fatal(err)
	}
	e := &bookmarkReadEnvironment{ctx: h.ctx, wait: func() error { return h.mark("bookmark-read-wait") }, flush: func() error { return h.mark("bookmark-read-flush") },
		labels: l.NativeBookmarkLabels,
		services: blockview.BookmarkServices{Query: l.NativeBookmarkBlocks, Parser: func(context.Context) (*lute.Lute, error) { return h.env.parser(), nil },
			Node:              func(_ context.Context, id string) (*ast.Node, error) { n, _, err := h.cell.node(id); return n, err },
			AttributeViewName: func(context.Context, string) (string, error) { return "", errors.New("unexpected AV read") },
		},
	}
	h.env.bookmarkRead = e
	return e
}
func TestBookmarkReadOwnersOriginalGroupingLabelsAndSourceUnchanged(t *testing.T) {
	m, store, root := nameSource(t)
	for _, page := range []string{"1760000000000000002", "1760000000000000003"} {
		l, err := m.Acquire(context.Background(), store, page)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(l.Release)
		h := newBookmarkHarness(t, l, root, page)
		attachBookmarkRead(t, h, l)
		before := nameDigest(t, root, page)
		got, err := h.tx.buildBookmarks()
		if err != nil || len(got) != 3 || got[0].Name != "" || got[1].Name != "Original" || got[1].Count != 2 || got[2].Name != "Original/sub" {
			t.Fatal(got, err)
		}
		if h.counts["bookmark-read-wait"] != 1 || h.counts["bookmark-read-flush"] != 1 {
			t.Fatal("panel queue order missing", h.counts)
		}
		labels, err := h.tx.bookmarkLabels()
		if err != nil || !reflect.DeepEqual(labels, []string{"Original", "Original/sub"}) {
			t.Fatal(labels, err)
		}
		if h.counts["bookmark-read-flush"] != 1 {
			t.Fatal("labels added an implicit flush")
		}
		got[1].Blocks[0].IAL["bookmark"] = "caller mutation"
		again, err := h.tx.buildBookmarks()
		if err != nil || again[1].Blocks[0].IAL["bookmark"] != "Original" {
			t.Fatal("result alias", again, err)
		}
		assertNameSource(t, h.cellHarness, before)
	}
}
func TestBookmarkReadWaitFlushLabelsFailuresAndLateCancellation(t *testing.T) {
	for _, point := range []string{"wait", "flush", "labels"} {
		for _, late := range []bool{false, true} {
			t.Run(point+map[bool]string{false: "/error", true: "/cancel"}[late], func(t *testing.T) {
				h, _, _ := acquireBookmarkHarness(t)
				e := &bookmarkReadEnvironment{ctx: h.ctx, services: blockview.BookmarkServices{Query: func(context.Context) ([]indexdb.BlockRecord, error) {
					t.Fatal("query after failed admission")
					return nil, nil
				}, Parser: func(context.Context) (*lute.Lute, error) { return h.env.parser(), nil }, Node: func(context.Context, string) (*ast.Node, error) { return nil, nil }, AttributeViewName: func(context.Context, string) (string, error) { return "", nil }}, wait: func() error { return nil }, flush: func() error { return nil }, labels: func(context.Context) ([]string, error) { return []string{"partial"}, nil }}
				failure := errors.New("owned read failed")
				bad := func() error {
					if late {
						h.cancel()
						return nil
					}
					return failure
				}
				switch point {
				case "wait":
					e.wait = bad
				case "flush":
					e.flush = bad
				case "labels":
					e.labels = func(context.Context) ([]string, error) { return []string{"partial"}, bad() }
				}
				h.env.bookmarkRead = e
				want := failure
				if late {
					want = context.Canceled
				}
				if point == "labels" {
					got, err := h.tx.bookmarkLabels()
					if got != nil || !errors.Is(err, want) {
						t.Fatal(got, err)
					}
				} else {
					got, err := h.tx.buildBookmarks()
					if got != nil || !errors.Is(err, want) {
						t.Fatal(got, err)
					}
				}
			})
		}
	}
}
func TestBookmarkReadMissingServicesAndOuterContextDoNotFallback(t *testing.T) {
	h, _, _ := acquireBookmarkHarness(t)
	if _, err := h.tx.buildBookmarks(); err == nil {
		t.Fatal("missing native environment accepted")
	}
	if _, err := h.tx.bookmarkLabels(); err == nil {
		t.Fatal("labels fell back to global")
	}
	e := &bookmarkReadEnvironment{ctx: context.Background(), wait: func() error { t.Fatal("missing service used wait"); return nil }, flush: func() error { return nil }, services: blockview.BookmarkServices{Legacy: true}}
	h.env.bookmarkRead = e
	if _, err := h.tx.buildBookmarks(); err == nil {
		t.Fatal("unbound services accepted")
	}
	e.labels = func(context.Context) ([]string, error) {
		t.Fatal("cancelled outer owner queried labels")
		return nil, nil
	}
	h.cancel()
	if _, err := h.tx.bookmarkLabels(); !errors.Is(err, context.Canceled) {
		t.Fatal("outer ctx lost", err)
	}
}
