// SPDX-License-Identifier: AGPL-3.0-or-later
// Original filesys/stat.go counting rules; no source save or global parser/AV.
package docquery

import (
	"context"
	"errors"
	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"slices"
	"strings"
	av "wiki-kernel/internal/avmodel"
)

type StatResult struct {
	RuneCount  int `json:"runeCount"`
	WordCount  int `json:"wordCount"`
	LinkCount  int `json:"linkCount"`
	ImageCount int `json:"imageCount"`
	RefCount   int `json:"refCount"`
	BlockCount int `json:"blockCount"`
}
type StatRequest struct {
	Mode, Content string
	IDs           []string
}
type StatServices struct {
	Trees          func(context.Context, []string) (map[string]*parse.Tree, error)
	Parser         func(context.Context) (*lute.Lute, error)
	AttributeView  func(context.Context, string) (*av.AttributeView, error)
	IgnoreAVErrors bool
}

var ErrStatDependency = errors.New("document_statistics_dependencies_unbound")

func ContentStatistics(ctx context.Context, content string, p *lute.Lute) (*StatResult, error) {
	if ctx == nil || p == nil || p.ParseOptions == nil || p.RenderOptions == nil {
		return nil, ErrStatDependency
	}
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	tree := p.BlockDOM2Tree(content)
	if tree == nil || tree.Root == nil {
		return nil, errors.New("document_statistics_invalid_tree")
	}
	ret := nodeStatistics(tree.Root)
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	return ret, nil
}
func nodeStatistics(n *ast.Node) *StatResult {
	r, w, l, i, f := n.Stat()
	return &StatResult{RuneCount: r, WordCount: w, LinkCount: l, ImageCount: i, RefCount: f}
}

func statNode(tree *parse.Tree, id string) (ret *ast.Node) {
	ast.Walk(tree.Root, func(n *ast.Node, enter bool) ast.WalkStatus {
		if enter && n.ID == id {
			ret = n
			return ast.WalkStop
		}
		return ast.WalkContinue
	})
	return
}

func Statistics(ctx context.Context, q StatRequest, s StatServices) (*StatResult, error) {
	if ctx == nil {
		return nil, ErrStatDependency
	}
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	q.IDs = slices.Clone(q.IDs)
	if q.Mode == "content" {
		if s.Parser == nil {
			return nil, ErrStatDependency
		}
		p, e := s.Parser(ctx)
		if e != nil {
			return nil, e
		}
		return ContentStatistics(ctx, q.Content, p)
	}
	if q.Mode != "tree" && q.Mode != "block" && q.Mode != "blocks" {
		return nil, errors.New("document_statistics_mode")
	}
	if s.Trees == nil {
		return nil, ErrStatDependency
	}
	trees, e := s.Trees(ctx, slices.Clone(q.IDs))
	if e != nil {
		return nil, e
	}
	if e = ctx.Err(); e != nil {
		return nil, e
	}
	if q.Mode == "blocks" {
		ret := &StatResult{BlockCount: len(q.IDs)}
		for _, id := range q.IDs {
			if e = ctx.Err(); e != nil {
				return nil, e
			}
			tree := trees[id]
			if tree == nil {
				continue
			}
			if tree.Root == nil {
				return nil, errors.New("document_statistics_invalid_tree")
			}
			n := statNode(tree, id)
			if n == nil {
				continue
			}
			v := nodeStatistics(n)
			ret.RuneCount += v.RuneCount
			ret.WordCount += v.WordCount
			ret.LinkCount += v.LinkCount
			ret.ImageCount += v.ImageCount
			ret.RefCount += v.RefCount
		}
		if e = ctx.Err(); e != nil {
			return nil, e
		}
		return ret, nil
	}
	if len(q.IDs) == 0 || trees[q.IDs[0]] == nil {
		return nil, nil
	}
	tree := trees[q.IDs[0]]
	if tree.Root == nil {
		return nil, errors.New("document_statistics_invalid_tree")
	}
	if q.Mode == "block" {
		n := statNode(tree, q.IDs[0])
		if n == nil {
			return nil, nil
		}
		if n.Type != ast.NodeDocument {
			ret := nodeStatistics(n)
			ret.BlockCount = 1
			if e = ctx.Err(); e != nil {
				return nil, e
			}
			return ret, nil
		}
	}
	return TreeStatistics(ctx, tree, s)
}

func TreeStatistics(ctx context.Context, tree *parse.Tree, s StatServices) (*StatResult, error) {
	if ctx == nil || s.Parser == nil || s.AttributeView == nil {
		return nil, ErrStatDependency
	}
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	if tree == nil || tree.Root == nil {
		return nil, errors.New("document_statistics_invalid_tree")
	}
	count := 0
	var databases []*ast.Node
	ast.Walk(tree.Root, func(n *ast.Node, enter bool) ast.WalkStatus {
		if ctx.Err() != nil {
			return ast.WalkStop
		}
		if !enter || n == tree.Root {
			return ast.WalkContinue
		}
		if n.IsBlock() {
			count++
		}
		if n.Type == ast.NodeAttributeView {
			databases = append(databases, n)
		}
		return ast.WalkContinue
	})
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	p, e := s.Parser(ctx)
	if e != nil {
		return nil, e
	}
	dbStat := &StatResult{}
	for _, n := range databases {
		if e = ctx.Err(); e != nil {
			return nil, e
		}
		if n.AttributeViewID == "" {
			continue
		}
		a, readErr := s.AttributeView(ctx, n.AttributeViewID)
		if e = ctx.Err(); e != nil {
			return nil, e
		}
		if readErr != nil || a == nil {
			if s.IgnoreAVErrors {
				continue
			}
			if readErr != nil {
				return nil, readErr
			}
			return nil, ErrStatDependency
		}
		var content strings.Builder
		for _, kv := range a.KeyValues {
			if kv == nil || kv.Key == nil {
				return nil, errors.New("document_statistics_invalid_av")
			}
			for _, v := range kv.Values {
				if e = ctx.Err(); e != nil {
					return nil, e
				}
				if v.IsBlank() {
					continue
				}
				switch kv.Key.Type {
				case av.KeyTypeURL:
					if v.URL == nil {
						return nil, errors.New("document_statistics_invalid_av")
					}
					dbStat.LinkCount++
					content.WriteString(v.URL.Content)
				case av.KeyTypeMAsset:
					for _, asset := range v.MAsset {
						if asset == nil {
							return nil, errors.New("document_statistics_invalid_av")
						}
						if asset.Type == av.AssetTypeImage {
							dbStat.ImageCount++
						}
					}
				case av.KeyTypeBlock:
					if v.Block == nil {
						return nil, errors.New("document_statistics_invalid_av")
					}
					if !v.IsDetached {
						dbStat.RefCount++
					}
					content.WriteString(v.Block.Content)
				case av.KeyTypeText:
					if v.Text == nil {
						return nil, errors.New("document_statistics_invalid_av")
					}
					content.WriteString(v.Text.Content)
				case av.KeyTypeNumber:
					if v.Number == nil {
						return nil, errors.New("document_statistics_invalid_av")
					}
					number := *v.Number
					number.FormatNumber()
					content.WriteString(number.FormattedContent)
				case av.KeyTypeEmail:
					if v.Email == nil {
						return nil, errors.New("document_statistics_invalid_av")
					}
					content.WriteString(v.Email.Content)
				case av.KeyTypePhone:
					if v.Phone == nil {
						return nil, errors.New("document_statistics_invalid_av")
					}
					content.WriteString(v.Phone.Content)
				}
			}
		}
		v, e := ContentStatistics(ctx, content.String(), p)
		if e != nil {
			return nil, e
		}
		dbStat.RuneCount += v.RuneCount
		dbStat.WordCount += v.WordCount
	}
	ret := nodeStatistics(tree.Root)
	ret.BlockCount = count
	ret.RuneCount += dbStat.RuneCount
	ret.WordCount += dbStat.WordCount
	ret.LinkCount += dbStat.LinkCount
	ret.ImageCount += dbStat.ImageCount
	ret.RefCount += dbStat.RefCount
	if e = ctx.Err(); e != nil {
		return nil, e
	}
	return ret, nil
}
