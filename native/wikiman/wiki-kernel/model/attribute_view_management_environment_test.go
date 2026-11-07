package model

import (
	"context"
	"encoding/json"
	"github.com/88250/lute"
	"github.com/88250/lute/parse"
	"github.com/vmihailenco/msgpack/v5"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"wiki-kernel/av"
	"wiki-kernel/internal/avrender"
	"wiki-kernel/internal/workspace"
	"wiki-kernel/treenode"
)

const managementMirror2 = "20261024110000-mir0002"
const managementMirror3 = "20261024110001-mir0003"

func managementFieldIDs(v *av.View) (ret []string) {
	for _, f := range managementFields(v) {
		ret = append(ret, f.ID)
	}
	return
}

func managementFields(v *av.View) (ret []*av.BaseField) {
	switch v.LayoutType {
	case av.LayoutTypeTable:
		for _, f := range v.Table.Columns {
			ret = append(ret, f.BaseField)
		}
	case av.LayoutTypeGallery:
		for _, f := range v.Gallery.CardFields {
			ret = append(ret, f.BaseField)
		}
	case av.LayoutTypeKanban:
		for _, f := range v.Kanban.Fields {
			ret = append(ret, f.BaseField)
		}
	}
	return
}

func managementSource(t *testing.T) (*workspace.WorkspaceManager, string, string) {
	t.Helper()
	m, store, root := rowSource(t)
	for _, page := range []string{"1760000000000000002", "1760000000000000003"} {
		file := filepath.Join(root, "pages", page, "data", settingsBox, settingsDoc+".docj")
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var doc map[string]interface{}
		if err = json.Unmarshal(raw, &doc); err != nil {
			t.Fatal(err)
		}
		children := doc["Children"].([]interface{})
		for _, id := range []string{managementMirror2, managementMirror3} {
			children = append(children, map[string]interface{}{"ID": id, "Type": "NodeAttributeView", "AttributeViewID": cellAV, "Properties": map[string]string{"id": id}})
		}
		doc["Children"] = children
		raw, err = json.Marshal(doc)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(file, raw, 0600); err != nil {
			t.Fatal(err)
		}
		raw, err = msgpack.Marshal(map[string][]string{cellAV: {rowMirror, managementMirror2, managementMirror3, rowMirror, "20261024090003-gon0001"}})
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(root, "pages", page, "data", "storage", "av", "blocks.msgpack"), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	return m, store, root
}

type managementHarness struct{ *rowHarness }

func newManagementHarness(t *testing.T, l *workspace.Lease, root, page, failure string, layout av.LayoutType) *managementHarness {
	r := newRowHarness(t, l, root, page, failure)
	h := r.cellHarness
	mh := &managementHarness{r}
	load := h.env.load
	h.env.load = func(box, p string, engine *lute.Lute) (*parse.Tree, error) {
		tree, err := load(box, p, engine)
		if err != nil {
			return nil, err
		}
		for _, id := range []string{rowMirror, managementMirror2, managementMirror3} {
			n := treenode.GetNodeInTree(tree, id)
			selected := string(layout)
			if id == managementMirror3 {
				selected = "other"
			}
			n.SetIALAttr(av.NodeAttrView, selected)
			n.AttributeViewType = string(layout)
		}
		return tree, nil
	}
	label := func(key string) string { return page + " " + key }
	h.cell.groups = func(a *av.AttributeView) error {
		if err := h.mark("groups:" + a.ID); err != nil {
			return err
		}
		for _, v := range a.Views {
			if v.GetGroupKey(a) == nil {
				continue
			}
			err := avrender.GenerateGroups(v, a, avrender.GroupEnvironment{Now: h.cell.now, Read: h.cell.read, NewView: func(layout av.LayoutType) (*av.View, error) {
				return newAttributeViewManagementView(layout, label, h.cell.newID), nil
			}, Render: func(a *av.AttributeView, v *av.View, q string) (av.Viewable, error) {
				e := cellRenderEnvironment(h)
				switch v.LayoutType {
				case av.LayoutTypeGallery:
					return avrender.Gallery(a, v, q, e)
				case av.LayoutTypeKanban:
					return avrender.Kanban(a, v, q, e)
				default:
					return avrender.Table(a, v, q, e)
				}
			}})
			if err != nil {
				return err
			}
		}
		return h.ctx.Err()
	}
	e := &attributeViewManagementEnvironment{ctx: h.ctx, read: h.cell.read, stage: h.cell.stage, view: h.tx.attributeViewSettingsEnvironment().view, groups: h.cell.groups,
		count: func(a *av.AttributeView, v, g *av.View) (int, error) {
			if err := h.mark("count"); err != nil {
				return 0, err
			}
			c, err := orderCollection(h, a, v, g)
			if err != nil {
				return 0, err
			}
			av.Calc(c.(av.Viewable), a)
			return c.CountItems(), nil
		},
		sortGroups: func(a *av.AttributeView, v *av.View) error {
			if err := h.mark("sort-groups"); err != nil {
				return err
			}
			avrender.SortGroups(a, v, h.cell.now())
			return nil
		},
		makeView: func(layout av.LayoutType) *av.View {
			if err := h.mark("make-view"); err != nil {
				return nil
			}
			return newAttributeViewManagementView(layout, label, h.cell.newID)
		}, language: label,
		cloneView: func(v *av.View) (*av.View, error) {
			if err := h.mark("clone-view"); err != nil {
				return nil, err
			}
			return cloneAttributeViewManagementView(v)
		},
		mirrors: r.historyEnv.mirrors, node: h.cell.node, attrs: h.cell.attrs, mirrorAttrs: h.cell.attrs,
		flushTrees: func(trees []*parse.Tree) error {
			if err := h.mark("flush-trees"); err != nil {
				return err
			}
			for _, tree := range trees {
				if err := h.tx.writeTree(tree); err != nil {
					return err
				}
			}
			return h.ctx.Err()
		}, reload: func(id string) error {
			if err := h.mark("management-reload"); err != nil {
				return err
			}
			h.events = append(h.events, "reload:"+id)
			return nil
		},
	}
	h.env.avManagement = e
	return mh
}
func managementPrepare(t *testing.T, h *managementHarness, layout av.LayoutType) *av.AttributeView {
	a := rowPrepare(t, h.rowHarness, layout)
	a.Views = append(a.Views, &av.View{ID: "other", Name: "other", LayoutType: av.LayoutTypeTable, Table: av.NewLayoutTable()})
	return a
}
func managementOp(action string, layout av.LayoutType) *Operation {
	op := &Operation{Action: action, AvID: cellAV, BlockID: rowMirror, ID: string(layout), KeyID: "number", Data: true}
	switch action {
	case "setAttrViewViewName", "setAttrViewViewIcon", "setAttrViewViewDesc":
		op.Data = " edited "
	case "setAttrViewCoverFrom", "setAttrViewCardSize", "setAttrViewCardAspectRatio":
		op.Data = float64(2)
	case "hideAttrViewGroup":
		op.Data = float64(2)
	case "setAttrViewGroup":
		op.Data = &av.ViewGroup{Field: "status", Method: av.GroupMethodValue, HideEmpty: true}
	case "addAttrViewView":
		op.ID = "added"
		op.Layout = layout
	case "duplicateAttrViewView":
		op.ID = "copy"
		op.PreviousID = string(layout)
	case "sortAttrViewView":
		op.PreviousID = "other"
	case "changeAttrViewLayout":
		op.Layout = av.LayoutTypeGallery
		if layout == op.Layout {
			op.Layout = av.LayoutTypeTable
		}
	case "setAttrViewBlockView":
		op.ID = "other"
	}
	return op
}
func assertManagementSource(t *testing.T, h *managementHarness, before map[string]string, registry []byte) {
	t.Helper()
	assertRowsUnchanged(t, h.rowHarness, before, registry)
}
func managementRegistry(t *testing.T, h *managementHarness) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(h.root, "pages", h.page, "data", "storage", "av", "blocks.msgpack"))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestAVManagementSettingsOwnersAndLayouts(t *testing.T) {
	m, store, root := managementSource(t)
	actions := []string{"setAttrViewViewName", "setAttrViewViewIcon", "setAttrViewViewDesc", "hideAttrViewName", "setAttrViewCoverFrom", "setAttrViewCoverFromAssetKeyID", "setAttrViewCardSize", "setAttrViewCardAspectRatio", "setAttrViewFitImage", "setAttrViewDisplayFieldName", "setAttrViewFillColBackgroundColor", "setAttrViewShowIcon", "setAttrViewWrapField", "syncAttrViewTableColWidth"}
	for _, page := range []string{"1760000000000000002", "1760000000000000003"} {
		t.Run(page, func(t *testing.T) {
			t.Parallel()
			l, err := m.Acquire(context.Background(), store, page)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(l.Release)
			for _, layout := range []av.LayoutType{av.LayoutTypeTable, av.LayoutTypeGallery, av.LayoutTypeKanban} {
				for _, action := range actions {
					t.Run(string(layout)+action, func(t *testing.T) {
						h := newManagementHarness(t, l, root, page, "", layout)
						a := managementPrepare(t, h, layout)
						v := a.GetView(string(layout))
						before := cellDigest(t, root, page)
						reg := managementRegistry(t, h)
						op := managementOp(action, layout)
						other := a.GetView("other")
						other.Table.Columns = []*av.ViewTableColumn{{BaseField: &av.BaseField{ID: "number"}}}
						if layout == av.LayoutTypeTable {
							for _, c := range v.Table.Columns {
								c.Width = "321px"
							}
						}
						untouched, _ := json.Marshal(other)
						h.tx.DoOperations = []*Operation{op}
						if ret := performTx(h.tx); ret != nil || !h.committed {
							t.Fatal(ret, h.counts)
						}
						switch action {
						case "setAttrViewViewName":
							if v.Name != "edited" {
								t.Fatal(v.Name)
							}
						case "setAttrViewViewIcon":
							if v.Icon != " edited " {
								t.Fatal(v.Icon)
							}
						case "setAttrViewViewDesc":
							if v.Desc != "edited" {
								t.Fatal(v.Desc)
							}
						case "hideAttrViewName":
							if !v.HideAttrViewName {
								t.Fatal("name visible")
							}
						case "syncAttrViewTableColWidth":
							if layout == av.LayoutTypeTable {
								if other.Table.Columns[0].Width != "321px" || h.counts["management-reload"] != 1 {
									t.Fatal("width sync")
								}
							} else if h.counts["stage:"+cellAV] != 0 {
								t.Fatal("card width staged")
							}
						default:
							var base *av.BaseLayout
							if layout == av.LayoutTypeTable {
								base = v.Table.BaseLayout
							} else if layout == av.LayoutTypeGallery {
								base = v.Gallery.BaseLayout
							} else {
								base = v.Kanban.BaseLayout
							}
							if action == "setAttrViewShowIcon" && !base.ShowIcon {
								t.Fatal("icon")
							}
							if action == "setAttrViewWrapField" {
								if !base.WrapField {
									t.Fatal("wrap")
								}
								for _, field := range managementFields(v) {
									if !field.Wrap {
										t.Fatal("field wrap")
									}
								}
							}
							if layout != av.LayoutTypeTable {
								var cover av.CoverFrom
								var size av.CardSize
								var ratio av.CardAspectRatio
								var fit, display bool
								var asset string
								if layout == av.LayoutTypeGallery {
									g := v.Gallery
									cover, size, ratio, fit, display, asset = g.CoverFrom, g.CardSize, g.CardAspectRatio, g.FitImage, g.DisplayFieldName, g.CoverFromAssetKeyID
								} else {
									g := v.Kanban
									cover, size, ratio, fit, display, asset = g.CoverFrom, g.CardSize, g.CardAspectRatio, g.FitImage, g.DisplayFieldName, g.CoverFromAssetKeyID
								}
								if (action == "setAttrViewCoverFrom" && cover != 2) || (action == "setAttrViewCoverFromAssetKeyID" && asset != "number") || (action == "setAttrViewCardSize" && size != 2) || (action == "setAttrViewCardAspectRatio" && ratio != 2) || (action == "setAttrViewFitImage" && !fit) || (action == "setAttrViewDisplayFieldName" && !display) || (action == "setAttrViewFillColBackgroundColor" && layout == av.LayoutTypeKanban && !v.Kanban.FillColBackgroundColor) {
									t.Fatal("card metadata", action)
								}
							}
						}
						if action != "syncAttrViewTableColWidth" {
							raw, _ := json.Marshal(other)
							if string(raw) != string(untouched) {
								t.Fatal("other view changed")
							}
						}
						assertManagementSource(t, h, before, reg)
					})
				}
			}
		})
	}
}

func TestAVManagementViewLifecycleAndMirrors(t *testing.T) {
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
				for _, action := range []string{"addAttrViewView", "duplicateAttrViewView", "removeAttrViewView", "sortAttrViewView", "setAttrViewBlockView", "changeAttrViewLayout"} {
					t.Run(string(layout)+action, func(t *testing.T) {
						h := newManagementHarness(t, l, root, page, "", layout)
						a := managementPrepare(t, h, layout)
						v := a.GetView(string(layout))
						before := cellDigest(t, root, page)
						reg := managementRegistry(t, h)
						op := managementOp(action, layout)
						v.Name = page + " " + string(layout)
						v.ItemIDs = []string{settingsBlock, cellSecond}
						v.Filters = []*av.ViewFilter{{Column: "number", Value: &av.Value{Type: av.KeyTypeNumber, Number: &av.ValueNumber{Content: 6}}}}
						v.Filters[0].Operator = av.FilterOperatorIsGreater
						if layout == av.LayoutTypeTable {
							v.Table.Columns[0].Calc = &av.FieldCalc{Operator: av.CalcOperatorSum}
						}
						expectedIDs := managementFieldIDs(v)
						h.tx.DoOperations = []*Operation{op}
						if ret := performTx(h.tx); ret != nil || !h.committed {
							t.Fatal(ret, h.counts)
						}
						switch action {
						case "addAttrViewView":
							new := a.GetView("added")
							if new == nil || new.LayoutType != layout || a.ViewID != "added" || !slices.Equal(managementFieldIDs(new), managementFieldIDs(a.Views[0])) || new.Name != page+" "+string(layout) {
								t.Fatal("add view", new)
							}
							new.ItemIDs[0] = "changed"
							if a.Views[0].ItemIDs[0] == "changed" {
								t.Fatal("new view item alias")
							}
							if h.attrs[rowMirror][av.NodeAttrView] != "added" {
								t.Fatal("active block not selected")
							}
						case "duplicateAttrViewView":
							new := a.GetView("copy")
							if new == nil || new.Name != v.Name+" (1)" || !slices.Equal(managementFieldIDs(new), expectedIDs) || !slices.Equal(new.ItemIDs, v.ItemIDs) {
								t.Fatal("duplicate view", new)
							}
							new.Filters[0].Value.Number.Content = 999
							new.ItemIDs[0] = "changed"
							if v.Filters[0].Value.Number.Content == 999 || v.ItemIDs[0] == "changed" {
								t.Fatal("duplicate alias")
							}
							if layout == av.LayoutTypeTable {
								new.Table.Columns[0].Calc.Operator = "Changed"
								if v.Table.Columns[0].Calc.Operator == "Changed" {
									t.Fatal("calc alias")
								}
							}
						case "removeAttrViewView":
							if a.GetView(string(layout)) != nil || a.ViewID == string(layout) || op.RetData == nil {
								t.Fatal("remove selection", a.ViewID, op.RetData)
							}
							for _, id := range []string{rowMirror, managementMirror2} {
								if h.attrs[id][av.NodeAttrView] != a.ViewID {
									t.Fatal("mirror fallback", id, h.attrs)
								}
							}
							if h.attrs[managementMirror3] != nil {
								t.Fatal("other mirror touched")
							}
							if h.counts["flush-trees"] != 1 {
								t.Fatal("mirror trees not staged")
							}
						case "sortAttrViewView":
							if a.Views[len(a.Views)-1] != v {
								t.Fatal("view order")
							}
						case "setAttrViewBlockView":
							if a.ViewID != "other" || h.attrs[rowMirror][av.NodeAttrView] != "other" {
								t.Fatal("block switch")
							}
						case "changeAttrViewLayout":
							if v.LayoutType != op.Layout || v.Name != page+" "+string(op.Layout) || !slices.Equal(managementFieldIDs(v), expectedIDs) {
								t.Fatal("layout inheritance", v.LayoutType, v.Name, managementFieldIDs(v), expectedIDs)
							}
							for _, id := range []string{rowMirror, managementMirror2} {
								n := treenode.GetNodeInTree(h.tx.trees[settingsDoc], id)
								if n.AttributeViewType != string(op.Layout) {
									t.Fatal("mirror type", id)
								}
							}
							if h.attrs[managementMirror3] != nil {
								t.Fatal("other mirror type touched")
							}
						}
						assertManagementSource(t, h, before, reg)
					})
				}
			}
		})
	}
}
