//go:build fts5

package workspace

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeAssetRenameCandidatesOwnBothSourceKinds(t *testing.T) {
	root := fixture(t)
	for _, page := range []string{pageA, pageB} {
		doc := filepath.Join(root, "pages", page, "data", boxID, docID+".docj")
		raw, e := os.ReadFile(doc)
		if e != nil {
			t.Fatal(e)
		}
		var d map[string]interface{}
		if e = json.Unmarshal(raw, &d); e != nil {
			t.Fatal(e)
		}
		d["Properties"].(map[string]interface{})["custom-data-assets-test"] = "assets/fixture.svg"
		raw, _ = json.Marshal(d)
		if e = os.WriteFile(doc, raw, 0600); e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(filepath.Join(root, "pages", page, "data/assets/fixture.svg"), []byte(page), 0600); e != nil {
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
		plan, e := l.PlanNativeAssetRename(ctx, "assets/fixture.svg", "renamed")
		if e != nil || len(plan.Changes) != 1 || string(plan.CopyData) != page || plan.CopyFrom != "assets/fixture.svg" || !strings.Contains(string(plan.Changes[0].After), plan.NewPath) {
			t.Fatal(plan, e)
		}
	}
	if digest(t, root) != before {
		t.Fatal("candidate published source")
	}
}
