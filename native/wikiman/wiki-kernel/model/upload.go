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
	"path/filepath"

	"github.com/88250/gulu"
	"github.com/gin-gonic/gin"
	"github.com/siyuan-note/filelock"
	"wiki-kernel/util"
)

func InsertLocalAssets(id string, paths []string, upload bool) (map[string]interface{}, error) {
	return InsertLocalAssetsContext(context.Background(), id, paths, upload)
}

func Upload(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(200, ret)
	out, err := UploadAssetsRequest(c.Request.Context(), c.Request, 32<<20)
	ret.Data = out
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
	}
}

func getAssetsDir(boxLocalPath, docDirLocalPath string) (assets string) {
	return getAssetsDirAt(util.DataDir, boxLocalPath, docDirLocalPath)
}

func getAssetsDirAt(dataRoot, boxLocalPath, docDirLocalPath string) (assets string) {
	assets = filepath.Join(docDirLocalPath, "assets")
	if !filelock.IsExist(assets) {
		assets = filepath.Join(boxLocalPath, "assets")
		if !filelock.IsExist(assets) {
			assets = filepath.Join(dataRoot, "assets")
		}
	}
	return
}
