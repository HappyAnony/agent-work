//go:build fts5

package model

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/88250/lute/parse"
	"golang.org/x/net/html"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
	"wiki-kernel/conf"
	"wiki-kernel/internal/docquery"
	"wiki-kernel/internal/wikiops"
	"wiki-kernel/internal/workspace"
	"wiki-kernel/util"
)

// This compares the actual inherited model+SQL+filesys pipeline against native
// wikiops on an equivalent source copy. Neither side is a mocked query result.
func TestR03OriginalAndNativeReadDifferentialRealEquivalentSources(t *testing.T) {
	c, _, def, oldFile, before := referenceModelFixture(t)
	c.Export = conf.NewExport()
	c.persistenceReadOnly = true
	ctx := context.Background()
	root := t.TempDir()
	if e := os.CopyFS(root, os.DirFS("../../fixtures/shared-wiki/valid")); e != nil {
		t.Fatal(e)
	}
	const page = "1760000000000000002"
	const box = "20261028140000-refbox1"
	const doc = "20261028140001-refdoc1"
	dest := filepath.Join(root, "pages", page, "data", box)
	if e := os.MkdirAll(dest, 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.CopyFS(dest, os.DirFS(filepath.Join(util.DataDir, box))); e != nil {
		t.Fatal(e)
	}
	m := workspace.NewManager(workspace.Options{ReadDefaults: true, Configuration: func(string, string) (conf.AppData, error) { return c.AppData.Clone() }, ConfigurationBusiness: func(string, string) (*conf.Business, error) { return c.business, nil }})
	store, e := m.OpenStore(ctx, root)
	if e != nil {
		t.Fatal(e)
	}
	defer m.Drain(ctx)
	invoke := func(action string, opts any) any {
		t.Helper()
		raw, _ := json.Marshal(opts)
		v, e := wikiops.Query(ctx, m, wikiops.QueryRequest{ProtocolVersion: 1, StoreID: store, PageID: page, Action: action, Options: raw})
		if e != nil {
			t.Fatal(action, e)
		}
		return v.Value
	}
	equal := func(name string, a, b any) {
		t.Helper()
		aa, e := json.Marshal(a)
		if e != nil {
			t.Fatal(e)
		}
		bb, e := json.Marshal(b)
		if e != nil {
			t.Fatal(e)
		}
		var av, bv any
		json.Unmarshal(aa, &av)
		json.Unmarshal(bb, &bv)
		if !reflect.DeepEqual(av, bv) {
			t.Fatalf("%s mismatch\noriginal %s\nnative   %s", name, aa, bb)
		}
	}
	block, e := GetBlockContext(ctx, def)
	if e != nil {
		t.Fatal(e)
	}
	equal("block", block, invoke("block.get", map[string]any{"ID": def}))
	children, e := GetChildBlocksContext(ctx, doc)
	if e != nil {
		t.Fatal(e)
	}
	equal("children", children, invoke("block.read", docquery.BlocksRequest{Mode: "children", ID: doc}).(docquery.BlocksResult).Children)
	md, e := GetBlockKramdownContext(ctx, def, "md")
	if e != nil {
		t.Fatal(e)
	}
	equal("markdown", r03CanonicalIAL(md), r03CanonicalIAL(invoke("block.read", docquery.BlocksRequest{Mode: "md", ID: def}).(docquery.BlocksResult).Text))
	dom, e := GetBlockDOMsContext(ctx, []string{def}, false)
	if e != nil {
		t.Fatal(e)
	}
	nativeDOM := invoke("block.dom", map[string]any{"IDs": []string{def}}).(map[string]string)
	for id, value := range dom {
		equal("DOM", r03CanonicalDOM(value), r03CanonicalDOM(nativeDOM[id]))
	}
	pos, e := BlockPositionContext(ctx, docquery.PositionRequest{Mode: "index", ID: def})
	if e != nil {
		t.Fatal(e)
	}
	equal("position", pos, invoke("block.position", docquery.PositionRequest{Mode: "index", ID: def}))
	relations, e := BlockRelationsContext(ctx, docquery.RelationRequest{Mode: "infos", IDs: []string{def, doc}})
	if e != nil {
		t.Fatal(e)
	}
	equal("relations", relations, invoke("block.relations", docquery.RelationRequest{Mode: "infos", IDs: []string{def, doc}}))
	fold, e := BlockFoldContext(ctx, docquery.FoldRequest{Mode: "state", ID: def})
	if e != nil {
		t.Fatal(e)
	}
	equal("fold", fold, invoke("block.fold", docquery.FoldRequest{Mode: "state", ID: def}))
	info, e := GetDocInfoContext(ctx, def)
	if e != nil {
		t.Fatal(e)
	}
	equal("info", info, invoke("document.info", map[string]any{"ID": def}))
	outlines, e := OutlineContext(ctx, doc, false)
	if e != nil {
		t.Fatal(e)
	}
	equal("outline", outlines, invoke("outline", map[string]any{"ID": doc}))
	text, e := GetBlockRefTextContext(ctx, def)
	if e != nil {
		t.Fatal(e)
	}
	equal("reference text", text, invoke("references.text", map[string]any{"ID": def}))
	for _, order := range []int{1, 2} {
		for _, method := range []int{0, 1, 2, 3} {
			query := "definition"
			if method == 2 {
				query = "select * from blocks where box='" + box + "' order by id"
			}
			blocks, n, roots, pages, mode, e := FullTextSearchBlockContext(ctx, query, []string{box}, nil, nil, method, order, 0, 1, 16)
			if e != nil {
				t.Fatal(method, e)
			}
			got := invoke("search", map[string]any{"Request": map[string]any{"Query": query, "Boxes": []string{box}, "Method": method, "OrderBy": order, "Page": 1, "PageSize": 16}}).(workspace.NativeSearchView)
			equal("search blocks", blocks, got.Blocks)
			equal("search counts", []any{n, roots, pages, mode}, []any{got.MatchedBlocks, got.MatchedRoots, got.PageCount, got.DocumentMode})
		}
	}
	raw, e := os.ReadFile(oldFile)
	if e != nil || !reflect.DeepEqual(raw, before) {
		t.Fatal("oracle read changed source", e)
	}
}

// Original dataparser reads Properties from a Go map; independent parses may
// order IAL/HTML attributes differently. Compare semantic attribute maps while
// retaining text, element/child order, IDs, values and counts exactly.
func r03CanonicalIAL(value string) string {
	return regexp.MustCompile(`\{:[^}\n]*\}`).ReplaceAllStringFunc(value, func(raw string) string {
		pairs := parse.Tokens2IAL([]byte(strings.TrimSuffix(strings.TrimPrefix(raw, "{:"), "}")))
		sort.Slice(pairs, func(i, j int) bool { return pairs[i][0] < pairs[j][0] })
		data, _ := json.Marshal(pairs)
		return string(data)
	})
}
func r03CanonicalDOM(value string) string {
	tree, e := html.Parse(strings.NewReader(value))
	if e != nil {
		return value
	}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		sort.Slice(n.Attr, func(i, j int) bool { return n.Attr[i].Key < n.Attr[j].Key })
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(tree)
	var out bytes.Buffer
	html.Render(&out, tree)
	return out.String()
}
