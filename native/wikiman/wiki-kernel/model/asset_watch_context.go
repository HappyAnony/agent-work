// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"os"
	"path"
	"path/filepath"
	"time"
	"wiki-kernel/cache"
	"wiki-kernel/conf"
	"wiki-kernel/internal/assetflow"
	"wiki-kernel/internal/indexrepair"
	"wiki-kernel/sql"
	"wiki-kernel/util"
)

type assetWatchContext struct {
	base  *legacyReindexOwner
	queue sql.LegacyAssetQueue
}

func captureAssetWatchContext(ctx context.Context) (*assetWatchContext, error) {
	base, e := captureLegacyReindexOwner(ctx)
	if e != nil {
		return nil, e
	}
	return &assetWatchContext{base: base, queue: sql.CaptureLegacyAssetQueue()}, nil
}
func (w *assetWatchContext) dispatch(ctx context.Context, event assetflow.Event) error {
	if e := w.base.guard(ctx); e != nil {
		return e
	}
	relative, e := filepath.Rel(filepath.Join(w.base.data, "assets"), event.Path)
	if e != nil || !filepath.IsLocal(relative) {
		return indexrepair.ErrSourcePath
	}
	if relative == "." {
		return nil
	}
	return w.base.run(ctx, func(c, life context.Context) error {
		return assetflow.Dispatch(c, event, assetflow.Handler{
			MarkSync: func(c context.Context) error {
				return w.base.owner.WithSettingSnapshot(c, func(v conf.AppData) error {
					if e := w.base.guard(c); e != nil {
						return e
					}
					if v.Sync == nil {
						return conf.ErrBusinessUnbound
					}
					w.base.owner.syncRuntime.same.Store(0)
					w.base.owner.planSyncAfter(time.Duration(v.Sync.Interval) * time.Second)
					return nil
				})
			},
			Refresh: func(c context.Context) (err error) {
				if err = w.base.guard(c); err != nil {
					return err
				}
				info, e := os.Lstat(w.base.data)
				if e != nil {
					return e
				}
				if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
					return indexrepair.ErrSourcePath
				}
				root, e := os.OpenRoot(w.base.data)
				if e != nil {
					return e
				}
				defer func() { err = errors.Join(err, root.Close()) }()
				now, e := root.Stat(".")
				if e != nil {
					return e
				}
				if !os.SameFile(info, now) {
					return indexrepair.ErrSourceChanged
				}
				if err = cache.LoadAssetsContext(c, root); err != nil {
					return err
				}
				return w.base.guard(c)
			},
			Index: func(c context.Context, e assetflow.Event) error {
				return indexAssetContentOwned(c, e.Path, e.Removed, w.base, w.queue)
			},
			InvalidateThumbnail: func(c context.Context, p string) (err error) {
				if !util.IsCompressibleAssetImage("assets/" + filepath.ToSlash(relative)) {
					return nil
				}
				if err = w.base.guard(c); err != nil {
					return err
				}
				info, e := os.Lstat(w.base.tempDir)
				if os.IsNotExist(e) {
					return nil
				}
				if e != nil {
					return e
				}
				if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
					return indexrepair.ErrSourcePath
				}
				root, e := os.OpenRoot(w.base.tempDir)
				if e != nil {
					return e
				}
				defer func() { err = errors.Join(err, root.Close()) }()
				source := indexrepair.Source{Data: root, Authorize: func(context.Context) error { return w.base.guard(c) }}
				name := path.Join("thumbnails/assets", filepath.ToSlash(relative))
				if e = source.Check(name, false); indexrepair.IsSourceNotExist(e) {
					return nil
				} else if e != nil {
					return e
				}
				thumbnail, e := root.Lstat(name)
				if e != nil {
					return e
				}
				if thumbnail.IsDir() {
					return source.RemoveDirectory(c, name)
				}
				return source.Remove(c, name)
			},
		})
	})
}
