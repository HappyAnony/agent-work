package model

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"wiki-kernel/av"
)

func TestOriginalAVCellContextFailuresCancelAndMalformedSource(t *testing.T) {
	m, store, root := cellSource(t)
	for _, page := range []string{"1760000000000000002", "1760000000000000003"} {
		t.Run(page, func(t *testing.T) {
			t.Parallel()
			l, e := m.Acquire(context.Background(), store, page)
			if e != nil {
				t.Fatal(e)
			}
			t.Cleanup(l.Release)
			cases := []struct{ scenario, failure string }{}
			for _, f := range []string{"read:" + cellAV, "groups:" + cellAV, "stage:" + cellAV, "related", "enqueue", "finish", "read:" + cellRelated, "groups:" + cellRelated, "stage:" + cellRelated, "reload"} {
				cases = append(cases, struct{ scenario, failure string }{"plain", f}, struct{ scenario, failure string }{"plain", "cancel-" + f})
			}
			for _, f := range []string{"node", "load", "names", "attrs", "tree-stage", "notify"} {
				cases = append(cases, struct{ scenario, failure string }{"bind", f}, struct{ scenario, failure string }{"bind", "cancel-" + f})
			}
			for _, f := range []string{"node", "load", "text", "attrs", "tree-stage", "notify"} {
				cases = append(cases, struct{ scenario, failure string }{"static", f}, struct{ scenario, failure string }{"static", "cancel-" + f})
			}
			for _, f := range []string{"read:" + cellDest, "groups:" + cellDest, "stage:" + cellDest} {
				cases = append(cases, struct{ scenario, failure string }{"relation", f}, struct{ scenario, failure string }{"relation", "cancel-" + f})
			}
			for _, s := range []string{"missing-block", "nil-source", "nil-kv", "nil-value", "nil-block", "nil-option", "missing-key", "null", "decode", "marshal", "wrong-type", "wrong-row", "wrong-key", "nil-relation", "nil-select", "missing-back-key", "bad-target", "nil-back", "pre-cancel"} {
				cases = append(cases, struct{ scenario, failure string }{s, ""})
			}
			for _, tc := range cases {
				t.Run(tc.scenario+"/"+tc.failure, func(t *testing.T) {
					before := cellDigest(t, root, page)
					h := newCellHarness(t, l, root, page, tc.failure)
					// Shape mutation is confined to detached candidates, never fixture source.
					key, data := "text", interface{}(map[string]interface{}{"text": map[string]interface{}{"content": "new"}})
					if tc.scenario == "bind" || tc.scenario == "missing-block" {
						key = "primary"
						id := settingsBlock
						if tc.scenario == "missing-block" {
							id = "missing"
						}
						data = map[string]interface{}{"isDetached": false, "block": map[string]interface{}{"id": id, "content": "new"}}
					}
					if tc.scenario == "static" {
						key = "primary"
						src, e := h.cell.read(cellAV)
						if e != nil {
							t.Fatal(e)
						}
						src.GetValue(key, settingsBlock).IsDetached = false
						data = map[string]interface{}{"block": map[string]interface{}{"id": settingsBlock, "content": "new"}}
					}
					if tc.scenario == "relation" || tc.scenario == "missing-back-key" || tc.scenario == "bad-target" || tc.scenario == "nil-back" {
						key = "relation"
						data = map[string]interface{}{"relation": map[string]interface{}{"blockIDs": []string{settingsBlock, cellSecond}}}
					}
					mutate := func(fn func(*av.AttributeView)) {
						src, e := h.cell.read(cellAV)
						if e != nil {
							t.Fatal(e)
						}
						fn(src)
					}
					switch tc.scenario {
					case "nil-source":
						h.cell.read = func(string) (*av.AttributeView, error) { return nil, nil }
					case "nil-kv":
						mutate(func(a *av.AttributeView) { a.KeyValues = append(a.KeyValues, nil) })
					case "nil-value":
						mutate(func(a *av.AttributeView) { a.KeyValues[0].Values = append(a.KeyValues[0].Values, nil) })
					case "nil-block":
						mutate(func(a *av.AttributeView) { a.KeyValues[0].Values[0].Block = nil })
					case "nil-option":
						mutate(func(a *av.AttributeView) { k, _ := a.GetKey("status"); k.Options = append(k.Options, nil) })
					case "missing-key":
						key = "missing"
					case "null":
						data = nil
					case "decode":
						data = "wrong"
					case "marshal":
						data = func() {}
					case "wrong-type":
						data = map[string]interface{}{"type": "number"}
					case "wrong-row":
						data = map[string]interface{}{"blockID": cellSecond}
					case "wrong-key":
						data = map[string]interface{}{"keyID": "number"}
					case "nil-relation":
						key = "relation"
						data = map[string]interface{}{"relation": nil}
					case "nil-select":
						key = "status"
						data = map[string]interface{}{"mSelect": []interface{}{nil}}
					case "missing-back-key":
						mutate(func(a *av.AttributeView) { k, _ := a.GetKey("relation"); k.Relation.BackKeyID = "missing" })
					case "bad-target":
						target, e := h.cell.read(cellDest)
						if e != nil {
							t.Fatal(e)
						}
						target.KeyValues = append(target.KeyValues, nil)
					case "nil-back":
						target, e := h.cell.read(cellDest)
						if e != nil {
							t.Fatal(e)
						}
						target.GetValue("back", cellSecond).Relation = nil
					case "pre-cancel":
						h.cancel()
					}
					h.tx.DoOperations = []*Operation{cellOp(key, data)}
					ret := performTx(h.tx)
					if ret == nil || h.committed || h.tx.state.Load() == 2 || strings.Contains(ret.msg, "PANIC RECOVERED") {
						t.Fatal("failure swallowed or panic instead of explicit error", ret, h.counts)
					}
					if tc.failure != "" && !strings.HasPrefix(tc.failure, "cancel-") && !strings.Contains(ret.msg, "synthetic "+tc.failure) {
						t.Fatal("wrong dependency failed", ret, h.counts)
					}
					if h.tx.m != nil {
						if !h.tx.m.TryLock() {
							t.Fatal("lock leaked")
						}
						h.tx.m.Unlock()
					}
					if !reflect.DeepEqual(before, cellDigest(t, root, page)) {
						t.Fatal("failed candidate changed source")
					}
				})
			}
		})
	}
}

func TestAVCellDependenciesRejectBeforeBeginAndScopeCannotOptIntoLegacy(t *testing.T) {
	m, store, root := cellSource(t)
	page := "1760000000000000002"
	l, e := m.Acquire(context.Background(), store, page)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(l.Release)
	for _, missing := range []string{"environment", "read", "stage", "groups", "node", "blockText", "names", "attrs", "related", "now", "newID", "color"} {
		t.Run(missing, func(t *testing.T) {
			h := newCellHarness(t, l, root, page, "")
			h.env.parser = func() *lute.Lute { t.Fatal("unbound dependencies admitted"); return nil }
			switch missing {
			case "environment":
				h.env.avCell = nil
			case "read":
				h.cell.read = nil
			case "stage":
				h.cell.stage = nil
			case "groups":
				h.cell.groups = nil
			case "node":
				h.cell.node = nil
			case "blockText":
				h.cell.blockText = nil
			case "names":
				h.cell.names = nil
			case "attrs":
				h.cell.attrs = nil
			case "related":
				h.cell.related = nil
			case "now":
				h.cell.now = nil
			case "newID":
				h.cell.newID = nil
			case "color":
				h.cell.color = nil
			}
			h.tx.DoOperations = []*Operation{cellOp("text", map[string]interface{}{})}
			ret := performTx(h.tx)
			if ret == nil || ret.msg != "transaction_av_cell_dependencies_unbound" || len(h.counts) != 0 {
				t.Fatal(ret, h.counts)
			}
		})
	}
	h := newCellHarness(t, l, root, page, "attrs")
	h.cell.legacy = true
	h.tx.DoOperations = []*Operation{cellOp("primary", map[string]interface{}{"isDetached": false, "block": map[string]interface{}{"id": settingsBlock, "content": "bind"}})}
	if ret := performTx(h.tx); ret == nil || !strings.Contains(ret.msg, "synthetic attrs") {
		t.Fatal("scoped service escaped into legacy best effort", ret)
	}
}

func TestAVCellBatchCompatibilityConsecutiveCandidatesAndFailure(t *testing.T) {
	m, store, root := cellSource(t)
	for _, page := range []string{"1760000000000000002", "1760000000000000003"} {
		l, e := m.Acquire(context.Background(), store, page)
		if e != nil {
			t.Fatal(e)
		}
		t.Cleanup(l.Release)
		for _, mode := range []string{"batch", "consecutive", "bad-outer", "nil-item", "bad-value", "pre-cancel", "empty"} {
			t.Run(page+"/"+mode, func(t *testing.T) {
				before := cellDigest(t, root, page)
				h := newCellHarness(t, l, root, page, "")
				op1 := cellOp("text", map[string]interface{}{"text": map[string]interface{}{"content": page}})
				op2 := cellOp("number", map[string]interface{}{"number": map[string]interface{}{"content": 42, "isNotEmpty": true}})
				h.tx.DoOperations = []*Operation{op1, op2}
				if mode == "consecutive" {
					if ret := performTx(h.tx); ret != nil {
						t.Fatal(ret)
					}
				} else {
					if e := h.tx.begin(); e != nil {
						t.Fatal(e)
					}
					values := []interface{}{map[string]interface{}{"keyID": "text", "itemID": settingsBlock, "value": op1.Data}, map[string]interface{}{"keyID": "number", "rowID": settingsBlock, "value": op2.Data}}
					if mode == "bad-outer" {
						values[1] = 42
					}
					if mode == "nil-item" {
						values[1] = map[string]interface{}{"keyID": "number", "itemID": nil, "rowID": settingsBlock, "value": op2.Data}
					}
					if mode == "bad-value" {
						values[1] = map[string]interface{}{"keyID": "number", "itemID": settingsBlock, "value": nil}
					}
					if mode == "pre-cancel" {
						h.cancel()
					}
					if mode == "empty" {
						values = nil
					}
					e = BatchUpdateAttributeViewCells(h.tx, cellAV, values)
					if mode == "batch" || mode == "empty" {
						if e != nil {
							h.tx.rollback()
							t.Fatal(e)
						}
						if e = h.tx.commit(); e != nil {
							t.Fatal(e)
						}
					} else {
						if e == nil {
							h.tx.rollback()
							t.Fatal("malformed batch succeeded")
						}
						h.tx.rollback()
					}
				}
				if mode == "batch" || mode == "consecutive" {
					a := h.cache[cellAV]
					if !h.committed || a.GetValue("text", settingsBlock).Text.Content != page || a.GetValue("number", settingsBlock).Number.Content != 42 || h.counts["stage:"+cellAV] != 2 || h.counts["reload"] != 1 {
						t.Fatal("lost earlier candidate/deferred dedup", h.counts)
					}
				}
				if mode == "bad-outer" || mode == "nil-item" {
					if h.counts["read:"+cellAV] != 0 {
						t.Fatal("bad outer batch read before validation")
					}
				}
				if !reflect.DeepEqual(before, cellDigest(t, root, page)) {
					t.Fatal("batch changed authority")
				}
			})
		}
	}
}

func TestAVCellOwnedAttributeAndNamesOriginalRules(t *testing.T) {
	n := &ast.Node{ID: settingsBlock, Type: ast.NodeParagraph}
	n.SetIALAttr("tags", "old")
	changed := 0
	invalid := func(string) error { return errors.New("bad attribute") }
	attrs := map[string]string{"Custom-Key": " value, ", "tags": "one,one,two", "clear": ""}
	old, e := setNodeAttrs0WithServices(n, attrs, invalid, func() error { changed++; return nil })
	if e != nil || old["tags"] != "old" || n.IALAttr("custom-key") != "value" || n.IALAttr("tags") != "one,two" || changed != 1 {
		t.Fatal("original normalization changed", old, attrs, e)
	}
	before := n.IALAttr("custom-key")
	if _, e = setNodeAttrs0WithServices(n, map[string]string{"bad key": "x"}, invalid, func() error { return nil }); e == nil || n.IALAttr("custom-key") != before {
		t.Fatal("bad name accepted")
	}
	if _, e = setNodeAttrs0WithServices(n, map[string]string{"tags": "new"}, invalid, func() error { return errors.New("owned tag event failed") }); e == nil {
		t.Fatal("tag failure swallowed")
	}
	n.AttributeViewType = "table"
	notifications := blockAttrTransactions(map[string]string{"custom-key": "before"}, n)
	if len(notifications) != 1 || len(notifications[0].DoOperations) != 1 || notifications[0].UndoOperations == nil {
		t.Fatal("original notification structure changed")
	}
	op := notifications[0].DoOperations[0]
	data := op.Data.(map[string]interface{})
	if op.Action != "updateAttrs" || op.ID != settingsBlock || data["data-av-type"] != "table" || data["old"].(map[string]string)["custom-key"] != "before" || data["new"].(map[string]string)["custom-key"] != "value" {
		t.Fatal(op, data)
	}
	// Name rendering shares the original template, separators and fallback.
	lookups := map[string]string{"a": "Alpha", "b": ""}
	lookup := func(id string) (string, error) {
		s, ok := lookups[id]
		if !ok {
			return "", errors.New("missing name")
		}
		return s, nil
	}
	s, e := attributeViewNamesWithServices("a,b", lookup, func() string { return "owner fallback" }, false)
	if e != nil || !strings.Contains(s, "Alpha") || !strings.Contains(s, "owner fallback") || strings.HasSuffix(s, "&nbsp;") {
		t.Fatal(s, e)
	}
	if _, e = attributeViewNamesWithServices("missing", lookup, func() string { return "fallback" }, false); e == nil {
		t.Fatal("scoped name error swallowed")
	}
	if s, e = attributeViewNamesWithServices("missing,a", lookup, func() string { return "fallback" }, true); e != nil || !strings.Contains(s, "Alpha") {
		t.Fatal("legacy name skip changed", s, e)
	}
}
