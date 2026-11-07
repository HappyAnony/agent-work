// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package avrender

import (
	"github.com/jinzhu/copier"
	av "wiki-kernel/internal/avmodel"
)

func PrepareGroup(view, groupView *av.View) error {
	var err error
	switch groupView.LayoutType {
	case av.LayoutTypeTable:
		// 这里需要使用深拷贝，因为字段上可能会带有计算（FieldCalc），每个分组视图的计算结果都需要分别存储在不同的字段实例上
		err = copier.CopyWithOption(&groupView.Table.Columns, &view.Table.Columns, copier.Option{DeepCopy: true})
		groupView.Table.ShowIcon = view.Table.ShowIcon
		groupView.Table.WrapField = view.Table.WrapField
	case av.LayoutTypeGallery:
		err = copier.CopyWithOption(&groupView.Gallery.CardFields, &view.Gallery.CardFields, copier.Option{DeepCopy: true})
		groupView.Gallery.ShowIcon = view.Gallery.ShowIcon
		groupView.Gallery.WrapField = view.Gallery.WrapField

		groupView.Gallery.CoverFrom = view.Gallery.CoverFrom
		groupView.Gallery.CoverFromAssetKeyID = view.Gallery.CoverFromAssetKeyID
		groupView.Gallery.CardAspectRatio = view.Gallery.CardAspectRatio
		groupView.Gallery.CardSize = view.Gallery.CardSize
		groupView.Gallery.FitImage = view.Gallery.FitImage
		groupView.Gallery.DisplayFieldName = view.Gallery.DisplayFieldName
	case av.LayoutTypeKanban:
		err = copier.CopyWithOption(&groupView.Kanban.Fields, &view.Kanban.Fields, copier.Option{DeepCopy: true})
		groupView.Kanban.ShowIcon = view.Kanban.ShowIcon
		groupView.Kanban.WrapField = view.Kanban.WrapField

		groupView.Kanban.CoverFrom = view.Kanban.CoverFrom
		groupView.Kanban.CoverFromAssetKeyID = view.Kanban.CoverFromAssetKeyID
		groupView.Kanban.CardAspectRatio = view.Kanban.CardAspectRatio
		groupView.Kanban.CardSize = view.Kanban.CardSize
		groupView.Kanban.FitImage = view.Kanban.FitImage
		groupView.Kanban.DisplayFieldName = view.Kanban.DisplayFieldName
		groupView.Kanban.FillColBackgroundColor = view.Kanban.FillColBackgroundColor
	}
	if err != nil {
		return err
	}

	groupView.Filters = view.Filters
	groupView.Sorts = view.Sorts
	return nil
}
