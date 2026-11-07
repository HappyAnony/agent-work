package model

import (
	"context"
	"database/sql"
	"errors"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"reflect"
	"slices"
	"testing"
	"time"
	"wiki-kernel/internal/anchor"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/workspace"
	"wiki-kernel/task"
	"wiki-kernel/treenode"
)

type queuedSourceEvent struct {
	kind  string
	key   string
	delay time.Duration
	run   func() error
}
type sourceEventHarness struct {
	*tagMutationHarness
	queued        []queuedSourceEvent
	pushed        []string
	changes       []anchor.Change
	countsPayload []any
	source        *sourceEventEnvironment
}

func newSourceEventHarness(t *testing.T, l *workspace.Lease, root, page string) *sourceEventHarness {
	t.Helper()
	h := &sourceEventHarness{tagMutationHarness: newTagMutationHarness(t, l, root, page)}
	def, _, err := h.cell.node(settingsBlock)
	if err != nil {
		t.Fatal(err)
	}
	def.SetIALAttr("name", page+" owner definition")
	ref, _, err := h.cell.node(nameBlock)
	if err != nil {
		t.Fatal(err)
	}
	ref.AppendChild(&ast.Node{Type: ast.NodeTextMark, TextMarkType: "block-ref", TextMarkBlockRefID: settingsBlock, TextMarkBlockRefSubtype: "d", TextMarkTextContent: "old anchor"})
	if err = l.WithNativeTransaction(h.ctx, indexdb.Main, func(tx *sql.Tx) error {
		return indexdb.InsertBlockReferences(h.ctx, tx, []indexdb.RefRecord{
			{ID: "source-ref-1", DefBlockID: settingsBlock, DefBlockRootID: settingsDoc, DefBlockParentID: settingsDoc, BlockID: nameBlock, RootID: nameDoc, Box: settingsBox, Path: "/" + nameDoc + ".docj", Type: "ref-d"},
			{ID: "source-ref-2", DefBlockID: settingsBlock, DefBlockRootID: settingsDoc, DefBlockParentID: settingsDoc, BlockID: nameBlock, RootID: nameDoc, Box: settingsBox, Path: "/" + nameDoc + ".docj", Type: "ref-d"},
		})
	}); err != nil {
		t.Fatal(err)
	}
	query := func(id string) (ret []indexdb.RefRecord, err error) {
		err = l.WithNativeReferences(h.ctx, func(q indexdb.References) (err error) { ret, err = q.ByDefinition(h.ctx, id, false); return })
		return
	}
	a := anchor.Services{
		References: func(ctx context.Context, n *ast.Node) ([]indexdb.RefRecord, []*ast.Node, error) {
			if err := h.mark("event-anchor-query"); err != nil {
				return nil, nil, err
			}
			return anchor.References(ctx, n, func(_ context.Context, id string) ([]indexdb.RefRecord, error) { return query(id) })
		},
		Load: func(_ context.Context, id string) (*parse.Tree, error) {
			if err := h.mark("event-anchor-load"); err != nil {
				return nil, err
			}
			return h.tx.loadTree(id)
		},
		Text: func(_ context.Context, n *ast.Node) (string, error) {
			return n.IALAttr("name"), h.mark("event-anchor-text")
		},
		Set: func(n *ast.Node, s string) error {
			if err := h.mark("event-anchor-set"); err != nil {
				return err
			}
			return anchor.SetText(n, s, func(string, string) error { return nil })
		},
		Embedded: indexdb.EmbeddedReference,
		Notify: func(_ context.Context, c anchor.Change) error {
			if err := h.mark("event-anchor-notify"); err != nil {
				return err
			}
			h.changes = append(h.changes, c)
			return nil
		},
		IndexRefs: func(context.Context, *parse.Tree) error { return h.mark("event-anchor-index") },
		UpdateAVs: func(context.Context, map[string]*ast.Node) error {
			return errors.New("native used external AV/global callback")
		},
		Stage: func(_ context.Context, t *parse.Tree) error {
			if err := h.mark("event-anchor-stage"); err != nil {
				return err
			}
			return h.tx.writeTree(t)
		},
	}
	e := &sourceEventEnvironment{ctx: h.ctx, anchor: &a,
		flush: func() error { return h.mark("event-flush") },
		tree: func(id string) (*parse.Tree, error) {
			if err := h.mark("event-tree"); err != nil {
				return nil, err
			}
			return h.tx.loadTree(id)
		},
		lookup: func(id string) (*treenode.BlockTree, error) {
			if err := h.mark("event-lookup"); err != nil {
				return nil, err
			}
			if id == "missing" {
				return nil, nil
			}
			_, t, err := h.cell.node(id)
			if err != nil {
				return nil, err
			}
			return &treenode.BlockTree{ID: id, RootID: t.ID, BoxID: t.Box, Path: t.Path}, nil
		},
		children: func(id string) (ret []string, err error) {
			if err = h.mark("event-children"); err != nil {
				return
			}
			err = l.WithNativeReferences(h.ctx, func(q indexdb.References) (err error) { ret, err = q.ChildDefinitionIDs(h.ctx, id); return })
			return
		},
		references: func(id string, children bool) (ret []string, err error) {
			if err = h.mark("event-references"); err != nil {
				return
			}
			err = l.WithNativeReferences(h.ctx, func(q indexdb.References) (err error) { ret, err = q.RefIDs(h.ctx, id, children); return })
			return
		},
		blockTrees: func(ids []string) (ret map[string]*treenode.BlockTree, err error) {
			if err = h.mark("event-blocktrees"); err != nil {
				return
			}
			ret = map[string]*treenode.BlockTree{}
			for _, id := range ids {
				bt, e := h.env.sourceEvents.lookup(id)
				if e != nil {
					return nil, e
				}
				ret[id] = bt
			}
			return
		},
		enqueue: func(kind, key string, delay time.Duration, run func() error) error {
			if err := h.mark("event-enqueue"); err != nil {
				return err
			}
			h.queued = append(h.queued, queuedSourceEvent{kind, key, delay, run})
			return nil
		},
		pushReload: func(id string) error {
			if err := h.mark("event-push"); err != nil {
				return err
			}
			h.pushed = append(h.pushed, id)
			return nil
		},
		pushCount: func(root, block string, defs []string, count, rootCount int) error {
			if err := h.mark("event-count"); err != nil {
				return err
			}
			h.countsPayload = []any{root, block, defs, count, rootCount}
			return nil
		},
	}
	h.source = e
	h.env.sourceEvents = e
	return h
}

func TestSourceEventOwnersRealReferencesAnchorsAVAndCounts(t *testing.T) {
	m, store, root := nameSource(t)
	for _, page := range []string{"1760000000000000002", "1760000000000000003"} {
		l, err := m.Acquire(context.Background(), store, page)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(l.Release)
		h := newSourceEventHarness(t, l, root, page)
		before := nameDigest(t, root, page)
		if err = h.tx.reloadProtyle(settingsDoc); err != nil {
			t.Fatal(err)
		}
		if len(h.changes) != 1 || h.changes[0].Text != page+" owner definition" || h.changes[0].RootID != nameDoc || h.changes[0].BlockID != nameBlock || h.staged[cellAV] == nil {
			t.Fatal("anchor/AV source event not inherited", h.changes, h.staged)
		}
		if len(h.queued) != 2 || len(h.pushed) != 0 {
			t.Fatal("duplicate root or premature notification", h.queued, h.pushed)
		}
		for i, q := range h.queued {
			if q.kind != task.ReloadProtyle || q.delay != 200*time.Millisecond || q.key != []string{nameDoc, settingsDoc}[i] {
				t.Fatal(q)
			}
			if err = q.run(); err != nil {
				t.Fatal(err)
			}
		}
		if !slices.Equal(h.pushed, []string{nameDoc, settingsDoc}) {
			t.Fatal(h.pushed)
		}
		for _, id := range []string{settingsDoc, settingsBlock} {
			if err = h.tx.refreshRefCount(id); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(h.countsPayload, []any{settingsDoc, id, []string{settingsBlock}, 1, 1}) {
				t.Fatal("distinct reference/count rules", h.countsPayload)
			}
		}
		count := h.counts["event-count"]
		if err = h.tx.refreshRefCount("missing"); err != nil || h.counts["event-count"] != count {
			t.Fatal(err)
		}
		assertNameSource(t, h.cellHarness, before)
	}
}

func TestSourceEventFailuresCancellationAndQueuedLifetime(t *testing.T) {
	m, store, root := nameSource(t)
	page := "1760000000000000002"
	l, err := m.Acquire(context.Background(), store, page)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(l.Release)
	for _, point := range []string{"event-tree", "event-children", "event-anchor-query", "event-anchor-load", "event-anchor-text", "event-anchor-set", "event-anchor-notify", "event-anchor-index", "event-anchor-stage", "tag-av-stage", "event-references", "event-blocktrees", "event-enqueue"} {
		for _, prefix := range []string{"", "cancel-"} {
			t.Run(prefix+point, func(t *testing.T) {
				h := newSourceEventHarness(t, l, root, page)
				before := nameDigest(t, root, page)
				h.failure = prefix + point
				if err := h.tx.reloadProtyle(settingsDoc); err == nil || h.counts[point] == 0 {
					t.Fatal("failure lost", err, point)
				}
				if len(h.pushed) != 0 || h.counts["finish"] != 0 {
					t.Fatal("published failed candidate")
				}
				assertNameSource(t, h.cellHarness, before)
			})
		}
	}
	for _, point := range []string{"event-flush", "event-lookup", "event-references", "event-children", "event-count"} {
		t.Run(point, func(t *testing.T) {
			h := newSourceEventHarness(t, l, root, page)
			h.failure = point
			if err := h.tx.refreshRefCount(settingsDoc); err == nil || h.counts[point] == 0 {
				t.Fatal(err, point)
			}
		})
	}
	h := newSourceEventHarness(t, l, root, page)
	if err = h.tx.reloadProtyle(settingsDoc); err != nil {
		t.Fatal(err)
	}
	h.cancel()
	for _, q := range h.queued {
		if err = q.run(); !errors.Is(err, context.Canceled) {
			t.Fatal("late task escaped context", err)
		}
	}
	if len(h.pushed) != 0 {
		t.Fatal("expired owner emitted reload")
	}
}

func TestSourceEventLateNilCancellationAndDependencyValidation(t *testing.T) {
	m, store, root := nameSource(t)
	page := "1760000000000000002"
	l, err := m.Acquire(context.Background(), store, page)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(l.Release)
	for _, scenario := range []string{"no-owner", "no-av", "no-anchor", "no-text", "bad-tree", "bad-blocktree", "bad-count-owner", "late-tree", "late-text", "late-enqueue", "late-count", "pre-cancel"} {
		t.Run(scenario, func(t *testing.T) {
			h := newSourceEventHarness(t, l, root, page)
			before := nameDigest(t, root, page)
			switch scenario {
			case "no-owner":
				h.env.sourceEvents = nil
			case "no-av":
				h.env.avText = nil
			case "no-anchor":
				h.source.anchor = nil
			case "no-text":
				h.source.anchor.Text = nil
			case "bad-tree":
				h.source.tree = func(string) (*parse.Tree, error) {
					return &parse.Tree{ID: "wrong", Root: &ast.Node{ID: "wrong", Type: ast.NodeDocument}}, nil
				}
			case "bad-blocktree":
				h.source.blockTrees = func([]string) (map[string]*treenode.BlockTree, error) {
					return map[string]*treenode.BlockTree{"bad": nil}, nil
				}
			case "bad-count-owner":
				h.source.lookup = func(string) (*treenode.BlockTree, error) {
					return &treenode.BlockTree{ID: "foreign", RootID: settingsDoc}, nil
				}
			case "late-tree":
				h.source.tree = func(string) (*parse.Tree, error) { h.cancel(); return nil, nil }
			case "late-text":
				h.source.anchor.Text = func(context.Context, *ast.Node) (string, error) { h.cancel(); return "late", nil }
			case "late-enqueue":
				h.source.enqueue = func(string, string, time.Duration, func() error) error { h.cancel(); return nil }
			case "late-count":
				h.source.pushCount = func(string, string, []string, int, int) error { h.cancel(); return nil }
			case "pre-cancel":
				h.cancel()
			}
			var err error
			if scenario == "bad-count-owner" || scenario == "late-count" {
				err = h.tx.refreshRefCount(settingsDoc)
			} else {
				err = h.tx.reloadProtyle(settingsDoc)
			}
			if err == nil {
				t.Fatal("invalid/late owner accepted")
			}
			if scenario == "late-text" && h.counts["event-anchor-set"] != 0 {
				t.Fatal("setter ran after late cancel")
			}
			assertNameSource(t, h.cellHarness, before)
		})
	}
}

func TestSourceEventMissingTreeAndOwnRootMultiplicity(t *testing.T) {
	m, store, root := nameSource(t)
	page := "1760000000000000002"
	l, err := m.Acquire(context.Background(), store, page)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(l.Release)
	h := newSourceEventHarness(t, l, root, page)
	h.source.tree = func(string) (*parse.Tree, error) { return nil, nil }
	h.source.blockTrees = func([]string) (map[string]*treenode.BlockTree, error) {
		return map[string]*treenode.BlockTree{"a": {RootID: settingsDoc}, "b": {RootID: settingsDoc}}, nil
	}
	if err = h.tx.reloadProtyle(settingsDoc); err != nil {
		t.Fatal(err)
	}
	for _, q := range h.queued {
		if err = q.run(); err != nil {
			t.Fatal(err)
		}
	}
	if !slices.Equal(h.pushed, []string{settingsDoc, settingsDoc}) {
		t.Fatal("own-root event deduplicated against embeds", h.pushed)
	}
	// Refcount needs no parser/anchor/AV/scheduler dependency.
	h.env.avText = nil
	h.source.anchor = nil
	h.source.enqueue = nil
	h.source.tree = nil
	if err = h.tx.refreshRefCount(settingsDoc); err != nil {
		t.Fatal("unrelated dependencies required", err)
	}
}

func TestSourceEventLegacySchedulerRetainsRootDeduplicationKeys(t *testing.T) {
	delayedUIOwnerFixture(t)
	e := legacySourceEventEnvironment()
	delivered := make(chan string, 3)
	for _, id := range []string{"root-a", "root-a", "root-b"} {
		id := id
		if err := e.enqueue(task.ReloadProtyle, id, -time.Millisecond, func() error { delivered <- id; return nil }); err != nil {
			t.Fatal(err)
		}
	}
	task.ExecAsyncTaskJob()
	got := []string{}
	for len(got) < 2 {
		select {
		case id := <-delivered:
			got = append(got, id)
		case <-time.After(time.Second):
			t.Fatal("different roots coalesced", got)
		}
	}
	slices.Sort(got)
	if !slices.Equal(got, []string{"root-a", "root-b"}) {
		t.Fatal(got)
	}
	select {
	case id := <-delivered:
		t.Fatal("same root was not coalesced", id)
	default:
	}
}

func TestSourceEventTagMutationComposesOwnerReload(t *testing.T) {
	m, store, root := nameSource(t)
	page := "1760000000000000002"
	l, err := m.Acquire(context.Background(), store, page)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(l.Release)
	h := newSourceEventHarness(t, l, root, page)
	before := nameDigest(t, root, page)
	h.env.tagMutation.reload = h.tx.reloadProtyle
	if err = h.tx.renameTag("AGI", "Target"); err != nil {
		t.Fatal(err)
	}
	if len(h.changes) != 1 || h.changes[0].Text != page+" owner definition" || len(h.queued) < 2 || len(h.pushed) != 0 {
		t.Fatal("owner chain bypassed", h.changes, h.queued, h.pushed)
	}
	if h.counts["tag-clear:true"] != 1 || h.counts["finish"] != 0 {
		t.Fatal(h.counts)
	}
	assertNameSource(t, h.cellHarness, before)
}
