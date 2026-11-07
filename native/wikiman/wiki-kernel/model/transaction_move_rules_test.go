package model

import (
	"context"
	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"testing"
	"wiki-kernel/internal/nativeparser"
	"wiki-kernel/treenode"
)

func TestOriginalMoveContextFoldedHeadingAndCycleRules(t *testing.T) {
	for _, kind := range []string{"heading", "own-folded-child", "parent-into-child"} {
		t.Run(kind, func(t *testing.T) {
			a := &ast.Node{ID: "root-a", Type: ast.NodeDocument}
			b := &ast.Node{ID: "root-b", Type: ast.NodeDocument}
			heading := &ast.Node{ID: "heading", Type: ast.NodeHeading, HeadingLevel: 1}
			heading.SetIALAttr("fold", "1")
			child := &ast.Node{ID: "folded-child", Type: ast.NodeParagraph}
			child.SetIALAttr("heading-fold", "1")
			a.AppendChild(heading)
			a.AppendChild(child)
			a.AppendChild(&ast.Node{ID: "next-heading", Type: ast.NodeHeading, HeadingLevel: 1})
			target := &ast.Node{ID: "target", Type: ast.NodeParagraph}
			b.AppendChild(target)
			if kind == "parent-into-child" {
				heading.Type = ast.NodeBlockquote
				heading.AppendChild(child)
			}
			trees := map[string]*parse.Tree{"root-a": {ID: "root-a", Root: a}, "root-b": {ID: "root-b", Root: b}}
			staged := 0
			history := 0
			env := &transactionEnvironment{ctx: context.Background(), now: func() string { return "20261103000000" }, parser: func() *lute.Lute { return nativeparser.NewLute(nativeparser.Defaults()) }, lookup: func(id string) *treenode.BlockTree {
				d := "root-a"
				if id == "target" || id == "root-b" {
					d = "root-b"
				}
				return &treenode.BlockTree{ID: id, RootID: d, Path: d}
			}, load: func(_, p string, _ *lute.Lute) (*parse.Tree, error) { return trees[p], nil }, cacheIAL: func(string, map[string]string) {}, stage: func(*parse.Tree) error { staged++; return nil }, finish: func(*Transaction) error { return nil }, moveHistory: func(context.Context, *parse.Tree) error { history++; return nil }, moveReferences: func(context.Context, ...string) error { return nil }}
			op := &Operation{Action: "move", ID: "heading", PreviousID: "target"}
			if kind == "own-folded-child" {
				op.PreviousID = "folded-child"
			}
			if kind == "parent-into-child" {
				op.PreviousID = ""
				op.ParentID = "folded-child"
			}
			tx := &Transaction{environment: env, DoOperations: []*Operation{op}}
			if e := performTx(tx); e != nil {
				t.Fatal(e)
			}
			if history != 1 {
				t.Fatal(history)
			}
			if kind == "heading" {
				if staged != 2 || target.Next != heading || heading.Next != child || heading.Parent != b || child.Parent != b || a.FirstChild.ID != "next-heading" {
					t.Fatal("folded group semantics", staged)
				}
			} else if staged != 0 || heading.Parent != a {
				t.Fatal("cycle changed candidate", staged)
			}
		})
	}
}
