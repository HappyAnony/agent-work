// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"github.com/88250/lute/ast"
	"net/url"
	"os"
	"path"
	"strings"
	"wiki-kernel/av"
	"wiki-kernel/util"
)

func ExportAv2CSVContext(ctx context.Context, avID, blockID string) (string, error) {
	m, err := captureMarkdownExport(ctx)
	if err != nil {
		return "", err
	}
	return m.exportCSV(avID, blockID)
}
func (m *markdownExportContext) exportCSV(avID, blockID string) (zipURL string, err error) {
	if err = m.guard(); err != nil {
		return "", err
	}
	if !ast.IsNodeIDPattern(avID) || !ast.IsNodeIDPattern(blockID) {
		return "", errors.New("csv_export_selection_invalid")
	}
	tree, err := m.embeddedTree(blockID)
	if err != nil {
		return "", err
	}
	var node *ast.Node
	ast.Walk(tree.Root, func(n *ast.Node, enter bool) ast.WalkStatus {
		if enter && n.ID == blockID {
			node = n
			return ast.WalkStop
		}
		return ast.WalkContinue
	})
	if node == nil || node.Type != ast.NodeAttributeView || node.AttributeViewID != avID {
		return "", errors.New("csv_export_block_mismatch")
	}
	a, table, err := m.exportAttributeTable(avID, node.IALAttr(av.NodeAttrView))
	if err != nil {
		return "", err
	}
	if a == nil || table == nil {
		return "", errors.New("csv_export_source_missing")
	}
	name := strings.TrimSpace(a.Name)
	if name == "" {
		name = m.read.binding.scope.commit.avUntitled
	}
	o, err := m.read.newArchiveOutput(name)
	if err != nil {
		return "", err
	}
	defer func() {
		err = errors.Join(err, o.finish(err == nil && zipURL != ""))
		if err != nil {
			zipURL = ""
		}
	}()
	name = o.prefix
	o.prefix = "" // Original CSV ZIP contains a single flat member.
	o.filename = strings.TrimSuffix(o.filename, util.DocZipExt) + ".db.zip"
	if err = o.useSubdirectory("csv"); err != nil {
		return "", err
	}
	f, err := o.payload.OpenFile(name+".csv", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return "", err
	}
	err = writeAttributeViewCSV(f, a, table, m.guard)
	if err == nil {
		err = f.Sync()
	}
	err = errors.Join(err, f.Close())
	if err != nil {
		return "", err
	}
	if err = o.pack(); err != nil {
		return "", err
	}
	if err = m.guard(); err != nil {
		return "", err
	}
	return "/export/csv/" + url.PathEscape(path.Base(o.filename)), nil
}
