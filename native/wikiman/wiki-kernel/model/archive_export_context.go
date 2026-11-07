// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"bytes"
	"context"
	"errors"
	"github.com/88250/gulu"
	"github.com/88250/lute/parse"
	"os"
	"path"
	"wiki-kernel/internal/indexrepair"
	"wiki-kernel/util"
)

type archiveExportContext struct {
	publication *archivePublication
	binding     *legacyAVBinding
	closeRoot   func(*os.Root) error // Trusted per-context resource backend; never request data.
	temp        string
	boxes       map[string]Box
	// Key by document ID so a moved/duplicate path cannot bind another version.
	documents  map[string]archiveExportDocument
	assetBoxes []string
	views      map[string]archiveExportView
	output     *archiveExportOutput
}

// Raw workspace/device exports bind publication directly; document exports
// retain their richer AV/document binding. Neither branch supplies fallback IO.
type archivePublication struct {
	ctx      context.Context
	guard    func() error
	language func(int) string
}

func (e *archiveExportContext) operationContext() context.Context {
	if e.publication != nil {
		return e.publication.ctx
	}
	return e.binding.scope.ctx
}
func (e *archiveExportContext) outputLanguage(code int) string {
	if e.publication != nil {
		return e.publication.language(code)
	}
	return e.binding.scope.commit.r.language(code)
}

type archiveExportSelection struct {
	box, rootDir, folder, name string
	paths                      []string
}

func captureArchiveExport(ctx context.Context) (*archiveExportContext, error) {
	if ctx == nil {
		return nil, errors.New("archive_export_context_unbound")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	b := newLegacyAVBinding(ctx)
	if err := b.liveSourceGuard(); err != nil {
		return nil, err
	}
	boxes, err := b.scope.commit.r.owner.notebooks(ctx)
	if err != nil {
		return nil, err
	}
	e := &archiveExportContext{binding: b, temp: util.TempDir, boxes: map[string]Box{}, documents: map[string]archiveExportDocument{}, views: map[string]archiveExportView{}}
	for _, box := range boxes {
		if box != nil {
			e.assetBoxes = append(e.assetBoxes, box.ID)
		}
		if box != nil && !box.Closed {
			e.boxes[box.ID] = *box
		}
	}
	return e, e.guard()
}
func (e *archiveExportContext) guard() error {
	if e == nil || (e.binding == nil && e.publication == nil) {
		return errors.New("archive_export_context_unbound")
	}
	if e.publication != nil {
		if e.publication.ctx == nil || e.publication.guard == nil || e.publication.language == nil {
			return errors.New("archive_export_context_unbound")
		}
		if err := e.publication.guard(); err != nil {
			return err
		}
	} else if err := e.binding.liveSourceGuard(); err != nil {
		return err
	}
	if e.temp != util.TempDir {
		return errors.New("archive_export_temp_owner_changed")
	}
	if e.output != nil {
		return e.output.check()
	}
	return nil
}
func (e *archiveExportContext) box(id string) (Box, error) {
	if err := e.guard(); err != nil {
		return Box{}, err
	}
	box, ok := e.boxes[id]
	if !ok {
		return Box{}, ErrBlockNotFound
	}
	return box, nil
}

func (e *archiveExportContext) single(id string) (archiveExportSelection, error) {
	if err := e.guard(); err != nil {
		return archiveExportSelection{}, err
	}
	block, err := e.binding.scope.commit.r.referenceLookup(e.binding.scope.ctx, id)
	if err != nil {
		return archiveExportSelection{}, err
	}
	if block == nil {
		return archiveExportSelection{}, ErrBlockNotFound
	}
	if _, err = e.box(block.BoxID); err != nil {
		return archiveExportSelection{}, err
	}
	children, err := e.documentPaths(block.BoxID, util.StripDocExt(block.Path), true)
	if err != nil {
		return archiveExportSelection{}, err
	}
	folder := path.Base(block.HPath)
	if folder == "." {
		folder = path.Base(block.Path)
	}
	return archiveExportSelection{box: block.BoxID, rootDir: path.Dir(block.Path), folder: folder, name: util.GetTreeID(block.Path), paths: append([]string{block.Path}, children...)}, e.guard()
}
func (e *archiveExportContext) notebook(id string) (archiveExportSelection, error) {
	box, err := e.box(id)
	if err != nil {
		return archiveExportSelection{}, err
	}
	paths, err := e.documentPaths(id, "/", false)
	if err != nil {
		return archiveExportSelection{}, err
	}
	return archiveExportSelection{box: id, rootDir: "/", folder: box.Name, paths: paths}, e.guard()
}
func (e *archiveExportContext) selectedTrees(box string, paths []string) (repairIOResult map[string]*parse.Tree, repairIOErr error) {
	if _, err := e.box(box); err != nil {
		return nil, err
	}
	io, closeIO, err := e.binding.scope.commit.r.openRepairIOChecked(e.binding.scope.ctx, false)
	if err != nil {
		return nil, err
	}
	defer func() {
		repairIOErr = joinRepairCloseError(repairIOErr, closeIO())
		if repairIOErr != nil {
			repairIOResult = nil
		}
	}()
	// Capture the exact bytes parsed by Load; never publish a global revision.
	var capturedTree *parse.Tree
	var capturedRaw []byte
	io.Remember = func(tree *parse.Tree, raw []byte) {
		capturedTree, capturedRaw = tree, bytes.Clone(raw)
	}
	trees := map[string]*parse.Tree{}
	for _, p := range paths {
		if err = e.guard(); err != nil {
			return nil, err
		}
		if !util.HasDocExt(p) {
			continue
		}
		if snapshot, ok := e.documents[util.GetTreeID(p)]; ok {
			if err := snapshot.check(snapshot.tree, box, p); err != nil {
				return nil, err
			}
			trees[snapshot.id] = snapshot.tree
			continue
		}
		capturedTree, capturedRaw = nil, nil
		tree, err := io.Load(e.binding.scope.ctx, box, p, e.binding.scope.commit.parser(), false)
		if err != nil {
			return nil, err
		}
		if capturedTree != tree || capturedRaw == nil {
			return nil, errors.New("export_document_bytes_unbound")
		}
		if err = e.guard(); err != nil {
			return nil, err
		}
		e.documents[util.GetTreeID(p)] = archiveExportDocument{tree: tree, id: tree.ID, box: box, path: p, raw: capturedRaw}
		// Keep the original old-Spec inline conversion on the detached export
		// candidate, without migrating the source file during selection.
		if tree.Root.Spec == "" {
			parse.NestedInlines2FlattedSpans(tree, false)
			tree.Root.Spec = "1"
		}
		trees[tree.ID] = tree
	}
	return trees, e.guard()
}
func (e *archiveExportContext) writeSort(folder, box string, trees map[string]*parse.Tree) (err error) {
	if _, err := e.box(box); err != nil {
		return err
	}
	raw, err := e.binding.scope.source(box + "/.siyuan/sort.json")
	missing := indexrepair.IsSourceNotExist(err)
	if err != nil && !missing {
		return err
	}
	full := map[string]int{}
	if !missing {
		if err = gulu.JSON.UnmarshalJSON(raw, &full); err != nil {
			return err
		}
		if full == nil {
			return errors.New("invalid_export_sort")
		}
	}
	selected := map[string]int{}
	for id := range trees {
		if order, ok := full[id]; ok {
			selected[id] = order
		}
	}
	root, closeRoot, err := e.openExportIO(folder)
	if err != nil {
		return err
	}
	defer func() { err = joinRepairCloseError(err, closeRoot()) }()
	source := indexrepair.Source{Data: root, Authorize: func(ctx context.Context) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		return e.guard()
	}}
	const target = ".siyuan/sort.json"
	before, err := source.Read(e.binding.scope.ctx, target)
	if err != nil && !indexrepair.IsSourceNotExist(err) {
		return err
	}
	if len(selected) == 0 {
		if err = source.Remove(e.binding.scope.ctx, target); err != nil && !os.IsNotExist(err) {
			return err
		}
		return e.guard()
	}
	after, err := gulu.JSON.MarshalJSON(selected)
	if err != nil {
		return err
	}
	return source.Rewrite(e.binding.scope.ctx, target, before, after)
}
