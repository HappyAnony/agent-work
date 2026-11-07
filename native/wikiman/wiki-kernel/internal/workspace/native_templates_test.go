//go:build fts5

package workspace

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	av "wiki-kernel/internal/avmodel"
)

func TestNativeTemplatesOwnedSourcePreviewCloneAndNoPublication(t *testing.T) {
	ctx := context.Background()
	root := infoSource(t)
	for _, page := range []string{pageA, pageB} {
		creationSettings(t, root, page, "/Daily", "actual.md")
		base := filepath.Join(root, "pages", page, "data")
		a := &av.AttributeView{ID: getDocAV, Name: page, Spec: 4, ViewID: "view", KeyIDs: []string{"primary"}, KeyValues: []*av.KeyValues{{Key: &av.Key{ID: "primary", Name: page + " field", Type: av.KeyTypeBlock}}}, Views: []*av.View{{ID: "view", Name: "Table", LayoutType: av.LayoutTypeTable, PageSize: 32, Table: &av.LayoutTable{BaseLayout: &av.BaseLayout{}, Columns: []*av.ViewTableColumn{{BaseField: &av.BaseField{ID: "primary"}}}}}}}
		raw, _ := json.Marshal(a)
		if e := os.WriteFile(filepath.Join(base, "storage", "av", getDocAV+".json"), raw, 0600); e != nil {
			t.Fatal(e)
		}
		text := ".action{.title}\n\n<div data-type=\"NodeAttributeView\" data-av-id=\"" + getDocAV + "\" data-av-type=\"table\"></div>\n"
		if e := os.WriteFile(filepath.Join(base, "templates", "actual.md"), []byte(text), 0600); e != nil {
			t.Fatal(e)
		}
	}
	m := open(t, root, lifecycleOptions(root))
	before := digest(t, root)
	for _, page := range []string{pageA, pageB} {
		l := getDocLease(t, m, page)
		p, e := l.PlanNativeTemplate(ctx, NativeTemplateRequest{Action: "render", Path: "templates/actual.md", ID: docID, Preview: true})
		if e != nil || !strings.Contains(p.DOM, page+" field") || len(p.AttributeViews) != 0 {
			t.Fatal(p, e)
		}
		p, e = l.PlanNativeTemplate(ctx, NativeTemplateRequest{Action: "render", Path: "templates/actual.md", ID: docID})
		if e != nil || len(p.AttributeViews) != 1 {
			t.Fatal(p, e)
		}
		for id, a := range p.AttributeViews {
			if id == getDocAV || a.Name != page {
				t.Fatal(a)
			}
		}
		p, e = l.PlanNativeTemplate(ctx, NativeTemplateRequest{Action: "save", ID: docID, Name: "saved"})
		if e != nil || len(p.SourceWrites) != 1 || !strings.Contains(string(p.SourceWrites["data/templates/saved.md"]), page) {
			t.Fatal(p, e)
		}
		p, e = l.PlanNativeTemplate(ctx, NativeTemplateRequest{Action: "save", ID: docID, Name: "actual"})
		if e != nil || p.Code != 1 || len(p.SourceWrites) != 0 {
			t.Fatal(p, e)
		}
		p, e = l.PlanNativeTemplate(ctx, NativeTemplateRequest{Action: "icon", ID: docID, Source: ".action{.id}"})
		if e != nil || p.Text != docID {
			t.Fatal(p, e)
		}
		if _, e = l.PlanNativeTemplate(ctx, NativeTemplateRequest{Action: "render", ID: docID, Path: "../escape.md"}); e == nil {
			t.Fatal("outside path accepted")
		}
	}
	if digest(t, root) != before {
		t.Fatal("template preview published source")
	}
}
