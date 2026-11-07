// SiYuan - Refactor your thinking
// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package sql

import (
	"context"
	"github.com/88250/gulu"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"github.com/siyuan-note/logging"
	"path/filepath"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/treenode"
	"wiki-kernel/util"
)

// Only the legacy adapter binds global path/cache/OCR functions. Both actual
// compat tree index/upsert and Context execute the same extracted AST walk.
func (p *treeProjection) fullRows(tree *parse.Tree) (blocks []*Block, spans []*Span, assets []*Asset, attrs []*Attribute, refs []*Ref, annotations []*FileAnnotationRef, err error) {
	rows, err := indexdb.ProjectTree(context.Background(), tree, p.parser, indexdb.TreeServices{
		Content: legacyContentServices(), IndexAssetPath: indexAssetPath,
		PrepareOCR: func(_ context.Context, n *ast.Node) error {
			if !treenode.IsNodeOCRed(n) {
				util.PushNodeOCRQueue(n)
			}
			return nil
		},
		AssetHash: func(_ context.Context, dest, box, doc string) (string, error) {
			boxPath := filepath.Join(util.DataDir, box)
			local := assetLocalPath(dest, boxPath, filepath.Join(boxPath, doc))
			if local == "" || gulu.File.IsDir(local) {
				return "", nil
			}
			hash, err := util.GetEtag(local)
			if err != nil {
				logging.LogErrorf("calc asset [%s] hash failed: %s", local, err)
			}
			return hash, err
		},
	}, func(_ context.Context, id string) (indexdb.BlockLocation, bool, error) {
		b := treenode.GetBlockTree(id)
		if b == nil {
			return indexdb.BlockLocation{}, false, nil
		}
		return indexdb.BlockLocation{ParentID: b.ParentID, RootID: b.RootID, Path: b.Path}, true, nil
	})
	if err != nil {
		return nil, nil, nil, nil, nil, nil, err
	}
	for _, v := range rows.Blocks {
		v := Block(v)
		blocks = append(blocks, &v)
	}
	for _, v := range rows.Spans {
		v := Span(v)
		spans = append(spans, &v)
	}
	for _, v := range rows.Assets {
		v := Asset(v)
		assets = append(assets, &v)
	}
	for _, v := range rows.Attributes {
		v := Attribute(v)
		attrs = append(attrs, &v)
	}
	for _, v := range rows.Refs {
		v := v
		refs = append(refs, &v)
	}
	for _, v := range rows.Annotations {
		v := v
		annotations = append(annotations, &v)
	}
	return
}
