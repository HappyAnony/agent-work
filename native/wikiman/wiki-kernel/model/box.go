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
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"
	"wiki-kernel/internal/doclife"
	"wiki-kernel/internal/notebooks"

	"github.com/88250/gulu"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/html"
	"github.com/88250/lute/parse"
	"github.com/siyuan-note/filelock"
	"github.com/siyuan-note/logging"
	"wiki-kernel/cache"
	"wiki-kernel/conf"
	"wiki-kernel/filesys"
	"wiki-kernel/sql"
	"wiki-kernel/treenode"
	"wiki-kernel/util"
)

// Box 笔记本。
type Box struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Icon     string `json:"icon"`
	Sort     int    `json:"sort"`
	SortMode int    `json:"sortMode"`
	Closed   bool   `json:"closed"`

	NewFlashcardCount int `json:"newFlashcardCount"`
	DueFlashcardCount int `json:"dueFlashcardCount"`
	FlashcardCount    int `json:"flashcardCount"`
}

func StatJob() { logOwnedTaskError("workspace statistics", StatJobContext(context.Background())) }

func ListNotebooks() ([]*Box, error) { return ListNotebooksContext(context.Background()) }

func (box *Box) GetConf() (ret *conf.BoxConf) {
	ret = conf.NewBoxConf()

	confPath := filepath.Join(util.DataDir, box.ID, ".siyuan/conf.json")
	if !filelock.IsExist(confPath) {
		return
	}

	data, err := filelock.ReadFile(confPath)
	if err != nil {
		logging.LogErrorf("read box conf [%s] failed: %s", confPath, err)
		return
	}

	if err = gulu.JSON.UnmarshalJSON(data, ret); err != nil {
		logging.LogErrorf("parse box conf [%s] failed: %s", confPath, err)
		return
	}

	icon := ret.Icon
	if strings.Contains(icon, ".") {
		// XSS through emoji name https://github.com/siyuan-note/siyuan/issues/15034
		icon = util.FilterUploadEmojiFileName(icon)
		ret.Icon = icon
	}
	return
}

func (box *Box) SaveConf(conf *conf.BoxConf) {
	confPath := filepath.Join(util.DataDir, box.ID, ".siyuan/conf.json")
	newData, err := gulu.JSON.MarshalIndentJSON(conf, "", "  ")
	if err != nil {
		logging.LogErrorf("marshal box conf [%s] failed: %s", confPath, err)
		return
	}

	oldData, err := filelock.ReadFile(confPath)
	if err != nil {
		box.saveConf0(newData)
		return
	}

	if bytes.Equal(newData, oldData) {
		return
	}

	box.saveConf0(newData)
}

func (box *Box) saveConf0(data []byte) {
	confPath := filepath.Join(util.DataDir, box.ID, ".siyuan/conf.json")
	if err := os.MkdirAll(filepath.Join(util.DataDir, box.ID, ".siyuan"), 0755); err != nil {
		logging.LogErrorf("save box conf [%s] failed: %s", confPath, err)
	}
	if err := filelock.WriteFile(confPath, data); err != nil {
		logging.LogErrorf("write box conf [%s] failed: %s", confPath, err)
		util.ReportFileSysFatalError(err)
		return
	}
}

func (box *Box) Ls(p string) (ret []*FileInfo, totals int, err error) {
	boxLocalPath := filepath.Join(util.DataDir, box.ID)
	if util.HasDocExt(p) {
		dir := util.StripDocExt(p)
		absDir := filepath.Join(boxLocalPath, dir)
		if gulu.File.IsDir(absDir) {
			p = dir
		} else {
			return
		}
	}

	entries, err := os.ReadDir(filepath.Join(util.DataDir, box.ID, p))
	if err != nil {
		return
	}

	for _, f := range entries {
		info, infoErr := f.Info()
		if nil != infoErr {
			logging.LogErrorf("read file info failed: %s", infoErr)
			continue
		}

		name := f.Name()
		if util.IsReservedFilename(name) {
			continue
		}
		if strings.HasSuffix(name, ".tmp") {
			// 移除写入失败时产生的并且早于 30 分钟前的临时文件，近期创建的临时文件可能正在写入中
			removePath := filepath.Join(util.DataDir, box.ID, p, name)
			if info.ModTime().Before(time.Now().Add(-30 * time.Minute)) {
				if removeErr := os.Remove(removePath); nil != removeErr {
					logging.LogWarnf("remove tmp file [%s] failed: %s", removePath, removeErr)
				}
			}
			continue
		}

		totals += 1
		fi := &FileInfo{}
		fi.name = name
		fi.isdir = f.IsDir()
		fi.size = info.Size()
		fPath := path.Join(p, name)
		if f.IsDir() {
			fPath += "/"
		}
		fi.path = fPath
		ret = append(ret, fi)
	}
	return
}

func (box *Box) Stat(p string) (ret *FileInfo) {
	absPath := filepath.Join(util.DataDir, box.ID, p)
	info, err := os.Stat(absPath)
	if err != nil {
		if !os.IsNotExist(err) {
			logging.LogErrorf("stat [%s] failed: %s", absPath, err)
		}
		return
	}
	ret = &FileInfo{
		path:  p,
		name:  info.Name(),
		size:  info.Size(),
		isdir: info.IsDir(),
	}
	return
}

func (box *Box) Exist(p string) bool {
	return filelock.IsExist(filepath.Join(util.DataDir, box.ID, p))
}

func (box *Box) Mkdir(path string) error {
	if err := os.Mkdir(filepath.Join(util.DataDir, box.ID, path), 0755); err != nil {
		msg := fmt.Sprintf(Conf.Language(6), box.Name, path, err)
		logging.LogErrorf("mkdir [path=%s] in box [%s] failed: %s", path, box.ID, err)
		return errors.New(msg)
	}
	IncSync()
	return nil
}

func (box *Box) MkdirAll(path string) error {
	if err := os.MkdirAll(filepath.Join(util.DataDir, box.ID, path), 0755); err != nil {
		msg := fmt.Sprintf(Conf.Language(6), box.Name, path, err)
		logging.LogErrorf("mkdir all [path=%s] in box [%s] failed: %s", path, box.ID, err)
		return errors.New(msg)
	}
	IncSync()
	return nil
}

func (box *Box) Move(oldPath, newPath string) error {
	boxLocalPath := filepath.Join(util.DataDir, box.ID)
	fromPath := filepath.Join(boxLocalPath, oldPath)
	toPath := filepath.Join(boxLocalPath, newPath)

	if err := filelock.Rename(fromPath, toPath); err != nil {
		msg := fmt.Sprintf(Conf.Language(5), box.Name, fromPath, err)
		logging.LogErrorf("move [path=%s] in box [%s] failed: %s", fromPath, box.Name, err)
		return errors.New(msg)
	}

	if oldDir := path.Dir(oldPath); ast.IsNodeIDPattern(path.Base(oldDir)) {
		fromDir := filepath.Join(boxLocalPath, oldDir)
		if util.IsEmptyDir(fromDir) {
			filelock.Remove(fromDir)
		}
	}
	IncSync()
	return nil
}

func (box *Box) Remove(path string) error {
	boxLocalPath := filepath.Join(util.DataDir, box.ID)
	filePath := filepath.Join(boxLocalPath, path)
	if err := filelock.Remove(filePath); err != nil {
		msg := fmt.Sprintf(Conf.Language(7), box.Name, path, err)
		logging.LogErrorf("remove [path=%s] in box [%s] failed: %s", path, box.ID, err)
		return errors.New(msg)
	}
	IncSync()
	return nil
}

func (box *Box) ListFiles(path string) (ret []*FileInfo) {
	fis, _, err := box.Ls(path)
	if err != nil {
		return
	}
	box.listFiles(&fis, &ret)
	return
}

func (box *Box) listFiles(files, ret *[]*FileInfo) {
	for _, file := range *files {
		if file.isdir {
			fis, _, err := box.Ls(file.path)
			if err == nil {
				box.listFiles(&fis, ret)
			}
			*ret = append(*ret, file)
		} else {
			*ret = append(*ret, file)
		}
	}
	return
}

type BoxInfo = notebooks.Info

func (box *Box) GetInfo() *BoxInfo {
	v, e := NotebookInfoContext(context.Background(), box.ID)
	if e != nil {
		logOwnedTaskError("notebook info", e)
	}
	return v
}

func isSkipFile(filename string) bool {
	return strings.HasPrefix(filename, ".") || "node_modules" == filename || "dist" == filename || "target" == filename
}

func moveTree(tree *parse.Tree) {
	treenode.SetBlockTreePath(tree)

	if hidden := tree.Root.IALAttr("custom-hidden"); "true" == hidden {
		tree.Root.RemoveIALAttr("custom-hidden")
		filesys.WriteTree(tree)
	}

	sql.RemoveTreeQueue(tree.ID)
	sql.IndexTreeQueue(tree)

	box := Conf.Box(tree.Box)
	box.renameSubTrees(tree)

	refreshDocInfo(tree)
}

func (box *Box) renameSubTrees(tree *parse.Tree) {
	subFiles := box.ListFiles(tree.Path)

	luteEngine := util.NewLute()
	for _, subFile := range subFiles {
		if !util.HasDocExt(subFile.path) {
			continue
		}

		subTree, err := filesys.LoadTree(box.ID, subFile.path, luteEngine) // LoadTree 会重新构造 HPath
		if err != nil {
			continue
		}

		treenode.SetBlockTreePath(subTree)
		sql.RenameSubTreeQueue(subTree)
		msg := fmt.Sprintf(Conf.Language(107), html.EscapeString(subTree.HPath))
		util.PushStatusBar(msg)
	}
}

func parseKTree(kramdown []byte) (ret *parse.Tree) {
	luteEngine := NewLute()
	ret = parse.Parse("", kramdown, luteEngine.ParseOptions)
	normalizeTree(ret)
	return
}

func normalizeTree(tree *parse.Tree) (string, string, string) {
	a, b, c, err := doclife.NormalizeCreationTemplate(context.Background(), tree, doclife.TemplateBodyServices{NewID: ast.NewNodeID, Now: time.Now, Warning: func(e error) { logging.LogWarnf("normalize tree: %s", e) }})
	if err != nil {
		logging.LogWarnf("normalize tree: %s", err)
	}
	return a, b, c
}

func VacuumDataIndex() {
	logOwnedTaskError("vacuum indexes", VacuumDataIndexContext(context.Background()))
}

func FullReindex() {
	if err := FullReindexContext(context.Background()); err != nil {
		logging.LogErrorf("full notebook rebuild not admitted: %s", err)
	}
}

func FullReindexContext(ctx context.Context) error {
	if Conf == nil {
		return conf.ErrBusinessUnbound
	}
	return Conf.scheduleFullNotebookReindexContext(ctx)
}

// The restore handler already owns the serial legacy task slot. Execute the
// original rebuild stages here rather than enqueueing children behind itself.
func reindexAfterCheckout(ctx context.Context) (err error) {
	if e := reindexOriginalData(ctx); e != nil {
		return e
	}
	r, e := captureIndexRepair(ctx)
	if e != nil {
		return e
	}
	defer func() { err = errors.Join(err, indexrepairDrain(ctx, r.queue)) }()
	if e = r.indexReferences(ctx); e != nil {
		return e
	}
	if e := r.queue.Flush(ctx); e != nil {
		return e
	}
	if e := ResetVirtualBlockRefCacheContext(ctx); e != nil {
		return e
	}
	if e := r.autoIndexEmbedBlocks(ctx); e != nil {
		return e
	}
	if e := r.queue.Flush(ctx); e != nil {
		return e
	}
	cache.ClearDocsIAL()
	cache.ClearBlocksIAL()
	return ctx.Err()
}

func reindexOriginalData(ctx context.Context) (err error) {
	pushSQLInsertBlocksFTSMsg.Store(true)
	pushSQLDeleteBlocksMsg.Store(true)
	defer func() {
		err = errors.Join(err, sql.FlushQueueContext(context.WithoutCancel(ctx)))
		pushSQLInsertBlocksFTSMsg.Store(false)
		pushSQLDeleteBlocksMsg.Store(false)
	}()

	util.PushEndlessProgress(Conf.language(35))
	defer util.PushClearProgress()

	if e := FlushTxQueueContext(ctx); e != nil {
		return e
	}

	if err := sql.InitDatabase(true); err != nil {
		return err
	}

	sql.IndexIgnoreCached = false
	openedBoxes, e := Conf.notebooks(ctx)
	if e != nil {
		return e
	}
	for _, openedBox := range openedBoxes {
		if openedBox.Closed {
			continue
		}
		if e := ctx.Err(); e != nil {
			return e
		}
		r, e := captureIndexRepair(ctx)
		if e != nil {
			return e
		}
		if e = r.indexNotebook(ctx, openedBox.ID); e != nil {
			return e
		}
	}
	LoadFlashcards()
	debug.FreeOSMemory()
	return nil
}

func ChangeBoxSort(ids []string) {
	logOwnedTaskError("notebook sort", ChangeBoxSortContext(context.Background(), ids))
}
func SetBoxIcon(id, icon string) {
	logOwnedTaskError("notebook icon", SetBoxIconContext(context.Background(), id, icon))
}

func (box *Box) UpdateHistoryGenerated() {
	if Conf != nil {
		Conf.historyGenerated.Store(box.ID, time.Now())
	}
}

func getBoxesByPaths(paths []string) (ret map[string]*Box) {
	ret = map[string]*Box{}
	var ids []string
	for _, p := range paths {
		ids = append(ids, util.GetTreeID(p))
	}

	bts := treenode.GetBlockTrees(ids)
	for _, id := range ids {
		bt := bts[id]
		if nil != bt {
			ret[bt.Path] = Conf.Box(bt.BoxID)
		}
	}
	return
}
