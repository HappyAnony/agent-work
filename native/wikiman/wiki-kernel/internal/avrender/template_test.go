package avrender

import (
	"context"
	"errors"
	"strings"
	"testing"
	"text/template"
	"time"
	av "wiki-kernel/internal/avmodel"
)

func TestOriginalTemplateModelFunctionsOrderAndFailures(t *testing.T) {
	now := time.Date(2026, 10, 22, 12, 0, 0, 0, time.Local)
	env := TemplateEnvironment{Now: func() time.Time { return now }, Name: func(a *av.AttributeView) string { return a.Name }, Funcs: BuiltInTemplateFuncs(func() time.Time { return now }, nil, template.FuncMap{})}
	kv := []*av.KeyValues{{Key: &av.Key{ID: "n", Name: "Number", Type: av.KeyTypeNumber}, Values: []*av.Value{{Type: av.KeyTypeNumber, Number: &av.ValueNumber{Content: 3, IsNotEmpty: true}}}}, {Key: &av.Key{ID: "t", Name: "Title", Type: av.KeyTypeText}, Values: []*av.Value{{Type: av.KeyTypeText, Text: &av.ValueText{Content: "hello"}}}}}
	ial := map[string]string{"id": "short", "updated": "invalid"}
	for _, tc := range []struct{ tpl, want string }{
		{`.action{.Number}/.action{index .id_mod "t"}`, "3/hello"},
		{`.action{pow 2 3}/.action{runeCount "你好"}`, "8/2"},
		{`.action{.missing}`, ""},
		{`<b>.action{.Title}`, "&lt;b&gt;hello"},
		{`.action{now | date "2006-01-02"}`, "2026-10-22"},
	} {
		s, e := RenderTemplateField(ial, kv, tc.tpl, env)
		if e != nil || s != tc.want {
			t.Fatal(tc.tpl, s, e)
		}
	}
	if len(ial) != 2 || ial["id"] != "short" || ial["updated"] != "invalid" {
		t.Fatal("mutated IAL", ial)
	}
	a := &av.AttributeView{KeyValues: []*av.KeyValues{{Key: &av.Key{ID: "second", Name: "Second", Type: av.KeyTypeTemplate, Template: `.action{.First}`}}, {Key: &av.Key{ID: "first", Name: "First", Type: av.KeyTypeTemplate, Template: "first"}}}}
	keys, resolved := GetTemplateKeysByResolutionOrder(a, env)
	if !resolved || len(keys) != 2 || keys[0].ID != "first" || keys[1].ID != "second" {
		t.Fatal("duplicate/out-of-order", keys, resolved)
	}
	a.KeyValues[1].Key.Template = `.action{.Second}`
	_, resolved = GetTemplateKeysByResolutionOrder(a, env)
	if resolved {
		t.Fatal("cycle resolved")
	}
	env.MaxOutput = 2
	if _, e := RenderTemplateField(ial, kv, "long", env); !errors.Is(e, ErrLimit) {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	env.MaxOutput = 0
	env.Context = ctx
	if _, e := RenderTemplateField(ial, kv, "x", env); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	env.Context = nil
	if _, e := RenderTemplateField(ial, kv, `.action{querySQL "select 1"}`, env); !errors.Is(e, ErrEnvironmentUnbound) {
		t.Fatal(e)
	}
	if _, e := RenderTemplateField(ial, kv, `.action{env "HOME"}`, env); e == nil || !strings.Contains(e.Error(), "not defined") {
		t.Fatal("environment exposed", e)
	}
}
