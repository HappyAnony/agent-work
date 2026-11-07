package avrender

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"
	av "wiki-kernel/internal/avmodel"
)

func mutationResolver(source string) *FieldResolver {
	now := func() time.Time { return time.Unix(0, 0) }
	return &FieldResolver{Context: context.Background(), MaxNodes: 10000, MaxDepth: 100, Read: func(string) (*av.AttributeView, error) { return nil, ErrShape }, Prepare: func(*av.AttributeView, av.Collection) error { return nil }, Templates: TemplateEnvironment{Now: now, Funcs: BuiltInTemplateFuncs(now, nil, nil)}}
}

func mutationFixture(source string) *av.AttributeView {
	const row = "20261023090000-row0001"
	return &av.AttributeView{ID: "A", KeyValues: []*av.KeyValues{
		{Key: &av.Key{ID: "primary", Name: "Name", Type: av.KeyTypeBlock}, Values: []*av.Value{{ID: row, BlockID: row, Type: av.KeyTypeBlock, Block: &av.ValueBlock{ID: row}}}},
		{Key: &av.Key{ID: "self", Name: "Self", Type: av.KeyTypeTemplate, Template: source}},
		{Key: &av.Key{ID: "leaf", Name: "Leaf", Type: av.KeyTypeTemplate, Template: "leaf"}},
		{Key: &av.Key{ID: "bad", Name: "Bad", Type: av.KeyTypeTemplate, Template: `.action{.Bad}`}},
	}}
}

func TestRuntimeMapMutatorsOverlayRefreshAndAliases(t *testing.T) {
	for _, tc := range []struct{ name, source, want string }{
		{"set-self", `.action{$_ := set .id_mod "self" "local"}.action{get .id_mod "self"}`, "local"},
		{"unset-self", `.action{$_ := unset .id_mod "self"}.action{hasKey .id_mod "self"}`, "false"},
		{"refresh-set", `.action{$m := .id_mod}.action{$_ := set $m "self" "local"}.action{.Leaf}/.action{get $m "self"}`, "leaf/local"},
		{"refresh-unset", `.action{$m := .id_mod}.action{$_ := unset $m "self"}.action{.Leaf}/.action{hasKey $m "self"}`, "leaf/false"},
		{"missing-tombstone", `.action{$_ := unset .id_mod "never"}.action{.Leaf}/.action{hasKey .id_mod "never"}`, "leaf/false"},
		{"set-root-name", `.action{$_ := set . "Self" "local"}.action{.Leaf}/.action{.Self}`, "leaf/local"},
		{"independent-name-id", `.action{$_ := set .id_mod "leaf" "local"}.action{.Leaf}/.action{get .id_mod "leaf"}`, "leaf/local"},
		{"namespace-replaced", `.action{$old := .id_mod}.action{$_ := set . "id_mod" (dict "leaf" "local")}.action{get $old "leaf"}/.action{get .id_mod "leaf"}`, "leaf/local"},
		{"namespace-deleted", `.action{$old := .id_mod}.action{$_ := unset . "id_mod"}.action{get $old "leaf"}/.action{hasKey . "id_mod"}`, "leaf/false"},
		{"nested-alias", `.action{$box := set (dict) "nested" .id_mod}.action{$_ := set (get $box "nested") "self" "local"}.action{.Leaf}/.action{dig "nested" "self" "missing" $box}`, "leaf/local"},
		{"overwrite-self", `.action{$_ := mergeOverwrite .id_mod (dict "self" "local")}.action{.Leaf}/.action{get .id_mod "self"}`, "leaf/local"},
		{"must-overwrite-self", `.action{$_ := mustMergeOverwrite .id_mod (dict "self" "local")}.action{.Leaf}/.action{get .id_mod "self"}`, "leaf/local"},
		{"merge-new", `.action{$_ := merge .id_mod (dict "new" "local")}.action{.Leaf}/.action{get .id_mod "new"}`, "leaf/local"},
		{"must-merge-new", `.action{$_ := mustMerge .id_mod (dict "new" "local")}.action{.Leaf}/.action{get .id_mod "new"}`, "leaf/local"},
		{"merge-deleted", `.action{$_ := unset .id_mod "self"}.action{$_ := merge .id_mod (dict "self" "new")}.action{.Leaf}/.action{get .id_mod "self"}`, "leaf/new"},
		{"source-shadowed", `.action{$d := dict "primary" "p" "self" "s" "leaf" "l" "bad" "b"}.action{$_ := merge $d .id_mod}.action{get $d "self"}`, "s"},
		{"raw-source-shadowed", `.action{$d := dict}.action{range keys .}.action{$_ := set $d . "kept"}.action{end}.action{$_ := merge $d $}.action{get $d "Self_raw"}`, "kept"},
		{"merge-nested-managed", `.action{$d := pick . "id_mod"}.action{$_ := mergeOverwrite $d (dict "id_mod" (dict "self" "local"))}.action{.Leaf}/.action{get .id_mod "self"}`, "leaf/local"},
		{"zero-overwrite", `.action{$_ := mergeOverwrite .id_mod (dict "self" 0)}.action{.Leaf}/.action{get .id_mod "self"}`, "leaf/0"},
		{"nil-overwrite", `.action{$_ := mergeOverwrite .id_mod (dict "self" nil)}.action{.Leaf}/.action{hasKey .id_mod "self"}/.action{get .id_mod "self" | default "nil"}`, "leaf/true/nil"},
		{"same-map", `.action{$m := .id_mod}.action{$_ := merge $m $m}.action{get $m "leaf"}`, "leaf"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := mutationResolver(tc.source)
			a := mutationFixture(tc.source)
			before := fmt.Sprintf("%#v", a.KeyValues[1].Key)
			v, e := r.Resolve(a, "self", "20261023090000-row0001")
			if e != nil || v.Template.Content != tc.want {
				t.Fatal(v, e, tc.want)
			}
			if r.states[FieldRef{"A", "bad", "20261023090000-row0001"}] != 0 {
				t.Fatal("unselected field evaluated")
			}
			if len(a.KeyValues[1].Values) != 0 || fmt.Sprintf("%#v", a.KeyValues[1].Key) != before {
				t.Fatal("source mutated")
			}
		})
	}
}

func TestMapMutatorsConditionalNameFallbackSkipsUnusedTemplate(t *testing.T) {
	source := `.action{$d := dict "primary" "p" "self" (dict "keep" "x") "leaf" "l" "bad" "b" "empty" (dict "keep" "y")}.action{$_ := merge $d .id_mod}.action{dig "empty" "keep" "fallback" $d}`
	a := mutationFixture(source)
	a.KeyValues[1].Key.Name = "Same"
	a.KeyValues = append(a.KeyValues, &av.KeyValues{Key: &av.Key{ID: "empty", Name: "Same", Type: av.KeyTypeNumber}})
	r := mutationResolver(source)
	v, e := r.Resolve(a, "self", "20261023090000-row0001")
	if e != nil || v.Template.Content != "y" {
		t.Fatal(v, e)
	}
}

func TestMapMutatorsOriginalMergeParity(t *testing.T) {
	now := func() time.Time { return time.Unix(0, 0) }
	for _, op := range []string{"merge", "mustMerge", "mergeOverwrite", "mustMergeOverwrite"} {
		for _, body := range []string{
			`(dict "x" 1 "blank" "" "nil" nil) (dict "x" 2 "blank" "fill" "nil" 3) (dict "x" 4)`,
			`(dict "nested" (dict "x" 1)) (dict "nested" (dict "y" 2))`,
			`(dict "list" (list 1 2)) (dict "list" (list 3))`,
			`(dict "list" (list 1 2)) (dict "list" (list))`,
			`(dict "x" 1) (dict "x" 0)`,
			`(dict "x" 1) (dict "x" nil)`,
			`(dict)`,
			`(nilMap) (dict "x" 1) (dict "y" 2)`,
			`(dict) (nilMap)`,
			`(dict "x" (dict "a" 1)) (dict "x" "string")`,
			`(dict "x" 1) (dict "x" (dict "a" 2))`,
		} {
			source := `.action{` + op + ` ` + body + ` | toJson}`
			env := TemplateEnvironment{Now: now, Funcs: BuiltInTemplateFuncs(now, nil, nil)}
			env.Funcs["nilMap"] = func() map[string]any { return nil }
			want, we := RenderTemplateField(nil, nil, source, env)
			env.Resolve = func(string) error { return nil }
			got, e := RenderTemplateField(nil, nil, source, env)
			if (e == nil) != (we == nil) || got != want {
				t.Fatal(source, got, want, e, we)
			}
		}
	}
	for _, source := range []string{`.action{set nil "x" 1}`, `.action{unset nil "x" | toJson}`, `.action{set (dict) "x" nil | toJson}`, `.action{unset (dict "x" 1) "x" | toJson}`, `.action{set (dict) 1 2}`, `.action{unset (dict) 1}`, `.action{merge 1 (dict)}`, `.action{mustMerge (dict) 1}`, `.action{mergeOverwrite (dict) "bad"}`, `.action{mustMergeOverwrite}`} {
		env := TemplateEnvironment{Now: now, Funcs: BuiltInTemplateFuncs(now, nil, nil)}
		want, we := RenderTemplateField(nil, nil, source, env)
		env.Resolve = func(string) error { return nil }
		got, e := RenderTemplateField(nil, nil, source, env)
		if (e == nil) != (we == nil) || got != want {
			t.Fatal(source, got, want, e, we)
		}
	}
}

func TestMapMutatorsRawProjectionPreservesFlagsAndNilShape(t *testing.T) {
	now := func() time.Time { return time.Unix(0, 0) }
	kvs := []*av.KeyValues{{Key: &av.Key{ID: "s", Name: "Choices", Type: av.KeyTypeMSelect}, Values: []*av.Value{{Type: av.KeyTypeMSelect, MSelect: []*av.ValueSelect{}, IsRenderAutoFill: true}}}}
	funcs := BuiltInTemplateFuncs(now, nil, nil)
	funcs["sameRaw"] = func(a, b *av.Value) bool { return a == b }
	source := `.action{.Choices_raw.IsRenderAutoFill}/.action{eq .Choices_raw.MSelect nil}/.action{sameRaw .Choices_raw (get .id_mod_raw "s")}`
	env := TemplateEnvironment{Now: now, Funcs: funcs}
	want, e := RenderTemplateField(nil, kvs, source, env)
	if e != nil {
		t.Fatal(e)
	}
	env.Resolve = func(string) error { return nil }
	got, e := RenderTemplateField(nil, kvs, source, env)
	if e != nil || got != want || got != "true/false/true" {
		t.Fatal(got, want, e)
	}
}

func TestMapMutatorsEqualStoredValueOverwriteIsNotLost(t *testing.T) {
	const row = "20261023090000-row0001"
	for _, op := range []string{"mergeOverwrite", "mustMergeOverwrite"} {
		source := `.action{$_ := ` + op + ` .id_mod (dict "self" "stored")}.action{.Leaf}/.action{get .id_mod "self"}`
		a := mutationFixture(source)
		a.KeyValues[1].Values = []*av.Value{{ID: row + "s", BlockID: row, KeyID: "self", Type: av.KeyTypeTemplate, Template: &av.ValueTemplate{Content: "stored"}}}
		r := mutationResolver(source)
		v, e := r.Resolve(a, "self", row)
		if e != nil || v.Template.Content != "leaf/stored" {
			t.Fatal(v, e)
		}
		if a.KeyValues[1].Values[0].Template.Content != "stored" {
			t.Fatal("stored source changed")
		}
		// Direct eager-model input makes the pre-write value exactly identical.
		// A diff-only ledger would lose this write and request the self dependency.
		kvs := []*av.KeyValues{
			{Key: &av.Key{ID: "self", Name: "Self", Type: av.KeyTypeTemplate}, Values: []*av.Value{{Type: av.KeyTypeTemplate, Template: &av.ValueTemplate{Content: "stored"}}}},
			{Key: &av.Key{ID: "leaf", Name: "Leaf", Type: av.KeyTypeTemplate}, Values: []*av.Value{{Type: av.KeyTypeTemplate, Template: &av.ValueTemplate{Content: "leaf"}}}},
		}
		now := func() time.Time { return time.Unix(0, 0) }
		env := TemplateEnvironment{Now: now, Funcs: BuiltInTemplateFuncs(now, nil, nil), Resolve: func(id string) error {
			if id == "self" {
				return ErrFieldCycle
			}
			return nil
		}}
		if got, e := RenderTemplateField(nil, kvs, source, env); e != nil || got != "leaf/stored" {
			t.Fatal(got, e)
		}
	}
}

func TestMapMutatorsSameSignatureOverridesRemainOpaque(t *testing.T) {
	now := func() time.Time { return time.Unix(0, 0) }
	for _, name := range []string{"set", "unset", "merge", "mustMerge", "mergeOverwrite", "mustMergeOverwrite"} {
		calls := 0
		funcs := BuiltInTemplateFuncs(now, nil, nil)
		source := ""
		var custom any
		switch name {
		case "set":
			custom = func(m map[string]any, _ string, _ any) map[string]any {
				calls++
				return map[string]any{"result": m["t"]}
			}
			source = `.action{get (set .id_mod "x" 1) "result"}`
		case "unset":
			custom = func(m map[string]any, _ string) map[string]any { calls++; return map[string]any{"result": m["t"]} }
			source = `.action{get (unset .id_mod "x") "result"}`
		case "merge", "mergeOverwrite":
			custom = func(m map[string]any, _ ...map[string]any) any { calls++; return map[string]any{"result": m["t"]} }
			source = `.action{get (` + name + ` .id_mod) "result"}`
		default:
			custom = func(m map[string]any, _ ...map[string]any) (any, error) {
				calls++
				return map[string]any{"result": m["t"]}, nil
			}
			source = `.action{get (` + name + ` .id_mod) "result"}`
		}
		funcs[name] = custom
		kvs := []*av.KeyValues{{Key: &av.Key{ID: "t", Name: "Title", Type: av.KeyTypeTemplate}, Values: []*av.Value{{Type: av.KeyTypeTemplate, Template: &av.ValueTemplate{Content: "pending"}}}}}
		env := TemplateEnvironment{Now: now, Funcs: funcs, Resolve: func(string) error { kvs[0].Values[0].Template.Content = "resolved"; return nil }}
		got, e := RenderTemplateField(nil, kvs, source, env)
		if e != nil || got != "resolved" || calls != 1 || reflect.ValueOf(funcs[name]).Pointer() != reflect.ValueOf(custom).Pointer() {
			t.Fatal(name, got, calls, e)
		}
	}
}

func TestMapMutatorsRawValuesArePrivateAndAliasesRemainStable(t *testing.T) {
	const row = "20261023090000-row0001"
	source := `.action{$n := .Number_raw}.action{$_ := mergeOverwrite .id_mod_raw (dict "number" .Other_raw)}.action{.Leaf}/.action{$n.Number.Content}/.action{.Number_raw.Number.Content}/.action{.Number}/.action{sameRaw $n (get .id_mod_raw "number")}`
	a := mutationFixture(source)
	for _, f := range []struct {
		id, name string
		number   float64
	}{{"number", "Number", 3}, {"other", "Other", 9}} {
		a.KeyValues = append(a.KeyValues, &av.KeyValues{Key: &av.Key{ID: f.id, Name: f.name, Type: av.KeyTypeNumber}, Values: []*av.Value{{ID: f.id, BlockID: row, Type: av.KeyTypeNumber, Number: &av.ValueNumber{Content: f.number, IsNotEmpty: true}}}})
	}
	a.KeyValues = append(a.KeyValues, &av.KeyValues{Key: &av.Key{ID: "observer", Name: "Observer", Type: av.KeyTypeTemplate, Template: `.action{.Number_raw.Number.Content}`}})
	r := mutationResolver(source)
	r.Templates.Funcs["sameRaw"] = func(a, b *av.Value) bool { return a == b }
	v, e := r.Resolve(a, "self", row)
	// Original Mergo mutates the old pointer, then rebinds the map entry to
	// the source pointer. Captured aliases must not be forcibly unified.
	if e != nil || v.Template.Content != "leaf/9/9/3/false" {
		t.Fatal(v, e)
	}
	n, e := r.Resolve(a, "number", row)
	if e != nil || n.Number.Content != 3 || a.KeyValues[4].Values[0].Number.Content != 3 {
		t.Fatal("shared cache/source raw mutated", n, e)
	}
	// Another field in the same operation must not observe the first's raw edits.
	v, e = r.Resolve(a, "observer", row)
	if e != nil || v.Template.Content != "3" {
		t.Fatal(v, e)
	}
}

func TestMapMutatorsFailuresLimitsAndFunctionOverrides(t *testing.T) {
	for _, source := range []string{`.action{$_ := merge .id_mod (dict "self" "x")}`, `.action{$_ := mergeOverwrite (dict) .id_mod}`, `.action{$_ := set .id_mod "self" .id_mod}.action{printf "%v" .id_mod}`} {
		r := mutationResolver(source)
		v, e := r.Resolve(mutationFixture(source), "self", "20261023090000-row0001")
		if v != nil || e == nil || (!errors.Is(e, ErrFieldCycle) && !errors.Is(e, ErrLimit)) {
			t.Fatal(v, e)
		}
	}
	now := func() time.Time { return time.Unix(0, 0) }
	for _, name := range []string{"set", "unset", "merge", "mustMerge", "mergeOverwrite", "mustMergeOverwrite"} {
		funcs := BuiltInTemplateFuncs(now, nil, nil)
		calls := 0
		custom := func(...any) any { calls++; return "custom" }
		funcs[name] = custom
		env := TemplateEnvironment{Now: now, Funcs: funcs, Resolve: func(string) error { return nil }}
		got, e := RenderTemplateField(nil, nil, `.action{`+name+` .id_mod}`, env)
		if e != nil || got != "custom" || calls != 1 || reflect.ValueOf(funcs[name]).Pointer() != reflect.ValueOf(custom).Pointer() {
			t.Fatal(name, got, e)
		}
	}
	for _, source := range []string{`.action{set .id_mod "x" 1}`, `.action{unset .id_mod "x"}`, `.action{merge .id_mod (dict "x" 1)}`} {
		ctx, cancel := context.WithCancel(context.Background())
		env := TemplateEnvironment{Now: now, Funcs: BuiltInTemplateFuncs(now, nil, nil), Context: ctx, Resolve: func(string) error { return nil }}
		env.Funcs["cancel"] = func() string { cancel(); return "" }
		_, e := RenderTemplateField(nil, nil, `.action{cancel}`+source, env)
		if !errors.Is(e, context.Canceled) {
			t.Fatal(source, e)
		}
		env.Context = context.Background()
		env.Charge = func(int) error { return ErrLimit }
		if _, e = RenderTemplateField(nil, nil, source, env); !errors.Is(e, ErrLimit) {
			t.Fatal(source, e)
		}
	}
	// Cyclic merge inputs must fail before entering Mergo, not hang in recursion.
	source := `.action{$m := dict}.action{$_ := set $m "self" $m}.action{merge $m (dict "x" 1)}`
	env := TemplateEnvironment{Now: now, Funcs: BuiltInTemplateFuncs(now, nil, nil), Resolve: func(string) error { return nil }}
	if _, e := RenderTemplateField(nil, nil, source, env); !errors.Is(e, ErrLimit) {
		t.Fatal(e)
	}
}
