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

package treenode

import (
	"context"
	"github.com/siyuan-note/logging"
	"wiki-kernel/internal/indexdb"
)

func ClearRedundantBlockTrees(boxID string, paths []string) {
	redundantPaths := getRedundantPaths(boxID, paths)
	for _, p := range redundantPaths {
		removeBlockTreesByPath(boxID, p)
	}
}

func getRedundantPaths(boxID string, paths []string) (ret []string) {
	ret, _, err := (indexdb.BlockTrees{DB: db}).PathDifference(context.Background(), boxID, paths)
	if err != nil {
		logging.LogErrorf("blocktree path difference failed: %s", err)
	}
	return ret
}

func removeBlockTreesByPath(boxID, path string) {
	if _, err := (indexdb.BlockTrees{DB: db}).Remove(context.Background(), indexdb.BlockTreeFilter{Kind: "box-path", BoxID: boxID, Value: path}); err != nil {
		logging.LogErrorf("remove blocktree path failed: %s", err)
	}
}

func GetNotExistPaths(boxID string, paths []string) (ret []string) {
	_, ret, err := (indexdb.BlockTrees{DB: db}).PathDifference(context.Background(), boxID, paths)
	if err != nil {
		logging.LogErrorf("blocktree missing paths failed: %s", err)
	}
	return ret
}
func GetRootUpdated() (ret map[string]string) {
	ret, err := (indexdb.BlockTrees{DB: db}).RootUpdated(context.Background())
	if err != nil {
		logging.LogErrorf("blocktree root updated failed: %s", err)
	}
	return ret
}
