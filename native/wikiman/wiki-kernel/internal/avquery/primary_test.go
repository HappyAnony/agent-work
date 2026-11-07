package avquery

import (
	"context"
	"errors"
	"math"
	"reflect"
	"testing"
)

func TestOriginalPrimarySelectionPagingIsolationAndFailures(t *testing.T) {
	ctx := context.Background()
	values := []PrimaryMeta{{BoundID: "known", Text: " Alpha ", Updated: 1}, {BoundID: "missing", Text: "Alpha", Updated: 99}, {Detached: true, Text: "ALPHA", Updated: 9}, {BoundID: "known", Text: "other", Updated: 8}}
	describe := func(v PrimaryMeta) (PrimaryMeta, error) { return v, nil }
	exists := func(_ context.Context, id string) (bool, error) { return id == "known", nil }
	for _, test := range []struct{ Page, Size, Count int }{{1, 1, 1}, {2, 1, 1}, {3, 1, 0}, {1, 0, 2}, {math.MaxInt, math.MaxInt, 0}, {1, math.MaxInt, 2}} {
		got, e := PrimaryValues(ctx, values, describe, exists, "alpha", test.Page, test.Size)
		if e != nil || len(got) != test.Count {
			t.Fatal(test, got, e)
		}
		if test.Page == 1 && len(got) > 0 && got[0].Updated != 9 {
			t.Fatal(got)
		}
	}
	if _, e := PrimaryValues(ctx, values, describe, exists, "", 0, 1); !errors.Is(e, ErrPrimaryPage) {
		t.Fatal(e)
	}
	sentinel := errors.New("dependency failed")
	if _, e := PrimaryValues(ctx, values, describe, func(context.Context, string) (bool, error) { return false, sentinel }, "", 1, 1); !errors.Is(e, sentinel) {
		t.Fatal(e)
	}
	if _, e := PrimaryValues(ctx, values, func(PrimaryMeta) (PrimaryMeta, error) { return PrimaryMeta{}, sentinel }, exists, "", 1, 1); !errors.Is(e, sentinel) {
		t.Fatal(e)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, e := PrimaryValues(canceled, values, describe, exists, "", 1, 1); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if values[0].Updated != 1 {
		t.Fatal("input sorted in place")
	}
	ids, e := MirrorIDs(ctx, []string{"known", "missing", "known"}, exists)
	if e != nil || !reflect.DeepEqual(ids, []string{"known"}) {
		t.Fatal(ids, e)
	}
	late, stop := context.WithCancel(ctx)
	defer stop()
	if out, e := PrimaryValues(late, values, describe, func(context.Context, string) (bool, error) { stop(); return false, nil }, "no-match", 100, 1); !errors.Is(e, context.Canceled) || out != nil {
		t.Fatal("late cancellation lost", out, e)
	}
}

func TestOriginalViewFallbackOrderAndEmptyID(t *testing.T) {
	type view struct{ ID string }
	views := []view{{"first"}, {"current"}, {""}}
	for _, test := range []struct{ Requested, Current, Want string }{{"first", "current", "first"}, {"missing", "current", "current"}, {"", "current", "current"}, {"missing", "missing", "first"}, {"", "", ""}} {
		got, e := CurrentView(context.Background(), views, test.Requested, test.Current, func(v view) string { return v.ID })
		if e != nil || got.ID != test.Want {
			t.Fatal(test, got, e)
		}
	}
	if _, e := CurrentView(context.Background(), []view{}, "", "", func(v view) string { return v.ID }); !errors.Is(e, ErrViewNotFound) {
		t.Fatal(e)
	}
}
