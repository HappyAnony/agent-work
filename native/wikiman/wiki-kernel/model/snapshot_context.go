// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"fmt"
	"github.com/88250/go-humanize"
	"github.com/siyuan-note/dejavu"
	"github.com/siyuan-note/dejavu/cloud"
	"github.com/siyuan-note/dejavu/cloudlock"
	"github.com/siyuan-note/eventbus"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"wiki-kernel/conf"
	"wiki-kernel/internal/cloudrepo"
	"wiki-kernel/internal/repoview"
	"wiki-kernel/util"
)

func (c *AppConf) newSnapshotRepoService(p metadataRepoPaths) (*cloudrepo.Service, error) {
	return cloudrepo.New(cloudrepo.Options{
		ReadRepository: func(ctx context.Context, v conf.AppData, q repoview.Request) (repoview.Result, error) {
			return c.executeRepositoryRead(ctx, v, q, p)
		},
		NotebookIndex: func(ctx context.Context, v conf.AppData, r cloudrepo.NotebookIndexRequest) error {
			if !c.restoreScope(p) {
				return cloudrepo.ErrDependency
			}
			return c.executeNotebookIndex(ctx, r)
		},
		IncrementalIndex: func(ctx context.Context, v conf.AppData, request cloudrepo.IncrementalIndexRequest) error {
			if !c.restoreScope(p) {
				return cloudrepo.ErrDependency
			}
			if e := FlushTxQueueContext(ctx); e != nil {
				return e
			}
			r, e := captureIndexRepair(ctx)
			if e != nil {
				return e
			}
			paths, e := r.expandIndexPaths(ctx, request.Paths)
			if e != nil {
				return e
			}
			var upserts, removes []string
			if request.Remove {
				removes = paths
			} else {
				upserts = paths
			}
			_, e = r.incrementalIndex(ctx, upserts, removes)
			return e
		},
		RefreshReferences: func(ctx context.Context, v conf.AppData, id string) error {
			if !c.restoreScope(p) {
				return cloudrepo.ErrDependency
			}
			if e := FlushTxQueueContext(ctx); e != nil {
				return e
			}
			r, e := captureIndexRepair(ctx)
			if e != nil {
				return e
			}
			if e = r.refreshReferences(ctx, id); e != nil {
				return e
			}
			return FlushTxQueueContext(ctx)
		},
		FullSync: func(ctx context.Context, v conf.AppData, r cloudrepo.FullSyncRequest) error {
			return c.executeLegacyFullSync(ctx, v, r, p)
		},
		Repair: func(ctx context.Context, v conf.AppData) error {
			if !c.restoreScope(p) {
				return cloudrepo.ErrDependency
			}
			c.m.Lock()
			readonly := c.persistenceReadOnly
			c.m.Unlock()
			if readonly {
				return ErrSettingReadOnly
			}
			return checkIndexContext(ctx)
		},
		Load: func(ctx context.Context) (out conf.AppData, e error) {
			e = c.WithSettingSnapshot(ctx, func(v conf.AppData) error { out = v; return nil })
			return
		},
		Execute: func(ctx context.Context, v conf.AppData, r cloudrepo.Request) (cloudrepo.Result, error) {
			return c.executeLegacyMetadata(ctx, v, r, p)
		},
		Directory: func(ctx context.Context, v conf.AppData, r cloudrepo.DirectoryRequest) (cloudrepo.DirectoryResult, error) {
			return c.executeLegacyDirectory(ctx, v, r, p, cloudlock.Options{})
		},
		Snapshot: func(ctx context.Context, v conf.AppData, r cloudrepo.SnapshotRequest) (cloudrepo.SnapshotResult, error) {
			return c.executeSnapshot(ctx, v, r, p)
		},
		LocalRepo: func(ctx context.Context, v conf.AppData, r cloudrepo.LocalRepoRequest, policy cloudrepo.RetentionPolicy) (cloudrepo.LocalRepoResult, error) {
			return c.executeLocalRepo(ctx, v, r, p, policy)
		},
		RepoKey: func(ctx context.Context, v conf.AppData, r cloudrepo.RepoKeyRequest, key []byte) (cloudrepo.RepoKeyResult, error) {
			return c.executeRepoKey(ctx, v, r, key, p)
		},
		PrepareCheckout: func(ctx context.Context, v conf.AppData, id string) (cloudrepo.CheckoutPlan, error) {
			return prepareRepoCheckout(ctx, v, id, p)
		},
		Restore: func(ctx context.Context, v conf.AppData, id string) (cloudrepo.RestoreResult, error) {
			return c.executeLegacyRestore(ctx, v, id, p, c.legacyRestorePorts(p))
		},
	})
}
func validateSnapshotPaths(p metadataRepoPaths) error {
	repo, e := canonicalDirectoryPath(p.Repo)
	if e != nil {
		return cloudrepo.ErrDependency
	}
	for _, root := range []string{p.Data, p.History, p.Temp} {
		resolved, e := canonicalDirectoryPath(root)
		if e != nil {
			return cloudrepo.ErrDependency
		}
		if directoryContains(repo, resolved) || directoryContains(resolved, repo) {
			return cloudrepo.ErrDependency
		}
	}
	return nil
}
func (c *AppConf) executeSnapshot(ctx context.Context, v conf.AppData, r cloudrepo.SnapshotRequest, p metadataRepoPaths) (out cloudrepo.SnapshotResult, err error) {
	c.m.Lock()
	readonly := c.persistenceReadOnly
	c.m.Unlock()
	if readonly {
		return out, ErrSettingReadOnly
	}
	if v.System == nil || v.Sync == nil || v.Repo == nil {
		return out, cloudrepo.ErrDependency
	}
	if !cloud.IsValidCloudDirName(v.Sync.CloudName) {
		if e := c.commitRepoName(ctx, v, "main"); e != nil {
			return out, e
		}
		v.Sync.CloudName = "main"
	}
	if e := validateSnapshotPaths(p); e != nil {
		return out, e
	}
	if r.Kind == "upload" || r.Kind == "download" || r.Kind == "cloud-untag" || r.Kind == "purge-cloud" {
		if e := validateLocalDirectoryTarget(v, p); e != nil {
			return out, e
		}
		if v.Sync.Provider == conf.ProviderLocal {
			root := filepath.Join(v.Sync.Local.Endpoint, v.Sync.CloudName)
			if e := filepath.WalkDir(root, func(_ string, d fs.DirEntry, e error) error {
				if os.IsNotExist(e) {
					return nil
				}
				if e != nil {
					return e
				}
				if d.Type()&os.ModeSymlink != 0 {
					return dejavu.ErrSnapshot
				}
				return ctx.Err()
			}); e != nil {
				return out, e
			}
		}
	}
	workspace, data := util.WorkspaceDir, util.DataDir
	ui := func(cctx context.Context, fn func()) {
		c.repoMu.Lock()
		defer c.repoMu.Unlock()
		if c.repoClosed || c != Conf || workspace != util.WorkspaceDir || data != util.DataDir || cctx.Err() != nil {
			return
		}
		fn()
	}
	lang := func(id int) string { return c.business.Language(v.Lang, id, true, v.CloudRegion) }
	progress := r.Kind == "upload" || r.Kind == "download" || r.Kind == "purge-cloud" || r.Kind == "purge-local"
	if progress {
		id := 116
		if r.Kind == "purge-cloud" {
			id = 223
		}
		if r.Kind == "purge-local" {
			id = 202
		}
		ui(ctx, func() { util.PushEndlessProgress(lang(id)) })
		defer ui(context.WithoutCancel(ctx), util.PushClearProgress)
	}
	var transports []*http.Transport
	p.Transports = &transports
	defer func() {
		for _, t := range transports {
			t.CloseIdleConnections()
		}
	}()
	repo, e := newMetadataRepository(ctx, v, p)
	if e != nil {
		return out, e
	}
	defer repo.CloseMetadata()
	// Owner-scoped projection of the original repo event messages. No global
	// subscriptions, shared progress counters, or captured mutable Conf are used.
	topics := map[string]int{eventbus.EvtCloudBeforeDownloadIndex: 164, eventbus.EvtCloudBeforeDownloadFiles: 165, eventbus.EvtCloudBeforeDownloadFile: 165, eventbus.EvtCloudBeforeDownloadChunks: 166, eventbus.EvtCloudBeforeDownloadChunk: 166, eventbus.EvtCloudBeforeUploadIndex: 168, eventbus.EvtCloudBeforeUploadFiles: 169, eventbus.EvtCloudBeforeUploadFile: 169, eventbus.EvtCloudBeforeUploadChunks: 170, eventbus.EvtCloudBeforeUploadChunk: 170, eventbus.EvtCloudBeforeUploadRef: 171, eventbus.EvtCloudLock: 186, eventbus.EvtCloudUnlock: 187, eventbus.EvtCloudPurgeListObjects: 224, eventbus.EvtCloudPurgeListIndexes: 225, eventbus.EvtCloudPurgeListRefs: 226, eventbus.EvtCloudPurgeDownloadIndexes: 227, eventbus.EvtCloudPurgeDownloadFiles: 228, eventbus.EvtCloudPurgeRemoveIndexes: 229, eventbus.EvtCloudPurgeRemoveIndexesV2: 230, eventbus.EvtCloudPurgeRemoveObjects: 231}
	result, e := repo.SnapshotContext(ctx, dejavu.SnapshotRequest{Kind: r.Kind, ID: r.ID, Tag: r.Tag}, dejavu.SnapshotOptions{Event: func(cctx context.Context, topic string, args ...interface{}) {
		id := topics[topic]
		if id == 0 {
			return
		}
		vals := []interface{}{}
		if len(args) > 0 {
			vals = args[1:]
		}
		if (id == 164 || id == 168) && len(vals) > 0 {
			if s, ok := vals[0].(string); ok && len(s) >= 7 {
				vals[0] = s[:7]
			}
		}
		if (id == 165 || id == 166 || id == 169 || id == 170) && len(vals) == 1 {
			vals = []interface{}{0, vals[0]}
		}
		msg := fmt.Sprintf(lang(id), vals...)
		ui(cctx, func() {
			util.SetBootDetails(msg)
			util.PushStatusBar(msg)
			if progress {
				util.PushEndlessProgress(msg)
			}
		})
	}})
	out = cloudrepo.SnapshotResult{Files: result.Files, Chunks: result.Chunks, Bytes: result.Bytes, Indexes: result.Indexes, Objects: result.Objects, Applied: result.Applied, Uncertain: result.Uncertain}
	if e != nil {
		cause := cloudrepo.ErrProvider
		for _, known := range []error{dejavu.ErrSnapshot, dejavu.ErrCloudBackupCountExceeded, dejavu.ErrCloudStorageSizeExceeded, cloudlock.ErrBusy, cloudlock.ErrLost, cloudlock.ErrUnlock, cloudlock.ErrAcquire} {
			if errors.Is(e, known) {
				cause = known
				break
			}
		}
		return out, errors.Join(cause, ctx.Err())
	}
	if progress {
		msg := ""
		size := humanize.BytesCustomCeil(uint64(out.Bytes), 2)
		timeout := 5000
		switch r.Kind {
		case "upload":
			msg = fmt.Sprintf(lang(152), out.Files, out.Chunks, size)
		case "download":
			msg = fmt.Sprintf(lang(153), out.Files, out.Chunks, size)
		case "purge-local":
			msg = fmt.Sprintf(lang(203), out.Indexes, out.Objects, size)
			timeout = 7000
		case "purge-cloud":
			msg = fmt.Sprintf(lang(232), out.Indexes, out.Objects, size)
			timeout = 7000
		}
		ui(ctx, func() { util.PushMsg(msg, timeout); util.PushStatusBar(msg) })
	}
	return out, nil
}
func ChangeSnapshotContext(ctx context.Context, kind, id, tag string) (cloudrepo.SnapshotResult, error) {
	owner := Conf
	workspace, data := util.WorkspaceDir, util.DataDir
	s, e := owner.CloudRepoService()
	if e != nil {
		return cloudrepo.SnapshotResult{}, e
	}
	out, e := s.Snapshot(ctx, cloudrepo.SnapshotRequest{Kind: kind, ID: id, Tag: tag})
	if out.Notice != 0 && workspace == util.WorkspaceDir && data == util.DataDir {
		owner.notifyRepo(ctx, owner.Language(out.Notice), 5000)
	}
	for _, v := range []struct {
		cause error
		id    int
	}{{cloudrepo.ErrKey, 26}, {cloudrepo.ErrTag, 151}, {cloudrepo.ErrProvider, 30}, {dejavu.ErrCloudBackupCountExceeded, 154}} {
		if errors.Is(e, v.cause) {
			e = directoryBusinessError{e, owner.Language(v.id)}
			break
		}
	}
	return out, e
}
