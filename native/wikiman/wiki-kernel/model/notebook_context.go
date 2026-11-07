// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"os"
	"path/filepath"
	"wiki-kernel/conf"
	"wiki-kernel/internal/indexrepair"
	"wiki-kernel/internal/notebooks"
	"wiki-kernel/sql"
	"wiki-kernel/treenode"
	"wiki-kernel/util"
)

// Compatibility source mutation is serialized and explicitly authorized by the
// private AppConf binding. Native readers never call this repair writer.
var notebookRepairGate = make(chan struct{}, 1)

func (c *AppConf) listNotebooksAt(ctx context.Context, root *os.Root, guard func(context.Context) error, after func(context.Context, string) error) (out []*Box, err error) {
	err = c.WithSettingSnapshot(ctx, func(data conf.AppData) error {
		if guard != nil {
			if e := guard(ctx); e != nil {
				return e
			}
		}
		select {
		case notebookRepairGate <- struct{}{}:
			defer func() { <-notebookRepairGate }()
		case <-ctx.Done():
			return ctx.Err()
		}
		order := 0
		if data.FileTree != nil {
			order = data.FileTree.Sort
		}
		plan, e := notebooks.Scan(ctx, root, ".siyuan", order)
		if e != nil {
			return e
		}
		if guard != nil {
			if e := guard(ctx); e != nil {
				return e
			}
		}
		c.m.Lock()
		writable := !c.persistenceReadOnly && c.confPath != ""
		c.m.Unlock()
		if len(plan.Repairs) > 0 && writable {
			if guard == nil {
				return notebooks.ErrPermission
			}
			if e = guard(ctx); e != nil {
				return e
			}
			repairCtx := context.WithValue(ctx, virtualCacheSnapshotKey{}, virtualCacheSnapshot{owner: c, data: data})
			if e = notebooks.Apply(repairCtx, root, plan.Repairs, after); e != nil {
				return e
			}
		}
		if guard != nil {
			if e := guard(ctx); e != nil {
				return e
			}
		}
		out = make([]*Box, 0, len(plan.Notebooks))
		for _, v := range plan.Notebooks {
			out = append(out, &Box{ID: v.ID, Name: v.Name, Icon: v.Icon, Sort: v.Sort, SortMode: v.SortMode, Closed: v.Closed})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return
}

type notebookListIO struct {
	openRoot  func(string) (*os.Root, error)
	closeRoot func(*os.Root) error
	list      func(*AppConf, context.Context, *os.Root, func(context.Context) error, func(context.Context, string) error) ([]*Box, error)
}

func (op notebookListIO) defaults() notebookListIO {
	if op.openRoot == nil {
		op.openRoot = os.OpenRoot
	}
	if op.closeRoot == nil {
		op.closeRoot = func(root *os.Root) error { return root.Close() }
	}
	if op.list == nil {
		op.list = func(c *AppConf, ctx context.Context, root *os.Root, guard func(context.Context) error, after func(context.Context, string) error) ([]*Box, error) {
			return c.listNotebooksAt(ctx, root, guard, after)
		}
	}
	return op
}

func ListNotebooksContext(ctx context.Context) ([]*Box, error) {
	return listNotebooksContextWithIO(ctx, notebookListIO{})
}
func listNotebooksContextWithIO(ctx context.Context, op notebookListIO) (out []*Box, err error) {
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	owner := Conf
	if owner == nil {
		return nil, conf.ErrBusinessUnbound
	}
	source, workspace := util.DataDir, util.WorkspaceDir
	r := &indexRepairContext{owner: owner, data: source, workspace: workspace, main: sql.CaptureLegacyRepairQueries(), queue: sql.CaptureLegacyMainQueue(), registry: treenode.CaptureLegacyRepairRegistry()}
	guardOwner := func(ctx context.Context) error {
		if e := ctx.Err(); e != nil {
			return e
		}
		if Conf != owner || util.DataDir != source || util.WorkspaceDir != workspace || r.main.DB != r.queue.DB || r.queue.DB != sql.CaptureLegacyMainQueue().DB || r.main.DB != sql.CaptureLegacyRepairQueries().DB || r.registry.DB != treenode.CaptureLegacyRepairRegistry().DB {
			return conf.ErrBusinessUnbound
		}
		owner.repoMu.Lock()
		closed := owner.repoClosed
		owner.repoMu.Unlock()
		if closed {
			return conf.ErrBusinessUnbound
		}
		return nil
	}
	if err = guardOwner(ctx); err != nil {
		return nil, err
	}
	base, err := filepath.Abs(source)
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(base)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, indexrepair.ErrSourcePath
	}
	guard := func(ctx context.Context) error {
		if e := guardOwner(ctx); e != nil {
			return e
		}
		now, e := os.Lstat(base)
		if e != nil {
			return e
		}
		if !now.IsDir() || now.Mode()&os.ModeSymlink != 0 || !os.SameFile(info, now) {
			return indexrepair.ErrSourceChanged
		}
		return nil
	}
	op = op.defaults()
	root, err := op.openRoot(base)
	if err != nil {
		return nil, err
	}
	closeIO := newRepairIOCloser([]*os.Root{root}, op.closeRoot)
	defer func() {
		err = joinRepairCloseError(err, closeIO())
		err = joinRepairCloseError(err, guard(ctx))
		if err != nil {
			out = nil
		}
	}()
	defer recoverRepositoryWorkflow(&err, "notebook_list")
	opened, err := root.Stat(".")
	if err != nil {
		return nil, err
	}
	if !os.SameFile(info, opened) {
		return nil, indexrepair.ErrSourceChanged
	}
	if err = guard(ctx); err != nil {
		return nil, err
	}
	return op.list(owner, ctx, root, guard, func(ctx context.Context, id string) error {
		if e := guard(ctx); e != nil {
			return e
		}
		// The settings snapshot already owns settingsGate. Reuse the captured repair
		// dependencies rather than capturing settings again inside the repair callback.
		if e := r.unindexNotebook(ctx, id); e != nil {
			return e
		}
		if e := guard(ctx); e != nil {
			return e
		}
		if owner.Editor != nil && owner.Search != nil {
			if err := ResetVirtualBlockRefCacheContext(ctx); err != nil {
				return err
			}
		}
		return guard(ctx)
	})
}
