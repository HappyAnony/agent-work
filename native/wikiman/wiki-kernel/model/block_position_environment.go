package model

import (
	"context"
	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"github.com/siyuan-note/logging"
	"wiki-kernel/av"
	"wiki-kernel/internal/docquery"
	"wiki-kernel/sql"
	"wiki-kernel/treenode"
)

type blockPositionEnvironment struct {
	ctx      context.Context
	services docquery.PositionServices
}

func legacyBlockPositionEnvironment() *blockPositionEnvironment {
	return &blockPositionEnvironment{ctx: context.Background(), services: docquery.PositionServices{
		Load: func(_ context.Context, id string) (*parse.Tree, error) {
			tree, _ := LoadTreeByBlockID(id)
			return tree, nil
		},
		Notebook: func(_ context.Context, id string) (string, bool, error) {
			box := Conf.Box(id)
			if box == nil {
				return "", false, nil
			}
			return box.Name, true, nil
		},
		RootPath: func(_ context.Context, box, path string) (string, error) {
			base := treenode.GetBlockTreeRootByPath(box, path)
			if base == nil {
				return "", nil
			}
			return base.HPath, nil
		},
		AVName: func(_ context.Context, id string) (string, error) {
			name, _ := av.GetAttributeViewName(id)
			return name, nil
		},
		Text:   legacyPositionText,
		Parser: func(context.Context) (*lute.Lute, error) { return NewLute(), nil },
	}}
}
func legacyPositionText(ctx context.Context, n *ast.Node, exclude []string, lineBreak bool) (string, error) {
	return docquery.BlockText(ctx, n, exclude, lineBreak, func(_ context.Context, n *ast.Node, exclude []string) (string, error) {
		return sql.NodeStaticContent(n, exclude, false, false, false), nil
	})
}
func (tx *Transaction) blockPosition(q docquery.PositionRequest) (docquery.PositionResult, error) {
	if tx != nil && tx.environment != nil && !tx.environment.legacy {
		e := tx.environment.blockPosition
		if e == nil {
			return docquery.PositionResult{}, docquery.ErrPositionDependency
		}
		return docquery.Position(tx.environment.ctx, q, e.services)
	}
	e := legacyBlockPositionEnvironment()
	return docquery.Position(e.ctx, q, e.services)
}
func legacyNodeBreadcrumb(n *ast.Node, exclude []string, embed bool, headingMode ...int) []*BlockPath {
	e := legacyBlockPositionEnvironment()
	q := docquery.PositionRequest{ExcludeTypes: exclude, Embed: embed}
	if len(headingMode) > 0 {
		q.HeadingMode = headingMode[0]
	}
	if n == nil {
		return []*BlockPath{}
	}
	ret, err := docquery.BreadcrumbFromNode(e.ctx, n, n.Box, n.Path, q, e.services)
	if err != nil {
		logging.LogErrorf("block breadcrumb failed: %s", err)
		return []*BlockPath{}
	}
	return ret
}
