package model

import (
	"context"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"slices"
	"testing"
	"wiki-kernel/av"
	"wiki-kernel/treenode"
)

func TestAVManagementGroupRulesAndStates(t *testing.T) {
	m, store, root := managementSource(t)
	for _, page := range []string{"1760000000000000002", "1760000000000000003"} {
		t.Run(page, func(t *testing.T) {
			t.Parallel()
			l, err := m.Acquire(context.Background(), store, page)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(l.Release)
			for _, layout := range []av.LayoutType{av.LayoutTypeTable, av.LayoutTypeGallery, av.LayoutTypeKanban} {
				for _, action := range []string{"setAttrViewGroup", "removeAttrViewGroup", "hideAttrViewGroup", "hideAttrViewAllGroups", "foldAttrViewGroup", "sortAttrViewGroup"} {
					t.Run(string(layout)+action, func(t *testing.T) {
						h := newManagementHarness(t, l, root, page, "", layout)
						a, v, old, target := orderGroup(t, h.cellHarness, layout, "status")
						before := cellDigest(t, root, page)
						reg := managementRegistry(t, h)
						op := managementOp(action, layout)
						op.ID = old
						op.PreviousID = target
						oldGroup := v.GetGroupByID(old)
						oldGroup.GroupFolded = true
						oldGroup.GroupHidden = 2
						h.tx.DoOperations = []*Operation{op}
						if ret := performTx(h.tx); ret != nil || !h.committed {
							t.Fatal(ret, h.counts)
						}
						switch action {
						case "setAttrViewGroup":
							if !v.Group.HideEmpty || h.counts["count"] == 0 || (v.GetGroupByGroupValue("Old").GroupFolded != (layout != av.LayoutTypeKanban)) || v.GetGroupByGroupValue("Old").GroupHidden != 2 {
								t.Fatal("group states or real count lost", h.counts)
							}
							if keep := v.GetGroupByGroupValue("Keep"); keep == nil || keep.GroupHidden != 1 {
								t.Fatal("empty group was not auto-hidden")
							}
						case "removeAttrViewGroup":
							if v.Group != nil || v.Groups != nil || v.GroupCreated != 0 {
								t.Fatal("group not cleared")
							}
						case "hideAttrViewGroup":
							if oldGroup.GroupHidden != 2 {
								t.Fatal("manual group hiding")
							}
						case "hideAttrViewAllGroups":
							for _, g := range v.Groups {
								if g.GroupHidden != 2 {
									t.Fatal("hide-all rule")
								}
							}
						case "foldAttrViewGroup":
							if !oldGroup.GroupFolded {
								t.Fatal("fold rule")
							}
						case "sortAttrViewGroup":
							if v.Group.Order != av.GroupOrderMan {
								t.Fatal("manual order not set")
							}
							var ids []string
							for i, g := range v.Groups {
								ids = append(ids, g.ID)
								if g.GroupSort != i {
									t.Fatal("manual sort indices")
								}
							}
							if slices.Index(ids, old) != slices.Index(ids, target)+1 {
								t.Fatal("group order", ids)
							}
						}
						assertManagementSource(t, h, before, reg)
						_ = a
					})
				}
				for _, scenario := range []string{"first-select", "checkbox", "unhide-empty", "filter-empty"} {
					t.Run(string(layout)+scenario, func(t *testing.T) {
						h := newManagementHarness(t, l, root, page, "", layout)
						a, v, _, _ := orderGroup(t, h.cellHarness, layout, "status")
						before := cellDigest(t, root, page)
						op := managementOp("setAttrViewGroup", layout)
						switch scenario {
						case "first-select":
							v.Group = nil
							v.Groups = nil
						case "checkbox":
							op.Data = &av.ViewGroup{Field: "check"}
							a.GetValue("check", settingsBlock).Checkbox.Checked = true
						case "unhide-empty":
							v.Group.HideEmpty = true
							for _, g := range v.Groups {
								g.GroupHidden = 1
							}
							op.Data = &av.ViewGroup{Field: "status", HideEmpty: false}
						case "filter-empty":
							v.Filters = []*av.ViewFilter{{Column: "number", Operator: av.FilterOperatorIsGreater, Value: &av.Value{Type: av.KeyTypeNumber, Number: &av.ValueNumber{Content: 9999, IsNotEmpty: true}}}}
						}
						h.tx.DoOperations = []*Operation{op}
						if ret := performTx(h.tx); ret != nil || !h.committed {
							t.Fatal(ret, h.counts)
						}
						switch scenario {
						case "first-select":
							if v.Group.Order != av.GroupOrderSelectOption {
								t.Fatal("select ordering")
							}
						case "checkbox":
							if v.Group.Order != av.GroupOrderMan || len(v.Groups) != 2 || v.Groups[0].GetGroupValue() != av.CheckboxCheckedStr {
								t.Fatal("checkbox order")
							}
						case "unhide-empty":
							if v.GetGroupByGroupValue("Keep").GroupHidden != 0 {
								t.Fatal("automatic hidden state not restored")
							}
						case "filter-empty":
							for _, g := range v.Groups {
								if g.GroupHidden != 1 {
									t.Fatal("hide-empty ignored actual filters", g.GetGroupValue(), g.GroupHidden)
								}
							}
						}
						assertFieldSourceUnchanged(t, h.cellHarness, before)
					})
				}
			}
		})
	}
}

func TestAVManagementSequentialLifecycleAndNoOps(t *testing.T) {
	m, store, root := managementSource(t)
	page := "1760000000000000002"
	l, err := m.Acquire(context.Background(), store, page)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(l.Release)
	for _, scenario := range []string{"mixed", "last-view", "same-layout", "same-order", "missing-group", "non-group-fold", "reuse-layout"} {
		t.Run(scenario, func(t *testing.T) {
			h := newManagementHarness(t, l, root, page, "", av.LayoutTypeTable)
			a := managementPrepare(t, h, av.LayoutTypeTable)
			v := a.Views[0]
			before := cellDigest(t, root, page)
			switch scenario {
			case "mixed":
				h.tx.DoOperations = []*Operation{{Action: "addAttrViewView", AvID: cellAV, BlockID: rowMirror, ID: "added", Layout: av.LayoutTypeGallery}, {Action: "setAttrViewViewName", AvID: cellAV, ID: "added", Data: " Mixed "}, {Action: "setAttrViewCardSize", AvID: cellAV, BlockID: rowMirror, Data: float64(2)}, {Action: "setAttrViewGroup", AvID: cellAV, BlockID: rowMirror, Data: &av.ViewGroup{Field: "status", HideEmpty: true}}, {Action: "removeAttrViewView", AvID: cellAV, BlockID: rowMirror}}
			case "last-view":
				a.Views = a.Views[:1]
				h.tx.DoOperations = []*Operation{managementOp("removeAttrViewView", av.LayoutTypeTable)}
			case "same-layout":
				op := managementOp("changeAttrViewLayout", av.LayoutTypeTable)
				op.Layout = av.LayoutTypeTable
				h.tx.DoOperations = []*Operation{op}
			case "same-order":
				op := managementOp("sortAttrViewView", av.LayoutTypeTable)
				op.PreviousID = op.ID
				h.tx.DoOperations = []*Operation{op}
			case "missing-group":
				a, v, _, _ = orderGroup(t, h.cellHarness, av.LayoutTypeTable, "status")
				op := managementOp("sortAttrViewGroup", av.LayoutTypeTable)
				op.ID = "missing"
				h.tx.DoOperations = []*Operation{op}
			case "non-group-fold":
				h.tx.DoOperations = []*Operation{managementOp("foldAttrViewGroup", av.LayoutTypeTable)}
			case "reuse-layout":
				v.Gallery = &av.LayoutGallery{BaseLayout: &av.BaseLayout{ID: "retained"}, CardSize: av.CardSizeSmall, CardFields: []*av.ViewGalleryCardField{{BaseField: &av.BaseField{ID: "number"}}}}
				h.tx.DoOperations = []*Operation{managementOp("changeAttrViewLayout", av.LayoutTypeTable)}
			}
			h.counts = map[string]int{}
			if ret := performTx(h.tx); ret != nil || !h.committed {
				t.Fatal(ret, h.counts)
			}
			switch scenario {
			case "mixed":
				if a.GetView("added") != nil || h.attrs[rowMirror][av.NodeAttrView] != a.ViewID || h.counts["count"] == 0 {
					t.Fatal("mixed candidate stale", h.counts)
				}
			case "reuse-layout":
				if v.Gallery.ID != "retained" || v.Gallery.CardSize != av.CardSizeSmall || len(v.Gallery.CardFields) != 1 {
					t.Fatal("layout cache overwritten")
				}
			default:
				if h.counts["stage:"+cellAV] != 0 || h.counts["attrs"] != 0 {
					t.Fatal("no-op changed source candidate", h.counts)
				}
			}
			assertFieldSourceUnchanged(t, h.cellHarness, before)
		})
	}
}

func TestAVManagementLegacyFactoryAndOwnedDefaults(t *testing.T) {
	e := (*Transaction)(nil).attributeViewManagementEnvironment()
	for _, action := range attributeViewManagementActions {
		if err := e.validate(action); err != nil {
			t.Fatal(action, err)
		}
	}
	for _, layout := range []av.LayoutType{av.LayoutTypeTable, av.LayoutTypeGallery, av.LayoutTypeKanban} {
		seq := 0
		v := newAttributeViewManagementView(layout, func(k string) string { return "owner " + k }, func() string { seq++; return "id" + string(rune('0'+seq)) })
		if v == nil || v.ID != "id1" || v.Name != "owner "+string(layout) || v.PageSize != av.ViewDefaultPageSize || len(v.Filters) != 0 || len(v.Sorts) != 0 {
			t.Fatal("owner view defaults", v)
		}
		if layout == av.LayoutTypeTable {
			if !v.Table.ShowIcon || v.Table.ID != "id2" {
				t.Fatal("table defaults")
			}
		} else if layout == av.LayoutTypeGallery {
			if v.Gallery.CoverFrom != av.CoverFromContentImage || v.Gallery.CardSize != av.CardSizeMedium || v.Gallery.CardAspectRatio != av.CardAspectRatio16_9 {
				t.Fatal("gallery defaults")
			}
		} else {
			if v.Kanban.CoverFrom != av.CoverFromContentImage || v.Kanban.CardSize != av.CardSizeMedium {
				t.Fatal("kanban defaults")
			}
		}
	}
}

func TestAVManagementMirrorCopiesShareDocument(t *testing.T) {
	m, store, root := managementSource(t)
	for _, page := range []string{"1760000000000000002", "1760000000000000003"} {
		t.Run(page, func(t *testing.T) {
			t.Parallel()
			l, err := m.Acquire(context.Background(), store, page)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(l.Release)
			h := newManagementHarness(t, l, root, page, "", av.LayoutTypeGallery)
			a := managementPrepare(t, h, av.LayoutTypeGallery)
			before := cellDigest(t, root, page)
			reg := managementRegistry(t, h)
			e := *h.env.avManagement
			// Legacy independent node loads may return separate objects for one document.
			e.node = func(id string) (*ast.Node, *parse.Tree, error) {
				tree, err := h.env.load(settingsBox, "/"+settingsDoc+".docj", h.env.parser())
				if err != nil {
					return nil, nil, err
				}
				return treenode.GetNodeInTree(tree, id), tree, nil
			}
			mirrors := e.mirrors
			e.mirrors = func(id string) ([]string, error) {
				ids, err := mirrors(id)
				if err != nil {
					return nil, err
				}
				ret := []string{managementMirror3}
				for _, id := range ids {
					if id != managementMirror3 {
						ret = append(ret, id)
					}
				}
				return ret, nil
			}
			if err := updateAttributeViewManagementMirrors(&e, a, "gallery", a.GetView("other")); err != nil {
				t.Fatal(err)
			}
			tree := h.tx.trees[settingsDoc]
			if tree == nil {
				t.Fatal("mirror document not staged")
			}
			for _, id := range []string{rowMirror, managementMirror2} {
				n := treenode.GetNodeInTree(tree, id)
				if n.IALAttr(av.NodeAttrView) != "other" || n.AttributeViewType != "table" {
					t.Fatal("independent node load lost update", id)
				}
			}
			if h.counts["flush-trees"] != 1 {
				t.Fatal("document flushed more than once")
			}
			assertManagementSource(t, h, before, reg)
		})
	}
}
