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

// ToolBox: API handlers for in-process workspace hot-switching.
// See docs/wiki-workspace-hotswitch/design.md §3.2.3.
//
// This file is kept separate from workspace.go (which hosts the upstream
// setWorkspaceDir handler) so upstream merges don't conflict with our
// additions.

package api

import (
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/88250/gulu"
	"github.com/gin-gonic/gin"
	"wiki-kernel/model"
	"wiki-kernel/util"
)

// switchWorkspaceDir performs an in-process workspace switch. Unlike
// setWorkspaceDir which merely updates workspace.json and (on mobile) exits
// the process, this handler invokes the WorkspaceManager pipeline that tears
// down the old workspace resources and initializes the new one while keeping
// the kernel process and HTTP port unchanged.
//
// Request body: {"path": "/absolute/workspace/path"}
// Response: {code, msg, data: {phase, oldDir, newDir}}
// HTTP status: 200 on success, 409 on concurrent switch request, 500 on
// pipeline failure (caller should inspect phase to decide retry).
func switchWorkspaceDir(c *gin.Context) {
	ret := gulu.Ret.NewResult()

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		c.JSON(http.StatusOK, ret)
		return
	}

	pathRaw, ok := arg["path"].(string)
	if !ok || strings.TrimSpace(pathRaw) == "" {
		ret.Code = -1
		ret.Msg = "path is required"
		c.JSON(http.StatusOK, ret)
		return
	}
	newPath := strings.TrimSpace(pathRaw)

	// Step 0a: basic path validation. We intentionally allow switching to
	// the current workspace when the manager is in Degraded state, since
	// that is the recovery path (design §3.2.14).
	currentPhase := util.GetWorkspacePhase()
	if newPath == util.WorkspaceDir && currentPhase == util.PhaseReady && !model.Conf.RestoreDegraded() {
		ret.Code = -1
		ret.Msg = model.Conf.Language(78) // "same workspace"
		c.JSON(http.StatusOK, ret)
		return
	}

	// Step 0b: reject cloud drive paths (reuses setWorkspaceDir logic).
	if util.IsCloudDrivePath(newPath) {
		ret.Code = -1
		ret.Msg = model.Conf.Language(196)
		c.JSON(http.StatusOK, ret)
		return
	}

	// Step 0c: reject nested workspaces.
	if !util.IsWorkspaceDir(newPath) {
		for p := filepath.Dir(newPath); !util.IsPartitionRootPath(p); p = filepath.Dir(p) {
			if util.IsWorkspaceDir(p) {
				ret.Code = -1
				ret.Msg = fmt.Sprintf(model.Conf.Language(256), newPath, p)
				c.JSON(http.StatusOK, ret)
				return
			}
		}
	}

	oldDir := util.WorkspaceDir

	// Step 1..4: delegate to the WorkspaceManager pipeline. The pipeline is
	// registered by the model package during boot (see model.init or wherever
	// RegisterWorkspaceSwitchFunc is called).
	err := util.SwitchWorkspace(newPath)
	if err != nil {
		if errors.Is(err, util.ErrSwitchInProgress) {
			ret.Code = -1
			ret.Msg = err.Error()
			c.JSON(http.StatusConflict, ret)
			return
		}
		ret.Code = -1
		ret.Msg = err.Error()
		ret.Data = map[string]interface{}{
			"phase":  string(util.GetWorkspacePhase()),
			"oldDir": oldDir,
			"newDir": newPath,
		}
		c.JSON(http.StatusInternalServerError, ret)
		return
	}

	ret.Data = map[string]interface{}{
		"phase":  string(util.GetWorkspacePhase()),
		"oldDir": oldDir,
		"newDir": newPath,
	}
	c.JSON(http.StatusOK, ret)
}

// getWorkspaceStatus returns the current workspace manager state. Always
// returns 200 regardless of phase — this is a diagnostic endpoint that must
// work in every phase including Degraded.
func getWorkspaceStatus(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	status := util.GetWorkspaceStatus()
	if model.Conf.RestoreDegraded() {
		status.Phase = util.PhaseDegraded
		status.Message = "workspace restoration requires recovery"
	}
	ret.Data = map[string]interface{}{
		"currentDir": status.CurrentDir,
		"phase":      string(status.Phase),
		"message":    status.Message,
	}
	c.JSON(http.StatusOK, ret)
}
