// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"encoding/json"
	"errors"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/internal/notebooks"
	"wiki-kernel/util"
)

func (c *AppConf) notebookGate() *attributeMutationGate {
	c.m.Lock()
	defer c.m.Unlock()
	if c.notebookMutationGate == nil {
		c.notebookMutationGate = newAttributeMutationGate()
	}
	return c.notebookMutationGate
}

func NotebookConfigurationContext(ctx context.Context, id string) (*conf.BoxConf, error) {
	return runWorkspaceMetadataRead(ctx, func(o *workspaceReadOwner, c context.Context) (out *conf.BoxConf, result error) {
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
		if e = io.Source.Check(id, false); e != nil {
			return nil, e
		}
		out, _, e = notebooks.Configuration(c, io.Source, id, ".siyuan")
		return out, e
	})
}
func SetNotebookConfigurationContext(ctx context.Context, id string, raw []byte) (*conf.BoxConf, error) {
	return mutateNotebookMetadata(ctx, notebooks.MetadataRequest{Kind: "configuration", ID: id, Patch: append(json.RawMessage(nil), raw...)})
}
func SetBoxIconContext(ctx context.Context, id, icon string) error {
	_, e := mutateNotebookMetadata(ctx, notebooks.MetadataRequest{Kind: "icon", ID: id, Icon: icon})
	return e
}
func ChangeBoxSortContext(ctx context.Context, ids []string) error {
	_, e := mutateNotebookMetadata(ctx, notebooks.MetadataRequest{Kind: "sort", IDs: append([]string(nil), ids...)})
	return e
}
func RenameBoxContext(ctx context.Context, id, name string) error {
	_, e := mutateNotebookMetadata(ctx, notebooks.MetadataRequest{Kind: "rename", ID: id, Name: name})
	return e
}
func mutateNotebookMetadata(ctx context.Context, q notebooks.MetadataRequest) (*conf.BoxConf, error) {
	return runWorkspaceMetadataRead(ctx, func(o *workspaceReadOwner, c context.Context) (out *conf.BoxConf, result error) {
		r, e := captureIndexRepair(c)
		if e != nil {
			return nil, e
		}
		if r.owner != o.base.owner || r.data != o.base.data {
			return nil, conf.ErrBusinessUnbound
		}
		gate := r.owner.notebookGate()
		result = gate.with(c, func() (err error) {
			if q.Kind == "rename" {
				if o.settings.Sync == nil {
					return conf.ErrBusinessUnbound
				}
				boxes, e := r.owner.business.Notebooks(c)
				if e != nil {
					return e
				}
				found := false
				for _, box := range boxes {
					if box.ID == q.ID && !box.Closed {
						found = true
						break
					}
				}
				if !found {
					return errors.New(r.language(0))
				}
			}
			if q.Kind == "sort" && o.settings.FileTree == nil {
				return conf.ErrBusinessUnbound
			}
			io, closeIO, e := r.openRepairIOChecked(c, true)
			if e != nil {
				return e
			}
			defer func() { err = joinRepairCloseError(err, closeIO()) }()
			q.Untitled = r.owner.business.Language(o.settings.Lang, 105, false, o.settings.CloudRegion)
			plan, e := notebooks.PrepareMetadata(c, io.Source, ".siyuan", q)
			if errors.Is(e, notebooks.ErrDailyPath) {
				return errors.New(r.language(49))
			}
			if errors.Is(e, notebooks.ErrNameLength) {
				return errors.New(r.language(106))
			}
			if e != nil {
				return e
			}
			for _, change := range plan.Changes {
				if e = io.Source.Rewrite(c, change.Path, change.Before, change.After); e != nil {
					return e
				}
			}
			out = plan.Configuration
			if q.Kind == "rename" {
				r.owner.syncRuntime.same.Store(0)
				r.owner.planSyncAfter(time.Duration(o.settings.Sync.Interval) * time.Second)
			}
			if q.Kind == "sort" {
				if o.settings.FileTree == nil {
					return conf.ErrBusinessUnbound
				}
				listed, e := notebooks.Scan(c, io.Source.Data, ".siyuan", o.settings.FileTree.Sort)
				if e != nil {
					return e
				}
				ids := []string{}
				for _, box := range listed.Notebooks {
					if !box.Closed {
						ids = append(ids, box.ID)
					}
				}
				r.owner.publishSyncUI(c, func() {
					util.BroadcastByType("main", "notebookSortChanged", 0, "", map[string]interface{}{"notebookIDs": ids})
				})
			}
			return c.Err()
		})
		return out, result
	})
}
