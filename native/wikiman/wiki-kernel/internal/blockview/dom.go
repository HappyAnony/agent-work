// SPDX-License-Identifier: AGPL-3.0-or-later
package blockview

import (
	"context"
	"errors"
	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/editor"
	"github.com/88250/lute/html"
	"github.com/88250/lute/parse"
	"regexp"
	"strings"
	"wiki-kernel/internal/docview"
	"wiki-kernel/internal/indexdb"
)

type DOMServices struct {
	Parser *lute.Lute
	Trees  func(context.Context, []string) (map[string]*parse.Tree, error)
	Load   func(context.Context, string) (*parse.Tree, error)
	Select func(context.Context, string) ([]indexdb.BlockRecord, error)
}

func DOMs(ctx context.Context, ids []string, embed bool, s DOMServices) (out map[string]string, err error) {
	if ctx == nil || s.Parser == nil || s.Parser.RenderOptions == nil || s.Trees == nil || (embed && (s.Load == nil || s.Select == nil)) {
		return nil, errors.New("block_dom_dependencies_unbound")
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	out = map[string]string{}
	if len(ids) == 0 {
		return out, nil
	}
	parser := *s.Parser
	options := *s.Parser.RenderOptions
	parser.RenderOptions = &options
	trees, err := s.Trees(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		tree := trees[id]
		if tree == nil {
			continue
		}
		node := docview.GetNodeInTree(tree, id)
		if node == nil {
			continue
		}
		if embed {
			if err = resolveEmbedContent(ctx, node, &parser, s); err != nil {
				return nil, err
			}
		}
		ast.Walk(node, func(n *ast.Node, enter bool) ast.WalkStatus {
			if ctx.Err() != nil {
				return ast.WalkStop
			}
			if enter && n.IsBlock() {
				if parent := ParentFoldedHeading(n); parent != nil {
					n.SetIALAttr("parent-heading", parent.ID)
				}
			}
			return ast.WalkContinue
		})
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		text := parser.RenderNodeBlockDOM(node)
		if embed {
			text = ProcessEmbedHTML(text)
		}
		out[id] = text
	}
	return out, ctx.Err()
}
func ParentFoldedHeading(node *ast.Node) (parentFoldedHeading *ast.Node) {
	if nil == node {
		return
	}

	currentLevel := 7
	if ast.NodeHeading == node.Type {
		currentLevel = node.HeadingLevel
	}
	for n := node.Previous; nil != n; n = n.Previous {
		if ast.NodeHeading != n.Type {
			continue
		}

		if n.HeadingLevel >= currentLevel {
			continue
		}
		currentLevel = n.HeadingLevel

		if "1" == n.IALAttr("fold") {
			if ast.NodeHeading != node.Type {
				parentFoldedHeading = n
			}
			if n.HeadingLevel < node.HeadingLevel {
				parentFoldedHeading = n
			}
		}
	}
	return
}
func resolveEmbedContent(ctx context.Context, n *ast.Node, luteEngine *lute.Lute, s DOMServices) (err error) {
	ast.Walk(n, func(node *ast.Node, entering bool) ast.WalkStatus {
		if ctx.Err() != nil {
			err = ctx.Err()
			return ast.WalkStop
		}
		if !entering || ast.NodeBlockQueryEmbed != node.Type {
			return ast.WalkContinue
		}

		// 获取嵌入块的查询语句
		scriptNode := node.ChildByType(ast.NodeBlockQueryEmbedScript)
		if nil == scriptNode {
			return ast.WalkContinue
		}
		stmt := scriptNode.TokensStr()
		stmt = html.UnescapeString(stmt)
		stmt = strings.ReplaceAll(stmt, editor.IALValEscNewLine, "\n")

		// 执行查询获取嵌入的块
		sqlBlocks, e := s.Select(ctx, stmt)
		if e != nil {
			err = e
			return ast.WalkStop
		}

		// 收集所有嵌入块的内容 HTML
		var embedContents []string
		for _, sqlBlock := range sqlBlocks {
			if "query_embed" == sqlBlock.Type {
				continue
			}

			subTree, e := s.Load(ctx, sqlBlock.ID)
			if e != nil {
				err = e
				return ast.WalkStop
			}
			if nil == subTree {
				continue
			}

			// 将内容转换为 HTML，直接使用原始 AST 节点渲染以保持正确的 data-node-id
			var contentHTML string
			if "d" == sqlBlock.Type {
				// 文档块：直接使用原始 AST 节点渲染，保持原始的 data-node-id
				contentHTML = luteEngine.RenderNodeBlockDOM(subTree.Root)
			} else if "h" == sqlBlock.Type {
				// 标题块：使用标题及其子块的原始 AST 节点渲染
				h := docview.GetNodeInTree(subTree, sqlBlock.ID)
				if nil == h {
					continue
				}
				var hChildren []*ast.Node
				hChildren = append(hChildren, h)
				hChildren = append(hChildren, docview.HeadingChildren(h)...)

				// 创建一个临时的文档节点来包含所有子节点
				tempRoot := &ast.Node{Type: ast.NodeDocument}
				for _, hChild := range hChildren {
					tempRoot.AppendChild(hChild)
				}
				contentHTML = luteEngine.RenderNodeBlockDOM(tempRoot)
			} else {
				// 其他块：直接使用原始 AST 节点渲染
				blockNode := docview.GetNodeInTree(subTree, sqlBlock.ID)
				if nil == blockNode {
					continue
				}
				contentHTML = luteEngine.RenderNodeBlockDOM(blockNode)
			}

			if contentHTML != "" {
				embedContents = append(embedContents, contentHTML)
			}
		}

		// 如果有内容，在嵌入块上添加内容标记
		if len(embedContents) > 0 {
			node.SetIALAttr("embed-content", strings.Join(embedContents, ""))
		}

		return ast.WalkContinue
	})
	return err
}
func ProcessEmbedHTML(htmlStr string) string {
	// 使用正则表达式查找所有带有 embed-content 属性的嵌入块
	embedPattern := `<div[^>]*data-type="NodeBlockQueryEmbed"[^>]*embed-content="[^"]*"[^>]*>`
	re := regexp.MustCompile(embedPattern)

	return re.ReplaceAllStringFunc(htmlStr, func(match string) string {
		// 提取 embed-content 属性值
		contentPattern := `embed-content="([^"]*)"`
		contentRe := regexp.MustCompile(contentPattern)
		contentMatches := contentRe.FindStringSubmatch(match)

		if len(contentMatches) > 1 {
			embedContent := contentMatches[1]
			// HTML 解码
			embedContent = html.UnescapeString(embedContent)

			// 移除 embed-content 属性，避免在最终 HTML 中显示
			cleanMatch := contentRe.ReplaceAllString(match, "")

			// 将内容插入到嵌入块内部
			return cleanMatch + embedContent + "</div>"
		}

		return match
	})
}
