// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package docview

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"

	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"github.com/88250/lute/render"
)

// PrepareWriteTree is the original empty-document/Spec/document-type preparation.
// It changes only a detached candidate. New IDs and source/index effects remain
// explicit ports of its caller; read loaders never invoke this function.
func PrepareWriteTree(ctx context.Context, tree *parse.Tree, paragraph func() *ast.Node) (empty bool, err error) {
	if ctx == nil || tree == nil || tree.Root == nil || paragraph == nil {
		return false, errors.New("invalid_tree_write")
	}
	if err = ctx.Err(); err != nil {
		return false, err
	}
	empty = tree.Root.FirstChild == nil
	if empty {
		p := paragraph()
		if p == nil || !ast.IsNodeIDPattern(p.ID) {
			return false, errors.New("invalid_empty_paragraph")
		}
		tree.Root.AppendChild(p)
		tree.Root.SetIALAttr("updated", p.ID[:14])
	}
	if tree.Root.Spec == "" {
		parse.NestedInlines2FlattedSpans(tree, false)
		tree.Root.Spec = "1"
	}
	tree.Root.SetIALAttr("type", "doc")
	return empty, ctx.Err()
}

func EncodeWriteTree(tree *parse.Tree, engine *lute.Lute, singleLine bool) ([]byte, error) {
	if tree == nil || tree.Root == nil || engine == nil {
		return nil, errors.New("invalid_tree_write")
	}
	data := render.NewJSONRenderer(tree, engine.RenderOptions).Render()
	data = bytes.ReplaceAll(data, []byte("\u0000"), nil)
	if !singleLine {
		var b bytes.Buffer
		if err := json.Indent(&b, data, "", "\t"); err != nil {
			return nil, err
		}
		data = b.Bytes()
	}
	return data, nil
}
