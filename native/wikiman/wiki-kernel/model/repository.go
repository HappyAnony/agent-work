// SiYuan - Refactor your thinking
// Copyright (c) 2020-present, b3log.org
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package model

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/88250/go-humanize"
	"github.com/88250/gulu"
	"github.com/88250/lute/html"
	"github.com/emirpasic/gods/sets/hashset"
	"github.com/siyuan-note/dejavu"
	"github.com/siyuan-note/dejavu/cloud"
	"github.com/siyuan-note/dejavu/entity"
	"github.com/siyuan-note/eventbus"
	"github.com/siyuan-note/httpclient"
	"github.com/siyuan-note/logging"
	"github.com/studio-b12/gowebdav"
	"golang.org/x/sync/errgroup"
	"wiki-kernel/conf"
	"wiki-kernel/internal/repoview"
	"wiki-kernel/sql"
	"wiki-kernel/treenode"
	"wiki-kernel/util"
)

// AutoPurgeRepoJob 自动清理数据仓库 https://github.com/siyuan-note/siyuan/issues/13091
func AutoPurgeRepoJob() { AutoPurgeRepoJobContext(context.Background()) }

type LeftRightDiff = repoview.Diff
type DiffFile = repoview.DiffFile
type DiffIndex = repoview.DiffIndex
type Snapshot = repoview.Snapshot
type TypeCount = repoview.TypeCount

func GetRepoFile(id string) ([]byte, string, error) {
	return GetRepoFileContext(context.Background(), id)
}
func OpenRepoSnapshotDoc(id string) (string, string, bool, int64, error) {
	return OpenRepoSnapshotDocContext(context.Background(), id)
}
func DiffRepoSnapshots(left, right string) (*LeftRightDiff, error) {
	return DiffRepoSnapshotsContext(context.Background(), left, right)
}
func GetRepoSnapshots(page int) ([]*Snapshot, int, int, error) {
	return GetRepoSnapshotsContext(context.Background(), page)
}

func ImportRepoKey(input string) (string, error) {
	out, e := ChangeRepoKeyContext(context.Background(), "import", input)
	return out.Normalized, e
}
func ResetRepo() error { _, e := ChangeRepoKeyContext(context.Background(), "reset", ""); return e }

func PurgeCloud() (err error) {
	_, err = ChangeSnapshotContext(context.Background(), "purge-cloud", "", "")
	return
}

func PurgeRepo() (err error) {
	_, err = ChangeSnapshotContext(context.Background(), "purge-local", "", "")
	return
}

func InitRepoKeyFromPassphrase(input string) error {
	_, e := ChangeRepoKeyContext(context.Background(), "passphrase", input)
	return e
}
func InitRepoKey() error { _, e := ChangeRepoKeyContext(context.Background(), "random", ""); return e }

func CheckoutRepo(id string) {
	if e := EnqueueRepoCheckoutContext(context.Background(), id); e != nil {
		Conf.NotifyCloudRepoError(context.Background(), e)
	}
}

// Kept for internal compatibility callers; public HTTP admits an owned task.
func checkoutRepo(id string) {
	s, e := Conf.CloudRepoService()
	if e == nil {
		_, e = s.Restore(context.Background(), id)
	}
	if e != nil {
		Conf.NotifyCloudRepoError(context.Background(), e)
	}
}

func DownloadCloudSnapshot(tag, id string) (err error) {
	_, err = ChangeSnapshotContext(context.Background(), "download", id, tag)
	return
}

func UploadCloudSnapshot(tag, id string) (err error) {
	_, err = ChangeSnapshotContext(context.Background(), "upload", id, tag)
	return
}

func RemoveCloudRepoTag(tag string) (err error) {
	_, err = ChangeSnapshotContext(context.Background(), "cloud-untag", "", tag)
	return
}

func GetCloudRepoTagSnapshots() (ret []*dejavu.Log, err error) {
	out, e := QueryCloudRepoContext(context.Background(), "tags", 1)
	if e != nil {
		return []*dejavu.Log{}, e
	}
	var v struct {
		Snapshots []*dejavu.Log `json:"snapshots"`
	}
	e = json.Unmarshal(out.Data, &v)
	return v.Snapshots, e
}

func GetCloudRepoSnapshots(page int) (ret []*dejavu.Log, pageCount, totalCount int, err error) {
	out, e := QueryCloudRepoContext(context.Background(), "logs", page)
	if e != nil {
		return []*dejavu.Log{}, 0, 0, e
	}
	var v struct {
		Snapshots  []*dejavu.Log `json:"snapshots"`
		PageCount  int           `json:"pageCount"`
		TotalCount int           `json:"totalCount"`
	}
	e = json.Unmarshal(out.Data, &v)
	return v.Snapshots, v.PageCount, v.TotalCount, e
}

func GetTagSnapshots() ([]*Snapshot, error) { return GetTagSnapshotsContext(context.Background()) }

func RemoveTagSnapshot(tag string) (err error) {
	_, err = ChangeSnapshotContext(context.Background(), "untag", "", tag)
	return
}

func TagSnapshot(id, name string) (err error) {
	_, err = ChangeSnapshotContext(context.Background(), "tag", id, name)
	return
}

func IndexRepo(memo string) error { return IndexRepoContext(context.Background(), memo) }

func waitForSyncingStorages() {
	_ = waitForSyncingStoragesContext(context.Background())
}

func waitForSyncingStoragesContext(ctx context.Context) error {
	return waitForSyncingStoragesOwnerContext(ctx, Conf)
}

func waitForSyncingStoragesOwnerContext(ctx context.Context, owner *AppConf) error {
	if ctx == nil {
		return errors.New("storage_sync_context_unbound")
	}
	if owner == nil {
		return errors.New("storage_sync_owner_unbound")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	for isSyncingStoragesOwner(owner) {
		timer := time.NewTimer(time.Second)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return ctx.Err()
		case <-timer.C:
		}
	}
	return ctx.Err()
}

func isSyncingStorages() bool {
	return isSyncingStoragesOwner(Conf)
}

func isSyncingStoragesOwner(owner *AppConf) bool {
	return owner != nil && (owner.syncRuntime.storages.Load() || owner.syncRuntime.booting.Load())
}

func IsSyncingFile(rootID string) (ret bool) {
	ret, _ = IsSyncingFileContext(context.Background(), rootID)
	return
}

func IsSyncingFileContext(ctx context.Context, id string) (bool, error) {
	o, e := captureLegacyReindexOwner(ctx)
	if e != nil {
		return false, e
	}
	_, found := o.owner.syncRuntime.files.Load(id)
	if e = o.guard(ctx); e != nil {
		return false, e
	}
	return found, nil
}

func syncRepoDownload(ctx context.Context) (err error) {
	repoOwner := Conf
	if 1 > len(Conf.Repo.Key) {
		repoOwner.planSyncAfter(fixSyncInterval)

		msg := Conf.Language(26)
		util.PushStatusBar(msg)
		util.PushErrMsg(msg, 0)
		err = errors.New(msg)
		return
	}

	repo, err := newRepositoryContext(ctx)
	if err != nil {
		repoOwner.planSyncAfter(fixSyncInterval)

		msg := fmt.Sprintf("sync repo failed: %s", err)
		logging.LogErrorf("%s", msg)
		util.PushStatusBar(msg)
		util.PushErrMsg(msg, 0)
		return
	}

	defer repo.CloseMetadata()
	logging.LogInfof("downloading data repo [device=%s, kernel=%s, provider=%d, mode=%s/%t]", Conf.System.ID, KernelID, Conf.Sync.Provider, "d", true)
	start := time.Now()
	_, _, err = indexRepoBeforeCloudSync(ctx, repo)
	if err != nil {
		repoOwner.planSyncAfter(fixSyncInterval)

		logging.LogErrorf("sync data repo download failed: %s", err)
		msg := fmt.Sprintf(Conf.Language(80), formatRepoErrorMsg(err))
		Conf.Sync.Stat = msg
		Conf.Save()
		util.PushStatusBar(msg)
		util.PushErrMsg(msg, 0)
		return
	}

	beforeSyncPetals, err := GetPetalsContext(ctx)
	if err != nil {
		return
	}

	eventSession, syncContext, err := captureLegacyRepoEventPayload(ctx, eventbus.CtxPushMsgToStatusBar)
	if err != nil {
		return
	}
	deactivateEvents := eventSession.activate()
	defer deactivateEvents()
	mergeResult, trafficStat, err := repo.SyncDownload(syncContext)
	elapsed := time.Since(start)
	if err != nil {
		repoOwner.planSyncAfter(fixSyncInterval)

		logging.LogErrorf("sync data repo download failed: %s", err)
		msg := fmt.Sprintf(Conf.Language(80), formatRepoErrorMsg(err))
		if errors.Is(err, dejavu.ErrCloudStorageSizeExceeded) {
			u := Conf.GetUser()
			msg = fmt.Sprintf(Conf.Language(43), humanize.BytesCustomCeil(uint64(u.UserSiYuanRepoSize), 2))
			if 2 == u.UserSiYuanSubscriptionPlan {
				msg = fmt.Sprintf(Conf.Language(68), humanize.BytesCustomCeil(uint64(u.UserSiYuanRepoSize), 2))
			}
		}
		Conf.Sync.Stat = msg
		Conf.Save()
		util.PushStatusBar(msg)
		util.PushErrMsg(msg, 0)
		return
	}

	util.PushStatusBar(fmt.Sprintf(Conf.Language(149), elapsed.Seconds()))
	Conf.Sync.Synced = util.CurrentTimeMillis()
	msg := fmt.Sprintf(Conf.Language(150), trafficStat.UploadFileCount, trafficStat.DownloadFileCount, trafficStat.UploadChunkCount, trafficStat.DownloadChunkCount, humanize.BytesCustomCeil(uint64(trafficStat.UploadBytes), 2), humanize.BytesCustomFloor(uint64(trafficStat.DownloadBytes), 2))
	Conf.Sync.Stat = msg
	Conf.Save()
	Conf.syncRuntime.errors.Store(0)
	Conf.syncRuntime.boot.Store(1)

	if err = calcPetalDiffContext(ctx, beforeSyncPetals, mergeResult); err != nil {
		return
	}
	if err = processSyncMergeResultOwner(ctx, repoOwner, false, true, mergeResult, trafficStat, "d", elapsed); err != nil {
		return
	}
	return
}

func syncRepoUpload(ctx context.Context) (err error) {
	repoOwner := Conf
	if 1 > len(Conf.Repo.Key) {
		repoOwner.planSyncAfter(fixSyncInterval)

		msg := Conf.Language(26)
		util.PushStatusBar(msg)
		util.PushErrMsg(msg, 0)
		err = errors.New(msg)
		return
	}

	repo, err := newRepositoryContext(ctx)
	if err != nil {
		repoOwner.planSyncAfter(fixSyncInterval)

		msg := fmt.Sprintf("sync repo failed: %s", err)
		logging.LogErrorf("%s", msg)
		util.PushStatusBar(msg)
		util.PushErrMsg(msg, 0)
		return
	}

	defer repo.CloseMetadata()
	logging.LogInfof("uploading data repo [device=%s, kernel=%s, provider=%d, mode=%s/%t]", Conf.System.ID, KernelID, Conf.Sync.Provider, "u", true)
	start := time.Now()
	_, _, err = indexRepoBeforeCloudSync(ctx, repo)
	if err != nil {
		repoOwner.planSyncAfter(fixSyncInterval)

		logging.LogErrorf("sync data repo upload failed: %s", err)
		msg := fmt.Sprintf(Conf.Language(80), formatRepoErrorMsg(err))
		Conf.Sync.Stat = msg
		Conf.Save()
		util.PushStatusBar(msg)
		util.PushErrMsg(msg, 0)
		return
	}

	eventSession, syncContext, err := captureLegacyRepoEventPayload(ctx, eventbus.CtxPushMsgToStatusBar)
	if err != nil {
		return err
	}
	deactivateEvents := eventSession.activate()
	defer deactivateEvents()
	trafficStat, err := repo.SyncUpload(syncContext)
	elapsed := time.Since(start)
	if err != nil {
		repoOwner.planSyncAfter(fixSyncInterval)

		logging.LogErrorf("sync data repo upload failed: %s", err)
		msg := fmt.Sprintf(Conf.Language(80), formatRepoErrorMsg(err))
		if errors.Is(err, dejavu.ErrCloudStorageSizeExceeded) {
			u := Conf.GetUser()
			msg = fmt.Sprintf(Conf.Language(43), humanize.BytesCustomCeil(uint64(u.UserSiYuanRepoSize), 2))
			if 2 == u.UserSiYuanSubscriptionPlan {
				msg = fmt.Sprintf(Conf.Language(68), humanize.BytesCustomCeil(uint64(u.UserSiYuanRepoSize), 2))
			}
		}
		Conf.Sync.Stat = msg
		Conf.Save()
		util.PushStatusBar(msg)
		util.PushErrMsg(msg, 0)
		return
	}

	util.PushStatusBar(fmt.Sprintf(Conf.Language(149), elapsed.Seconds()))
	Conf.Sync.Synced = util.CurrentTimeMillis()
	msg := fmt.Sprintf(Conf.Language(150), trafficStat.UploadFileCount, trafficStat.DownloadFileCount, trafficStat.UploadChunkCount, trafficStat.DownloadChunkCount, humanize.BytesCustomCeil(uint64(trafficStat.UploadBytes), 2), humanize.BytesCustomCeil(uint64(trafficStat.DownloadBytes), 2))
	Conf.Sync.Stat = msg
	Conf.Save()
	Conf.syncRuntime.errors.Store(0)
	Conf.syncRuntime.boot.Store(1)

	if err = processSyncMergeResultOwner(ctx, repoOwner, false, true, &dejavu.MergeResult{}, trafficStat, "u", elapsed); err != nil {
		return
	}
	return
}

func bootSyncRepo(ctx context.Context) (err error) {
	repoOwner := Conf
	if 1 > len(repoOwner.Repo.Key) {
		repoOwner.syncRuntime.errors.Add(1)
		repoOwner.planSyncAfter(fixSyncInterval)

		msg := repoOwner.Language(26)
		util.PushStatusBar(msg)
		util.PushErrMsg(msg, 0)
		err = errors.New(msg)
		return
	}

	repo, err := newRepositoryContext(ctx)
	if err != nil {
		repoOwner.syncRuntime.errors.Add(1)
		repoOwner.planSyncAfter(fixSyncInterval)

		msg := fmt.Sprintf("sync repo failed: %s", html.EscapeString(err.Error()))
		logging.LogErrorf("%s", msg)
		util.PushStatusBar(msg)
		util.PushErrMsg(msg, 0)
		return
	}

	defer repo.CloseMetadata()
	repoOwner.syncRuntime.booting.Store(true)

	fetchRepo, fetchOpenErr := newRepositoryContext(ctx)
	if fetchOpenErr != nil {
		repoOwner.syncRuntime.booting.Store(false)
		return fetchOpenErr
	}
	defer fetchRepo.CloseMetadata()
	var fetchedFiles []*entity.File
	err = runBootSyncWorkers(ctx, func(workerContext context.Context) (workerErr error) {
		_, _, workerErr = indexRepoBeforeCloudSync(workerContext, repo)
		return
	}, func(workerContext context.Context) (workerErr error) {
		eventSession, syncContext, e := captureLegacyRepoEventPayload(workerContext, eventbus.CtxPushMsgToStatusBar)
		if e != nil {
			return e
		}
		deactivateEvents := eventSession.activate()
		defer deactivateEvents()
		latest, e := fetchRepo.GetCloudLatest(syncContext)
		if e != nil {
			return e
		}
		fetchedFiles, workerErr = fetchRepo.GetSyncCloudFiles(latest, syncContext)
		return
	})
	if err != nil {
		repoOwner.syncRuntime.errors.Add(1)
		repoOwner.planSyncAfter(fixSyncInterval)
		repoOwner.Sync.Stat = fmt.Sprintf(repoOwner.Language(80), formatRepoErrorMsg(err))
		repoOwner.Save()
		util.PushStatusBar(repoOwner.Sync.Stat)
		util.PushErrMsg(repoOwner.Sync.Stat, 0)
		repoOwner.syncRuntime.boot.Store(2)
		repoOwner.syncRuntime.booting.Store(false)
		return
	}

	repoOwner.syncRuntime.files.Clear()
	repoOwner.syncRuntime.storages.Store(false)
	for _, fetchedFile := range fetchedFiles {
		name := path.Base(fetchedFile.Path)
		if util.HasDocExt(name) {
			id := name[:len(name)-3]
			repoOwner.syncRuntime.files.Store(id, true)
			continue
		}
		if strings.HasPrefix(fetchedFile.Path, "/storage/") {
			repoOwner.syncRuntime.storages.Store(true)
		}
	}

	if len(fetchedFiles) > 0 {
		p := metadataRepoPaths{Data: util.DataDir, Repo: util.RepoDir, History: util.HistoryDir, Temp: util.TempDir, Workspace: util.WorkspaceDir}
		if !repoOwner.scheduleBootSync(p) {
			repoOwner.syncRuntime.booting.Store(false)
		}
	} else {
		repoOwner.syncRuntime.booting.Store(false)
	}

	return
}

func runBootSyncWorkers(ctx context.Context, workers ...func(context.Context) error) error {
	group, groupContext := errgroup.WithContext(ctx)
	for _, worker := range workers {
		worker := worker
		group.Go(func() (workerErr error) {
			defer func() {
				if recover() != nil {
					workerErr = errors.New("boot sync worker failed")
				}
			}()
			return worker(groupContext)
		})
	}
	return group.Wait()
}

func syncRepo(ctx context.Context, exit, byHand bool) (dataChanged bool, err error) {
	repoOwner := Conf
	repoWorkspace, repoData := util.WorkspaceDir, util.DataDir
	if 1 > len(Conf.Repo.Key) {
		Conf.syncRuntime.errors.Add(1)
		repoOwner.planSyncAfter(fixSyncInterval)

		msg := Conf.Language(26)
		util.PushStatusBar(msg)
		util.PushErrMsg(msg, 0)
		err = errors.New(msg)
		return
	}

	repo, err := newRepositoryContext(ctx)
	if err != nil {
		Conf.syncRuntime.errors.Add(1)
		repoOwner.planSyncAfter(fixSyncInterval)

		msg := fmt.Sprintf("sync repo failed: %s", err)
		logging.LogErrorf("%s", msg)
		util.PushStatusBar(msg)
		util.PushErrMsg(msg, 0)
		return
	}

	defer repo.CloseMetadata()
	logging.LogInfof("syncing data repo [device=%s, kernel=%s, provider=%d, mode=%s/%t]", Conf.System.ID, KernelID, Conf.Sync.Provider, "a", byHand)
	start := time.Now()
	beforeIndex, afterIndex, err := indexRepoBeforeCloudSync(ctx, repo)
	if err != nil {
		Conf.syncRuntime.errors.Add(1)
		repoOwner.planSyncAfter(fixSyncInterval)

		logging.LogErrorf("sync data repo failed: %s", err)
		msg := fmt.Sprintf(Conf.Language(80), formatRepoErrorMsg(err))
		Conf.Sync.Stat = msg
		Conf.Save()
		util.PushStatusBar(msg)
		if 1 > Conf.syncRuntime.errors.Load() || byHand {
			util.PushErrMsg(msg, 0)
		}
		if exit {
			Conf.syncRuntime.exit.Store(2)
		}
		return
	}

	beforeSyncPetals, err := GetPetalsContext(ctx)
	if err != nil {
		return
	}

	eventSession, syncContext, err := captureLegacyRepoEventPayload(ctx, eventbus.CtxPushMsgToStatusBar)
	if err != nil {
		return false, err
	}
	deactivateEvents := eventSession.activate()
	defer deactivateEvents()
	mergeResult, trafficStat, err := repo.Sync(syncContext)
	elapsed := time.Since(start)
	if err != nil {
		Conf.syncRuntime.errors.Add(1)
		repoOwner.planSyncAfter(fixSyncInterval)

		logging.LogErrorf("sync data repo failed: %s", err)
		msg := fmt.Sprintf(Conf.Language(80), formatRepoErrorMsg(err))
		if errors.Is(err, dejavu.ErrCloudStorageSizeExceeded) {
			u := Conf.GetUser()
			msg = fmt.Sprintf(Conf.Language(43), humanize.BytesCustomCeil(uint64(u.UserSiYuanRepoSize), 2))
			if 2 == u.UserSiYuanSubscriptionPlan {
				msg = fmt.Sprintf(Conf.Language(68), humanize.BytesCustomCeil(uint64(u.UserSiYuanRepoSize), 2))
			}
		}
		Conf.Sync.Stat = msg
		Conf.Save()
		util.PushStatusBar(msg)
		if 1 > Conf.syncRuntime.errors.Load() || byHand {
			util.PushErrMsg(msg, 0)
		}
		if exit {
			Conf.syncRuntime.exit.Store(2)
		}
		return
	}

	dataChanged = nil == beforeIndex || beforeIndex.ID != afterIndex.ID || mergeResult.DataChanged()

	util.PushStatusBar(fmt.Sprintf(Conf.Language(149), elapsed.Seconds()))
	Conf.Sync.Synced = util.CurrentTimeMillis()
	msg := fmt.Sprintf(Conf.Language(150), trafficStat.UploadFileCount, trafficStat.DownloadFileCount, trafficStat.UploadChunkCount, trafficStat.DownloadChunkCount, humanize.BytesCustomCeil(uint64(trafficStat.UploadBytes), 2), humanize.BytesCustomCeil(uint64(trafficStat.DownloadBytes), 2))
	Conf.Sync.Stat = msg
	Conf.Save()
	Conf.syncRuntime.errors.Store(0)

	if err = calcPetalDiffContext(ctx, beforeSyncPetals, mergeResult); err != nil {
		return
	}
	if err = processSyncMergeResultOwner(ctx, repoOwner, exit, byHand, mergeResult, trafficStat, "a", elapsed); err != nil {
		if exit {
			Conf.syncRuntime.exit.Store(2)
		}
		return
	}

	if !exit && repoOwner == Conf && repoWorkspace == util.WorkspaceDir && repoData == util.DataDir {
		repoOwner.scheduleLocalRepo(context.Background(), false, true)
	}
	return
}

func calcPetalDiffContext(ctx context.Context, beforeSyncPetals []*Petal, mergeResult *dejavu.MergeResult) error {
	var upsertPetals, removePetals []string
	afterSyncPetals, err := GetPetalsContext(ctx)
	if err != nil {
		return err
	}
	for _, afterSyncPetal := range afterSyncPetals {
		if beforeSyncPetal := getPetalByName(afterSyncPetal.Name, beforeSyncPetals); nil != beforeSyncPetal {
			a, _ := gulu.JSON.MarshalJSON(afterSyncPetal)
			b, _ := gulu.JSON.MarshalJSON(beforeSyncPetal)
			if !bytes.Equal(a, b) {
				upsertPetals = append(upsertPetals, afterSyncPetal.Name)
			}
		} else {
			upsertPetals = append(upsertPetals, afterSyncPetal.Name)
		}
	}
	for _, beforeSyncPetal := range beforeSyncPetals {
		if nil == getPetalByName(beforeSyncPetal.Name, afterSyncPetals) {
			removePetals = append(removePetals, beforeSyncPetal.Name)
		}
	}

	mergeResult.UpsertPetals = gulu.Str.RemoveDuplicatedElem(upsertPetals)
	mergeResult.RemovePetals = gulu.Str.RemoveDuplicatedElem(removePetals)
	return nil
}

func processSyncMergeResult(ctx context.Context, exit, byHand bool, mergeResult *dejavu.MergeResult, trafficStat *dejavu.TrafficStat, mode string, elapsed time.Duration) (err error) {
	return processSyncMergeResultOwner(ctx, Conf, exit, byHand, mergeResult, trafficStat, mode, elapsed)
}

func processSyncMergeResultOwner(ctx context.Context, repoOwner *AppConf, exit, byHand bool, mergeResult *dejavu.MergeResult, trafficStat *dejavu.TrafficStat, mode string, elapsed time.Duration) (err error) {
	queue := sql.LegacyMainQueue{DB: sql.CaptureLegacyRepairQueries().DB}
	defer func() { err = errors.Join(err, queue.Flush(context.WithoutCancel(ctx))) }()
	logging.LogInfof("synced data repo [device=%s, kernel=%s, provider=%d, mode=%s/%t, ufc=%d, dfc=%d, ucc=%d, dcc=%d, ub=%s, db=%s] in [%.2fs], merge result [conflicts=%d, upserts=%d, removes=%d]\n\n",
		Conf.System.ID, KernelID, Conf.Sync.Provider, mode, byHand,
		trafficStat.UploadFileCount, trafficStat.DownloadFileCount, trafficStat.UploadChunkCount, trafficStat.DownloadChunkCount, humanize.BytesCustomCeil(uint64(trafficStat.UploadBytes), 2), humanize.BytesCustomCeil(uint64(trafficStat.DownloadBytes), 2),
		elapsed.Seconds(),
		len(mergeResult.Conflicts), len(mergeResult.Upserts), len(mergeResult.Removes))

	//logSyncMergeResult(mergeResult)

	var needReloadFiletree bool
	if 0 < len(mergeResult.Conflicts) {
		luteEngine := util.NewLute()
		needReloadFiletree, err = applySyncConflictCopies(ctx, mergeResult)
		if err != nil {
			return err
		}

		historyDir := filepath.Join(util.HistoryDir, mergeResult.Time.Format("2006-01-02-150405")+"-sync")
		if e := indexHistoryDirContext(ctx, filepath.Base(historyDir), luteEngine); e != nil {
			return e
		}
	}

	if 1 > len(mergeResult.Upserts) && 1 > len(mergeResult.Removes) && 1 > len(mergeResult.Conflicts) { // 没有数据变更
		repoOwner.syncRuntime.same.Add(1)
		if 10 < repoOwner.syncRuntime.same.Load() {
			repoOwner.syncRuntime.same.Store(5)
		}
		if !byHand {
			delay := time.Minute * time.Duration(int(math.Pow(2, float64(repoOwner.syncRuntime.same.Load()))))
			if fixSyncInterval.Minutes() > delay.Minutes() {
				delay = time.Minute * 8
			}
			repoOwner.planSyncAfter(delay)
		}
		return
	}

	// 有数据变更，需要重建索引
	var upserts, removes []string
	var upsertTrees int
	// 可能需要重新加载部分功能
	var needReloadFlashcard, needReloadOcrTexts, needReloadPlugin bool
	upsertCodePluginSet := hashset.New() // 插件代码变更 data/plugins/
	upsertDataPluginSet := hashset.New() // 插件存储数据变更 data/storage/petal/
	needUnindexBoxes, needIndexBoxes := map[string]bool{}, map[string]bool{}
	for _, file := range mergeResult.Upserts {
		upserts = append(upserts, file.Path)
		if strings.HasPrefix(file.Path, "/storage/riff/") {
			needReloadFlashcard = true
		}

		if strings.HasPrefix(file.Path, "/assets/ocr-texts.json") {
			needReloadOcrTexts = true
		}

		if strings.HasSuffix(file.Path, "/.siyuan/conf.json") {
			needReloadFiletree = true
			boxID := strings.TrimSuffix(strings.TrimPrefix(file.Path, "/"), "/.siyuan/conf.json")
			needUnindexBoxes[boxID] = true
			needIndexBoxes[boxID] = true
		}

		if strings.HasPrefix(file.Path, "/storage/petal/") {
			needReloadPlugin = true
			if parts := strings.Split(file.Path, "/"); 3 < len(parts) {
				if pluginName := parts[3]; "petals.json" != pluginName {
					upsertDataPluginSet.Add(pluginName)
				}
			}
		}

		if strings.HasPrefix(file.Path, "/plugins/") {
			if parts := strings.Split(file.Path, "/"); 2 < len(parts) {
				needReloadPlugin = true
				upsertCodePluginSet.Add(parts[2])
			}
		}

		if util.HasDocExt(file.Path) {
			upsertTrees++
		}
	}

	removeWidgetDirSet, removePluginSet := hashset.New(), hashset.New()
	for _, file := range mergeResult.Removes {
		removes = append(removes, file.Path)
		if strings.HasPrefix(file.Path, "/storage/riff/") {
			needReloadFlashcard = true
		}

		if strings.HasPrefix(file.Path, "/assets/ocr-texts.json") {
			needReloadOcrTexts = true
		}

		if strings.HasSuffix(file.Path, "/.siyuan/conf.json") {
			needReloadFiletree = true
			boxID := strings.TrimSuffix(strings.TrimPrefix(file.Path, "/"), "/.siyuan/conf.json")
			needUnindexBoxes[boxID] = true
		}

		if strings.HasPrefix(file.Path, "/storage/petal/") {
			needReloadPlugin = true
			if parts := strings.Split(file.Path, "/"); 3 < len(parts) {
				if pluginName := parts[3]; "petals.json" != pluginName {
					upsertDataPluginSet.Add(pluginName)
				}
			}
		}

		if strings.HasPrefix(file.Path, "/plugins/") {
			if parts := strings.Split(file.Path, "/"); 2 < len(parts) {
				needReloadPlugin = true
				removePluginSet.Add(parts[2])
			}
		}

		if strings.HasPrefix(file.Path, "/widgets/") {
			if parts := strings.Split(file.Path, "/"); 2 < len(parts) {
				removeWidgetDirSet.Add(parts[2])
			}
		}
	}

	for _, upsertPetal := range mergeResult.UpsertPetals {
		needReloadPlugin = true
		upsertCodePluginSet.Add(upsertPetal)
	}
	for _, removePetal := range mergeResult.RemovePetals {
		needReloadPlugin = true
		removePluginSet.Add(removePetal)
	}

	if needReloadFlashcard {
		LoadFlashcards()
	}

	if needReloadOcrTexts {
		util.LoadAssetsTexts()
	}

	if needReloadPlugin {
		if repoOwner != Conf {
			return conf.ErrBusinessUnbound
		}
		if err = PushReloadPluginContext(ctx, upsertCodePluginSet, upsertDataPluginSet, removePluginSet, ""); err != nil {
			return err
		}
	}

	for _, widgetDir := range removeWidgetDirSet.Values() {
		widgetDirPath := filepath.Join(util.DataDir, "widgets", widgetDir.(string))
		gulu.File.RemoveEmptyDirs(widgetDirPath)
	}

	Conf.syncRuntime.files.Clear()
	Conf.syncRuntime.storages.Store(false)

	if needFullReindex(upsertTrees) { // 改进同步后全量重建索引判断 https://github.com/siyuan-note/siyuan/issues/5764
		if err = reindexAfterCheckout(ctx); err != nil {
			return
		}
		util.ReloadUI()
		return
	}

	if exit { // 退出时同步不用推送事件
		return
	}

	var indexOwner *indexRepairContext
	if len(needUnindexBoxes) > 0 || len(needIndexBoxes) > 0 {
		indexOwner, err = captureIndexRepair(ctx)
		if err != nil {
			return err
		}
	}
	for boxID := range needUnindexBoxes {
		if box := Conf.GetBox(boxID); nil != box {
			if e := indexOwner.unindexNotebook(ctx, boxID); e != nil {
				return e
			}
		}
	}
	for boxID := range needIndexBoxes {
		if box := Conf.GetBox(boxID); nil != box {
			if e := indexOwner.indexNotebook(ctx, boxID); e != nil {
				return e
			}
			if e := indexOwner.indexReferences(ctx); e != nil {
				return e
			}
		}
	}

	needReloadUI := 0 < len(needUnindexBoxes) || 0 < len(needIndexBoxes)
	if needReloadUI {
		util.ReloadUI()
	}

	util.IncBootProgress(3, "Sync reindexing...")
	indexed, e := incrementalReindexContext(ctx, upserts, removes)
	if e != nil {
		return e
	}
	upsertRootIDs, removeRootIDs := indexed.UpsertRootIDs, indexed.RemoveRootIDs
	needReloadFiletree = !needReloadUI && (needReloadFiletree || 0 < len(upsertRootIDs) || 0 < len(removeRootIDs))
	if needReloadFiletree {
		ReloadFiletree()
	}

	owner := Conf
	owner.scheduleSyncUI(func(life context.Context) {

		if 0 < len(upsertRootIDs) || 0 < len(removeRootIDs) {
			owner.publishSyncUI(life, func() {
				util.BroadcastByType("main", "syncMergeResult", 0, "",
					map[string]interface{}{"upsertRootIDs": upsertRootIDs, "removeRootIDs": removeRootIDs})
			})
		}

		timer := time.NewTimer(2 * time.Second)
		defer timer.Stop()
		select {
		case <-life.Done():
			return
		case <-timer.C:
		}
		owner.publishSyncUI(life, func() { util.PushStatusBar(fmt.Sprintf(owner.Language(149), elapsed.Seconds())) })

		if 0 < len(mergeResult.Conflicts) {
			syConflict := false
			for _, file := range mergeResult.Conflicts {
				if util.HasDocExt(file.Path) {
					syConflict = true
					break
				}
			}

			if syConflict {
				// 数据同步发生冲突时在界面上进行提醒 https://github.com/siyuan-note/siyuan/issues/7332
				owner.publishSyncUI(life, func() { util.PushMsg(owner.Language(108), 7000) })
			}
		}
	})
	return
}

func logSyncMergeResult(mergeResult *dejavu.MergeResult) {
	if 1 > len(mergeResult.Conflicts) && 1 > len(mergeResult.Upserts) && 1 > len(mergeResult.Removes) {
		return
	}

	if 0 < len(mergeResult.Conflicts) {
		logBuilder := bytes.Buffer{}
		for i, f := range mergeResult.Conflicts {
			logBuilder.WriteString("  ")
			logBuilder.WriteString(f.Path)
			if i < len(mergeResult.Conflicts)-1 {
				logBuilder.WriteString("\n")
			}
		}
		logging.LogInfof("sync conflicts:\n%s", logBuilder.String())
	}
	if 0 < len(mergeResult.Upserts) {
		logBuilder := bytes.Buffer{}
		for i, f := range mergeResult.Upserts {
			logBuilder.WriteString("  ")
			logBuilder.WriteString(f.Path)
			if i < len(mergeResult.Upserts)-1 {
				logBuilder.WriteString("\n")
			}
		}
		logging.LogInfof("sync merge upserts:\n%s", logBuilder.String())
	}
	if 0 < len(mergeResult.Removes) {
		logBuilder := bytes.Buffer{}
		for i, f := range mergeResult.Removes {
			logBuilder.WriteString("  ")
			logBuilder.WriteString(f.Path)
			if i < len(mergeResult.Removes)-1 {
				logBuilder.WriteString("\n")
			}
		}
		logging.LogInfof("sync merge removes:\n%s", logBuilder.String())
	}
}

func needFullReindex(upsertTrees int) bool {
	return 0.2 < float64(upsertTrees)/float64(treenode.CountTrees())
}

func indexRepoBeforeCloudSync(ctx context.Context, repo *dejavu.Repo) (beforeIndex, afterIndex *entity.Index, err error) {
	start := time.Now()

	beforeIndex, _ = repo.Latest()
	if err = FlushTxQueueContext(ctx); err != nil {
		return
	}

	// Use the owned original-codec indexer: a same-second edit must be backed
	// up before an authoritative download can replace the source.
	_, ui, err := captureLegacyRepoEventPayload(ctx, eventbus.CtxPushMsgToStatusBar)
	if err != nil {
		return beforeIndex, nil, err
	}
	_, err = repo.LocalRepoContext(ctx, dejavu.LocalRepoRequest{Kind: "index", Memo: "[Sync] Cloud sync"}, dejavu.SnapshotOptions{Event: func(_ context.Context, topic string, args ...interface{}) {
		if len(args) > 0 {
			args[0] = ui
		}
		eventbus.Publish(topic, args...)
	}}, nil)
	if err == nil {
		afterIndex, err = repo.Latest()
	}

	if err != nil {
		logging.LogErrorf("index data repo before cloud sync failed: %s", err)
		return
	}
	elapsed := time.Since(start)

	if nil == beforeIndex || beforeIndex.ID != afterIndex.ID {
		// 对新创建的快照需要更新备注，加入耗时统计
		afterIndex.Memo = fmt.Sprintf("[Sync] Cloud sync, completed in %.2fs", elapsed.Seconds())
		if err = repo.PutIndex(afterIndex); err != nil {
			util.PushStatusBar("Save data snapshot for cloud sync failed")
			logging.LogErrorf("put index into data repo before cloud sync failed: %s", err)
			return
		}
		util.PushStatusBar(fmt.Sprintf(Conf.Language(147), elapsed.Seconds()))
	} else {
		util.PushStatusBar(fmt.Sprintf(Conf.Language(148), elapsed.Seconds()))
	}

	if Conf.Repo.SyncIndexTiming < elapsed.Milliseconds() {
		logging.LogWarnf("index data repo before cloud sync elapsed [%dms]", elapsed.Milliseconds())
		if count, e := repo.CountIndexes(); e == nil {
			Conf.scheduleIndexNotice(count, metadataRepoPaths{Data: util.DataDir, Repo: util.RepoDir, History: util.HistoryDir, Temp: util.TempDir, Workspace: util.WorkspaceDir})
		}
	}
	return
}

func newRepository() (*dejavu.Repo, error) { return newRepositoryContext(nil) }
func newRepositoryContext(ctx context.Context) (ret *dejavu.Repo, err error) {
	owner := Conf
	owner.m.Lock()
	if owner.Repo == nil {
		owner.m.Unlock()
		return nil, errRepoConfigurationChanged
	}
	capturedKey := append([]byte{}, owner.Repo.Key...)
	owner.m.Unlock()
	workspace, data, repoPath, history, temp := util.WorkspaceDir, util.DataDir, util.RepoDir, util.HistoryDir, util.TempDir
	cloudConf, err := buildCloudConf()
	if err != nil {
		return
	}
	if ctx != nil && owner.Sync.Provider == conf.ProviderLocal {
		if e := validateLocalDirectoryTarget(owner.AppData, metadataRepoPaths{Data: data, Repo: repoPath, History: history, Temp: temp, Workspace: workspace}); e != nil {
			return nil, e
		}
	}

	cloudConf.UserAgent = util.UserAgent
	cloudConf.RequestContext = ctx
	var cloudRepo cloud.Cloud
	var oldTransports []*http.Transport
	switch Conf.Sync.Provider {
	case conf.ProviderSiYuan:
		cloudRepo = cloud.NewSiYuan(&cloud.BaseCloud{Conf: cloudConf})
	case conf.ProviderS3:
		transport := httpclient.NewTransport(cloudConf.S3.SkipTlsVerify)
		oldTransports = append(oldTransports, transport)
		s3HTTPClient := &http.Client{Transport: transport}
		s3HTTPClient.Timeout = time.Duration(cloudConf.S3.Timeout) * time.Second
		cloudRepo = cloud.NewS3(&cloud.BaseCloud{Conf: cloudConf}, s3HTTPClient)
	case conf.ProviderWebDAV:
		webdavClient := gowebdav.NewClient(cloudConf.WebDAV.Endpoint, cloudConf.WebDAV.Username, cloudConf.WebDAV.Password)
		a := cloudConf.WebDAV.Username + ":" + cloudConf.WebDAV.Password
		auth := "Basic " + base64.StdEncoding.EncodeToString([]byte(a))
		webdavClient.SetHeader("Authorization", auth)
		webdavClient.SetHeader("User-Agent", util.UserAgent)
		webdavClient.SetTimeout(time.Duration(cloudConf.WebDAV.Timeout) * time.Second)
		transport := httpclient.NewTransport(cloudConf.WebDAV.SkipTlsVerify)
		oldTransports = append(oldTransports, transport)
		webdavClient.SetTransport(transport)
		cloudRepo = cloud.NewWebDAV(&cloud.BaseCloud{Conf: cloudConf}, webdavClient)
	case conf.ProviderLocal:
		cloudRepo = cloud.NewLocal(&cloud.BaseCloud{Conf: cloudConf})
	default:
		err = fmt.Errorf("unknown cloud provider [%d]", Conf.Sync.Provider)
		return
	}

	var providerClose func()
	if ctx != nil {
		cloudRepo, providerClose, err = cloud.OpenContext(cloudRepo, ctx)
		for _, transport := range oldTransports {
			transport.CloseIdleConnections()
		}
		if err != nil {
			return nil, err
		}
	}

	ignoreLines := getSyncIgnoreLines()
	ignoreLines = append(ignoreLines, "/.siyuan/conf.json") // 忽略旧版同步配置
	ret, err = dejavu.NewRepo(data, repoPath, history, temp, owner.System.ID, owner.System.Name, owner.System.OS, capturedKey, ignoreLines, cloudRepo)
	if err != nil {
		logging.LogErrorf("init data repo failed: %s", err)
		if providerClose != nil {
			providerClose()
		}
		return
	}
	ret.BindProviderCleanup(providerClose)
	ret.BindAdmission(func() error {
		owner.m.Lock()
		defer owner.m.Unlock()
		if owner != Conf || workspace != util.WorkspaceDir || data != util.DataDir || repoPath != util.RepoDir || history != util.HistoryDir || temp != util.TempDir || owner.Repo == nil || !bytes.Equal(capturedKey, owner.Repo.Key) {
			return errRepoConfigurationChanged
		}
		return nil
	})
	return
}

func subscribeRepoEvents() {
	// Repository subscriptions persist across workspace hot switches. Wrap every
	// context-bearing callback with the operation token gate instead of reading
	// whichever workspace happens to be selected when the callback arrives.
	subscribeLegacyEvent := subscribeLegacyRepoEvent
	subscribeLegacyEvent(eventbus.EvtIndexBeforeWalkData, func(context map[string]interface{}, path string) {
		msg := fmt.Sprintf(legacyRepoEventLanguage(context, 158, true), path)
		util.SetBootDetails(msg)
		util.ContextPushMsg(context, msg)
	})

	subscribeLegacyEvent(eventbus.EvtIndexWalkData, func(context map[string]interface{}, path string) {
		msg := fmt.Sprintf(legacyRepoEventLanguage(context, 158, true), filepath.Base(path))
		if 0 == legacyRepoEventCount(context, "index-walk")%1024 {
			util.SetBootDetails(msg)
			util.ContextPushMsg(context, msg)
		}
	})
	subscribeLegacyEvent(eventbus.EvtIndexBeforeGetLatestFiles, func(context map[string]interface{}, total int) {
		msg := fmt.Sprintf(legacyRepoEventLanguage(context, 159, true), 0, total)
		util.SetBootDetails(msg)
		util.ContextPushMsg(context, msg)
	})

	subscribeLegacyEvent(eventbus.EvtIndexGetLatestFile, func(context map[string]interface{}, count int, total int) {
		msg := fmt.Sprintf(legacyRepoEventLanguage(context, 159, true), count, total)
		if 0 == count%64 {
			util.SetBootDetails(msg)
			util.ContextPushMsg(context, msg)
		}
	})
	subscribeLegacyEvent(eventbus.EvtIndexUpsertFiles, func(context map[string]interface{}, total int) {
		msg := fmt.Sprintf(legacyRepoEventLanguage(context, 160, true), 0, total)
		util.SetBootDetails(msg)
		util.ContextPushMsg(context, msg)
	})
	subscribeLegacyEvent(eventbus.EvtIndexUpsertFile, func(context map[string]interface{}, count int, total int) {
		msg := fmt.Sprintf(legacyRepoEventLanguage(context, 160, true), count, total)
		if 0 == count%32 {
			util.SetBootDetails(msg)
			util.ContextPushMsg(context, msg)
		}
	})

	subscribeLegacyEvent(eventbus.EvtCheckoutBeforeWalkData, func(context map[string]interface{}, path string) {
		msg := fmt.Sprintf(legacyRepoEventLanguage(context, 161, true), path)
		util.SetBootDetails(msg)
		util.ContextPushMsg(context, msg)
	})
	subscribeLegacyEvent(eventbus.EvtCheckoutWalkData, func(context map[string]interface{}, path string) {
		msg := fmt.Sprintf(legacyRepoEventLanguage(context, 161, true), filepath.Base(path))
		if 0 == legacyRepoEventCount(context, "checkout-walk")%512 {
			util.SetBootDetails(msg)
			util.ContextPushMsg(context, msg)
		}
	})
	subscribeLegacyEvent(eventbus.EvtCheckoutUpsertFiles, func(context map[string]interface{}, total int) {
		msg := fmt.Sprintf(legacyRepoEventLanguage(context, 162, true), 0, total)
		util.SetBootDetails(msg)
		legacyRepoSetProgressPart(context, total)
		util.ContextPushMsg(context, msg)
	})
	subscribeLegacyEvent(eventbus.EvtCheckoutUpsertFile, func(context map[string]interface{}, count, total int) {
		msg := fmt.Sprintf(legacyRepoEventLanguage(context, 162, true), count, total)
		util.IncBootProgress(legacyRepoEventProgressPart(context), msg)
		if 0 == legacyRepoEventCount(context, "checkout-upsert")%32 {
			util.ContextPushMsg(context, msg)
		}
	})
	subscribeLegacyEvent(eventbus.EvtCheckoutRemoveFiles, func(context map[string]interface{}, total int) {
		msg := fmt.Sprintf(legacyRepoEventLanguage(context, 163, true), 0, total)
		util.SetBootDetails(msg)
		legacyRepoSetProgressPart(context, total)
		util.ContextPushMsg(context, msg)
	})

	subscribeLegacyEvent(eventbus.EvtCheckoutRemoveFile, func(context map[string]interface{}, count, total int) {
		msg := fmt.Sprintf(legacyRepoEventLanguage(context, 163, true), count, total)
		util.IncBootProgress(legacyRepoEventProgressPart(context), msg)
		if 0 == count%64 {
			util.ContextPushMsg(context, msg)
		}
	})

	subscribeLegacyEvent(eventbus.EvtCloudBeforeDownloadIndex, func(context map[string]interface{}, id string) {
		msg := fmt.Sprintf(legacyRepoEventLanguage(context, 164, true), legacyRepoShortID(id))
		util.IncBootProgress(1, msg)
		util.ContextPushMsg(context, msg)
	})

	subscribeLegacyEvent(eventbus.EvtCloudBeforeDownloadFiles, func(context map[string]interface{}, total int) {
		msg := fmt.Sprintf(legacyRepoEventLanguage(context, 165, true), 0, total)
		util.SetBootDetails(msg)
		legacyRepoSetProgressPart(context, total)
		util.ContextPushMsg(context, msg)
	})

	subscribeLegacyEvent(eventbus.EvtCloudBeforeDownloadFile, func(context map[string]interface{}, count, total int) {
		msg := fmt.Sprintf(legacyRepoEventLanguage(context, 165, true), count, total)
		util.IncBootProgress(legacyRepoEventProgressPart(context), msg)
		if 0 == count%8 {
			util.ContextPushMsg(context, msg)
		}
	})
	subscribeLegacyEvent(eventbus.EvtCloudBeforeDownloadChunks, func(context map[string]interface{}, total int) {
		msg := fmt.Sprintf(legacyRepoEventLanguage(context, 166, true), 0, total)
		util.SetBootDetails(msg)
		legacyRepoSetProgressPart(context, total)
		util.ContextPushMsg(context, msg)
	})
	subscribeLegacyEvent(eventbus.EvtCloudBeforeDownloadChunk, func(context map[string]interface{}, count, total int) {
		msg := fmt.Sprintf(legacyRepoEventLanguage(context, 166, true), count, total)
		util.IncBootProgress(legacyRepoEventProgressPart(context), msg)
		if 0 == count%8 {
			util.ContextPushMsg(context, msg)
		}
	})
	subscribeLegacyEvent(eventbus.EvtCloudBeforeDownloadRef, func(context map[string]interface{}, ref string) {
		msg := fmt.Sprintf(legacyRepoEventLanguage(context, 167, true), ref)
		util.IncBootProgress(1, msg)
		util.ContextPushMsg(context, msg)
	})
	subscribeLegacyEvent(eventbus.EvtCloudBeforeUploadIndex, func(context map[string]interface{}, id string) {
		msg := fmt.Sprintf(legacyRepoEventLanguage(context, 168, true), legacyRepoShortID(id))
		util.IncBootProgress(1, msg)
		util.ContextPushMsg(context, msg)
	})
	subscribeLegacyEvent(eventbus.EvtCloudBeforeUploadFiles, func(context map[string]interface{}, total int) {
		msg := fmt.Sprintf(legacyRepoEventLanguage(context, 169, true), 0, total)
		util.SetBootDetails(msg)
		util.ContextPushMsg(context, msg)
	})
	subscribeLegacyEvent(eventbus.EvtCloudBeforeUploadFile, func(context map[string]interface{}, count, total int) {
		msg := fmt.Sprintf(legacyRepoEventLanguage(context, 169, true), count, total)
		if 0 == count%8 {
			util.SetBootDetails(msg)
			util.ContextPushMsg(context, msg)
		}
	})
	subscribeLegacyEvent(eventbus.EvtCloudBeforeUploadChunks, func(context map[string]interface{}, total int) {
		msg := fmt.Sprintf(legacyRepoEventLanguage(context, 170, true), 0, total)
		util.SetBootDetails(msg)
		util.ContextPushMsg(context, msg)
	})
	subscribeLegacyEvent(eventbus.EvtCloudBeforeUploadChunk, func(context map[string]interface{}, count, total int) {
		msg := fmt.Sprintf(legacyRepoEventLanguage(context, 170, true), count, total)
		if 0 == count%8 {
			util.SetBootDetails(msg)
			util.ContextPushMsg(context, msg)
		}
	})
	subscribeLegacyEvent(eventbus.EvtCloudBeforeUploadRef, func(context map[string]interface{}, ref string) {
		msg := fmt.Sprintf(legacyRepoEventLanguage(context, 171, true), ref)
		util.SetBootDetails(msg)
		util.ContextPushMsg(context, msg)
	})
	subscribeLegacyEvent(eventbus.EvtCloudLock, func(context map[string]interface{}) {
		msg := legacyRepoEventLanguage(context, 186, true)
		util.SetBootDetails(msg)
		util.ContextPushMsg(context, msg)
	})
	subscribeLegacyEvent(eventbus.EvtCloudUnlock, func(context map[string]interface{}) {
		msg := legacyRepoEventLanguage(context, 187, true)
		util.SetBootDetails(msg)
		util.ContextPushMsg(context, msg)
	})
	subscribeLegacyEvent(eventbus.EvtCloudBeforeUploadIndexes, func(context map[string]interface{}) {
		msg := legacyRepoEventLanguage(context, 208, true)
		util.SetBootDetails(msg)
		util.ContextPushMsg(context, msg)
	})
	subscribeLegacyEvent(eventbus.EvtCloudBeforeUploadCheckIndex, func(context map[string]interface{}) {
		msg := legacyRepoEventLanguage(context, 209, true)
		util.SetBootDetails(msg)
		util.ContextPushMsg(context, msg)
	})
	subscribeLegacyEvent(eventbus.EvtCloudBeforeFixObjects, func(context map[string]interface{}, count, total int) {
		msg := fmt.Sprintf(legacyRepoEventLanguage(context, 210, true), count, total)
		util.SetBootDetails(msg)
		util.ContextPushMsg(context, msg)
	})
	subscribeLegacyEvent(eventbus.EvtCloudAfterFixObjects, func(context map[string]interface{}) {
		msg := legacyRepoEventLanguage(context, 211, true)
		util.SetBootDetails(msg)
		util.ContextPushMsg(context, msg)
	})
	subscribeActiveLegacyRepoEvent(eventbus.EvtCloudCorrupted, func(session *legacyRepoEventSession) {
		util.PushErrMsg(session.language(220, false), 30000)
	})
	subscribeLegacyEvent(eventbus.EvtCloudPurgeListObjects, func(context map[string]interface{}) {
		util.ContextPushMsg(context, legacyRepoEventLanguage(context, 224, false))
	})
	subscribeLegacyEvent(eventbus.EvtCloudPurgeListIndexes, func(context map[string]interface{}) {
		util.ContextPushMsg(context, legacyRepoEventLanguage(context, 225, false))
	})
	subscribeLegacyEvent(eventbus.EvtCloudPurgeListRefs, func(context map[string]interface{}) {
		util.ContextPushMsg(context, legacyRepoEventLanguage(context, 226, false))
	})
	subscribeLegacyEvent(eventbus.EvtCloudPurgeDownloadIndexes, func(context map[string]interface{}) {
		util.ContextPushMsg(context, legacyRepoEventLanguage(context, 227, false))
	})
	subscribeLegacyEvent(eventbus.EvtCloudPurgeDownloadFiles, func(context map[string]interface{}) {
		util.ContextPushMsg(context, legacyRepoEventLanguage(context, 228, false))
	})
	subscribeLegacyEvent(eventbus.EvtCloudPurgeRemoveIndexes, func(context map[string]interface{}) {
		util.ContextPushMsg(context, legacyRepoEventLanguage(context, 229, false))
	})
	subscribeLegacyEvent(eventbus.EvtCloudPurgeRemoveIndexesV2, func(context map[string]interface{}) {
		util.ContextPushMsg(context, legacyRepoEventLanguage(context, 230, false))
	})
	subscribeLegacyEvent(eventbus.EvtCloudPurgeRemoveObjects, func(context map[string]interface{}) {
		util.ContextPushMsg(context, legacyRepoEventLanguage(context, 231, false))
	})
}

func buildCloudConf() (ret *cloud.Conf, err error) {
	if !cloud.IsValidCloudDirName(Conf.Sync.CloudName) {
		logging.LogWarnf("invalid cloud repo name, rename it to [main]")
		Conf.Sync.CloudName = "main"
		Conf.Save()
	}

	userId, token, availableSize := "0", "", int64(1024*1024*1024*1024*2)
	if nil != Conf.User && conf.ProviderSiYuan == Conf.Sync.Provider {
		u := Conf.GetUser()
		userId = u.UserId
		token = u.UserToken
		availableSize = u.GetCloudRepoAvailableSize()
	}

	ret = &cloud.Conf{
		Dir:           Conf.Sync.CloudName,
		UserID:        userId,
		Token:         token,
		AvailableSize: availableSize,
		Server:        util.GetCloudServer(),
	}

	switch Conf.Sync.Provider {
	case conf.ProviderSiYuan:
		ret.Endpoint = util.GetCloudSyncServer()
	case conf.ProviderS3:
		ret.S3 = &cloud.ConfS3{
			Endpoint:       Conf.Sync.S3.Endpoint,
			AccessKey:      Conf.Sync.S3.AccessKey,
			SecretKey:      Conf.Sync.S3.SecretKey,
			Bucket:         Conf.Sync.S3.Bucket,
			Region:         Conf.Sync.S3.Region,
			PathStyle:      Conf.Sync.S3.PathStyle,
			SkipTlsVerify:  Conf.Sync.S3.SkipTlsVerify,
			Timeout:        Conf.Sync.S3.Timeout,
			ConcurrentReqs: Conf.Sync.S3.ConcurrentReqs,
		}
	case conf.ProviderWebDAV:
		ret.WebDAV = &cloud.ConfWebDAV{
			Endpoint:       Conf.Sync.WebDAV.Endpoint,
			Username:       Conf.Sync.WebDAV.Username,
			Password:       Conf.Sync.WebDAV.Password,
			SkipTlsVerify:  Conf.Sync.WebDAV.SkipTlsVerify,
			Timeout:        Conf.Sync.WebDAV.Timeout,
			ConcurrentReqs: Conf.Sync.WebDAV.ConcurrentReqs,
		}
	case conf.ProviderLocal:
		ret.Local = &cloud.ConfLocal{
			Endpoint:       Conf.Sync.Local.Endpoint,
			Timeout:        Conf.Sync.Local.Timeout,
			ConcurrentReqs: Conf.Sync.Local.ConcurrentReqs,
		}
	default:
		err = fmt.Errorf("invalid provider [%d]", Conf.Sync.Provider)
		return
	}
	return
}

type Backup struct {
	Size    int64  `json:"size"`
	HSize   string `json:"hSize"`
	Updated string `json:"updated"`
	SaveDir string `json:"saveDir"` // 本地备份数据存放目录路径
}

type Sync struct {
	Size      int64  `json:"size"`
	HSize     string `json:"hSize"`
	Updated   string `json:"updated"`
	CloudName string `json:"cloudName"` // 云端同步数据存放目录名
	SaveDir   string `json:"saveDir"`   // 本地同步数据存放目录路径
}

func GetCloudSpace() (s *Sync, b *Backup, hSize, hAssetSize, hTotalSize, hExchangeSize, hTrafficUploadSize, hTrafficDownloadSize, hTrafficAPIGet, hTrafficAPIPut string, err error) {
	out, e := QueryCloudRepoContext(context.Background(), "space", 1)
	if e != nil {
		err = e
		return
	}
	var v cloudSpaceProjection
	err = json.Unmarshal(out.Data, &v)
	return v.Sync, v.Backup, v.HSize, v.HAssetSize, v.HTotalSize, v.HExchangeSize, v.HTrafficUploadSize, v.HTrafficDownloadSize, v.HTrafficAPIGet, v.HTrafficAPIPut, err
}
