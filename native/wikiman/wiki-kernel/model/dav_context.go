// SiYuan - Refactor your thinking
// Copyright (c) 2020-present, b3log.org
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package model

import (
	"context"
	"errors"

	"golang.org/x/sync/errgroup"
)

const davWorkerLimit = 8

var errDAVContextUnbound = errors.New("dav_context_unbound")

func runDAVTasks(ctx context.Context, tasks []func(context.Context) error) error {
	if ctx == nil {
		return errDAVContextUnbound
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(tasks) == 0 {
		return nil
	}

	jobs := make(chan func(context.Context) error, len(tasks))
	for _, task := range tasks {
		if task == nil {
			return errors.New("dav_task_unbound")
		}
		jobs <- task
	}
	close(jobs)

	group, life := errgroup.WithContext(ctx)
	workers := min(davWorkerLimit, len(tasks))
	for range workers {
		group.Go(func() error {
			for {
				select {
				case <-life.Done():
					return life.Err()
				case task, ok := <-jobs:
					if !ok {
						return nil
					}
					if err := life.Err(); err != nil {
						return err
					}
					if err := task(life); err != nil {
						return err
					}
				}
			}
		})
	}
	return group.Wait()
}
