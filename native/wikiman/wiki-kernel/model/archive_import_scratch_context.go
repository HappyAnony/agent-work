// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"github.com/88250/lute/render"
	"io/fs"
	"os"
	"path"
	"sort"
	"strings"
	"sync"
	"wiki-kernel/av"
	"wiki-kernel/internal/indexrepair"
	"wiki-kernel/util"
)

func (e *ArchiveImportContext) archiveRootCloser() func(*os.Root) error {
	if e.closeRoot != nil {
		return e.closeRoot
	}
	return func(root *os.Root) error { return root.Close() }
}
func (e *ArchiveImportContext) openScratch(folder string) (indexrepair.Source, func() error, error) {
	if err := e.guard(); err != nil {
		return indexrepair.Source{}, nil, err
	}
	closeRoot := e.archiveRootCloser()
	root, rel, owned, err := e.borrowImportScratch(folder)
	if err != nil {
		return indexrepair.Source{}, nil, err
	}
	if owned && rel != "." {
		if err = (indexrepair.Source{Data: root}).Check(rel, false); err != nil {
			return indexrepair.Source{}, nil, errors.Join(err, closeRoot(root))
		}
		child, openErr := root.OpenRoot(rel)
		err = errors.Join(openErr, closeRoot(root))
		if err != nil {
			if child != nil {
				err = errors.Join(err, closeRoot(child))
			}
			return indexrepair.Source{}, nil, err
		}
		root = child
	} else if !owned {
		root, err = openArchivePublicationSource(folder)
		if err != nil {
			return indexrepair.Source{}, nil, err
		}
	}
	var once sync.Once
	var closeErr error
	closeIO := func() error { once.Do(func() { closeErr = closeRoot(root) }); return closeErr }
	return indexrepair.Source{Data: root, Authorize: func(ctx context.Context) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		return e.guard()
	}}, closeIO, nil
}

type archiveScratchWalkIO struct {
	readDir func(context.Context, indexrepair.Source, string) ([]fs.DirEntry, error)
}

func (e *ArchiveImportContext) walkScratch(source indexrepair.Source, name string, fn func(string, fs.DirEntry) error) error {
	return e.walkScratchWithIO(source, name, fn, archiveScratchWalkIO{})
}
func (e *ArchiveImportContext) walkScratchWithIO(source indexrepair.Source, name string, fn func(string, fs.DirEntry) error, op archiveScratchWalkIO) error {
	if err := e.guard(); err != nil {
		return err
	}
	if name != "." {
		if err := source.Check(name, false); err != nil {
			return err
		}
	}
	if op.readDir == nil {
		op.readDir = func(ctx context.Context, source indexrepair.Source, name string) ([]fs.DirEntry, error) {
			return source.ReadDir(ctx, name)
		}
	}
	info, err := source.Data.Stat(name)
	if err != nil {
		return err
	}
	var walk func(string, fs.DirEntry) error
	walk = func(name string, entry fs.DirEntry) error {
		if guardErr := e.guard(); guardErr != nil {
			return guardErr
		}
		if entry.Type()&fs.ModeSymlink != 0 {
			return indexrepair.ErrSourcePath
		}
		if callbackErr := fn(name, entry); callbackErr != nil || !entry.IsDir() {
			if callbackErr == fs.SkipDir && entry.IsDir() {
				return nil
			}
			return callbackErr
		}
		entries, readErr := op.readDir(e.ctx, source, name)
		if readErr != nil {
			return readErr
		}
		if guardErr := e.guard(); guardErr != nil {
			return guardErr
		}
		for _, child := range entries {
			if walkErr := walk(path.Join(name, child.Name()), child); walkErr != nil {
				if walkErr == fs.SkipDir {
					break
				}
				return walkErr
			}
		}
		return nil
	}
	err = walk(name, fs.FileInfoToDirEntry(info))
	if err == fs.SkipDir || err == fs.SkipAll {
		return nil
	}
	return err
}

// Read/validate the entire AV set before changing its owned scratch files.
// Keep original whole-JSON substitutions; this does not upgrade old AV specs.
func (e *ArchiveImportContext) remapScratchAVs(folder string, blockIDs map[string]string, newID func() string) (mapped map[string]string, present bool, err error) {
	return e.remapScratchAVsWithIO(folder, blockIDs, newID, archiveScratchWalkIO{})
}
func (e *ArchiveImportContext) remapScratchAVsWithIO(folder string, blockIDs map[string]string, newID func() string, op archiveScratchWalkIO) (mapped map[string]string, present bool, err error) {
	if newID == nil {
		return nil, false, errors.New("archive_av_id_generator_unbound")
	}
	source, closeIO, err := e.openScratch(folder)
	if err != nil {
		return nil, false, err
	}
	defer func() {
		err = errors.Join(err, closeIO())
		if err != nil {
			mapped = nil
		}
	}()
	const base = "storage/av"
	if err = source.Check(base, false); os.IsNotExist(err) {
		return map[string]string{}, false, nil
	} else if err != nil {
		return nil, false, err
	}
	info, err := source.Data.Stat(base)
	if err != nil {
		return nil, false, err
	}
	if !info.IsDir() {
		return nil, false, indexrepair.ErrSourcePath
	}
	var files []string
	raws := map[string][]byte{}
	ids := map[string]string{}
	oldIDs := map[string]bool{}
	err = e.walkScratchWithIO(source, base, func(name string, d fs.DirEntry) error {
		if d.IsDir() {
			return nil
		}
		old := strings.TrimSuffix(d.Name(), ".json")
		if !strings.HasSuffix(d.Name(), ".json") || !ast.IsNodeIDPattern(old) {
			return nil
		}
		if oldIDs[old] {
			return errors.New("ambiguous_archive_av_id")
		}
		oldIDs[old] = true
		raw, err := source.Read(e.ctx, name)
		if err != nil {
			return err
		}
		a, err := av.DecodeAttributeView(raw)
		if err != nil {
			return err
		}
		if a == nil || a.ID != old {
			return errors.New("invalid_archive_av_source")
		}
		files = append(files, name)
		raws[name] = raw
		return nil
	}, op)
	if err != nil {
		return nil, true, err
	}
	used := map[string]bool{}
	for _, name := range files {
		if err = e.guard(); err != nil {
			return nil, true, err
		}
		id := newID()
		if !ast.IsNodeIDPattern(id) || oldIDs[id] || used[id] {
			return nil, true, errors.New("archive_av_id_collision")
		}
		used[id] = true
		old := strings.TrimSuffix(path.Base(name), ".json")
		ids[old] = id
		target := path.Join(path.Dir(name), id+".json")
		if err = source.Check(target, true); err != nil {
			return nil, true, err
		}
		if _, err = source.Data.Lstat(target); err == nil {
			return nil, true, indexrepair.ErrSourceChanged
		} else if !os.IsNotExist(err) {
			return nil, true, err
		}
	}
	var replacements []string
	keys := make([]string, 0, len(blockIDs))
	for old := range blockIDs {
		keys = append(keys, old)
	}
	sort.Strings(keys)
	for _, old := range keys {
		replacements = append(replacements, old, blockIDs[old])
	}
	blocks := strings.NewReplacer(replacements...)
	avKeys := make([]string, 0, len(ids))
	for old := range ids {
		avKeys = append(avKeys, old)
	}
	sort.Strings(avKeys)
	for _, name := range files {
		raw := raws[name]
		after := bytes.Clone(raw)
		for _, old := range avKeys {
			after = bytes.ReplaceAll(after, []byte(old), []byte(ids[old]))
		}
		after = []byte(blocks.Replace(string(after)))
		if !bytes.Equal(raw, after) {
			if err = source.Rewrite(e.ctx, name, raw, after); err != nil {
				return nil, true, err
			}
		}
		target := path.Join(path.Dir(name), ids[strings.TrimSuffix(path.Base(name), ".json")]+".json")
		if err = source.Rename(e.ctx, name, target); err != nil {
			return nil, true, err
		}
	}
	return ids, true, e.guard()
}
func (e *ArchiveImportContext) writeScratchDocuments(folder string, trees map[string]*parse.Tree, engine *lute.Lute) (err error) {
	source, closeIO, err := e.openScratch(folder)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, closeIO()) }()
	ids := make([]string, 0, len(trees))
	for id := range trees {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if err = e.guard(); err != nil {
			return err
		}
		tree := trees[id]
		if tree == nil || tree.Root == nil || tree.ID != id || tree.Root.ID != id || !ast.IsNodeIDPattern(id) || !strings.HasPrefix(tree.Path, "/") || path.Clean(tree.Path) != tree.Path {
			return errors.New("invalid_archive_scratch_tree")
		}
		util.PushEndlessProgress(e.language(73) + " " + fmt.Sprintf(e.language(70), tree.Root.IALAttr("title")))
		name := strings.TrimPrefix(tree.Path, "/")
		before, err := source.Read(e.ctx, name)
		if err != nil {
			return err
		}
		if tree.Root.Spec == "" {
			parse.NestedInlines2FlattedSpans(tree, false)
			tree.Root.Spec = "1"
		}
		data := render.NewJSONRenderer(tree, engine.RenderOptions).Render()
		if !e.binding.scope.commit.r.singleLine {
			var buf bytes.Buffer
			if err = json.Indent(&buf, data, "", "\t"); err != nil {
				return err
			}
			data = buf.Bytes()
		}
		if err = source.Rewrite(e.ctx, name, before, data); err != nil {
			return err
		}
		target := path.Join(path.Dir(name), id+util.DocExt)
		if target != name {
			if err = source.Rename(e.ctx, name, target); err != nil {
				return err
			}
		}
	}
	return nil
}

// Rename deepest components first. Each target must be absent; unrelated
// prefix-like siblings never enter an ancestor's path rewrite.
func (e *ArchiveImportContext) renameScratchDirectories(folder string, ids map[string]string) (err error) {
	return e.renameScratchDirectoriesWithIO(folder, ids, archiveScratchWalkIO{})
}
func (e *ArchiveImportContext) renameScratchDirectoriesWithIO(folder string, ids map[string]string, op archiveScratchWalkIO) (err error) {
	source, closeIO, err := e.openScratch(folder)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, closeIO()) }()
	var directories []string
	err = e.walkScratchWithIO(source, ".", func(name string, d fs.DirEntry) error {
		if name != "." && d.IsDir() && ast.IsNodeIDPattern(d.Name()) {
			directories = append(directories, name)
		}
		return nil
	}, op)
	if err != nil {
		return err
	}
	sort.Slice(directories, func(i, j int) bool {
		a, b := strings.Count(directories[i], "/"), strings.Count(directories[j], "/")
		if a == b {
			return directories[i] < directories[j]
		}
		return a > b
	})
	for _, name := range directories {
		next := ids[path.Base(name)]
		if next == "" || next == path.Base(name) {
			continue
		}
		if !ast.IsNodeIDPattern(next) {
			return indexrepair.ErrSourcePath
		}
		if err = source.Rename(e.ctx, name, path.Join(path.Dir(name), next)); err != nil {
			return err
		}
	}
	return e.guard()
}
func (e *ArchiveImportContext) removeScratchStorage(folder string) (err error) {
	return e.removeScratchStorageWithIO(folder, archiveScratchWalkIO{})
}
func (e *ArchiveImportContext) removeScratchStorageWithIO(folder string, op archiveScratchWalkIO) (err error) {
	source, closeIO, err := e.openScratch(folder)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, closeIO()) }()
	if err = source.Check("storage", false); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	if err = e.walkScratchWithIO(source, "storage", func(string, fs.DirEntry) error { return nil }, op); err != nil {
		return err
	}
	if err = e.guard(); err != nil {
		return err
	}
	return source.Data.RemoveAll("storage")
}
