package avrender

import (
	"context"
	"errors"
	"testing"
	"time"
	av "wiki-kernel/internal/avmodel"
)

func TestFieldResolverBudgetsCancellationFailuresAndHiddenCycles(t *testing.T) {
	const row = "20261019090002-par0001"
	makeFixture := func() *av.AttributeView {
		return &av.AttributeView{ID: "A", KeyValues: []*av.KeyValues{
			{Key: &av.Key{ID: "primary", Name: "Name", Type: av.KeyTypeBlock}, Values: []*av.Value{{ID: row, KeyID: "primary", BlockID: row, Type: av.KeyTypeBlock, Block: &av.ValueBlock{ID: row}}}},
			{Key: &av.Key{ID: "t1", Name: "T1", Type: av.KeyTypeTemplate, Template: `.action{index .id_mod "t2"}`}},
			{Key: &av.Key{ID: "t2", Name: "T2", Type: av.KeyTypeTemplate, Template: `.action{.T3}`}},
			{Key: &av.Key{ID: "t3", Name: "T3", Type: av.KeyTypeTemplate, Template: "leaf"}},
			{Key: &av.Key{ID: "unused", Name: "Unused", Type: av.KeyTypeTemplate, Template: `.action{.Unused}`}},
		}}
	}
	for _, failure := range []string{"", "depth", "nodes", "prepare", "cancel-before", "cancel-during", "dynamic"} {
		t.Run(failure, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			a := makeFixture()
			now := func() time.Time { return time.Unix(0, 0) }
			want := errors.New("synthetic prepare")
			r := &FieldResolver{Context: ctx, MaxNodes: 1000, MaxDepth: 100, Read: func(string) (*av.AttributeView, error) { t.Fatal("unexpected source read"); return nil, nil }, Prepare: func(*av.AttributeView, av.Collection) error { return nil }, Templates: TemplateEnvironment{Now: now, Name: func(*av.AttributeView) string { return "A" }, Funcs: BuiltInTemplateFuncs(now, nil, nil)}}
			switch failure {
			case "depth":
				r.MaxDepth = 2
			case "nodes":
				r.MaxNodes = 2
			case "prepare":
				r.Prepare = func(*av.AttributeView, av.Collection) error { return want }
			case "cancel-before":
				cancel()
			case "cancel-during":
				r.Prepare = func(*av.AttributeView, av.Collection) error { cancel(); return nil }
			case "dynamic":
				a.KeyValues[1].Key.Template = `.action{$k := "t3"}.action{index .id_mod $k}`
			}
			v, e := r.Resolve(a, "t1", row)
			if failure == "" || failure == "dynamic" {
				if e != nil || v.Template.Content != "leaf" {
					t.Fatal(v, e)
				}
				v2, e := r.Resolve(a, "t1", row)
				if e != nil || v2 != v {
					t.Fatal("memoization", e)
				}
			} else {
				if v != nil || e == nil {
					t.Fatal(v, e)
				}
				if (failure == "depth" || failure == "nodes") && !errors.Is(e, ErrLimit) {
					t.Fatal(e)
				}
				if failure == "prepare" && !errors.Is(e, want) {
					t.Fatal(e)
				}
				if (failure == "cancel-before" || failure == "cancel-during") && !errors.Is(e, context.Canceled) {
					t.Fatal(e)
				}
			}
		})
	}
}
