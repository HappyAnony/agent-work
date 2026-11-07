// SPDX-License-Identifier: AGPL-3.0-or-later
package avrender

import (
	"context"
	"errors"
	av "wiki-kernel/internal/avmodel"
)

// ProjectTable preserves the original image export's ID-only card fields;
// gallery/kanban Hidden flags deliberately do not become table Hidden flags.
func ProjectTable(view *av.View) {
	switch view.LayoutType {
	case av.LayoutTypeGallery:
		view.Table = av.NewLayoutTable()
		for _, f := range view.Gallery.CardFields {
			view.Table.Columns = append(view.Table.Columns, &av.ViewTableColumn{BaseField: &av.BaseField{ID: f.ID}})
		}
	case av.LayoutTypeKanban:
		view.Table = av.NewLayoutTable()
		for _, f := range view.Kanban.Fields {
			view.Table.Columns = append(view.Table.Columns, &av.ViewTableColumn{BaseField: &av.BaseField{ID: f.ID}})
		}
	}
}

func Images(ctx context.Context, table *av.Table) (images []string, err error) {
	if table == nil {
		return nil, ErrShape
	}
	hidden := map[string]bool{}
	for _, column := range table.Columns {
		if column == nil || column.BaseInstanceField == nil {
			return nil, ErrShape
		}
		hidden[column.ID] = column.Hidden
	}
	for _, row := range table.Rows {
		if e := ctx.Err(); e != nil {
			return nil, e
		}
		if row == nil {
			return nil, ErrShape
		}
		for _, cell := range row.Cells {
			if cell == nil || cell.BaseValue == nil {
				return nil, ErrShape
			}
			if cell.Value == nil || cell.Value.Type != av.KeyTypeMAsset || hidden[cell.Value.KeyID] {
				continue
			}
			for _, asset := range cell.Value.MAsset {
				if asset == nil {
					return nil, errors.New("invalid_av_images_asset")
				}
				if asset.Type == av.AssetTypeImage {
					images = append(images, asset.Content)
				}
			}
		}
	}
	return images, ctx.Err()
}

// BlockValues is the inherited attribute-panel row projection; the caller
// chooses admitted mirrors, renders computed values, and owns repair policy.
func BlockValues(a *av.AttributeView, row string) []*av.KeyValues {
	values := []*av.KeyValues{}
	for _, kv := range a.KeyValues {
		if kv.Key.Type == av.KeyTypeLineNumber {
			continue
		}
		out := &av.KeyValues{Key: kv.Key}
		for _, v := range kv.Values {
			if v.BlockID == row {
				out.Values = append(out.Values, v)
			}
		}
		values = append(values, out)
	}
	return values
}
