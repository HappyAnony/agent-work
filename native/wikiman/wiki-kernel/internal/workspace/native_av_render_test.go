//go:build fts5

package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	av "wiki-kernel/internal/avmodel"
	"wiki-kernel/internal/avrender"
)

func TestNativeOriginalTableRenderFilterSortIsolationAndFailures(t *testing.T) {
	ctx := context.Background()
	root := fixture(t)
	const id = "20261022090000-av00001"
	for _, page := range []string{pageA, pageB} {
		a := &av.AttributeView{Spec: 4, ID: id, Name: page, ViewID: "view", Views: []*av.View{{ID: "view", Name: page, LayoutType: av.LayoutTypeTable, PageSize: 2, Table: &av.LayoutTable{BaseLayout: &av.BaseLayout{}, Columns: []*av.ViewTableColumn{{BaseField: &av.BaseField{ID: "primary"}}, {BaseField: &av.BaseField{ID: "number"}, Calc: &av.FieldCalc{Operator: av.CalcOperatorSum}}}}, Filters: []*av.ViewFilter{{Column: "removed", Operator: av.FilterOperatorIsEmpty}, {Column: "number", Operator: av.FilterOperatorIsGreater, Value: &av.Value{Type: av.KeyTypeNumber, Number: &av.ValueNumber{Content: 1, IsNotEmpty: true}}}}, Sorts: []*av.ViewSort{{Column: "number", Order: av.SortOrderDesc}}}}, KeyValues: []*av.KeyValues{{Key: &av.Key{ID: "primary", Type: av.KeyTypeBlock}}, {Key: &av.Key{ID: "number", Type: av.KeyTypeNumber}}}}
		for i, row := range []string{blockID, "20261022090100-row0001", "20261022090200-row0002"} {
			a.KeyValues[0].Values = append(a.KeyValues[0].Values, &av.Value{ID: row, KeyID: "primary", BlockID: row, Type: av.KeyTypeBlock, IsDetached: i > 0, Block: &av.ValueBlock{ID: row, Content: page, Updated: int64(i + 1)}})
			a.KeyValues[1].Values = append(a.KeyValues[1].Values, &av.Value{ID: row + "n", KeyID: "number", BlockID: row, Type: av.KeyTypeNumber, UpdatedAt: 1, Number: &av.ValueNumber{Content: float64(i + 1), IsNotEmpty: true}})
		}
		raw, e := json.Marshal(a)
		if e != nil {
			t.Fatal(e)
		}
		dir := filepath.Join(root, "pages", page, "data", "storage", "av")
		if e = os.MkdirAll(dir, 0700); e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(filepath.Join(dir, id+".json"), raw, 0600); e != nil {
			t.Fatal(e)
		}
	}
	before := digest(t, root)
	m := open(t, root, Options{Configuration: originalProjectionConfiguration})
	for _, page := range []string{pageA, pageB} {
		l, e := m.Acquire(ctx, storeID, page)
		if e != nil {
			t.Fatal(e)
		}
		raw, e := l.NativeAttributeViewTable(ctx, id, "", page, 1, 1)
		if e != nil {
			t.Fatal(e)
		}
		var table av.Table
		if e = json.Unmarshal(raw, &table); e != nil {
			t.Fatal(e)
		}
		if table.Name != page || table.RowCount != 2 || len(table.Rows) != 1 || table.Rows[0].Cells[1].Value.Number.Content != 3 || table.Columns[1].Calc.Result.Number.Content != 5 {
			t.Fatalf("lost original filter/sort/calc: %s", raw)
		}
		raw[0] = 'x'
		raw, e = l.NativeAttributeViewTable(ctx, id, "view", page, 2, 1)
		if e != nil || !strings.Contains(string(raw), page) {
			t.Fatal(string(raw), e)
		}
		if e = json.Unmarshal(raw, &table); e != nil || table.Rows[0].Cells[1].Value.Number.Content != 2 {
			t.Fatal(string(raw), e)
		}
		raw, e = l.NativeAttributeViewTable(ctx, id, "", "", 999, 1)
		if e != nil || !strings.Contains(string(raw), `"rows":[]`) {
			t.Fatal(string(raw), e)
		}
		canceled, cancel := context.WithCancel(ctx)
		cancel()
		if raw, e = l.NativeAttributeViewTable(canceled, id, "", "", 1, 1); !errors.Is(e, context.Canceled) || raw != nil {
			t.Fatal(string(raw), e)
		}
		for _, bad := range []struct {
			id, view string
			p, size  int
		}{{"../escape", "", 1, 1}, {id, "", 0, 1}, {id, "", 1, 1025}} {
			if raw, e = l.NativeAttributeViewTable(ctx, bad.id, bad.view, "", bad.p, bad.size); e == nil || raw != nil {
				t.Fatal("unexpected partial result", string(raw), e)
			}
		}
		l.Release()
	}
	if after := digest(t, root); after != before {
		t.Fatal("render changed authoritative source")
	}
}

func TestNativeOriginalTableRejectsUnboundAndMalformed(t *testing.T) {
	for _, typ := range []av.KeyType{av.KeyTypeTemplate, av.KeyTypeRelation, av.KeyTypeRollup} {
		a := &av.AttributeView{Spec: 4, KeyValues: []*av.KeyValues{{Key: &av.Key{ID: "unsupported", Type: typ}}}}
		if !errors.Is(validateNativeAVRender(a), avrender.ErrShape) {
			t.Fatal(typ)
		}
	}
	if !errors.Is(validateNativeAVRender(&av.AttributeView{Spec: 4, KeyValues: []*av.KeyValues{nil}}), avrender.ErrShape) {
		t.Fatal("nil key accepted")
	}
}
