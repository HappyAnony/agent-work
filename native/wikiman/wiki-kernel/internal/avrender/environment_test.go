package avrender

import (
	"errors"
	"testing"
	av "wiki-kernel/internal/avmodel"
)

func TestTableRequiresEveryHandlerDependency(t *testing.T) {
	for _, missing := range []string{"remove", "filter", "attrs", "auto", "template"} {
		e := Environment{
			RemoveMissingField: func(*av.AttributeView, *av.View, string) error { return nil },
			FilterItems:        func(map[string][]*av.KeyValues) error { return nil },
			BlockAttrs:         func([]string) (map[string]map[string]string, error) { return nil, nil },
			AutoValues:         func(*av.AttributeView, av.Collection, map[string]map[string]string) error { return nil },
			Templates:          func(*av.AttributeView, *av.View, av.Collection, map[string]map[string]string) error { return nil },
		}
		switch missing {
		case "remove":
			e.RemoveMissingField = nil
		case "filter":
			e.FilterItems = nil
		case "attrs":
			e.BlockAttrs = nil
		case "auto":
			e.AutoValues = nil
		case "template":
			e.Templates = nil
		}
		if out, err := Table(nil, nil, "", e); out != nil || !errors.Is(err, ErrEnvironmentUnbound) {
			t.Fatal(missing, out, err)
		}
	}
}
