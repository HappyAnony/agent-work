package avquery

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestIdentityMapOriginalCollisionsEmptyAndMissing(t *testing.T) {
	values := [][2]string{{"row-a", "bound"}, {"row-b", "bound"}, {"row-b", "last"}, {"detached", ""}}
	describe := func(v [2]string) (string, string, error) { return v[0], v[1], nil }
	for _, toItems := range []bool{false, true} {
		ids := []string{"row-a", "row-b", "bound", "last", "", "missing", "bound"}
		ret, err := IdentityMap(context.Background(), values, ids, toItems, describe)
		if err != nil {
			t.Fatal(err)
		}
		want := EmptyIdentityMap(ids)
		if toItems {
			want["bound"] = "row-b"
			want["last"] = "row-b"
			want[""] = "detached"
		} else {
			want["row-a"] = "bound"
			want["row-b"] = "last"
		}
		if !reflect.DeepEqual(ret, want) {
			t.Fatal(ret, want)
		}
	}
	ret, err := IdentityMap(context.Background(), values, []string{}, true, describe)
	if err != nil || ret == nil || len(ret) != 0 {
		t.Fatal("empty object changed", ret, err)
	}
}
func TestIdentityMapFailuresCancellationAndNoPartial(t *testing.T) {
	failure := errors.New("bad record")
	for _, scenario := range []string{"describe", "cancel", "late", "nil-describe", "nil-context"} {
		ctx, cancel := context.WithCancel(context.Background())
		describe := func(v int) (string, string, error) {
			if v == 2 {
				if scenario == "describe" {
					return "", "", failure
				}
				cancel()
			}
			return "row", "block", nil
		}
		if scenario == "cancel" {
			cancel()
		}
		if scenario == "nil-describe" {
			describe = nil
		}
		if scenario == "nil-context" {
			ctx = nil
		}
		ret, err := IdentityMap(ctx, []int{1, 2}, []string{"block"}, true, describe)
		if err == nil || ret != nil {
			t.Fatal(scenario, ret, err)
		}
		cancel()
	}
}
