// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"github.com/88250/lute/parse"
	"github.com/siyuan-note/logging"
	"time"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/indexrepair"
	"wiki-kernel/internal/taskgroup"
	"wiki-kernel/util"
)

func (r *indexRepairContext) referenceLookup(ctx context.Context, id string) (*indexdb.BlockTreeRecord, error) {
	if e := r.guard(ctx); e != nil {
		return nil, e
	}
	rows, e := r.registry.Query(ctx, indexdb.BlockTreeFilter{Kind: "id", Value: id})
	if e != nil {
		return nil, e
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return &rows[0], nil
}
func (r *indexRepairContext) referenceCount(ctx context.Context, id string) (*indexrepair.ReferenceCount, error) {
	q := indexdb.References{DB: r.main.DB}
	return indexrepair.ReadReferenceCount(ctx, id, indexrepair.CountServices{Lookup: r.referenceLookup, References: q.RefIDs, Children: q.ChildDefinitionIDs})
}
func (r *indexRepairContext) refreshReferences(ctx context.Context, id string) error {
	if e := r.guard(ctx); e != nil {
		return e
	}
	// Include accepted but not-yet-flushed references before discovering roots.
	if e := r.queue.Flush(ctx); e != nil {
		return e
	}

	q := indexdb.References{DB: r.main.DB}
	ids, e := indexrepair.RefreshReferences(ctx, id, indexrepair.ReferenceServices{
		References: func(ctx context.Context, id string) ([]indexdb.RefRecord, error) {
			return q.ByDefinition(ctx, id, true)
		},
		Lookup: r.referenceLookup,
		Load: func(ctx context.Context, id string) (*parse.Tree, error) {
			bt, e := r.referenceLookup(ctx, id)
			if e != nil {
				return nil, e
			}
			if bt == nil {
				return nil, indexrepair.ErrReferenceTree
			}
			t, e := r.loadRepairTree(ctx, bt.BoxID, bt.Path, util.NewLute())
			if e != nil {
				return nil, e
			}
			return t, r.guard(ctx)
		},
		Index: func(ctx context.Context, t *parse.Tree) error {
			if e := r.guard(ctx); e != nil {
				return e
			}
			return r.queue.UpdateRefs(ctx, t)
		},
		Flush: func(ctx context.Context) error {
			if e := r.guard(ctx); e != nil {
				return e
			}
			return r.queue.Flush(ctx)
		},
	})
	if e != nil {
		return e
	}
	return r.owner.scheduleReferenceCounts(ctx, r, ids, util.SQLFlushInterval)
}
func RefreshBacklinkContext(ctx context.Context, id string) error {
	owner := Conf
	service, e := owner.CloudRepoService()
	if e != nil {
		return e
	}
	return service.RefreshReferences(ctx, id)
}

type referenceCountTask struct {
	ctx    context.Context
	cancel context.CancelFunc
}

// Keep original per-block debounce, but bind timer, query and publication to
// this owner. Replaced timers are canceled and remain tracked until they return.
func (c *AppConf) scheduleReferenceCounts(ctx context.Context, r *indexRepairContext, ids []string, delay time.Duration) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	if len(ids) == 0 {
		return nil
	}
	if _, e := c.CloudRepoService(); e != nil {
		return e
	}
	c.repoMu.Lock()
	defer c.repoMu.Unlock()
	if c.repoClosed {
		return errors.New("reference_refresh_owner_closed")
	}
	if c.repoTasks == nil {
		c.repoTasks = taskgroup.Open()
	}
	if c.repoReferenceCounts == nil {
		c.repoReferenceCounts = map[string]*referenceCountTask{}
	}
	for _, id := range ids {
		id := id
		if old := c.repoReferenceCounts[id]; old != nil {
			old.cancel()
		}
		job, cancel := context.WithCancel(context.Background())
		token := &referenceCountTask{ctx: job, cancel: cancel}
		c.repoReferenceCounts[id] = token
		if !c.repoTasks.Go(func(life context.Context) {
			stop := context.AfterFunc(life, cancel)
			defer func() { stop(); cancel() }()
			defer func() {
				if recover() != nil {
					logging.LogErrorf("reference count task failed")
				}
				c.repoMu.Lock()
				if c.repoReferenceCounts[id] == token {
					delete(c.repoReferenceCounts, id)
				}
				c.repoMu.Unlock()
			}()
			timer := time.NewTimer(delay)
			defer timer.Stop()
			select {
			case <-job.Done():
				return
			case <-timer.C:
			}
			if r.guard(job) != nil {
				return
			}
			if e := r.queue.Flush(job); e != nil {
				if job.Err() == nil {
					logging.LogErrorf("reference count flush failed: %s", e)
				}
				return
			}
			count, e := r.referenceCount(job, id)
			if e != nil {
				if job.Err() == nil {
					logging.LogErrorf("reference count read failed: %s", e)
				}
				return
			}
			if count != nil {
				c.publishSyncUI(job, func() {
					util.PushSetDefRefCount(count.RootID, count.BlockID, count.DefinitionIDs, count.Count, count.RootCount)
				})
			}
		}) {
			cancel()
			delete(c.repoReferenceCounts, id)
			return errors.New("reference_refresh_owner_closed")
		}
	}
	return nil
}

// Called where the old delayed callback was enqueued, never when its timer
// fires. The captured binding prevents it following a later global selection.
func scheduleLegacyReferenceCount(id string) {
	ctx := context.Background()
	r, e := captureIndexRepair(ctx)
	if e == nil {
		e = r.owner.scheduleReferenceCounts(ctx, r, []string{id}, util.SQLFlushInterval)
	}
	if e != nil {
		logging.LogErrorf("schedule reference count failed: %s", e)
	}
}
