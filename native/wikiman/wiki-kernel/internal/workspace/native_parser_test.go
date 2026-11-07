package workspace

import (
	"context"
	"testing"

	"github.com/88250/lute"
	"wiki-kernel/internal/nativeparser"
)

func TestNativeParserUsesExplicitPageSettings(t *testing.T) {
	ctx := context.Background()
	root := fixture(t)
	before := digest(t, root)
	m := open(t, root, Options{})
	a, err := m.Acquire(ctx, storeID, pageA)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Release()
	b, err := m.Acquire(ctx, storeID, pageB)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Release()
	settings := nativeparser.EditorOptions{Markdown: nativeparser.Defaults()}
	var first *lute.Lute
	if err = a.WithNativeParser(ctx, settings, func(engine *lute.Lute) error { first = engine; engine.SetInlineMath(false); return nil }); err != nil {
		t.Fatal(err)
	}
	for _, lease := range []*Lease{a, b} {
		if err = lease.WithNativeParser(ctx, settings, func(engine *lute.Lute) error {
			if engine == first || !engine.ParseOptions.InlineMath {
				t.Error("borrowed mutable parser escaped")
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	if digest(t, root) != before {
		t.Fatal("parser saved defaults")
	}
}
