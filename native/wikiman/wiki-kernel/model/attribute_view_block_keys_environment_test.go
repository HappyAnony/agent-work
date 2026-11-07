package model

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"wiki-kernel/av"
	"wiki-kernel/internal/avrender"
	"wiki-kernel/internal/workspace"
)

type blockKeysHarness struct {
	*boundHarness
	order      []string
	registered [][2]string
}

func newBlockKeysHarness(t *testing.T, l *workspace.Lease, root, page, failure string) *blockKeysHarness {
	d := newBoundHarness(t, l, root, page, failure)
	h := &blockKeysHarness{boundHarness: d}
	mark := func(s string) error { h.order = append(h.order, s); return d.mark(s) }
	e := &attributeViewBlockKeysEnvironment{ctx: d.ctx,
		wait: func() error { return mark("keys-wait") },
		attrs: func(id string) (map[string]string, error) {
			if err := mark("keys-attrs"); err != nil {
				return nil, err
			}
			n, _, err := d.cell.node(id)
			if err != nil {
				return nil, err
			}
			return parse.IAL2Map(n.KramdownIAL), nil
		},
		read: func(id string) (*av.AttributeView, error) {
			if err := mark("keys-read:" + id); err != nil {
				return nil, err
			}
			if a := d.staged[id]; a != nil {
				return a, nil
			}
			return d.cell.read(id)
		},
		clone: func(a *av.AttributeView) (*av.AttributeView, error) {
			if err := mark("keys-clone"); err != nil {
				return nil, err
			}
			return cloneAttributeViewPreview(a)
		},
		node: func(id string) (*ast.Node, *parse.Tree, error) {
			if err := mark("keys-node"); err != nil {
				return nil, nil, err
			}
			if d.env.lookup(id) == nil {
				return nil, nil, nil
			}
			return d.cell.node(id)
		},
		newView: func() (*av.View, error) {
			if err := mark("keys-new-view"); err != nil {
				return nil, err
			}
			v := &av.View{ID: d.cell.newID(), Name: page + " owner table", LayoutType: av.LayoutTypeTable, Table: av.NewLayoutTable(), PageSize: 16}
			v.Table.Columns = []*av.ViewTableColumn{{BaseField: &av.BaseField{ID: "primary"}}}
			return v, nil
		},
		stage: func(a *av.AttributeView) error {
			if err := mark("keys-stage"); err != nil {
				return err
			}
			return d.cell.stage(a)
		},
		render: func(a *av.AttributeView, v *av.View) error {
			if err := mark("keys-render"); err != nil {
				return err
			}
			env := cellRenderEnvironment(d.cellHarness)
			switch v.LayoutType {
			case av.LayoutTypeGallery:
				_, err := avrender.Gallery(a, v, "", env)
				return err
			case av.LayoutTypeKanban:
				_, err := avrender.Kanban(a, v, "", env)
				return err
			default:
				_, err := avrender.Table(a, v, "", env)
				return err
			}
		},
		mirrors: func(id string) ([]string, error) {
			if err := mark("keys-mirrors"); err != nil {
				return nil, err
			}
			return slices.Clone(d.registry[id]), nil
		},
		allMirrors: func() ([]string, error) {
			if err := mark("keys-all-mirrors"); err != nil {
				return nil, err
			}
			return []string{"vanished", rowMirror, rowMirror}, nil
		},
		upsert: func(id, block string) error {
			if err := mark("keys-upsert"); err != nil {
				return err
			}
			h.registered = append(h.registered, [2]string{id, block})
			if !slices.Contains(d.registry[id], block) {
				d.registry[id] = append(d.registry[id], block)
			}
			return nil
		},
		unbind: func(id, block string) error {
			if err := mark("keys-unbind"); err != nil {
				return err
			}
			return unbindBlockAv(d.tx, id, block)
		},
		remove: func(n *ast.Node, id string, tree *parse.Tree) error {
			if err := mark("keys-remove"); err != nil {
				return err
			}
			return removeNodeAvIDWithEnvironment(n, id, tree, &attributeViewRowEnvironment{cell: d.cell, exists: func(id string) (bool, error) {
				if err := mark("keys-exists"); err != nil {
					return false, err
				}
				return id == cellAV || id == cellDest || id == cellRelated, nil
			}})
		}, untitled: func() string { d.mark("keys-untitled"); return page + " owner untitled" },
	}
	d.env.avBlockKeys = e
	return h
}

func blockKeysNode(t *testing.T, h *blockKeysHarness) (*ast.Node, *parse.Tree) {
	t.Helper()
	n, tree, err := h.cell.node(settingsBlock)
	if err != nil {
		t.Fatal(err)
	}
	n.SetIALAttr(av.NodeAttrNameAvs, cellAV)
	return n, tree
}

func TestAVBlockKeysOwnersLayoutsRowIdentityAndOriginalProjection(t *testing.T) {
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
				h := newBlockKeysHarness(t, l, root, page, "")
				before := nameDigest(t, root, page)
				n, _ := blockKeysNode(t, h)
				a, err := h.cell.read(cellAV)
				if err != nil {
					t.Fatal(err)
				}
				var selected string
				for _, v := range a.Views {
					if v.LayoutType == layout {
						selected = v.ID
					}
				}
				if selected == "" {
					t.Fatal("layout fixture missing")
				}
				n.SetIALAttr(av.NodeAttrView, selected)
				for _, kv := range a.KeyValues {
					for _, v := range kv.Values {
						if v.BlockID == settingsBlock {
							v.BlockID = "independent-row-id"
						}
					}
				}
				a.KeyValues = append(a.KeyValues, &av.KeyValues{Key: &av.Key{ID: "line", Type: av.KeyTypeLineNumber}})
				a.KeyIDs = []string{"number", "obsolete", "primary", "line"}
				original := mustDuplicateJSON(t, a)
				ret, err := h.tx.blockAttributeViewKeys(settingsBlock)
				if err != nil || len(ret) != 1 {
					t.Fatal(err, ret, h.order)
				}
				r := ret[0]
				if r.AvID != cellAV || r.AvName != page+" "+cellAV || !slices.Equal(r.BlockIDs, []string{rowMirror}) || r.KeyValues[0].Key.ID != "number" {
					t.Fatal("projection/order", r)
				}
				for _, kv := range r.KeyValues {
					if kv.Key.Type == av.KeyTypeLineNumber {
						t.Fatal("line number leaked")
					}
					for _, v := range kv.Values {
						if v.BlockID != "independent-row-id" {
							t.Fatal("bound ID used as row ID", v)
						}
					}
				}
				if !reflect.DeepEqual(original, mustDuplicateJSON(t, a)) || h.staged[cellAV].ViewID != selected || slices.Contains(h.staged[cellAV].KeyIDs, "obsolete") || h.counts["keys-render"] != 1 || h.counts["keys-upsert"] != 0 || h.counts["attrs"] != 0 {
					t.Fatal("query mutated source or repaired unnecessarily", h.counts)
				}
				assertNameSource(t, h.cellHarness, before)
			}
		})
	}
}

func TestAVBlockKeysCorrectiveUnbindReturnsPriorResultsOnly(t *testing.T) {
	m, store, root := nameSource(t)
	page := "1760000000000000002"
	l, err := m.Acquire(context.Background(), store, page)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(l.Release)
	for _, first := range []bool{true, false} {
		h := newBlockKeysHarness(t, l, root, page, "")
		before := nameDigest(t, root, page)
		n, _ := blockKeysNode(t, h)
		members := cellDest + "," + cellAV + "," + cellRelated
		if !first {
			members = cellAV + "," + cellDest + "," + cellRelated
		}
		n.SetIALAttr(av.NodeAttrNameAvs, members)
		a, err := h.cell.read(cellDest)
		if err != nil {
			t.Fatal(err)
		}
		a.GetBlockKeyValues().Values[0].Block.ID = "old-cut-block"
		ret, err := h.tx.blockAttributeViewKeys(settingsBlock)
		want := 0
		if !first {
			want = 1
		}
		if err != nil || len(ret) != want || h.counts["keys-unbind"] != 1 || h.counts["keys-read:"+cellRelated] != 0 {
			t.Fatal("did not stop after correction", err, h.order)
		}
		if n.IALAttr(av.NodeAttrNameAvs) != cellAV+","+cellRelated || len(h.notifications) == 0 || h.counts["finish"] != 0 {
			t.Fatal("correction lost owner effects", h.counts)
		}
		assertNameSource(t, h.cellHarness, before)
	}
}

func TestAVBlockKeysMirrorScanRegistrationAndMissingMirrorRemoval(t *testing.T) {
	m, store, root := nameSource(t)
	page := "1760000000000000002"
	l, err := m.Acquire(context.Background(), store, page)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(l.Release)
	for _, scenario := range []string{"scan", "missing", "document"} {
		h := newBlockKeysHarness(t, l, root, page, "")
		before := nameDigest(t, root, page)
		n, tree := blockKeysNode(t, h)
		h.registry = map[string][]string{}
		id := settingsBlock
		if scenario != "scan" {
			h.env.avBlockKeys.allMirrors = func() ([]string, error) { return []string{"vanished"}, nil }
		}
		if scenario == "document" {
			id = tree.ID
			n = tree.Root
			n.SetIALAttr(av.NodeAttrNameAvs, cellAV)
			n.SetIALAttr("custom-hidden", "true")
			a, err := h.cell.read(cellAV)
			if err != nil {
				t.Fatal(err)
			}
			a.GetBlockKeyValues().Values[0].Block.ID = id
			h.env.avBlockKeys.node = func(id string) (*ast.Node, *parse.Tree, error) {
				if id == tree.ID {
					return tree.Root, tree, nil
				}
				return nil, nil, nil
			}
		}
		ret, err := h.tx.blockAttributeViewKeys(id)
		if err != nil {
			t.Fatal(err, h.order)
		}
		if scenario == "scan" {
			if len(ret) != 1 || !reflect.DeepEqual(h.registered, [][2]string{{cellAV, rowMirror}}) || !slices.Equal(ret[0].BlockIDs, []string{rowMirror}) {
				t.Fatal("mirror rebuild", ret, h.registered)
			}
		} else {
			if len(ret) != 0 || n.IALAttr(av.NodeAttrNameAvs) != "" || len(h.notifications) == 0 || n.IALAttr("custom-hidden") != "" {
				t.Fatal("missing mirror correction", n.KramdownIAL, h.counts)
			}
		}
		assertNameSource(t, h.cellHarness, before)
	}
}

func TestAVBlockKeysViewCreationFallbackDuplicateAndIsolation(t *testing.T) {
	m, store, root := nameSource(t)
	page := "1760000000000000002"
	l, err := m.Acquire(context.Background(), store, page)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(l.Release)
	for _, scenario := range []string{"empty", "empty-name", "no-views", "invalid-current", "stale-selected", "duplicate", "detached-payload"} {
		h := newBlockKeysHarness(t, l, root, page, "")
		before := nameDigest(t, root, page)
		n, _ := blockKeysNode(t, h)
		a, err := h.cell.read(cellAV)
		if err != nil {
			t.Fatal(err)
		}
		switch scenario {
		case "empty":
			n.RemoveIALAttr(av.NodeAttrNameAvs)
		case "empty-name":
			a.Name = "  "
		case "no-views":
			a.Views = nil
		case "invalid-current":
			a.ViewID = "absent"
		case "stale-selected":
			n.SetIALAttr(av.NodeAttrView, "absent")
		case "duplicate":
			n.SetIALAttr(av.NodeAttrNameAvs, cellAV+","+cellAV)
		case "detached-payload":
			a.GetBlockKeyValues().Values[0].IsDetached = true
		}
		original := mustDuplicateJSON(t, a)
		ret, err := h.tx.blockAttributeViewKeys(settingsBlock)
		if err != nil {
			t.Fatal(err, h.order)
		}
		want := 1
		if scenario == "empty" {
			want = 0
		}
		if scenario == "duplicate" {
			want = 2
		}
		if len(ret) != want {
			t.Fatal(scenario, len(ret))
		}
		if scenario == "empty-name" && ret[0].AvName != page+" owner untitled" {
			t.Fatal(ret[0].AvName)
		}
		if scenario == "duplicate" && h.counts["keys-read:"+cellAV] != 1 {
			t.Fatal("duplicate read not cached", h.counts)
		}
		if scenario == "no-views" && h.counts["keys-new-view"] != 1 {
			t.Fatal("empty table not created")
		}
		if !reflect.DeepEqual(original, mustDuplicateJSON(t, a)) {
			t.Fatal("renderer mutated owner source cache")
		}
		assertNameSource(t, h.cellHarness, before)
	}
}

func TestAVBlockKeysFailuresAndCancellationReturnNoPartialResults(t *testing.T) {
	m, store, root := nameSource(t)
	page := "1760000000000000002"
	l, err := m.Acquire(context.Background(), store, page)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(l.Release)
	for _, point := range []string{"keys-wait", "keys-attrs", "keys-read:" + cellAV, "keys-read:" + cellDest, "keys-clone", "keys-node", "keys-new-view", "keys-render", "keys-stage", "keys-mirrors", "keys-all-mirrors", "keys-upsert", "keys-unbind", "keys-remove", "keys-exists", "names", "attrs", "tree-stage", "notify"} {
		for _, prefix := range []string{"", "cancel-"} {
			t.Run(prefix+point, func(t *testing.T) {
				h := newBlockKeysHarness(t, l, root, page, "")
				before := nameDigest(t, root, page)
				n, _ := blockKeysNode(t, h)
				n.SetIALAttr(av.NodeAttrView, "selected")
				if point == "keys-read:"+cellDest {
					n.SetIALAttr(av.NodeAttrNameAvs, cellAV+","+cellDest)
				}
				if point == "keys-new-view" {
					a, err := h.cell.read(cellAV)
					if err != nil {
						t.Fatal(err)
					}
					a.Views = nil
				}
				if point == "keys-upsert" || point == "keys-all-mirrors" || point == "keys-remove" || point == "keys-exists" {
					h.registry = map[string][]string{}
				}
				if point == "keys-remove" || point == "keys-exists" {
					h.env.avBlockKeys.allMirrors = func() ([]string, error) { return nil, nil }
					if point == "keys-exists" {
						n.SetIALAttr(av.NodeAttrNameAvs, cellAV+","+cellDest)
					}
				}
				if point == "keys-unbind" || point == "names" || point == "attrs" || point == "tree-stage" || point == "notify" {
					a, err := h.cell.read(cellAV)
					if err != nil {
						t.Fatal(err)
					}
					a.GetBlockKeyValues().Values[0].Block.ID = "old-block"
				}
				h.failure = prefix + point
				ret, err := h.tx.blockAttributeViewKeys(settingsBlock)
				if err == nil || ret != nil || h.committed || h.counts["finish"] != 0 || h.counts[point] == 0 {
					t.Fatal("failure swallowed", ret, err, h.order, h.counts)
				}
				assertNameSource(t, h.cellHarness, before)
			})
		}
	}
}

func TestAVBlockKeysMissingDependenciesBadSourceAndNoGlobalFallback(t *testing.T) {
	m, store, root := nameSource(t)
	page := "1760000000000000002"
	l, err := m.Acquire(context.Background(), store, page)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(l.Release)
	for _, scenario := range []string{"missing-env", "missing-wait", "missing-attrs", "missing-read", "missing-clone", "missing-node", "missing-stage", "missing-render", "missing-mirrors", "missing-all", "missing-upsert", "missing-unbind", "missing-remove", "missing-new-view", "missing-name", "cancelled", "empty-id", "nil-av", "wrong-av", "nil-primary", "nil-view", "nil-field", "clone-alias", "clone-nil", "bad-tree"} {
		t.Run(scenario, func(t *testing.T) {
			h := newBlockKeysHarness(t, l, root, page, "")
			before := nameDigest(t, root, page)
			blockKeysNode(t, h)
			e := h.env.avBlockKeys
			e.legacy = true
			id := settingsBlock
			switch scenario {
			case "missing-env":
				h.env.avBlockKeys = nil
			case "missing-wait":
				e.wait = nil
			case "missing-attrs":
				e.attrs = nil
			case "missing-read":
				e.read = nil
			case "missing-clone":
				e.clone = nil
			case "missing-node":
				e.node = nil
			case "missing-stage":
				e.stage = nil
			case "missing-render":
				e.render = nil
			case "missing-mirrors":
				e.mirrors = nil
			case "missing-all":
				e.allMirrors = nil
			case "missing-upsert":
				e.upsert = nil
			case "missing-unbind":
				e.unbind = nil
			case "missing-remove":
				e.remove = nil
			case "missing-new-view":
				e.newView = nil
			case "missing-name":
				e.untitled = nil
			case "cancelled":
				h.cancel()
			case "empty-id":
				id = ""
			case "nil-av":
				e.read = func(string) (*av.AttributeView, error) { return nil, nil }
			case "clone-alias":
				e.clone = func(a *av.AttributeView) (*av.AttributeView, error) { return a, nil }
			case "clone-nil":
				e.clone = func(*av.AttributeView) (*av.AttributeView, error) { return nil, nil }
			case "bad-tree":
				e.node = func(string) (*ast.Node, *parse.Tree, error) { return &ast.Node{ID: id}, &parse.Tree{}, nil }
			default:
				a, err := h.cell.read(cellAV)
				if err != nil {
					t.Fatal(err)
				}
				switch scenario {
				case "wrong-av":
					a.ID = cellDest
				case "nil-primary":
					a.GetBlockKeyValues().Key.Type = av.KeyTypeText
				case "nil-view":
					a.Views = append(a.Views, nil)
				case "nil-field":
					a.KeyValues = append(a.KeyValues, nil)
				}
			}
			ret, err := h.tx.blockAttributeViewKeys(id)
			if err == nil || ret != nil || h.counts["finish"] != 0 {
				t.Fatal("invalid dependency/source admitted", ret, err, h.counts)
			}
			assertNameSource(t, h.cellHarness, before)
		})
	}
}

func TestAVBlockKeysLegacyReadSkipsAndCancelledCallbacks(t *testing.T) {
	// No global services are used: the compatibility policy still skips an absent
	// AV, while native post-service cancellation cannot return a partial success.
	e := legacyAttributeViewBlockKeysEnvironment()
	// Explicit algorithm fixture; real unbound owner ports remain errors.
	e.ownerGuard = nil
	e.complete = nil
	e.strictDependencies = false
	e.wait = func() error { return nil }
	e.attrs = func(string) (map[string]string, error) { return map[string]string{av.NodeAttrNameAvs: "missing"}, nil }
	e.read = func(string) (*av.AttributeView, error) { return nil, errors.New("missing") }
	if ret, err := blockAttributeViewKeysWithEnvironment(settingsBlock, e); err != nil || len(ret) != 0 {
		t.Fatal(ret, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	e.ctx = ctx
	e.legacy = false
	e.wait = func() error { cancel(); return nil }
	if ret, err := blockAttributeViewKeysWithEnvironment(settingsBlock, e); ret != nil || !errors.Is(err, context.Canceled) {
		t.Fatal("late cancellation lost", ret, err)
	}
}

func TestAVBlockKeysPostServiceCancellationAndDetachedSource(t *testing.T) {
	m, store, root := nameSource(t)
	page := "1760000000000000002"
	l, err := m.Acquire(context.Background(), store, page)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(l.Release)
	for _, point := range []string{"clone", "render", "stage", "unbind", "name"} {
		t.Run(point, func(t *testing.T) {
			h := newBlockKeysHarness(t, l, root, page, "")
			before := nameDigest(t, root, page)
			blockKeysNode(t, h)
			e := h.env.avBlockKeys
			a, err := h.cell.read(cellAV)
			if err != nil {
				t.Fatal(err)
			}
			if point == "name" {
				a.Name = ""
			}
			if point == "unbind" {
				a.GetBlockKeyValues().Values[0].Block.ID = "old-cut"
			}
			original := mustDuplicateJSON(t, a)
			switch point {
			case "clone":
				f := e.clone
				e.clone = func(a *av.AttributeView) (*av.AttributeView, error) { copy, err := f(a); h.cancel(); return copy, err }
			case "render":
				f := e.render
				e.render = func(a *av.AttributeView, v *av.View) error { err := f(a, v); h.cancel(); return err }
			case "stage":
				f := e.stage
				e.stage = func(a *av.AttributeView) error { err := f(a); h.cancel(); return err }
			case "unbind":
				f := e.unbind
				e.unbind = func(a, b string) error { err := f(a, b); h.cancel(); return err }
			case "name":
				e.untitled = func() string { h.cancel(); return "owner name" }
			}
			ret, err := h.tx.blockAttributeViewKeys(settingsBlock)
			if ret != nil || !errors.Is(err, context.Canceled) {
				t.Fatal("late callback cancel lost", ret, err, h.order)
			}
			if !reflect.DeepEqual(original, mustDuplicateJSON(t, a)) {
				t.Fatal("detached read mutated owner AV")
			}
			assertNameSource(t, h.cellHarness, before)
		})
	}
}
