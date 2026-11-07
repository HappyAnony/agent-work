package model

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/88250/lute"
	"github.com/88250/lute/parse"
	"wiki-kernel/av"
)

func orderFailureFixture(t *testing.T, h *cellHarness, action string) (*av.AttributeView, *Operation) {
	a := orderPrepare(t, h, av.LayoutTypeTable)
	op := orderOp(action)
	if action == "sortAttrViewRow" {
		var v *av.View
		var old, target string
		a, v, old, target = orderGroup(t, h, av.LayoutTypeTable, "status")
		op.ID = settingsBlock
		op.PreviousID = cellSecond
		op.GroupID = old
		op.TargetGroupID = target
		a.KeyValues = append(a.KeyValues, &av.KeyValues{Key: &av.Key{ID: "template", Name: "Label", Type: av.KeyTypeTemplate, Template: ".action{.Text}"}})
		v.Filters = []*av.ViewFilter{{Column: "relation", Operator: av.FilterOperatorIsNotEmpty, Value: &av.Value{Type: av.KeyTypeRelation, Relation: &av.ValueRelation{BlockIDs: []string{cellSecond}}}}}
	}
	h.counts = map[string]int{}
	return a, op
}

func TestAVOrderContextFailuresCancellationAndAdmission(t *testing.T) {
	m, store, root := fieldSource(t)
	for _, page := range []string{"1760000000000000002", "1760000000000000003"} {
		t.Run(page, func(t *testing.T) {
			t.Parallel()
			l, err := m.Acquire(context.Background(), store, page)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(l.Release)
			for _, action := range []string{"sortAttrViewRow", "sortAttrViewCol", "sortAttrViewKey", "duplicateAttrViewKey"} {
				points := []string{"read:" + cellAV, "stage:" + cellAV, "finish"}
				if action == "sortAttrViewCol" || action == "sortAttrViewRow" {
					points = append(points, "load")
				}
				if action == "duplicateAttrViewKey" {
					points = append(points, "clone")
				}
				if action == "sortAttrViewRow" {
					points = append(points, "near", "template", "reverse", "read:"+cellDest, "stage:"+cellDest, "groups:"+cellDest, "groups:"+cellAV)
				}
				failures := []string{"pre-cancel", "nil-source", "nil-kv", "nil-key", "nil-value", "nil-view", "nil-layout", "nil-field", "nil-base", "nil-group", "nil-option"}
				if action == "sortAttrViewRow" {
					failures = append(failures, "nil-filter", "bad-filter", "nil-group-value", "missing-group-key")
				}
				if action == "duplicateAttrViewKey" {
					failures = append(failures, "nil-clone")
				}
				if action == "sortAttrViewCol" || action == "sortAttrViewRow" {
					failures = append(failures, "nil-tree", "no-views")
				}
				for _, p := range points {
					failures = append(failures, p, "cancel-"+p)
				}
				for _, failure := range failures {
					t.Run(action+"/"+failure, func(t *testing.T) {
						h := orderHarness(t, l, root, page, "")
						before := cellDigest(t, root, page)
						a, op := orderFailureFixture(t, h, action)
						e := h.env.avOrder
						switch failure {
						case "pre-cancel":
							h.cancel()
						case "nil-source":
							e.read = func(string) (*av.AttributeView, error) { return nil, nil }
						case "nil-kv":
							a.KeyValues = append(a.KeyValues, nil)
						case "nil-key":
							a.KeyValues[0].Key = nil
						case "nil-value":
							a.KeyValues[0].Values = append(a.KeyValues[0].Values, nil)
						case "nil-view":
							a.Views = append(a.Views, nil)
						case "nil-layout":
							a.Views[0].Table = nil
						case "nil-field":
							a.Views[0].Table.Columns = append(a.Views[0].Table.Columns, nil)
						case "nil-base":
							a.Views[0].Table.Columns[0].BaseField = nil
						case "nil-group":
							a.Views[0].Groups = append(a.Views[0].Groups, nil)
						case "nil-option":
							k, _ := a.GetKey("status")
							k.Options = append(k.Options, nil)
						case "nil-filter":
							a.Views[0].Filters = append(a.Views[0].Filters, nil)
						case "bad-filter":
							a.Views[0].Filters = []*av.ViewFilter{{Column: "text", Value: &av.Value{Type: av.KeyTypeText}}}
						case "nil-group-value":
							a.Views[0].GetGroupByID(op.TargetGroupID).GroupVal = nil
						case "missing-group-key":
							a.Views[0].Group.Field = "missing"
						case "nil-clone":
							e.cloneKey = func(*av.Key) (*av.Key, error) { return nil, nil }
						case "nil-tree":
							h.env.load = func(string, string, *lute.Lute) (*parse.Tree, error) { return nil, nil }
						case "no-views":
							a.Views = nil
						default:
							h.failure = failure
						}
						h.tx.DoOperations = []*Operation{op}
						ret := performTx(h.tx)
						if ret == nil || h.committed || h.tx.state.Load() == 2 || strings.Contains(ret.msg, "PANIC RECOVERED") {
							t.Fatal("failure swallowed/panic", ret, h.counts)
						}
						if failure != "finish" && failure != "cancel-finish" && h.counts["finish"] != 0 {
							t.Fatal("failed candidate finished")
						}
						assertFieldSourceUnchanged(t, h, before)
					})
				}
				deps := []string{"environment", "read", "stage"}
				if action == "sortAttrViewCol" || action == "sortAttrViewRow" {
					deps = append(deps, "view")
				}
				if action == "duplicateAttrViewKey" {
					deps = append(deps, "clone")
				}
				if action == "sortAttrViewRow" {
					deps = append(deps, "groups", "defaults", "context", "near", "template", "reverse", "clock", "id")
				}
				for _, dep := range deps {
					t.Run(action+"/missing-"+dep, func(t *testing.T) {
						h := orderHarness(t, l, root, page, "")
						before := cellDigest(t, root, page)
						e := h.env.avOrder
						switch dep {
						case "environment":
							h.env.avOrder = nil
						case "read":
							e.read = nil
						case "stage":
							e.stage = nil
						case "view":
							e.view = nil
						case "clone":
							e.cloneKey = nil
						case "groups":
							e.groups = nil
						case "defaults":
							e.defaults = nil
						case "context":
							e.defaults.ctx = nil
						case "near":
							e.defaults.near = nil
						case "template":
							e.defaults.templateKeys = nil
						case "reverse":
							e.defaults.reverse = nil
						case "clock":
							e.defaults.now = nil
						case "id":
							e.defaults.newID = nil
						}
						h.tx.DoOperations = []*Operation{orderOp(action)}
						ret := performTx(h.tx)
						if ret == nil || h.committed || len(h.counts) != 0 || h.tx.m != nil {
							t.Fatal("missing dependency admitted", ret, h.counts)
						}
						assertFieldSourceUnchanged(t, h, before)
					})
				}
			}
		})
	}
}

func TestAVOrderContextCallbackCancellationAndMixedEdits(t *testing.T) {
	m, store, root := fieldSource(t)
	for _, page := range []string{"1760000000000000002", "1760000000000000003"} {
		t.Run(page, func(t *testing.T) {
			t.Parallel()
			l, err := m.Acquire(context.Background(), store, page)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(l.Release)
			for _, point := range []string{"read", "view", "stage", "clone", "groups", "near", "template", "reverse", "clock", "id"} {
				t.Run("after-"+point, func(t *testing.T) {
					h := orderHarness(t, l, root, page, "")
					before := cellDigest(t, root, page)
					_, op := orderFailureFixture(t, h, "sortAttrViewRow")
					e := h.env.avOrder
					switch point {
					case "read":
						fn := e.read
						e.read = func(id string) (*av.AttributeView, error) { a, err := fn(id); h.cancel(); return a, err }
					case "view":
						fn := e.view
						e.view = func(a *av.AttributeView, id string) (*av.View, error) { v, err := fn(a, id); h.cancel(); return v, err }
					case "stage":
						fn := e.stage
						e.stage = func(a *av.AttributeView) error { err := fn(a); h.cancel(); return err }
					case "clone":
						op = orderOp("duplicateAttrViewKey")
						fn := e.cloneKey
						e.cloneKey = func(k *av.Key) (*av.Key, error) { k, err := fn(k); h.cancel(); return k, err }
					case "groups":
						fn := e.groups
						e.groups = func(a *av.AttributeView) error { err := fn(a); h.cancel(); return err }
					case "near":
						fn := e.defaults.near
						e.defaults.near = func(a *av.AttributeView, v, g *av.View, p string) (av.Item, error) {
							item, err := fn(a, v, g, p)
							h.cancel()
							return item, err
						}
					case "template":
						fn := e.defaults.templateKeys
						e.defaults.templateKeys = func(a *av.AttributeView, k *av.Key) ([]*av.Key, error) {
							keys, err := fn(a, k)
							h.cancel()
							return keys, err
						}
					case "reverse":
						fn := e.defaults.reverse
						e.defaults.reverse = func(a *av.AttributeView, k *av.Key, v *av.Value) error { err := fn(a, k, v); h.cancel(); return err }
					case "clock":
						fn := e.defaults.now
						e.defaults.now = func() time.Time { h.cancel(); return fn() }
					case "id":
						fn := e.defaults.newID
						e.defaults.newID = func() string { h.cancel(); return fn() }
					}
					h.tx.DoOperations = []*Operation{op}
					ret := performTx(h.tx)
					if ret == nil || h.committed || h.counts["finish"] != 0 {
						t.Fatal("post callback cancellation swallowed", ret, h.counts)
					}
					assertFieldSourceUnchanged(t, h, before)
				})
			}
			for _, scenario := range []string{"success", "last-stage", "bad-second", "unaudited"} {
				t.Run("mixed/"+scenario, func(t *testing.T) {
					h := orderHarness(t, l, root, page, "")
					before := cellDigest(t, root, page)
					a := orderPrepare(t, h, av.LayoutTypeTable)
					k, _ := a.GetKey("date")
					k.Date = &av.Date{AutoFillNow: true}
					ops := []*Operation{{Action: "duplicateAttrViewKey", AvID: cellAV, KeyID: "date", NextID: "date-copy"}, {Action: "setAttrViewColDateFillCreated", AvID: cellAV, ID: "date-copy", Data: false}, {Action: "sortAttrViewCol", AvID: cellAV, ID: "date-copy", PreviousID: "primary"}, {Action: "sortAttrViewKey", AvID: cellAV, ID: "date-copy", PreviousID: "primary"}, {Action: "sortAttrViewRow", AvID: cellAV, ID: cellSecond}}
					if scenario == "bad-second" {
						ops[1].Data = 42
					}
					if scenario == "unaudited" {
						ops = append(ops, &Operation{Action: "insertAttrViewBlock", AvID: cellAV})
					}
					if scenario == "last-stage" {
						fn := h.env.avOrder.stage
						h.env.avOrder.stage = func(a *av.AttributeView) error {
							if a.Views[0].ItemIDs[0] == cellSecond {
								return errors.New("synthetic last stage")
							}
							return fn(a)
						}
					}
					h.tx.DoOperations = ops
					ret := performTx(h.tx)
					if scenario == "success" {
						if ret != nil || !h.committed {
							t.Fatal(ret)
						}
						copy, _ := a.GetKey("date-copy")
						if !k.Date.AutoFillNow || copy.Date.AutoFillNow || slices.Index(a.KeyIDs, "date-copy") != slices.Index(a.KeyIDs, "primary")+1 || fieldIDs(a.Views[0])[1] != "date-copy" || a.Views[0].ItemIDs[0] != cellSecond {
							t.Fatal("mixed candidate lost state")
						}
					} else if ret == nil || h.committed {
						t.Fatal("failed candidate finished", ret)
					}
					if (scenario == "bad-second" || scenario == "unaudited") && len(h.counts) != 0 {
						t.Fatal("partial admission")
					}
					assertFieldSourceUnchanged(t, h, before)
				})
			}
		})
	}
}
