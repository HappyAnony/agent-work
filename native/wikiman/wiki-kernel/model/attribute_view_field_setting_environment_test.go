package model

import (
	"context"
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"

	"wiki-kernel/av"
	"wiki-kernel/internal/workspace"
)

var fieldSettingActions = []string{"setAttrViewColWidth", "setAttrViewColWrap", "setAttrViewColHidden", "setAttrViewColPin", "setAttrViewColIcon", "setAttrViewColDesc", "updateAttrViewColNumberFormat", "setAttrViewColCalc", "setAttrViewColDateFillCreated", "setAttrViewColDateFillSpecificTime", "setAttrViewCreatedIncludeTime", "setAttrViewUpdatedIncludeTime"}

func fieldSettingOp(action string, on bool) *Operation {
	op := &Operation{Action: action, AvID: cellAV, BlockID: settingsBlock, ID: "number", Data: on, Typ: "number", Format: "percent"}
	switch action {
	case "setAttrViewColWidth":
		op.Data = " 240px "
	case "setAttrViewColIcon":
		op.Data = "icon-test"
	case "setAttrViewColDesc":
		op.Data = " untrimmed description "
	case "setAttrViewColCalc":
		op.Data = map[string]interface{}{"operator": "Sum"}
	case "setAttrViewColDateFillCreated", "setAttrViewColDateFillSpecificTime":
		op.ID = "date"
	case "setAttrViewCreatedIncludeTime":
		op.ID = "created"
	case "setAttrViewUpdatedIncludeTime":
		op.ID = "updated"
	}
	return op
}

func fieldSettingHarness(t *testing.T, l *workspace.Lease, root, page, failure string) *cellHarness {
	h := newCellHarness(t, l, root, page, failure)
	h.env.avConfig = &attributeViewEnvironment{read: h.cell.read, stage: h.cell.stage}
	h.env.avCell = nil
	return h
}

func fieldSettingPrepare(t *testing.T, h *cellHarness, layout av.LayoutType) *av.AttributeView {
	a, err := h.cell.read(cellAV)
	if err != nil {
		t.Fatal(err)
	}
	a.ViewID = string(layout)
	for _, typ := range []av.KeyType{av.KeyTypeCreated, av.KeyTypeUpdated} {
		id := string(typ)
		a.KeyValues = append(a.KeyValues, &av.KeyValues{Key: &av.Key{ID: id, Type: typ}})
	}
	for _, v := range a.Views {
		for _, id := range []string{"created", "updated"} {
			addFieldFixture(v, id)
		}
		settingFields(v, func(f *av.BaseField) { f.Wrap = true })
	}
	kv, _ := a.GetKeyValues("date")
	kv.Key.Date = nil
	kv.Values[0].Date = nil
	kv.Values[1].Date = &av.ValueDate{Content: 12345, IsNotEmpty: true, IsNotTime: true, HasEndDate: true, Content2: 23456, IsNotEmpty2: true}
	kv.Values = append(kv.Values, &av.Value{ID: "empty-date", Type: av.KeyTypeDate, KeyID: "date", BlockID: "empty-row", Date: &av.ValueDate{Content: 99, IsNotEmpty: false, IsNotTime: true}})
	a.RenderedViewables = map[string]av.Viewable{"stale": &av.Table{}}
	h.counts = map[string]int{}
	return a
}

func settingFields(v *av.View, fn func(*av.BaseField)) {
	if v.Table != nil {
		for _, f := range v.Table.Columns {
			fn(f.BaseField)
		}
	}
	if v.Gallery != nil {
		for _, f := range v.Gallery.CardFields {
			fn(f.BaseField)
		}
	}
	if v.Kanban != nil {
		for _, f := range v.Kanban.Fields {
			fn(f.BaseField)
		}
	}
}
func settingField(v *av.View, id string) *av.BaseField {
	var ret *av.BaseField
	settingFields(v, func(f *av.BaseField) {
		if f.ID == id {
			ret = f
		}
	})
	return ret
}

func expectedFieldSetting(a *av.AttributeView, op *Operation, viewID string) int {
	v := a.GetView(viewID)
	key, _ := a.GetKey(op.ID)
	switch op.Action {
	case "setAttrViewColWidth":
		if v.Table == nil {
			return 0
		}
		for _, f := range v.Table.Columns {
			if f.ID == op.ID {
				f.Width = op.Data.(string)
				break
			}
		}
	case "setAttrViewColPin":
		if v.Table == nil {
			return 0
		}
		for _, f := range v.Table.Columns {
			if f.ID == op.ID {
				f.Pin = op.Data.(bool)
				break
			}
		}
	case "setAttrViewColHidden":
		if f := settingField(v, op.ID); f != nil {
			f.Hidden = op.Data.(bool)
		}
	case "setAttrViewColWrap":
		all := true
		settingFields(v, func(f *av.BaseField) {
			if f.ID == op.ID {
				f.Wrap = op.Data.(bool)
			}
			all = all && f.Wrap
		})
		if v.Table != nil {
			v.Table.WrapField = all
		}
		if v.Gallery != nil {
			v.Gallery.WrapField = all
		}
		if v.Kanban != nil {
			v.Kanban.WrapField = all
		}
	case "setAttrViewColIcon":
		if key != nil {
			key.Icon = op.Data.(string)
		}
	case "setAttrViewColDesc":
		if key != nil {
			key.Desc = op.Data.(string)
		}
	case "updateAttrViewColNumberFormat":
		if key != nil && key.Type == av.KeyTypeNumber && op.Typ == "number" {
			key.NumberFormat = av.NumberFormat(op.Format)
		}
	case "setAttrViewColCalc":
		if v.Table == nil {
			return 0
		}
		for _, f := range v.Table.Columns {
			if f.ID == op.ID {
				f.Calc = &av.FieldCalc{Operator: av.CalcOperatorSum}
				break
			}
		}
	case "setAttrViewColDateFillCreated":
		if key == nil || key.Type != av.KeyTypeDate {
			return 0
		}
		if key.Date == nil {
			key.Date = &av.Date{}
		}
		key.Date.AutoFillNow = op.Data.(bool)
	case "setAttrViewColDateFillSpecificTime":
		if key == nil || key.Type != av.KeyTypeDate {
			return 0
		}
		if key.Date == nil {
			key.Date = &av.Date{}
		}
		key.Date.FillSpecificTime = op.Data.(bool)
		kv, _ := a.GetKeyValues(op.ID)
		for _, value := range kv.Values {
			if value.IsEmpty() {
				if value.Date == nil {
					value.Date = &av.ValueDate{}
				}
				value.Date.IsNotTime = !op.Data.(bool)
			}
		}
	case "setAttrViewCreatedIncludeTime":
		if key == nil {
			return 0
		}
		if key.Created == nil {
			key.Created = &av.Created{}
		}
		key.Created.IncludeTime = op.Data.(bool)
	case "setAttrViewUpdatedIncludeTime":
		if key == nil {
			return 0
		}
		if key.Updated == nil {
			key.Updated = &av.Updated{}
		}
		key.Updated.IncludeTime = op.Data.(bool)
	}
	return 1
}

func TestOriginalAVFieldSettingsRulesLayoutsAndOwners(t *testing.T) {
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
				for _, action := range fieldSettingActions {
					for _, scenario := range []string{"on", "off", "missing", "wrong-type"} {
						t.Run(string(layout)+"/"+action+"/"+scenario, func(t *testing.T) {
							h := fieldSettingHarness(t, l, root, page, "")
							before := cellDigest(t, root, page)
							a := fieldSettingPrepare(t, h, layout)
							op := fieldSettingOp(action, scenario != "off")
							if scenario == "missing" {
								op.ID = "missing"
							}
							if scenario == "wrong-type" {
								op.ID = "text"
							}
							want, err := jsonCloneAV(a)
							if err != nil {
								t.Fatal(err)
							}
							stages := expectedFieldSetting(want, op, string(layout))
							h.tx.DoOperations = []*Operation{op}
							if ret := performTx(h.tx); ret != nil || !h.committed {
								t.Fatal(ret, h.counts)
							}
							gotRaw, _ := json.Marshal(a)
							wantRaw, _ := json.Marshal(want)
							if string(gotRaw) != string(wantRaw) {
								t.Fatal("original setting/untouched fields differ", string(gotRaw), string(wantRaw))
							}
							if len(a.RenderedViewables) != 0 || h.counts["stage:"+cellAV] != stages {
								t.Fatal("memo/stage rule changed", h.counts)
							}
							if !strings.HasPrefix(a.Name, page) {
								t.Fatal("wrong owner")
							}
							for name := range h.counts {
								if strings.HasPrefix(name, "groups:") || strings.HasPrefix(name, "reload:") || name == "related" {
									t.Fatal("invented side effect", h.counts)
								}
							}
							if !attributeViewFieldSettingNeedsView(action) && h.counts["load"] != 0 {
								t.Fatal("metadata setting unnecessarily loaded source tree")
							}
							assertFieldSourceUnchanged(t, h, before)
						})
					}
				}
			}
		})
	}
}

func TestAVFieldSettingsViewSelectionAndMixedCandidateVisibility(t *testing.T) {
	m, store, root := fieldSource(t)
	for _, page := range []string{"1760000000000000002", "1760000000000000003"} {
		t.Run(page, func(t *testing.T) {
			t.Parallel()
			l, err := m.Acquire(context.Background(), store, page)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(l.Release)
			for _, action := range fieldSettingActions {
				if !attributeViewFieldSettingNeedsView(action) {
					continue
				}
				for _, scenario := range []string{"selected", "empty-block", "missing-block", "missing-node", "no-attribute", "stale-view", "stale-default", "first-fallback"} {
					t.Run(action+"/"+scenario, func(t *testing.T) {
						h := fieldSettingHarness(t, l, root, page, "")
						before := cellDigest(t, root, page)
						a := fieldSettingPrepare(t, h, av.LayoutTypeGallery)
						op := fieldSettingOp(action, true)
						selected := "gallery"
						var ops []*Operation
						switch scenario {
						case "selected":
							ops = append(ops, &Operation{Action: "setAttrs", ID: settingsBlock, Data: `{"custom-sy-av-view":"table"}`})
							selected = "table"
						case "empty-block":
							op.BlockID = ""
						case "missing-block":
							op.BlockID = "missing"
						case "missing-node":
							op.BlockID = "not-in-source"
						case "stale-view":
							ops = append(ops, &Operation{Action: "setAttrs", ID: settingsBlock, Data: `{"custom-sy-av-view":"stale"}`})
						case "stale-default":
							a.ViewID = "stale"
							selected = "table"
						case "first-fallback":
							a.ViewID = ""
							selected = "table"
						}
						want, _ := jsonCloneAV(a)
						expectedFieldSetting(want, op, selected)
						ops = append(ops, op)
						h.tx.DoOperations = ops
						if ret := performTx(h.tx); ret != nil || !h.committed {
							t.Fatal(ret, h.counts)
						}
						gotRaw, _ := json.Marshal(a)
						wantRaw, _ := json.Marshal(want)
						if string(gotRaw) != string(wantRaw) {
							t.Fatal("wrong view/fallback", selected, string(gotRaw), string(wantRaw))
						}
						if scenario == "selected" && (h.counts["load"] != 1 || h.counts["tree-stage"] != 1) {
							t.Fatal("did not share staged source tree", h.counts)
						}
						assertFieldSourceUnchanged(t, h, before)
					})
				}
			}
			for _, scenario := range []string{"success", "last-stage", "bad-second", "unaudited"} {
				t.Run("mixed/"+scenario, func(t *testing.T) {
					h := fieldSettingHarness(t, l, root, page, "")
					before := cellDigest(t, root, page)
					a := fieldSettingPrepare(t, h, av.LayoutTypeTable)
					h.env.avFields = &attributeViewFieldEnvironment{read: h.cell.read, stage: h.cell.stage}
					ops := []*Operation{fieldOp("addAttrViewCol", "added", "text"), {Action: "setAttrViewColDesc", AvID: cellAV, ID: "added", Data: "added desc"}, {Action: "setAttrViewColWrap", AvID: cellAV, ID: "added", Data: true}}
					for _, action := range fieldSettingActions {
						ops = append(ops, fieldSettingOp(action, true))
					}
					if scenario == "bad-second" {
						ops[1].Data = 42
					}
					if scenario == "unaudited" {
						ops = append(ops, &Operation{Action: "sortAttrViewCol", AvID: cellAV})
					}
					if scenario == "last-stage" {
						old := h.env.avConfig.stage
						h.env.avConfig.stage = func(a *av.AttributeView) error {
							key, _ := a.GetKey("updated")
							if key.Updated != nil {
								return context.Canceled
							}
							return old(a)
						}
					}
					h.tx.DoOperations = ops
					ret := performTx(h.tx)
					if scenario == "success" {
						if ret != nil || !h.committed {
							t.Fatal(ret)
						}
						key, _ := a.GetKey("added")
						if key.Desc != "added desc" || !settingField(a.Views[0], "added").Wrap {
							t.Fatal("consecutive candidate lost")
						}
					} else if ret == nil || h.committed {
						t.Fatal("bad mixed batch finished", ret)
					}
					if (scenario == "bad-second" || scenario == "unaudited") && !reflect.DeepEqual(h.counts, map[string]int{}) {
						t.Fatal("bad mixed batch partially admitted", h.counts)
					}
					assertFieldSourceUnchanged(t, h, before)
				})
			}
		})
	}
}

func TestAVFieldSettingsWrapAggregationAcrossFields(t *testing.T) {
	m, store, root := fieldSource(t)
	page := "1760000000000000002"
	l, err := m.Acquire(context.Background(), store, page)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Release()
	for _, layout := range []av.LayoutType{av.LayoutTypeTable, av.LayoutTypeGallery, av.LayoutTypeKanban} {
		h := fieldSettingHarness(t, l, root, page, "")
		before := cellDigest(t, root, page)
		a := fieldSettingPrepare(t, h, layout)
		v := a.GetView(string(layout))
		settingFields(v, func(f *av.BaseField) { f.Wrap = false })
		for _, id := range fieldIDs(v) {
			h.tx.DoOperations = append(h.tx.DoOperations, &Operation{Action: "setAttrViewColWrap", AvID: cellAV, ID: id, Data: true})
		}
		if ret := performTx(h.tx); ret != nil || !h.committed {
			t.Fatal(ret)
		}
		all := v.Table != nil && v.Table.WrapField || v.Gallery != nil && v.Gallery.WrapField || v.Kanban != nil && v.Kanban.WrapField
		if !all {
			t.Fatal("WrapField did not aggregate all columns")
		}
		if !slices.Contains(fieldIDs(v), "number") {
			t.Fatal("field set changed")
		}
		assertFieldSourceUnchanged(t, h, before)
	}
}
