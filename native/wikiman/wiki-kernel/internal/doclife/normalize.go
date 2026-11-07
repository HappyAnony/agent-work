// SPDX-License-Identifier: AGPL-3.0-or-later
// Original normalizeTree from model/box.go, with explicit clock/ID/warnings.
package doclife

import (
	"bytes"
	"context"
	"fmt"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/html"
	"github.com/88250/lute/lex"
	"github.com/88250/lute/parse"
	"github.com/araddon/dateparse"
	"gopkg.in/yaml.v3"
	"strings"
	"time"
)

func templateParagraph(s TemplateBodyServices) *ast.Node {
	n := &ast.Node{ID: s.NewID(), Type: ast.NodeParagraph}
	n.SetIALAttr("id", n.ID)
	n.SetIALAttr("updated", templateTimeID(n.ID, s))
	return n
}
func templateTimeID(id string, s TemplateBodyServices) string {
	if len(id) >= 14 {
		return id[:14]
	}
	return s.Now().Format("20060102150405")
}
func (s TemplateBodyServices) warnf(format string, args ...interface{}) {
	if s.Warning != nil {
		s.Warning(fmt.Errorf(format, args...))
	}
}
func NormalizeCreationTemplate(ctx context.Context, t *parse.Tree, s TemplateBodyServices) (string, string, string, error) {
	if ctx == nil || t == nil || t.Root == nil || s.NewID == nil || s.Now == nil {
		return "", "", "", ErrDependency
	}
	if e := ctx.Err(); e != nil {
		return "", "", "", e
	}
	a, b, c := normalizeCreationBody(ctx, t, s)
	return a, b, c, ctx.Err()
}
func normalizeCreationBody(ctx context.Context, tree *parse.Tree, s TemplateBodyServices) (yfmRootID, yfmTitle, yfmUpdated string) {
	if nil == tree.Root.FirstChild {
		tree.Root.AppendChild(templateParagraph(s))
	} else if !tree.Root.FirstChild.IsBlock() || ast.NodeKramdownBlockIAL == tree.Root.FirstChild.Type {
		tree.Root.PrependChild(templateParagraph(s))
	}

	var unlinks []*ast.Node
	ast.Walk(tree.Root, func(n *ast.Node, entering bool) ast.WalkStatus {
		if ctx.Err() != nil {
			return ast.WalkStop
		}
		if !entering {
			return ast.WalkContinue
		}

		if n.IsEmptyBlockIAL() {
			// 空段落保留
			p := &ast.Node{Type: ast.NodeParagraph}
			p.KramdownIAL = parse.Tokens2IAL(n.Tokens)
			p.ID = p.IALAttr("id")
			n.InsertBefore(p)
			return ast.WalkContinue
		}

		id := n.IALAttr("id")
		if "" == id && n.IsBlock() {
			n.SetIALAttr("id", n.ID)
		}

		if "" == n.IALAttr("id") && (ast.NodeParagraph == n.Type || ast.NodeList == n.Type || ast.NodeListItem == n.Type || ast.NodeBlockquote == n.Type ||
			ast.NodeMathBlock == n.Type || ast.NodeCodeBlock == n.Type || ast.NodeHeading == n.Type || ast.NodeTable == n.Type || ast.NodeThematicBreak == n.Type ||
			ast.NodeYamlFrontMatter == n.Type || ast.NodeBlockQueryEmbed == n.Type || ast.NodeSuperBlock == n.Type || ast.NodeAttributeView == n.Type ||
			ast.NodeHTMLBlock == n.Type || ast.NodeIFrame == n.Type || ast.NodeWidget == n.Type || ast.NodeAudio == n.Type || ast.NodeVideo == n.Type) {
			n.ID = s.NewID()
			n.KramdownIAL = [][]string{{"id", n.ID}}
			n.InsertAfter(&ast.Node{Type: ast.NodeKramdownBlockIAL, Tokens: []byte("{: id=\"" + n.ID + "\"}")})
			n.SetIALAttr("updated", templateTimeID(n.ID, s))
		}
		if "" == n.ID && 0 < len(n.KramdownIAL) && ast.NodeDocument != n.Type {
			n.ID = n.IALAttr("id")
		}

		if ast.NodeHTMLBlock == n.Type {
			tokens := bytes.TrimSpace(n.Tokens)
			if !bytes.HasPrefix(tokens, []byte("<div>")) {
				tokens = []byte("<div>\n" + string(tokens))
			}
			if !bytes.HasSuffix(tokens, []byte("</div>")) {
				tokens = append(tokens, []byte("\n</div>")...)
			}
			n.Tokens = tokens
			return ast.WalkContinue
		}

		if ast.NodeInlineHTML == n.Type {
			n.Type = ast.NodeText
			return ast.WalkContinue
		}

		if ast.NodeParagraph == n.Type && nil != n.FirstChild && ast.NodeTaskListItemMarker == n.FirstChild.Type {
			// 踢掉任务列表的第一个子节点左侧空格
			n.FirstChild.Next.Tokens = bytes.TrimLeft(n.FirstChild.Next.Tokens, " ")
			// 调整 li.p.tlim 为 li.tlim.p
			n.InsertBefore(n.FirstChild)
		}

		if ast.NodeLinkTitle == n.Type {
			// 避免重复转义图片标题内容 Repeat the escaped content of the image title https://github.com/siyuan-note/siyuan/issues/11681
			n.Tokens = html.UnescapeBytes(n.Tokens)
		}

		if ast.NodeYamlFrontMatterContent == n.Type {
			// Parsing YAML Front Matter as document custom attributes when importing Markdown files https://github.com/siyuan-note/siyuan/issues/10878
			attrs := map[string]interface{}{}
			parseErr := yaml.Unmarshal(n.Tokens, &attrs)
			if parseErr != nil {
				s.warnf("parse YAML front matter [%s] failed: %s", n.Tokens, parseErr)
				return ast.WalkContinue
			}

			for attrK, attrV := range attrs {
				// Improve parsing of YAML Front Matter when importing Markdown https://github.com/siyuan-note/siyuan/issues/12962
				if "title" == attrK {
					yfmTitle = fmt.Sprint(attrV)
					tree.Root.SetIALAttr("title", yfmTitle)
					continue
				}
				if "date" == attrK {
					created, parseTimeErr := dateparse.ParseIn(fmt.Sprint(attrV), time.Local)
					if nil == parseTimeErr {
						yfmRootID = created.Format("20060102150405") + "-" + s.NewID()[15:]
						tree.Root.ID = yfmRootID
						tree.Root.SetIALAttr("id", yfmRootID)
					}
					continue
				}
				if "lastmod" == attrK {
					updated, parseTimeErr := dateparse.ParseIn(fmt.Sprint(attrV), time.Local)
					if nil == parseTimeErr {
						yfmUpdated = updated.Format("20060102150405")
						tree.Root.SetIALAttr("updated", yfmUpdated)
					}
					continue
				}
				if "tags" == attrK && nil != attrV {
					var tags string
					if str, ok := attrV.(string); ok {
						tags = strings.TrimSpace(str)
						tree.Root.SetIALAttr("tags", tags)
						continue
					}

					for _, tag := range attrV.([]any) {
						tagStr := fmt.Sprintf("%v", tag)
						if "" == tag {
							continue
						}
						tagStr = strings.TrimLeft(tagStr, "#,'\"")
						tagStr = strings.TrimRight(tagStr, "#,'\"")
						tags += tagStr + ","
					}
					tags = strings.TrimRight(tags, ",")
					tags = strings.TrimSpace(tags)
					if "" != tags {
						tree.Root.SetIALAttr("tags", tags)
					}
					continue
				}

				validKeyName := true
				for i := 0; i < len(attrK); i++ {
					if !lex.IsASCIILetterNumHyphen(attrK[i]) {
						validKeyName = false
						break
					}
				}
				if !validKeyName {
					s.warnf("invalid YAML key [%s] in [%s]", attrK, n.ID)
					continue
				}

				tree.Root.SetIALAttr("custom-"+attrK, fmt.Sprint(attrV))
			}

			// Import the YAML at the beginning of the Markdown as a code block https://github.com/siyuan-note/siyuan/issues/16488
			codeBlock := &ast.Node{Type: ast.NodeCodeBlock}
			openMarker := &ast.Node{Type: ast.NodeCodeBlockFenceOpenMarker, Tokens: []byte("```"), CodeBlockFenceLen: 3}
			codeBlock.AppendChild(openMarker)
			info := &ast.Node{Type: ast.NodeCodeBlockFenceInfoMarker, CodeBlockInfo: []byte("yaml")}
			codeBlock.AppendChild(info)
			content := []byte("---\n")
			content = append(content, n.Tokens...)
			content = append(content, []byte("\n---")...)
			code := &ast.Node{Type: ast.NodeCodeBlockCode, Tokens: content}
			codeBlock.AppendChild(code)
			closeMarker := &ast.Node{Type: ast.NodeCodeBlockFenceCloseMarker, Tokens: []byte("```"), CodeBlockFenceLen: 3}
			codeBlock.AppendChild(closeMarker)
			tree.Root.PrependChild(codeBlock)
		}

		if ast.NodeYamlFrontMatter == n.Type {
			unlinks = append(unlinks, n)
		}

		return ast.WalkContinue
	})
	for _, n := range unlinks {
		n.Unlink()
	}

	rootIAL := parse.Tokens2IAL(tree.Root.LastChild.Tokens)
	for _, kv := range rootIAL {
		tree.Root.SetIALAttr(kv[0], kv[1])
	}
	return
}
