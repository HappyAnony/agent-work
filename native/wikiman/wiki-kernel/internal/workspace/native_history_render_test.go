//go:build fts5

package workspace

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeHistoryRenderSamePathOwnerIsolation(t *testing.T) {
	ctx := context.Background()
	root := getDocSource(t)
	m := open(t, root, getDocOptions())
	for _, page := range []string{pageA, pageB} {
		l := getDocLease(t, m, page)
		raw, e := os.ReadFile(filepath.Join(root, "pages", page, "data", boxID, docID+".docj"))
		if e != nil {
			t.Fatal(e)
		}
		raw = []byte(strings.ReplaceAll(string(raw), "SharedNeedle", "SharedNeedle "+page))
		name := filepath.Join(l.workspace.paths.RuntimeDir, "history", "stamp", "history.docj")
		if e = os.MkdirAll(filepath.Dir(name), 0700); e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(name, raw, 0600); e != nil {
			t.Fatal(e)
		}
		out, e := l.NativeRenderHistory(ctx, "stamp/history.docj", "SharedNeedle", true)
		if e != nil || out.ID != docID || !strings.Contains(out.Content, "search-mark") || !strings.Contains(out.Content, page) {
			t.Fatal(page, out, e)
		}
		if out, e = l.NativeRenderHistory(ctx, "../outside", "", false); e == nil || out.Content != "" {
			t.Fatal(out, e)
		}
		if e = os.WriteFile(name, []byte("corrupt"), 0600); e != nil {
			t.Fatal(e)
		}
		if out, e = l.NativeRenderHistory(ctx, "stamp/history.docj", "", false); e == nil || out.Content != "" {
			t.Fatal(out, e)
		}
	}
}
