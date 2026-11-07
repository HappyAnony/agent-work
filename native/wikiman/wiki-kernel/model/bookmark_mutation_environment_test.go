package model

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/88250/lute/parse"
	"reflect"
	"testing"
	"time"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/workspace"
)

type bookmarkHarness struct {
	*blockAttributeHarness
	bookmark *bookmarkMutationEnvironment
	steps    []string
	stages   []string
	cleared  int
}

func newBookmarkHarness(t *testing.T, l *workspace.Lease, root, page string) *bookmarkHarness {
	t.Helper()
	h := &bookmarkHarness{blockAttributeHarness: newBlockAttributeHarness(t, l, root, page)}
	for id, label := range map[string]string{settingsDoc: "Original", settingsBlock: "Original", cellSecond: "Original/sub", nameBlock: "Other"} {
		n, _, err := h.cell.node(id)
		if err != nil {
			t.Fatal(err)
		}
		n.SetIALAttr("bookmark", label)
	}
	// Use the actual owner's original-schema SQLite, including a broad SQL
	// false positive and a stale/deleted block. AST remains authoritative.
	rows := []indexdb.BlockRecord{
		{ID: settingsDoc, RootID: settingsDoc, Box: settingsBox, IAL: `bookmark="Original"`},
		{ID: settingsBlock, RootID: settingsDoc, Box: settingsBox, IAL: `bookmark="Original"`},
		{ID: cellSecond, RootID: settingsDoc, Box: settingsBox, IAL: `bookmark="Original/sub"`},
		{ID: nameBlock, RootID: nameDoc, Box: settingsBox, IAL: `custom-bookmark="Original"`, Content: page},
		{ID: "20261026140000-deleted", RootID: nameDoc, Box: settingsBox, IAL: `bookmark="Original"`},
	}
	if err := l.WithNativeTransaction(h.ctx, indexdb.Main, func(tx *sql.Tx) error { return indexdb.InsertBlocks(h.ctx, tx, rows, false, nil) }); err != nil {
		t.Fatal(err)
	}
	mark := func(s string) error { h.steps = append(h.steps, s); return h.mark(s) }
	e := &bookmarkMutationEnvironment{ctx: h.ctx, gate: h.attributes.gate,
		query: func() (ret []indexdb.BlockRecord, err error) {
			if err = mark("bookmark-query"); err != nil {
				return
			}
			return l.NativeBookmarkBlocks(h.ctx)
		},
		load: func(id string) (*parse.Tree, error) {
			if err := mark("bookmark-load"); err != nil {
				return nil, err
			}
			return h.tx.loadTree(id)
		},
		stage: func(tree *parse.Tree) error {
			if err := mark("bookmark-stage"); err != nil {
				return err
			}
			h.stages = append(h.stages, tree.ID)
			return h.tx.writeTree(tree)
		},
		cache: func(id string, attrs map[string]string) error {
			if err := mark("bookmark-cache"); err != nil {
				return err
			}
			h.attrs[id] = attrs
			return nil
		},
		progress: func(kind, value string) error { return mark("bookmark-progress:" + kind) },
		clear:    func() error { h.cleared++; return mark("bookmark-clear") },
		delay:    func() error { return mark("bookmark-delay") }, reload: func() error { return mark("bookmark-reload") },
		invalid: func(s string) error { return fmt.Errorf("%s invalid %s", page, s) }, empty: func() error { return fmt.Errorf("%s empty", page) },
	}
	h.bookmark = e
	h.env.bookmarkMutation = e
	return h
}
func acquireBookmarkHarness(t *testing.T) (*bookmarkHarness, string, string) {
	t.Helper()
	m, store, root := nameSource(t)
	page := "1760000000000000002"
	l, err := m.Acquire(context.Background(), store, page)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(l.Release)
	return newBookmarkHarness(t, l, root, page), root, page
}
func TestBookmarkMutationOwnersExactNamesOriginalSaveOrderAndNoSourceWrites(t *testing.T) {
	m, store, root := nameSource(t)
	for _, page := range []string{"1760000000000000002", "1760000000000000003"} {
		l, err := m.Acquire(context.Background(), store, page)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(l.Release)
		h := newBookmarkHarness(t, l, root, page)
		before := nameDigest(t, root, page)
		if err = h.tx.renameBookmark("Original", "  "+page+"/renamed  "); err != nil {
			t.Fatal(err)
		}
		for _, id := range []string{settingsDoc, settingsBlock} {
			n, _, _ := h.cell.node(id)
			if n.IALAttr("bookmark") != page+"/renamed" || h.attrs[id]["bookmark"] != page+"/renamed" {
				t.Fatal("owner/exact rename", id, n.KramdownIAL)
			}
		}
		for id, want := range map[string]string{cellSecond: "Original/sub", nameBlock: "Other"} {
			n, _, _ := h.cell.node(id)
			if n.IALAttr("bookmark") != want {
				t.Fatal("broad SQL candidate changed unrelated bookmark", id)
			}
		}
		if len(h.stages) != 2 || h.counts["bookmark-load"] != 2 || h.counts["bookmark-cache"] != 2 || h.counts["bookmark-reload"] != 1 || h.cleared != 0 {
			t.Fatal("original per-root save/refresh", h.steps, h.stages)
		}
		// Query remains stale, so removing the newly named value must still use
		// live AST values instead of the derived SQL label.
		if err = h.tx.removeBookmark(page + "/renamed"); err != nil {
			t.Fatal(err)
		}
		n, _, _ := h.cell.node(settingsBlock)
		if n.IALAttr("bookmark") != "" || h.attrs[settingsBlock]["bookmark"] != "" {
			t.Fatal("remove failed")
		}
		if n.IALAttr("custom-preserved") != "untouched" {
			t.Fatal("unrelated attrs lost")
		}
		assertNameSource(t, h.cellHarness, before)
	}
}
func TestBookmarkMutationValidationAndNoGlobalFallback(t *testing.T) {
	for _, kind := range []string{"same", "marker", "empty", "missing", "readonly", "bad-row", "bad-tree", "cancelled", "empty-query"} {
		t.Run(kind, func(t *testing.T) {
			h, _, _ := acquireBookmarkHarness(t)
			var err error
			switch kind {
			case "same":
				err = h.tx.renameBookmark("Original", " Original ")
			case "marker":
				err = h.tx.renameBookmark("Original", "bad*name")
			case "empty":
				err = h.tx.renameBookmark("Original", " \t ")
			case "missing":
				h.env.bookmarkMutation = nil
				err = h.tx.removeBookmark("Original")
			case "readonly":
				h.bookmark.readOnly = true
				err = h.tx.removeBookmark("Original")
			case "bad-row":
				h.bookmark.query = func() ([]indexdb.BlockRecord, error) { return []indexdb.BlockRecord{{ID: settingsBlock}}, nil }
				err = h.tx.removeBookmark("Original")
			case "bad-tree":
				h.bookmark.load = func(string) (*parse.Tree, error) { return &parse.Tree{}, nil }
				err = h.tx.removeBookmark("Original")
			case "cancelled":
				h.cancel()
				err = h.tx.removeBookmark("Original")
			case "empty-query":
				h.bookmark.query = func() ([]indexdb.BlockRecord, error) { return nil, nil }
				err = h.tx.removeBookmark("")
			}
			if kind == "same" || kind == "empty-query" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil {
				t.Fatal("invalid request succeeded", kind)
			}
			if kind == "same" && len(h.steps) != 0 {
				t.Fatal("same name had effects", h.steps)
			}
			if kind == "empty-query" && (h.counts["bookmark-reload"] != 1 || len(h.stages) != 0) {
				t.Fatal("empty query original refresh", h.steps)
			}
		})
	}
}
func TestBookmarkMutationFailuresAndLateCancellationNeverReload(t *testing.T) {
	for _, point := range []string{"bookmark-progress:rename", "bookmark-query", "bookmark-load", "bookmark-cache", "bookmark-progress:save", "bookmark-stage", "bookmark-delay", "bookmark-reload"} {
		for _, late := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/%t", point, late), func(t *testing.T) {
				h, root, page := acquireBookmarkHarness(t)
				before := nameDigest(t, root, page)
				if !late {
					h.failure = point
				} else {
					e := h.bookmark
					switch point {
					case "bookmark-query":
						f := e.query
						e.query = func() (v []indexdb.BlockRecord, err error) { v, err = f(); h.cancel(); return }
					case "bookmark-load":
						f := e.load
						e.load = func(id string) (v *parse.Tree, err error) { v, err = f(id); h.cancel(); return }
					case "bookmark-cache":
						f := e.cache
						e.cache = func(id string, a map[string]string) error { err := f(id, a); h.cancel(); return err }
					case "bookmark-stage":
						f := e.stage
						e.stage = func(tree *parse.Tree) error { err := f(tree); h.cancel(); return err }
					case "bookmark-delay":
						f := e.delay
						e.delay = func() error { err := f(); h.cancel(); return err }
					case "bookmark-reload":
						f := e.reload
						e.reload = func() error { err := f(); h.cancel(); return err }
					default:
						f := e.progress
						e.progress = func(k, v string) error {
							err := f(k, v)
							if "bookmark-progress:"+k == point {
								h.cancel()
							}
							return err
						}
					}
				}
				err := h.tx.renameBookmark("Original", "changed")
				if err == nil || (late && !errors.Is(err, context.Canceled)) {
					t.Fatal("failure/cancellation lost", err, h.steps)
				}
				if h.cleared != 1 {
					t.Fatal("failed owner progress not cleared", h.steps)
				}
				if point != "bookmark-reload" && h.counts["bookmark-reload"] != 0 {
					t.Fatal("failure announced success", h.steps)
				}
				assertNameSource(t, h.cellHarness, before)
			})
		}
	}
}
func TestBookmarkMutationGateCancellationBeforeSideEffects(t *testing.T) {
	h, _, _ := acquireBookmarkHarness(t)
	g := h.bookmark.gate
	<-g.token
	done := make(chan error, 1)
	go func() { done <- h.tx.removeBookmark("Original") }()
	h.cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("gate wait ignored cancellation")
	}
	g.token <- struct{}{}
	if !reflect.DeepEqual(h.steps, []string(nil)) {
		t.Fatal("cancelled gate had effects", h.steps)
	}
}

func TestBookmarkMutationGateSerializesLiveTreeReads(t *testing.T) {
	h, _, _ := acquireBookmarkHarness(t)
	entered, release := make(chan struct{}), make(chan struct{})
	stage := h.bookmark.stage
	first := true
	h.bookmark.stage = func(tree *parse.Tree) error {
		if first {
			first = false
			close(entered)
			<-release
		}
		return stage(tree)
	}
	rename := make(chan error, 1)
	go func() { rename <- h.tx.renameBookmark("Original", "Latest") }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("rename did not stage")
	}
	remove := make(chan error, 1)
	go func() { remove <- h.tx.removeBookmark("Latest") }()
	select {
	case err := <-remove:
		close(release)
		t.Fatal("second operation escaped owner gate", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	for _, done := range []chan error{rename, remove} {
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(time.Second):
			t.Fatal("owner mutation deadlocked")
		}
	}
	for _, id := range []string{settingsDoc, settingsBlock} {
		n, _, _ := h.cell.node(id)
		if n.IALAttr("bookmark") != "" {
			t.Fatal("second mutation used stale tree", id)
		}
	}
}

func TestBookmarkMutationMissingServicesFailBeforeEffects(t *testing.T) {
	h, _, _ := acquireBookmarkHarness(t)
	for _, field := range []string{"ctx", "gate", "query", "load", "stage", "cache", "progress", "clear", "delay", "reload", "invalid", "empty"} {
		e := *h.bookmark
		// Unexported environment fields cannot be assigned by reflection.
		switch field {
		case "ctx":
			e.ctx = nil
		case "gate":
			e.gate = nil
		case "query":
			e.query = nil
		case "load":
			e.load = nil
		case "stage":
			e.stage = nil
		case "cache":
			e.cache = nil
		case "progress":
			e.progress = nil
		case "clear":
			e.clear = nil
		case "delay":
			e.delay = nil
		case "reload":
			e.reload = nil
		case "invalid":
			e.invalid = nil
		case "empty":
			e.empty = nil
		}
		if err := mutateBookmarkWithEnvironment("Original", "Next", true, &e); err == nil {
			t.Fatal("missing service accepted", field)
		}
	}
	if len(h.steps) != 0 {
		t.Fatal("dependency check had effects", h.steps)
	}
	e := *h.bookmark
	e.invalid = func(string) error { return nil }
	e.empty = func() error { return nil }
	for _, next := range []string{"bad*name", " "} {
		if err := mutateBookmarkWithEnvironment("Original", next, true, &e); err == nil {
			t.Fatal("bad error formatter accepted invalid name")
		}
	}
}
