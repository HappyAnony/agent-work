//go:build fts5

package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	av "wiki-kernel/internal/avmodel"
	"wiki-kernel/internal/avrender"
)

func TestNativeMapMutatorsLayoutsGroupsIsolationAndRawCache(t *testing.T) {
	for _, rawMode := range []bool{false, true} {
		for _, grouped := range []bool{false, true} {
			t.Run(fmt.Sprintf("raw=%t/group=%t", rawMode, grouped), func(t *testing.T) {
				ctx := context.Background()
				root := fixture(t)
				for _, page := range []string{pageA, pageB} {
					group := ""
					if grouped {
						group = "template"
					}
					a := computedFixture(page, group)
					if page == pageB {
						for _, v := range a.KeyValues[1].Values {
							v.Number.Content += 10
						}
					}
					a.KeyValues[4].Key.Template = `.action{$m := .id_mod}.action{$_ := set $m "scratch" .Name}.action{$_ := unset $m "never"}.action{$_ := merge $m (dict "first" "a")}.action{$_ := mustMerge $m (dict "second" "b")}.action{$_ := mergeOverwrite $m (dict "template" "local")}.action{$_ := mustMergeOverwrite $m (dict "template" "last")}.action{.number}:.action{get $m "scratch"}:.action{get $m "template"}:.action{hasKey $m "never"}`
					if rawMode {
						other := &av.KeyValues{Key: &av.Key{ID: "other", Name: "Other", Type: av.KeyTypeNumber}}
						content := float64(9)
						if page == pageB {
							content = 19
						}
						for _, v := range a.KeyValues[1].Values {
							other.Values = append(other.Values, &av.Value{ID: v.ID + "o", KeyID: "other", BlockID: v.BlockID, Type: av.KeyTypeNumber, Number: &av.ValueNumber{Content: content, IsNotEmpty: true}})
						}
						a.KeyValues = append(a.KeyValues, other, &av.KeyValues{Key: &av.Key{ID: "observer", Name: "Observer", Type: av.KeyTypeTemplate, Template: `.action{.number_raw.Number.Content}`}})
						a.KeyValues[4].Key.Template = `.action{$n := .number_raw}.action{$_ := mergeOverwrite .id_mod_raw (dict "number" .Other_raw)}.action{.Name}:.action{$n.Number.Content}:.action{.number}`
						for _, view := range a.Views {
							switch view.LayoutType {
							case av.LayoutTypeTable:
								view.Table.Columns = append(view.Table.Columns, &av.ViewTableColumn{BaseField: &av.BaseField{ID: "observer"}})
							case av.LayoutTypeGallery:
								view.Gallery.CardFields = append(view.Gallery.CardFields, &av.ViewGalleryCardField{BaseField: &av.BaseField{ID: "observer"}})
							case av.LayoutTypeKanban:
								view.Kanban.Fields = append(view.Kanban.Fields, &av.ViewKanbanField{BaseField: &av.BaseField{ID: "observer"}})
							}
						}
					}
					writeLayoutFixture(t, root, page, a)
					writeLayoutFixture(t, root, page, layoutFixture(relatedAV, page, 0))
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
							t.Fatal(page, layout, e)
						}
						want := "1:" + page + "0:last:false"
						observed := "1"
						badRaw := "9"
						if page == pageB {
							want = "11:" + page + "0:last:false"
							observed = "11"
							badRaw = "19"
						}
						if rawMode {
							want = page + "0:" + badRaw + ":" + observed
						}
						var data any
						if e = json.Unmarshal(raw, &data); e != nil {
							t.Fatal(e)
						}
						labels, observers := 0, 0
						var walk func(any)
						walk = func(x any) {
							switch v := x.(type) {
							case map[string]any:
								if v["type"] == "template" {
									if tpl, ok := v["template"].(map[string]any); ok {
										content, _ := tpl["content"].(string)
										if v["keyID"] == "template" && content == want {
											labels++
										}
										if v["keyID"] == "observer" {
											if content == badRaw {
												t.Fatal("raw cache leaked between fields", page, content)
											}
											if content == observed {
												observers++
											}
										}
									}
								}
								for _, child := range v {
									walk(child)
								}
							case []any:
								for _, child := range v {
									walk(child)
								}
							}
						}
						walk(data)
						if labels == 0 || (rawMode && observers == 0) {
							t.Fatal(page, layout, want, string(raw))
						}
					}
					l.Release()
				}
				if digest(t, root) != before {
					t.Fatal("map mutation saved authoritative source")
				}
			})
		}
	}
}

func TestNativeMapMutatorsFailuresAreIsolatedAndNoPartialJSON(t *testing.T) {
	for _, source := range []string{`.action{$_ := merge .id_mod (dict "template" "x")}`, `.action{$_ := mustMerge .id_mod (dict "template" "x")}`, `.action{$_ := mergeOverwrite (dict) .id_mod}`, `.action{$_ := mustMergeOverwrite (dict) .id_mod}`, `.action{$_ := set .id_mod "template" .id_mod}.action{merge .id_mod (dict "x" 1)}`} {
		t.Run(source, func(t *testing.T) {
			ctx := context.Background()
			root := fixture(t)
			for _, page := range []string{pageA, pageB} {
				a := computedFixture(page, "")
				if page == pageA {
					a.KeyValues[4].Key.Template = source
				}
				writeLayoutFixture(t, root, page, a)
				writeLayoutFixture(t, root, page, layoutFixture(relatedAV, page, 0))
			}
			before := digest(t, root)
			m := open(t, root, Options{Configuration: originalProjectionConfiguration, ConfigurationBusiness: layoutLanguages})
			l, e := m.Acquire(ctx, storeID, pageA)
			if e != nil {
				t.Fatal(e)
			}
			defer l.Release()
			for _, layout := range []string{"table", "gallery", "kanban"} {
				raw, e := l.NativeAttributeViewRender(ctx, fieldAV, layout, "", 1, 10, nil)
				if raw != nil || (!errors.Is(e, avrender.ErrFieldCycle) && !errors.Is(e, avrender.ErrLimit)) {
					t.Fatal(string(raw), e)
				}
			}
			other, e := m.Acquire(ctx, storeID, pageB)
			if e != nil {
				t.Fatal(e)
			}
			defer other.Release()
			if _, e = other.NativeAttributeViewRender(ctx, fieldAV, "table", "", 1, 10, nil); e != nil {
				t.Fatal("failed owner poisoned other page", e)
			}
			if digest(t, root) != before {
				t.Fatal("failed mutation saved source")
			}
		})
	}
}
