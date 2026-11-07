package model

import (
	"context"
	"errors"
	"fmt"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"reflect"
	"testing"
	"wiki-kernel/internal/workspace"
)

type blockAttributeHarness struct {
	*sourceEventHarness
	jobs       []func(context.Context) error
	payloads   [][]*Transaction
	staged     []string
	attributes *blockAttributeEnvironment
}

func newBlockAttributeHarness(t *testing.T, l *workspace.Lease, root, page string) *blockAttributeHarness {
	h := &blockAttributeHarness{sourceEventHarness: newSourceEventHarness(t, l, root, page)}
	h.source.launch = func(fn func(context.Context) error) error {
		if err := h.mark("attrs-launch"); err != nil {
			return err
		}
		h.jobs = append(h.jobs, fn)
		return nil
	}
	e := &blockAttributeEnvironment{ctx: h.ctx, gate: newAttributeMutationGate(),
		wait: func() error { return h.mark("attrs-wait") },
		load: func(id string) (*parse.Tree, error) {
			if err := h.mark("attrs-load"); err != nil {
				return nil, err
			}
			return h.tx.loadTree(id)
		},
		loadMany: func(ids []string) (ret map[string]*parse.Tree, err error) {
			if err = h.mark("attrs-load-many"); err != nil {
				return
			}
			ret = map[string]*parse.Tree{}
			for _, id := range ids {
				ret[id], err = h.tx.loadTree(id)
				if err != nil {
					return nil, err
				}
			}
			return
		},
		stage: func(tree *parse.Tree) error {
			if err := h.mark("attrs-stage"); err != nil {
				return err
			}
			h.staged = append(h.staged, tree.ID)
			return h.tx.writeTree(tree)
		},
		cache: func(id string, attrs map[string]string) error {
			if err := h.mark("attrs-cache"); err != nil {
				return err
			}
			h.attrs[id] = copyBlockAttributes(attrs)
			return nil
		},
		sync: func() error { return h.mark("attrs-sync") },
		notify: func(old map[string]string, n *ast.Node) error {
			if err := h.mark("attrs-notify"); err != nil {
				return err
			}
			h.payloads = append(h.payloads, blockAttrTransactions(old, n))
			return nil
		},
		invalid: func(id string) error { return fmt.Errorf("%s invalid %s", page, id) }, missing: func(id string) error { return fmt.Errorf("%s missing %s", page, id) },
		tags:       func() error { return h.mark("attrs-tags") },
		background: func(string, string) error { return errors.New("native used external attribute background") },
	}
	h.source.attributeGate = e.gate
	h.attributes = e
	h.env.blockAttributes = e
	return h
}

func TestBlockAttributesOwnersOriginalRulesAndActualAnchorAVRefresh(t *testing.T) {
	m, store, root := nameSource(t)
	for _, page := range []string{"1760000000000000002", "1760000000000000003"} {
		l, err := m.Acquire(context.Background(), store, page)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(l.Release)
		h := newBlockAttributeHarness(t, l, root, page)
		before := nameDigest(t, root, page)
		input := map[string]string{"NAME": "  " + page + " new definition,  ", "CUSTOM-MIXED": "  retained,  ", "tags": "one, one, two"}
		copy := copyBlockAttributes(input)
		if err = h.tx.setBlockAttributes(settingsBlock, input); err != nil {
			t.Fatal(err)
		}
		n, _, _ := h.cell.node(settingsBlock)
		if n.IALAttr("name") != page+" new definition" || n.IALAttr("custom-mixed") != "retained" || n.IALAttr("tags") != "one,two" || !reflect.DeepEqual(input, copy) {
			t.Fatal("original normalization or native input isolation", n.KramdownIAL, input)
		}
		if len(h.jobs) != 1 || len(h.changes) != 0 || len(h.payloads) != 1 || h.payloads[0][0].DoOperations[0].Action != "updateAttrs" || h.counts["attrs-sync"] != 1 {
			t.Fatal("write/event/background phases", h.counts)
		}
		if err = h.jobs[0](h.ctx); err != nil {
			t.Fatal(err)
		}
		if len(h.changes) != 1 || h.changes[0].Text != page+" new definition" || h.cellHarness.staged[cellAV] == nil {
			t.Fatal("original references/AV chain", h.changes)
		}
		if err = h.tx.setBlockAttributes(settingsBlock, map[string]string{"custom-mixed": "", "tags": ""}); err != nil {
			t.Fatal(err)
		}
		if n.IALAttr("custom-mixed") != "" || n.IALAttr("tags") != "" {
			t.Fatal("empty attrs not removed")
		}
		assertNameSource(t, h.cellHarness, before)
	}
}

func TestBlockAttributeBatchSharesRootPreservesOrderAndNoBackground(t *testing.T) {
	m, store, root := nameSource(t)
	page := "1760000000000000002"
	l, err := m.Acquire(context.Background(), store, page)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(l.Release)
	h := newBlockAttributeHarness(t, l, root, page)
	before := nameDigest(t, root, page)
	input := []map[string]interface{}{{"id": settingsBlock, "attrs": map[string]string{"NAME": "First"}}, {"id": cellSecond, "attrs": map[string]string{"memo": "Other block"}}, {"id": settingsBlock, "attrs": map[string]string{"name": "Last"}}, {"id": nameBlock, "attrs": map[string]string{"alias": "Cross document"}}}
	if err = h.tx.batchSetBlockAttributes(input); err != nil {
		t.Fatal(err)
	}
	n, _, _ := h.cell.node(settingsBlock)
	other, _, _ := h.cell.node(cellSecond)
	if n.IALAttr("name") != "Last" || other.IALAttr("memo") != "Other block" || len(h.staged) != 2 || len(h.payloads) != 4 || len(h.jobs) != 0 || h.counts["attrs-sync"] != 1 {
		t.Fatal("batch roots/order/side effects", h.staged, h.counts)
	}
	if h.payloads[0][0].DoOperations[0].Data.(map[string]interface{})["new"].(map[string]string)["name"] != "First" {
		t.Fatal("earlier event payload mutated")
	}
	// The transaction-local helper retains its original no-background behavior.
	if err = setNodeAttrsWithTx(h.tx, n, h.tx.trees[settingsDoc], map[string]string{"memo": "In transaction"}); err != nil || len(h.jobs) != 0 {
		t.Fatal(err)
	}
	assertNameSource(t, h.cellHarness, before)
}

func TestBlockAttributeRefreshReadsLatestTreeAndDropsRemovedNodes(t *testing.T) {
	m, store, root := nameSource(t)
	page := "1760000000000000002"
	l, err := m.Acquire(context.Background(), store, page)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(l.Release)
	h := newBlockAttributeHarness(t, l, root, page)
	before := nameDigest(t, root, page)
	latestTree := h.tx.trees[settingsDoc]
	h.source.tree = func(id string) (*parse.Tree, error) {
		if id != settingsDoc {
			t.Fatal(id)
		}
		return latestTree, nil
	}
	if err = h.tx.setBlockAttributes(settingsBlock, map[string]string{"name": "Queued old"}); err != nil {
		t.Fatal(err)
	}
	// Replace the tree the owner will read, rather than mutating the queued AST.
	old := h.tx.trees[settingsDoc]
	fresh := &parse.Tree{ID: old.ID, Box: old.Box, Path: old.Path, Root: &ast.Node{ID: old.ID, Type: ast.NodeDocument}}
	n := &ast.Node{ID: settingsBlock, Type: ast.NodeParagraph}
	n.SetIALAttr("name", "Latest owner value")
	fresh.Root.AppendChild(n)
	latestTree = fresh
	if err = h.jobs[0](h.ctx); err != nil {
		t.Fatal(err)
	}
	if len(h.changes) != 1 || h.changes[0].Text != "Latest owner value" {
		t.Fatal("stale handler AST used", h.changes)
	}
	n.Unlink()
	changes := len(h.changes)
	if err = h.jobs[0](h.ctx); err != nil || len(h.changes) != changes {
		t.Fatal("removed block refreshed", err)
	}
	assertNameSource(t, h.cellHarness, before)
}

func TestBlockAttributesFailureMissingServicesAndCancellation(t *testing.T) {
	m, store, root := nameSource(t)
	page := "1760000000000000002"
	l, err := m.Acquire(context.Background(), store, page)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(l.Release)
	for _, point := range []string{"attrs-wait", "attrs-load", "attrs-tags", "attrs-stage", "attrs-sync", "attrs-cache", "attrs-notify", "attrs-launch"} {
		for _, prefix := range []string{"", "cancel-"} {
			t.Run(prefix+point, func(t *testing.T) {
				h := newBlockAttributeHarness(t, l, root, page)
				before := nameDigest(t, root, page)
				h.failure = prefix + point
				if err := h.tx.setBlockAttributes(settingsBlock, map[string]string{"name": "Value", "tags": "new"}); err == nil || h.counts[point] == 0 {
					t.Fatal(point, err)
				}
				if h.counts["finish"] != 0 {
					t.Fatal("candidate committed")
				}
				assertNameSource(t, h.cellHarness, before)
			})
		}
	}
	for _, scenario := range []string{"no-owner", "no-source", "no-launch", "no-av", "invalid-name", "missing-node", "bad-tree", "late-notify", "readonly", "readonly-cancelled"} {
		t.Run(scenario, func(t *testing.T) {
			h := newBlockAttributeHarness(t, l, root, page)
			attrs := map[string]string{"name": "Value"}
			id := settingsBlock
			switch scenario {
			case "no-owner":
				h.env.blockAttributes = nil
			case "no-source":
				h.env.sourceEvents = nil
			case "no-launch":
				h.source.launch = nil
			case "no-av":
				h.env.avText = nil
			case "invalid-name":
				attrs = map[string]string{"invalid key": "value"}
			case "missing-node":
				id = "not-in-tree"
				h.attributes.load = func(string) (*parse.Tree, error) { return h.tx.loadTree(settingsBlock) }
			case "bad-tree":
				h.attributes.load = func(string) (*parse.Tree, error) {
					return &parse.Tree{ID: "foreign", Root: &ast.Node{ID: "foreign", Type: ast.NodeDocument}}, nil
				}
			case "late-notify":
				h.attributes.notify = func(map[string]string, *ast.Node) error { h.cancel(); return nil }
			case "readonly", "readonly-cancelled":
				h.env.blockAttributes = &blockAttributeEnvironment{readOnly: true}
				h.env.sourceEvents = nil
				if scenario == "readonly-cancelled" {
					h.cancel()
				}
			}
			err := h.tx.setBlockAttributes(id, attrs)
			if scenario == "readonly" {
				if err != nil || len(h.jobs) != 0 {
					t.Fatal(err)
				}
			} else if err == nil {
				t.Fatal("invalid service/identity accepted")
			}
			if scenario == "late-notify" && len(h.jobs) != 0 {
				t.Fatal("scheduled after cancellation")
			}
		})
	}
}

func TestBlockAttributeJobOwnerCancellationLateErrorsAndNoFinish(t *testing.T) {
	m, store, root := nameSource(t)
	page := "1760000000000000002"
	l, err := m.Acquire(context.Background(), store, page)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(l.Release)
	for _, scenario := range []string{"owner-cancel", "job-cancel", "late-job-cancel", "flush", "tree", "anchor-text", "late-av-text", "bad-tree", "nil-job"} {
		t.Run(scenario, func(t *testing.T) {
			h := newBlockAttributeHarness(t, l, root, page)
			before := nameDigest(t, root, page)
			if err := h.tx.setBlockAttributes(settingsBlock, map[string]string{"name": "Scheduled"}); err != nil {
				t.Fatal(err)
			}
			job, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch scenario {
			case "owner-cancel":
				h.cancel()
			case "job-cancel":
				cancel()
			case "late-job-cancel":
				h.source.tree = func(string) (*parse.Tree, error) { cancel(); return h.tx.loadTree(settingsBlock) }
			case "flush":
				h.failure = "event-flush"
			case "tree":
				h.failure = "event-tree"
			case "anchor-text":
				h.failure = "event-anchor-text"
			case "late-av-text":
				h.env.avText.text = func(*ast.Node, string) (string, string, error) { cancel(); return "", "late", nil }
			case "bad-tree":
				h.source.tree = func(string) (*parse.Tree, error) {
					return &parse.Tree{ID: "foreign", Root: &ast.Node{ID: "foreign", Type: ast.NodeDocument}}, nil
				}
			case "nil-job":
				job = nil
			}
			if scenario == "owner-cancel" {
				if err := h.jobs[0](job); !errors.Is(err, context.Canceled) || len(h.changes) != 0 {
					t.Fatal(err, h.changes)
				}
				assertNameSource(t, h.cellHarness, before)
				return
			}
			// Services were captured at admission; mutate the captured source through
			// a second admission for deliberate callback fault injection.
			e, err := h.tx.sourceEventServices(true)
			if err != nil {
				t.Fatal(err)
			}
			h.jobs = nil
			if err = queueAttributeReferences(settingsBlock, settingsDoc, e); err != nil {
				t.Fatal(err)
			}
			if err = h.jobs[0](job); err == nil || (scenario != "late-av-text" && len(h.changes) != 0) || h.counts["finish"] != 0 {
				t.Fatal("job failure lost", err, h.changes)
			}
			if scenario == "late-av-text" && h.counts["tag-av-stage"] != 0 {
				t.Fatal("AV staged after job cancellation")
			}
			assertNameSource(t, h.cellHarness, before)
		})
	}
}

func TestBlockAttributeBatchRejectsBadInputsAndSplitCandidates(t *testing.T) {
	m, store, root := nameSource(t)
	page := "1760000000000000002"
	l, err := m.Acquire(context.Background(), store, page)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(l.Release)
	for _, scenario := range []string{"bad-id", "bad-attrs", "missing", "split-root", "load-error", "stage-error", "nil-invalid-formatter", "nil-missing-formatter"} {
		t.Run(scenario, func(t *testing.T) {
			h := newBlockAttributeHarness(t, l, root, page)
			before := nameDigest(t, root, page)
			input := []map[string]interface{}{{"id": settingsBlock, "attrs": map[string]string{"name": "First"}}, {"id": cellSecond, "attrs": map[string]string{"name": "Second"}}}
			switch scenario {
			case "bad-id":
				input[0]["id"] = 42
			case "bad-attrs":
				input[0]["attrs"] = map[string]interface{}{"name": true}
			case "missing", "nil-missing-formatter":
				h.attributes.loadMany = func([]string) (map[string]*parse.Tree, error) { return nil, nil }
				if scenario == "nil-missing-formatter" {
					h.attributes.missing = func(string) error { return nil }
				}
			case "split-root":
				h.attributes.loadMany = func([]string) (map[string]*parse.Tree, error) {
					tree := h.tx.trees[settingsDoc]
					copy := *tree
					return map[string]*parse.Tree{settingsBlock: tree, cellSecond: &copy}, nil
				}
			case "load-error":
				h.failure = "attrs-load-many"
			case "stage-error":
				h.failure = "attrs-stage"
			case "nil-invalid-formatter":
				input[0]["attrs"] = map[string]string{"bad name": "value"}
				h.attributes.invalid = func(string) error { return nil }
			}
			if err := h.tx.batchSetBlockAttributes(input); err == nil {
				t.Fatal("invalid batch accepted")
			}
			if len(h.jobs) != 0 || h.counts["finish"] != 0 {
				t.Fatal("background/commit from batch")
			}
			assertNameSource(t, h.cellHarness, before)
		})
	}
}

func TestBlockAttributeRefreshSerializesForegroundAndCancelledWaiters(t *testing.T) {
	m, store, root := nameSource(t)
	page := "1760000000000000002"
	l, err := m.Acquire(context.Background(), store, page)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(l.Release)
	h := newBlockAttributeHarness(t, l, root, page)
	before := nameDigest(t, root, page)
	entered, release := make(chan struct{}), make(chan struct{})
	h.source.flush = func() error { close(entered); <-release; return nil }
	if err = h.tx.setBlockAttributes(settingsBlock, map[string]string{"name": "Before queued refresh"}); err != nil {
		t.Fatal(err)
	}
	jobDone := make(chan error, 1)
	go func() { jobDone <- h.jobs[0](h.ctx) }()
	<-entered
	// A cancelled waiter cannot run under, or after, this active refresh.
	cancelled, cancel := context.WithCancel(h.ctx)
	cancel()
	if err = h.attributes.gate.with(cancelled, func() error { t.Error("cancelled waiter ran"); return nil }); !errors.Is(err, context.Canceled) {
		t.Error(err)
	}
	waiting := make(chan struct{})
	h.attributes.wait = func() error { close(waiting); return nil }
	foreground := make(chan error, 1)
	go func() {
		foreground <- h.tx.setBlockAttributes(settingsBlock, map[string]string{"name": "After refresh"})
	}()
	<-waiting
	select {
	case err := <-foreground:
		t.Error("foreground bypassed active refresh", err)
	default:
	}
	close(release)
	if err = <-jobDone; err != nil {
		t.Fatal(err)
	}
	if err = <-foreground; err != nil {
		t.Fatal(err)
	}
	n, _, err := h.cell.node(settingsBlock)
	if err != nil || n.IALAttr("name") != "After refresh" {
		t.Fatal("stale refresh overwrote new attribute", err)
	}
	assertNameSource(t, h.cellHarness, before)
}
