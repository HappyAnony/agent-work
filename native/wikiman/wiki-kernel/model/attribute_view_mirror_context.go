// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"errors"
	"github.com/88250/gulu"
	"github.com/88250/lute/ast"
	"github.com/vmihailenco/msgpack/v5"
	"wiki-kernel/av"
)

// These compatibility mirror ports use the captured source and original lock.
// The live-owner guard also rejects a closed owner, including empty batches.
func (b *legacyAVBinding) mirrorRegistry(write bool, fn func(map[string][]string) (bool, error)) (repairIOErr error) {
	if e := b.liveSourceGuard(); e != nil {
		return e
	}
	if write && b.scope.readOnly {
		return errors.New("AV read-only scope cannot change mirrors")
	}
	av.AttributeViewBlocksLock.Lock()
	defer av.AttributeViewBlocksLock.Unlock()
	if e := b.liveSourceGuard(); e != nil {
		return e
	}
	before, rows, e := b.readMirrorRegistry()
	if e != nil {
		return e
	}
	changed, e := fn(rows)
	if e != nil {
		return e
	}
	if e = b.liveSourceGuard(); e != nil {
		return e
	}
	if !write || !changed {
		return nil
	}
	after, e := msgpack.Marshal(rows)
	if e != nil {
		return e
	}
	io, closeIO, e := b.scope.commit.r.openRepairIOChecked(b.scope.ctx, true)
	if e != nil {
		return e
	}
	defer func() { repairIOErr = joinRepairCloseError(repairIOErr, closeIO()) }()
	return io.Source.Rewrite(b.scope.ctx, "storage/av/blocks.msgpack", before, after)
}

// Preserve the original raw registration count, before stale-member pruning.
func (b *legacyAVBinding) isMirror(id string) (ret bool, err error) {
	err = b.mirrorRegistry(false, func(rows map[string][]string) (bool, error) { ret = len(rows[id]) > 1; return false, nil })
	return
}
func (b *legacyAVBinding) registerMirror(id, block string) (ret bool, err error) {
	err = b.mirrorRegistry(true, func(rows map[string][]string) (bool, error) {
		old := len(rows[id])
		rows[id] = gulu.Str.RemoveDuplicatedElem(append(rows[id], block))
		ret = old != len(rows[id]) && old != 0
		return true, nil // Original upsert rewrites even an existing registration.
	})
	if err != nil {
		ret = false
	}
	return
}
func (b *legacyAVBinding) registerMirrors(nodes []*ast.Node) error {
	return b.mirrorRegistry(true, func(rows map[string][]string) (bool, error) {
		for _, n := range nodes {
			if n == nil || n.Type != ast.NodeAttributeView || n.AttributeViewID == "" || n.ID == "" {
				continue
			}
			rows[n.AttributeViewID] = gulu.Str.RemoveDuplicatedElem(append(rows[n.AttributeViewID], n.ID))
		}
		return true, nil
	})
}
func (b *legacyAVBinding) removeMirror(tx *Transaction, id, block string) (ret bool, err error) {
	err = b.mirrorRegistry(true, func(rows map[string][]string) (bool, error) {
		old := rows[id]
		if old == nil {
			return false, nil
		}
		var next []string
		for _, member := range old {
			if member == block {
				continue
			}
			exists, e := b.blockExistsFor(tx, member)
			if e != nil {
				return false, e
			}
			if exists {
				next = append(next, member)
			}
		}
		rows[id] = next
		ret = len(next) != len(old)
		return true, nil // Keep the original empty key and survivor order/duplicates.
	})
	if err != nil {
		ret = false
	}
	return
}
