// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"wiki-kernel/conf"
	"wiki-kernel/internal/notebooks"
	"wiki-kernel/util"
)

type NotebookOpenPolicy struct {
	Message      string
	CloseTimeout int
}

func NotebookOpenPolicyContext(ctx context.Context, id string) (out NotebookOpenPolicy, err error) {
	return notebookSourcePolicyContext(ctx, id, true)
}
func NotebookRemovePolicyContext(ctx context.Context, id string) (NotebookOpenPolicy, error) {
	return notebookSourcePolicyContext(ctx, id, false)
}
func notebookSourcePolicyContext(ctx context.Context, id string, opening bool) (out NotebookOpenPolicy, err error) {
	o, e := captureLegacyReindexOwner(ctx)
	if e != nil {
		return out, e
	}
	err = o.owner.WithSettingSnapshot(ctx, func(v conf.AppData) error {
		o.owner.m.Lock()
		readonly := o.owner.persistenceReadOnly
		o.owner.m.Unlock()
		guide := notebooks.Guide(id)
		code := 0
		if (readonly || util.ReadOnly) && !guide {
			code = 34
			out.CloseTimeout = 5000
		} else if opening && guide && util.Container == util.ContainerIOS {
			code = 215
			out.CloseTimeout = 7000
		}
		if code != 0 {
			out.Message = o.owner.business.Language(v.Lang, code, true, v.CloudRegion)
		}
		return o.guard(ctx)
	})
	if err != nil {
		return NotebookOpenPolicy{}, err
	}
	return out, nil
}
func NotebookOpenProgressContext(ctx context.Context) (func(), error) {
	o, e := captureLegacyReindexOwner(ctx)
	if e != nil {
		return nil, e
	}
	message := ""
	if e = o.owner.WithSettingSnapshot(ctx, func(v conf.AppData) error {
		message = o.owner.business.Language(v.Lang, 45, true, v.CloudRegion)
		return o.guard(ctx)
	}); e != nil {
		return nil, e
	}
	key := ""
	o.owner.publishSyncUI(ctx, func() { key = util.PushMsg(message, 1000*60*15) })
	return func() { o.owner.publishSyncUI(context.WithoutCancel(ctx), func() { util.PushClearMsg(key) }) }, nil
}
