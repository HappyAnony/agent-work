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
	"path/filepath"

	"strings"
	"sync"
	"sync/atomic"
	"time"
	"wiki-kernel/internal/anchor"

	"wiki-kernel/internal/flashcard"

	"github.com/88250/gulu"
	"github.com/88250/lute"
	"github.com/88250/lute/ast"

	"github.com/88250/lute/parse"
	"github.com/siyuan-note/filelock"
	"github.com/siyuan-note/logging"

	"wiki-kernel/cache"
	"wiki-kernel/internal/editcore"
	"wiki-kernel/internal/txqueue"
	"wiki-kernel/treenode"
	"wiki-kernel/util"
)

func IsMoveOutlineHeading(transactions *[]*Transaction) bool {
	for _, tx := range *transactions {
		for _, op := range tx.DoOperations {
			if "moveOutlineHeading" == op.Action {
				return true
			}
		}
	}
	return false
}

func FlushTxQueue() {
	if err := FlushTxQueueContext(context.Background()); err != nil {
		logging.LogErrorf("drain transaction queue failed: %s", err)
	}
}
func FlushTxQueueContext(ctx context.Context) error {
	legacyTxQueueLock.Lock()
	queue := legacyTxQueue
	legacyTxQueueLock.Unlock()
	if queue != nil {
		return queue.Drain(ctx)
	}
	return ctx.Err()
}

var (
	legacyTxQueueLock sync.Mutex
	legacyTxQueue     *txqueue.Queue[*Transaction]
)

// StartTransactionQueue is only for the transitional single-workspace runtime.
// New workspace services must own an independent queue with contextual callbacks.
func StartTransactionQueue() {
	legacyTxQueueLock.Lock()
	defer legacyTxQueueLock.Unlock()
	if legacyTxQueue == nil {
		legacyTxQueue = txqueue.Open(7, flushTx)
	}
}

func CloseTransactionQueue(ctx context.Context) error {
	legacyTxQueueLock.Lock()
	queue := legacyTxQueue
	legacyTxQueueLock.Unlock()
	if queue == nil {
		return nil
	}
	return queue.Close(ctx)
}

func flushTx(tx *Transaction) {
	var result error
	defer func() {
		if recovered := recover(); recovered != nil {
			result = fmt.Errorf("transaction_execution_panicked: %v", recovered)
			logging.LogErrorf("%s", result)
		}
		if tx.execution != nil {
			tx.execution.complete(result)
		}
	}()
	if tx.completion != nil {
		runBoundTransaction(tx)
		return
	}

	start := time.Now()
	if txErr := performTx(tx); nil != txErr {
		result = errors.New(txErr.msg)
		if txErr.code == TxErrCodeAfterCommit {
			result = fmt.Errorf("%w: %s", errTransactionAfterCommit, txErr.msg)
		}
		reportTransactionFailure(txErr, tx)
		if txErr.code == TxErrCodeBlockNotFound {
			return
		}
	}
	elapsed := time.Now().Sub(start).Milliseconds()
	if 0 < len(tx.DoOperations) {
		if 2000 < elapsed {
			logging.LogWarnf("op tx [%dms]", elapsed)
		}
	}
}

func reportTransactionFailure(failure *TxErr, tx *Transaction) {
	if tx != nil && tx.environment != nil && tx.environment.reportFailure != nil {
		tx.environment.reportFailure(failure, tx)
		return
	}
	reportLegacyTransactionFailure(failure, tx, "", "")
}

func reportLegacyTransactionFailure(failure *TxErr, tx *Transaction, syncingMessage, attributeViewMessage string) {
	if failure == nil {
		return
	}
	switch failure.code {
	case TxErrCodeAfterCommit:
		logging.LogErrorf("transaction committed but publication failed: %s", failure.msg)
		util.PushTxErr("Transaction committed; refresh failed", failure.code, nil)
	case TxErrCodeBlockNotFound:
		util.PushTxErr("Transaction failed", failure.code, nil)
	case TxErrCodeDataIsSyncing:
		util.PushMsg(syncingMessage, 5000)
	case TxErrHandleAttributeView:
		util.PushMsg(attributeViewMessage, 5000)
		logging.LogErrorf("handle attribute view failed: %s", failure.msg)
	default:
		txData, _ := gulu.JSON.MarshalJSON(tx)
		logging.LogFatalf(logging.ExitCodeFatal, "transaction failed [%d]: %s\n  tx [%s]", failure.code, failure.msg, txData)
	}
}

func PerformTransactions(transactions *[]*Transaction) error {
	return PerformTransactionsContext(context.Background(), transactions)
}

func PerformTransactionsContext(ctx context.Context, transactions *[]*Transaction) error {
	if ctx == nil {
		return errors.New("transaction_context_unbound")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if transactions == nil {
		return errors.New("transaction_batch_unbound")
	}
	if err := ValidateTransactionsContext(ctx, *transactions); err != nil {
		return err
	}
	legacyTxQueueLock.Lock()
	queue := legacyTxQueue
	legacyTxQueueLock.Unlock()
	if queue == nil {
		return txqueue.ErrClosed
	}
	for _, tx := range *transactions {
		if len(tx.DoOperations) > 0 {
			base, err := captureLegacyReindexOwner(ctx)
			if err != nil {
				return err
			}
			ctx = context.WithValue(ctx, legacyAdmissionKey{}, base)
			break
		}
	}
	for _, tx := range *transactions {
		if err := ctx.Err(); err != nil {
			return err
		}
		tx.m = &sync.Mutex{}
		tx.environment = legacyTransactionEnvironmentContext(ctx)
		if len(tx.DoOperations) > 0 {
			if err := bindLegacyCommit(ctx, tx.environment, nil); err != nil {
				return err
			}
		}
		tx.execution = &transactionExecution{done: make(chan struct{})}
		if err := queue.Submit(ctx, tx); err != nil {
			tx.execution.complete(err)
			return err
		}
	}
	return nil
}

const (
	TxErrCodeBlockNotFound   = 0
	TxErrCodeDataIsSyncing   = 1
	TxErrCodeWriteTree       = 2
	TxErrHandleAttributeView = 3
	TxErrCodeAfterCommit     = 4
)

type TxErr struct {
	code int
	msg  string
	id   string
}

func performTx(tx *Transaction) (ret *TxErr) {
	if 1 > len(tx.DoOperations) {
		return
	}

	//os.MkdirAll("pprof", 0755)
	//cpuProfile, _ := os.Create("pprof/cpu_profile_tx")
	//pprof.StartCPUProfile(cpuProfile)
	//defer pprof.StopCPUProfile()

	var err error
	if err = tx.begin(); err != nil {
		if strings.Contains(err.Error(), "database is closed") {
			return
		}
		logging.LogErrorf("begin tx failed: %s", err)
		ret = &TxErr{msg: err.Error()}
		return
	}

	defer func() {
		if e := recover(); nil != e {
			msg := fmt.Sprintf("PANIC RECOVERED: %v\n\t%s\n", e, logging.ShortStack())
			logging.LogErrorf("%s", msg)
			ret = &TxErr{code: TxErrCodeWriteTree, msg: msg}

			if 1 == tx.state.Load() {
				tx.rollback()
				return
			}
		}
	}()

	isLargeInsert, largeErr := tx.processLargeInsert()
	if largeErr != nil {
		tx.rollback()
		return largeErr
	}
	if !isLargeInsert {
		tx.processUndoInsertWithFoldedHeading()
		for _, op := range tx.DoOperations {
			switch op.Action {
			case "create":
				ret = tx.doCreate(op)
			case "update":
				ret = tx.doUpdate(op)
			case "insert":
				ret = tx.doInsert(op)
			case "delete":
				ret = tx.doDelete(op)
			case "move":
				ret = tx.doMove(op)
			case "moveOutlineHeading":
				ret = tx.doMoveOutlineHeading(op)
			case "append":
				ret = tx.doAppend(op)
			case "appendInsert":
				ret = tx.doAppendInsert(op)
			case "prependInsert":
				ret = tx.doPrependInsert(op)
			case "foldHeading":
				ret = tx.doFoldHeading(op)
			case "unfoldHeading":
				ret = tx.doUnfoldHeading(op)
			case "setAttrs":
				ret = tx.doSetAttrs(op)
			case "doUpdateUpdated":
				ret = tx.doUpdateUpdated(op)
			case "addFlashcards":
				ret = tx.doAddFlashcards(op)
			case "removeFlashcards":
				ret = tx.doRemoveFlashcards(op)
			case "setAttrViewName":
				ret = tx.doSetAttrViewName(op)
			case "setAttrViewFilters":
				ret = tx.doSetAttrViewFilters(op)
			case "setAttrViewSorts":
				ret = tx.doSetAttrViewSorts(op)
			case "setAttrViewPageSize":
				ret = tx.doSetAttrViewPageSize(op)
			case "setAttrViewColWidth":
				ret = tx.doSetAttrViewColumnWidth(op)
			case "setAttrViewColWrap":
				ret = tx.doSetAttrViewColumnWrap(op)
			case "setAttrViewColHidden":
				ret = tx.doSetAttrViewColumnHidden(op)
			case "setAttrViewColPin":
				ret = tx.doSetAttrViewColumnPin(op)
			case "setAttrViewColIcon":
				ret = tx.doSetAttrViewColumnIcon(op)
			case "setAttrViewColDesc":
				ret = tx.doSetAttrViewColumnDesc(op)
			case "insertAttrViewBlock":
				ret = tx.doInsertAttrViewBlock(op)
			case "removeAttrViewBlock":
				ret = tx.doRemoveAttrViewBlock(op)
			case "addAttrViewCol":
				ret = tx.doAddAttrViewColumn(op)
			case "updateAttrViewCol":
				ret = tx.doUpdateAttrViewColumn(op)
			case "removeAttrViewCol":
				ret = tx.doRemoveAttrViewColumn(op)
			case "sortAttrViewRow":
				ret = tx.doSortAttrViewRow(op)
			case "sortAttrViewCol":
				ret = tx.doSortAttrViewColumn(op)
			case "sortAttrViewKey":
				ret = tx.doSortAttrViewKey(op)
			case "updateAttrViewCell":
				ret = tx.doUpdateAttrViewCell(op)
			case "updateAttrViewColOptions":
				ret = tx.doUpdateAttrViewColOptions(op)
			case "removeAttrViewColOption":
				ret = tx.doRemoveAttrViewColOption(op)
			case "updateAttrViewColOption":
				ret = tx.doUpdateAttrViewColOption(op)
			case "setAttrViewColOptionDesc":
				ret = tx.doSetAttrViewColOptionDesc(op)
			case "setAttrViewColCalc":
				ret = tx.doSetAttrViewColCalc(op)
			case "updateAttrViewColNumberFormat":
				ret = tx.doUpdateAttrViewColNumberFormat(op)
			case "replaceAttrViewBlock":
				ret = tx.doReplaceAttrViewBlock(op)
			case "updateAttrViewColTemplate":
				ret = tx.doUpdateAttrViewColTemplate(op)
			case "addAttrViewView":
				ret = tx.doAddAttrViewView(op)
			case "removeAttrViewView":
				ret = tx.doRemoveAttrViewView(op)
			case "setAttrViewViewName":
				ret = tx.doSetAttrViewViewName(op)
			case "setAttrViewViewIcon":
				ret = tx.doSetAttrViewViewIcon(op)
			case "setAttrViewViewDesc":
				ret = tx.doSetAttrViewViewDesc(op)
			case "duplicateAttrViewView":
				ret = tx.doDuplicateAttrViewView(op)
			case "sortAttrViewView":
				ret = tx.doSortAttrViewView(op)
			case "updateAttrViewColRelation":
				ret = tx.doUpdateAttrViewColRelation(op)
			case "updateAttrViewColRollup":
				ret = tx.doUpdateAttrViewColRollup(op)
			case "hideAttrViewName":
				ret = tx.doHideAttrViewName(op)
			case "setAttrViewColDateFillCreated":
				ret = tx.doSetAttrViewColDateFillCreated(op)
			case "setAttrViewColDateFillSpecificTime":
				ret = tx.doSetAttrViewColDateFillSpecificTime(op)
			case "setAttrViewCreatedIncludeTime":
				ret = tx.doSetAttrViewCreatedIncludeTime(op)
			case "setAttrViewUpdatedIncludeTime":
				ret = tx.doSetAttrViewUpdatedIncludeTime(op)
			case "duplicateAttrViewKey":
				ret = tx.doDuplicateAttrViewKey(op)
			case "setAttrViewCoverFrom":
				ret = tx.doSetAttrViewCoverFrom(op)
			case "setAttrViewCoverFromAssetKeyID":
				ret = tx.doSetAttrViewCoverFromAssetKeyID(op)
			case "setAttrViewCardSize":
				ret = tx.doSetAttrViewCardSize(op)
			case "setAttrViewFitImage":
				ret = tx.doSetAttrViewFitImage(op)
			case "setAttrViewDisplayFieldName":
				ret = tx.doSetAttrViewDisplayFieldName(op)
			case "setAttrViewFillColBackgroundColor":
				ret = tx.doSetAttrViewFillColBackgroundColor(op)
			case "setAttrViewShowIcon":
				ret = tx.doSetAttrViewShowIcon(op)
			case "setAttrViewWrapField":
				ret = tx.doSetAttrViewWrapField(op)
			case "changeAttrViewLayout":
				ret = tx.doChangeAttrViewLayout(op)
			case "setAttrViewBlockView":
				ret = tx.doSetAttrViewBlockView(op)
			case "setAttrViewCardAspectRatio":
				ret = tx.doSetAttrViewCardAspectRatio(op)
			case "setAttrViewGroup":
				ret = tx.doSetAttrViewGroup(op)
			case "hideAttrViewGroup":
				ret = tx.doHideAttrViewGroup(op)
			case "hideAttrViewAllGroups":
				ret = tx.doHideAttrViewAllGroups(op)
			case "foldAttrViewGroup":
				ret = tx.doFoldAttrViewGroup(op)
			case "syncAttrViewTableColWidth":
				ret = tx.doSyncAttrViewTableColWidth(op)
			case "removeAttrViewGroup":
				ret = tx.doRemoveAttrViewGroup(op)
			case "sortAttrViewGroup":
				ret = tx.doSortAttrViewGroup(op)
			}

			if nil != ret {
				tx.rollback()
				return
			}
		}
	}

	if cr := tx.commit(); nil != cr {
		logging.LogErrorf("commit tx failed: %s", cr)
		if errors.Is(cr, errTransactionAfterCommit) {
			return &TxErr{code: TxErrCodeAfterCommit, msg: cr.Error()}
		}
		return &TxErr{msg: cr.Error()}
	}
	return
}

func (tx *Transaction) processUndoInsertWithFoldedHeading() {
	s := tx.r04Document()
	defer tx.r04Accept(s)
	s.ProcessUndoInsertWithFoldedHeading()
}

func largeInsertAnchor(ops []*Operation) (string, bool) {
	return editcore.
		LargeInsertAnchor(ops)
}

func (tx *Transaction) processLargeInsert() (bool, *TxErr) {
	s := tx.r04Document()
	defer tx.r04Accept(s)
	v, f := s.ProcessLargeInsert()
	return v, r04Failure(f)
}

func (tx *Transaction) doMove(operation *Operation) (ret *TxErr) {
	s := tx.r04Document()
	defer tx.r04Accept(s)
	return r04Failure(s.DoMove(operation))
}

func isMovingFoldHeadingIntoSelf(targetNode *ast.Node, headingChildren []*ast.Node) bool {
	return editcore.
		IsMovingFoldHeadingIntoSelf(targetNode,
			headingChildren,
		)
}

func isMovingParentIntoChild(srcNode, targetNode *ast.Node) bool {
	return editcore.
		IsMovingParentIntoChild(srcNode, targetNode)
}

func (tx *Transaction) doPrependInsert(operation *Operation) (ret *TxErr) {
	s := tx.r04Document()
	defer tx.r04Accept(s)
	return r04Failure(s.DoPrependInsert(operation))
}

func (tx *Transaction) doAppendInsert(operation *Operation) (ret *TxErr) {
	s := tx.r04Document()
	defer tx.r04Accept(s)
	return r04Failure(s.DoAppendInsert(operation))
}

func (tx *Transaction) doAppend(operation *Operation) (ret *TxErr) {
	s := tx.r04Document()
	defer tx.r04Accept(s)
	return r04Failure(s.DoAppend(operation))
}

func (tx *Transaction) doDelete(operation *Operation) (ret *TxErr) {
	s := tx.r04Document()
	defer tx.r04Accept(s)
	return r04Failure(s.DoDelete(operation))
}

func syncDelete2AvBlock(node *ast.Node, nodeTree *parse.Tree, tx *Transaction) error {
	return deleteAttributeViewBindings(node, nodeTree, tx.attributeViewBoundEnvironment())
}

func (tx *Transaction) doLargeInsert(previousID string) (ret *TxErr) {
	s := tx.r04Document()
	defer tx.r04Accept(s)
	return r04Failure(s.DoLargeInsert(previousID))
}

func (tx *Transaction) doInsert(operation *Operation) (ret *TxErr) {
	s := tx.r04Document()
	defer tx.r04Accept(s)
	return r04Failure(s.DoInsert(operation))
}

func (tx *Transaction) processGlobalAssets(tree *parse.Tree) (err error) {
	if tx == nil || tx.environment == nil || tx.environment.ctx == nil || tx.environment.resolveGlobalAsset == nil || tree == nil || tree.Root == nil {
		return errors.New("transaction_global_assets_unbound")
	}
	ctx := tx.environment.ctx
	if err = ctx.Err(); err != nil {
		return err
	}
	// A transaction can insert into multiple documents with different local
	// asset directories. Resolve the destination for this tree, not the first.
	assetsDir := getAssetsDirAt(tx.environment.dataRoot, filepath.Join(tx.environment.dataRoot, tree.Box), filepath.Dir(filepath.Join(tx.environment.dataRoot, tree.Box, tree.Path)))
	if filepath.Clean(assetsDir) == filepath.Join(tx.environment.dataRoot, "assets") {
		return nil
	}

	// 本地资源文件需要移动到用户手动建立的 assets 下 https://github.com/siyuan-note/siyuan/issues/2410
	ast.Walk(tree.Root, func(n *ast.Node, entering bool) ast.WalkStatus {
		if err = ctx.Err(); err != nil {
			return ast.WalkStop
		}
		if !entering {
			return ast.WalkContinue
		}

		if ast.NodeLinkDest == n.Type && bytes.HasPrefix(n.Tokens, []byte("assets/")) {
			assetP := gulu.Str.FromBytes(n.Tokens)
			assetPath, found, e := tx.environment.resolveGlobalAsset(tx.environment.ctx, assetP)
			if nil != e {
				err = e
				return ast.WalkStop
			}
			if err = ctx.Err(); err != nil {
				return ast.WalkStop
			}

			if !found {
				// 非全局 assets 则跳过
				return ast.WalkContinue
			}

			// 只有全局 assets 才移动到相对 assets
			targetP := filepath.Join(assetsDir, filepath.Base(assetPath))
			if e = filelock.Rename(assetPath, targetP); e != nil {
				err = e
				return ast.WalkStop
			}
			if err = ctx.Err(); err != nil {
				return ast.WalkStop
			}
		}
		return ast.WalkContinue
	})
	return err
}

func resolveTransactionGlobalAsset(ctx context.Context, dataRoot, workspaceRoot, relative string) (string, bool, error) {
	if err := ctx.Err(); err != nil {
		return "", false, err
	}
	relative = strings.TrimSpace(relative)
	if i := strings.Index(relative, "?"); i >= 0 {
		relative = relative[:i]
	}
	candidate := filepath.Clean(filepath.Join(dataRoot, relative))
	if !util.IsSubPath(workspaceRoot, candidate) || !util.IsSubPath(filepath.Join(dataRoot, "assets"), candidate) {
		return "", false, nil
	}
	if _, err := os.Stat(candidate); err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, err
	}
	return candidate, true, nil
}

func (tx *Transaction) doUpdate(operation *Operation) (ret *TxErr) {
	s := tx.r04Document()
	defer tx.r04Accept(s)
	return r04Failure(s.DoUpdate(operation))
}

func legacyUpdateAV(ctx context.Context, updatedNode *ast.Node, tree *parse.Tree, tx *Transaction) error {
	return transactionUpdateAV(ctx, updatedNode, tree, tx)
}

func unfoldHeadingWithSink(ctx context.Context, heading, currentNode *ast.Node, sink func(*ast.Node, *ast.Node) error) error {
	return editcore.
		UnfoldHeadingWithSink(ctx, heading, currentNode,

			sink)
}

func getRefDefIDs(node *ast.Node) (refDefIDs []string) {
	return editcore.
		GetRefDefIDs(node)
}

func getRemovedNodes(oldNode, newNode *ast.Node) (ret []*ast.Node) {
	return editcore.
		GetRemovedNodes(oldNode, newNode)
}

func upsertAvBlockRel(node *ast.Node) {
	_ = registerUpdatedAttributeViews(node, legacyAttributeViewUpdateEnvironment(nil))
}

func (tx *Transaction) doUpdateUpdated(operation *Operation) (ret *TxErr) {
	s := tx.r04Document()
	defer tx.r04Accept(s)
	return r04Failure(s.DoUpdateUpdated(operation))
}

func (tx *Transaction) doCreate(operation *Operation) (ret *TxErr) {
	s := tx.r04Document()
	defer tx.r04Accept(s)
	return r04Failure(s.DoCreate(operation))
}

func (tx *Transaction) doSetAttrs(operation *Operation) (ret *TxErr) {
	s := tx.r04Document()
	defer tx.r04Accept(s)
	return r04Failure(s.DoSetAttrs(operation))
}

func refreshUpdated(node *ast.Node) {
	refreshUpdatedAt(node, util.CurrentTimeSecondsStr())
}

func refreshUpdatedAt(node *ast.Node, updated string) {
	editcore.RefreshUpdatedAt(node, updated)
}

func createdUpdated(node *ast.Node) {
	createdUpdatedWithCache(node, cache.PutBlockIAL)
}

func createdUpdatedWithCache(node *ast.Node, putIAL func(string, map[string]string)) {
	editcore.CreatedUpdatedWithCache(node, putIAL)
}

type Operation = editcore.Operation

// 用于添加/删除闪卡

// 属性视图 ID
// 用于从属性视图中删除行
// 用于添加属性视图行（包括绑定块）{id, content, isDetached}
// 用于标识是否未绑定块，仅存在于属性视图中
// 属性视图列名
// 属性视图列类型
// 属性视图列格式化
// 属性视图字段 ID
// 属性视图行 ID
// 属性视图关联列是否是双向关系
// 属性视图关联列回链关联列的 ID
// 属性视图删除关联目标
// 属性视图布局类型
// 属性视图分组视图 ID
// 属性视图目标分组视图 ID
// 属性视图视图 ID
// 是否忽略默认填充

// 上下文信息

type Transaction struct {
	execution      *transactionExecution
	completion     *transactionCompletion
	Timestamp      int64        `json:"timestamp"`
	DoOperations   []*Operation `json:"doOperations"`
	UndoOperations []*Operation `json:"undoOperations"`

	trees        map[string]*parse.Tree // 事务中变更的树
	nodes        map[string]*ast.Node   // 事务中变更的节点
	relatedAvIDs []string               // 事务中变更的属性视图 ID

	flashcardAfterCommit []flashcard.AttributeEffect
	reloadAfterCommit    []string // synchronous owner callback; no detached WaitForCommit goroutine
	environment          *transactionEnvironment
	luteEngine           *lute.Lute
	m                    *sync.Mutex
	state                atomic.Int32 // 0: 初始化，1：未提交，:2: 已提交，3: 已回滚
}

func (tx *Transaction) WaitForCommit() {
	_ = tx.WaitForCommitContext(context.Background())
}

func (tx *Transaction) WaitForCommitContext(ctx context.Context) error {
	if ctx == nil {
		return errors.New("transaction_context_unbound")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if tx == nil {
		return errors.New("transaction_unbound")
	}
	if tx.execution != nil {
		return tx.execution.wait(ctx)
	}
	if tx.state.Load() == 3 {
		return errors.New("transaction_rolled_back")
	}
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		if 1 != tx.state.Load() {
			if tx.state.Load() == 3 {
				return errors.New("transaction_rolled_back")
			}
			return ctx.Err()
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (tx *Transaction) begin() (err error) {
	if err = tx.environment.validate(tx.DoOperations); err != nil {
		return err
	}
	if tx.m == nil {
		tx.m = &sync.Mutex{}
	}
	tx.reloadAfterCommit = nil
	tx.flashcardAfterCommit = nil
	tx.trees = map[string]*parse.Tree{}
	tx.nodes = map[string]*ast.Node{}
	tx.luteEngine = tx.environment.parser()
	tx.m.Lock()
	tx.state.Store(1)
	return
}

func (tx *Transaction) commit() (err error) {
	if err = tx.environment.ctx.Err(); err == nil {
		err = tx.environment.finish(tx)
	}
	if err != nil {
		tx.rollback()
		return err
	}
	tx.state.Store(2)
	tx.m.Unlock()
	if tx.environment.afterCommit != nil {
		effects, reloads := tx.flashcardAfterCommit, tx.reloadAfterCommit
		tx.flashcardAfterCommit, tx.reloadAfterCommit = nil, nil
		// Source commit is already complete. A publication failure must not run
		// rollback or unlock the transaction mutex a second time.
		if err := tx.environment.afterCommit(tx.environment.ctx, effects, reloads); err != nil {
			return fmt.Errorf("%w: %w", errTransactionAfterCommit, err)
		}
		return nil
	}
	tx.publishFlashcardEffects()
	for _, id := range tx.reloadAfterCommit {
		tx.environment.reload(id)
	}
	tx.reloadAfterCommit = nil
	return nil
}

func (tx *Transaction) commitLegacy() error {
	if tx.environment == nil || tx.environment.commitContext == nil {
		return errors.New("transaction_commit_context_unbound")
	}
	return tx.environment.commitContext.finish(tx.environment.ctx, tx)
}

func (tx *Transaction) rollback() {
	tx.trees, tx.nodes = nil, nil
	tx.flashcardAfterCommit = nil
	tx.state.Store(3)
	tx.m.Unlock()
	return
}

func (tx *Transaction) loadTreeByBlockTree(bt *treenode.BlockTree) (ret *parse.Tree, err error) {
	s := tx.r04Document()
	defer tx.r04Accept(s)
	return s.LoadTreeByBlockTree(r04Location(bt))
}

func (tx *Transaction) loadTree(id string) (ret *parse.Tree, err error) {
	s := tx.r04Document()
	defer tx.r04Accept(s)
	return s.LoadTree(id)
}

func (tx *Transaction) writeTree(tree *parse.Tree) (err error) {
	s := tx.r04Document()
	defer tx.r04Accept(s)
	return s.WriteTree(tree)
}

type queuedLegacyAnchorRefresh struct {
	tree  *parse.Tree
	owner *legacyAnchorContext
}

var updateRefTextRenameDocs = map[string]queuedLegacyAnchorRefresh{}
var updateRefTextRenameDocLock sync.Mutex

func updateRefTextRenameDoc(renamedTree *parse.Tree) {
	if err := updateRefTextRenameDocContext(context.Background(), renamedTree); err != nil {
		logging.LogErrorf("queue renamed anchor refresh failed: %s", err)
	}
}

func updateRefTextRenameDocContext(ctx context.Context, renamedTree *parse.Tree) error {
	if renamedTree == nil || renamedTree.Root == nil {
		return errors.New("invalid_renamed_anchor_tree")
	}
	owner, err := captureLegacyAnchorContext(ctx)
	if err != nil {
		return err
	}
	updateRefTextRenameDocLock.Lock()
	updateRefTextRenameDocs[renamedTree.ID] = queuedLegacyAnchorRefresh{tree: renamedTree, owner: owner}
	updateRefTextRenameDocLock.Unlock()
	return nil
}

func FlushUpdateRefTextRenameDocJob() {
	logOwnedTaskError("periodic anchor refresh", FlushUpdateRefTextRenameDocJobContext(context.Background()))
}

func flushUpdateRefTextRenameDoc() {
	if err := flushUpdateRefTextRenameDocContext(context.Background()); err != nil {
		logging.LogErrorf("flush renamed anchor refresh failed: %s", err)
	}
}

func flushUpdateRefTextRenameDocContext(ctx context.Context) (err error) {
	updateRefTextRenameDocLock.Lock()
	pending := updateRefTextRenameDocs
	updateRefTextRenameDocs = map[string]queuedLegacyAnchorRefresh{}
	updateRefTextRenameDocLock.Unlock()

	for _, refresh := range pending {
		if refresh.owner == nil || refresh.tree == nil || refresh.tree.Root == nil {
			err = errors.Join(err, anchor.ErrDependency)
			continue
		}
		e := refresh.owner.refresh(ctx, map[string]*ast.Node{refresh.tree.Root.ID: refresh.tree.Root}, map[string]*parse.Tree{refresh.tree.ID: refresh.tree})
		err = errors.Join(err, e)
	}
	return err
}
