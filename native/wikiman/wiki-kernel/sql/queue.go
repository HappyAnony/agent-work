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

package sql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"path"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/88250/lute/parse"
	"github.com/siyuan-note/eventbus"
	"github.com/siyuan-note/logging"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/task"
	"wiki-kernel/treenode"
	"wiki-kernel/util"
)

var (
	operationQueue     []*dbQueueOperation
	lastQueueFailure   error
	lastQueueFailureDB *sql.DB
	dbQueueLock        = sync.Mutex{}
	txLock             = make(chan struct{}, 1)
)

type dbQueueOperation struct {
	ownerDB                       *sql.DB
	eventOwner                    interface{}
	inQueueTime                   time.Time
	action                        string      // upsert/delete/delete_id/rename/rename_sub_tree/delete_box/delete_box_refs/index/delete_ids/update_block_content/delete_assets
	indexTree                     *parse.Tree // index
	upsertTree                    *parse.Tree // upsert/update_refs/delete_refs
	removeTreeBox, removeTreePath string      // delete
	removeTreeID                  string      // delete_id
	removeTreeIDs                 []string    // delete_ids
	box                           string      // delete_box/delete_box_refs/index
	renameTree                    *parse.Tree // rename/rename_sub_tree
	block                         *Block      // update_block_content
	id                            string      // index_node
	removeAssetHashes             []string    // delete_assets
}

func FlushTxJob() {
	task.AppendTask(task.DatabaseIndexCommit, FlushQueue)
}

func WaitFlushTx() {
	// Preserve the original initial batching interval for legacy callers.
	time.Sleep(util.SQLFlushInterval + 50*time.Millisecond)
	if e := WaitFlushTxContext(context.Background()); e != nil {
		logging.LogErrorf("wait main queue failed: %s", e)
	}
}
func WaitFlushTxContext(ctx context.Context) error {
	q := CaptureLegacyMainQueue()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		if e := ctx.Err(); e != nil {
			return e
		}
		if q.DB != db {
			return ErrMainQueueOwner
		}
		dbQueueLock.Lock()
		n := len(operationQueue)
		failure, failedDB := lastQueueFailure, lastQueueFailureDB
		foreign := n > 0 && operationQueue[0].ownerDB != q.DB
		active := flushingTx.Load()
		dbQueueLock.Unlock()
		if foreign {
			return ErrMainQueueOwner
		}
		if failure != nil && failedDB == q.DB {
			return failure
		}
		if n == 0 && !active {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func isWritingDatabase(d time.Duration) bool {
	time.Sleep(d)
	dbQueueLock.Lock()
	defer dbQueueLock.Unlock()
	if 0 < len(operationQueue) || flushingTx.Load() {
		return true
	}
	return false
}

func ClearQueue() {
	dbQueueLock.Lock()
	defer dbQueueLock.Unlock()
	operationQueue = nil
	lastQueueFailure = nil
	lastQueueFailureDB = nil
}

var flushingTx = atomic.Bool{}

var ErrMainQueueOwner = errors.New("legacy_main_queue_owner_changed")
var ErrMainQueueOperation = errors.New("legacy_main_queue_operation_failed")

// This adapter pins compatibility queue/database identity; operation bodies
// still require the original single-workspace lifecycle barrier.
type LegacyMainQueue struct{ DB *sql.DB }

func CaptureLegacyMainQueue() LegacyMainQueue { return LegacyMainQueue{DB: db} }
func FlushQueue() {
	if e := FlushQueueContext(context.Background()); e != nil {
		logging.LogErrorf("flush main queue failed: %s", e)
	}
}
func FlushQueueContext(ctx context.Context) error { return CaptureLegacyMainQueue().Flush(ctx) }
func (q LegacyMainQueue) Flush(ctx context.Context) (err error) {
	select {
	case txLock <- struct{}{}:
		defer func() { <-txLock }()
	case <-ctx.Done():
		return ctx.Err()
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if q.DB != db {
		return ErrMainQueueOwner
	}
	flushingTx.Store(true)
	defer flushingTx.Store(false)
	ops := getOperations()
	if len(ops) == 0 {
		dbQueueLock.Lock()
		e := lastQueueFailure
		if lastQueueFailureDB != q.DB {
			e = nil
		}
		dbQueueLock.Unlock()
		return e
	}
	next := 0
	// Only confirmed commits consume queue entries. The failed/unstarted tail is
	// prepended before newer work. A retry never replays a successful prefix.
	defer func() {
		if recover() != nil {
			err = ErrMainQueueOperation
		}
		if next < len(ops) {
			dbQueueLock.Lock()
			operationQueue = append(ops[next:], operationQueue...)
			dbQueueLock.Unlock()
			ClearCache()
		}
		dbQueueLock.Lock()
		if err != nil && next < len(ops) {
			lastQueueFailure = err
			lastQueueFailureDB = q.DB
		} else if lastQueueFailureDB == q.DB {
			lastQueueFailure = nil
			lastQueueFailureDB = nil
		}
		dbQueueLock.Unlock()
	}()
	if q.DB == nil {
		return indexdb.ErrNoDatabase
	}
	start := time.Now()
	var rename *dbQueueOperation
	for _, op := range ops {
		if op.action == "rename_sub_tree" {
			rename = op
			break
		}
	}
	if rename != nil {
		childCount := treenode.CountBlockTreesByPathPrefix(path.Dir(rename.renameTree.Path))
		if childCount > 512 {
			secs := math.Log(float64(childCount)/512+1) / math.Log(2)
			secs = math.Max(1, math.Min(12, secs))
			timer := time.NewTimer(time.Duration(secs * float64(time.Second)))
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}
	}
	ui := map[string]interface{}{eventbus.CtxPushMsg: eventbus.CtxPushMsgToStatusBar}
	if len(ops) > 512 {
		disableCache()
		defer enableCache()
	}
	total, current := map[string]int{}, map[string]int{}
	for _, op := range ops {
		total[op.action]++
	}
	for i, op := range ops {
		if op.ownerDB != q.DB {
			return ErrMainQueueOwner
		}
		if err = ctx.Err(); err != nil {
			return err
		}
		if q.DB != db {
			return ErrMainQueueOwner
		}
		current[op.action]++
		ui["current"], ui["total"] = current[op.action], total[op.action]
		LegacySQLEventContext(op.eventOwner, ui)
		// Finish one admitted derived transaction before honoring cancellation.
		// Original helper callbacks can also touch caches; clear them on failure.
		err = func() (e error) {
			tx, e := indexdb.Begin(context.WithoutCancel(ctx), q.DB)
			if e != nil {
				return e
			}
			defer func() {
				if recover() != nil {
					e = ErrMainQueueOperation
				}
				if e != nil {
					_ = tx.Rollback()
				}
			}()
			if e = execOp(op, tx, ui); e != nil {
				return e
			}
			return indexdb.Commit(tx)
		}()
		if err != nil {
			return fmt.Errorf("main_queue_%s: %w", op.action, err)
		}
		next = i + 1
		if i > 16 && i%128 == 0 {
			debug.FreeOSMemory()
		}
	}
	if len(ops) > 128 {
		debug.FreeOSMemory()
	}
	if elapsed := time.Since(start).Milliseconds(); elapsed > 7000 {
		logging.LogInfof("database op tx [%dms]", elapsed)
	}
	// Publish the original completion signal only after the entire accepted batch.
	util.BroadcastByType("main", "databaseIndexCommit", 0, "", nil)
	eventbus.Publish(eventbus.EvtSQLIndexFlushed, LegacySQLEventContext(ops[len(ops)-1].eventOwner, nil))
	return ctx.Err()
}

func execOp(op *dbQueueOperation, tx *sql.Tx, context map[string]interface{}) (err error) {
	switch op.action {
	case "index":
		err = indexTree(tx, op.indexTree, context)
	case "upsert":
		err = upsertTree(tx, op.upsertTree, context)
	case "delete":
		err = batchDeleteByPathPrefix(tx, op.removeTreeBox, op.removeTreePath)
	case "delete_id":
		err = deleteByRootID(tx, op.removeTreeID, context)
	case "delete_ids":
		err = batchDeleteByRootIDs(tx, op.removeTreeIDs, context)
	case "rename":
		err = batchUpdateHPath(tx, op.renameTree, context)
		if err != nil {
			break
		}
		err = updateRootContent(tx, path.Base(op.renameTree.HPath), op.renameTree.Root.IALAttr("updated"), op.renameTree.ID)
	case "rename_sub_tree":
		err = batchUpdatePath(tx, op.renameTree, context)
	case "delete_box":
		err = deleteByBoxTx(tx, op.box)
	case "delete_box_refs":
		err = deleteRefsByBoxTx(tx, op.box)
	case "update_refs":
		err = upsertRefs(tx, op.upsertTree)
	case "delete_refs":
		err = deleteRefs(tx, op.upsertTree)
	case "update_block_content":
		err = updateBlockContent(tx, op.block)
	case "delete_assets":
		err = deleteAssetsByHashes(tx, op.removeAssetHashes)
	case "index_node":
		err = indexNode(tx, op.id)
	default:
		msg := fmt.Sprintf("unknown operation [%s]", op.action)
		logging.LogErrorf("%s", msg)
		err = errors.New(msg)
	}
	return
}

func IndexNodeQueue(id string) {
	eventOwner := captureLegacySQLEventOwner()
	dbQueueLock.Lock()
	defer dbQueueLock.Unlock()

	newOp := &dbQueueOperation{ownerDB: db, eventOwner: eventOwner, id: id, inQueueTime: time.Now(), action: "index_node"}
	for i, op := range operationQueue {
		if op.ownerDB != db {
			continue
		}
		if "index_node" == op.action && op.id == id {
			operationQueue[i] = newOp
			return
		}
	}
	appendOperation(newOp)
}

func BatchRemoveAssetsQueue(hashes []string) {
	if 1 > len(hashes) {
		return
	}

	eventOwner := captureLegacySQLEventOwner()
	dbQueueLock.Lock()
	defer dbQueueLock.Unlock()

	newOp := &dbQueueOperation{ownerDB: db, eventOwner: eventOwner, removeAssetHashes: append([]string(nil), hashes...), inQueueTime: time.Now(), action: "delete_assets"}
	appendOperation(newOp)
}

func (q LegacyMainQueue) RemoveAssets(ctx context.Context, hashes []string) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	if q.DB == nil {
		return ErrMainQueueOwner
	}
	eventOwner := captureLegacySQLEventOwner()
	dbQueueLock.Lock()
	defer dbQueueLock.Unlock()
	if e := ctx.Err(); e != nil {
		return e
	}
	if q.DB != db {
		return ErrMainQueueOwner
	}
	if len(hashes) > 0 {
		appendOperation(&dbQueueOperation{ownerDB: q.DB, eventOwner: eventOwner, removeAssetHashes: append([]string(nil), hashes...), inQueueTime: time.Now(), action: "delete_assets"})
	}
	return nil
}

func UpdateBlockContentQueue(block *Block) {
	if block == nil {
		return
	}
	copy := *block
	eventOwner := captureLegacySQLEventOwner()
	dbQueueLock.Lock()
	defer dbQueueLock.Unlock()
	appendUpdateBlockContent(db, eventOwner, &copy)
}

// UpdateBlockContent admits one derived-content projection against the
// captured database. The caller can mutate its block after admission without
// changing the queued operation.
func (q LegacyMainQueue) UpdateBlockContent(ctx context.Context, block *Block) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if block == nil {
		return ErrMainQueueOperation
	}
	copy := *block
	eventOwner := captureLegacySQLEventOwner()
	dbQueueLock.Lock()
	defer dbQueueLock.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if q.DB == nil || q.DB != db {
		return ErrMainQueueOwner
	}
	appendUpdateBlockContent(q.DB, eventOwner, &copy)
	return nil
}

func appendUpdateBlockContent(owner *sql.DB, eventOwner interface{}, block *Block) {
	newOp := &dbQueueOperation{ownerDB: owner, eventOwner: eventOwner, block: block, inQueueTime: time.Now(), action: "update_block_content"}
	for i, op := range operationQueue {
		if op.ownerDB != owner {
			continue
		}
		if "update_block_content" == op.action && op.block.ID == block.ID {
			operationQueue[i] = newOp
			return
		}
	}
	appendOperation(newOp)
}

func DeleteRefsTreeQueue(tree *parse.Tree) {
	eventOwner := captureLegacySQLEventOwner()
	dbQueueLock.Lock()
	defer dbQueueLock.Unlock()

	newOp := &dbQueueOperation{ownerDB: db, eventOwner: eventOwner, upsertTree: tree, inQueueTime: time.Now(), action: "delete_refs"}
	for i, op := range operationQueue {
		if op.ownerDB != db {
			continue
		}
		if "delete_refs" == op.action && op.upsertTree.ID == tree.ID {
			operationQueue[i] = newOp
			return
		}
	}
	appendOperation(newOp)
}

func UpdateRefsTreeQueue(tree *parse.Tree) {
	eventOwner := captureLegacySQLEventOwner()
	dbQueueLock.Lock()
	defer dbQueueLock.Unlock()
	appendUpdateRefsTree(db, eventOwner, tree)
}

// UpdateRefs admits one reference projection against the captured database.
func (q LegacyMainQueue) UpdateRefs(ctx context.Context, tree *parse.Tree) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	eventOwner := captureLegacySQLEventOwner()
	dbQueueLock.Lock()
	defer dbQueueLock.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if q.DB == nil || q.DB != db {
		return ErrMainQueueOwner
	}
	if tree == nil || tree.Root == nil {
		return ErrMainQueueOperation
	}
	appendUpdateRefsTree(q.DB, eventOwner, tree)
	return nil
}

func appendUpdateRefsTree(owner *sql.DB, eventOwner interface{}, tree *parse.Tree) {
	newOp := &dbQueueOperation{ownerDB: owner, eventOwner: eventOwner, upsertTree: tree, inQueueTime: time.Now(), action: "update_refs"}
	for i, op := range operationQueue {
		if op.ownerDB != owner {
			continue
		}
		if "update_refs" == op.action && op.upsertTree.ID == tree.ID {
			operationQueue[i] = newOp
			return
		}
	}
	appendOperation(newOp)
}

func DeleteBoxRefsQueue(boxID string) {
	eventOwner := captureLegacySQLEventOwner()
	dbQueueLock.Lock()
	defer dbQueueLock.Unlock()

	newOp := &dbQueueOperation{ownerDB: db, eventOwner: eventOwner, box: boxID, inQueueTime: time.Now(), action: "delete_box_refs"}
	for i, op := range operationQueue {
		if op.ownerDB != db {
			continue
		}
		if "delete_box_refs" == op.action && op.box == boxID {
			operationQueue[i] = newOp
			return
		}
	}
	appendOperation(newOp)
}

func DeleteBoxQueue(boxID string) {
	eventOwner := captureLegacySQLEventOwner()
	dbQueueLock.Lock()
	defer dbQueueLock.Unlock()
	appendDeleteBox(db, eventOwner, boxID)
}

// DeleteBox admits one notebook deletion against the captured database.
func (q LegacyMainQueue) DeleteBox(ctx context.Context, boxID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	eventOwner := captureLegacySQLEventOwner()
	dbQueueLock.Lock()
	defer dbQueueLock.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if q.DB == nil || q.DB != db {
		return ErrMainQueueOwner
	}
	appendDeleteBox(q.DB, eventOwner, boxID)
	return nil
}

func appendDeleteBox(owner *sql.DB, eventOwner interface{}, boxID string) {
	newOp := &dbQueueOperation{ownerDB: owner, eventOwner: eventOwner, box: boxID, inQueueTime: time.Now(), action: "delete_box"}
	for i, op := range operationQueue {
		if op.ownerDB != owner {
			continue
		}
		if "delete_box" == op.action && op.box == boxID {
			operationQueue[i] = newOp
			return
		}
	}
	appendOperation(newOp)
}

func IndexTreeQueue(tree *parse.Tree) {
	eventOwner := captureLegacySQLEventOwner()
	dbQueueLock.Lock()
	defer dbQueueLock.Unlock()
	appendIndexTree(db, eventOwner, tree)
}
func (q LegacyMainQueue) Index(ctx context.Context, tree *parse.Tree) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	eventOwner := captureLegacySQLEventOwner()
	dbQueueLock.Lock()
	defer dbQueueLock.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if q.DB == nil || q.DB != db {
		return ErrMainQueueOwner
	}
	if tree == nil || tree.Root == nil {
		return ErrMainQueueOperation
	}
	appendIndexTree(q.DB, eventOwner, tree)
	return nil
}
func appendIndexTree(owner *sql.DB, eventOwner interface{}, tree *parse.Tree) {
	newOp := &dbQueueOperation{ownerDB: owner, eventOwner: eventOwner, indexTree: tree, inQueueTime: time.Now(), action: "index"}
	for i, op := range operationQueue {
		if op.ownerDB != owner {
			continue
		}
		if "index" == op.action && op.indexTree.ID == tree.ID { // 相同树则覆盖
			operationQueue[i] = newOp
			return
		}
	}
	appendOperation(newOp)
}

func UpsertTreeQueue(tree *parse.Tree) {
	eventOwner := captureLegacySQLEventOwner()
	dbQueueLock.Lock()
	defer dbQueueLock.Unlock()
	appendUpsertTree(db, eventOwner, tree)
}

// Fixed-handle admission for compatibility writers. The original public facade
// retains its void semantics; this port returns cancellation/owner failures.
func (q LegacyMainQueue) Upsert(ctx context.Context, tree *parse.Tree) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	eventOwner := captureLegacySQLEventOwner()
	dbQueueLock.Lock()
	defer dbQueueLock.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if q.DB == nil || q.DB != db {
		return ErrMainQueueOwner
	}
	if tree == nil || tree.Root == nil {
		return ErrMainQueueOperation
	}
	appendUpsertTree(q.DB, eventOwner, tree)
	return nil
}
func appendUpsertTree(owner *sql.DB, eventOwner interface{}, tree *parse.Tree) {
	newOp := &dbQueueOperation{ownerDB: owner, eventOwner: eventOwner, upsertTree: tree, inQueueTime: time.Now(), action: "upsert"}
	for i, op := range operationQueue {
		if op.ownerDB != owner {
			continue
		}
		if "upsert" == op.action && op.upsertTree.ID == tree.ID { // 相同树则覆盖
			operationQueue[i] = newOp
			return
		}
	}
	appendOperation(newOp)
}

func RenameTreeQueue(tree *parse.Tree) {
	eventOwner := captureLegacySQLEventOwner()
	dbQueueLock.Lock()
	defer dbQueueLock.Unlock()

	newOp := &dbQueueOperation{ownerDB: db, eventOwner: eventOwner,
		renameTree:  tree,
		inQueueTime: time.Now(),
		action:      "rename",
	}
	for i, op := range operationQueue {
		if op.ownerDB != db {
			continue
		}
		if "rename" == op.action && op.renameTree.ID == tree.ID { // 相同树则覆盖
			operationQueue[i] = newOp
			return
		}
	}
	appendOperation(newOp)
}

func RenameSubTreeQueue(tree *parse.Tree) {
	eventOwner := captureLegacySQLEventOwner()
	dbQueueLock.Lock()
	defer dbQueueLock.Unlock()

	newOp := &dbQueueOperation{ownerDB: db, eventOwner: eventOwner,
		renameTree:  tree,
		inQueueTime: time.Now(),
		action:      "rename_sub_tree",
	}
	for i, op := range operationQueue {
		if op.ownerDB != db {
			continue
		}
		if "rename_sub_tree" == op.action && op.renameTree.ID == tree.ID { // 相同树则覆盖
			operationQueue[i] = newOp
			return
		}
	}
	appendOperation(newOp)
}

func RemoveTreeQueue(rootID string) {
	eventOwner := captureLegacySQLEventOwner()
	dbQueueLock.Lock()
	defer dbQueueLock.Unlock()
	appendRemoveTree(db, eventOwner, rootID)
}

// Remove admits one root deletion against the captured database.
func (q LegacyMainQueue) Remove(ctx context.Context, rootID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	eventOwner := captureLegacySQLEventOwner()
	dbQueueLock.Lock()
	defer dbQueueLock.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if q.DB == nil || q.DB != db {
		return ErrMainQueueOwner
	}
	appendRemoveTree(q.DB, eventOwner, rootID)
	return nil
}

func appendRemoveTree(owner *sql.DB, eventOwner interface{}, rootID string) {
	newOp := &dbQueueOperation{ownerDB: owner, eventOwner: eventOwner, removeTreeID: rootID, inQueueTime: time.Now(), action: "delete_id"}
	for i, op := range operationQueue {
		if op.ownerDB != owner {
			continue
		}
		if "delete_id" == op.action && op.removeTreeID == rootID {
			operationQueue[i] = newOp
			return
		}
	}
	appendOperation(newOp)
}

func BatchRemoveTreeQueue(rootIDs []string) {
	if 1 > len(rootIDs) {
		return
	}

	eventOwner := captureLegacySQLEventOwner()
	dbQueueLock.Lock()
	defer dbQueueLock.Unlock()

	newOp := &dbQueueOperation{ownerDB: db, eventOwner: eventOwner, removeTreeIDs: append([]string(nil), rootIDs...), inQueueTime: time.Now(), action: "delete_ids"}
	appendOperation(newOp)
}

// BatchRemove admits one derived-index deletion against the database captured
// by the caller. A workspace/database replacement cannot redirect the work to
// the newly selected compatibility queue.
func (q LegacyMainQueue) BatchRemove(ctx context.Context, rootIDs []string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	eventOwner := captureLegacySQLEventOwner()
	dbQueueLock.Lock()
	defer dbQueueLock.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if q.DB == nil || q.DB != db {
		return ErrMainQueueOwner
	}
	if len(rootIDs) == 0 {
		return nil
	}
	newOp := &dbQueueOperation{ownerDB: q.DB, eventOwner: eventOwner, removeTreeIDs: append([]string(nil), rootIDs...), inQueueTime: time.Now(), action: "delete_ids"}
	appendOperation(newOp)
	return nil
}

func RemoveTreePathQueue(treeBox, treePathPrefix string) {
	eventOwner := captureLegacySQLEventOwner()
	dbQueueLock.Lock()
	defer dbQueueLock.Unlock()

	newOp := &dbQueueOperation{ownerDB: db, eventOwner: eventOwner, removeTreeBox: treeBox, removeTreePath: treePathPrefix, inQueueTime: time.Now(), action: "delete"}
	for i, op := range operationQueue {
		if op.ownerDB != db {
			continue
		}
		if "delete" == op.action && (op.removeTreeBox == treeBox && op.removeTreePath == treePathPrefix) {
			operationQueue[i] = newOp
			return
		}
	}
	appendOperation(newOp)
}

// RemovePath admits one path-prefix deletion against the captured database.
func (q LegacyMainQueue) RemovePath(ctx context.Context, treeBox, treePathPrefix string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	eventOwner := captureLegacySQLEventOwner()
	dbQueueLock.Lock()
	defer dbQueueLock.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if q.DB == nil || q.DB != db {
		return ErrMainQueueOwner
	}
	newOp := &dbQueueOperation{ownerDB: q.DB, eventOwner: eventOwner, removeTreeBox: treeBox, removeTreePath: treePathPrefix, inQueueTime: time.Now(), action: "delete"}
	for i, op := range operationQueue {
		if op.ownerDB == q.DB && op.action == "delete" && op.removeTreeBox == treeBox && op.removeTreePath == treePathPrefix {
			operationQueue[i] = newOp
			return nil
		}
	}
	appendOperation(newOp)
	return nil
}

func getOperations() (ops []*dbQueueOperation) {
	dbQueueLock.Lock()
	defer dbQueueLock.Unlock()

	ops = operationQueue
	operationQueue = nil
	return
}

func appendOperation(op *dbQueueOperation) {
	operationQueue = append(operationQueue, op)
	eventbus.Publish(eventbus.EvtSQLIndexChanged, LegacySQLEventContext(op.eventOwner, nil))
}
