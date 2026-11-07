package avrender

import (
	"errors"
	"reflect"
	"testing"
	"time"
	av "wiki-kernel/internal/avmodel"
)

func TestFieldDependenciesNamesIDsRawControlScopesAndDynamicErrors(t *testing.T) {
	a := &av.AttributeView{KeyValues: []*av.KeyValues{{Key: &av.Key{ID: "first", Name: "First"}}, {Key: &av.Key{ID: "second", Name: "Second"}}}}
	env := TemplateEnvironment{Funcs: BuiltInTemplateFuncs(time.Now, nil, nil)}
	for _, tc := range []struct {
		expr string
		ids  []string
	}{
		{`.action{.First}/.action{.Second_raw.Text.Content}`, []string{"first", "second"}},
		{`.action{index .id_mod "first"}/.action{get .id_mod_raw "second"}`, []string{"first", "second"}},
		{`.action{if .First}.action{.Second}.action{else}.action{.First}.action{end}`, []string{"first", "second"}},
		{`.action{range .First}.action{.}.action{$.Second}.action{end}`, []string{"first", "second"}},
		{`.action{define "child"}.action{.Second}.action{end}.action{template "child" .}`, []string{"second"}},
		{`.action{with .First}.action{.Content}.action{$.Second}.action{end}`, []string{"first", "second"}},
		{`.action{range .First}.action{index . "Local"}.action{else}.action{.Second}.action{end}`, []string{"first", "second"}},
		{`.action{with .First}.action{index $.id_mod "second"}.action{else}.action{.Second}.action{end}`, []string{"first", "second"}},
	} {
		deps, e := FieldDependencies(a, tc.expr, env)
		if e != nil {
			t.Fatal(tc.expr, e)
		}
		ids := []string{}
		for _, d := range deps {
			ids = append(ids, d.ID)
		}
		if !reflect.DeepEqual(ids, tc.ids) {
			t.Fatal(tc.expr, ids, tc.ids)
		}
	}
	for _, s := range []string{`.action{$x := "first"}.action{index .id_mod $x}`, `.action{.id_mod}`, `.action{.}`} {
		if _, e := FieldDependencies(a, s, env); !errors.Is(e, ErrDynamicDependency) {
			t.Fatal(s, e)
		}
	}
}
