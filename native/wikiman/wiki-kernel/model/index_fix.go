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
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/html"
	"github.com/88250/lute/parse"
	"github.com/siyuan-note/logging"
	"wiki-kernel/conf"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/indexrepair"
	"wiki-kernel/sql"
	"wiki-kernel/treenode"
	"wiki-kernel/util"
)

// Each owner runs all five original stages once after sync. Partial failure or
// cancellation does not consume the marker. Queue writes are drained even on
// cancellation before the repository lease can be released.
type indexRepairContext struct {
	queryOnly     bool                 // Read adapters cannot acquire a source write capability.
	guideWrite    bool                 // Trusted legacy guide installation only; never bound by native/request source APIs.
	closeRoot     func(*os.Root) error // Trusted per-context resource backend, never request data.
	revisions     sync.Map
	owner         *AppConf
	data          string
	workspace     string
	singleLine    bool
	warningSize   int
	main          indexdb.RepairQueries
	queue         sql.LegacyMainQueue
	registry      treenode.RepairRegistry
	caseSensitive bool
	language      func(int) string
}

type indexRepairWalkIO struct {
	readDir func(context.Context, indexrepair.Source, string) ([]fs.DirEntry, error)
}
type indexRepairManifestEntry struct {
	name string
	info fs.FileInfo
}

func (r *indexRepairContext) repairManifest(ctx context.Context, source indexrepair.Source, root string, op indexRepairWalkIO) ([]indexRepairManifestEntry, error) {
	if err := r.guard(ctx); err != nil {
		return nil, err
	}
	if err := source.Check(root, false); err != nil {
		return nil, err
	}
	rootInfo, err := source.Data.Lstat(root)
	if err != nil {
		return nil, err
	}
	if !rootInfo.IsDir() || rootInfo.Mode()&fs.ModeSymlink != 0 {
		return nil, indexrepair.ErrSourcePath
	}
	readDir := op.readDir
	if readDir == nil {
		readDir = func(ctx context.Context, source indexrepair.Source, name string) ([]fs.DirEntry, error) {
			return source.ReadDir(ctx, name)
		}
	}
	manifest := []indexRepairManifestEntry{{name: root, info: rootInfo}}
	var scan func(string) error
	scan = func(folder string) error {
		if err := r.guard(ctx); err != nil {
			return err
		}
		before, err := source.Data.Lstat(folder)
		if err != nil {
			return err
		}
		if !before.IsDir() || before.Mode()&fs.ModeSymlink != 0 {
			return indexrepair.ErrSourcePath
		}
		entries, err := readDir(ctx, source, folder)
		if err != nil {
			return err
		}
		after, err := source.Data.Lstat(folder)
		if err != nil {
			return err
		}
		if !after.IsDir() || after.Mode()&fs.ModeSymlink != 0 || !os.SameFile(after, before) || after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) {
			return indexrepair.ErrSourceChanged
		}
		for _, entry := range entries {
			if err = r.guard(ctx); err != nil {
				return err
			}
			name := path.Join(folder, entry.Name())
			if err = source.Check(name, false); err != nil {
				return err
			}
			info, statErr := source.Data.Lstat(name)
			if statErr != nil {
				return statErr
			}
			if info.Mode()&fs.ModeSymlink != 0 || info.IsDir() != entry.IsDir() || info.Mode().Type() != entry.Type() {
				return indexrepair.ErrSourceChanged
			}
			if info.IsDir() && strings.HasPrefix(entry.Name(), ".") {
				continue
			}
			if !info.IsDir() && !info.Mode().IsRegular() {
				return indexrepair.ErrSourcePath
			}
			manifest = append(manifest, indexRepairManifestEntry{name: name, info: info})
			if info.IsDir() {
				if err = scan(name); err != nil {
					return err
				}
			}
		}
		return r.guard(ctx)
	}
	if err = scan(root); err != nil {
		return nil, err
	}
	for _, entry := range manifest {
		if err = r.guard(ctx); err != nil {
			return nil, err
		}
		if err = source.Check(entry.name, false); err != nil {
			return nil, err
		}
		info, statErr := source.Data.Lstat(entry.name)
		if statErr != nil {
			return nil, statErr
		}
		if info.Mode()&fs.ModeSymlink != 0 || info.IsDir() != entry.info.IsDir() || info.Mode().Type() != entry.info.Mode().Type() || !os.SameFile(info, entry.info) || info.Size() != entry.info.Size() || !info.ModTime().Equal(entry.info.ModTime()) {
			return nil, indexrepair.ErrSourceChanged
		}
	}
	return manifest, r.guard(ctx)
}

func captureIndexRepair(ctx context.Context) (*indexRepairContext, error) {
	base, e := captureLegacyReindexOwner(ctx)
	if e != nil {
		return nil, e
	}
	c := base.owner
	r := &indexRepairContext{owner: c, data: util.DataDir, workspace: util.WorkspaceDir, singleLine: util.UseSingleLineSave, warningSize: util.LargeFileWarningSize, main: sql.CaptureLegacyRepairQueries(), queue: sql.CaptureLegacyMainQueue(), registry: treenode.CaptureLegacyRepairRegistry()}
	if r.main.DB != r.queue.DB {
		return nil, conf.ErrBusinessUnbound
	}
	e = c.WithSettingSnapshot(ctx, func(v conf.AppData) error {
		if v.Search == nil {
			return conf.ErrBusinessUnbound
		}
		r.caseSensitive = v.Search.CaseSensitive
		if v.FileTree != nil {
			r.singleLine = v.FileTree.UseSingleLineSave
			r.warningSize = v.FileTree.LargeFileWarningSize
		}
		r.language = func(id int) string { return c.business.Language(v.Lang, id, true, v.CloudRegion) }
		return nil
	})
	if e != nil {
		return nil, e
	}
	return r, nil
}

// Only the compatibility binding checks process selectors; the reader/registry
// themselves hold fixed handles. A stale binding never enqueues on a new owner.
func (r *indexRepairContext) guard(ctx context.Context) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	if r == nil || r.owner == nil || r.owner != Conf || r.data != util.DataDir || r.workspace != util.WorkspaceDir || r.main.DB != r.queue.DB || r.queue.DB != sql.CaptureLegacyMainQueue().DB || r.main.DB != sql.CaptureLegacyRepairQueries().DB || r.registry.DB != treenode.CaptureLegacyRepairRegistry().DB {
		return conf.ErrBusinessUnbound
	}
	r.owner.m.Lock()
	readonly := r.owner.persistenceReadOnly
	r.owner.m.Unlock()
	if readonly && !r.queryOnly && !r.guideWrite {
		return ErrSettingReadOnly
	}
	return nil
}

func (r *indexRepairContext) pushRepairStatus(ctx context.Context, message string) error {
	if err := r.guard(ctx); err != nil {
		return err
	}
	util.PushStatusBar(message)
	return nil
}

func (r *indexRepairContext) setRepairBootDetails(ctx context.Context, message string) error {
	if err := r.guard(ctx); err != nil {
		return err
	}
	util.SetBootDetails(message)
	return nil
}

func (r *indexRepairContext) incRepairBootProgress(ctx context.Context, part int32, message string) error {
	if err := r.guard(ctx); err != nil {
		return err
	}
	util.IncBootProgress(part, message)
	return nil
}

func (r *indexRepairContext) publishRepairRefresh(ctx context.Context, message string) error {
	if err := r.guard(ctx); err != nil {
		return err
	}
	util.ReloadUI()
	util.PushMsg(message, 5000)
	return nil
}

func checkIndexContext(ctx context.Context) error {
	if util.ContainerAndroid == util.Container || util.ContainerIOS == util.Container || util.ContainerHarmony == util.Container {
		return nil
	}
	r, e := captureIndexRepair(ctx)
	if e != nil {
		return e
	}
	return runIndexRepairStages(ctx, []func(context.Context) error{
		r.removeDuplicateDatabaseIndex, r.resetDuplicateBlocksOnFileSys,
		r.fixBlockTreeByFileSys, r.fixDatabaseIndexByBlockTree, r.removeDuplicateDatabaseRefs,
	}, func() error { return r.queue.Flush(context.WithoutCancel(ctx)) }, func() error { return r.pushRepairStatus(ctx, r.language(185)) })
}

func runIndexRepairStages(ctx context.Context, stages []func(context.Context) error, flush func() error, complete func() error) (err error) {
	drained := true
	drain := func() (e error) {
		defer func() {
			if recover() != nil {
				e = errors.New("index_repair_drain_failed")
			}
		}()
		return flush()
	}
	defer func() {
		if recover() != nil {
			err = errors.New("index_repair_stage_failed")
		}
		if !drained {
			err = errors.Join(err, drain())
		}
	}()
	for i, stage := range stages {
		if err = ctx.Err(); err != nil {
			return
		}
		drained = false
		err = stage(ctx)
		err = errors.Join(err, drain())
		drained = true
		if err != nil {
			err = fmt.Errorf("index_repair_stage_%d: %w", i+1, err)
			return
		}
	}
	if err = ctx.Err(); err != nil {
		return
	}
	if complete != nil {
		return complete()
	}
	return nil
}

func recoverRepositoryWorkflow(err *error, operation string) {
	if recover() != nil {
		logging.LogErrorf("%s panic: %s", operation, debug.Stack())
		*err = errors.New(operation + "_failed")
	}
}

// removeDuplicateDatabaseRefs 删除重复的数据库引用关系。
func (r *indexRepairContext) removeDuplicateDatabaseRefs(ctx context.Context) (err error) {
	if e := r.guard(ctx); e != nil {
		return e
	}
	defer recoverRepositoryWorkflow(&err, "index_repair_stage")

	if err = r.pushRepairStatus(ctx, fmt.Sprintf(r.language(58), 5, 5)); err != nil {
		return err
	}
	duplicatedRootIDs, err := r.main.DuplicatedReferenceRoots(ctx)
	if err != nil {
		return err
	}
	for _, rootID := range duplicatedRootIDs {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if e := r.refreshReferences(ctx, rootID); e != nil {
			return e
		}
	}

	for _, rootID := range duplicatedRootIDs {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		logging.LogWarnf("exist more than one ref duplicated [%s], reindex it", rootID)
	}
	return
}

// removeDuplicateDatabaseIndex 删除重复的数据库索引。
func (r *indexRepairContext) removeDuplicateDatabaseIndex(ctx context.Context) (err error) {
	if e := r.guard(ctx); e != nil {
		return e
	}
	defer recoverRepositoryWorkflow(&err, "index_repair_stage")

	if err = r.pushRepairStatus(ctx, fmt.Sprintf(r.language(58), 1, 5)); err != nil {
		return err
	}
	ids, e := indexrepair.DuplicateRoots(ctx, r.main, r.caseSensitive)
	if e != nil {
		return e
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err = r.queue.BatchRemove(ctx, ids); err != nil {
		return err
	}
	deletes := len(ids)

	if 0 < deletes {
		logging.LogWarnf("exist more than one tree duplicated [%d], reindex it", deletes)
	}
	return
}

// resetDuplicateBlocksOnFileSys 重置重复 ID 的块。 https://github.com/siyuan-note/siyuan/issues/7357
func (r *indexRepairContext) resetDuplicateBlocksOnFileSys(ctx context.Context) (err error) {
	return r.resetDuplicateBlocksOnFileSysWithIO(ctx, indexRepairWalkIO{})
}
func (r *indexRepairContext) resetDuplicateBlocksOnFileSysWithIO(ctx context.Context, op indexRepairWalkIO) (err error) {
	if e := r.guard(ctx); e != nil {
		return e
	}
	defer recoverRepositoryWorkflow(&err, "index_repair_stage")

	if err = r.pushRepairStatus(ctx, fmt.Sprintf(r.language(58), 2, 5)); err != nil {
		return err
	}
	boxes, err := r.owner.notebooks(ctx)
	if err != nil {
		return err
	}
	ioctx, closeIO, e := r.openRepairIOChecked(ctx, true)
	if e != nil {
		return e
	}
	defer func() { err = joinRepairCloseError(err, closeIO()) }()
	luteEngine := lute.New()
	blockIDs := map[string]bool{}
	needRefreshUI := false
	for _, box := range boxes {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		// 校验索引阶段自动删除历史遗留的笔记本 history 文件夹
		if e = ioctx.Source.RemoveLegacyHistory(ctx, box.ID); e != nil {
			return e
		}
		boxPath := box.ID
		var duplicatedTrees []*parse.Tree
		manifest, walkErr := r.repairManifest(ctx, ioctx.Source, boxPath, op)
		if walkErr != nil {
			return walkErr
		}
		for _, entry := range manifest[1:] {
			if e = r.guard(ctx); e != nil {
				return e
			}
			if entry.info.IsDir() || !util.HasDocExt(entry.name) || strings.Contains(entry.name, "/assets/") {
				continue
			}
			base := path.Base(entry.name)
			if !ast.IsNodeIDPattern(util.StripDocExt(base)) {
				logging.LogWarnf("invalid doc file name [%s]", entry.name)
				if _, e = ioctx.Source.Archive(ctx, entry.name, time.Now().Format("2006-01-02-150405")); e != nil {
					return e
				}
				continue
			}

			p := entry.name[len(boxPath):]
			tree, loadErr := ioctx.Load(ctx, box.ID, p, luteEngine, true)
			if nil != loadErr {
				logging.LogErrorf("load tree [%s] failed: %s", p, loadErr)
				return loadErr
			}

			needOverwrite := false
			ast.Walk(tree.Root, func(n *ast.Node, entering bool) ast.WalkStatus {
				if !entering || !n.IsBlock() {
					return ast.WalkContinue
				}

				if "" == n.ID {
					needOverwrite = true
					treenode.ResetNodeID(n)
					return ast.WalkContinue
				}

				if !blockIDs[n.ID] {
					blockIDs[n.ID] = true
					return ast.WalkContinue
				}

				// 存在重复的块 ID

				if ast.NodeDocument == n.Type {
					// 如果是文档根节点，则重置这颗树
					// 这里不能在迭代中重置，因为如果这个文档存在子文档的话，重置时会重命名子文档文件夹，后续迭代可能会导致子文档 ID 重复
					duplicatedTrees = append(duplicatedTrees, tree)
					return ast.WalkStop
				}

				// 其他情况，重置节点 ID
				needOverwrite = true
				treenode.ResetNodeID(n)
				needRefreshUI = true
				return ast.WalkContinue
			})

			if needOverwrite {
				logging.LogWarnf("exist more than one node with the same id in tree [%s], reset it", box.ID+p)
				if writeErr := r.writeRepairTree(ctx, ioctx, tree, luteEngine); writeErr != nil {
					logging.LogErrorf("write tree [%s] failed: %s", p, writeErr)
					return writeErr
				}
			}
		}

		for _, tree := range duplicatedTrees {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			absPath := path.Join(boxPath, tree.Path)
			logging.LogWarnf("exist more than one tree with the same id [%s], reset it", absPath)
			if e := r.recreateTree(ctx, tree, absPath); e != nil {
				return e
			}
			needRefreshUI = true
		}
	}

	if err = closeIO(); err != nil {
		return err
	}
	if needRefreshUI {
		if err = r.publishRepairRefresh(ctx, r.language(190)); err != nil {
			return err
		}
	}
	return
}

func (r *indexRepairContext) recreateTree(ctx context.Context, tree *parse.Tree, absPath string) (err error) {
	if e := r.guard(ctx); e != nil {
		return e
	}
	ioctx, closeIO, e := r.openRepairIOChecked(ctx, true)
	if e != nil {
		return e
	}
	defer func() { err = joinRepairCloseError(err, closeIO()) }()
	unlock := ioctx.Source.Lock(absPath)
	defer unlock()
	original, e := ioctx.Source.Read(ctx, absPath)
	if e != nil {
		return e
	}
	loaded, ok := r.revisions.Load(tree.Box + tree.Path)
	if !ok || sha256.Sum256(original) != loaded.([32]byte) {
		return indexrepair.ErrSourceChanged
	}
	previous := tree.Path
	critical := context.WithoutCancel(ctx)
	if _, e = r.registry.Remove(critical, indexdb.BlockTreeFilter{Kind: "prefix", Value: util.StripDocExt(previous)}); e != nil {
		return e
	}
	if _, e = r.registry.Remove(critical, indexdb.BlockTreeFilter{Kind: "root", Value: tree.ID}); e != nil {
		return e
	}
	if e = resetTreeWithServices(critical, tree, "", true, resetTreeServices{NewID: ast.NewNodeID, Now: time.Now, Untitled: func() string { return r.language(16) }, Bind: func(ctx context.Context, a, b string) error { return r.bindRepairAV(ctx, ioctx.Source, a, b) }}); e != nil {
		return e
	}
	if e = ioctx.Write(critical, tree, nil, lute.New()); e != nil {
		return e
	}
	from := util.StripDocExt(absPath)
	to := path.Join(path.Dir(from), tree.ID)
	if info, e := ioctx.Source.Data.Lstat(from); e == nil && info.IsDir() {
		if e = ioctx.Source.Rename(critical, from, to); e != nil {
			return e
		}
	} else if e != nil && !os.IsNotExist(e) {
		return e
	}
	current, e := ioctx.Source.Read(critical, absPath)
	if e != nil {
		return e
	}
	if !bytes.Equal(current, original) {
		return indexrepair.ErrSourceChanged
	}
	// recreateTree already holds this exact path's non-reentrant lock. Keep that
	// outer lock through the checked mutation and all borrowed-directory closes.
	removing := ioctx.Source
	removing.Lock = nil
	return removing.Remove(critical, absPath)
}

// fixBlockTreeByFileSys 通过文件系统订正块树。
func (r *indexRepairContext) fixBlockTreeByFileSys(ctx context.Context) (err error) {
	return r.fixBlockTreeByFileSysWithIO(ctx, indexRepairWalkIO{})
}
func (r *indexRepairContext) fixBlockTreeByFileSysWithIO(ctx context.Context, op indexRepairWalkIO) (err error) {
	if e := r.guard(ctx); e != nil {
		return e
	}
	defer recoverRepositoryWorkflow(&err, "index_repair_stage")

	if err = r.pushRepairStatus(ctx, fmt.Sprintf(r.language(58), 3, 5)); err != nil {
		return err
	}
	boxes, err := r.owner.notebooks(ctx)
	if err != nil {
		return err
	}
	ioctx, closeIO, e := r.openRepairIOChecked(ctx, false)
	if e != nil {
		return e
	}
	defer func() { err = joinRepairCloseError(err, closeIO()) }()
	luteEngine := lute.New()
	for _, box := range boxes {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if box.Closed {
			continue
		}
		boxPath := box.ID
		var paths []string
		manifest, walkErr := r.repairManifest(ctx, ioctx.Source, boxPath, op)
		if walkErr != nil {
			return walkErr
		}
		for _, entry := range manifest[1:] {
			if entry.info.IsDir() || !util.HasDocExt(entry.name) || strings.Contains(entry.name, "/assets/") {
				continue
			}
			paths = append(paths, entry.name[len(boxPath):])
		}

		size := len(paths)

		// 清理块树中的冗余数据
		redundantPaths, missingPaths, e := r.registry.PathDifference(ctx, box.ID, paths)
		if e != nil {
			return e
		}
		for _, p := range redundantPaths {
			if _, e := r.registry.Remove(ctx, indexdb.BlockTreeFilter{Kind: "box-path", BoxID: box.ID, Value: p}); e != nil {
				return e
			}
		}

		// 重新索引缺失的块树

		for i, p := range missingPaths {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			id := path.Base(p)
			id = util.StripDocExt(id)
			if !ast.IsNodeIDPattern(id) {
				continue
			}

			if e := r.reindexTreeByPath(ctx, box.ID, p, i, size, luteEngine); e != nil {
				return e
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
		}

		if ctx.Err() != nil {
			return ctx.Err()
		}
	}

	// 清理已关闭的笔记本块树
	// Reuse the same detached notebook snapshot for closed-box cleanup.
	for _, box := range boxes {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if box.Closed {
			if _, e := r.registry.Remove(ctx, indexdb.BlockTreeFilter{Kind: "box", Value: box.ID}); e != nil {
				return e
			}
		}
	}
	return
}

// fixDatabaseIndexByBlockTree 通过块树订正数据库索引。
func (r *indexRepairContext) fixDatabaseIndexByBlockTree(ctx context.Context) (err error) {
	if e := r.guard(ctx); e != nil {
		return e
	}
	defer recoverRepositoryWorkflow(&err, "index_repair_stage")

	if err = r.pushRepairStatus(ctx, fmt.Sprintf(r.language(58), 4, 5)); err != nil {
		return err
	}
	reindex, remove, e := indexrepair.Reconcile(ctx, r.main, r.registry.BlockTrees)
	if e != nil {
		return e
	}
	luteEngine := util.NewLute()
	for i, id := range reindex {
		if e = ctx.Err(); e != nil {
			return e
		}
		if e = r.reindexTree(ctx, id, i, len(reindex), luteEngine); e != nil {
			return e
		}
	}
	if e = ctx.Err(); e != nil {
		return e
	}
	if err = r.queue.BatchRemove(ctx, remove); err != nil {
		return err
	}
	return nil
}

func (r *indexRepairContext) reindexTreeByPath(ctx context.Context, box, p string, i, size int, luteEngine *lute.Lute) (err error) {
	if e := r.guard(ctx); e != nil {
		return e
	}
	tree, err := r.loadRepairTree(ctx, box, p, luteEngine)
	if err != nil {
		return err
	}

	if e := r.reindexTree0(ctx, tree, i, size); e != nil {
		return e
	}
	return
}

func (r *indexRepairContext) reindexTree(ctx context.Context, rootID string, i, size int, luteEngine *lute.Lute) (err error) {
	if e := r.guard(ctx); e != nil {
		return e
	}
	rows, e := r.registry.Query(ctx, indexdb.BlockTreeFilter{Kind: "id", Value: rootID})
	if e != nil {
		return e
	}
	if len(rows) == 0 {
		logging.LogWarnf("root block [%s] not found", rootID)
		return
	}

	root := rows[0]
	tree, err := r.loadRepairTree(ctx, root.BoxID, root.Path, luteEngine)
	if err != nil {
		if os.IsNotExist(err) {
			// 文件系统上没有找到该 .docj 文件，则订正块树
			if _, e := r.registry.Remove(ctx, indexdb.BlockTreeFilter{Kind: "root", Value: rootID}); e != nil {
				return e
			}
			return nil
		}
		return err
	}

	if e := r.reindexTree0(ctx, tree, i, size); e != nil {
		return e
	}
	return
}

func (r *indexRepairContext) reindexTree0(ctx context.Context, tree *parse.Tree, i, size int) (err error) {
	if e := r.guard(ctx); e != nil {
		return e
	}
	updated := tree.Root.IALAttr("updated")
	if "" == updated {
		updated = util.TimeFromID(tree.Root.ID)
		tree.Root.SetIALAttr("updated", updated)
		ioctx, closeIO, e := r.openRepairIOChecked(ctx, true)
		if e != nil {
			return e
		}
		defer func() { err = joinRepairCloseError(err, closeIO()) }()
		if e := r.writeRepairTree(ctx, ioctx, tree, lute.New()); e != nil {
			return e
		}
		critical := context.WithoutCancel(ctx)
		if e := r.registry.Upsert(critical, tree); e != nil {
			return e
		}
		if e := r.queue.Upsert(critical, tree); e != nil {
			return e
		}

	} else {
		if e := r.registry.Upsert(ctx, tree); e != nil {
			return e
		}
		if e := r.queue.Index(ctx, tree); e != nil {
			return e
		}
	}

	if 0 == i%64 {
		if e := r.pushRepairStatus(ctx, fmt.Sprintf(r.language(183), i, size, html.EscapeString(path.Base(tree.HPath)))); e != nil {
			return e
		}
	}
	return
}
