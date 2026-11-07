package model

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"wiki-kernel/av"
	"wiki-kernel/internal/workspace"
)

func previewHarness(t *testing.T, l *workspace.Lease, root, page, failure string) *rowHarness {
	h := newRowHarness(t, l, root, page, failure)
	d := h.env.avOrder.defaults
	h.env.avPreview = &attributeViewPreviewEnvironment{ctx: h.ctx, read: h.cell.read,
		clone: func(a *av.AttributeView) (*av.AttributeView, error) {
			if err := h.mark("preview-clone"); err != nil {
				return nil, err
			}
			return cloneAttributeViewPreview(a)
		}, near: d.near, templateKeys: d.templateKeys,
		now:   func() time.Time { h.mark("preview-clock"); return h.cell.now() },
		newID: func() string { h.mark("preview-id"); return h.cell.newID() },
	}
	return h
}

func assertPreviewReadOnly(t *testing.T, h *rowHarness, a *av.AttributeView, snapshot []byte, before map[string]string) {
	t.Helper()
	raw, err := json.Marshal(a)
	if err != nil || !slices.Equal(snapshot, raw) {
		t.Fatal("preview mutated owner AV", err)
	}
	if h.counts["reverse"] != 0 || len(h.staged) != 0 || len(h.notifications) != 0 || h.counts["finish"] != 0 || h.committed || len(h.histories) != 0 || len(h.attrs) != 0 {
		t.Fatal("preview wrote/notified", h.counts)
	}
	assertFieldSourceUnchanged(t, h.cellHarness, before)
}

func TestAVPreviewOwnersLayoutsFiltersGroupsAndPrimary(t *testing.T) {
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
				for _, scenario := range []string{"number", "primary", "date", "template", "relation", "asset", "existing-select"} {
					t.Run(string(layout)+"/"+scenario, func(t *testing.T) {
						h := previewHarness(t, l, root, page, "")
						a, v, _, target := orderGroup(t, h.cellHarness, layout, "status")
						group := target
						prev := cellSecond
						adding := "preview-row"
						for _, value := range a.KeyValues[2].Values {
							value.Number.Content = 9
							value.Number.IsNotEmpty = true
						}
						v.Filters = []*av.ViewFilter{{Column: "number", Operator: av.FilterOperatorIsGreater, Value: &av.Value{Type: av.KeyTypeNumber, Number: &av.ValueNumber{Content: 5, IsNotEmpty: true}}}}
						switch scenario {
						case "primary":
							v.Filters = append(v.Filters, &av.ViewFilter{Column: "primary", Operator: av.FilterOperatorContains, Value: &av.Value{Type: av.KeyTypeBlock, Block: &av.ValueBlock{Content: "New"}}})
						case "date":
							a.KeyValues[3].Key.Date = &av.Date{AutoFillNow: true}
							a.KeyValues[3].Values[1].Date = &av.ValueDate{Content: 777, IsNotEmpty: true, IsNotTime: true}
							v.Filters = append(v.Filters, &av.ViewFilter{Column: "date", Operator: av.FilterOperatorIsNotEmpty, Value: &av.Value{Type: av.KeyTypeDate, Date: &av.ValueDate{IsNotEmpty: true}}})
						case "template":
							a.KeyValues = append(a.KeyValues, &av.KeyValues{Key: &av.Key{ID: "tpl", Name: "Template", Type: av.KeyTypeTemplate, Template: ".action{.Text}"}})
							addFieldFixture(v, "tpl")
							a.GetValue("text", cellSecond).Text.Content = page + " nearby"
							v.Filters = append(v.Filters, &av.ViewFilter{Column: "tpl", Operator: av.FilterOperatorContains, Value: &av.Value{Type: av.KeyTypeText, Text: &av.ValueText{Content: page + " nearby"}}})
						case "relation":
							v.Filters = append(v.Filters, &av.ViewFilter{Column: "relation", Operator: av.FilterOperatorIsNotEmpty, Value: &av.Value{Type: av.KeyTypeRelation, Relation: &av.ValueRelation{BlockIDs: []string{settingsBlock}}}})
						case "asset":
							a.GetValue("asset", cellSecond).MAsset = []*av.ValueAsset{{Name: "image", Content: "assets/a.png", Type: av.AssetTypeImage}}
							v.Filters = append(v.Filters, &av.ViewFilter{Column: "asset", Operator: av.FilterOperatorIsNotEmpty, Value: &av.Value{Type: av.KeyTypeMAsset, MAsset: []*av.ValueAsset{}}})
						case "existing-select":
							prev = "not-found"
							adding = settingsBlock
							v.Filters = nil
							targetGroup := v.GetGroupByGroupValue("Keep")
							if targetGroup == nil {
								t.Fatal("empty target missing")
							}
							group = targetGroup.ID
						}
						before := cellDigest(t, root, page)
						snapshot := mustDuplicateJSON(t, a)
						h.counts = map[string]int{}
						values, err := h.tx.attrViewAddingBlockDefaultValues(cellAV, string(layout), group, prev, adding)
						if err != nil {
							t.Fatal(err, h.counts)
						}
						if scenario == "existing-select" {
							if values["status"] == nil || values["status"].MSelect[0].Content != "Keep" {
								t.Fatal("existing select group default", values)
							}
						} else {
							if values["number"] == nil || values["number"].Number.Content != 6 {
								t.Fatal("filter default", values)
							}
							if scenario != "asset" && (values["status"] == nil || values["status"].MSelect[0].Content != "New") {
								t.Fatal("group default", values)
							}
						}
						switch scenario {
						case "primary":
							if values["primary"] == nil || values["primary"].Block.Content != "" {
								t.Fatal("primary flicker suppression")
							}
						case "date":
							if values["date"] == nil || values["date"].Date.Content != 12345 || !values["date"].Date.IsNotTime {
								t.Fatal("owner date clock/precision", values)
							}
						case "template":
							if values["text"] == nil || values["text"].Text.Content != page+" nearby" || h.counts["template"] == 0 {
								t.Fatal("owner template dependency", values, h.counts)
							}
						case "relation":
							if values["relation"] == nil || !slices.Equal(values["relation"].Relation.BlockIDs, []string{settingsBlock}) {
								t.Fatal("relation preview missing")
							}
						case "asset":
							if values["asset"] == nil || len(values["asset"].MAsset) != 1 || values["status"] != nil {
								t.Fatal("asset early return changed")
							}
						}
						if scenario != "existing-select" {
							if values["number"].BlockID != adding || values["number"].CreatedAt != 12345 || values["number"].UpdatedAt != 13345 || values["number"].ID == "" {
								t.Fatal("original generated metadata")
							}
						}
						assertPreviewReadOnly(t, h, a, snapshot, before)
						for _, value := range values {
							if value != nil && value.Text != nil {
								value.Text.Content = "caller edit"
							}
							if value != nil && len(value.MSelect) > 0 {
								value.MSelect[0].Content = "caller option"
							}
						}
						assertPreviewReadOnly(t, h, a, snapshot, before)
					})
				}
			}
		})
	}
}

func TestAVPreviewSelectionEarlyReturnAndSequentialCandidates(t *testing.T) {
	m, store, root := rowSource(t)
	page := "1760000000000000002"
	l, err := m.Acquire(context.Background(), store, page)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(l.Release)
	for _, scenario := range []string{"no-rules", "stale-group-no-rules", "requested", "current", "first", "empty-adding", "sorted-neighbor", "missing-previous", "continuous"} {
		t.Run(scenario, func(t *testing.T) {
			h := previewHarness(t, l, root, page, "")
			a := orderPrepare(t, h.cellHarness, av.LayoutTypeTable)
			viewID, group, prev, adding := "table", "", "", "preview"
			for i, v := range a.Views {
				v.Filters = []*av.ViewFilter{{Column: "number", Operator: av.FilterOperatorIsEqual, Value: &av.Value{Type: av.KeyTypeNumber, Number: &av.ValueNumber{Content: float64(i + 1), IsNotEmpty: true}}}}
			}
			switch scenario {
			case "no-rules", "stale-group-no-rules":
				a.Views[0].Filters = nil
				group = "stale"
			case "requested":
				viewID = "gallery"
			case "current":
				viewID = "stale"
				a.ViewID = "gallery"
			case "first":
				viewID = "stale"
				a.ViewID = "stale"
			case "empty-adding":
				adding = ""
			case "sorted-neighbor", "missing-previous":
				a.Views[0].Filters = []*av.ViewFilter{{Column: "text", Operator: av.FilterOperatorIsNotEmpty, Value: &av.Value{Type: av.KeyTypeText, Text: &av.ValueText{Content: "x"}}}}
				a.GetValue("text", settingsBlock).Text.Content = "Alpha"
				a.GetValue("text", cellSecond).Text.Content = "Zulu"
				a.Views[0].Sorts = []*av.ViewSort{{Column: "text", Order: av.SortOrderDesc}}
				if scenario == "missing-previous" {
					prev = "absent"
				}
			}
			before := cellDigest(t, root, page)
			snapshot := mustDuplicateJSON(t, a)
			h.counts = map[string]int{}
			values, err := h.tx.attrViewAddingBlockDefaultValues(cellAV, viewID, group, prev, adding)
			if err != nil {
				t.Fatal(err)
			}
			switch scenario {
			case "no-rules", "stale-group-no-rules":
				if len(values) != 0 || h.counts["near"] != 0 || h.counts["preview-id"] != 0 {
					t.Fatal("early return changed")
				}
			case "requested", "current":
				if values["number"].Number.Content != 2 {
					t.Fatal("view selection fallback")
				}
			case "sorted-neighbor":
				if values["text"] == nil || values["text"].Text.Content != "Zulu" {
					t.Fatal("sorted first item lost", values)
				}
			case "missing-previous":
				if len(values) != 0 {
					t.Fatal("missing previous must not fallback", values)
				}
			default:
				if values["number"] == nil || values["number"].Number.Content != 1 || values["number"].BlockID != adding {
					t.Fatal("identity/default", values)
				}
			}
			assertPreviewReadOnly(t, h, a, snapshot, before)
			if scenario == "continuous" {
				a.Views[0].Filters[0].Value.Number.Content = 8
				snapshot = mustDuplicateJSON(t, a)
				second, err := h.tx.attrViewAddingBlockDefaultValues(cellAV, "table", "", "", "next")
				if err != nil || second["number"].Number.Content != 8 || second["number"].ID == values["number"].ID {
					t.Fatal("staged configuration not read", err)
				}
				assertPreviewReadOnly(t, h, a, snapshot, before)
			}
		})
	}
}

func TestAVPreviewFailureCancellationAndStandalone(t *testing.T) {
	m, store, root := rowSource(t)
	page := "1760000000000000002"
	l, err := m.Acquire(context.Background(), store, page)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(l.Release)
	for _, point := range []string{"read:" + cellAV, "preview-clone", "near", "template", "preview-clock", "preview-id"} {
		for _, prefix := range []string{"", "cancel-"} {
			if prefix == "" && (point == "preview-clock" || point == "preview-id") {
				continue
			}
			t.Run(prefix+point, func(t *testing.T) {
				h := previewHarness(t, l, root, page, "")
				a, v, _, group := orderGroup(t, h.cellHarness, av.LayoutTypeTable, "status")
				v.Filters = []*av.ViewFilter{{Column: "number", Operator: av.FilterOperatorIsEqual, Value: &av.Value{Type: av.KeyTypeNumber, Number: &av.ValueNumber{Content: 5, IsNotEmpty: true}}}}
				a.KeyValues = append(a.KeyValues, &av.KeyValues{Key: &av.Key{ID: "tpl", Type: av.KeyTypeTemplate, Template: ".action{.Text}"}})
				h.failure = prefix + point
				before := cellDigest(t, root, page)
				snapshot := mustDuplicateJSON(t, a)
				h.counts = map[string]int{}
				values, err := h.tx.attrViewAddingBlockDefaultValues(cellAV, "table", group, "", rowFirst)
				if err == nil || values != nil || h.counts[point] == 0 {
					t.Fatal("failure yielded partial result", err, values, h.counts)
				}
				if prefix != "" && !errors.Is(err, context.Canceled) {
					t.Fatal(err)
				}
				assertPreviewReadOnly(t, h, a, snapshot, before)
			})
		}
	}
	h := previewHarness(t, l, root, page, "")
	h.env.avPreview = nil
	if values, err := h.tx.attrViewAddingBlockDefaultValues(cellAV, "", "", "", ""); err == nil || values != nil || len(h.counts) != 0 {
		t.Fatal("missing adapter fell back")
	}
	h.tx.DoOperations = []*Operation{{Action: "getAttrViewAddingBlockDefaultValues", AvID: cellAV}}
	if ret := performTx(h.tx); ret == nil || !strings.Contains(ret.msg, "not_contextualized") {
		t.Fatal("preview became transaction action", ret)
	}
	if err := legacyAttributeViewPreviewEnvironment().validate(); err != nil {
		t.Fatal(err)
	}
	if values := GetAttrViewAddingBlockDefaultValues("", "", "", "", ""); !reflect.DeepEqual(values, map[string]*av.Value{}) {
		t.Fatal("legacy error shape", values)
	}
}
