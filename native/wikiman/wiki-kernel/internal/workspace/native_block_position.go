package workspace

import (
	"context"
	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"slices"
	"wiki-kernel/internal/docquery"
	"wiki-kernel/internal/indexdb"
)

func (l *Lease) NativeBlockIndex(ctx context.Context, id string) (int, error) {
	r, e := l.NativeBlockPosition(ctx, docquery.PositionRequest{Mode: "index", ID: id})
	return r.Index, e
}
func (l *Lease) NativeBlocksIndexes(ctx context.Context, ids []string) (map[string]int, error) {
	r, e := l.NativeBlockPosition(ctx, docquery.PositionRequest{Mode: "indexes", IDs: ids})
	return r.Indexes, e
}
func (l *Lease) NativeBlockBreadcrumb(ctx context.Context, id string, exclude []string, embed bool, headingMode int) ([]*docquery.BlockPath, error) {
	r, e := l.NativeBlockPosition(ctx, docquery.PositionRequest{Mode: "breadcrumb", ID: id, ExcludeTypes: exclude, Embed: embed, HeadingMode: headingMode})
	return r.Breadcrumb, e
}

// No main flush, nested Lease calls, source repair or default notebook/parser.
// Root HPath is read from this owner's registry (not synthesized from a title).
func (l *Lease) NativeBlockPosition(ctx context.Context, q docquery.PositionRequest) (ret docquery.PositionResult, err error) {
	q.IDs = slices.Clone(q.IDs)
	q.ExcludeTypes = slices.Clone(q.ExcludeTypes)
	err = l.WithNativeIndexes(ctx, func(bundle *indexdb.Bundle) error {
		r, w := l.workspace.resources, l.workspace
		s := r.positionServices(w, bundle)
		var e error
		ret, e = docquery.Position(ctx, q, s)
		return e
	})
	if err != nil {
		return docquery.PositionResult{}, err
	}
	return
}

func (r *resources) positionServices(w *WorkspaceContext, bundle *indexdb.Bundle) docquery.PositionServices {
	content := r.nativeContent
	content.AttributeView = func(ctx context.Context, id string, _ bool) (string, error) {
		name, _, e := r.readNativeAV(ctx, w, id)
		return name, e
	}
	return docquery.PositionServices{
		Load: func(ctx context.Context, id string) (*parse.Tree, error) {
			trees, e := r.documentInfoTrees(ctx, w, []string{id})
			if e != nil {
				return nil, e
			}
			return trees[id], nil
		},
		Notebook: func(ctx context.Context, id string) (string, bool, error) {
			boxes, e := r.configurationBusiness.Notebooks(ctx)
			if e != nil {
				return "", false, e
			}
			for _, b := range boxes {
				if b.ID == id {
					return b.Name, true, nil
				}
			}
			return "", false, ctx.Err()
		},
		RootPath: func(ctx context.Context, box, path string) (string, error) {
			db, e := bundle.Database(indexdb.BlockTree)
			if e != nil {
				return "", e
			}
			rows, e := (indexdb.BlockTrees{DB: db}).Query(ctx, indexdb.BlockTreeFilter{Kind: "root-path", BoxID: box, Value: path})
			if e != nil {
				return "", e
			}
			if len(rows) == 0 {
				return "", nil
			}
			return rows[0].HPath, nil
		},
		AVName: func(ctx context.Context, id string) (string, error) {
			name, _, e := r.readNativeAV(ctx, w, id)
			return name, e
		},
		Text: func(ctx context.Context, n *ast.Node, exclude []string, lineBreak bool) (string, error) {
			return docquery.BlockText(ctx, n, exclude, lineBreak, func(ctx context.Context, n *ast.Node, exclude []string) (string, error) {
				return indexdb.StaticContent(ctx, n, exclude, false, false, false, content)
			})
		},
		Parser: func(ctx context.Context) (*lute.Lute, error) { return r.documentInfoParser(ctx, w) },
	}
}
