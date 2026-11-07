// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"wiki-kernel/conf"
	"wiki-kernel/internal/doclife"
	"wiki-kernel/internal/notebooks"
)

func FileTreeConfigurationContext(ctx context.Context) (out *conf.FileTree, err error) {
	base, err := captureLegacyReindexOwner(ctx)
	if err != nil {
		return nil, err
	}
	err = base.owner.WithSettingSnapshot(ctx, func(v conf.AppData) error {
		if v.FileTree == nil {
			return conf.ErrBusinessUnbound
		}
		out = v.FileTree
		return base.guard(ctx)
	})
	if err != nil {
		out = nil
	}
	return
}

func CreateSavePathContext(ctx context.Context, current string, reference bool) (box, p string, err error) {
	type result struct{ box, path string }
	out, err := runWorkspaceRead(ctx, func(o *workspaceReadOwner, c context.Context) (out result, e error) {
		if o.settings.FileTree == nil {
			return out, conf.ErrBusinessUnbound
		}
		boxes, e := o.base.owner.business.Notebooks(c)
		if e != nil {
			return out, e
		}
		opened := map[string]bool{}
		for _, b := range boxes {
			opened[b.ID] = !b.Closed
		}
		var local *conf.BoxConf
		if opened[current] {
			io, closeIO, e := o.r.openRepairIOChecked(c, false)
			if e != nil {
				return out, e
			}
			local, _, e = notebooks.Configuration(c, io.Source, current, ".siyuan")
			e = joinRepairCloseError(e, closeIO())
			if e != nil {
				return out, e
			}
		}
		globalBox, globalPath := o.settings.FileTree.DocCreateSaveBox, o.settings.FileTree.DocCreateSavePath
		localBox, localPath := "", ""
		if reference {
			globalBox, globalPath = o.settings.FileTree.RefCreateSaveBox, o.settings.FileTree.RefCreateSavePath
		}
		if local != nil {
			localBox, localPath = local.DocCreateSaveBox, local.DocCreateSavePath
			if reference {
				localBox, localPath = local.RefCreateSaveBox, local.RefCreateSavePath
			}
		}
		out.box, p = doclife.SelectCreateSavePath(current, globalBox, globalPath, localBox, localPath, local != nil, reference, func(id string) bool { return opened[id] })
		out.path, e = doclife.RenderCreationText(c, p, false, o.funcs(c), nil)
		return out, e
	})
	return out.box, out.path, err
}
