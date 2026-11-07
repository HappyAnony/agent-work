package avmodel

import (
	"testing"
	"time"
)

func TestOriginalSpecProjectionRulesAndExplicitClock(t *testing.T) {
	const row = "20261019090002-par0001"
	const valueID = "20261019090003-num0001"
	a := &AttributeView{Spec: 0, KeyValues: []*KeyValues{
		{Key: &Key{ID: "primary", Type: KeyTypeBlock}, Values: []*Value{{ID: row, KeyID: "primary", BlockID: row, Type: KeyTypeBlock, Block: &ValueBlock{ID: row}}}},
		{Key: &Key{ID: "number", Type: KeyTypeNumber}, Values: []*Value{{ID: valueID, KeyID: "number", BlockID: row, Type: KeyTypeNumber, Number: &ValueNumber{Content: 3}}}},
		{Key: &Key{ID: "created", Type: KeyTypeCreated}}, {Key: &Key{ID: "updated", Type: KeyTypeUpdated}},
	}, Views: []*View{{ID: "table", LayoutType: LayoutTypeTable, Table: NewLayoutTable()}, {ID: "gallery", LayoutType: LayoutTypeGallery, Gallery: NewLayoutGallery()}}}
	a.Views[0].Table.RowIDs = []string{row}
	a.Views[1].Gallery.CardIDs = []string{row}
	a.Views[0].Table.Filters = []*ViewFilter{{Column: "number", Operator: FilterOperatorIsNotEmpty}, {Column: "missing"}}
	a.Views[0].Table.Sorts = []*ViewSort{{Column: "number", Order: SortOrderDesc}, {Column: "missing"}}
	a.Views[0].Table.PageSize = 7
	generated := false
	if e := UpgradeInMemory(a, UpgradeEnvironment{Now: func() time.Time { return time.UnixMilli(12345) }, NewID: func() string { generated = true; return "new" }}); e != nil {
		t.Fatal(e)
	}
	created, _ := time.ParseInLocation("20060102150405", row[:14], time.Local)
	if a.Spec != 4 || generated || a.KeyValues[0].Values[0].Block.Created != created.UnixMilli() || a.KeyValues[0].Values[0].UpdatedAt != created.UnixMilli() {
		t.Fatal(a, generated)
	}
	if !a.KeyValues[1].Values[0].Number.IsNotEmpty || !a.KeyValues[2].Key.Created.IncludeTime || !a.KeyValues[3].Key.Updated.IncludeTime {
		t.Fatal("value/include-time rules")
	}
	v := a.Views[0]
	if len(v.Filters) != 1 || v.Filters[0].Value == nil || v.Filters[0].Value.Type != KeyTypeNumber || len(v.Sorts) != 1 || v.PageSize != 7 || !v.Table.ShowIcon || len(v.ItemIDs) != 1 || v.ItemIDs[0] != row || len(a.Views[1].ItemIDs) != 1 {
		t.Fatal("legacy rule/item migration", v)
	}
	if e := UpgradeInMemory(a, UpgradeEnvironment{}); e != nil {
		t.Fatal("modern projection should be idempotent", e)
	}
}
