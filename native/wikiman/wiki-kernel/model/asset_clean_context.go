// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"fmt"
	"github.com/88250/go-humanize"
	"os"
	"path/filepath"
	"strings"
	"time"
	"wiki-kernel/cache"
	"wiki-kernel/conf"
	"wiki-kernel/internal/assetflow"
	"wiki-kernel/internal/indexrepair"
	"wiki-kernel/sql"
	"wiki-kernel/util"
)

func RemoveUnusedAssetsContext(ctx context.Context) ([]string, error) {
	return cleanAssetsContext(ctx, nil)
}
func RemoveUnusedAssetContext(ctx context.Context, name string) (string, error) {
	v, e := cleanAssetsContext(ctx, []string{name})
	if e != nil {
		return "", e
	}
	if len(v) == 0 {
		return "", nil
	}
	return v[0], nil
}
func cleanAssetsContext(ctx context.Context, selected []string) (out []string, err error) {
	o, e := captureWorkspaceRead(ctx)
	if e != nil {
		return nil, e
	}
	r, e := captureIndexRepair(ctx)
	if e != nil {
		return nil, e
	}
	history, e := captureHistoryIndexContext(ctx)
	if e != nil {
		return nil, e
	}
	assetQueue := sql.CaptureLegacyAssetQueue()
	if r.owner != o.base.owner || history.owner != r.owner || r.data != o.base.data {
		return nil, conf.ErrBusinessUnbound
	}
	selected = append([]string(nil), selected...)
	err = o.base.run(ctx, func(c, life context.Context) (result error) {
		if e := r.guard(c); e != nil {
			return e
		}
		if o.settings.Sync == nil {
			return conf.ErrBusinessUnbound
		}
		if selected == nil {
			selected, e = UnusedAssetsContext(c)
			if e != nil {
				return e
			}
		}
		if len(selected) == 0 {
			out = []string{}
			return nil
		}
		io, closeIO, e := r.openRepairIOChecked(c, true)
		if e != nil {
			return e
		}
		defer func() { result = joinRepairCloseError(result, closeIO()) }()
		// A missing single selection remains an idempotent original result.
		if len(selected) == 1 {
			if e = io.Source.Check(strings.TrimSuffix(selected[0], "/"), false); indexrepair.IsSourceNotExist(e) {
				out = []string{filepath.Join(r.data, selected[0])}
				return nil
			} else if e != nil {
				return e
			}
		}
		parent, e := os.OpenRoot(filepath.Dir(o.base.history))
		if e != nil {
			return e
		}
		defer func() { result = errors.Join(result, parent.Close()) }()
		historyParent := indexrepair.Source{Data: parent, Authorize: r.guard}
		base := filepath.Base(o.base.history)
		if e = historyParent.EnsureDirectory(c, base); e != nil {
			return e
		}
		root, e := parent.OpenRoot(base)
		if e != nil {
			return e
		}
		defer func() { result = errors.Join(result, root.Close()) }()
		backup := indexrepair.Source{Data: root, Authorize: r.guard}
		stamp := time.Now().Format("2006-01-02-150405") + "-" + HistoryOpClean
		message := ""
		o.base.owner.publishSyncUI(c, func() { message = util.PushMsg(r.language(100), 30000) })
		defer func() {
			o.base.owner.publishSyncUI(life, func() {
				if result != nil {
					util.PushClearMsg(message)
				}
			})
		}()
		cleaned, e := assetflow.Clean(c, io.Source, backup, selected, stamp)
		// Source publication is a sequential prefix; reconcile every successful
		// removal even if a later source or index operation reports a failure.
		critical := context.WithoutCancel(c)
		var tails []error
		if len(cleaned.Files) > 0 {
			tails = append(tails, r.queue.RemoveAssets(critical, cleaned.Hashes), r.queue.Flush(critical))
			for i, name := range cleaned.Files {
				if e := r.guard(critical); e != nil {
					tails = append(tails, e)
					break
				}
				cache.RemoveAssetHash(cleaned.Hashes[i])
				cache.RemoveAsset(name)
				util.RemoveAssetText(name)
				tails = append(tails, assetQueue.DeletePath(critical, name))
			}
			tails = append(tails, assetQueue.Flush(critical, nil))
		}
		if cleaned.Removed > 0 {
			tails = append(tails, history.index(critical, stamp, o.parser), cache.LoadAssetsContext(critical, io.Source.Data))
			o.base.owner.syncRuntime.same.Store(0)
			o.base.owner.planSyncAfter(time.Duration(o.settings.Sync.Interval) * time.Second)
		}
		result = errors.Join(append([]error{e}, tails...)...)
		if result != nil {
			return result
		}
		out = []string{}
		for _, name := range cleaned.Paths {
			out = append(out, filepath.Join(r.data, name))
		}
		o.base.owner.publishSyncUI(life, func() {
			util.PushUpdateMsg(message, fmt.Sprintf(r.language(91), len(out), humanize.BytesCustomCeil(uint64(cleaned.Size), 2)), 7000)
		})
		return c.Err()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
