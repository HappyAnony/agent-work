// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sync"

	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"github.com/siyuan-note/filelock"
	"github.com/vmihailenco/msgpack/v5"

	"wiki-kernel/av"
	"wiki-kernel/cache"
	"wiki-kernel/filesys"
	"wiki-kernel/internal/indexrepair"
	"wiki-kernel/util"
)

func (r *indexRepairContext) repairRootCloser() func(*os.Root) error {
	if r.closeRoot != nil {
		return r.closeRoot
	}
	return func(root *os.Root) error { return root.Close() }
}
func newRepairIOCloser(roots []*os.Root, closeRoot func(*os.Root) error) func() error {
	captured := append([]*os.Root(nil), roots...)
	var once sync.Once
	var err error
	return func() error {
		once.Do(func() {
			for i := len(captured) - 1; i >= 0; i-- {
				err = errors.Join(err, closeRoot(captured[i]))
			}
		})
		return err
	}
}
func joinRepairCloseError(primary, closing error) error {
	if primary == nil {
		return closing
	}
	if closing == nil || errors.Is(primary, closing) {
		return primary
	}
	return errors.Join(primary, closing)
}
func (r *indexRepairContext) openRepairIOChecked(ctx context.Context, writable bool) (filesys.TreeContext, func() error, error) {
	return r.openRepairIOCheckedWithCloser(ctx, writable, r.repairRootCloser())
}

// The closer is a trusted resource-backend capability, not request data.
func (r *indexRepairContext) openRepairIOCheckedWithCloser(ctx context.Context, writable bool, closeRoot func(*os.Root) error) (filesys.TreeContext, func() error, error) {
	if writable && r != nil && r.queryOnly {
		return filesys.TreeContext{}, nil, ErrSettingReadOnly
	}
	if closeRoot == nil {
		return filesys.TreeContext{}, nil, errors.New("repair_root_closer_unbound")
	}
	if e := r.guard(ctx); e != nil {
		return filesys.TreeContext{}, nil, e
	}
	root, e := os.OpenRoot(r.data)
	if e != nil {
		return filesys.TreeContext{}, nil, e
	}
	s := indexrepair.Source{Data: root}
	roots := []*os.Root{root}

	if writable {
		archive, e := os.OpenRoot(r.workspace)
		if e != nil {
			return filesys.TreeContext{}, nil, errors.Join(e, newRepairIOCloser(roots, closeRoot)())
		}
		roots = append(roots, archive)
		s.ArchiveRoot = archive
		s.Authorize = r.guard
		s.Lock = func(name string) func() {
			p := filepath.Join(r.data, filepath.FromSlash(name))
			filelock.Lock(p)
			return func() { filelock.Unlock(p) }
		}
	}
	ioctx := filesys.TreeContext{Source: s, Remember: func(t *parse.Tree, b []byte) { r.revisions.Store(t.Box+t.Path, sha256.Sum256(b)) }, SingleLine: r.singleLine, Upsert: r.registry.Upsert, Written: func(ctx context.Context, t *parse.Tree, size uint64) error {
		if e := r.guard(ctx); e != nil {
			return e
		}
		cache.PutDocIAL(t.Path, parse.IAL2MapUnEsc(t.Root.KramdownIAL))
		if r.warningSize > 0 && size > uint64(r.warningSize)*1024*1024 {
			util.PushErrMsg(fmt.Sprintf(r.language(268), t.Root.IALAttr("title")+" "+path.Base(t.Path), r.warningSize), 7000)
		}
		return nil
	}}
	return ioctx, newRepairIOCloser(roots, closeRoot), nil
}
func (r *indexRepairContext) loadRepairTree(ctx context.Context, box, p string, engine *lute.Lute) (tree *parse.Tree, err error) {
	c, closeIO, err := r.openRepairIOChecked(ctx, !r.queryOnly)
	if err != nil {
		return nil, err
	}
	defer func() {
		err = errors.Join(err, closeIO())
		if err != nil {
			tree = nil
		}
	}()
	return c.Load(ctx, box, p, engine, !r.queryOnly)
}

func (r *indexRepairContext) writeRepairTree(ctx context.Context, c filesys.TreeContext, t *parse.Tree, engine *lute.Lute) error {
	revision, ok := r.revisions.Load(t.Box + t.Path)
	if !ok {
		return indexrepair.ErrSourceChanged
	}
	name := t.Box + t.Path
	before, e := c.Source.Read(ctx, name)
	if e != nil {
		return e
	}
	if sha256.Sum256(before) != revision.([32]byte) {
		return indexrepair.ErrSourceChanged
	}
	return c.Write(ctx, t, before, engine)
}
func (r *indexRepairContext) bindRepairAV(ctx context.Context, s indexrepair.Source, a, b string) error {
	return r.bindRepairAVBatch(ctx, s, []*ast.Node{{Type: ast.NodeAttributeView, AttributeViewID: a, ID: b}})
}
func (r *indexRepairContext) bindRepairAVBatch(ctx context.Context, s indexrepair.Source, nodes []*ast.Node) error {
	if e := r.guard(ctx); e != nil {
		return e
	}
	av.AttributeViewBlocksLock.Lock()
	defer av.AttributeViewBlocksLock.Unlock()
	const name = "storage/av/blocks.msgpack"
	before, e := s.Read(ctx, name)
	if e != nil && !indexrepair.IsSourceNotExist(e) {
		return e
	}
	rows := map[string][]string{}
	if e == nil {
		if e = msgpack.Unmarshal(before, &rows); e != nil {
			return e
		}
	}
	if rows == nil {
		return errors.New("invalid AV mirror registry")
	}
	for _, n := range nodes {
		if n.Type != ast.NodeAttributeView || n.AttributeViewID == "" || n.ID == "" {
			continue
		}
		a, b := n.AttributeViewID, n.ID
		found := false
		for _, id := range rows[a] {
			if id == b {
				found = true
				break
			}
		}
		if !found {
			rows[a] = append(rows[a], b)
		}
	}
	after, e := msgpack.Marshal(rows)
	if e != nil {
		return e
	}
	return s.Rewrite(ctx, name, before, after)
}
