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
	"database/sql"
	"os"
	"runtime"
	"runtime/debug"
	"sync"

	"github.com/88250/gulu"
	"github.com/88250/lute/parse"
	"github.com/siyuan-note/logging"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/util"
)

type BlockTree struct {
	ID       string // 块 ID
	RootID   string // 根 ID
	ParentID string // 父 ID
	BoxID    string // 笔记本 ID
	Path     string // 文档数据路径
	HPath    string // 文档可读路径
	Updated  string // 更新时间
	Type     string // 类型
}

var (
	db *sql.DB
)

func initDatabase(forceRebuild bool) (err error) {
	existed := gulu.File.IsExist(util.BlockTreeDBPath)
	initDBConnection()

	if !forceRebuild {
		if !existed {
			forceRebuild = true
		}
	}
	if !forceRebuild {
		return
	}

	closeDatabase()
	if gulu.File.IsExist(util.BlockTreeDBPath) {
		if err = removeDatabaseFile(); err != nil {
			logging.LogErrorf("remove database file [%s] failed: %s", util.BlockTreeDBPath, err)
			err = nil
		}
	}

	initDBConnection()
	initDBTables()

	logging.LogInfof("reinitialized database [%s]", util.BlockTreeDBPath)
	return
}

func initDBTables() {
	if err := indexdb.Reset(context.Background(), db, indexdb.BlockTree, util.DatabaseVer); err != nil {
		logging.LogFatalf(logging.ExitCodeReadOnlyDatabase, "initialize original index failed: %s", err)
	}
}

func initDBConnection() {
	if nil != db {
		closeDatabase()
	}

	util.LogDatabaseSize(util.BlockTreeDBPath)
	var err error
	db, err = indexdb.OpenFile(context.Background(), util.BlockTreeDBPath, 7)
	if err != nil {
		logging.LogFatalf(logging.ExitCodeReadOnlyDatabase, "open original index failed: %s", err)
	}
}

func CloseDatabase() {
	closeDatabase()
}

func closeDatabase() {
	if nil == db {
		return
	}

	if err := db.Close(); err != nil {
		logging.LogErrorf("close database failed: %s", err)
	}
	debug.FreeOSMemory()
	runtime.GC() // 没有这句的话文件句柄不会释放，后面就无法删除文件
	return
}

func removeDatabaseFile() (err error) {
	err = os.RemoveAll(util.BlockTreeDBPath)
	if err != nil {
		return
	}
	err = os.RemoveAll(util.BlockTreeDBPath + "-shm")
	if err != nil {
		return
	}
	err = os.RemoveAll(util.BlockTreeDBPath + "-wal")
	if err != nil {
		return
	}
	return
}

func GetBlockTreesByType(typ string) (ret []*BlockTree) {
	rows, err := (indexdb.BlockTrees{DB: db}).Query(context.Background(), indexdb.BlockTreeFilter{Kind: "type", Value: typ})
	if err != nil {
		logging.LogErrorf("query blocktree failed: %s", err)
		return
	}
	for _, v := range rows {
		row := BlockTree(v)
		ret = append(ret, &row)
	}
	return
}

func GetBlockTreeByPath(path string) (ret *BlockTree) {
	rows, err := (indexdb.BlockTrees{DB: db}).Query(context.Background(), indexdb.BlockTreeFilter{Kind: "path", Value: path})
	if err != nil {
		logging.LogErrorf("query blocktree failed: %s", err)
		return
	}
	if len(rows) > 0 {
		row := BlockTree(rows[0])
		ret = &row
	}
	return
}

func CountTrees() (ret int) {
	ret, err := (indexdb.BlockTrees{DB: db}).Count(context.Background(), indexdb.BlockTreeFilter{Kind: "type", Value: "d"})
	if err != nil {
		logging.LogErrorf("count blocktrees failed: %s", err)
	}
	return ret
}

func CountBlocks() (ret int) {
	ret, err := (indexdb.BlockTrees{DB: db}).Count(context.Background(), indexdb.BlockTreeFilter{Kind: "all"})
	if err != nil {
		logging.LogErrorf("count blocktrees failed: %s", err)
	}
	return ret
}

func GetBlockTreeRootByPath(boxID, path string) (ret *BlockTree) {
	rows, err := (indexdb.BlockTrees{DB: db}).Query(context.Background(), indexdb.BlockTreeFilter{Kind: "root-path", Value: path, BoxID: boxID})
	if err != nil {
		logging.LogErrorf("query blocktree failed: %s", err)
		return
	}
	if len(rows) > 0 {
		row := BlockTree(rows[0])
		ret = &row
	}
	return
}

func GetBlockTreeRootByHPath(boxID, hPath string) (ret *BlockTree) {
	rows, err := (indexdb.BlockTrees{DB: db}).Query(context.Background(), indexdb.BlockTreeFilter{Kind: "root-hpath", Value: hPath, BoxID: boxID})
	if err != nil {
		logging.LogErrorf("query blocktree failed: %s", err)
		return
	}
	if len(rows) > 0 {
		row := BlockTree(rows[0])
		ret = &row
	}
	return
}

func GetBlockTreeRootsByHPath(boxID, hPath string) (ret []*BlockTree) {
	rows, err := (indexdb.BlockTrees{DB: db}).Query(context.Background(), indexdb.BlockTreeFilter{Kind: "root-hpath", Value: hPath, BoxID: boxID})
	if err != nil {
		logging.LogErrorf("query blocktree failed: %s", err)
		return
	}
	for _, v := range rows {
		row := BlockTree(v)
		ret = append(ret, &row)
	}
	return
}

func GetBlockTreeByHPathPreferredParentID(boxID, hPath, preferredParentID string) (ret *BlockTree) {
	rows, err := (indexdb.BlockTrees{DB: db}).Query(context.Background(), indexdb.BlockTreeFilter{Kind: "preferred", Value: hPath, BoxID: boxID, ParentID: preferredParentID})
	if err != nil {
		logging.LogErrorf("query blocktree failed: %s", err)
		return
	}
	if len(rows) > 0 {
		row := BlockTree(rows[0])
		ret = &row
	}
	return
}

func ExistBlockTree(id string) bool {
	rows, err := (indexdb.BlockTrees{DB: db}).Query(context.Background(), indexdb.BlockTreeFilter{Kind: "id", Value: id})
	if err != nil {
		logging.LogErrorf("exist blocktree failed: %s", err)
	}
	return len(rows) > 0
}

func ExistBlockTrees(ids []string) (ret map[string]bool) {
	ret = map[string]bool{}
	for _, id := range ids {
		ret[id] = false
	}
	for id := range GetBlockTrees(ids) {
		ret[id] = true
	}
	return
}

func GetBlockTrees(ids []string) (ret map[string]*BlockTree) {
	ret = map[string]*BlockTree{}
	rows, err := (indexdb.BlockTrees{DB: db}).Query(context.Background(), indexdb.BlockTreeFilter{Kind: "ids", IDs: ids})
	if err != nil {
		logging.LogErrorf("query blocktrees failed: %s", err)
		return
	}
	for _, v := range rows {
		v := BlockTree(v)
		ret[v.ID] = &v
	}
	return
}

func GetBlockTree(id string) (ret *BlockTree) {
	if id == "" {
		return nil
	}
	rows, err := (indexdb.BlockTrees{DB: db}).Query(context.Background(), indexdb.BlockTreeFilter{Kind: "id", Value: id})
	if err != nil {
		logging.LogErrorf("query blocktree failed: %s", err)
		return
	}
	if len(rows) > 0 {
		row := BlockTree(rows[0])
		ret = &row
	}
	return
}

func SetBlockTreePath(tree *parse.Tree) {
	if err := (indexdb.BlockTrees{DB: db}).Replace(context.Background(), tree.ID, indexdb.ProjectBlockTree(tree)); err != nil {
		logging.LogErrorf("replace blocktree path failed: %s", err)
	}
}

func RemoveBlockTreesByRootID(rootID string) {
	if _, err := (indexdb.BlockTrees{DB: db}).Remove(context.Background(), indexdb.BlockTreeFilter{Kind: "root", Value: rootID}); err != nil {
		logging.LogErrorf("remove blocktrees failed: %s", err)
	}
}

func CountBlockTreesByPathPrefix(pathPrefix string) (ret int) {
	ret, err := (indexdb.BlockTrees{DB: db}).Count(context.Background(), indexdb.BlockTreeFilter{Kind: "prefix", Value: pathPrefix})
	if err != nil {
		logging.LogErrorf("count blocktrees failed: %s", err)
	}
	return ret
}

func GetBlockTreesByPathPrefix(pathPrefix string) (ret []*BlockTree) {
	rows, err := (indexdb.BlockTrees{DB: db}).Query(context.Background(), indexdb.BlockTreeFilter{Kind: "prefix", Value: pathPrefix})
	if err != nil {
		logging.LogErrorf("query blocktree failed: %s", err)
		return
	}
	for _, v := range rows {
		row := BlockTree(v)
		ret = append(ret, &row)
	}
	return
}

func GetBlockTreesByRootID(rootID string) (ret []*BlockTree) {
	rows, err := (indexdb.BlockTrees{DB: db}).Query(context.Background(), indexdb.BlockTreeFilter{Kind: "root", Value: rootID})
	if err != nil {
		logging.LogErrorf("query blocktree failed: %s", err)
		return
	}
	for _, v := range rows {
		row := BlockTree(v)
		ret = append(ret, &row)
	}
	return
}

func RemoveBlockTreesByPathPrefix(pathPrefix string) {
	if _, err := (indexdb.BlockTrees{DB: db}).Remove(context.Background(), indexdb.BlockTreeFilter{Kind: "prefix", Value: pathPrefix}); err != nil {
		logging.LogErrorf("remove blocktrees failed: %s", err)
	}
}

func GetBlockTreesByBoxID(boxID string) (ret []*BlockTree) {
	rows, err := (indexdb.BlockTrees{DB: db}).Query(context.Background(), indexdb.BlockTreeFilter{Kind: "box", Value: boxID})
	if err != nil {
		logging.LogErrorf("query blocktree failed: %s", err)
		return
	}
	for _, v := range rows {
		row := BlockTree(v)
		ret = append(ret, &row)
	}
	return
}

func RemoveBlockTreesByBoxID(boxID string) (ids []string) {
	ids, err := (indexdb.BlockTrees{DB: db}).Remove(context.Background(), indexdb.BlockTreeFilter{Kind: "box", Value: boxID})
	if err != nil {
		logging.LogErrorf("remove box blocktrees failed: %s", err)
	}
	return ids
}

func RemoveBlockTree(id string) {
	if _, err := (indexdb.BlockTrees{DB: db}).Remove(context.Background(), indexdb.BlockTreeFilter{Kind: "id", Value: id}); err != nil {
		logging.LogErrorf("remove blocktrees failed: %s", err)
	}
}

var indexBlockTreeLock = sync.Mutex{}

func IndexBlockTree(tree *parse.Tree) {
	indexBlockTreeLock.Lock()
	defer indexBlockTreeLock.Unlock()
	if err := (indexdb.BlockTrees{DB: db}).Index(context.Background(), tree); err != nil {
		logging.LogErrorf("index blocktree failed: %s", err)
	}
}

func UpsertBlockTree(tree *parse.Tree) {
	indexBlockTreeLock.Lock()
	defer indexBlockTreeLock.Unlock()
	if err := (indexdb.BlockTrees{DB: db}).Upsert(context.Background(), tree); err != nil {
		logging.LogErrorf("upsert blocktree failed: %s", err)
	}
}

func InitBlockTree(force bool) {
	err := initDatabase(force)
	if err != nil {
		logging.LogErrorf("init database failed: %s", err)
		os.Exit(logging.ExitCodeReadOnlyDatabase)
		return
	}
	return
}

func CeilTreeCount(count int) int {
	if 100 > count {
		return 100
	}

	for i := 1; i < 40; i++ {
		if count < i*500 {
			return i * 500
		}
	}
	return 500*40 + 1
}

func CeilBlockCount(count int) int {
	if 5000 > count {
		return 5000
	}

	for i := 1; i < 100; i++ {
		if count < i*10000 {
			return i * 10000
		}
	}
	return 10000*100 + 1
}
