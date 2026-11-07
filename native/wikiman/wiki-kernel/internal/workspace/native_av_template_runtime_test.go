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

func TestNativeRuntimeTemplateBranchesDynamicAliasesAndOwnerIsolation(t *testing.T) {
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
				// The else branch is a genuine self-cycle but must not execute.
				a.KeyValues[4].Key.Template = `.action{$m := .id_mod}.action{$k := "number"}.action{if gt (index $m $k) 0.0}.action{get $m "primary"}:.action{index $m $k}.action{else}.action{.Label}.action{end}`
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
					count := 0
					var walk func(any)
					walk = func(x any) {
						switch v := x.(type) {
						case map[string]any:
							if v["type"] == "template" {
								if tpl, ok := v["template"].(map[string]any); ok {
									content, _ := tpl["content"].(string)
									want := page + "0:1"
									if page == pageB {
										want = page + "0:11"
									}
									if content == want {
										count++
									}
									if !strings.HasPrefix(content, page) {
										t.Fatal("wrong owner", content)
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
						t.Fatal("missing computed first row", page, layout, string(raw))
					}
				}
				l.Release()
			}
			if digest(t, root) != before {
				t.Fatal("read saved source")
			}
		})
	}
}

func TestNativeRuntimeTemplateExecutedCycleDoesNotPublishPartialJSON(t *testing.T) {
	ctx := context.Background()
	root := fixture(t)
	a := computedFixture(pageA, "")
	a.KeyValues[4].Key.Template = `.action{if true}.action{$m := .id_mod}.action{index $m "template"}.action{else}safe.action{end}`
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
		if raw != nil || !errors.As(e, &cycle) || len(cycle.Path) != 2 {
			t.Fatal(string(raw), e)
		}
	}
	if digest(t, root) != before {
		t.Fatal("cycle changed source")
	}
}
