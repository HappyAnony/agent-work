// SiYuan - Refactor your thinking
// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
// Extracted original sql/database.go projection; dependencies are explicit.
package indexdb

import (
	"bytes"
	"github.com/88250/gulu"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/editor"
	"github.com/88250/lute/html"
	"github.com/88250/lute/parse"
	"path"
	"strings"
	"unicode/utf8"
	"wiki-kernel/internal/nodetype"
)

func (projection *treeProjector) fromTree(node *ast.Node, tree *parse.Tree) (blocks []*BlockRecord, spans []*SpanRecord, assets []*AssetRecord, attributes []*AttributeRecord) {
	rootID := tree.Root.ID
	boxID := tree.Box
	p := tree.Path
	ast.Walk(node, func(n *ast.Node, entering bool) ast.WalkStatus {
		if projection.err != nil || projection.ctx.Err() != nil {
			return ast.WalkStop
		}
		if !entering {
			return ast.WalkContinue
		}

		// 构造行级元素
		spanBlocks, spanSpans, spanAssets, spanAttrs, walkStatus := projection.buildSpanFromNode(n, tree, rootID, boxID, p)
		if 0 < len(spanBlocks) {
			blocks = append(blocks, spanBlocks...)
		}
		if 0 < len(spanSpans) {
			spans = append(spans, spanSpans...)
		}
		if 0 < len(spanAssets) {
			assets = append(assets, spanAssets...)
		}
		if 0 < len(spanAttrs) {
			attributes = append(attributes, spanAttrs...)
		}

		// 构造属性
		attrs := buildAttributeFromNode(n, rootID, boxID, p)
		if 0 < len(attrs) {
			attributes = append(attributes, attrs...)
		}
		if -1 != walkStatus {
			return walkStatus
		}

		// 构造块级元素
		if "" == n.ID || !n.IsBlock() {
			return ast.WalkContinue
		}

		b, attrs := projection.buildBlockFromNode(n, tree)
		blocks = append(blocks, b)
		if 0 < len(attrs) {
			attributes = append(attributes, attrs...)
		}
		return ast.WalkContinue
	})
	return
}

func buildAttributeFromNode(n *ast.Node, rootID, boxID, p string) (attributes []*AttributeRecord) {
	switch n.Type {
	case ast.NodeKramdownSpanIAL:
		parentBlock := parentBlock(n)
		attrs := parse.IALValMap(n)
		for name, val := range attrs {
			if !isAttr(name) {
				continue
			}

			attr := &AttributeRecord{
				ID:      ast.NewNodeID(),
				Name:    name,
				Value:   val,
				Type:    "s",
				BlockID: parentBlock.ID,
				RootID:  rootID,
				Box:     boxID,
				Path:    p,
			}
			attributes = append(attributes, attr)
		}
	case ast.NodeKramdownBlockIAL:
		attrs := parse.IALValMap(n)
		for name, val := range attrs {
			if !isAttr(name) {
				continue
			}

			attr := &AttributeRecord{
				ID:      ast.NewNodeID(),
				Name:    name,
				Value:   val,
				Type:    "b",
				BlockID: n.ID,
				RootID:  rootID,
				Box:     boxID,
				Path:    p,
			}
			attributes = append(attributes, attr)
		}
	}
	return
}

func isAttr(name string) bool {
	return strings.HasPrefix(name, "custom-") || "name" == name || "alias" == name || "memo" == name || "bookmark" == name || "fold" == name || "heading-fold" == name || "style" == name
}

func (projection *treeProjector) buildSpanFromNode(n *ast.Node, tree *parse.Tree, rootID, boxID, p string) (blocks []*BlockRecord, spans []*SpanRecord, assets []*AssetRecord, attributes []*AttributeRecord, walkStatus ast.WalkStatus) {
	switch n.Type {
	case ast.NodeImage:
		text := n.Text()
		markdown := projection.markdown(n, false)
		parentBlock := parentBlock(n)
		span := &SpanRecord{
			ID:       ast.NewNodeID(),
			BlockID:  parentBlock.ID,
			RootID:   rootID,
			Box:      boxID,
			Path:     p,
			Content:  text,
			Markdown: markdown,
			Type:     nodetype.TypeAbbr(n.Type.String()),
			IAL:      ialString(n),
		}
		spans = append(spans, span)
		walkStatus = ast.WalkSkipChildren

		destNode := n.ChildByType(ast.NodeLinkDest)
		if nil == destNode {
			return
		}

		// assetsLinkDestsInTree

		if !isAssetDest(destNode.Tokens) {
			return
		}

		dest := gulu.Str.FromBytes(destNode.Tokens)
		var title string
		if titleNode := n.ChildByType(ast.NodeLinkTitle); nil != titleNode {
			title = gulu.Str.FromBytes(titleNode.Tokens)
		}

		hash := projection.assetHash(dest, boxID, p)
		name := path.Base(dest)
		asset := &AssetRecord{
			ID:      ast.NewNodeID(),
			BlockID: parentBlock.ID,
			RootID:  rootID,
			Box:     boxID,
			DocPath: p,
			Path:    dest,
			Name:    name,
			Title:   title,
			Hash:    hash,
		}
		assets = append(assets, asset)
		return
	case ast.NodeTextMark:
		typ := nodetype.TypeAbbr(n.Type.String()) + " " + n.TextMarkType
		text := strings.TrimSuffix(n.Content(), string(gulu.ZWJ))
		markdown := projection.markdown(n, false)
		markdown = strings.ReplaceAll(markdown, string(gulu.ZWJ)+"#", "#")
		parentBlock := parentBlock(n)
		span := &SpanRecord{
			ID:       ast.NewNodeID(),
			BlockID:  parentBlock.ID,
			RootID:   rootID,
			Box:      boxID,
			Path:     p,
			Content:  text,
			Markdown: markdown,
			Type:     typ,
			IAL:      ialString(n),
		}
		spans = append(spans, span)

		if n.IsTextMarkType("a") {
			dest := n.TextMarkAHref
			if isAssetDest([]byte(dest)) {
				var title string
				if titleNode := n.ChildByType(ast.NodeLinkTitle); nil != titleNode {
					title = gulu.Str.FromBytes(titleNode.Tokens)
				}

				hash := projection.assetHash(dest, boxID, p)
				name := path.Base(dest)
				asset := &AssetRecord{
					ID:      ast.NewNodeID(),
					BlockID: parentBlock.ID,
					RootID:  rootID,
					Box:     boxID,
					DocPath: p,
					Path:    dest,
					Name:    name,
					Title:   title,
					Hash:    hash,
				}
				assets = append(assets, asset)
			}
		}
		walkStatus = ast.WalkSkipChildren
		return
	case ast.NodeDocument:
		if asset := projection.titleAsset(n, boxID, p); nil != asset {
			assets = append(assets, asset)
		}
		if tags := docTags(n); 0 < len(tags) {
			spans = append(spans, tags...)
		}
	case ast.NodeInlineHTML, ast.NodeHTMLBlock, ast.NodeIFrame, ast.NodeWidget, ast.NodeAudio, ast.NodeVideo:
		nodes, err := html.ParseFragment(bytes.NewReader(n.Tokens), &html.Node{Type: html.ElementNode})
		if err != nil {
			projection.fail(err)
			walkStatus = ast.WalkContinue
			return
		}
		if 1 > len(nodes) &&
			ast.NodeHTMLBlock != n.Type { // HTML 块若内容为空时无法在数据库中查询到 https://github.com/siyuan-note/siyuan/issues/4691
			walkStatus = ast.WalkContinue
			return
		}

		if ast.NodeHTMLBlock == n.Type || ast.NodeIFrame == n.Type || ast.NodeWidget == n.Type || ast.NodeAudio == n.Type || ast.NodeVideo == n.Type {
			b, attrs := projection.buildBlockFromNode(n, tree)
			blocks = append(blocks, b)
			attributes = append(attributes, attrs...)
		}

		if ast.NodeInlineHTML == n.Type {
			// 没有行级 HTML，只有块级 HTML，这里转换为块
			n.ID = ast.NewNodeID()
			n.SetIALAttr("id", n.ID)
			n.SetIALAttr("updated", n.ID[:14])
			b, attrs := projection.buildBlockFromNode(n, tree)
			b.Type = ast.NodeHTMLBlock.String()
			blocks = append(blocks, b)
			attributes = append(attributes, attrs...)
			walkStatus = ast.WalkContinue
			projection.warn("inline HTML is converted to HTML block")
			return
		}

		if 1 > len(nodes) {
			walkStatus = ast.WalkContinue
			return
		}

		var src []byte
		for _, attr := range nodes[0].Attr {
			if "src" == attr.Key || strings.HasPrefix(attr.Key, "data-assets") || strings.HasPrefix(attr.Key, "custom-data-assets") {
				src = gulu.Str.ToBytes(attr.Val)
				break
			}
		}
		if 1 > len(src) {
			walkStatus = ast.WalkContinue
			return
		}

		if !isAssetDest(src) {
			walkStatus = ast.WalkContinue
			return
		}

		dest := string(src)
		hash := projection.assetHash(dest, boxID, p)
		parentBlock := parentBlock(n)
		if ast.NodeInlineHTML != n.Type {
			parentBlock = n
		}
		name := path.Base(dest)
		asset := &AssetRecord{
			ID:      ast.NewNodeID(),
			BlockID: parentBlock.ID,
			RootID:  rootID,
			Box:     boxID,
			DocPath: p,
			Path:    dest,
			Name:    name,
			Title:   "",
			Hash:    hash,
		}
		assets = append(assets, asset)
		walkStatus = ast.WalkSkipChildren
		return
	}
	walkStatus = -1
	return
}

func (projection *treeProjector) buildBlockFromNode(n *ast.Node, tree *parse.Tree) (block *BlockRecord, attributes []*AttributeRecord) {
	boxID := tree.Box
	p := tree.Path
	rootID := tree.Root.ID
	name := html.UnescapeString(n.IALAttr("name"))
	alias := html.UnescapeString(n.IALAttr("alias"))
	memo := html.UnescapeString(n.IALAttr("memo"))
	tag := tagFromNode(n)

	var content, fcontent, markdown, parentID string
	ialContent := ialString(n)
	hash := projection.nodeHash(n, tree)
	var length int
	if ast.NodeDocument == n.Type {
		content = n.IALAttr("title")
		fcontent = content
		length = utf8.RuneCountInString(fcontent)
	} else if n.IsContainerBlock() {
		markdown = projection.markdown(n, false)
		projection.ocr(n)
		content = projection.static(n, nil, true, projection.options.IndexAssetPath, true)

		fc := firstLeafBlock(n)
		projection.ocr(fc)
		fcontent = projection.static(fc, nil, true, false, true)

		parentID = physicalParent(n)
		if h := headingParent(n); nil != h { // 如果在标题块下方，则将标题块作为父节点
			parentID = h.ID
		}
		length = utf8.RuneCountInString(fcontent)
	} else {
		markdown = projection.markdown(n, false)
		projection.ocr(n)
		content = projection.static(n, nil, true, projection.options.IndexAssetPath, true)

		parentID = physicalParent(n)
		if h := headingParent(n); nil != h {
			parentID = h.ID
		}
		length = utf8.RuneCountInString(content)
	}

	// 剔除零宽空格 Database index content/markdown values no longer contain zero-width spaces https://github.com/siyuan-note/siyuan/issues/15204
	fcontent = strings.ReplaceAll(fcontent, editor.Zwsp, "")
	content = strings.ReplaceAll(content, editor.Zwsp, "")
	markdown = strings.ReplaceAll(markdown, editor.Zwsp, "")

	// 剔除标签结尾处的零宽连字符 Improve search for emojis in tags https://github.com/siyuan-note/siyuan/issues/15391
	fcontent = strings.ReplaceAll(fcontent, string(gulu.ZWJ)+"#", "#")
	content = strings.ReplaceAll(content, string(gulu.ZWJ)+"#", "#")
	markdown = strings.ReplaceAll(markdown, string(gulu.ZWJ)+"#", "#")

	block = &BlockRecord{
		ID:       n.ID,
		ParentID: parentID,
		RootID:   rootID,
		Hash:     hash,
		Box:      boxID,
		Path:     p,
		HPath:    tree.HPath,
		Name:     name,
		Alias:    alias,
		Memo:     memo,
		Tag:      tag,
		Content:  content,
		FContent: fcontent,
		Markdown: markdown,
		Length:   length,
		Type:     nodetype.TypeAbbr(n.Type.String()),
		SubType:  subType(n),
		IAL:      ialContent,
		Sort:     nSort(n),
		Created:  projection.created(n.ID),
		Updated:  n.IALAttr("updated"),
	}

	attrs := parse.IAL2Map(n.KramdownIAL)
	for attrName, attrVal := range attrs {
		if !isAttr(attrName) {
			continue
		}

		attr := &AttributeRecord{
			ID:      ast.NewNodeID(),
			Name:    attrName,
			Value:   attrVal,
			Type:    "b",
			BlockID: n.ID,
			RootID:  rootID,
			Box:     boxID,
			Path:    p,
		}
		attributes = append(attributes, attr)
	}
	return
}

func tagFromNode(node *ast.Node) (ret string) {
	tagBuilder := bytes.Buffer{}

	if ast.NodeDocument == node.Type {
		tagIAL := html.UnescapeString(node.IALAttr("tags"))
		tags := strings.Split(tagIAL, ",")
		for _, t := range tags {
			t = strings.TrimSpace(t)
			if "" == t {
				continue
			}
			tagBuilder.WriteString("#")
			tagBuilder.WriteString(t)
			tagBuilder.WriteString("# ")
		}
		return strings.TrimSpace(tagBuilder.String())
	}

	ast.Walk(node, func(n *ast.Node, entering bool) ast.WalkStatus {
		if !entering {
			return ast.WalkContinue
		}

		if n.IsTextMarkType("tag") {
			tagBuilder.WriteString("#")
			tagBuilder.WriteString(n.Content())
			tagBuilder.WriteString("# ")
		}
		return ast.WalkContinue
	})
	return strings.TrimSpace(tagBuilder.String())
}

func nSort(n *ast.Node) int {
	switch n.Type {
	// 以下为块级元素
	case ast.NodeHeading:
		return 5
	case ast.NodeParagraph:
		return 10
	case ast.NodeCodeBlock:
		return 10
	case ast.NodeMathBlock:
		return 10
	case ast.NodeTable:
		return 10
	case ast.NodeHTMLBlock:
		return 10
	case ast.NodeList:
		return 20
	case ast.NodeListItem:
		return 20
	case ast.NodeBlockquote:
		return 20
	case ast.NodeSuperBlock:
		return 30
	case ast.NodeAttributeView:
		return 30
	case ast.NodeDocument:
		return 0
	case ast.NodeText, ast.NodeTextMark:
		if n.IsTextMarkType("tag") {
			return 205
		}
		return 200
	}
	return 100
}
