// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"github.com/88250/lute/ast"
	"os"
	"path"
	"path/filepath"
	"time"
	"wiki-kernel/cache"
	"wiki-kernel/conf"
	"wiki-kernel/internal/assetflow"
)

func InsertLocalAssetsContext(ctx context.Context, id string, paths []string, upload bool) (out map[string]interface{}, err error) {
	o, e := captureWorkspaceRead(ctx)
	if e != nil {
		return nil, e
	}
	r, e := captureIndexRepair(ctx)
	if e != nil {
		return nil, e
	}
	if r.owner != o.base.owner || r.data != o.base.data {
		return nil, conf.ErrBusinessUnbound
	}
	paths = append([]string(nil), paths...)
	err = o.base.run(ctx, func(c, life context.Context) (result error) {
		if e := r.guard(c); e != nil {
			return e
		}
		if o.settings.Sync == nil {
			return conf.ErrBusinessUnbound
		}
		io, closeIO, e := r.openRepairIOChecked(c, true)
		if e != nil {
			return e
		}
		defer func() { result = joinRepairCloseError(result, closeIO()) }()
		target := "assets"
		if id != "" {
			loc, e := r.referenceLookup(c, id)
			if e != nil {
				return e
			}
			if loc != nil {
				target, e = assetflow.TargetDirectory(c, io.Source, loc.BoxID, loc.Path)
				if e != nil {
					return e
				}
			}
		}
		if e = io.Source.EnsureDirectory(c, target); e != nil {
			return e
		}
		byHash := ownedAssetHashLookup(c, r, io.Source)
		out, e = assetflow.Insert(c, paths, upload, assetflow.InsertPorts{
			NewID: ast.NewNodeID,
			Directory: func(c context.Context, name string) (bool, error) {
				if e := r.guard(c); e != nil {
					return false, e
				}
				info, e := os.Stat(name)
				if e != nil {
					return false, e
				}
				return info.IsDir(), nil
			},
			Inside: func(c context.Context, name string) (bool, error) {
				absolute, e := filepath.Abs(name)
				if e != nil {
					return false, e
				}
				rel, e := filepath.Rel(filepath.Join(r.data, filepath.FromSlash(target)), absolute)
				return e == nil && filepath.IsLocal(rel), c.Err()
			},
			Read: func(c context.Context, name string) (assetflow.LocalFile, error) {
				if e := r.guard(c); e != nil {
					return assetflow.LocalFile{}, e
				}
				info, e := os.Stat(name)
				if e != nil {
					return assetflow.LocalFile{}, e
				}
				if info.IsDir() {
					return assetflow.LocalFile{Directory: true}, nil
				}
				selected := capturePDFWatermarkFile(name)
				raw, e := selected.read(512<<20, func() error { return r.guard(c) })
				return assetflow.LocalFile{Data: raw}, e
			},
			ByHash: byHash,
			Write: func(c context.Context, name string, raw []byte) error {
				return io.Source.Rewrite(c, path.Join(target, name), nil, raw)
			},
			Remember: func(c context.Context, hash, p string) error {
				if e := r.guard(c); e != nil {
					return e
				}
				cache.SetAssetHash(hash, p)
				return nil
			},
		})
		if e != nil {
			return e
		}
		o.base.owner.syncRuntime.same.Store(0)
		o.base.owner.planSyncAfter(time.Duration(o.settings.Sync.Interval) * time.Second)
		return c.Err()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
