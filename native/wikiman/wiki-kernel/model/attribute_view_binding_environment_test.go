package model

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"github.com/88250/lute/render"
	"wiki-kernel/av"
	"wiki-kernel/treenode"
)

func TestAVBindingOwnersHelpersOrderedMembersAndOriginalEffects(t *testing.T) {
	m, store, root := nameSource(t)
	for _, page := range []string{"1760000000000000002", "1760000000000000003"} {
		t.Run(page, func(t *testing.T) {
			t.Parallel()
			lease, err := m.Acquire(context.Background(), store, page)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(lease.Release)
			for _, loaded := range []bool{false, true} {
				h := newBoundHarness(t, lease, root, page, "")
				before := nameDigest(t, root, page)
				node, tree, err := h.cell.node(settingsBlock)
				if err != nil {
					t.Fatal(err)
				}
				node.SetIALAttr(av.NodeAttrNameAvs, cellDest+","+cellDest+","+cellAV)
				node.SetIALAttr(av.NodeAttrViewStaticText+"-"+cellAV, "static anchor")
				a, err := h.cell.read(cellRelated)
				if err != nil {
					t.Fatal(err)
				}
				a.Name = "" // The owner language fallback remains part of name formatting.
				// Helpers require only the binding projection, not unrelated cell services.
				projection := *h.cell
				projection.read, projection.stage, projection.groups = nil, nil, nil
				projection.related, projection.now, projection.newID, projection.color = nil, nil, nil, nil
				if loaded {
					projection.node = nil
				}
				h.env.avCell = &projection
				if loaded {
					err = bindBlockAv0(h.tx, cellRelated, node, tree)
				} else {
					err = bindBlockAv(h.tx, cellRelated, settingsBlock)
				}
				if err != nil {
					t.Fatal(err)
				}
				want := cellDest + "," + cellAV + "," + cellRelated
				if node.IALAttr(av.NodeAttrNameAvs) != want || h.attrs[settingsBlock][av.NodeAttrNameAvs] != want {
					t.Fatal("member order/dedup", node.KramdownIAL)
				}
				labels := h.attrs[settingsBlock][av.NodeAttrViewNames]
				if !strings.Contains(labels, page+" "+cellDest) || !strings.Contains(labels, page+" untitled") || strings.Index(labels, cellDest) > strings.Index(labels, cellAV) {
					t.Fatal("owner labels", labels)
				}
				if node.IALAttr("custom-preserved") != "untouched" || node.IALAttr(av.NodeAttrViewStaticText+"-"+cellAV) != "static anchor" || len(h.notifications) == 0 {
					t.Fatal("original attrs side effects lost", h.counts)
				}
				payload := h.notifications[0].DoOperations[0].Data.(map[string]interface{})
				if payload["old"].(map[string]string)[av.NodeAttrNameAvs] != cellDest+","+cellDest+","+cellAV || payload["new"].(map[string]string)[av.NodeAttrNameAvs] != want {
					t.Fatal("old/new notification alias", payload)
				}
				if loaded {
					projection.node = h.cell.node
				}
				if err := unbindBlockAv(h.tx, cellAV, settingsBlock); err != nil {
					t.Fatal(err)
				}
				if node.IALAttr(av.NodeAttrNameAvs) != cellDest+","+cellRelated {
					t.Fatal("unbind removed another database")
				}
				raw := render.NewJSONRenderer(tree, h.env.parser().RenderOptions).Render()
				if strings.Contains(string(raw), `"av-names"`) || !strings.Contains(string(raw), `"custom-avs"`) {
					t.Fatal("derived labels in authoritative serialization", string(raw))
				}
				if h.counts["finish"] != 0 || h.counts["id"] != 0 || h.counts["related"] != 0 || h.counts["groups:"+cellAV] != 0 {
					t.Fatal("helper expanded business effects", h.counts)
				}
				assertNameSource(t, h.cellHarness, before)
			}
		})
	}
}

func TestAVBindingContinuousCandidatesAndEmptyMemberRules(t *testing.T) {
	m, store, root := nameSource(t)
	page := "1760000000000000002"
	lease, err := m.Acquire(context.Background(), store, page)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(lease.Release)
	h := newBoundHarness(t, lease, root, page, "")
	before := nameDigest(t, root, page)
	node, tree, err := h.cell.node(settingsBlock)
	if err != nil {
		t.Fatal(err)
	}
	node.SetIALAttr(av.NodeAttrNameAvs, "")
	if err := unbindBlockAv(h.tx, cellAV, settingsBlock); err != nil {
		t.Fatal(err)
	}
	if h.counts["attrs"] != 0 || h.counts["names"] != 0 {
		t.Fatal("empty unbind wrote")
	}
	for _, id := range []string{cellAV, cellDest, cellAV} {
		if err := bindBlockAv(h.tx, id, settingsBlock); err != nil {
			t.Fatal(err)
		}
	}
	if node.IALAttr(av.NodeAttrNameAvs) != cellAV+","+cellDest || h.counts["attrs"] != 3 {
		t.Fatal("continuous candidates not reused", node.KramdownIAL, h.counts)
	}
	if err := unbindBlockAv(h.tx, cellAV, settingsBlock); err != nil {
		t.Fatal(err)
	}
	lastLabel := node.IALAttr(av.NodeAttrViewNames)
	if err := unbindBlockAv(h.tx, cellDest, settingsBlock); err != nil {
		t.Fatal(err)
	}
	// Original names contract: an empty result doesn't overwrite the old label.
	if node.IALAttr(av.NodeAttrNameAvs) != "" || node.IALAttr(av.NodeAttrViewNames) != lastLabel {
		t.Fatal("last member rule changed")
	}
	if h.tx.trees[settingsDoc] != tree {
		t.Fatal("lost candidate tree")
	}
	assertNameSource(t, h.cellHarness, before)
}

func TestAVBindingFailuresAndLateCancellation(t *testing.T) {
	m, store, root := nameSource(t)
	page := "1760000000000000002"
	lease, err := m.Acquire(context.Background(), store, page)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(lease.Release)
	for _, loaded := range []bool{false, true} {
		for _, point := range []string{"node", "load", "names", "read:" + cellRelated, "attrs", "tree-stage", "notify"} {
			if loaded && (point == "node" || point == "load") {
				continue
			}
			for _, prefix := range []string{"", "cancel-"} {
				t.Run(point+prefix+map[bool]string{true: "loaded"}[loaded], func(t *testing.T) {
					h := newBoundHarness(t, lease, root, page, "")
					before := nameDigest(t, root, page)
					node, tree, err := h.cell.node(settingsBlock)
					if err != nil {
						t.Fatal(err)
					}
					h.failure = prefix + point
					h.counts = map[string]int{}
					if !loaded {
						delete(h.tx.trees, settingsDoc)
					}
					if loaded {
						err = bindBlockAv0(h.tx, cellRelated, node, tree)
					} else {
						err = bindBlockAv(h.tx, cellRelated, settingsBlock)
					}
					if err == nil || (prefix != "" && !errors.Is(err, context.Canceled)) || h.committed || h.counts["finish"] != 0 || h.counts[point] == 0 {
						t.Fatal("failure lost", err, h.counts)
					}
					assertNameSource(t, h.cellHarness, before)
				})
			}
		}
	}
}

func TestAVBindingDependencyIdentityAndNoGlobalFallback(t *testing.T) {
	m, store, root := nameSource(t)
	page := "1760000000000000002"
	lease, err := m.Acquire(context.Background(), store, page)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(lease.Release)
	for _, scenario := range []string{"missing-env", "missing-context", "missing-node", "missing-names", "missing-attrs", "cancelled", "empty-id", "nil-node", "nil-tree", "nil-root", "wrong-node", "foreign-node", "wrong-tree", "empty-box", "empty-path"} {
		t.Run(scenario, func(t *testing.T) {
			h := newBoundHarness(t, lease, root, page, "")
			before := nameDigest(t, root, page)
			node, tree, err := h.cell.node(settingsBlock)
			if err != nil {
				t.Fatal(err)
			}
			id := cellRelated
			// A native transaction cannot opt into the legacy fallback via its slot.
			h.env.avCell.legacy = true
			switch scenario {
			case "missing-env":
				h.env.avCell = nil
			case "missing-context":
				h.env.ctx = nil
			case "missing-node":
				h.env.avCell.node = nil
			case "missing-names":
				h.env.avCell.names = nil
			case "missing-attrs":
				h.env.avCell.attrs = nil
			case "cancelled":
				h.cancel()
			case "empty-id":
				id = ""
			case "nil-node":
				node = nil
			case "nil-tree":
				tree = nil
			case "nil-root":
				tree.Root = nil
			case "wrong-node":
				copy := *node
				copy.ID = nameBlock
				node = &copy
			case "foreign-node":
				copy := *node
				node = &copy
			case "wrong-tree":
				tree.ID = nameDoc
			case "empty-box":
				tree.Box = ""
			case "empty-path":
				tree.Path = ""
			}
			if scenario == "missing-node" {
				err = bindBlockAv(h.tx, id, settingsBlock)
			} else {
				err = bindBlockAv0(h.tx, id, node, tree)
			}
			if err == nil || h.counts["attrs"] != 0 || h.counts["names"] != 0 {
				t.Fatal("invalid source/dependency admitted", err, h.counts)
			}
			assertNameSource(t, h.cellHarness, before)
		})
	}
}

func TestAVBindingLiveInsertAndCellRebindShareCandidates(t *testing.T) {
	m, store, root := nameSource(t)
	for _, page := range []string{"1760000000000000002", "1760000000000000003"} {
		lease, err := m.Acquire(context.Background(), store, page)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(lease.Release)
		h := newBoundHarness(t, lease, root, page, "")
		before := nameDigest(t, root, page)
		h.tx.DoOperations = []*Operation{boundInsert(boundInserted), cellOp("primary", map[string]interface{}{"isDetached": true, "block": map[string]interface{}{"id": settingsBlock, "content": "detached literal"}}), cellOp("primary", map[string]interface{}{"isDetached": false, "block": map[string]interface{}{"id": nameBlock, "content": "rebound literal"}})}
		if ret := performTx(h.tx); ret != nil || !h.committed {
			t.Fatal(ret, h.counts)
		}
		if slices.Contains(strings.Split(h.attrs[settingsBlock][av.NodeAttrNameAvs], ","), cellAV) || !slices.Contains(strings.Split(h.attrs[nameBlock][av.NodeAttrNameAvs], ","), cellAV) || !slices.Contains(strings.Split(h.attrs[settingsBlock][av.NodeAttrNameAvs], ","), cellDest) {
			t.Fatal("mixed candidates lost binding order", h.attrs)
		}
		if treenode.GetNodeInTree(h.tx.trees[settingsDoc], boundInserted) == nil || h.tx.trees[nameDoc] == nil || len(h.notifications) == 0 {
			t.Fatal("live effects missing")
		}
		assertNameSource(t, h.cellHarness, before)
	}
}

func TestAVBindingLegacySkipsAndBestEffortBoundary(t *testing.T) {
	node := &ast.Node{ID: settingsBlock, Type: ast.NodeParagraph}
	tree := &parse.Tree{}
	failure := errors.New("synthetic legacy attrs")
	attrsCalls, namesCalls := 0, 0
	e := &attributeViewCellEnvironment{ctx: context.Background(), legacy: true, node: func(string) (*ast.Node, *parse.Tree, error) { return node, tree, nil }, names: func(string) (string, error) { namesCalls++; return "", nil }, attrs: func(*ast.Node, *parse.Tree, map[string]string) error { attrsCalls++; return failure }}
	if err := e.binding(cellAV, settingsBlock, false); err != nil || attrsCalls != 0 || namesCalls != 0 {
		t.Fatal("empty unbind changed", err)
	}
	if err := e.binding(cellAV, settingsBlock, true); err != nil || attrsCalls != 1 {
		t.Fatal("legacy cell attrs no longer best effort", err)
	}
	if err := bindBlockAv0WithEnvironment(cellAV, node, tree, true, cellAttributeViewBindingEnvironment(e)); !errors.Is(err, failure) {
		t.Fatal("direct helper lost error", err)
	}
	e.names = func(string) (string, error) { return "", failure }
	if err := e.binding(cellAV, settingsBlock, true); !errors.Is(err, failure) {
		t.Fatal("names error swallowed", err)
	}
	e.node = func(string) (*ast.Node, *parse.Tree, error) { return nil, nil, failure }
	if err := e.binding(cellAV, settingsBlock, true); err != nil {
		t.Fatal("vanished legacy block no longer skipped", err)
	}
}

func TestAVBindingPostServiceCancellation(t *testing.T) {
	for _, point := range []string{"node", "names", "attrs"} {
		ctx, cancel := context.WithCancel(context.Background())
		root := &ast.Node{ID: settingsDoc, Type: ast.NodeDocument}
		node := &ast.Node{ID: settingsBlock, Type: ast.NodeParagraph}
		root.AppendChild(node)
		tree := &parse.Tree{ID: settingsDoc, Box: settingsBox, Path: "/" + settingsDoc + ".docj", Root: root}
		calls := []string{}
		mark := func(name string) {
			calls = append(calls, name)
			if name == point {
				cancel()
			}
		}
		e := &attributeViewBindingEnvironment{ctx: ctx, node: func(string) (*ast.Node, *parse.Tree, error) { mark("node"); return node, tree, nil }, names: func(string) (string, error) { mark("names"); return "owned", nil }, attrs: func(*ast.Node, *parse.Tree, map[string]string) error { mark("attrs"); return nil }}
		if err := bindBlockAvWithEnvironment(cellAV, settingsBlock, true, e); !errors.Is(err, context.Canceled) {
			t.Fatal("late cancel lost", point, err)
		}
		if calls[len(calls)-1] != point {
			t.Fatal("continued after cancellation", calls)
		}
		cancel()
	}
}
