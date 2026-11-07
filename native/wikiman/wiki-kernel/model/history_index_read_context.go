// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/88250/lute"
	"wiki-kernel/internal/historyindex"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/indexrepair"
)

// Invocation-local ports retain the production scanner and descriptor ownership.
type historyIndexIO struct {
	openRoot  func(string) (*os.Root, error)
	closeRoot func(*os.Root) error
	plan      func(context.Context, indexrepair.Source, string, string, *time.Location, *lute.Lute) ([]indexdb.HistoryEntry, error)
}

func (op historyIndexIO) defaults() historyIndexIO {
	if op.openRoot == nil {
		op.openRoot = os.OpenRoot
	}
	if op.closeRoot == nil {
		op.closeRoot = func(r *os.Root) error { return r.Close() }
	}
	if op.plan == nil {
		op.plan = historyindex.Plan
	}
	return op
}

// scan publishes no queue entries. Even a missing directory must complete checked
// close and final owner/path validation before it can be accepted as a no-op.
func (h *historyIndexContext) scan(ctx context.Context, name string, engine *lute.Lute, op historyIndexIO) (entries []indexdb.HistoryEntry, present bool, err error) {
	if err = h.guard(ctx); err != nil {
		return nil, false, err
	}
	if _, _, err = historyindex.Directory(name, time.Local); err != nil {
		return nil, false, err
	}
	op = op.defaults()
	base, err := filepath.Abs(h.root)
	if err != nil {
		return nil, false, err
	}
	rootInfo, err := os.Lstat(base)
	if indexrepair.IsSourceNotExist(err) {
		return nil, false, h.guard(ctx)
	}
	if err != nil {
		return nil, false, err
	}
	if !rootInfo.IsDir() || rootInfo.Mode()&fs.ModeSymlink != 0 {
		return nil, false, indexrepair.ErrSourcePath
	}
	root, err := op.openRoot(base)
	if err != nil {
		return nil, false, err
	}
	closeIO := newRepairIOCloser([]*os.Root{root}, op.closeRoot)
	var selected fs.FileInfo
	absent := false
	check := func() error {
		if err := h.guard(ctx); err != nil {
			return err
		}
		now, err := os.Lstat(base)
		if err != nil {
			return err
		}
		if !now.IsDir() || now.Mode()&fs.ModeSymlink != 0 || !os.SameFile(rootInfo, now) {
			return indexrepair.ErrSourceChanged
		}
		if selected == nil && !absent {
			return nil
		}
		now, err = os.Lstat(filepath.Join(base, name))
		if absent {
			if indexrepair.IsSourceNotExist(err) {
				return nil
			}
			if err != nil {
				return err
			}
			return indexrepair.ErrSourceChanged
		}
		if err != nil {
			return err
		}
		if !now.IsDir() || now.Mode()&fs.ModeSymlink != 0 || !os.SameFile(selected, now) {
			return indexrepair.ErrSourceChanged
		}
		return nil
	}
	defer func() {
		err = joinRepairCloseError(err, closeIO())
		err = joinRepairCloseError(err, check())
		if err != nil {
			entries = nil
			present = false
		}
	}()
	// Recover the scanner before resource cleanup so its failure and any close
	// failure reach the caller together; neither can admit a queue candidate.
	defer recoverRepositoryWorkflow(&err, "history_index_scan")
	opened, err := root.Stat(".")
	if err != nil {
		return nil, false, err
	}
	if !os.SameFile(rootInfo, opened) {
		return nil, false, indexrepair.ErrSourceChanged
	}
	if err = check(); err != nil {
		return nil, false, err
	}
	source := indexrepair.Source{Data: root}
	if err = source.Check(name, false); indexrepair.IsSourceNotExist(err) {
		absent = true
		return nil, false, nil
	} else if err != nil {
		return nil, false, err
	}
	selected, err = root.Lstat(name)
	if err != nil {
		return nil, false, err
	}
	if !selected.IsDir() {
		return nil, false, historyindex.ErrDirectory
	}
	if err = check(); err != nil {
		return nil, false, err
	}
	entries, err = op.plan(ctx, source, name, h.untitled, time.Local, engine)
	if err != nil {
		return nil, false, err
	}
	if err = check(); err != nil {
		return nil, false, err
	}
	return entries, true, nil
}
