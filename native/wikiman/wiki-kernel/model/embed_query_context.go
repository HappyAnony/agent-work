// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"slices"
	"wiki-kernel/conf"
	"wiki-kernel/internal/blockview"
)

func EmbedsContext(ctx context.Context, embedID string, q blockview.EmbedRequest) (out []*EmbedBlock, err error) {
	q.IncludeIDs = slices.Clone(q.IncludeIDs)
	q.ExcludeIDs = slices.Clone(q.ExcludeIDs)
	o, e := captureWorkspaceRead(ctx)
	if e != nil {
		return nil, e
	}
	o.base.owner.m.Lock()
	readonly := o.base.owner.persistenceReadOnly
	o.base.owner.m.Unlock()
	var writer *indexRepairContext
	// Publish/read-only rendering and an unbound preview have no derived-index
	// publication. Writable compatibility reads retain the original embed index.
	if !readonly && embedID != "" {
		writer, e = captureIndexRepair(ctx)
		if e != nil {
			return nil, e
		}
		if writer.owner != o.base.owner || writer.data != o.base.data {
			return nil, conf.ErrBusinessUnbound
		}
	}
	err = o.base.run(ctx, func(op, life context.Context) error {
		var e error
		out, e = blockview.ReadEmbeds(op, q, o.blockReadServices())
		if e != nil {
			return e
		}
		if writer != nil {
			values := make([]string, 0, len(out))
			for _, v := range out {
				if v != nil && v.Block != nil {
					values = append(values, v.Block.Markdown)
				}
			}
			if e = writer.updateEmbedBlockContent(op, embedID, values); e != nil {
				return e
			}
			if e = writer.queue.Flush(context.WithoutCancel(op)); e != nil {
				return e
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return
}
func GetEmbedBlockContext(ctx context.Context, id string, include []string, headingMode int, breadcrumb bool) ([]*EmbedBlock, error) {
	return EmbedsContext(ctx, id, blockview.EmbedRequest{IncludeIDs: include, HeadingMode: headingMode, Breadcrumb: breadcrumb})
}
func SearchEmbedBlockContext(ctx context.Context, id, stmt string, exclude []string, headingMode int, breadcrumb bool) ([]*EmbedBlock, error) {
	return EmbedsContext(ctx, id, blockview.EmbedRequest{Query: true, SQL: stmt, ExcludeIDs: exclude, HeadingMode: headingMode, Breadcrumb: breadcrumb})
}
