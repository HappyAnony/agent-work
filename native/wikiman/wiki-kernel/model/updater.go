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
	"os/exec"
	"strings"
	"wiki-kernel/internal/announcement"

	"github.com/88250/gulu"
	"github.com/siyuan-note/logging"
	"wiki-kernel/util"
)

func execNewVerInstallPkg(newVerInstallPkgPath string) {
	logging.LogInfof("installing the new version [%s]", newVerInstallPkgPath)
	var cmd *exec.Cmd
	if gulu.OS.IsWindows() {
		cmd = exec.Command(newVerInstallPkgPath)
	} else if gulu.OS.IsDarwin() {
		exec.Command("chmod", "+x", newVerInstallPkgPath).CombinedOutput()
		cmd = exec.Command("open", newVerInstallPkgPath)
	}
	gulu.CmdAttr(cmd)
	cmdErr := cmd.Run()
	if nil != cmdErr {
		logging.LogErrorf("exec install new version failed: %s", cmdErr)
		return
	}
}

var newVerInstallPkgPath string

type Announcement = announcement.Item

func getAnnouncements() []*Announcement {
	items, e := GetAnnouncementsContext(context.Background())
	if e != nil {
		logging.LogErrorf("get announcement failed")
		return nil
	}
	return items
}

func skipNewVerInstallPkg() bool {
	if !gulu.OS.IsWindows() && !gulu.OS.IsDarwin() {
		return true
	}
	if util.ISMicrosoftStore || util.ContainerStd != util.Container {
		return true
	}
	if !Conf.System.DownloadInstallPkg {
		return true
	}
	if gulu.OS.IsWindows() {
		plat := strings.ToLower(Conf.System.OSPlatform)
		// Windows 7, 8 and Server 2012 are no longer supported https://github.com/siyuan-note/siyuan/issues/7347
		if strings.Contains(plat, " 7 ") || strings.Contains(plat, " 8 ") || strings.Contains(plat, "2012") {
			return true
		}
	}
	return false
}
