// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package blockview

import (
	"bytes"
	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/html"
	"regexp"
	"strings"
)

func Outline(heading *ast.Node, luteEngine *lute.Lute) (ret string) {
	if nil == heading {
		return ""
	}

	if ast.NodeDocument == heading.Type {
		return heading.IALAttr("title")
	}

	buf := bytes.Buffer{}
	buf.Grow(4096)
	ast.Walk(heading, func(n *ast.Node, entering bool) ast.WalkStatus {
		if !entering {
			switch n.Type {
			case ast.NodeHeading:
				// Show heading block appearance style in the Outline Panel https://github.com/siyuan-note/siyuan/issues/7872
				if style := n.IALAttr("style"); "" != style {
					buf.WriteString("</span>")
				}
			}
			return ast.WalkContinue
		}

		if style := n.IALAttr("style"); "" != style {
			if strings.Contains(style, "font-size") { // 大纲字号不应该跟随字体设置 https://github.com/siyuan-note/siyuan/issues/7202
				style = regexp.MustCompile("font-size:.*?;").ReplaceAllString(style, "font-size: inherit;")
				n.SetIALAttr("style", style)
			}
		}

		switch n.Type {
		case ast.NodeHeading:
			// Show heading block appearance style in the Outline Panel https://github.com/siyuan-note/siyuan/issues/7872
			if style := n.IALAttr("style"); "" != style {
				buf.WriteString("<span style=\"")
				buf.WriteString(style)
				buf.WriteString("\">")
			}
		case ast.NodeText, ast.NodeLinkText, ast.NodeCodeBlockCode, ast.NodeMathBlockContent:
			tokens := html.EscapeHTML(n.Tokens)
			tokens = bytes.ReplaceAll(tokens, []byte(" "), []byte("&nbsp;")) // 大纲面板条目中无法显示多个空格 https://github.com/siyuan-note/siyuan/issues/4370
			buf.Write(tokens)
		case ast.NodeBackslashContent:
			buf.Write(html.EscapeHTML(n.Tokens))
		case ast.NodeTextMark:
			dom := luteEngine.RenderNodeBlockDOM(n)
			buf.WriteString(dom)
			return ast.WalkSkipChildren
		case ast.NodeEmoji:
			dom := luteEngine.RenderNodeBlockDOM(n)
			buf.WriteString(dom)
			return ast.WalkSkipChildren
		case ast.NodeImage:
			if title := n.ChildByType(ast.NodeLinkTitle); nil != title {
				// 标题后直接跟图片时图片的提示文本不再渲染到大纲中 https://github.com/siyuan-note/siyuan/issues/6278
				title.Unlink()
			}
			dom := luteEngine.RenderNodeBlockDOM(n)
			buf.WriteString(dom)
			return ast.WalkSkipChildren
		}
		return ast.WalkContinue
	})

	ret = strings.TrimSpace(buf.String())
	ret = strings.ReplaceAll(ret, "\n", "")
	return
}
