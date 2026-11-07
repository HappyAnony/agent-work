// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"os"
	"path"
	"time"

	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"github.com/siyuan-note/dataparser"
	"github.com/siyuan-note/dejavu"
	"wiki-kernel/conf"
	"wiki-kernel/internal/indexrepair"
	"wiki-kernel/sql"
	"wiki-kernel/util"
)

type conflictCopy struct {
	tree     *parse.Tree
	previous string
}

// The compatibility caller already holds the repository's serial source gate.
// All temporary reads/JSON parsing finish before the first AV/source mutation.
func applySyncConflictCopies(ctx context.Context, result *dejavu.MergeResult) (reload bool, err error) {
	defer recoverRepositoryWorkflow(&err, "sync_conflict_copy")
	if result == nil {
		return false, indexrepair.ErrSourcePath
	}
	enabled := false
	untitled := ""
	owner := Conf
	if owner == nil {
		return false, conf.ErrBusinessUnbound
	}
	if e := owner.WithSettingSnapshot(ctx, func(v conf.AppData) error {
		if v.Sync == nil {
			return conf.ErrBusinessUnbound
		}
		enabled = v.Sync.GenerateConflictDoc
		untitled = owner.business.Language(v.Lang, 16, false, v.CloudRegion)
		return nil
	}); e != nil {
		return false, e
	}
	if !enabled {
		return false, nil
	}
	r, e := captureIndexRepair(ctx)
	if e != nil {
		return false, e
	}
	temporary := util.TempDir
	guard := func(ctx context.Context) error {
		if e := r.guard(ctx); e != nil {
			return e
		}
		if temporary != util.TempDir {
			return conf.ErrBusinessUnbound
		}
		return nil
	}
	if e = guard(ctx); e != nil {
		return false, e
	}
	paths := make([]string, len(result.Conflicts))
	for i, f := range result.Conflicts {
		if f == nil {
			return false, indexrepair.ErrSourcePath
		}
		paths[i] = f.Path
	}
	docs, e := indexrepair.DocumentPaths(paths)
	if e != nil {
		return false, e
	}
	if len(docs) == 0 {
		return true, ctx.Err()
	}
	root, e := os.OpenRoot(temporary)
	if e != nil {
		return false, e
	}
	closeSource := newRepairIOCloser([]*os.Root{root}, r.repairRootCloser())
	defer func() {
		err = joinRepairCloseError(err, closeSource())
		if err != nil {
			reload = false
		}
	}()
	source := indexrepair.Source{Data: root}
	engine := util.NewLute()
	stamp := result.Time.Format("2006-01-02-150405")
	var copies []conflictCopy
	for _, p := range docs {
		if e = guard(ctx); e != nil {
			return false, e
		}
		raw, e := source.Read(ctx, path.Join("repo/sync/conflicts", stamp, p.Relative))
		if e != nil {
			return false, e
		}
		tree, e := dataparser.ParseJSONWithoutFix(raw, engine.ParseOptions)
		if e != nil {
			return false, e
		}
		if tree == nil || tree.Root == nil || tree.Root.Type != ast.NodeDocument {
			return false, indexrepair.ErrReferenceTree
		}
		tree.Box, tree.Path = p.Box, p.Path
		tree.Root.Box, tree.Root.Path = p.Box, p.Path
		copies = append(copies, conflictCopy{tree: tree, previous: p.Path})
	}
	ioctx, closeIO, e := r.openRepairIOChecked(ctx, true)
	if e != nil {
		return false, e
	}
	defer func() {
		err = joinRepairCloseError(err, closeIO())
		if err != nil {
			reload = false
		}
	}()
	defer func() {
		err = errors.Join(err, indexrepairDrain(ctx, sql.LegacyMainQueue{DB: r.main.DB}))
		if err != nil {
			reload = false
		}
	}()
	var pending []commitNotice
	var commit *legacyCommitContext
	for _, copy := range copies {
		if e = guard(ctx); e != nil {
			return false, e
		}
		if e = resetTreeWithServices(ctx, copy.tree, "Conflicted", true, resetTreeServices{NewID: ast.NewNodeID, Now: time.Now, Untitled: func() string { return untitled }, Bind: func(ctx context.Context, a, b string) error {
			if e := guard(ctx); e != nil {
				return e
			}
			return r.bindRepairAV(ctx, ioctx.Source, a, b)
		}}); e != nil {
			return false, e
		}
		if e = r.createConflictTree(ctx, copy.tree, guard, func(c *legacyCommitContext, n []commitNotice) error {
			commit = c
			pending = append(pending, n...)
			return nil
		}); e != nil {
			return false, e
		}
		if e = (sql.LegacyMainQueue{DB: r.main.DB}).Flush(context.WithoutCancel(ctx)); e != nil {
			return false, e
		}
		boxes, e := owner.notebooks(ctx)
		if e != nil {
			return false, e
		}
		for _, box := range boxes {
			if box.ID == copy.tree.Box && !box.Closed {
				if e = guard(ctx); e != nil {
					return false, e
				}
				if e = r.addConflictSort(ctx, box.ID, copy.previous, copy.tree.ID); e != nil {
					return false, e
				}
				break
			}
		}
	}

	if e = guard(ctx); e != nil {
		return false, e
	}
	if e = indexrepairDrain(ctx, sql.LegacyMainQueue{DB: r.main.DB}); e != nil {
		return false, e
	}
	if e = closeIO(); e != nil {
		return false, e
	}
	if e = closeSource(); e != nil {
		return false, e
	}
	if commit != nil {
		if e = commit.complete(ctx, pending); e != nil {
			return false, e
		}
	}
	return true, nil
}
