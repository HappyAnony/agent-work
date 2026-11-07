// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"bytes"
	"context"
	"errors"
	"github.com/88250/gulu"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"github.com/siyuan-note/logging"
	"path"
	"strings"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/internal/doclife"
	"wiki-kernel/internal/nativeparser"
)

// Preparation owns candidates and links for exactly one import. It grants no
// filesystem write authority; the remaining legacy publication ports are separate.
type markdownImportPreparation struct {
	ctx       context.Context
	archive   *ArchiveImportContext
	authorize func() error
	language  func(int) string
	options   nativeparser.EditorOptions
	trees     []*parse.Tree
	links     map[string]string
	failure   error
	// Completion outlives candidate cleanup until enclosing ZIP/source resources close.
	completion []commitNotice
}

func captureMarkdownImportPreparation(ctx context.Context) (*markdownImportPreparation, error) {
	e, err := CaptureArchiveImport(ctx)
	if err != nil {
		return nil, err
	}
	var options nativeparser.EditorOptions
	err = e.binding.scope.commit.r.owner.WithSettingSnapshot(ctx, func(data conf.AppData) error {
		var err error
		options, err = data.ParserOptions(e.binding.scope.commit.r.owner.customEmojis.Snapshot())
		return err
	})
	if err != nil {
		return nil, err
	}
	m := newMarkdownImportPreparation(ctx, options, e.guard)
	m.archive = e
	m.language = e.language
	return m, m.guard()
}
func newMarkdownImportPreparation(ctx context.Context, options nativeparser.EditorOptions, authorize func() error) *markdownImportPreparation {
	emojis := make(map[string]string, len(options.Emojis))
	for name, value := range options.Emojis {
		emojis[name] = value
	}
	options.Emojis = emojis
	return &markdownImportPreparation{ctx: ctx, options: options, authorize: authorize, links: map[string]string{}}
}
func (m *markdownImportPreparation) guard() error {
	if m == nil || m.ctx == nil || m.authorize == nil {
		return errors.New("markdown_import_preparation_unbound")
	}
	if m.failure != nil {
		return m.failure
	}
	if err := m.ctx.Err(); err != nil {
		return err
	}
	return m.authorize()
}
func (m *markdownImportPreparation) discard() { m.trees = nil; m.links = nil }
func (m *markdownImportPreparation) walk(root *ast.Node, fn func(*ast.Node, bool) ast.WalkStatus) {
	ast.Walk(root, func(n *ast.Node, entering bool) ast.WalkStatus {
		if err := m.guard(); err != nil {
			m.failure = err
			return ast.WalkStop
		}
		return fn(n, entering)
	})
}
func (m *markdownImportPreparation) prepareLinks() (err error) {
	defer func() {
		if err != nil {
			if m != nil {
				m.failure = err
				m.discard()
			}
		}
	}()
	if err = m.guard(); err != nil {
		return err
	}
	for _, tree := range m.trees {
		if tree == nil || tree.Root == nil {
			return errors.New("markdown_import_tree_unbound")
		}
	}
	m.links = map[string]string{}
	for _, stage := range []func(){m.initSearchLinks, m.convertMdHyperlinks2WikiLinks, m.convertWikiLinksAndTags, m.mergeTextAndHandlerNestedInlines} {
		if err = m.guard(); err != nil {
			return err
		}
		stage()
		if err = m.guard(); err != nil {
			return err
		}
	}
	return nil
}

func (m *markdownImportPreparation) parse(markdown []byte) (ret *parse.Tree, yfmRootID, yfmTitle, yfmUpdated string, err error) {
	if err = m.guard(); err != nil {
		if m != nil {
			m.failure = err
			m.discard()
		}
		return
	}
	defer func() {
		if err == nil {
			err = m.guard()
		}
		if err != nil {
			m.failure = err
			m.discard()
			ret = nil
			yfmRootID, yfmTitle, yfmUpdated = "", "", ""
		}
	}()
	luteEngine := nativeparser.NewStdLute(m.options.Markdown)
	luteEngine.SetYamlFrontMatter(true) // 解析 YAML Front Matter https://github.com/siyuan-note/siyuan/issues/10878
	ret = parse.Parse("", markdown, luteEngine.ParseOptions)
	if nil == ret {
		return
	}
	yfmRootID, yfmTitle, yfmUpdated, err = doclife.NormalizeCreationTemplate(m.ctx, ret, doclife.TemplateBodyServices{NewID: ast.NewNodeID, Now: time.Now, Warning: func(e error) { logging.LogWarnf("normalize Markdown import: %s", e) }})
	if err != nil {
		return
	}
	htmlBlock2Inline(ret)
	parse.TextMarks2Inlines(ret) // 先将 TextMark 转换为 Inlines https://github.com/siyuan-note/siyuan/issues/13056
	parse.NestedInlines2FlattedSpansHybrid(ret, false)
	return
}

func (m *markdownImportPreparation) initSearchLinks() {
	for _, tree := range m.trees {
		m.walk(tree.Root, func(n *ast.Node, entering bool) ast.WalkStatus {
			if !entering || (ast.NodeDocument != n.Type && ast.NodeHeading != n.Type) {
				return ast.WalkContinue
			}

			nodePath := tree.HPath + "#"
			if ast.NodeHeading == n.Type {
				nodePath += n.Text()
			}

			m.links[nodePath] = n.ID
			return ast.WalkContinue
		})
	}
}

func (m *markdownImportPreparation) convertMdHyperlinks2WikiLinks() {
	// Supports converting relative path hyperlinks into document block references after importing Markdown https://github.com/siyuan-note/siyuan/issues/13817

	var unlinks []*ast.Node
	for _, tree := range m.trees {
		m.walk(tree.Root, func(n *ast.Node, entering bool) ast.WalkStatus {
			if !entering || ast.NodeTextMark != n.Type {
				return ast.WalkContinue
			}

			if "a" != n.TextMarkType {
				return ast.WalkContinue
			}

			linkText := n.TextMarkTextContent
			if "" == linkText {
				return ast.WalkContinue
			}
			linkDest := n.TextMarkAHref
			if "" == linkDest {
				return ast.WalkContinue
			}
			if strings.HasPrefix(linkDest, "assets/") {
				return ast.WalkContinue
			}
			if !strings.HasSuffix(linkDest, ".md") && !strings.HasSuffix(linkDest, ".markdown") {
				return ast.WalkContinue
			}
			linkDest = strings.TrimSuffix(linkDest, ".md")
			linkDest = strings.TrimSuffix(linkDest, ".markdown")

			buf := bytes.Buffer{}
			buf.WriteString("[[")
			buf.WriteString(linkDest)
			buf.WriteString("|")
			buf.WriteString(linkText)
			buf.WriteString("]]")

			wikilinkNode := &ast.Node{Type: ast.NodeText, Tokens: buf.Bytes()}
			n.InsertBefore(wikilinkNode)
			unlinks = append(unlinks, n)
			return ast.WalkContinue
		})
	}

	for _, n := range unlinks {
		n.Unlink()
	}
}

func (m *markdownImportPreparation) convertWikiLinksAndTags() {
	for _, tree := range m.trees {
		m.convertWikiLinksAndTags0(tree)
	}
}

func (m *markdownImportPreparation) convertWikiLinksAndTags0(tree *parse.Tree) {
	m.walk(tree.Root, func(n *ast.Node, entering bool) ast.WalkStatus {
		if !entering || ast.NodeText != n.Type {
			return ast.WalkContinue
		}

		text := n.TokensStr()
		length := len(text)
		start, end := 0, length
		for {
			part := text[start:end]
			if idx := strings.Index(part, "]]"); 0 > idx {
				break
			} else {
				end = start + idx
			}
			if idx := strings.Index(part, "[["); 0 > idx {
				break
			} else {
				start += idx
			}
			if end <= start {
				break
			}

			link := path.Join(path.Dir(tree.HPath), text[start+2:end]) // 统一转为绝对路径方便后续查找
			linkText := path.Base(link)
			dynamicAnchorText := true
			if linkParts := strings.Split(link, "|"); 1 < len(linkParts) {
				link = linkParts[0]
				linkText = linkParts[1]
				dynamicAnchorText = false
			}
			link, linkText = strings.TrimSpace(link), strings.TrimSpace(linkText)
			if !strings.Contains(link, "#") {
				link += "#" // 在结尾统一带上锚点方便后续查找
			}

			id := m.searchLinkID(link)
			if "" == id {
				start, end = end, length
				continue
			}

			linkText = strings.TrimPrefix(linkText, "/")
			repl := "((" + id + " '" + linkText + "'))"
			if !dynamicAnchorText {
				repl = "((" + id + " \"" + linkText + "\"))"
			}
			end += 2
			text = text[:start] + repl + text[end:]
			start, end = start+len(repl), len(text)
			length = end
		}

		text = convertTags(text, m.options.Markdown.InlineTag) // 导入标签语法
		n.Tokens = gulu.Str.ToBytes(text)
		return ast.WalkContinue
	})
}

func convertTags(text string, enabled bool) (ret string) {
	if !enabled {
		return text
	}

	pos, i := -1, 0
	tokens := []byte(text)
	for ; i < len(tokens); i++ {
		if '#' == tokens[i] && (0 == i || ' ' == tokens[i-1] || (-1 < pos && '#' == tokens[pos])) {
			if i < len(tokens)-1 && '#' == tokens[i+1] {
				pos = -1
				continue
			}
			pos = i
			continue
		}

		if -1 < pos && ' ' == tokens[i] {
			tokens = append(tokens, 0)
			copy(tokens[i+1:], tokens[i:])
			tokens[i] = '#'
			pos = -1
			i++
		}
	}
	if -1 < pos && pos < i {
		tokens = append(tokens, '#')
	}
	return string(tokens)
}

func (m *markdownImportPreparation) searchLinkID(link string) (id string) {
	id = m.links[link]
	if "" != id {
		return
	}

	baseName := path.Base(link)
	for searchLink, searchID := range m.links {
		if path.Base(searchLink) == baseName {
			return searchID
		}
	}
	return
}

func (m *markdownImportPreparation) mergeTextAndHandlerNestedInlines() {
	luteEngine := nativeparser.NewEditor(m.options)
	luteEngine.SetHTMLTag2TextMark(true)
	for _, tree := range m.trees {
		tree.MergeText()

		var unlinkTextNodes []*ast.Node
		m.walk(tree.Root, func(n *ast.Node, entering bool) ast.WalkStatus {
			if !entering || ast.NodeText != n.Type {
				return ast.WalkContinue
			}

			if nil == n.Tokens {
				return ast.WalkContinue
			}

			t := parse.Inline("", n.Tokens, luteEngine.ParseOptions) // 使用行级解析
			parse.NestedInlines2FlattedSpans(t, false)
			var children []*ast.Node
			for c := t.Root.FirstChild.FirstChild; nil != c; c = c.Next {
				children = append(children, c)
			}
			for _, c := range children {
				n.InsertBefore(c)
			}
			unlinkTextNodes = append(unlinkTextNodes, n)
			return ast.WalkContinue
		})

		for _, node := range unlinkTextNodes {
			node.Unlink()
		}
	}
}

// No late lookup of the selected process owner when admitting completion.
func (m *markdownImportPreparation) completeSync() error {
	if m == nil || m.archive == nil {
		return errors.New("markdown_import_source_unbound")
	}
	defer func() { m.completion = nil }()
	if err := m.guard(); err != nil {
		return err
	}
	return m.archive.binding.scope.commit.completeUI(m.ctx, m.completion, true)
}
