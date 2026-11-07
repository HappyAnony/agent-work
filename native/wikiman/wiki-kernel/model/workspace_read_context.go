// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"strings"
	"wiki-kernel/av"
	"wiki-kernel/internal/avrender"
	"wiki-kernel/internal/docquery"
	"wiki-kernel/internal/docview"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/indexrepair"
)

func runWorkspaceRead[T any](ctx context.Context, fn func(*workspaceReadOwner, context.Context) (T, error)) (out T, err error) {
	o, e := captureWorkspaceRead(ctx)
	if e != nil {
		return out, e
	}
	err = o.base.run(ctx, func(op, life context.Context) error { var e error; out, e = fn(o, op); return e })
	if err != nil {
		var zero T
		return zero, err
	}
	return
}

// Missing registry IDs are a normal absence; a known corrupt or missing source
// is an error. Reads never migrate source or acquire the repair writer.
func (o *workspaceReadOwner) loadOptional(ctx context.Context, id string) (*parse.Tree, error) {
	bt, e := o.r.referenceLookup(ctx, id)
	if e != nil {
		return nil, e
	}
	if bt == nil {
		return nil, nil
	}
	return o.r.loadRepairTree(ctx, bt.BoxID, bt.Path, o.parser)
}
func (o *workspaceReadOwner) trees(ctx context.Context, ids []string) (map[string]*parse.Tree, error) {
	out := map[string]*parse.Tree{}
	loaded := map[string]*parse.Tree{}
	for _, id := range ids {
		if e := o.r.guard(ctx); e != nil {
			return nil, e
		}
		bt, e := o.r.referenceLookup(ctx, id)
		if e != nil {
			return nil, e
		}
		if bt == nil {
			continue
		}
		key := bt.BoxID + bt.Path
		t := loaded[key]
		if t == nil {
			t, e = o.r.loadRepairTree(ctx, bt.BoxID, bt.Path, o.parser)
			if e != nil {
				return nil, e
			}
			loaded[key] = t
		}
		out[id] = t
	}
	return out, ctx.Err()
}
func (o *workspaceReadOwner) raw(ctx context.Context, name string) (raw []byte, err error) {
	io, closeIO, e := o.r.openRepairIOChecked(ctx, false)
	if e != nil {
		return nil, e
	}
	defer func() {
		err = errors.Join(err, closeIO())
		if err != nil {
			raw = nil
		}
	}()
	return io.Source.Read(ctx, name)
}
func (o *workspaceReadOwner) avName(ctx context.Context, id string) (string, error) {
	if !ast.IsNodeIDPattern(id) {
		return "", errors.New("attribute_view_invalid_id")
	}
	raw, e := o.raw(ctx, "storage/av/"+id+".json")
	if e != nil {
		return "", e
	}
	a, e := av.DecodeAttributeView(raw)
	if e != nil {
		return "", e
	}
	if a == nil || a.ID != id {
		return "", errors.New("attribute_view_identity_invalid")
	}
	return a.Name, ctx.Err()
}
func (o *workspaceReadOwner) content() indexdb.ContentServices {
	var texts map[string]string
	var loaded bool
	return indexdb.ContentServices{AssetText: func(c context.Context, p string) (string, error) {
		if !loaded {
			raw, e := o.raw(c, "assets/ocr-texts.json")
			if e != nil && !indexrepair.IsSourceNotExist(e) {
				return "", e
			}
			if e == nil {
				if e = json.Unmarshal(raw, &texts); e != nil {
					return "", e
				}
			}
			loaded = true
		}
		return texts[p], c.Err()
	}, AttributeView: func(c context.Context, id string, full bool) (string, error) {
		if !full {
			return o.avName(c, id)
		}
		if !ast.IsNodeIDPattern(id) {
			return "", errors.New("attribute_view_invalid_id")
		}
		raw, e := o.raw(c, "storage/av/"+id+".json")
		if e != nil {
			return "", e
		}
		a, e := av.DecodeAttributeView(raw)
		if e != nil {
			return "", e
		}
		if a == nil || a.ID != id {
			return "", errors.New("attribute_view_identity_invalid")
		}
		return avrender.SearchText(c, a)
	}}
}
func (o *workspaceReadOwner) text(ctx context.Context, n *ast.Node, exclude []string, lineBreak bool) (string, error) {
	return docquery.BlockText(ctx, n, exclude, lineBreak, func(c context.Context, n *ast.Node, exclude []string) (string, error) {
		return indexdb.StaticContent(c, n, exclude, false, false, false, o.content())
	})
}

func (o *workspaceReadOwner) avNames(ctx context.Context, ids string) (string, error) {
	return docview.AVNames(ctx, ids, o.avName, func(c context.Context) (string, error) {
		if e := o.r.guard(c); e != nil {
			return "", e
		}
		value := o.base.owner.business.Language(o.settings.Lang, 105, false, o.settings.CloudRegion)
		if value == "" {
			return "", errors.New("attribute_view_language_missing")
		}
		return value, nil
	})
}
func (o *workspaceReadOwner) renderNodes(ctx context.Context, nodes []*ast.Node, parser *lute.Lute) (string, error) {
	return docview.RenderNodes(ctx, nodes, parser, o.avNames)
}

func (o *workspaceReadOwner) referenceDefs(ctx context.Context, id, root string) (defs []*RefDefs, original map[string]string, err error) {
	if e := o.r.guard(ctx); e != nil {
		return nil, nil, e
	}
	q := indexdb.References{DB: o.r.main.DB}
	if id == root {
		values, e := q.ChildDefinitions(ctx, root)
		if e != nil {
			return nil, nil, e
		}
		for ref, ids := range values {
			defs = append(defs, &RefDefs{RefID: ref, DefIDs: append([]string(nil), ids...)})
		}
	} else {
		ids, e := q.RefIDs(ctx, id, false)
		if e != nil {
			return nil, nil, e
		}
		for _, ref := range ids {
			defs = append(defs, &RefDefs{RefID: ref, DefIDs: []string{id}})
		}
	}
	if o.settings.Search == nil {
		return nil, nil, errors.New("reference_search_settings_missing")
	}
	original, err = docquery.PromoteRefDefs(ctx, defs, docquery.RefPromotionServices{CaseSensitive: o.settings.Search.CaseSensitive, Parser: func(c context.Context) (*lute.Lute, error) { return o.parser, o.r.guard(c) }, Blocks: func(c context.Context, ids []string) ([]indexdb.BlockRecord, error) {
		rows, e := o.r.main.Blocks(c, ids)
		if e != nil {
			return nil, e
		}
		out := make([]indexdb.BlockRecord, 0, len(rows))
		for _, v := range rows {
			out = append(out, *v)
		}
		return out, c.Err()
	}})
	return
}
func (o *workspaceReadOwner) subfiles(ctx context.Context, t *parse.Tree, batch bool) (count int, err error) {
	io, closeIO, e := o.r.openRepairIOChecked(ctx, false)
	if e != nil {
		return 0, e
	}
	defer func() { err = errors.Join(err, closeIO()) }()
	entries, e := io.Source.ReadDir(ctx, t.Box+strings.TrimSuffix(t.Path, ".docj"))
	if indexrepair.IsSourceNotExist(e) && !batch {
		return 0, nil
	}
	if e != nil {
		return 0, e
	}
	for _, entry := range entries {
		if e := ctx.Err(); e != nil {
			return 0, e
		}
		if strings.HasSuffix(entry.Name(), ".docj") {
			count++
		}
	}
	return count, nil
}
