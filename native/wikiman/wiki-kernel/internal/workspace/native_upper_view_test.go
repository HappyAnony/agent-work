//go:build fts5

package workspace

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"wiki-kernel/conf"
	"wiki-kernel/internal/indexdb"
)

func TestNativeFullTextUIAndTagAVUpperQueriesIsolateOwners(t *testing.T) {
	ctx := context.Background()
	root := fixture(t)
	const avID = "20261022090000-av00001"
	for _, page := range []string{pageA, pageB} {
		dir := filepath.Join(root, "pages", page, "data", "storage", "av")
		if e := os.MkdirAll(dir, 0700); e != nil {
			t.Fatal(e)
		}
		doc := map[string]any{"id": avID, "spec": 0, "future": "keep", "keyValues": []any{map[string]any{"key": map[string]any{"id": "same-key", "name": page + " AGI", "type": "text", "future": map[string]int{"keep": 9}}}, map[string]any{"key": map[string]any{"id": "same-rel", "name": "Relation", "type": "relation", "relation": map[string]string{"avID": avID}}}}}
		raw, _ := json.Marshal(doc)
		if e := os.WriteFile(filepath.Join(dir, avID+".json"), raw, 0600); e != nil {
			t.Fatal(e)
		}
	}
	before := digest(t, root)
	m := open(t, root, Options{Configuration: func(a, b string) (conf.AppData, error) {
		c, e := originalProjectionConfiguration(a, b)
		c.Tag = conf.NewTag()
		c.FileTree = conf.NewFileTree()
		return c, e
	}})
	for _, page := range []string{pageA, pageB} {
		l, e := m.Acquire(ctx, storeID, page)
		if e != nil {
			t.Fatal(e)
		}
		if e = l.QueueNativeTree(ctx, boxID, docID, "index"); e != nil {
			t.Fatal(e)
		}
		if e = l.FlushNativeMainQueue(ctx); e != nil {
			t.Fatal(e)
		}
		for _, group := range []int{0, 1} {
			got, e := l.SearchNativeBlockView(ctx, indexdb.BlockSearchRequest{Query: "knowledge", Page: 1, PageSize: 32, OrderBy: 5}, group)
			if e != nil || got.MatchedBlocks != 1 || len(got.Blocks) != 1 {
				t.Fatal(got, e)
			}
			block := got.Blocks[0]
			if group == 1 {
				if block.ID != docID || len(block.Children) != 1 {
					t.Fatal(block)
				}
				block = block.Children[0]
			}
			want := "Synthetic"
			if page == pageB {
				want = "Workspace B"
			}
			if !strings.Contains(block.Content, want) || !strings.Contains(block.Content, "<mark>") || block.Type != "NodeParagraph" || block.RefCount != 1 {
				t.Fatal("UI projection owner/count/type/markup", block, page)
			}
		}
		if e = l.WithNativeIndexes(ctx, func(b *indexdb.Bundle) error {
			db, _ := b.Database(indexdb.Main)
			_, e := db.Exec("INSERT INTO spans(id,block_id,root_id,box,path,content,markdown,type,ial) VALUES(?,?,?,?,?,?,?,?,?)", "upper-tag", blockID, docID, boxID, "/doc.docj", page+"/中文10", page, "tag", "")
			return e
		}); e != nil {
			t.Fatal(e)
		}
		tree, e := l.BuildNativeTags(ctx, true)
		if e != nil {
			t.Fatal(e)
		}
		found := false
		for _, n := range tree.Tags {
			if n.Label == page {
				found = true
				if len(n.Children) != 1 || n.Children[0].Count != 1 {
					t.Fatal(n)
				}
			}
		}
		if !found {
			t.Fatal("tag selected wrong owner", tree)
		}
		keys, e := l.NativeAttributeViewKeys(ctx, avID, "nonrelation", "AGI", nil)
		if e != nil || len(keys) != 1 || !strings.Contains(string(keys[0]), page) || !strings.Contains(string(keys[0]), "future") {
			t.Fatal(keys, e)
		}
		keys[0][0] = 'x'
		again, e := l.NativeAttributeViewKeys(ctx, avID, "ids", "", []string{"same-key", "same-key"})
		if e != nil || len(again) != 2 || again[0][0] != '{' {
			t.Fatal("key copy/order", again, e)
		}
		l.Release()
	}
	if digest(t, root) != before {
		t.Fatal("upper reads repaired or changed source")
	}
}
