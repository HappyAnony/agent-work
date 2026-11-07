package avrender

import (
	"context"
	"errors"
	"os"
	"testing"
	av "wiki-kernel/internal/avmodel"
)

func furtherFixture(typ av.KeyType, self bool) (*av.AttributeView, *av.AttributeView) {
	a := &av.AttributeView{ID: "source", KeyValues: []*av.KeyValues{{Key: &av.Key{ID: "relation", Type: av.KeyTypeRelation, Relation: &av.Relation{AvID: "dest"}}}, {Key: &av.Key{ID: "rollup", Type: av.KeyTypeRollup, Rollup: &av.Rollup{RelationKeyID: "relation", KeyID: "target"}}}}}
	b := &av.AttributeView{ID: "dest", KeyValues: []*av.KeyValues{{Key: &av.Key{ID: "target", Type: typ}}}, Views: []*av.View{{ID: "first"}, {ID: "second"}}}
	if self {
		b.ID = a.ID
		a.KeyValues = append(a.KeyValues, b.KeyValues...)
		a.Views = b.Views
		a.KeyValues[0].Key.Relation.AvID = a.ID
		b = a
	}
	return a, b
}

func TestFurtherCollectionsOriginalKindsFirstViewAndCache(t *testing.T) {
	for _, typ := range []av.KeyType{av.KeyTypeNumber, av.KeyTypeTemplate, av.KeyTypeUpdated, av.KeyTypeCreated, av.KeyTypeRelation} {
		for _, self := range []bool{false, true} {
			a, b := furtherFixture(typ, self)
			reads, renders := 0, 0
			cache := map[string]*av.AttributeView{}
			e := FurtherEnvironment{Read: func(id string) (*av.AttributeView, error) {
				reads++
				if id != b.ID {
					t.Fatal(id)
				}
				return b, nil
			}, Render: func(a *av.AttributeView, v *av.View, q string) (av.Viewable, error) {
				renders++
				if v.ID != "first" || q != "" {
					t.Fatal("view fallback changed")
				}
				return &av.Table{}, nil
			}}
			for i := 0; i < 2; i++ {
				ret, err := FurtherCollections(context.Background(), a, cache, e)
				if err != nil {
					t.Fatal(err)
				}
				value, ok := ret["rollup"]
				want := typ == av.KeyTypeTemplate || (!self && typ != av.KeyTypeNumber)
				if !ok || (value != nil) != want {
					t.Fatal(typ, self, ret)
				}
			}
			wantRender := 0
			if typ == av.KeyTypeTemplate || (!self && typ != av.KeyTypeNumber) {
				wantRender = 2
			}
			if reads != 1 || renders != wantRender {
				t.Fatal("cache/condition", reads, renders)
			}
		}
	}
}

func TestFurtherCollectionsMissingSourcesShapeAndCancellation(t *testing.T) {
	failure := errors.New("owner failure")
	for _, scenario := range []string{"read", "read-cancel", "render", "render-cancel", "missing", "allow-missing", "bad-relation", "nil-key", "wrong-id", "empty-views"} {
		ctx, cancel := context.WithCancel(context.Background())
		a, b := furtherFixture(av.KeyTypeTemplate, false)
		e := FurtherEnvironment{Read: func(string) (*av.AttributeView, error) { return b, nil }, Render: func(*av.AttributeView, *av.View, string) (av.Viewable, error) { return &av.Table{}, nil }}
		switch scenario {
		case "read":
			e.Read = func(string) (*av.AttributeView, error) { return nil, failure }
		case "read-cancel":
			e.Read = func(string) (*av.AttributeView, error) { cancel(); return b, nil }
		case "render":
			e.Render = func(*av.AttributeView, *av.View, string) (av.Viewable, error) { return nil, failure }
		case "render-cancel":
			e.Render = func(*av.AttributeView, *av.View, string) (av.Viewable, error) { cancel(); return &av.Table{}, nil }
		case "missing", "allow-missing":
			e.Read = func(string) (*av.AttributeView, error) { return nil, failure }
			e.AllowMissing = scenario == "allow-missing"
		case "bad-relation":
			a.KeyValues[0].Key.Relation = nil
		case "nil-key":
			b.KeyValues[0].Key = nil
		case "wrong-id":
			b.ID = "other"
		case "empty-views":
			b.Views = nil
		}
		ret, err := FurtherCollections(ctx, a, map[string]*av.AttributeView{}, e)
		if scenario == "allow-missing" {
			if err != nil || len(ret) != 0 {
				t.Fatal(ret, err)
			}
		} else if err == nil || ret != nil {
			t.Fatal("failure became partial success", scenario, ret, err)
		}
		cancel()
	}
}

func TestFurtherCollectionsSkipsOnlyDeclaredMissing(t *testing.T) {
	a, _ := furtherFixture(av.KeyTypeTemplate, false)
	for _, failure := range []error{os.ErrNotExist, errors.New("corrupt source"), context.Canceled} {
		env := FurtherEnvironment{Missing: os.IsNotExist, Read: func(string) (*av.AttributeView, error) { return nil, failure }, Render: func(*av.AttributeView, *av.View, string) (av.Viewable, error) {
			t.Fatal("unexpected render")
			return nil, nil
		}}
		out, err := FurtherCollections(context.Background(), a, map[string]*av.AttributeView{}, env)
		if os.IsNotExist(failure) {
			if err != nil || len(out) != 0 {
				t.Fatal("missing not skipped", out, err)
			}
		} else if !errors.Is(err, failure) {
			t.Fatal("non-missing error swallowed", err)
		}
	}
}
