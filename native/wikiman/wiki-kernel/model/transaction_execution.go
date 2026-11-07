package model

import (
	"context"
	"sync"
)

type transactionExecution struct {
	done chan struct{}
	once sync.Once
	err  error
}

func (e *transactionExecution) complete(err error) {
	e.once.Do(func() { e.err = err; close(e.done) })
}

func (e *transactionExecution) wait(ctx context.Context) error {
	select {
	case <-e.done:
		return e.err
	case <-ctx.Done():
		return ctx.Err()
	}
}
