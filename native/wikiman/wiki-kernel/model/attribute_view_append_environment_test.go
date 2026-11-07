package model

import (
	"context"
	"encoding/json"
	"reflect"
	"slices"
	"testing"
	"time"
	"wiki-kernel/av"
	"wiki-kernel/internal/workspace"
)

func appendHarness(t *testing.T, l *workspace.Lease, root, page, failure string, layout av.LayoutType) *managementHarness {
	h := newManagementHarness(t, l, root, page, failure, layout)
	h.env.avAppend = &attributeViewAppendEnvironment{ctx: h.ctx, read: h.cell.read, stage: h.cell.stage, groups: h.cell.groups,
		reload: func(id string) error {
			if err := h.mark("append-reload"); err != nil {
				return err
			}
			h.events = append(h.events, "reload:"+id)
			return nil
		},
		now: func() time.Time { h.mark("append-clock"); return h.cell.now() }, newID: func() string {
			if err := h.mark("append-id"); err != nil {
				return ""
			}
			return h.cell.newID()
		}, cloneValue: func(v *av.Value) (*av.Value, error) {
			if err := h.mark("append-clone"); err != nil {
				return nil, err
			}
			return cloneAttributeViewAppendValue(v)
		},
	}
	return h
}
func appendValues(page string) [][]*av.Value {
	primary := &av.Value{ID: "caller", KeyID: "primary", BlockID: "caller-row", Type: av.KeyTypeText, IsDetached: false, CreatedAt: 1, UpdatedAt: 2, IsRenderAutoFill: true, Block: &av.ValueBlock{ID: settingsBlock, Content: page + " appended", Icon: "icon"}}
	return [][]*av.Value{
		{primary, {KeyID: "status", MSelect: []*av.ValueSelect{{Content: "Old", Color: "99"}}}, {KeyID: "number", Number: &av.ValueNumber{Content: 23.5, IsNotEmpty: true}}, {KeyID: "date", Date: &av.ValueDate{Content: 777, IsNotEmpty: true}}, {KeyID: "relation", Relation: &av.ValueRelation{BlockIDs: []string{settingsBlock}}}},
		{primary, {KeyID: "status", MSelect: []*av.ValueSelect{{Content: "Batch new", Color: "7"}}}, {KeyID: "text", Text: &av.ValueText{Content: "raw text"}}, {KeyID: "multi", MSelect: []*av.ValueSelect{{Content: "multi new", Color: "9"}, {Content: "multi new", Color: "4"}}}},
	}
}

func TestAVAppendOwnersLayoutsValuesAndGroups(t *testing.T) {
	m, store, root := managementSource(t)
	for _, page := range []string{"1760000000000000002", "1760000000000000003"} {
		t.Run(page, func(t *testing.T) {
			t.Parallel()
			l, err := m.Acquire(context.Background(), store, page)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(l.Release)
			for _, layout := range []av.LayoutType{av.LayoutTypeTable, av.LayoutTypeGallery, av.LayoutTypeKanban} {
				t.Run(string(layout), func(t *testing.T) {
					h := appendHarness(t, l, root, page, "", layout)
					a, _, _, _ := orderGroup(t, h.cellHarness, layout, "status")
					before := cellDigest(t, root, page)
					reg := managementRegistry(t, h)
					values := appendValues(page)
					input, _ := json.Marshal(values)
					oldLen := len(a.GetBlockKeyValues().Values)
					oldOptions := len(a.KeyValues[4].Key.Options)
					if err := h.tx.appendAttributeViewDetachedBlocksWithValues(cellAV, values); err != nil {
						t.Fatal(err, h.counts)
					}
					raw, _ := json.Marshal(values)
					if string(raw) != string(input) {
						t.Fatal("append changed caller input")
					}
					primary := a.GetBlockKeyValues().Values[oldLen:]
					if len(primary) != 2 || primary[0] == primary[1] || primary[0].Block == primary[1].Block || primary[0].BlockID == primary[1].BlockID {
						t.Fatal("shared input rows aliased")
					}
					ids := []string{primary[0].BlockID, primary[1].BlockID}
					for _, v := range primary {
						if v.Type != av.KeyTypeBlock || !v.IsDetached || v.Block.ID != "" || v.Block.Content != page+" appended" || v.Block.Icon != "icon" || v.CreatedAt != 12345 || v.UpdatedAt != 12345 || v.Block.Created != 12345 || v.Block.Updated != 12345 || v.IsRenderAutoFill {
							t.Fatal("original detached metadata lost", v)
						}
					}
					for _, view := range a.Views {
						if !slices.Equal(view.ItemIDs, []string{settingsBlock, cellSecond, ids[0], ids[1]}) {
							t.Fatal("append order", view.ID, view.ItemIDs)
						}
						group := view.GetGroupByGroupValue("Batch new")
						if group == nil || !slices.Contains(group.GroupItemIDs, ids[1]) {
							t.Fatal("original regroup lost row", view.ID)
						}
					}
					if a.GetValue("number", ids[0]).Number.Content != 23.5 || a.GetValue("date", ids[0]).Date.Content != 777 || a.GetValue("text", ids[1]).Text.Content != "raw text" {
						t.Fatal("value payload changed")
					}
					status, _ := a.GetKey("status")
					multi, _ := a.GetKey("multi")
					if status.GetOption("Batch new").Color != "7" || a.GetValue("status", ids[0]).MSelect[0].Color != "2" || len(status.Options) != oldOptions+1 || len(multi.Options) != 1 {
						t.Fatal("option creation/color rules")
					}
					for _, opt := range a.GetValue("multi", ids[1]).MSelect {
						if opt.Color != "9" {
							t.Fatal("existing option color not preserved")
						}
					}
					if h.counts["append-clock"] != 1 || h.counts["append-clone"] != len(values[0])+len(values[1]) || h.counts["stage:"+cellAV] != 1 || h.counts["append-reload"] != 1 || h.counts["stage:"+cellDest] != 0 || len(h.notifications) != 0 || h.counts["related"] != 0 {
						t.Fatal("append introduced unrelated effects", h.counts)
					}
					primary[0].Block.Content = "only first row"
					if primary[1].Block.Content == primary[0].Block.Content || values[0][0].Block.Content == primary[0].Block.Content {
						t.Fatal("post-append aliases")
					}
					assertManagementSource(t, h, before, reg)
				})
			}
		})
	}
}

func TestAVAppendEmptySparseAndSequentialCandidates(t *testing.T) {
	m, store, root := managementSource(t)
	page := "1760000000000000002"
	l, err := m.Acquire(context.Background(), store, page)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(l.Release)
	for _, scenario := range []string{"empty-batch", "empty-row", "sparse-row", "sequential", "mixed-edit"} {
		t.Run(scenario, func(t *testing.T) {
			h := appendHarness(t, l, root, page, "", av.LayoutTypeTable)
			a := managementPrepare(t, h, av.LayoutTypeTable)
			before := cellDigest(t, root, page)
			rows := appendValues(page)
			old := len(a.GetBlockKeyValues().Values)
			if scenario == "empty-batch" {
				rows = nil
			}
			if scenario == "empty-row" {
				rows = [][]*av.Value{nil}
			}
			if scenario == "sparse-row" {
				rows = [][]*av.Value{{{KeyID: "text", Text: &av.ValueText{Content: "sparse"}}}}
			}
			input, _ := json.Marshal(rows)
			if err := h.tx.appendAttributeViewDetachedBlocksWithValues(cellAV, rows); err != nil {
				t.Fatal(err)
			}
			switch scenario {
			case "empty-batch":
				if len(a.GetBlockKeyValues().Values) != old || len(a.Views[0].ItemIDs) != 2 || h.counts["append-id"] != 0 {
					t.Fatal("empty batch rule")
				}
			case "empty-row", "sparse-row":
				if len(a.Views[0].ItemIDs) != 3 || len(a.GetBlockKeyValues().Values) != old {
					t.Fatal("original partial-row semantics changed")
				}
			case "sequential":
				if err := h.tx.appendAttributeViewDetachedBlocksWithValues(cellAV, rows); err != nil {
					t.Fatal(err)
				}
				if len(a.GetBlockKeyValues().Values) != old+4 || h.counts["stage:"+cellAV] != 2 {
					t.Fatal("sequential append lost candidate")
				}
			case "mixed-edit":
				id := a.GetBlockKeyValues().Values[old].BlockID
				h.tx.DoOperations = []*Operation{{Action: "updateAttrViewCell", AvID: cellAV, RowID: id, KeyID: "number", Data: map[string]interface{}{"number": map[string]interface{}{"content": float64(99), "isNotEmpty": true}}}}
				if ret := performTx(h.tx); ret != nil || !h.committed {
					t.Fatal(ret)
				}
				if a.GetValue("number", id).Number.Content != 99 || len(a.GetBlockKeyValues().Values) != old+2 {
					t.Fatal("mixed candidate lost append")
				}
			}
			raw, _ := json.Marshal(rows)
			if !reflect.DeepEqual(input, raw) {
				t.Fatal("input mutated")
			}
			assertFieldSourceUnchanged(t, h.cellHarness, before)
		})
	}
}
