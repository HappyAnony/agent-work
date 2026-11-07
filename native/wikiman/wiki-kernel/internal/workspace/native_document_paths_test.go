//go:build fts5

package workspace

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"wiki-kernel/internal/docquery"
)

func pathQuerySource(t *testing.T) string {
	t.Helper()
	root := documentTreeFixture(t)
	for _, page := range []string{pageA, pageB} {
		for _, x := range []struct{ p, title string }{{docID + ".docj", "Parent &amp; " + page}, {docID + "/20261026110000-child01.docj", "Child &amp; " + page}} {
			f := filepath.Join(root, "pages", page, "data", boxID, x.p)
			raw, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			var d map[string]interface{}
			if err = json.Unmarshal(raw, &d); err != nil {
				t.Fatal(err)
			}
			d["Properties"].(map[string]interface{})["title"] = x.title
			raw, _ = json.Marshal(d)
			if err = os.WriteFile(f, raw, 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	return root
}
func TestNativeDocumentPathsRealOwnerTreeRegistryAndHTMLRules(t *testing.T) {
	ctx := context.Background()
	root := pathQuerySource(t)
	before := digest(t, root)
	m := open(t, root, treeOptions(root))
	for _, page := range []string{pageA, pageB} {
		l, err := m.Acquire(ctx, storeID, page)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(l.Release)
		child := "20261026110000-child01"
		p := "/" + docID + "/" + child + ".docj"
		hpath := "/Parent & " + page + "/Child & " + page
		for _, mode := range []string{"path", "id", "location", "full"} {
			q := docquery.PathRequest{Mode: mode, Box: boxID, Path: p, ID: child}
			r, err := l.NativeDocumentPaths(ctx, q)
			want := hpath
			if mode == "full" {
				want = page + want
			}
			if err != nil || r.HPath != want || r.Path != p || r.Box != boxID {
				t.Fatal(mode, r, err)
			}
		}
		batch, err := l.NativeDocumentPaths(ctx, docquery.PathRequest{Mode: "batch", Paths: []string{p, p, "/missing.docj"}})
		if err != nil || len(batch.Paths) != 1 || batch.Paths[0] != page+"/Parent & "+page+"/Child & "+page {
			t.Fatal(batch, err)
		}
		ids, err := l.NativeDocumentPaths(ctx, docquery.PathRequest{Mode: "ids", Box: boxID, HPath: hpath})
		if err != nil || len(ids.IDs) != 1 || ids.IDs[0] != child {
			t.Fatal(ids, err)
		}
		if _, err = l.NativeDocumentPaths(ctx, docquery.PathRequest{Mode: "path", Box: boxID, Path: "/wrong/" + child + ".docj"}); err == nil {
			t.Fatal("path silently redirected to matching ID")
		}
		cancelled, cancel := context.WithCancel(ctx)
		cancel()
		if _, err = l.NativeDocumentPaths(cancelled, docquery.PathRequest{Mode: "path", Path: "/"}); err == nil {
			t.Fatal("cancelled root shortcut succeeded")
		}
		l.Release()
		if _, err = l.NativeDocumentPaths(ctx, docquery.PathRequest{Mode: "id", ID: child}); err == nil {
			t.Fatal("released owner succeeded")
		}
	}
	if digest(t, root) != before {
		t.Fatal("path lookup changed source")
	}
}
func TestNativeDocumentPathsParentFallbackAndMissingParentNeverRepairs(t *testing.T) {
	ctx := context.Background()
	root := pathQuerySource(t)
	m := open(t, root, treeOptions(root))
	l, err := m.Acquire(ctx, storeID, pageA)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Release()
	parent := filepath.Join(root, "pages", pageA, "data", boxID, docID+".docj")
	raw, err := os.ReadFile(parent)
	if err != nil {
		t.Fatal(err)
	}
	var d map[string]interface{}
	_ = json.Unmarshal(raw, &d)
	d["Properties"].(map[string]interface{})["title"] = ""
	raw, _ = json.Marshal(d)
	if err = os.WriteFile(parent, raw, 0600); err != nil {
		t.Fatal(err)
	}
	q := docquery.PathRequest{Mode: "path", Box: boxID, Path: "/" + docID + "/20261026110000-child01.docj"}
	r, err := l.NativeDocumentPaths(ctx, q)
	if err != nil || r.HPath != "/Untitled/Child & "+pageA {
		t.Fatal(r, err)
	}
	if err = os.Remove(parent); err != nil {
		t.Fatal(err)
	}
	before := digest(t, root)
	if _, err = l.NativeDocumentPaths(ctx, q); err == nil {
		t.Fatal("missing parent read succeeded")
	}
	if digest(t, root) != before {
		t.Fatal("missing parent was repaired")
	}
}
