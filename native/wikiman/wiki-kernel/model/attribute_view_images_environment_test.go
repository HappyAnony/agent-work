package model

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"

	"wiki-kernel/av"
	"wiki-kernel/internal/avmodel"
	"wiki-kernel/internal/avrender"
)

func imageQueryHarness(t *testing.T, h *boundHarness, layout av.LayoutType) *av.AttributeView {
	t.Helper()
	a, err := h.cell.read(cellAV)
	if err != nil {
		t.Fatal(err)
	}
	primary := a.GetBlockKeyValues()
	primary.Values = primary.Values[:2]
	for i, v := range primary.Values {
		v.IsDetached = true
		v.Block.Content = []string{"first image row", "second image row"}[i]
	}
	number, _ := a.GetKeyValues("number")
	number.Values = number.Values[:2]
	for i, v := range number.Values {
		v.Number.Content = float64(i + 1)
		v.Number.IsNotEmpty = true
		v.UpdatedAt = 1
	}
	assets := &av.KeyValues{Key: &av.Key{ID: "images", Type: av.KeyTypeMAsset}}
	secret := &av.KeyValues{Key: &av.Key{ID: "secret", Type: av.KeyTypeMAsset}}
	for i, id := range []string{settingsBlock, cellSecond} {
		assets.Values = append(assets.Values, &av.Value{ID: id + "-images", KeyID: "images", BlockID: id, Type: av.KeyTypeMAsset, MAsset: []*av.ValueAsset{{Type: av.AssetTypeImage, Content: h.page + []string{"/first.png", "/second.png"}[i]}, {Type: av.AssetTypeFile, Content: "assets/not-an-image.pdf"}, {Type: av.AssetTypeImage, Content: h.page + "/duplicate.png"}}})
		secret.Values = append(secret.Values, &av.Value{ID: id + "-secret", KeyID: "secret", BlockID: id, Type: av.KeyTypeMAsset, MAsset: []*av.ValueAsset{{Type: av.AssetTypeImage, Content: h.page + "/secret.png"}}})
	}
	a.KeyValues = []*av.KeyValues{primary, number, assets, secret}
	v := &av.View{ID: "images", LayoutType: layout, PageSize: 1, Sorts: []*av.ViewSort{{Column: "number", Order: av.SortOrderDesc}}}
	switch layout {
	case av.LayoutTypeTable:
		v.Table = av.NewLayoutTable()
		for _, id := range []string{"primary", "number", "images", "secret"} {
			v.Table.Columns = append(v.Table.Columns, &av.ViewTableColumn{BaseField: &av.BaseField{ID: id, Hidden: id == "secret"}})
		}
	case av.LayoutTypeGallery:
		v.Gallery = av.NewLayoutGallery()
		for _, id := range []string{"primary", "number", "images", "secret"} {
			v.Gallery.CardFields = append(v.Gallery.CardFields, &av.ViewGalleryCardField{BaseField: &av.BaseField{ID: id, Hidden: id == "secret"}})
		}
	case av.LayoutTypeKanban:
		v.Kanban = av.NewLayoutKanban()
		for _, id := range []string{"primary", "number", "images", "secret"} {
			v.Kanban.Fields = append(v.Kanban.Fields, &av.ViewKanbanField{BaseField: &av.BaseField{ID: id, Hidden: id == "secret"}})
		}
	}
	a.Views = []*av.View{v}
	a.ViewID = v.ID
	h.env.avImages = &attributeViewImagesEnvironment{ctx: h.ctx, read: h.cell.read, clone: func(a *av.AttributeView) (*av.AttributeView, error) {
		if err := h.mark("images-clone"); err != nil {
			return nil, err
		}
		return cloneAttributeViewPreview(a)
	},
		further: func(a *av.AttributeView, c map[string]*av.AttributeView) (map[string]av.Collection, error) {
			if err := h.mark("images-further"); err != nil {
				return nil, err
			}
			c["owner-marker"] = a
			return avrender.FurtherCollections(h.ctx, a, c, avrender.FurtherEnvironment{Read: func(id string) (*av.AttributeView, error) {
				source, err := h.cell.read(id)
				if err != nil {
					return nil, err
				}
				return cloneAttributeViewPreview(source)
			}, Render: func(a *av.AttributeView, v *av.View, q string) (av.Viewable, error) {
				return avrender.Table(a, v, q, cellRenderEnvironment(h.cellHarness))
			}})
		},
		render: func(a *av.AttributeView, v *av.View, q string) (*av.Table, error) {
			if err := h.mark("images-render"); err != nil {
				return nil, err
			}
			return avrender.Table(a, v, q, cellRenderEnvironment(h.cellHarness))
		},
		filter: func(table *av.Table, a *av.AttributeView, f map[string]av.Collection, c map[string]*av.AttributeView) error {
			if err := h.mark("images-filter"); err != nil {
				return err
			}
			if c["owner-marker"] != a {
				return errors.New("lost owner cache")
			}
			avmodel.Filter(table, a, f, c)
			return nil
		},
		sort: func(table *av.Table, a *av.AttributeView) error {
			if err := h.mark("images-sort"); err != nil {
				return err
			}
			avmodel.Sort(table, a)
			return nil
		},
	}
	return a
}

func TestAVImagesOwnersThreeLayoutsOriginalOrderAndProjection(t *testing.T) {
	m, store, root := nameSource(t)
	for _, page := range []string{"1760000000000000002", "1760000000000000003"} {
		t.Run(page, func(t *testing.T) {
			t.Parallel()
			l, err := m.Acquire(context.Background(), store, page)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(l.Release)
			for _, layout := range []av.LayoutType{av.LayoutTypeTable, av.LayoutTypeGallery, av.LayoutTypeKanban} {
				h := newBoundHarness(t, l, root, page, "")
				before := nameDigest(t, root, page)
				a := imageQueryHarness(t, h, layout)
				original := mustDuplicateJSON(t, a)
				images, err := h.tx.currentAttributeViewImages(cellAV, "", "")
				if err != nil {
					t.Fatal(err, h.counts)
				}
				want := []string{page + "/second.png", page + "/duplicate.png"}
				if layout != av.LayoutTypeTable {
					want = append(want, page+"/secret.png")
				}
				want = append(want, page+"/first.png", page+"/duplicate.png")
				if layout != av.LayoutTypeTable {
					want = append(want, page+"/secret.png")
				}
				if !slices.Equal(images, want) {
					t.Fatal(layout, images, want)
				}
				if !reflect.DeepEqual(original, mustDuplicateJSON(t, a)) || h.counts["finish"] != 0 || len(h.staged) != 0 || len(h.notifications) != 0 {
					t.Fatal("query changed source/candidate state", h.counts)
				}
				assertNameSource(t, h.cellHarness, before)
			}
		})
	}
}

func TestAVImagesActualQueryFilterViewFallbackAndEmptyResults(t *testing.T) {
	m, store, root := nameSource(t)
	page := "1760000000000000002"
	l, err := m.Acquire(context.Background(), store, page)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(l.Release)
	for _, scenario := range []string{"query", "filter", "stale-explicit", "explicit", "no-match", "no-images", "hidden", "duplicate"} {
		h := newBoundHarness(t, l, root, page, "")
		before := nameDigest(t, root, page)
		a := imageQueryHarness(t, h, av.LayoutTypeTable)
		q, view := "", ""
		want := []string{page + "/second.png", page + "/duplicate.png", page + "/first.png", page + "/duplicate.png"}
		switch scenario {
		case "query":
			q = "second"
			want = want[:2]
		case "filter":
			a.Views[0].Filters = []*av.ViewFilter{{Column: "number", Operator: av.FilterOperatorIsGreater, Value: &av.Value{Type: av.KeyTypeNumber, Number: &av.ValueNumber{Content: 1, IsNotEmpty: true}}}}
			want = want[:2]
		case "stale-explicit":
			view = "not-found"
		case "explicit":
			copy := *a.Views[0]
			copy.ID = "other"
			copy.Sorts = nil
			copy.ItemIDs = []string{settingsBlock, cellSecond}
			a.Views = append(a.Views, &copy)
			view = copy.ID
			want = []string{page + "/first.png", page + "/duplicate.png", page + "/second.png", page + "/duplicate.png"}
		case "no-match":
			q = "absent row"
			want = nil
		case "no-images":
			a.KeyValues[2].Values = nil
			want = nil
		case "hidden":
			a.Views[0].Table.Columns[2].Hidden = true
			want = nil
		case "duplicate":
			a.KeyValues[2].Values[0].MAsset = append(a.KeyValues[2].Values[0].MAsset, &av.ValueAsset{Type: av.AssetTypeImage, Content: page + "/first.png"})
			want = append(want, page+"/first.png")
		}
		original := mustDuplicateJSON(t, a)
		images, err := h.tx.currentAttributeViewImages(cellAV, view, q)
		if err != nil || !slices.Equal(images, want) {
			t.Fatal(scenario, images, want, err)
		}
		if len(want) == 0 && images != nil {
			t.Fatal("original nil result shape changed")
		}
		if !reflect.DeepEqual(original, mustDuplicateJSON(t, a)) {
			t.Fatal("source mutated")
		}
		assertNameSource(t, h.cellHarness, before)
	}
}

func TestAVImagesFailuresCancelAndNoPartialResults(t *testing.T) {
	m, store, root := nameSource(t)
	page := "1760000000000000002"
	l, err := m.Acquire(context.Background(), store, page)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(l.Release)
	for _, point := range []string{"read:" + cellAV, "images-clone", "images-further", "images-render", "images-filter", "images-sort"} {
		for _, prefix := range []string{"", "cancel-"} {
			t.Run(prefix+point, func(t *testing.T) {
				h := newBoundHarness(t, l, root, page, "")
				before := nameDigest(t, root, page)
				imageQueryHarness(t, h, av.LayoutTypeTable)
				h.failure = prefix + point
				images, err := h.tx.currentAttributeViewImages(cellAV, "", "")
				if err == nil || images != nil || h.counts[point] == 0 || h.counts["finish"] != 0 {
					t.Fatal("failure lost", images, err, h.counts)
				}
				assertNameSource(t, h.cellHarness, before)
			})
		}
	}
}

func TestAVImagesDependenciesShapesAndNoFallback(t *testing.T) {
	m, store, root := nameSource(t)
	page := "1760000000000000002"
	l, err := m.Acquire(context.Background(), store, page)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(l.Release)
	for _, scenario := range []string{"env", "ctx", "read", "clone", "further", "render", "filter", "sort", "cancel", "id", "nil-source", "wrong-source", "nil-key", "nil-value", "nil-asset", "nil-view", "nil-layout", "nil-filter", "nil-sort", "missing-current", "no-view", "clone-alias", "clone-nil", "nil-table", "nil-column", "nil-row", "nil-cell"} {
		t.Run(scenario, func(t *testing.T) {
			h := newBoundHarness(t, l, root, page, "")
			before := nameDigest(t, root, page)
			a := imageQueryHarness(t, h, av.LayoutTypeTable)
			e := h.env.avImages
			id := cellAV
			switch scenario {
			case "env":
				h.env.avImages = nil
			case "ctx":
				h.env.ctx = nil
			case "read":
				e.read = nil
			case "clone":
				e.clone = nil
			case "further":
				e.further = nil
			case "render":
				e.render = nil
			case "filter":
				e.filter = nil
			case "sort":
				e.sort = nil
			case "cancel":
				h.cancel()
			case "id":
				id = ""
			case "nil-source":
				e.read = func(string) (*av.AttributeView, error) { return nil, nil }
			case "wrong-source":
				a.ID = cellDest
			case "nil-key":
				a.KeyValues[0].Key = nil
			case "nil-value":
				a.KeyValues[0].Values[0] = nil
			case "nil-asset":
				a.KeyValues[2].Values[0].MAsset[0] = nil
			case "nil-view":
				a.Views[0] = nil
			case "nil-layout":
				a.Views[0].Table = nil
			case "nil-filter":
				a.Views[0].Filters = []*av.ViewFilter{nil}
			case "nil-sort":
				a.Views[0].Sorts = []*av.ViewSort{nil}
			case "missing-current":
				a.ViewID = "absent"
			case "no-view":
				a.Views = nil
			case "clone-alias":
				e.clone = func(a *av.AttributeView) (*av.AttributeView, error) { return a, nil }
			case "clone-nil":
				e.clone = func(*av.AttributeView) (*av.AttributeView, error) { return nil, nil }
			default:
				e.render = func(*av.AttributeView, *av.View, string) (*av.Table, error) {
					switch scenario {
					case "nil-table":
						return nil, nil
					case "nil-column":
						return &av.Table{Columns: []*av.TableColumn{nil}}, nil
					case "nil-row":
						return &av.Table{Rows: []*av.TableRow{nil}}, nil
					default:
						return &av.Table{Rows: []*av.TableRow{{Cells: []*av.TableCell{nil}}}}, nil
					}
				}
			}
			images, err := h.tx.currentAttributeViewImages(id, "", "")
			if err == nil || images != nil || h.counts["finish"] != 0 {
				t.Fatal("invalid dependency/source accepted", scenario, images, err)
			}
			assertNameSource(t, h.cellHarness, before)
		})
	}
}

func TestAVImagesLateServiceCancellationAndOriginalExportProjection(t *testing.T) {
	m, store, root := nameSource(t)
	page := "1760000000000000002"
	l, err := m.Acquire(context.Background(), store, page)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(l.Release)
	for _, point := range []string{"read", "clone", "further", "render", "filter", "sort"} {
		h := newBoundHarness(t, l, root, page, "")
		before := nameDigest(t, root, page)
		imageQueryHarness(t, h, av.LayoutTypeGallery)
		e := h.env.avImages
		switch point {
		case "read":
			f := e.read
			e.read = func(id string) (*av.AttributeView, error) { a, err := f(id); h.cancel(); return a, err }
		case "clone":
			f := e.clone
			e.clone = func(a *av.AttributeView) (*av.AttributeView, error) { b, err := f(a); h.cancel(); return b, err }
		case "further":
			f := e.further
			e.further = func(a *av.AttributeView, c map[string]*av.AttributeView) (map[string]av.Collection, error) {
				b, err := f(a, c)
				h.cancel()
				return b, err
			}
		case "render":
			f := e.render
			e.render = func(a *av.AttributeView, v *av.View, q string) (*av.Table, error) {
				b, err := f(a, v, q)
				h.cancel()
				return b, err
			}
		case "filter":
			f := e.filter
			e.filter = func(t *av.Table, a *av.AttributeView, b map[string]av.Collection, c map[string]*av.AttributeView) error {
				err := f(t, a, b, c)
				h.cancel()
				return err
			}
		case "sort":
			f := e.sort
			e.sort = func(t *av.Table, a *av.AttributeView) error { err := f(t, a); h.cancel(); return err }
		}
		images, err := h.tx.currentAttributeViewImages(cellAV, "", "")
		if images != nil || !errors.Is(err, context.Canceled) {
			t.Fatal("late cancel lost", point, err)
		}
		assertNameSource(t, h.cellHarness, before)
	}
}
