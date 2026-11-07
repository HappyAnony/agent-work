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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/88250/gulu"
	"github.com/88250/lute/html"
	"github.com/gorilla/websocket"
	"github.com/siyuan-note/dejavu"
	"github.com/siyuan-note/dejavu/cloud"
	"github.com/siyuan-note/logging"
	"wiki-kernel/conf"
	"wiki-kernel/internal/cloudrepo"
	"wiki-kernel/internal/taskgroup"
	"wiki-kernel/util"
)

func legacySyncDataDownload(ctx context.Context) (err error) {
	defer recoverRepositoryWorkflow(&err, "full_sync")

	if !checkSync(false, false, true) {
		return
	}

	util.BroadcastByType("main", "syncing", 0, Conf.Language(81), nil)
	if !isProviderOnline(true) { // 这个操作比较耗时，所以要先推送 syncing 事件后再判断网络，这样才能给用户更即时的反馈
		util.BroadcastByType("main", "syncing", 2, Conf.Language(28), nil)
		return cloudrepo.ErrProvider
	}

	lockSync()
	defer unlockSync()

	now := util.CurrentTimeMillis()
	Conf.Sync.Synced = now

	err = syncRepoDownload(ctx)
	code := 1
	if err != nil {
		code = 2
	}
	util.BroadcastByType("main", "syncing", code, Conf.Sync.Stat, nil)
	return
}

func legacySyncDataUpload(ctx context.Context) (err error) {
	defer recoverRepositoryWorkflow(&err, "full_sync")

	if !checkSync(false, false, true) {
		return
	}

	util.BroadcastByType("main", "syncing", 0, Conf.Language(81), nil)
	if !isProviderOnline(true) { // 这个操作比较耗时，所以要先推送 syncing 事件后再判断网络，这样才能给用户更即时的反馈
		util.BroadcastByType("main", "syncing", 2, Conf.Language(28), nil)
		return cloudrepo.ErrProvider
	}

	lockSync()
	defer unlockSync()

	now := util.CurrentTimeMillis()
	Conf.Sync.Synced = now

	err = syncRepoUpload(ctx)
	code := 1
	if err != nil {
		code = 2
	}
	util.BroadcastByType("main", "syncing", code, Conf.Sync.Stat, nil)
	return
}

var (
	fixSyncInterval = 5 * time.Minute
)

func SyncDataJob() { logOwnedTaskError("periodic sync", SyncDataJobContext(context.Background())) }

func legacyBootSyncData(ctx context.Context) (err error) {
	defer recoverRepositoryWorkflow(&err, "full_sync")

	if Conf.Sync.Perception {
		connectSyncWebSocketContext(ctx, Conf)
	}

	if !checkSync(true, false, false) {
		return
	}

	if !isProviderOnline(false) {
		Conf.syncRuntime.boot.Store(2)
		util.PushErrMsg(Conf.Language(76), 7000)
		return cloudrepo.ErrProvider
	}

	lockSync()
	defer unlockSync()

	util.IncBootProgress(3, "Syncing data from the cloud...")
	Conf.syncRuntime.boot.Store(1)
	logging.LogInfof("sync before boot")

	now := util.CurrentTimeMillis()
	Conf.Sync.Synced = now
	util.BroadcastByType("main", "syncing", 0, Conf.Language(81), nil)
	err = bootSyncRepo(ctx)
	code := 1
	if err != nil {
		code = 2
	}
	util.BroadcastByType("main", "syncing", code, Conf.Sync.Stat, nil)
	return
}

func SyncData(byHand bool) {
	syncData(false, byHand)
}

func lockSync() {
	Conf.syncRuntime.lock.Lock()
	Conf.syncRuntime.running.Store(true)
}

func unlockSync() {
	Conf.syncRuntime.running.Store(false)
	Conf.syncRuntime.lock.Unlock()
}

func legacySyncData(ctx context.Context, exit, byHand bool) (err error) {
	defer recoverRepositoryWorkflow(&err, "full_sync")

	if !checkSync(false, exit, byHand) {
		return
	}

	lockSync()
	defer unlockSync()

	util.BroadcastByType("main", "syncing", 0, Conf.Language(81), nil)
	if !exit && !isProviderOnline(byHand) { // 这个操作比较耗时，所以要先推送 syncing 事件后再判断网络，这样才能给用户更即时的反馈
		util.BroadcastByType("main", "syncing", 2, Conf.Language(28), nil)
		return cloudrepo.ErrProvider
	}

	if exit {
		Conf.syncRuntime.exit.Store(1)
		logging.LogInfof("sync before exit")
		msgId := util.PushMsg(Conf.Language(81), 1000*60*15)
		defer func() {
			util.PushClearMsg(msgId)
		}()
	}

	now := util.CurrentTimeMillis()
	Conf.Sync.Synced = now

	dataChanged, err := syncRepo(ctx, exit, byHand)
	code := 1
	if err != nil {
		code = 2
	}
	util.BroadcastByType("main", "syncing", code, Conf.Sync.Stat, nil)

	conn := currentSyncWebSocket()
	if nil == conn && Conf.Sync.Perception {
		// 如果 websocket 连接已经断开，则重新连接
		connectSyncWebSocketContext(ctx, Conf)
		conn = currentSyncWebSocket()
	}

	if 1 == Conf.Sync.Mode && nil != conn && Conf.Sync.Perception && dataChanged {
		// 如果处于自动同步模式且不是由 WS 触发的同步，则通知其他设备上的内核进行同步
		request := map[string]interface{}{
			"cmd":    "synced",
			"synced": Conf.Sync.Synced,
		}
		if writeErr := conn.WriteJSON(request); nil != writeErr {
			logging.LogErrorf("write websocket message failed: %v", writeErr)
		}
	}
	return
}

func checkSync(boot, exit, byHand bool) bool {
	if 2 == Conf.Sync.Mode && !boot && !exit && !byHand { // 手动模式下只有启动和退出进行同步
		return false
	}

	if 3 == Conf.Sync.Mode && !byHand { // 完全手动模式下只有手动进行同步
		return false
	}

	if !Conf.Sync.Enabled {
		if byHand {
			util.PushMsg(Conf.Language(124), 5000)
		}
		return false
	}

	if !cloud.IsValidCloudDirName(Conf.Sync.CloudName) {
		if byHand {
			util.PushMsg(Conf.Language(123), 5000)
		}
		return false
	}

	switch Conf.Sync.Provider {
	case conf.ProviderSiYuan:
		if !IsSubscriber() {
			return false
		}
	case conf.ProviderWebDAV, conf.ProviderS3, conf.ProviderLocal:
		if !IsPaidUser() {
			return false
		}
	}

	if 7 < Conf.syncRuntime.errors.Load() && !byHand {
		logging.LogErrorf("failed to auto-sync too many times, delay auto-sync 64 minutes")
		util.PushErrMsg(Conf.Language(125), 1000*60*60)
		Conf.planSyncAfter(64 * time.Minute)
		return false
	}
	return true
}

func SetCloudSyncDir(name string) {
	owner := Conf
	if _, e := ChangeCloudDirectoryContext(context.Background(), "set", name); e != nil {
		owner.notifyRepo(context.Background(), e.Error(), 5000)
	}
}

func setLegacySyncValue(ctx context.Context, kind string, value interface{}, output interface{}) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	normalized, err := SetSyncSettingContext(ctx, kind, raw)
	if err != nil {
		return err
	}
	if output != nil {
		return json.Unmarshal(normalized, output)
	}
	return nil
}
func SetSyncGenerateConflictDoc(v bool) {
	logOwnedTaskError("sync setting", setLegacySyncValue(context.Background(), "syncConflict", v, nil))
}
func SetSyncEnable(v bool) {
	logOwnedTaskError("sync setting", setLegacySyncValue(context.Background(), "syncEnabled", v, nil))
}
func SetSyncInterval(v int) {
	logOwnedTaskError("sync setting", setLegacySyncValue(context.Background(), "syncInterval", v, nil))
}
func SetSyncMode(v int) {
	logOwnedTaskError("sync setting", setLegacySyncValue(context.Background(), "syncMode", v, nil))
}
func SetSyncPerception(v bool) {
	logOwnedTaskError("sync perception setting", SetSyncPerceptionContext(context.Background(), v))
}
func SetSyncPerceptionContext(ctx context.Context, v bool) error {
	return setLegacySyncValue(ctx, "syncPerception", v, nil)
}
func SetSyncProvider(v int) error {
	return setLegacySyncValue(context.Background(), "syncProvider", v, nil)
}
func SetSyncProviderS3(v *conf.S3) error {
	return setLegacySyncValue(context.Background(), "syncS3", v, v)
}
func SetSyncProviderWebDAV(v *conf.WebDAV) error {
	return setLegacySyncValue(context.Background(), "syncWebDAV", v, v)
}
func SetSyncProviderLocal(v *conf.Local) error {
	return setLegacySyncValue(context.Background(), "syncLocal", v, v)
}

func CreateCloudSyncDir(name string) (err error) {
	_, err = ChangeCloudDirectoryContext(context.Background(), "create", name)
	return
}

func RemoveCloudSyncDir(name string) (err error) {
	_, err = ChangeCloudDirectoryContext(context.Background(), "remove", name)
	return
}

func ListCloudSyncDir() (syncDirs []*Sync, hSize string, err error) {
	out, e := QueryCloudRepoContext(context.Background(), "repos", 1)
	if e != nil {
		return []*Sync{}, "", e
	}
	var v struct {
		SyncDirs []*Sync `json:"syncDirs"`
		HSize    string  `json:"hSize"`
	}
	e = json.Unmarshal(out.Data, &v)
	return v.SyncDirs, v.HSize, e
}

func formatRepoErrorMsg(err error) string {
	msg := html.EscapeString(err.Error())
	if errors.Is(err, cloud.ErrCloudAuthFailed) {
		msg = Conf.Language(31)
	} else if errors.Is(err, cloud.ErrCloudObjectNotFound) {
		msg = Conf.Language(129)
	} else if errors.Is(err, dejavu.ErrLockCloudFailed) {
		msg = Conf.Language(188)
	} else if errors.Is(err, dejavu.ErrCloudLocked) {
		msg = Conf.Language(189)
	} else if errors.Is(err, dejavu.ErrRepoFatal) {
		msg = Conf.Language(23)
	} else if errors.Is(err, cloud.ErrSystemTimeIncorrect) {
		msg = Conf.Language(195)
	} else if errors.Is(err, cloud.ErrDeprecatedVersion) {
		msg = Conf.Language(212)
	} else if errors.Is(err, cloud.ErrCloudCheckFailed) {
		msg = Conf.Language(213)
	} else if errors.Is(err, cloud.ErrCloudServiceUnavailable) {
		msg = Conf.language(219)
	} else if errors.Is(err, cloud.ErrCloudForbidden) {
		msg = Conf.language(249)
	} else if errors.Is(err, cloud.ErrCloudTooManyRequests) {
		msg = Conf.language(250)
	} else {
		logging.LogErrorf("sync failed caused by network: %s", msg)
		msgLowerCase := strings.ToLower(msg)
		if strings.Contains(msgLowerCase, "permission denied") || strings.Contains(msg, "access is denied") {
			msg = Conf.Language(33)
		} else if strings.Contains(msgLowerCase, "region was not a valid") {
			msg = Conf.language(254)
		} else if strings.Contains(msgLowerCase, "device or resource busy") || strings.Contains(msg, "is being used by another") {
			msg = fmt.Sprintf(Conf.Language(85), err)
		} else if strings.Contains(msgLowerCase, "cipher: message authentication failed") {
			msg = Conf.Language(135)
		} else if strings.Contains(msgLowerCase, "no such host") || strings.Contains(msgLowerCase, "connection failed") || strings.Contains(msgLowerCase, "hostname resolution") || strings.Contains(msgLowerCase, "No address associated with hostname") {
			msg = Conf.Language(24)
		} else if strings.Contains(msgLowerCase, "net/http: request canceled while waiting for connection") || strings.Contains(msgLowerCase, "exceeded while awaiting") || strings.Contains(msgLowerCase, "context deadline exceeded") || strings.Contains(msgLowerCase, "timeout") || strings.Contains(msgLowerCase, "context cancellation while reading body") {
			msg = Conf.Language(24)
		} else if strings.Contains(msgLowerCase, "connection") || strings.Contains(msgLowerCase, "refused") || strings.Contains(msgLowerCase, "socket") || strings.Contains(msgLowerCase, "eof") || strings.Contains(msgLowerCase, "closed") || strings.Contains(msgLowerCase, "network") {
			msg = Conf.Language(28)
		}
	}
	msg += " (Provider: " + conf.ProviderToStr(Conf.Sync.Provider) + ")"
	return msg
}

func getSyncIgnoreLines() (ret []string) {
	ignore := filepath.Join(util.DataDir, ".siyuan", "syncignore")
	err := os.MkdirAll(filepath.Dir(ignore), 0755)
	if err != nil {
		return
	}
	if !gulu.File.IsExist(ignore) {
		if err = gulu.File.WriteFileSafer(ignore, nil, 0644); err != nil {
			logging.LogErrorf("create syncignore [%s] failed: %s", ignore, err)
			return
		}
	}
	data, err := os.ReadFile(ignore)
	if err != nil {
		logging.LogErrorf("read syncignore [%s] failed: %s", ignore, err)
		return
	}
	dataStr := string(data)
	dataStr = strings.ReplaceAll(dataStr, "\r\n", "\n")
	ret = strings.Split(dataStr, "\n")

	// 默认忽略帮助文档
	ret = append(ret, "20210808180117-6v0mkxr/**/*")
	ret = append(ret, "20210808180117-czj9bvb/**/*")
	ret = append(ret, "20211226090932-5lcq56f/**/*")
	ret = append(ret, "20240530133126-axarxgx/**/*")

	ret = gulu.Str.RemoveDuplicatedElem(ret)
	return
}

func IncSync() {
	_ = IncSyncContext(context.Background())
}

func planSyncAfter(d time.Duration) {
	Conf.planSyncAfter(d)
}

func (c *AppConf) planSyncAfter(d time.Duration) {
	if c == nil {
		return
	}
	c.syncRuntime.planMu.Lock()
	c.syncRuntime.plan = time.Now().Add(d)
	c.syncRuntime.planMu.Unlock()
}

func isProviderOnline(byHand bool) (ret bool) {
	owner := Conf
	var checkURL string
	skipTlsVerify := false
	timeout := 3000
	switch Conf.Sync.Provider {
	case conf.ProviderSiYuan:
		checkURL = util.GetCloudSyncServer()
	case conf.ProviderS3:
		checkURL = Conf.Sync.S3.Endpoint
		skipTlsVerify = Conf.Sync.S3.SkipTlsVerify
		timeout = Conf.Sync.S3.Timeout * 1000
	case conf.ProviderWebDAV:
		checkURL = Conf.Sync.WebDAV.Endpoint
		skipTlsVerify = Conf.Sync.WebDAV.SkipTlsVerify
		timeout = Conf.Sync.WebDAV.Timeout * 1000
	case conf.ProviderLocal:
		checkURL = "file://" + Conf.Sync.Local.Endpoint
		timeout = Conf.Sync.Local.Timeout * 1000
	default:
		logging.LogWarnf("unknown provider: %d", Conf.Sync.Provider)
		return false
	}

	if ret = util.IsOnline(checkURL, skipTlsVerify, timeout); !ret {
		if 1 > Conf.syncRuntime.errors.Load() || byHand {
			util.PushErrMsg(Conf.Language(76)+" (Provider: "+conf.ProviderToStr(Conf.Sync.Provider)+")", 5000)
		}
		if !byHand {
			owner.planSyncAfter(fixSyncInterval)
			owner.syncRuntime.errors.Add(1)
		}
	}
	return
}

type syncWebSocketConnection interface {
	ReadJSON(interface{}) error
	WriteJSON(interface{}) error
	SetCloseHandler(func(int, string) error)
	Close() error
}

var (
	webSocketConn       syncWebSocketConnection
	webSocketConnLock   = sync.Mutex{}
	syncWebSocketCancel context.CancelFunc
	syncWebSocketWorker chan struct{}
	syncWebSocketOwner  *AppConf
)

type OnlineKernel struct {
	ID       string `json:"id"`
	Hostname string `json:"hostname"`
	OS       string `json:"os"`
	Ver      string `json:"ver"`
}

func GetOnlineKernels() (ret []*OnlineKernel) {
	ret, _ = GetOnlineKernelsContext(context.Background())
	return
}

var closedSyncWebSocket = atomic.Bool{}

func currentSyncWebSocket() syncWebSocketConnection {
	webSocketConnLock.Lock()
	defer webSocketConnLock.Unlock()
	return webSocketConn
}

func closeSyncWebSocket() {
	if err := closeSyncWebSocketContext(context.Background()); err != nil {
		logging.LogErrorf("close sync websocket failed: %s", err)
	}
}

func closeSyncWebSocketContext(ctx context.Context) error {
	webSocketConnLock.Lock()
	conn, cancel, done := webSocketConn, syncWebSocketCancel, syncWebSocketWorker
	webSocketConn = nil
	closedSyncWebSocket.Store(true)
	webSocketConnLock.Unlock()
	if cancel != nil {
		cancel()
	}
	if conn != nil {
		_ = conn.Close()
	}
	if done != nil {
		select {
		case <-done:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	logging.LogInfof("sync websocket closed")
	return nil
}

func connectSyncWebSocket() {
	connectSyncWebSocketContext(context.Background(), Conf)
}

func connectSyncWebSocketContext(ctx context.Context, owner *AppConf) (err error) {
	defer recoverRepositoryWorkflow(&err, "sync_websocket_connect")

	if ctx == nil || owner == nil || owner.Sync == nil {
		return conf.ErrBusinessUnbound
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if !owner.Sync.Enabled || !isSubscriberOwner(owner) || conf.ProviderSiYuan != owner.Sync.Provider {
		return nil
	}

	if util.ContainerDocker == util.Container {
		return nil
	}

	webSocketConnLock.Lock()
	if nil != webSocketConn || syncWebSocketWorker != nil {
		foreign := syncWebSocketOwner != nil && syncWebSocketOwner != owner
		webSocketConnLock.Unlock()
		if foreign {
			return conf.ErrBusinessUnbound
		}
		return nil
	}

	identity, identityErr := captureSyncWebSocketIdentity(owner)
	if identityErr != nil {
		webSocketConnLock.Unlock()
		return identityErr
	}
	conn, dialErr := dialSyncWebSocketIdentity(ctx, identity)
	if nil != dialErr {
		webSocketConnLock.Unlock()
		logging.LogWarnf("connect sync websocket failed: %s", dialErr)
		return dialErr
	}
	owner.repoMu.Lock()
	if owner.repoClosed || owner != Conf {
		owner.repoMu.Unlock()
		webSocketConnLock.Unlock()
		_ = conn.Close()
		return conf.ErrBusinessUnbound
	}
	if owner.repoTasks == nil {
		owner.repoTasks = taskgroup.Open()
	}
	workerContext, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	webSocketConn, syncWebSocketCancel, syncWebSocketWorker, syncWebSocketOwner = conn, cancel, done, owner
	closedSyncWebSocket.Store(false)

	conn.SetCloseHandler(func(code int, text string) error {
		logging.LogWarnf("sync websocket closed: %d, %s", code, text)
		return nil
	})
	accepted := owner.repoTasks.Go(func(ownerContext context.Context) {
		stop := context.AfterFunc(ownerContext, cancel)
		defer stop()
		runSyncWebSocket(workerContext, conn, done, ownerSyncWebSocketPorts(owner, identity))
	})
	owner.repoMu.Unlock()
	webSocketConnLock.Unlock()
	if !accepted {
		cancel()
		_ = conn.Close()
		webSocketConnLock.Lock()
		if syncWebSocketWorker == done {
			webSocketConn, syncWebSocketCancel, syncWebSocketWorker, syncWebSocketOwner = nil, nil, nil, nil
		}
		webSocketConnLock.Unlock()
		close(done)
		return conf.ErrBusinessUnbound
	}
	logging.LogInfof("sync websocket connected")
	return nil
}

type syncWebSocketPorts struct {
	dial    func(context.Context) (syncWebSocketConnection, error)
	wait    func(context.Context, time.Duration) error
	user    func() bool
	synced  func(context.Context)
	kernels func(context.Context, []*OnlineKernel) error
	closed  func()
}

func ownerSyncWebSocketPorts(owner *AppConf, identity syncWebSocketIdentity) syncWebSocketPorts {
	return syncWebSocketPorts{
		kernels: func(ctx context.Context, values []*OnlineKernel) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			if !syncWebSocketOwnerCurrent(owner) {
				return conf.ErrBusinessUnbound
			}
			owner.syncRuntime.replaceKernels(values)
			return nil
		},
		closed: func() { owner.syncRuntime.replaceKernels(nil) },
		dial: func(ctx context.Context) (syncWebSocketConnection, error) {
			return dialSyncWebSocketIdentity(ctx, identity)
		},
		wait: waitSyncWebSocket,
		user: func() bool { return syncWebSocketOwnerCurrent(owner) },
		synced: func(ctx context.Context) {
			if syncWebSocketOwnerCurrent(owner) {
				_, _ = owner.syncDataContext(ctx, "download", true)
			}
		},
	}
}

func syncWebSocketOwnerCurrent(owner *AppConf) bool {
	if owner == nil || owner != Conf || owner.GetUser() == nil {
		return false
	}
	webSocketConnLock.Lock()
	active := syncWebSocketOwner == owner && syncWebSocketWorker != nil
	webSocketConnLock.Unlock()
	if !active {
		return false
	}
	owner.repoMu.Lock()
	defer owner.repoMu.Unlock()
	return !owner.repoClosed
}

func isSubscriberOwner(owner *AppConf) bool {
	u := owner.GetUser()
	return nil != u && (-1 == u.UserSiYuanProExpireTime || 0 < u.UserSiYuanProExpireTime) && 0 == u.UserSiYuanSubscriptionStatus
}

func waitSyncWebSocket(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func runSyncWebSocket(ctx context.Context, initial syncWebSocketConnection, done chan struct{}, ports syncWebSocketPorts) {
	current := initial
	defer func() {
		if current != nil {
			_ = current.Close()
		}
		webSocketConnLock.Lock()
		if syncWebSocketWorker == done {
			if ports.closed != nil {
				ports.closed()
			}
			webSocketConn, syncWebSocketCancel, syncWebSocketWorker, syncWebSocketOwner = nil, nil, nil, nil
			closedSyncWebSocket.Store(true)
		}
		webSocketConnLock.Unlock()
		close(done)
	}()
	defer logging.Recover()
	stopClose := context.AfterFunc(ctx, func() {
		webSocketConnLock.Lock()
		if syncWebSocketWorker == done && webSocketConn != nil {
			_ = webSocketConn.Close()
		}
		webSocketConnLock.Unlock()
	})
	defer stopClose()

	for {
		result := gulu.Ret.NewResult()
		if readErr := current.ReadJSON(&result); readErr != nil {
			if ports.wait == nil || ports.dial == nil || ports.user == nil || ports.wait(ctx, time.Second) != nil || closedSyncWebSocket.Load() {
				return
			}
			reconnected := false
			for retries := 0; retries < 7; retries++ {
				if ports.wait(ctx, 7*time.Second) != nil || !ports.user() {
					return
				}
				next, dialErr := ports.dial(ctx)
				if dialErr != nil {
					logging.LogWarnf("reconnect sync websocket failed: %s", dialErr)
					continue
				}
				next.SetCloseHandler(func(code int, text string) error {
					logging.LogWarnf("sync websocket closed: %d, %s", code, text)
					return nil
				})
				webSocketConnLock.Lock()
				if syncWebSocketWorker != done || ctx.Err() != nil {
					webSocketConnLock.Unlock()
					_ = next.Close()
					return
				}
				webSocketConn = next
				webSocketConnLock.Unlock()
				_ = current.Close()
				current = next
				logging.LogInfof("sync websocket reconnected")
				reconnected = true
				break
			}
			if !reconnected {
				logging.LogWarnf("reconnect sync websocket failed, do not retry")
				return
			}
			continue
		}

		logging.LogInfof("sync websocket message: %v", result)
		data := result.Data.(map[string]interface{})
		switch data["cmd"].(string) {
		case "synced":
			if ports.synced != nil {
				ports.synced(ctx)
			}
		case "kernels":
			kernels := []*OnlineKernel{}
			for _, kernel := range data["kernels"].([]interface{}) {
				kernelMap := kernel.(map[string]interface{})
				kernels = append(kernels, &OnlineKernel{ID: kernelMap["id"].(string), Hostname: kernelMap["hostname"].(string), OS: kernelMap["os"].(string), Ver: kernelMap["ver"].(string)})
			}
			if ports.kernels != nil {
				if err := ports.kernels(ctx, kernels); err != nil {
					return
				}
			}
		}
	}
}

var KernelID = gulu.Rand.String(7)

func dialSyncWebSocket() (c *websocket.Conn, err error) {
	return dialSyncWebSocketContext(context.Background())
}

func dialSyncWebSocketContext(ctx context.Context) (c *websocket.Conn, err error) {
	identity, identityErr := captureSyncWebSocketIdentity(Conf)
	if identityErr != nil {
		return nil, identityErr
	}
	return dialSyncWebSocketIdentity(ctx, identity)

}

type syncWebSocketIdentity struct {
	endpoint string
	header   http.Header
}

func captureSyncWebSocketIdentity(owner *AppConf) (syncWebSocketIdentity, error) {
	if owner == nil || owner.Sync == nil {
		return syncWebSocketIdentity{}, errors.New("sync websocket owner unavailable")
	}
	u := owner.GetUser()
	if u == nil {
		return syncWebSocketIdentity{}, errors.New("sync websocket user unavailable")
	}
	header := http.Header{}
	header.Set("User-Agent", util.UserAgent)
	header.Set("x-siyuan-uid", u.UserId)
	header.Set("x-wiki-kernel", KernelID)
	header.Set("x-siyuan-ver", util.Ver)
	header.Set("x-siyuan-os", runtime.GOOS)
	header.Set("x-siyuan-hostname", util.GetDeviceName())
	header.Set("x-siyuan-repo", owner.Sync.CloudName)
	return syncWebSocketIdentity{endpoint: util.GetCloudWebSocketServer() + "/apis/siyuan/dejavu/ws", header: header}, nil
}

func dialSyncWebSocketIdentity(ctx context.Context, identity syncWebSocketIdentity) (c *websocket.Conn, err error) {
	c, _, err = websocket.DefaultDialer.DialContext(ctx, identity.endpoint, identity.header.Clone())
	if err == nil {
		closedSyncWebSocket.Store(false)
	}
	return
}
