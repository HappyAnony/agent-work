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
	"os"
	"path/filepath"

	"github.com/88250/gulu"
	"github.com/siyuan-note/logging"
	"wiki-kernel/util"
)

func MoveLocalShorthands(boxID, hPath, parentID string) (retIDs []string, err error) {
	return MoveLocalShorthandsContext(context.Background(), boxID, hPath, parentID)
}

func WatchLocalShorthands() {
	shorthandsDir := filepath.Join(util.ShortcutsPath, "shorthands")
	if !gulu.File.IsDir(shorthandsDir) {
		return
	}

	entries, err := os.ReadDir(shorthandsDir)
	if nil != err {
		logging.LogErrorf("read dir [%s] failed: %s", shorthandsDir, err)
		return
	}

	shorthandCount := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		if filepath.Ext(entry.Name()) != ".md" {
			continue
		}

		shorthandCount++
	}

	if 1 > shorthandCount {
		return
	}

	util.PushLocalShorthandCount(shorthandCount)
}
