//go:build fts5

package workspace

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/internal/configtypes"
	"wiki-kernel/internal/docquery"
	"wiki-kernel/internal/doctree"
)

const treeOther = "20261026120000-other01"

func documentTreeFixture(t *testing.T) string {
	t.Helper()
	root := fixture(t)
	for _, page := range []string{pageA, pageB} {
		base := filepath.Join(root, "pages", page, "data", boxID)
		if err := os.MkdirAll(filepath.Join(base, docID), 0700); err != nil {
			t.Fatal(err)
		}
		for _, x := range []struct{ id, folder, hidden string }{{treeOther, "", ""}, {"20261026130000-hide001", "", "true"}, {"20261026110000-child01", docID, ""}, {"20261026110001-child02", docID, "true"}} {
			doc := map[string]interface{}{"ID": x.id, "Type": "NodeDocument", "Properties": map[string]string{"id": x.id, "title": page + " " + x.id, "custom-hidden": x.hidden, "updated": "20261026140000"}, "Children": []interface{}{}}
			raw, _ := json.Marshal(doc)
			if err := os.WriteFile(filepath.Join(base, x.folder, x.id+".docj"), raw, 0600); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.MkdirAll(filepath.Join(base, ".wiki"), 0700); err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(map[string]int{docID: 2, treeOther: 1})
		if err := os.WriteFile(filepath.Join(base, ".wiki", "sort.json"), raw, 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(base, "stale.tmp"), []byte("keep tmp"), 0600); err != nil {
			t.Fatal(err)
		}
		old := time.Now().Add(-time.Hour)
		if err := os.Chtimes(filepath.Join(base, "stale.tmp"), old, old); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(base, "notes.txt"), []byte("{}"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}
func treeOptions(root string) Options {
	return Options{Configuration: func(_, page string) (conf.AppData, error) {
		c, _ := originalProjectionConfiguration("", page)
		c.FileTree = conf.NewFileTree()
		c.Lang = "en_US"
		return c, nil
	}, ConfigurationBusiness: func(_, page string) (*conf.Business, error) {
		raw, e := os.ReadFile("../../../wiki-app/appearance/langs/en_US.json")
		if e != nil {
			return nil, e
		}
		var lang struct {
			Time map[string]string `json:"_time"`
		}
		if e = json.Unmarshal(raw, &lang); e != nil {
			return nil, e
		}
		return conf.NewBusiness(func(context.Context) ([]conf.Notebook, error) {
			return []conf.Notebook{{ID: boxID, Name: page, SortMode: configtypes.SortModeFileTree}}, nil
		}, nil, nil, map[string]map[string]string{"en_US": lang.Time}), nil
	}}
}
func TestNativeDocumentTreeRealFilesNestedHiddenSortRefsAndNoWrites(t *testing.T) {
	ctx := context.Background()
	root := documentTreeFixture(t)
	before := digest(t, root)
	m := open(t, root, treeOptions(root))
	for _, page := range []string{pageA, pageB} {
		l, err := m.Acquire(ctx, storeID, page)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(l.Release)
		if err = l.QueueNativeTree(ctx, boxID, docID, "index"); err != nil {
			t.Fatal(err)
		}
		if err = l.FlushNativeMainQueue(ctx); err != nil {
			t.Fatal(err)
		}
		q := doctree.Request{Box: boxID, Path: "/", Sort: configtypes.SortModeUnassigned, Max: 1}
		got, total, err := l.NativeListDocumentTree(ctx, q, nil)
		if err != nil || len(got) != 1 || total != 2 || got[0].ID != treeOther || !strings.HasPrefix(got[0].Name, page) || got[0].HCtime == "" {
			t.Fatal(got, total, err)
		}
		q.Max = 10
		got, total, err = l.NativeListDocumentTree(ctx, q, nil)
		if err != nil || total != 2 || got[1].ID != docID || got[1].SubFileCount != 1 || got[1].Count != 1 {
			t.Fatal(got, total, err)
		}
		got[0].Name = "caller mutation"
		again, _, err := l.NativeListDocumentTree(ctx, q, nil)
		if err != nil || !strings.HasPrefix(again[0].Name, page) {
			t.Fatal(again, err)
		}
		q.ShowHidden = true
		got, total, err = l.NativeListDocumentTree(ctx, q, nil)
		if err != nil || total != 3 {
			t.Fatal(total, err)
		}
		for _, f := range got {
			if f.ID == docID && f.SubFileCount != 1 {
				t.Fatal("showHidden changed child count")
			}
		}
		q.ShowHidden = false
		q.Path = "/" + docID + ".docj"
		got, total, err = l.NativeListDocumentTree(ctx, q, nil)
		if err != nil || total != 1 || got[0].ID != "20261026110000-child01" {
			t.Fatal(got, total, err)
		}
		q.Path = "/" + treeOther + ".docj"
		got, total, err = l.NativeListDocumentTree(ctx, q, nil)
		if err != nil || total != 0 || got == nil {
			t.Fatal("missing children folder", got, total, err)
		}
		q.Path = "/"
		q.Flashcard = true
		if _, _, err = l.NativeListDocumentTree(ctx, q, nil); err == nil {
			t.Fatal("missing card services succeeded")
		}
		cards := &docquery.Flashcards{Open: func(context.Context) (bool, error) { return true, nil }, Count: func(_ context.Context, scope, id string) (docquery.Counts, error) {
			if id == docID {
				return docquery.Counts{Total: 2, New: 1}, nil
			}
			return docquery.Counts{}, nil
		}}
		got, total, err = l.NativeListDocumentTree(ctx, q, cards)
		if err != nil || total != 1 || got[0].FlashcardCount != 2 {
			t.Fatal(got, total, err)
		}
		cancelled, cancel := context.WithCancel(ctx)
		cancel()
		if _, _, err = l.NativeListDocumentTree(cancelled, q, cards); err == nil {
			t.Fatal("cancelled read succeeded")
		}
		l.Release()
		if _, _, err = l.NativeListDocumentTree(ctx, q, cards); err == nil {
			t.Fatal("released read succeeded")
		}
	}
	if digest(t, root) != before {
		t.Fatal("list wrote source or removed tmp")
	}
}
func TestNativeDocumentTreeRejectsTraversalSymlinksAndCorruptionWithoutRepair(t *testing.T) {
	for _, kind := range []string{"traversal", "symlink", "corrupt"} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			root := documentTreeFixture(t)
			base := filepath.Join(root, "pages", pageA, "data", boxID)
			m := open(t, root, treeOptions(root))
			l, err := m.Acquire(ctx, storeID, pageA)
			if err != nil {
				t.Fatal(err)
			}
			defer l.Release()
			if kind == "symlink" {
				target := filepath.Join(t.TempDir(), "safe")
				if err := os.WriteFile(target, []byte("test only"), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, filepath.Join(base, "link.docj")); err != nil {
					t.Fatal(err)
				}
			}
			if kind == "corrupt" {
				if err := os.WriteFile(filepath.Join(base, treeOther+".docj"), []byte("broken JSON"), 0600); err != nil {
					t.Fatal(err)
				}
			}

			q := doctree.Request{Box: boxID, Path: "/", Max: 10}
			if kind == "traversal" {
				q.Path = "/../"
			}
			if got, total, err := l.NativeListDocumentTree(ctx, q, nil); err == nil || got != nil || total != 0 {
				t.Fatal("bad source/path was accepted", got, total, err)
			}
			if kind == "corrupt" {
				raw, err := os.ReadFile(filepath.Join(base, treeOther+".docj"))
				if err != nil || string(raw) != "broken JSON" {
					t.Fatal("read moved or repaired corruption", err)
				}
			}
		})
	}
}
