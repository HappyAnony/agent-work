//go:build fts5

package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"wiki-kernel/conf"
	av "wiki-kernel/internal/avmodel"
	"wiki-kernel/internal/avrender"
)

func layoutFixture(id, page string, cover av.CoverFrom) *av.AttributeView {
	a := &av.AttributeView{Spec: 4, ID: id, Name: page, ViewID: "gallery", KeyValues: []*av.KeyValues{{Key: &av.Key{ID: "primary", Name: "Name", Type: av.KeyTypeBlock}}, {Key: &av.Key{ID: "number", Type: av.KeyTypeNumber}}, {Key: &av.Key{ID: "select", Name: "Status", Type: av.KeyTypeSelect, Options: []*av.SelectOption{{Name: "X"}, {Name: "Y"}, {Name: "Empty"}}}}, {Key: &av.Key{ID: "asset", Type: av.KeyTypeMAsset}}}}
	rows := []string{blockID, "20261023090001-row0001", "20261023090002-row0002"}
	for i, id := range rows {
		a.KeyValues[0].Values = append(a.KeyValues[0].Values, &av.Value{ID: id, KeyID: "primary", BlockID: id, Type: av.KeyTypeBlock, IsDetached: i > 0, Block: &av.ValueBlock{ID: id, Content: page + fmt.Sprint(i), Updated: int64(i + 1)}})
		a.KeyValues[1].Values = append(a.KeyValues[1].Values, &av.Value{ID: id + "n", KeyID: "number", BlockID: id, Type: av.KeyTypeNumber, Number: &av.ValueNumber{Content: float64(i + 1), IsNotEmpty: true}})
		opt := "X"
		if i == 1 {
			opt = "Y"
		}
		a.KeyValues[2].Values = append(a.KeyValues[2].Values, &av.Value{ID: id + "s", KeyID: "select", BlockID: id, Type: av.KeyTypeSelect, MSelect: []*av.ValueSelect{{Content: opt}}})
		a.KeyValues[3].Values = append(a.KeyValues[3].Values, &av.Value{ID: id + "a", KeyID: "asset", BlockID: id, Type: av.KeyTypeMAsset, MAsset: []*av.ValueAsset{{Content: "assets/" + page + ".png"}}})
	}
	for _, typ := range []av.LayoutType{av.LayoutTypeTable, av.LayoutTypeGallery, av.LayoutTypeKanban} {
		v := &av.View{ID: string(typ), Name: page, LayoutType: typ, PageSize: 2, Sorts: []*av.ViewSort{{Column: "number", Order: av.SortOrderDesc}}}
		switch typ {
		case av.LayoutTypeTable:
			v.Table = av.NewLayoutTable()
			for _, k := range a.KeyValues {
				v.Table.Columns = append(v.Table.Columns, &av.ViewTableColumn{BaseField: &av.BaseField{ID: k.Key.ID}, Calc: &av.FieldCalc{Operator: av.CalcOperatorSum}})
			}
		case av.LayoutTypeGallery:
			v.Gallery = av.NewLayoutGallery()
			v.Gallery.CoverFrom = cover
			v.Gallery.CoverFromAssetKeyID = "asset"
			for _, k := range a.KeyValues {
				v.Gallery.CardFields = append(v.Gallery.CardFields, &av.ViewGalleryCardField{BaseField: &av.BaseField{ID: k.Key.ID}})
			}
		case av.LayoutTypeKanban:
			v.Kanban = av.NewLayoutKanban()
			v.Kanban.CoverFrom = cover
			v.Kanban.CoverFromAssetKeyID = "asset"
			for _, k := range a.KeyValues {
				v.Kanban.Fields = append(v.Kanban.Fields, &av.ViewKanbanField{BaseField: &av.BaseField{ID: k.Key.ID}})
			}
		}
		a.Views = append(a.Views, v)
	}
	return a
}

func writeLayoutFixture(t *testing.T, root, page string, a *av.AttributeView) {
	t.Helper()
	raw, e := json.Marshal(a)
	if e != nil {
		t.Fatal(e)
	}
	dir := filepath.Join(root, "pages", page, "data", "storage", "av")
	if e = os.MkdirAll(dir, 0700); e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(dir, a.ID+".json"), raw, 0600); e != nil {
		t.Fatal(e)
	}
}

func layoutLanguages(_, page string) (*conf.Business, error) {
	return conf.NewBusiness(nil, map[string]map[int]string{"en_US": {264: page + " %s Blank", 265: "Outside", 259: "Last %d days", 260: "Yesterday", 261: "Today", 262: "Tomorrow", 263: "Next %d days"}}, nil), nil
}

func TestNativeOriginalLayoutsCoversGroupsAndIsolation(t *testing.T) {
	ctx := context.Background()
	root := fixture(t)
	for _, page := range []string{pageA, pageB} {
		for cover := av.CoverFromNone; cover <= av.CoverFromContentBlock; cover++ {
			id := fmt.Sprintf("202610230900%02d-av00001", cover)
			a := layoutFixture(id, page, cover)
			writeLayoutFixture(t, root, page, a)
			grouped := layoutFixture(fmt.Sprintf("202610230901%02d-av00001", cover), page, cover)
			for _, v := range grouped.Views {
				v.Group = &av.ViewGroup{Field: "select", Method: av.GroupMethodValue, Order: av.GroupOrderSelectOption, HideEmpty: true}
			}
			writeLayoutFixture(t, root, page, grouped)
		}
	}
	before := digest(t, root)
	m := open(t, root, Options{Configuration: originalProjectionConfiguration, ConfigurationBusiness: layoutLanguages})
	for _, page := range []string{pageA, pageB} {
		l, e := m.Acquire(ctx, storeID, page)
		if e != nil {
			t.Fatal(e)
		}
		defer l.Release()
		for cover := av.CoverFromNone; cover <= av.CoverFromContentBlock; cover++ {
			for _, typ := range []string{"table", "gallery", "kanban"} {
				for _, grouped := range []bool{false, true} {
					id := fmt.Sprintf("202610230900%02d-av00001", cover)
					if grouped {
						id = fmt.Sprintf("202610230901%02d-av00001", cover)
					}
					raw, e := l.NativeAttributeViewRender(ctx, id, typ, "", 1, 10, nil)
					if e != nil {
						t.Fatalf("%s/%s/%d/%v: %v", page, typ, cover, grouped, e)
					}
					var data struct {
						Name      string
						Groups    []json.RawMessage
						Cards     []struct{ CoverURL, CoverContent string }
						Rows      []json.RawMessage
						CardCount int
					}
					if e = json.Unmarshal(raw, &data); e != nil || data.Name != page {
						t.Fatal(string(raw), e)
					}
					if grouped || typ == "kanban" {
						if len(data.Groups) != 4 || len(data.Rows) != 0 || len(data.Cards) != 0 {
							t.Fatalf("missing original groups: %s", raw)
						}
					} else if typ == "gallery" {
						if len(data.Cards) != 3 || data.CardCount != 3 {
							t.Fatal(string(raw))
						}
					}
					if (typ == "gallery" || typ == "kanban") && cover == av.CoverFromAssetField && !strings.Contains(string(raw), "assets/"+page+".png") {
						t.Fatal("asset cover lost", string(raw))
					}
					if (typ == "gallery" || typ == "kanban") && (cover == av.CoverFromContentImage || cover == av.CoverFromContentBlock) {
						marker := "Synthetic knowledge"
						foreign := "Workspace B knowledge"
						if page == pageB {
							marker, foreign = foreign, marker
						}
						if !strings.Contains(string(raw), marker) || strings.Contains(string(raw), foreign) {
							t.Fatal("source cover cross owner", string(raw))
						}
					}
					raw[0] = 'x'
					again, e := l.NativeAttributeViewRender(ctx, id, typ, "", 999, 1, nil)
					if e != nil || len(again) == 0 || again[0] != '{' {
						t.Fatal("copy/paging failure", e)
					}
				}
			}
		}
		canceled, cancel := context.WithCancel(ctx)
		cancel()
		if out, e := l.NativeAttributeViewRender(canceled, "20261023090000-av00001", "gallery", "", 1, 1, nil); out != nil || !errors.Is(e, context.Canceled) {
			t.Fatal(string(out), e)
		}
	}
	if after := digest(t, root); before != after {
		t.Fatal("layout/group/cover changed source")
	}
}

func TestNativeOriginalLayoutMissingDependencyAndMalformed(t *testing.T) {
	ctx := context.Background()
	root := fixture(t)
	const id = "20261023090200-av00001"
	a := layoutFixture(id, pageA, av.CoverFromNone)
	writeLayoutFixture(t, root, pageA, a)
	m := open(t, root, Options{Configuration: originalProjectionConfiguration})
	l, e := m.Acquire(ctx, storeID, pageA)
	if e != nil {
		t.Fatal(e)
	}
	defer l.Release()
	if out, e := l.NativeAttributeViewRender(ctx, id, "kanban", "", 1, 2, nil); out != nil || !errors.Is(e, conf.ErrBusinessUnbound) {
		t.Fatal("missing language accepted", string(out), e)
	}
	if out, e := l.NativeAttributeViewRender(ctx, id, "gallery", "", 1, 2, map[string]NativeAVPaging{"unknown": {Page: 0, PageSize: 1}}); out != nil || !errors.Is(e, ErrBudget) {
		t.Fatal(string(out), e)
	}
	for _, mutate := range []func(*av.AttributeView){func(a *av.AttributeView) { a.Views[1].Gallery.CardFields = []*av.ViewGalleryCardField{nil} }, func(a *av.AttributeView) { a.Views[1].Gallery.BaseLayout = nil }, func(a *av.AttributeView) { a.Views[1].Groups = []*av.View{nil} }} {
		bad := layoutFixture(id, pageA, av.CoverFromNone)
		mutate(bad)
		writeLayoutFixture(t, root, pageA, bad)
		if out, e := l.NativeAttributeViewRender(ctx, id, "gallery", "", 1, 2, nil); e == nil || out != nil {
			t.Fatal("invalid layout accepted", string(out), e)
		}
	}
	if e := validateNativeAVGroup(&av.View{Group: &av.ViewGroup{Method: av.GroupMethodRangeNum, Range: &av.GroupRange{NumStart: 0, NumEnd: 10, NumStep: 0}}}, &av.Key{Type: av.KeyTypeNumber}); !errors.Is(e, avrender.ErrShape) {
		t.Fatal(e)
	}
}
