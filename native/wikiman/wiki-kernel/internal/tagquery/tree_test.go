package tagquery

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestTagTreeLimitsCancellationAndNestedCounts(t *testing.T) {
	got, over, e := Build(context.Background(), map[string]int{"A/B/C": 3, "A": 2}, 4, 1, false)
	if e != nil || !over || len(got) != 1 || got[0].Count != 2 || got[0].Children[0].Children[0].Count != 3 {
		t.Fatal(got, over, e)
	}
	if _, _, e = Build(context.Background(), map[string]int{strings.Repeat("A/", 257): 1}, 4, 100, true); e == nil {
		t.Fatal("unbounded hierarchy")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, e = Build(ctx, map[string]int{}, 4, 100, true); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
}
