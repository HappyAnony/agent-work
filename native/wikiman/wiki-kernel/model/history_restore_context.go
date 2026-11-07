// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"github.com/88250/lute/ast"
	"os"
	"path/filepath"
	"strings"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/internal/cloudrepo"
	"wiki-kernel/internal/indexrepair"
	"wiki-kernel/util"
)

func RollbackAssetsHistoryContext(ctx context.Context, name string) error {
	return restoreHistoryResource(ctx, name, false)
}
func RollbackNotebookHistoryContext(ctx context.Context, name string) error {
	return restoreHistoryResource(ctx, name, true)
}

func restoreHistoryResource(ctx context.Context, name string, notebook bool) error {
	o, e := captureLegacyReindexOwner(ctx)
	if e != nil {
		return e
	}
	r, e := captureIndexRepair(ctx)
	if e != nil {
		return e
	}
	if o.owner != r.owner || o.data != r.data {
		return conf.ErrBusinessUnbound
	}
	selected := filepath.Clean(name)
	relative, e := filepath.Rel(o.history, selected)
	inside := e == nil && relative != "." && relative != ".." && !strings.HasPrefix(relative, ".."+string(os.PathSeparator))
	if !inside && !notebook {
		selected = filepath.Join(o.workspace, strings.TrimLeft(name, "/\\"))
		relative, e = filepath.Rel(o.history, selected)
		inside = e == nil && relative != "." && relative != ".." && !strings.HasPrefix(relative, ".."+string(os.PathSeparator))
	}
	if !inside {
		return indexrepair.ErrSourcePath
	}
	return o.run(ctx, func(op, life context.Context) (result error) {
		if e := r.guard(op); e != nil {
			return e
		}
		o.owner.m.Lock()
		bound := o.owner.confPath != ""
		interval := 0
		if o.owner.Sync != nil {
			interval = o.owner.Sync.Interval
		}
		lang, region := o.owner.Lang, o.owner.CloudRegion
		o.owner.m.Unlock()
		if !bound {
			return conf.ErrBusinessUnbound
		}
		info, e := os.Lstat(o.history)
		if os.IsNotExist(e) {
			return nil
		}
		if e != nil {
			return e
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return indexrepair.ErrSourcePath
		}
		history, e := os.OpenRoot(o.history)
		if e != nil {
			return e
		}
		defer func() { result = errors.Join(result, history.Close()) }()
		opened, e := history.Stat(".")
		if e != nil {
			return e
		}
		if !os.SameFile(info, opened) {
			return indexrepair.ErrSourceChanged
		}
		input := indexrepair.Source{Data: history}
		relative = filepath.ToSlash(relative)
		if e = input.Check(relative, false); os.IsNotExist(e) {
			return nil
		} else if e != nil {
			return e
		}
		ioctx, closeIO, e := r.openRepairIOChecked(op, true)
		if e != nil {
			return e
		}
		defer func() { result = errors.Join(result, closeIO()) }()
		if notebook {
			id := filepath.Base(selected)
			if !ast.IsNodeIDPattern(id) {
				return indexrepair.ErrSourcePath
			}
			if e = ioctx.Source.CopyDirectoryFrom(op, input, relative, id); e != nil {
				return e
			}
			service, e := o.owner.CloudRepoService()
			if e != nil {
				return e
			}
			if e = service.NotebookIndex(op, cloudrepo.NotebookIndexRequest{Kind: "full"}); e != nil {
				return e
			}
		} else {
			raw, e := input.Read(op, relative)
			if e != nil {
				return e
			}
			target := "assets/" + filepath.Base(selected)
			before, e := ioctx.Source.Read(op, target)
			if e != nil && !indexrepair.IsSourceNotExist(e) {
				return e
			}
			if e = ioctx.Source.Rewrite(op, target, before, raw); e != nil {
				return e
			}
			o.owner.publishSyncUI(life, func() { util.PushMsg(o.owner.business.Language(lang, 102, true, region), 3000) })
		}
		o.owner.syncRuntime.same.Store(0)
		o.owner.planSyncAfter(time.Duration(interval) * time.Second)
		return nil
	})
}
