// SPDX-License-Identifier: AGPL-3.0-or-later
package workspace

import (
	"context"
	"time"

	"github.com/88250/lute/ast"
	"wiki-kernel/internal/doclife"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/textprefs"
)

func (l *Lease) PlanNativeStructure(ctx context.Context, request doclife.StructureRequest) (out doclife.StructurePlan, err error) {
	err = l.WithNativeIndexes(ctx, func(b *indexdb.Bundle) error {
		r, w := l.workspace.resources, l.workspace
		base, inputs, guard, err := r.documentLifecycleServices(ctx, w, b, "")
		if err != nil {
			return err
		}
		registry, err := b.Database(indexdb.BlockTree)
		if err != nil {
			return err
		}
		parser, err := base.Parser(ctx)
		if err != nil {
			return err
		}
		services := doclife.StructureServices{Lifecycle: base, Body: doclife.TemplateBodyServices{NewID: base.NewID, Now: time.Now},
			ByPath: func(c context.Context, box, p string) (doclife.Location, bool, error) {
				rows, err := (indexdb.BlockTrees{DB: registry}).Query(c, indexdb.BlockTreeFilter{Kind: "root-path", BoxID: box, Value: p})
				if err != nil || len(rows) == 0 {
					return doclife.Location{}, false, err
				}
				row := rows[0]
				return doclife.Location{Box: row.BoxID, Path: row.Path, HPath: row.HPath, ID: row.ID, Document: true}, true, nil
			},
			Text: func(c context.Context, node *ast.Node) (string, error) {
				if r.configuration.Editor == nil {
					return "", ErrDocument
				}
				return doclife.HeadingTitle(c, node, r.configuration.Editor.BlockRefDynamicAnchorTextMaxLen, r.readBlockContent(c, w))
			},
			ListText: func(c context.Context, node *ast.Node) (string, error) {
				title, err := indexdb.ContainerText(c, node, func(c context.Context, id string) (string, error) {
					return indexdb.ReferenceText(c, id, parser, r.nativeBlock)
				})
				return textprefs.FilterFileName(title), err
			},
			CanRemove: func(c context.Context, loc doclife.Location) error {
				children, err := base.Children(c, loc.Box, loc.Path)
				if err == nil && len(children) > 0 {
					return ErrDocument
				}
				return err
			},
		}
		out, err = doclife.PrepareStructure(ctx, request, services)
		if err == nil {
			err = guard.Confirm(ctx)
		}
		if err == nil {
			out.SourceHashes = inputs
		}
		return err
	})
	if err != nil {
		return doclife.StructurePlan{}, err
	}
	return
}
