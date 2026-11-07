// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"

	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"github.com/siyuan-note/filelock"
	"wiki-kernel/filesys"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/indexrepair"
	"wiki-kernel/internal/nativeparser"
	"wiki-kernel/sql"
	"wiki-kernel/util"
)

// Source and derived publication are sequential. This is not a receipt, rollback
// transaction or Native workspace write capability.
type markdownImportWriter struct {
	preparation *markdownImportPreparation
	io          filesys.TreeContext
	queue       sql.LegacyMainQueue
	parser      *lute.Lute
	identity    fs.FileInfo
	box         string
	boxIdentity fs.FileInfo
	closeIO     func() error
	admitted    bool
	closeOnce   sync.Once
	closeErr    error
}

func (m *markdownImportPreparation) openDocumentWriter(box string) (*markdownImportWriter, error) {
	if err := m.guard(); err != nil {
		return nil, err
	}
	r := m.archive.binding.scope.commit.r
	ioctx, closeIO, err := r.openRepairIOCheckedWithCloser(m.ctx, false, m.archive.archiveRootCloser())
	if err != nil {
		return nil, err
	}
	info, err := ioctx.Source.Data.Stat(".")
	if err != nil {
		return nil, errors.Join(err, closeIO())
	}
	if err := validateArchiveDestination(box, "/"); err != nil {
		return nil, errors.Join(err, closeIO())
	}
	if err := ioctx.Source.Check(box, false); err != nil {
		return nil, errors.Join(err, closeIO())
	}
	boxInfo, err := ioctx.Source.Data.Lstat(box)
	if err != nil {
		return nil, errors.Join(err, closeIO())
	}
	if !boxInfo.IsDir() {
		return nil, errors.Join(indexrepair.ErrSourcePath, closeIO())
	}
	w := &markdownImportWriter{preparation: m, io: ioctx, queue: sql.LegacyMainQueue{DB: r.main.DB}, parser: nativeparser.NewLute(m.options.Markdown), identity: info, box: box, boxIdentity: boxInfo, closeIO: closeIO}
	w.io.Source.Authorize = w.guard
	w.io.Source.Lock = func(name string) func() {
		p := filepath.Join(r.data, filepath.FromSlash(name))
		filelock.Lock(p)
		return func() { filelock.Unlock(p) }
	}
	// Empty-document normalization uses the original writer hook, now after its
	// source save. Nonempty trees enter the same captured registry below.
	w.io.Upsert = w.index
	w.io.Written = func(ctx context.Context, t *parse.Tree, size uint64) error {
		if err := w.guard(ctx); err != nil {
			return err
		}
		m.archive.binding.scope.commit.ial(t.Path, nil, parse.IAL2MapUnEsc(t.Root.KramdownIAL))
		if r.warningSize > 0 && size > uint64(r.warningSize)*1024*1024 {
			util.PushErrMsg(fmt.Sprintf(r.language(268), t.Root.IALAttr("title")+" "+path.Base(t.Path), r.warningSize), 7000)
		}
		return nil
	}
	if err = w.guard(m.ctx); err != nil {
		return nil, errors.Join(err, closeIO())
	}
	return w, nil
}
func (w *markdownImportWriter) guard(ctx context.Context) error {
	if w == nil || w.preparation == nil || w.preparation.archive == nil {
		return errors.New("markdown_writer_unbound")
	}
	r := w.preparation.archive.binding.scope.commit.r
	// Honor the supplied context so Source.Rewrite may finish one admitted
	// publication with WithoutCancel, without admitting another canceled tree.
	if err := r.guard(ctx); err != nil {
		return err
	}
	r.owner.repoMu.Lock()
	closed := r.owner.repoClosed
	r.owner.repoMu.Unlock()
	if closed {
		return errors.New("markdown_writer_owner_closed")
	}
	info, err := os.Lstat(r.data)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&fs.ModeSymlink != 0 || !os.SameFile(info, w.identity) {
		return indexrepair.ErrSourceChanged
	}
	if err := w.io.Source.Check(w.box, false); err != nil {
		return err
	}
	boxInfo, err := w.io.Source.Data.Lstat(w.box)
	if err != nil {
		return err
	}
	if !boxInfo.IsDir() || !os.SameFile(boxInfo, w.boxIdentity) {
		return indexrepair.ErrSourceChanged
	}
	return nil
}
func (w *markdownImportWriter) index(ctx context.Context, t *parse.Tree) error {
	if err := w.guard(ctx); err != nil {
		return err
	}
	return w.preparation.archive.binding.scope.commit.r.registry.Index(ctx, t)
}
func (w *markdownImportWriter) write(ctx context.Context, box string, t *parse.Tree) error {
	if err := w.guard(ctx); err != nil {
		return err
	}
	if t == nil || t.Root == nil || t.Box != box || box != w.box || !ast.IsNodeIDPattern(box) || !ast.IsNodeIDPattern(t.ID) || t.Root.ID != t.ID || !path.IsAbs(t.Path) || path.Clean(t.Path) != t.Path || path.Base(t.Path) != t.ID+util.DocExt || strings.Contains(t.Path, "\\") {
		return indexrepair.ErrSourcePath
	}
	name := path.Join(box, strings.TrimPrefix(t.Path, "/"))
	if err := w.io.Source.Check(name, true); err != nil {
		return err
	}
	// Imports allocate new document IDs. Reject existing source/index collisions
	// instead of overwriting another document or indexing it twice.
	if _, err := w.io.Source.Read(ctx, name); err == nil {
		return indexrepair.ErrSourceChanged
	} else if !indexrepair.IsSourceNotExist(err) {
		return err
	}
	rows, err := w.preparation.archive.binding.scope.commit.r.registry.Query(ctx, indexdb.BlockTreeFilter{Kind: "id", Value: t.ID})
	if err != nil {
		return err
	}
	if len(rows) > 0 {
		return indexrepair.ErrSourceChanged
	}
	empty := t.Root.FirstChild == nil
	if err = w.io.Write(ctx, t, nil, w.parser); err != nil {
		return err
	}
	if !empty {
		if err = w.index(ctx, t); err != nil {
			return err
		}
	}
	if err = w.guard(ctx); err != nil {
		return err
	}
	if err = w.queue.Index(ctx, t); err != nil {
		return err
	}
	w.admitted = true
	return nil
}
func (w *markdownImportWriter) close() error {
	w.closeOnce.Do(func() {
		if w.admitted {
			w.closeErr = w.queue.Flush(context.WithoutCancel(w.preparation.ctx))
		}
		w.closeErr = errors.Join(w.closeErr, w.closeIO())
	})
	return w.closeErr
}
func (m *markdownImportPreparation) publishDocuments(source *markdownImportSource, box string) (err error) {
	w, err := m.openDocumentWriter(box)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, w.close()) }()
	for i, t := range m.trees {
		if err = source.guard(); err != nil {
			return err
		}
		if err = w.write(m.ctx, box, t); err != nil {
			return err
		}
		if i%4 == 0 {
			util.PushEndlessProgress(fmt.Sprintf(m.language(66), fmt.Sprintf("%d/%d ", i, len(m.trees))+t.HPath))
		}
	}
	return source.guard()
}
