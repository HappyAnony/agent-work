// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"fmt"
	"github.com/siyuan-note/dejavu"
	"github.com/siyuan-note/eventbus"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/internal/cloudrepo"
	"wiki-kernel/internal/taskgroup"
	"wiki-kernel/task"
	"wiki-kernel/util"
)

type restorePorts struct {
	Suspend func(context.Context) (func(context.Context) error, error)
	Reindex func(context.Context) error
}

type restoreWatcherPorts struct {
	closeAssets func(context.Context) error
	closeEmojis func(context.Context) error
	openAssets  func(context.Context) error
	openEmojis  func(context.Context) error
}

func suspendRestoreWatchers(ctx context.Context, p restoreWatcherPorts) (func(context.Context) error, error) {
	if p.closeAssets == nil || p.closeEmojis == nil || p.openAssets == nil || p.openEmojis == nil {
		return nil, cloudrepo.ErrDependency
	}
	if err := p.closeAssets(ctx); err != nil {
		return nil, err
	}
	if err := p.closeEmojis(ctx); err != nil {
		return nil, errors.Join(err, p.openAssets(context.WithoutCancel(ctx)))
	}
	return func(resumeCtx context.Context) error {
		if err := p.openAssets(resumeCtx); err != nil {
			return err
		}
		if err := p.openEmojis(resumeCtx); err != nil {
			return errors.Join(err, p.closeAssets(context.WithoutCancel(resumeCtx)))
		}
		return resumeCtx.Err()
	}, nil
}

func (c *AppConf) restoreScope(p metadataRepoPaths) bool {
	return c == Conf && p.Workspace == util.WorkspaceDir && p.Data == util.DataDir && p.Repo == util.RepoDir && p.History == util.HistoryDir && p.Temp == util.TempDir
}

func (c *AppConf) legacyRestorePorts(p metadataRepoPaths) restorePorts {
	return restorePorts{
		Suspend: func(ctx context.Context) (func(context.Context) error, error) {
			if e := ctx.Err(); e != nil {
				return nil, e
			}
			if !c.restoreScope(p) {
				return nil, cloudrepo.ErrClosed
			}
			resume, err := suspendRestoreWatchers(ctx, restoreWatcherPorts{
				closeAssets: CloseWatchAssetsContext,
				closeEmojis: CloseWatchEmojisContext,
				openAssets:  WatchAssetsContext,
				openEmojis:  WatchEmojisContext,
			})
			if err != nil {
				return nil, err
			}
			return func(resumeCtx context.Context) error {
				c.repoMu.Lock()
				defer c.repoMu.Unlock()
				if !c.repoClosed && !c.repoCheckoutClosed && !c.repoRestoreFailed && c.restoreScope(p) && task.Accepting() && !util.IsExiting.Load() {
					return resume(resumeCtx)
				}
				return nil
			}, nil
		},
		Reindex: func(ctx context.Context) error {
			if !c.restoreScope(p) {
				return cloudrepo.ErrClosed
			}
			return reindexAfterCheckout(ctx)
		},
	}
}

func (c *AppConf) executeLegacyRestore(ctx context.Context, v conf.AppData, id string, p metadataRepoPaths, ports restorePorts) (out cloudrepo.RestoreResult, err error) {
	if !c.restoreScope(p) || ports.Suspend == nil || ports.Reindex == nil {
		return out, cloudrepo.ErrDependency
	}
	c.m.Lock()
	readonly := c.persistenceReadOnly
	c.m.Unlock()
	if readonly {
		return out, ErrSettingReadOnly
	}
	if v.Repo == nil || v.Sync == nil || v.System == nil {
		return out, cloudrepo.ErrDependency
	}
	if e := validateSnapshotPaths(p); e != nil {
		return out, e
	}
	source, e := canonicalDirectoryPath(p.Data)
	if e != nil {
		return out, cloudrepo.ErrDependency
	}
	config, e := canonicalDirectoryPath(c.confPath)
	if e != nil || directoryContains(source, config) {
		return out, cloudrepo.ErrDependency
	}
	if _, e := prepareRepoCheckout(ctx, v, id, p); e != nil {
		return out, e
	}
	if e := FlushTxQueueContext(ctx); e != nil {
		return out, e
	}
	resume, e := ports.Suspend(ctx)
	if e != nil {
		return out, e
	}
	if resume == nil {
		return out, cloudrepo.ErrDependency
	}
	defer func() {
		e := finishRestoreWatchers(context.WithoutCancel(ctx), resume)
		if e != nil {
			out.Uncertain = true
			c.repoMu.Lock()
			c.repoRestoreFailed = true
			c.repoMu.Unlock()
		}
		err = errors.Join(err, e)
	}()
	defer func() {
		if recover() != nil {
			out.Uncertain = true
			c.repoMu.Lock()
			c.repoRestoreFailed = true
			c.repoMu.Unlock()
			err = task.ErrHandler
		}
	}()
	lang := func(id int) string { return c.business.Language(v.Lang, id, true, v.CloudRegion) }
	ui := func(cctx context.Context, fn func()) {
		c.repoMu.Lock()
		defer c.repoMu.Unlock()
		if !c.repoClosed && c.restoreScope(p) && cctx.Err() == nil && !util.IsExiting.Load() {
			fn()
		}
	}
	ui(ctx, func() { util.PushEndlessProgress(lang(63)) })
	defer ui(context.WithoutCancel(ctx), util.PushClearProgress)
	err = c.WithSettingSnapshot(ctx, func(current conf.AppData) error {
		if !reflect.DeepEqual(current.Repo, v.Repo) || !reflect.DeepEqual(current.Sync, v.Sync) {
			return errRepoConfigurationChanged
		}
		c.m.Lock()
		defer c.m.Unlock()
		current.Sync.Enabled = false
		if e := c.saveRepoKeyDataLocked(current); e != nil {
			return e
		}
		c.Sync = current.Sync
		out.SyncPaused = true
		return nil
	})
	if err != nil {
		return out, err
	}
	ignore, e := os.ReadFile(filepath.Join(p.Data, ".siyuan", "syncignore"))
	if e != nil && !os.IsNotExist(e) {
		return out, cloudrepo.ErrProvider
	}
	p.Ignore = append(strings.Split(strings.ReplaceAll(string(ignore), "\r\n", "\n"), "\n"), "20210808180117-6v0mkxr/**/*", "20210808180117-czj9bvb/**/*", "20211226090932-5lcq56f/**/*", "20240530133126-axarxgx/**/*")
	var transports []*http.Transport
	p.Transports = &transports
	defer func() {
		for _, t := range transports {
			t.CloseIdleConnections()
		}
	}()
	// A non-cancelled provider identity is needed by original getFiles in the
	// critical section. Preflight and backup still use their explicit Contexts.
	repo, e := newMetadataRepository(context.WithoutCancel(ctx), v, p)
	if e != nil {
		return out, cloudrepo.ErrProvider
	}
	defer repo.CloseMetadata()
	event := func(cctx context.Context, topic string, args ...interface{}) {
		ids := map[string]int{eventbus.EvtIndexBeforeWalkData: 158, eventbus.EvtIndexBeforeGetLatestFiles: 159, eventbus.EvtIndexUpsertFiles: 160, eventbus.EvtCheckoutBeforeWalkData: 161, eventbus.EvtCheckoutUpsertFiles: 162, eventbus.EvtCheckoutUpsertFile: 162, eventbus.EvtCheckoutRemoveFiles: 163, eventbus.EvtCheckoutRemoveFile: 163}
		id := ids[topic]
		if id == 0 {
			return
		}
		vals := []interface{}{}
		if len(args) > 0 {
			vals = append(vals, args[1:]...)
		}
		if topic == eventbus.EvtIndexBeforeGetLatestFiles || topic == eventbus.EvtIndexUpsertFiles || topic == eventbus.EvtCheckoutUpsertFiles || topic == eventbus.EvtCheckoutRemoveFiles {
			vals = append([]interface{}{0}, vals...)
		}
		msg := fmt.Sprintf(lang(id), vals...)
		ui(ctx, func() { util.PushStatusBar(msg); util.PushEndlessProgress(msg) })
	}
	backup, e := repo.LocalRepoContext(ctx, dejavu.LocalRepoRequest{Kind: "index", Memo: "Backup before checkout"}, dejavu.SnapshotOptions{Event: event}, nil)
	if backup.Applied {
		out.BackupID = backup.ID
	}
	if e != nil {
		out.Uncertain = backup.Uncertain
		return out, errors.Join(cloudrepo.ErrRestoreBackup, cloudrepo.ErrProvider, ctx.Err())
	}
	if e = ctx.Err(); e != nil {
		return out, e
	}
	// Once source writes start, cancellation is deferred through checkout and
	// derived rebuild. Owner and legacy queue both join this critical callback.
	critical := context.WithoutCancel(ctx)
	effect, e := repo.CheckoutOwned(ctx, id, event)
	out.Applied, out.SourceTouched, out.Uncertain = effect.Applied, effect.Touched, effect.Uncertain
	if effect.Applied || effect.Touched {
		reindexErr := ports.Reindex(critical)
		out.Reindexed = reindexErr == nil
		if reindexErr != nil {
			out.Uncertain = true
		}
		e = errors.Join(e, reindexErr)
		c.repoMu.Lock()
		c.repoRestoreFailed = e != nil
		c.repoMu.Unlock()
	}
	out.CancellationDeferred = effect.Started && ctx.Err() != nil
	if e != nil {
		return out, errors.Join(cloudrepo.ErrProvider, e, ctx.Err())
	}
	ui(ctx, util.ReloadUI)
	if v.Sync.Enabled {
		c.scheduleRestoreNotice(p, v)
	}
	return out, ctx.Err()
}

func (c *AppConf) RestoreDegraded() bool {
	if c == nil {
		return false
	}
	c.repoMu.Lock()
	defer c.repoMu.Unlock()
	return c.repoRestoreFailed
}

func (c *AppConf) scheduleRestoreNotice(p metadataRepoPaths, v conf.AppData) {
	c.repoMu.Lock()
	defer c.repoMu.Unlock()
	if c.repoClosed || !c.restoreScope(p) {
		return
	}
	if c.repoTasks == nil {
		c.repoTasks = taskgroup.Open()
	}
	c.repoTasks.Go(func(ctx context.Context) {
		if waitRepoDelay(ctx, 7*time.Second) != nil {
			return
		}
		_ = c.WithSettingSnapshot(ctx, func(current conf.AppData) error {
			if current.Sync == nil || current.Sync.Enabled || current.Repo == nil || !reflect.DeepEqual(current.Repo.Key, v.Repo.Key) {
				return nil
			}
			c.repoMu.Lock()
			defer c.repoMu.Unlock()
			if !c.repoClosed && !c.repoCheckoutClosed && !c.repoRestoreFailed && c.restoreScope(p) && ctx.Err() == nil && task.Accepting() && !util.IsExiting.Load() {
				util.PushMsg(c.business.Language(v.Lang, 134, true, v.CloudRegion), 0)
			}
			return nil
		})
	})
}

func EnqueueRepoCheckoutContext(ctx context.Context, id string) error {
	if !cloudrepo.ValidSnapshotID(id) {
		return cloudrepo.ErrSnapshotID
	}
	if e := ctx.Err(); e != nil {
		return e
	}
	c := Conf
	s, e := c.CloudRepoService()
	if e != nil {
		return e
	}
	c.repoMu.Lock()
	defer c.repoMu.Unlock()
	if c.repoClosed || c.repoCheckoutClosed || c != Conf {
		return cloudrepo.ErrClosed
	}
	if c.repoCheckouts == nil {
		c.repoCheckouts = map[string]*task.ContextTask{}
	}
	if c.repoCheckouts[id] != nil {
		return nil
	}
	// The accepted asynchronous job belongs to the owner, not the HTTP request
	// that ends as soon as the original null success is returned.
	if c.repoTasks == nil {
		c.repoTasks = taskgroup.Open()
	}
	var life context.Context
	ready := make(chan struct{})
	queued := make(chan *task.ContextTask, 1)
	if !c.repoTasks.Go(func(owner context.Context) {
		life = owner
		close(ready)
		h := <-queued
		if h == nil {
			return
		}
		_ = h.Wait(context.Background())
		c.repoMu.Lock()
		delete(c.repoCheckouts, id)
		c.repoMu.Unlock()
	}) {
		return cloudrepo.ErrClosed
	}
	<-ready
	if e := ctx.Err(); e != nil {
		queued <- nil
		return e
	}
	scope := metadataRepoPaths{Workspace: util.WorkspaceDir, Data: util.DataDir, Repo: util.RepoDir, History: util.HistoryDir, Temp: util.TempDir}
	h, e := task.AppendContext(life, task.RepoCheckout, func(op context.Context) error {
		if !c.restoreScope(scope) {
			return cloudrepo.ErrClosed
		}
		out, e := s.Restore(op, id)
		_ = out
		if e != nil {
			msg := c.Language(141)
			if errors.Is(e, cloudrepo.ErrKey) {
				msg = c.Language(26)
			}
			if errors.Is(e, cloudrepo.ErrRestoreBackup) {
				msg = fmt.Sprintf(c.Language(140), cloudrepo.ErrProvider)
			}
			c.notifyRepo(op, msg, 7000)
		}
		return e
	})
	queued <- h
	if e != nil {
		return e
	}
	c.repoCheckouts[id] = h
	return nil
}

func finishRestoreWatchers(ctx context.Context, resume func(context.Context) error) (err error) {
	defer func() {
		if recover() != nil {
			err = task.ErrHandler
		}
	}()
	return resume(ctx)
}

// The compatibility switch calls this before IsExiting/DB teardown, so an
// admitted critical restore can finish its original rebuild on the old owner.
func (c *AppConf) StopRepoCheckouts(ctx context.Context) error {
	c.repoMu.Lock()
	c.repoCheckoutClosed = true
	list := []*task.ContextTask{}
	for _, h := range c.repoCheckouts {
		list = append(list, h)
	}
	c.repoMu.Unlock()
	for _, h := range list {
		h.Cancel()
	}
	for _, h := range list {
		if e := h.Join(ctx); e != nil {
			return e
		}
	}
	return nil
}
