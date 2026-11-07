package avrender

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
	av "wiki-kernel/internal/avmodel"
)

func TestRuntimeMapReadersSelectedMembersAndNestedResults(t *testing.T) {
	for _, tc := range []struct {
		name, source, want string
		cycle              bool
	}{
		{"dig", `.action{dig "leaf" "fallback" .id_mod}`, "ok", false},
		{"dig-nested-root", `.action{dig "id_mod" "leaf" "fallback" .}`, "ok", false},
		{"dig-missing", `.action{dig "missing" "bad" "fallback" .id_mod}`, "fallback", false},
		{"dig-unused-default", `.action{dig "leaf" .id_mod .id_mod}`, "ok", false},
		{"pluck-order", `.action{pluck "leaf" .id_mod .id_mod | join ","}`, "ok,ok", false},
		{"pluck-distinct-order", `.action{pluck "leaf" (dict "leaf" "first") .id_mod | join ","}`, "first,ok", false},
		{"pluck-empty-not-missing", `.action{pluck "leaf" (dict "leaf" "") .id_mod | join ","}`, ",ok", false},
		{"pluck-missing", `.action{pluck "absent" .id_mod | len}`, "0", false},
		{"pick-get", `.action{get (pick .id_mod "leaf" "absent") "leaf"}`, "ok", false},
		{"pick-raw", `.action{(index (pick .id_mod_raw "leaf") "leaf").Template.Content}`, "ok", false},
		{"omit", `.action{get (omit .id_mod "self" "bad") "leaf"}`, "ok", false},
		{"keys", `.action{keys .id_mod | sortAlpha | join ","}`, "bad,leaf,primary,self", false},
		{"root-keys", `.action{keys . | has "Self"}`, "true", false},
		{"has-key-no-value", `.action{hasKey . "Bad"}`, "true", false},
		{"values-subset", `.action{values (pick .id_mod "leaf") | join ","}`, "ok", false},
		{"keys-duplicate-maps", `.action{keys .id_mod .id_mod | len}`, "8", false},
		{"nested-pluck-select", `.action{index (pluck "id_mod" .) 0 "leaf"}`, "ok", false},
		{"nested-pick-select", `.action{dig "id_mod" "leaf" "fallback" (pick . "id_mod")}`, "ok", false},
		{"named-scope", `.action{define "x"}.action{dig "leaf" "fallback" .}.action{end}.action{template "x" .id_mod}`, "ok", false},
		{"nested-pluck-output", `.action{printf "%v" (pluck "id_mod" .)}`, "", true},
		{"nested-pick-output", `.action{printf "%v" (pick . "id_mod")}`, "", true},
		{"dig-selected-cycle", `.action{dig "self" "fallback" .id_mod}`, "", true},
		{"pluck-selected-cycle", `.action{pluck "self" .id_mod}`, "", true},
		{"pick-selected-cycle", `.action{pick .id_mod "self"}`, "", true},
		{"omit-retained-cycle", `.action{omit .id_mod "bad"}`, "", true},
		{"values-cycle", `.action{values .id_mod}`, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			const row = "20261022120000-row0001"
			a := &av.AttributeView{ID: "A", KeyValues: []*av.KeyValues{
				{Key: &av.Key{ID: "primary", Name: "Name", Type: av.KeyTypeBlock}, Values: []*av.Value{{ID: row, BlockID: row, Type: av.KeyTypeBlock, Block: &av.ValueBlock{ID: row}}}},
				{Key: &av.Key{ID: "self", Name: "Self", Type: av.KeyTypeTemplate, Template: tc.source}},
				{Key: &av.Key{ID: "leaf", Name: "Leaf", Type: av.KeyTypeTemplate, Template: "ok"}},
				{Key: &av.Key{ID: "bad", Name: "Bad", Type: av.KeyTypeTemplate, Template: `.action{.Bad}`}},
			}}
			now := func() time.Time { return time.Unix(0, 0) }
			r := &FieldResolver{Context: context.Background(), MaxNodes: 5000, MaxDepth: 100, Read: func(string) (*av.AttributeView, error) { return nil, ErrShape }, Prepare: func(*av.AttributeView, av.Collection) error { return nil }, Templates: TemplateEnvironment{Now: now, Funcs: BuiltInTemplateFuncs(now, nil, nil)}}
			v, e := r.Resolve(a, "self", row)
			if tc.cycle {
				var cycle *FieldCycleError
				if v != nil || !errors.As(e, &cycle) || len(cycle.Path) < 2 {
					t.Fatal(v, e)
				}
			} else if e != nil || v.Template.Content != tc.want {
				t.Fatal(v, e, tc.want)
			}
			if !tc.cycle && r.states[FieldRef{"A", "bad", row}] != 0 {
				t.Fatal("unselected bad field resolved")
			}
			if len(a.KeyValues[1].Values) != 0 || a.KeyValues[1].Key.Template != tc.source {
				t.Fatal("source mutated")
			}
		})
	}
}

func TestMapReadersMatchOriginalTypesDefaultsAndErrors(t *testing.T) {
	now := func() time.Time { return time.Unix(0, 0) }
	kvs := []*av.KeyValues{
		{Key: &av.Key{ID: "n", Name: "Number", Type: av.KeyTypeNumber}, Values: []*av.Value{{Type: av.KeyTypeNumber, Number: &av.ValueNumber{Content: 3, IsNotEmpty: true}}}},
		{Key: &av.Key{ID: "t", Name: "Title", Type: av.KeyTypeTemplate}, Values: []*av.Value{{Type: av.KeyTypeTemplate, Template: &av.ValueTemplate{Content: "resolved"}}}},
	}
	for _, source := range []string{
		`.action{printf "%T:%v" (dig "n" nil .id_mod) (dig "n" nil .id_mod)}`,
		`.action{printf "%T" (pick .id_mod "n")}/.action{printf "%T" (pluck "n" .id_mod)}/.action{printf "%T" (keys .)}/.action{printf "%T" (values .id_mod)}`,
		`.action{dig "absent" nil .id_mod | default "fallback"}`,
		`.action{keys .id_mod .id_mod | sortAlpha | join ","}`,
		`.action{values .id_mod | toStrings | sortAlpha | join ","}`,
		`.action{omit .id_mod "n" | keys | sortAlpha | join ","}`,
		`.action{dig "n" "x" "fallback" .id_mod}`,
		`.action{dig 1 "fallback" .id_mod}`,
		`.action{dig "n"}`,
		`.action{pick .id_mod 1}`,
		`.action{pluck "n" "wrong"}`,
		`.action{keys nil}/.action{values nil}`,
	} {
		env := TemplateEnvironment{Now: now, Funcs: BuiltInTemplateFuncs(now, nil, nil)}
		want, we := RenderTemplateField(nil, kvs, source, env)
		env.Resolve = func(string) error { return nil }
		got, e := RenderTemplateField(nil, kvs, source, env)
		if (e == nil) != (we == nil) || got != want {
			t.Fatal(source, got, want, e, we)
		}
		if e != nil { // Error context gains private hooks, but preserve upstream reason.
			for _, marker := range []string{"dig needs at least three arguments", "interface conversion", "expected string", "expected map[string]interface {}"} {
				if strings.Contains(we.Error(), marker) && !strings.Contains(e.Error(), marker) {
					t.Fatal(e, we)
				}
			}
		}
	}
}

func TestMapReadersOverridesAreOpaqueAndCallerMapIsUnchanged(t *testing.T) {
	now := func() time.Time { return time.Unix(0, 0) }
	for _, name := range []string{"get", "hasKey", "empty", "default", "dig", "pluck", "pick", "omit", "keys", "values"} {
		funcs := BuiltInTemplateFuncs(now, nil, nil)
		calls := 0
		custom := func(...any) any { calls++; return "custom" }
		funcs[name] = custom
		source := `.action{` + name + ` .id_mod}`
		env := TemplateEnvironment{Now: now, Funcs: funcs, Resolve: func(string) error { return nil }}
		got, e := RenderTemplateField(nil, nil, source, env)
		if e != nil || got != "custom" || calls != 1 {
			t.Fatal(name, got, calls, e)
		}
		if reflect.ValueOf(funcs[name]).Pointer() != reflect.ValueOf(custom).Pointer() {
			t.Fatal("caller override changed")
		}
	}
}

func TestMapReadersOptionalMembershipAndExcludedFields(t *testing.T) {
	now := func() time.Time { return time.Unix(0, 0) }
	for _, tc := range []struct {
		source, want string
		resolve      bool
	}{
		{`.action{keys . | has "Related"}`, "true", true},
		{`.action{hasKey . "Related"}`, "true", true},
		{`.action{hasKey . "Self"}`, "true", false},
		{`.action{omit . "Related" "Related_raw" "Total" "Total_raw" "Self" "Self_raw" "id_mod" "id_mod_raw" | keys | len}`, "0", false},
	} {
		kvs := []*av.KeyValues{
			{Key: &av.Key{ID: "t", Name: "Self", Type: av.KeyTypeTemplate}, Values: []*av.Value{{Type: av.KeyTypeTemplate, Template: &av.ValueTemplate{Content: "unresolved"}}}},
			{Key: &av.Key{ID: "r", Name: "Related", Type: av.KeyTypeRelation}, Values: []*av.Value{{Type: av.KeyTypeRelation, Relation: &av.ValueRelation{}}}},
			{Key: &av.Key{ID: "s", Name: "Total", Type: av.KeyTypeRollup}, Values: []*av.Value{{Type: av.KeyTypeRollup, Rollup: &av.ValueRollup{}}}},
		}
		visits := map[string]int{}
		env := TemplateEnvironment{Now: now, Funcs: BuiltInTemplateFuncs(now, nil, nil), Resolve: func(id string) error {
			visits[id]++
			if id == "t" {
				return ErrFieldCycle
			}
			if id == "r" {
				kvs[1].Values[0].Relation.Contents = []*av.Value{{Type: av.KeyTypeText, Text: &av.ValueText{Content: "related"}}}
			}
			if id == "s" {
				kvs[2].Values[0].Rollup.Contents = []*av.Value{{Type: av.KeyTypeNumber, Number: &av.ValueNumber{Content: 3}}}
			}
			return nil
		}}
		got, e := RenderTemplateField(nil, kvs, tc.source, env)
		if e != nil || got != tc.want || visits["t"] != 0 || (visits["r"] > 0) != tc.resolve {
			t.Fatal(tc.source, got, e, visits)
		}
	}
	// An absent optional later writer must not force evaluation of the earlier
	// template value just to establish presence of an already defined name.
	kvs := []*av.KeyValues{
		{Key: &av.Key{ID: "t", Name: "Same", Type: av.KeyTypeTemplate}, Values: []*av.Value{{Type: av.KeyTypeTemplate, Template: &av.ValueTemplate{}}}},
		{Key: &av.Key{ID: "r", Name: "Same", Type: av.KeyTypeRelation}, Values: []*av.Value{{Type: av.KeyTypeRelation, Relation: &av.ValueRelation{}}}},
	}
	env := TemplateEnvironment{Now: now, Funcs: BuiltInTemplateFuncs(now, nil, nil), Resolve: func(id string) error {
		if id == "t" {
			return ErrFieldCycle
		}
		return nil
	}}
	if got, e := RenderTemplateField(nil, kvs, `.action{hasKey . "Same"}`, env); e != nil || got != "true" {
		t.Fatal(got, e)
	}
}

func TestMapReadersSameSignatureOverridesReceiveMaterializedInputs(t *testing.T) {
	now := func() time.Time { return time.Unix(0, 0) }
	for _, tc := range []struct {
		name, source, want string
		fn                 any
	}{
		{"get", `.action{get .id_mod "t"}`, "resolved", func(m map[string]any, k string) any { return m[k] }},
		{"hasKey", `.action{hasKey .id_mod "ignored"}`, "true", func(m map[string]any, _ string) bool { return m["t"] == "resolved" }},
		{"empty", `.action{empty .id_mod}`, "false", func(v any) bool { return v.(map[string]any)["t"] != "resolved" }},
		{"default", `.action{default .id_mod}`, "resolved", func(v any, _ ...any) any { return v.(map[string]any)["t"] }},
		{"dig", `.action{dig "t" "fallback" .id_mod}`, "resolved", func(args ...any) (any, error) { return args[len(args)-1].(map[string]any)["t"], nil }},
		{"pluck", `.action{pluck "t" .id_mod | join ","}`, "resolved", func(_ string, m ...map[string]any) []any { return []any{m[0]["t"]} }},
		{"pick", `.action{get (pick .id_mod "t") "t"}`, "resolved", func(m map[string]any, _ ...string) map[string]any { return m }},
		{"omit", `.action{get (omit .id_mod "ignored") "t"}`, "resolved", func(m map[string]any, _ ...string) map[string]any { return m }},
		{"keys", `.action{keys .id_mod | join ","}`, "resolved", func(m ...map[string]any) []string { return []string{m[0]["t"].(string)} }},
		{"values", `.action{values .id_mod | join ","}`, "resolved", func(m map[string]any) []any { return []any{m["t"]} }},
	} {
		kvs := []*av.KeyValues{{Key: &av.Key{ID: "t", Name: "Title", Type: av.KeyTypeTemplate}, Values: []*av.Value{{Type: av.KeyTypeTemplate, Template: &av.ValueTemplate{Content: "unresolved"}}}}}
		funcs := BuiltInTemplateFuncs(now, nil, nil)
		funcs[tc.name] = tc.fn
		env := TemplateEnvironment{Now: now, Funcs: funcs, Resolve: func(string) error { kvs[0].Values[0].Template.Content = "resolved"; return nil }}
		got, e := RenderTemplateField(nil, kvs, tc.source, env)
		if e != nil || got != tc.want {
			t.Fatal(tc.name, got, e)
		}
		if reflect.ValueOf(funcs[tc.name]).Pointer() != reflect.ValueOf(tc.fn).Pointer() {
			t.Fatal("mutated caller override")
		}
	}
}

func TestMapReadersNestedMaterializationCancellationAndBudgets(t *testing.T) {
	now := func() time.Time { return time.Unix(0, 0) }
	kvs := []*av.KeyValues{{Key: &av.Key{ID: "t", Name: "Self", Type: av.KeyTypeTemplate}, Values: []*av.Value{{Type: av.KeyTypeTemplate, Template: &av.ValueTemplate{Content: "unresolved"}}}}}
	for _, source := range []string{`.action{dig "t" "fallback" .id_mod}`, `.action{pluck "t" .id_mod}`, `.action{pick .id_mod "t"}`, `.action{omit .id_mod "absent"}`, `.action{values .id_mod}`} {
		want := errors.New("explicit dependency failure")
		env := TemplateEnvironment{Now: now, Funcs: BuiltInTemplateFuncs(now, nil, nil), Resolve: func(string) error { return want }}
		got, e := RenderTemplateField(nil, kvs, source, env)
		if got != "" || !errors.Is(e, want) {
			t.Fatal(source, got, e)
		}
		env.Resolve = func(string) error { return context.Canceled }
		if _, e = RenderTemplateField(nil, kvs, source, env); !errors.Is(e, context.Canceled) {
			t.Fatal(source, e)
		}
	}
	for _, source := range []string{`.action{keys .id_mod}`, `.action{pluck "id_mod" .}`, `.action{pick . "id_mod"}`} {
		env := TemplateEnvironment{Now: now, Funcs: BuiltInTemplateFuncs(now, nil, nil), Resolve: func(string) error { return nil }, Charge: func(int) error { return ErrLimit }}
		if _, e := RenderTemplateField(nil, kvs, source, env); !errors.Is(e, ErrLimit) {
			t.Fatal(source, e)
		}
	}
	cyclic := map[string]any{}
	cyclic["self"] = cyclic
	var nested any = "leaf"
	for i := 0; i < 258; i++ {
		nested = []any{nested}
	}
	l := &lazyTemplate{model: map[string]any{}, maps: map[uintptr]map[string][]string{}, env: TemplateEnvironment{Context: context.Background()}}
	if _, e := l.whole(cyclic); !errors.Is(e, ErrLimit) {
		t.Fatal("cyclic container allowed into formatting", e)
	}
	shared := map[string]any{"leaf": "ok"}
	if _, e := l.whole([]any{shared, shared}); e != nil {
		t.Fatal("shared alias mistaken for active cycle", e)
	}
	if _, e := l.whole(nested); !errors.Is(e, ErrLimit) {
		t.Fatal("nested depth unbounded", e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	l.env.Context = ctx
	if _, e := l.whole([]any{map[string]any{"nested": "value"}}); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	env := TemplateEnvironment{Now: now, Funcs: BuiltInTemplateFuncs(now, nil, nil), Resolve: func(string) error { return nil }}
	if _, e := RenderTemplateField(nil, nil, `.action{$m := dict}.action{$_ := set $m "self" $m}.action{printf "%v" $m}`, env); !errors.Is(e, ErrLimit) {
		t.Fatal("cyclic local map reached formatter", e)
	}
}
