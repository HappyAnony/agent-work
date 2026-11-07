// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"github.com/88250/lute"
	"github.com/siyuan-note/eventbus"
	"wiki-kernel/conf"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/sql"
	"wiki-kernel/util"
)

type historyIndexContext struct {
	owner                     *AppConf
	root, workspace, untitled string
	queue                     sql.LegacyHistoryQueue
	notify                    func(context.Context, indexdb.HistoryProgress) error
}

func captureHistoryIndexContext(ctx context.Context) (*historyIndexContext, error) {
	owner := Conf
	if owner == nil {
		return nil, conf.ErrBusinessUnbound
	}
	h := &historyIndexContext{owner: owner, root: util.HistoryDir, workspace: util.WorkspaceDir, queue: sql.CaptureLegacyHistoryQueue()}
	if e := h.guard(ctx); e != nil {
		return nil, e
	}
	if e := owner.WithSettingSnapshot(ctx, func(v conf.AppData) error {
		h.untitled = owner.business.Language(v.Lang, 16, false, v.CloudRegion)
		return nil
	}); e != nil {
		return nil, e
	}
	h.notify = func(ctx context.Context, p indexdb.HistoryProgress) error {
		if e := h.guard(ctx); e != nil {
			return e
		}
		owner.repoMu.Lock()
		closed := owner.repoClosed
		owner.repoMu.Unlock()
		if closed {
			return nil
		}
		eventbus.Publish(eventbus.EvtSQLInsertHistory, h.queue.EventContext(map[string]interface{}{eventbus.CtxPushMsg: eventbus.CtxPushMsgToStatusBar, "current": p.Current, "total": p.Total}))
		return nil
	}
	return h, nil
}
func (h *historyIndexContext) guard(ctx context.Context) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	if h.owner != Conf || h.root != util.HistoryDir || h.workspace != util.WorkspaceDir || h.queue.DB != sql.CaptureLegacyHistoryQueue().DB {
		return conf.ErrBusinessUnbound
	}
	h.owner.repoMu.Lock()
	closed := h.owner.repoClosed
	h.owner.repoMu.Unlock()
	if closed {
		return conf.ErrBusinessUnbound
	}
	return nil
}
func indexHistoryDirContext(ctx context.Context, name string, engine *lute.Lute) error {
	h, e := captureHistoryIndexContext(ctx)
	if e != nil {
		return e
	}
	return h.index(ctx, name, engine)
}
func (h *historyIndexContext) index(ctx context.Context, name string, engine *lute.Lute) (err error) {
	return h.indexWithIO(ctx, name, engine, historyIndexIO{})
}
func (h *historyIndexContext) indexWithIO(ctx context.Context, name string, engine *lute.Lute, op historyIndexIO) (err error) {
	defer recoverRepositoryWorkflow(&err, "history_index")
	entries, present, e := h.scan(ctx, name, engine, op)
	if e != nil {
		return e
	}
	if !present {
		return nil
	}
	if e = h.guard(ctx); e != nil {
		return e
	}
	if e = h.queue.ReplaceDirectory(ctx, name, entries); e != nil {
		return e
	}
	// An accepted batch is actually drained before its borrowed roots/DB can be
	// released. Progress values are published only after the real transaction.
	var progress []indexdb.HistoryProgress
	critical := context.WithoutCancel(ctx)
	if e = h.queue.Flush(critical, func(p indexdb.HistoryProgress) { progress = append(progress, p) }); e != nil {
		return e
	}
	if e = h.guard(ctx); e != nil {
		return e
	}
	for _, p := range progress {
		if e = h.guard(ctx); e != nil {
			return e
		}
		if h.notify != nil {
			if e = h.notify(ctx, p); e != nil {
				return e
			}
		}
	}

	return nil
}
