// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	stdsql "database/sql"
	"errors"
	"os"
	"path/filepath"

	"github.com/siyuan-note/eventbus"
	"github.com/siyuan-note/logging"
	"wiki-kernel/conf"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/taskgroup"
	"wiki-kernel/sql"
	"wiki-kernel/task"
	"wiki-kernel/util"
)

type legacyReindexOwner struct {
	owner                             *AppConf
	workspace, data, history, tempDir string
}
type legacyAdmissionKey struct{}

type legacyVirtualCacheEnvironment struct {
	owner                         *legacyReindexOwner
	db                            *stdsql.DB
	name, alias, anchor, document bool
	ignore, refIgnore             []string
}

func captureLegacyReindexOwner(ctx context.Context) (*legacyReindexOwner, error) {
	if ctx == nil {
		return nil, conf.ErrBusinessUnbound
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	o, _ := ctx.Value(legacyAdmissionKey{}).(*legacyReindexOwner)
	if o == nil {
		o = &legacyReindexOwner{owner: Conf, workspace: util.WorkspaceDir, data: util.DataDir, history: util.HistoryDir, tempDir: util.TempDir}
	}
	if err := o.guard(ctx); err != nil {
		return nil, err
	}
	return o, nil
}

func (o *legacyReindexOwner) guard(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if o == nil || o.owner == nil || o.owner != Conf || o.workspace != util.WorkspaceDir || o.data != util.DataDir || o.history != util.HistoryDir || o.tempDir != util.TempDir {
		return conf.ErrBusinessUnbound
	}
	if admitted, _ := ctx.Value(legacyAdmissionKey{}).(*legacyReindexOwner); admitted != nil && (admitted.owner != o.owner || admitted.workspace != o.workspace || admitted.data != o.data || admitted.history != o.history || admitted.tempDir != o.tempDir) {
		return conf.ErrBusinessUnbound
	}
	o.owner.repoMu.Lock()
	closed := o.owner.repoClosed
	o.owner.repoMu.Unlock()
	if closed {
		return conf.ErrBusinessUnbound
	}
	return nil
}

func (o *legacyReindexOwner) enqueue(ctx context.Context, action string, run func(context.Context, context.Context) error) (*task.ContextTask, error) {
	if err := o.guard(ctx); err != nil {
		return nil, err
	}
	h, err := task.AppendUniqueContext(ctx, action, func(request context.Context) error {
		if err := o.guard(request); err != nil {
			return err
		}
		done := make(chan error, 1)
		o.owner.repoMu.Lock()
		if o.owner.repoClosed || o.owner != Conf {
			o.owner.repoMu.Unlock()
			return conf.ErrBusinessUnbound
		}
		if o.owner.repoTasks == nil {
			o.owner.repoTasks = taskgroup.Open()
		}
		accepted := o.owner.repoTasks.Go(func(life context.Context) {
			merged, cancel := context.WithCancel(life)
			stop := context.AfterFunc(request, cancel)
			defer stop()
			defer cancel()
			defer func() {
				if recover() != nil {
					done <- task.ErrHandler
				}
			}()
			done <- run(merged, life)
		})
		o.owner.repoMu.Unlock()
		if !accepted {
			return conf.ErrBusinessUnbound
		}
		return <-done
	})
	if errors.Is(err, task.ErrDuplicate) {
		return nil, nil
	}
	return h, err
}

func startHistoryReindex(ctx context.Context) (*task.ContextTask, error) {
	o, err := captureLegacyReindexOwner(ctx)
	if err != nil {
		return nil, err
	}
	return o.enqueue(ctx, task.HistoryDatabaseIndexFull, func(request, life context.Context) error {
		h, err := captureHistoryIndexContext(request)
		if err != nil {
			return err
		}
		dirs, err := os.ReadDir(o.history)
		if err != nil {
			return err
		}
		engine := util.NewLute()
		manifests := map[string][]indexdb.HistoryEntry{}
		for _, dir := range dirs {
			if err = request.Err(); err != nil {
				return err
			}
			if !dir.IsDir() {
				continue
			}
			entries, present, scanErr := h.scan(request, dir.Name(), engine, historyIndexIO{})
			if scanErr != nil {
				return scanErr
			}
			if present {
				manifests[dir.Name()] = entries
			}
		}
		if err = o.guard(request); err != nil {
			return err
		}
		critical := context.WithoutCancel(life)
		o.owner.notifyRepo(critical, o.owner.Language(192), 7*1000)
		if err = sql.InitHistoryDatabaseContext(critical, true); err != nil {
			return err
		}
		queue := sql.CaptureLegacyHistoryQueue()
		for name, entries := range manifests {
			if err = queue.ReplaceDirectory(critical, name, entries); err != nil {
				return err
			}
		}
		return queue.Flush(critical, func(p indexdb.HistoryProgress) {
			if o.guard(context.Background()) == nil {
				eventbus.Publish(eventbus.EvtSQLInsertHistory, queue.EventContext(map[string]interface{}{eventbus.CtxPushMsg: eventbus.CtxPushMsgToStatusBar, "current": p.Current, "total": p.Total}))
			}
		})
	})
}

func startAssetContentReindex(ctx context.Context) (*task.ContextTask, error) {
	o, err := captureLegacyReindexOwner(ctx)
	if err != nil {
		return nil, err
	}
	extraction, err := captureAssetExtraction(o.tempDir)
	if err != nil {
		return nil, err
	}
	root := filepath.Join(o.data, "assets")
	return o.enqueue(ctx, task.AssetContentDatabaseIndexFull, func(request, life context.Context) error {
		items, err := assetContentSearcher.scanWithExtraction(request, root, o.guard, extraction)
		if err != nil {
			return err
		}
		if err = o.guard(request); err != nil {
			return err
		}
		critical := context.WithoutCancel(life)
		o.owner.notifyRepo(critical, o.owner.Language(216), 7*1000)
		if err = sql.InitAssetContentDatabaseContext(critical, true); err != nil {
			return err
		}
		queue := sql.CaptureLegacyAssetQueue()
		if err = queue.Index(critical, items); err != nil {
			return err
		}
		return queue.Flush(critical, func(p indexdb.AssetProgress) {
			if o.guard(context.Background()) == nil {
				eventbus.Publish(eventbus.EvtSQLInsertAssetContent, queue.EventContext(map[string]interface{}{eventbus.CtxPushMsg: eventbus.CtxPushMsgToStatusBar, "current": p.Current, "total": p.Total}))
			}
		})
	})
}

func captureLegacyVirtualCache(ctx context.Context) (*legacyVirtualCacheEnvironment, error) {
	if snapshot, ok := ctx.Value(virtualCacheSnapshotKey{}).(virtualCacheSnapshot); ok {
		if snapshot.owner != Conf {
			return nil, conf.ErrBusinessUnbound
		}
		return virtualCacheFromSnapshot(ctx, snapshot.data)
	}
	var data conf.AppData
	owner := Conf
	if owner == nil {
		return nil, conf.ErrBusinessUnbound
	}
	if err := owner.WithSettingSnapshot(ctx, func(v conf.AppData) error { data = v; return nil }); err != nil {
		return nil, err
	}
	if owner != Conf {
		return nil, conf.ErrBusinessUnbound
	}
	return virtualCacheFromSnapshot(ctx, data)
}

type virtualCacheSnapshotKey struct{}
type virtualCacheSnapshot struct {
	owner *AppConf
	data  conf.AppData
}

// Called while a trusted settings callback already owns settingsGate.
func virtualCacheFromSnapshot(ctx context.Context, v conf.AppData) (*legacyVirtualCacheEnvironment, error) {
	o, err := captureLegacyReindexOwner(ctx)
	if err != nil {
		return nil, err
	}
	e := &legacyVirtualCacheEnvironment{owner: o, db: sql.CaptureLegacyMainQueue().DB}
	if v.Search == nil {
		return nil, conf.ErrBusinessUnbound
	}
	e.name, e.alias, e.anchor, e.document = v.Search.VirtualRefName, v.Search.VirtualRefAlias, v.Search.VirtualRefAnchor, v.Search.VirtualRefDoc
	e.ignore = append([]string(nil), getSearchIgnoreLines()...)
	e.refIgnore = append([]string(nil), getRefSearchIgnoreLines()...)
	if err = e.guard(ctx); err != nil {
		return nil, err
	}
	return e, nil
}

func (e *legacyVirtualCacheEnvironment) guard(ctx context.Context) error {
	if e == nil || e.owner == nil {
		return conf.ErrBusinessUnbound
	}
	if err := e.owner.guard(ctx); err != nil {
		return err
	}
	if e.db == nil || e.db != sql.CaptureLegacyMainQueue().DB {
		return conf.ErrBusinessUnbound
	}
	return nil
}

func (e *legacyVirtualCacheEnvironment) rebuild(ctx context.Context) error {
	if err := e.guard(ctx); err != nil {
		return err
	}
	keywords, err := (indexdb.References{DB: e.db}).VirtualKeywords(ctx, e.name, e.alias, e.anchor, e.document, e.ignore, e.refIgnore)
	if err != nil {
		return err
	}
	if err = e.guard(ctx); err != nil {
		return err
	}
	virtualBlockRefCache.Clear()
	if !virtualBlockRefCache.Set("virtual_ref", keywords, 1) {
		return errors.New("virtual_cache_publication_rejected")
	}
	virtualBlockRefCache.Wait()
	return e.guard(ctx)
}

func startVirtualBlockRefCache(ctx context.Context) (*task.ContextTask, error) {
	e, err := captureLegacyVirtualCache(ctx)
	if err != nil {
		return nil, err
	}
	return e.owner.enqueue(ctx, task.CacheVirtualBlockRef, func(request, _ context.Context) error {
		return e.rebuild(request)
	})
}

func logOwnedTaskError(kind string, err error) {
	if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, task.ErrDuplicate) {
		logging.LogErrorf("%s failed: %s", kind, err)
	}
}
