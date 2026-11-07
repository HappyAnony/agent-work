// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"bytes"
	"context"
	"errors"
	"github.com/88250/gulu"
	"io/fs"
	"os"
	"path"
	"wiki-kernel/internal/indexrepair"
	"wiki-kernel/util"
)

type archiveMergeIO struct {
	walk archiveScratchWalkIO
	read func(context.Context, indexrepair.Source, string) ([]byte, error)
}

type archiveMergeEntry struct {
	name string
	info fs.FileInfo
}

type archiveMergeManifest struct {
	entries []archiveMergeEntry
}

type archiveResourceIO struct {
	walk      archiveScratchWalkIO
	merge     func(string, string) error
	removeAll func(*os.Root, string) error
}

type archiveEmojiIO struct {
	readDir func(context.Context, indexrepair.Source, string) ([]fs.DirEntry, error)
	rename  func(*os.Root, string, string) error
}

type archiveEmojiRename struct {
	from, to string
	info     fs.FileInfo
	dir      bool
}

type archiveResourceEntry struct {
	name string
	info fs.FileInfo
}

type archiveResourceManifest struct {
	directories []archiveResourceEntry
	candidates  []archiveResourceEntry
}

func captureArchiveMergeEntry(source indexrepair.Source, name string, entry fs.DirEntry) (archiveMergeEntry, error) {
	info, err := source.Data.Lstat(name)
	if err != nil {
		return archiveMergeEntry{}, err
	}
	if info.Mode()&fs.ModeSymlink != 0 || info.IsDir() != entry.IsDir() || info.Mode().Type() != entry.Type() {
		return archiveMergeEntry{}, indexrepair.ErrSourceChanged
	}
	if !info.IsDir() && !info.Mode().IsRegular() {
		return archiveMergeEntry{}, indexrepair.ErrSourcePath
	}
	return archiveMergeEntry{name: name, info: info}, nil
}

func (e *ArchiveImportContext) checkArchiveMergeEntry(source indexrepair.Source, candidate archiveMergeEntry) error {
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

func (e *ArchiveImportContext) checkArchiveMergeManifest(source indexrepair.Source, manifest archiveMergeManifest) error {
	for _, candidate := range manifest.entries {
		if err := e.checkArchiveMergeEntry(source, candidate); err != nil {
			return err
		}
	}
	return e.guard()
}

func captureArchiveResourceEntry(source indexrepair.Source, name string, entry fs.DirEntry) (archiveResourceEntry, error) {
	info, err := source.Data.Lstat(name)
	if err != nil {
		return archiveResourceEntry{}, err
	}
	if info.Mode()&fs.ModeSymlink != 0 || !info.IsDir() || !entry.IsDir() || info.Mode().Type() != entry.Type() {
		return archiveResourceEntry{}, indexrepair.ErrSourceChanged
	}
	return archiveResourceEntry{name: name, info: info}, nil
}

func (e *ArchiveImportContext) checkArchiveResourceEntry(source indexrepair.Source, candidate archiveResourceEntry) error {
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
	if now.Mode()&fs.ModeSymlink != 0 || !now.IsDir() || !os.SameFile(now, candidate.info) || now.Size() != candidate.info.Size() || !now.ModTime().Equal(candidate.info.ModTime()) {
		return indexrepair.ErrSourceChanged
	}
	return nil
}

func (e *ArchiveImportContext) checkArchiveResourceManifest(source indexrepair.Source, manifest archiveResourceManifest) error {
	for _, candidate := range manifest.directories {
		if err := e.checkArchiveResourceEntry(source, candidate); err != nil {
			return err
		}
	}
	return e.guard()
}

func openArchivePublicationSource(folder string) (*os.Root, error) {
	info, err := os.Lstat(folder)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&fs.ModeSymlink != 0 {
		return nil, indexrepair.ErrSourcePath
	}
	return os.OpenRoot(folder)
}

// Incoming files retain the original overwrite/merge policy, but each existing
// destination is version checked. This is sequential publication, not rollback.
func (e *ArchiveImportContext) rewriteArchiveFile(target indexrepair.Source, name string, before, after []byte) error {
	if err := e.guard(); err != nil {
		return err
	}
	return target.Rewrite(e.ctx, name, before, after)
}
func (e *ArchiveImportContext) mergeArchiveDirectory(folder, destination string) (err error) {
	return e.mergeArchiveDirectoryWithIO(folder, destination, archiveMergeIO{})
}

func (e *ArchiveImportContext) mergeArchiveDirectoryWithIO(folder, destination string, op archiveMergeIO) (err error) {
	if err := e.guard(); err != nil {
		return err
	}
	read, closeSource, err := e.openScratch(folder)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, closeSource()) }()
	manifest := archiveMergeManifest{}
	err = e.walkScratchWithIO(read, ".", func(name string, entry fs.DirEntry) error {
		candidate, captureErr := captureArchiveMergeEntry(read, name, entry)
		if captureErr != nil {
			return captureErr
		}
		manifest.entries = append(manifest.entries, candidate)
		return nil
	}, op.walk)
	if err != nil {
		return err
	}
	if err = e.checkArchiveMergeManifest(read, manifest); err != nil {
		return err
	}
	if op.read == nil {
		op.read = func(ctx context.Context, source indexrepair.Source, name string) ([]byte, error) {
			return source.Read(ctx, name)
		}
	}
	io, closeIO, err := e.binding.scope.commit.r.openRepairIOCheckedWithCloser(e.ctx, true, e.archiveRootCloser())
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, closeIO()) }()
	for _, candidate := range manifest.entries {
		if err := e.guard(); err != nil {
			return err
		}
		if err := e.checkArchiveMergeEntry(read, candidate); err != nil {
			return err
		}
		// A full data archive merges its children into the existing data root.
		if candidate.name == "." && destination == "." {
			continue
		}
		target := path.Join(destination, candidate.name)
		if err := io.Source.Check(target, true); err != nil {
			return err
		}
		if candidate.info.IsDir() {
			if err := io.Source.Data.MkdirAll(target, 0755); err != nil {
				return err
			}
			if err := io.Source.Check(target, false); err != nil {
				return err
			}
			continue
		}
		incoming, err := op.read(e.ctx, read, candidate.name)
		if err != nil {
			return err
		}
		if err := e.checkArchiveMergeEntry(read, candidate); err != nil {
			return err
		}
		before, err := io.Source.Read(e.ctx, target)
		if err != nil && !indexrepair.IsSourceNotExist(err) {
			return err
		}
		if err == nil && bytes.Equal(before, incoming) {
			continue
		}
		if err = e.rewriteArchiveFile(io.Source, target, before, incoming); err != nil {
			return err
		}
	}
	return e.checkArchiveMergeManifest(read, manifest)
}
func (e *ArchiveImportContext) mergeArchiveSort(folder, box string, ids map[string]string) (err error) {
	if err := validateArchiveDestination(box, "/"); err != nil {
		return err
	}
	if err := e.guard(); err != nil {
		return err
	}
	source, closeSource, err := e.openScratch(folder)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, closeSource()) }()
	root := source.Data
	const name = ".siyuan/sort.json"
	raw, err := source.Read(e.ctx, name)
	if indexrepair.IsSourceNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	incoming := map[string]int{}
	if err = gulu.JSON.UnmarshalJSON(raw, &incoming); err != nil {
		return err
	}
	if incoming == nil {
		return errors.New("invalid_archive_sort")
	}
	io, closeIO, err := e.binding.scope.commit.r.openRepairIOCheckedWithCloser(e.ctx, true, e.archiveRootCloser())
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, closeIO()) }()
	target := box + "/" + name
	before, err := io.Source.Read(e.ctx, target)
	if err != nil && !indexrepair.IsSourceNotExist(err) {
		return err
	}
	merged := map[string]int{}
	if err == nil {
		if err = gulu.JSON.UnmarshalJSON(before, &merged); err != nil {
			return err
		}
		if merged == nil {
			return errors.New("invalid_notebook_sort")
		}
	}
	for old, order := range incoming {
		if next := ids[old]; next != "" {
			merged[next] = order
		}
	}
	after, err := gulu.JSON.MarshalJSON(merged)
	if err != nil {
		return err
	}
	if err = e.rewriteArchiveFile(io.Source, target, before, after); err != nil {
		return err
	}
	if err = e.guard(); err != nil {
		return err
	}
	return root.Remove(name)
}
func (e *ArchiveImportContext) sanitizeArchiveEmojis(root *os.Root, folder string) error {
	return e.sanitizeArchiveEmojisWithIO(root, folder, archiveEmojiIO{})
}

func (e *ArchiveImportContext) sanitizeArchiveEmojisWithIO(root *os.Root, folder string, op archiveEmojiIO) error {
	if err := e.guard(); err != nil {
		return err
	}
	source := indexrepair.Source{Data: root}
	if err := source.Check(folder, false); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	if op.readDir == nil {
		op.readDir = func(ctx context.Context, source indexrepair.Source, name string) ([]fs.DirEntry, error) {
			return source.ReadDir(ctx, name)
		}
	}
	if op.rename == nil {
		op.rename = func(root *os.Root, from, to string) error { return root.Rename(from, to) }
	}
	entries, err := op.readDir(e.ctx, source, folder)
	if err != nil {
		return err
	}
	plans := make([]archiveEmojiRename, 0, len(entries))
	targets := map[string]bool{}
	for _, d := range entries {
		from := path.Join(folder, d.Name())
		if err := source.Check(from, false); err != nil {
			return err
		}
		info, err := root.Lstat(from)
		if err != nil {
			return err
		}
		if info.Mode()&fs.ModeSymlink != 0 || info.IsDir() != d.IsDir() || info.Mode().Type() != d.Type() {
			return indexrepair.ErrSourceChanged
		}
		name := d.Name()
		if !util.IsValidUploadFileName(name) {
			name = util.FilterUploadEmojiFileName(name)
		}
		if name == "" || name == "." || name == ".." || path.Base(name) != name {
			return indexrepair.ErrSourcePath
		}
		to := path.Join(folder, name)
		if to != from {
			if targets[to] {
				return os.ErrExist
			}
			targets[to] = true
			if _, err := root.Lstat(to); err == nil {
				return os.ErrExist
			} else if !os.IsNotExist(err) {
				return err
			}
		}
		plans = append(plans, archiveEmojiRename{from: from, to: to, info: info, dir: d.IsDir()})
	}
	for _, plan := range plans {
		if err := e.guard(); err != nil {
			return err
		}
		now, err := root.Lstat(plan.from)
		if err != nil {
			return err
		}
		if now.Mode()&fs.ModeSymlink != 0 || !os.SameFile(now, plan.info) || now.Size() != plan.info.Size() || !now.ModTime().Equal(plan.info.ModTime()) {
			return indexrepair.ErrSourceChanged
		}
		if plan.to != plan.from {
			if err := op.rename(root, plan.from, plan.to); err != nil {
				return err
			}
		}
		if plan.dir {
			if err := e.sanitizeArchiveEmojisWithIO(root, plan.to, op); err != nil {
				return err
			}
		}
	}
	return nil
}
func (e *ArchiveImportContext) publishArchiveResources(folder, kind string) (err error) {
	return e.publishArchiveResourcesWithIO(folder, kind, archiveResourceIO{})
}

func (e *ArchiveImportContext) publishArchiveResourcesWithIO(folder, kind string, op archiveResourceIO) (err error) {
	if kind != "assets" && kind != "emojis" {
		return indexrepair.ErrSourcePath
	}
	if err := e.guard(); err != nil {
		return err
	}
	source, closeSource, err := e.openScratch(folder)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, closeSource()) }()
	root := source.Data
	if kind == "emojis" {
		if err = e.sanitizeArchiveEmojis(root, "emojis"); err != nil {
			return err
		}
	}
	manifest := archiveResourceManifest{}
	err = e.walkScratchWithIO(source, ".", func(name string, entry fs.DirEntry) error {
		if !entry.IsDir() {
			return nil
		}
		candidate, captureErr := captureArchiveResourceEntry(source, name, entry)
		if captureErr != nil {
			return captureErr
		}
		manifest.directories = append(manifest.directories, candidate)
		if entry.Name() == kind {
			manifest.candidates = append(manifest.candidates, candidate)
			return fs.SkipDir
		}
		return nil
	}, op.walk)
	if err != nil {
		return err
	}
	if err = e.checkArchiveResourceManifest(source, manifest); err != nil {
		return err
	}
	if op.merge == nil {
		op.merge = e.mergeArchiveDirectory
	}
	if op.removeAll == nil {
		op.removeAll = func(root *os.Root, name string) error { return root.RemoveAll(name) }
	}
	for _, candidate := range manifest.candidates {
		if err = e.checkArchiveResourceEntry(source, candidate); err != nil {
			return err
		}
		// The extraction root is owned scratch. Destination IO is independently rooted.
		if err = op.merge(path.Join(folder, candidate.name), kind); err != nil {
			return err
		}
		if err = e.guard(); err != nil {
			return err
		}
		if err = e.checkArchiveResourceEntry(source, candidate); err != nil {
			return err
		}
		if err = op.removeAll(root, candidate.name); err != nil {
			return err
		}
		if err = e.guard(); err != nil {
			return err
		}
	}
	return nil
}
