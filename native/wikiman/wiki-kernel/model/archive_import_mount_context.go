// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/88250/lute"
	"github.com/88250/lute/parse"
	"io/fs"
	"os"
	"path"
	"strings"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/indexrepair"
	"wiki-kernel/sql"
	"wiki-kernel/util"
)

type archiveMountIO struct {
	walk  archiveScratchWalkIO
	merge func(string, string) error
}

type archiveMountEntry struct {
	name string
	info fs.FileInfo
}

type archiveMountManifest struct {
	directories []archiveMountEntry
	documents   []archiveMountEntry
}

func captureArchiveMountEntry(source indexrepair.Source, name string, entry fs.DirEntry) (archiveMountEntry, error) {
	info, err := source.Data.Lstat(name)
	if err != nil {
		return archiveMountEntry{}, err
	}
	if info.Mode()&fs.ModeSymlink != 0 || info.IsDir() != entry.IsDir() || info.Mode().Type() != entry.Type() {
		return archiveMountEntry{}, indexrepair.ErrSourceChanged
	}
	return archiveMountEntry{name: name, info: info}, nil
}

func (e *ArchiveImportContext) checkArchiveMountEntry(source indexrepair.Source, candidate archiveMountEntry) error {
	if err := e.guard(); err != nil {
		return err
	}
	if candidate.info == nil {
		return indexrepair.ErrSourceChanged
	}
	if candidate.name != "." {
		if err := source.Check(candidate.name, false); err != nil {
			return err
		}
	}
	now, err := source.Data.Lstat(candidate.name)
	if err != nil {
		return err
	}
	if now.Mode()&fs.ModeSymlink != 0 || now.Mode().Type() != candidate.info.Mode().Type() || !os.SameFile(now, candidate.info) || now.Size() != candidate.info.Size() || !now.ModTime().Equal(candidate.info.ModTime()) {
		return indexrepair.ErrSourceChanged
	}
	return nil
}

func (e *ArchiveImportContext) checkArchiveMountManifest(source indexrepair.Source, manifest archiveMountManifest) error {
	for _, candidate := range manifest.directories {
		if err := e.checkArchiveMountEntry(source, candidate); err != nil {
			return err
		}
	}
	for _, candidate := range manifest.documents {
		if err := e.checkArchiveMountEntry(source, candidate); err != nil {
			return err
		}
	}
	return e.guard()
}

func (e *ArchiveImportContext) archiveTarget(box, to string) (string, error) {
	if err := validateArchiveDestination(box, to); err != nil {
		return "", err
	}
	if err := e.guard(); err != nil {
		return "", err
	}
	if to == "/" {
		return box, nil
	}
	rows, err := e.binding.scope.commit.r.registry.Query(e.ctx, indexdb.BlockTreeFilter{Kind: "root-path", BoxID: box, Value: to})
	if err != nil {
		return "", err
	}
	if len(rows) == 0 {
		return "", ErrBlockNotFound
	}
	if rows[0].BoxID != box || rows[0].Path != to {
		return "", indexrepair.ErrSourcePath
	}
	return path.Join(box, strings.TrimPrefix(util.StripDocExt(rows[0].Path), "/")), e.guard()
}
func (e *ArchiveImportContext) mountArchive(folder, box, to string, engine *lute.Lute) (err error) {
	return e.mountArchiveWithIO(folder, box, to, engine, archiveMountIO{})
}

func (e *ArchiveImportContext) mountArchiveWithIO(folder, box, to string, engine *lute.Lute, op archiveMountIO) (err error) {
	target, err := e.archiveTarget(box, to)
	if err != nil {
		return err
	}
	source, closeSource, err := e.openScratch(folder)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, closeSource()) }()
	manifest := archiveMountManifest{}
	err = e.walkScratchWithIO(source, ".", func(name string, entry fs.DirEntry) error {
		if entry.IsDir() {
			candidate, captureErr := captureArchiveMountEntry(source, name, entry)
			if captureErr != nil {
				return captureErr
			}
			manifest.directories = append(manifest.directories, candidate)
			if name != "." && strings.HasPrefix(entry.Name(), ".") {
				return fs.SkipDir
			}
			return nil
		}
		if !util.HasDocExt(entry.Name()) {
			return nil
		}
		candidate, captureErr := captureArchiveMountEntry(source, name, entry)
		if captureErr != nil {
			return captureErr
		}
		if !candidate.info.Mode().IsRegular() {
			return indexrepair.ErrSourcePath
		}
		manifest.documents = append(manifest.documents, candidate)
		return nil
	}, op.walk)
	if err != nil {
		return err
	}
	if err = e.checkArchiveMountManifest(source, manifest); err != nil {
		return err
	}
	if op.merge == nil {
		op.merge = e.mergeArchiveDirectory
	}
	if err = op.merge(folder, target); err != nil {
		return err
	}
	if err = e.checkArchiveMountManifest(source, manifest); err != nil {
		return err
	}
	r := e.binding.scope.commit.r
	io, closeIO, err := r.openRepairIOCheckedWithCloser(e.ctx, true, e.archiveRootCloser())
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, closeIO()) }()
	q := sql.LegacyMainQueue{DB: r.main.DB}
	admitted := false
	// Even if a later tree fails, join already-admitted indexing before returning.
	defer func() {
		if admitted {
			err = errors.Join(err, q.Flush(context.WithoutCancel(e.ctx)))
		}
	}()
	admit := func(ctx context.Context, t *parse.Tree) error {
		if err := e.guard(); err != nil {
			return err
		}
		if err := r.registry.Upsert(ctx, t); err != nil {
			return err
		}
		if err := q.Index(ctx, t); err != nil {
			return err
		}
		admitted = true
		return nil
	}
	// Preserve original parent/empty-document repair indexing through fixed ports.
	io.Upsert = admit
	for _, candidate := range manifest.documents {
		if err = e.guard(); err != nil {
			return err
		}
		name := candidate.name
		p := "/" + strings.TrimPrefix(path.Join(target, name), box+"/")
		tree, loadErr := io.Load(e.ctx, box, p, engine, true)
		if loadErr != nil {
			return loadErr
		}
		if err = admit(e.ctx, tree); err != nil {
			return err
		}
		util.PushEndlessProgress(e.language(73) + " " + fmt.Sprintf(e.language(70), tree.Root.IALAttr("title")))
	}
	return e.guard()
}
func (e *ArchiveImportContext) completeArchive(ids map[string]string) error {
	if err := e.guard(); err != nil {
		return err
	}
	raw, err := json.Marshal(ids)
	if err != nil {
		return err
	}
	return e.binding.scope.commit.completeUI(e.ctx, []commitNotice{{kind: "ids", payload: raw}}, true)
}

func (e *ArchiveImportContext) finishArchive(folder, box, to string, engine *lute.Lute, ids map[string]string) error {
	if err := e.mountArchive(folder, box, to, engine); err != nil {
		return err
	}
	return e.completeArchive(ids)
}
