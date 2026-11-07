// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"wiki-kernel/conf"
	"wiki-kernel/internal/doclife"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/util"
)

func HeadingOperationsContext(ctx context.Context, q doclife.HeadingOperationsRequest) (*Transaction, error) {
	o, e := captureWorkspaceRead(ctx)
	if e != nil {
		return nil, e
	}
	var out *Transaction
	e = o.base.run(ctx, func(op, life context.Context) error {
		tree, e := o.load(op, q.ID)
		if e != nil {
			return e
		}
		plan, e := doclife.PrepareHeadingOperations(op, tree, q, doclife.HeadingOperationsServices{Parser: o.parser, NewID: ast.NewNodeID, Counts: (indexdb.References{DB: o.r.main.DB}).DefinitionCounts})
		if e != nil {
			return e
		}
		if !plan.Changed {
			return nil
		}
		convert := func(values []doclife.HeadingOperation) []*Operation {
			var out []*Operation
			for _, v := range values {
				op := &Operation{Action: v.Action, ID: v.ID, ParentID: v.ParentID, PreviousID: v.PreviousID}
				if v.DOM != "" {
					op.Data = v.DOM
				}
				if v.IgnoreProcess {
					op.Context = map[string]interface{}{"ignoreProcess": "true"}
				}
				out = append(out, op)
			}
			return out
		}
		out = &Transaction{DoOperations: convert(plan.Do), UndoOperations: convert(plan.Undo)}
		if plan.UnfoldedID != "" {
			if e = o.base.guard(op); e != nil {
				return e
			}
			o.base.owner.publishSyncUI(life, func() {
				util.BroadcastByType("protyle", "unfoldHeading", 0, "", map[string]interface{}{"id": plan.UnfoldedID, "currentNodeID": plan.UnfoldedID})
			})
		}
		return nil
	})
	if e != nil {
		return nil, e
	}
	return out, nil
}
func GetHeadingDeleteTransactionContext(ctx context.Context, id string) (*Transaction, error) {
	return HeadingOperationsContext(ctx, doclife.HeadingOperationsRequest{Mode: "delete", ID: id})
}
func GetHeadingInsertTransactionContext(ctx context.Context, id string) (*Transaction, error) {
	return HeadingOperationsContext(ctx, doclife.HeadingOperationsRequest{Mode: "insert", ID: id})
}
func GetHeadingLevelTransactionContext(ctx context.Context, id string, level int) (*Transaction, error) {
	return HeadingOperationsContext(ctx, doclife.HeadingOperationsRequest{Mode: "level", ID: id, Level: level})
}

func AppendHeadingChildrenContext(ctx context.Context, id, dom string) error {
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
		loc, e := r.referenceLookup(op, id)
		if e != nil {
			return e
		}
		if loc == nil {
			return ErrBlockNotFound
		}
		tree, e := r.loadRepairTree(op, loc.BoxID, loc.Path, o.parser)
		if e != nil {
			return e
		}
		changed, e := doclife.AppendHeadingChildren(op, tree, id, dom, o.parser)
		if e != nil {
			return e
		}
		if !changed {
			return nil
		}
		return (&legacyAnchorContext{r: r, commit: env.commitContext}).stage(op, tree, nil)
	})
}
