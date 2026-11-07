//go:build fts5

package model

import (
	"context"
	"errors"
	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"github.com/siyuan-note/dataparser"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"wiki-kernel/av"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/nativeparser"
	"wiki-kernel/internal/workspace"
	"wiki-kernel/treenode"
)

func TestOriginalUnfoldContextReferencesRendererAndPostCommitCallback(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	if err := os.CopyFS(root, os.DirFS("../../fixtures/shared-wiki/valid")); err != nil {
		t.Fatal(err)
	}
	dataB := filepath.Join(root, "pages", "1760000000000000003", "data")
	if err := os.MkdirAll(dataB, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.CopyFS(dataB, os.DirFS(filepath.Join(root, "pages", "1760000000000000002", "data"))); err != nil {
		t.Fatal(err)
	}
	m := workspace.NewManager(workspace.Options{})
	store, err := m.OpenStore(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Drain(ctx)
	const box = "20261019090000-box0001"
	const doc = "20261019090001-doc0001"
	for _, page := range []string{"1760000000000000002", "1760000000000000003"} {
		lease, err := m.Acquire(ctx, store, page)
		if err != nil {
			t.Fatal(err)
		}
		defer lease.Release()
		raw, err := lease.ReadDocument(ctx, box, doc)
		if err != nil {
			t.Fatal(err)
		}
		for _, failure := range []string{"", "reference", "names", "commit"} {
			t.Run(page+"/"+failure, func(t *testing.T) {
				engine := nativeparser.NewLute(nativeparser.Markdown{})
				tree, _, err := dataparser.ParseJSON(raw, engine.ParseOptions)
				if err != nil {
					t.Fatal(err)
				}
				tree.ID, tree.Box, tree.Path = doc, box, "/"+doc+".docj"
				parent := tree.Root.FirstChild
				parent.Type = ast.NodeHeading
				parent.HeadingLevel = 1
				parent.SetIALAttr("fold", "1")
				heading := parent.Next
				heading.Type = ast.NodeHeading
				heading.HeadingLevel = 2
				heading.SetIALAttr("fold", "1")
				heading.SetIALAttr("heading-fold", "1")
				for heading.FirstChild != nil {
					heading.FirstChild.Unlink()
				}
				heading.AppendChild(&ast.Node{Type: ast.NodeText, Tokens: []byte("subheading")})
				child := heading.Next
				child.SetIALAttr("fold", "1")
				child.SetIALAttr("heading-fold", "1")
				child.SetIALAttr(av.NodeAttrNameAvs, "scoped-av")
				if err = lease.WithNativeIndexes(ctx, func(b *indexdb.Bundle) error {
					db, _ := b.Database(indexdb.Main)
					_, err := db.Exec("DELETE FROM refs;INSERT INTO refs(id,def_block_id) VALUES('one',?);INSERT INTO refs(id,def_block_id) VALUES('two',?)", child.ID, child.ID)
					if err == nil && page == "1760000000000000003" {
						_, err = db.Exec("INSERT INTO refs(id,def_block_id) VALUES('three',?)", child.ID)
					}
					return err
				}); err != nil {
					t.Fatal(err)
				}
				callbacks, stages := 0, 0
				env := &transactionEnvironment{ctx: ctx, parser: func() *lute.Lute { return engine }, lookup: func(string) *treenode.BlockTree { return &treenode.BlockTree{RootID: doc, BoxID: box, Path: tree.Path} }, load: func(string, string, *lute.Lute) (*parse.Tree, error) { return tree, nil }, stage: func(*parse.Tree) error { stages++; return nil }, cacheIAL: func(string, map[string]string) {}, sync: func() {}, indexTree: func(*parse.Tree) {},
					editorParser: func() *lute.Lute { return nativeparser.NewEditor(nativeparser.EditorOptions{}) },
					referenceCounts: func(ctx context.Context, ids []string) (ret map[string]int, err error) {
						if failure == "reference" {
							return nil, errors.New("synthetic query failure")
						}
						err = lease.WithNativeReferences(ctx, func(q indexdb.References) error { ret, err = q.DefinitionCounts(ctx, ids); return err })
						return
					},
					avNames: func(ctx context.Context, ids string) (string, error) {
						if failure == "names" {
							return "", errors.New("synthetic AV failure")
						}
						if ids != "scoped-av" {
							t.Fatal(ids)
						}
						return "owner title " + page, nil
					},
					finish: func(*Transaction) error {
						if failure == "commit" {
							return errors.New("synthetic finish failure")
						}
						return nil
					},
				}
				tx := &Transaction{environment: env, DoOperations: []*Operation{{Action: "unfoldHeading", ID: heading.ID}}}
				env.reload = func(id string) {
					callbacks++
					if id != doc || lease.PageID() != page || tx.state.Load() != 2 {
						t.Error("callback before commit", id, tx.state.Load())
					}
					if _, err := lease.Configuration(ctx); err != nil {
						t.Error(err)
					}
				}
				result := performTx(tx)
				if failure != "" {
					if result == nil || callbacks != 0 || tx.state.Load() != 3 {
						t.Fatal("failed operation refreshed UI", result, callbacks, tx.state.Load())
					}
					return
				}
				html, ok := tx.DoOperations[0].RetData.(string)
				refcount := `refcount="2"`
				if page == "1760000000000000003" {
					refcount = `refcount="3"`
				}
				if result != nil || !ok || !strings.Contains(html, refcount) || !strings.Contains(html, "owner title "+page) || callbacks != 1 || stages != 1 || parent.IALAttr("fold") != "" || heading.IALAttr("fold") != "" || child.IALAttr("heading-fold") != "" {
					t.Fatal(result, html, callbacks, stages)
				}
				after, err := lease.ReadDocument(ctx, box, doc)
				if err != nil || string(after) != string(raw) {
					t.Fatal("staging seam modified source", err)
				}
			})
		}
	}
}
