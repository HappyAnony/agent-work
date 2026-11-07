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
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/88250/gulu"
	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"github.com/siyuan-note/filelock"
	"github.com/siyuan-note/logging"
	"wiki-kernel/cache"
	"wiki-kernel/conf"
	"wiki-kernel/internal/doclife"
	"wiki-kernel/internal/taskgroup"
	"wiki-kernel/sql"
	"wiki-kernel/task"
	"wiki-kernel/treenode"
	"wiki-kernel/util"
)

// History is explicitly started before returning to boot. No timer or worker
// exists at import time, and Stop cannot miss a not-yet-scheduled goroutine.
var historyCtxMu sync.Mutex
var historyOwner *taskgroup.Group
var historyTicker *time.Ticker

type legacyAutoHistoryOwner struct {
	owner                                   *AppConf
	workspace, data, history, temp, working string
	interval, retention                     int
}

func captureLegacyAutoHistoryOwner(ctx context.Context) (*legacyAutoHistoryOwner, error) {
	if ctx == nil {
		return nil, conf.ErrBusinessUnbound
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	o := &legacyAutoHistoryOwner{owner: Conf, workspace: util.WorkspaceDir, data: util.DataDir, history: util.HistoryDir, temp: util.TempDir, working: util.WorkingDir}
	if o.owner == nil {
		return nil, conf.ErrBusinessUnbound
	}
	if err := o.owner.WithSettingSnapshot(ctx, func(v conf.AppData) error {
		if v.Editor == nil {
			return conf.ErrBusinessUnbound
		}
		o.interval, o.retention = v.Editor.GenerateHistoryInterval, v.Editor.HistoryRetentionDays
		return nil
	}); err != nil {
		return nil, err
	}
	if err := o.guard(ctx); err != nil {
		return nil, err
	}
	return o, nil
}

func (o *legacyAutoHistoryOwner) current() bool {
	return o != nil && o.owner != nil && o.owner == Conf && o.workspace == util.WorkspaceDir && o.data == util.DataDir && o.history == util.HistoryDir && o.temp == util.TempDir && o.working == util.WorkingDir
}

func (o *legacyAutoHistoryOwner) guard(ctx context.Context) error {
	if ctx == nil {
		return conf.ErrBusinessUnbound
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if !o.current() {
		return conf.ErrBusinessUnbound
	}
	o.owner.repoMu.Lock()
	closed := o.owner.repoClosed
	o.owner.repoMu.Unlock()
	if closed {
		return conf.ErrBusinessUnbound
	}
	return nil
}

func (o *legacyAutoHistoryOwner) run(ctx context.Context, fn func(context.Context, *legacyAutoHistoryOwner) error) error {
	if fn == nil {
		return conf.ErrBusinessUnbound
	}
	if err := o.guard(ctx); err != nil {
		return err
	}
	done := make(chan error, 1)
	o.owner.repoMu.Lock()
	if o.owner.repoClosed || !o.current() || ctx.Err() != nil {
		o.owner.repoMu.Unlock()
		return conf.ErrBusinessUnbound
	}
	if o.owner.repoTasks == nil {
		o.owner.repoTasks = taskgroup.Open()
	}
	accepted := o.owner.repoTasks.Go(func(life context.Context) {
		merged, cancel := context.WithCancel(life)
		stop := context.AfterFunc(ctx, cancel)
		defer stop()
		defer cancel()
		done <- fn(merged, o)
	})
	o.owner.repoMu.Unlock()
	if !accepted {
		return conf.ErrBusinessUnbound
	}
	return <-done
}

func enqueueAutoFileHistory(ctx context.Context, owner *legacyAutoHistoryOwner, group *taskgroup.Group, generate func(context.Context, *legacyAutoHistoryOwner) error) error {
	if owner == nil || group == nil || generate == nil {
		return conf.ErrBusinessUnbound
	}
	h, err := task.AppendUniqueContext(ctx, task.HistoryGenerateFile, func(request context.Context) error {
		return owner.run(request, generate)
	})
	if errors.Is(err, task.ErrDuplicate) {
		return nil
	}
	if err != nil {
		return err
	}
	// The ticker owner retains the task handle until the actual handler exits.
	// A caller timeout or cancellation never turns a live history writer into
	// detached work during workspace teardown.
	if !group.Go(func(life context.Context) {
		stop := context.AfterFunc(life, h.Cancel)
		defer stop()
		_ = h.Join(context.Background())
	}) {
		h.Cancel()
		return conf.ErrBusinessUnbound
	}
	return nil
}

func AutoGenerateFileHistory() {
	historyCtxMu.Lock()
	defer historyCtxMu.Unlock()
	if historyOwner != nil {
		select {
		case <-historyOwner.Done():
		default:
			return
		}
	}
	o, err := captureLegacyAutoHistoryOwner(context.Background())
	if err != nil {
		return
	}
	minutes := o.interval
	if minutes <= 0 {
		minutes = 3600
	}
	ticker := time.NewTicker(time.Minute * time.Duration(minutes))
	historyTicker = ticker
	group := taskgroup.Open()
	historyOwner = group
	group.Go(func(ctx context.Context) {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if ctx.Err() != nil {
					return
				}
				_ = enqueueAutoFileHistory(ctx, o, group, generateFileHistoryForOwner)
			}
		}
	})
}

func StopAutoGenerateFileHistory() {
	historyCtxMu.Lock()
	defer historyCtxMu.Unlock()
	if historyOwner != nil {
		historyOwner.Stop()
	}
	if historyTicker != nil {
		historyTicker.Stop()
		historyTicker = nil
	}
}

func DrainAutoGenerateFileHistory() {
	_ = DrainAutoGenerateFileHistoryContext(context.Background())
}
func DrainAutoGenerateFileHistoryContext(ctx context.Context) error {
	historyCtxMu.Lock()
	owner := historyOwner
	historyCtxMu.Unlock()
	if owner == nil {
		return nil
	}
	return owner.Close(ctx)
}

func generateFileHistory() {
	o, err := captureLegacyAutoHistoryOwner(context.Background())
	if err != nil {
		return
	}
	_ = o.run(context.Background(), generateFileHistoryForOwner)
}

func generateFileHistoryForOwner(ctx context.Context, owner *legacyAutoHistoryOwner) error {
	defer logging.Recover()

	if err := owner.guard(ctx); err != nil {
		return err
	}
	interval, retention := owner.interval, owner.retention
	if err := owner.owner.WithSettingSnapshot(ctx, func(v conf.AppData) error {
		if v.Editor == nil {
			return conf.ErrBusinessUnbound
		}
		interval, retention = v.Editor.GenerateHistoryInterval, v.Editor.HistoryRetentionDays
		return nil
	}); err != nil {
		return err
	}
	if 1 > interval {
		return nil
	}

	FlushTxQueue()
	if err := owner.guard(ctx); err != nil {
		return err
	}

	// 生成文档历史
	for _, box := range owner.owner.GetOpenedBoxes() {
		if err := owner.guard(ctx); err != nil {
			return err
		}
		box.generateDocHistory0()
	}

	// 生成资源文件历史
	if err := owner.guard(ctx); err != nil {
		return err
	}
	generateAssetsHistory()

	if err := owner.guard(ctx); err != nil {
		return err
	}
	clearOutdatedHistoryDirWithRetention(owner.history, retention)

	// 以下部分是老版本的历史数据，不再保留
	for _, box := range owner.owner.GetBoxes() {
		if err := owner.guard(ctx); err != nil {
			return err
		}
		historyDir := filepath.Join(owner.data, box.ID, ".siyuan", "history")
		os.RemoveAll(historyDir)
	}
	if err := owner.guard(ctx); err != nil {
		return err
	}
	historyDir := filepath.Join(owner.data, "assets", ".siyuan", "history")
	os.RemoveAll(historyDir)
	historyDir = filepath.Join(owner.data, ".siyuan", "history")
	os.RemoveAll(historyDir)
	return owner.guard(ctx)
}

func ChangeHistoryTick(minutes int) {
	historyCtxMu.Lock()
	defer historyCtxMu.Unlock()
	if 0 >= minutes {
		minutes = 3600
	}
	if historyTicker != nil {
		historyTicker.Reset(time.Minute * time.Duration(minutes))
	}
}

func ClearWorkspaceHistory() error { return ClearWorkspaceHistoryContext(context.Background()) }

func GetDocHistoryContent(historyPath, keyword string, highlight bool) (string, string, string, bool, error) {
	return GetDocHistoryContentContext(context.Background(), historyPath, keyword, highlight)
}

func RollbackDocHistory(boxID, historyPath string) error {
	return RollbackDocHistoryContext(context.Background(), boxID, historyPath)
}

func RollbackDocHistoryContext(ctx context.Context, boxID, historyPath string) error {
	_, err := runLegacyDocumentLifecycle(ctx, doclife.Request{Action: "restore", Box: boxID, HistoryPath: historyPath})
	return err
}

func getRollbackDockPath(boxID, historyPath string, workingDoc *treenode.BlockTree) (destPath, parentHPath string, err error) {
	var parentID string
	baseName := filepath.Base(historyPath)
	var parentWorkingDoc *treenode.BlockTree
	if nil != workingDoc {
		parentID = path.Base(path.Dir(workingDoc.Path))
		parentWorkingDoc = treenode.GetBlockTree(parentID)
	} else {
		parentID = filepath.Base(filepath.Dir(historyPath))
		parentWorkingDoc = treenode.GetBlockTree(parentID)
	}

	if nil != parentWorkingDoc {
		// 父路径如果是文档，则恢复到父路径下
		parentDir := util.StripDocExt(parentWorkingDoc.Path)
		parentDir = filepath.Join(util.DataDir, boxID, parentDir)
		if err = os.MkdirAll(parentDir, 0755); err != nil {
			return
		}
		destPath = filepath.Join(parentDir, baseName)
		parentHPath = parentWorkingDoc.HPath
	} else {
		// 父路径如果不是文档，则恢复到笔记本根路径下
		destPath = filepath.Join(util.DataDir, boxID, baseName)
	}
	return
}

func RollbackAssetsHistory(name string) error {
	return RollbackAssetsHistoryContext(context.Background(), name)
}
func RollbackNotebookHistory(name string) error {
	return RollbackNotebookHistoryContext(context.Background(), name)
}

type History struct {
	HCreated string         `json:"hCreated"`
	Items    []*HistoryItem `json:"items"`
}

type HistoryItem struct {
	Title    string `json:"title"`
	Path     string `json:"path"`
	Op       string `json:"op"`
	Notebook string `json:"notebook"` // 仅用于文档历史
}

func FullTextSearchHistory(query, box, op string, typ, page int) ([]string, int, int) {
	a, b, c, e := FullTextSearchHistoryContext(context.Background(), query, box, op, typ, page)
	if e != nil {
		logging.LogErrorf("history search: %s", e)
	}
	return a, b, c
}
func FullTextSearchHistoryItems(created, query, box, op string, typ int) []*HistoryItem {
	out, e := FullTextSearchHistoryItemsContext(context.Background(), created, query, box, op, typ)
	if e != nil {
		logging.LogErrorf("history items: %s", e)
	}
	return out
}

func GetNotebookHistory() ([]*History, error) { return GetNotebookHistoryContext(context.Background()) }

func generateAssetsHistory() {
	assets := recentModifiedAssets()
	if 1 > len(assets) {
		return
	}

	historyDir, err := GetHistoryDir(HistoryOpUpdate)
	if err != nil {
		logging.LogErrorf("get history dir failed: %s", err)
		return
	}

	for _, file := range assets {
		historyPath := filepath.Join(historyDir, "assets", strings.TrimPrefix(file, filepath.Join(util.DataDir, "assets")))
		if err = os.MkdirAll(filepath.Dir(historyPath), 0755); err != nil {
			logging.LogErrorf("generate history failed: %s", err)
			return
		}

		if err = filelock.Copy(file, historyPath); err != nil {
			logging.LogErrorf("copy file [%s] to [%s] failed: %s", file, historyPath, err)
			return
		}
	}

	indexHistoryDir(filepath.Base(historyDir), util.NewLute())
	return
}

func (box *Box) generateDocHistory0() {
	files := box.recentModifiedDocs()
	if 1 > len(files) {
		return
	}

	historyDir, err := GetHistoryDir(HistoryOpUpdate)
	if err != nil {
		logging.LogErrorf("get history dir failed: %s", err)
		return
	}

	luteEngine := util.NewLute()
	for _, file := range files {
		historyPath := filepath.Join(historyDir, box.ID, strings.TrimPrefix(file, filepath.Join(util.DataDir, box.ID)))
		if err = os.MkdirAll(filepath.Dir(historyPath), 0755); err != nil {
			logging.LogErrorf("generate history failed: %s", err)
			return
		}

		var data []byte
		if data, err = filelock.ReadFile(file); err != nil {
			logging.LogErrorf("generate history failed: %s", err)
			return
		}

		if err = gulu.File.WriteFileSafer(historyPath, data, 0644); err != nil {
			logging.LogErrorf("generate history failed: %s", err)
			return
		}

		if util.HasDocExt(file) {
			tree, loadErr := loadTree(file, luteEngine)
			if nil != loadErr {
				logging.LogErrorf("load tree [%s] failed: %s", file, loadErr)
			} else {
				generateAvHistory(tree, historyDir)
			}
		}
	}

	indexHistoryDir(filepath.Base(historyDir), util.NewLute())
	return
}

func clearOutdatedHistoryDir(historyDir string) {
	retention := 0
	if Conf != nil && Conf.Editor != nil {
		retention = Conf.Editor.HistoryRetentionDays
	}
	clearOutdatedHistoryDirWithRetention(historyDir, retention)
}

func clearOutdatedHistoryDirWithRetention(historyDir string, retention int) {
	if !gulu.File.IsExist(historyDir) {
		return
	}

	dirs, err := os.ReadDir(historyDir)
	if err != nil {
		logging.LogErrorf("clear history [%s] failed: %s", historyDir, err)
		return
	}

	now := time.Now()
	ago := now.Add(-24 * time.Hour * time.Duration(retention)).Unix()
	var removes []string
	for _, dir := range dirs {
		dirInfo, err := dir.Info()
		if err != nil {
			logging.LogErrorf("read history dir [%s] failed: %s", dir.Name(), err)
			continue
		}
		if dirInfo.ModTime().Unix() < ago {
			removes = append(removes, filepath.Join(historyDir, dir.Name()))
		}
	}
	for _, dir := range removes {
		if err = os.RemoveAll(dir); err != nil {
			logging.LogWarnf("remove history dir [%s] failed: %s", dir, err)
			continue
		}
		//logging.LogInfof("auto removed history dir [%s]", dir)
	}

	// 清理历史库
	sql.DeleteOutdatedHistories(ago)
}

func (box *Box) recentModifiedDocs() (ret []string) {
	owner := Conf
	latestHistoryTime := time.Time{}
	if v, ok := owner.historyGenerated.Load(box.ID); ok {
		latestHistoryTime = v.(time.Time)
	}
	filelock.Walk(filepath.Join(util.DataDir, box.ID), func(path string, d fs.DirEntry, err error) error {
		if nil != err || nil == d {
			return nil
		}
		if isSkipFile(d.Name()) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if d.IsDir() {
			return nil
		}

		info, err := d.Info()
		if nil != err {
			return err
		}

		if info.ModTime().After(latestHistoryTime) {
			ret = append(ret, path)
		}
		return nil
	})
	owner.historyGenerated.Store(box.ID, time.Now())
	return
}

var assetsLatestHistoryTime = time.Now().Unix()

func recentModifiedAssets() (ret []string) {
	assets := cache.GetAssets()
	for _, asset := range assets {
		if asset.Updated > assetsLatestHistoryTime {
			absPath := filepath.Join(util.DataDir, asset.Path)
			if filelock.IsHidden(absPath) {
				continue
			}
			ret = append(ret, absPath)
		}
	}
	assetsLatestHistoryTime = time.Now().Unix()
	return
}

const (
	HistoryOpClean   = "clean"
	HistoryOpUpdate  = "update"
	HistoryOpDelete  = "delete"
	HistoryOpFormat  = "format"
	HistoryOpSync    = "sync"
	HistoryOpReplace = "replace"
	HistoryOpOutline = "outline"
)

func generateOpTypeHistory(tree *parse.Tree, opType string) {
	historyDir, err := GetHistoryDir(opType)
	if err != nil {
		logging.LogErrorf("get history dir failed: %s", err)
		return
	}

	historyPath := filepath.Join(historyDir, tree.Box, tree.Path)
	if err = os.MkdirAll(filepath.Dir(historyPath), 0755); err != nil {
		logging.LogErrorf("generate history failed: %s", err)
		return
	}

	var data []byte
	if data, err = filelock.ReadFile(filepath.Join(util.DataDir, tree.Box, tree.Path)); err != nil {
		logging.LogErrorf("generate history failed: %s", err)
		return
	}

	if err = gulu.File.WriteFileSafer(historyPath, data, 0644); err != nil {
		logging.LogErrorf("generate history failed: %s", err)
		return
	}

	generateAvHistory(tree, historyDir)

	indexHistoryDir(filepath.Base(historyDir), util.NewLute())
}

func generateAvHistory(tree *parse.Tree, historyDir string) {
	avNodes := tree.Root.ChildrenByType(ast.NodeAttributeView)
	for _, avNode := range avNodes {
		srcAvPath := filepath.Join(util.DataDir, "storage", "av", avNode.AttributeViewID+".json")
		destAvPath := filepath.Join(historyDir, "storage", "av", avNode.AttributeViewID+".json")
		if copyErr := filelock.Copy(srcAvPath, destAvPath); nil != copyErr {
			logging.LogErrorf("copy av [%s] failed: %s", srcAvPath, copyErr)
		}
	}
}

func GetHistoryDir(suffix string) (ret string, err error) {
	return getHistoryDir(suffix, time.Now())
}

func getHistoryDir(suffix string, t time.Time) (ret string, err error) {
	ret = filepath.Join(util.HistoryDir, t.Format("2006-01-02-150405")+"-"+suffix)
	if err = os.MkdirAll(ret, 0755); err != nil {
		logging.LogErrorf("make history dir failed: %s", err)
		return
	}
	return
}

func ReindexHistory() {
	logOwnedTaskError("history reindex admission", ReindexHistoryContext(context.Background()))
}

func ReindexHistoryContext(ctx context.Context) error {
	_, err := startHistoryReindex(ctx)
	return err
}

var validOps = []string{HistoryOpClean, HistoryOpUpdate, HistoryOpDelete, HistoryOpFormat, HistoryOpSync, HistoryOpReplace, HistoryOpOutline}

const (
	HistoryTypeDocName = 0 // Search docs by doc name
	HistoryTypeDoc     = 1 // Search docs by doc name and content
	HistoryTypeAsset   = 2 // Search assets
	HistoryTypeDocID   = 3 // Search docs by doc id
)

func indexHistoryDir(name string, engine *lute.Lute) {
	if e := indexHistoryDirContext(context.Background(), name, engine); e != nil {
		logging.LogErrorf("index history directory failed: %s", e)
	}
}

func subscribeSQLHistoryEvents() {
	subscribeLegacyEvent(util.EvtSQLHistoryRebuild, func(payload map[string]interface{}) {
		withLegacySQLEvent(payload, func(ctx context.Context, _ *legacySQLEventSession) {
			if err := ReindexHistoryContext(ctx); err != nil && ctx.Err() == nil {
				logging.LogErrorf("history reindex admission failed: %s", err)
			}
		})
	})
}
