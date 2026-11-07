// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"wiki-kernel/internal/editcore"
	"wiki-kernel/treenode"
)

func r04Failure(f *editcore.Failure) *TxErr {
	if f == nil {
		return nil
	}
	return &TxErr{code: f.Code, msg: f.Msg, id: f.ID}
}
func r04Location(b *treenode.BlockTree) *editcore.BlockLocation {
	if b == nil {
		return nil
	}
	v := editcore.BlockLocation(*b)
	return &v
}
func (tx *Transaction) r04Accept(s *editcore.DocumentTransaction) {
	tx.trees = s.Trees
	tx.nodes = s.Nodes
	tx.relatedAvIDs = s.RelatedAvIDs
	tx.reloadAfterCommit = s.ReloadAfterCommit
	tx.DoOperations = s.DoOperations
	tx.UndoOperations = s.UndoOperations
}
func (tx *Transaction) r04Update(s *editcore.DocumentTransaction) {
	s.Trees = tx.trees
	s.Nodes = tx.nodes
	s.RelatedAvIDs = tx.relatedAvIDs
	s.ReloadAfterCommit = tx.reloadAfterCommit
	s.DoOperations = tx.DoOperations
	s.UndoOperations = tx.UndoOperations
}
func (tx *Transaction) r04Document() *editcore.DocumentTransaction {
	s := &editcore.DocumentTransaction{LuteEngine: tx.luteEngine}
	tx.r04Update(s)
	e := tx.environment
	if e == nil {
		return s
	}
	s.Environment = &editcore.DocumentEnvironment{ReadGuard: e.readGuard, ApplyHeadingEffects: e.applyHeadingEffects, Diagnostic: transactionEditDiagnostic, LookupBlock: func(id string) (*editcore.BlockLocation, error) {
		b, err := e.lookupBlock(id)
		return r04Location(b), err
	},
		AvNames:           e.avNames,
		CacheIAL:          e.cacheIAL,
		Ctx:               e.ctx,
		DeleteBlockTree:   e.deleteBlockTree,
		DeleteReferences:  e.deleteReferences,
		EditorParser:      e.editorParser,
		InsertAVRegistry:  e.insertAVRegistry,
		InsertReferences:  e.insertReferences,
		Legacy:            e.legacy,
		Load:              e.load,
		MoveHistory:       e.moveHistory,
		MoveReferences:    e.moveReferences,
		Now:               e.now,
		OutlineHistory:    e.outlineHistory,
		ReferenceCounts:   e.referenceCounts,
		ReloadUI:          e.reloadUI,
		Stage:             e.stage,
		UpdateCacheRef:    e.updateCacheRef,
		UpdateDynamicText: e.updateDynamicText,
		UpdateReferences:  e.updateReferences,
		UpdateUnfold:      e.updateUnfold,
	}
	s.Environment.DeleteAV = func(c context.Context, n *ast.Node, t *parse.Tree, state *editcore.DocumentTransaction) error {
		tx.r04Accept(state)
		err := e.deleteAV(c, n, t, tx)
		tx.r04Update(state)
		return err
	}
	s.Environment.InsertAVBind = func(c context.Context, n *ast.Node, t *parse.Tree, state *editcore.DocumentTransaction) error {
		tx.r04Accept(state)
		err := e.insertAVBind(c, n, t, tx)
		tx.r04Update(state)
		return err
	}
	s.Environment.UpdateAV = func(c context.Context, n *ast.Node, t *parse.Tree, state *editcore.DocumentTransaction) error {
		tx.r04Accept(state)
		err := e.updateAV(c, n, t, tx)
		tx.r04Update(state)
		return err
	}
	s.Environment.InsertAssets = func(c context.Context, t *parse.Tree, state *editcore.DocumentTransaction) error {
		tx.r04Accept(state)
		err := e.insertAssets(c, t, tx)
		tx.r04Update(state)
		return err
	}
	s.Environment.UpdateEvent = func(c context.Context, states []*editcore.DocumentTransaction) error {
		events := make([]*Transaction, len(states))
		for i, state := range states {
			events[i] = &Transaction{DoOperations: state.DoOperations, UndoOperations: state.UndoOperations}
		}
		return e.updateEvent(c, events)
	}
	return s
}
