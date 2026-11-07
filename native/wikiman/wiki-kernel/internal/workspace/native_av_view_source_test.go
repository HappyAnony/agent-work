//go:build fts5

package workspace

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestNativeAVViewKnownSourceFailureIsNotMissingBlockFallback(t *testing.T) {
	ctx := context.Background()
	root := fixture(t)
	const id = "20261022090000-av00001"
	dir := filepath.Join(root, "pages", pageA, "data", "storage", "av")
	if e := os.MkdirAll(dir, 0700); e != nil {
		t.Fatal(e)
	}
	file := filepath.Join(dir, id+".json")
	if e := os.WriteFile(file, []byte(`{"id":"`+id+`","views":[{"id":"one"}]}`), 0600); e != nil {
		t.Fatal(e)
	}
	m := open(t, root, Options{Configuration: originalProjectionConfiguration})
	l, e := m.Acquire(ctx, storeID, pageA)
	if e != nil {
		t.Fatal(e)
	}
	defer l.Release()
	if _, e = l.NativeAttributeViewView(ctx, id, blockID); e != nil {
		t.Fatal(e)
	}
	doc := filepath.Join(root, "pages", pageA, "data", boxID, docID+".docj")
	if e = os.Rename(doc, doc+".held"); e != nil {
		t.Fatal(e)
	}
	before := digest(t, root)
	if out, e := l.NativeAttributeViewView(ctx, id, blockID); e == nil || out.View != nil {
		t.Fatal("known broken source masked by view fallback", out, e)
	}
	if digest(t, root) != before {
		t.Fatal("view query repaired missing source")
	}
	if e = os.WriteFile(file, []byte(`{"views":[{"id":"one","filters":[5]}]}`), 0600); e != nil {
		t.Fatal(e)
	}
	before = digest(t, root)
	if out, e := l.NativeAttributeViewView(ctx, id, ""); e == nil || out.View != nil {
		t.Fatal("invalid view rule accepted", out, e)
	}
	if digest(t, root) != before {
		t.Fatal("invalid view rules changed source")
	}
}
