package model

import (
	"context"
	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"strings"
	"testing"
	"wiki-kernel/av"
)

func TestAVRowsDefaultsGroupsReverseAndBatch(t *testing.T) {
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
				for _, ignore := range []bool{false, true} {
					t.Run(string(layout)+string(rune('0'+boolInt(ignore))), func(t *testing.T) {
						h := newRowHarness(t, l, root, page, "")
						a, v, _, target := orderGroup(t, h.cellHarness, layout, "status")
						before := cellDigest(t, root, page)
						numbers, _ := a.GetKeyValues("number")
						for i, value := range numbers.Values {
							value.Number.Content = float64(7 + i*2)
							value.Number.IsNotEmpty = true
						}
						v.Filters = []*av.ViewFilter{{Column: "number", Operator: av.FilterOperatorIsGreater, Value: &av.Value{Type: av.KeyTypeNumber, Number: &av.ValueNumber{Content: 5, IsNotEmpty: true}}}, {Column: "relation", Operator: av.FilterOperatorIsNotEmpty, Value: &av.Value{Type: av.KeyTypeRelation, Relation: &av.ValueRelation{BlockIDs: []string{cellSecond}}}}}
						a.KeyValues = append(a.KeyValues, &av.KeyValues{Key: &av.Key{ID: "template", Name: "Label", Type: av.KeyTypeTemplate, Template: ".action{.Text}"}})
						dateKV, _ := a.GetKeyValues("date")
						dateKV.Key.Date = &av.Date{AutoFillNow: true, FillSpecificTime: true}
						dateKV.Values = append(dateKV.Values, &av.Value{ID: "explicit", KeyID: "date", BlockID: rowFirst, Type: av.KeyTypeDate, Date: &av.ValueDate{Content: 777, IsNotEmpty: true, IsNotTime: true}})
						op := rowInsert(layout, true)
						op.Srcs = op.Srcs[:1]
						op.GroupID = target
						op.PreviousID = cellSecond
						op.IgnoreDefaultFill = ignore
						h.tx.DoOperations = []*Operation{op}
						if ret := performTx(h.tx); ret != nil || !h.committed {
							t.Fatal(ret, h.counts)
						}
						if got := a.GetValue("date", rowFirst); got == nil || got.Date.Content != 777 || !got.Date.IsNotTime {
							t.Fatal("explicit date overwritten", got)
						}
						if ignore {
							if value := a.GetValue("number", rowFirst); (value != nil && value.Number != nil && value.Number.Content == 6) || h.counts["near"] != 0 || h.counts["reverse"] != 0 {
								t.Fatal("ignored defaults executed")
							}
						} else {
							if a.GetValue("number", rowFirst).Number.Content != 6 || a.GetValue("status", rowFirst).MSelect[0].Content != "New" {
								t.Fatal("filter/group defaults lost")
							}
							dest := h.cache[cellDest]
							if dest == nil {
								t.Fatal("reverse destination not read", h.counts)
							}
							back := dest.GetValue("back", settingsBlock)
							if back == nil || !strings.Contains(strings.Join(back.Relation.BlockIDs, ","), rowFirst) || h.counts["template"] == 0 || h.counts["reverse"] == 0 {
								t.Fatal("reverse/template default lost", h.counts)
							}
						}
						assertFieldSourceUnchanged(t, h.cellHarness, before)
					})
				}
			}
			for _, scenario := range []string{"batch", "mixed", "document-remove", "missing-bound"} {
				t.Run(scenario, func(t *testing.T) {
					h := newRowHarness(t, l, root, page, "")
					a := rowPrepare(t, h, av.LayoutTypeTable)
					before := cellDigest(t, root, page)
					switch scenario {
					case "batch":
						if err := batchReplaceAttributeViewBlocksWithEnvironment(cellAV, false, []map[string]string{{settingsBlock: settingsBlock}, {cellSecond: cellSecond}}, h.env.avRows); err != nil {
							t.Fatal(err)
						}
						for _, row := range []string{settingsBlock, cellSecond} {
							val := a.GetBlockKeyValues().GetValue(row)
							if val.IsDetached || val.Block.ID != row {
								t.Fatal("batch candidate lost")
							}
						}
						if h.counts["stage:"+cellAV] != 1 {
							t.Fatal("batch stage count", h.counts)
						}
					case "mixed":
						insert := rowInsert(av.LayoutTypeTable, true)
						h.tx.DoOperations = []*Operation{insert, {Action: "replaceAttrViewBlock", AvID: cellAV, PreviousID: rowFirst, NextID: settingsBlock}, {Action: "removeAttrViewBlock", AvID: cellAV, SrcIDs: []string{rowNext}}}
						if ret := performTx(h.tx); ret != nil || !h.committed {
							t.Fatal(ret)
						}
						if a.GetBlockKeyValues().GetValue(rowFirst).Block.ID != settingsBlock || a.GetBlockKeyValues().GetValue(rowNext) != nil {
							t.Fatal("mixed candidate lost")
						}
					default:
						val := a.GetBlockKeyValues().GetValue(settingsBlock)
						val.IsDetached = false
						val.Block.ID = settingsDoc
						load := h.env.load
						h.env.load = func(box, p string, engine *lute.Lute) (*parse.Tree, error) {
							tree, err := load(box, p, engine)
							if err == nil {
								tree.Root.SetIALAttr("custom-hidden", "true")
								tree.Root.SetIALAttr(av.NodeAttrNameAvs, cellAV+","+cellDest+",20261024090003-gon0001")
							}
							return tree, err
						}
						if scenario == "missing-bound" {
							val.Block.ID = "missing"
						}
						h.tx.DoOperations = []*Operation{{Action: "removeAttrViewBlock", AvID: cellAV, SrcIDs: []string{settingsBlock}}}
						if ret := performTx(h.tx); ret != nil || !h.committed {
							t.Fatal(ret)
						}
						if scenario == "document-remove" {
							attrs := h.attrs[settingsDoc]
							if _, ok := attrs["custom-hidden"]; ok || attrs[av.NodeAttrNameAvs] != cellDest || !strings.Contains(attrs[av.NodeAttrViewNames], page) {
								t.Fatal("document cleanup", attrs)
							}
						} else if h.counts["attrs"] != 0 {
							t.Fatal("missing source block not skipped")
						}
					}
					assertFieldSourceUnchanged(t, h.cellHarness, before)
				})
			}
		})
	}
}
func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func TestAVRowsDefaultDependencyFailures(t *testing.T) {
	m, store, root := rowSource(t)
	for _, page := range []string{"1760000000000000002", "1760000000000000003"} {
		t.Run(page, func(t *testing.T) {
			t.Parallel()
			l, err := m.Acquire(context.Background(), store, page)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(l.Release)
			for _, point := range []string{"near", "template", "reverse", "read:" + cellDest, "stage:" + cellDest, "groups:" + cellDest} {
				for _, prefix := range []string{"", "cancel-"} {
					t.Run(prefix+point, func(t *testing.T) {
						h := newRowHarness(t, l, root, page, "")
						a, v, _, target := orderGroup(t, h.cellHarness, av.LayoutTypeTable, "status")
						before := cellDigest(t, root, page)
						v.Filters = []*av.ViewFilter{{Column: "relation", Operator: av.FilterOperatorIsNotEmpty, Value: &av.Value{Type: av.KeyTypeRelation, Relation: &av.ValueRelation{BlockIDs: []string{settingsBlock}}}}}
						a.KeyValues = append(a.KeyValues, &av.KeyValues{Key: &av.Key{ID: "template", Name: "Label", Type: av.KeyTypeTemplate, Template: ".action{.Text}"}})
						op := rowInsert(av.LayoutTypeTable, true)
						op.Srcs = op.Srcs[:1]
						op.GroupID = target
						op.PreviousID = cellSecond
						h.failure = prefix + point
						h.counts = map[string]int{}
						h.tx.DoOperations = []*Operation{op}
						if ret := performTx(h.tx); ret == nil || h.committed || h.counts[point] == 0 || h.counts["finish"] != 0 {
							t.Fatal("default failure escaped", ret, h.counts)
						}
						assertFieldSourceUnchanged(t, h.cellHarness, before)
					})
				}
			}
		})
	}
}

func TestAVRowBlockLabelPrecedence(t *testing.T) {
	n := &ast.Node{Type: ast.NodeParagraph}
	n.SetIALAttr("icon", "icon")
	n.SetIALAttr("name", " A & B ")
	calls := 0
	ref := func(*ast.Node) (string, error) { calls++; return " reference ", nil }
	fallback := func() string { return "owner untitled" }
	icon, text, err := attributeViewBlockTextWithServices(n, "", ref, fallback)
	if err != nil || icon != "icon" || text != "A &amp; B" || calls != 0 {
		t.Fatal(icon, text, err, calls)
	}
	n.SetIALAttr(av.NodeAttrViewStaticText+"-"+cellAV, "static")
	_, text, err = attributeViewBlockTextWithServices(n, cellAV, ref, fallback)
	if err != nil || text != "static" {
		t.Fatal(text, err)
	}
	n.RemoveIALAttr("name")
	_, text, err = attributeViewBlockTextWithServices(n, "", ref, fallback)
	if err != nil || text != "reference" || calls != 1 {
		t.Fatal(text, err, calls)
	}
	_, text, err = attributeViewBlockTextWithServices(n, "", func(*ast.Node) (string, error) { return " ", nil }, fallback)
	if err != nil || text != "owner untitled" {
		t.Fatal(text, err)
	}
}

func TestAVRowsLegacyStandaloneRejectsUnboundOwner(t *testing.T) {
	old := Conf
	Conf = nil
	defer func() { Conf = old }()
	e := (*Transaction)(nil).attributeViewRowEnvironment()
	for _, action := range []string{"insertAttrViewBlock", "removeAttrViewBlock", "replaceAttrViewBlock"} {
		if err := e.validate(action); err != nil {
			t.Fatal(err)
		}
	}
	a := &av.AttributeView{ViewID: "v", Views: []*av.View{{ID: "v"}}}
	v, err := e.view(a, "")
	if err == nil || v != nil {
		t.Fatal("unbound owner selected standalone view", v, err)
	}
}
