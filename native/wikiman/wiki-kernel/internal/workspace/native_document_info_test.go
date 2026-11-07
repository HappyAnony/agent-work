//go:build fts5

package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/88250/lute/parse"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	av "wiki-kernel/internal/avmodel"
	"wiki-kernel/internal/docquery"
	"wiki-kernel/internal/indexdb"
)

const infoList = "20261027190000-list001"
const infoItem = "20261027190001-item001"
const infoChild = "20261027190100-child01"

func infoSource(t *testing.T) string {
	t.Helper()
	root := getDocSource(t)
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
		props := d["Properties"].(map[string]interface{})
		props["custom-avs"] = getDocAV
		props["icon"] = "book"
		props["scroll"] = `{"zoomInId":"` + blockID + `","startId":"missing"}`
		children := d["Children"].([]interface{})
		ref := children[len(children)-1].(map[string]interface{})
		ref["Children"] = append(ref["Children"].([]interface{}), map[string]interface{}{"Type": "NodeTextMark", "TextMarkType": "block-ref", "TextMarkBlockRefID": "20261019090003-cod0001", "TextMarkBlockRefSubtype": "d", "TextMarkTextContent": "other reference"})
		li := map[string]interface{}{"ID": infoItem, "Type": "NodeListItem", "Properties": map[string]string{"id": infoItem}, "ListData": map[string]interface{}{"Typ": 0}, "Children": []interface{}{ref}}
		children[len(children)-1] = map[string]interface{}{"ID": infoList, "Type": "NodeList", "Properties": map[string]string{"id": infoList}, "ListData": map[string]interface{}{"Typ": 0}, "Children": []interface{}{li}}
		d["Children"] = children
		raw, _ = json.Marshal(d)
		if e = os.WriteFile(file, raw, 0600); e != nil {
			t.Fatal(e)
		}
		dir := filepath.Join(root, "pages", page, "data", boxID, docID)
		if e = os.MkdirAll(filepath.Join(dir, "counted.docj"), 0700); e != nil {
			t.Fatal(e)
		}
		child := map[string]interface{}{"ID": infoChild, "Type": "NodeDocument", "Properties": map[string]string{"id": infoChild, "title": "Child"}, "Children": []interface{}{map[string]interface{}{"ID": "20261027190101-par0001", "Type": "NodeParagraph", "Properties": map[string]string{"id": "20261027190101-par0001"}, "Children": []interface{}{map[string]string{"Type": "NodeText", "Data": "child"}}}}}
		raw, _ = json.Marshal(child)
		if e = os.WriteFile(filepath.Join(dir, infoChild+".docj"), raw, 0600); e != nil {
			t.Fatal(e)
		}
	}
	return root
}

func TestNativeDocumentInfoRealOwnersPromotionScrollAndBatchRules(t *testing.T) {
	ctx := context.Background()
	root := infoSource(t)
	before := digest(t, root)
	m := open(t, root, getDocOptions())
	a, b := getDocLease(t, m, pageA), getDocLease(t, m, pageB)
	queueGetDoc(t, a)
	queueGetDoc(t, b)
	for _, l := range []*Lease{a, b} {
		info, e := l.NativeDocInfo(ctx, docID)
		if e != nil || info.Name != "Title "+l.PageID() || info.Icon != "book" || info.SubFileCount != 2 || info.RefCount != 1 || !reflect.DeepEqual(info.RefIDs, []string{infoItem}) || len(info.AttrViews) != 1 || info.AttrViews[0].Name != l.PageID()+" database" || info.IAL["scroll"] == "" {
			t.Fatal(info, e)
		}
		if l == a && b.workspace.resources.contentQueue.Pending() != 2 {
			t.Fatal("A drained B")
		}
		rows, e := l.NativeDocsInfo(ctx, []string{docID, "missing", blockID, docID}, false, false)
		if e != nil || len(rows) != 3 || rows[1].ID != blockID || rows[1].RootID != docID || rows[0].RefIDs != nil || rows[0].AttrViews != nil {
			t.Fatal(rows, e)
		}
		info.IAL["title"] = "changed"
		rows[0].IAL["title"] = "changed"
		again, e := l.NativeDocInfo(ctx, docID)
		if e != nil || again.Name != "Title "+l.PageID() {
			t.Fatal(again, e)
		}
		rows, e = l.NativeDocsInfo(ctx, []string{docID}, true, true)
		if e != nil || len(rows) != 1 || rows[0].RefCount != 1 || rows[0].AttrViews[0].Name != l.PageID()+" database" {
			t.Fatal(rows, e)
		}
		// Preserve the original bulk reader's directory-failure behavior. It does
		// not silently mkdir a leaf's missing children directory.
		if rows, e = l.NativeDocsInfo(ctx, []string{docID, getDocOther}, false, false); e == nil || rows != nil {
			t.Fatal("batch partial success", rows, e)
		}
		if info, e = l.NativeDocInfo(ctx, getDocOther); e != nil || info.SubFileCount != 0 {
			t.Fatal(info, e)
		}
	}
	if digest(t, root) != before {
		t.Fatal("metadata read changed source")
	}
}

func TestNativeDocumentInfoFailuresCancellationAndOwnerScrollRegistry(t *testing.T) {
	ctx := context.Background()
	root := infoSource(t)
	m := open(t, root, getDocOptions())
	l := getDocLease(t, m, pageA)
	queueGetDoc(t, l)
	if _, e := l.NativeDocInfo(ctx, docID); e != nil {
		t.Fatal(e)
	}
	if e := l.WithNativeIndexes(ctx, func(b *indexdb.Bundle) error {
		db, e := b.Database(indexdb.BlockTree)
		if e != nil {
			return e
		}
		_, e = db.ExecContext(ctx, "DELETE FROM blocktrees WHERE id=?", blockID)
		return e
	}); e != nil {
		t.Fatal(e)
	}
	info, e := l.NativeDocInfo(ctx, docID)
	if e != nil || info.IAL["scroll"] != "" {
		t.Fatal("scroll used foreign/global registry", info, e)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if r, e := l.NativeDocInfo(canceled, docID); !errors.Is(e, context.Canceled) || r != nil {
		t.Fatal(r, e)
	}
	if _, e = l.NativeDocInfo(ctx, "missing"); !errors.Is(e, ErrDocument) {
		t.Fatal(e)
	}
	if e = os.Remove(filepath.Join(root, "pages", pageA, "data", "storage", "av", getDocAV+".json")); e != nil {
		t.Fatal(e)
	}
	if r, e := l.NativeDocInfo(ctx, docID); e == nil || r != nil {
		t.Fatal("missing AV fallback", r, e)
	}
	l.Release()
	if _, e = l.NativeDocsInfo(ctx, nil, false, false); e == nil {
		t.Fatal("released lease accepted")
	}
}

func TestNativeDocumentStatisticsRealAVAndNoImplicitFlush(t *testing.T) {
	ctx := context.Background()
	root := infoSource(t)
	for _, page := range []string{pageA, pageB} {
		file := filepath.Join(root, "pages", page, "data", boxID, docID+".docj")
		raw, _ := os.ReadFile(file)
		var d map[string]interface{}
		_ = json.Unmarshal(raw, &d)
		d["Children"] = append(d["Children"].([]interface{}), map[string]interface{}{"ID": "20261027190200-avnode1", "Type": "NodeAttributeView", "AttributeViewID": getDocAV, "Properties": map[string]string{"id": "20261027190200-avnode1"}})
		raw, _ = json.Marshal(d)
		if e := os.WriteFile(file, raw, 0600); e != nil {
			t.Fatal(e)
		}
		text := "Short"
		if page == pageB {
			text = "Much longer independent knowledge with several additional words"
		}
		a := &av.AttributeView{ID: getDocAV, Spec: 4, Name: page + " database", ViewID: "view", Views: []*av.View{{ID: "view", LayoutType: av.LayoutTypeTable, Table: &av.LayoutTable{BaseLayout: &av.BaseLayout{}, Columns: []*av.ViewTableColumn{{BaseField: &av.BaseField{ID: "primary"}}}}}}, KeyValues: []*av.KeyValues{
			{Key: &av.Key{ID: "primary", Type: av.KeyTypeBlock}, Values: []*av.Value{{ID: "primary-value", KeyID: "primary", BlockID: blockID, Type: av.KeyTypeBlock, Block: &av.ValueBlock{ID: blockID, Content: "Bound"}}}},
			{Key: &av.Key{ID: "text", Type: av.KeyTypeText}, Values: []*av.Value{{ID: "text-value", KeyID: "text", BlockID: blockID, Type: av.KeyTypeText, Text: &av.ValueText{Content: text}}}},
			{Key: &av.Key{ID: "url", Type: av.KeyTypeURL}, Values: []*av.Value{{ID: "url-value", KeyID: "url", BlockID: blockID, Type: av.KeyTypeURL, URL: &av.ValueURL{Content: "https://example.test"}}}},
		}}
		raw, _ = json.Marshal(a)
		if e := os.WriteFile(filepath.Join(root, "pages", page, "data", "storage", "av", getDocAV+".json"), raw, 0600); e != nil {
			t.Fatal(e)
		}
	}
	before := digest(t, root)
	opts := getDocOptions()
	opts.NativeContent = bookmarkViewOptions(root).NativeContent
	m := open(t, root, opts)
	var counts []int
	for _, page := range []string{pageA, pageB} {
		l := getDocLease(t, m, page)
		queueGetDoc(t, l)
		tree, e := l.NativeDocumentStatistics(ctx, docquery.StatRequest{Mode: "tree", IDs: []string{blockID}})
		if e != nil || tree.LinkCount != 1 || tree.RefCount != 3 || tree.BlockCount != 6 {
			t.Fatal(tree, e)
		}
		counts = append(counts, tree.RuneCount)
		if l.workspace.resources.contentQueue.Pending() != 2 {
			t.Fatal("stats implicitly flushed main queue")
		}
		blocks, e := l.NativeDocumentStatistics(ctx, docquery.StatRequest{Mode: "blocks", IDs: []string{blockID, docID, blockID, "missing"}})
		if e != nil || blocks.BlockCount != 4 || blocks.LinkCount != 0 {
			t.Fatal(blocks, e)
		}
		content, e := l.NativeDocumentStatistics(ctx, docquery.StatRequest{Mode: "content", Content: `<div data-type="NodeParagraph"><div contenteditable="true">Hello 世界</div></div>`})
		if e != nil || content.BlockCount != 0 || content.RuneCount == 0 {
			t.Fatal(content, e)
		}
		var actual *parse.Tree
		if e = l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
			var e error
			actual, e = l.workspace.resources.locatedTree(ctx, l.workspace, boxID, docID)
			return e
		}); e != nil {
			t.Fatal(e)
		}
		_, _, _, _, baseRefs := actual.Root.Stat()
		if tree.RefCount != baseRefs+1 {
			t.Fatal("AV bound reference missing", tree)
		}
	}
	if counts[1] <= counts[0] {
		t.Fatal("AV stats crossed owners", counts)
	}
	if digest(t, root) != before {
		t.Fatal("stats wrote source")
	}
	l := getDocLease(t, m, pageA)
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if v, e := l.NativeDocumentStatistics(canceled, docquery.StatRequest{Mode: "tree", IDs: []string{docID}}); !errors.Is(e, context.Canceled) || v != nil {
		t.Fatal(v, e)
	}
	if e := os.Remove(filepath.Join(root, "pages", pageA, "data", "storage", "av", getDocAV+".json")); e != nil {
		t.Fatal(e)
	}
	if v, e := l.NativeDocumentStatistics(ctx, docquery.StatRequest{Mode: "tree", IDs: []string{docID}}); e == nil || v != nil {
		t.Fatal("missing AV stats fallback", v, e)
	}
	l.Release()
	if _, e := l.NativeDocumentStatistics(ctx, docquery.StatRequest{Mode: "blocks"}); e == nil {
		t.Fatal("released statistics accepted")
	}
}
