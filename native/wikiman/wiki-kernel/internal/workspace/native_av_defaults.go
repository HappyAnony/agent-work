// SPDX-License-Identifier: AGPL-3.0-or-later
package workspace

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	av "wiki-kernel/internal/avmodel"
	"wiki-kernel/internal/editcore"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/nativeparser"
)

// NativeAttributeViewDefaults uses the same filter/group calculation as the
// original row insertion, with read-only dependencies and disposable AV data.
func (l *Lease) NativeAttributeViewDefaults(ctx context.Context, id, viewID, groupID, previous, adding string) (out map[string]*av.Value, err error) {
	defer func() {
		if v := recover(); v != nil {
			out, err = nil, fmt.Errorf("original_av_defaults_failed: %T", v)
		}
	}()
	err = l.WithNativeIndexes(ctx, func(b *indexdb.Bundle) error {
		r, w := l.workspace.resources, l.workspace
		opts, e := r.configuredParserOptions(ctx, w)
		if e != nil {
			return e
		}
		p := &nativeEdit{ctx: ctx, w: w, r: r, bundle: b, parser: nativeparser.NewEditor(opts),
			indexer: nativeparser.NewConfiguredIndex(opts), now: time.Now(), before: map[string][]byte{},
			normalized: map[string][]byte{}, avs: map[string]*av.AttributeView{}, avBefore: map[string][]byte{},
			tx: &editcore.DocumentTransaction{Trees: map[string]*parse.Tree{}, Nodes: map[string]*ast.Node{}}}
		a, e := p.readAV(id)
		if e != nil {
			return e
		}
		if e = editcore.ValidateUpdatedAttributeView(a); e != nil {
			return e
		}
		view, e := a.GetCurrentView(viewID)
		if e != nil {
			return e
		}
		if len(view.Filters) == 0 && !view.IsGroupView() {
			return nil
		}
		group := view
		if groupID != "" {
			group = view.GetGroupByID(groupID)
			if group == nil {
				return ErrDocument
			}
		}
		values, e := editcore.AttributeViewDefaultValues(a, view, group, previous, adding, true, &editcore.AttributeViewDefaultEnvironment{
			Ctx: ctx, Near: p.near, TemplateKeys: p.templateKeys, Now: func() time.Time { return p.now }, NewID: ast.NewNodeID,
			Reverse: func(*av.AttributeView, *av.Key, *av.Value) error { return errors.New("av_preview_write_forbidden") },
		})
		if e != nil {
			return e
		}
		for _, v := range values {
			if v == nil {
				return ErrDocument
			}
			if v.Type == av.KeyTypeBlock {
				if v.Block == nil {
					return ErrDocument
				}
				v.Block.Content = ""
			}
		}
		if len(values) > 0 {
			out = values
		}
		return ctx.Err()
	})
	if err != nil {
		return nil, err
	}
	return
}
