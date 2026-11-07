// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"github.com/88250/lute"
	"github.com/88250/lute/parse"
	"wiki-kernel/internal/docquery"
	"wiki-kernel/internal/indexdb"
)

func BlockPositionContext(ctx context.Context, q docquery.PositionRequest) (docquery.PositionResult, error) {
	q.IDs = append([]string(nil), q.IDs...)
	q.ExcludeTypes = append([]string(nil), q.ExcludeTypes...)
	return runWorkspaceRead(ctx, func(o *workspaceReadOwner, c context.Context) (docquery.PositionResult, error) {
		return docquery.Position(c, q, o.positionServices())
	})
}
func BlockRelationsContext(ctx context.Context, q docquery.RelationRequest) (docquery.RelationResult, error) {
	q.IDs = append([]string(nil), q.IDs...)
	return runWorkspaceRead(ctx, func(o *workspaceReadOwner, c context.Context) (docquery.RelationResult, error) {
		return docquery.Relations(c, q, docquery.RelationServices{Trees: o.trees, Load: o.loadOptional, BlockMissing: ErrBlockNotFound})
	})
}
func BlockFoldContext(ctx context.Context, q docquery.FoldRequest) (docquery.FoldResult, error) {
	return runWorkspaceRead(ctx, func(o *workspaceReadOwner, c context.Context) (docquery.FoldResult, error) {
		return docquery.Fold(c, q, docquery.FoldServices{Load: o.loadOptional})
	})
}
func DocumentInfosContext(ctx context.Context, q docquery.InfoRequest) ([]*BlockInfo, error) {
	q.IDs = append([]string(nil), q.IDs...)
	return runWorkspaceRead(ctx, func(o *workspaceReadOwner, c context.Context) ([]*BlockInfo, error) {
		return docquery.Infos(c, q, docquery.InfoServices{Wait: FlushTxQueueContext, Trees: o.trees,
			Exists: func(c context.Context, id string) (bool, error) {
				bt, e := o.r.referenceLookup(c, id)
				return bt != nil, e
			},
			References: func(c context.Context, id, root string) ([]string, error) {
				defs, _, e := o.referenceDefs(c, id, root)
				if e != nil {
					return nil, e
				}
				ids := make([]string, 0, len(defs))
				for _, d := range defs {
					ids = append(ids, d.RefID)
				}
				return ids, c.Err()
			},
			AVName: o.avName, DefaultAVName: func(c context.Context) (string, error) {
				if e := o.r.guard(c); e != nil {
					return "", e
				}
				name := o.base.owner.business.Language(o.settings.Lang, 105, false, o.settings.CloudRegion)
				if name == "" {
					return "", errors.New("document_info_language_missing")
				}
				return name, nil
			},
			Subfiles: func(c context.Context, t *parse.Tree) (int, error) { return o.subfiles(c, t, q.Batch) },
		})
	})
}
func GetDocInfoContext(ctx context.Context, id string) (*BlockInfo, error) {
	rows, e := DocumentInfosContext(ctx, docquery.InfoRequest{IDs: []string{id}})
	if e != nil {
		return nil, e
	}
	if len(rows) == 0 {
		return nil, ErrBlockNotFound
	}
	return rows[0], nil
}
func GetDocsInfoContext(ctx context.Context, ids []string, refs, avs bool) ([]*BlockInfo, error) {
	return DocumentInfosContext(ctx, docquery.InfoRequest{IDs: ids, Batch: true, References: refs, AV: avs})
}
func GetBlockIndexContext(ctx context.Context, id string) (int, error) {
	r, e := BlockPositionContext(ctx, docquery.PositionRequest{Mode: "index", ID: id})
	return r.Index, e
}
func GetBlocksIndexesContext(ctx context.Context, ids []string) (map[string]int, error) {
	r, e := BlockPositionContext(ctx, docquery.PositionRequest{Mode: "indexes", IDs: ids})
	return r.Indexes, e
}
func GetDOMTextContext(ctx context.Context, dom string) (string, error) {
	r, e := BlockPositionContext(ctx, docquery.PositionRequest{Mode: "dom-text", Content: dom})
	return r.Text, e
}
func BuildBlockBreadcrumbContext(ctx context.Context, id string, exclude []string) ([]*BlockPath, error) {
	r, e := BlockPositionContext(ctx, docquery.PositionRequest{Mode: "breadcrumb", ID: id, ExcludeTypes: exclude})
	return r.Breadcrumb, e
}
func GetBlockTreeInfosContext(ctx context.Context, ids []string) (map[string]*BlockTreeInfo, error) {
	r, e := BlockRelationsContext(ctx, docquery.RelationRequest{Mode: "infos", IDs: ids})
	return r.Infos, e
}
func GetBlockSiblingIDContext(ctx context.Context, id string) (string, string, string, error) {
	r, e := BlockRelationsContext(ctx, docquery.RelationRequest{Mode: "sibling", ID: id})
	return r.IDs.Parent, r.IDs.Previous, r.IDs.Next, e
}
func GetBlockRelevantIDsContext(ctx context.Context, id string) (string, string, string, error) {
	r, e := BlockRelationsContext(ctx, docquery.RelationRequest{Mode: "relevant", ID: id})
	return r.IDs.Parent, r.IDs.Previous, r.IDs.Next, e
}
func GetUnfoldedParentIDContext(ctx context.Context, id string) (string, error) {
	r, e := BlockFoldContext(ctx, docquery.FoldRequest{Mode: "unfolded-parent", ID: id})
	return r.ParentID, e
}
func IsBlockFoldedContext(ctx context.Context, id string) (bool, bool, error) {
	r, e := BlockFoldContext(ctx, docquery.FoldRequest{Mode: "state", ID: id})
	return r.IsFolded, r.IsRoot, e
}

func (o *workspaceReadOwner) positionServices() docquery.PositionServices {
	return docquery.PositionServices{Load: o.loadOptional, Text: o.text, AVName: o.avName, Parser: func(c context.Context) (*lute.Lute, error) { return o.parser, o.r.guard(c) },
		Notebook: func(c context.Context, id string) (string, bool, error) {
			books, e := o.base.owner.business.Notebooks(c)
			if e != nil {
				return "", false, e
			}
			for _, book := range books {
				if book.ID == id {
					return book.Name, true, c.Err()
				}
			}
			return "", false, c.Err()
		},
		RootPath: func(c context.Context, box, p string) (string, error) {
			if e := o.r.guard(c); e != nil {
				return "", e
			}
			rows, e := o.r.registry.Query(c, indexdb.BlockTreeFilter{Kind: "root-path", BoxID: box, Value: p})
			if e != nil {
				return "", e
			}
			if len(rows) == 0 {
				return "", nil
			}
			return rows[0].HPath, c.Err()
		},
	}
}
