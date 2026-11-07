// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package avrender

import (
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"slices"
	av "wiki-kernel/internal/avmodel"
)

func Kanban(attrView *av.AttributeView, view *av.View, query string, env Environment) (ret *av.Kanban, err error) {
	if err = env.validate(); err != nil {
		return nil, err
	}
	if env.Cover.Parser == nil || env.Trees == nil {
		return nil, ErrEnvironmentUnbound
	}
	viewable := attrView.RenderedViewables[view.ID]
	if nil != viewable {
		ret = viewable.(*av.Kanban)
		return
	}

	ret = &av.Kanban{
		BaseInstance:           av.NewViewBaseInstance(view),
		CoverFrom:              view.Kanban.CoverFrom,
		CoverFromAssetKeyID:    view.Kanban.CoverFromAssetKeyID,
		CardAspectRatio:        view.Kanban.CardAspectRatio,
		CardSize:               view.Kanban.CardSize,
		FitImage:               view.Kanban.FitImage,
		DisplayFieldName:       view.Kanban.DisplayFieldName,
		FillColBackgroundColor: view.Kanban.FillColBackgroundColor,
		Fields:                 []*av.KanbanField{},
		Cards:                  []*av.KanbanCard{},
	}

	// 组装字段
	// Repair callbacks can remove fields from the live slice.
	for _, field := range slices.Clone(view.Kanban.Fields) {
		key, getErr := attrView.GetKey(field.ID)
		if nil != getErr {
			// 找不到字段则在视图中删除
			if err = env.RemoveMissingField(attrView, view, field.ID); err != nil {
				return nil, err
			}
			continue
		}

		ret.Fields = append(ret.Fields, &av.KanbanField{
			BaseInstanceField: &av.BaseInstanceField{
				ID:           key.ID,
				Name:         key.Name,
				Type:         key.Type,
				Icon:         key.Icon,
				Wrap:         field.Wrap,
				Hidden:       field.Hidden,
				Desc:         key.Desc,
				Calc:         field.Calc,
				Options:      key.Options,
				NumberFormat: key.NumberFormat,
				Template:     key.Template,
				Relation:     key.Relation,
				Rollup:       key.Rollup,
				Date:         key.Date,
				Created:      key.Created,
				Updated:      key.Updated,
			},
		})
	}

	cardsValues := GenerateItems(attrView, view) // 生成卡片
	if err = env.FilterItems(cardsValues); err != nil {
		return nil, err
	} // 过滤掉不存在的卡片

	// 批量加载绑定块对应的树
	var ialIDs []string
	for _, keyValues := range cardsValues {
		for _, kValues := range keyValues {
			blockVal := kValues.GetBlockValue()
			if nil != blockVal && !blockVal.IsDetached {
				ialIDs = append(ialIDs, blockVal.Block.ID)
			}
		}
	}
	boundTrees, err := env.Trees(ialIDs)
	if err != nil {
		return nil, err
	}

	// 生成卡片字段值
	for cardID, cardValues := range cardsValues {
		var kanbanCard av.KanbanCard
		for _, field := range ret.Fields {
			var fieldValue *av.KanbanFieldValue
			for _, keyValues := range cardValues {
				if keyValues.Key.ID == field.ID {
					fieldValue = &av.KanbanFieldValue{
						BaseValue: &av.BaseValue{
							ID:        keyValues.Values[0].ID,
							Value:     keyValues.Values[0],
							ValueType: field.Type,
						},
					}
					break
				}
			}
			if nil == fieldValue {
				fieldValue = &av.KanbanFieldValue{
					BaseValue: &av.BaseValue{
						ID:        cardID[:14] + ast.NewNodeID()[14:],
						ValueType: field.Type,
					},
				}
			}
			kanbanCard.ID = cardID

			filedDateIsTime := false
			if nil != field.Date {
				filedDateIsTime = field.Date.FillSpecificTime
			}
			FillBaseValue(fieldValue.BaseValue, field.ID, cardID, field.NumberFormat, field.Template, filedDateIsTime)
			kanbanCard.Values = append(kanbanCard.Values, fieldValue)
		}

		if err = fillAttributeViewKanbanCardCover(attrView, view, cardValues, &kanbanCard, cardID, env.Cover, boundTrees); err != nil {
			return nil, err
		}
		ret.Cards = append(ret.Cards, &kanbanCard)
	}

	// 回填补全数据
	FillKeyValues(attrView, ret)

	// 批量获取块属性以提升性能
	ials, err := env.BlockAttrs(ialIDs)
	if err != nil {
		return nil, err
	}

	// 渲染自动生成的字段值，比如关联、汇总、创建时间和更新时间
	if err = env.AutoValues(attrView, ret, ials); err != nil {
		return nil, err
	}

	// 最后渲染模板字段，这样模板就可以使用汇总、关联、创建时间和更新时间的值了
	if err = env.Templates(attrView, view, ret, ials); err != nil {
		return nil, err
	}

	FilterByQuery(query, ret)
	ManualSort(view, ret)
	return
}

func fillAttributeViewKanbanCardCover(attrView *av.AttributeView, view *av.View, cardValues []*av.KeyValues, kanbanCard *av.KanbanCard, cardID string, services CoverServices, trees map[string]*parse.Tree) error {
	switch view.Kanban.CoverFrom {
	case av.CoverFromNone:
	case av.CoverFromContentImage:
		blockValue := av.GetKeyBlockValue(cardValues)
		if blockValue.IsDetached {
			break
		}

		tree := trees[blockValue.Block.ID]
		if nil == tree {
			break
		}
		node := nodeInTree(tree, blockValue.Block.ID)
		if nil == node {
			break
		}

		if ast.NodeDocument == node.Type {
			if titleImg := docTitleImage(node); "" != titleImg {
				kanbanCard.CoverURL = titleImg
				break
			}

			if titleImgCSS := node.IALAttr("title-img"); "" != titleImgCSS {
				kanbanCard.CoverURL = titleImgCSS
				break
			}
		}

		ast.Walk(node, func(n *ast.Node, entering bool) ast.WalkStatus {
			if !entering {
				return ast.WalkContinue
			}

			if ast.NodeImage != n.Type {
				return ast.WalkContinue
			}

			dest := n.ChildByType(ast.NodeLinkDest)
			if nil == dest {
				return ast.WalkContinue
			}
			kanbanCard.CoverURL = dest.TokensStr()
			return ast.WalkStop
		})

		if "" == kanbanCard.CoverURL {
			kanbanCard.CoverContent = renderCoverContentBlock(node, services)

			return nil
		}
	case av.CoverFromAssetField:
		if "" == view.Kanban.CoverFromAssetKeyID {
			break
		}

		assetValue := attrView.GetValue(view.Kanban.CoverFromAssetKeyID, cardID)
		if nil == assetValue || 1 > len(assetValue.MAsset) {
			break
		}

		p := assetValue.MAsset[0].Content
		if services.IsImage == nil {
			return ErrEnvironmentUnbound
		}
		isImage, err := services.IsImage(p)
		if err != nil {
			return err
		}
		if isImage {
			kanbanCard.CoverURL = p
		}

		return nil
	case av.CoverFromContentBlock:
		blockValue := av.GetKeyBlockValue(cardValues)
		if blockValue.IsDetached {
			break
		}

		tree := trees[blockValue.Block.ID]
		if nil == tree {
			break
		}
		node := nodeInTree(tree, blockValue.Block.ID)
		if nil == node {
			break
		}
		kanbanCard.CoverContent = renderCoverContentBlock(node, services)
	}
	return nil
}
