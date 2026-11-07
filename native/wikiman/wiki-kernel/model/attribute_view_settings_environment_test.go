package model

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/88250/lute"
	"github.com/88250/lute/parse"
	"github.com/siyuan-note/dataparser"
	"wiki-kernel/av"
	"wiki-kernel/internal/nativeparser"
	"wiki-kernel/internal/workspace"
	"wiki-kernel/treenode"
)

const settingsAV = "20261023090600-av00001"
const settingsBox = "20261019090000-box0001"
const settingsDoc = "20261019090001-doc0001"
const settingsBlock = "20261019090002-par0001"

func settingsSource(t *testing.T) (*workspace.WorkspaceManager, string) {
	t.Helper()
	root := t.TempDir()
	if err := os.CopyFS(root, os.DirFS("../../fixtures/shared-wiki/valid")); err != nil {
		t.Fatal(err)
	}
	pages := []string{"1760000000000000002", "1760000000000000003"}
	b := filepath.Join(root, "pages", pages[1], "data")
	if err := os.MkdirAll(b, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.CopyFS(b, os.DirFS(filepath.Join(root, "pages", pages[0], "data"))); err != nil {
		t.Fatal(err)
	}
	for i, page := range pages {
		file := filepath.Join(root, "pages", page, "data", settingsBox, settingsDoc+".docj")
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		view := []string{"first", "second"}[i]
		raw = []byte(strings.Replace(string(raw), `"id": "`+settingsBlock+`"`, `"id": "`+settingsBlock+`", "custom-sy-av-view":"`+view+`"`, 1))
		if err = os.WriteFile(file, raw, 0600); err != nil {
			t.Fatal(err)
		}
		a := &av.AttributeView{Spec: 4, ID: settingsAV, Name: page, ViewID: "default"}
		for _, id := range []string{"first", "second", "default"} {
			a.Views = append(a.Views, &av.View{ID: id, PageSize: 17, Filters: []*av.ViewFilter{{Column: "old"}}, Sorts: []*av.ViewSort{{Column: "old"}}})
		}
		raw, err = json.Marshal(a)
		if err != nil {
			t.Fatal(err)
		}
		dir := filepath.Join(root, "pages", page, "data", "storage", "av")
		if err = os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(dir, settingsAV+".json"), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	m := workspace.NewManager(workspace.Options{})
	store, err := m.OpenStore(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := m.Drain(context.Background()); err != nil {
			t.Error(err)
		}
	})
	return m, store
}

func settingsOperation(action string) *Operation {
	op := &Operation{Action: action, AvID: settingsAV, BlockID: settingsBlock}
	switch action {
	case "setAttrViewFilters":
		op.Data = []interface{}{map[string]interface{}{"column": "number", "operator": "Is greater", "value": map[string]interface{}{"type": "number", "number": map[string]interface{}{"content": 2, "isNotEmpty": true}}}}
	case "setAttrViewSorts":
		op.Data = []interface{}{map[string]interface{}{"column": "number", "order": "DESC"}}
	case "setAttrViewPageSize":
		op.Data = float64(9.75)
	}
	return op
}

func TestOriginalAVViewSettingsContextOwnersFallbacksAndFailures(t *testing.T) {
	m, store := settingsSource(t)
	for i, page := range []string{"1760000000000000002", "1760000000000000003"} {
		lease, err := m.Acquire(context.Background(), store, page)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(lease.Release)
		before, err := lease.ReadNativeAttributeView(context.Background(), settingsAV)
		if err != nil {
			t.Fatal(err)
		}
		docBefore, err := lease.ReadDocument(context.Background(), settingsBox, settingsDoc)
		if err != nil {
			t.Fatal(err)
		}
		for _, action := range []string{"setAttrViewFilters", "setAttrViewSorts", "setAttrViewPageSize"} {
			for _, variant := range []string{"selected", "empty-block", "missing-block", "missing-node", "stale-view", "no-attribute", "stale-default", "first-fallback", "read", "load", "nil-tree", "stage", "finish", "cancel-read", "cancel-load", "cancel-stage", "no-views", "bad-data", "decode", "marshal"} {
				if action == "setAttrViewPageSize" && (variant == "decode" || variant == "marshal") {
					continue
				}
				t.Run(page+"/"+action+"/"+variant, func(t *testing.T) {
					ctx, cancel := context.WithCancel(context.Background())
					defer cancel()
					counts := map[string]int{}
					want := errors.New("synthetic " + variant)
					mark := func(name string) error {
						counts[name]++
						if variant == name {
							return want
						}
						if variant == "cancel-"+name {
							cancel()
						}
						return ctx.Err()
					}
					var staged *av.AttributeView
					op := settingsOperation(action)
					if variant == "empty-block" {
						op.BlockID = ""
					}
					if variant == "missing-node" {
						op.BlockID = "20261023090000-gone001"
					}
					if variant == "bad-data" {
						op.Data = 42
					}
					if variant == "decode" {
						op.Data = []interface{}{42}
					}
					if variant == "marshal" {
						op.Data = []interface{}{func() {}}
					}
					env := &transactionEnvironment{ctx: ctx,
						parser: func() *lute.Lute { counts["parser"]++; return nativeparser.NewLute(nativeparser.Defaults()) },
						lookup: func(id string) *treenode.BlockTree {
							counts["lookup"]++
							if variant == "missing-block" {
								return nil
							}
							return &treenode.BlockTree{ID: id, RootID: settingsDoc, BoxID: settingsBox, Path: "/" + settingsDoc + ".docj"}
						},
						load: func(box, path string, engine *lute.Lute) (*parse.Tree, error) {
							if err := mark("load"); err != nil {
								return nil, err
							}
							if variant == "nil-tree" {
								return nil, nil
							}
							if box != settingsBox || path != "/"+settingsDoc+".docj" {
								t.Fatal("wrong owner locator", box, path)
							}
							raw, err := lease.ReadDocument(ctx, box, settingsDoc)
							if err != nil {
								return nil, err
							}
							tree, _, err := dataparser.ParseJSON(raw, engine.ParseOptions)
							if err != nil {
								return nil, err
							}
							if variant == "stale-view" || variant == "first-fallback" {
								treenode.GetNodeInTree(tree, settingsBlock).SetIALAttr(av.NodeAttrView, "gone")
							}
							if variant == "no-attribute" {
								treenode.GetNodeInTree(tree, settingsBlock).SetIALAttr(av.NodeAttrView, "")
							}
							return tree, nil
						},
						stage: func(*parse.Tree) error { t.Fatal("unexpected doc stage"); return nil }, cacheIAL: func(string, map[string]string) { t.Fatal("unexpected cache mutation") },
						avConfig: &attributeViewEnvironment{read: func(id string) (*av.AttributeView, error) {
							if err := mark("read"); err != nil {
								return nil, err
							}
							raw, err := lease.ReadNativeAttributeView(ctx, id)
							if err != nil {
								return nil, err
							}
							a := &av.AttributeView{}
							err = json.Unmarshal(raw, a)
							if variant == "no-views" {
								a.Views = nil
							}
							if variant == "stale-default" || variant == "first-fallback" {
								a.ViewID = "gone"
							}
							return a, err
						}, stage: func(a *av.AttributeView) error {
							if err := mark("stage"); err != nil {
								return err
							}
							staged = a
							return nil
						}},
						finish: func(*Transaction) error { return mark("finish") },
					}
					tx := &Transaction{environment: env, DoOperations: []*Operation{op}}
					result := performTx(tx)
					failed := map[string]bool{"read": true, "load": true, "nil-tree": true, "stage": true, "finish": true, "cancel-read": true, "cancel-load": true, "cancel-stage": true, "no-views": true, "bad-data": true, "decode": true, "marshal": true}[variant]
					if failed {
						if result == nil || tx.state.Load() == 2 {
							t.Fatal("failure committed", result, counts)
						}
						if variant != "finish" && counts["finish"] != 0 {
							t.Fatal("failed candidate reached finish", counts)
						}
						if variant == "bad-data" && (counts["parser"] != 0 || counts["read"] != 0) {
							t.Fatal("malformed request admitted", counts)
						}
						if variant != "finish" && counts["stage"] > 0 && variant != "stage" && variant != "cancel-stage" {
							t.Fatal("invalid source staged", counts)
						}
					} else {
						if result != nil || staged == nil || counts["stage"] != 1 || counts["finish"] != 1 || tx.state.Load() != 2 {
							t.Fatal(result, counts)
						}
						if staged.Name != page {
							t.Fatal("owner leak", staged.Name)
						}
						selected := []string{"first", "second"}[i]
						switch variant {
						case "empty-block", "missing-block", "missing-node", "stale-view", "no-attribute":
							selected = "default"
						case "first-fallback":
							selected = "first"
						}
						expected := &av.AttributeView{}
						if err := json.Unmarshal(before, expected); err != nil {
							t.Fatal(err)
						}
						if variant == "stale-default" || variant == "first-fallback" {
							expected.ViewID = "gone"
						}
						for _, v := range expected.Views {
							if v.ID != selected {
								continue
							}
							switch action {
							case "setAttrViewFilters":
								raw, _ := json.Marshal(op.Data)
								if err := json.Unmarshal(raw, &v.Filters); err != nil {
									t.Fatal(err)
								}
							case "setAttrViewSorts":
								raw, _ := json.Marshal(op.Data)
								if err := json.Unmarshal(raw, &v.Sorts); err != nil {
									t.Fatal(err)
								}
							case "setAttrViewPageSize":
								v.PageSize = 9
							}
						}
						if !reflect.DeepEqual(staged, expected) {
							t.Fatalf("original algorithm/selected view mismatch: got %+v want %+v", staged, expected)
						}
					}
					if tx.m != nil {
						if !tx.m.TryLock() {
							t.Fatal("lock leaked")
						}
						tx.m.Unlock()
					}
					after, err := lease.ReadNativeAttributeView(context.Background(), settingsAV)
					if err != nil || string(after) != string(before) {
						t.Fatal("AV source changed", err)
					}
					docAfter, err := lease.ReadDocument(context.Background(), settingsBox, settingsDoc)
					if err != nil || string(docAfter) != string(docBefore) {
						t.Fatal("doc source changed", err)
					}
				})
			}
		}
	}
}

func TestAVViewSettingsAdmissionAndNumericCompatibility(t *testing.T) {
	for _, action := range []string{"setAttrViewFilters", "setAttrViewSorts", "setAttrViewPageSize"} {
		for _, missing := range []string{"config", "read", "stage"} {
			t.Run(action+"/"+missing, func(t *testing.T) {
				e := &transactionEnvironment{ctx: context.Background(), parser: func() *lute.Lute { t.Fatal("admitted missing dependency"); return nil }, lookup: func(string) *treenode.BlockTree { return nil }, load: func(string, string, *lute.Lute) (*parse.Tree, error) { return nil, nil }, stage: func(*parse.Tree) error { return nil }, cacheIAL: func(string, map[string]string) {}, finish: func(*Transaction) error { return nil }, avConfig: &attributeViewEnvironment{read: func(string) (*av.AttributeView, error) { t.Fatal("read before admission"); return nil, nil }, stage: func(*av.AttributeView) error { return nil }}}
				switch missing {
				case "config":
					e.avConfig = nil
				case "read":
					e.avConfig.read = nil
				case "stage":
					e.avConfig.stage = nil
				}
				if result := performTx(&Transaction{environment: e, DoOperations: []*Operation{settingsOperation(action)}}); result == nil || result.msg != "transaction_av_view_dependencies_unbound" {
					t.Fatal(result)
				}
			})
		}
	}
	for _, n := range []float64{0, -1, 9.75, -9.75, 256} {
		op := settingsOperation("setAttrViewPageSize")
		op.Data = n
		a := &av.AttributeView{Views: []*av.View{{ID: "v"}}}
		calls := 0
		e := &attributeViewSettingsEnvironment{read: func(string) (*av.AttributeView, error) { return a, nil }, view: func(a *av.AttributeView, _ string) (*av.View, error) { return a.GetCurrentView("") }, stage: func(*av.AttributeView) error { calls++; return nil }}
		if err := setAttributeViewSettings(context.Background(), op, e); err != nil || a.Views[0].PageSize != int(n) || calls != 1 {
			t.Fatal(n, err)
		}
	}
	for _, data := range []interface{}{nil, 1, "5", math.NaN(), math.Inf(1), math.Inf(-1), math.Ldexp(1, 63), -math.Ldexp(1, 64)} {
		op := settingsOperation("setAttrViewPageSize")
		op.Data = data
		if err := validateAttributeViewSettings(op); err == nil {
			t.Fatal("invalid page size accepted", data)
		}
	}
	for _, action := range []string{"setAttrViewFilters", "setAttrViewSorts"} {
		for _, data := range [][]interface{}{nil, {}} {
			op := settingsOperation(action)
			op.Data = data
			a := &av.AttributeView{Views: []*av.View{{ID: "v", Filters: []*av.ViewFilter{{Column: "old"}}, Sorts: []*av.ViewSort{{Column: "old"}}}}}
			e := &attributeViewSettingsEnvironment{read: func(string) (*av.AttributeView, error) { return a, nil }, view: func(a *av.AttributeView, _ string) (*av.View, error) { return a.GetCurrentView("") }, stage: func(*av.AttributeView) error { return nil }}
			if err := setAttributeViewSettings(context.Background(), op, e); err != nil {
				t.Fatal(err)
			}
			if action == "setAttrViewFilters" && (len(a.Views[0].Filters) != 0 || (a.Views[0].Filters == nil) != (data == nil)) {
				t.Fatal("filter null/empty changed")
			}
			if action == "setAttrViewSorts" && (len(a.Views[0].Sorts) != 0 || (a.Views[0].Sorts == nil) != (data == nil)) {
				t.Fatal("sort null/empty changed")
			}
		}
	}
}

func TestAVViewSettingsMixedTransactionAndCandidateVisibility(t *testing.T) {
	m, store := settingsSource(t)
	for _, page := range []string{"1760000000000000002", "1760000000000000003"} {
		lease, err := m.Acquire(context.Background(), store, page)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(lease.Release)
		before, err := lease.ReadNativeAttributeView(context.Background(), settingsAV)
		if err != nil {
			t.Fatal(err)
		}
		docBefore, err := lease.ReadDocument(context.Background(), settingsBox, settingsDoc)
		if err != nil {
			t.Fatal(err)
		}
		for _, failure := range []string{"", "second-stage", "finish", "cancel-second", "bad-second", "unaudited", "pre-cancel"} {
			t.Run(page+"/"+failure, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				if failure == "pre-cancel" {
					cancel()
				}
				counts := map[string]int{}
				var candidate *av.AttributeView
				env := &transactionEnvironment{ctx: ctx, parser: func() *lute.Lute { counts["parser"]++; return nativeparser.NewLute(nativeparser.Defaults()) },
					lookup: func(id string) *treenode.BlockTree {
						return &treenode.BlockTree{ID: id, RootID: settingsDoc, BoxID: settingsBox, Path: "/" + settingsDoc + ".docj"}
					},
					load: func(string, string, *lute.Lute) (*parse.Tree, error) {
						counts["load"]++
						tree, _, e := dataparser.ParseJSON(docBefore, nativeparser.NewLute(nativeparser.Defaults()).ParseOptions)
						return tree, e
					},
					stage: func(*parse.Tree) error { counts["doc-stage"]++; return nil }, cacheIAL: func(string, map[string]string) {},
					avConfig: &attributeViewEnvironment{read: func(string) (*av.AttributeView, error) {
						counts["read"]++
						if candidate == nil {
							candidate = &av.AttributeView{}
							if e := json.Unmarshal(before, candidate); e != nil {
								return nil, e
							}
						}
						return candidate, nil
					},
						stage: func(*av.AttributeView) error {
							counts["stage"]++
							if counts["stage"] == 2 {
								if failure == "second-stage" {
									return errors.New("second stage failed")
								}
								if failure == "cancel-second" {
									cancel()
								}
							}
							return nil
						}},
					finish: func(*Transaction) error {
						counts["finish"]++
						if failure == "finish" {
							return errors.New("finish failed")
						}
						return nil
					},
				}
				ops := []*Operation{{Action: "setAttrs", ID: settingsBlock, Data: `{"custom-sy-av-view":"second"}`}, settingsOperation("setAttrViewFilters"), settingsOperation("setAttrViewSorts"), settingsOperation("setAttrViewPageSize")}
				if failure == "bad-second" {
					ops[2].Data = "wrong"
				}
				if failure == "unaudited" {
					ops[2].Action = "updateAttrViewCell"
				}
				tx := &Transaction{environment: env, DoOperations: ops}
				result := performTx(tx)
				if failure == "" {
					if result != nil || counts["stage"] != 3 || counts["read"] != 3 || counts["load"] != 1 || counts["finish"] != 1 || counts["doc-stage"] != 1 {
						t.Fatal(result, counts)
					}
					v := candidate.Views[1]
					if v.ID != "second" || v.PageSize != 9 || v.Filters[0].Column != "number" || v.Sorts[0].Column != "number" || candidate.Views[0].PageSize != 17 || candidate.Views[2].PageSize != 17 {
						t.Fatal("candidate lost prior edits or staged block selector ignored", candidate)
					}
				} else {
					if result == nil || tx.state.Load() == 2 {
						t.Fatal("failure committed", result, counts)
					}
					if failure != "finish" && counts["finish"] != 0 {
						t.Fatal("partial candidate finished", counts)
					}
					if (failure == "pre-cancel" || failure == "bad-second" || failure == "unaudited") && len(counts) != 0 {
						t.Fatal("batch was partially admitted", counts)
					}
				}
				if tx.m != nil {
					if !tx.m.TryLock() {
						t.Fatal("lock leaked")
					}
					tx.m.Unlock()
				}
				after, e := lease.ReadNativeAttributeView(context.Background(), settingsAV)
				if e != nil || string(after) != string(before) {
					t.Fatal("source changed", e)
				}
				docAfter, e := lease.ReadDocument(context.Background(), settingsBox, settingsDoc)
				if e != nil || string(docAfter) != string(docBefore) {
					t.Fatal("source doc changed", e)
				}
			})
		}
	}
}
