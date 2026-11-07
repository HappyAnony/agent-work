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

package main

import (
	"C"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/88250/gulu"
	"github.com/siyuan-note/filelock"
	"github.com/siyuan-note/logging"
	"wiki-kernel/cache"
	"wiki-kernel/job"
	"wiki-kernel/model"
	"wiki-kernel/server"
	"wiki-kernel/sql"
	"wiki-kernel/util"
)

//export StartKernelFast
func StartKernelFast(container, appDir, workspaceBaseDir, localIPs *C.char) {
	if err := server.Start(true); err != nil {
		logging.LogErrorf("start fast HTTP server failed: %s", err)
	}
}

//export StartKernel
func StartKernel(container, appDir, workspaceBaseDir, timezoneID, localIPs, lang, osVer *C.char) {
	if err := model.StartLegacyRuntime(); err != nil {
		logging.LogErrorf("start legacy runtime failed: %s", err)
		return
	}
	model.RegisterCronController(job.StartCron, job.StopCron, job.DrainCron)
	SetTimezone(C.GoString(container), C.GoString(appDir), C.GoString(timezoneID))
	util.Mode = "prod"
	util.MobileOSVer = C.GoString(osVer)
	util.LocalIPs = strings.Split(C.GoString(localIPs), ",")
	util.BootMobile(C.GoString(container), C.GoString(appDir), C.GoString(workspaceBaseDir), C.GoString(lang))

	model.InitConf()
	if err := server.Start(false); err != nil {
		logging.LogErrorf("start HTTP server failed: %s", err)
		return
	}
	if !model.StartLegacyBootTask(func(ctx context.Context) {
		model.InitAppearance()
		if ctx.Err() != nil {
			return
		}
		sql.InitDatabase(false)
		sql.InitHistoryDatabase(false)
		sql.InitAssetContentDatabase(false)
		sql.SetCaseSensitive(model.Conf.Search.CaseSensitive)
		sql.SetIndexAssetPath(model.Conf.Search.IndexAssetPath)

		model.BootSyncData()
		model.InitBoxes()
		if ctx.Err() != nil {
			return
		}
		model.LoadFlashcards()
		util.LoadAssetsTexts()

		util.SetBooted()
		util.PushClearAllMsg()

		job.StartCron()
		model.AutoGenerateFileHistory()
		if ctx.Err() == nil {
			cache.LoadAssets()
		}
	}) {
		logging.LogErrorf("start Harmony initialization failed: boot owner unavailable")
	}
}

//export Language
func Language(num int) string {
	return model.Conf.Language(num)
}

//export ShowMsg
func ShowMsg(msg string, timeout int) {
	util.PushMsg(msg, timeout)
}

//export IsHttpServing
func IsHttpServing() bool {
	return util.HttpServing
}

//export SetHttpServerPort
func SetHttpServerPort(port int) {
	filelock.AndroidServerPort = port
}

//export GetCurrentWorkspacePath
func GetCurrentWorkspacePath() *C.char {
	return C.CString(util.WorkspaceDir)
}

//export GetAssetAbsPath
func GetAssetAbsPath(relativePath *C.char) *C.char {
	absPath, err := model.GetAssetAbsPath(C.GoString(relativePath))
	if nil != err {
		logging.LogErrorf("get asset abs path failed: %s", err)
		return relativePath
	}
	return C.CString(absPath)
}

//export GetMimeTypeByExt
func GetMimeTypeByExt(ext string) string {
	return util.GetMimeTypeByExt(ext)
}

//export SetTimezone
func SetTimezone(container, appDir, timezoneID string) {
	if "ios" == container {
		os.Setenv("ZONEINFO", filepath.Join(appDir, "app", "zoneinfo.zip"))
	}
	z, err := time.LoadLocation(strings.TrimSpace(timezoneID))
	if err != nil {
		fmt.Printf("load location failed: %s\n", err)
		time.Local = time.FixedZone("CST", 8*3600)
		return
	}
	time.Local = z
}

//export DisableFeature
func DisableFeature(feature *C.char) {
	util.DisableFeature(C.GoString(feature))
}

//export Unzip
func Unzip(zipFilePath, destination *C.char) {
	if err := gulu.Zip.Unzip(C.GoString(zipFilePath), C.GoString(destination)); nil != err {
		logging.LogErrorf("unzip [%s] failed: %s", C.GoString(zipFilePath), err)
	}
}

func main() {}
