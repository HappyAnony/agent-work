// SPDX-License-Identifier: AGPL-3.0-or-later
// Original GetDoc model algorithm with explicit owner services.
package docview

import (
	"bytes"
	"context"
	"errors"
	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"maps"
	"slices"
)

type Request struct {
	StartID, EndID, ID, Query      string
	Index, QueryMethod, Mode, Size int
	QueryTypes                     map[string]bool
	OriginalRefs                   map[string]string
	Backlink, Highlight            bool
	UseDynamicSize                 bool
}
type Result struct {
	BlockCount                                        int
	DOM, ParentID, Parent2ID, RootID, Type, Box, Path string
	EOF, Scroll, BacklinkExpand                       bool
	Keywords                                          []string
}
type Recent struct{ ID, Icon, Title, Box, Path, HPath string }

// Native callers must hold their owner admission and exclusive candidate-tree
// access for this call. These callbacks receive the outer request context; no
// default loader, SQL connection, renderer or writer is installed implicitly.
type Services struct {
	Ctx                       context.Context
	Legacy, CaseSensitive     bool
	DynamicLoadBlocks         int
	BlockMissing, TreeMissing error
	Wait                      func(context.Context) error
	Load                      func(context.Context, string) (*parse.Tree, error)
	Parser                    func(context.Context) (*lute.Lute, error)
	Backlink                  func(context.Context, *ast.Node, map[string]string) ([]*ast.Node, bool, error)
	References                func(context.Context, string) (map[string]int, error)
	VirtualKeywords           func(context.Context, *ast.Node) ([]string, error)
	Keywords                  func(context.Context, string, map[string]bool, int, string) ([]string, error)
	AvNames                   func(context.Context, string) (string, error)
	Virtual                   func(context.Context, *ast.Node, *[]*ast.Node, []string, map[string]int, *lute.Lute) (bool, error)
	Render                    func(context.Context, *parse.Tree, *lute.Lute) (string, error)
	Recent                    func(context.Context, Recent) error
}

func (e *Services) guard(fn func() error) error {
	if err := e.Ctx.Err(); err != nil {
		return err
	}
	if err := fn(); err != nil {
		return err
	}
	return e.Ctx.Err()
}
func (e *Services) validate() error {
	if e == nil || e.Ctx == nil || e.Wait == nil || e.Load == nil || e.Parser == nil || e.Backlink == nil || e.References == nil || e.VirtualKeywords == nil || e.Keywords == nil || e.AvNames == nil || e.Virtual == nil || e.Render == nil || e.Recent == nil || e.BlockMissing == nil || e.TreeMissing == nil {
		return errors.New("get_doc_dependencies_unbound")
	}
	return e.Ctx.Err()
}
func Read(q Request, e *Services) (r Result, err error) {
	if err = e.validate(); err != nil {
		return Result{}, err
	}
	q.QueryTypes = maps.Clone(q.QueryTypes)
	if q.UseDynamicSize {
		q.Size = e.DynamicLoadBlocks
	}
	q.OriginalRefs = maps.Clone(q.OriginalRefs)
	if !e.Legacy {
		defer func() {
			if recover() != nil {
				r = Result{}
				err = errors.New("get_doc_invalid_view")
			}
		}()
	}
	r.BlockCount, r.DOM, r.ParentID, r.Parent2ID, r.RootID, r.Type, r.EOF, r.Scroll, r.Box, r.Path, r.BacklinkExpand, r.Keywords, err = getDocCore(q.StartID, q.EndID, q.ID, q.Index, q.Query, q.QueryTypes, q.QueryMethod, q.Mode, q.Size, q.Backlink, q.OriginalRefs, q.Highlight, e)
	if err != nil {
		return Result{}, err
	}
	if err = e.Ctx.Err(); err != nil {
		return Result{}, err
	}
	r.Keywords = slices.Clone(r.Keywords)
	return
}
func snapshotGetDocTree(tree *parse.Tree) func() {
	before := *tree
	before.Marks = slices.Clone(tree.Marks)
	nodes := map[*ast.Node]ast.Node{}
	lists := map[*ast.ListData]ast.ListData{}
	ast.Walk(tree.Root, func(n *ast.Node, enter bool) ast.WalkStatus {
		if enter {
			copy := *n
			copy.Tokens = bytes.Clone(n.Tokens)
			copy.CodeBlockOpenFence = bytes.Clone(n.CodeBlockOpenFence)
			copy.CodeBlockInfo = bytes.Clone(n.CodeBlockInfo)
			copy.CodeBlockCloseFence = bytes.Clone(n.CodeBlockCloseFence)
			copy.TableAligns = slices.Clone(n.TableAligns)
			copy.LinkRefLabel = bytes.Clone(n.LinkRefLabel)
			copy.FootnotesRefLabel = bytes.Clone(n.FootnotesRefLabel)
			copy.FootnotesRefs = slices.Clone(n.FootnotesRefs)
			copy.HtmlEntityTokens = bytes.Clone(n.HtmlEntityTokens)
			if n.ListData != nil {
				list := *n.ListData
				list.Marker = bytes.Clone(n.ListData.Marker)
				lists[n.ListData] = list
			}
			copy.Children = slices.Clone(n.Children)
			copy.Properties = maps.Clone(n.Properties)
			if n.KramdownIAL != nil {
				copy.KramdownIAL = make([][]string, len(n.KramdownIAL))
				for i, v := range n.KramdownIAL {
					copy.KramdownIAL[i] = slices.Clone(v)
				}
			}
			nodes[n] = copy
		}
		return ast.WalkContinue
	})
	return func() {
		for list, before := range lists {
			*list = before
		}
		for n, copy := range nodes {
			*n = copy
		}
		*tree = before
	}
}
