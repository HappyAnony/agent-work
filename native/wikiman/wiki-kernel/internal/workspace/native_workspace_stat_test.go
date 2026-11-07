//go:build fts5

package workspace

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"wiki-kernel/internal/workspacestat"
)

func TestNativeWorkspaceStatisticsSameIDsFixedSource(t *testing.T) {
	root := fixture(t)
	for i, page := range []string{pageA, pageB} {
		if e := os.WriteFile(filepath.Join(root, "pages", page, "data/assets/fixture.svg"), make([]byte, 3+i*4), 0600); e != nil {
			t.Fatal(e)
		}
	}
	before := digest(t, root)
	m := open(t, root, lifecycleOptions(root))
	ctx := context.Background()
	results := []workspacestat.Result{}
	for _, page := range []string{pageA, pageB} {
		l, e := m.Acquire(ctx, storeID, page)
		if e != nil {
			t.Fatal(e)
		}
		out, e := l.NativeWorkspaceStatistics(ctx)
		if e != nil || out.TreeCount < 1 || out.BlockCount < 1 {
			t.Fatal(out, e)
		}
		results = append(results, out)
		canceled, cancel := context.WithCancel(ctx)
		cancel()
		if out, e = l.NativeWorkspaceStatistics(canceled); !errors.Is(e, context.Canceled) || out != (workspacestat.Result{}) {
			t.Fatal(out, e)
		}
		l.Release()
	}
	if results[1].AssetsSize-results[0].AssetsSize != 4 {
		t.Fatal(results)
	}
	if digest(t, root) != before {
		t.Fatal("statistics changed source")
	}
}
