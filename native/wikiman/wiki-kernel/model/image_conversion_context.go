// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"image"
	"image/jpeg"
	"image/png"
	"regexp"
	"strings"

	"github.com/88250/gulu"
	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"github.com/88250/lute/render"
	"github.com/siyuan-note/logging"
	"wiki-kernel/util"
)

// HTML conversion has no external source folder; it shares the same captured
// output/root/close policy as local Markdown imports without inventing a source.
func withImageConversionAssets(ctx context.Context, work func(*markdownImportAssets) error) (err error) {
	e, err := CaptureArchiveImport(ctx)
	if err != nil {
		return err
	}
	return e.withImageConversionAssets(work)
}
func (e *ArchiveImportContext) withImageConversionAssets(work func(*markdownImportAssets) error) (err error) {
	if err = e.guard(); err != nil {
		return err
	}
	ctx := e.ctx
	ioctx, closeIO, err := e.binding.scope.commit.r.openRepairIOCheckedWithCloser(ctx, false, e.archiveRootCloser())
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, closeIO()) }()
	identity, err := ioctx.Source.Data.Stat(".")
	if err != nil {
		return err
	}
	a := &markdownImportAssets{authority: e, target: ioctx.Source, identity: identity, close: closeIO}
	if err = a.guard(); err != nil {
		return err
	}
	if err = work(a); err != nil {
		return err
	}
	return a.guard()
}
func (a *markdownImportAssets) base64Image(n *ast.Node, dest, folder string) error {
	if err := a.guard(); err != nil {
		return err
	}
	if n == nil || (n.Type != ast.NodeLinkDest && !n.IsTextMarkType("a")) {
		return errors.New("invalid_base64_image_node")
	}
	sep := strings.Index(dest, ";base64,")
	if n == nil || sep < 5 || !strings.HasPrefix(dest, "data:image") {
		return errors.New("invalid_base64_image")
	}
	payload := strings.TrimSpace(dest[sep+8:])
	payload = regexp.MustCompile(`(?i)%0A`).ReplaceAllString(payload, "\n")
	raw, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return err
	}
	if err = a.guard(); err != nil {
		return err
	}
	typ := dest[5:sep]
	var img image.Image
	ext := ""
	switch typ {
	case "image/png":
		img, err = png.Decode(bytes.NewReader(raw))
		ext = ".png"
	case "image/jpeg":
		img, err = jpeg.Decode(bytes.NewReader(raw))
		ext = ".jpg"
	case "image/svg+xml":
		ext = ".svg"
	default:
		return nil // Original unsupported MIME policy: retain the data URI.
	}
	if err != nil {
		return err
	}
	if err = a.guard(); err != nil {
		return err
	}
	var output bytes.Buffer
	writer := markdownAssetWriter{Writer: &output, guard: a.guard}
	switch typ {
	case "image/png":
		err = png.Encode(writer, img)
	case "image/jpeg":
		err = jpeg.Encode(writer, img, &jpeg.Options{Quality: 100})
	case "image/svg+xml":
		err = archiveCopy(a.authority.ctx, writer, bytes.NewReader(raw))
	}
	if err != nil {
		return err
	}
	if err = a.guard(); err != nil {
		return err
	}
	name := "image" + ext
	if n.Parent != nil {
		if alt := n.Parent.ChildByType(ast.NodeLinkText); alt != nil {
			name = alt.TokensStr() + ext
		}
	}
	name = util.AssetName(util.FilterUploadFileName(name), ast.NewNodeID())
	if err = a.publish(folder, name, bytes.NewReader(output.Bytes()), a.guard, func() error { return nil }); err != nil {
		return err
	}
	if n.Type == ast.NodeLinkDest {
		n.Tokens = []byte("assets/" + name)
	} else if n.IsTextMarkType("a") {
		n.TextMarkAHref = "assets/" + name
	} else {
		return errors.New("invalid_base64_image_node")
	}
	return nil
}
func (a *markdownImportAssets) htmlBlockSVG(n *ast.Node, folder string) error {
	if err := a.guard(); err != nil {
		return err
	}
	if n == nil {
		return errors.New("invalid_svg_image_node")
	}
	matches := regexp.MustCompile(`(?i)<svg[^>]*>(.*?)</svg>`).FindSubmatch(n.Tokens)
	if len(matches) < 2 {
		return nil
	}
	name := util.AssetName("image.svg", ast.NewNodeID())
	if err := a.publish(folder, name, bytes.NewReader(matches[0]), a.guard, func() error { return nil }); err != nil {
		return err
	}
	n.Type = ast.NodeParagraph
	img := &ast.Node{Type: ast.NodeImage}
	for _, child := range []*ast.Node{{Type: ast.NodeBang}, {Type: ast.NodeOpenBracket}, {Type: ast.NodeLinkText, Tokens: []byte("image")}, {Type: ast.NodeCloseBracket}, {Type: ast.NodeOpenParen}, {Type: ast.NodeLinkDest, Tokens: []byte("assets/" + name)}, {Type: ast.NodeCloseParen}} {
		img.AppendChild(child)
	}
	n.AppendChild(img)
	return nil
}
func (a *markdownImportAssets) htmlTree(htmlStr string, engine *lute.Lute) (tree *parse.Tree, withMath bool, err error) {
	if engine == nil {
		return nil, false, errors.New("html_parser_unbound")
	}
	if err = a.guard(); err != nil {
		return
	}
	tree = engine.HTML2Tree(gulu.Str.RemovePUA(htmlStr))
	if tree == nil || tree.Root == nil {
		return nil, false, errors.New("html_tree_unbound")
	}
	ast.Walk(tree.Root, func(n *ast.Node, enter bool) ast.WalkStatus {
		if err = a.guard(); err != nil {
			return ast.WalkStop
		}
		if !enter {
			return ast.WalkContinue
		}
		switch n.Type {
		case ast.NodeHTMLBlock:
			if bytes.HasPrefix(n.Tokens, []byte("<pre ")) && bytes.HasSuffix(n.Tokens, []byte("</pre>")) {
				if bytes.Contains(n.Tokens, []byte("data:image/svg+xml;base64")) {
					matches := regexp.MustCompile(`(?sU)<pre [^>]*>(.*)</pre>`).FindSubmatch(n.Tokens)
					if len(matches) >= 2 {
						n.Tokens = matches[1]
					}
					sub := parse.Inline("", n.Tokens, engine.ParseOptions)
					if sub != nil && sub.Root != nil && sub.Root.FirstChild != nil {
						n.Type = ast.NodeParagraph
						var children []*ast.Node
						for c := sub.Root.FirstChild.FirstChild; c != nil; c = c.Next {
							children = append(children, c)
						}
						for _, c := range children {
							n.AppendChild(c)
						}
					}
				} else if bytes.Contains(n.Tokens, []byte("<svg")) {
					if err = a.htmlBlockSVG(n, "assets"); err != nil {
						return ast.WalkStop
					}
				}
			}
		case ast.NodeText:
			if n.ParentIs(ast.NodeTableCell) {
				n.Tokens = bytes.ReplaceAll(n.Tokens, []byte("\\|"), []byte("|"))
				n.Tokens = bytes.ReplaceAll(n.Tokens, []byte("|"), []byte("\\|"))
				n.Tokens = bytes.ReplaceAll(n.Tokens, []byte("\\<br /\\>"), []byte("<br />"))
			}
		case ast.NodeInlineMath:
			withMath = true
		case ast.NodeLinkDest:
			dest := n.TokensStr()
			if strings.HasPrefix(dest, "data:image") && strings.Contains(dest, ";base64,") {
				if err = a.base64Image(n, dest, "assets"); err != nil {
					return ast.WalkStop
				}
			}
		}
		return ast.WalkContinue
	})
	if err != nil {
		return nil, false, err
	}
	return tree, withMath, a.guard()
}

// Compatibility shape retained; all in-repository callers use the error-valued
// handler below. No legacy filesystem conversion backend remains in this wrapper.
func HTML2Tree(htmlStr string, engine *lute.Lute) (*parse.Tree, bool) {
	tree, math, err := HTML2TreeWithContext(context.Background(), htmlStr, engine)
	if err != nil {
		logging.LogErrorf("convert HTML images failed: %s", err)
	}
	return tree, math
}
func HTML2TreeWithContext(ctx context.Context, htmlStr string, engine *lute.Lute) (tree *parse.Tree, withMath bool, err error) {
	err = withImageConversionAssets(ctx, func(a *markdownImportAssets) error {
		var err error
		tree, withMath, err = a.htmlTree(htmlStr, engine)
		return err
	})
	if err != nil {
		return nil, false, err
	}
	return
}
func HTML2Markdown(htmlStr string, engine *lute.Lute, contexts ...context.Context) (markdown string, withMath bool, err error) {
	err = withImageConversionAssets(legacyAVContext(contexts), func(a *markdownImportAssets) error {
		tree, math, err := a.htmlTree(htmlStr, engine)
		if err != nil {
			return err
		}
		withMath = math
		renderer := render.NewFormatRenderer(tree, engine.RenderOptions)
		for kind, fn := range engine.HTML2MdRendererFuncs {
			renderer.ExtRendererFuncs[kind] = fn
		}
		markdown = gulu.Str.FromBytes(renderer.Render())
		return a.guard()
	})
	if err != nil {
		return "", false, err
	}
	return
}
