// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"github.com/88250/lute/ast"
	"os"
	"path/filepath"
	"time"
	"wiki-kernel/cache"
	"wiki-kernel/conf"
	"wiki-kernel/internal/assetflow"
	"wiki-kernel/internal/indexrepair"
	"wiki-kernel/internal/notebooks"
)

func RemoveBoxContext(ctx context.Context, id string) error {
	_, err := runWorkspaceMetadataRead(ctx, func(o *workspaceReadOwner, c context.Context) (out struct{}, result error) {
		if !ast.IsNodeIDPattern(id) {
			return out, indexrepair.ErrSourcePath
		}
		r, e := captureIndexRepair(c)
		if e != nil {
			return out, e
		}
		if r.owner != o.base.owner || r.data != o.base.data {
			return out, conf.ErrBusinessUnbound
		}
		r.guideWrite = notebooks.Guide(id)
		result = r.owner.notebookGate().with(c, func() (result error) {
			if e := r.guard(c); e != nil {
				return e
			}
			if o.settings.Sync == nil {
				return conf.ErrBusinessUnbound
			}
			if e := FlushTxQueueContext(c); e != nil {
				return e
			}
			io, closeIO, e := r.openRepairIOChecked(c, true)
			if e != nil {
				return e
			}
			defer func() { result = joinRepairCloseError(result, closeIO()) }()
			if e = io.Source.Check(id, false); indexrepair.IsSourceNotExist(e) {
				return nil
			} else if e != nil {
				return e
			}
			assets := []string{}
			if !notebooks.Guide(id) {
				assets, e = assetflow.AssetDirectories(c, io.Source, id)
				if e != nil {
					return e
				}
			}
			var historyRoot, parent *os.Root
			stamp := time.Now().Format("2006-01-02-150405") + "-" + HistoryOpDelete
			defer func() {
				if historyRoot != nil {
					result = errors.Join(result, historyRoot.Close())
				}
				if parent != nil {
					result = errors.Join(result, parent.Close())
				}
			}()
			return notebooks.Remove(c, id, notebooks.RemovePorts{
				Backup: func(c context.Context, id string) error {
					var e error
					parent, e = os.OpenRoot(filepath.Dir(o.base.history))
					if e != nil {
						return e
					}
					storage := indexrepair.Source{Data: parent, Authorize: r.guard}
					name := filepath.Base(o.base.history)
					if e = storage.EnsureDirectory(c, name); e != nil {
						return e
					}
					historyRoot, e = parent.OpenRoot(name)
					if e != nil {
						return e
					}
					backup := indexrepair.Source{Data: historyRoot, Authorize: r.guard}
					return backup.CopyDirectoryFrom(c, io.Source, id, stamp+"/"+id)
				},
				Assets: func(c context.Context, id string) error {
					for _, name := range assets {
						if e := io.Source.CopyDirectoryFrom(c, io.Source, name, "assets"); e != nil {
							return e
						}
					}
					return cache.LoadAssetsContext(c, io.Source.Data)
				},
				Unmount: func(c context.Context, id string) error {
					if _, e := publishNotebookClosed(c, io.Source, id, true); e != nil {
						return e
					}
					return r.unindexNotebook(context.WithoutCancel(c), id)
				},
				Delete: func(c context.Context, id string) error { return io.Source.RemoveDirectory(c, id) },
				Changed: func(c context.Context, id string) error {
					if e := r.guard(c); e != nil {
						return e
					}
					r.owner.syncRuntime.same.Store(0)
					r.owner.planSyncAfter(time.Duration(o.settings.Sync.Interval) * time.Second)
					return c.Err()
				},
			})
		})
		return out, result
	})
	return err
}
