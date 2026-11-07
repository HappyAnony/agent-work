// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/internal/doclife"
	"wiki-kernel/sql"
)

func AutoSpaceContext(ctx context.Context, id string) error {
	o, e := captureWorkspaceRead(ctx)
	if e != nil {
		return e
	}
	r, e := captureIndexRepair(ctx)
	if e != nil {
		return e
	}
	if r.owner != o.base.owner || r.data != o.base.data {
		return conf.ErrBusinessUnbound
	}
	return o.base.run(ctx, func(op, life context.Context) error {
		if e := r.guard(op); e != nil {
			return e
		}
		if e := FlushTxQueueContext(op); e != nil {
			return e
		}
		env := legacyTransactionEnvironmentContext(op)
		env.ctx = op
		env.parser = func() *lute.Lute { return o.parser }
		if e := bindLegacyCommit(op, env, r); e != nil {
			return e
		}
		bt, e := r.referenceLookup(op, id)
		if e != nil {
			return e
		}
		if bt == nil {
			return ErrBlockNotFound
		}
		tree, e := r.loadRepairTree(op, bt.BoxID, bt.Path, o.parser)
		if e != nil {
			return e
		}
		h := &historyIndexContext{owner: r.owner, root: o.base.history, workspace: r.workspace, queue: sql.CaptureLegacyHistoryQueue(), untitled: r.language(16)}
		if e := writeTransactionHistory(op, env.commitContext, h, tree, time.Now(), func(name string) error { return h.index(op, name, o.parser) }, HistoryOpFormat); e != nil {
			return e
		}
		next, e := doclife.Format(op, tree, o.parser, doclife.TemplateBodyServices{NewID: ast.NewNodeID, Now: time.Now})
		if e != nil {
			return e
		}
		if e = (&legacyAnchorContext{r: r, commit: env.commitContext}).stage(op, next, next); e != nil {
			return e
		}
		if e = op.Err(); e != nil {
			return e
		}
		return reloadProtyleWithEnvironment(tree.ID, sourceEventEnvironmentForOwner(life, &legacyAnchorContext{r: r, commit: env.commitContext}, nil))
	})
}
