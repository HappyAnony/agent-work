// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"wiki-kernel/internal/anchor"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/treenode"
)

// legacyAnchorContext freezes every compatibility dependency used by one
// dynamic-anchor refresh. It is an adapter for the original algorithm, not a
// new source transaction; R04 still owns atomic source commit and receipts.
type legacyAnchorContext struct {
	r      *indexRepairContext
	commit *legacyCommitContext
}

func captureLegacyAnchorContext(ctx context.Context) (*legacyAnchorContext, error) {
	r, err := captureIndexRepair(ctx)
	if err != nil {
		return nil, err
	}
	env := legacyTransactionEnvironmentContext(ctx)
	env.ctx = ctx
	if err = bindLegacyCommit(ctx, env, r); err != nil {
		return nil, err
	}
	return &legacyAnchorContext{r: r, commit: env.commitContext}, nil
}

func (o *legacyAnchorContext) services(ctx context.Context) anchor.Services {
	var mu sync.Mutex
	pendingRefs := map[string]*parse.Tree{}
	notices := map[string][]commitNotice{}
	s := anchor.Services{Embedded: treenode.GetEmbedBlockRef, StrictLoad: true}
	s.References = func(ctx context.Context, n *ast.Node) ([]indexdb.RefRecord, []*ast.Node, error) {
		if err := o.r.guard(ctx); err != nil {
			return nil, nil, err
		}
		return anchor.References(ctx, n, o.commit.refs)
	}
	s.Load = func(ctx context.Context, id string) (*parse.Tree, error) {
		bt, err := o.r.referenceLookup(ctx, id)
		if err != nil {
			return nil, err
		}
		if bt == nil {
			return nil, ErrBlockNotFound
		}
		return o.r.loadRepairTree(ctx, bt.BoxID, bt.Path, o.commit.parser())
	}
	s.Text = func(ctx context.Context, n *ast.Node) (string, error) {
		return anchor.Text(ctx, n, o.commit.anchorMax, indexdb.ContentServices{AttributeView: func(ctx context.Context, id string, full bool) (string, error) {
			if err := o.r.guard(ctx); err != nil {
				return "", err
			}
			return o.commit.avContext(ctx).title(id)
		}})
	}
	s.Set = func(n *ast.Node, text string) error {
		if err := o.r.guard(ctx); err != nil {
			return err
		}
		return anchor.SetText(n, text, o.commit.dynamic)
	}
	s.Notify = func(ctx context.Context, change anchor.Change) error {
		if err := o.r.guard(ctx); err != nil {
			return err
		}
		mu.Lock()
		notices[change.RootID] = append(notices[change.RootID], commitNotice{kind: "anchor", change: change, delay: 200 * time.Millisecond})
		mu.Unlock()
		return nil
	}
	s.IndexRefs = func(ctx context.Context, tree *parse.Tree) error {
		if err := o.r.guard(ctx); err != nil {
			return err
		}
		mu.Lock()
		pendingRefs[tree.ID] = tree
		mu.Unlock()
		return nil
	}
	s.UpdateAVs = func(ctx context.Context, nodes map[string]*ast.Node) error {
		binding := bindAVOperation(o.commit, ctx)
		return updateAttributeViewBlockTextWithEnvironment(nodes, legacyAVTextEnvironment(ctx, binding))
	}
	s.Stage = func(ctx context.Context, tree *parse.Tree) error {
		mu.Lock()
		refs := pendingRefs[tree.ID]
		delete(pendingRefs, tree.ID)
		pending := append([]commitNotice(nil), notices[tree.ID]...)
		delete(notices, tree.ID)
		mu.Unlock()
		if err := o.stage(ctx, tree, refs); err != nil {
			return err
		}
		if len(pending) > 0 {
			return o.commit.completeUI(ctx, pending, false)
		}
		return ctx.Err()
	}
	return s
}

func (o *legacyAnchorContext) stage(ctx context.Context, tree, refs *parse.Tree) (err error) {
	if err = o.r.guard(ctx); err != nil {
		return err
	}
	ioctx, closeIO, err := o.r.openRepairIOChecked(ctx, true)
	if err != nil {
		return err
	}
	defer func() { err = joinRepairCloseError(err, closeIO()) }()
	if _, known := o.r.revisions.Load(tree.Box + tree.Path); !known {
		raw, readErr := ioctx.Source.Read(ctx, tree.Box+tree.Path)
		if readErr != nil {
			return readErr
		}
		ioctx.Remember(tree, raw)
	}
	if err = o.r.writeRepairTree(ctx, ioctx, tree, o.commit.parser()); err != nil {
		return err
	}
	critical := context.WithoutCancel(ctx)
	if err = o.r.guard(critical); err != nil {
		return err
	}
	if err = o.r.registry.Upsert(critical, tree); err != nil {
		return err
	}
	admitted := false
	defer func() {
		if admitted {
			err = errors.Join(err, indexrepairDrain(critical, o.r.queue))
		}
	}()
	if err = o.r.queue.Upsert(critical, tree); err != nil {
		return err
	}
	admitted = true
	if refs != nil {
		if err = o.r.queue.UpdateRefs(critical, refs); err != nil {
			return err
		}
	}
	if err = o.r.queue.Flush(critical); err != nil {
		return err
	}
	admitted = false
	return ctx.Err()
}

func (o *legacyAnchorContext) refresh(ctx context.Context, nodes map[string]*ast.Node, trees map[string]*parse.Tree) error {
	if o == nil || o.r == nil || o.commit == nil {
		return anchor.ErrDependency
	}
	if err := o.r.guard(ctx); err != nil {
		return err
	}
	return anchor.Refresh(ctx, nodes, trees, o.services(ctx))
}

func legacyAnchorServicesContext(ctx context.Context) (anchor.Services, error) {
	o, err := captureLegacyAnchorContext(ctx)
	if err != nil {
		return anchor.Services{}, err
	}
	return o.services(ctx), nil
}
