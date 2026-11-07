// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package filesys

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"html"
	"io/fs"
	"path"
	"strings"

	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"github.com/siyuan-note/dataparser"

	"wiki-kernel/internal/docast"
	"wiki-kernel/internal/docview"
	"wiki-kernel/internal/indexrepair"
	"wiki-kernel/treenode"
)

// TreeContext has no implicit source writer, path/config/cache selection or
// registry fallback. Reader mode never migrates Spec, changes IDs or adds parents.
type TreeContext struct {
	Source     indexrepair.Source
	SingleLine bool
	Upsert     func(context.Context, *parse.Tree) error
	Remember   func(*parse.Tree, []byte)
	Written    func(context.Context, *parse.Tree, uint64) error
}

func documentPath(box, p string) (string, error) {
	if !ast.IsNodeIDPattern(box) || !strings.HasPrefix(p, "/") || !strings.HasSuffix(p, ".docj") {
		return "", indexrepair.ErrSourcePath
	}
	rel := strings.TrimPrefix(p, "/")
	if !fs.ValidPath(rel) || strings.Contains(rel, "\\") || !ast.IsNodeIDPattern(strings.TrimSuffix(path.Base(rel), ".docj")) {
		return "", indexrepair.ErrSourcePath
	}
	return box + "/" + rel, nil
}
func (c TreeContext) encode(tree *parse.Tree, engine *lute.Lute) ([]byte, error) {
	return docview.EncodeWriteTree(tree, engine, c.SingleLine)
}

func (c TreeContext) save(ctx context.Context, tree *parse.Tree, before []byte, engine *lute.Lute) error {
	name, e := documentPath(tree.Box, tree.Path)
	if e != nil {
		return e
	}
	data, e := c.encode(tree, engine)
	if e != nil {
		return e
	}
	if before != nil {
		data, e = docast.SerializeDocument([][]byte{before}, data)
		if e != nil {
			return e
		}
	}
	if e = c.Source.Rewrite(ctx, name, before, data); e != nil {
		return e
	}
	if c.Remember != nil {
		c.Remember(tree, bytes.Clone(data))
	}
	if c.Written != nil {
		return c.Written(context.WithoutCancel(ctx), tree, uint64(len(data)))
	}
	return nil
}
func (c TreeContext) Load(ctx context.Context, box, p string, engine *lute.Lute, repair bool) (*parse.Tree, error) {
	if repair {
		if c.Source.Authorize == nil {
			return nil, indexrepair.ErrSourceWrite
		}
		if e := c.Source.Authorize(ctx); e != nil {
			return nil, e
		}
	}
	name, e := documentPath(box, p)
	if e != nil {
		return nil, e
	}
	raw, e := c.Source.Read(ctx, name)
	if e != nil {
		return nil, e
	}
	var tree *parse.Tree
	changed := false
	if repair {
		tree, changed, e = dataparser.ParseJSON(raw, engine.ParseOptions)
	} else {
		tree, e = dataparser.ParseJSONWithoutFix(raw, engine.ParseOptions)
	}
	if e != nil {
		return nil, e
	}
	if tree == nil || tree.Root == nil {
		return nil, errors.New("invalid_tree")
	}
	tree.Box, tree.Path = box, p
	tree.Root.Box, tree.Root.Path = box, p
	expected := strings.TrimSuffix(path.Base(p), ".docj")
	if repair {
		if tree.Root.Spec == "" {
			parse.NestedInlines2FlattedSpans(tree, false)
			tree.Root.Spec = "1"
			changed = true
		}
		if tree.ID != expected {
			tree.ID = expected
			tree.Root.ID = expected
			tree.Root.SetIALAttr("id", expected)
			changed = true
		}
	} else if tree.ID != expected {
		return nil, indexrepair.ErrReferenceTree
	}
	parts := strings.Split(strings.TrimPrefix(p, "/"), "/")
	titles := []string{}
	for i := 0; i < len(parts)-1; i++ {
		parentPath := "/" + strings.Join(parts[:i+1], "/") + ".docj"
		parentName, _ := documentPath(box, parentPath)
		parentRaw, e := c.Source.Read(ctx, parentName)
		title := "Untitled"
		if indexrepair.IsSourceNotExist(e) {
			if repair {
				if c.Upsert == nil {
					return nil, indexrepair.ErrSourceWrite
				}
				parent := treenode.NewTree(box, parentPath, "/"+strings.Join(append(append([]string(nil), titles...), title), "/"), title)
				if e = c.Write(ctx, parent, nil, engine); e != nil {
					return nil, e
				}
				if c.Upsert == nil {
					return nil, indexrepair.ErrSourceWrite
				}
				if e = c.Upsert(ctx, parent); e != nil {
					return nil, e
				}
			}
		} else if e != nil {
			return nil, e
		} else {
			var doc struct{ Properties map[string]string }
			if e = json.Unmarshal(parentRaw, &doc); e != nil {
				return nil, e
			}
			if doc.Properties["title"] != "" {
				title = html.UnescapeString(doc.Properties["title"])
			}
		}
		titles = append(titles, title)
	}
	tree.HPath = "/" + strings.Join(append(titles, tree.Root.IALAttr("title")), "/")
	tree.Hash = treenode.NodeHash(tree.Root, tree, engine)
	if !changed && c.Remember != nil {
		c.Remember(tree, bytes.Clone(raw))
	}
	if changed {
		if e = c.save(ctx, tree, raw, engine); e != nil {
			return nil, e
		}
	}
	return tree, ctx.Err()
}

// Write retains original normalization, empty paragraph and formatting behavior,
// but all source writes and registry/cache effects must be explicitly bound.
func (c TreeContext) Write(ctx context.Context, tree *parse.Tree, before []byte, engine *lute.Lute) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	if tree == nil || tree.Root == nil {
		return errors.New("invalid_tree")
	}
	if c.Source.Authorize == nil {
		return indexrepair.ErrSourceWrite
	}
	if e := c.Source.Authorize(ctx); e != nil {
		return e
	}
	empty := tree.Root.FirstChild == nil
	if empty && c.Upsert == nil {
		return indexrepair.ErrSourceWrite
	}
	if _, e := docview.PrepareWriteTree(ctx, tree, func() *ast.Node { return docview.NewParagraph("") }); e != nil {
		return e
	}
	if e := c.save(ctx, tree, before, engine); e != nil {
		return e
	}
	if empty {
		if c.Upsert == nil {
			return indexrepair.ErrSourceWrite
		}
		return c.Upsert(context.WithoutCancel(ctx), tree)
	}
	return nil
}
