package model

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"wiki-kernel/av"
)

func TestAVIdentityOwnersBothDirectionsOriginalRulesAndReadOnly(t *testing.T) {
	m, store, root := nameSource(t)
	for _, page := range []string{"1760000000000000002", "1760000000000000003"} {
		t.Run(page, func(t *testing.T) {
			t.Parallel()
			l, err := m.Acquire(context.Background(), store, page)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(l.Release)
			h := newBoundHarness(t, l, root, page, "")
			before := nameDigest(t, root, page)
			a, err := h.cell.read(cellAV)
			if err != nil {
				t.Fatal(err)
			}
			primary := a.GetBlockKeyValues()
			primary.Values = []*av.Value{{BlockID: page + "-row-a", Block: &av.ValueBlock{ID: settingsBlock}}, {BlockID: page + "-row-b", IsDetached: true, Block: &av.ValueBlock{ID: settingsBlock}}, {BlockID: page + "-row-b", Block: &av.ValueBlock{ID: "dangling-block"}}, {BlockID: page + "-detached", IsDetached: true, Block: &av.ValueBlock{}}}
			original := mustDuplicateJSON(t, a)
			for i := 0; i < 2; i++ {
				ret, err := h.tx.attributeViewItemIDs(cellAV, []string{settingsBlock, "dangling-block", "missing", "", settingsBlock})
				want := map[string]string{settingsBlock: page + "-row-b", "dangling-block": page + "-row-b", "missing": "", "": page + "-detached"}
				if err != nil || !reflect.DeepEqual(ret, want) {
					t.Fatal(ret, want, err)
				}
				ret, err = h.tx.attributeViewBoundBlockIDs(cellAV, []string{page + "-row-a", page + "-row-b", page + "-detached", "missing"})
				want = map[string]string{page + "-row-a": settingsBlock, page + "-row-b": "dangling-block", page + "-detached": "", "missing": ""}
				if err != nil || !reflect.DeepEqual(ret, want) {
					t.Fatal(ret, want, err)
				}
			}
			if !reflect.DeepEqual(original, mustDuplicateJSON(t, a)) || len(h.staged) != 0 || len(h.notifications) != 0 || h.counts["finish"] != 0 || h.counts["node"] != 0 {
				t.Fatal("lookup mutated state or queried block registry", h.counts)
			}
			assertNameSource(t, h.cellHarness, before)
		})
	}
}

func TestAVIdentityDependencySourceFailuresAndLateCancellation(t *testing.T) {
	m, store, root := nameSource(t)
	page := "1760000000000000002"
	l, err := m.Acquire(context.Background(), store, page)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(l.Release)
	for _, scenario := range []string{"missing-env", "missing-read", "missing-context", "cancel", "late-cancel", "read", "nil-source", "wrong-id", "nil-key", "no-primary", "nil-value", "nil-block", "empty-id"} {
		for _, toItems := range []bool{false, true} {
			t.Run(scenario+map[bool]string{true: "/items"}[toItems], func(t *testing.T) {
				h := newBoundHarness(t, l, root, page, "")
				before := nameDigest(t, root, page)
				a, err := h.cell.read(cellAV)
				if err != nil {
					t.Fatal(err)
				}
				id := cellAV
				switch scenario {
				case "missing-env":
					h.env.avCell = nil
				case "missing-read":
					h.env.avCell.read = nil
				case "missing-context":
					h.env.ctx = nil
				case "cancel":
					h.cancel()
				case "late-cancel":
					h.env.avCell.read = func(string) (*av.AttributeView, error) { h.cancel(); return a, nil }
				case "read":
					h.env.avCell.read = func(string) (*av.AttributeView, error) { return nil, errors.New("owner failure") }
				case "nil-source":
					h.env.avCell.read = func(string) (*av.AttributeView, error) { return nil, nil }
				case "wrong-id":
					a.ID = cellDest
				case "nil-key":
					a.KeyValues[0].Key = nil
				case "no-primary":
					a.GetBlockKeyValues().Key.Type = av.KeyTypeText
				case "nil-value":
					a.GetBlockKeyValues().Values[0] = nil
				case "nil-block":
					a.GetBlockKeyValues().Values[0].Block = nil
				case "empty-id":
					id = ""
				}
				var ret map[string]string
				if toItems {
					ret, err = h.tx.attributeViewItemIDs(id, []string{settingsBlock})
				} else {
					ret, err = h.tx.attributeViewBoundBlockIDs(id, []string{settingsBlock})
				}
				if err == nil || ret != nil || h.counts["finish"] != 0 {
					t.Fatal(scenario, ret, err)
				}
				assertNameSource(t, h.cellHarness, before)
			})
		}
	}
}

func TestAVIdentityMinimalServicesEmptyRequestsAndConsecutiveCandidates(t *testing.T) {
	reads := 0
	a := &av.AttributeView{ID: cellAV, Views: []*av.View{nil}, KeyValues: []*av.KeyValues{{Key: &av.Key{Type: av.KeyTypeBlock}, Values: []*av.Value{{BlockID: "row", Block: &av.ValueBlock{ID: "before"}}}}}}
	tx := &Transaction{environment: &transactionEnvironment{ctx: context.Background(), avCell: &attributeViewCellEnvironment{read: func(string) (*av.AttributeView, error) { reads++; return a, nil }}}}
	ret, err := tx.attributeViewItemIDs(cellAV, nil)
	if err != nil || ret == nil || len(ret) != 0 || reads != 1 {
		t.Fatal("empty input must still read source", ret, err, reads)
	}
	a.KeyValues[0].Values[0].Block.ID = "after"
	ret, err = tx.attributeViewBoundBlockIDs(cellAV, []string{"row"})
	if err != nil || ret["row"] != "after" || reads != 2 {
		t.Fatal(ret, err, reads)
	}
	a.KeyValues[0].Values = nil
	ret, err = tx.attributeViewItemIDs(cellAV, []string{"missing"})
	if err != nil || ret["missing"] != "" {
		t.Fatal(ret, err)
	}
}
