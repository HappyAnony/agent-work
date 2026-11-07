// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package avrender

import (
	"bytes"
	"fmt"
	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/html"
	"github.com/88250/lute/parse"
	"github.com/88250/lute/render"
	"net/url"
	"slices"
	"strings"
	av "wiki-kernel/internal/avmodel"
)

func Gallery(attrView *av.AttributeView, view *av.View, query string, env Environment) (ret *av.Gallery, err error) {
	if err = env.validate(); err != nil {
		return nil, err
	}
	if env.Cover.Parser == nil || env.Trees == nil {
		return nil, ErrEnvironmentUnbound
	}
	viewable := attrView.RenderedViewables[view.ID]
	if nil != viewable {
		ret = viewable.(*av.Gallery)
		return
	}

	ret = &av.Gallery{
		BaseInstance:        av.NewViewBaseInstance(view),
		CoverFrom:           view.Gallery.CoverFrom,
		CoverFromAssetKeyID: view.Gallery.CoverFromAssetKeyID,
		CardAspectRatio:     view.Gallery.CardAspectRatio,
		CardSize:            view.Gallery.CardSize,
		FitImage:            view.Gallery.FitImage,
		DisplayFieldName:    view.Gallery.DisplayFieldName,
		Fields:              []*av.GalleryField{},
		Cards:               []*av.GalleryCard{},
	}

	// 组装字段
	// Repair callbacks can remove fields from the live slice.
	for _, field := range slices.Clone(view.Gallery.CardFields) {
		key, getErr := attrView.GetKey(field.ID)
		if nil != getErr {
			// 找不到字段则在视图中删除
			if err = env.RemoveMissingField(attrView, view, field.ID); err != nil {
				return nil, err
			}
			continue
		}

		ret.Fields = append(ret.Fields, &av.GalleryField{
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
		var galleryCard av.GalleryCard
		for _, field := range ret.Fields {
			var fieldValue *av.GalleryFieldValue
			for _, keyValues := range cardValues {
				if keyValues.Key.ID == field.ID {
					fieldValue = &av.GalleryFieldValue{
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
				fieldValue = &av.GalleryFieldValue{
					BaseValue: &av.BaseValue{
						ID:        cardID[:14] + ast.NewNodeID()[14:],
						ValueType: field.Type,
					},
				}
			}
			galleryCard.ID = cardID

			filedDateIsTime := false
			if nil != field.Date {
				filedDateIsTime = field.Date.FillSpecificTime
			}
			FillBaseValue(fieldValue.BaseValue, field.ID, cardID, field.NumberFormat, field.Template, filedDateIsTime)
			galleryCard.Values = append(galleryCard.Values, fieldValue)
		}

		if err = fillAttributeViewGalleryCardCover(attrView, view, cardValues, &galleryCard, cardID, env.Cover, boundTrees); err != nil {
			return nil, err
		}
		ret.Cards = append(ret.Cards, &galleryCard)
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

func fillAttributeViewGalleryCardCover(attrView *av.AttributeView, view *av.View, cardValues []*av.KeyValues, galleryCard *av.GalleryCard, cardID string, services CoverServices, trees map[string]*parse.Tree) error {
	switch view.Gallery.CoverFrom {
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
				galleryCard.CoverURL = titleImg
				break
			}

			if titleImgCSS := node.IALAttr("title-img"); "" != titleImgCSS {
				galleryCard.CoverURL = titleImgCSS
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
			galleryCard.CoverURL = dest.TokensStr()
			return ast.WalkStop
		})

		if "" == galleryCard.CoverURL {
			galleryCard.CoverContent = renderCoverContentBlock(node, services)

			return nil
		}
	case av.CoverFromAssetField:
		if "" == view.Gallery.CoverFromAssetKeyID {
			break
		}

		assetValue := attrView.GetValue(view.Gallery.CoverFromAssetKeyID, cardID)
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
			galleryCard.CoverURL = p
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
		galleryCard.CoverContent = renderCoverContentBlock(node, services)
	}
	return nil
}
func renderCoverContentBlock(node *ast.Node, services CoverServices) string {
	luteEngine := services.Parser
	if luteEngine == nil {
		panic(ErrEnvironmentUnbound)
	}
	if node == nil {
		return ""
	}
	isDoc := ast.NodeDocument == node.Type
	if isDoc {
		node = node.FirstChild
	}
	if node == nil {
		return ""
	}
	heading := node
	isHeading := ast.NodeHeading == node.Type
	headingLevel := node.HeadingLevel
	if isHeading {
		node = node.Next
	}

	buf := bytes.Buffer{}
	for c := node; nil != c; c = c.Next {
		if isHeading && ast.NodeHeading == c.Type && c.HeadingLevel <= headingLevel {
			if 1 > buf.Len() {
				buf.WriteString(renderBlockDOMByNode(heading, luteEngine, services))
			}
			break
		}
		buf.WriteString(renderBlockDOMByNode(c, luteEngine, services))
		if (!isDoc && !isHeading) || 1024*4 < buf.Len() {
			break
		}
	}
	return buf.String()
}

func renderBlockDOMByNode(node *ast.Node, luteEngine *lute.Lute, services CoverServices) string {
	if node == nil {
		return ""
	}
	restore := snapshotCoverNodes(node)
	defer restore()
	tree := &parse.Tree{Root: &ast.Node{Type: ast.NodeDocument}, Context: &parse.Context{ParseOption: luteEngine.ParseOptions}}
	blockRenderer := render.NewProtyleRenderer(tree, luteEngine.RenderOptions)
	blockRenderer.Options.ProtyleContenteditable = false
	resetIDs := map[string]string{}
	ast.Walk(node, func(n *ast.Node, entering bool) ast.WalkStatus {
		if entering {
			if n.IsBlock() {
				// 内容图中不需要渲染数据库角标 https://github.com/siyuan-note/siyuan/issues/15057
				ial := parse.IAL2Map(n.KramdownIAL)
				delete(ial, av.NodeAttrNameAvs)

				// 重置 data-node-id 的值，避免触发前端绑定的事件 https://github.com/siyuan-note/siyuan/issues/15088
				newID := ast.NewNodeID()
				resetIDs[newID] = n.ID
				n.ID, ial["id"] = newID, newID
				n.KramdownIAL = parse.Map2IAL(ial)

				if ast.NodeIFrame == n.Type || ast.NodeAudio == n.Type || ast.NodeVideo == n.Type {
					// 禁止自动播放 Disable automatic video playback in database card view https://github.com/siyuan-note/siyuan/issues/15212
					dest := nodeSrc(n)
					oldDest := dest
					if (strings.HasPrefix(dest, "http://") || strings.HasPrefix(dest, "https://")) && !strings.Contains(dest, "autoplay") {
						dest = html.UnescapeHTMLStr(dest)
						destURL, err := url.Parse(dest)
						if nil == err {
							q := destURL.Query()
							q.Set("autoplay", "0")
							destURL.RawQuery = q.Encode()
							dest = destURL.String()
							dest = html.EscapeHTMLStr(dest)
							n.Tokens = bytes.ReplaceAll(n.Tokens, []byte(oldDest), []byte(dest))
						} else {
							if services.Warning != nil {
								services.Warning(fmt.Errorf("parse url [%s] failed: %w", dest, err))
							}
						}
					}
				}
			}
		}
		rendererFunc := blockRenderer.RendererFuncs[n.Type]
		return rendererFunc(n, entering)
	})
	ret := strings.TrimSpace(blockRenderer.Writer.String())
	if strings.HasPrefix(ret, "<li") {
		ret = "<ul>" + ret + "</ul>"
	}
	ast.Walk(node, func(n *ast.Node, entering bool) ast.WalkStatus {
		if entering {
			if n.IsBlock() {
				// 还原上面重置 data-node-id 的值
				ial := parse.IAL2Map(n.KramdownIAL)
				oldID := resetIDs[n.ID]
				n.ID, ial["id"] = oldID, oldID
				n.KramdownIAL = parse.Map2IAL(ial)
			}
		}
		return ast.WalkContinue
	})
	return ret
}
