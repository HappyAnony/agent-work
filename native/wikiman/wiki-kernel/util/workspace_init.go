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

// ToolBox: Re-entrant workspace initialization used by hot-switch.
// See docs/wiki-workspace-hotswitch/design.md §3.2.7.
//
// Unlike the original Boot() -> initWorkspaceDir() path this function:
//   - Returns errors instead of calling os.Exit / LogFatalf
//   - Does not read CLI flags; accepts an explicit path argument
//   - Can be called multiple times (no panic on re-init)
//   - Updates every global path variable including AppearancePath/ThemesPath/IconsPath
//   - Re-acquires the workspace .lock (caller must release the previous lock first)

package util

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/88250/gulu"
	"github.com/gofrs/flock"
	"github.com/siyuan-note/logging"
)

var ErrWorkspaceLocked = errors.New("workspace locked by another kernel instance")

// InitWorkspaceAt initializes all workspace-related global variables for the
// given path and acquires the .lock file. It is safe to call multiple times
// within the same process lifetime.
//
// The caller is responsible for:
//  - Releasing the previous workspace lock before calling this function
//  - Calling os.Setenv("TMPDIR") if needed (done here to mirror initWorkspaceDir)
//  - Persisting the new path to workspace.json if desired (done here)
//
// This function does NOT call InitConf / InitDatabase / StartCron etc.; those
// are orchestrated by the hot-switch pipeline (see design doc Step 3).
func InitWorkspaceAt(path string) error {
	if path == "" {
		return errors.New("workspace path cannot be empty")
	}

	if !gulu.File.IsDir(path) {
		if err := os.MkdirAll(path, 0755); err != nil && !os.IsExist(err) {
			return fmt.Errorf("create workspace folder [%s] failed: %w", path, err)
		}
	}

	// Persist new workspace to ~/.config/siyuan/workspace.json, keeping the
	// most-recently-used entry at the end of the list (same semantics as
	// initWorkspaceDir).
	workspacePaths, _ := ReadWorkspacePaths()
	workspacePaths = append(workspacePaths, path)
	if err := WriteWorkspacePaths(workspacePaths); err != nil {
		return fmt.Errorf("write workspace conf failed: %w", err)
	}

	WorkspaceDir = path
	WorkspaceName = filepath.Base(WorkspaceDir)
	ConfDir = filepath.Join(WorkspaceDir, "conf")
	DataDir = filepath.Join(WorkspaceDir, "data")
	RepoDir = filepath.Join(WorkspaceDir, "repo")
	HistoryDir = filepath.Join(WorkspaceDir, "history")
	TempDir = filepath.Join(WorkspaceDir, "temp")

	osTmpDir := filepath.Join(TempDir, "os")
	_ = os.RemoveAll(osTmpDir)
	if err := os.MkdirAll(osTmpDir, 0755); err != nil {
		return fmt.Errorf("create os tmp dir [%s] failed: %w", osTmpDir, err)
	}
	_ = os.RemoveAll(filepath.Join(TempDir, "repo"))
	_ = os.Setenv("TMPDIR", osTmpDir)
	_ = os.Setenv("TEMP", osTmpDir)
	_ = os.Setenv("TMP", osTmpDir)

	DBPath = filepath.Join(TempDir, DBName)
	HistoryDBPath = filepath.Join(TempDir, "history.db")
	AssetContentDBPath = filepath.Join(TempDir, "asset_content.db")
	BlockTreeDBPath = filepath.Join(TempDir, "blocktree.db")
	SnippetsPath = filepath.Join(DataDir, "snippets")

	// Appearance paths: mirror Boot() logic. In dev mode themes/icons are
	// served from the working dir; in prod they live under the workspace.
	AppearancePath = filepath.Join(ConfDir, "appearance")
	if "dev" == Mode {
		ThemesPath = filepath.Join(WorkingDir, "appearance", "themes")
		IconsPath = filepath.Join(WorkingDir, "appearance", "icons")
	} else {
		ThemesPath = filepath.Join(AppearancePath, "themes")
		IconsPath = filepath.Join(AppearancePath, "icons")
	}

	// Create the subdirectory tree (errors-returning version of initPathDir).
	if err := initPathDirE(); err != nil {
		return err
	}

	// Re-acquire the workspace lock. The caller must have released the
	// previous lock before we reach this point.
	if err := tryLockWorkspaceE(); err != nil {
		return err
	}

	// Refresh the log path pointer. We intentionally do NOT re-point the
	// logging backend — tracing subscriber reconfiguration mid-process is
	// complex and the design doc (§3.2.10) keeps the original log target.
	LogPath = filepath.Join(TempDir, "siyuan.log")

	return nil
}

// initPathDirE is the error-returning version of initPathDir used by
// InitWorkspaceAt. The original initPathDir calls logging.LogFatalf which
// terminates the process on failure — unacceptable during hot-switch.
func initPathDirE() error {
	dirs := []string{
		ConfDir,
		DataDir,
		TempDir,
		filepath.Join(DataDir, "assets"),
		filepath.Join(DataDir, "templates"),
		filepath.Join(DataDir, "widgets"),
		filepath.Join(DataDir, "plugins"),
		filepath.Join(DataDir, "emojis"),
		// data/public is used by `/public/*` URL serving, see upstream issue 8593
		filepath.Join(DataDir, "public"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil && !os.IsExist(err) {
			return fmt.Errorf("create folder [%s] failed: %w", d, err)
		}
	}
	return nil
}

// tryLockWorkspaceE is the error-returning version of tryLockWorkspace.
func tryLockWorkspaceE() error {
	WorkspaceLock = flock.New(filepath.Join(WorkspaceDir, ".lock"))
	ok, err := WorkspaceLock.TryLock()
	if ok {
		return nil
	}
	if err != nil {
		logging.LogErrorf("lock workspace [%s] failed: %s", WorkspaceDir, err)
		return fmt.Errorf("%w: %v", ErrWorkspaceLocked, err)
	}
	logging.LogErrorf("lock workspace [%s] failed", WorkspaceDir)
	return ErrWorkspaceLocked
}
