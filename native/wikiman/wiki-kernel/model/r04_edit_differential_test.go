//go:build fts5

package model

import (
	"bytes"
	"context"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"

	"wiki-kernel/conf"
	"wiki-kernel/internal/editcore"
	"wiki-kernel/internal/sourcetx"
	"wiki-kernel/internal/wikiops"
	"wiki-kernel/internal/workspace"
	"wiki-kernel/util"
)

func TestR04OriginalDispatchLedgerIsExhaustive(t *testing.T) {
	f, e := parser.ParseFile(token.NewFileSet(), "transaction.go", nil, 0)
	if e != nil {
		t.Fatal(e)
	}
	want := []string{}
	for _, d := range f.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "performTx" {
			continue
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			s, ok := n.(*ast.SwitchStmt)
			if !ok {
				return true
			}
			tag, ok := s.Tag.(*ast.SelectorExpr)
			if !ok || tag.Sel.Name != "Action" {
				return true
			}
			for _, statement := range s.Body.List {
				for _, label := range statement.(*ast.CaseClause).List {
					value, e := strconv.Unquote(label.(*ast.BasicLit).Value)
					if e != nil {
						t.Fatal(e)
					}
					if value != "create" {
						want = append(want, value)
					}
				}
			}
			return false
		})
	}
	sort.Strings(want)
	if len(want) != 74 || !reflect.DeepEqual(want, editcore.EditingActions()) {
		t.Fatal("original operation admission differs", want, editcore.EditingActions())
	}
}

func TestR04OriginalAndNativeEditDifferentialWithDynamicReferences(t *testing.T) {
	c, env, _, oldFile, _ := commitFixture(t)
	c.Export = conf.NewExport()
	ctx := context.Background()
	root := t.TempDir()
	if e := os.CopyFS(root, os.DirFS("../../fixtures/shared-wiki/valid")); e != nil {
		t.Fatal(e)
	}
	const page = "1760000000000000002"
	const box = "20261028140000-refbox1"
	const def = "20261028140002-refdef1"
	const defDoc = "20261028140001-refdoc1"
	const refDoc = "20261028140003-refdoc2"
	name := box + "/" + defDoc + ".docj"
	oldFile = filepath.Join(filepath.Dir(oldFile), defDoc+".docj")
	for _, p := range []string{page, "1760000000000000003"} {
		dest := filepath.Join(root, "pages", p, "data", box)
		if e := os.MkdirAll(dest, 0700); e != nil {
			t.Fatal(e)
		}
		if e := os.CopyFS(dest, os.DirFS(filepath.Join(util.DataDir, box))); e != nil {
			t.Fatal(e)
		}
	}
	options := workspace.Options{ReadDefaults: true, Configuration: func(string, string) (conf.AppData, error) { return c.AppData.Clone() }, ConfigurationBusiness: func(string, string) (*conf.Business, error) { return c.business, nil }}
	m := workspace.NewManager(options)
	store, e := m.OpenStore(ctx, root)
	if e != nil {
		t.Fatal(e)
	}
	defer m.Drain(ctx)
	read := func(file string) []byte {
		t.Helper()
		b, e := os.ReadFile(file)
		if e != nil {
			t.Fatal(e)
		}
		return b
	}
	nativeDir := filepath.Join(root, "pages", page, "data", box)
	file := filepath.Join(nativeDir, filepath.Base(oldFile))
	other := filepath.Join(root, "pages", "1760000000000000003", "data", name)
	otherBefore := read(other)
	dom := `<div data-node-id="` + def + `" data-type="NodeParagraph" class="p"><div contenteditable="true"><strong>same original change</strong></div><div class="protyle-attr" contenteditable="false"></div></div>`
	ops := func() []*Operation {
		return []*Operation{{Action: "update", ID: def, Data: dom}, {Action: "setAttrs", ID: def, Data: `{"custom-differential":"same"}`}, {Action: "doUpdateUpdated", ID: def, Data: "20261105142000"}}
	}
	if failure := performTx(&Transaction{environment: env, DoOperations: ops()}); failure != nil {
		t.Fatalf("original edit failed: %+v", failure)
	}
	result, e := wikiops.Edit(ctx, m, wikiops.EditRequest{ProtocolVersion: 1, StoreID: store, PageID: page, OperationID: "r04-differential", ExpectedSources: map[string]string{name: sourcetx.Hash(read(file))}, Operations: ops()})
	if e != nil || !result.IndexFresh || result.Receipt == nil || !result.Receipt.SourceCommitted {
		t.Fatal(result, e)
	}
	block := func(raw []byte) any {
		var doc struct{ Children []map[string]any }
		if e := json.Unmarshal(raw, &doc); e != nil {
			t.Fatal(e)
		}
		for _, b := range doc.Children {
			if b["ID"] == def {
				return b
			}
		}
		t.Fatal("definition not found")
		return nil
	}
	if !reflect.DeepEqual(block(read(oldFile)), block(read(file))) {
		t.Fatalf("edited block differs\noriginal %s\nnative   %s", read(oldFile), read(file))
	}
	for _, p := range []string{filepath.Join(filepath.Dir(oldFile), refDoc+".docj"), filepath.Join(nativeDir, refDoc+".docj")} {
		if raw := read(p); !strings.Contains(string(raw), "same original change") {
			t.Fatal("dynamic anchor was not committed", string(raw))
		}
	}
	if !bytes.Equal(otherBefore, read(other)) {
		t.Fatal("same ID under different owner was modified")
	}
}
