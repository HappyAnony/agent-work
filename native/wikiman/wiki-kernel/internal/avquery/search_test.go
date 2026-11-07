package avquery

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func TestOriginalRankingLimitBeforeOrphansAndDependencyFailures(t *testing.T) {
	ctx := context.Background()
	s := Services{Strict: true, Relations: map[string][]string{}, Resolve: func(_ context.Context, ids []string) (Location, bool, error) {
		id := ids[0]
		ids[0] = "outside"
		return Location{BlockID: id, HPath: "/original"}, id != "0", nil
	}, Views: func(context.Context, string) ([]View, error) {
		return []View{{ID: "view", Name: "Original", Layout: "table"}}, nil
	}}
	for i := 0; i < 14; i++ {
		id := fmt.Sprint(i)
		s.Entries = append(s.Entries, Entry{ID: id, Name: "AGI planner", Updated: int64(14 - i)})
		s.Relations[id] = []string{id}
	}
	got, e := Search(ctx, "", nil, s)
	if e != nil || len(got) != 11 || got[0].AvID != "1" || got[10].AvID != "11" || got[0].Children[0].ViewLayout != "table" {
		t.Fatal(got, e)
	}
	if s.Relations["0"][0] != "0" {
		t.Fatal("resolve mutated shared relationships")
	}
	got, e = Search(ctx, "agi PLANNER", []string{"1"}, s)
	if e != nil || len(got) != 11 || got[0].AvID != "2" {
		t.Fatal(got, e)
	}
	got, e = Search(ctx, "missing", nil, s)
	if e != nil || len(got) != 0 {
		t.Fatal(got, e)
	}
	sentinel := errors.New("source unavailable")
	s.Views = func(context.Context, string) ([]View, error) { return nil, sentinel }
	if _, e = Search(ctx, "", nil, s); !errors.Is(e, sentinel) {
		t.Fatal(e)
	}
	s.Strict = false
	if got, e = Search(ctx, "", nil, s); e != nil || len(got) != 0 {
		t.Fatal(got, e)
	}
	s.Strict = true
	s.Resolve = func(context.Context, []string) (Location, bool, error) { return Location{}, false, sentinel }
	if _, e = Search(ctx, "", nil, s); !errors.Is(e, sentinel) {
		t.Fatal(e)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, e = Search(canceled, "", nil, s); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
}
