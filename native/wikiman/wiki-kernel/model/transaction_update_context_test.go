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
	"wiki-kernel/internal/nativeparser"
	"wiki-kernel/internal/workspace"
	"wiki-kernel/treenode"
)

func TestOriginalUpdateContextSourceAndEffectFailures(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	if e := os.CopyFS(root, os.DirFS("../../fixtures/shared-wiki/valid")); e != nil {
		t.Fatal(e)
	}
	const pageA = "1760000000000000002"
	const pageB = "1760000000000000003"
	const box = "20261019090000-box0001"
	const doc = "20261019090001-doc0001"
	const id = "20261019090002-par0001"
	const headingID = "20261023090000-hea0001"
	b := filepath.Join(root, "pages", pageB, "data")
	if e := os.MkdirAll(b, 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.CopyFS(b, os.DirFS(filepath.Join(root, "pages", pageA, "data"))); e != nil {
		t.Fatal(e)
	}
	m := workspace.NewManager(workspace.Options{})
	store, e := m.OpenStore(ctx, root)
	if e != nil {
		t.Fatal(e)
	}
	defer m.Drain(ctx)
	for _, page := range []string{pageA, pageB} {
		lease, e := m.Acquire(ctx, store, page)
		if e != nil {
			t.Fatal(e)
		}
		defer lease.Release()
		before, e := lease.ReadDocument(ctx, box, doc)
		if e != nil {
			t.Fatal(e)
		}
		for _, variant := range []string{"plain", "removed", "unfold", "event"} {
			failures := []string{"", "load", "cache", "dynamic", "references", "stage", "av", "finish", "missing", "cancel"}
			if variant == "removed" {
				failures = append(failures, "delete")
			}
			if variant == "unfold" {
				failures = append(failures, "unfold")
			}
			if variant == "event" {
				failures = append(failures, "event")
			}
			for _, failure := range failures {
				t.Run(page+"/"+variant+"/"+failure, func(t *testing.T) {
					callCtx, cancel := context.WithCancel(ctx)
					defer cancel()
					want := errors.New("synthetic " + failure)
					counts := map[string]int{}
					var candidate *parse.Tree
					mark := func(name string) error {
						counts[name]++
						if failure == name {
							return want
						}
						if failure == "cancel" && name == "cache" {
							cancel()
						}
						return callCtx.Err()
					}
					env := &transactionEnvironment{
						ctx: callCtx, parser: func() *lute.Lute { return nativeparser.NewLute(nativeparser.Defaults()) },
						lookup: func(key string) *treenode.BlockTree {
							if failure == "missing" {
								return nil
							}
							return &treenode.BlockTree{ID: key, RootID: doc, BoxID: box, Path: "/" + doc + ".docj"}
						},
						load: func(b, p string, engine *lute.Lute) (*parse.Tree, error) {
							if e := mark("load"); e != nil {
								return nil, e
							}
							raw, e := lease.ReadDocument(ctx, b, doc)
							if e != nil {
								return nil, e
							}
							tree, e := dataparser.ParseJSONWithoutFix(raw, engine.ParseOptions)
							if e != nil {
								return nil, e
							}
							tree.ID, tree.Box, tree.Path = doc, b, p
							n := treenode.GetNodeInTree(tree, id)
							if variant == "removed" {
								n.Type = ast.NodeBlockquote
								for n.FirstChild != nil {
									n.FirstChild.Unlink()
								}
								child := &ast.Node{ID: "20261023090001-chd0001", Type: ast.NodeParagraph}
								child.SetIALAttr("id", child.ID)
								child.AppendChild(&ast.Node{Type: ast.NodeText, Tokens: []byte(page + " old child")})
								n.AppendChild(child)
							}
							if variant == "unfold" || variant == "event" {
								h := &ast.Node{ID: headingID, Type: ast.NodeHeading, HeadingLevel: 1}
								if variant == "unfold" {
									h.HeadingLevel = 2
									n.Type = ast.NodeHeading
									n.HeadingLevel = 1
								}
								h.SetIALAttr("id", headingID)
								h.SetIALAttr("fold", "1")
								h.AppendChild(&ast.Node{Type: ast.NodeText, Tokens: []byte("folded")})
								n.InsertBefore(h)
							}
							return tree, nil
						},
						stage: func(tree *parse.Tree) error { candidate = tree; return mark("stage") }, cacheIAL: func(string, map[string]string) { counts["ial"]++ }, finish: func(*Transaction) error { return mark("finish") },
						updateCacheRef: func(_ context.Context, tree *parse.Tree, n *ast.Node) error {
							if tree.Box != box || n.TextMarkBlockRefID != id {
								t.Fatal("wrong reference candidate", tree, n)
							}
							return mark("cache")
						},
						updateDynamicText: func(context.Context, string) (string, error) { return page + " anchor", mark("dynamic") },
						updateReferences:  func(context.Context, ...string) error { return mark("references") },
						updateUnfold:      func(context.Context, *ast.Node, *ast.Node) error { return mark("unfold") },
						updateEvent: func(_ context.Context, events []*Transaction) error {
							if len(events) != 1 || events[0].DoOperations[0].PreviousID != headingID {
								t.Fatal(events)
							}
							return mark("event")
						},
						updateAV: func(context.Context, *ast.Node, *parse.Tree, *Transaction) error { return mark("av") },
						deleteAV: func(context.Context, *ast.Node, *parse.Tree, *Transaction) error { return mark("delete") },
					}
					typ, subtype := "NodeParagraph", ""
					if variant == "event" {
						typ, subtype = "NodeHeading", ` data-subtype="h1"`
					}
					data := `<div data-type="` + typ + `"` + subtype + ` data-node-id="` + id + `"><div contenteditable="true"><span data-type="block-ref" data-subtype="d" data-id="` + id + `">stale</span> ` + page + ` updated</div></div>`
					tx := &Transaction{environment: env, DoOperations: []*Operation{{Action: "update", ID: id, Data: data}}}
					got := performTx(tx)
					if failure == "" {
						if got != nil || candidate == nil || tx.state.Load() != 2 || counts["stage"] != 1 || counts["finish"] != 1 || counts["cache"] != 1 || counts["dynamic"] != 1 || counts["references"] != 1 {
							t.Fatal(got, counts, tx.state.Load())
						}
						n := treenode.GetNodeInTree(candidate, id)
						if n == nil || !strings.Contains(n.Text(), page) {
							t.Fatal(n)
						}
						anchorFound := false
						ast.Walk(n, func(n *ast.Node, enter bool) ast.WalkStatus {
							if enter && n.TextMarkTextContent == page+" anchor" {
								anchorFound = true
							}
							return ast.WalkContinue
						})
						if !anchorFound {
							t.Fatal("dynamic anchor not inherited", n)
						}
						if variant == "unfold" && counts["unfold"] != 1 {
							t.Fatal(counts)
						}
						if variant == "event" && counts["event"] != 1 {
							t.Fatal(counts)
						}
						if variant == "removed" && counts["delete"] != 1 {
							t.Fatal(counts)
						}
					} else if got == nil || tx.state.Load() != 3 {
						t.Fatal("failure accepted", got, counts, tx.state.Load())
					}
					if !tx.m.TryLock() {
						t.Fatal("lock retained")
					}
					tx.m.Unlock()
					after, e := lease.ReadDocument(ctx, box, doc)
					if e != nil || string(after) != string(before) {
						t.Fatal("candidate changed authoritative source", e)
					}
				})
			}
		}
	}
}

func TestOriginalUpdateDependenciesRequiredBeforeBegin(t *testing.T) {
	for _, missing := range []string{"cache", "dynamic", "refs", "unfold", "event", "av", "delete"} {
		calls := 0
		e := &transactionEnvironment{ctx: context.Background(), parser: func() *lute.Lute { calls++; return nil }, lookup: func(string) *treenode.BlockTree { calls++; return nil }, load: func(string, string, *lute.Lute) (*parse.Tree, error) { calls++; return nil, nil }, stage: func(*parse.Tree) error { calls++; return nil }, cacheIAL: func(string, map[string]string) { calls++ }, finish: func(*Transaction) error { calls++; return nil }, updateCacheRef: func(context.Context, *parse.Tree, *ast.Node) error { return nil }, updateDynamicText: func(context.Context, string) (string, error) { return "", nil }, updateReferences: func(context.Context, ...string) error { return nil }, updateUnfold: func(context.Context, *ast.Node, *ast.Node) error { return nil }, updateEvent: func(context.Context, []*Transaction) error { return nil }, updateAV: func(context.Context, *ast.Node, *parse.Tree, *Transaction) error { return nil }, deleteAV: func(context.Context, *ast.Node, *parse.Tree, *Transaction) error { return nil }}
		switch missing {
		case "cache":
			e.updateCacheRef = nil
		case "dynamic":
			e.updateDynamicText = nil
		case "refs":
			e.updateReferences = nil
		case "unfold":
			e.updateUnfold = nil
		case "event":
			e.updateEvent = nil
		case "av":
			e.updateAV = nil
		case "delete":
			e.deleteAV = nil
		}
		tx := &Transaction{environment: e, DoOperations: []*Operation{{Action: "update"}}}
		if got := performTx(tx); got == nil || calls != 0 || tx.state.Load() != 0 {
			t.Fatal(missing, got, calls, tx.state.Load())
		}
	}
}
