//go:build fts5

package workspace

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestNativeNotebookHistoryRealOwnerIsolation(t *testing.T) {
	ctx := context.Background()
	root := getDocSource(t)
	m := open(t, root, getDocOptions())
	for _, page := range []string{pageA, pageB} {
		l := getDocLease(t, m, page)
		name := filepath.Join(l.workspace.paths.RuntimeDir, "history", "2026-11-04-120000-delete", boxID, ".siyuan", "conf.json")
		if e := os.MkdirAll(filepath.Dir(name), 0700); e != nil {
			t.Fatal(e)
		}
		if e := os.WriteFile(name, []byte(fmt.Sprintf(`{"name":%q}`, page)), 0600); e != nil {
			t.Fatal(e)
		}
		out, e := l.NativeNotebookHistory(ctx)
		if e != nil || len(out) != 1 || out[0].Name != page {
			t.Fatal(page, out, e)
		}
		out[0].Name = "mutated"
		again, e := l.NativeNotebookHistory(ctx)
		if e != nil || again[0].Name != page {
			t.Fatal(again, e)
		}
	}
}
