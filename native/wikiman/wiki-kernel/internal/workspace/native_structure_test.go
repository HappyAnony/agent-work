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
	"wiki-kernel/internal/docview"
)

func structureSource(t *testing.T) string {
	t.Helper()
	root := getDocSource(t)
	for _, page := range []string{pageA, pageB} {
		p := filepath.Join(root, "pages", page, "data", boxID, docID+".docj")
		raw, e := os.ReadFile(p)
		if e != nil {
			t.Fatal(e)
		}
		var d map[string]interface{}
		if e = json.Unmarshal(raw, &d); e != nil {
			t.Fatal(e)
		}
		n := d["Children"].([]interface{})[0].(map[string]interface{})
		n["Type"] = "NodeHeading"
		n["HeadingLevel"] = 2
		n["Children"] = []interface{}{map[string]string{"Type": "NodeText", "Data": "New / heading " + page}}
		raw, _ = json.Marshal(d)
		if e = os.WriteFile(p, raw, 0600); e != nil {
			t.Fatal(e)
		}
	}
	return root
}
func TestNativeStructureOriginalConversionsAndNoPublication(t *testing.T) {
	root := structureSource(t)
	m := open(t, root, lifecycleOptions(root))
	before := digest(t, root)
	ctx := context.Background()
	for _, page := range []string{pageA, pageB} {
		l := getDocLease(t, m, page)
		ownerTerm := "OwnerTermA"
		if page == pageB {
			ownerTerm = "OwnerTermB"
		}
		p, e := l.PlanNativeStructure(ctx, doclife.StructureRequest{Action: "doc-to-heading", ID: getDocOther, TargetID: blockID, After: true})
		if e != nil || !p.DeleteSource || p.Target.ID != docID || !strings.Contains(p.Target.Root.Text(), ownerTerm) {
			t.Fatal(p, e)
		}
		if n := docview.GetNodeInTree(p.Target, getDocOther); n == nil || n.Type.String() != "NodeHeading" {
			t.Fatal("document identity not retained as heading", p)
		}
		p, e = l.PlanNativeStructure(ctx, doclife.StructureRequest{Action: "heading-to-doc", ID: blockID, TargetBox: boxID, TargetPath: "/"})
		if e != nil || p.DeleteSource || p.Target.ID != blockID || !strings.Contains(p.Target.Root.IALAttr("title"), page) || strings.Contains(p.Target.Root.IALAttr("title"), "/") || len(p.Effects) != 1 || p.Target.Root.FirstChild == nil {
			t.Fatal(p, e)
		}
		if docview.GetNodeInTree(p.Source, blockID) != nil {
			t.Fatal("heading remained in source candidate")
		}
	}
	if digest(t, root) != before {
		t.Fatal("structural candidates changed source")
	}
}
