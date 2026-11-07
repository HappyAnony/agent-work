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

const fieldAV = "20261023090500-av00001"
const relatedAV = "20261023090501-av00002"

func computedFixture(page string, group string) *av.AttributeView {
	a := layoutFixture(fieldAV, page, av.CoverFromNone)
	a.KeyValues = append(a.KeyValues,
		&av.KeyValues{Key: &av.Key{ID: "template", Name: "Label", Type: av.KeyTypeTemplate, Template: `.action{.Name}-.action{.number}`}},
		&av.KeyValues{Key: &av.Key{ID: "relation", Name: "Related", Type: av.KeyTypeRelation, Relation: &av.Relation{AvID: relatedAV}}},
		&av.KeyValues{Key: &av.Key{ID: "rollup", Name: "Total", Type: av.KeyTypeRollup, Rollup: &av.Rollup{RelationKeyID: "relation", KeyID: "number", Calc: &av.RollupCalc{Operator: av.CalcOperatorSum}}}},
	)
	a.KeyValues[1].Key.Name = "number"
	for _, v := range a.KeyValues[0].Values {
		a.KeyValues[5].Values = append(a.KeyValues[5].Values, &av.Value{ID: v.ID + "r", KeyID: "relation", BlockID: v.BlockID, Type: av.KeyTypeRelation, Relation: &av.ValueRelation{BlockIDs: []string{blockID}}})
	}
	for _, v := range a.Views {
		if group != "" {
			v.Group = &av.ViewGroup{Field: group, Method: av.GroupMethodValue}
		}
		for _, k := range a.KeyValues[4:] {
			switch v.LayoutType {
			case av.LayoutTypeTable:
				v.Table.Columns = append(v.Table.Columns, &av.ViewTableColumn{BaseField: &av.BaseField{ID: k.Key.ID}})
			case av.LayoutTypeGallery:
				v.Gallery.CardFields = append(v.Gallery.CardFields, &av.ViewGalleryCardField{BaseField: &av.BaseField{ID: k.Key.ID}})
			case av.LayoutTypeKanban:
				v.Kanban.Fields = append(v.Kanban.Fields, &av.ViewKanbanField{BaseField: &av.BaseField{ID: k.Key.ID}})
			}
		}
	}
	return a
}

func TestNativeTemplatesRelationsRollupsLayoutsGroupsIsolation(t *testing.T) {
	for _, group := range []string{"", "template", "relation", "rollup"} {
		t.Run(group, func(t *testing.T) {
			ctx := context.Background()
			root := fixture(t)
			for _, page := range []string{pageA, pageB} {
				a := computedFixture(page, group)
				writeLayoutFixture(t, root, page, a)
				dest := layoutFixture(relatedAV, page, av.CoverFromNone)
				if page == pageB {
					dest.KeyValues[1].Values[0].Number.Content = 17
				}
				writeLayoutFixture(t, root, page, dest)
			}
			before := digest(t, root)
			m := open(t, root, Options{Configuration: originalProjectionConfiguration, ConfigurationBusiness: layoutLanguages})
			for _, page := range []string{pageA, pageB} {
				l, e := m.Acquire(ctx, storeID, page)
				if e != nil {
					t.Fatal(e)
				}
				for _, layout := range []string{"table", "gallery", "kanban"} {
					raw, e := l.NativeAttributeViewRender(ctx, fieldAV, layout, "", 1, 10, nil)
					if e != nil {
						t.Fatalf("%s/%s: %v", page, layout, e)
					}
					other := pageA
					if page == pageA {
						other = pageB
					}
					if strings.Contains(string(raw), other) {
						t.Fatal("cross workspace leak", string(raw))
					}
					if !strings.Contains(string(raw), page+"0-1") {
						t.Fatal("template missing", string(raw))
					}
					var view map[string]json.RawMessage
					if e = json.Unmarshal(raw, &view); e != nil {
						t.Fatal(e)
					}
					// Decode recursively to assert computed values rather than merely source rule metadata.
					n := 0
					var scan func(any)
					scan = func(x any) {
						switch v := x.(type) {
						case map[string]any:
							if v["type"] == "rollup" {
								if r, ok := v["rollup"].(map[string]any); ok {
									if vals, ok := r["contents"].([]any); ok && len(vals) > 0 {
										expected := float64(1)
										if page == pageB {
											expected = 17
										}
										num := vals[0].(map[string]any)["number"].(map[string]any)["content"]
										if num != expected {
											t.Fatalf("rollup %v want %v", num, expected)
										}
										n++
									}
								}
							}
							for _, x := range v {
								scan(x)
							}
						case []any:
							for _, x := range v {
								scan(x)
							}
						}
					}
					var value any
					json.Unmarshal(raw, &value)
					scan(value)
					if n == 0 {
						t.Fatal("no computed rollup", string(raw))
					}
				}
				l.Release()
			}
			if digest(t, root) != before {
				t.Fatal("render saved authoritative source")
			}
		})
	}
}

func TestNativeComputedFieldFailures(t *testing.T) {
	for _, failure := range []string{"missing", "symlink", "cycle", "recursion", "sql-write", "duration", "cancel"} {
		t.Run(failure, func(t *testing.T) {
			ctx := context.Background()
			root := fixture(t)
			a := computedFixture(pageA, "template")
			dest := layoutFixture(relatedAV, pageA, av.CoverFromNone)
			switch failure {
			case "cycle":
				a.KeyValues[4].Key.Template = `.action{.Label}`
			case "recursion":
				a.KeyValues[4].Key.Template = `.action{.Total}`
				a.KeyValues[6].Key.Rollup.KeyID = "template"
				dest = computedFixture(pageA, "")
				dest.ID = relatedAV
				dest.KeyValues[5].Key.Relation.AvID = fieldAV
				dest.KeyValues[6].Key.Rollup.KeyID = "template"
				dest.KeyValues[4].Key.Template = `.action{.Total}`
			case "sql-write":
				a.KeyValues[4].Key.Template = `.action{querySQL "delete from blocks"}`
			case "duration":
				dest.KeyValues[1].Key.Type = av.KeyTypeDate
				for _, v := range dest.KeyValues[1].Values {
					v.Type = av.KeyTypeDate
					v.Date = &av.ValueDate{Content: 1, IsNotEmpty: true}
				}
				a.KeyValues[6].Key.Rollup.Calc.Operator = av.CalcOperatorRange
			}
			writeLayoutFixture(t, root, pageA, a)
			writeLayoutFixture(t, root, pageA, dest)
			writeLayoutFixture(t, root, pageB, dest)
			path := filepath.Join(root, "pages", pageA, "data", "storage", "av", relatedAV+".json")
			if failure == "missing" || failure == "symlink" {
				if e := os.Remove(path); e != nil {
					t.Fatal(e)
				}
				if failure == "symlink" {
					if e := os.Symlink(filepath.Join(root, "pages", pageB, "data", "storage", "av", relatedAV+".json"), path); e != nil {
						t.Fatal(e)
					}
				}
			}
			before := digest(t, root)
			m := open(t, root, Options{Configuration: originalProjectionConfiguration, ConfigurationBusiness: layoutLanguages})
			l, e := m.Acquire(ctx, storeID, pageA)
			if e != nil {
				t.Fatal(e)
			}
			defer l.Release()
			if failure == "cancel" {
				c, cancel := context.WithCancel(ctx)
				cancel()
				ctx = c
			}
			raw, e := l.NativeAttributeViewRender(ctx, fieldAV, "table", "", 1, 10, nil)
			if e == nil || raw != nil {
				t.Fatal("partial success", string(raw), e)
			}
			if failure == "cycle" && !errors.Is(e, avrender.ErrFieldCycle) {
				t.Fatal(e)
			}
			if failure == "recursion" && !errors.Is(e, avrender.ErrFieldCycle) {
				t.Fatal(e)
			}
			if digest(t, root) != before {
				t.Fatal("failure mutated source")
			}
		})
	}
}

func TestNativeSelfRelationNumberRollup(t *testing.T) {
	ctx := context.Background()
	root := fixture(t)
	a := computedFixture(pageA, "relation")
	a.KeyValues[5].Key.Relation.AvID = fieldAV
	writeLayoutFixture(t, root, pageA, a)
	before := digest(t, root)
	m := open(t, root, Options{Configuration: originalProjectionConfiguration, ConfigurationBusiness: layoutLanguages})
	l, e := m.Acquire(ctx, storeID, pageA)
	if e != nil {
		t.Fatal(e)
	}
	defer l.Release()
	raw, e := l.NativeAttributeViewRender(ctx, fieldAV, "table", "", 1, 10, nil)
	if e != nil {
		t.Fatal(e)
	}
	var out struct{ Groups []json.RawMessage }
	if e = json.Unmarshal(raw, &out); e != nil {
		t.Fatal(e)
	}
	found := false
	for _, g := range out.Groups {
		var group av.Table
		if e = json.Unmarshal(g, &group); e != nil {
			t.Fatal(e)
		}
		if group.RowCount > 0 {
			found = true
			for _, row := range group.Rows {
				if row.Cells[6].Value.Rollup.Contents[0].Number.Content != 1 {
					t.Fatal(string(raw))
				}
			}
		}
	}
	if !found || digest(t, root) != before {
		t.Fatal("self relation source/result", string(raw))
	}
}

func TestNativeTemplateOwnerServicesAndComputedRollupFilter(t *testing.T) {
	ctx := context.Background()
	root := fixture(t)
	for _, page := range []string{pageA, pageB} {
		a := computedFixture(page, "")
		a.KeyValues[4].Key.Template = `.action{(getBlock "` + blockID + `").Content}|.action{getHPathByID "` + blockID + `"}|.action{(statBlock "` + blockID + `").BlockCount}|.action{len (queryBlocks "SELECT * FROM blocks")}|.action{len (querySpans "SELECT * FROM spans")}|.action{len (querySQL "SELECT * FROM blocks")}`
		// Target template evaluation must be available to rollup filtering, not just
		// display. No global destination AV or previously rendered result is allowed.
		a.KeyValues[6].Key.Rollup.KeyID = "template"
		a.KeyValues[6].Key.Rollup.Calc = nil
		a.Views[0].Filters = []*av.ViewFilter{{Column: "rollup", Operator: av.FilterOperatorContains, Value: &av.Value{Type: av.KeyTypeRollup, Rollup: &av.ValueRollup{Contents: []*av.Value{{Type: av.KeyTypeTemplate, Template: &av.ValueTemplate{Content: page}}}}}}}
		writeLayoutFixture(t, root, page, a)
		dest := computedFixture(page, "")
		dest.ID = relatedAV
		dest.KeyValues = dest.KeyValues[:5]
		for _, v := range dest.Views {
			switch v.LayoutType {
			case av.LayoutTypeTable:
				v.Table.Columns = v.Table.Columns[:5]
			case av.LayoutTypeGallery:
				v.Gallery.CardFields = v.Gallery.CardFields[:5]
			case av.LayoutTypeKanban:
				v.Kanban.Fields = v.Kanban.Fields[:5]
			}
		}
		writeLayoutFixture(t, root, page, dest)
	}
	before := digest(t, root)
	m := open(t, root, Options{Configuration: originalProjectionConfiguration})
	for _, page := range []string{pageA, pageB} {
		l, e := m.Acquire(ctx, storeID, page)
		if e != nil {
			t.Fatal(e)
		}
		defer l.Release()
		if e = l.QueueNativeTree(ctx, boxID, docID, "index"); e != nil {
			t.Fatal(e)
		}
		if e = l.FlushNativeMainQueue(ctx); e != nil {
			t.Fatal(e)
		}
		raw, e := l.NativeAttributeViewRender(ctx, fieldAV, "table", "", 1, 10, nil)
		if e != nil {
			t.Fatal(e)
		}
		var out av.Table
		if e = json.Unmarshal(raw, &out); e != nil {
			t.Fatal(e)
		}
		if out.RowCount != 3 || len(out.Rows) != 3 {
			t.Fatal("rollup template filter", string(raw))
		}
		expected := "Synthetic knowledge"
		other := "Workspace B knowledge"
		if page == pageB {
			expected, other = other, expected
		}
		label := out.Rows[0].Cells[4].Value.Template.Content
		if !strings.Contains(label, expected) || strings.Contains(label, other) || !strings.Contains(label, "|1|4|1|4") {
			t.Fatal("owner template services", label)
		}
		l.Release()
	}
	if digest(t, root) != before {
		t.Fatal("template services wrote source")
	}
}
