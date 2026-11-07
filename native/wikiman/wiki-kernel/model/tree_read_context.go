// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"path"
	"strings"
	"time"
	"wiki-kernel/task"
)

func LoadTreeByBlockIDContext(ctx context.Context, id string) (*parse.Tree, error) {
	return runWorkspaceRead(ctx, func(o *workspaceReadOwner, c context.Context) (*parse.Tree, error) {
		return o.loadByBlockID(c, id, false)
	})
}
func LoadTreeByPathContext(ctx context.Context, box, path string) (*parse.Tree, error) {
	return runWorkspaceRead(ctx, func(o *workspaceReadOwner, c context.Context) (*parse.Tree, error) {
		return o.r.loadRepairTree(c, box, path, o.parser)
	})
}
func LoadTreeByBlockIDWithReindexContext(ctx context.Context, id string) (*parse.Tree, error) {
	return runWorkspaceRead(ctx, func(o *workspaceReadOwner, c context.Context) (*parse.Tree, error) {
		return o.loadByBlockID(c, id, true)
	})
}

func (o *workspaceReadOwner) loadByBlockID(ctx context.Context, id string, reindex bool) (*parse.Tree, error) {
	if !ast.IsNodeIDPattern(id) {
		return nil, ErrTreeNotFound
	}
	loc, e := o.r.referenceLookup(ctx, id)
	if e != nil {
		return nil, e
	}
	if loc != nil {
		return o.r.loadRepairTree(ctx, loc.BoxID, loc.Path, o.parser)
	}
	if task.ContainIndexTask() {
		return nil, ErrIndexing
	}
	if !reindex {
		return nil, ErrTreeNotFound
	}
	o.base.owner.m.Lock()
	now := time.Now()
	allowed := now.Sub(o.base.owner.treeSearchAt) >= 3*time.Second
	if allowed {
		o.base.owner.treeSearchAt = now
	}
	o.base.owner.m.Unlock()
	if !allowed {
		return nil, ErrTreeNotFound
	}
	boxes, e := o.base.owner.business.Notebooks(ctx)
	if e != nil {
		return nil, e
	}
	ioctx, closeIO, e := o.r.openRepairIOChecked(ctx, false)
	if e != nil {
		return nil, e
	}
	var found *parse.Tree
	var scanErr error
	for _, box := range boxes {
		if box.Closed {
			continue
		}
		manifest, e := o.r.repairManifest(ctx, ioctx.Source, box.ID, indexRepairWalkIO{})
		if e != nil {
			scanErr = e
			break
		}
		for _, entry := range manifest {
			if entry.info.IsDir() || path.Base(entry.name) != id+".docj" {
				continue
			}
			p := "/" + strings.TrimPrefix(entry.name, box.ID+"/")
			found, scanErr = ioctx.Load(ctx, box.ID, p, o.parser, false)
			break
		}
		if found != nil || scanErr != nil {
			break
		}
	}
	if e = errors.Join(scanErr, closeIO()); e != nil {
		return nil, e
	}
	if found == nil {
		return nil, ErrTreeNotFound
	}
	if e = o.r.guard(ctx); e != nil {
		return nil, e
	}
	if e = o.r.registry.Upsert(ctx, found); e != nil {
		return nil, e
	}
	if e = o.r.queue.Index(ctx, found); e != nil {
		return nil, e
	}
	// An admitted derived operation must be drained even if the request ends.
	if e = o.r.queue.Flush(context.WithoutCancel(ctx)); e != nil {
		return nil, e
	}
	return found, ctx.Err()
}
