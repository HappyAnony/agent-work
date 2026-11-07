package model

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"wiki-kernel/av"
	"wiki-kernel/treenode"
)

func TestAVBoundTransactionFailuresAndCancellation(t *testing.T) {
	m, store, root := nameSource(t)
	page := "1760000000000000002"
	l, err := m.Acquire(context.Background(), store, page)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(l.Release)
	for _, mode := range []string{"insert", "delete"} {
		points := []string{"read:" + cellAV, "bound-exists", "bound-load", "load", "names", "tree-stage"}
		if mode == "insert" {
			points = append(points, "bound-assets", "bound-references", "bound-register", "bound-attrs", "attrs", "notify", "bound-tree")
		} else {
			points = append(points, "bound-mirror", "bound-remove", "name-cache", "name-notify", "bound-tree", "bound-reload")
		}
		for _, point := range points {
			for _, prefix := range []string{"", "cancel-"} {
				t.Run(mode+"/"+prefix+point, func(t *testing.T) {
					d := newBoundHarness(t, l, root, page, prefix+point)
					before := nameDigest(t, root, page)
					if mode == "insert" {
						d.tx.DoOperations = []*Operation{boundInsert(boundInserted)}
					} else {
						d.tx.DoOperations = []*Operation{{Action: "delete", ID: rowMirror}}
					}
					ret := performTx(d.tx)
					if ret == nil || d.committed || d.tx.state.Load() != 3 || d.counts["finish"] != 0 || d.counts[point] == 0 || strings.Contains(ret.msg, "PANIC RECOVERED") {
						t.Fatal("error was lost", ret, d.counts)
					}
					assertNameSource(t, d.cellHarness, before)
				})
			}
		}
	}
}

func TestAVBoundCollectionValidationAndDependencyGates(t *testing.T) {
	m, store, root := nameSource(t)
	page := "1760000000000000002"
	l, err := m.Acquire(context.Background(), store, page)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(l.Release)
	for _, scenario := range []string{"nil-source", "nil-key", "nil-primary-payload", "no-primary", "nil-tree", "nil-root", "empty-tree-id", "conflicting-tree", "pre-cancel", "late-load-cancel", "known-source-failure", "missing-env", "missing-ctx", "missing-exists", "missing-load"} {
		t.Run(scenario, func(t *testing.T) {
			d := newBoundHarness(t, l, root, page, "")
			a, err := d.cell.read(cellAV)
			if err != nil {
				t.Fatal(err)
			}
			e := d.env.avBound
			switch scenario {
			case "nil-source":
				a = nil
			case "nil-key":
				a.KeyValues[0].Key = nil
			case "nil-primary-payload":
				a.KeyValues[0].Values[0].Block = nil
			case "no-primary":
				a.KeyValues[0].Key.Type = av.KeyTypeText
			case "nil-tree":
				e.load = func(string) (*parse.Tree, error) { return nil, nil }
			case "nil-root", "empty-tree-id", "conflicting-tree":
				load := e.load
				calls := 0
				e.load = func(id string) (*parse.Tree, error) {
					tree, err := load(id)
					if err != nil {
						return nil, err
					}
					calls++
					if scenario == "nil-root" {
						tree.Root = nil
					}
					if scenario == "empty-tree-id" {
						tree.ID = ""
					}
					if scenario == "conflicting-tree" && calls == 2 {
						clone := *tree
						clone.Path = "/conflict.docj"
						tree = &clone
					}
					return tree, nil
				}
			case "pre-cancel":
				d.cancel()
			case "late-load-cancel":
				load := e.load
				e.load = func(id string) (*parse.Tree, error) { tree, err := load(id); d.cancel(); return tree, err }
			case "known-source-failure":
				e.load = func(string) (*parse.Tree, error) { return nil, errors.New("source unavailable") }
			case "missing-env":
				d.env.avBound = nil
			case "missing-ctx":
				d.env.ctx = nil
			case "missing-exists":
				e.exists = nil
			case "missing-load":
				e.load = nil
			}
			d.counts = map[string]int{}
			before := nameDigest(t, root, page)
			trees, nodes, err := d.tx.getAttrViewBoundNodes(a)
			if err == nil || trees != nil || nodes != nil || d.counts["finish"] != 0 {
				t.Fatal("invalid collect returned partial data", scenario, err)
			}
			if strings.HasPrefix(scenario, "missing-") && len(d.counts) != 0 {
				t.Fatal("missing dependency read data", d.counts)
			}
			assertNameSource(t, d.cellHarness, before)
		})
	}
	// Legacy keeps its stale/unreadable-source skip; scoped mode above propagates it.
	d := newBoundHarness(t, l, root, page, "")
	a, _ := d.cell.read(cellAV)
	e := *d.env.avBound
	e.legacy = true
	e.load = func(string) (*parse.Tree, error) { return nil, errors.New("legacy unavailable") }
	if _, nodes, err := collectAttributeViewBoundNodes(a, &e); err != nil || len(nodes) != 0 {
		t.Fatal("legacy skip changed", err)
	}
	if err := (*Transaction)(nil).attributeViewBoundEnvironment().validate("delete"); err != nil {
		t.Fatal("legacy factory", err)
	}
	// Unknown large-batch shapes and an absent binding adapter still fail admission.
	for _, scenario := range []string{"adapter", "shape", "binding-dependency", "insert-port", "delete-port"} {
		t.Run("large-gate-"+scenario, func(t *testing.T) {
			d := newBoundHarness(t, l, root, page, "")
			d.tx.DoOperations = *boundLargeOps()
			switch scenario {
			case "adapter":
				d.env.avBound = nil
			case "shape":
				d.tx.DoOperations[1].PreviousID = cellSecond
			case "binding-dependency":
				d.env.avBound.names = nil
			case "insert-port":
				d.env.insertAssets = nil
			case "delete-port":
				d.env.deleteAV = nil
			}
			if ret := performTx(d.tx); ret == nil || len(d.counts) != 0 || d.tx.m != nil {
				t.Fatal("large batch missing dependency admitted", ret, d.counts)
			}
		})
	}
}

func TestAVBoundEffectDependencyGates(t *testing.T) {
	m, store, root := nameSource(t)
	page := "1760000000000000002"
	l, err := m.Acquire(context.Background(), store, page)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(l.Release)
	for _, mode := range []string{"insert", "delete"} {
		deps := []string{"environment", "ctx", "read", "names", "exists", "load"}
		if mode == "insert" {
			deps = append(deps, "attrs", "tree")
		} else {
			deps = append(deps, "stage", "groups", "mirror", "remove", "cache", "notify", "tree", "reload")
		}
		for _, dep := range deps {
			t.Run(mode+"/"+dep, func(t *testing.T) {
				d := newBoundHarness(t, l, root, page, "")
				e := d.env.avBound
				switch dep {
				case "environment":
					d.env.avBound = nil
				case "ctx":
					d.env.ctx = nil
				case "read":
					e.read = nil
				case "names":
					e.names = nil
				case "exists":
					e.exists = nil
				case "load":
					e.load = nil
				case "attrs":
					e.attrs = nil
				case "stage":
					e.stage = nil
				case "groups":
					e.groups = nil
				case "mirror":
					e.isMirror = nil
				case "remove":
					e.removeMirror = nil
				case "cache":
					e.cache = nil
				case "notify":
					e.notify = nil
				case "tree":
					e.tree = nil
				case "reload":
					e.reload = nil
				}
				n := &ast.Node{Type: ast.NodeAttributeView, ID: boundInserted, AttributeViewID: cellAV}
				tree := &parse.Tree{ID: settingsDoc, Box: settingsBox, Path: "/" + settingsDoc + ".docj", Root: &ast.Node{Type: ast.NodeDocument, ID: settingsDoc}}
				var err error
				if mode == "insert" {
					err = transactionInsertAVBind(context.Background(), n, tree, d.tx)
				} else {
					err = syncDelete2AvBlock(n, tree, d.tx)
				}
				if err == nil || len(d.counts) != 0 {
					t.Fatal("unbound effects ran", mode, dep, err, d.counts)
				}
			})
		}
	}
}

func TestAVBoundUpdateRemovedChildUsesOwnedDeleteChain(t *testing.T) {
	m, store, root := nameSource(t)
	page := "1760000000000000002"
	l, err := m.Acquire(context.Background(), store, page)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(l.Release)
	const container = "20261025010200-quo0001"
	for _, failure := range []string{"", "bound-remove", "bound-load", "name-notify"} {
		t.Run(failure, func(t *testing.T) {
			d := newBoundHarness(t, l, root, page, failure)
			lookup, load := d.env.lookup, d.env.load
			d.env.lookup = func(id string) *treenode.BlockTree {
				if id == container {
					return &treenode.BlockTree{ID: id, RootID: settingsDoc, BoxID: settingsBox, Path: "/" + settingsDoc + ".docj"}
				}
				return lookup(id)
			}
			d.env.load = func(box, p string, engine *lute.Lute) (*parse.Tree, error) {
				tree, err := load(box, p, engine)
				if err != nil {
					return nil, err
				}
				if tree.ID == settingsDoc {
					mirror := treenode.GetNodeInTree(tree, rowMirror)
					quote := &ast.Node{Type: ast.NodeBlockquote, ID: container}
					quote.SetIALAttr("id", container)
					mirror.InsertBefore(quote)
					mirror.Unlink()
					quote.AppendChild(mirror)
				}
				return tree, nil
			}
			d.env.updateCacheRef = func(context.Context, *parse.Tree, *ast.Node) error { return nil }
			d.env.updateDynamicText = func(context.Context, string) (string, error) { return "", nil }
			d.env.updateReferences = func(context.Context, ...string) error { return nil }
			d.env.updateUnfold = func(context.Context, *ast.Node, *ast.Node) error { return nil }
			d.env.updateEvent = func(context.Context, []*Transaction) error { return nil }
			d.env.updateAV = func(context.Context, *ast.Node, *parse.Tree, *Transaction) error { return nil }
			d.tx.DoOperations = []*Operation{{Action: "update", ID: container, Data: `<div data-node-id="` + container + `" data-type="NodeParagraph"><div contenteditable="true">Replacement body</div></div>`}}
			before := nameDigest(t, root, page)
			ret := performTx(d.tx)
			if failure != "" {
				if ret == nil || d.committed || d.counts["finish"] != 0 {
					t.Fatal("update cleanup failure committed", ret, d.counts)
				}
			} else {
				if ret != nil {
					t.Fatal(ret)
				}
				assertBoundFinish(t, d, before)
				if d.counts["bound-remove"] != 1 || treenode.GetNodeInTree(d.tx.trees[settingsDoc], rowMirror) != nil {
					t.Fatal("removed AV child not processed", d.counts)
				}
				for _, id := range []string{settingsBlock, cellSecond, nameBlock} {
					if strings.Contains(d.attrs[id][av.NodeAttrNameAvs], cellAV) {
						t.Fatal("update retained removed database membership")
					}
				}
			}
			assertNameSource(t, d.cellHarness, before)
		})
	}
}
