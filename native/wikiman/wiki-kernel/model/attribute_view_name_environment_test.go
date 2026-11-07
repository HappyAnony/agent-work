package model

import (
	"context"
	"encoding/json"
	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"github.com/88250/lute/render"
	"github.com/siyuan-note/dataparser"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"
	"wiki-kernel/av"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/workspace"
	"wiki-kernel/treenode"
)

const nameDoc = "20261024140000-doc0002"
const nameBlock = "20261024140001-par0002"

func nameSource(t *testing.T) (*workspace.WorkspaceManager, string, string) {
	t.Helper()
	m, store, root := rowSource(t)
	for _, page := range []string{"1760000000000000002", "1760000000000000003"} {
		file := filepath.Join(root, "pages", page, "data", settingsBox, settingsDoc+".docj")
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var doc map[string]interface{}
		if err = json.Unmarshal(raw, &doc); err != nil {
			t.Fatal(err)
		}
		for _, child := range doc["Children"].([]interface{}) {
			n := child.(map[string]interface{})
			if n["ID"] == settingsBlock || n["ID"] == cellSecond {
				attrs := n["Properties"].(map[string]interface{})
				attrs[av.NodeAttrNameAvs] = cellAV + "," + cellDest
				attrs[av.NodeAttrViewNames] = page + " old names"
				attrs["custom-preserved"] = "untouched"
			}
		}
		raw, err = json.Marshal(doc)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(file, raw, 0600); err != nil {
			t.Fatal(err)
		}
		second := map[string]interface{}{"ID": nameDoc, "Type": "NodeDocument", "Spec": "1", "Properties": map[string]string{"id": nameDoc, "title": "Second owner document"}, "UnknownExtension": page, "Children": []interface{}{map[string]interface{}{"ID": nameBlock, "Type": "NodeParagraph", "Properties": map[string]string{"id": nameBlock, av.NodeAttrNameAvs: cellAV + "," + cellDest, av.NodeAttrViewNames: page + " other old"}, "Children": []interface{}{map[string]string{"Type": "NodeText", "Data": page + " second body"}}}}}
		raw, err = json.Marshal(second)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(root, "pages", page, "data", settingsBox, nameDoc+".docj"), raw, 0600); err != nil {
			t.Fatal(err)
		}
		for _, id := range []string{cellAV, cellDest} {
			file := filepath.Join(root, "pages", page, "data", "storage", "av", id+".json")
			raw, err = os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			var a av.AttributeView
			if err = json.Unmarshal(raw, &a); err != nil {
				t.Fatal(err)
			}
			primary := a.GetBlockKeyValues()
			for _, v := range primary.Values {
				v.IsDetached = false
			}
			primary.Values = append(primary.Values, &av.Value{ID: "cross-document", KeyID: primary.Key.ID, BlockID: nameBlock, Type: av.KeyTypeBlock, Block: &av.ValueBlock{ID: nameBlock, Content: page + " other"}})
			raw, err = json.Marshal(&a)
			if err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(file, raw, 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	return m, store, root
}
func newNameHarness(t *testing.T, l *workspace.Lease, root, page, failure string) *cellHarness {
	t.Helper()
	if err := l.RebuildNativeBlockTree(context.Background()); err != nil {
		t.Fatal(err)
	}
	rows, err := l.NativeBlockTrees(context.Background(), indexdb.BlockTreeFilter{Kind: "all"})
	if err != nil {
		t.Fatal(err)
	}
	registry := map[string]*treenode.BlockTree{}
	for _, r := range rows {
		registry[r.ID] = &treenode.BlockTree{ID: r.ID, RootID: r.RootID, BoxID: r.BoxID, Path: r.Path}
	}
	h := newCellHarness(t, l, root, page, failure)
	h.env.lookup = func(id string) *treenode.BlockTree { return registry[id] }
	h.env.load = func(box, p string, engine *lute.Lute) (*parse.Tree, error) {
		if err := h.mark("load"); err != nil {
			return nil, err
		}
		id := strings.TrimSuffix(path.Base(p), ".docj")
		raw, err := l.ReadDocument(h.ctx, box, id)
		if err != nil {
			return nil, err
		}
		tree, _, err := dataparser.ParseJSON(raw, engine.ParseOptions)
		if err != nil {
			return nil, err
		}
		tree.ID, tree.Box, tree.Path = id, box, p
		return tree, nil
	}
	h.env.avName = &attributeViewNameEnvironment{ctx: h.ctx, read: h.cell.read, stage: h.cell.stage,
		exists: func(id string) (bool, error) {
			if err := h.mark("exists"); err != nil {
				return false, err
			}
			return registry[id] != nil, nil
		}, node: h.cell.node,
		name: func(id string) (string, error) {
			if err := h.mark("name"); err != nil {
				return "", err
			}
			a, err := h.cell.read(id)
			if err != nil {
				return "", err
			}
			return a.Name, nil
		}, untitled: func() string { h.mark("untitled"); return page + " owner untitled" },
		tree: func(tree *parse.Tree) error {
			if err := h.mark("name-tree"); err != nil {
				return err
			}
			return h.tx.writeTree(tree)
		}, cache: func(id string, attrs map[string]string) error {
			if err := h.mark("name-cache"); err != nil {
				return err
			}
			h.env.cacheIAL(id, attrs)
			return nil
		}, notify: func(old map[string]string, n *ast.Node) error {
			if err := h.mark("name-notify"); err != nil {
				return err
			}
			h.notifications = append(h.notifications, blockAttrTransactions(old, n)...)
			return nil
		},
	}
	return h
}
func nameOp(id, text string) *Operation {
	return &Operation{Action: "setAttrViewName", ID: id, Data: text}
}
func nameDigest(t *testing.T, root, page string) map[string]string {
	t.Helper()
	m := cellDigest(t, root, page)
	for _, f := range []string{filepath.Join(settingsBox, nameDoc+".docj"), "storage/av/blocks.msgpack"} {
		raw, err := os.ReadFile(filepath.Join(root, "pages", page, "data", f))
		if err != nil {
			t.Fatal(err)
		}
		m[f] = string(raw)
	}
	return m
}
func assertNameSource(t *testing.T, h *cellHarness, before map[string]string) {
	t.Helper()
	if !reflect.DeepEqual(before, nameDigest(t, h.root, h.page)) {
		t.Fatal("rename candidate wrote source")
	}
	if h.tx.m != nil {
		if !h.tx.m.TryLock() {
			t.Fatal("rename lock leaked")
		}
		h.tx.m.Unlock()
	}
}

func TestAVNameOwnersRulesBoundRefreshAndNotifications(t *testing.T) {
	m, store, root := nameSource(t)
	for _, page := range []string{"1760000000000000002", "1760000000000000003"} {
		t.Run(page, func(t *testing.T) {
			t.Parallel()
			l, err := m.Acquire(context.Background(), store, page)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(l.Release)
			for _, scenario := range []string{"trim", "newline", "unicode-limit", "empty", "other-empty", "detached", "missing-registry", "missing-node", "no-bindings", "empty-membership", "multiple"} {
				t.Run(scenario, func(t *testing.T) {
					h := newNameHarness(t, l, root, page, "")
					before := nameDigest(t, root, page)
					a, err := h.cell.read(cellAV)
					if err != nil {
						t.Fatal(err)
					}
					text := "  renamed  "
					want := "renamed"
					expected := 3
					switch scenario {
					case "newline":
						text = "  first\nsecond\rthird  "
						want = "first second\rthird"
					case "unicode-limit":
						text = strings.Repeat("知🙂", 300)
						want = strings.Repeat("知🙂", 256)
					case "empty":
						text = " \n\t "
						want = ""
					case "other-empty":
						b, err := h.cell.read(cellDest)
						if err != nil {
							t.Fatal(err)
						}
						b.Name = ""
					case "detached":
						a.GetBlockKeyValues().Values[0].IsDetached = true
						expected = 2
					case "missing-registry":
						a.GetBlockKeyValues().Values[0].Block.ID = "20261024140002-gon0001"
						expected = 2
					case "missing-node":
						node := h.env.avName.node
						h.env.avName.node = func(id string) (*ast.Node, *parse.Tree, error) {
							n, t, err := node(id)
							if id == settingsBlock {
								n = nil
							}
							return n, t, err
						}
						expected = 2
					case "no-bindings":
						for _, v := range a.GetBlockKeyValues().Values {
							v.IsDetached = true
						}
						expected = 0
					case "empty-membership":
						load := h.env.load
						h.env.load = func(box, p string, engine *lute.Lute) (*parse.Tree, error) {
							tree, err := load(box, p, engine)
							if err == nil {
								if n := treenode.GetNodeInTree(tree, settingsBlock); n != nil {
									n.RemoveIALAttr(av.NodeAttrNameAvs)
								}
							}
							return tree, err
						}
					}
					h.counts = map[string]int{}
					op := nameOp(cellAV, text)
					op.AvID = cellRelated // Original ID, never AvID, selects the renamed database.
					h.tx.DoOperations = []*Operation{op}
					if scenario == "multiple" {
						h.tx.DoOperations = append(h.tx.DoOperations, nameOp(cellDest, " second renamed "), nameOp(cellAV, " final renamed "))
						want = "final renamed"
						expected = 9
					}
					if ret := performTx(h.tx); ret != nil || !h.committed {
						t.Fatal(ret, h.counts)
					}
					if a.Name != want || utf8.RuneCountInString(a.Name) > 512 || h.cache[cellRelated] != nil {
						t.Fatal("rename target/rules", a.Name)
					}
					if len(h.notifications) != expected || h.counts["name-cache"] != expected {
						t.Fatal("bound refresh count", len(h.notifications), h.counts)
					}
					if expected > 0 {
						attrs := h.attrs[nameBlock]
						label := want
						if label == "" {
							label = page + " owner untitled"
						}
						if !strings.Contains(attrs[av.NodeAttrViewNames], ">"+label+"</span>") || !strings.Contains(attrs[av.NodeAttrNameAvs], cellDest) {
							t.Fatal("fresh current name or other bindings lost", attrs)
						}
						other := page + " " + cellDest
						if scenario == "other-empty" {
							other = page + " owner untitled"
						}
						if scenario == "multiple" {
							other = "second renamed"
						}
						if !strings.Contains(attrs[av.NodeAttrViewNames], other) {
							t.Fatal("other database name lost", attrs)
						}
						for _, tx := range h.notifications {
							op := tx.DoOperations[0]
							if op.Action != "updateAttrs" || len(tx.UndoOperations) != 0 {
								t.Fatal("original notification shape")
							}
							payload := op.Data.(map[string]interface{})
							old := payload["old"].(map[string]string)
							next := payload["new"].(map[string]string)
							if old[av.NodeAttrViewNames] == next[av.NodeAttrViewNames] && scenario != "multiple" {
								t.Fatal("old payload mutated")
							}
						}
						if scenario != "detached" && scenario != "missing-registry" && scenario != "missing-node" {
							if scenario == "empty-membership" {
								if h.attrs[settingsBlock][av.NodeAttrViewNames] != "" {
									t.Fatal("empty membership changed")
								}
							} else if h.attrs[settingsBlock]["custom-preserved"] != "untouched" {
								t.Fatal("unrelated attribute changed")
							}
						}
					}
					assertNameSource(t, h, before)
				})
			}
		})
	}
}

func TestAVNameDerivedLabelsStayOutOfDocumentSource(t *testing.T) {
	m, store, root := nameSource(t)
	page := "1760000000000000002"
	l, err := m.Acquire(context.Background(), store, page)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(l.Release)
	h := newNameHarness(t, l, root, page, "")
	before := nameDigest(t, root, page)
	h.tx.DoOperations = []*Operation{nameOp(cellAV, "Derived label")}
	if ret := performTx(h.tx); ret != nil {
		t.Fatal(ret)
	}
	for _, id := range []string{settingsDoc, nameDoc} {
		tree := h.tx.trees[id]
		raw := render.NewJSONRenderer(tree, h.env.parser().RenderOptions).Render()
		if strings.Contains(string(raw), `"av-names"`) || !strings.Contains(string(raw), `"custom-avs"`) {
			t.Fatal("derived labels changed authoritative serialization", string(raw))
		}
	}
	if !strings.Contains(h.attrs[settingsBlock][av.NodeAttrViewNames], "Derived label") {
		t.Fatal("candidate cache lost derived label")
	}
	assertNameSource(t, h, before)
}
