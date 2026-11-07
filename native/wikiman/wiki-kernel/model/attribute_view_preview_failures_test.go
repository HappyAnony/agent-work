package model

import (
	"context"
	"errors"
	"math"
	"testing"

	"wiki-kernel/av"
)

func TestAVPreviewInvalidInputsAndNoPartialResults(t *testing.T) {
	m, store, root := rowSource(t)
	page := "1760000000000000002"
	l, err := m.Acquire(context.Background(), store, page)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(l.Release)
	for _, scenario := range []string{"empty-id", "nil-source", "wrong-source", "nil-key", "nil-value", "no-views", "nil-view", "nil-layout", "nil-base", "nil-field", "nil-group", "nil-filter", "bad-filter", "bad-date-filter", "nil-sort", "missing-group", "missing-group-key", "nil-group-value", "nil-option", "nil-clone", "alias-clone", "wrong-clone", "bad-clone", "nan-clone", "nil-template-key", "empty-generated-id", "repeated-generated-id", "pre-cancel", "clone-late-cancel", "near-late-cancel", "template-late-cancel", "nil-calculated-value"} {
		t.Run(scenario, func(t *testing.T) {
			h := previewHarness(t, l, root, page, "")
			a, v, _, group := orderGroup(t, h.cellHarness, av.LayoutTypeTable, "status")
			v.Filters = []*av.ViewFilter{{Column: "number", Operator: av.FilterOperatorIsEqual, Value: &av.Value{Type: av.KeyTypeNumber, Number: &av.ValueNumber{Content: 7, IsNotEmpty: true}}}}
			id := cellAV
			e := h.env.avPreview
			switch scenario {
			case "empty-id":
				id = ""
			case "nil-source":
				e.read = func(string) (*av.AttributeView, error) { return nil, nil }
			case "wrong-source":
				a.ID = cellDest
			case "nil-key":
				a.KeyValues[0].Key = nil
			case "nil-value":
				a.KeyValues[0].Values[0] = nil
			case "no-views":
				a.Views = nil
			case "nil-view":
				a.Views = append(a.Views, nil)
			case "nil-layout":
				v.Table = nil
			case "nil-base":
				v.Table.BaseLayout = nil
			case "nil-field":
				v.Table.Columns = append(v.Table.Columns, nil)
			case "nil-group":
				v.Groups = append(v.Groups, nil)
			case "nil-filter":
				v.Filters = append(v.Filters, nil)
			case "bad-filter":
				v.Filters[0].Value = &av.Value{Type: av.KeyTypeText}
			case "bad-date-filter":
				v.Filters[0].Value = &av.Value{Type: av.KeyTypeDate}
			case "nil-sort":
				v.Sorts = []*av.ViewSort{nil}
			case "missing-group":
				group = "absent"
			case "missing-group-key":
				v.Group.Field = "absent"
			case "nil-group-value":
				v.GetGroupByID(group).GroupVal = nil
			case "nil-option":
				a.KeyValues[4].Key.Options = append(a.KeyValues[4].Key.Options, nil)
			case "nil-clone":
				e.clone = func(*av.AttributeView) (*av.AttributeView, error) { return nil, nil }
			case "alias-clone":
				e.clone = func(a *av.AttributeView) (*av.AttributeView, error) { return a, nil }
			case "wrong-clone", "bad-clone":
				clone := e.clone
				e.clone = func(a *av.AttributeView) (*av.AttributeView, error) {
					copy, err := clone(a)
					if err == nil {
						if scenario == "wrong-clone" {
							copy.ID = "wrong"
						} else {
							copy.Views = append(copy.Views, nil)
						}
					}
					return copy, err
				}
			case "nan-clone":
				e.clone = func(a *av.AttributeView) (*av.AttributeView, error) {
					copy, err := cloneAttributeViewPreview(a)
					if err != nil {
						return nil, err
					}
					copy.KeyValues[2].Values[0].Number.Content = math.NaN()
					return cloneAttributeViewPreview(copy)
				}
			case "nil-template-key", "template-late-cancel", "nil-calculated-value":
				a.KeyValues = append(a.KeyValues, &av.KeyValues{Key: &av.Key{ID: "tpl", Type: av.KeyTypeTemplate, Template: ".action{.Text}"}})
				v.Filters = append(v.Filters, &av.ViewFilter{Column: "tpl", Operator: av.FilterOperatorContains, Value: &av.Value{Type: av.KeyTypeText, Text: &av.ValueText{Content: "old"}}})
				if scenario == "nil-template-key" {
					e.templateKeys = func(*av.AttributeView, *av.Key) ([]*av.Key, error) { return []*av.Key{nil}, nil }
				}
				if scenario == "template-late-cancel" {
					lookup := e.templateKeys
					e.templateKeys = func(a *av.AttributeView, k *av.Key) ([]*av.Key, error) {
						keys, err := lookup(a, k)
						h.cancel()
						return keys, err
					}
				}
				if scenario == "nil-calculated-value" {
					e.near = func(*av.AttributeView, *av.View, *av.View, string) (av.Item, error) {
						return &av.TableRow{ID: cellSecond}, nil
					}
					e.templateKeys = func(*av.AttributeView, *av.Key) ([]*av.Key, error) {
						return []*av.Key{{ID: "absent", Type: av.KeyTypeText}}, nil
					}
				}
			case "empty-generated-id":
				e.newID = func() string { return "" }
			case "repeated-generated-id":
				e.newID = func() string { return "same" }
			case "pre-cancel":
				h.cancel()
			case "clone-late-cancel":
				clone := e.clone
				e.clone = func(a *av.AttributeView) (*av.AttributeView, error) { a, err := clone(a); h.cancel(); return a, err }
			case "near-late-cancel":
				near := e.near
				e.near = func(a *av.AttributeView, v, g *av.View, p string) (av.Item, error) {
					item, err := near(a, v, g, p)
					h.cancel()
					return item, err
				}
			}
			before := cellDigest(t, root, page)
			snapshot := mustDuplicateJSON(t, a)
			h.counts = map[string]int{}
			values, err := h.tx.attrViewAddingBlockDefaultValues(id, "table", group, "", rowFirst)
			if err == nil || values != nil {
				t.Fatal("invalid preview produced partial result", scenario, err, values)
			}
			if scenario == "pre-cancel" && len(h.counts) != 0 {
				t.Fatal("cancelled preview read")
			}
			assertPreviewReadOnly(t, h, a, snapshot, before)
		})
	}
}

func TestAVPreviewDependencyGatesAndTransactionContext(t *testing.T) {
	m, store, root := rowSource(t)
	page := "1760000000000000002"
	l, err := m.Acquire(context.Background(), store, page)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(l.Release)
	for _, dep := range []string{"environment", "ctx", "read", "clone", "near", "template", "clock", "id"} {
		t.Run(dep, func(t *testing.T) {
			h := previewHarness(t, l, root, page, "")
			e := h.env.avPreview
			switch dep {
			case "environment":
				h.env.avPreview = nil
			case "ctx":
				h.env.ctx = nil
			case "read":
				e.read = nil
			case "clone":
				e.clone = nil
			case "near":
				e.near = nil
			case "template":
				e.templateKeys = nil
			case "clock":
				e.now = nil
			case "id":
				e.newID = nil
			}
			values, err := h.tx.attrViewAddingBlockDefaultValues(cellAV, "", "", "", "")
			if err == nil || values != nil || len(h.counts) != 0 {
				t.Fatal("missing dependency did work", dep, err, h.counts)
			}
		})
	}
	h := previewHarness(t, l, root, page, "")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	h.env.ctx = ctx
	if out, err := h.tx.attrViewAddingBlockDefaultValues(cellAV, "", "", "", ""); !errors.Is(err, context.Canceled) || out != nil || len(h.counts) != 0 {
		t.Fatal("preview ignored transaction ctx", err)
	}
}
