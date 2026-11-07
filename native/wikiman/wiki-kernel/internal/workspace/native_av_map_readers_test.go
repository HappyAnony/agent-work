//go:build fts5

package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"wiki-kernel/internal/avrender"
)

func TestNativeMapReadersLayoutsGroupsAndOwnerIsolation(t *testing.T) {
	for _, grouped := range []bool{false, true} {
		t.Run(fmt.Sprint(grouped), func(t *testing.T) {
			ctx := context.Background()
			root := fixture(t)
			for _, page := range []string{pageA, pageB} {
				group := ""
				if grouped {
					group = "template"
				}
				a := computedFixture(page, group)
				a.KeyValues[4].Key.Template = `.action{$m := .id_mod}.action{get (pick $m "primary") "primary"}:.action{dig "number" "fallback" $m}:.action{pluck "number" $m | first}:.action{values (pick $m "number") | first}:.action{keys (omit $m "template") | len}:.action{keys $m | len}`
				if page == pageB {
					for _, v := range a.KeyValues[1].Values {
						v.Number.Content += 10
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
					var data any
					if e = json.Unmarshal(raw, &data); e != nil {
						t.Fatal(e)
					}
					want := page + "0:1:1:1:6:7"
					if page == pageB {
						want = page + "0:11:11:11:6:7"
					}
					count := 0
					var walk func(any)
					walk = func(x any) {
						switch v := x.(type) {
						case map[string]any:
							if v["type"] == "template" {
								if tpl, ok := v["template"].(map[string]any); ok {
									content, _ := tpl["content"].(string)
									if !strings.HasPrefix(content, page) {
										t.Fatal("wrong owner", content)
									}
									if content == want {
										count++
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
					if count == 0 {
						t.Fatal(page, layout, want, string(raw))
					}
				}
				l.Release()
			}
			if digest(t, root) != before {
				t.Fatal("map read saved source")
			}
		})
	}
}

func TestNativeMapReadersExecutedCyclesHaveNoPartialResult(t *testing.T) {
	for _, source := range []string{`.action{dig "template" "fallback" .id_mod}`, `.action{pluck "template" .id_mod}`, `.action{pick .id_mod "template"}`, `.action{omit .id_mod "absent"}`, `.action{values .id_mod}`, `.action{printf "%v" (pluck "id_mod" .)}`} {
		t.Run(source, func(t *testing.T) {
			ctx := context.Background()
			root := fixture(t)
			a := computedFixture(pageA, "")
			a.KeyValues[4].Key.Template = source
			writeLayoutFixture(t, root, pageA, a)
			writeLayoutFixture(t, root, pageA, layoutFixture(relatedAV, pageA, 0))
			before := digest(t, root)
			m := open(t, root, Options{Configuration: originalProjectionConfiguration, ConfigurationBusiness: layoutLanguages})
			l, e := m.Acquire(ctx, storeID, pageA)
			if e != nil {
				t.Fatal(e)
			}
			defer l.Release()
			for _, layout := range []string{"table", "gallery", "kanban"} {
				raw, e := l.NativeAttributeViewRender(ctx, fieldAV, layout, "", 1, 10, nil)
				var cycle *avrender.FieldCycleError
				if raw != nil || !errors.As(e, &cycle) || len(cycle.Path) < 2 {
					t.Fatal(layout, string(raw), e)
				}
				for _, ref := range cycle.Path {
					if ref.AV != fieldAV {
						t.Fatal("wrong AV in cycle", cycle.Path)
					}
				}
			}
			if digest(t, root) != before {
				t.Fatal("cycle mutated source")
			}
		})
	}
}

func TestNativeMapReadersContainerCycleRejectedBeforeFormatting(t *testing.T) {
	ctx := context.Background()
	root := fixture(t)
	a := computedFixture(pageA, "")
	a.KeyValues[4].Key.Template = `.action{$m := dict}.action{$_ := set $m "self" $m}.action{printf "%v" $m}`
	writeLayoutFixture(t, root, pageA, a)
	writeLayoutFixture(t, root, pageA, layoutFixture(relatedAV, pageA, 0))
	before := digest(t, root)
	m := open(t, root, Options{Configuration: originalProjectionConfiguration, ConfigurationBusiness: layoutLanguages})
	l, e := m.Acquire(ctx, storeID, pageA)
	if e != nil {
		t.Fatal(e)
	}
	defer l.Release()
	for _, layout := range []string{"table", "gallery", "kanban"} {
		raw, e := l.NativeAttributeViewRender(ctx, fieldAV, layout, "", 1, 10, nil)
		if raw != nil || !errors.Is(e, avrender.ErrLimit) {
			t.Fatal(layout, string(raw), e)
		}
	}
	if digest(t, root) != before {
		t.Fatal("container failure mutated source")
	}
}
