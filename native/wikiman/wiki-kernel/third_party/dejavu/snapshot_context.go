// Copyright (c) 2022-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package dejavu

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"github.com/88250/gulu"
	"github.com/siyuan-note/dejavu/cloud"
	"github.com/siyuan-note/dejavu/cloudlock"
	"github.com/siyuan-note/dejavu/entity"
	"github.com/siyuan-note/eventbus"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sync"
	"sync/atomic"
)

var ErrSnapshot = errors.New("snapshot_invalid_or_incomplete")

type SnapshotRequest struct{ Kind, ID, Tag string }
type SnapshotResult struct {
	Files, Chunks, Indexes, Objects int
	Bytes                           int64
	Applied, Uncertain              bool
}
type SnapshotOptions struct {
	Event func(context.Context, string, ...interface{})
	// Explicit trusted provider adapter; nil uses the original provider factory.
	Open func(cloud.Cloud, context.Context) (cloud.Cloud, func(), error)
}
type snapshotScope struct {
	indexing bool
	previous map[string]*entity.File
	ctx      context.Context
	event    func(context.Context, string, ...interface{})
	mutated  atomic.Bool
	open     func(cloud.Cloud, context.Context) (cloud.Cloud, func(), error)
}

func (repo *Repo) emit(topic string, args ...interface{}) {
	if s := repo.snapshot; s != nil {
		if s.ctx.Err() == nil && s.event != nil {
			s.event(s.ctx, topic, args...)
		}
	} else {
		eventbus.Publish(topic, args...)
	}
}
func (repo *Repo) traffic(t *cloud.Traffic) {
	if repo.snapshot != nil {
		repo.cloud.AddTraffic(t)
	} else {
		go repo.cloud.AddTraffic(t)
	}
}

// SnapshotContext operates only on repository objects/refs, never checks out
// documents into DataPath. Shared coordination admission is cancellation aware.
func (repo *Repo) SnapshotContext(ctx context.Context, r SnapshotRequest, o SnapshotOptions) (out SnapshotResult, err error) {
	if e := lock.LockContext(ctx); e != nil {
		return out, e
	}
	defer lock.Unlock()
	if r.Kind != "purge-local" && r.Kind != "purge-cloud" && !validTag(r.Tag, r.Kind == "download") {
		return out, ErrSnapshot
	}
	if (r.Kind == "tag" || r.Kind == "upload" || r.Kind == "download") && !validScopedObjectID(r.ID) {
		return out, ErrSnapshot
	}
	// No symlink in a private repository may redirect cache writes/deletes.
	if e := filepath.WalkDir(repo.Path, func(_ string, d fs.DirEntry, e error) error {
		if os.IsNotExist(e) {
			return nil
		}
		if e != nil {
			return e
		}
		if d.Type()&os.ModeSymlink != 0 {
			return ErrSnapshot
		}
		return ctx.Err()
	}); e != nil {
		return out, e
	}
	original := repo.cloud
	if o.Open == nil {
		o.Open = cloud.OpenContext
	}
	s := &snapshotScope{ctx: ctx, event: o.Event, open: o.Open}
	repo.snapshot = s
	repo.store.operation = ctx
	repo.store.modified = func() { s.mutated.Store(true) }
	defer func() {
		if recover() != nil {
			err = ErrSnapshot
		}
		if err != nil && s.mutated.Load() {
			out.Uncertain = true
		}
		repo.snapshot = nil
		repo.store.operation = nil
		repo.store.modified = nil
		repo.cloud = original
	}()
	run := func(c context.Context) error {
		s.ctx = c
		repo.store.operation = c
		client, done, e := o.Open(original, c)
		if e != nil {
			return e
		}
		defer done()
		repo.cloud = &snapshotCloud{Cloud: client, s: s}
		switch r.Kind {
		case "tag":
			if e = repo.store.check(); e != nil {
				return e
			}
			s.mutated.Store(true)
			e = repo.AddTag(r.ID, r.Tag)
		case "untag":
			s.mutated.Store(true)
			e = repo.RemoveTag(r.Tag)
		case "cloud-untag":
			e = repo.RemoveCloudRepoTag(r.Tag)
		case "upload":
			id, getErr := repo.GetTag(r.Tag)
			if getErr != nil {
				return getErr
			}
			if id != r.ID {
				return ErrSnapshot
			}
			out.Files, out.Chunks, out.Bytes, e = repo.uploadTagIndex(r.Tag, r.ID, nil)
		case "download":
			out.Files, out.Chunks, out.Bytes, e = repo.downloadIndex(r.ID, nil)
			if e == nil && r.Tag != "" {
				e = repo.AddTag(r.ID, r.Tag)
			}
		case "purge-local":
			stat, e2 := repo.store.Purge()
			e = e2
			if stat != nil {
				out.Indexes, out.Objects, out.Bytes = stat.Indexes, stat.Objects, stat.Size
			}
		case "purge-cloud":
			stat, e2 := repo.purgeCloud(nil)
			e = e2
			if stat != nil {
				out.Indexes, out.Objects, out.Bytes = stat.Indexes, stat.Objects, stat.Size
			}
		default:
			return ErrSnapshot
		}
		out.Applied = e == nil
		if e == nil {
			e = c.Err()
		}
		return e
	}
	if r.Kind == "tag" || r.Kind == "untag" || r.Kind == "purge-local" {
		err = run(ctx)
		return
	}
	opts := cloudlock.Options{}
	token := make([]byte, 16)
	if _, err = rand.Read(token); err != nil {
		return
	}
	opts.Owner = repo.DeviceID + "-" + hex.EncodeToString(token)
	opts.Read = func(c context.Context) ([]byte, error) {
		client, done, e := o.Open(original, c)
		if e != nil {
			return nil, e
		}
		defer done()
		b, e := client.DownloadObject(lockSyncKey)
		if errors.Is(e, cloud.ErrCloudObjectNotFound) {
			return nil, nil
		}
		return b, e
	}
	opts.Write = func(c context.Context, b []byte) error {
		client, done, e := o.Open(original, c)
		if e != nil {
			return e
		}
		defer done()
		_, e = client.UploadBytes(lockSyncKey, b, true)
		return e
	}
	opts.Remove = func(c context.Context) error {
		client, done, e := o.Open(original, c)
		if e != nil {
			return e
		}
		defer done()
		e = client.RemoveObject(lockSyncKey)
		if errors.Is(e, cloud.ErrCloudObjectNotFound) {
			return nil
		}
		return e
	}
	opts.Event = func(c context.Context, acquire bool) {
		if o.Event != nil && c.Err() == nil {
			topic := eventbus.EvtCloudUnlock
			if acquire {
				topic = eventbus.EvtCloudLock
			}
			o.Event(c, topic)
		}
	}
	err = cloudlock.WithLock(ctx, opts, run)
	return
}
func validTag(s string, empty bool) bool {
	if empty && s == "" {
		return true
	}
	if s == "" || s == "." || s == ".." || len(s) > 255 || !gulu.File.IsValidFilename(s) {
		return false
	}
	for _, c := range s {
		if c < 32 || c == 127 {
			return false
		}
	}
	return true
}

type snapshotCloud struct {
	cloud.Cloud
	s *snapshotScope
}

func (c *snapshotCloud) UploadObject(p string, o bool) (int64, error) {
	if e := c.GetConf().Context().Err(); e != nil {
		return 0, e
	}
	c.s.mutated.Store(true)
	return c.Cloud.UploadObject(p, o)
}
func (c *snapshotCloud) UploadBytes(p string, b []byte, o bool) (int64, error) {
	if e := c.GetConf().Context().Err(); e != nil {
		return 0, e
	}
	c.s.mutated.Store(true)
	return c.Cloud.UploadBytes(p, b, o)
}
func (c *snapshotCloud) RemoveObject(p string) error {
	if e := c.GetConf().Context().Err(); e != nil {
		return e
	}
	if !validSnapshotPath(p) {
		return ErrSnapshot
	}
	c.s.mutated.Store(true)
	return c.Cloud.RemoveObject(p)
}
func validSnapshotPath(p string) bool {
	if p == "indexes-v2.json" {
		return true
	}
	if path.Clean(p) != p || !fs.ValidPath(p) {
		return false
	}
	return true
}

// Bounded workers cancel on first failure and always join actual provider IO.
func (repo *Repo) snapshotBatch(ids []string, work func(string, int) error) error {
	if len(ids) == 0 {
		return nil
	}
	n := repo.cloud.GetConcurrentReqs()
	if n < 1 {
		n = 1
	}
	if n > 64 {
		n = 64
	}
	if n > len(ids) {
		n = len(ids)
	}
	c, cancel := context.WithCancel(repo.snapshot.ctx)
	defer cancel()
	jobs := make(chan int)
	var wg sync.WaitGroup
	var once sync.Once
	var first error
	original := repo.cloud
	source := original.(*snapshotCloud).Cloud
	client, done, e := repo.snapshot.open(source, c)
	if e != nil {
		return e
	}
	defer done()
	repo.cloud = &snapshotCloud{Cloud: client, s: repo.snapshot}
	previous := repo.store.operation
	repo.store.operation = c
	defer func() { repo.cloud = original; repo.store.operation = previous }()
	fail := func(e error) { once.Do(func() { first = e; cancel() }) }
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
				case <-c.Done():
					return
				case i, ok := <-jobs:
					if !ok {
						return
					}
					if c.Err() != nil {
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
loop:
	for i := range ids {
		select {
		case <-c.Done():
			break loop
		case jobs <- i:
		}
	}
	close(jobs)
	wg.Wait()
	if first != nil {
		return first
	}
	return repo.snapshot.ctx.Err()
}
