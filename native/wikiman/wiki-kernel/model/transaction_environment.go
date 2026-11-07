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
	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"wiki-kernel/cache"
	"wiki-kernel/filesys"
	"wiki-kernel/internal/flashcard"
	"wiki-kernel/internal/nativeparser"
	"wiki-kernel/sql"
	"wiki-kernel/treenode"
	"wiki-kernel/util"
)

// This private migration seam is not a new source-write handler. Only audited
// operations can use a non-legacy environment; all other operations fail before
// begin. R04 must supply the authoritative source transaction/receipt adapter.
type transactionEnvironment struct {
	commitContext      *legacyCommitContext
	avConfig           *attributeViewEnvironment
	avOptions          *attributeViewOptionEnvironment
	avFields           *attributeViewFieldEnvironment
	avOrder            *attributeViewOrderEnvironment
	avRows             *attributeViewRowEnvironment
	avManagement       *attributeViewManagementEnvironment
	avName             *attributeViewNameEnvironment
	avAppend           *attributeViewAppendEnvironment
	avDuplicate        *attributeViewDuplicateEnvironment
	avPreview          *attributeViewPreviewEnvironment
	avBound            *attributeViewBoundEnvironment
	tagNavigation      *tagNavigationEnvironment
	sourceEvents       *sourceEventEnvironment
	blockAttributes    *blockAttributeEnvironment
	findReplace        *findReplaceEnvironment
	bookmarkMutation   *bookmarkMutationEnvironment
	bookmarkRead       *bookmarkReadEnvironment
	documentSearch     *documentSearchEnvironment
	documentTree       *documentTreeEnvironment
	documentPaths      *documentPathEnvironment
	documentInfo       *documentInfoEnvironment
	blockPosition      *blockPositionEnvironment
	blockRelations     *blockRelationEnvironment
	blockFold          *blockFoldEnvironment
	documentLifecycle  *documentLifecycleEnvironment
	documentCreation   *documentCreationEnvironment
	flashcard          *flashcardEnvironment
	getDoc             *getDocEnvironment
	tagMutation        *tagMutationEnvironment
	avText             *attributeViewTextEnvironment
	avImages           *attributeViewImagesEnvironment
	avBlockKeys        *attributeViewBlockKeysEnvironment
	avUpdate           *attributeViewUpdateEnvironment
	avImport           *attributeViewImportEnvironment
	avCell             *attributeViewCellEnvironment
	ctx                context.Context
	legacy             bool
	parser             func() *lute.Lute
	lookup             func(string) *treenode.BlockTree
	lookupChecked      func(context.Context, string) (*treenode.BlockTree, error)
	load               func(string, string, *lute.Lute) (*parse.Tree, error)
	stage              func(*parse.Tree) error
	cacheIAL           func(string, map[string]string)
	sync               func()
	indexTree          func(*parse.Tree)
	headingEffects     func(context.Context, *parse.Tree) error
	afterCommit        func(context.Context, []flashcard.AttributeEffect, []string) error
	finish             func(*Transaction) error
	editorParser       func() *lute.Lute
	referenceCounts    func(context.Context, []string) (map[string]int, error)
	avNames            func(context.Context, string) (string, error)
	reload             func(string)
	moveHistory        func(context.Context, *parse.Tree) error
	outlineHistory     func(context.Context, *parse.Tree) error
	moveReferences     func(context.Context, ...string) error
	deleteReferences   func(context.Context, ...string) error
	deleteBlockTree    func(context.Context, string) error
	deleteAV           func(context.Context, *ast.Node, *parse.Tree, *Transaction) error
	insertAssets       func(context.Context, *parse.Tree, *Transaction) error
	insertReferences   func(context.Context, ...string) error
	insertAVRegistry   func(context.Context, *ast.Node) error
	insertAVBind       func(context.Context, *ast.Node, *parse.Tree, *Transaction) error
	updateCacheRef     func(context.Context, *parse.Tree, *ast.Node) error
	updateDynamicText  func(context.Context, string) (string, error)
	updateReferences   func(context.Context, ...string) error
	updateUnfold       func(context.Context, *ast.Node, *ast.Node) error
	updateEvent        func(context.Context, []*Transaction) error
	updateAV           func(context.Context, *ast.Node, *parse.Tree, *Transaction) error
	reportFailure      func(*TxErr, *Transaction)
	dataRoot           string
	workspaceRoot      string
	resolveGlobalAsset func(context.Context, string) (string, bool, error)
	reloadUI           func()
	now                func() string
}

func legacyTransactionEnvironment() *transactionEnvironment {
	return legacyTransactionEnvironmentContext(context.Background())
}

func legacyTransactionEnvironmentContext(ctx context.Context) *transactionEnvironment {
	// Capture parser settings at admission, not later when a worker runs.
	settings, settingsErr := originalMarkdownOptionsContext(ctx)
	editorOptions, editorErr := originalEditorOptionsContext(ctx)
	syncingMessage, attributeViewMessage := "", ""
	if Conf != nil {
		syncingMessage = Conf.Language(222)
		attributeViewMessage = Conf.language(258)
	}
	dataRoot, workspaceRoot := util.DataDir, util.WorkspaceDir
	return &transactionEnvironment{
		ctx: ctx, legacy: true,
		dataRoot: dataRoot, workspaceRoot: workspaceRoot,
		resolveGlobalAsset: func(ctx context.Context, relative string) (string, bool, error) {
			return resolveTransactionGlobalAsset(ctx, dataRoot, workspaceRoot, relative)
		},
		reloadUI: util.ReloadUI,
		now:      util.CurrentTimeSecondsStr,
		reportFailure: func(failure *TxErr, tx *Transaction) {
			reportLegacyTransactionFailure(failure, tx, syncingMessage, attributeViewMessage)
		},
		updateCacheRef: func(_ context.Context, t *parse.Tree, n *ast.Node) error { sql.CacheRef(t, n); return nil },
		updateDynamicText: func(_ context.Context, id string) (string, error) {
			if s, ok := treenode.DynamicRefTexts.Load(id); ok {
				return s.(string), nil
			}
			return "", nil
		},
		updateReferences: unboundTransactionReferenceCounts,
		updateUnfold: func(_ context.Context, h, n *ast.Node) error {
			util.BroadcastByType("protyle", "unfoldHeading", 0, "", map[string]interface{}{"id": h.ID, "currentNodeID": n.ID})
			return nil
		},
		updateEvent: func(_ context.Context, txs []*Transaction) error {
			evt := util.NewCmdResult("transactions", 0, util.PushModeBroadcast)
			evt.Data = txs
			util.PushEvent(evt)
			return nil
		},
		updateAV:         legacyUpdateAV,
		insertAssets:     func(_ context.Context, t *parse.Tree, tx *Transaction) error { return tx.processGlobalAssets(t) },
		insertReferences: unboundTransactionReferenceCounts,
		insertAVRegistry: func(_ context.Context, n *ast.Node) error { upsertAvBlockRel(n); return nil },
		insertAVBind:     transactionInsertAVBind,
		parser: func() *lute.Lute {
			if settingsErr != nil {
				panic(settingsErr)
			}
			return nativeparser.NewLute(settings)
		},
		lookup: treenode.GetBlockTree, load: filesys.LoadTree,
		stage:    func(tree *parse.Tree) error { treenode.UpsertBlockTree(tree); return nil },
		cacheIAL: cache.PutBlockIAL,
		sync:     IncSync, indexTree: sql.UpsertTreeQueue,
		finish: func(tx *Transaction) error { return tx.commitLegacy() },
		editorParser: func() *lute.Lute {
			if editorErr != nil {
				panic(editorErr)
			}
			return nativeparser.NewEditor(editorOptions)
		},
		referenceCounts: func(_ context.Context, ids []string) (map[string]int, error) { return sql.QueryRefCount(ids), nil },
		avNames:         func(_ context.Context, ids string) (string, error) { return getAvNames(ids), nil },
		reload:          ReloadProtyle,
		moveHistory:     func(_ context.Context, t *parse.Tree) error { generateOpTypeHistory(t, HistoryOpUpdate); return nil },
		deleteBlockTree: func(_ context.Context, id string) error { treenode.RemoveBlockTree(id); return nil },
		deleteAV: func(_ context.Context, n *ast.Node, t *parse.Tree, tx *Transaction) error {
			return syncDelete2AvBlock(n, t, tx)
		},
		deleteReferences: unboundTransactionReferenceCounts,
		moveReferences:   unboundTransactionReferenceCounts,
	}
}

func (e *transactionEnvironment) validate(ops []*Operation) error {
	if e == nil || e.ctx == nil || e.parser == nil || e.lookup == nil || e.load == nil || e.stage == nil || e.cacheIAL == nil || e.finish == nil {
		return errors.New("transaction_environment_unbound")
	}
	if e.commitContext != nil {
		if err := e.commitContext.r.guard(e.ctx); err != nil {
			return err
		}
	}
	if err := e.ctx.Err(); err != nil {
		return err
	}
	if !e.legacy {
		if len(ops) > 128 && len(ops) > 0 && ops[0] != nil && ops[0].Action == "insert" {
			if _, large := largeInsertAnchor(ops); !large || e.avBound == nil {
				return errors.New("transaction_large_batch_not_contextualized")
			}
			b := *e.avBound
			b.ctx, b.legacy = e.ctx, false
			if err := b.validate("insert"); err != nil {
				return err
			}
			if err := b.validate("delete"); err != nil {
				return err
			}
		}
		for _, op := range ops {
			if op == nil || (op.Action != "addFlashcards" && op.Action != "removeFlashcards" && op.Action != "create" && op.Action != "doUpdateUpdated" && op.Action != "setAttrs" && op.Action != "foldHeading" && op.Action != "unfoldHeading" && op.Action != "append" && op.Action != "appendInsert" && op.Action != "prependInsert" && op.Action != "move" && op.Action != "moveOutlineHeading" && op.Action != "delete" && op.Action != "insert" && op.Action != "update" && op.Action != "updateAttrViewColTemplate" && op.Action != "updateAttrViewColRelation" && op.Action != "updateAttrViewColRollup" && op.Action != "updateAttrViewCell" && op.Action != "setAttrViewName" && !isAttributeViewManagementAction(op.Action) && !isAttributeViewRowAction(op.Action) && !isAttributeViewOrderAction(op.Action) && !isAttributeViewFieldSettingAction(op.Action) && !isAttributeViewFieldAction(op.Action) && !isAttributeViewOptionAction(op.Action) && !isAttributeViewSettingsAction(op.Action)) {
				return errors.New("transaction_operation_not_contextualized")
			}
			if op.Action == "addFlashcards" || op.Action == "removeFlashcards" {
				if err := e.flashcard.validateMutation(e.ctx); err != nil {
					return err
				}
			}
			if op.Action == "setAttrViewName" {
				if err := validateAttributeViewNameInput(op); err != nil {
					return err
				}
				if err := e.avName.validate(); err != nil {
					return err
				}
			}
			if isAttributeViewManagementAction(op.Action) {
				if err := validateAttributeViewManagementInput(op); err != nil {
					return err
				}
				if err := e.avManagement.validate(op.Action); err != nil {
					return err
				}
			}
			if isAttributeViewRowAction(op.Action) {
				if err := validateAttributeViewRowInput(op); err != nil {
					return err
				}
				if err := e.avRows.validate(op.Action); err != nil {
					return err
				}
			}
			if isAttributeViewOrderAction(op.Action) {
				if err := validateAttributeViewOrderInput(op); err != nil {
					return err
				}
				if err := e.avOrder.validate(op.Action); err != nil {
					return err
				}
			}
			if isAttributeViewFieldSettingAction(op.Action) {
				if e.avConfig == nil || e.avConfig.read == nil || e.avConfig.stage == nil {
					return errors.New("transaction_av_field_setting_dependencies_unbound")
				}
				if err := validateAttributeViewFieldSettingInput(op); err != nil {
					return err
				}
			}
			if isAttributeViewFieldAction(op.Action) {
				if err := e.avFields.validate(op.Action); err != nil {
					return err
				}
				if err := validateAttributeViewFieldInput(op); err != nil {
					return err
				}
			}
			if isAttributeViewOptionAction(op.Action) {
				if err := e.avOptions.validate(op.Action); err != nil {
					return err
				}
				if err := validateAttributeViewOptionInput(op); err != nil {
					return err
				}
			}
			if op.Action == "updateAttrViewCell" {
				if e.avCell == nil {
					return errors.New("transaction_av_cell_dependencies_unbound")
				}
				cell := *e.avCell
				cell.legacy = false
				cell.ctx = e.ctx
				if err := cell.validate(); err != nil {
					return err
				}
			}
			if isAttributeViewSettingsAction(op.Action) {
				if e.avConfig == nil || e.avConfig.read == nil || e.avConfig.stage == nil {
					return errors.New("transaction_av_view_dependencies_unbound")
				}
				if err := validateAttributeViewSettings(op); err != nil {
					return err
				}
			}
			if op.Action == "updateAttrViewColTemplate" || op.Action == "updateAttrViewColRelation" || op.Action == "updateAttrViewColRollup" {
				if err := e.avConfig.validate(); err != nil {
					return err
				}
			}
			if op.Action == "update" && (e.updateCacheRef == nil || e.updateDynamicText == nil || e.updateReferences == nil || e.updateUnfold == nil || e.updateEvent == nil || e.updateAV == nil || e.deleteAV == nil) {
				return errors.New("transaction_update_dependencies_unbound")
			}
			if op.Action == "insert" && (e.insertAssets == nil || e.insertReferences == nil || e.insertAVRegistry == nil || e.insertAVBind == nil) {
				return errors.New("transaction_insert_dependencies_unbound")
			}
			if op.Action == "delete" && (e.deleteReferences == nil || e.deleteBlockTree == nil || e.deleteAV == nil) {
				return errors.New("transaction_delete_dependencies_unbound")
			}
			if op.Action == "moveOutlineHeading" && e.outlineHistory == nil {
				return errors.New("transaction_outline_history_unbound")
			}
			if op.Action == "move" && e.now == nil {
				return errors.New("transaction_clock_unbound")
			}
			if op.Action == "move" && (e.moveHistory == nil || e.moveReferences == nil) {
				return errors.New("transaction_move_dependencies_unbound")
			}
			if (op.Action == "foldHeading" || op.Action == "unfoldHeading") && (e.sync == nil || e.indexTree == nil) {
				return errors.New("transaction_heading_dependencies_unbound")
			}
			if op.Action == "unfoldHeading" && (e.editorParser == nil || e.referenceCounts == nil || e.avNames == nil || e.reload == nil) {
				return errors.New("transaction_unfold_dependencies_unbound")
			}
		}
	}
	return nil
}
