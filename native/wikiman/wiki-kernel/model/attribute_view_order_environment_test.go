package model

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
	"text/template"

	"wiki-kernel/av"
	"wiki-kernel/internal/avmodel"
	"wiki-kernel/internal/avrender"
	"wiki-kernel/internal/workspace"
)

func orderHarness(t *testing.T, l *workspace.Lease, root, page, failure string) *cellHarness {
	h := newCellHarness(t, l, root, page, failure)
	h.env.avConfig = &attributeViewEnvironment{read: h.cell.read, stage: h.cell.stage}
	h.env.avOrder = &attributeViewOrderEnvironment{read: h.cell.read, stage: h.cell.stage, view: h.tx.attributeViewSettingsEnvironment().view, groups: h.cell.groups, cloneKey: func(k *av.Key) (*av.Key, error) {
		if err := h.mark("clone"); err != nil {
			return nil, err
		}
		return cloneAttributeViewOrderKey(k)
	}}
	h.env.avOrder.defaults = &attributeViewDefaultEnvironment{ctx: h.ctx, now: h.cell.now, newID: h.cell.newID,
		near: func(a *av.AttributeView, v, g *av.View, p string) (av.Item, error) {
			if err := h.mark("near"); err != nil {
				return nil, err
			}
			c, err := orderCollection(h, a, v, g)
			if err != nil {
				return nil, err
			}
			return selectAttributeViewNearItem(c, p), nil
		},
		templateKeys: func(a *av.AttributeView, k *av.Key) ([]*av.Key, error) {
			if err := h.mark("template"); err != nil {
				return nil, err
			}
			e := avrender.TemplateEnvironment{Context: h.ctx, Funcs: template.FuncMap{}, Now: h.cell.now}
			if _, err := avrender.TemplateVars(k.Template, e); err != nil {
				return nil, err
			}
			return avrender.GetTemplateKeyRelevantKeys(a, k, e), nil
		},
		reverse: func(a *av.AttributeView, k *av.Key, v *av.Value) error {
			if err := h.mark("reverse"); err != nil {
				return err
			}
			e := *h.cell
			e.ctx = h.ctx
			e.legacy = false
			return updateTwoWayRelationDestWithEnvironment(&e, a, k, v, 1, []string{})
		}}
	return h
}
func orderCollection(h *cellHarness, a *av.AttributeView, v, g *av.View) (av.Collection, error) {
	if err := avrender.PrepareGroup(v, g); err != nil {
		return nil, err
	}
	e := cellRenderEnvironment(h)
	e.Templates = func(a *av.AttributeView, v *av.View, c av.Collection, ials map[string]map[string]string) error {
		return avrender.FillTemplateValues(a, v, c, ials, avrender.TemplateEnvironment{Context: h.ctx, Funcs: template.FuncMap{}, Now: h.cell.now, Name: func(a *av.AttributeView) string { return a.Name }})
	}
	var c av.Collection
	var err error
	switch g.LayoutType {
	case av.LayoutTypeTable:
		c, err = avrender.Table(a, g, "", e)
	case av.LayoutTypeGallery:
		c, err = avrender.Gallery(a, g, "", e)
	case av.LayoutTypeKanban:
		c, err = avrender.Kanban(a, g, "", e)
	}
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, errors.New("missing collection")
	}
	a.ReadAttributeView = h.cell.read
	if err = avmodel.FilterChecked(c.(av.Viewable), a, nil, map[string]*av.AttributeView{a.ID: a}); err != nil {
		return nil, err
	}
	av.Sort(c.(av.Viewable), a)
	return c, nil
}
func orderPrepare(t *testing.T, h *cellHarness, layout av.LayoutType) *av.AttributeView {
	a, err := h.cell.read(cellAV)
	if err != nil {
		t.Fatal(err)
	}
	a.ViewID = string(layout)
	a.KeyIDs = nil
	for _, kv := range a.KeyValues {
		a.KeyIDs = append(a.KeyIDs, kv.Key.ID)
	}
	for _, v := range a.Views {
		v.Group = nil
		v.Groups = nil
		v.ItemIDs = []string{settingsBlock, cellSecond}
	}
	a.RenderedViewables = map[string]av.Viewable{"stale": &av.Table{}}
	h.counts = map[string]int{}
	return a
}
func orderOp(action string) *Operation {
	return &Operation{Action: action, AvID: cellAV, BlockID: settingsBlock, ID: "number", KeyID: "number", NextID: "copy-number", PreviousID: "primary"}
}
func orderGroup(t *testing.T, h *cellHarness, layout av.LayoutType, key string) (*av.AttributeView, *av.View, string, string) {
	a := orderPrepare(t, h, layout)
	kv, _ := a.GetKeyValues("status")
	kv.Key.Type = av.KeyTypeSelect
	kv.Key.Options = []*av.SelectOption{{Name: "Old", Color: "2"}, {Name: "New", Color: "4"}, {Name: "Keep", Color: "6"}}
	kv.Values[0].MSelect = []*av.ValueSelect{{Content: "Old", Color: "2"}}
	kv.Values[1].MSelect = []*av.ValueSelect{{Content: "New", Color: "4"}}
	for i, v := range a.GetBlockKeyValues().Values {
		v.Block.Content = []string{"Old", "New"}[i]
	}
	for _, v := range a.Views {
		v.Group = &av.ViewGroup{Field: key, Method: av.GroupMethodValue}
		v.Groups = nil
	}
	a.RenderedViewables = map[string]av.Viewable{}
	if err := h.cell.groups(a); err != nil {
		t.Fatal(err)
	}
	v := a.GetView(string(layout))
	old, new := "", ""
	for _, g := range v.Groups {
		switch g.GetGroupValue() {
		case "Old":
			old = g.ID
		case "New":
			new = g.ID
		}
	}
	if old == "" || new == "" {
		t.Fatal("fixture groups missing", old, new)
	}
	h.counts = map[string]int{}
	return a, v, old, new
}

func TestOriginalAVOrderRulesLayoutsAndOwners(t *testing.T) {
	m, store, root := fieldSource(t)
	for _, page := range []string{"1760000000000000002", "1760000000000000003"} {
		t.Run(page, func(t *testing.T) {
			t.Parallel()
			l, err := m.Acquire(context.Background(), store, page)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(l.Release)
			for _, layout := range []av.LayoutType{av.LayoutTypeTable, av.LayoutTypeGallery, av.LayoutTypeKanban} {
				for _, action := range []string{"sortAttrViewRow", "sortAttrViewCol", "sortAttrViewKey"} {
					for _, scenario := range []string{"after", "front", "missing-previous", "self", "missing-current"} {
						t.Run(string(layout)+"/"+action+"/"+scenario, func(t *testing.T) {
							h := orderHarness(t, l, root, page, "")
							before := cellDigest(t, root, page)
							a := orderPrepare(t, h, layout)
							v := a.GetView(string(layout))
							op := orderOp(action)
							oldOther := map[string]string{}
							for _, other := range a.Views {
								if other.ID != v.ID {
									b, _ := json.Marshal(other)
									oldOther[other.ID] = string(b)
								}
							}
							var old []string
							switch action {
							case "sortAttrViewRow":
								old = slices.Clone(v.ItemIDs)
								op.ID = cellSecond
								op.PreviousID = settingsBlock
							case "sortAttrViewCol":
								old = fieldIDs(v)
							case "sortAttrViewKey":
								old = slices.Clone(a.KeyIDs)
							}
							switch scenario {
							case "front":
								op.PreviousID = ""
							case "missing-previous":
								op.PreviousID = "gone"
							case "self":
								op.PreviousID = op.ID
							case "missing-current":
								op.ID = "gone"
							}
							want := slices.Clone(old)
							stages := 1
							if scenario == "self" {
								stages = 0
							} else {
								idx := slices.Index(want, op.ID)
								if idx < 0 && action != "sortAttrViewRow" {
									stages = 0
								} else {
									if idx >= 0 {
										want = slices.Delete(want, idx, idx+1)
									}
									at := slices.Index(want, op.PreviousID) + 1
									want = slices.Insert(want, at, op.ID)
								}
							}
							h.tx.DoOperations = []*Operation{op}
							if ret := performTx(h.tx); ret != nil || !h.committed {
								t.Fatal(ret, h.counts)
							}
							var got []string
							switch action {
							case "sortAttrViewRow":
								got = v.ItemIDs
							case "sortAttrViewCol":
								got = fieldIDs(v)
							case "sortAttrViewKey":
								got = a.KeyIDs
							}
							if !reflect.DeepEqual(got, want) || h.counts["stage:"+cellAV] != stages {
								t.Fatal("original order/no-op changed", got, want, h.counts)
							}
							for _, other := range a.Views {
								if saved, ok := oldOther[other.ID]; ok {
									b, _ := json.Marshal(other)
									if string(b) != saved {
										t.Fatal("other view changed")
									}
								}
							}
							if !strings.HasPrefix(a.Name, page) {
								t.Fatal("wrong owner")
							}
							assertFieldSourceUnchanged(t, h, before)
						})
					}
				}
			}
		})
	}
}

func TestOriginalAVOrderGroupsFiltersAndReverseRelations(t *testing.T) {
	m, store, root := fieldSource(t)
	for _, page := range []string{"1760000000000000002", "1760000000000000003"} {
		t.Run(page, func(t *testing.T) {
			t.Parallel()
			l, err := m.Acquire(context.Background(), store, page)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(l.Release)
			for _, layout := range []av.LayoutType{av.LayoutTypeTable, av.LayoutTypeGallery, av.LayoutTypeKanban} {
				for _, scenario := range []string{"same", "across", "multi", "filtered", "reverse", "empty-target", "target-already", "target-missing", "group-missing", "generated-template", "generated-created", "generated-updated"} {
					t.Run(string(layout)+"/"+scenario, func(t *testing.T) {
						h := orderHarness(t, l, root, page, "")
						before := cellDigest(t, root, page)
						a, v, old, target := orderGroup(t, h, layout, "status")
						status, _ := a.GetKeyValues("status")
						op := orderOp("sortAttrViewRow")
						op.ID = settingsBlock
						op.PreviousID = cellSecond
						op.GroupID = old
						op.TargetGroupID = target
						if scenario == "same" {
							op.TargetGroupID = old
							op.PreviousID = ""
						}
						if scenario == "multi" {
							status.Key.Type = av.KeyTypeMSelect
							for _, val := range status.Values {
								val.Type = av.KeyTypeMSelect
							}
							status.Values[0].MSelect = append(status.Values[0].MSelect, &av.ValueSelect{Content: "Keep", Color: "6"})
						}
						if scenario == "filtered" {
							n, _ := a.GetKeyValues("number")
							n.Values[0].Number = &av.ValueNumber{Content: 7, IsNotEmpty: true}
							n.Values[1].Number = &av.ValueNumber{Content: 9, IsNotEmpty: true}
							v.Filters = []*av.ViewFilter{{Column: "number", Operator: av.FilterOperatorIsGreater, Value: &av.Value{Type: av.KeyTypeNumber, Number: &av.ValueNumber{Content: 5, IsNotEmpty: true}}}}
						}
						if scenario == "reverse" {
							kv, _ := a.GetKeyValues("relation")
							kv.Values[1].Relation.BlockIDs = []string{cellSecond}
							dest, _ := h.cell.read(cellDest)
							dest.GetValue("back", cellSecond).Relation.BlockIDs = nil
							v.Filters = []*av.ViewFilter{{Column: "relation", Operator: av.FilterOperatorIsNotEmpty, Value: &av.Value{Type: av.KeyTypeRelation, Relation: &av.ValueRelation{BlockIDs: []string{cellSecond}}}}}
						}
						if scenario == "empty-target" {
							v.GetGroupByID(target).GroupItemIDs = nil
							op.PreviousID = ""
						}
						if scenario == "target-already" {
							v.GetGroupByID(target).GroupItemIDs = append(v.GetGroupByID(target).GroupItemIDs, settingsBlock)
						}
						if scenario == "target-missing" {
							op.TargetGroupID = "missing"
						}
						if scenario == "group-missing" {
							op.GroupID = "missing"
						}
						if strings.HasPrefix(scenario, "generated-") {
							typ := av.KeyType(strings.TrimPrefix(scenario, "generated-"))
							status.Key.Type = typ
						}
						h.tx.DoOperations = []*Operation{op}
						if ret := performTx(h.tx); ret != nil || !h.committed {
							t.Fatal(ret, h.counts)
						}
						cross := scenario == "across" || scenario == "multi" || scenario == "filtered" || scenario == "reverse" || scenario == "empty-target"
						if cross {
							vals := status.Values[0].MSelect
							if av.MSelectExistOption(vals, "Old") || !av.MSelectExistOption(vals, "New") {
								t.Fatal("group value did not move", vals)
							}
							if scenario == "multi" && !av.MSelectExistOption(vals, "Keep") {
								t.Fatal("unrelated multiselect removed")
							}
							if h.counts["near"] != 1 || h.counts["groups:"+cellAV] != 1 {
								t.Fatal("default/group core not executed", h.counts)
							}
						}
						if scenario == "filtered" && a.GetValue("number", settingsBlock).Number.Content != 6 {
							t.Fatal("filter default lost")
						}
						if scenario == "reverse" {
							dest := h.staged[cellDest]
							if dest == nil || !slices.Contains(dest.GetValue("back", cellSecond).Relation.BlockIDs, settingsBlock) || h.counts["reverse"] == 0 {
								t.Fatal("reverse relation not staged", h.counts)
							}
						}
						if strings.HasPrefix(scenario, "generated-") && (h.counts["stage:"+cellAV] != 0 || h.counts["near"] != 0) {
							t.Fatal("generated group should reject cross-move as no-op")
						}
						if scenario == "same" || scenario == "group-missing" {
							if h.counts["groups:"+cellAV] != 0 || h.counts["near"] != 0 {
								t.Fatal("ordinary/same group added default effects")
							}
						}
						assertFieldSourceUnchanged(t, h, before)
					})
				}
			}
		})
	}
}

func TestOriginalAVDuplicateConfigurationIsolationAndLayouts(t *testing.T) {
	m, store, root := fieldSource(t)
	for _, page := range []string{"1760000000000000002", "1760000000000000003"} {
		t.Run(page, func(t *testing.T) {
			t.Parallel()
			l, err := m.Acquire(context.Background(), store, page)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(l.Release)
			for _, key := range []string{"text", "number", "date", "status", "multi", "primary", "relation", "missing"} {
				t.Run(key, func(t *testing.T) {
					h := orderHarness(t, l, root, page, "")
					before := cellDigest(t, root, page)
					a := orderPrepare(t, h, av.LayoutTypeTable)
					k, _ := a.GetKey(key)
					if k != nil {
						k.Name = "Name (2)"
						k.Date = &av.Date{AutoFillNow: true}
						k.Options = []*av.SelectOption{{Name: "Old", Color: "2"}}
					}
					for _, v := range a.Views {
						if f := settingField(v, key); f != nil {
							f.Wrap = true
							f.Hidden = true
							f.Desc = "field desc"
						}
						if v.Table != nil {
							for _, c := range v.Table.Columns {
								if c.ID == key {
									c.Width = "123px"
									c.Pin = true
									c.Calc = &av.FieldCalc{Operator: av.CalcOperatorSum}
								}
							}
						}
					}
					op := orderOp("duplicateAttrViewKey")
					op.KeyID = key
					op.NextID = "copied"
					h.tx.DoOperations = []*Operation{op}
					if ret := performTx(h.tx); ret != nil || !h.committed {
						t.Fatal(ret)
					}
					copy, _ := a.GetKey("copied")
					supported := key != "primary" && key != "relation" && key != "missing"
					if (copy != nil) != supported {
						t.Fatal("copy restriction changed")
					}
					if supported {
						kv, _ := a.GetKeyValues("copied")
						if len(kv.Values) != 0 || copy.Name != "Name (3)" {
							t.Fatal("copied values/name changed")
						}
						for _, v := range a.Views {
							ids := fieldIDs(v)
							if slices.Index(ids, "copied") != slices.Index(ids, key)+1 {
								t.Fatal("copy not adjacent", ids)
							}
							f := settingField(v, "copied")
							if !f.Hidden || !f.Wrap || f.Desc != "field desc" {
								t.Fatal("field settings lost")
							}
							if v.Table != nil {
								for _, c := range v.Table.Columns {
									if c.ID == "copied" && (!c.Pin || c.Width != "123px" || c.Calc != nil) {
										t.Fatal("original table copy policy changed")
									}
								}
							}
						}
						copy.Options[0].Color = "9"
						copy.Date.AutoFillNow = false
						if k.Options[0].Color != "2" || !k.Date.AutoFillNow {
							t.Fatal("nested copy aliases original")
						}
					}
					assertFieldSourceUnchanged(t, h, before)
				})
			}
		})
	}
}
