// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"errors"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"os"
	"time"
	"wiki-kernel/av"
	"wiki-kernel/internal/avmodel"
	"wiki-kernel/internal/avrender"
	"wiki-kernel/treenode"
)

// Live compatibility initialization has a separate create-only capability. A
// missing source is not permission to overwrite corrupt, linked or changed data.
func (b *legacyAVBinding) initialize(id string) (*av.AttributeView, error) {
	if e := b.guard(); e != nil {
		return nil, e
	}
	if !ast.IsNodeIDPattern(id) {
		return nil, avrender.ErrShape
	}
	a, e := b.scope.load(id, false)
	if e == nil {
		return a, nil
	}
	if !os.IsNotExist(e) {
		return nil, e
	}
	a, e = avmodel.DefaultAttributeView(id, b.scope.commit.avLabels, ast.NewNodeID)
	if e != nil {
		return nil, e
	}
	if e = b.create(a); e != nil {
		return nil, e
	}
	return a, nil
}
func (b *legacyAVBinding) create(a *av.AttributeView) (repairIOErr error) {
	if b.err == nil && b.scope.readOnly {
		return errors.New("AV read-only scope cannot create")
	}
	if e := b.guard(); e != nil {
		return e
	}
	if a == nil || !ast.IsNodeIDPattern(a.ID) {
		return avrender.ErrShape
	}
	raw, e := av.EncodeAttributeView(a, b.scope.commit.r.singleLine)
	if e != nil {
		return e
	}
	root, closeIO, e := b.scope.commit.r.openRepairIOChecked(b.scope.ctx, true)
	if e != nil {
		return e
	}
	defer func() { repairIOErr = joinRepairCloseError(repairIOErr, closeIO()) }()
	if e = root.Source.Rewrite(b.scope.ctx, "storage/av/"+a.ID+".json", nil, raw); e != nil {
		return e
	}
	if e = closeIO(); e != nil {
		delete(b.scope.versions, a.ID)
		delete(b.scope.cached, a.ID)
		return e
	}
	b.scope.versions[a.ID] = append([]byte(nil), raw...)
	b.scope.bind(a)
	b.scope.cached[a.ID] = a
	return nil
}
func (b *legacyAVBinding) waitForStorages() error {
	for {
		if e := b.guard(); e != nil {
			return e
		}
		owner := b.scope.commit.r.owner
		owner.repoMu.Lock()
		closed := owner.repoClosed
		owner.repoMu.Unlock()
		if closed {
			return errors.New("AV initialization owner closed")
		}
		if !owner.syncRuntime.storages.Load() && !owner.syncRuntime.booting.Load() {
			return nil
		}
		timer := time.NewTimer(time.Second)
		select {
		case <-b.scope.ctx.Done():
			timer.Stop()
			return b.scope.ctx.Err()
		case <-timer.C:
		}
	}
}
func (b *legacyAVBinding) saveUpgraded(a *av.AttributeView) error {
	if e := b.guard(); e != nil {
		return e
	}
	if e := avmodel.UpgradeInMemory(a, avmodel.UpgradeEnvironment{Now: func() time.Time { return b.scope.now }, NewID: ast.NewNodeID}); e != nil {
		return e
	}
	return b.save(a)
}
func (b *legacyAVBinding) selectView(a *av.AttributeView, viewID, nodeID string) (*av.View, error) {
	if e := b.adopt(a); e != nil {
		return nil, e
	}
	e := &attributeViewBlockKeysEnvironment{ctx: b.scope.ctx, stage: b.saveUpgraded,
		newView: func() (*av.View, error) {
			v, _, _, err := avmodel.DefaultTable(ast.NewNodeID(), b.scope.commit.avLabels, ast.NewNodeID)
			return v, err
		},
		node: func(id string) (*ast.Node, *parse.Tree, error) {
			trees, e := b.scope.trees([]string{id})
			if e != nil {
				return nil, nil, e
			}
			t := trees[id]
			if t == nil {
				return nil, nil, nil
			}
			return treenode.GetNodeInTree(t, id), t, nil
		},
	}
	return selectBlockAttributeView(a, viewID, nodeID, e)
}
func (b *legacyAVBinding) prepareView(a *av.AttributeView, v *av.View) error {
	if e := b.guard(); e != nil {
		return e
	}
	changed, e := avmodel.CheckView(a, v, avmodel.RepairEnvironment{Now: func() time.Time { return b.scope.now }, NewID: ast.NewNodeID, Location: time.Local})
	if e != nil {
		return e
	}
	if changed {
		if e = b.saveUpgraded(a); e != nil {
			return e
		}
	}
	spec := a.Spec
	if spec < 1 {
		// Original model upgradeSpec1: dedupe IDs and refresh only the last bound
		// primary value's icon. Reading source trees never repairs old document Spec.
		ids := []string{}
		values := map[string]*av.Value{}
		for _, kv := range a.KeyValues {
			if kv.Key.Type == av.KeyTypeBlock {
				for _, value := range kv.Values {
					if !value.IsDetached {
						if values[value.BlockID] == nil {
							ids = append(ids, value.BlockID)
						}
						values[value.BlockID] = value
					}
				}
			}
		}
		trees, e := b.scope.trees(ids)
		if e != nil {
			return e
		}
		for _, id := range ids {
			if t := trees[id]; t != nil {
				if n := treenode.GetNodeInTree(t, id); n != nil {
					if value := values[id]; value.Block != nil {
						value.Block.Icon = n.IALAttr("icon")
					}
				}
			}
		}
	}
	if e = avmodel.UpgradeInMemory(a, avmodel.UpgradeEnvironment{Now: func() time.Time { return b.scope.now }, NewID: ast.NewNodeID}); e != nil {
		return e
	}
	if spec != a.Spec {
		return b.save(a)
	}
	return b.guard()
}
