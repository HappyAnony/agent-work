//go:build fts5

package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"
	"wiki-kernel/conf"
	av "wiki-kernel/internal/avmodel"
	"wiki-kernel/internal/avrender"
)

func durationLanguages(_ string, page string) (*conf.Business, error) {
	all := map[string]map[string]string{}
	for _, lang := range []string{"en_US", "zh_CN"} {
		raw, e := os.ReadFile("../../../wiki-app/appearance/langs/" + lang + ".json")
		if e != nil {
			return nil, e
		}
		var doc struct {
			Time map[string]string `json:"_time"`
		}
		if e = json.Unmarshal(raw, &doc); e != nil {
			return nil, e
		}
		all[lang] = doc.Time
	}
	return conf.NewBusiness(nil, map[string]map[int]string{"en_US": {264: "Blank %s", 265: "Outside"}, "zh_CN": {264: "空 %s", 265: "之外"}}, nil, all), nil
}

func TestNativeOldSpecsReadOnlyAndDurationLanguages(t *testing.T) {
	ctx := context.Background()
	root := fixture(t)
	for spec := 0; spec <= 4; spec++ {
		for _, page := range []string{pageA, pageB} {
			a := layoutFixture(fmt.Sprintf("2026102309100%d-av00001", spec), page, av.CoverFromNone)
			a.Spec = spec
			for _, kv := range a.KeyValues {
				for _, v := range kv.Values {
					v.CreatedAt = 0
					v.UpdatedAt = 0
				}
			}
			for _, v := range a.KeyValues[1].Values {
				v.Type = av.KeyTypeDate
				v.Number = nil
				v.Date = &av.ValueDate{Content: 100000 + int64(len(v.BlockID))*1000, IsNotEmpty: true}
			}
			a.KeyValues[1].Key.Type = av.KeyTypeDate
			a.KeyValues[1].Values[1].Date.Content = 160000
			for _, view := range a.Views {
				view.Sorts = nil
				view.PageSize = 2
				if view.Table != nil {
					view.Table.Columns[1].Calc = &av.FieldCalc{Operator: av.CalcOperatorRange}
					if spec < 2 {
						view.Table.Filters = []*av.ViewFilter{{Column: "primary", Operator: av.FilterOperatorContains, Value: &av.Value{Type: av.KeyTypeText, Text: &av.ValueText{Content: page}}}}
						view.Table.PageSize = 2
					}
					if spec < 3 {
						view.Table.RowIDs = []string{a.KeyValues[0].Values[2].BlockID, a.KeyValues[0].Values[1].BlockID, a.KeyValues[0].Values[0].BlockID}
					}
				}
				if view.Gallery != nil {
					view.Gallery.CardFields[1].Calc = &av.FieldCalc{Operator: av.CalcOperatorRange}
					if spec < 3 {
						view.Gallery.CardIDs = []string{a.KeyValues[0].Values[2].BlockID, a.KeyValues[0].Values[1].BlockID, a.KeyValues[0].Values[0].BlockID}
					}
				}
				if view.Kanban != nil {
					view.Kanban.Fields[1].Calc = &av.FieldCalc{Operator: av.CalcOperatorRange}
				}
			}
			// A one-minute exact range, independent of today's clock or machine locale.
			a.KeyValues[1].Values[0].Date.Content = 100000
			a.KeyValues[1].Values[2].Date.Content = 100000
			writeLayoutFixture(t, root, page, a)
		}
	}
	before := digest(t, root)
	m := open(t, root, Options{Configuration: func(s, p string) (conf.AppData, error) {
		c, e := originalProjectionConfiguration(s, p)
		c.Lang = "en_US"
		if p == pageB {
			c.Lang = "zh_CN"
		}
		return c, e
	}, ConfigurationBusiness: durationLanguages})
	for spec := 0; spec <= 4; spec++ {
		for _, page := range []string{pageA, pageB} {
			l, e := m.Acquire(ctx, storeID, page)
			if e != nil {
				t.Fatal(e)
			}
			defer l.Release()
			id := fmt.Sprintf("2026102309100%d-av00001", spec)
			original, e := l.ReadNativeAttributeView(ctx, id)
			if e != nil {
				t.Fatal(e)
			}
			var stored av.AttributeView
			json.Unmarshal(original, &stored)
			if stored.Spec != spec {
				t.Fatal("raw source upgraded")
			}
			for _, typ := range []string{"table", "gallery"} {
				raw, e := l.NativeAttributeViewRender(ctx, id, typ, "", 1, 10, nil)
				if e != nil {
					t.Fatal(spec, page, typ, e)
				}
				var out map[string]any
				json.Unmarshal(raw, &out)
				field := "columns"
				if typ == "gallery" {
					field = "fields"
				}
				value := out[field].([]any)[1].(map[string]any)["calc"].(map[string]any)["result"].(map[string]any)["date"].(map[string]any)["formattedContent"]
				want := "1 minute"
				if page == pageB {
					want = "1 分钟"
				}
				if value != want {
					t.Fatal(spec, page, typ, value, want)
				}
			}
			l.Release()
		}
	}
	if digest(t, root) != before {
		t.Fatal("old source rewritten")
	}
}

func TestNativeCompatibilityRejectsAmbiguousAndFutureWithoutSaving(t *testing.T) {
	for _, variant := range []string{"future", "negative", "missing-key", "missing-value", "nil-block"} {
		a := layoutFixture(fieldAV, pageA, av.CoverFromNone)
		a.Spec = 0
		switch variant {
		case "future":
			a.Spec = 5
		case "negative":
			a.Spec = -1
		case "missing-key":
			a.KeyValues[0].Key.ID = ""
		case "missing-value":
			a.KeyValues[0].Values[0].ID = ""
		case "nil-block":
			a.KeyValues[0].Values[0].Block = nil
		}
		raw, _ := json.Marshal(a)
		_, e := decodeNativeAV(raw, fieldAV, time.Unix(0, 0))
		if e == nil {
			t.Fatal(variant)
		}
		if variant == "future" && !errors.Is(e, avrender.ErrUnsupported) {
			t.Fatal(e)
		}
	}
}

func TestNativeLocalizedDurationRollupAndIndependentGroupCalc(t *testing.T) {
	ctx := context.Background()
	root := fixture(t)
	for _, page := range []string{pageA, pageB} {
		a := computedFixture(page, "select")
		dest := layoutFixture(relatedAV, page, av.CoverFromNone)
		for _, view := range a.Views {
			view.GroupCalc = &av.GroupCalc{Field: "number", FieldCalc: &av.FieldCalc{Operator: av.CalcOperatorRange}}
			if view.Gallery != nil {
				view.Gallery.CardFields[1].Calc = &av.FieldCalc{Operator: av.CalcOperatorRange}
			}
		}
		for _, candidate := range []*av.AttributeView{a, dest} {
			candidate.KeyValues[1].Key.Type = av.KeyTypeDate
			for i, v := range candidate.KeyValues[1].Values {
				v.Type = av.KeyTypeDate
				v.Number = nil
				v.Date = &av.ValueDate{Content: 100000 + int64(i%2)*60000, IsNotEmpty: true}
			}
		}
		for _, v := range a.KeyValues[2].Values {
			v.MSelect = []*av.ValueSelect{{Content: "X"}}
		}
		g, _ := avrenderGroupForTest("X", "duration-group", []string{a.KeyValues[0].Values[0].BlockID, a.KeyValues[0].Values[1].BlockID, a.KeyValues[0].Values[2].BlockID})
		g.GroupCalc = &av.GroupCalc{Field: "number", FieldCalc: &av.FieldCalc{Operator: av.CalcOperatorRange}}
		a.Views[1].Groups = []*av.View{g}
		a.KeyValues[6].Key.Rollup.Calc = &av.RollupCalc{Operator: av.CalcOperatorRange}
		for _, v := range a.KeyValues[5].Values {
			v.Relation.BlockIDs = []string{dest.KeyValues[0].Values[0].BlockID, dest.KeyValues[0].Values[1].BlockID}
		}
		writeLayoutFixture(t, root, page, a)
		writeLayoutFixture(t, root, page, dest)
	}
	before := digest(t, root)
	m := open(t, root, Options{Configuration: func(s, p string) (conf.AppData, error) {
		c, e := originalProjectionConfiguration(s, p)
		c.Lang = "en_US"
		if p == pageB {
			c.Lang = "zh_CN"
		}
		return c, e
	}, ConfigurationBusiness: durationLanguages})
	for _, page := range []string{pageA, pageB} {
		l, e := m.Acquire(ctx, storeID, page)
		if e != nil {
			t.Fatal(e)
		}
		defer l.Release()
		raw, e := l.NativeAttributeViewRender(ctx, fieldAV, "gallery", "", 1, 10, nil)
		if e != nil {
			t.Fatal(e)
		}
		var out struct {
			Groups []struct {
				CardCount int                `json:"cardCount"`
				Fields    []*av.GalleryField `json:"fields"`
				GroupCalc *av.GroupCalc      `json:"groupCalc"`
				Cards     []*av.GalleryCard  `json:"cards"`
			}
		}
		if e = json.Unmarshal(raw, &out); e != nil {
			t.Fatal(e)
		}
		found := false
		want := "1 minute"
		if page == pageB {
			want = "1 分钟"
		}
		for _, g := range out.Groups {
			if g.CardCount == 0 {
				continue
			}
			found = true
			if len(g.Fields) < 2 || g.Fields[1].Calc == nil || g.Fields[1].Calc.Result == nil || g.Fields[1].Calc.Result.Date == nil || g.GroupCalc == nil || g.GroupCalc.FieldCalc == nil || g.GroupCalc.FieldCalc.Result == nil || g.GroupCalc.FieldCalc.Result.Date == nil {
				t.Fatal("missing duration projection", string(raw))
			}
			if g.Fields[1].Calc.Result.Date.FormattedContent != want || g.GroupCalc.FieldCalc.Result.Date.FormattedContent != want {
				t.Fatal(string(raw))
			}
			for _, card := range g.Cards {
				if card.GetValue("rollup").Rollup.Contents[0].Date.FormattedContent != want {
					t.Fatal(string(raw))
				}
			}
		}
		if !found {
			t.Fatal("no group")
		}
		l.Release()
	}
	if digest(t, root) != before {
		t.Fatal("duration wrote source")
	}
}
