// SPDX-License-Identifier: AGPL-3.0-or-later
package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"time"
	"wiki-kernel/internal/assetflow"
	av "wiki-kernel/internal/avmodel"
	"wiki-kernel/internal/blockview"
	"wiki-kernel/internal/doctree"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/indexrepair"
	"wiki-kernel/internal/pageprefs"
)

func (l *Lease) NativeRecentUpdatedBlocks(ctx context.Context) (out []*blockview.Block, err error) {
	err = l.WithNativeIndexes(ctx, func(b *indexdb.Bundle) error {
		r, w := l.workspace.resources, l.workspace
		if r.configuration.Search == nil {
			return indexdb.ErrProjection
		}
		ignore, e := nativeIgnoreLines(ctx, w, "searchignore")
		if e != nil {
			return e
		}
		mobile := false
		if system := r.configuration.System; system != nil {
			mobile = system.Container == "ios" || system.Container == "android" || system.Container == "harmony"
		}
		db, e := b.Database(indexdb.Main)
		if e != nil {
			return e
		}
		out, e = blockview.RecentUpdated(ctx, db, *r.configuration.Search, mobile, ignore)
		return e
	})
	if err != nil {
		return nil, err
	}
	return
}

func (l *Lease) NativeReadStorage(ctx context.Context, action, key, sortBy string) (json.RawMessage, error) {
	switch action {
	case "recent.get", "criteria.get", "local.get", "outline.get":
	default:
		return nil, errors.New("storage_read_action_required")
	}
	plan, e := l.PlanNativeStorage(ctx, pageprefs.Request{Action: action, Key: key, SortBy: sortBy})
	if e != nil {
		return nil, e
	}
	return plan.Value, nil // Proposed compaction/repair writes are deliberately never applied.
}

func (l *Lease) NativeDocumentAssets(ctx context.Context, id string, images bool) (out []string, err error) {
	err = l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
		r, w := l.workspace.resources, l.workspace
		tree, e := r.nativeTreeByBlock(ctx, w, id)
		if e != nil {
			return e
		}
		if images {
			out, e = assetflow.ImageDests(ctx, tree.Root)
			return e
		}
		out, e = assetflow.LinkDests(tree.Root, func(id string) (*av.AttributeView, error) {
			raw, _, e := readNativeAVSource(ctx, w, id)
			if e != nil {
				return nil, e
			}
			return decodeNativeAV(raw, id, time.Now())
		})
		return e
	})
	if err != nil {
		return nil, err
	}
	return
}
func (l *Lease) NativeDocumentTreeIDs(ctx context.Context, box, path string) (out []*doctree.IDNode, err error) {
	err = l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
		var e error
		out, e = doctree.IDs(ctx, indexrepair.Source{Data: l.workspace.data}, box, path)
		return e
	})
	if err != nil {
		return nil, err
	}
	return
}
