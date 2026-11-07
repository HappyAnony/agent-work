// SPDX-License-Identifier: AGPL-3.0-or-later
package workspace

import (
	"context"
	"wiki-kernel/internal/docquery"
	"wiki-kernel/internal/indexdb"
)

func (l *Lease) NativeBlockLocation(ctx context.Context, id string) (out map[string]string, err error) {
	err = l.WithNativeIndexes(ctx, func(b *indexdb.Bundle) error {
		r, w := l.workspace.resources, l.workspace
		s, e := r.blockReadServices(ctx, w, b)
		if e != nil {
			return e
		}
		tree, e := s.Load(ctx, id)
		if e != nil {
			return e
		}
		out, e = docquery.BlockLocation(ctx, id, tree, s.Parser, s.Content)
		return e
	})
	if err != nil {
		return nil, err
	}
	return
}
