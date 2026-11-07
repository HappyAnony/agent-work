package model

import (
	"context"
	"reflect"
	"sort"
	"strings"
	"testing"
	"wiki-kernel/av"
	"wiki-kernel/internal/avquery"
)

func TestOriginalAVPrimaryAlgorithmEquivalent(t *testing.T) {
	values := []*av.Value{{ID: "one", Type: av.KeyTypeBlock, Block: &av.ValueBlock{ID: "known", Content: "  Alpha 中文  ", Updated: 1}}, {ID: "two", Type: av.KeyTypeBlock, IsDetached: true, Block: &av.ValueBlock{Content: "ALPHA other", Updated: 8}}, {ID: "three", Type: av.KeyTypeBlock, Block: &av.ValueBlock{ID: "gone", Content: "alpha", Updated: 99}}, {ID: "four", Type: av.KeyTypeBlock, Block: &av.ValueBlock{ID: "known", Content: "Beta", Updated: 3}}}
	exists := func(_ context.Context, id string) (bool, error) { return id == "known", nil }
	for _, query := range []string{"", "ALpha", "中文", " Alpha "} {
		for _, size := range []int{0, 1, 2, 16} {
			for _, page := range []int{1, 2} {
				var original []*av.Value
				for _, v := range values {
					if !v.IsDetached && v.Block.ID != "known" {
						continue
					}
					if strings.Contains(strings.ToLower(strings.TrimSpace(v.Block.Content)), strings.ToLower(query)) {
						original = append(original, v)
					}
				}
				sort.Slice(original, func(i, j int) bool { return original[i].Block.Updated > original[j].Block.Updated })
				limit := size
				if limit < 1 {
					limit = 16
				}
				start := (page - 1) * limit
				if start > len(original) {
					continue
				}
				end := min(start+limit, len(original))
				original = original[start:end]
				got, e := avquery.PrimaryValues(context.Background(), values, func(v *av.Value) (avquery.PrimaryMeta, error) {
					return avquery.PrimaryMeta{BoundID: v.Block.ID, Text: v.String(true), Updated: v.Block.Updated, Detached: v.IsDetached}, nil
				}, exists, query, page, size)
				if e != nil || !reflect.DeepEqual(got, original) {
					if len(got) == 0 && len(original) == 0 {
						continue
					}
					t.Fatal(query, page, size, got, original, e)
				}
			}
		}
	}
	for _, requested := range []string{"", "a", "b", "missing"} {
		v := &av.AttributeView{ViewID: "b", Views: []*av.View{{ID: "a"}, {ID: "b"}}}
		want := v.Views[1]
		if requested == "a" {
			want = v.Views[0]
		}
		got, e := v.GetCurrentView(requested)
		if e != nil || got != want {
			t.Fatal(requested, got, e)
		}
	}
}
