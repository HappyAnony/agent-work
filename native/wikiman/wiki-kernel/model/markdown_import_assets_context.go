// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/88250/lute/ast"
	"github.com/88250/lute/html"
	"github.com/88250/lute/parse"
	"github.com/siyuan-note/filelock"
	"wiki-kernel/internal/indexrepair"
	"wiki-kernel/util"
)

type markdownImportAssets struct {
	source     *markdownImportSource
	authority  *ArchiveImportContext
	target     indexrepair.Source
	identity   fs.FileInfo
	close      func() error
	extraGuard func() error
}

func (s *markdownImportSource) openAssets() (*markdownImportAssets, error) {
	if err := s.guard(); err != nil {
		return nil, err
	}
	m := s.preparation
	ioctx, closeIO, err := m.archive.binding.scope.commit.r.openRepairIOCheckedWithCloser(m.ctx, false, m.archive.archiveRootCloser())
	if err != nil {
		return nil, err
	}
	info, err := ioctx.Source.Data.Stat(".")
	if err != nil {
		return nil, errors.Join(err, closeIO())
	}
	a := &markdownImportAssets{source: s, authority: m.archive, target: ioctx.Source, identity: info, close: closeIO}
	if err = a.guard(); err != nil {
		return nil, errors.Join(err, closeIO())
	}
	return a, nil
}
func (a *markdownImportAssets) guard() error {
	if a == nil || a.authority == nil {
		return errors.New("image_assets_context_unbound")
	}
	if err := a.authority.guard(); err != nil {
		return err
	}
	if a.source != nil {
		if err := a.source.guard(); err != nil {
			return err
		}
	}
	info, err := os.Lstat(a.authority.binding.scope.commit.r.data)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&fs.ModeSymlink != 0 || !os.SameFile(info, a.identity) {
		return indexrepair.ErrSourceChanged
	}
	if a.extraGuard != nil {
		return a.extraGuard()
	}
	return nil
}
func (a *markdownImportAssets) directory(box, docPath string) (string, error) {
	if err := validateArchiveDestination(box, docPath); err != nil {
		return "", err
	}
	if err := a.guard(); err != nil {
		return "", err
	}
	for _, folder := range []string{path.Join(box, path.Dir(strings.TrimPrefix(docPath, "/")), "assets"), path.Join(box, "assets"), "assets"} {
		if err := a.target.Check(folder, true); err != nil {
			return "", err
		}
		info, err := a.target.Data.Lstat(folder)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return "", err
		}
		if !info.IsDir() {
			return "", indexrepair.ErrSourcePath
		}
		return folder, a.guard()
	}
	return "assets", a.guard()
}

// Directory imports use their inventory; single-file imports can additionally
// borrow relative regular files beneath the selected file's captured parent.
func (s *markdownImportSource) assetEntry(current string) (entry markdownSourceEntry, exists bool, err error) {
	if err = s.guard(); err != nil {
		return
	}
	name, err := s.relative(current)
	if err != nil {
		return entry, false, err
	}
	if name != "." {
		if err = (indexrepair.Source{Data: s.root}).Check(name, false); err != nil {
			if os.IsNotExist(err) {
				return entry, false, nil
			}
			return entry, false, err
		}
	}
	info, err := s.root.Lstat(name)
	if err != nil {
		return entry, false, err
	}
	if !info.IsDir() && !info.Mode().IsRegular() {
		return entry, false, indexrepair.ErrSourcePath
	}
	for _, saved := range s.entries {
		if saved.name == name {
			return saved, true, s.check(saved)
		}
	}
	if s.directory {
		return entry, false, indexrepair.ErrSourceChanged
	}
	entry = markdownSourceEntry{name: name, info: info}
	return entry, true, s.check(entry)
}

type markdownAssetWriter struct {
	io.Writer
	guard func() error
}

func (w markdownAssetWriter) Write(p []byte) (int, error) {
	if err := w.guard(); err != nil {
		return 0, err
	}
	return w.Writer.Write(p)
}

// Only new generated asset names are published. Failed writes/close/verification
// remove this invocation's output by inode, never an unrelated replacement.
// Publication is sequential; no multi-file rollback or atomic visibility is claimed.
func (a *markdownImportAssets) publish(folder, name string, input io.Reader, verify, closeInput func() error, metadata ...fs.FileInfo) (err error) {
	var created bool
	var identity fs.FileInfo
	inputClosed := false
	closeOnce := func() error {
		if inputClosed {
			return nil
		}
		inputClosed = true
		return closeInput()
	}
	target := path.Join(folder, name)
	defer func() {
		err = errors.Join(err, closeOnce())
		if err == nil && created {
			err = a.guard()
			if err == nil {
				err = a.target.Check(target, false)
			}
			if err == nil {
				info, statErr := a.target.Data.Lstat(target)
				err = statErr
				if err == nil && (identity == nil || !os.SameFile(info, identity)) {
					err = indexrepair.ErrSourceChanged
				}
			}
		}
		if err != nil && created {
			cleanup := a.target.Check(target, false)
			if os.IsNotExist(cleanup) {
				return
			}
			if cleanup == nil {
				info, statErr := a.target.Data.Lstat(target)
				cleanup = statErr
				if cleanup == nil {
					if identity == nil || !os.SameFile(info, identity) {
						cleanup = indexrepair.ErrSourceChanged
					} else {
						cleanup = a.target.Data.Remove(target)
					}
				}
			}
			err = errors.Join(err, cleanup)
		}
	}()
	if !fs.ValidPath(folder) || strings.Contains(folder, "\\") || !fs.ValidPath(name) || path.Base(name) != name || strings.Contains(name, "\\") {
		return indexrepair.ErrSourcePath
	}
	if err = a.guard(); err != nil {
		return err
	}
	if err = a.target.Check(target, true); err != nil {
		return err
	}
	if err = a.target.Data.MkdirAll(folder, 0755); err != nil {
		return err
	}
	if err = a.target.Check(folder, false); err != nil {
		return err
	}
	output, err := a.target.Data.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return err
	}
	created = true
	err = func() (err error) {
		defer func() { err = errors.Join(err, closeOnce(), output.Close()) }()
		identity, err = output.Stat()
		if err != nil {
			return err
		}
		if len(metadata) > 0 {
			if err = output.Chmod(metadata[0].Mode().Perm()); err != nil {
				return err
			}
		}
		if err = archiveCopy(a.authority.ctx, markdownAssetWriter{Writer: output, guard: a.guard}, input); err != nil {
			return err
		}
		if err = output.Sync(); err != nil {
			return err
		}
		if err = verify(); err != nil {
			return err
		}
		if len(metadata) > 0 {
			if err = a.target.Check(target, false); err != nil {
				return err
			}
			info, statErr := a.target.Data.Lstat(target)
			if statErr != nil {
				return statErr
			}
			if !os.SameFile(info, identity) {
				return indexrepair.ErrSourceChanged
			}
			stamp := metadata[0].ModTime()
			if err = a.target.Data.Chtimes(target, stamp, stamp); err != nil {
				return err
			}
			if err = output.Sync(); err != nil {
				return err
			}
		}
		return a.guard()
	}()
	return err
}
func (a *markdownImportAssets) copy(current, folder string, names ...string) (string, error) {
	filelock.Lock(current)
	defer filelock.Unlock(current)
	entry, exists, err := a.source.assetEntry(current)
	if err != nil {
		return "", err
	}
	if !exists {
		return "", fs.ErrNotExist
	}
	if entry.info.IsDir() {
		return a.copyDirectory(current, folder, entry, names...)
	}
	input, err := a.source.root.Open(entry.name)
	if err != nil {
		return "", err
	}
	name := util.AssetName(util.FilterUploadFileName(filepath.Base(current)), ast.NewNodeID())
	if len(names) > 0 {
		name = names[0]
	}
	err = a.publish(folder, name, input, func() error {
		info, err := input.Stat()
		if err != nil {
			return err
		}
		if !os.SameFile(info, entry.info) {
			return indexrepair.ErrSourceChanged
		}
		return a.source.check(entry)
	}, func() error { return errors.Join(input.Close(), a.source.check(entry)) }, entry.info)
	if err != nil {
		return "", err
	}
	return name, nil
}

// Directory attachments retain original relative names, including empty folders.
// Completed children may remain after a later failure; no recursive rollback may
// erase files another writer added to the new directory.
func (a *markdownImportAssets) copyDirectory(current, folder string, entry markdownSourceEntry, names ...string) (string, error) {
	return a.copyDirectoryWithIO(current, folder, entry, archiveScratchWalkIO{}, names...)
}

func (a *markdownImportAssets) copyDirectoryWithIO(current, folder string, entry markdownSourceEntry, op archiveScratchWalkIO, names ...string) (string, error) {
	var entries []markdownSourceEntry
	if a.source.directory {
		prefix := ""
		if entry.name != "." {
			prefix = entry.name + "/"
		}
		for _, candidate := range a.source.entries {
			if candidate.name == entry.name || strings.HasPrefix(candidate.name, prefix) {
				entries = append(entries, candidate)
			}
		}
	} else {
		source := indexrepair.Source{Data: a.source.root, Authorize: func(context.Context) error { return a.guard() }}
		err := a.authority.walkScratchWithIO(source, entry.name, func(name string, d fs.DirEntry) error {
			if err := a.guard(); err != nil {
				return err
			}
			if name != "." {
				if err := source.Check(name, false); err != nil {
					return err
				}
			}
			info, err := a.source.root.Lstat(name)
			if err != nil {
				return err
			}
			if info.Mode()&fs.ModeSymlink != 0 || info.IsDir() != d.IsDir() || info.Mode().Type() != d.Type() {
				return indexrepair.ErrSourceChanged
			}
			if !info.IsDir() && !info.Mode().IsRegular() {
				return indexrepair.ErrSourcePath
			}
			entries = append(entries, markdownSourceEntry{name: name, info: info})
			return nil
		}, op)
		if err != nil {
			return "", err
		}
	}
	for _, candidate := range entries {
		if err := a.source.check(candidate); err != nil {
			return "", err
		}
	}
	if err := a.guard(); err != nil {
		return "", err
	}
	name := util.AssetName(util.FilterUploadFileName(filepath.Base(current)), ast.NewNodeID())
	if len(names) > 0 {
		name = names[0]
	}
	target := path.Join(folder, name)
	if err := a.target.Check(target, true); err != nil {
		return "", err
	}
	if err := a.target.Data.MkdirAll(folder, 0755); err != nil {
		return "", err
	}
	if err := a.target.Data.Mkdir(target, 0755); err != nil {
		return "", err
	}
	identity, err := a.target.Data.Lstat(target)
	if err != nil {
		return "", err
	}
	owned := *a
	owned.extraGuard = func() error {
		if err := a.target.Check(target, false); err != nil {
			return err
		}
		info, err := a.target.Data.Lstat(target)
		if err != nil {
			return err
		}
		if !os.SameFile(info, identity) {
			return indexrepair.ErrSourceChanged
		}
		return nil
	}
	for _, candidate := range entries {
		if err := owned.guard(); err != nil {
			return "", err
		}
		if err := a.source.check(candidate); err != nil {
			return "", err
		}
		if candidate.name == entry.name {
			continue
		}
		rel := strings.TrimPrefix(candidate.name, entry.name+"/")
		if entry.name == "." {
			rel = candidate.name
		}
		dest := path.Join(target, rel)
		if candidate.info.IsDir() {
			if err := a.target.Check(dest, true); err != nil {
				return "", err
			}
			if err := a.target.Data.MkdirAll(dest, 0755); err != nil {
				return "", err
			}
			continue
		}
		input, err := a.source.root.Open(candidate.name)
		if err != nil {
			return "", err
		}
		err = owned.publish(path.Dir(dest), path.Base(dest), input, func() error { return a.source.check(candidate) }, func() error { return errors.Join(input.Close(), a.source.check(candidate)) }, candidate.info)
		if err != nil {
			return "", err
		}
	}
	if err := a.source.check(entry); err != nil {
		return "", err
	}
	for i := len(entries) - 1; i >= 0; i-- {
		candidate := entries[i]
		if !candidate.info.IsDir() {
			continue
		}
		if err := owned.guard(); err != nil {
			return "", err
		}
		rel := strings.TrimPrefix(candidate.name, entry.name+"/")
		if entry.name == "." {
			rel = candidate.name
		}
		if candidate.name == entry.name {
			rel = ""
		}
		dest := path.Join(target, rel)
		if err := a.target.Check(dest, false); err != nil {
			return "", err
		}
		stamp := candidate.info.ModTime()
		if err := a.target.Data.Chtimes(dest, stamp, stamp); err != nil {
			return "", err
		}
	}
	return name, owned.guard()
}

func (a *markdownImportAssets) rewrite(tree *parse.Tree, currentPath, folder string, done map[string]string) error {
	var result error
	ast.Walk(tree.Root, func(n *ast.Node, enter bool) ast.WalkStatus {
		if result = a.guard(); result != nil {
			return ast.WalkStop
		}
		if !enter || (n.Type != ast.NodeLinkDest && !n.IsTextMarkType("a")) {
			return ast.WalkContinue
		}
		dest := n.TextMarkAHref
		if n.Type == ast.NodeLinkDest {
			dest = n.TokensStr()
		}
		if strings.HasPrefix(dest, "data:image") && strings.Contains(dest, ";base64,") {
			if result = a.base64Image(n, dest, folder); result != nil {
				return ast.WalkStop
			}
			return ast.WalkContinue
		}
		dest = string(html.DecodeDestination([]byte(dest)))
		if n.Type == ast.NodeLinkDest {
			n.Tokens = []byte(dest)
		} else {
			n.TextMarkAHref = dest
		}
		if !util.IsRelativePath(dest) || dest == "" {
			return ast.WalkContinue
		}
		absolute := filepath.Join(filepath.Dir(currentPath), dest)
		if a.source.directory && (strings.HasSuffix(absolute, ".md") || strings.HasSuffix(absolute, ".markdown")) {
			relative, err := a.source.relative(absolute)
			inAssets := false
			if err == nil {
				for _, part := range strings.Split(path.Dir(relative), "/") {
					if part == "assets" {
						inAssets = true
						break
					}
				}
			}
			if !inAssets {
				return ast.WalkContinue
			}
		}
		// Backslashes are rejected before filepath normalization on every platform.
		if strings.Contains(dest, "\\") {
			result = indexrepair.ErrSourcePath
			return ast.WalkStop
		}
		_, exists, err := a.source.assetEntry(absolute)
		if err != nil {
			result = err
			return ast.WalkStop
		}
		if !exists {
			return ast.WalkContinue
		}
		name := done[absolute]
		if name == "" {
			name, result = a.copy(absolute, folder)
			if result != nil {
				return ast.WalkStop
			}
			done[absolute] = name
		}
		if n.Type == ast.NodeLinkDest {
			n.Tokens = []byte("assets/" + name)
		} else {
			n.TextMarkAHref = "assets/" + name
		}
		return ast.WalkContinue
	})
	return result
}
