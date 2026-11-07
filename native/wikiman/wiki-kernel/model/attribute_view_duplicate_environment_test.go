package model

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"wiki-kernel/av"
	"wiki-kernel/internal/workspace"
	"wiki-kernel/treenode"
)

type duplicateHarness struct {
	*cellHarness
	raw   map[string][]byte
	reg   map[string]string
	trees map[string]*parse.Tree
}

func newDuplicateHarness(t *testing.T, l *workspace.Lease, root, page, failure string) *duplicateHarness {
	h := newNameHarness(t, l, root, page, failure)
	d := &duplicateHarness{h, map[string][]byte{}, map[string]string{}, map[string]*parse.Tree{}}
	e := &attributeViewDuplicateEnvironment{ctx: h.ctx,
		source: func(id string) (*av.AttributeView, []byte, error) {
			if err := h.mark("duplicate-source"); err != nil {
				return nil, nil, err
			}
			a, err := h.cell.read(id) // Real owner reader on first access; then candidates.
			if err != nil {
				return nil, nil, err
			}
			raw, err := json.Marshal(a)
			return a, raw, err
		}, usedID: func(id string) (bool, error) {
			if err := h.mark("duplicate-used"); err != nil {
				return false, err
			}
			if h.cache[id] != nil || d.reg[id] != "" || h.env.lookup(id) != nil {
				return true, nil
			}
			for _, block := range d.reg {
				if id == block {
					return true, nil
				}
			}
			return false, nil
		}, newID: func() string { h.mark("duplicate-id"); return h.cell.newID() },
		now: func() time.Time {
			h.mark("duplicate-clock")
			return time.Date(2026, 10, 24, 23, 45, 6, 0, time.FixedZone("owner", 8*60*60))
		},
		registry: func(id, block string) error {
			if err := h.mark("duplicate-registry"); err != nil {
				return err
			}
			d.reg[id] = block
			return nil
		}, stage: func(id string, raw []byte) error {
			if err := h.mark("duplicate-stage"); err != nil {
				return err
			}
			a := &av.AttributeView{}
			if err := json.Unmarshal(raw, a); err != nil {
				return err
			}
			d.raw[id] = slices.Clone(raw)
			h.cache[id], h.staged[id] = a, a
			return nil
		},
		bindings: &attributeViewDuplicateBindingsEnvironment{
			exists: h.env.avName.exists,
			// Deliberately return independent trees for each mirror load.
			node: func(id string) (*ast.Node, *parse.Tree, error) {
				if err := h.mark("duplicate-node"); err != nil {
					return nil, nil, err
				}
				bt := h.env.lookup(id)
				if tree := d.trees[bt.RootID]; tree != nil {
					return treenode.GetNodeInTree(tree, id), tree, nil
				}
				tree, err := h.env.load(bt.BoxID, bt.Path, h.env.parser())
				if err != nil {
					return nil, nil, err
				}
				return treenode.GetNodeInTree(tree, id), tree, nil
			}, name: h.env.avName.name, untitled: h.env.avName.untitled,
			attrs: func(n *ast.Node, attrs map[string]string) (map[string]string, error) {
				if err := h.mark("duplicate-attrs"); err != nil {
					return nil, err
				}
				return setNodeAttrs0WithServices(n, attrs, func(string) error { return errors.New("invalid attr") }, func() error { return h.mark("tags") })
			}, cache: h.env.avName.cache, notify: h.env.avName.notify,
			tree: func(tree *parse.Tree) error {
				if err := h.mark("duplicate-tree"); err != nil {
					return err
				}
				if err := h.env.stage(tree); err != nil {
					return err
				}
				d.trees[tree.ID] = tree
				return nil
			},
		},
	}
	h.env.avDuplicate = e
	return d
}

func TestAVDuplicateOwnersLayoutsAndBindings(t *testing.T) {
	m, store, root := nameSource(t)
	for _, page := range []string{"1760000000000000002", "1760000000000000003"} {
		t.Run(page, func(t *testing.T) {
			t.Parallel()
			l, err := m.Acquire(context.Background(), store, page)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(l.Release)
			for _, layout := range []av.LayoutType{av.LayoutTypeTable, av.LayoutTypeGallery, av.LayoutTypeKanban} {
				t.Run(string(layout), func(t *testing.T) {
					d := newDuplicateHarness(t, l, root, page, "")
					a := orderPrepare(t, d.cellHarness, layout)
					for _, v := range a.Views {
						v.Group = &av.ViewGroup{Field: "status", Method: av.GroupMethodValue}
						v.Groups = []*av.View{{ID: "saved-group", Name: "Saved group", GroupItemIDs: []string{settingsBlock}}}
					}
					a.Name = page + " original " + cellAV
					a.KeyValues[1].Values[0].Text.Content = "raw " + cellAV + " suffix"
					// Bound row identity intentionally differs from its primary payload ID.
					a.GetBlockKeyValues().Values[0].Block.ID = "not-the-row-identity"
					before, input := nameDigest(t, root, page), mustDuplicateJSON(t, a)
					id, block, err := d.tx.duplicateDatabaseBlock(cellAV)
					if err != nil {
						t.Fatal(err, d.counts)
					}
					copy := d.staged[id]
					if id == cellAV || block == id || d.reg[id] != block || copy == nil || copy.ID != id || copy.Name != a.Name+" (Duplicated 2026-10-24 23:45:06)" {
						t.Fatal("duplicate identity/name", id, block, copy)
					}
					if copy.KeyValues[1].Values[0].Text.Content != "raw "+id+" suffix" || copy.Spec != a.Spec || copy.ViewID != a.ViewID || len(copy.Views) != len(a.Views) || copy.Views[0].LayoutType != a.Views[0].LayoutType || !slices.Equal(copy.Views[0].ItemIDs, a.Views[0].ItemIDs) || !slices.Equal(copy.Views[0].Groups[0].GroupItemIDs, a.Views[0].Groups[0].GroupItemIDs) {
						t.Fatal("raw replacement/layout/order changed")
					}
					for _, kv := range copy.KeyValues {
						if kv.Key.Relation != nil && (kv.Key.Relation.IsTwoWay || kv.Key.Relation.BackKeyID != "") {
							t.Fatal("two-way relation not disconnected")
						}
						if kv.Key.ID == "back" && kv.Key.Relation.AvID != id {
							t.Fatal("self relation ID not rewritten")
						}
					}
					if len(d.trees) != 2 || d.counts["duplicate-tree"] != 2 || d.counts["duplicate-clock"] != 1 || d.counts["duplicate-id"] != 2 || d.counts["duplicate-registry"] != 1 || d.counts["duplicate-stage"] != 1 {
						t.Fatal("effect counts", d.counts, len(d.trees))
					}
					for _, v := range copy.GetBlockKeyValues().Values {
						if v.IsDetached {
							continue
						}
						var n *ast.Node
						for _, tree := range d.trees {
							if next := treenode.GetNodeInTree(tree, v.BlockID); next != nil {
								n = next
							}
						}
						if n == nil || n.IALAttr(av.NodeAttrNameAvs) != cellAV+","+cellDest+","+id || n.IALAttr(av.NodeAttrViewNames) != "" || !strings.Contains(d.attrs[n.ID][av.NodeAttrViewNames], copy.Name) || !strings.Contains(d.attrs[n.ID][av.NodeAttrViewNames], page) {
							t.Fatal("binding/candidate name cache", v.BlockID, d.attrs)
						}
					}
					if !slices.Equal(input, mustDuplicateJSON(t, a)) || len(d.notifications) == 0 || d.counts["tags"] != 0 || d.counts["related"] != 0 {
						t.Fatal("source mutation or unrelated effects", d.counts)
					}
					copy.KeyValues[1].Values[0].Text.Content = "copy-only"
					if !slices.Equal(input, mustDuplicateJSON(t, a)) {
						t.Fatal("copy aliases original")
					}
					assertNameSource(t, d.cellHarness, before)
				})
			}
		})
	}
}

func mustDuplicateJSON(t *testing.T, a *av.AttributeView) []byte {
	t.Helper()
	raw, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestAVDuplicateEmptyDetachedMissingAndSequential(t *testing.T) {
	m, store, root := nameSource(t)
	page := "1760000000000000002"
	l, err := m.Acquire(context.Background(), store, page)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(l.Release)
	for _, scenario := range []string{"empty-name", "empty-values", "detached", "vanished", "missing-membership", "one-way", "sequential"} {
		t.Run(scenario, func(t *testing.T) {
			d := newDuplicateHarness(t, l, root, page, "")
			a, err := d.cell.read(cellAV)
			if err != nil {
				t.Fatal(err)
			}
			switch scenario {
			case "empty-name":
				a.Name = ""
			case "empty-values":
				a.GetBlockKeyValues().Values = nil
			case "detached":
				for _, v := range a.GetBlockKeyValues().Values {
					v.IsDetached = true
				}
			case "vanished":
				d.env.avDuplicate.bindings.exists = func(string) (bool, error) { return false, nil }
			case "one-way":
				a.KeyValues[6].Key.Relation.IsTwoWay = false
			case "missing-membership":
				load := d.env.load
				d.env.load = func(box, p string, engine *lute.Lute) (*parse.Tree, error) {
					tree, err := load(box, p, engine)
					if err == nil {
						for _, id := range []string{settingsBlock, cellSecond, nameBlock} {
							if n := treenode.GetNodeInTree(tree, id); n != nil {
								n.RemoveIALAttr(av.NodeAttrNameAvs)
							}
						}
					}
					return tree, err
				}
			}
			before := nameDigest(t, root, page)
			id, _, err := d.tx.duplicateDatabaseBlock(cellAV)
			if err != nil {
				t.Fatal(err)
			}
			if scenario == "empty-name" && (d.staged[id].Name != "" || d.counts["duplicate-clock"] != 0) {
				t.Fatal("empty name clock/name changed")
			}
			if scenario == "empty-values" || scenario == "detached" || scenario == "vanished" {
				if len(d.trees) != 0 || len(d.notifications) != 0 {
					t.Fatal("skipped records produced binding effects")
				}
			}
			if scenario == "missing-membership" {
				if len(d.trees) != 2 {
					t.Fatal("first memberships not persisted")
				}
				for _, tree := range d.trees {
					for _, bid := range []string{settingsBlock, cellSecond, nameBlock} {
						if n := treenode.GetNodeInTree(tree, bid); n != nil && n.IALAttr(av.NodeAttrNameAvs) != id {
							t.Fatal("initial membership not added")
						}
					}
				}
			}
			if scenario == "one-way" && d.staged[id].KeyValues[6].Key.Relation.BackKeyID != "back" {
				t.Fatal("one-way metadata changed")
			}
			if scenario == "sequential" {
				second, _, err := d.tx.duplicateDatabaseBlock(id)
				if err != nil || second == id || len(d.staged) != 2 || !strings.Contains(d.staged[second].Name, d.staged[id].Name) {
					t.Fatal("sequential duplicate", err)
				}
			}
			assertNameSource(t, d.cellHarness, before)
		})
	}
}

func TestAVDuplicateFailuresAndCancellation(t *testing.T) {
	m, store, root := nameSource(t)
	page := "1760000000000000002"
	l, err := m.Acquire(context.Background(), store, page)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(l.Release)
	for _, point := range []string{"duplicate-source", "read:" + cellAV, "duplicate-used", "duplicate-clock", "duplicate-registry", "duplicate-stage", "exists", "duplicate-node", "load", "name", "duplicate-attrs", "name-cache", "name-notify", "duplicate-tree", "tree-stage"} {
		for _, prefix := range []string{"", "cancel-"} {
			if point == "duplicate-clock" && prefix == "" {
				continue // Clock returns a time, so only its cancellation can fail.
			}
			t.Run(prefix+point, func(t *testing.T) {
				d := newDuplicateHarness(t, l, root, page, prefix+point)
				before := nameDigest(t, root, page)
				_, _, err := d.tx.duplicateDatabaseBlock(cellAV)
				if err == nil {
					t.Fatal("ignored failure", point, d.counts)
				}
				if prefix != "" && !errors.Is(err, context.Canceled) {
					t.Fatal(err)
				}
				if d.committed || d.counts["finish"] != 0 {
					t.Fatal("failure finished")
				}
				assertNameSource(t, d.cellHarness, before)
			})
		}
	}
}

func TestAVDuplicateDependencyGatesAndStandalone(t *testing.T) {
	m, store, root := nameSource(t)
	l, err := m.Acquire(context.Background(), store, "1760000000000000002")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(l.Release)
	for _, binding := range []bool{false, true} {
		d := newDuplicateHarness(t, l, root, "1760000000000000002", "")
		var obj reflect.Value
		if binding {
			obj = reflect.ValueOf(d.env.avDuplicate.bindings).Elem()
		} else {
			obj = reflect.ValueOf(d.env.avDuplicate).Elem()
		}
		for i := 0; i < obj.NumField(); i++ {
			if obj.Field(i).Kind() != reflect.Func {
				continue
			}
			// Reflection only enumerates names; private fields cannot be mutated with Set.
			name := obj.Type().Field(i).Name
			// Lifecycle hooks/whole-name projection are optional for explicit candidate adapters.
			if name == "ownerGuard" || name == "complete" || name == "names" {
				continue
			}
			t.Run(name, func(t *testing.T) {
				x := *d.env.avDuplicate
				b := *x.bindings
				x.bindings = &b
				switch name {
				case "source":
					x.source = nil
				case "usedID":
					x.usedID = nil
				case "newID":
					x.newID = nil
				case "now":
					x.now = nil
				case "registry":
					x.registry = nil
				case "stage":
					x.stage = nil
				case "exists":
					b.exists = nil
				case "node":
					b.node = nil
				case "name":
					b.name = nil
				case "untitled":
					b.untitled = nil
				case "attrs":
					b.attrs = nil
				case "cache":
					b.cache = nil
				case "notify":
					b.notify = nil
				case "tree":
					b.tree = nil
				}
				if _, _, err := duplicateDatabaseBlockWithEnvironment(cellAV, &x); err == nil || len(d.counts) != 0 {
					t.Fatal("unbound effect", name, err, d.counts)
				}
			})
		}
	}
	d := newDuplicateHarness(t, l, root, "1760000000000000002", "")
	d.env.avDuplicate = nil
	if _, _, err = d.tx.duplicateDatabaseBlock(cellAV); err == nil || len(d.counts) != 0 {
		t.Fatal("missing adapter fell back")
	}
	d.tx.DoOperations = []*Operation{{Action: "duplicateDatabaseBlock", AvID: cellAV}}
	if ret := performTx(d.tx); ret == nil || d.committed {
		t.Fatal("standalone admitted as transaction")
	}
	legacy := legacyAttributeViewDuplicateEnvironment()
	if !legacy.legacy || legacy.validate() != nil {
		t.Fatal("legacy factory")
	}
	if _, _, err := DuplicateDatabaseBlock(""); err == nil {
		t.Fatal("invalid legacy id")
	}
}
