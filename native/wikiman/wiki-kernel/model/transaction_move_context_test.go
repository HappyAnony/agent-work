package model

import (
	"context"
	"errors"
	"github.com/88250/lute"
	"github.com/88250/lute/parse"
	"github.com/siyuan-note/dataparser"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"wiki-kernel/internal/nativeparser"
	"wiki-kernel/internal/workspace"
	"wiki-kernel/treenode"
)

func TestOriginalMoveContextCrossDocumentCandidatesAndFailures(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	if e := os.CopyFS(root, os.DirFS("../../fixtures/shared-wiki/valid")); e != nil {
		t.Fatal(e)
	}
	const a = "1760000000000000002"
	const b = "1760000000000000003"
	const box = "20261019090000-box0001"
	const doc = "20261019090001-doc0001"
	const id = "20261019090002-par0001"
	const other = "20261022090001-doc0002"
	const target = "20261022090002-par0002"
	source, e := os.ReadFile(filepath.Join(root, "pages", a, "data", box, doc+".docj"))
	if e != nil {
		t.Fatal(e)
	}
	for _, page := range []string{a, b} {
		dir := filepath.Join(root, "pages", page, "data", box)
		if e = os.MkdirAll(dir, 0700); e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(filepath.Join(dir, doc+".docj"), source, 0600); e != nil {
			t.Fatal(e)
		}
		raw := strings.NewReplacer(doc, other, id, target, "20261019090003-cod0001", "20261022090003-cod0002", "20261019090004-ref0001", "20261022090004-ref0002").Replace(string(source))
		if e = os.WriteFile(filepath.Join(dir, other+".docj"), []byte(raw), 0600); e != nil {
			t.Fatal(e)
		}
	}
	m := workspace.NewManager(workspace.Options{})
	store, e := m.OpenStore(ctx, root)
	if e != nil {
		t.Fatal(e)
	}
	defer m.Drain(ctx)
	for _, page := range []string{a, b} {
		lease, e := m.Acquire(ctx, store, page)
		if e != nil {
			t.Fatal(e)
		}
		for _, position := range []string{"parent", "previous", "same"} {
			for _, failure := range []string{"", "history", "source-load", "target-load", "source-stage", "target-stage", "references", "finish", "missing"} {
				if position == "same" && (failure == "target-load" || failure == "target-stage" || failure == "references") {
					continue
				}
				t.Run(page+"/"+position+"/"+failure, func(t *testing.T) {
					staged := map[string]*parse.Tree{}
					histories, refCalls, finishes := 0, 0, 0
					sentinel := errors.New("synthetic " + failure)
					env := &transactionEnvironment{ctx: ctx, now: func() string { return "20261103000000" }, parser: func() *lute.Lute { return nativeparser.NewLute(nativeparser.Defaults()) }, lookup: func(key string) *treenode.BlockTree {
						if failure == "missing" {
							return nil
						}
						d := doc
						if key == other || key == target {
							d = other
						}
						return &treenode.BlockTree{ID: key, RootID: d, BoxID: box, Path: "/" + d + ".docj"}
					}, load: func(_ string, p string, engine *lute.Lute) (*parse.Tree, error) {
						d := strings.TrimSuffix(filepath.Base(p), ".docj")
						if (d == doc && failure == "source-load") || (d == other && failure == "target-load") {
							return nil, sentinel
						}
						raw, e := lease.ReadDocument(ctx, box, d)
						if e != nil {
							return nil, e
						}
						tree, e := dataparser.ParseJSONWithoutFix(raw, engine.ParseOptions)
						if e != nil {
							return nil, e
						}
						tree.ID, tree.Box, tree.Path = d, box, p
						return tree, nil
					}, cacheIAL: func(string, map[string]string) {}, stage: func(tree *parse.Tree) error {
						if (tree.ID == doc && failure == "source-stage") || (tree.ID == other && failure == "target-stage") {
							return sentinel
						}
						staged[tree.ID] = tree
						return nil
					}, finish: func(*Transaction) error {
						finishes++
						if failure == "finish" {
							return sentinel
						}
						return nil
					}, moveHistory: func(_ context.Context, tree *parse.Tree) error {
						histories++
						if treenode.GetNodeInTree(tree, id) == nil {
							t.Fatal("history admission after mutation")
						}
						if failure == "history" {
							return sentinel
						}
						return nil
					}, moveReferences: func(_ context.Context, ids ...string) error {
						refCalls++
						if len(ids) != 2 || ids[0] != doc || ids[1] != id {
							t.Fatal(ids)
						}
						if failure == "references" {
							return sentinel
						}
						return nil
					}}
					op := &Operation{Action: "move", ID: id, ParentID: other}
					if position == "previous" {
						op.ParentID = ""
						op.PreviousID = target
					}
					if position == "same" {
						op.ParentID = doc
					}
					tx := &Transaction{environment: env, DoOperations: []*Operation{op}}
					result := performTx(tx)
					if failure == "" {
						if result != nil || tx.state.Load() != 2 || finishes != 1 || histories != 1 {
							t.Fatal(result, tx.state.Load(), finishes, histories)
						}
						if tx.nodes[id].IALAttr("updated") != "20261103000000" {
							t.Fatal("move did not use captured clock", tx.nodes[id].IALAttr("updated"))
						}
						if position != "same" {
							if len(staged) != 2 || refCalls != 1 || treenode.GetNodeInTree(staged[doc], id) != nil || treenode.GetNodeInTree(staged[other], id) == nil {
								t.Fatal("cross-doc move candidates", staged, refCalls)
							}
						} else if len(staged) != 1 || refCalls != 0 || staged[doc].Root.FirstChild.ID != id {
							t.Fatal(staged, refCalls)
						}
					} else if result == nil || tx.state.Load() != 3 {
						t.Fatal("failed move accepted", result, tx.state.Load())
					}
					if !tx.m.TryLock() {
						t.Fatal("move retained lock")
					}
					tx.m.Unlock()
					after, e := lease.ReadDocument(ctx, box, doc)
					if e != nil || string(after) != string(source) {
						t.Fatal("candidate changed source", e)
					}
				})
			}
		}
		lease.Release()
	}
}
