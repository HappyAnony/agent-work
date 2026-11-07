// SPDX-License-Identifier: AGPL-3.0-or-later
package workspace

import (
	"context"
	"wiki-kernel/internal/blockview"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/pageprefs"
)

func (l *Lease) NativeOutline(ctx context.Context, id string, preview bool) (out []*blockview.Path, err error) {
	if err = l.FlushNativeMainQueue(ctx); err != nil {
		return nil, err
	}
	err = l.WithNativeIndexes(ctx, func(b *indexdb.Bundle) error {
		r, w := l.workspace.resources, l.workspace
		trees, e := r.documentInfoTrees(ctx, w, []string{id})
		if e != nil {
			return e
		}
		if trees[id] == nil {
			out = []*blockview.Path{}
			return ctx.Err()
		}
		parser, e := r.documentInfoParser(ctx, w)
		if e != nil {
			return e
		}
		value, e := r.pagePreferences.Do(ctx, pageprefs.Request{Action: "outline.get", Key: id}, r.preferencesBackend(w))
		if e != nil {
			return e
		}
		storage, ok := value.(map[string]interface{})
		if !ok {
			return ErrDocument
		}
		if preview && r.configuration.Export == nil {
			return ErrDocument
		}
		addTitle := preview && r.configuration.Export.AddTitle
		out, e = blockview.OutlineView(ctx, trees[id], parser, addTitle, storage)
		return e
	})
	if err != nil {
		return nil, err
	}
	return
}
