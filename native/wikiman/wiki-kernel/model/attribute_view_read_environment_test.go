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
)

func readPrimaryFixture(t *testing.T, h *blockKeysHarness) *av.AttributeView {
	t.Helper()
	a, err := h.cell.read(cellAV)
	if err != nil {
		t.Fatal(err)
	}
	a.Name = "  " + h.page + " primary  "
	a.GetBlockKeyValues().Values = []*av.Value{
		{ID: "first", BlockID: "row-first", Block: &av.ValueBlock{ID: settingsBlock, Content: h.page + " Alpha", Updated: 1}},
		{ID: "detached", BlockID: "row-detached", IsDetached: true, Block: &av.ValueBlock{Content: h.page + " ALPHA", Updated: 9}},
		{ID: "gone", BlockID: "row-gone", Block: &av.ValueBlock{ID: "20261026170000-gon0001", Content: "alpha", Updated: 99}},
		{ID: "other", BlockID: "row-other", Block: &av.ValueBlock{ID: nameBlock, Content: h.page + " alphaBeta", Updated: 5}},
	}
	for _, v := range a.GetBlockKeyValues().Values {
		v.Type = av.KeyTypeBlock
	}
	return a
}

func TestAVReadOwnersPrimaryPagingExistenceAndNoMutation(t *testing.T) {
	m, store, root := nameSource(t)
	for _, page := range []string{"1760000000000000002", "1760000000000000003"} {
		t.Run(page, func(t *testing.T) {
			t.Parallel()
			l, err := m.Acquire(context.Background(), store, page)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(l.Release)
			h := newBlockKeysHarness(t, l, root, page, "")
			before := nameDigest(t, root, page)
			a := readPrimaryFixture(t, h)
			original := mustDuplicateJSON(t, a)
			for p := 1; p <= 3; p++ {
				r, err := h.tx.attributeViewPrimary(cellAV, "aLpHa", p, 2)
				if err != nil || r.name != page+" primary" || !slices.Equal(r.blocks, []string{rowMirror}) {
					t.Fatal(err, r)
				}
				ids := []string{}
				for _, v := range r.values.Values {
					ids = append(ids, v.ID)
				}
				want := [][]string{{"detached", "other"}, {"first"}, {}}[p-1]
				if !slices.Equal(ids, want) {
					t.Fatal(p, ids, want)
				}
			}
			r, err := h.tx.attributeViewPrimary(cellAV, "", 1, 0)
			if err != nil || len(r.values.Values) != 3 {
				t.Fatal("default page size", r, err)
			}
			if !reflect.DeepEqual(original, mustDuplicateJSON(t, a)) || h.counts["keys-stage"] != 0 || h.counts["keys-render"] != 0 || h.counts["finish"] != 0 {
				t.Fatal("read mutated cache or produced writes", h.counts)
			}
			assertNameSource(t, h.cellHarness, before)
		})
	}
}

func TestAVReadRulesOwnersAllLayoutsAndViewFallback(t *testing.T) {
	m, store, root := nameSource(t)
	for _, page := range []string{"1760000000000000002", "1760000000000000003"} {
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
			var selected *av.View
			for _, v := range a.Views {
				v.Filters = []*av.ViewFilter{{Column: "number", Operator: ">", Value: &av.Value{Type: av.KeyTypeNumber, Number: &av.ValueNumber{Content: float64(len(v.ID)), IsNotEmpty: true}}}}
				v.Sorts = []*av.ViewSort{{Column: v.ID, Order: av.SortOrderAsc}}
				if v.LayoutType == layout {
					selected = v
				}
			}
			if selected == nil {
				t.Fatal("missing fixture layout")
			}
			n.SetIALAttr(av.NodeAttrView, selected.ID)
			original := mustDuplicateJSON(t, a)
			filters, sorts, err := h.tx.attributeViewRules(cellAV, settingsBlock)
			if err != nil || !reflect.DeepEqual(filters, selected.Filters) || !reflect.DeepEqual(sorts, selected.Sorts) {
				t.Fatal("selected view", err, sorts)
			}
			filters[0].Value.Number.Content = 999
			if !reflect.DeepEqual(original, mustDuplicateJSON(t, a)) {
				t.Fatal("rules alias owner cache")
			}
			n.SetIALAttr(av.NodeAttrView, "stale")
			_, sorts, err = h.tx.attributeViewRules(cellAV, settingsBlock)
			if err != nil || sorts[0].Column != a.GetView(a.ViewID).ID {
				t.Fatal("stale selection fallback", err, sorts)
			}
			_, sorts, err = h.tx.attributeViewRules(cellAV, "missing-node")
			if err != nil || sorts[0].Column != a.ViewID {
				t.Fatal("vanished node fallback", err, sorts)
			}
			assertNameSource(t, h.cellHarness, before)
		}
	}
}

func TestAVReadFullObjectsMinimalDependenciesAndEmptyShapes(t *testing.T) {
	m, store, root := nameSource(t)
	page := "1760000000000000002"
	l, err := m.Acquire(context.Background(), store, page)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(l.Release)
	h := newBlockKeysHarness(t, l, root, page, "")
	before := nameDigest(t, root, page)
	a, err := h.cell.read(cellAV)
	if err != nil {
		t.Fatal(err)
	}
	original := mustDuplicateJSON(t, a)
	b := h.env.avBlockKeys
	b.mirrors, b.untitled, b.node, b.stage, b.render, b.upsert, b.remove, b.unbind = nil, nil, nil, nil, nil, nil, nil, nil
	copy, err := h.tx.readAttributeView(cellAV)
	if err != nil || copy == a || !reflect.DeepEqual(original, mustDuplicateJSON(t, copy)) {
		t.Fatal("full object", err)
	}
	copy.Name = "returned object edit"
	copy.GetBlockKeyValues().Values[0].Block.Content = "edited"
	if !reflect.DeepEqual(original, mustDuplicateJSON(t, a)) {
		t.Fatal("full source alias")
	}
	b.node = h.cell.node
	a.GetView(a.ViewID).Filters = nil
	a.GetView(a.ViewID).Sorts = nil
	f, s, err := h.tx.attributeViewRules(cellAV, "")
	if err != nil || f == nil || s == nil || len(f) != 0 || len(s) != 0 {
		t.Fatal("empty rules must remain arrays", f, s, err)
	}
	b.mirrors = func(string) ([]string, error) { return nil, nil }
	b.untitled = func() string { return page + " untitled" }
	a.Name = "  "
	r, err := h.tx.attributeViewPrimary(cellAV, "", 1, 16)
	if err != nil || r.name != page+" untitled" {
		t.Fatal("owner language", r, err)
	}
	assertNameSource(t, h.cellHarness, before)
}

func TestAVReadFailuresCancellationAndNoPartialNativeResults(t *testing.T) {
	m, store, root := nameSource(t)
	page := "1760000000000000002"
	l, err := m.Acquire(context.Background(), store, page)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(l.Release)
	for _, mode := range []string{"source", "primary", "rules"} {
		points := []string{"keys-wait", "keys-read:" + cellAV, "keys-clone"}
		if mode == "primary" {
			points = append(points, "keys-mirrors", "exists")
		}
		if mode == "rules" {
			points = append(points, "keys-node")
		}
		for _, point := range points {
			for _, prefix := range []string{"", "cancel-"} {
				t.Run(mode+"/"+prefix+point, func(t *testing.T) {
					h := newBlockKeysHarness(t, l, root, page, "")
					before := nameDigest(t, root, page)
					readPrimaryFixture(t, h)
					h.failure = prefix + point
					e := h.tx.attributeViewReadEnvironment()
					if point == "exists" {
						e.exists = func(context.Context, string) (bool, error) {
							if prefix != "" {
								h.cancel()
								return true, nil
							}
							return false, errors.New("owner exists failure")
						}
					}
					var err error
					switch mode {
					case "source":
						var a *av.AttributeView
						a, err = readAttributeViewWithEnvironment(cellAV, mode, e)
						if a != nil {
							t.Fatal("partial object")
						}
					case "primary":
						var r attributeViewPrimaryResult
						r, err = attributeViewPrimaryWithEnvironment(cellAV, "", 1, 16, e)
						if r.name != "" || r.values != nil || r.blocks != nil {
							t.Fatal("partial primary", r)
						}
					case "rules":
						f, s, failure := attributeViewRulesWithEnvironment(cellAV, settingsBlock, e)
						err = failure
						if f != nil || s != nil {
							t.Fatal("partial rules")
						}
					}
					if err == nil || h.counts["finish"] != 0 {
						t.Fatal("failure swallowed", mode, point, h.counts)
					}
					assertNameSource(t, h.cellHarness, before)
				})
			}
		}
	}
}

func TestAVReadMissingServicesBadSourcesAndNoLegacyFallback(t *testing.T) {
	m, store, root := nameSource(t)
	page := "1760000000000000002"
	l, err := m.Acquire(context.Background(), store, page)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(l.Release)
	for _, scenario := range []string{"env", "ctx", "wait", "read", "clone", "nil-source", "wrong-id", "clone-alias", "clone-nil", "mirrors", "exists", "name", "node", "bad-tree", "no-primary", "nil-primary", "bad-page", "no-view", "late-read", "late-clone", "late-name"} {
		t.Run(scenario, func(t *testing.T) {
			h := newBlockKeysHarness(t, l, root, page, "")
			before := nameDigest(t, root, page)
			a := readPrimaryFixture(t, h)
			e := h.tx.attributeViewReadEnvironment()
			mode := "source"
			pageNumber := 1
			switch scenario {
			case "env":
				e = nil
			case "ctx":
				e.ctx = nil
			case "wait":
				e.wait = nil
			case "read":
				e.read = nil
			case "clone":
				e.clone = nil
			case "nil-source":
				e.read = func(string) (*av.AttributeView, error) { return nil, nil }
			case "wrong-id":
				a.ID = cellDest
			case "clone-alias":
				e.clone = func(a *av.AttributeView) (*av.AttributeView, error) { return a, nil }
			case "clone-nil":
				e.clone = func(*av.AttributeView) (*av.AttributeView, error) { return nil, nil }
			case "mirrors":
				mode = "primary"
				e.mirrors = nil
			case "exists":
				mode = "primary"
				e.exists = nil
			case "name":
				mode = "primary"
				e.untitled = nil
			case "node":
				mode = "rules"
				e.node = nil
			case "bad-tree":
				mode = "rules"
				h.env.avBlockKeys.node = func(string) (*ast.Node, *parse.Tree, error) { return &ast.Node{ID: settingsBlock}, &parse.Tree{}, nil }
				e = h.tx.attributeViewReadEnvironment()
			case "no-primary":
				mode = "primary"
				a.GetBlockKeyValues().Key.Type = av.KeyTypeText
			case "nil-primary":
				mode = "primary"
				a.GetBlockKeyValues().Values[0].Block = nil
			case "bad-page":
				mode = "primary"
				pageNumber = 0
			case "no-view":
				mode = "rules"
				a.Views = nil
			case "late-read":
				e.read = func(string) (*av.AttributeView, error) { h.cancel(); return a, nil }
			case "late-clone":
				f := e.clone
				e.clone = func(a *av.AttributeView) (*av.AttributeView, error) { b, err := f(a); h.cancel(); return b, err }
			case "late-name":
				mode = "primary"
				a.Name = ""
				e.untitled = func() string { h.cancel(); return "owner" }
			}
			var err error
			switch mode {
			case "source":
				_, err = readAttributeViewWithEnvironment(cellAV, mode, e)
			case "primary":
				_, err = attributeViewPrimaryWithEnvironment(cellAV, "", pageNumber, 16, e)
			case "rules":
				_, _, err = attributeViewRulesWithEnvironment(cellAV, settingsBlock, e)
			}
			if err == nil || h.counts["finish"] != 0 {
				t.Fatal("bad dependency/source accepted", scenario, err)
			}
			assertNameSource(t, h.cellHarness, before)
		})
	}
	h := newBlockKeysHarness(t, l, root, page, "")
	h.env.avBlockKeys = nil
	if a, err := h.tx.readAttributeView(cellAV); a != nil || err == nil {
		t.Fatal("missing slot fell back")
	}
}

func TestAVReadLegacyCompatibilityFailuresAndCurrentViewRules(t *testing.T) {
	a := &av.AttributeView{ID: cellAV, Name: "  title  ", KeyValues: []*av.KeyValues{{Key: &av.Key{Type: av.KeyTypeBlock}, Values: []*av.Value{}}}, Views: []*av.View{{ID: "first"}}, ViewID: "first"}
	e := &attributeViewReadEnvironment{ctx: context.Background(), legacy: true, wait: func() error { return nil }, read: func(string) (*av.AttributeView, error) { return a, nil }, clone: cloneAttributeViewPreview, mirrors: func(string) ([]string, error) { return []string{"mirror"}, nil }, exists: func(context.Context, string) (bool, error) { return true, nil }, node: func(string) (*ast.Node, *parse.Tree, error) { return nil, nil, errors.New("vanished legacy block") }, untitled: func() string { return "untitled" }}
	r, err := attributeViewPrimaryWithEnvironment(cellAV, "", 0, 16, e)
	if err == nil || r.name != "title" || !slices.Equal(r.blocks, []string{"mirror"}) || r.values == nil {
		t.Fatal("legacy primary error shape changed", r, err)
	}
	f, s, err := attributeViewRulesWithEnvironment(cellAV, "gone", e)
	if err != nil || f == nil || s == nil {
		t.Fatal("legacy view fallback", f, s, err)
	}
}
