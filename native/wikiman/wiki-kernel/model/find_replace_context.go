// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"fmt"
	"github.com/88250/lute"
	"github.com/88250/lute/parse"
	"os"
	"path/filepath"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/internal/blockview"
	"wiki-kernel/internal/doclife"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/sql"
	"wiki-kernel/task"
	"wiki-kernel/treenode"
	"wiki-kernel/util"
)

func FindReplaceContext(ctx context.Context, keyword, replacement string, replaceTypes map[string]bool, ids, paths, boxes []string, types map[string]bool, method, order, group int) error {
	return findReplaceContext(ctx, findReplaceRequest{Keyword: keyword, Replacement: replacement, ReplaceTypes: replaceTypes, IDs: ids, Paths: paths, Boxes: boxes, Types: types, Method: method, OrderBy: order, GroupBy: group})
}
func findReplaceContext(ctx context.Context, q findReplaceRequest) error {
	q = copyFindReplaceRequest(q)
	o, e := captureWorkspaceRead(ctx)
	if e != nil {
		return e
	}
	r, e := captureIndexRepair(ctx)
	if e != nil {
		return e
	}
	ui, e := captureLegacyDelayedUIOwner(ctx)
	if e != nil {
		return e
	}
	if r.owner != o.base.owner || r.data != o.base.data || ui.owner != r.owner || ui.data != r.data {
		return conf.ErrBusinessUnbound
	}
	o.base.owner.m.Lock()
	if o.base.owner.findReplaceGate == nil {
		o.base.owner.findReplaceGate = newAttributeMutationGate()
	}
	gate := o.base.owner.findReplaceGate
	o.base.owner.m.Unlock()
	return o.base.run(ctx, func(op, life context.Context) error {
		if e := r.guard(op); e != nil {
			return e
		}
		if o.settings.Search == nil || o.settings.FileTree == nil {
			return conf.ErrBusinessUnbound
		}
		if e := FlushTxQueueContext(op); e != nil {
			return e
		}
		txenv := legacyTransactionEnvironmentContext(op)
		txenv.ctx = op
		txenv.parser = func() *lute.Lute { return o.parser }
		if e := bindLegacyCommit(op, txenv, r); e != nil {
			return e
		}
		commit := txenv.commitContext
		anchorOwner := &legacyAnchorContext{r: r, commit: commit}
		avBinding := bindAVOperation(commit, op)
		events := sourceEventEnvironmentWithOwners(op, life, anchorOwner, ui, nil)
		events.legacy = false
		history := &historyIndexContext{owner: r.owner, root: o.base.history, workspace: r.workspace, queue: sql.CaptureLegacyHistoryQueue(), untitled: r.language(16)}
		now := time.Now()
		historyName := now.Format("2006-01-02-150405") + "-" + HistoryOpReplace
		historyPath := filepath.Join(history.root, historyName)
		language := func(code int, html bool) string {
			return r.owner.business.Language(o.settings.Lang, code, html, o.settings.CloudRegion)
		}
		lifecycle := &legacyDocumentLifecycleContext{r: r, owner: r.owner, data: r.data, history: o.base.history, markdown: o.parserOptions.Markdown, allowDeeper: o.settings.FileTree.AllowCreateDeeper, language: func(code int) string { return language(code, true) }, plain: func(code int) string { return language(code, false) }}
		env := &findReplaceEnvironment{ctx: op, caseSensitive: o.settings.Search.CaseSensitive, gate: gate,
			parser: func() (*lute.Lute, error) { return o.parser, r.guard(op) }, reject: func(code int) error {
				if e := r.guard(op); e != nil {
					return e
				}
				return errors.New(language(code, false))
			},
			search: func(q findReplaceRequest) ([]string, error) {
				ignore, e := o.ignore(op, false)
				if e != nil {
					return nil, e
				}
				result, e := indexdb.SearchAllBlocks(op, r.main.DB, *o.settings.Search, indexdb.BlockSearchRequest{Query: q.Keyword, Boxes: q.Boxes, Paths: q.Paths, Types: q.Types, Method: q.Method, OrderBy: q.OrderBy, Ignore: ignore})
				if e != nil {
					return nil, e
				}
				values, e := blockview.FullText(op, result, q.GroupBy, q.OrderBy, o.settings.Search.CaseSensitive, o.searchView())
				if e != nil {
					return nil, e
				}
				ids := []string{}
				for _, b := range values {
					ids = append(ids, b.ID)
				}
				return ids, op.Err()
			},
			lookup: func(id string) (*treenode.BlockTree, error) {
				b, e := r.referenceLookup(op, id)
				if e != nil || b == nil {
					return nil, e
				}
				return &treenode.BlockTree{ID: b.ID, RootID: b.RootID, ParentID: b.ParentID, BoxID: b.BoxID, Path: b.Path, HPath: b.HPath, Updated: b.Updated, Type: b.Type}, nil
			},
			load: func(id string) (*parse.Tree, error) {
				loc, e := r.referenceLookup(op, id)
				if e != nil {
					return nil, e
				}
				if loc == nil {
					return nil, ErrBlockNotFound
				}
				return r.loadRepairTree(op, loc.BoxID, loc.Path, o.parser)
			},
			beginHistory: func() (string, error) {
				if e := history.guard(op); e != nil {
					return "", e
				}
				if e := os.MkdirAll(historyPath, 0755); e != nil {
					return "", e
				}
				return historyPath, nil
			},
			history: func(dir string, t *parse.Tree) error {
				if dir != historyPath {
					return errors.New("replace_history_identity_invalid")
				}
				return writeTransactionHistory(op, commit, history, t, now, func(string) error { return nil }, HistoryOpReplace)
			},
			indexHistory: func(dir string) error {
				if dir != historyPath {
					return errors.New("replace_history_identity_invalid")
				}
				return history.index(op, historyName, o.parser)
			},
			stage: func(t *parse.Tree) error { return anchorOwner.stage(op, t, nil) },
			rename: func(t *parse.Tree, title string) error {
				_, e := lifecycle.execute(op, doclife.Request{Action: "rename", Box: t.Box, Path: t.Path, Title: title})
				return e
			},
			progress: func(kind string, i, total int) error {
				if e := r.guard(op); e != nil {
					return e
				}
				code := 206
				if kind == "rename" {
					code = 207
				}
				if !ui.publish(life, func() { util.PushEndlessProgress(fmt.Sprintf(ui.language(code, false), i, total)) }) {
					return conf.ErrBusinessUnbound
				}
				return nil
			},
			clear: func(success bool) error {
				if !ui.publish(life, func() {
					if success {
						util.PushClearProgress()
					} else {
						util.ClearPushProgress(100)
					}
				}) {
					return conf.ErrBusinessUnbound
				}
				return nil
			},
			flush:  func() error { return r.queue.Flush(context.WithoutCancel(op)) },
			tags:   func() error { return ui.schedule(life, task.ReloadTag, 200*time.Millisecond, util.PushReloadTag) },
			reload: func(id string) error { return reloadProtyleWithEnvironment(id, events) },
			refCount: func(id string) error {
				return r.owner.scheduleReferenceCounts(op, r, []string{id}, 700*time.Millisecond)
			}, av: legacyAVTextEnvironment(op, avBinding),
		}
		return findReplaceWithEnvironment(q, env)
	})
}
