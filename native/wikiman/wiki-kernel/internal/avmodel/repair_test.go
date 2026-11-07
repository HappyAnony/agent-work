// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package avmodel

import (
	"strings"
	"testing"
	"time"
)

func TestOriginalAVRepairRulesAndMarkers(t *testing.T) {
	now := time.Date(2026, 10, 29, 1, 2, 3, 0, time.UTC)
	v := &View{ID: "view", LayoutType: LayoutTypeTable, Table: NewLayoutTable(), Filters: []*ViewFilter{{Column: "missing"}, {Column: "primary"}}, Sorts: []*ViewSort{{Column: "missing"}, {Column: "primary"}}, Group: &ViewGroup{Field: "missing"}}
	value := &Value{Type: KeyTypeText, BlockID: "20261029000000-row0001", Text: &ValueText{Content: "legacy block"}}
	fallback := &View{LayoutType: LayoutTypeGallery, Table: NewLayoutTable()}
	a := &AttributeView{Name: strings.Repeat("中", 513) + "\n", Views: []*View{v, fallback, {LayoutType: LayoutTypeGallery}, {LayoutType: LayoutTypeGallery}}, KeyValues: []*KeyValues{{Key: &Key{ID: "primary", Type: KeyTypeBlock}, Values: []*Value{value}}}}
	changed, e := CheckView(a, v, RepairEnvironment{Now: func() time.Time { return now }, NewID: func() string { return "20261029000001-new0001" }, Location: time.UTC})
	if e != nil || !changed {
		t.Fatal(changed, e)
	}
	if len(v.Filters) != 1 || len(v.Sorts) != 1 || v.Group != nil || len(a.Views) != 2 || fallback.LayoutType != LayoutTypeTable {
		t.Fatal("repair rule changed")
	}
	if value.Type != KeyTypeBlock || value.Block.Content != "legacy block" || value.Block.Created != time.Date(2026, 10, 29, 0, 0, 0, 0, time.UTC).UnixMilli() || value.Block.Updated != value.Block.Created {
		t.Fatal("block conversion changed")
	}
	if len([]rune(a.Name)) != 512 {
		t.Fatal("title cap changed")
	}
	a.Name = "line\nline"
	v.Group = &ViewGroup{Field: "missing"}
	changed, e = CheckView(a, v, RepairEnvironment{Now: time.Now, NewID: func() string { return "id" }, Location: time.UTC})
	if e != nil || changed || a.Name != "line line" || v.Group != nil {
		t.Fatal("original no-save marker semantics changed", changed, e)
	}
}
func TestAVFactoryUsesExplicitLabelsAndHasNoRuntime(t *testing.T) {
	labels := map[string]string{"table": "Captured table", "key": "Captured key", "select": "Captured select"}
	next := 0
	a, e := DefaultAttributeView("source", labels, func() string { next++; return "id" + strings.Repeat("x", next) })
	if e != nil || a.Spec != CurrentSpec || len(a.KeyValues) != 2 || a.ViewID != a.Views[0].ID || a.Views[0].PageSize != ViewDefaultPageSize {
		t.Fatal(a, e)
	}
	if a.Views[0].Name != labels["table"] || a.KeyValues[0].Key.Name != labels["key"] || a.KeyValues[1].Key.Type != KeyTypeSelect || a.ReadAttributeView != nil || a.DurationFormatter != nil {
		t.Fatal("defaults/runtime changed")
	}
	if _, e = DefaultAttributeView("source", nil, func() string { return "id" }); e == nil {
		t.Fatal("unbound labels accepted")
	}
	if _, _, _, e = DefaultTable("block", labels, nil); e == nil {
		t.Fatal("unbound IDs accepted")
	}
}
func TestAVRepairRejectsShapeBeforeMutation(t *testing.T) {
	a := &AttributeView{Name: "before\nname", Views: []*View{{}}, KeyValues: []*KeyValues{nil}}
	if _, e := CheckView(a, a.Views[0], RepairEnvironment{Now: time.Now, NewID: func() string { return "id" }, Location: time.UTC}); e == nil || a.Name != "before\nname" {
		t.Fatal("malformed source mutated")
	}
}
