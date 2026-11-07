// SiYuan - Refactor your thinking
// Copyright (c) 2020-present, b3log.org
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

// Extracted original SQL content walk. External data is explicitly owner-bound.
package indexdb

import (
	"bytes"
	"context"
	"github.com/88250/gulu"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/editor"
	"github.com/88250/lute/html"
	"github.com/88250/lute/render"
	"strings"
)

type ContentServices struct {
	AssetText     func(context.Context, string) (string, error)
	AttributeView func(context.Context, string, bool) (string, error)
	Warning       func(string)
}

func StaticContent(ctx context.Context, node *ast.Node, excludeTypes []string, includeTextMarkATitleURL, includeAssetPath, fullAttributeView bool, services ContentServices) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	var contentErr error
	if nil == node {
		return "", nil
	}

	if ast.NodeDocument == node.Type {
		return node.IALAttr("title"), nil
	}

	if ast.NodeAttributeView == node.Type {
		if services.AttributeView == nil {
			return "", ErrProjection
		}
		return services.AttributeView(ctx, node.AttributeViewID, fullAttributeView)
	}

	buf := bytes.Buffer{}
	buf.Grow(4096)
	lastSpace := false
	ast.Walk(node, func(n *ast.Node, entering bool) ast.WalkStatus {
		if err := ctx.Err(); err != nil {
			contentErr = err
			return ast.WalkStop
		}
		if !entering {
			return ast.WalkContinue
		}

		if n.IsContainerBlock() {
			if !lastSpace {
				buf.WriteByte(' ')
				lastSpace = true
			}
			return ast.WalkContinue
		}

		if gulu.Str.Contains(n.Type.String(), excludeTypes) {
			return ast.WalkContinue
		}

		switch n.Type {
		case ast.NodeTableCell:
			// 表格块写入数据库表时在单元格之间添加空格 https://github.com/siyuan-note/siyuan/issues/7654
			if 0 < buf.Len() && ' ' != buf.Bytes()[buf.Len()-1] {
				buf.WriteByte(' ')
			}
		case ast.NodeImage:
			linkDest := n.ChildByType(ast.NodeLinkDest)
			var linkDestStr, ocrText string
			if nil != linkDest {
				linkDestStr = linkDest.TokensStr()
				if services.AssetText == nil {
					contentErr = ErrProjection
					return ast.WalkStop
				}
				ocrText, contentErr = services.AssetText(ctx, linkDestStr)
				if contentErr != nil {
					return ast.WalkStop
				}
			}

			linkText := n.ChildByType(ast.NodeLinkText)
			if nil != linkText {
				buf.Write(linkText.Tokens)
				buf.WriteByte(' ')
			}
			if "" != ocrText {
				buf.WriteString(ocrText)
				buf.WriteByte(' ')
			}
			if nil != linkDest {
				if !bytes.HasPrefix(linkDest.Tokens, []byte("assets/")) || includeAssetPath {
					buf.Write(linkDest.Tokens)
					buf.WriteByte(' ')
				}
			}
			if linkTitle := n.ChildByType(ast.NodeLinkTitle); nil != linkTitle {
				buf.Write(linkTitle.Tokens)
			}
			return ast.WalkSkipChildren
		case ast.NodeLinkText:
			buf.Write(n.Tokens)
			buf.WriteByte(' ')
		case ast.NodeLinkDest:
			buf.Write(n.Tokens)
			buf.WriteByte(' ')
		case ast.NodeLinkTitle:
			buf.Write(n.Tokens)
		case ast.NodeText, ast.NodeCodeBlockCode, ast.NodeMathBlockContent, ast.NodeHTMLBlock:
			tokens := n.Tokens
			if isChartCodeBlockCode(n) {
				// 图表块的内容在数据库 `blocks` 表 `content` 字段中被转义 https://github.com/siyuan-note/siyuan/issues/6326
				tokens = html.UnescapeHTML(tokens)
			}
			buf.Write(tokens)
		case ast.NodeTextMark:
			for _, excludeType := range excludeTypes {
				if strings.HasPrefix(excludeType, "NodeTextMark-") {
					if n.IsTextMarkType(excludeType[len("NodeTextMark-"):]) {
						return ast.WalkContinue
					}
				}
			}

			if n.IsTextMarkType("tag") {
				buf.WriteByte('#')
			}
			buf.WriteString(n.Content())
			if n.IsTextMarkType("tag") {
				buf.WriteByte('#')
			}
			if n.IsTextMarkType("a") && includeTextMarkATitleURL {
				// 搜索不到超链接元素的 URL 和标题 https://github.com/siyuan-note/siyuan/issues/7352
				if "" != n.TextMarkATitle {
					buf.WriteString(" " + unescapeHTML(n.TextMarkATitle))
				}

				if !strings.HasPrefix(n.TextMarkAHref, "assets/") || includeAssetPath {
					href := unescapeHTML(n.TextMarkAHref)
					buf.WriteString(" " + unescapeHTML(href))
				}
			}
		case ast.NodeBackslashContent:
			buf.Write(n.Tokens)
		case ast.NodeAudio, ast.NodeVideo:
			buf.WriteString(nodeSrcTokens(n, services.Warning))
			buf.WriteByte(' ')
		}
		lastSpace = false
		return ast.WalkContinue
	})

	// 这里不要 trim，否则无法搜索首尾空格
	// Improve search and replace for spaces https://github.com/siyuan-note/siyuan/issues/10231
	return buf.String(), contentErr
}
func isChartCodeBlockCode(code *ast.Node) bool {
	if nil == code.Previous || ast.NodeCodeBlockFenceInfoMarker != code.Previous.Type || 1 > len(code.Previous.CodeBlockInfo) {
		return false
	}

	language := gulu.Str.FromBytes(code.Previous.CodeBlockInfo)
	language = strings.ReplaceAll(language, editor.Caret, "")
	return render.NoHighlight(language)
}
func unescapeHTML(s string) string {
	if strings.TrimSpace(s) == "" {
		return s
	}
	return html.UnescapeString(s)
}
func nodeSrcTokens(n *ast.Node, warn func(string)) string {
	if index := bytes.Index(n.Tokens, []byte(`src="`)); index > 0 {
		src := n.Tokens[index+len(`src="`):]
		if index = bytes.Index(src, []byte(`"`)); index > 0 {
			return strings.TrimSpace(string(src[:index]))
		}
		if warn != nil {
			warn("src is missing the closing double quote in tree [" + n.Box + n.Path + "] ")
		}
	}
	return ""
}
