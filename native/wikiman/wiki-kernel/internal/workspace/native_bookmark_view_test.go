//go:build fts5

package workspace

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"wiki-kernel/internal/indexdb"
)

func bookmarkViewSource(t *testing.T) string {
	t.Helper()
	root := fixture(t)
	for _, page := range []string{pageA, pageB} {
		base := filepath.Join(root, "pages", page, "data")
		f := filepath.Join(base, boxID, docID+".docj")
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		var doc map[string]interface{}
		if err = json.Unmarshal(raw, &doc); err != nil {
			t.Fatal(err)
		}
		doc["Properties"].(map[string]interface{})["bookmark"] = "Root"
		children := doc["Children"].([]interface{})
		children[0].(map[string]interface{})["Properties"].(map[string]interface{})["bookmark"] = "A"
		children = append(children, map[string]interface{}{"ID": "20261026195000-avnode1", "Type": "NodeAttributeView", "AttributeViewID": "20261026195100-dbase01", "Properties": map[string]string{"id": "20261026195000-avnode1", "bookmark": "A"}}, map[string]interface{}{"ID": "20261026195200-named01", "Type": "NodeParagraph", "Properties": map[string]string{"id": "20261026195200-named01", "bookmark": "z", "name": page + " preferred name"}, "Children": []interface{}{map[string]string{"Type": "NodeText", "Data": "Hidden body"}}})
		doc["Children"] = children
		raw, err = json.Marshal(doc)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(f, raw, 0600); err != nil {
			t.Fatal(err)
		}
		dir := filepath.Join(base, "storage", "av")
		if err = os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		raw, _ = json.Marshal(map[string]interface{}{"id": "20261026195100-dbase01", "name": page + " database", "spec": 4, "views": []interface{}{}, "keyValues": []interface{}{}})
		if err = os.WriteFile(filepath.Join(dir, "20261026195100-dbase01.json"), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func bookmarkViewOptions(root string) Options {
	return Options{Configuration: originalProjectionConfiguration, NativeContent: func(store, page string) (indexdb.ContentServices, error) {
		return indexdb.ContentServices{AttributeView: func(ctx context.Context, id string, _ bool) (string, error) {
			if err := ctx.Err(); err != nil {
				return "", err
			}
			raw, err := os.ReadFile(filepath.Join(root, "pages", page, "data", "storage", "av", id+".json"))
			if err != nil {
				return "", err
			}
			var doc struct {
				Name string `json:"name"`
			}
			if err = json.Unmarshal(raw, &doc); err != nil {
				return "", err
			}
			return doc.Name, nil
		}}, nil
	}}
}
func TestNativeBookmarkViewProjectsRealSourceWithOwnerNamesAndFlush(t *testing.T) {
	ctx := context.Background()
	root := bookmarkViewSource(t)
	before := digest(t, root)
	m := open(t, root, bookmarkViewOptions(root))
	for _, page := range []string{pageA, pageB} {
		l, err := m.Acquire(ctx, storeID, page)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(l.Release)
		if err = l.QueueNativeTree(ctx, boxID, docID, "index"); err != nil {
			t.Fatal(err)
		}
		// Panel must drain this owner's accepted main queue before querying.
		got, err := l.NativeBookmarks(ctx)
		if err != nil || len(got) != 3 {
			t.Fatal(got, err)
		}
		if got[0].Name != "A" || got[0].Count != 2 || got[1].Name != "Root" || got[2].Name != "z" {
			t.Fatal("original groups", got)
		}
		wantBody := "Synthetic&nbsp;knowledge"
		if page == pageB {
			wantBody = "Workspace&nbsp;B&nbsp;knowledge"
		}
		content := map[string]string{}
		for _, b := range got[0].Blocks {
			content[b.ID] = b.Content
		}
		if !strings.Contains(content[blockID], wantBody) || content["20261026195000-avnode1"] != page+" database" || got[2].Blocks[0].Content != page+" preferred name" {
			t.Fatal("foreign owner presentation", content, got[2].Blocks[0].Content)
		}
		labels, err := l.NativeBookmarkLabels(ctx)
		if err != nil || !reflect.DeepEqual(labels, []string{"A", "Root", "z"}) {
			t.Fatal(labels, err)
		}
		got[0].Blocks[0].IAL["bookmark"] = "caller mutation"
		again, err := l.NativeBookmarks(ctx)
		if err != nil || again[0].Blocks[0].IAL["bookmark"] != "A" {
			t.Fatal("result alias", again, err)
		}
		cancelled, cancel := context.WithCancel(ctx)
		cancel()
		if _, err = l.NativeBookmarks(cancelled); err == nil {
			t.Fatal("cancelled panel succeeded")
		}
		if _, err = l.NativeBookmarkLabels(cancelled); err == nil {
			t.Fatal("cancelled labels succeeded")
		}
		l.Release()
		if _, err = l.NativeBookmarks(ctx); err == nil {
			t.Fatal("released panel succeeded")
		}
		if _, err = l.NativeBookmarkLabels(ctx); err == nil {
			t.Fatal("released labels succeeded")
		}
	}
	if digest(t, root) != before {
		t.Fatal("panel wrote source")
	}
}
func TestNativeBookmarkViewRejectsMissingOwnerAVWithoutSourceRepair(t *testing.T) {
	ctx := context.Background()
	root := bookmarkViewSource(t)
	m := open(t, root, bookmarkViewOptions(root))
	l, err := m.Acquire(ctx, storeID, pageA)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Release()
	if err = l.QueueNativeTree(ctx, boxID, docID, "index"); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "pages", pageA, "data", "storage", "av", "20261026195100-dbase01.json")); err != nil {
		t.Fatal(err)
	}
	before := digest(t, root)
	if got, err := l.NativeBookmarks(ctx); err == nil || got != nil {
		t.Fatal("missing AV treated as successful blank title", got, err)
	}
	if digest(t, root) != before {
		t.Fatal("read repaired source")
	}
}
