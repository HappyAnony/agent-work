// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"io/fs"
	"slices"
	"wiki-kernel/av"
	"wiki-kernel/cache"
	"wiki-kernel/conf"
	"wiki-kernel/internal/assetcache"
	"wiki-kernel/internal/assetflow"
)

func DocImageAssetsContext(ctx context.Context, id string) ([]string, error) {
	return runWorkspaceRead(ctx, func(o *workspaceReadOwner, c context.Context) ([]string, error) {
		tree, e := o.load(c, id)
		if e != nil {
			return nil, e
		}
		return assetflow.ImageDests(c, tree.Root)
	})
}
func DocAssetsContext(ctx context.Context, id string) ([]string, error) {
	return runWorkspaceRead(ctx, func(o *workspaceReadOwner, c context.Context) ([]string, error) {
		tree, e := o.load(c, id)
		if e != nil {
			return nil, e
		}
		return assetflow.LinkDests(tree.Root, func(id string) (*av.AttributeView, error) {
			raw, e := o.raw(c, "storage/av/"+id+".json")
			if e != nil {
				return nil, e
			}
			a, e := av.DecodeAttributeView(raw)
			if e == nil && (a == nil || a.ID != id) {
				return nil, conf.ErrBusinessUnbound
			}
			return a, e
		})
	})
}
func SearchAssetsByNameContext(ctx context.Context, keyword string, exts []string) ([]*cache.Asset, error) {
	exts = slices.Clone(exts)
	return runWorkspaceRead(ctx, func(o *workspaceReadOwner, c context.Context) (out []*cache.Asset, result error) {
		if o.settings.Search == nil || o.settings.Search.Limit < 0 {
			return nil, conf.ErrBusinessUnbound
		}
		io, closeIO, e := o.r.openRepairIOChecked(c, false)
		if e != nil {
			return nil, e
		}
		defer func() {
			result = joinRepairCloseError(result, closeIO())
			if result != nil {
				out = nil
			}
		}()
		if e = io.Source.Check("assets", true); e != nil {
			return nil, e
		}
		assets, e := fs.Sub(io.Source.Data.FS(), "assets")
		if e != nil {
			return nil, e
		}
		var snapshot assetcache.Cache
		if e = snapshot.Load(c, assets, nil); e != nil {
			return nil, e
		}
		defer snapshot.Close()
		return assetcache.SearchNames(keyword, exts, snapshot.Snapshot(), o.settings.Search.Limit, o.settings.Search.CaseSensitive), c.Err()
	})
}
