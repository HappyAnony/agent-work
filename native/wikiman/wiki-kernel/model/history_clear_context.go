// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"os"
	"path"
	"path/filepath"
	"wiki-kernel/conf"
	"wiki-kernel/internal/indexrepair"
	"wiki-kernel/sql"
	"wiki-kernel/util"
)

func ClearWorkspaceHistoryContext(ctx context.Context) error {
	o, e := captureLegacyReindexOwner(ctx)
	if e != nil {
		return e
	}
	queue := sql.CaptureLegacyHistoryQueue()
	if queue.DB == nil {
		return conf.ErrBusinessUnbound
	}
	guard := func(c context.Context) error {
		if e := o.guard(c); e != nil {
			return e
		}
		if queue.DB != sql.CaptureLegacyHistoryQueue().DB {
			return conf.ErrBusinessUnbound
		}
		o.owner.m.Lock()
		readonly := o.owner.persistenceReadOnly
		bound := o.owner.confPath != ""
		o.owner.m.Unlock()
		if readonly {
			return ErrSettingReadOnly
		}
		if !bound {
			return conf.ErrBusinessUnbound
		}
		return nil
	}
	return o.run(ctx, func(op, life context.Context) (result error) {
		if e := guard(op); e != nil {
			return e
		}
		o.owner.m.Lock()
		lang, region := o.owner.Lang, o.owner.CloudRegion
		o.owner.m.Unlock()
		messageID := ""
		o.owner.publishSyncUI(op, func() { messageID = util.PushMsg(o.owner.business.Language(lang, 100, true, region), 1000*60*15) })
		defer func() {
			o.owner.publishSyncUI(life, func() {
				if result == nil {
					util.PushUpdateMsg(messageID, o.owner.business.Language(lang, 99, true, region), 5000)
				} else {
					util.PushClearMsg(messageID)
				}
			})
		}()
		boxes, e := o.owner.business.Notebooks(op)
		if e != nil {
			return e
		}
		parent, e := os.OpenRoot(filepath.Dir(o.history))
		if e != nil {
			return e
		}
		defer func() { result = errors.Join(result, parent.Close()) }()
		history := indexrepair.Source{Data: parent, Authorize: guard}
		if e = history.RemoveDirectory(op, filepath.Base(o.history)); e != nil {
			return e
		}
		// Clear on the captured queue; rebuilding the global DB would invalidate
		// unrelated admitted readers and discard other pending owner operations.
		if e = queue.Clear(op); e != nil {
			return e
		}
		if e = queue.Flush(context.WithoutCancel(op), nil); e != nil {
			return e
		}
		data, e := os.OpenRoot(o.data)
		if e != nil {
			return e
		}
		defer func() { result = errors.Join(result, data.Close()) }()
		source := indexrepair.Source{Data: data, Authorize: guard}
		for _, box := range boxes {
			if e = source.RemoveDirectory(op, path.Join(box.ID, ".siyuan", "history")); e != nil {
				return e
			}
		}
		for _, name := range []string{".siyuan/history", "assets/.siyuan/history"} {
			if e = source.RemoveDirectory(op, name); e != nil {
				return e
			}
		}
		return nil
	})
}
