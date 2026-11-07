package model

import (
	"context"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"math"
	"strings"
	"testing"
	"wiki-kernel/av"
)

func managementFailureSetup(t *testing.T, h *managementHarness, action string, layout av.LayoutType) (*av.AttributeView, *Operation) {
	t.Helper()
	a := managementPrepare(t, h, layout)
	op := managementOp(action, layout)
	switch action {
	case "setAttrViewGroup", "removeAttrViewGroup", "sortAttrViewGroup", "hideAttrViewGroup", "hideAttrViewAllGroups", "foldAttrViewGroup":
		var old, target string
		a, _, old, target = orderGroup(t, h.cellHarness, layout, "status")
		op.ID = old
		op.PreviousID = target
	}
	h.counts = map[string]int{}
	return a, op
}
func managementFailureLayout(action string) av.LayoutType {
	if action == "syncAttrViewTableColWidth" {
		return av.LayoutTypeTable
	}
	if action == "setAttrViewFillColBackgroundColor" {
		return av.LayoutTypeKanban
	}
	return av.LayoutTypeGallery
}

func TestAVManagementFailuresAndCancellation(t *testing.T) {
	m, store, root := managementSource(t)
	for _, page := range []string{"1760000000000000002", "1760000000000000003"} {
		t.Run(page, func(t *testing.T) {
			t.Parallel()
			l, err := m.Acquire(context.Background(), store, page)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(l.Release)
			for _, action := range attributeViewManagementActions {
				points := []string{"read:" + cellAV, "stage:" + cellAV, "finish"}
				switch action {
				case "setAttrViewViewName", "setAttrViewViewIcon", "setAttrViewViewDesc", "sortAttrViewView", "syncAttrViewTableColWidth":
				default:
					points = append(points, "load")
				}
				switch action {
				case "setAttrViewGroup":
					points = append(points, "groups:"+cellAV, "count", "management-reload")
				case "sortAttrViewGroup":
					points = append(points, "sort-groups")
				case "syncAttrViewTableColWidth":
					points = append(points, "management-reload")
				case "addAttrViewView", "duplicateAttrViewView", "setAttrViewBlockView", "changeAttrViewLayout":
					points = append(points, "node", "attrs", "tree-stage", "notify")
					if action != "setAttrViewBlockView" {
						points = append(points, "make-view")
					}
					if action == "duplicateAttrViewView" {
						points = append(points, "clone-view")
					}
					if action == "changeAttrViewLayout" {
						points = append(points, "mirrors", "groups:"+cellAV, "management-reload")
					}
				case "removeAttrViewView":
					points = append(points, "mirrors", "node", "attrs", "tree-stage", "notify", "flush-trees")
				}
				for _, point := range points {
					for _, prefix := range []string{"", "cancel-"} {
						t.Run(action+prefix+point, func(t *testing.T) {
							h := newManagementHarness(t, l, root, page, "", managementFailureLayout(action))
							_, op := managementFailureSetup(t, h, action, managementFailureLayout(action))
							before := cellDigest(t, root, page)
							reg := managementRegistry(t, h)
							h.failure = prefix + point
							h.tx.DoOperations = []*Operation{op}
							ret := performTx(h.tx)
							if ret == nil || h.committed || h.tx.state.Load() != 3 || h.counts[point] == 0 {
								t.Fatal("failure escaped", ret, point, h.counts)
							}
							if point != "finish" && h.counts["finish"] != 0 {
								t.Fatal("failed operation finished")
							}
							assertManagementSource(t, h, before, reg)
						})
					}
				}
			}
		})
	}
}

func TestAVManagementMissingDependenciesAndBadInput(t *testing.T) {
	m, store, root := managementSource(t)
	for _, page := range []string{"1760000000000000002", "1760000000000000003"} {
		t.Run(page, func(t *testing.T) {
			t.Parallel()
			l, err := m.Acquire(context.Background(), store, page)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(l.Release)
			for _, action := range attributeViewManagementActions {
				for _, missing := range []string{"environment", "ctx", "read", "stage", "view", "groups", "count", "sort", "make", "language", "clone", "mirrors", "node", "attrs", "mirror-attrs", "flush", "reload"} {
					t.Run(action+"/"+missing, func(t *testing.T) {
						layout := managementFailureLayout(action)
						h := newManagementHarness(t, l, root, page, "", layout)
						_, op := managementFailureSetup(t, h, action, layout)
						before := cellDigest(t, root, page)
						e := h.env.avManagement
						switch missing {
						case "environment":
							h.env.avManagement = nil
						case "ctx":
							e.ctx = nil
						case "read":
							e.read = nil
						case "stage":
							e.stage = nil
						case "view":
							e.view = nil
						case "groups":
							e.groups = nil
						case "count":
							e.count = nil
						case "sort":
							e.sortGroups = nil
						case "make":
							e.makeView = nil
						case "language":
							e.language = nil
						case "clone":
							e.cloneView = nil
						case "mirrors":
							e.mirrors = nil
						case "node":
							e.node = nil
						case "attrs":
							e.attrs = nil
						case "mirror-attrs":
							e.mirrorAttrs = nil
						case "flush":
							e.flushTrees = nil
						case "reload":
							e.reload = nil
						}
						// Only dependencies used by this operation must prevent admission.
						var validation error
						if h.env.avManagement == nil {
							validation = (*attributeViewManagementEnvironment)(nil).validate(action)
						} else {
							validation = e.validate(action)
						}
						if validation == nil {
							return
						}
						h.tx.DoOperations = []*Operation{op}
						if ret := performTx(h.tx); ret == nil || h.committed || len(h.counts) != 0 {
							t.Fatal("partial dependency admission", ret, h.counts)
						}
						assertFieldSourceUnchanged(t, h.cellHarness, before)
					})
				}
			}
			for _, scenario := range []string{"bad-bool", "bad-string", "nan", "infinity", "overflow", "bad-layout", "nil-group", "bad-group", "duplicate-id", "nil-source", "nil-view", "nil-layout", "nil-base", "nil-filter", "nil-sort", "nil-group-source", "nil-primary", "nil-node", "known-load", "nil-factory", "nil-clone", "unknown-last", "bad-last", "pre-cancel"} {
				t.Run(scenario, func(t *testing.T) {
					h := newManagementHarness(t, l, root, page, "", av.LayoutTypeGallery)
					a := managementPrepare(t, h, av.LayoutTypeGallery)
					before := cellDigest(t, root, page)
					op := managementOp("setAttrViewFitImage", av.LayoutTypeGallery)
					ops := []*Operation{op}
					v := a.GetView("gallery")
					switch scenario {
					case "bad-bool":
						op.Data = "true"
					case "bad-string":
						op.Action = "setAttrViewViewName"
						op.Data = 1
					case "nan", "infinity", "overflow":
						op.Action = "setAttrViewCardSize"
						op.Data = math.NaN()
						if scenario == "infinity" {
							op.Data = math.Inf(1)
						}
						if scenario == "overflow" {
							op.Data = math.Exp2(63)
						}
					case "bad-layout":
						op.Action = "changeAttrViewLayout"
						op.Layout = "unknown"
					case "nil-group":
						op.Action = "setAttrViewGroup"
						op.Data = (*av.ViewGroup)(nil)
					case "bad-group":
						op.Action = "setAttrViewGroup"
						op.Data = map[string]interface{}{"field": true}
					case "duplicate-id":
						op.Action = "addAttrViewView"
						op.Layout = av.LayoutTypeTable
					case "nil-source":
						h.env.avManagement.read = func(string) (*av.AttributeView, error) { return nil, nil }
					case "nil-view":
						a.Views = append(a.Views, nil)
					case "nil-layout":
						v.Gallery = nil
					case "nil-base":
						v.Gallery.BaseLayout = nil
					case "nil-filter":
						v.Filters = append(v.Filters, nil)
					case "nil-sort":
						v.Sorts = append(v.Sorts, nil)
					case "nil-group-source":
						v.Groups = append(v.Groups, nil)
					case "nil-primary":
						a.GetBlockKeyValues().Values[0].Block = nil
					case "nil-node":
						op = managementOp("addAttrViewView", av.LayoutTypeGallery)
						ops[0] = op
						h.env.avManagement.node = func(string) (*ast.Node, *parse.Tree, error) { return nil, nil, nil }
					case "known-load":
						h.failure = "load"
					case "nil-factory":
						op = managementOp("addAttrViewView", av.LayoutTypeGallery)
						ops[0] = op
						h.env.avManagement.makeView = func(av.LayoutType) *av.View { return nil }
					case "nil-clone":
						op = managementOp("duplicateAttrViewView", av.LayoutTypeGallery)
						ops[0] = op
						h.env.avManagement.cloneView = func(*av.View) (*av.View, error) { return nil, nil }
					case "unknown-last":
						ops = append(ops, &Operation{Action: "unaudited"})
					case "bad-last":
						ops = append(ops, &Operation{Action: "setAttrViewCardSize", AvID: cellAV, Data: "bad"})
					case "pre-cancel":
						h.cancel()
					}
					h.tx.DoOperations = ops
					ret := performTx(h.tx)
					if ret == nil || h.committed || (ret != nil && strings.Contains(ret.msg, "PANIC")) {
						t.Fatal("malformed candidate finished", ret, h.counts)
					}
					if (scenario == "unknown-last" || scenario == "bad-last" || scenario == "pre-cancel") && len(h.counts) != 0 {
						t.Fatal("partial batch admitted")
					}
					assertFieldSourceUnchanged(t, h.cellHarness, before)
				})
			}
		})
	}
}
