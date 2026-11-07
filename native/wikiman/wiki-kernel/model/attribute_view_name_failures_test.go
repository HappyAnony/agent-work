package model

import (
	"context"
	"errors"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"strings"
	"testing"
	"wiki-kernel/av"
	"wiki-kernel/cache"
	"wiki-kernel/treenode"
)

func TestAVNameFailuresCancellationAndAdmission(t *testing.T) {
	m, store, root := nameSource(t)
	for _, page := range []string{"1760000000000000002", "1760000000000000003"} {
		t.Run(page, func(t *testing.T) {
			t.Parallel()
			l, err := m.Acquire(context.Background(), store, page)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(l.Release)
			for _, point := range []string{"read:" + cellAV, "stage:" + cellAV, "exists", "node", "load", "name", "read:" + cellDest, "name-cache", "name-notify", "name-tree", "tree-stage", "finish", "untitled"} {
				for _, prefix := range []string{"", "cancel-"} {
					// The language snapshot returns text, not an error. Cancellation still gates effects.
					if point == "untitled" && prefix == "" {
						continue
					}
					t.Run(prefix+point, func(t *testing.T) {
						h := newNameHarness(t, l, root, page, "")
						before := nameDigest(t, root, page)
						text := " renamed "
						if point == "untitled" {
							text = ""
						}
						h.failure = prefix + point
						h.tx.DoOperations = []*Operation{nameOp(cellAV, text)}
						ret := performTx(h.tx)
						if ret == nil || h.committed || h.tx.state.Load() != 3 || strings.Contains(ret.msg, "PANIC") || h.counts[point] == 0 {
							t.Fatal("name failure escaped", ret, h.counts)
						}
						if point != "finish" && h.counts["finish"] != 0 {
							t.Fatal("failed name operation reached finish")
						}
						if point == "stage:"+cellAV && (h.counts["exists"] != 0 || h.counts["name-notify"] != 0) {
							t.Fatal("failed save refreshed bindings")
						}
						assertNameSource(t, h, before)
					})
				}
			}
			for _, missing := range []string{"environment", "ctx", "read", "stage", "exists", "node", "name", "untitled", "tree", "cache", "notify"} {
				t.Run("missing-"+missing, func(t *testing.T) {
					h := newNameHarness(t, l, root, page, "")
					before := nameDigest(t, root, page)
					e := h.env.avName
					switch missing {
					case "environment":
						h.env.avName = nil
					case "ctx":
						e.ctx = nil
					case "read":
						e.read = nil
					case "stage":
						e.stage = nil
					case "exists":
						e.exists = nil
					case "node":
						e.node = nil
					case "name":
						e.name = nil
					case "untitled":
						e.untitled = nil
					case "tree":
						e.tree = nil
					case "cache":
						e.cache = nil
					case "notify":
						e.notify = nil
					}
					h.tx.DoOperations = []*Operation{nameOp(cellAV, "new")}
					if ret := performTx(h.tx); ret == nil || h.committed || len(h.counts) != 0 {
						t.Fatal("missing dependency admitted", ret, h.counts)
					}
					assertNameSource(t, h, before)
				})
			}
			for _, scenario := range []string{"bad-type", "empty-id", "bad-last", "unknown-last", "pre-cancel", "nil-source", "wrong-av", "nil-kv", "nil-key", "nil-value", "no-primary", "nil-block", "nil-tree", "wrong-node", "nil-root", "tree-identity", "late-notify", "late-stage"} {
				t.Run(scenario, func(t *testing.T) {
					h := newNameHarness(t, l, root, page, "")
					before := nameDigest(t, root, page)
					a, err := h.cell.read(cellAV)
					if err != nil {
						t.Fatal(err)
					}
					op := nameOp(cellAV, " new ")
					ops := []*Operation{op}
					e := h.env.avName
					switch scenario {
					case "bad-type":
						op.Data = 123
					case "empty-id":
						op.ID = ""
					case "bad-last":
						ops = append(ops, &Operation{Action: "setAttrViewName", ID: cellDest, Data: false})
					case "unknown-last":
						ops = append(ops, &Operation{Action: "unknown"})
					case "pre-cancel":
						h.cancel()
					case "nil-source":
						e.read = func(string) (*av.AttributeView, error) { return nil, nil }
					case "wrong-av":
						a.ID = cellRelated
					case "nil-kv":
						a.KeyValues = append(a.KeyValues, nil)
					case "nil-key":
						a.KeyValues[0].Key = nil
					case "nil-value":
						a.KeyValues[0].Values = append(a.KeyValues[0].Values, nil)
					case "no-primary":
						a.KeyValues[0].Key.Type = av.KeyTypeText
					case "nil-block":
						a.GetBlockKeyValues().Values[0].Block = nil
					case "nil-tree":
						e.node = func(string) (*ast.Node, *parse.Tree, error) { return nil, nil, nil }
					case "wrong-node":
						node := e.node
						e.node = func(id string) (*ast.Node, *parse.Tree, error) {
							_, tree, err := node(id)
							if err != nil {
								return nil, nil, err
							}
							return tree.Root, tree, nil
						}
					case "nil-root":
						node := e.node
						e.node = func(id string) (*ast.Node, *parse.Tree, error) {
							n, tree, err := node(id)
							if err != nil {
								return nil, nil, err
							}
							tree.Root = nil
							return n, tree, nil
						}
					case "tree-identity":
						node := e.node
						e.node = func(id string) (*ast.Node, *parse.Tree, error) {
							n, tree, err := node(id)
							if err != nil {
								return nil, nil, err
							}
							if id == cellSecond {
								copy := *tree
								copy.Path = "/different.docj"
								tree = &copy
							}
							return n, tree, nil
						}
					case "late-notify":
						notify := e.notify
						e.notify = func(old map[string]string, n *ast.Node) error {
							if err := notify(old, n); err != nil {
								return err
							}
							h.cancel()
							return nil
						}
					case "late-stage":
						stage := e.stage
						e.stage = func(a *av.AttributeView) error {
							if err := stage(a); err != nil {
								return err
							}
							h.cancel()
							return nil
						}
					}
					h.counts = map[string]int{}
					h.tx.DoOperations = ops
					ret := performTx(h.tx)
					if ret == nil || h.committed || strings.Contains(ret.msg, "PANIC") {
						t.Fatal("invalid candidate finished", ret, h.counts)
					}
					if (scenario == "bad-type" || scenario == "empty-id" || scenario == "bad-last" || scenario == "unknown-last" || scenario == "pre-cancel") && len(h.counts) != 0 {
						t.Fatal("partial admission", h.counts)
					}
					assertNameSource(t, h, before)
				})
			}
		})
	}
}

func TestAVNameSharedDocumentAndFreshLookup(t *testing.T) {
	m, store, root := nameSource(t)
	for _, page := range []string{"1760000000000000002", "1760000000000000003"} {
		t.Run(page, func(t *testing.T) {
			t.Parallel()
			l, err := m.Acquire(context.Background(), store, page)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(l.Release)
			h := newNameHarness(t, l, root, page, "")
			before := nameDigest(t, root, page)
			e := h.env.avName
			e.node = func(id string) (*ast.Node, *parse.Tree, error) {
				bt := h.env.lookup(id)
				tree, err := h.env.load(bt.BoxID, bt.Path, h.env.parser())
				if err != nil {
					return nil, nil, err
				}
				return treenode.GetNodeInTree(tree, id), tree, nil
			}
			name := e.name
			e.name = func(id string) (string, error) {
				if id == cellAV {
					return "stale storage name", nil
				}
				return name(id)
			}
			h.tx.DoOperations = []*Operation{nameOp(cellAV, " fresh snapshot ")}
			if ret := performTx(h.tx); ret != nil || !h.committed {
				t.Fatal(ret)
			}
			if h.counts["name-tree"] != 2 {
				t.Fatal("expected one stage per document", h.counts)
			}
			for _, id := range []string{settingsBlock, cellSecond} {
				n := treenode.GetNodeInTree(h.tx.trees[settingsDoc], id)
				if !strings.Contains(n.IALAttr(av.NodeAttrViewNames), "fresh snapshot") || strings.Contains(n.IALAttr(av.NodeAttrViewNames), "stale storage") {
					t.Fatal("shared tree/fresh name lost", id)
				}
			}
			assertNameSource(t, h, before)
		})
	}
}

func TestAVNameStandaloneFactoryAndLegacyMissingBinding(t *testing.T) {
	oldOwner := Conf
	Conf = nil
	defer func() { Conf = oldOwner }()
	e := (*Transaction)(nil).attributeViewNameEnvironment()
	if err := e.validate(); err != nil {
		t.Fatal(err)
	}
	if _, err := e.read(cellAV); err == nil {
		t.Fatal("unbound source reader succeeded")
	}
	e = explicitLegacyNameFixture()
	// Preserve the legacy missing-block skip in an explicitly injected algorithm fixture.
	a := &av.AttributeView{ID: cellAV, KeyValues: []*av.KeyValues{{Key: &av.Key{ID: "primary", Type: av.KeyTypeBlock}, Values: []*av.Value{{BlockID: settingsBlock, Block: &av.ValueBlock{ID: settingsBlock}}}}}}
	e.read = func(string) (*av.AttributeView, error) { return a, nil }
	e.stage = func(*av.AttributeView) error { return nil }
	e.exists = func(string) (bool, error) { return true, nil }
	e.node = func(string) (*ast.Node, *parse.Tree, error) { return nil, nil, errors.New("legacy missing") }
	if err := runAttributeViewName(nameOp(cellAV, "legacy"), e); err != nil || a.Name != "legacy" {
		t.Fatal(err)
	}
}

func TestAVNameLegacyCacheFollowsNotification(t *testing.T) {
	const id = "20261024145900-cac0001"
	if err := cache.OpenLegacyIAL(); err != nil {
		t.Fatal(err)
	}
	defer cache.CloseLegacyIAL()
	defer cache.RemoveBlockIAL(id)
	n := &ast.Node{Type: ast.NodeParagraph, ID: id}
	n.SetIALAttr(av.NodeAttrNameAvs, cellAV)
	n.SetIALAttr(av.NodeAttrViewNames, "cached old label")
	tree := &parse.Tree{ID: settingsDoc, Box: settingsBox, Path: "/" + settingsDoc + ".docj", Root: &ast.Node{Type: ast.NodeDocument, ID: settingsDoc}}
	tree.Root.AppendChild(n)
	a := &av.AttributeView{ID: cellAV, KeyValues: []*av.KeyValues{{Key: &av.Key{ID: "primary", Type: av.KeyTypeBlock}, Values: []*av.Value{{BlockID: id, Block: &av.ValueBlock{ID: id}}}}}}
	cache.PutBlockIAL(id, parse.IAL2Map(n.KramdownIAL))
	e := explicitLegacyNameFixture()
	e.read = func(string) (*av.AttributeView, error) { return a, nil }
	e.stage = func(*av.AttributeView) error { return nil }
	e.exists = func(string) (bool, error) { return true, nil }
	e.node = func(string) (*ast.Node, *parse.Tree, error) { return n, tree, nil }
	notified := false
	e.notify = func(old map[string]string, n *ast.Node) error {
		if cache.GetBlockIAL(id)[av.NodeAttrViewNames] != n.IALAttr(av.NodeAttrViewNames) {
			t.Fatal("notification observed an older cached name")
		}
		notified = true
		return nil
	}
	if err := runAttributeViewName(nameOp(cellAV, "new cached name"), e); err != nil || !notified {
		t.Fatal(err)
	}
	if cache.GetBlockIAL(id)[av.NodeAttrViewNames] != n.IALAttr(av.NodeAttrViewNames) {
		t.Fatal("published cache did not retain notification value")
	}
}

// Explicit old algorithm policy, without pretending to be a bound runtime.
func explicitLegacyNameFixture() *attributeViewNameEnvironment {
	return &attributeViewNameEnvironment{ctx: context.Background(), legacy: true, read: av.ParseAttributeView, stage: av.SaveAttributeView,
		exists: func(string) (bool, error) { return false, nil }, node: func(string) (*ast.Node, *parse.Tree, error) { return nil, nil, nil }, name: av.GetAttributeViewName, untitled: func() string { return "Untitled" },
		tree: func(*parse.Tree) error { return nil }, cache: func(id string, attrs map[string]string) error { cache.PublishBlockIAL(id, attrs); return nil }, notify: func(map[string]string, *ast.Node) error { return nil }}
}
