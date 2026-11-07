package model

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"github.com/vmihailenco/msgpack/v5"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"wiki-kernel/av"
	"wiki-kernel/conf"
	"wiki-kernel/internal/workspace"
)

const rowMirror = "20261024090000-mir0001"
const rowFirst = "20261024090001-row0001"
const rowNext = "20261024090002-row0002"

func rowSource(t *testing.T) (*workspace.WorkspaceManager, string, string) {
	t.Helper()
	_, _, root := fieldSource(t)
	m := workspace.NewManager(workspace.Options{Configuration: func(string, string) (conf.AppData, error) {
		return conf.AppData{Editor: conf.NewEditor(), Export: conf.NewExport(), Search: conf.NewSearch()}, nil
	}})
	store, err := m.OpenStore(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := m.Drain(context.Background()); err != nil {
			t.Error(err)
		}
	})
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
		doc["UnknownExtension"] = map[string]string{"owner": page}
		children := doc["Children"].([]interface{})
		doc["Children"] = append(children, map[string]interface{}{"ID": rowMirror, "Type": "NodeAttributeView", "AttributeViewID": cellAV, "Properties": map[string]string{"id": rowMirror}})
		raw, err = json.Marshal(doc)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(file, raw, 0600); err != nil {
			t.Fatal(err)
		}
		raw, err = msgpack.Marshal(map[string][]string{cellAV: {rowMirror, rowMirror, "20261024090003-gon0001"}})
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(root, "pages", page, "data", "storage", "av", "blocks.msgpack"), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	return m, store, root
}

type rowHarness struct {
	*cellHarness
	histories  []attributeViewRowHistory
	historyEnv *attributeViewRowHistoryEnvironment
}

func newRowHarness(t *testing.T, l *workspace.Lease, root, page, failure string) *rowHarness {
	h := orderHarness(t, l, root, page, failure)
	h.tx.luteEngine = h.env.parser()
	h.tx.trees = map[string]*parse.Tree{}
	r := &rowHarness{cellHarness: h}
	history := &attributeViewRowHistoryEnvironment{
		mirrors: func(id string) ([]string, error) {
			if err := h.mark("mirrors"); err != nil {
				return nil, err
			}
			ret, err := l.NativeAttributeViewPrimaryValues(h.ctx, id, "", 1, 16)
			return ret.DatabaseBlockIDs, err
		},
		tree: func(id string) (*parse.Tree, error) {
			if err := h.mark("history-tree"); err != nil {
				return nil, err
			}
			return h.tx.loadTree(id)
		},
		document: func(tree *parse.Tree) ([]byte, error) {
			if err := h.mark("history-document"); err != nil {
				return nil, err
			}
			return l.ReadDocument(h.ctx, tree.Box, tree.ID)
		},
		attribute: func(a *av.AttributeView) ([]byte, error) {
			if err := h.mark("history-av"); err != nil {
				return nil, err
			}
			return json.Marshal(a)
		},
		stage: func(p attributeViewRowHistory) error {
			if err := h.mark("history-stage"); err != nil {
				return err
			}
			r.histories = append(r.histories, p)
			return nil
		},
	}
	r.historyEnv = history
	h.env.avRows = &attributeViewRowEnvironment{cell: h.cell, defaults: h.env.avOrder.defaults, view: h.env.avOrder.view,
		text: func(n *ast.Node) (string, string, error) {
			return attributeViewBlockTextWithServices(n, "", h.cell.blockText, func() string { return page + " untitled" })
		},
		optionalNode: func(id string) (*ast.Node, *parse.Tree, error) {
			if err := h.mark("optional-node"); err != nil {
				return nil, nil, err
			}
			if h.env.lookup(id) == nil {
				return nil, nil, nil
			}
			return h.cell.node(id)
		},
		exists: func(id string) (bool, error) {
			if err := h.mark("exists"); err != nil {
				return false, err
			}
			_, err := l.ReadNativeAttributeView(h.ctx, id)
			if errors.Is(err, fs.ErrNotExist) {
				return false, nil
			}
			return err == nil, err
		},
		duplicate: func(a *av.AttributeView) error {
			if err := h.mark("duplicate"); err != nil {
				return err
			}
			h.events = append(h.events, fmt.Sprintf(page+" duplicate %s", a.Name))
			return nil
		},
		history: func(a *av.AttributeView) error { return stageAttributeViewRowHistory(h.ctx, a, history) },
	}
	return r
}
func rowInsert(layout av.LayoutType, detached bool) *Operation {
	return &Operation{Action: "insertAttrViewBlock", AvID: cellAV, ViewID: string(layout), Srcs: []map[string]interface{}{{"itemID": rowFirst, "id": settingsBlock, "content": "first", "isDetached": detached}, {"itemID": rowNext, "content": "second", "isDetached": true}}}
}
func rowPrepare(t *testing.T, h *rowHarness, layout av.LayoutType) *av.AttributeView {
	a := orderPrepare(t, h.cellHarness, layout)
	// Start detached rows without stale bindings, as a real detached record does.
	for _, v := range a.GetBlockKeyValues().Values {
		v.Block.ID = ""
	}
	return a
}
func assertRowsUnchanged(t *testing.T, h *rowHarness, before map[string]string, registry []byte) {
	t.Helper()
	assertFieldSourceUnchanged(t, h.cellHarness, before)
	after, err := os.ReadFile(filepath.Join(h.root, "pages", h.page, "data", "storage", "av", "blocks.msgpack"))
	if err != nil || !slices.Equal(registry, after) {
		t.Fatal("mirror registry changed", err)
	}
}
func TestAVRowsOwnersLayoutsInsertRemoveAndHistory(t *testing.T) {
	m, store, root := rowSource(t)
	for _, page := range []string{"1760000000000000002", "1760000000000000003"} {
		t.Run(page, func(t *testing.T) {
			t.Parallel()
			l, err := m.Acquire(context.Background(), store, page)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(l.Release)
			for _, layout := range []av.LayoutType{av.LayoutTypeTable, av.LayoutTypeGallery, av.LayoutTypeKanban} {
				for _, detached := range []bool{true, false} {
					for _, previous := range []string{"", settingsBlock, "gone"} {
						t.Run(fmt.Sprint(layout, detached, previous), func(t *testing.T) {
							h := newRowHarness(t, l, root, page, "")
							a := rowPrepare(t, h, layout)
							before := cellDigest(t, root, page)
							reg, err := os.ReadFile(filepath.Join(root, "pages", page, "data", "storage", "av", "blocks.msgpack"))
							if err != nil {
								t.Fatal(err)
							}
							date, _ := a.GetKey("date")
							date.Date = &av.Date{AutoFillNow: true, FillSpecificTime: false}
							insert := rowInsert(layout, detached)
							insert.PreviousID = previous
							original, _ := json.Marshal(insert.Srcs)
							h.tx.DoOperations = []*Operation{insert}
							if ret := performTx(h.tx); ret != nil || !h.committed {
								t.Fatal(ret, h.counts)
							}
							raw, _ := json.Marshal(insert.Srcs)
							if string(raw) != string(original) {
								t.Fatal("caller batch mutated")
							}
							for _, v := range a.Views {
								want := []string{rowFirst, rowNext, settingsBlock, cellSecond}
								if previous == settingsBlock {
									want = []string{settingsBlock, rowFirst, rowNext, cellSecond}
								}
								if previous == "gone" {
									want = []string{settingsBlock, cellSecond, rowNext, rowFirst}
								}
								if !reflect.DeepEqual(v.ItemIDs, want) {
									t.Fatal("original insertion order", v.ID, v.ItemIDs, want)
								}
							}
							first := a.GetBlockKeyValues().GetValue(rowFirst)
							if first == nil || first.IsDetached != detached || (!detached && (!strings.Contains(first.Block.Content, page) || first.Block.ID != settingsBlock)) {
								t.Fatal("bound owner/text", first)
							}
							for _, row := range []string{rowFirst, rowNext} {
								v := a.GetValue("date", row)
								if v == nil || v.Date == nil || v.Date.Content != 12345 || !v.Date.IsNotTime || v.UpdatedAt != 13345 {
									t.Fatal("date fill", v)
								}
							}
							assertRowsUnchanged(t, h, before, reg)
							// A fresh transaction retains all staged candidates, but cannot publish them.
							h.tx.state.Store(0)
							h.committed = false
							h.tx.DoOperations = []*Operation{{Action: "removeAttrViewBlock", AvID: cellAV, SrcIDs: []string{rowFirst, rowNext}}}
							if ret := performTx(h.tx); ret != nil || !h.committed {
								t.Fatal(ret, h.counts)
							}
							for _, kv := range a.KeyValues {
								if kv.GetValue(rowFirst) != nil || kv.GetValue(rowNext) != nil {
									t.Fatal("deleted row survived", kv.Key.ID)
								}
							}
							for _, v := range a.Views {
								if slices.Contains(v.ItemIDs, rowFirst) || slices.Contains(v.ItemIDs, rowNext) {
									t.Fatal("deleted order survived")
								}
							}
							if !detached && h.attrs[settingsBlock][av.NodeAttrNameAvs] != "" {
								t.Fatal("binding survived removal", h.attrs)
							}
							if len(h.histories) != 1 || len(h.histories[0].Files) != 2 || h.histories[0].AVID != cellAV || h.histories[0].Operation != HistoryOpUpdate {
								t.Fatal("mirror history missing", h.histories)
							}
							snapshot := h.histories[0]
							if !strings.Contains(string(snapshot.Files[0].Data), `"UnknownExtension":{"owner":"`+page+`"}`) || snapshot.Files[0].Path != settingsBox+"/"+settingsDoc+".docj" {
								t.Fatal("raw owner history lost")
							}
							var saved av.AttributeView
							if err = json.Unmarshal(snapshot.Files[1].Data, &saved); err != nil || saved.GetBlockKeyValues().GetValue(rowFirst) != nil {
								t.Fatal("post-remove AV history", err)
							}
							assertRowsUnchanged(t, h, before, reg)
						})
					}
				}
			}
		})
	}
}

func TestAVRowsReplaceDuplicateAndOriginalDetachedSemantics(t *testing.T) {
	m, store, root := rowSource(t)
	for _, page := range []string{"1760000000000000002", "1760000000000000003"} {
		t.Run(page, func(t *testing.T) {
			t.Parallel()
			l, err := m.Acquire(context.Background(), store, page)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(l.Release)
			for _, layout := range []av.LayoutType{av.LayoutTypeTable, av.LayoutTypeGallery, av.LayoutTypeKanban} {
				for _, scenario := range []string{"bind", "rebind", "duplicate-replace", "duplicate-insert", "detach", "missing-row"} {
					t.Run(string(layout)+scenario, func(t *testing.T) {
						h := newRowHarness(t, l, root, page, "")
						a := rowPrepare(t, h, layout)
						before := cellDigest(t, root, page)
						primary := a.GetBlockKeyValues()
						first := primary.GetValue(settingsBlock)
						op := &Operation{Action: "replaceAttrViewBlock", AvID: cellAV, PreviousID: settingsBlock, NextID: settingsBlock}
						switch scenario {
						case "rebind":
							first.IsDetached = false
							first.Block.ID = cellSecond
						case "duplicate-replace":
							primary.GetValue(cellSecond).Block.ID = settingsBlock
							primary.GetValue(cellSecond).IsDetached = false
						case "duplicate-insert":
							first.Block.ID = settingsBlock
							first.IsDetached = false
							op = rowInsert(layout, false)
							op.Srcs = op.Srcs[:1]
						case "detach":
							first.Block.ID = settingsBlock
							first.IsDetached = false
							op.IsDetached = true
						case "missing-row":
							op.PreviousID = "gone"
						}
						h.tx.DoOperations = []*Operation{op}
						if ret := performTx(h.tx); ret != nil || !h.committed {
							t.Fatal(ret, h.counts)
						}
						switch scenario {
						case "duplicate-insert":
							if len(primary.Values) != 2 || h.counts["duplicate"] != 1 || !strings.Contains(strings.Join(h.events, " "), page+" duplicate ") {
								t.Fatal("duplicate message/row", h.events)
							}
						case "duplicate-replace":
							if primary.GetValue(cellSecond).UpdatedAt != 12345 || !first.IsDetached {
								t.Fatal("duplicate rebind changed original row")
							}
						case "detach":
							if !first.IsDetached || first.Block.ID != "" || h.counts["attrs"] != 0 {
								t.Fatal("original detached conversion changed")
							}
						case "missing-row":
							if h.counts["attrs"] != 0 {
								t.Fatal("missing row bound a node")
							}
						default:
							if first.IsDetached || first.Block.ID != settingsBlock || !strings.Contains(first.Block.Content, page) || !strings.Contains(h.attrs[settingsBlock][av.NodeAttrNameAvs], cellAV) {
								t.Fatal("rebind owner failure", first, h.attrs)
							}
						}
						assertFieldSourceUnchanged(t, h.cellHarness, before)
					})
				}
			}
		})
	}
}
