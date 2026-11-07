//go:build fts5

package workspace

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestIntegrityAssetReferencesIncludeClosedNotebooks(t *testing.T) {
	root := fixture(t)
	box := filepath.Join(root, "pages", pageA, "data", boxID)
	metadata := filepath.Join(box, ".wiki", "conf.json")
	raw, err := os.ReadFile(metadata)
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]json.RawMessage
	if err = json.Unmarshal(raw, &config); err != nil {
		t.Fatal(err)
	}
	config["closed"] = json.RawMessage("true")
	raw, err = json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(metadata, raw, 0600); err != nil {
		t.Fatal(err)
	}
	doc := `{"ID":"` + docID + `","Type":"NodeDocument","Properties":{"title":"Closed notebook"},"Children":[{"ID":"` +
		blockID + `","Type":"NodeParagraph","Children":[{"Type":"NodeImage","Children":[{"Type":"NodeLinkDest","Data":"assets/r10-missing.png"}]}]}]}`
	if err = os.WriteFile(filepath.Join(box, docID+".docj"), []byte(doc), 0600); err != nil {
		t.Fatal(err)
	}
	before := digest(t, root)
	m := open(t, root, Options{ReadDefaults: true})
	ctx := context.Background()
	l, err := m.Acquire(ctx, storeID, pageA)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Release()
	interactive, err := l.NativeAssetReferences(ctx, true)
	if err != nil || len(interactive) != 0 {
		t.Fatal(interactive, err)
	}
	missing, err := l.NativeMissingAssetsInAllNotebooks(ctx)
	if err != nil || !reflect.DeepEqual(missing, []string{"assets/r10-missing.png"}) {
		t.Fatal(missing, err)
	}
	if digest(t, root) != before {
		t.Fatal("asset integrity check published source")
	}
}

func TestNativeAssetReferencesOwnSourceAndNoPublication(t *testing.T) {
	root := fixture(t)
	for _, page := range []string{pageA, pageB} {
		name := filepath.Join(root, "pages", page, "data", "assets", page+"-unused.txt")
		if e := os.MkdirAll(filepath.Dir(name), 0700); e != nil {
			t.Fatal(e)
		}
		if e := os.WriteFile(name, []byte(page), 0600); e != nil {
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
		defer l.Release()
		out, e := l.NativeAssetReferences(ctx, false)
		if e != nil || !reflect.DeepEqual(out, []string{"assets/" + page + "-unused.txt", "assets/fixture.svg"}) {
			t.Fatal(page, out, e)
		}
		names, e := l.NativeSearchAssetNames(ctx, page+"-unused", []string{".txt"})
		if e != nil || len(names) != 1 || names[0].Path != out[0] {
			t.Fatal(names, e)
		}
	}
	if digest(t, root) != before {
		t.Fatal("asset reads published source")
	}
}
