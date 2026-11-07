package model

import (
	"context"
	"strings"
	"testing"

	"github.com/88250/lute"
	"github.com/88250/lute/parse"
	"wiki-kernel/av"
)

func TestAVFieldSettingsFailuresCancellationAndAdmission(t *testing.T) {
	m, store, root := fieldSource(t)
	for _, page := range []string{"1760000000000000002", "1760000000000000003"} {
		t.Run(page, func(t *testing.T) {
			t.Parallel()
			l, err := m.Acquire(context.Background(), store, page)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(l.Release)
			for _, action := range fieldSettingActions {
				failures := []string{"read:" + cellAV, "stage:" + cellAV, "finish", "cancel-read:" + cellAV, "cancel-stage:" + cellAV, "cancel-finish", "pre-cancel", "nil-source", "nil-kv", "nil-key", "nil-value", "nil-view", "nil-layout", "nil-field", "nil-base"}
				if attributeViewFieldSettingNeedsView(action) {
					failures = append(failures, "load", "cancel-load", "nil-tree", "empty-tree", "no-views")
				}
				if action != "updateAttrViewColNumberFormat" {
					failures = append(failures, "bad-data")
				}
				if action == "setAttrViewColCalc" {
					failures = append(failures, "marshal", "decode")
				}
				for _, failure := range failures {
					t.Run(action+"/"+failure, func(t *testing.T) {
						h := fieldSettingHarness(t, l, root, page, "")
						before := cellDigest(t, root, page)
						a := fieldSettingPrepare(t, h, av.LayoutTypeTable)
						op := fieldSettingOp(action, true)
						switch failure {
						case "nil-source":
							h.env.avConfig.read = func(string) (*av.AttributeView, error) { return nil, nil }
						case "nil-kv":
							a.KeyValues = append(a.KeyValues, nil)
						case "nil-key":
							a.KeyValues[0].Key = nil
						case "nil-value":
							kv, _ := a.GetKeyValues("date")
							kv.Values = append(kv.Values, nil)
						case "nil-view":
							a.Views = append(a.Views, nil)
						case "nil-layout":
							a.Views[0].Table = nil
						case "nil-field":
							a.Views[0].Table.Columns = append(a.Views[0].Table.Columns, nil)
						case "nil-base":
							a.Views[0].Table.Columns[0].BaseField = nil
						case "nil-tree":
							h.env.load = func(string, string, *lute.Lute) (*parse.Tree, error) { return nil, nil }
						case "empty-tree":
							h.env.load = func(string, string, *lute.Lute) (*parse.Tree, error) { return &parse.Tree{}, nil }
						case "no-views":
							a.Views = nil
						case "pre-cancel":
							h.cancel()
						case "bad-data":
							op.Data = nil
						case "marshal":
							op.Data = func() {}
						case "decode":
							op.Data = map[string]interface{}{"operator": 42}
						default:
							h.failure = failure
						}
						h.tx.DoOperations = []*Operation{op}
						ret := performTx(h.tx)
						if ret == nil || h.committed || h.tx.state.Load() == 2 || strings.Contains(ret.msg, "PANIC RECOVERED") {
							t.Fatal("failure swallowed/panic", ret, h.counts)
						}
						if failure != "finish" && failure != "cancel-finish" && h.counts["finish"] != 0 {
							t.Fatal("failed candidate finished", h.counts)
						}
						if failure == "bad-data" && len(h.counts) != 0 {
							t.Fatal("bad primitive entered transaction", h.counts)
						}
						assertFieldSourceUnchanged(t, h, before)
					})
				}
				for _, dep := range []string{"configuration", "read", "stage", "lookup", "load"} {
					t.Run(action+"/missing-"+dep, func(t *testing.T) {
						h := fieldSettingHarness(t, l, root, page, "")
						before := cellDigest(t, root, page)
						switch dep {
						case "configuration":
							h.env.avConfig = nil
						case "read":
							h.env.avConfig.read = nil
						case "stage":
							h.env.avConfig.stage = nil
						case "lookup":
							h.env.lookup = nil
						case "load":
							h.env.load = nil
						}
						h.tx.DoOperations = []*Operation{fieldSettingOp(action, true)}
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

func TestAVFieldSettingsCancellationAfterCallbacksAndDirectDependencies(t *testing.T) {
	m, store, root := fieldSource(t)
	page := "1760000000000000002"
	l, err := m.Acquire(context.Background(), store, page)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Release()
	for _, point := range []string{"read", "view", "stage"} {
		t.Run(point, func(t *testing.T) {
			h := fieldSettingHarness(t, l, root, page, "")
			before := cellDigest(t, root, page)
			a := fieldSettingPrepare(t, h, av.LayoutTypeTable)
			e := &attributeViewSettingsEnvironment{read: func(string) (*av.AttributeView, error) {
				if point == "read" {
					h.cancel()
				}
				return a, nil
			}, view: func(a *av.AttributeView, _ string) (*av.View, error) {
				if point == "view" {
					h.cancel()
				}
				return a.Views[0], nil
			}, stage: func(*av.AttributeView) error {
				if point == "stage" {
					h.cancel()
				}
				return nil
			}}
			if err := runAttributeViewFieldSetting(h.ctx, fieldSettingOp("setAttrViewColWidth", true), e); err != context.Canceled {
				t.Fatal("post callback cancellation ignored", point, err)
			}
			assertFieldSourceUnchanged(t, h, before)
		})
	}
	for _, missing := range []string{"environment", "read", "stage", "view", "nil-view"} {
		t.Run(missing, func(t *testing.T) {
			h := fieldSettingHarness(t, l, root, page, "")
			a := fieldSettingPrepare(t, h, av.LayoutTypeTable)
			e := &attributeViewSettingsEnvironment{read: h.cell.read, stage: h.cell.stage, view: func(*av.AttributeView, string) (*av.View, error) { return a.Views[0], nil }}
			switch missing {
			case "environment":
				e = nil
			case "read":
				e.read = nil
			case "stage":
				e.stage = nil
			case "view":
				e.view = nil
			case "nil-view":
				e.view = func(*av.AttributeView, string) (*av.View, error) { return nil, nil }
			}
			if err := runAttributeViewFieldSetting(h.ctx, fieldSettingOp("setAttrViewColWidth", true), e); err == nil {
				t.Fatal("missing direct service accepted")
			}
		})
	}
	for _, action := range []string{"setAttrViewColIcon", "setAttrViewColDesc", "updateAttrViewColNumberFormat", "setAttrViewColDateFillCreated", "setAttrViewColDateFillSpecificTime", "setAttrViewCreatedIncludeTime", "setAttrViewUpdatedIncludeTime"} {
		h := fieldSettingHarness(t, l, root, page, "")
		fieldSettingPrepare(t, h, av.LayoutTypeTable)
		if err := runAttributeViewFieldSetting(h.ctx, fieldSettingOp(action, true), &attributeViewSettingsEnvironment{read: h.cell.read, stage: h.cell.stage}); err != nil {
			t.Fatal("metadata incorrectly requires view callback", action, err)
		}
	}
}

type fieldSettingCalcMarshaler struct {
	calls  *int
	cancel context.CancelFunc
}

func (m fieldSettingCalcMarshaler) MarshalJSON() ([]byte, error) {
	*m.calls++
	if m.cancel != nil {
		m.cancel()
	}
	return []byte(`{"operator":"Sum"}`), nil
}

func TestAVFieldSettingsCalcJSONCompatibilityAndCancellation(t *testing.T) {
	m, store, root := fieldSource(t)
	page := "1760000000000000002"
	l, err := m.Acquire(context.Background(), store, page)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Release()
	for _, layout := range []av.LayoutType{av.LayoutTypeTable, av.LayoutTypeGallery, av.LayoutTypeKanban} {
		for _, scenario := range []string{"map", "typed", "typed-null", "custom", "cancel-custom", "decode", "marshal"} {
			t.Run(string(layout)+"/"+scenario, func(t *testing.T) {
				h := fieldSettingHarness(t, l, root, page, "")
				before := cellDigest(t, root, page)
				a := fieldSettingPrepare(t, h, layout)
				op := fieldSettingOp("setAttrViewColCalc", true)
				calls := 0
				switch scenario {
				case "typed":
					op.Data = &av.FieldCalc{Operator: av.CalcOperatorSum}
				case "typed-null":
					op.Data = (*av.FieldCalc)(nil)
				case "custom":
					op.Data = fieldSettingCalcMarshaler{calls: &calls}
				case "cancel-custom":
					op.Data = fieldSettingCalcMarshaler{calls: &calls, cancel: h.cancel}
				case "decode":
					op.Data = map[string]interface{}{"operator": 42}
				case "marshal":
					op.Data = func() {}
				}
				h.tx.DoOperations = []*Operation{op}
				ret := performTx(h.tx)
				failed := scenario == "marshal" || scenario == "cancel-custom" || scenario == "decode" && layout == av.LayoutTypeTable
				if failed {
					if ret == nil || h.committed {
						t.Fatal("calc failure swallowed", ret)
					}
				} else if ret != nil || !h.committed {
					t.Fatal("original calc shape rejected", ret)
				}
				if strings.Contains(scenario, "custom") && calls != 1 {
					t.Fatal("JSON evaluated more than once", calls)
				}
				if layout == av.LayoutTypeTable && !failed {
					for _, c := range a.Views[0].Table.Columns {
						if c.ID == "number" {
							if c.Calc == nil {
								t.Fatal("missing calc")
							}
							if scenario != "typed-null" && c.Calc.Operator != av.CalcOperatorSum {
								t.Fatal("typed calc lost")
							}
						}
					}
				}
				assertFieldSourceUnchanged(t, h, before)
			})
		}
	}
	for _, action := range fieldSettingActions {
		op := fieldSettingOp(action, true)
		if action == "updateAttrViewColNumberFormat" {
			continue
		}
		for _, bad := range []interface{}{nil, 42, []interface{}{}} {
			op.Data = bad
			if err := validateAttributeViewFieldSettingInput(op); action != "setAttrViewColCalc" && err == nil {
				t.Fatal("primitive input accepted", action, bad)
			}
		}
	}
	if err := validateAttributeViewFieldSettingInput(nil); err == nil {
		t.Fatal("nil operation accepted")
	}
	if err := runLegacyAttributeViewFieldSetting("setAttrViewColIcon", nil); err == nil {
		t.Fatal("nil wrapper accepted")
	}
	// Original wrappers need no Action field on their inputs. The malformed icon
	// fails before any global read while proving the wrapper fixes its own action.
	if err := setAttributeViewColIcon(&Operation{Data: 42}); err == nil || !strings.Contains(err.Error(), "invalid_av_field_setting_string") {
		t.Fatal("wrapper action not preserved", err)
	}
}
