//go:build fts5

package workspace

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestNativeInvalidReferencesActualSourceOwnersAndCorruption(t *testing.T) {
	ctx := context.Background()
	root := positionSource(t)
	file := filepath.Join(root, "pages", pageA, "data", boxID, docID+".docj")
	raw, e := os.ReadFile(file)
	if e != nil {
		t.Fatal(e)
	}
	raw = bytes.ReplaceAll(raw, []byte(`"TextMarkBlockRefID":"`+blockID+`"`), []byte(`"TextMarkBlockRefID":"20261104195500-missing"`))
	if e = os.WriteFile(file, raw, 0600); e != nil {
		t.Fatal(e)
	}
	before := digest(t, root)
	m := open(t, root, lifecycleOptions(root))
	for _, page := range []string{pageA, pageB} {
		l := getDocLease(t, m, page)
		queueGetDoc(t, l)
		if e = l.FlushNativeMainQueue(ctx); e != nil {
			t.Fatal(e)
		}
		out, e := l.NativeInvalidReferences(ctx, 1, 32)
		want := 0
		if page == pageA {
			want = 1
		}
		if e != nil || out.MatchedBlocks != want || len(out.Blocks) != want {
			t.Fatal(page, out, e)
		}
		if out, e = l.NativeInvalidReferences(ctx, 99999999, 1); e != nil || len(out.Blocks) != 0 || out.MatchedBlocks != want {
			t.Fatal(out, e)
		}
	}
	if digest(t, root) != before {
		t.Fatal("scan published source")
	}
	if e = os.WriteFile(file, []byte("corrupt"), 0600); e != nil {
		t.Fatal(e)
	}
	l := getDocLease(t, m, pageA)
	if out, e := l.NativeInvalidReferences(ctx, 1, 32); e == nil || out.Blocks != nil {
		t.Fatal("corruption returned partial", out, e)
	}
}
