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
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/88250/gulu"
	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"github.com/siyuan-note/filelock"
	"github.com/siyuan-note/logging"
	"wiki-kernel/cache"
	"wiki-kernel/filesys"
	"wiki-kernel/internal/doclife"
	"wiki-kernel/internal/docquery"
	"wiki-kernel/internal/doctree"
	"wiki-kernel/sql"
	"wiki-kernel/treenode"
	"wiki-kernel/util"
)

type File = doctree.File

func (box *Box) docFromFileInfo(fileInfo *FileInfo, ial map[string]string) *File {
	ret, err := doctree.Metadata(context.Background(), doctree.Entry{Path: fileInfo.path, Name: fileInfo.name, Size: fileInfo.size, Directory: fileInfo.isdir}, ial, time.Local, func(t time.Time) (string, error) { return util.HumanizeTime(t, Conf.Lang), nil })
	if err != nil {
		logging.LogErrorf("document metadata failed: %s", err)
		return nil
	}
	return ret
}

func (box *Box) docIAL(p string) (ret map[string]string) {
	name := strings.ToLower(filepath.Base(p))
	if !util.HasDocExt(name) {
		return nil
	}

	ret = cache.GetDocIAL(p)
	if nil != ret {
		return ret
	}

	filePath := filepath.Join(util.DataDir, box.ID, p)
	ret = filesys.DocIAL(filePath)
	if 1 > len(ret) {
		logging.LogWarnf("properties not found in file [%s]", filePath)
		box.moveCorruptedData(filePath)
		return nil
	}
	cache.PutDocIAL(p, ret)
	return ret
}

func (box *Box) moveCorruptedData(filePath string) {
	base := filepath.Base(filePath)
	to := filepath.Join(util.WorkspaceDir, "corrupted", time.Now().Format("2006-01-02-150405"), box.ID, base)
	if copyErr := filelock.Copy(filePath, to); nil != copyErr {
		logging.LogErrorf("copy corrupted data file [%s] failed: %s", filePath, copyErr)
		return
	}
	if removeErr := filelock.Remove(filePath); nil != removeErr {
		logging.LogErrorf("remove corrupted data file [%s] failed: %s", filePath, removeErr)
		return
	}
	logging.LogWarnf("moved corrupted data file [%s] to [%s]", filePath, to)
}

func SearchDocsByKeyword(keyword string, flashcard bool, excludeIDs []string) []map[string]string {
	ret, err := SearchDocsByKeywordContext(context.Background(), keyword, flashcard, excludeIDs)
	if err != nil {
		logging.LogErrorf("document search failed: %s", err)
		return []map[string]string{}
	}
	return ret
}

type FileInfo struct {
	path  string
	name  string
	size  int64
	isdir bool
}

func ListDocTree(boxID, listPath string, sortMode int, flashcard, showHidden bool, maxListCount int) ([]*File, int, error) {
	return ListDocTreeContext(context.Background(), boxID, listPath, sortMode, flashcard, showHidden, maxListCount)
}

func GetDoc(startID, endID, id string, index int, query string, queryTypes map[string]bool, queryMethod, mode, size int, isBacklink bool, refs map[string]string, highlight bool) (int, string, string, string, string, string, bool, bool, string, string, bool, []string, error) {
	return GetDocContext(context.Background(), startID, endID, id, index, query, queryTypes, queryMethod, mode, size, isBacklink, refs, highlight)
}
func loadNodesByStartEnd(tree *parse.Tree, start, end string) ([]*ast.Node, bool) {
	nodes, eof, _ := loadNodesByStartEndWithContext(context.Background(), tree, start, end, Conf.Editor.DynamicLoadBlocks)
	return nodes, eof
}
func loadNodesByMode(node *ast.Node, index, mode, size int, isDoc, isHeading bool) ([]*ast.Node, bool) {
	nodes, eof, _ := loadNodesByModeWithContext(context.Background(), node, index, mode, size, isDoc, isHeading)
	return nodes, eof
}

func writeTreeUpsertQueue(tree *parse.Tree) (err error) {
	size, err := filesys.WriteTree(tree)
	if err != nil {
		return
	}
	sql.UpsertTreeQueue(tree)
	refreshDocInfoWithSize(tree, size)
	return
}

func indexWriteTreeIndexQueue(tree *parse.Tree) (err error) {
	treenode.IndexBlockTree(tree)
	_, err = filesys.WriteTree(tree)
	if err != nil {
		return
	}
	sql.IndexTreeQueue(tree)
	return
}

func indexWriteTreeUpsertQueue(tree *parse.Tree) (err error) {
	treenode.UpsertBlockTree(tree)
	return writeTreeUpsertQueue(tree)
}

func renameWriteJSONQueue(tree *parse.Tree) (err error) {
	size, err := filesys.WriteTree(tree)
	if err != nil {
		return
	}
	sql.RenameTreeQueue(tree)
	treenode.UpsertBlockTree(tree)
	refreshDocInfoWithSize(tree, size)
	return
}

func DuplicateDoc(tree *parse.Tree) error {
	if tree == nil {
		return doclife.ErrMissing
	}
	next, err := DuplicateDocContext(context.Background(), tree.ID)
	if err == nil {
		*tree = *next
	}
	return err
}

func createTreeTx(tree *parse.Tree) error {
	return createTreeTxContext(context.Background(), tree)
}

func createTreeTxContext(ctx context.Context, tree *parse.Tree) error {
	transaction := &Transaction{DoOperations: []*Operation{{Action: "create", Data: tree}}}
	return PerformTransactionsContext(ctx, &[]*Transaction{transaction})
}

type contextMutex chan struct{}

func (m contextMutex) Lock()   { m <- struct{}{} }
func (m contextMutex) Unlock() { <-m }
func (m contextMutex) TryLock() bool {
	select {
	case m <- struct{}{}:
		return true
	default:
		return false
	}
}
func (m contextMutex) LockContext(ctx context.Context) error {
	if ctx == nil {
		return errors.New("document_creation_context_unbound")
	}
	select {
	case m <- struct{}{}:
		if err := ctx.Err(); err != nil {
			<-m
			return err
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

var createDocLock = make(contextMutex, 1)

func CreateDocByMd(boxID, p, title, md string, sorts []string) (*parse.Tree, error) {
	return CreateDocByMdContext(context.Background(), boxID, p, title, md, sorts)
}

func CreateDocByMdContext(ctx context.Context, boxID, p, title, md string, sorts []string) (*parse.Tree, error) {
	if err := createDocLock.LockContext(ctx); err != nil {
		return nil, err
	}
	defer createDocLock.Unlock()
	plan, err := runLegacyDocumentLifecycle(ctx, doclife.Request{Action: "create", Box: boxID, Path: p, Title: title, Markdown: md, Sorts: sorts})
	return plan.Tree, err
}

func CreateWithMarkdown(tags, boxID, hPath, md, parentID, id string, withMath bool, clippingHref string) (string, error) {
	return CreateWithMarkdownContext(context.Background(), tags, boxID, hPath, md, parentID, id, withMath, clippingHref)
}

func CreateWithMarkdownContext(ctx context.Context, tags, boxID, hPath, md, parentID, id string, withMath bool, clippingHref string) (string, error) {
	if err := createDocLock.LockContext(ctx); err != nil {
		return "", err
	}
	defer createDocLock.Unlock()
	r, e := runOwnedDocumentCreation(ctx, doclife.CreationRequest{Mode: "markdown", Tags: tags, Box: boxID, HPath: hPath, Markdown: md, ParentID: parentID, ID: id, WithMath: withMath, ClippingHref: clippingHref})
	return r.ID, e
}

const DailyNoteAttrPrefix = doclife.DailyNoteAttrPrefix

func CreateDailyNote(boxID string) (string, bool, error) {
	return CreateDailyNoteContext(context.Background(), boxID)
}

func CreateDailyNoteContext(ctx context.Context, boxID string) (string, bool, error) {
	if err := createDocLock.LockContext(ctx); err != nil {
		return "", false, err
	}
	defer createDocLock.Unlock()
	r, e := runOwnedDocumentCreation(ctx, doclife.CreationRequest{Mode: "daily", Box: boxID})
	return r.Path, r.Existed, e
}

func GetHPathByPath(boxID, p string) (string, error) {
	r, e := DocumentPathsContext(context.Background(), docquery.PathRequest{Mode: "path", Box: boxID, Path: p})
	return r.HPath, e
}
func GetHPathsByPaths(paths []string) ([]string, error) {
	r, e := DocumentPathsContext(context.Background(), docquery.PathRequest{Mode: "batch", Paths: paths})
	return r.Paths, e
}
func GetHPathByID(id string) (string, error) {
	r, e := DocumentPathsContext(context.Background(), docquery.PathRequest{Mode: "id", ID: id})
	return r.HPath, e
}
func GetPathByID(id string) (string, string, error) {
	r, e := DocumentPathsContext(context.Background(), docquery.PathRequest{Mode: "location", ID: id})
	return r.Path, r.Box, e
}
func GetFullHPathByID(id string) (string, error) {
	r, e := DocumentPathsContext(context.Background(), docquery.PathRequest{Mode: "full", ID: id})
	return r.HPath, e
}
func GetIDsByHPath(hpath, boxID string) ([]string, error) {
	r, e := DocumentPathsContext(context.Background(), docquery.PathRequest{Mode: "ids", Box: boxID, HPath: hpath})
	return r.IDs, e
}

func MoveDocs(fromPaths []string, toBoxID, toPath string, callback interface{}) error {
	return MoveDocsContext(context.Background(), fromPaths, toBoxID, toPath, callback)

}

func MoveDocsContext(ctx context.Context, fromPaths []string, toBoxID, toPath string, callback interface{}) error {
	_, err := runLegacyDocumentLifecycle(ctx, doclife.Request{Action: "move", Paths: fromPaths, ToBox: toBoxID, ToPath: toPath, Callback: callback})
	return err
}

func countSubDocs(box, p string) (ret int) {
	p = util.StripDocExt(p)
	_ = filelock.Walk(filepath.Join(util.DataDir, box, p), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if util.HasDocExt(path) {
			ret++
		}
		return nil
	})
	return
}

func moveDoc(fromBox *Box, fromPath string, toBox *Box, toPath string, luteEngine *lute.Lute, callback interface{}) (newPath string, err error) {
	isSameBox := fromBox.ID == toBox.ID

	if isSameBox {
		if !fromBox.Exist(toPath) {
			err = ErrBlockNotFound
			return
		}
	} else {
		if !toBox.Exist(toPath) {
			err = ErrBlockNotFound
			return
		}
	}

	tree, err := filesys.LoadTree(fromBox.ID, fromPath, luteEngine)
	if err != nil {
		err = ErrBlockNotFound
		return
	}

	fromParentTree := loadParentTree(tree)

	moveToRoot := "/" == toPath
	toBlockID := tree.ID
	fromFolder := path.Join(path.Dir(fromPath), tree.ID)
	toFolder := "/"
	if !moveToRoot {
		var toTree *parse.Tree
		if isSameBox {
			toTree, err = filesys.LoadTree(fromBox.ID, toPath, luteEngine)
		} else {
			toTree, err = filesys.LoadTree(toBox.ID, toPath, luteEngine)
		}
		if err != nil {
			err = ErrBlockNotFound
			return
		}

		toBlockID = toTree.ID
		toFolder = path.Join(path.Dir(toPath), toBlockID)
	}

	if isSameBox {
		if err = fromBox.MkdirAll(toFolder); err != nil {
			return
		}
	} else {
		if err = toBox.MkdirAll(toFolder); err != nil {
			return
		}
	}

	needMoveSubDocs := fromBox.Exist(fromFolder)
	if needMoveSubDocs {
		// 移动子文档文件夹

		newFolder := path.Join(toFolder, tree.ID)
		if isSameBox {
			if err = fromBox.Move(fromFolder, newFolder); err != nil {
				return
			}
		} else {
			absFromPath := filepath.Join(util.DataDir, fromBox.ID, fromFolder)
			absToPath := filepath.Join(util.DataDir, toBox.ID, newFolder)
			if filelock.IsExist(absToPath) {
				filelock.Remove(absToPath)
			}
			if err = filelock.Rename(absFromPath, absToPath); err != nil {
				msg := fmt.Sprintf(Conf.Language(5), fromBox.Name, fromPath, err)
				logging.LogErrorf("move [path=%s] in box [%s] failed: %s", fromPath, fromBox.ID, err)
				err = errors.New(msg)
				return
			}
		}
	}

	newPath = path.Join(toFolder, tree.ID+util.DocExt)

	if isSameBox {
		if err = fromBox.Move(fromPath, newPath); err != nil {
			return
		}

		tree, err = filesys.LoadTree(fromBox.ID, newPath, luteEngine)
		if err != nil {
			return
		}

		moveTree(tree)
	} else {
		absFromPath := filepath.Join(util.DataDir, fromBox.ID, fromPath)
		absToPath := filepath.Join(util.DataDir, toBox.ID, newPath)
		if err = filelock.Rename(absFromPath, absToPath); err != nil {
			msg := fmt.Sprintf(Conf.Language(5), fromBox.Name, fromPath, err)
			logging.LogErrorf("move [path=%s] in box [%s] failed: %s", fromPath, fromBox.ID, err)
			err = errors.New(msg)
			return
		}

		tree, err = filesys.LoadTree(toBox.ID, newPath, luteEngine)
		if err != nil {
			return
		}

		moveTree(tree)
		moveSorts(tree.ID, fromBox.ID, toBox.ID)
	}

	if needMoveSubDocs {
		// 将其所有子文档的移动事件推送到前端 https://github.com/siyuan-note/siyuan/issues/11661
		subDocsFolder := path.Join(toFolder, tree.ID)
		syFiles := listSyFiles(path.Join(toBox.ID, subDocsFolder))
		for _, syFile := range syFiles {
			relPath := strings.TrimPrefix(syFile, "/"+path.Join(toBox.ID, toFolder))
			subFromPath := path.Join(path.Dir(fromPath), relPath)
			subToPath := path.Join(toFolder, relPath)

			evt := util.NewCmdResult("moveDoc", 0, util.PushModeBroadcast)
			evt.Data = map[string]interface{}{
				"fromNotebook": fromBox.ID,
				"fromPath":     subFromPath,
				"toNotebook":   toBox.ID,
				"toPath":       path.Dir(subToPath) + util.DocExt,
				"newPath":      subToPath,
			}
			evt.Callback = callback
			util.PushEvent(evt)
		}
	}

	evt := util.NewCmdResult("moveDoc", 0, util.PushModeBroadcast)
	evt.Data = map[string]interface{}{
		"fromNotebook": fromBox.ID,
		"fromPath":     fromPath,
		"toNotebook":   toBox.ID,
		"toPath":       toPath,
		"newPath":      newPath,
	}
	evt.Callback = callback
	util.PushEvent(evt)

	refreshDocInfo(fromParentTree)
	return
}

func RemoveDoc(boxID, p string) {
	if err := RemoveDocContext(context.Background(), boxID, p); err != nil {
		logging.LogErrorf("remove document failed: %s", err)
	}
}

func RemoveDocContext(ctx context.Context, boxID, p string) error {
	_, err := runLegacyDocumentLifecycle(ctx, doclife.Request{Action: "delete", Box: boxID, Path: p})
	if err != nil {
		return err
	}
	return nil
}

func RemoveDocs(paths []string) {
	if err := RemoveDocsContext(context.Background(), paths); err != nil {
		logging.LogErrorf("remove documents failed: %s", err)
	}
}

func RemoveDocsContext(ctx context.Context, paths []string) error {
	_, err := runLegacyDocumentLifecycle(ctx, doclife.Request{Action: "delete", Paths: paths})
	if err != nil {
		return err
	}
	return nil
}

func removeDoc(box *Box, p string, luteEngine *lute.Lute) (tree *parse.Tree, childrenDir string, err error) {
	tree, err = filesys.LoadTree(box.ID, p, luteEngine)
	if nil == tree {
		return
	}

	historyDir, err := GetHistoryDir(HistoryOpDelete)
	if err != nil {
		logging.LogErrorf("get history dir failed: %s", err)
		return
	}

	historyPath := filepath.Join(historyDir, box.ID, p)
	absPath := filepath.Join(util.DataDir, box.ID, p)
	if err = filelock.Copy(absPath, historyPath); err != nil {
		logging.LogErrorf("backup [path=%s] to history [%s] failed: %s", absPath, historyPath, err)
		return
	}

	generateAvHistory(tree, historyDir)
	copyDocAssetsToDataAssets(box.ID, p)

	removeIDs := treenode.RootChildIDs(tree.ID)
	dir := path.Dir(p)
	childrenDir = path.Join(dir, tree.ID)
	existChildren := box.Exist(childrenDir)
	if existChildren {
		absChildrenDir := filepath.Join(util.DataDir, tree.Box, childrenDir)
		historyPath = filepath.Join(historyDir, tree.Box, childrenDir)
		if err = filelock.Copy(absChildrenDir, historyPath); err != nil {
			logging.LogErrorf("backup [path=%s] to history [%s] failed: %s", absChildrenDir, historyPath, err)
			return
		}
	}
	indexHistoryDir(filepath.Base(historyDir), util.NewLute())

	allRemoveRootIDs := []string{tree.ID}
	allRemoveRootIDs = append(allRemoveRootIDs, removeIDs...)
	allRemoveRootIDs = gulu.Str.RemoveDuplicatedElem(allRemoveRootIDs)
	for _, rootID := range allRemoveRootIDs {
		removeTree, _ := LoadTreeByBlockID(rootID)
		if nil == removeTree {
			continue
		}

		syncDelete2AvBlock(removeTree.Root, removeTree, nil)
	}

	if existChildren {
		if err = box.Remove(childrenDir); err != nil {
			logging.LogErrorf("remove children dir [%s%s] failed: %s", box.ID, childrenDir, err)
			return
		}
		logging.LogInfof("removed children dir [%s%s]", box.ID, childrenDir)
	}
	if err = box.Remove(p); err != nil {
		logging.LogErrorf("remove [%s%s] failed: %s", box.ID, p, err)
		return
	}
	logging.LogInfof("removed doc [%s%s]", box.ID, p)

	box.removeSort(removeIDs)
	RemoveRecentDoc(removeIDs)
	if "/" != dir {
		others, err := os.ReadDir(filepath.Join(util.DataDir, box.ID, dir))
		if err == nil && 1 > len(others) {
			box.Remove(dir)
		}
	}

	evt := util.NewCmdResult("removeDoc", 0, util.PushModeBroadcast)
	evt.Data = map[string]interface{}{
		"ids": removeIDs,
	}
	util.PushEvent(evt)

	refreshParentDocInfo(tree)
	return
}

func RenameDoc(boxID, p, title string) error {
	return RenameDocContext(context.Background(), boxID, p, title)
}

func RenameDocContext(ctx context.Context, boxID, p, title string) error {
	_, err := runLegacyDocumentLifecycle(ctx, doclife.Request{Action: "rename", Box: boxID, Path: p, Title: title})
	return err
}

func createDoc(boxID, p, title, dom string) (*parse.Tree, error) {
	plan, err := runLegacyDocumentLifecycle(context.Background(), doclife.Request{Action: "create-dom", Box: boxID, Path: p, Title: title, DOM: dom})
	return plan.Tree, err
}

func removeInvisibleCharsInTitle(title string) string { return doclife.CleanTitle(title) }

func moveSorts(rootID, fromBox, toBox string) {
	root := treenode.GetBlockTree(rootID)
	if nil == root {
		return
	}

	fromRootSorts := map[string]int{}
	ids := treenode.RootChildIDs(rootID)
	fromConfPath := filepath.Join(util.DataDir, fromBox, ".siyuan", "sort.json")
	fromFullSortIDs := map[string]int{}
	if filelock.IsExist(fromConfPath) {
		data, err := filelock.ReadFile(fromConfPath)
		if err != nil {
			logging.LogErrorf("read sort conf failed: %s", err)
			return
		}

		if err = gulu.JSON.UnmarshalJSON(data, &fromFullSortIDs); err != nil {
			logging.LogErrorf("unmarshal sort conf failed: %s", err)
		}
	}
	for _, id := range ids {
		fromRootSorts[id] = fromFullSortIDs[id]
	}

	toConfPath := filepath.Join(util.DataDir, toBox, ".siyuan", "sort.json")
	toFullSortIDs := map[string]int{}
	if filelock.IsExist(toConfPath) {
		data, err := filelock.ReadFile(toConfPath)
		if err != nil {
			logging.LogErrorf("read sort conf failed: %s", err)
			return
		}

		if err = gulu.JSON.UnmarshalJSON(data, &toFullSortIDs); err != nil {
			logging.LogErrorf("unmarshal sort conf failed: %s", err)
			return
		}
	}

	for id, sortVal := range fromRootSorts {
		toFullSortIDs[id] = sortVal
	}

	data, err := gulu.JSON.MarshalJSON(toFullSortIDs)
	if err != nil {
		logging.LogErrorf("marshal sort conf failed: %s", err)
		return
	}
	if err = filelock.WriteFile(toConfPath, data); err != nil {
		logging.LogErrorf("write sort conf failed: %s", err)
		return
	}

	sortIDs := map[string]int{}
	bt := treenode.GetBlockTree(rootID)
	if nil != bt {
		parentPath := path.Dir(bt.Path)
		docs, _, listErr := ListDocTree(toBox, parentPath, util.SortModeUnassigned, false, false, 102400)
		if listErr != nil {
			logging.LogErrorf("list doc tree failed: %s", err)
			return
		}

		for _, doc := range docs {
			sortIDs[doc.ID] = doc.Sort
		}

		pushFiletreeSortChanged(sortIDs)
	}

}

func ChangeFileTreeSort(boxID string, paths []string) {
	logOwnedTaskError("change document sort", ChangeFileTreeSortContext(context.Background(), boxID, paths))
}

func (box *Box) fillSort(files *[]*File) {
	confPath := filepath.Join(util.DataDir, box.ID, ".siyuan", "sort.json")
	if !filelock.IsExist(confPath) {
		return
	}

	data, err := filelock.ReadFile(confPath)
	if err != nil {
		logging.LogErrorf("read sort conf failed: %s", err)
		return
	}

	fullSortIDs := map[string]int{}
	if err = gulu.JSON.UnmarshalJSON(data, &fullSortIDs); err != nil {
		logging.LogErrorf("unmarshal sort conf failed: %s", err)
		return
	}

	for _, f := range *files {
		id := util.StripDocExt(f.ID)
		f.Sort = fullSortIDs[id]
	}
}

func (box *Box) removeSort(ids []string) {
	logOwnedTaskError("remove document sort", mutateDocumentSortContext(context.Background(), doctree.SortMutation{Mode: "remove", Box: box.ID, Remove: append([]string(nil), ids...)}, false))
}

func (box *Box) setSortByConf(parent, id string) {
	logOwnedTaskError("insert document sort", InsertDocumentSortContext(context.Background(), box.ID, parent, id, "", nil))
}

func (box *Box) addMaxSort(parent, id string) {
	top := false
	logOwnedTaskError("append document sort", InsertDocumentSortContext(context.Background(), box.ID, parent, id, "", &top))
}

func (box *Box) addMinSort(parent, id string) {
	top := true
	logOwnedTaskError("prepend document sort", InsertDocumentSortContext(context.Background(), box.ID, parent, id, "", &top))
}

func (box *Box) setSortVal(id string, rank int) {
	logOwnedTaskError("set document sort", mutateDocumentSortContext(context.Background(), doctree.SortMutation{Mode: "merge", Box: box.ID, Values: map[string]int{id: rank}}, false))
}

func (box *Box) addSort(previous, id string) {
	logOwnedTaskError("insert document sort", InsertDocumentSortContext(context.Background(), box.ID, "", id, previous, nil))
}

func (box *Box) setSort(values map[string]int) {
	copied := map[string]int{}
	for id, rank := range values {
		copied[id] = rank
	}
	logOwnedTaskError("merge document sort", mutateDocumentSortContext(context.Background(), doctree.SortMutation{Mode: "merge", Box: box.ID, Values: copied, SkipMissing: true}, true))
}

func pushFiletreeSortChanged(sortIDs map[string]int) {
	if 1 > len(sortIDs) {
		return
	}

	var childIDs []string
	for sortID := range sortIDs {
		childIDs = append(childIDs, sortID)
	}
	sort.Slice(childIDs, func(i, j int) bool {
		return sortIDs[childIDs[i]] < sortIDs[childIDs[j]]
	})

	firstID := childIDs[0]
	bt := treenode.GetBlockTree(firstID)
	if nil == bt {
		return
	}

	parentPath := path.Dir(bt.Path)
	util.BroadcastByType("main", "filetreeSortChanged", 0, "", map[string]any{
		"parentPath": parentPath,
		"childIDs":   childIDs,
	})
}
