package notebooks

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestNotebookRemoveOriginalOrderAndFailurePrefix(t *testing.T) {
	for _, guide := range []bool{false, true} {
		for _, fail := range []string{"", "backup", "assets", "unmount", "delete", "changed"} {
			calls := []string{}
			injected := errors.New("injected")
			port := func(name string) func(context.Context, string) error {
				return func(context.Context, string) error {
					calls = append(calls, name)
					if name == fail {
						return injected
					}
					return nil
				}
			}
			id := "20261019090000-box0001"
			want := []string{"backup", "assets", "unmount", "delete", "changed"}
			if guide {
				id = "20210808180117-czj9bvb"
				want = want[2:]
			}
			e := Remove(context.Background(), id, RemovePorts{port("backup"), port("assets"), port("unmount"), port("delete"), port("changed")})
			expected := []string{}
			hasFailure := false
			for _, name := range want {
				expected = append(expected, name)
				if name == fail {
					hasFailure = true
					break
				}
			}
			if !reflect.DeepEqual(calls, expected) || (hasFailure && !errors.Is(e, injected)) || (!hasFailure && e != nil) {
				t.Fatal(guide, fail, calls, e)
			}
		}
	}
}
