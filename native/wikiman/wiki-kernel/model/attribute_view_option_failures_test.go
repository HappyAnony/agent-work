package model

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/88250/lute"
	"wiki-kernel/av"
)

func validOptionOp(action, key string) *Operation {
	var data interface{}
	switch action {
	case "updateAttrViewColOptions":
		data = []*av.SelectOption{{Name: "New", Color: "8"}}
	case "removeAttrViewColOption":
		data = "Old"
	case "updateAttrViewColOption":
		data = renameOption("Old", "New", "8", "new")
	case "setAttrViewColOptionDesc":
		data = map[string]interface{}{"name": "Old", "desc": "new"}
	}
	return optionOp(action, key, data)
}

func TestAVOptionContextFailuresCancellationAndAdmission(t *testing.T) {
	m, store, root := optionSource(t)
	for _, page := range []string{"1760000000000000002", "1760000000000000003"} {
		t.Run(page, func(t *testing.T) {
			t.Parallel()
			l, e := m.Acquire(context.Background(), store, page)
			if e != nil {
				t.Fatal(e)
			}
			t.Cleanup(l.Release)
			for _, action := range []string{"updateAttrViewColOptions", "removeAttrViewColOption", "updateAttrViewColOption", "setAttrViewColOptionDesc"} {
				failures := []string{"read:" + cellAV, "stage:" + cellAV, "finish", "cancel-read:" + cellAV, "cancel-stage:" + cellAV, "cancel-finish", "pre-cancel", "nil-source", "nil-kv", "nil-key", "nil-option", "missing-key", "bad-data"}
				if action != "setAttrViewColOptionDesc" {
					failures = append(failures, "groups:"+cellAV, "cancel-groups:"+cellAV, "nil-view", "nil-filter", "nil-filter-option")
				}
				if action == "updateAttrViewColOption" || action == "removeAttrViewColOption" {
					failures = append(failures, "nil-value-option")
				}
				if action == "updateAttrViewColOptions" {
					failures = append(failures, "marshal", "nil-input-option")
				}
				for _, failure := range failures {
					t.Run(action+"/"+failure, func(t *testing.T) {
						before := cellDigest(t, root, page)
						injected := ""
						if strings.HasPrefix(failure, "read:") || strings.HasPrefix(failure, "stage:") || strings.HasPrefix(failure, "groups:") || strings.HasPrefix(failure, "cancel-") || failure == "finish" {
							injected = failure
						}
						h := optionHarness(t, l, root, page, injected)
						op := validOptionOp(action, "status")
						mutate := func(fn func(*av.AttributeView)) {
							a, e := h.cell.read(cellAV)
							if e != nil {
								t.Fatal(e)
							}
							fn(a)
						}
						switch failure {
						case "nil-source":
							h.env.avOptions.read = func(string) (*av.AttributeView, error) { return nil, nil }
						case "nil-kv":
							mutate(func(a *av.AttributeView) { a.KeyValues = append(a.KeyValues, nil) })
						case "nil-key":
							mutate(func(a *av.AttributeView) { a.KeyValues[0].Key = nil })
						case "nil-option":
							mutate(func(a *av.AttributeView) { k, _ := a.GetKey("status"); k.Options = append(k.Options, nil) })
						case "nil-view":
							mutate(func(a *av.AttributeView) { a.Views = append(a.Views, nil) })
						case "nil-filter":
							mutate(func(a *av.AttributeView) { a.Views[0].Filters = append(a.Views[0].Filters, nil) })
						case "nil-filter-option":
							mutate(func(a *av.AttributeView) {
								a.Views[0].Filters[0].Value.MSelect = append(a.Views[0].Filters[0].Value.MSelect, nil)
							})
						case "nil-value-option":
							mutate(func(a *av.AttributeView) {
								a.GetValue("status", settingsBlock).MSelect = append(a.GetValue("status", settingsBlock).MSelect, nil)
							})
						case "missing-key":
							op.ID = "missing"
						case "bad-data":
							op.Data = 42
						case "marshal":
							op.Data = func() {}
						case "nil-input-option":
							op.Data = []interface{}{nil}
						case "pre-cancel":
							h.cancel()
						}
						h.tx.DoOperations = []*Operation{op}
						ret := performTx(h.tx)
						if failure == "missing-key" && action == "updateAttrViewColOptions" {
							if ret != nil || !h.committed || len(h.staged) != 0 {
								t.Fatal("original missing-key no-op changed", ret, h.counts)
							}
						} else {
							if ret == nil || h.committed || h.tx.state.Load() == 2 || strings.Contains(ret.msg, "PANIC RECOVERED") {
								t.Fatal("failure swallowed/panic", ret, h.counts)
							}
							if injected != "" && !strings.HasPrefix(injected, "cancel-") && !strings.Contains(ret.msg, "synthetic "+failure) {
								t.Fatal("wrong failure", ret)
							}
							if failure != "finish" && failure != "cancel-finish" && h.counts["finish"] != 0 {
								t.Fatal("failed candidate finished")
							}
						}
						if h.tx.m != nil {
							if !h.tx.m.TryLock() {
								t.Fatal("lock leaked")
							}
							h.tx.m.Unlock()
						}
						if !reflect.DeepEqual(before, cellDigest(t, root, page)) {
							t.Fatal("failure changed source")
						}
					})
				}
			}
			for _, action := range []string{"updateAttrViewColOptions", "removeAttrViewColOption", "updateAttrViewColOption", "setAttrViewColOptionDesc"} {
				for _, missing := range []string{"environment", "read", "stage", "groups"} {
					if action == "setAttrViewColOptionDesc" && missing == "groups" {
						continue
					}
					t.Run(action+"/missing-"+missing, func(t *testing.T) {
						h := optionHarness(t, l, root, page, "")
						h.env.parser = func() *lute.Lute { t.Fatal("missing dependency admitted"); return nil }
						switch missing {
						case "environment":
							h.env.avOptions = nil
						case "read":
							h.env.avOptions.read = nil
						case "stage":
							h.env.avOptions.stage = nil
						case "groups":
							h.env.avOptions.groups = nil
						}
						h.tx.DoOperations = []*Operation{validOptionOp(action, "status")}
						ret := performTx(h.tx)
						if ret == nil || ret.msg != "transaction_av_option_dependencies_unbound" || len(h.counts) != 0 {
							t.Fatal(ret, h.counts)
						}
					})
				}
			}
		})
	}
}

type optionJSON struct {
	calls  *int
	cancel context.CancelFunc
}

func (v optionJSON) MarshalJSON() ([]byte, error) {
	*v.calls++
	if v.cancel != nil {
		v.cancel()
	}
	return []byte(`[{"name":"Old","color":"8","desc":"one call"}]`), nil
}

func TestAVOptionSingleDecodeMalformedKeysAndCancelledCodec(t *testing.T) {
	m, store, root := optionSource(t)
	page := "1760000000000000002"
	l, e := m.Acquire(context.Background(), store, page)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(l.Release)
	for _, cancelled := range []bool{false, true} {
		h := optionHarness(t, l, root, page, "")
		calls := 0
		data := optionJSON{calls: &calls}
		if cancelled {
			data.cancel = h.cancel
		}
		h.tx.DoOperations = []*Operation{optionOp("updateAttrViewColOptions", "status", data)}
		ret := performTx(h.tx)
		if calls != 1 {
			t.Fatal("input marshaler replayed", calls)
		}
		if cancelled {
			if ret == nil || h.committed || len(h.staged) != 0 || h.counts["groups:"+cellAV] != 0 {
				t.Fatal(ret, h.counts)
			}
		} else if ret != nil || !h.committed {
			t.Fatal(ret)
		}
	}
	for _, action := range []string{"updateAttrViewColOption", "setAttrViewColOptionDesc"} {
		keys := []string{"name", "desc"}
		if action == "updateAttrViewColOption" {
			keys = []string{"oldName", "newName", "newDesc", "newColor"}
		}
		for _, key := range keys {
			for _, mode := range []string{"missing", "nil", "wrong-type"} {
				h := optionHarness(t, l, root, page, "")
				op := validOptionOp(action, "status")
				data := op.Data.(map[string]interface{})
				switch mode {
				case "missing":
					delete(data, key)
				case "nil":
					data[key] = nil
				case "wrong-type":
					data[key] = 42
				}
				h.env.parser = func() *lute.Lute { t.Fatal("bad map admitted"); return nil }
				h.tx.DoOperations = []*Operation{op}
				if ret := performTx(h.tx); ret == nil || len(h.counts) != 0 {
					t.Fatal(action, key, mode, ret)
				}
			}
		}
	}
	if err := runAttributeViewOption(context.Background(), nil, nil); err == nil {
		t.Fatal("nil operation accepted")
	}
}

func TestAVOptionMixedCellTransactionAndAdjacentFilterDeletion(t *testing.T) {
	m, store, root := optionSource(t)
	for _, page := range []string{"1760000000000000002", "1760000000000000003"} {
		l, e := m.Acquire(context.Background(), store, page)
		if e != nil {
			t.Fatal(e)
		}
		t.Cleanup(l.Release)
		for _, failure := range []string{"", "last-stage", "bad-second", "unaudited"} {
			t.Run(page+"/"+failure, func(t *testing.T) {
				before := cellDigest(t, root, page)
				h := optionHarness(t, l, root, page, "")
				h.env.avCell = h.cell
				a, e := h.cell.read(cellAV)
				if e != nil {
					t.Fatal(e)
				}
				h.tx.DoOperations = []*Operation{optionOp("updateAttrViewColOptions", "status", []*av.SelectOption{{Name: "Old", Color: "3"}, {Name: "Other", Color: "4"}}), optionOp("updateAttrViewColOption", "status", renameOption("Old", "New", "8", "new")), optionOp("setAttrViewColOptionDesc", "status", map[string]interface{}{"name": "New", "desc": " final "}), cellOp("status", map[string]interface{}{"mSelect": []interface{}{map[string]interface{}{"content": "New", "color": "99"}}}), optionOp("removeAttrViewColOption", "status", "Other")}
				stage := h.env.avOptions.stage
				h.env.avOptions.stage = func(a *av.AttributeView) error {
					if failure == "last-stage" && h.counts["stage:"+cellAV] == 4 {
						return context.Canceled
					}
					return stage(a)
				}
				if failure == "bad-second" {
					h.tx.DoOperations[1].Data = map[string]interface{}{"oldName": "Old"}
				}
				if failure == "unaudited" {
					h.tx.DoOperations[1].Action = "addAttrViewCol"
				}
				ret := performTx(h.tx)
				if failure == "" {
					kv, _ := a.GetKeyValues("status")
					if ret != nil || !h.committed || !reflect.DeepEqual(optionNames(kv.Key), []string{"New"}) || kv.Key.Options[0].Desc != " final " || !reflect.DeepEqual(optionValues(kv.Values[0]), []string{"New"}) || kv.Values[0].MSelect[0].Color != "8" || h.counts["stage:"+cellAV] != 5 || h.counts["related"] != 1 {
						t.Fatal("lost mixed candidates", ret, h.counts)
					}
				} else if ret == nil || h.committed {
					t.Fatal("failed mixed batch committed", ret)
				}
				if (failure == "bad-second" || failure == "unaudited") && (len(h.staged) != 0 || h.counts["groups:"+cellAV] != 0) {
					t.Fatal("batch partially admitted")
				}
				if !reflect.DeepEqual(before, cellDigest(t, root, page)) {
					t.Fatal("mixed transaction changed source")
				}
			})
		}
		h := optionHarness(t, l, root, page, "")
		before := cellDigest(t, root, page)
		a, e := h.cell.read(cellAV)
		if e != nil {
			t.Fatal(e)
		}
		for _, v := range a.Views {
			v.Filters = []*av.ViewFilter{{Column: "status", Operator: av.FilterOperatorIsEqual, Value: &av.Value{Type: av.KeyTypeSelect, MSelect: []*av.ValueSelect{{Content: "Old"}}}}, {Column: "status", Operator: av.FilterOperatorIsNotEqual, Value: &av.Value{Type: av.KeyTypeSelect, MSelect: []*av.ValueSelect{{Content: "Old"}}}}, {Column: "status", Operator: av.FilterOperatorIsNotEmpty, Value: &av.Value{Type: av.KeyTypeSelect, MSelect: []*av.ValueSelect{{Content: "Old"}}}}}
		}
		h.tx.DoOperations = []*Operation{optionOp("removeAttrViewColOption", "status", "Old")}
		if ret := performTx(h.tx); ret != nil {
			t.Fatal(ret)
		}
		for _, v := range a.Views {
			if len(v.Filters) != 1 || v.Filters[0].Operator != av.FilterOperatorIsNotEmpty || v.Filters[0].Value.MSelect[0].Content != "Old" {
				t.Fatal("adjacent removals skipped or emptiness rule changed", v.Filters)
			}
		}
		if !reflect.DeepEqual(before, cellDigest(t, root, page)) {
			t.Fatal("adjacent-filter candidate changed source")
		}
	}
}

func TestAVOptionCancellationAfterSuccessfulCallbacks(t *testing.T) {
	m, store, root := optionSource(t)
	page := "1760000000000000002"
	l, e := m.Acquire(context.Background(), store, page)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(l.Release)
	for _, at := range []string{"read", "groups", "stage"} {
		t.Run(at, func(t *testing.T) {
			before := cellDigest(t, root, page)
			h := optionHarness(t, l, root, page, "")
			env := h.env.avOptions
			switch at {
			case "read":
				read := env.read
				env.read = func(id string) (*av.AttributeView, error) { a, e := read(id); h.cancel(); return a, e }
			case "groups":
				groups := env.groups
				env.groups = func(a *av.AttributeView) error { e := groups(a); h.cancel(); return e }
			case "stage":
				stage := env.stage
				env.stage = func(a *av.AttributeView) error { e := stage(a); h.cancel(); return e }
			}
			h.tx.DoOperations = []*Operation{validOptionOp("updateAttrViewColOptions", "status")}
			ret := performTx(h.tx)
			if ret == nil || h.committed || h.counts["finish"] != 0 {
				t.Fatal("post-callback cancellation ignored", ret, h.counts)
			}
			if at == "read" && h.counts["groups:"+cellAV] != 0 {
				t.Fatal("cancelled read reached grouping")
			}
			if at != "stage" && len(h.staged) != 0 {
				t.Fatal("cancelled step reached staging")
			}
			if !reflect.DeepEqual(before, cellDigest(t, root, page)) {
				t.Fatal("cancellation changed source")
			}
		})
	}
}

func TestAVOptionConsecutiveEditsResetOriginalRenderingMemo(t *testing.T) {
	m, store, root := optionSource(t)
	page := "1760000000000000002"
	l, e := m.Acquire(context.Background(), store, page)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(l.Release)
	h := optionHarness(t, l, root, page, "")
	a, e := h.cell.read(cellAV)
	if e != nil {
		t.Fatal(e)
	}
	stale := &av.Table{}
	a.RenderedViewables = map[string]av.Viewable{"v": stale}
	groups := h.env.avOptions.groups
	calls := 0
	h.env.avOptions.groups = func(a *av.AttributeView) error {
		calls++
		if len(a.RenderedViewables) != 0 {
			t.Fatal("stale projection reused")
		}
		if e := groups(a); e != nil {
			return e
		}
		a.RenderedViewables["v"] = stale
		return nil
	}
	h.tx.DoOperations = []*Operation{validOptionOp("updateAttrViewColOptions", "status"), validOptionOp("updateAttrViewColOption", "status"), validOptionOp("setAttrViewColOptionDesc", "status")}
	if ret := performTx(h.tx); ret != nil || calls != 2 || !h.committed || len(a.RenderedViewables) != 0 {
		t.Fatal(ret, calls)
	}
}
