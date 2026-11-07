// SiYuan - Refactor your thinking
// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
// Original sql/asset.go and treenode helpers with explicit dependencies/errors.
package indexdb

import (
	"context"
	"crypto/sha256"
	"fmt"
	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/html"
	"github.com/88250/lute/parse"
	"path"
	"sort"
	"strings"
)

type SpanRecord struct{ ID, BlockID, RootID, Box, Path, Content, Markdown, Type, IAL string }
type AssetRecord struct{ ID, BlockID, RootID, Box, DocPath, Path, Name, Title, Hash string }
type AttributeRecord struct{ ID, Name, Value, Type, BlockID, RootID, Box, Path string }

// TreeRecords owns detached values, never a mutable AST or source-write authority.
type TreeRecords struct {
	RootID, Box, Path string
	Blocks            []BlockRecord
	Spans             []SpanRecord
	Assets            []AssetRecord
	Attributes        []AttributeRecord
	Refs              []RefRecord
	Annotations       []AnnotationRecord
	IgnoreLines       []string
}

// TreeServices run synchronously under the owner's admission. PrepareOCR may
// populate owner-local OCR text, but must not save source or start unowned work.
// A missing service fails only when required. Disabled OCR binds an explicit no-op.
type TreeServices struct {
	Content          ContentServices
	AssetHash        func(context.Context, string, string, string) (string, error)
	PrepareOCR       func(context.Context, *ast.Node) error
	IndexAssetPath   bool
	LocalDefinitions bool
}
type treeProjector struct {
	ctx     context.Context
	parser  *lute.Lute
	options TreeServices
	err     error
}

func ProjectTree(ctx context.Context, tree *parse.Tree, parser *lute.Lute, options TreeServices, lookup ReferenceLookup) (projected TreeRecords, projectErr error) {
	// Malformed imported ASTs and failing borrowed services cannot publish partial
	// rows or terminate a CLI/server process through a renderer panic.
	defer func() {
		if value := recover(); value != nil {
			projected = TreeRecords{}
			projectErr = fmt.Errorf("%w: tree projection panic (%T)", ErrProjection, value)
		}
	}()
	if tree == nil || tree.Root == nil || tree.Root.Type != ast.NodeDocument || parser == nil || lookup == nil {
		return TreeRecords{}, ErrProjection
	}
	p := &treeProjector{ctx: ctx, parser: parser, options: options}
	blocks, spans, assets, attrs := p.fromTree(tree.Root, tree)
	if p.err != nil {
		return TreeRecords{}, p.err
	}
	if err := ctx.Err(); err != nil {
		return TreeRecords{}, err
	}
	ret := TreeRecords{RootID: tree.Root.ID, Box: tree.Box, Path: tree.Path}
	for _, v := range blocks {
		ret.Blocks = append(ret.Blocks, *v)
	}
	for _, v := range spans {
		ret.Spans = append(ret.Spans, *v)
	}
	for _, v := range assets {
		ret.Assets = append(ret.Assets, *v)
	}
	for _, v := range attrs {
		ret.Attributes = append(ret.Attributes, *v)
	}
	var err error
	definitionLookup := lookup
	if options.LocalDefinitions {
		local := map[string]BlockLocation{}
		for _, b := range ProjectBlockTree(tree) {
			local[b.ID] = BlockLocation{ParentID: b.ParentID, RootID: b.RootID, Path: b.Path}
		}
		definitionLookup = func(ctx context.Context, id string) (BlockLocation, bool, error) {
			if loc, ok := local[id]; ok {
				return loc, true, nil
			}
			return lookup(ctx, id)
		}
	}
	ret.Refs, ret.Annotations, err = ProjectReferences(ctx, tree, parser, definitionLookup)
	return ret, err
}
func (p *treeProjector) fail(err error) {
	if p.err == nil {
		p.err = err
	}
}
func (p *treeProjector) warn(msg string) {
	if p.options.Content.Warning != nil {
		p.options.Content.Warning(msg)
	}
}
func (p *treeProjector) markdown(n *ast.Node, format bool) string {
	var s string
	var err error
	if format {
		s, err = lute.FormatNodeSync(n, p.parser.ParseOptions, p.parser.RenderOptions)
	} else {
		s, err = lute.ProtyleExportMdNodeSync(n, p.parser.ParseOptions, p.parser.RenderOptions)
	}
	p.fail(err)
	return s
}
func (p *treeProjector) static(n *ast.Node, exclude []string, a, b, c bool) string {
	s, err := StaticContent(p.ctx, n, exclude, a, b, c, p.options.Content)
	p.fail(err)
	return s
}
func (p *treeProjector) created(id string) string {
	if len(id) < 14 {
		p.fail(ErrProjection)
		return ""
	}
	return id[:14]
}
func (p *treeProjector) assetHash(dest, box, doc string) string {
	if p.options.AssetHash == nil {
		p.fail(ErrProjection)
		return ""
	}
	s, err := p.options.AssetHash(p.ctx, dest, box, doc)
	p.fail(err)
	return s
}
func (p *treeProjector) ocr(n *ast.Node) {
	if n == nil {
		return
	}
	needed := false
	ast.Walk(n, func(x *ast.Node, e bool) ast.WalkStatus {
		if e && x.Type == ast.NodeImage && x.ChildByType(ast.NodeLinkDest) != nil {
			needed = true
			return ast.WalkStop
		}
		return ast.WalkContinue
	})
	if !needed {
		return
	}
	if p.options.PrepareOCR == nil {
		p.fail(ErrProjection)
		return
	}
	p.fail(p.options.PrepareOCR(p.ctx, n))
}
func (p *treeProjector) nodeHash(n *ast.Node, tree *parse.Tree) string {
	sort.Slice(n.KramdownIAL, func(i, j int) bool { return n.KramdownIAL[i][0] < n.KramdownIAL[j][0] })
	md := ""
	if n.Type != ast.NodeDocument {
		md = p.markdown(n, true)
	}
	parent := physicalParent(n)
	if h := headingParent(n); h != nil {
		parent = h.ID
	}
	data := tree.Box + tree.Path + tree.HPath + string(parse.IAL2Tokens(n.KramdownIAL)) + md + parent
	return fmt.Sprintf("%x", sha256.Sum256([]byte(data)))[:7]
}
func physicalParent(n *ast.Node) string {
	if n.Parent != nil {
		return n.Parent.ID
	}
	return ""
}
func ialString(n *ast.Node) string {
	if len(n.KramdownIAL) == 0 {
		return ""
	}
	return string(parse.IAL2Tokens(n.KramdownIAL))
}
func isAssetDest(dest []byte) bool { return strings.HasPrefix(string(dest), "assets/") }
func headingParent(n *ast.Node) *ast.Node {
	if n == nil {
		return nil
	}
	level := 16
	if n.Type == ast.NodeHeading {
		level = n.HeadingLevel
	}
	for x := n.Previous; x != nil; x = x.Previous {
		if x.Type == ast.NodeHeading && x.HeadingLevel < level {
			return x
		}
	}
	return n.Parent
}
func firstLeafBlock(n *ast.Node) (ret *ast.Node) {
	ast.Walk(n, func(x *ast.Node, e bool) ast.WalkStatus {
		if !e || x.IsMarker() {
			return ast.WalkContinue
		}
		if !x.IsContainerBlock() {
			ret = x
			return ast.WalkStop
		}
		return ast.WalkContinue
	})
	return
}
func subType(n *ast.Node) string {
	switch n.Type {
	case ast.NodeList, ast.NodeListItem:
		if n.ListData == nil {
			return ""
		}
		switch n.ListData.Typ {
		case 0:
			return "u"
		case 1:
			return "o"
		case 3:
			return "t"
		}
	case ast.NodeHeading:
		if n.HeadingLevel >= 1 && n.HeadingLevel <= 6 {
			return fmt.Sprint("h", n.HeadingLevel)
		}
	}
	return ""
}
func docTags(n *ast.Node) (ret []*SpanRecord) {
	if v := n.IALAttr("tags"); v != "" {
		for _, tag := range strings.Split(v, ",") {
			ret = append(ret, &SpanRecord{ID: ast.NewNodeID(), BlockID: n.ID, RootID: n.ID, Box: n.Box, Path: n.Path, Content: tag, Markdown: "#" + html.EscapeString(tag) + "#", Type: "tag"})
		}
	}
	return
}
func (p *treeProjector) titleAsset(n *ast.Node, box, doc string) *AssetRecord {
	const prefix = "background-image: url("
	v := strings.ReplaceAll(html.UnescapeString(strings.TrimSpace(n.IALAttr("title-img"))), "background-image:url(", prefix)
	start := strings.Index(v, prefix)
	if start < 0 {
		return nil
	}
	start += len(prefix)
	end := strings.LastIndex(v, ")")
	if end < start {
		p.fail(ErrProjection)
		return nil
	}
	dest := strings.TrimSuffix(strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(v[start:end], "\""), "'"), "\""), "'")
	if !strings.HasPrefix(dest, "assets/") {
		return nil
	}
	return &AssetRecord{ID: ast.NewNodeID(), BlockID: n.ID, RootID: n.ID, Box: box, DocPath: doc, Path: dest, Name: path.Base(dest), Title: "title-img", Hash: p.assetHash(dest, box, doc)}
}
