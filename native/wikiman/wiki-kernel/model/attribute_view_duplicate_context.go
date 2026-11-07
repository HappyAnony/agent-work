// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"github.com/88250/lute/render"
	"github.com/vmihailenco/msgpack/v5"
	"os"
	"path"
	"time"
	"wiki-kernel/av"
	"wiki-kernel/cache"
	"wiki-kernel/filesys"
	"wiki-kernel/internal/avrender"
	"wiki-kernel/sql"
	"wiki-kernel/util"
)

func (b *legacyAVBinding) duplicateSource(id string) (*av.AttributeView, []byte, error) {
	if e := b.liveSourceGuard(); e != nil {
		return nil, nil, e
	}
	if !ast.IsNodeIDPattern(id) {
		return nil, nil, avrender.ErrShape
	}
	raw, e := b.scope.source("storage/av/" + id + ".json")
	if e != nil {
		return nil, nil, e
	}
	a, e := av.DecodeAttributeView(raw)
	if e != nil {
		return nil, nil, e
	}
	if a == nil || a.ID != id {
		return nil, nil, avrender.ErrShape
	}
	b.scope.bind(a)
	b.scope.cached[id] = a
	b.scope.versions[id] = bytes.Clone(raw)
	return a, bytes.Clone(raw), b.liveSourceGuard()
}

// Caller holds the original registry lock. Corruption is not absence.
func (b *legacyAVBinding) readMirrorRegistry() ([]byte, map[string][]string, error) {
	raw, e := b.scope.source("storage/av/blocks.msgpack")
	if os.IsNotExist(e) {
		return nil, map[string][]string{}, nil
	}
	if e != nil {
		return nil, nil, e
	}
	var rows map[string][]string
	if e = msgpack.Unmarshal(raw, &rows); e != nil {
		return nil, nil, e
	}
	if rows == nil {
		return nil, nil, errors.New("invalid AV mirror registry")
	}
	return raw, rows, nil
}
func duplicateRegistryUses(rows map[string][]string, id string) bool {
	for key, ids := range rows {
		if key == id {
			return true
		}
		for _, block := range ids {
			if block == id {
				return true
			}
		}
	}
	return false
}
func (b *legacyAVBinding) duplicateFileOrBlockUsed(tx *Transaction, id string) (repairIOResult bool, repairIOErr error) {
	if e := b.liveSourceGuard(); e != nil {
		return false, e
	}
	if !ast.IsNodeIDPattern(id) {
		return false, avrender.ErrShape
	}
	io, closeIO, e := b.scope.commit.r.openRepairIOChecked(b.scope.ctx, false)
	if e != nil {
		return false, e
	}
	defer func() {
		repairIOErr = joinRepairCloseError(repairIOErr, closeIO())
		if repairIOErr != nil {
			repairIOResult = false
		}
	}()
	name := "storage/av/" + id + ".json"
	if e = io.Source.Check(name, true); e != nil {
		return false, e
	}
	_, e = io.Source.Data.Lstat(name)
	if e == nil {
		return true, nil
	}
	if !os.IsNotExist(e) {
		return false, e
	}
	return b.blockExistsFor(tx, id)
}
func (b *legacyAVBinding) duplicateIDUsed(tx *Transaction, id string) (bool, error) {
	if used, e := b.duplicateFileOrBlockUsed(tx, id); e != nil || used {
		return used, e
	}
	av.AttributeViewBlocksLock.Lock()
	defer av.AttributeViewBlocksLock.Unlock()
	if e := b.liveSourceGuard(); e != nil {
		return false, e
	}
	_, rows, e := b.readMirrorRegistry()
	if e != nil {
		return false, e
	}
	return duplicateRegistryUses(rows, id), b.liveSourceGuard()
}
func (b *legacyAVBinding) reserveDuplicateIDs(tx *Transaction, id, block string) (repairIOErr error) {
	if e := b.liveSourceGuard(); e != nil {
		return e
	}
	if b.scope.readOnly {
		return errors.New("AV read-only scope cannot reserve IDs")
	}
	if !ast.IsNodeIDPattern(id) || !ast.IsNodeIDPattern(block) || id == block {
		return avrender.ErrShape
	}
	av.AttributeViewBlocksLock.Lock()
	defer av.AttributeViewBlocksLock.Unlock()
	before, rows, e := b.readMirrorRegistry()
	if e != nil {
		return e
	}
	for _, candidate := range []string{id, block} {
		used, e := b.duplicateFileOrBlockUsed(tx, candidate)
		if e != nil {
			return e
		}
		if used || duplicateRegistryUses(rows, candidate) {
			return errors.New("av_duplicate_id_collision")
		}
	}
	rows[id] = []string{block}
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
func (b *legacyAVBinding) createDuplicateSource(id string, raw []byte) (repairIOErr error) {
	if e := b.liveSourceGuard(); e != nil {
		return e
	}
	if b.scope.readOnly {
		return errors.New("AV read-only scope cannot copy source")
	}
	if !ast.IsNodeIDPattern(id) {
		return avrender.ErrShape
	}
	a, e := av.DecodeAttributeView(raw)
	if e != nil {
		return e
	}
	if a == nil || a.ID != id {
		return avrender.ErrShape
	}
	io, closeIO, e := b.scope.commit.r.openRepairIOChecked(b.scope.ctx, true)
	if e != nil {
		return e
	}
	defer func() { repairIOErr = joinRepairCloseError(repairIOErr, closeIO()) }()
	if e = io.Source.Rewrite(b.scope.ctx, "storage/av/"+id+".json", nil, raw); e != nil {
		return e
	}
	if e = closeIO(); e != nil {
		delete(b.scope.versions, id)
		delete(b.scope.cached, id)
		return e
	}
	b.scope.bind(a)
	b.scope.cached[id] = a
	b.scope.versions[id] = bytes.Clone(raw)
	return nil
}
func duplicateBindings(tx *Transaction, b *legacyAVBinding) *attributeViewDuplicateBindingsEnvironment {
	publish := cache.CaptureBlockIALPublisher()
	return &attributeViewDuplicateBindingsEnvironment{strictDependencies: true, names: b.names,
		exists: func(id string) (bool, error) { return b.blockExistsFor(tx, id) }, node: func(id string) (*ast.Node, *parse.Tree, error) { return b.node(tx, id) },
		name: func(id string) (string, error) {
			a, e := b.readCandidate(id)
			if e != nil {
				return "", e
			}
			return a.Name, nil
		}, untitled: func() string {
			if b.err != nil {
				return ""
			}
			return b.scope.commit.avUntitled
		},
		attrs: func(n *ast.Node, attrs map[string]string) (old map[string]string, e error) {
			e = b.effect(func() error { old, e = setNodeAttrs0(n, attrs); return e })
			return
		},
		cache: func(id string, attrs map[string]string) error {
			return b.effect(func() error { publish(id, attrs); return nil })
		},
		notify: func(old map[string]string, n *ast.Node) error {
			return b.effect(func() error { pushBroadcastAttrTransactions(old, n); return nil })
		},
		tree: func(t *parse.Tree) error { return b.writeDuplicateTree(t) }}
}

// Publish the guarded document before derived index admission. The mirror
// update retains its post-tree position; admitted primary writes are joined.
func (b *legacyAVBinding) writeDuplicateTree(t *parse.Tree) (err error) {
	if t == nil || t.Root == nil {
		return avrender.ErrShape
	}
	if err = b.liveSourceGuard(); err != nil {
		return
	}
	if b.scope.readOnly {
		return errors.New("AV read-only scope cannot write bindings")
	}
	r := b.scope.commit.r
	ctx := b.scope.ctx
	q := sql.LegacyMainQueue{DB: r.main.DB}
	io, closeIO, err := r.openRepairIOChecked(ctx, true)
	if err != nil {
		return
	}
	defer func() { err = joinRepairCloseError(err, closeIO()) }()
	originalWritten := io.Written
	io.Written = func(ctx context.Context, t *parse.Tree, size uint64) error {
		if e := originalWritten(ctx, t, size); e != nil {
			return e
		}
		return b.duplicateDocInfo(io, t, size)
	}
	if err = r.writeRepairTree(ctx, io, t, b.scope.commit.parser()); err != nil {
		return
	}
	if err = r.registry.Upsert(ctx, t); err != nil {
		return
	}
	if err = q.Upsert(ctx, t); err != nil {
		return
	}
	defer func() { err = errors.Join(err, q.Flush(context.WithoutCancel(ctx))) }()
	return r.bindRepairAVBatch(ctx, io.Source, t.Root.ChildrenByType(ast.NodeAttributeView))
}
func (b *legacyAVBinding) duplicateDocInfo(io filesys.TreeContext, t *parse.Tree, size uint64) error {
	collect := func(tree *parse.Tree, size uint64) error {
		dir := tree.Box + util.StripDocExt(tree.Path)
		count := 0
		if e := io.Source.Check(dir, false); e != nil && !os.IsNotExist(e) {
			return e
		}
		f, e := io.Source.Data.Open(dir)
		if e != nil && !os.IsNotExist(e) {
			return e
		}
		if e == nil {
			defer f.Close()
			entries, e := f.ReadDir(-1)
			if e != nil {
				return e
			}
			if tree.Root.IALAttr("custom-hidden") != "true" {
				for _, entry := range entries {
					if util.HasDocExt(entry.Name()) {
						count++
					}
				}
			}
		}
		data, e := json.Marshal(projectReloadDocInfo(tree, size, count, b.scope.commit.relative))
		if e != nil {
			return e
		}
		b.notices = append(b.notices, commitNotice{kind: "info", payload: data, delay: 500 * time.Millisecond})
		return nil
	}
	if e := collect(t, size); e != nil {
		return e
	}
	if parent := path.Dir(t.Path); parent != "/" && parent != "." {
		// Parent metadata does not migrate source or replace an admitted revision.
		io.Remember = nil
		p, e := io.Load(b.scope.ctx, t.Box, parent+util.DocExt, b.scope.commit.parser(), false)
		if e != nil && !os.IsNotExist(e) {
			return e
		}
		if e == nil {
			return collect(p, uint64(len(render.NewJSONRenderer(p, lute.New().RenderOptions).Render())))
		}
	}
	return b.liveSourceGuard()
}

func (b *legacyAVBinding) liveSourceGuard() error {
	if e := b.guard(); e != nil {
		return e
	}
	owner := b.scope.commit.r.owner
	owner.repoMu.Lock()
	closed := owner.repoClosed
	owner.repoMu.Unlock()
	if closed {
		return errors.New("AV duplicate owner closed")
	}
	return nil
}
