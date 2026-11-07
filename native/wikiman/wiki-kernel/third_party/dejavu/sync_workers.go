// Copyright (c) 2022-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package dejavu

import (
	"context"
	"github.com/siyuan-note/eventbus"
	"path"
	"sync"
	"sync/atomic"
)

// Every accepted worker is joined on success, failure and panic. Failure stops
// dispatch; already-running original provider calls finish under their timeout.
// No mutable provider/codec is released or replaced while workers still use it.
func (repo *Repo) joinedSyncBatch(ids []string, work func(string, int) error) error {
	return joinedSyncBatch(repo.cloud.GetConf().Context(), repo.cloud.GetConcurrentReqs(), ids, work)
}
func joinedSyncBatch(parent context.Context, n int, ids []string, work func(string, int) error) error {
	if len(ids) == 0 {
		return parent.Err()
	}
	if n < 1 {
		n = 1
	}
	if n > 64 {
		n = 64
	}
	if n > len(ids) {
		n = len(ids)
	}
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	var wg sync.WaitGroup
	var once sync.Once
	var first error
	fail := func(e error) { once.Do(func() { first = e; cancel() }) }
	jobs := make(chan int)
	for j := 0; j < n; j++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if recover() != nil {
					fail(ErrSnapshot)
				}
			}()
			for {
				select {
				case <-ctx.Done():
					return
				case i, ok := <-jobs:
					if !ok {
						return
					}
					if ctx.Err() != nil {
						return
					}
					if e := work(ids[i], i+1); e != nil {
						fail(e)
						return
					}
				}
			}
		}()
	}
dispatch:
	for i := range ids {
		select {
		case <-ctx.Done():
			break dispatch
		case jobs <- i:
		}
	}
	close(jobs)
	wg.Wait()
	if first != nil {
		return first
	}
	return parent.Err()
}
func (repo *Repo) uploadSyncObjects(ids []string, chunks bool, ui map[string]interface{}) (int64, error) {
	topic, item := eventbus.EvtCloudBeforeUploadFiles, eventbus.EvtCloudBeforeUploadFile
	if chunks {
		topic, item = eventbus.EvtCloudBeforeUploadChunks, eventbus.EvtCloudBeforeUploadChunk
	}
	for _, id := range ids {
		if !validScopedObjectID(id) {
			return 0, ErrSnapshot
		}
	}
	repo.emit(topic, ui, len(ids))
	var bytes atomic.Int64
	e := repo.joinedSyncBatch(ids, func(id string, count int) error {
		repo.emit(item, ui, count, len(ids))
		n, e := repo.cloud.UploadObject(path.Join("objects", id[:2], id[2:]), false)
		bytes.Add(n)
		return e
	})
	return bytes.Load(), e
}
