// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"slices"
	"wiki-kernel/conf"
	"wiki-kernel/internal/anchor"
	"wiki-kernel/internal/doclife"
	"wiki-kernel/internal/docview"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/util"
)

func runReferenceChange(ctx context.Context, fn func(context.Context, context.Context, *workspaceReadOwner, *legacyAnchorContext) error) error {
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
		return fn(op, life, o, &legacyAnchorContext{r: r, commit: env.commitContext})
	})
}

func loadReferenceChangeTree(ctx context.Context, a *legacyAnchorContext, id string, cache map[string]*parse.Tree) (*parse.Tree, error) {
	loc, e := a.r.referenceLookup(ctx, id)
	if e != nil {
		return nil, e
	}
	if loc == nil {
		return nil, ErrBlockNotFound
	}
	key := loc.BoxID + loc.Path
	if tree := cache[key]; tree != nil {
		return tree, nil
	}
	tree, e := a.r.loadRepairTree(ctx, loc.BoxID, loc.Path, a.commit.parser())
	if e == nil {
		cache[key] = tree
	}
	return tree, e
}

func TransferBlockRefContext(ctx context.Context, fromID, toID string, ids []string) error {
	ids = slices.Clone(ids)
	return runReferenceChange(ctx, func(op, life context.Context, o *workspaceReadOwner, a *legacyAnchorContext) error {
		cache := map[string]*parse.Tree{}
		target, e := loadReferenceChangeTree(op, a, toID, cache)
		if e != nil {
			return e
		}
		node := docview.GetNodeInTree(target, toID)
		if node == nil {
			return ErrBlockNotFound
		}
		text, e := anchor.Text(op, node, a.commit.anchorMax, o.content())
		if e != nil {
			return e
		}
		if len(ids) == 0 {
			ids, e = (indexdb.References{DB: a.r.main.DB}).RefIDs(op, fromID, false)
			if e != nil {
				return e
			}
		}
		// All detached inputs are read before the first source publication.
		trees := make([]*parse.Tree, len(ids))
		for i, id := range ids {
			trees[i], e = loadReferenceChangeTree(op, a, id, cache)
			if e != nil {
				return e
			}
		}
		staged := map[string]bool{}
		order := []*parse.Tree{}
		for i, id := range ids {
			changed, e := doclife.TransferReferences(op, trees[i], id, fromID, toID, text)
			if e != nil {
				return e
			}
			key := trees[i].Box + trees[i].Path
			if changed && !staged[key] {
				staged[key] = true
				order = append(order, trees[i])
			}
		}
		for _, tree := range order {
			if e = a.stage(op, tree, nil); e != nil {
				return e
			}
		}
		return nil
	})
}

func SwapBlockRefContext(ctx context.Context, refID, defID string, includeChildren bool) error {
	return runReferenceChange(ctx, func(op, life context.Context, o *workspaceReadOwner, a *legacyAnchorContext) error {
		cache := map[string]*parse.Tree{}
		ref, e := loadReferenceChangeTree(op, a, refID, cache)
		if e != nil {
			return e
		}
		def, e := loadReferenceChangeTree(op, a, defID, cache)
		if e != nil {
			return e
		}
		changed, e := doclife.SwapReferences(op, ref, def, refID, defID, includeChildren, doclife.ReferenceSwapPorts{NewID: ast.NewNodeID, Updated: refreshUpdated})
		if e != nil || !changed {
			return e
		}
		if e = a.stage(op, ref, nil); e != nil {
			return e
		}
		if def != ref {
			if e = a.stage(op, def, nil); e != nil {
				return e
			}
		}
		o.base.owner.publishSyncUI(life, util.ReloadUI)
		return nil
	})
}
