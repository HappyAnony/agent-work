// SPDX-License-Identifier: AGPL-3.0-or-later
package docview

import (
	"bytes"
	"github.com/88250/gulu"
	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/editor"
	"github.com/88250/lute/html"
	"github.com/88250/lute/lex"
	"github.com/88250/lute/parse"
	"github.com/88250/lute/render"
	"sort"
	"strings"
	"wiki-kernel/search"
)

func IsInFoldedHeading(node, currentHeading *ast.Node) bool {
	if nil == node {
		return false
	}

	heading := HeadingParent(node)
	if nil == heading {
		return false
	}
	if "1" == heading.IALAttr("heading-fold") || "1" == heading.IALAttr("fold") {
		return true
	}
	if heading == currentHeading {
		// node 就在当前标题层级下的话不递归继续查询，直接返回不折叠
		return false
	}
	return IsInFoldedHeading(heading, currentHeading)
}
func HeadingChildren(heading *ast.Node) (ret []*ast.Node) {
	start := heading.Next
	if nil == start {
		return
	}
	if ast.NodeKramdownBlockIAL == start.Type {
		start = start.Next // 跳过 heading 的 IAL
	}

	currentLevel := heading.HeadingLevel
	for n := start; nil != n; n = n.Next {
		if ast.NodeHeading == n.Type {
			if currentLevel >= n.HeadingLevel {
				break
			}
		}
		ret = append(ret, n)
	}
	return
}
func HeadingParent(node *ast.Node) *ast.Node {
	if nil == node {
		return nil
	}

	currentLevel := 16
	if ast.NodeHeading == node.Type {
		currentLevel = node.HeadingLevel
	}

	for n := node.Previous; nil != n; n = n.Previous {
		if ast.NodeHeading == n.Type && n.HeadingLevel < currentLevel {
			return n
		}
	}
	return node.Parent
}
func CountBlockNodes(node *ast.Node) (ret int) {
	ast.Walk(node, func(n *ast.Node, entering bool) ast.WalkStatus {
		if !entering || !n.IsBlock() || ast.NodeList == n.Type || ast.NodeBlockquote == n.Type || ast.NodeSuperBlock == n.Type {
			return ast.WalkContinue
		}

		if "1" == n.IALAttr("fold") {
			ret++
			return ast.WalkSkipChildren
		}

		ret++
		return ast.WalkContinue
	})
	return
}

// ParentNodesWithHeadings 返回所有父级节点。
// 注意:返回的父级节点包括了标题节点，并且不保证父级层次顺序。
func ParentBlock(node *ast.Node) *ast.Node {
	for p := node.Parent; nil != p; p = p.Parent {
		if "" != p.ID && p.IsBlock() {
			return p
		}
	}
	return nil
}
func GetNodeInTree(tree *parse.Tree, id string) (ret *ast.Node) {
	ast.Walk(tree.Root, func(n *ast.Node, entering bool) ast.WalkStatus {
		if !entering {
			return ast.WalkContinue
		}

		if id == n.ID {
			ret = n
			ret.Box = tree.Box
			ret.Path = tree.Path
			return ast.WalkStop
		}
		return ast.WalkContinue
	})
	return
}
func IsChartCodeBlockCode(code *ast.Node) bool {
	if nil == code.Previous || ast.NodeCodeBlockFenceInfoMarker != code.Previous.Type || 1 > len(code.Previous.CodeBlockInfo) {
		return false
	}

	language := gulu.Str.FromBytes(code.Previous.CodeBlockInfo)
	language = strings.ReplaceAll(language, editor.Caret, "")
	return render.NoHighlight(language)
}
func IsBlockRef(n *ast.Node) bool {
	if nil == n {
		return false
	}
	return (ast.NodeTextMark == n.Type && n.IsTextMarkType("block-ref")) || ast.NodeBlockRef == n.Type
}
func IsFileAnnotationRef(n *ast.Node) bool {
	if nil == n {
		return false
	}
	return ast.NodeTextMark == n.Type && n.IsTextMarkType("file-annotation-ref")
}
func BacklinkNodes(n *ast.Node, originalRefBlockIDs map[string]string) (ret []*ast.Node, expand bool) {
	expand = true
	if ast.NodeListItem == n.Type {
		if nil == n.FirstChild {
			return
		}

		c := n.FirstChild
		if 3 == n.ListData.Typ {
			c = n.FirstChild.Next
		}

		if c != n.LastChild { // 存在子列表
			for ; nil != c; c = c.Next {
				if originalRefBlockIDs[n.ID] != c.ID {
					continue
				}

				for liFirstBlockSpan := c.FirstChild; nil != liFirstBlockSpan; liFirstBlockSpan = liFirstBlockSpan.Next {
					if IsBlockRef(liFirstBlockSpan) {
						continue
					}
					if "" != strings.TrimSpace(liFirstBlockSpan.Text()) {
						expand = false
						break
					}
				}
			}
		}

		ret = append(ret, n)
	} else if ast.NodeHeading == n.Type {
		c := n.FirstChild
		if nil == c {
			return
		}

		for headingFirstSpan := c; nil != headingFirstSpan; headingFirstSpan = headingFirstSpan.Next {
			if IsBlockRef(headingFirstSpan) {
				continue
			}
			if "" != strings.TrimSpace(headingFirstSpan.Text()) {
				expand = false
				break
			}
		}

		ret = append(ret, n)
		cc := HeadingChildren(n)
		ret = append(ret, cc...)
	} else {
		ret = append(ret, n)
	}
	return
}
func MarkReplaceSpan(n *ast.Node, unlinks *[]*ast.Node, keywords []string, markSpanDataType string, luteEngine *lute.Lute, caseSensitive bool) bool {
	if ast.NodeText == n.Type {
		text := n.Content()
		escapedText := escapeHTML(text)
		escapedKeywords := make([]string, len(keywords))
		for i, keyword := range keywords {
			escapedKeywords[i] = escapeHTML(keyword)
		}
		hText := search.EncloseHighlighting(escapedText, escapedKeywords, search.GetMarkSpanStart(markSpanDataType), search.GetMarkSpanEnd(), caseSensitive, false)
		if hText != escapedText {
			text = hText
		}
		n.Tokens = gulu.Str.ToBytes(text)
		if bytes.Contains(n.Tokens, []byte(search.MarkDataType)) {
			linkTree := parse.Inline("", n.Tokens, luteEngine.ParseOptions)
			var children []*ast.Node
			for c := linkTree.Root.FirstChild.FirstChild; nil != c; c = c.Next {
				children = append(children, c)
			}
			for _, c := range children {
				n.InsertBefore(c)
			}
			*unlinks = append(*unlinks, n)
			return true
		}
	} else if ast.NodeTextMark == n.Type {
		// 搜索结果高亮支持大部分行级元素 https://github.com/siyuan-note/siyuan/issues/6745

		if n.IsTextMarkType("inline-math") || n.IsTextMarkType("inline-memo") {
			return false
		}

		var text string
		if n.IsTextMarkType("code") {
			// code 在前面的 n.
			for i, k := range keywords {
				keywords[i] = html.EscapeString(k)
			}
			text = n.TextMarkTextContent
		} else {
			text = n.Content()
		}

		startTag := search.GetMarkSpanStart(markSpanDataType)
		text = search.EncloseHighlighting(text, keywords, startTag, search.GetMarkSpanEnd(), caseSensitive, false)
		if strings.Contains(text, search.MarkDataType) {
			dataType := search.GetMarkSpanStart(n.TextMarkType + " " + search.MarkDataType)
			text = strings.ReplaceAll(text, startTag, dataType)
			tokens := gulu.Str.ToBytes(text)
			linkTree := parse.Inline("", tokens, luteEngine.ParseOptions)
			var children []*ast.Node
			for c := linkTree.Root.FirstChild.FirstChild; nil != c; c = c.Next {
				if ast.NodeText == c.Type {
					c.Type = ast.NodeTextMark
					c.TextMarkType = n.TextMarkType
					c.TextMarkTextContent = string(c.Tokens)
					if n.IsTextMarkType("a") {
						c.TextMarkAHref, c.TextMarkATitle = n.TextMarkAHref, n.TextMarkATitle
					} else if IsBlockRef(n) {
						c.TextMarkBlockRefID = n.TextMarkBlockRefID
						c.TextMarkBlockRefSubtype = n.TextMarkBlockRefSubtype
					} else if IsFileAnnotationRef(n) {
						c.TextMarkFileAnnotationRefID = n.TextMarkFileAnnotationRefID
					}
				} else if ast.NodeTextMark == c.Type {
					if n.IsTextMarkType("a") {
						c.TextMarkAHref, c.TextMarkATitle = n.TextMarkAHref, n.TextMarkATitle
					} else if IsBlockRef(n) {
						c.TextMarkBlockRefID = n.TextMarkBlockRefID
						c.TextMarkBlockRefSubtype = n.TextMarkBlockRefSubtype
					} else if IsFileAnnotationRef(n) {
						c.TextMarkFileAnnotationRefID = n.TextMarkFileAnnotationRefID
					}
				}

				children = append(children, c)
				if nil != n.Next && ast.NodeKramdownSpanIAL == n.Next.Type {
					c.KramdownIAL = n.KramdownIAL
					ial := &ast.Node{Type: ast.NodeKramdownSpanIAL, Tokens: n.Next.Tokens}
					children = append(children, ial)
				}
			}
			for _, c := range children {
				n.InsertBefore(c)
			}
			*unlinks = append(*unlinks, n)
			return true
		}
	}
	return false
}

// markReplaceSpanWithSplit 用于处理虚拟引用和反链提及高亮。
func MarkSplit(text string, keywords []string, replacementStart, replacementEnd string, caseSensitive bool) (ret string) {
	// 虚拟引用和反链提及关键字按最长匹配优先 https://github.com/siyuan-note/siyuan/issues/7465
	sort.Slice(keywords, func(i, j int) bool { return len(keywords[i]) > len(keywords[j]) })

	tmp := search.EncloseHighlighting(text, keywords, replacementStart, replacementEnd, caseSensitive, true)
	parts := strings.Split(tmp, replacementEnd)
	buf := bytes.Buffer{}
	for i := 0; i < len(parts); i++ {
		if i >= len(parts)-1 {
			buf.WriteString(parts[i])
			break
		}

		if nextPart := parts[i+1]; 0 < len(nextPart) && lex.IsASCIILetter(nextPart[0]) {
			// 取消已经高亮的部分
			part := strings.ReplaceAll(parts[i], replacementStart, "")
			buf.WriteString(part)
			continue
		}

		buf.WriteString(parts[i])
		buf.WriteString(replacementEnd)
	}
	ret = buf.String()
	return
}

func containsSubStr(s string, subStrs []string) bool {
	for _, v := range subStrs {
		if strings.Contains(s, v) {
			return true
		}
	}
	return false
}
func escapeHTML(s string) (ret string) {
	ret = s
	if "" == strings.TrimSpace(ret) {
		return
	}

	ret = html.EscapeString(ret)
	return
}
