package avrender

import (
	"context"
	"errors"
	"reflect"
	"regexp"
	"testing"
	"text/template"
	"time"
	av "wiki-kernel/internal/avmodel"
)

func TestRuntimeTemplateBranchesDynamicKeysAliasesAndCycles(t *testing.T) {
	for _, tc := range []struct {
		name, source, want string
		cycle              bool
	}{
		{"dynamic-id", `.action{$key := "leaf"}.action{index .id_mod $key}`, "ok", false},
		{"dynamic-name", `.action{$key := "Leaf"}.action{get . $key}`, "ok", false},
		{"alias", `.action{$m := .id_mod}.action{$key := "leaf"}.action{get $m $key}`, "ok", false},
		{"alias-field", `.action{$m := .}.action{$m.Leaf}`, "ok", false},
		{"alias-raw", `.action{$m := .id_mod_raw}.action{(index $m "leaf").Template.Content}`, "ok", false},
		{"if-dead-cycle", `.action{if false}.action{.Self}.action{else}.action{.Leaf}.action{end}`, "ok", false},
		{"if-live-cycle", `.action{if true}.action{.Self}.action{end}`, "", true},
		{"and-short-circuit", `.action{and false .Self}`, "false", false},
		{"or-short-circuit", `.action{or true .Self}`, "true", false},
		{"with-map", `.action{with .id_mod}.action{index . "leaf"}.action{end}`, "ok", false},
		{"range-local-root", `.action{range list "x" "y"}.action{.}:.action{$.Leaf};.action{end}`, "x:ok;y:ok;", false},
		{"named-template", `.action{define "part"}.action{index . "leaf"}.action{end}.action{template "part" .id_mod}`, "ok", false},
		{"map-length", `.action{len .id_mod}`, "3", false},
		{"missing-key", `.action{index .id_mod "absent" | default "fallback"}`, "fallback", false},
		{"whole-print", `.action{printf "%v" .id_mod}`, "", true},
		{"whole-pipe", `.action{.id_mod | printf "%v"}`, "", true},
		{"whole-range", `.action{range .id_mod}.action{.}.action{end}`, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			const row = "20261022120000-row0001"
			a := &av.AttributeView{ID: "A", KeyValues: []*av.KeyValues{
				{Key: &av.Key{ID: "primary", Name: "Name", Type: av.KeyTypeBlock}, Values: []*av.Value{{ID: row, KeyID: "primary", BlockID: row, Type: av.KeyTypeBlock, Block: &av.ValueBlock{ID: row}}}},
				{Key: &av.Key{ID: "self", Name: "Self", Type: av.KeyTypeTemplate, Template: tc.source}},
				{Key: &av.Key{ID: "leaf", Name: "Leaf", Type: av.KeyTypeTemplate, Template: "ok"}},
			}}
			now := func() time.Time { return time.Unix(0, 0) }
			r := &FieldResolver{Context: context.Background(), MaxNodes: 1000, MaxDepth: 100, Read: func(string) (*av.AttributeView, error) { t.Fatal("unexpected read"); return nil, nil }, Prepare: func(*av.AttributeView, av.Collection) error { return nil }, Templates: TemplateEnvironment{Now: now, Funcs: BuiltInTemplateFuncs(now, nil, nil)}}
			v, err := r.Resolve(a, "self", row)
			if tc.cycle {
				var cycle *FieldCycleError
				if v != nil || !errors.As(err, &cycle) || len(cycle.Path) != 2 || cycle.Path[0] != (FieldRef{"A", "self", row}) || cycle.Path[1] != cycle.Path[0] {
					t.Fatal(v, err)
				}
				if len(r.stack) != 0 || r.states[FieldRef{"A", "self", row}] != 0 {
					t.Fatal("failed traversal leaked")
				}
			} else if err != nil || v.Template.Content != tc.want {
				t.Fatal(v, err, tc.want)
			}
			if a.KeyValues[1].Key.Template != tc.source || len(a.KeyValues[1].Values) != 0 {
				t.Fatal("source mutated")
			}
		})
	}
}

func TestLazyTemplateConditionExecutesOnceAndPropagatesDependencyErrors(t *testing.T) {
	now := func() time.Time { return time.Unix(0, 0) }
	kvs := []*av.KeyValues{{Key: &av.Key{ID: "t", Name: "Title", Type: av.KeyTypeTemplate}, Values: []*av.Value{{Type: av.KeyTypeTemplate, Template: &av.ValueTemplate{Content: "resolved"}}}}}
	for _, failure := range []bool{false, true} {
		calls := 0
		funcs := BuiltInTemplateFuncs(now, nil, nil)
		funcs["once"] = func() bool { calls++; return false }
		want := errors.New("owner dependency failed")
		env := TemplateEnvironment{Now: now, Funcs: funcs, Resolve: func(string) error {
			if failure {
				return want
			}
			return nil
		}}
		got, e := RenderTemplateField(nil, kvs, `.action{if once}unused.action{else}.action{$k := "t"}.action{index .id_mod $k}.action{end}`, env)
		if calls != 1 {
			t.Fatal("condition pre-evaluated", calls)
		}
		if failure {
			if !errors.Is(e, want) || got != "" {
				t.Fatal(got, e)
			}
		} else if e != nil || got != "resolved" {
			t.Fatal(got, e)
		}
	}
}

func TestLazyTemplateMatchesOriginalTypedModelAndScopes(t *testing.T) {
	now := func() time.Time { return time.Date(2026, 10, 22, 12, 0, 0, 0, time.Local) }
	kvs := []*av.KeyValues{
		{Key: &av.Key{ID: "n", Name: "Number", Type: av.KeyTypeNumber}, Values: []*av.Value{{Type: av.KeyTypeNumber, Number: &av.ValueNumber{Content: 3, IsNotEmpty: true}}}},
		{Key: &av.Key{ID: "d", Name: "Date", Type: av.KeyTypeDate}, Values: []*av.Value{{Type: av.KeyTypeDate, Date: &av.ValueDate{Content: now().UnixMilli(), IsNotEmpty: true}}}},
		{Key: &av.Key{ID: "t", Name: "Title", Type: av.KeyTypeTemplate}, Values: []*av.Value{{Type: av.KeyTypeTemplate, Template: &av.ValueTemplate{Content: "resolved"}}}},
	}
	funcs := BuiltInTemplateFuncs(now, nil, template.FuncMap{})
	for _, source := range []string{
		`.action{addf .Number 2}/.action{.Date.Format "2006"}/.action{.Title_raw.Template.Content}`,
		`.action{$m := .id_mod}.action{$k := "n"}.action{printf "%T:%v" (index $m $k) (index $m $k)}`,
		`.action{with .Date}.action{.Format "2006"}.action{end}`,
		`.action{define "x"}.action{.Title}.action{end}.action{template "x" .}`,
		`.action{range list "a" "b"}.action{.}:.action{$.Number};.action{end}`,
		`.action{printf "%v" .}`,
		`.action{toJson .}`,
		`.action{range $k,$v := .id_mod}.action{$k}:.action{$v};.action{end}`,
		`.action{get . "missing"}/.action{index (list "x" "y") 1}`,
	} {
		env := TemplateEnvironment{Now: now, Funcs: funcs}
		want, err := RenderTemplateField(nil, kvs, source, env)
		if err != nil {
			t.Fatal(err)
		}
		var visited []string
		env.Resolve = func(id string) error { visited = append(visited, id); return nil }
		got, err := RenderTemplateField(nil, kvs, source, env)
		if source == `.action{printf "%v" .}` {
			// Native raw objects are now private per-evaluation copies; addresses
			// are not a stable result contract. All non-address output still matches.
			addresses := regexp.MustCompile(`0x[0-9a-f]+`)
			got, want = addresses.ReplaceAllString(got, "<pointer>"), addresses.ReplaceAllString(want, "<pointer>")
		}
		if err != nil || got != want {
			t.Fatal(source, got, want, err)
		}
		if len(visited) == 0 && source != `.action{get . "missing"}/.action{index (list "x" "y") 1}` {
			t.Fatal("not instrumented", source)
		}
	}
	if _, ok := funcs[lazyPrepare]; ok {
		t.Fatal("mutated caller functions")
	}
	// The copied map remains identical after repeated executions.
	if !reflect.DeepEqual(reflect.ValueOf(funcs["get"]).Pointer(), reflect.ValueOf(BuiltInTemplateFuncs(now, nil, nil)["get"]).Pointer()) {
		t.Fatal("get replaced")
	}
}

func TestLazyTemplateDuplicateNamePrecedenceAndRefreshBudget(t *testing.T) {
	now := func() time.Time { return time.Unix(0, 0) }
	for _, nonempty := range []bool{false, true} {
		kvs := []*av.KeyValues{
			{Key: &av.Key{ID: "old", Name: "Same", Type: av.KeyTypeTemplate}, Values: []*av.Value{{Type: av.KeyTypeTemplate, Template: &av.ValueTemplate{Content: "old"}}}},
			{Key: &av.Key{ID: "new", Name: "Same", Type: av.KeyTypeNumber}, Values: []*av.Value{{Type: av.KeyTypeNumber, Number: &av.ValueNumber{Content: 4, IsNotEmpty: nonempty}}}},
		}
		for _, source := range []string{`.action{.Same}`, `.action{index .id_mod "new"}`} {
			env := TemplateEnvironment{Now: now, Funcs: BuiltInTemplateFuncs(now, nil, nil)}
			want, e := RenderTemplateField(nil, kvs, source, env)
			if e != nil {
				t.Fatal(e)
			}
			var visits []string
			env.Resolve = func(id string) error {
				visits = append(visits, id)
				if nonempty && id == "old" {
					t.Fatal("overwritten field resolved")
				}
				return nil
			}
			got, e := RenderTemplateField(nil, kvs, source, env)
			if e != nil || got != want {
				t.Fatal(got, want, e)
			}
			if !nonempty && !reflect.DeepEqual(visits, []string{"new", "old"}) {
				t.Fatal(visits)
			}
			env.Charge = func(n int) error {
				if n != len(kvs) {
					t.Fatal(n)
				}
				return ErrLimit
			}
			if _, e = RenderTemplateField(nil, kvs, source, env); !errors.Is(e, ErrLimit) {
				t.Fatal(e)
			}
		}
	}
}

func TestRuntimeTemplateShadowedSelfAndWholeRoot(t *testing.T) {
	for _, whole := range []bool{false, true} {
		const row = "20261022120000-row0001"
		source := `.action{.Same}`
		if whole {
			source = `.action{printf "%v" .}`
		}
		a := &av.AttributeView{ID: "A", KeyValues: []*av.KeyValues{
			{Key: &av.Key{ID: "p", Name: "Name", Type: av.KeyTypeBlock}, Values: []*av.Value{{ID: row, BlockID: row, Type: av.KeyTypeBlock, Block: &av.ValueBlock{ID: row}}}},
			{Key: &av.Key{ID: "self", Name: "Same", Type: av.KeyTypeTemplate, Template: source}},
			{Key: &av.Key{ID: "number", Name: "Same", Type: av.KeyTypeNumber}, Values: []*av.Value{{ID: row + "n", BlockID: row, Type: av.KeyTypeNumber, Number: &av.ValueNumber{Content: 4, IsNotEmpty: true}}}},
		}}
		now := func() time.Time { return time.Unix(0, 0) }
		r := &FieldResolver{Context: context.Background(), MaxNodes: 1000, MaxDepth: 100, Read: func(string) (*av.AttributeView, error) { return nil, ErrShape }, Prepare: func(*av.AttributeView, av.Collection) error { return nil }, Templates: TemplateEnvironment{Now: now, Funcs: BuiltInTemplateFuncs(now, nil, nil)}}
		v, e := r.Resolve(a, "self", row)
		if whole {
			if v != nil || !errors.Is(e, ErrFieldCycle) {
				t.Fatal("whole root leaked shadowed formula", v, e)
			}
		} else if e != nil || v.Template.Content != "4" {
			t.Fatal(v, e)
		}
	}
}

func TestLazyTemplateIndexOverrideReceivesResolvedMap(t *testing.T) {
	now := func() time.Time { return time.Unix(0, 0) }
	kvs := []*av.KeyValues{{Key: &av.Key{ID: "t", Name: "Title", Type: av.KeyTypeTemplate}, Values: []*av.Value{{Type: av.KeyTypeTemplate, Template: &av.ValueTemplate{Content: "not-resolved"}}}}}
	funcs := BuiltInTemplateFuncs(now, nil, nil)
	funcs["index"] = func(m map[string]any, k string) any { return m[k] }
	env := TemplateEnvironment{Now: now, Funcs: funcs, Resolve: func(string) error { kvs[0].Values[0].Template.Content = "resolved"; return nil }}
	got, e := RenderTemplateField(nil, kvs, `.action{index .id_mod "t"}`, env)
	if e != nil || got != "resolved" {
		t.Fatal(got, e)
	}
}
