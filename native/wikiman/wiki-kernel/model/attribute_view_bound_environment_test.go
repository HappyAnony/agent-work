package model

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"wiki-kernel/av"
	"wiki-kernel/internal/avrender"
	"wiki-kernel/internal/workspace"
	"wiki-kernel/treenode"
)

const boundInserted = "20261025010000-mir0001"

type boundHarness struct {
	*cellHarness
	registry map[string][]string
	deleted  map[string]bool
}

func newBoundHarness(t *testing.T, l *workspace.Lease, root, page, failure string) *boundHarness {
	h := newNameHarness(t, l, root, page, failure)
	h.tx.trees = map[string]*parse.Tree{}
	h.tx.nodes = map[string]*ast.Node{}
	h.tx.luteEngine = h.env.parser()
	d := &boundHarness{h, map[string][]string{cellAV: {rowMirror}}, map[string]bool{}}
	lookup := h.env.lookup
	h.env.lookup = func(id string) *treenode.BlockTree {
		if d.deleted[id] {
			return nil
		}
		return lookup(id)
	}
	// The original SQL renderer removes field-only rows with a missing primary;
	// deleting a source block intentionally leaves those other field payloads.
	h.cell.groups = func(a *av.AttributeView) error {
		if err := h.mark("groups:" + a.ID); err != nil {
			return err
		}
		for _, v := range a.Views {
			if v.GetGroupKey(a) == nil {
				continue
			}
			err := avrender.GenerateGroups(v, a, avrender.GroupEnvironment{Now: h.cell.now, Read: h.cell.read, NewView: avrender.NewGroupView, Render: func(a *av.AttributeView, v *av.View, q string) (av.Viewable, error) {
				env := cellRenderEnvironment(h)
				env.FilterItems = func(items map[string][]*av.KeyValues) error {
					for row, kvs := range items {
						var primary *av.Value
						for _, kv := range kvs {
							if kv.Key.Type == av.KeyTypeBlock && len(kv.Values) > 0 {
								primary = kv.Values[0]
								break
							}
						}
						if primary == nil || primary.Block == nil || (!primary.IsDetached && h.env.lookup(primary.Block.ID) == nil) {
							delete(items, row)
						}
					}
					return h.ctx.Err()
				}
				switch v.LayoutType {
				case av.LayoutTypeGallery:
					return avrender.Gallery(a, v, q, env)
				case av.LayoutTypeKanban:
					return avrender.Kanban(a, v, q, env)
				default:
					return avrender.Table(a, v, q, env)
				}
			}})
			if err != nil {
				return err
			}
		}
		return h.ctx.Err()
	}
	h.env.avBound = &attributeViewBoundEnvironment{ctx: h.ctx, read: h.cell.read, stage: h.cell.stage, groups: h.cell.groups,
		exists: func(id string) (bool, error) {
			if err := h.mark("bound-exists"); err != nil {
				return false, err
			}
			return h.env.lookup(id) != nil || h.tx.nodes[id] != nil, nil
		},
		load: func(id string) (*parse.Tree, error) {
			if err := h.mark("bound-load"); err != nil {
				return nil, err
			}
			return h.tx.loadTree(id)
		},
		isMirror: func(id string) (bool, error) {
			if err := h.mark("bound-mirror"); err != nil {
				return false, err
			}
			return len(d.registry[id]) > 1, nil
		},
		removeMirror: func(id, block string) (bool, error) {
			if err := h.mark("bound-remove"); err != nil {
				return false, err
			}
			old := d.registry[id]
			var next []string
			for _, v := range old {
				if v != block && (h.env.lookup(v) != nil || h.tx.nodes[v] != nil) {
					next = append(next, v)
				}
			}
			d.registry[id] = next
			return len(old) != len(next), nil
		},
		names: h.cell.names,
		attrs: func(n *ast.Node, tree *parse.Tree, attrs map[string]string) error {
			if err := h.mark("bound-attrs"); err != nil {
				return err
			}
			return h.cell.attrs(n, tree, attrs)
		},
		cache: h.env.avName.cache, notify: h.env.avName.notify,
		tree: func(tree *parse.Tree) error {
			if err := h.mark("bound-tree"); err != nil {
				return err
			}
			return h.tx.writeTree(tree)
		},
		reload: func(id string) error {
			if err := h.mark("bound-reload"); err != nil {
				return err
			}
			h.events = append(h.events, "bound-reload:"+id)
			return nil
		},
	}
	h.env.insertAssets = func(context.Context, *parse.Tree, *Transaction) error { return h.mark("bound-assets") }
	h.env.insertReferences = func(context.Context, ...string) error { return h.mark("bound-references") }
	h.env.insertAVRegistry = func(_ context.Context, n *ast.Node) error {
		if err := h.mark("bound-register"); err != nil {
			return err
		}
		ast.Walk(n, func(n *ast.Node, entering bool) ast.WalkStatus {
			if entering && n.Type == ast.NodeAttributeView && !slices.Contains(d.registry[n.AttributeViewID], n.ID) {
				d.registry[n.AttributeViewID] = append(d.registry[n.AttributeViewID], n.ID)
			}
			return ast.WalkContinue
		})
		return nil
	}
	h.env.insertAVBind = transactionInsertAVBind
	h.env.deleteReferences = func(context.Context, ...string) error { return h.mark("bound-delete-refs") }
	h.env.deleteBlockTree = func(_ context.Context, id string) error {
		if err := h.mark("bound-delete-tree"); err != nil {
			return err
		}
		d.deleted[id] = true
		return nil
	}
	h.env.deleteAV = func(_ context.Context, n *ast.Node, t *parse.Tree, tx *Transaction) error {
		return syncDelete2AvBlock(n, t, tx)
	}
	return d
}

func boundInsert(id string) *Operation {
	return &Operation{Action: "insert", ParentID: settingsDoc, Data: `<div data-node-id="` + id + `" data-type="NodeAttributeView" data-av-id="` + cellAV + `" data-av-type="table"></div>`}
}

func assertBoundFinish(t *testing.T, d *boundHarness, before map[string]string) {
	t.Helper()
	if !d.committed || d.tx.state.Load() != 2 || d.counts["finish"] != 1 {
		t.Fatal("candidate did not finish", d.counts)
	}
	assertNameSource(t, d.cellHarness, before)
}

func TestAVBoundCollectionOwnersIdentitiesAndTrees(t *testing.T) {
	m, store, root := nameSource(t)
	for _, page := range []string{"1760000000000000002", "1760000000000000003"} {
		t.Run(page, func(t *testing.T) {
			t.Parallel()
			l, err := m.Acquire(context.Background(), store, page)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(l.Release)
			for _, scenario := range []string{"bound-id", "detached", "missing-block", "vanished-node", "independent-trees"} {
				t.Run(scenario, func(t *testing.T) {
					d := newBoundHarness(t, l, root, page, "")
					a, err := d.cell.read(cellAV)
					if err != nil {
						t.Fatal(err)
					}
					primary := a.GetBlockKeyValues()
					primary.Values[0].BlockID = "row-identity-is-different"
					switch scenario {
					case "detached":
						primary.Values[1].IsDetached = true
					case "missing-block":
						primary.Values[1].Block.ID = "missing"
					case "vanished-node":
						load := d.env.avBound.load
						d.env.avBound.load = func(id string) (*parse.Tree, error) {
							tree, err := load(id)
							if err == nil && id == cellSecond {
								if n := treenode.GetNodeInTree(tree, id); n != nil {
									n.Unlink()
								}
							}
							return tree, err
						}
					case "independent-trees":
						d.env.avBound.load = func(id string) (*parse.Tree, error) {
							bt := d.env.lookup(id)
							return d.env.load(bt.BoxID, bt.Path, d.env.parser())
						}
					}
					before := nameDigest(t, root, page)
					trees, nodes, err := d.tx.getAttrViewBoundNodes(a)
					if err != nil {
						t.Fatal(err)
					}
					want := 3
					if scenario == "detached" || scenario == "missing-block" || scenario == "vanished-node" {
						want = 2
					}
					if len(nodes) != want || trees[settingsBlock] == nil || trees["row-identity-is-different"] != nil {
						t.Fatal("binding identity/skip", len(nodes), trees)
					}
					if scenario == "independent-trees" && trees[settingsBlock] != trees[cellSecond] {
						t.Fatal("same document has independent candidates")
					}
					for _, n := range nodes {
						if !strings.Contains(trees[n.ID].Root.Text(), page) {
							t.Fatal("owner data leak", n.ID)
						}
					}
					if len(d.staged) != 0 || len(d.attrs) != 0 || len(d.notifications) != 0 {
						t.Fatal("collector wrote")
					}
					assertNameSource(t, d.cellHarness, before)
				})
			}
		})
	}
}

func TestAVBoundActualInsertDeleteTransactions(t *testing.T) {
	m, store, root := nameSource(t)
	for _, page := range []string{"1760000000000000002", "1760000000000000003"} {
		t.Run(page, func(t *testing.T) {
			t.Parallel()
			l, err := m.Acquire(context.Background(), store, page)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(l.Release)
			for _, scenario := range []string{"insert", "insert-gallery", "insert-kanban", "explicit-view", "no-current-view", "delete-last", "delete-mirror", "delete-bound-block", "delete-then-insert"} {
				t.Run(scenario, func(t *testing.T) {
					d := newBoundHarness(t, l, root, page, "")
					a, err := d.cell.read(cellAV)
					if err != nil {
						t.Fatal(err)
					}
					before := nameDigest(t, root, page)
					lastMirrorType := ""
					stage := d.env.stage
					d.env.stage = func(tree *parse.Tree) error {
						if err := stage(tree); err != nil {
							return err
						}
						if n := treenode.GetNodeInTree(tree, boundInserted); n != nil {
							lastMirrorType = n.AttributeViewType
						}
						return nil
					}
					switch scenario {
					case "insert", "insert-gallery", "insert-kanban", "explicit-view", "no-current-view":
						if scenario == "insert-gallery" {
							a.ViewID = "gallery"
						}
						if scenario == "insert-kanban" {
							a.ViewID = "kanban"
						}
						op := boundInsert(boundInserted)
						if scenario == "explicit-view" {
							a.ViewID = "gallery"
							op.Data = strings.Replace(op.Data.(string), `data-av-type="table"`, `data-av-type="table" custom-sy-av-view="custom-selected"`, 1)
						}
						if scenario == "no-current-view" {
							a.ViewID = "absent"
						}
						d.tx.DoOperations = []*Operation{op}
					case "delete-last":
						d.tx.DoOperations = []*Operation{{Action: "delete", ID: rowMirror}}
					case "delete-mirror":
						d.registry[cellAV] = append(d.registry[cellAV], "future-mirror")
						d.tx.DoOperations = []*Operation{{Action: "delete", ID: rowMirror}}
					case "delete-bound-block":
						d.tx.DoOperations = []*Operation{{Action: "delete", ID: settingsBlock}}
					case "delete-then-insert":
						d.tx.DoOperations = []*Operation{{Action: "delete", ID: rowMirror}, boundInsert(boundInserted)}
					}
					if ret := performTx(d.tx); ret != nil {
						t.Fatal(ret, d.counts)
					}
					assertBoundFinish(t, d, before)
					if scenario == "delete-bound-block" {
						if a.GetBlockKeyValues().GetValue(settingsBlock) != nil || a.GetValue("number", settingsBlock) == nil || d.counts["groups:"+cellAV] == 0 || d.staged[cellAV] == nil {
							t.Fatal("original primary-only pruning", d.counts)
						}
						return
					}
					for _, id := range []string{settingsBlock, cellSecond, nameBlock} {
						bt := d.env.lookup(id)
						tree := d.tx.trees[bt.RootID]
						if tree == nil {
							if scenario == "delete-mirror" {
								continue
							}
							t.Fatal("bound tree not loaded")
						}
						n := treenode.GetNodeInTree(tree, id)
						members := strings.Split(n.IALAttr(av.NodeAttrNameAvs), ",")
						if scenario == "delete-last" {
							if slices.Contains(members, cellAV) || !slices.Contains(members, cellDest) || strings.Contains(d.attrs[id][av.NodeAttrViewNames], a.Name) {
								t.Fatal("last mirror did not unlink", id, members, d.attrs[id])
							}
						} else if !slices.Contains(members, cellAV) {
							t.Fatal("binding lost", id, members)
						}
					}
					if scenario == "delete-last" {
						if d.counts["bound-tree"] != 2 || d.counts["name-cache"] != 3 || d.counts["name-notify"] != 3 {
							t.Fatal("dedup/cache notification effects", d.counts)
						}
						for _, tx := range d.notifications {
							for _, op := range tx.DoOperations {
								if op.Action == "updateAttrs" {
									data := op.Data.(map[string]interface{})
									old := data["old"].(map[string]string)
									next := data["new"].(map[string]string)
									if strings.Contains(old[av.NodeAttrNameAvs], cellAV) && strings.Contains(next[av.NodeAttrNameAvs], cellAV) {
										t.Fatal("notification membership did not change")
									}
								}
							}
						}
					}
					if scenario == "delete-mirror" && d.counts["bound-load"] != 0 {
						t.Fatal("surviving mirror unbound records")
					}
					if strings.HasPrefix(scenario, "insert") || scenario == "explicit-view" || scenario == "no-current-view" || scenario == "delete-then-insert" {
						n := treenode.GetNodeInTree(d.tx.trees[settingsDoc], boundInserted)
						if n == nil || !slices.Contains(d.registry[cellAV], boundInserted) {
							t.Fatal("inserted mirror absent")
						}
						if scenario == "explicit-view" {
							if n.IALAttr(av.NodeAttrView) != "custom-selected" {
								t.Fatal("explicit view overwritten", n.KramdownIAL)
							}
							if lastMirrorType != "gallery" {
								t.Fatal("snapshot sink retained old mirror layout", lastMirrorType)
							}
						} else if scenario != "no-current-view" && n.IALAttr(av.NodeAttrView) != a.ViewID {
							t.Fatal("current view not inherited", n.KramdownIAL)
						}
					}
				})
			}
		})
	}
}

func boundLargeOps() *[]*Operation {
	ops := make([]*Operation, 0, 129)
	for i := 0; i < 128; i++ {
		id := fmt.Sprintf("202610250101%02d-ins0001", i)
		data := `<div data-node-id="` + id + `" data-type="NodeParagraph"><div contenteditable="true">large ` + fmt.Sprint(i) + `</div></div>`
		if i == 0 {
			id = boundInserted
			data = boundInsert(id).Data.(string)
		}
		ops = append(ops, &Operation{Action: "insert", PreviousID: settingsBlock, Data: data})
	}
	ops = append(ops, &Operation{Action: "delete", ID: cellSecond})
	return &ops
}

func TestAVBoundLargePasteActualChainAndFailures(t *testing.T) {
	m, store, root := nameSource(t)
	page := "1760000000000000002"
	l, err := m.Acquire(context.Background(), store, page)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(l.Release)
	for _, failure := range []string{"", "bound-assets", "cancel-bound-assets", "bound-references", "bound-register", "bound-attrs", "bound-load", "read:" + cellAV, "tree-stage", "bound-delete-tree", "groups:" + cellAV, "bound-reload"} {
		t.Run(failure, func(t *testing.T) {
			d := newBoundHarness(t, l, root, page, failure)
			before := nameDigest(t, root, page)
			d.tx.DoOperations = *boundLargeOps()
			ret := performTx(d.tx)
			if failure == "" {
				if ret != nil {
					t.Fatal(ret)
				}
				assertBoundFinish(t, d, before)
				if d.counts["bound-assets"] != 128 || d.counts["bound-register"] != 128 || d.counts["bound-references"] != 128 {
					t.Fatal("large path bypassed owner ports", d.counts)
				}
				tree := d.tx.trees[settingsDoc]
				anchor := treenode.GetNodeInTree(tree, settingsBlock)
				if anchor.Next == nil || anchor.Next.Text() != "large 127" || treenode.GetNodeInTree(tree, cellSecond) != nil {
					t.Fatal("large paste order/delete lost")
				}
				if d.tx.DoOperations[0].ID != boundInserted {
					t.Fatal("operation identity not returned")
				}
			} else {
				if ret == nil || d.committed || d.tx.state.Load() != 3 || d.counts["finish"] != 0 {
					t.Fatal("large path error committed", ret, d.counts, d.tx.state.Load())
				}
				if strings.Contains(ret.msg, "PANIC RECOVERED") {
					t.Fatal(ret)
				}
				if failure == "cancel-bound-assets" && d.counts["bound-register"] != 0 {
					t.Fatal("cancelled assets continued")
				}
				assertNameSource(t, d.cellHarness, before)
			}
		})
	}
}
