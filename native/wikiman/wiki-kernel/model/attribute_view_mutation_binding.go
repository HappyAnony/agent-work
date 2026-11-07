// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"github.com/vmihailenco/msgpack/v5"
	"os"
	"path"
	"strings"
	"wiki-kernel/av"
	"wiki-kernel/internal/avquery"
	"wiki-kernel/internal/avrender"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/treenode"
)

// Compatibility tree writes still use the admitted FIFO's original writer.
// Read identity comes from the captured registry and rooted read-only loader;
// transaction candidates win over disk so pending edits are not discarded.
func (b *legacyAVBinding) tree(tx *Transaction, id string) (*parse.Tree, error) {
	if e := b.guard(); e != nil {
		return nil, e
	}
	if !ast.IsNodeIDPattern(id) {
		return nil, ErrBlockNotFound
	}
	bt, e := b.scope.commit.r.referenceLookup(b.scope.ctx, id)
	if e != nil {
		return nil, e
	}
	if tx != nil {
		var pending *parse.Tree
		if bt != nil {
			pending = tx.trees[bt.RootID]
		} else {
			for _, candidate := range tx.trees {
				if candidate != nil && candidate.Root != nil && treenode.GetNodeInTree(candidate, id) != nil {
					pending = candidate
					break
				}
			}
		}
		if pending != nil {
			if pending.Root == nil || pending.Root.ID != pending.ID || !ast.IsNodeIDPattern(pending.ID) || !ast.IsNodeIDPattern(pending.Box) || path.Clean(pending.Path) != pending.Path || path.Base(pending.Path) != pending.ID+".docj" {
				return nil, errors.New("invalid AV transaction tree identity")
			}
			rootMeta, err := b.scope.commit.r.referenceLookup(b.scope.ctx, pending.ID)
			if err != nil {
				return nil, err
			}
			if (bt != nil && (pending.ID != bt.RootID || pending.Box != bt.BoxID || pending.Path != bt.Path)) || (rootMeta != nil && (pending.Box != rootMeta.BoxID || pending.Path != rootMeta.Path)) {
				return nil, errors.New("invalid AV transaction tree identity")
			}
			return pending, b.guard()
		}
	}
	if bt == nil {
		return nil, ErrBlockNotFound
	}
	trees, e := b.scope.trees([]string{id})
	if e != nil {
		return nil, e
	}
	t := trees[id]
	if t == nil {
		return nil, ErrBlockNotFound
	}
	if tx != nil {
		if tx.trees == nil {
			tx.trees = map[string]*parse.Tree{}
		}
		tx.trees[t.ID] = t
	}
	return t, b.guard()
}
func (b *legacyAVBinding) node(tx *Transaction, id string) (*ast.Node, *parse.Tree, error) {
	t, e := b.tree(tx, id)
	if e != nil {
		return nil, nil, e
	}
	return treenode.GetNodeInTree(t, id), t, b.guard()
}
func (b *legacyAVBinding) blockExists(id string) (bool, error) {
	if e := b.guard(); e != nil {
		return false, e
	}
	bt, e := b.scope.commit.r.referenceLookup(b.scope.ctx, id)
	return bt != nil, e
}
func (b *legacyAVBinding) existsAV(id string) (bool, error) {
	if e := b.guard(); e != nil {
		return false, e
	}
	if !ast.IsNodeIDPattern(id) {
		return false, avrender.ErrShape
	}
	_, e := b.scope.source("storage/av/" + id + ".json")
	if os.IsNotExist(e) {
		return false, nil
	}
	return e == nil, e
}
func (b *legacyAVBinding) names(ids string) (string, error) {
	if e := b.guard(); e != nil {
		return "", e
	}
	// Original name projection skips genuinely missing AVs (stale bindings), but
	// corruption, IO errors and changed owners must never look like absence.
	var present []string
	names := map[string]string{}
	if ids != "" {
		for _, id := range strings.Split(ids, ",") {
			a, e := b.readCandidate(id)
			if os.IsNotExist(e) {
				continue
			}
			if e != nil {
				return "", e
			}
			present = append(present, id)
			names[id] = a.Name
		}
	}
	return attributeViewNamesWithServices(strings.Join(present, ","), func(id string) (string, error) { return names[id], nil }, func() string { return b.scope.commit.avUntitled }, false)
}

func (b *legacyAVBinding) effect(fn func() error) error {
	if e := b.guard(); e != nil {
		return e
	}
	if e := fn(); e != nil {
		return e
	}
	return b.guard()
}
func (b *legacyAVBinding) attributes(tx *Transaction, n *ast.Node, t *parse.Tree, attrs map[string]string) error {
	return b.effect(func() error {
		if tx != nil {
			return setNodeAttrsWithTx(tx, n, t, attrs)
		}
		return setNodeAttrs(n, t, attrs)
	})
}

// Dependency parsing shares the renderer's captured function names and original
// invalid-template policy. Parsing never evaluates querySQL or other functions.
func (b *legacyAVBinding) templateKeys(a *av.AttributeView, k *av.Key) ([]*av.Key, error) {
	if e := b.guard(); e != nil {
		return nil, e
	}
	keys := avrender.GetTemplateKeyRelevantKeys(a, k, b.scope.templates())
	return keys, b.guard()
}

func (b *legacyAVBinding) blockExistsFor(tx *Transaction, id string) (bool, error) {
	if e := b.guard(); e != nil {
		return false, e
	}
	if tx != nil {
		for _, t := range tx.trees {
			if t != nil && t.Root != nil && treenode.GetNodeInTree(t, id) != nil {
				_, e := b.tree(tx, id)
				return e == nil, e
			}
		}
	}
	return b.blockExists(id)
}

// The original mirror registry lock and set/dedup semantics, with rooted reads
// and the captured registry (including admitted pending nodes). Missing is empty;
// corruption and IO errors cannot masquerade as no mirrors.
func (b *legacyAVBinding) mirrors(tx *Transaction, id string) ([]string, error) {
	if e := b.guard(); e != nil {
		return nil, e
	}
	av.AttributeViewBlocksLock.Lock()
	defer av.AttributeViewBlocksLock.Unlock()
	if e := b.guard(); e != nil {
		return nil, e
	}
	raw, e := b.scope.source("storage/av/blocks.msgpack")
	if os.IsNotExist(e) {
		return []string{}, nil
	}
	if e != nil {
		return nil, e
	}
	var rows map[string][]string
	if e = msgpack.Unmarshal(raw, &rows); e != nil {
		return nil, e
	}
	if rows == nil {
		return nil, errors.New("invalid AV mirror registry")
	}
	return avquery.MirrorIDs(b.scope.ctx, rows[id], func(_ context.Context, id string) (bool, error) { return b.blockExistsFor(tx, id) })
}

// Read-only queries may repair detached projection candidates, never source files.
func readonlyAVBinding(tx *Transaction, ctx context.Context) *legacyAVBinding {
	var b *legacyAVBinding
	if tx != nil {
		b = legacyAVBindingForTransaction(tx)
	} else {
		b = newLegacyAVBinding(ctx)
	}
	if b.err == nil {
		b.scope.readOnly = true
	}
	return b
}
func (b *legacyAVBinding) allMirrors(tx *Transaction) ([]string, error) {
	if e := b.guard(); e != nil {
		return nil, e
	}
	rows, e := b.scope.commit.r.registry.Query(b.scope.ctx, indexdb.BlockTreeFilter{Kind: "type", Value: "av"})
	if e != nil {
		return nil, e
	}
	var ids []string
	seen := map[string]bool{}
	add := func(id string) {
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	for _, row := range rows {
		add(row.ID)
	}
	if tx != nil {
		for _, tree := range tx.trees {
			if tree == nil || tree.Root == nil {
				continue
			}
			ast.Walk(tree.Root, func(n *ast.Node, enter bool) ast.WalkStatus {
				if enter && n.Type == ast.NodeAttributeView {
					add(n.ID)
				}
				return ast.WalkContinue
			})
		}
	}
	return ids, b.guard()
}
func (b *legacyAVBinding) upsertMirror(id, block string) (repairIOErr error) {
	if e := b.guard(); e != nil {
		return e
	}
	if b.scope.readOnly {
		return errors.New("AV read-only scope cannot register mirrors")
	}
	if !ast.IsNodeIDPattern(id) || !ast.IsNodeIDPattern(block) {
		return avrender.ErrShape
	}
	io, closeIO, e := b.scope.commit.r.openRepairIOChecked(b.scope.ctx, true)
	if e != nil {
		return e
	}
	defer func() { repairIOErr = joinRepairCloseError(repairIOErr, closeIO()) }()
	return b.scope.commit.r.bindRepairAV(b.scope.ctx, io.Source, id, block)
}
