// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package notebookindex

import (
	"context"
	"errors"
	"github.com/88250/lute/ast"
	"sync"
	"wiki-kernel/internal/indexrepair"
	"wiki-kernel/internal/taskgroup"
)

type DocumentPath = indexrepair.DocumentPath
type Services struct {
	Workers int
	Process func(context.Context, DocumentPath) ([]*ast.Node, error)
	Flush   func(context.Context) error
	Bind    func(context.Context, []*ast.Node) error
	History func(context.Context) error
}

var ErrDependency = errors.New("notebook_index_unbound")

func guardedDrain(ctx context.Context, flush func(context.Context) error) (err error) {
	defer func() {
		if recover() != nil {
			err = errors.New("notebook_index_drain_failed")
		}
	}()
	return flush(context.WithoutCancel(ctx))
}

// Rebuild joins every admitted worker before draining and before AV /
// history completion. A submission canceled before starting has no wait token.
func Rebuild(ctx context.Context, docs []DocumentPath, s Services) (err error) {
	defer func() {
		if recover() != nil {
			err = errors.New("notebook_index_completion_failed")
		}
	}()
	if s.Process == nil || s.Flush == nil || s.Bind == nil || s.History == nil {
		return ErrDependency
	}
	if err = ctx.Err(); err != nil {
		return
	}
	workers := s.Workers
	if workers < 1 || workers > 4 {
		workers = 4
	}
	life, cancel := context.WithCancel(ctx)
	defer cancel()
	jobs := make(chan DocumentPath)
	owner := taskgroup.Open()
	var mu sync.Mutex
	var nodes []*ast.Node
	fail := func(e error) {
		if e != nil {
			mu.Lock()
			err = errors.Join(err, e)
			mu.Unlock()
			cancel()
		}
	}
	for i := 0; i < workers; i++ {
		owner.Go(func(_ context.Context) {
			defer func() {
				if recover() != nil {
					fail(errors.New("notebook_index_worker_failed"))
				}
			}()
			for p := range jobs {
				if life.Err() != nil {
					return
				}
				av, e := s.Process(life, p)
				if e != nil {
					fail(e)
					return
				}
				mu.Lock()
				nodes = append(nodes, av...)
				mu.Unlock()
			}
		})
	}
send:
	for _, p := range docs {
		select {
		case <-life.Done():
			break send
		case jobs <- p:
		}
	}
	close(jobs)
	_ = owner.Close(context.WithoutCancel(ctx)) // Actual join, even after request cancellation.
	err = errors.Join(err, ctx.Err(), guardedDrain(ctx, s.Flush))
	if err != nil {
		return
	}
	if err = s.Bind(ctx, nodes); err != nil {
		return
	}
	if err = ctx.Err(); err != nil {
		return
	}
	return s.History(ctx)
}
