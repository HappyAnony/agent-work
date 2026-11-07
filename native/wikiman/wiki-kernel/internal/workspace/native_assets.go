package workspace

import (
	"context"
	"database/sql"
	"errors"
	"github.com/siyuan-note/filelock"
	"io/fs"
	"path/filepath"
	"time"
	"wiki-kernel/internal/assetcache"
	"wiki-kernel/internal/indexdb"
)

func (r *resources) loadAssets(ctx context.Context, w *WorkspaceContext) error {
	if w.data == nil {
		return ErrDocument
	}
	info, err := w.data.Lstat("assets")
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if err == nil && (!info.IsDir() || info.Mode()&fs.ModeSymlink != 0) {
		return ErrDocument
	}
	root, err := fs.Sub(w.data.FS(), "assets")
	if err != nil {
		return err
	}
	return r.nativeAssets.Load(ctx, root, func(name string) bool { return filelock.IsHidden(filepath.Join(w.paths.DataDir, "assets", name)) })
}
func (l *Lease) ReloadNativeAssets(ctx context.Context) error {
	return l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error { return l.workspace.resources.loadAssets(ctx, l.workspace) })
}
func (l *Lease) NativeAssets(ctx context.Context) (ret map[string]assetcache.Asset, err error) {
	err = l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error { ret = l.workspace.resources.nativeAssets.Snapshot(); return nil })
	return
}
func (l *Lease) NativeAssetByHash(ctx context.Context, hash string) (ret assetcache.Hash, found bool, err error) {
	err = l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
		var readErr error
		ret, found = l.workspace.resources.nativeAssets.GetHash(hash, func(p string) bool {
			f, e := openNativeFile(l.workspace, p)
			if errors.Is(e, fs.ErrNotExist) {
				return false
			}
			if e != nil {
				readErr = e
				return false
			}
			defer f.Close()
			info, e := f.Stat()
			if e != nil {
				readErr = e
				return false
			}
			actual, e := indexdb.AssetEtag(ctx, f, info.Size())
			if e != nil {
				readErr = e
				return false
			}
			return actual == hash
		})
		if readErr != nil || found {
			return readErr
		}
		db, e := l.workspace.resources.native.Database(indexdb.Main)
		if e != nil {
			return e
		}
		var p, box, doc string
		e = db.QueryRowContext(ctx, "SELECT path,box,docpath FROM assets WHERE hash=? LIMIT 1", hash).Scan(&p, &box, &doc)
		if errors.Is(e, sql.ErrNoRows) {
			return nil
		}
		if e != nil {
			return e
		}
		actual, e := originalAssetHash(ctx, l.workspace, p, box, doc)
		if e != nil {
			return e
		}
		if actual != hash {
			return nil
		}
		ret, found = l.workspace.resources.nativeAssets.GetHash(hash, func(string) bool { return true })
		return nil
	})
	return
}

func (l *Lease) StartNativeAssetRefresh(ctx context.Context, interval time.Duration) (*Task, error) {
	return l.StartPeriodicTask(ctx, interval, func(ctx context.Context, l *Lease) error { return l.ReloadNativeAssets(ctx) })
}
func (l *Lease) RemoveNativeAssetCache(ctx context.Context, p string) error {
	return l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error { l.workspace.resources.nativeAssets.Remove(p); return nil })
}
