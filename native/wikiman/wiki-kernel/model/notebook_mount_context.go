// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/internal/indexrepair"
	"wiki-kernel/internal/notebooks"
	"wiki-kernel/task"
	"wiki-kernel/util"
)

func publishNotebookClosed(ctx context.Context, source indexrepair.Source, id string, closed bool) (*conf.BoxConf, error) {
	plan, e := notebooks.PrepareMetadata(ctx, source, ".siyuan", notebooks.MetadataRequest{Kind: "closed", ID: id, Closed: closed})
	if e != nil {
		return nil, e
	}
	for _, change := range plan.Changes {
		if e = source.Rewrite(ctx, change.Path, change.Before, change.After); e != nil {
			return nil, e
		}
	}
	return plan.Configuration, nil
}
func MountContext(ctx context.Context, id string) (already bool, err error) {
	_, err = runWorkspaceMetadataRead(ctx, func(o *workspaceReadOwner, c context.Context) (out struct{}, result error) {
		r, e := captureIndexRepair(c)
		if e != nil {
			return out, e
		}
		ui, e := captureLegacyDelayedUIOwner(c)
		if e != nil {
			return out, e
		}
		if r.owner != o.base.owner || r.data != o.base.data || ui.owner != r.owner {
			return out, conf.ErrBusinessUnbound
		}
		guide := notebooks.Guide(id)
		r.guideWrite = guide
		result = r.owner.notebookGate().with(c, func() (result error) {
			if e := r.guard(c); e != nil {
				return e
			}
			if o.settings.FileTree == nil {
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
			remount := false
			if guide {
				if e = io.Source.Check(id, false); e == nil {
					remount = true
					if _, e = publishNotebookClosed(c, io.Source, id, true); e != nil {
						return e
					}
					if e = r.unindexNotebook(c, id); e != nil {
						return e
					}
				} else if !indexrepair.IsSourceNotExist(e) {
					return e
				}
				if e = io.Source.RemoveDirectory(c, id); e != nil {
					return e
				}
				listed, e := notebooks.Scan(c, io.Source.Data, ".siyuan", o.settings.FileTree.Sort)
				if e != nil {
					return e
				}
				sort := 0
				if len(listed.Notebooks) > 0 {
					sort = listed.Notebooks[0].Sort - 1
				}
				guideRoot := captureHTMLResourceRoot(filepath.Join(ui.working, "guide"))
				root, e := guideRoot.open()
				if e != nil {
					return e
				}
				input := indexrepair.Source{Data: root}
				e = io.Source.CopyDirectoryFrom(c, input, id, id)
				if e == nil {
					p := id + "/storage/av"
					check := input.Check(p, false)
					if check == nil {
						e = io.Source.CopyDirectoryFrom(c, input, p, "storage/av")
					} else if !indexrepair.IsSourceNotExist(check) {
						e = check
					}
				}
				e = errors.Join(e, root.Close())
				if e != nil {
					return e
				}
				value, before, e := notebooks.Configuration(c, io.Source, id, ".siyuan")
				if e != nil {
					return e
				}
				value.Closed = true
				value.Sort = sort
				raw, e := json.MarshalIndent(value, "", "  ")
				if e != nil {
					return e
				}
				if e = io.Source.Rewrite(c, id+"/.siyuan/conf.json", before, raw); e != nil {
					return e
				}
				if e = r.owner.WithSettingSnapshot(c, func(v conf.AppData) error {
					if !v.OpenHelp {
						return nil
					}
					if e := o.base.guard(c); e != nil {
						return e
					}
					v.OpenHelp = false
					r.owner.m.Lock()
					defer r.owner.m.Unlock()
					if e := r.owner.saveDataLocked(v); e != nil {
						return e
					}
					r.owner.AppData = v
					return nil
				}); e != nil {
					return e
				}
				message := ui.language(52, true)
				if e = ui.schedule(context.WithoutCancel(c), task.PushMsg, 3*time.Second, func() { util.PushErrMsg(message, 7000) }, message, 7000); e != nil {
					return e
				}
			}
			value, _, e := notebooks.Configuration(c, io.Source, id, ".siyuan")
			if e != nil {
				return e
			}
			if !value.Closed {
				already = true
				return nil
			}
			if _, e = publishNotebookClosed(c, io.Source, id, false); e != nil {
				return e
			}
			if e = r.indexNotebook(c, id); e != nil {
				return e
			}
			if e = r.indexReferences(c); e != nil {
				return e
			}
			if e = ResetVirtualBlockRefCacheContext(c); e != nil {
				return e
			}
			if _, _, e = ListDocTreeContext(c, id, "/", util.SortModeUnassigned, false, false, o.settings.FileTree.MaxListCount); e != nil {
				return e
			}
			r.owner.publishSyncUI(c, func() { util.ClearPushProgress(100) })
			already = remount
			return c.Err()
		})
		return out, result
	})
	if err != nil {
		return false, err
	}
	return already, nil
}

func UnmountContext(ctx context.Context, id string) error {
	_, e := runWorkspaceMetadataRead(ctx, func(o *workspaceReadOwner, c context.Context) (out struct{}, result error) {
		r, e := captureIndexRepair(c)
		if e != nil {
			return out, e
		}
		if r.owner != o.base.owner || r.data != o.base.data {
			return out, conf.ErrBusinessUnbound
		}
		result = r.owner.notebookGate().with(c, func() (result error) {
			if e := FlushTxQueueContext(c); e != nil {
				return e
			}
			io, closeIO, e := r.openRepairIOChecked(c, true)
			if e != nil {
				return e
			}
			defer func() { result = joinRepairCloseError(result, closeIO()) }()
			if _, e = publishNotebookClosed(c, io.Source, id, true); e != nil {
				return e
			}
			if e = r.unindexNotebook(context.WithoutCancel(c), id); e != nil {
				return e
			}
			r.owner.publishSyncUI(context.WithoutCancel(c), func() {
				evt := util.NewCmdResult("unmount", 0, util.PushModeBroadcast)
				evt.Data = map[string]interface{}{"box": id}
				util.PushEvent(evt)
			})
			return c.Err()
		})
		return out, result
	})
	return e
}
