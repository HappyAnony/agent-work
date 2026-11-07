//go:build fts5

package workspace

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"wiki-kernel/internal/doclife"
)

func TestNativeHeadingOperationsMoveAndAppendActualNoPublication(t *testing.T) {
	ctx := context.Background()
	root := structureSource(t)
	const second = "20261104190001-head001"
	for _, page := range []string{pageA, pageB} {
		file := filepath.Join(root, "pages", page, "data", boxID, docID+".docj")
		raw, e := os.ReadFile(file)
		if e != nil {
			t.Fatal(e)
		}
		var d map[string]interface{}
		if e = json.Unmarshal(raw, &d); e != nil {
			t.Fatal(e)
		}
		d["Children"] = append(d["Children"].([]interface{}), map[string]interface{}{"ID": second, "Type": "NodeHeading", "HeadingLevel": 2, "Properties": map[string]string{"id": second}, "Children": []interface{}{map[string]string{"Type": "NodeText", "Data": "second " + page}}})
		raw, _ = json.Marshal(d)
		if e = os.WriteFile(file, raw, 0600); e != nil {
			t.Fatal(e)
		}
	}
	before := digest(t, root)
	m := open(t, root, lifecycleOptions(root))
	for _, page := range []string{pageA, pageB} {
		l := getDocLease(t, m, page)
		for _, mode := range []string{"insert", "delete", "level"} {
			v, e := l.PlanNativeHeadingOperations(ctx, doclife.HeadingOperationsRequest{Mode: mode, ID: blockID, Level: 3})
			if e != nil || !v.Plan.Changed || len(v.Plan.Do) == 0 || len(v.SourceHashes) != 1 {
				t.Fatal(page, mode, v, e)
			}
		}
		moved, e := l.PlanNativeHeadingMove(ctx, doclife.HeadingMoveRequest{ID: blockID, PreviousID: second})
		if e != nil || !moved.Changed || moved.Tree.Root.FirstChild.ID == blockID || len(moved.SourceHashes) != 1 {
			t.Fatal(moved, e)
		}
		appended, e := l.PlanNativeAppendHeadingChildren(ctx, blockID, `<div data-node-id="20261104190002-par0002" data-type="NodeParagraph"><div contenteditable="true">native appended</div></div>`)
		if e != nil || !strings.Contains(appended.Tree.Root.Text(), "native appended") || len(appended.SourceHashes) != 1 {
			t.Fatal(appended, e)
		}
	}
	if digest(t, root) != before {
		t.Fatal("candidate published source")
	}
}
