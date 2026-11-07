//go:build fts5

package workspace

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"wiki-kernel/internal/indexrepair"
	"wiki-kernel/internal/workspacefile"
)

func TestNativeFileReadSamePathsDifferentOwners(t *testing.T) {
	root := fixture(t)
	for _, page := range []string{pageA, pageB} {
		if e := os.WriteFile(filepath.Join(root, "pages", page, "data/assets/owned.txt"), []byte(page), 0600); e != nil {
			t.Fatal(e)
		}
	}
	before := digest(t, root)
	m := open(t, root, lifecycleOptions(root))
	ctx := context.Background()
	for _, page := range []string{pageA, pageB} {
		l, e := m.Acquire(ctx, storeID, page)
		if e != nil {
			t.Fatal(e)
		}
		out, e := l.NativeFileRead(ctx, workspacefile.Request{Action: "read", Path: "assets/owned.txt"})
		if e != nil || string(out.Data) != page {
			t.Fatal(e, string(out.Data))
		}
		if _, e = l.NativeFileRead(ctx, workspacefile.Request{Action: "write", Path: "assets/owned.txt", Data: []byte("bad")}); !errors.Is(e, indexrepair.ErrSourceWrite) {
			t.Fatal(e)
		}
		cancelled, cancel := context.WithCancel(ctx)
		cancel()
		if _, e = l.NativeFileRead(cancelled, workspacefile.Request{Action: "read", Path: "assets/owned.txt"}); !errors.Is(e, context.Canceled) {
			t.Fatal(e)
		}
		l.Release()
	}
	if digest(t, root) != before {
		t.Fatal("native file read changed source")
	}
}
