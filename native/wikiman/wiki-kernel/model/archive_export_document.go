// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"github.com/88250/lute/parse"
	"path"
	"wiki-kernel/internal/indexrepair"
	"wiki-kernel/util"
)

// Raw bytes and the detached candidate come from one Load, not two source reads.
// AV records have a separate raw cache; attachments stream independently.
type archiveExportDocument struct {
	tree          *parse.Tree
	id, box, path string
	raw           []byte
}

func (d archiveExportDocument) check(tree *parse.Tree, box, p string) error {
	if tree == nil || tree.Root == nil || d.tree != tree || d.raw == nil || d.box != box || d.path != p || tree.Box != box || tree.Path != p || tree.ID != d.id || tree.Root.ID != d.id || tree.Root.Box != box || tree.Root.Path != p {
		return errors.New("export_document_identity_changed")
	}
	return nil
}
func (e *archiveExportContext) writeDocument(folder, relative string, tree *parse.Tree) error {
	if err := e.guard(); err != nil {
		return err
	}
	if tree == nil {
		return errors.New("export_document_unbound")
	}
	snapshot, ok := e.documents[tree.ID]
	if !ok {
		return errors.New("export_document_unbound")
	}
	if err := snapshot.check(tree, tree.Box, tree.Path); err != nil {
		return err
	}
	if _, err := e.box(snapshot.box); err != nil {
		return err
	}
	if path.Base(relative) != snapshot.id+util.DocExt {
		return indexrepair.ErrSourcePath
	}
	return e.writeExportBytes(folder, relative, snapshot.raw)
}
func (e *archiveExportContext) writeExportBytes(folder, relative string, raw []byte) (err error) {
	if err := e.guard(); err != nil {
		return err
	}
	root, closeRoot, err := e.openExportIO(folder)
	if err != nil {
		return err
	}
	defer func() { err = joinRepairCloseError(err, closeRoot()) }()
	target := indexrepair.Source{Data: root, Authorize: func(ctx context.Context) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		return e.guard()
	}}
	before, err := target.Read(e.binding.scope.ctx, relative)
	if err != nil && !indexrepair.IsSourceNotExist(err) {
		return err
	}
	return target.Rewrite(e.binding.scope.ctx, relative, before, raw)
}
