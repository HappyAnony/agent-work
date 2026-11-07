package model

import (
	"context"
	"errors"
	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"strings"
	"testing"
	"wiki-kernel/av"
)

func rowFailurePrepare(t *testing.T, h *rowHarness, action string) *Operation {
	a := rowPrepare(t, h, av.LayoutTypeTable)
	op := rowInsert(av.LayoutTypeTable, false)
	if action == "removeAttrViewBlock" {
		op = &Operation{Action: action, AvID: cellAV, SrcIDs: []string{settingsBlock}}
		v := a.GetBlockKeyValues().GetValue(settingsBlock)
		v.IsDetached = false
		v.Block.ID = settingsBlock
		// Removal must prune missing databases and retain/name existing ones.
		load := h.env.load
		h.env.load = func(box, p string, engine *lute.Lute) (*parse.Tree, error) {
			tree, err := load(box, p, engine)
			if err == nil {
				tree.Root.FirstChild.SetIALAttr(av.NodeAttrNameAvs, cellAV+","+cellDest+",20261024090003-gon0001")
			}
			return tree, err
		}
	}
	if action == "replaceAttrViewBlock" {
		op = &Operation{Action: action, AvID: cellAV, PreviousID: settingsBlock, NextID: cellSecond}
		v := a.GetBlockKeyValues().GetValue(settingsBlock)
		v.IsDetached = false
		v.Block.ID = settingsBlock
	}
	h.counts = map[string]int{}
	return op
}
func TestAVRowsFailuresCancellationAndAdmission(t *testing.T) {
	m, store, root := rowSource(t)
	for _, page := range []string{"1760000000000000002", "1760000000000000003"} {
		t.Run(page, func(t *testing.T) {
			t.Parallel()
			l, err := m.Acquire(context.Background(), store, page)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(l.Release)
			for _, action := range []string{"insertAttrViewBlock", "removeAttrViewBlock", "replaceAttrViewBlock"} {
				points := []string{"read:" + cellAV, "stage:" + cellAV, "groups:" + cellAV, "finish", "node", "attrs", "tree-stage", "notify", "names"}
				if action == "insertAttrViewBlock" || action == "replaceAttrViewBlock" {
					points = append(points, "text")
				}
				if action != "insertAttrViewBlock" {
					points = append(points, "related", "enqueue")
				}
				if action == "removeAttrViewBlock" {
					points = append(points, "optional-node", "exists", "mirrors", "history-tree", "history-document", "history-av", "history-stage")
				}
				for _, point := range points {
					for _, prefix := range []string{"", "cancel-"} {
						failure := prefix + point
						t.Run(action+failure, func(t *testing.T) {
							h := newRowHarness(t, l, root, page, "")
							op := rowFailurePrepare(t, h, action)
							before := cellDigest(t, root, page)
							h.failure = failure
							h.tx.DoOperations = []*Operation{op}
							ret := performTx(h.tx)
							if ret == nil || h.committed || h.tx.state.Load() != 3 {
								t.Fatal("failure/cancellation escaped", failure, ret, h.counts)
							}
							if h.counts[point] == 0 {
								t.Fatal("failure point untested", point)
							}
							if point != "finish" && h.counts["finish"] != 0 {
								t.Fatal("failed operation reached finish")
							}
							assertFieldSourceUnchanged(t, h.cellHarness, before)
						})
					}
				}
				for _, missing := range []string{"environment", "cell", "read", "stage", "groups", "node", "text", "names", "attrs", "related", "now", "newID", "defaults", "view", "duplicate", "optional", "exists", "history"} {
					if (missing == "defaults" || missing == "view" || missing == "duplicate") && action != "insertAttrViewBlock" {
						continue
					}
					if (missing == "optional" || missing == "exists" || missing == "history") && action != "removeAttrViewBlock" {
						continue
					}
					if missing == "text" && action == "removeAttrViewBlock" {
						continue
					}
					t.Run(action+"missing-"+missing, func(t *testing.T) {
						h := newRowHarness(t, l, root, page, "")
						op := rowFailurePrepare(t, h, action)
						before := cellDigest(t, root, page)
						e := h.env.avRows
						c := *e.cell
						e.cell = &c
						switch missing {
						case "environment":
							h.env.avRows = nil
						case "cell":
							e.cell = nil
						case "read":
							c.read = nil
						case "stage":
							c.stage = nil
						case "groups":
							c.groups = nil
						case "node":
							c.node = nil
						case "text":
							e.text = nil
						case "names":
							c.names = nil
						case "attrs":
							c.attrs = nil
						case "related":
							c.related = nil
						case "now":
							c.now = nil
						case "newID":
							c.newID = nil
						case "defaults":
							e.defaults = nil
						case "view":
							e.view = nil
						case "duplicate":
							e.duplicate = nil
						case "optional":
							e.optionalNode = nil
						case "exists":
							e.exists = nil
						case "history":
							e.history = nil
						}
						h.tx.DoOperations = []*Operation{op}
						if ret := performTx(h.tx); ret == nil || len(h.counts) != 0 || h.committed {
							t.Fatal("missing dependency admitted", ret, h.counts)
						}
						assertFieldSourceUnchanged(t, h.cellHarness, before)
					})
				}
			}
			for _, scenario := range []string{"bad-second", "unknown", "nil-source", "nil-primary", "missing-view", "missing-group", "nil-date", "missing-node", "known-load", "pre-cancel", "nil-tree"} {
				t.Run(scenario, func(t *testing.T) {
					h := newRowHarness(t, l, root, page, "")
					a := rowPrepare(t, h, av.LayoutTypeTable)
					before := cellDigest(t, root, page)
					op := rowInsert(av.LayoutTypeTable, false)
					ops := []*Operation{op}
					switch scenario {
					case "bad-second":
						op.Srcs = append(op.Srcs, map[string]interface{}{"isDetached": "false"})
					case "unknown":
						ops = append(ops, &Operation{Action: "unaudited"})
					case "nil-source":
						h.cell.read = func(string) (*av.AttributeView, error) { return nil, nil }
					case "nil-primary":
						a.GetBlockKeyValues().Values[0].Block = nil
					case "missing-view":
						op.ViewID = "gone"
					case "missing-group":
						op.GroupID = "gone"
					case "nil-date":
						k, _ := a.GetKey("date")
						k.Date = &av.Date{AutoFillNow: true}
						a.KeyValues[3].Values = append(a.KeyValues[3].Values, &av.Value{BlockID: rowFirst, IsRenderAutoFill: true})
					case "missing-node":
						op.Srcs[0]["id"] = "20261024090003-gon0001"
					case "known-load":
						h.failure = "load"
					case "pre-cancel":
						h.cancel()
					case "nil-tree":
						h.cell.node = func(string) (*ast.Node, *parse.Tree, error) { return nil, nil, nil }
					}
					h.tx.DoOperations = ops
					ret := performTx(h.tx)
					if ret == nil || h.committed {
						t.Fatal("malformed candidate admitted", ret)
					}
					if (scenario == "bad-second" || scenario == "unknown" || scenario == "pre-cancel") && len(h.counts) != 0 {
						t.Fatal("partial admission", h.counts)
					}
					assertFieldSourceUnchanged(t, h.cellHarness, before)
				})
			}
		})
	}
}

func TestAVRowsDuplicateNotificationAndHistoryFailures(t *testing.T) {
	m, store, root := rowSource(t)
	l, err := m.Acquire(context.Background(), store, "1760000000000000002")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Release()
	for _, failure := range []string{"duplicate", "cancel-duplicate"} {
		t.Run(failure, func(t *testing.T) {
			h := newRowHarness(t, l, root, "1760000000000000002", "")
			a := rowPrepare(t, h, av.LayoutTypeTable)
			a.GetBlockKeyValues().Values[0].Block.ID = settingsBlock
			op := rowInsert(av.LayoutTypeTable, false)
			op.Srcs = op.Srcs[:1]
			h.failure = failure
			h.tx.DoOperations = []*Operation{op}
			if ret := performTx(h.tx); ret == nil || h.committed || h.counts["duplicate"] != 1 {
				t.Fatal(ret, h.counts)
			}
		})
	}
	for _, scenario := range []string{"nil", "unsafe-path", "missing-tree", "document", "attribute", "stage", "cancel"} {
		t.Run(scenario, func(t *testing.T) {
			h := newRowHarness(t, l, root, "1760000000000000002", "")
			a := rowPrepare(t, h, av.LayoutTypeTable)
			e := *h.historyEnv
			switch scenario {
			case "nil":
				e.stage = nil
			case "unsafe-path":
				e.tree = func(string) (*parse.Tree, error) {
					return &parse.Tree{ID: settingsDoc, Box: settingsBox, Path: "/../escape.docj"}, nil
				}
			case "missing-tree":
				e.tree = func(string) (*parse.Tree, error) { return nil, nil }
			case "document":
				e.document = func(*parse.Tree) ([]byte, error) { return nil, errors.New("document") }
			case "attribute":
				e.attribute = func(*av.AttributeView) ([]byte, error) { return nil, errors.New("attribute") }
			case "stage":
				e.stage = func(attributeViewRowHistory) error { return errors.New("stage") }
			case "cancel":
				h.cancel()
			}
			if err := stageAttributeViewRowHistory(h.ctx, a, &e); err == nil || len(h.histories) != 0 {
				t.Fatal("history failure escaped", err)
			}
		})
	}
	// Raw snapshots are detached from callback buffers, including unknown extensions.
	h := newRowHarness(t, l, root, "1760000000000000002", "")
	a := rowPrepare(t, h, av.LayoutTypeTable)
	raw := []byte(`{"unknown":"snapshot"}`)
	h.historyEnv.document = func(*parse.Tree) ([]byte, error) { return raw, nil }
	h.historyEnv.attribute = func(*av.AttributeView) ([]byte, error) { return raw, nil }
	if err := stageAttributeViewRowHistory(h.ctx, a, h.historyEnv); err != nil {
		t.Fatal(err)
	}
	raw[2] = 'X'
	for _, f := range h.histories[0].Files {
		if !strings.Contains(string(f.Data), "unknown") || len(f.SHA256) != 64 {
			t.Fatal("history alias/hash")
		}
	}
}
