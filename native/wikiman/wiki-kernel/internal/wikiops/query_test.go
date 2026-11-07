//go:build fts5

package wikiops

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	av "wiki-kernel/internal/avmodel"
	"wiki-kernel/internal/workspace"
)

const r03Box = "20261019090000-box0001"
const r03Doc = "20261019090001-doc0001"
const r03Block = "20261019090002-par0001"
const r03AV = "20261105090000-av00001"
const r03PageB = "1760000000000000003"

func r03Fixture(t *testing.T) (string, *workspace.WorkspaceManager, string) {
	t.Helper()
	root := t.TempDir()
	if e := os.CopyFS(root, os.DirFS("../../../fixtures/shared-wiki/valid")); e != nil {
		t.Fatal(e)
	}
	source := filepath.Join(root, "pages", testPage, "data")
	target := filepath.Join(root, "pages", r03PageB, "data")
	if e := os.MkdirAll(target, 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.CopyFS(target, os.DirFS(source)); e != nil {
		t.Fatal(e)
	}
	for _, page := range []string{testPage, r03PageB} {
		base := filepath.Join(root, "pages", page, "data")
		for _, dir := range []string{"templates", "widgets/sample"} {
			if e := os.MkdirAll(filepath.Join(base, dir), 0700); e != nil {
				t.Fatal(e)
			}
		}
		if e := os.WriteFile(filepath.Join(base, "templates", "sample.md"), []byte("template"), 0600); e != nil {
			t.Fatal(e)
		}
		if e := os.WriteFile(filepath.Join(base, "widgets", "sample", "widget.json"), []byte(`{"name":"sample","displayName":{"default":"Sample"},"author":"fixture","version":"1.0.0","minAppVersion":"0.0.0","description":{"default":"fixture"}}`), 0600); e != nil {
			t.Fatal(e)
		}

		if e := os.Mkdir(filepath.Join(base, r03Box, r03Doc), 0700); e != nil {
			t.Fatal(e)
		}
		file := filepath.Join(base, r03Box, r03Doc+".docj")
		raw, e := os.ReadFile(file)
		if e != nil {
			t.Fatal(e)
		}
		var ast map[string]json.RawMessage
		if e = json.Unmarshal(raw, &ast); e != nil {
			t.Fatal(e)
		}
		ast["UnknownFuture"] = json.RawMessage(`{"int":900719925474099312345}`)
		raw, _ = json.Marshal(ast)
		if page == r03PageB {
			raw = bytes.ReplaceAll(raw, []byte("Synthetic knowledge"), []byte("OtherOwner knowledge"))
		}
		if e = os.WriteFile(file, raw, 0600); e != nil {
			t.Fatal(e)
		}
		if e = os.MkdirAll(filepath.Join(base, "assets"), 0700); e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(filepath.Join(base, "assets", "owned.txt"), []byte("attachment "+page), 0600); e != nil {
			t.Fatal(e)
		}
		a := r03LayoutFixture(r03AV, page, av.CoverFromNone)
		raw, _ = json.Marshal(a)
		if e = os.MkdirAll(filepath.Join(base, "storage", "av"), 0700); e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(filepath.Join(base, "storage", "av", r03AV+".json"), raw, 0600); e != nil {
			t.Fatal(e)
		}
	}
	m := workspace.NewManager(workspace.Options{ReadDefaults: true})
	id, e := m.OpenStore(context.Background(), root)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if e := m.Drain(context.Background()); e != nil {
			t.Error(e)
		}
	})
	return root, m, id
}
func r03SourceHashes(t *testing.T, root string) map[string]string {
	all := snapshotFiles(t, root)
	for name := range all {
		if strings.HasPrefix(name, ".local/") {
			delete(all, name)
		}
	}
	return all
}

func TestR03QueryEveryRegisteredActionRealSourceAndReadOnly(t *testing.T) {
	root, m, id := r03Fixture(t)
	if dir := os.Getenv("WIKI_R03_EVIDENCE_DIR"); dir != "" {
		data, _ := json.MarshalIndent(QueryActions(), "", "  ")
		if e := os.WriteFile(filepath.Join(dir, "query-map.json"), append(data, '\n'), 0600); e != nil {
			t.Fatal(e)
		}
	}
	before := r03SourceHashes(t, root)
	samples := map[string]any{
		"templates.search": map[string]any{"Keyword": ""}, "widgets.search": map[string]any{"Keyword": ""},
		"document.assets": map[string]any{"ID": r03Doc}, "document.treeids": map[string]any{"Box": r03Box, "Path": "/"}, "references.block": map[string]any{"ID": r03Block}, "bookmarks.labels": map[string]any{}, "file.read": map[string]any{"Path": "assets/owned.txt"}, "file.list": map[string]any{"Path": "assets"},
		"blocks.recent": map[string]any{}, "notebook.info": map[string]any{"ID": r03Box}, "storage": map[string]any{"Action": "local.get"},
		"block.get":     map[string]any{"ID": r03Block},
		"document.info": map[string]any{"ID": r03Doc},
		"document.ast":  map[string]any{"BoxID": r03Box, "DocID": r03Doc},
		"asset.get":     map[string]any{"AssetName": "owned.txt"}, "asset.list": map[string]any{},
		"references.query": map[string]any{"Mode": "counts", "IDs": []string{r03Block}}, "references.invalid": map[string]any{"Page": 1, "PageSize": 20},
		"av.images": map[string]any{"ID": r03AV, "ViewID": "table"}, "av.mirrors": map[string]any{"ID": r03AV}, "block.avkeys": map[string]any{"ID": r03Block}, "block.attributes.batch": map[string]any{"IDs": []string{r03Doc, r03Block}},
		"document.view": map[string]any{"ID": r03Doc, "Size": 50}, "document.search": map[string]any{"Keyword": "knowledge"},
		"document.tree": map[string]any{"Box": r03Box, "Path": "/"}, "document.paths": map[string]any{"Mode": "id", "ID": r03Doc},
		"document.infos": map[string]any{"IDs": []string{r03Doc}, "References": true}, "document.statistics": map[string]any{"Mode": "tree", "IDs": []string{r03Doc}},
		"block.read": map[string]any{"Mode": "block", "ID": r03Block}, "block.dom": map[string]any{"IDs": []string{r03Block}},
		"block.position": map[string]any{"Mode": "breadcrumb", "ID": r03Block}, "block.relations": map[string]any{"Mode": "infos", "IDs": []string{r03Block}},
		"block.fold": map[string]any{"Mode": "state", "ID": r03Block}, "block.location": map[string]any{"ID": r03Block}, "block.attributes": map[string]any{"ID": r03Doc},
		"outline": map[string]any{"ID": r03Doc}, "references.search": map[string]any{"RootID": r03Doc, "Keyword": "knowledge", "BeforeLen": 12},
		"references.backlinks": map[string]any{"Mode": "links", "ID": r03Block}, "references.definitions": map[string]any{"ID": r03Block}, "references.text": map[string]any{"ID": r03Block}, "references.check": map[string]any{"IDs": []string{r03Block}},
		"references.embeds": map[string]any{"IncludeIDs": []string{r03Block}}, "graph": map[string]any{},
		"search": map[string]any{"Request": map[string]any{"Query": "knowledge", "Page": 1, "PageSize": 20}}, "sql": map[string]any{"SQL": "select id,content from blocks order by id"},
		"tags.search": map[string]any{"Keyword": "x"}, "tags.tree": map[string]any{}, "tags.blocks": map[string]any{"Keyword": "x"}, "tags.spans": map[string]any{"Kind": "path", "Value": ""}, "bookmarks": map[string]any{},
		"av.get": map[string]any{"ID": r03AV}, "av.name": map[string]any{"ID": r03AV}, "av.search": map[string]any{"Keyword": ""},
		"av.keys": map[string]any{"ID": r03AV, "Kind": "ids"}, "av.primary": map[string]any{"ID": r03AV, "Page": 1, "PageSize": 10}, "av.view": map[string]any{"ID": r03AV},
		"av.render": map[string]any{"ID": r03AV, "ViewID": "table", "Page": 1, "PageSize": 10}, "av.items": map[string]any{"ID": r03AV, "IDs": []string{r03Block}}, "av.bound": map[string]any{"ID": r03AV, "IDs": []string{r03Block}},
		"assets.names": map[string]any{"Keyword": "owned"}, "assets.references": map[string]any{}, "assets.search": map[string]any{"Query": "attachment", "Page": 1, "PageSize": 20},
		"configuration": map[string]any{}, "configuration.notebooks": map[string]any{}, "configuration.notebook": map[string]any{"ID": r03Box}, "configuration.emojis": map[string]any{}, "index.rebuild": map[string]any{},
	}
	// Detail IDs are disposable-index identities, obtained from actual search.
	raw, _ := json.Marshal(samples["assets.search"])
	v, e := Query(context.Background(), m, QueryRequest{1, id, testPage, "assets.search", raw})
	if e != nil {
		t.Fatal("asset index", e)
	}
	data, _ := json.Marshal(v.Value)
	var found struct{ Contents []struct{ ID string } }
	json.Unmarshal(data, &found)
	if len(found.Contents) != 1 {
		t.Fatal(string(data))
	}
	samples["assets.content"] = map[string]any{"ID": found.Contents[0].ID}
	for _, entry := range QueryActions() {
		t.Run(entry.Action, func(t *testing.T) {
			opts, ok := samples[entry.Action]
			if !ok {
				t.Fatal("action lacks real acceptance sample")
			}
			raw, _ := json.Marshal(opts)
			result, e := Query(context.Background(), m, QueryRequest{1, id, testPage, entry.Action, raw})
			if e != nil || result.SourceHash == "" || result.Value == nil {
				t.Fatal(result, e)
			}
			if entry.Original == "" || entry.Service == "" {
				t.Fatal("missing original/service map")
			}
		})
	}
	if !reflect.DeepEqual(before, r03SourceHashes(t, root)) {
		t.Fatal("read actions altered source")
	}
}

func TestR03QueryIdentityCancellationStrictSchemaNoPartial(t *testing.T) {
	root, m, id := r03Fixture(t)
	before := r03SourceHashes(t, root)
	ctx := context.Background()
	for _, page := range []string{testPage, r03PageB} {
		raw := json.RawMessage(`{"Request":{"Query":"knowledge","Page":1,"PageSize":20}}`)
		result, e := Query(ctx, m, QueryRequest{1, id, page, "search", raw})
		if e != nil {
			t.Fatal(e)
		}
		b, _ := json.Marshal(result.Value)
		want := "Synthetic"
		other := "OtherOwner"
		if page == r03PageB {
			want, other = other, want
		}
		if !strings.Contains(string(b), want) || strings.Contains(string(b), other) {
			t.Fatal("same ID cross-page", string(b))
		}
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	q := QueryRequest{1, id, testPage, "configuration", nil}
	if result, e := Query(canceled, m, q); !errors.Is(e, context.Canceled) || result.Value != nil {
		t.Fatal(result, e)
	}
	for _, raw := range []string{`{"protocolVersion":1,"storeId":"s","pageId":"` + testPage + `","action":"sql","options":{"SQL":"delete from blocks"}}`, `{"protocolVersion":1,"storeId":"s","pageId":"` + testPage + `","action":"configuration","options":{"root":"outside"}}`, `{"protocolVersion":1,"storeId":"s","pageId":"` + testPage + `","action":"configuration","options":null}`, `{"protocolVersion":1,"storeId":"s","pageId":"` + testPage + `","action":"configuration","options":{"x":1,"x":2}}`} {
		if _, e := DecodeQueryRequest(strings.NewReader(raw)); e == nil {
			t.Fatal("invalid query accepted", raw)
		}
	}
	if !reflect.DeepEqual(before, r03SourceHashes(t, root)) {
		t.Fatal("failed reads altered source")
	}
}
func r03LayoutFixture(id, page string, cover av.CoverFrom) *av.AttributeView {
	a := &av.AttributeView{Spec: 4, ID: id, Name: page, ViewID: "gallery", KeyValues: []*av.KeyValues{{Key: &av.Key{ID: "primary", Name: "Name", Type: av.KeyTypeBlock}}, {Key: &av.Key{ID: "number", Type: av.KeyTypeNumber}}, {Key: &av.Key{ID: "select", Name: "Status", Type: av.KeyTypeSelect, Options: []*av.SelectOption{{Name: "X"}, {Name: "Y"}, {Name: "Empty"}}}}, {Key: &av.Key{ID: "asset", Type: av.KeyTypeMAsset}}}}
	rows := []string{r03Block, "20261023090001-row0001", "20261023090002-row0002"}
	for i, id := range rows {
		a.KeyValues[0].Values = append(a.KeyValues[0].Values, &av.Value{ID: id, KeyID: "primary", BlockID: id, Type: av.KeyTypeBlock, IsDetached: i > 0, Block: &av.ValueBlock{ID: id, Content: page + fmt.Sprint(i), Updated: int64(i + 1)}})
		a.KeyValues[1].Values = append(a.KeyValues[1].Values, &av.Value{ID: id + "n", KeyID: "number", BlockID: id, Type: av.KeyTypeNumber, Number: &av.ValueNumber{Content: float64(i + 1), IsNotEmpty: true}})
		opt := "X"
		if i == 1 {
			opt = "Y"
		}
		a.KeyValues[2].Values = append(a.KeyValues[2].Values, &av.Value{ID: id + "s", KeyID: "select", BlockID: id, Type: av.KeyTypeSelect, MSelect: []*av.ValueSelect{{Content: opt}}})
		a.KeyValues[3].Values = append(a.KeyValues[3].Values, &av.Value{ID: id + "a", KeyID: "asset", BlockID: id, Type: av.KeyTypeMAsset, MAsset: []*av.ValueAsset{{Content: "assets/" + page + ".png"}}})
	}
	for _, typ := range []av.LayoutType{av.LayoutTypeTable, av.LayoutTypeGallery, av.LayoutTypeKanban} {
		v := &av.View{ID: string(typ), Name: page, LayoutType: typ, PageSize: 2, Sorts: []*av.ViewSort{{Column: "number", Order: av.SortOrderDesc}}}
		switch typ {
		case av.LayoutTypeTable:
			v.Table = av.NewLayoutTable()
			for _, k := range a.KeyValues {
				v.Table.Columns = append(v.Table.Columns, &av.ViewTableColumn{BaseField: &av.BaseField{ID: k.Key.ID}, Calc: &av.FieldCalc{Operator: av.CalcOperatorSum}})
			}
		case av.LayoutTypeGallery:
			v.Gallery = av.NewLayoutGallery()
			v.Gallery.CoverFrom = cover
			v.Gallery.CoverFromAssetKeyID = "asset"
			for _, k := range a.KeyValues {
				v.Gallery.CardFields = append(v.Gallery.CardFields, &av.ViewGalleryCardField{BaseField: &av.BaseField{ID: k.Key.ID}})
			}
		case av.LayoutTypeKanban:
			v.Kanban = av.NewLayoutKanban()
			v.Kanban.CoverFrom = cover
			v.Kanban.CoverFromAssetKeyID = "asset"
			for _, k := range a.KeyValues {
				v.Kanban.Fields = append(v.Kanban.Fields, &av.ViewKanbanField{BaseField: &av.BaseField{ID: k.Key.ID}})
			}
		}
		a.Views = append(a.Views, v)
	}
	return a
}

func TestR03QueryAllOriginalReferenceModesAndOldAVSpecsAcrossLayouts(t *testing.T) {
	root, m, id := r03Fixture(t)
	ctx := context.Background()
	// Put real AV mirror/row bindings in the source copy. No read-time repair or
	// mirror registration is required to discover them.
	base := filepath.Join(root, "pages", testPage, "data")
	file := filepath.Join(base, r03Box, r03Doc+".docj")
	raw, e := os.ReadFile(file)
	if e != nil {
		t.Fatal(e)
	}
	var d map[string]any
	if e = json.Unmarshal(raw, &d); e != nil {
		t.Fatal(e)
	}
	children := d["Children"].([]any)
	children[0].(map[string]any)["Properties"].(map[string]any)[av.NodeAttrNameAvs] = r03AV
	mirror := "20261105095000-mirror1"
	d["Children"] = append(children, map[string]any{"ID": mirror, "Type": "NodeAttributeView", "AttributeViewID": r03AV, "Properties": map[string]string{"id": mirror}})
	raw, _ = json.Marshal(d)
	if e = os.WriteFile(file, raw, 0600); e != nil {
		t.Fatal(e)
	}
	call := func(action string, opts any) QueryResult {
		t.Helper()
		raw, _ := json.Marshal(opts)
		v, e := Query(ctx, m, QueryRequest{1, id, testPage, action, raw})
		if e != nil {
			t.Fatal(action, e)
		}
		return v
	}
	for _, mode := range []string{"counts", "root-counts", "root-children-counts", "duplicated-roots", "child-definitions", "child-definition-ids", "ref-ids", "pair", "block-children", "definition", "definition-roots", "referring-roots", "recent", "annotation", "definitions-by-text"} {
		t.Run(mode, func(t *testing.T) {
			call("references.query", map[string]any{"Mode": mode, "ID": r03Block, "OtherID": r03Block, "IDs": []string{r03Block}, "Keyword": "knowledge"})
		})
	}
	for spec := 0; spec <= 4; spec++ {
		a := r03LayoutFixture(r03AV, testPage, av.CoverFromNone)
		a.Spec = spec
		for _, kv := range a.KeyValues {
			for _, v := range kv.Values {
				for _, asset := range v.MAsset {
					asset.Type = av.AssetTypeImage
				}
			}
		}
		raw, _ = json.Marshal(a)
		var doc map[string]json.RawMessage
		json.Unmarshal(raw, &doc)
		doc["FutureNumber"] = json.RawMessage(`900719925474099312345`)
		raw, _ = json.Marshal(doc)
		source := filepath.Join(base, "storage", "av", r03AV+".json")
		if e = os.WriteFile(source, raw, 0600); e != nil {
			t.Fatal(e)
		}
		before := r03SourceHashes(t, root)
		for _, layout := range []string{"table", "gallery", "kanban"} {
			call("av.render", map[string]any{"ID": r03AV, "ViewID": layout, "Page": 1, "PageSize": 10})
			value := call("av.images", map[string]any{"ID": r03AV, "ViewID": layout})
			images, ok := value.Value.([]string)
			if !ok || len(images) != 3 {
				t.Fatal(spec, layout, value)
			}
		}
		panel := call("block.avkeys", map[string]any{"ID": r03Block})
		values, ok := panel.Value.([]workspace.NativeBlockAVKeys)
		if !ok || len(values) != 1 || len(values[0].KeyValues) != 4 || len(values[0].BlockIDs) != 1 || values[0].BlockIDs[0] != mirror {
			t.Fatal("panel/mirror projection", panel)
		}
		sourceRead := call("av.get", map[string]any{"ID": r03AV})
		original, ok := sourceRead.Value.(json.RawMessage)
		if !ok || !bytes.Contains(original, []byte("900719925474099312345")) {
			t.Fatal("unknown AV field lost", sourceRead)
		}
		if !reflect.DeepEqual(before, r03SourceHashes(t, root)) {
			t.Fatal("old AV upgraded or repaired on read", spec)
		}
	}
}

func TestR03RawFutureNodesAndConfigurationDoNotRequireIndexingOrRepair(t *testing.T) {
	root, m, id := r03Fixture(t)
	file := filepath.Join(root, "pages", testPage, "data", r03Box, r03Doc+".docj")
	raw, e := os.ReadFile(file)
	if e != nil {
		t.Fatal(e)
	}
	var d map[string]json.RawMessage
	if e = json.Unmarshal(raw, &d); e != nil {
		t.Fatal(e)
	}
	d["Children"] = json.RawMessage(`[{"ID":"20261105100000-future1","Type":"NodeFutureOpaque","Future":{"big":900719925474099312345}}]`)
	raw, _ = json.Marshal(d)
	if e = os.WriteFile(file, raw, 0600); e != nil {
		t.Fatal(e)
	}
	before := r03SourceHashes(t, root)
	opts, _ := json.Marshal(map[string]string{"BoxID": r03Box, "DocID": r03Doc})
	value, e := Query(context.Background(), m, QueryRequest{1, id, testPage, "document.ast", opts})
	if e != nil {
		t.Fatal(e)
	}
	got, ok := value.Value.(json.RawMessage)
	if !ok || !bytes.Equal(got, raw) {
		t.Fatal("opaque source was rewritten", value)
	}
	if _, e = Query(context.Background(), m, QueryRequest{1, id, testPage, "configuration", nil}); e != nil {
		t.Fatal("configuration unnecessarily indexed opaque content", e)
	}
	if !reflect.DeepEqual(before, r03SourceHashes(t, root)) {
		t.Fatal("future source changed")
	}
}

func TestR03EmptyWorkspaceFileReadsFailWithoutInitializingData(t *testing.T) {
	root, m, id := r03Fixture(t)
	const page = "1760000000000000004"
	before := r03SourceHashes(t, root)
	for _, action := range []string{"file.read", "file.list"} {
		opts := json.RawMessage(`{"Path":"assets/missing.txt"}`)
		result, e := Query(context.Background(), m, QueryRequest{1, id, page, action, opts})
		if e == nil || result.Value != nil {
			t.Fatal(action, result, e)
		}
	}
	if !reflect.DeepEqual(before, r03SourceHashes(t, root)) {
		t.Fatal("missing data was initialized")
	}
}

func TestR03CorruptNotebookMetadataReturnsFailureWithoutRepairOrPartial(t *testing.T) {
	root, m, id := r03Fixture(t)
	file := filepath.Join(root, "pages", testPage, "data", r03Box, ".wiki", "conf.json")
	if e := os.WriteFile(file, []byte(`{"broken":`), 0600); e != nil {
		t.Fatal(e)
	}
	before := r03SourceHashes(t, root)
	for _, action := range []string{"configuration.notebooks", "document.search"} {
		result, e := Query(context.Background(), m, QueryRequest{1, id, testPage, action, nil})
		if e == nil || result.Value != nil {
			t.Fatal("corrupt metadata hidden as a successful partial result", action, result, e)
		}
	}
	if !reflect.DeepEqual(before, r03SourceHashes(t, root)) {
		t.Fatal("read repaired corrupt metadata")
	}
}
