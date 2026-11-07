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

func TestNativeRecursiveFieldGraphBeyondSevenViewsAndOwnerIsolation(t *testing.T) {
	ctx := context.Background()
	root := fixture(t)
	const count = 12
	for _, page := range []string{pageA, pageB} {
		for i := 0; i < count; i++ {
			id := fmt.Sprintf("202610230911%02d-av00001", i)
			next := min(i+1, count-1)
			a := computedFixture(page, "template")
			a.ID = id
			a.KeyValues[5].Key.Relation.AvID = fmt.Sprintf("202610230911%02d-av00001", next)
			a.KeyValues[6].Key.Rollup.KeyID = "template"
			a.KeyValues[6].Key.Rollup.Calc = nil
			a.KeyValues[4].Key.Template = `.action{index .Total 0}`
			if i == count-1 {
				a.KeyValues[4].Key.Template = `.action{index .id_mod "number"}`
				if page == pageB {
					a.KeyValues[1].Values[0].Number.Content = 17
				}
			}
			writeLayoutFixture(t, root, page, a)
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
		for _, typ := range []string{"table", "gallery", "kanban"} {
			raw, e := l.NativeAttributeViewRender(ctx, "20261023091100-av00001", typ, "", 1, 10, nil)
			if e != nil {
				t.Fatal(page, typ, e)
			}
			var out struct{ Groups []json.RawMessage }
			if e = json.Unmarshal(raw, &out); e != nil {
				t.Fatal(e)
			}
			found := false
			want := "1"
			if page == pageB {
				want = "17"
			}
			for _, g := range out.Groups {
				var group map[string]any
				json.Unmarshal(g, &group)
				items := group["rows"]
				if typ != "table" {
					items = group["cards"]
				}
				for _, item := range items.([]any) {
					m := item.(map[string]any)
					cells := m["cells"]
					if typ != "table" {
						cells = m["values"]
					}
					_ = cells
					found = true
				}
				// Scan typed values independently of layout-specific cell nesting.
				var walk func(any)
				walk = func(x any) {
					switch v := x.(type) {
					case map[string]any:
						if v["type"] == "template" {
							if tpl, ok := v["template"].(map[string]any); ok {
								if tpl["content"] != want {
									t.Fatalf("%s/%s: template=%v want %s", page, typ, tpl["content"], want)
								}
							}
						}
						for _, z := range v {
							walk(z)
						}
					case []any:
						for _, z := range v {
							walk(z)
						}
					}
				}
				walk(group)
			}
			if !found {
				t.Fatal("no recursively computed group rows", string(raw))
			}
		}
		l.Release()
	}
	if digest(t, root) != before {
		t.Fatal("recursive projection saved source")
	}
}

func TestNativeFieldCycleHasExplicitIdentityPathAndNoPartialResult(t *testing.T) {
	ctx := context.Background()
	root := fixture(t)
	a := computedFixture(pageA, "template")
	a.KeyValues[5].Key.Relation.AvID = fieldAV
	a.KeyValues[6].Key.Rollup.KeyID = "template"
	a.KeyValues[6].Key.Rollup.Calc = nil
	a.KeyValues[4].Key.Template = `.action{index .Total 0}`
	writeLayoutFixture(t, root, pageA, a)
	before := digest(t, root)
	m := open(t, root, Options{Configuration: originalProjectionConfiguration, ConfigurationBusiness: layoutLanguages})
	l, e := m.Acquire(ctx, storeID, pageA)
	if e != nil {
		t.Fatal(e)
	}
	defer l.Release()
	raw, e := l.NativeAttributeViewRender(ctx, fieldAV, "table", "", 1, 10, nil)
	var cycle *avrender.FieldCycleError
	if raw != nil || !errors.As(e, &cycle) || len(cycle.Path) < 3 {
		t.Fatal(string(raw), e)
	}
	for _, f := range cycle.Path {
		if f.AV != fieldAV || !strings.Contains(f.Row, "-") {
			t.Fatal(cycle.Path)
		}
	}
	if digest(t, root) != before {
		t.Fatal("cycle mutated source")
	}
}
