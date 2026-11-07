//go:build fts5

package model

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"wiki-kernel/internal/doctree"
	"wiki-kernel/util"
)

func TestDocumentSortActualReorderMergeRemoveAndFailure(t *testing.T) {
	c, _, _, doc, _ := referenceModelFixture(t)
	box := filepath.Base(filepath.Dir(doc))
	id := util.GetTreeID(doc)
	other := "20261028140001-refdoc1"
	if other == id {
		other = "20261028140003-refdoc2"
	}
	unrelated := "20261104100000-other01"
	filename := archiveWrite(t, util.DataDir, box+"/.siyuan/sort.json", []byte(`{"`+unrelated+`":88}`))
	paths := []string{"/" + id + ".docj"}
	c.persistenceReadOnly = true
	if e := ChangeFileTreeSortContext(context.Background(), box, paths); e == nil {
		t.Fatal("readonly sort written")
	}
	c.persistenceReadOnly = false
	if e := ChangeFileTreeSortContext(context.Background(), box, paths); e != nil {
		t.Fatal(e)
	}
	raw, e := os.ReadFile(filename)
	if e != nil {
		t.Fatal(e)
	}
	values := map[string]int{}
	if e = json.Unmarshal(raw, &values); e != nil {
		t.Fatal(e)
	}
	if values[id] != 1 || values[other] != 2 || values[unrelated] != 88 {
		t.Fatal(values)
	}
	if e = mutateDocumentSortContext(context.Background(), doctree.SortMutation{Mode: "merge", Box: box, Values: map[string]int{id: -1}}, false); e != nil {
		t.Fatal(e)
	}
	if e = mutateDocumentSortContext(context.Background(), doctree.SortMutation{Mode: "remove", Box: box, Remove: []string{other}}, false); e != nil {
		t.Fatal(e)
	}
	raw, _ = os.ReadFile(filename)
	values = map[string]int{}
	json.Unmarshal(raw, &values)
	if values[id] != -1 || values[unrelated] != 88 {
		t.Fatal(values)
	}
	if _, ok := values[other]; ok {
		t.Fatal(values)
	}
	if e = os.WriteFile(filename, []byte("corrupt"), 0600); e != nil {
		t.Fatal(e)
	}
	if e = ChangeFileTreeSortContext(context.Background(), box, paths); e == nil {
		t.Fatal("corrupt sort overwritten")
	}
	raw, _ = os.ReadFile(filename)
	if !bytes.Equal(raw, []byte("corrupt")) {
		t.Fatal("failed sort changed source")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if e = ChangeFileTreeSortContext(ctx, box, paths); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
}
