// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"fmt"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/internal/cloudrepo"
	"wiki-kernel/internal/taskgroup"
	"wiki-kernel/util"
)

// Native owners do not acquire the compatibility source-write capability.
func (c *AppConf) executeLegacyFullSync(ctx context.Context, v conf.AppData, r cloudrepo.FullSyncRequest, p metadataRepoPaths) (err error) {
	defer recoverRepositoryWorkflow(&err, "full_sync")
	if !c.restoreScope(p) {
		return cloudrepo.ErrDependency
	}
	c.m.Lock()
	readonly := c.persistenceReadOnly
	c.m.Unlock()
	if readonly {
		return ErrSettingReadOnly
	}
	if v.Sync == nil || v.Repo == nil {
		return cloudrepo.ErrDependency
	}
	switch r.Mode {
	case "normal":
		return legacySyncData(ctx, false, r.ByHand)
	case "exit":
		return legacySyncData(ctx, true, r.ByHand)
	case "upload":
		return legacySyncDataUpload(ctx)
	case "download":
		return legacySyncDataDownload(ctx)
	case "boot":
		return legacyBootSyncData(ctx)
	case "boot-followup":
		defer c.syncRuntime.booting.Store(false)
		if !checkSync(false, false, false) {
			return nil
		}
		_, e := syncRepo(ctx, false, false)
		return e
	default:
		return cloudrepo.ErrOperation
	}
}
func SyncDataContext(ctx context.Context, mode string, byHand bool) (cloudrepo.FullSyncResult, error) {
	owner := Conf
	return owner.syncDataContext(ctx, mode, byHand)
}

func (c *AppConf) syncDataContext(ctx context.Context, mode string, byHand bool) (cloudrepo.FullSyncResult, error) {
	if ctx == nil {
		return cloudrepo.FullSyncResult{}, cloudrepo.ErrDependency
	}
	if err := ctx.Err(); err != nil {
		return cloudrepo.FullSyncResult{}, err
	}
	if c == nil {
		return cloudrepo.FullSyncResult{}, cloudrepo.ErrDependency
	}
	s, e := c.CloudRepoService()
	if e != nil {
		return cloudrepo.FullSyncResult{}, e
	}
	return s.FullSync(ctx, cloudrepo.FullSyncRequest{Mode: mode, ByHand: byHand})
}
func SyncDataDownload() { _, _ = SyncDataContext(context.Background(), "download", true) }
func SyncDataUpload()   { _, _ = SyncDataContext(context.Background(), "upload", true) }
func BootSyncData()     { _, _ = SyncDataContext(context.Background(), "boot", false) }
func syncData(exit, byHand bool) {
	mode := "normal"
	if exit {
		mode = "exit"
	}
	_, _ = SyncDataContext(context.Background(), mode, byHand)
}
func (c *AppConf) scheduleBootSync(p metadataRepoPaths) bool {
	c.repoMu.Lock()
	defer c.repoMu.Unlock()
	if c.repoClosed {
		return false
	}
	if c.repoTasks == nil {
		c.repoTasks = taskgroup.Open()
	}
	s := c.repoOwner
	return c.repoTasks.Go(func(ctx context.Context) {
		defer c.syncRuntime.booting.Store(false)
		if !c.restoreScope(p) {
			return
		}
		_, e := s.FullSync(ctx, cloudrepo.FullSyncRequest{Mode: "boot-followup"})
		if e != nil && ctx.Err() == nil {
			c.notifyRepo(ctx, fmt.Sprintf("boot background sync failed: %v", e), 5000)
		}
	})
}

// Delayed UI work retains its owner and is joined on switch/exit. The repository
// handle is not retained by a detached goroutine after its caller returns.
func (c *AppConf) scheduleIndexNotice(count int, p metadataRepoPaths) {
	if count < 128 {
		return
	}
	c.repoMu.Lock()
	defer c.repoMu.Unlock()
	if c.repoClosed || c.syncRuntime.noticePending {
		return
	}
	c.syncRuntime.noticePending = true
	if c.repoTasks == nil {
		c.repoTasks = taskgroup.Open()
	}
	if !c.repoTasks.Go(func(ctx context.Context) {
		if util.WaitForUILoadedContext(ctx) != nil {
			return
		}
		timer := time.NewTimer(3 * time.Second)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		if c.restoreScope(p) {
			c.notifyRepo(ctx, c.Language(218), 24000)
		}
	}) {
		c.syncRuntime.noticePending = false
	}
}

// UI-only callbacks are cancelable and joined independently of critical source
// phases. They may publish only while their captured owner still owns paths.
func (c *AppConf) scheduleSyncUI(fn func(context.Context)) {
	c.repoMu.Lock()
	defer c.repoMu.Unlock()
	if c.repoClosed {
		return
	}
	p := metadataRepoPaths{Data: util.DataDir, Repo: util.RepoDir, History: util.HistoryDir, Temp: util.TempDir, Workspace: util.WorkspaceDir}
	if c.repoTasks == nil {
		c.repoTasks = taskgroup.Open()
	}
	c.repoTasks.Go(func(ctx context.Context) {
		if util.WaitForUILoadedContext(ctx) != nil || !c.restoreScope(p) {
			return
		}
		fn(ctx)
	})
}

func (c *AppConf) publishSyncUI(ctx context.Context, fn func()) {
	c.repoMu.Lock()
	defer c.repoMu.Unlock()
	if c.repoClosed || c != Conf || ctx.Err() != nil {
		return
	}
	fn()
}
