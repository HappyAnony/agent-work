package model

import (
	"context"
	"slices"
	"testing"
	"wiki-kernel/av"
	"wiki-kernel/internal/avrender"
)

func TestAVOrderDefaultCoreFiltersTemplatesDatesAndBoundPrimary(t *testing.T) {
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
				for _, scenario := range []string{"text", "number", "check", "url", "detached-primary", "bound-primary", "date-filter", "template-filter", "asset-early-return"} {
					t.Run(string(layout)+"/"+scenario, func(t *testing.T) {
						h := orderHarness(t, l, root, page, "")
						before := cellDigest(t, root, page)
						a := orderPrepare(t, h, layout)
						v := a.GetView(string(layout))
						g, _ := avrender.NewGroupView(layout)
						g.GroupItemIDs = []string{}
						g.GroupVal = &av.Value{Type: av.KeyTypeText, Text: &av.ValueText{Content: "Moved"}}
						key := scenario
						switch scenario {
						case "detached-primary", "bound-primary":
							key = "primary"
						case "date-filter", "template-filter", "asset-early-return":
							key = "status"
						}
						v.Group = &av.ViewGroup{Field: key}
						d := h.env.avOrder.defaults
						switch scenario {
						case "number":
							g.GroupVal.Text.Content = "10 - 20"
						case "check":
							g.GroupVal = &av.Value{Type: av.KeyTypeCheckbox, Checkbox: &av.ValueCheckbox{Checked: true}}
						case "bound-primary":
							a.GetValue("primary", settingsBlock).IsDetached = false
						case "date-filter", "template-filter", "asset-early-return":
							var old, target string
							a, v, old, target = orderGroup(t, h, layout, "status")
							_ = old
							g = v.GetGroupByID(target)
							if scenario == "date-filter" {
								kv, _ := a.GetKeyValues("date")
								kv.Key.Date = &av.Date{AutoFillNow: true}
								kv.Values[1].Date = &av.ValueDate{Content: 123, IsNotEmpty: true, IsNotTime: true}
								v.Filters = []*av.ViewFilter{{Column: "date", Operator: av.FilterOperatorIsNotEmpty, Value: &av.Value{Type: av.KeyTypeDate, Date: &av.ValueDate{IsNotEmpty: true}}}}
							}
							if scenario == "template-filter" {
								a.GetValue("text", settingsBlock).Text.Content = "source"
								a.GetValue("text", cellSecond).Text.Content = "target"
								a.KeyValues = append(a.KeyValues, &av.KeyValues{Key: &av.Key{ID: "tpl", Name: "Label", Type: av.KeyTypeTemplate, Template: ".action{.Text}"}})
								addFieldFixture(v, "tpl")
								v.Filters = []*av.ViewFilter{{Column: "tpl", Operator: av.FilterOperatorContains, Value: &av.Value{Type: av.KeyTypeText, Text: &av.ValueText{Content: "target"}}}}
							}
							if scenario == "asset-early-return" {
								a.GetValue("asset", cellSecond).MAsset = []*av.ValueAsset{{Name: "file", Content: "assets/a.png", Type: av.AssetTypeImage}}
								v.Filters = []*av.ViewFilter{{Column: "asset", Operator: av.FilterOperatorIsNotEmpty, Value: &av.Value{Type: av.KeyTypeMAsset, MAsset: []*av.ValueAsset{}}}}
							}
						}
						err = fillAttributeViewDefaultValues(a, v, g, "", settingsBlock, false, d)
						if err != nil {
							t.Fatal(err)
						}
						switch scenario {
						case "text":
							if a.GetValue("text", settingsBlock).Text.Content != "Moved" {
								t.Fatal("text group default")
							}
						case "number":
							n := a.GetValue("number", settingsBlock).Number
							if n.Content != 10 || !n.IsNotEmpty {
								t.Fatal("range default")
							}
						case "check":
							if !a.GetValue("check", settingsBlock).Checkbox.Checked {
								t.Fatal("checkbox default")
							}
						case "url":
							if a.GetValue("url", settingsBlock).URL.Content != "Moved" {
								t.Fatal("URL default")
							}
						case "detached-primary":
							if a.GetValue("primary", settingsBlock).Block.Content != "Moved" {
								t.Fatal("detached primary default")
							}
						case "bound-primary":
							if a.GetValue("primary", settingsBlock).Block.Content == "Moved" {
								t.Fatal("bound primary overwritten")
							}
						case "date-filter":
							value := a.GetValue("date", settingsBlock)
							if value.Date.Content != h.cell.now().UnixMilli() || !value.Date.IsNotTime || !value.Date.IsNotEmpty {
								t.Fatal("owner clock/precision lost", value)
							}
						case "template-filter":
							value := a.GetValue("text", settingsBlock)
							if value.Text.Content != "target" || value.CreatedAt != 1 || value.UpdatedAt != 2 || h.counts["template"] != 1 {
								t.Fatal("template dependency/near row lost", value, h.counts)
							}
						case "asset-early-return":
							if len(a.GetValue("asset", settingsBlock).MAsset) != 1 || !av.MSelectExistOption(a.GetValue("status", settingsBlock).MSelect, "Old") {
								t.Fatal("original attachment early return changed")
							}
						}
						assertFieldSourceUnchanged(t, h, before)
					})
				}
			}
		})
	}
}

func TestAVOrderKeyRepairSelectionAndInvalidInput(t *testing.T) {
	m, store, root := fieldSource(t)
	page := "1760000000000000002"
	l, err := m.Acquire(context.Background(), store, page)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Release()
	for _, layout := range []av.LayoutType{av.LayoutTypeTable, av.LayoutTypeGallery, av.LayoutTypeKanban} {
		h := orderHarness(t, l, root, page, "")
		before := cellDigest(t, root, page)
		a := orderPrepare(t, h, layout)
		a.KeyIDs = []string{"number", "gone", "number"}
		h.tx.DoOperations = []*Operation{{Action: "sortAttrViewKey", AvID: cellAV, ID: "text", PreviousID: "primary"}}
		if ret := performTx(h.tx); ret != nil || !h.committed {
			t.Fatal(ret)
		}
		if slices.Contains(a.KeyIDs, "gone") || slices.Index(a.KeyIDs, "text") != slices.Index(a.KeyIDs, "primary")+1 {
			t.Fatal("key repair/order changed", a.KeyIDs)
		}
		for _, kv := range a.KeyValues {
			if !slices.Contains(a.KeyIDs, kv.Key.ID) {
				t.Fatal("missing key not repaired")
			}
		}
		duplicates := 0
		for _, id := range a.KeyIDs {
			if id == "number" {
				duplicates++
			}
		}
		if duplicates != 2 {
			t.Fatal("original duplicate-ID repair policy changed")
		}
		assertFieldSourceUnchanged(t, h, before)
	}
	for _, action := range []string{"sortAttrViewRow", "sortAttrViewCol"} {
		for _, scenario := range []string{"selected", "missing-block", "stale-view", "empty-block", "missing-default"} {
			h := orderHarness(t, l, root, page, "")
			before := cellDigest(t, root, page)
			a := orderPrepare(t, h, av.LayoutTypeGallery)
			op := orderOp(action)
			if action == "sortAttrViewRow" {
				op.ID = cellSecond
				op.PreviousID = ""
			}
			selected := "gallery"
			var ops []*Operation
			switch scenario {
			case "selected":
				ops = append(ops, &Operation{Action: "setAttrs", ID: settingsBlock, Data: `{"custom-sy-av-view":"table"}`})
				selected = "table"
			case "stale-view":
				ops = append(ops, &Operation{Action: "setAttrs", ID: settingsBlock, Data: `{"custom-sy-av-view":"stale"}`})
			case "empty-block":
				op.BlockID = ""
			case "missing-block":
				op.BlockID = "missing"
			case "missing-default":
				a.ViewID = "stale"
				selected = "table"
			}
			if action == "sortAttrViewCol" {
				op.PreviousID = ""
			}
			ops = append(ops, op)
			h.tx.DoOperations = ops
			if ret := performTx(h.tx); ret != nil || !h.committed {
				t.Fatal(ret)
			}
			v := a.GetView(selected)
			if action == "sortAttrViewRow" && v.ItemIDs[0] != cellSecond || action == "sortAttrViewCol" && fieldIDs(v)[0] != "number" {
				t.Fatal("wrong view selected")
			}
			assertFieldSourceUnchanged(t, h, before)
		}
	}
	for _, op := range []*Operation{nil, {Action: "unknown"}, {Action: "duplicateAttrViewKey", KeyID: "x", NextID: "x"}, {Action: "duplicateAttrViewKey", KeyID: "x"}} {
		if err := validateAttributeViewOrderInput(op); err == nil {
			t.Fatal("bad operation admitted")
		}
	}
	if err := runLegacyAttributeViewOrder("sortAttrViewCol", nil); err == nil {
		t.Fatal("nil wrapper operation admitted")
	}
}
