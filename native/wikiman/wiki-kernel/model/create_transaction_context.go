// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/88250/lute/parse"
	"wiki-kernel/internal/txqueue"
)

var errBoundTransaction = errors.New("owned_create_transaction_failed")

type transactionCompletion struct {
	ctx   context.Context
	guard func(context.Context) error
	done  chan error
}

// This is a trusted compatibility create adapter, not a Native source writer.
// Retain the original FIFO worker, doCreate and legacy finish. The repository
// lease pins its original global finish dependencies until the callback returns.
func (r *indexRepairContext) createConflictTree(ctx context.Context, tree *parse.Tree, guard func(context.Context) error, collect ...func(*legacyCommitContext, []commitNotice) error) error {
	if e := guard(ctx); e != nil {
		return e
	}
	legacyTxQueueLock.Lock()
	queue := legacyTxQueue
	legacyTxQueueLock.Unlock()
	if queue == nil {
		return txqueue.ErrClosed
	}
	env := legacyTransactionEnvironmentContext(ctx)
	env.ctx = ctx
	if e := bindLegacyCommit(ctx, env, r); e != nil {
		return e
	}
	if len(collect) > 0 {
		env.commitContext.onComplete = func(_ context.Context, n []commitNotice) error { return collect[0](env.commitContext, n) }
	}
	env.stage = func(t *parse.Tree) error {
		if e := guard(ctx); e != nil {
			return e
		}
		return r.registry.Upsert(ctx, t)
	}
	finish := env.finish
	env.finish = func(tx *Transaction) error {
		if e := guard(ctx); e != nil {
			return e
		}
		return finish(tx)
	}
	completion := &transactionCompletion{ctx: ctx, guard: guard, done: make(chan error, 1)}
	tx := &Transaction{m: &sync.Mutex{}, environment: env, completion: completion, DoOperations: []*Operation{{Action: "create", Data: tree}}}
	if e := queue.Submit(ctx, tx); e != nil {
		return e
	}
	// An admitted write may ignore cancellation inside old finish helpers. Wait
	// for its actual result; returning on ctx.Done would release its dependencies.
	result := <-completion.done
	return errors.Join(result, ctx.Err())
}
func runBoundTransaction(tx *Transaction) {
	var err error
	defer func() {
		if recover() != nil {
			err = fmt.Errorf("%w: panic", errBoundTransaction)
		}
		tx.completion.done <- err
	}()
	if err = tx.completion.guard(tx.completion.ctx); err != nil {
		return
	}
	if failure := performTx(tx); failure != nil {
		err = fmt.Errorf("%w: %s", errBoundTransaction, failure.msg)
	}
}
