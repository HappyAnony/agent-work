//go:build fts5

package workspace

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"wiki-kernel/internal/doctree"
)

func TestNativeDocumentSortCandidatesFixedMetadata(t *testing.T) {
	root := fixture(t)
	for i, page := range []string{pageA, pageB} {
		folder := filepath.Join(root, "pages", page, "data", boxID, ".wiki")
		if e := os.MkdirAll(folder, 0700); e != nil {
			t.Fatal(e)
		}
		raw, _ := json.Marshal(map[string]int{docID: i + 10})
		if e := os.WriteFile(filepath.Join(folder, "sort.json"), raw, 0600); e != nil {
			t.Fatal(e)
		}
	}
	before := digest(t, root)
	m := open(t, root, lifecycleOptions(root))
	ctx := context.Background()
	for i, page := range []string{pageA, pageB} {
		l, e := m.Acquire(ctx, storeID, page)
		if e != nil {
			t.Fatal(e)
		}
		plan, e := l.PlanNativeDocumentSort(ctx, doctree.SortMutation{Mode: "merge", Box: boxID, Values: map[string]int{docID: 3}})
		if e != nil {
			t.Fatal(e)
		}
		var old, next map[string]int
		json.Unmarshal(plan.Before, &old)
		json.Unmarshal(plan.After, &next)
		if old[docID] != i+10 || next[docID] != 3 || plan.Name != boxID+"/.wiki/sort.json" {
			t.Fatal(plan, old, next)
		}
		l.Release()
	}
	if digest(t, root) != before {
		t.Fatal("candidate source published")
	}
}
