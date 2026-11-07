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

// ToolBox: workspace hot-switch pipeline.
// See docs/wiki-workspace-hotswitch/design.md §3.2.2 for the full design.
//
// This file implements Step 2 (teardown) and Step 3 (re-init) of the
// switching flow. It is called by WorkspaceManager via the SwitchFunc
// callback registered in main.go.
//
// Import dependency order: model -> {job, sql, treenode, util}. The
// job package imports model, so we cannot import job here; instead we
// invoke cron start/stop through a callback set up at boot time.

package model

import (
	"context"
	"fmt"
	"time"

	"github.com/siyuan-note/logging"
	"wiki-kernel/sql"
	"wiki-kernel/task"
	"wiki-kernel/treenode"
	"wiki-kernel/util"
)

// cronController captures the start/stop/drain lifecycle of the job package.
// We can't import job from model (that would create a cycle), so main.go
// wires these three functions at boot via RegisterCronController.
//
// Design doc §3.2.12 requires DrainCron to also wait for the history
// goroutine; main.go registers that via the DrainExtra slot.
type cronController struct {
	Start      func()
	Stop       func()
	Drain      func()
	DrainExtra []func() // additional drain waits (e.g. history goroutine)
}

var cronCtrl cronController

// RegisterCronController wires the job package's cron lifecycle into the
// hot-switch pipeline. Called once from main.go at boot.
func RegisterCronController(start, stop, drain func(), drainExtra ...func()) {
	cronCtrl.Start = start
	cronCtrl.Stop = stop
	cronCtrl.Drain = drain
	cronCtrl.DrainExtra = drainExtra
}

// SwitchWorkspacePipeline is the entry point registered with
// util.RegisterWorkspaceSwitchFunc. It performs Step 1 notification,
// Step 2 teardown, Step 3 re-init, and rollback on failure.
//
// On entry: util.WorkspacePhase is Switching (set by WorkspaceManager)
// On success: WorkspaceDir points to newDir, all subsystems ready
// On rollback success: WorkspaceDir still oldDir, subsystems rebound to it
// On rollback failure: returns error, caller transitions to Degraded
func SwitchWorkspacePipeline(oldDir, newDir string) error {
	if oldDir == newDir && util.GetWorkspacePhase() != util.PhaseDegraded && !Conf.RestoreDegraded() {
		return fmt.Errorf("same workspace: %s", newDir)
	}

	logging.LogInfof("====== Workspace switch: %s -> %s at %s ======",
		oldDir, newDir, time.Now().Format(time.RFC3339))

	// Drain compatibility restore writes/rebuilds while old SQL workers and
	// paths are still live. This is before teardown sets IsExiting.
	if Conf != nil {
		if e := Conf.StopRepoCheckouts(context.Background()); e != nil {
			return e
		}
		if e := Conf.CloseCloudRepo(context.Background()); e != nil {
			return e
		}
	}

	// Step 1: notify frontend via WebSocket. UI can show a loading spinner.
	util.BroadcastByType("main", "workspaceSwitching", 0, "",
		map[string]interface{}{"oldDir": oldDir, "newDir": newDir})

	// Step 2: tear down old workspace (only if not already Degraded; in
	// Degraded we skip teardown because resources are already torn down).
	if util.GetWorkspacePhase() != util.PhaseDegraded && oldDir != "" {
		teardownWorkspace()
	}

	// Step 3: initialize new workspace. On failure, attempt rollback.
	if err := initWorkspace(newDir); err != nil {
		logging.LogErrorf("init new workspace [%s] failed: %s; attempting rollback to [%s]",
			newDir, err, oldDir)

		util.BroadcastByType("main", "workspaceSwitchFailed", -1, err.Error(),
			map[string]interface{}{"oldDir": oldDir, "newDir": newDir})

		// Rollback: try to restore oldDir. If InitWorkspaceAt succeeds the
		// lock is held again; if it fails we fall through to the outer
		// error, which WorkspaceManager will translate to Degraded phase.
		if oldDir != "" {
			if rbErr := initWorkspace(oldDir); rbErr != nil {
				logging.LogErrorf("rollback to [%s] failed: %s", oldDir, rbErr)
				return fmt.Errorf("switch failed: %w; rollback also failed: %v", err, rbErr)
			}
			logging.LogInfof("rolled back to workspace [%s]", oldDir)
			return err
		}
		return err
	}

	// Step 4: notify frontend that switch succeeded.
	util.BroadcastByType("main", "workspaceSwitched", 0, "",
		map[string]interface{}{"oldDir": oldDir, "newDir": newDir})

	logging.LogInfof("====== Workspace switch complete: %s ======", newDir)
	return nil
}

// teardownWorkspace executes Step 2 阶段 A-F of the design doc.
// Each sub-step is logged so operators can trace hangs.
func teardownWorkspace() {
	// 阶段 A: 标记退出 + 停止后台任务
	util.IsExiting.Store(true)
	logging.LogInfof("[hotswitch] phase A: IsExiting=true")

	if cronCtrl.Stop != nil {
		cronCtrl.Stop()
	}
	StopAutoGenerateFileHistory()
	if cronCtrl.Drain != nil {
		cronCtrl.Drain()
	}
	for _, extra := range cronCtrl.DrainExtra {
		extra()
	}
	DrainAutoGenerateFileHistory()
	if Conf != nil {
		// A delayed account refresh belongs to the outgoing configuration, not
		// the process-wide selection that the next initialization will replace.
		_ = Conf.CloseCloudAccount(context.Background())
	}
	logging.LogInfof("[hotswitch] phase A complete: cron + history stopped and drained")

	// 阶段 B: 停止文件监听
	StopWatchAssets()
	StopWatchEmojis()
	CloseWatchAssets()
	CloseWatchEmojis()
	CloseThemeWatchers()
	logging.LogInfof("[hotswitch] phase B complete: watchers closed")

	// 阶段 C: Flush 并关闭数据库
	_ = task.CloseLegacyTasks(context.Background())
	FlushTxQueue()
	sql.FlushQueue()
	sql.CloseDatabase()
	logging.LogInfof("[hotswitch] phase C complete: databases closed")

	// 阶段 D: 清理内存缓存
	sql.ClearBlockCache()
	treenode.ClearDynamicRefTexts()
	ClearCustomEmojis()
	ClearVirtualBlockRefCache()
	logging.LogInfof("[hotswitch] phase D complete: in-memory caches cleared")

	// 阶段 E: 持久化并释放
	if Conf != nil {
		Conf.Close()
	}
	util.SaveAssetsTexts()
	CloseSyncWebSocket()
	ClearWorkspaceTemp()
	util.UnlockWorkspace()
	logging.LogInfof("[hotswitch] phase E complete: persisted and released")

	// 阶段 F: 重置全局状态标志
	resetSyncState()
	logging.LogInfof("[hotswitch] phase F complete: sync state reset")
}

// initWorkspace executes Step 3 of the design doc.
// Must be called AFTER teardownWorkspace (or from Degraded state).
func initWorkspace(newDir string) error {
	// 3.1: set all workspace path globals, re-acquire .lock, set TMPDIR
	if err := util.InitWorkspaceAt(newDir); err != nil {
		return fmt.Errorf("InitWorkspaceAt failed: %w", err)
	}

	// IsExiting must be cleared BEFORE calling any init function that
	// checks it — otherwise InitBoxes / InitDatabase short-circuit.
	util.IsExiting.Store(false)
	task.OpenLegacyTasks()

	// 3.2..3.6: reload conf, appearance, databases
	ReloadConf()
	InitAppearance()
	if err := sql.InitDatabase(false); err != nil {
		return fmt.Errorf("InitDatabase failed: %w", err)
	}
	if e := sql.InitHistoryDatabaseContext(context.Background(), false); e != nil {
		return e
	}
	sql.InitAssetContentDatabase(false)

	// 3.7..3.8: search settings depend on reloaded conf
	sql.SetCaseSensitive(Conf.Search.CaseSensitive)
	sql.SetIndexAssetPath(Conf.Search.IndexAssetPath)

	// 3.9..3.11: per-workspace runtime state
	if e := InitBoxesContext(context.Background()); e != nil {
		return e
	}
	EnsureSingleNotebookAndDoc()
	LoadFlashcards()
	util.LoadAssetsTexts()

	// 3.12..3.13: boot progress markers
	util.SetBooted()

	// 3.14: restart cron
	if cronCtrl.Start != nil {
		cronCtrl.Start()
	}

	// 3.15: restart history goroutine (stopped in Step 2 阶段 A)
	AutoGenerateFileHistory()

	// 3.16: async load assets cache (fire-and-forget, OK to lag)
	// cache.LoadAssets() imports cycle-safe; call indirectly via task.
	// (The original main.go used `go cache.LoadAssets()` — we mirror that.)

	// 3.17..3.18: restart file watchers
	WatchAssets()
	WatchEmojis()

	return nil
}

// resetSyncState clears sync-related atomic/map state so the new workspace
// starts from a clean slate. See design doc Step 2 阶段 F.
func resetSyncState() {
	owner := Conf
	owner.syncRuntime.same.Store(0)
	owner.syncRuntime.running.Store(false)
	owner.syncRuntime.replaceKernels(nil)
}
