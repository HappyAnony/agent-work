// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"fmt"
	"github.com/88250/lute/ast"
	"path"
	"strings"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/internal/assetflow"
	"wiki-kernel/internal/indexrepair"
	"wiki-kernel/util"
)

func RenameAssetContext(ctx context.Context, old, name string) (out string, err error) {
	o, e := captureWorkspaceRead(ctx)
	if e != nil {
		return "", e
	}
	r, e := captureIndexRepair(ctx)
	if e != nil {
		return "", e
	}
	if r.owner != o.base.owner || r.data != o.base.data {
		return "", conf.ErrBusinessUnbound
	}
	err = o.base.run(ctx, func(c, life context.Context) (result error) {
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
		boxes, e := o.base.owner.business.Notebooks(c)
		if e != nil {
			return e
		}
		ids := []string{}
		for _, box := range boxes {
			ids = append(ids, box.ID)
		}
		plan, e := assetflow.PrepareRename(c, io.Source, ids, old, name, ast.NewNodeID)
		if errors.Is(e, assetflow.ErrFilename) {
			return errors.New(r.language(151))
		}
		if e != nil {
			return e
		}
		if plan.NewPath == "" {
			return nil
		}
		o.base.owner.publishSyncUI(c, func() { util.PushEndlessProgress(r.language(110)) })
		defer o.base.owner.publishSyncUI(life, util.PushClearProgress)
		// Preserve the original copy: old assets remain available until a later
		// explicit cleanup. Each changed source keeps its individual CAS boundary.
		if e = io.Source.Rewrite(c, plan.CopyTo, nil, plan.CopyData); e != nil {
			return e
		}
		if plan.AnnotationPresent {
			if e = io.Source.Rewrite(c, plan.NewPath+".sya", nil, plan.Annotation); e != nil {
				return e
			}
		}
		defer func() { result = errors.Join(result, r.queue.Flush(context.WithoutCancel(c))) }()
		for _, change := range plan.Changes {
			if e = io.Source.Rewrite(c, change.Name, change.Before, change.After); e != nil {
				return e
			}
			critical := context.WithoutCancel(c)
			if change.Document {
				box, p, ok := strings.Cut(change.Name, "/")
				if !ok {
					return indexrepair.ErrSourcePath
				}
				tree, e := io.Load(critical, box, "/"+p, o.parser, false)
				if e != nil {
					return e
				}
				if e = r.registry.Upsert(critical, tree); e != nil {
					return e
				}
				if e = r.queue.Upsert(critical, tree); e != nil {
					return e
				}
				o.base.owner.publishSyncUI(life, func() {
					util.PushEndlessProgress(fmt.Sprintf(r.language(111), util.EscapeHTML(tree.Root.IALAttr("title"))))
				})
			} else {
				o.base.owner.publishSyncUI(life, func() {
					util.PushEndlessProgress(fmt.Sprintf(r.language(111), util.EscapeHTML(path.Base(change.Name))))
				})
			}
		}
		if e = r.guard(c); e != nil {
			return e
		}
		if text := util.GetAssetText(old); text != "" {
			util.SetAssetText(plan.NewPath, text)
		}
		o.base.owner.syncRuntime.same.Store(0)
		o.base.owner.planSyncAfter(time.Duration(o.settings.Sync.Interval) * time.Second)
		out = plan.NewPath
		return c.Err()
	})
	if err != nil {
		return "", err
	}
	return out, nil
}
