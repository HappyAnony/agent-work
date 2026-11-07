// Copyright (c) 2022-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package dejavu

import "context"

type CheckoutEffect struct{ Applied, Touched, Uncertain, Started bool }

// Caller chooses the critical-section cancellation policy. Runtime compatibility
// restoration shields source writes, then joins original derived-index rebuilds.
// This is not a native source transaction or an atomic multi-file checkout.
func (repo *Repo) CheckoutOwned(ctx context.Context, id string, event func(context.Context, string, ...interface{})) (out CheckoutEffect, err error) {
	if !validScopedObjectID(id) {
		return out, ErrSnapshot
	}
	if err = lock.LockContext(ctx); err != nil {
		return
	}
	defer lock.Unlock()
	oldScope, oldOp, oldScoped := repo.snapshot, repo.store.operation, repo.store.scoped
	s := &snapshotScope{ctx: ctx, event: event}
	repo.snapshot = s
	repo.store.operation = ctx
	repo.store.scoped = true
	defer func() {
		if recover() != nil {
			err = ErrSnapshot
		}
		out.Touched = s.mutated.Load()
		out.Uncertain = err != nil && out.Touched
		repo.snapshot = oldScope
		repo.store.operation = oldOp
		repo.store.scoped = oldScoped
	}()
	if err = checkoutNoSymlinks(ctx, repo.Path); err != nil {
		return
	}
	idx, e := repo.store.GetIndex(id)
	if e != nil {
		return out, e
	}
	if err = repo.validateCheckoutTarget(idx); err != nil {
		return
	}
	if err = repo.validateCheckoutSource(); err != nil {
		return
	}
	if err = ctx.Err(); err != nil {
		return
	}
	// The cancellable admission/validation phase is complete. From here the
	// original multi-file writer is joined to completion rather than interrupted.
	critical := context.WithoutCancel(ctx)
	s.ctx = critical
	repo.store.operation = critical
	out.Started = true
	_, _, err = repo.checkoutLocked(id, nil)
	out.Applied = err == nil
	return
}
func (repo *Repo) markCheckoutWrite() {
	if repo.snapshot != nil {
		repo.snapshot.mutated.Store(true)
	}
}
