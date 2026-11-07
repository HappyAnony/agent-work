package avmodel

import (
	"errors"
	"testing"
	"time"
)

func TestOriginalFilterMissingColumnAndNilValue(t *testing.T) {
	table := &Table{BaseInstance: &BaseInstance{Filters: []*ViewFilter{{Column: "gone", Operator: FilterOperatorIsNotEmpty}, {Column: "text", Operator: FilterOperatorContains, Value: &Value{Type: KeyTypeText, Text: &ValueText{Content: "keep"}}}}}, Columns: []*TableColumn{{BaseInstanceField: &BaseInstanceField{ID: "text"}}}, Rows: []*TableRow{{ID: "a", Cells: []*TableCell{{BaseValue: &BaseValue{Value: &Value{Type: KeyTypeText, Text: &ValueText{Content: "keep"}}}}}}, {ID: "b", Cells: []*TableCell{{BaseValue: &BaseValue{Value: &Value{Type: KeyTypeText, Text: &ValueText{Content: "drop"}}}}}}, {ID: "c", Cells: []*TableCell{{BaseValue: &BaseValue{}}}}}}
	if e := FilterChecked(table, &AttributeView{}, nil, nil); e != nil || len(table.Rows) != 1 || table.Rows[0].ID != "a" {
		t.Fatal(table.Rows, e)
	}
}

func TestRelatedReaderAndDurationAreExplicit(t *testing.T) {
	a := &AttributeView{}
	a.readRelated("same-id")
	if !errors.Is(a.readError, ErrRelatedReadUnbound) {
		t.Fatal(a.readError)
	}
	want := errors.New("synthetic read failure")
	a.ReadAttributeView = func(string) (*AttributeView, error) { return nil, want }
	a.readRelated("same-id")
	if !errors.Is(a.readError, want) {
		t.Fatal(a.readError)
	}
	called := false
	got := NewFormattedValueDate(1, 1001, DateFormatDuration, false, true, func(a, b time.Time) string { called = true; return "owned duration" })
	if !called || got.FormattedContent != "owned duration" {
		t.Fatal(got)
	}
}

func TestRollupFilterPropagatesOwnedReaderFailure(t *testing.T) {
	a := &AttributeView{KeyValues: []*KeyValues{
		{Key: &Key{ID: "rollup", Type: KeyTypeRollup, Rollup: &Rollup{RelationKeyID: "relation", KeyID: "target"}}},
		{Key: &Key{ID: "relation", Type: KeyTypeRelation, Relation: &Relation{AvID: "same-id"}}, Values: []*Value{{KeyID: "relation", BlockID: "row", Type: KeyTypeRelation, Relation: &ValueRelation{BlockIDs: []string{"target-row"}}}}},
	}}
	table := &Table{BaseInstance: &BaseInstance{Filters: []*ViewFilter{{Column: "rollup", Operator: FilterOperatorIsEqual, Value: &Value{Type: KeyTypeRollup, Rollup: &ValueRollup{}}}}}, Columns: []*TableColumn{{BaseInstanceField: &BaseInstanceField{ID: "rollup"}}}, Rows: []*TableRow{{ID: "row", Cells: []*TableCell{{BaseValue: &BaseValue{Value: &Value{KeyID: "rollup", Type: KeyTypeRollup, Rollup: &ValueRollup{}}}}}}}}
	if e := FilterChecked(table, a, nil, map[string]*AttributeView{}); !errors.Is(e, ErrRelatedReadUnbound) {
		t.Fatal(e)
	}
	want := errors.New("owned source error")
	a.ReadAttributeView = func(string) (*AttributeView, error) { return nil, want }
	table.Rows = []*TableRow{{ID: "row", Cells: []*TableCell{{BaseValue: &BaseValue{Value: &Value{KeyID: "rollup", Type: KeyTypeRollup, Rollup: &ValueRollup{}}}}}}}
	if e := FilterChecked(table, a, nil, map[string]*AttributeView{}); !errors.Is(e, want) {
		t.Fatal(e)
	}
}
