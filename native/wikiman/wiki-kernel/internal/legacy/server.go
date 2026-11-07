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

//go:build !mobile

package legacy

import (
	"context"
	"fmt"
	"github.com/siyuan-note/logging"
	"wiki-kernel/cache"
	"wiki-kernel/job"
	"wiki-kernel/model"
	"wiki-kernel/server"
	"wiki-kernel/sql"
	"wiki-kernel/util"
)

// Start preserves the original single-workspace server during the staged
// context migration. New CLI handlers must never import this package: model
// still has process-global resources that belong to the legacy GUI lifecycle.
func Start() error {
	if err := model.StartLegacyRuntime(); err != nil {
		return err
	}
	util.Boot()

	// ToolBox: wire the workspace hot-switch pipeline.
	// See docs/wiki-workspace-hotswitch/design.md.
	//   - RegisterCronController provides the job package's Start/Stop/Drain
	//     hooks (model cannot import job directly).
	//   - RegisterWorkspaceSwitchFunc provides util.SwitchWorkspace its
	//     Step 2/3 pipeline implementation.
	model.RegisterCronController(
		job.StartCron,
		job.StopCron,
		job.DrainCron,
	)
	util.RegisterWorkspaceSwitchFunc(model.SwitchWorkspacePipeline)

	model.InitConf()
	if !model.StartLegacyBootTask(util.InitTesseractContext) {
		logging.LogWarnf("start Tesseract initialization failed: boot owner unavailable")
	}
	model.InitAppearance()
	if err := sql.InitDatabase(false); err != nil {
		return fmt.Errorf("initialize compatibility database: %w", err)
	}
	if e := sql.InitHistoryDatabaseContext(context.Background(), false); e != nil {
		return fmt.Errorf("initialize history index: %w", e)
	}
	sql.InitAssetContentDatabase(false)
	sql.SetCaseSensitive(model.Conf.Search.CaseSensitive)
	sql.SetIndexAssetPath(model.Conf.Search.IndexAssetPath)

	model.BootSyncData()
	if err := model.InitBoxesContext(context.Background()); err != nil {
		return fmt.Errorf("initialize notebooks: %w", err)
	}
	model.EnsureSingleNotebookAndDoc() // M7: single-notebook mode bootstrap (no-op in multi-mode)
	model.LoadFlashcards()
	util.LoadAssetsTexts()

	util.SetBooted()
	util.PushClearAllMsg()
	// Do not admit getConf/mutation handlers until DBs and single-mode IDs are
	// initialized. An early getConf formerly started notebook bootstrap itself.
	if err := server.Start(false); err != nil {
		return fmt.Errorf("start compatibility HTTP server: %w", err)
	}

	job.StartCron()

	if !model.StartLegacyBootTask(func(context.Context) { util.LoadSysFonts() }) {
		logging.LogWarnf("load system fonts failed: boot owner unavailable")
	}
	model.AutoGenerateFileHistory()
	if !model.StartLegacyBootTask(func(context.Context) { cache.LoadAssets() }) {
		logging.LogWarnf("load assets failed: boot owner unavailable")
	}
	if !model.StartLegacyBootTask(util.CheckFileSysStatusContext) {
		logging.LogWarnf("start filesystem status check failed: boot owner unavailable")
	}

	model.WatchAssets()
	model.WatchEmojis()
	model.HandleSignal()
	return nil
}
