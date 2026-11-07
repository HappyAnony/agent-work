// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"github.com/siyuan-note/logging"
	"time"
	"wiki-kernel/av"
	"wiki-kernel/internal/avmodel"
	"wiki-kernel/internal/avrender"
	"wiki-kernel/internal/taskgroup"
	"wiki-kernel/treenode"
)

// Capture at adapter construction, not inside a delayed callback. A failed
// binding remains an error-valued port; it never falls back to current globals.
// Other operation-specific legacy mutations still need separate migration.
type legacyAVBinding struct {
	scope        *commitAVContext
	err          error
	notices      []commitNotice
	detachedSave bool // Historical read projection may repair only its candidate.
}

func newLegacyAVBinding(ctx context.Context) *legacyAVBinding {
	b := &legacyAVBinding{}
	env := legacyTransactionEnvironmentContext(ctx)
	b.err = bindLegacyCommit(ctx, env, nil)
	if b.err == nil {
		return bindAVOperation(env.commitContext, ctx)
	}
	return b
}
func (b *legacyAVBinding) guard() error {
	if b.err != nil {
		return b.err
	}
	return b.scope.guard()
}
func (b *legacyAVBinding) read(id string) (*av.AttributeView, error) {
	if e := b.guard(); e != nil {
		return nil, e
	}
	return b.scope.read(id)
}
func (b *legacyAVBinding) clone(a *av.AttributeView) (*av.AttributeView, error) {
	if e := b.guard(); e != nil {
		return nil, e
	}
	return b.scope.clone(a)
}
func (b *legacyAVBinding) save(a *av.AttributeView) error {
	if e := b.guard(); e != nil {
		return e
	}
	if b.detachedSave && b.scope.readOnly {
		if a == nil {
			return avrender.ErrShape
		}
		b.scope.bind(a)
		b.scope.cached[a.ID] = a
		return b.guard()
	}
	return b.scope.save(a)
}
func (b *legacyAVBinding) sources(id string) ([]string, error) {
	if e := b.guard(); e != nil {
		return nil, e
	}
	return b.scope.sources(id)
}
func (b *legacyAVBinding) text(n *ast.Node, id string) (string, string, error) {
	if e := b.guard(); e != nil {
		return "", "", e
	}
	return b.scope.text(n, id)
}

// The original caller may have already edited its detached candidate. Capture
// the source revision without discarding those edits or clearing group state.
func (b *legacyAVBinding) adopt(a *av.AttributeView) error {
	if e := b.guard(); e != nil {
		return e
	}
	if a == nil {
		return avrender.ErrShape
	}
	if _, known := b.scope.versions[a.ID]; !known {
		if _, e := b.scope.read(a.ID); e != nil {
			return e
		}
	}
	b.scope.bind(a)
	b.scope.cached[a.ID] = a
	return nil
}
func (b *legacyAVBinding) groups(a *av.AttributeView) error {
	if e := b.adopt(a); e != nil {
		return e
	}
	// Consecutive edits share candidates, but never a projection of an older value.
	for _, candidate := range b.scope.cached {
		candidate.RenderedViewables = map[string]av.Viewable{}
	}
	return b.scope.groups(a)
}
func (b *legacyAVBinding) group(a *av.AttributeView, v *av.View) error {
	if e := b.adopt(a); e != nil {
		return e
	}
	if v == nil {
		return avrender.ErrShape
	}
	return avrender.GenerateGroups(v, a, avrender.GroupEnvironment{Now: func() time.Time { return b.scope.now }, Read: b.scope.read, Render: b.scope.render, NewView: avrender.NewGroupView})
}
func (b *legacyAVBinding) reload(id string) error {
	if e := b.guard(); e != nil {
		return e
	}
	b.notices = append(b.notices, commitNotice{kind: "av", id: id, delay: 200 * time.Millisecond})
	return nil
}
func (b *legacyAVBinding) related(id string, tx *Transaction) error {
	if e := b.guard(); e != nil {
		return e
	}
	return collectRelatedAttributeViews(b.scope.ctx, id, b.sources, func(ids []string) error {
		if tx != nil {
			tx.relatedAvIDs = append(tx.relatedAvIDs, ids...)
			return nil
		}
		return b.scope.flush(ids, b.reload)
	})
}
func (b *legacyAVBinding) complete() error {
	if e := b.guard(); e != nil {
		return e
	}
	if len(b.notices) == 0 {
		return nil
	}
	if e := b.scope.commit.completeUI(b.scope.ctx, b.notices, false); e != nil {
		return e
	}
	b.notices = nil
	return nil
}

func legacyAVBindingForTransaction(tx *Transaction) *legacyAVBinding {
	if tx != nil && tx.environment != nil && tx.environment.commitContext != nil {
		return bindAVOperation(tx.environment.commitContext, tx.environment.ctx)
	}
	ctx := context.Background()
	if tx != nil && tx.environment != nil && tx.environment.ctx != nil {
		ctx = tx.environment.ctx
	}
	return newLegacyAVBinding(ctx)
}

func legacyAVContext(contexts []context.Context) context.Context {
	if len(contexts) > 0 && contexts[0] != nil {
		return contexts[0]
	}
	return context.Background()
}
func transactionAVContext(tx *Transaction) context.Context {
	if tx != nil && tx.environment != nil && tx.environment.ctx != nil {
		return tx.environment.ctx
	}
	return context.Background()
}

func (b *legacyAVBinding) filter(out av.Viewable, a *av.AttributeView) error {
	if e := b.guard(); e != nil {
		return e
	}
	further, e := avrender.FurtherCollections(b.scope.ctx, a, b.scope.cached, avrender.FurtherEnvironment{Read: b.scope.read, Render: b.scope.render})
	if e != nil {
		return e
	}
	return avmodel.FilterChecked(out, a, further, b.scope.cached)
}
func (b *legacyAVBinding) groupRender(a *av.AttributeView, v, g *av.View, q string) (av.Viewable, error) {
	if e := b.adopt(a); e != nil {
		return nil, e
	}
	if e := avrender.PrepareGroup(v, g); e != nil {
		return nil, e
	}
	return b.scope.render(a, g, q)
}
func (b *legacyAVBinding) process(out av.Viewable, v *av.View, a *av.AttributeView, page, size int) error {
	if out == nil {
		return av.ErrViewNotFound
	}
	if e := b.filter(out, a); e != nil {
		return e
	}
	av.Sort(out, a)
	av.Calc(out, a)
	if e := b.guard(); e != nil {
		return e
	}
	return paginateAttributeView(out, v, page, size)
}
func (b *legacyAVBinding) near(a *av.AttributeView, v, g *av.View, previous string) (av.Item, error) {
	out, e := b.groupRender(a, v, g, "")
	if e != nil {
		return nil, e
	}
	if e = b.filter(out, a); e != nil {
		return nil, e
	}
	av.Sort(out, a)
	return selectAttributeViewNearItem(out.(av.Collection), previous), b.guard()
}

func (b *legacyAVBinding) count(a *av.AttributeView, v, g *av.View) (int, error) {
	out, e := b.groupRender(a, v, g, "")
	if e != nil {
		return 0, e
	}
	if e = b.process(out, v, a, 1, -1); e != nil {
		return 0, e
	}
	return out.(av.Collection).CountItems(), nil
}

func (b *legacyAVBinding) withContext(ctx context.Context) *legacyAVBinding {
	if b.err != nil {
		return &legacyAVBinding{err: b.err}
	}
	local := bindAVOperation(b.scope.commit, ctx)
	local.scope.readOnly = b.scope.readOnly
	return local
}

// Preserve the original 100ms delay, but own the real callback and pass only
// copied IDs plus frozen dependencies across the asynchronous boundary.
func (b *legacyAVBinding) scheduleUpdated(ids []string) error {
	if e := b.guard(); e != nil {
		return e
	}
	frozen := *b.scope.commit
	frozen.avWarning = nil
	ids = append([]string(nil), ids...)
	owner := frozen.r.owner
	owner.repoMu.Lock()
	defer owner.repoMu.Unlock()
	if owner.repoClosed {
		return errors.New("AV refresh owner closed")
	}
	if owner.repoTasks == nil {
		owner.repoTasks = taskgroup.Open()
	}
	request := b.scope.ctx
	if !owner.repoTasks.Go(func(life context.Context) {
		ctx, cancel := context.WithCancel(life)
		stop := context.AfterFunc(request, cancel)
		defer func() { stop(); cancel() }()
		defer func() {
			if recover() != nil {
				logging.LogErrorf("AV refresh callback failed")
			}
		}()
		timer := time.NewTimer(100 * time.Millisecond)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		local := bindAVOperation(&frozen, ctx)
		e := &attributeViewUpdateEnvironment{ctx: ctx, strictDependencies: true, read: local.read, sources: local.sources, groups: local.groups, stage: local.save, reload: local.reload, complete: local.complete}
		if err := refreshUpdatedAttributeViews(ids, e); err != nil && ctx.Err() == nil {
			logging.LogErrorf("AV refresh failed: %s", err)
		}
	}) {
		return errors.New("AV refresh owner closed")
	}
	return nil
}

// Each operation owns its warning/event queue even when it inherits a
// transaction's immutable commit dependencies.
func bindAVOperation(c *legacyCommitContext, ctx context.Context) *legacyAVBinding {
	// Exclude mutable finish hooks and callbacks that retain the parent commit.
	copy := legacyCommitContext{r: c.r, parser: c.parser, interval: c.interval, relative: c.relative, anchorMax: c.anchorMax, duration: c.duration, avUntitled: c.avUntitled, avLanguage: c.avLanguage, avLabels: c.avLabels, prod: c.prod, refs: c.refs, ial: c.ial, dynamic: c.dynamic, publish: c.publish}
	b := &legacyAVBinding{scope: copy.avContext(ctx)}
	copy.avWarning = func(message string) {
		b.notices = append(b.notices, commitNotice{kind: "guide", payload: mustCommitJSON(message)})
	}
	return b
}
func (b *legacyAVBinding) readCandidate(id string) (*av.AttributeView, error) {
	if e := b.guard(); e != nil {
		return nil, e
	}
	return b.scope.load(id, false)
}
func (b *legacyAVBinding) readView(a *av.AttributeView, block string) (*av.View, error) {
	if e := b.guard(); e != nil {
		return nil, e
	}
	e := &attributeViewReadEnvironment{ctx: b.scope.ctx, node: func(id string) (*ast.Node, *parse.Tree, error) {
		trees, e := b.scope.trees([]string{id})
		if e != nil {
			return nil, nil, e
		}
		t := trees[id]
		if t == nil {
			return nil, nil, nil
		}
		return treenode.GetNodeInTree(t, id), t, nil
	}}
	return selectAttributeViewReadView(a, block, e)
}
func (b *legacyAVBinding) relation(src, dest string, add bool) error {
	if b.err == nil && b.scope.readOnly {
		return errors.New("AV read-only scope cannot change relations")
	}
	if e := b.guard(); e != nil {
		return e
	}
	if !ast.IsNodeIDPattern(src) || !ast.IsNodeIDPattern(dest) {
		return avrender.ErrShape
	}
	const name = "storage/av/relations.msgpack"
	return av.MutateRelations(b.scope.ctx, src, dest, add, func() ([]byte, error) { return b.scope.source(name) }, func(before, after []byte) (repairIOErr error) {
		if e := b.guard(); e != nil {
			return e
		}
		root, closeIO, e := b.scope.commit.r.openRepairIOChecked(b.scope.ctx, true)
		if e != nil {
			return e
		}
		defer func() { repairIOErr = joinRepairCloseError(repairIOErr, closeIO()) }()
		return root.Source.Rewrite(b.scope.ctx, name, before, after)
	})
}
func (b *legacyAVBinding) removeRelation(src, dest string) error { return b.relation(src, dest, false) }
func (b *legacyAVBinding) upsertRelation(src, dest string) error { return b.relation(src, dest, true) }
