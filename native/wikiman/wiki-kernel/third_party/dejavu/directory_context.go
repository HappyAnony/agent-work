// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package dejavu

import (
	"context"
	"github.com/siyuan-note/dejavu/cloud"
	"sync"
)

// The coordination gate is retained while old Index/Checkout/Sync callers still
// require it. New directory callers can cancel admission; it owns no data/path.
type repositoryGate struct {
	once sync.Once
	gate chan struct{}
}

func (g *repositoryGate) init() { g.once.Do(func() { g.gate = make(chan struct{}, 1) }) }
func (g *repositoryGate) LockContext(ctx context.Context) error {
	g.init()
	select {
	case g.gate <- struct{}{}:
		if e := ctx.Err(); e != nil {
			<-g.gate
			return e
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (g *repositoryGate) Lock()   { _ = g.LockContext(context.Background()) }
func (g *repositoryGate) Unlock() { <-g.gate }
func WithRepositoryContext(ctx context.Context, fn func() error) error {
	if e := lock.LockContext(ctx); e != nil {
		return e
	}
	defer lock.Unlock()
	return fn()
}

// ScopedCloud exposes provider IO only to trusted adapters, never request JSON.
func (repo *Repo) ScopedCloud() cloud.Cloud { return repo.cloud }
