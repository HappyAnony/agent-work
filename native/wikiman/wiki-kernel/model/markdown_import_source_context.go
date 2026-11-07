// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"

	"github.com/siyuan-note/dataparser"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/indexrepair"
	"wiki-kernel/util"
)

type markdownSourceDirEntry struct {
	fs.DirEntry
	name string
}

func (d markdownSourceDirEntry) Name() string { return d.name }

type markdownSourceEntry struct {
	name string
	info fs.FileInfo
}
type markdownImportSource struct {
	preparation   *markdownImportPreparation
	local, folder string
	root          *os.Root
	identity      fs.FileInfo
	directory     bool
	entries       []markdownSourceEntry
	closeRoot     func(*os.Root) error
	once          sync.Once
	closeErr      error
}

func (s *markdownImportSource) close() error {
	s.once.Do(func() { s.closeErr = s.closeRoot(s.root) })
	return s.closeErr
}
func (s *markdownImportSource) guard() error {
	if err := s.preparation.guard(); err != nil {
		return err
	}
	info, err := os.Lstat(s.folder)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&fs.ModeSymlink != 0 || !os.SameFile(info, s.identity) {
		return indexrepair.ErrSourceChanged
	}
	return nil
}
func (s *markdownImportSource) check(entry markdownSourceEntry) error {
	if err := s.guard(); err != nil {
		return err
	}
	if entry.name != "." {
		if err := (indexrepair.Source{Data: s.root}).Check(entry.name, false); err != nil {
			return err
		}
	}
	info, err := s.root.Stat(entry.name)
	if err != nil {
		return err
	}
	if !os.SameFile(info, entry.info) || info.Mode().Type() != entry.info.Mode().Type() || !info.IsDir() && (info.Size() != entry.info.Size() || !info.ModTime().Equal(entry.info.ModTime())) {
		return indexrepair.ErrSourceChanged
	}
	return nil
}
func (m *markdownImportPreparation) openLocalSource(local string) (out *markdownImportSource, err error) {
	return m.openLocalSourceWithIO(local, archiveScratchWalkIO{})
}

func (m *markdownImportPreparation) openLocalSourceWithIO(local string, op archiveScratchWalkIO) (out *markdownImportSource, err error) {
	if err = m.guard(); err != nil {
		return nil, err
	}
	if local == "" {
		return nil, indexrepair.ErrSourcePath
	}
	local, err = filepath.Abs(local)
	if err != nil {
		return nil, err
	}
	// Resolve only the selected path's parent (e.g. macOS /tmp). The selected
	// file/directory itself and all descendants must not be symbolic links.
	parent, err := filepath.EvalSymlinks(filepath.Dir(local))
	if err != nil {
		return nil, err
	}
	local = filepath.Join(parent, filepath.Base(local))
	info, err := os.Lstat(local)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() && !info.Mode().IsRegular() {
		return nil, indexrepair.ErrSourcePath
	}
	folder := local
	if !info.IsDir() {
		folder = filepath.Dir(local)
	}
	root, err := os.OpenRoot(folder)
	if err != nil {
		return nil, err
	}
	s := &markdownImportSource{preparation: m, local: local, folder: folder, root: root, directory: info.IsDir(), closeRoot: m.archive.archiveRootCloser()}
	defer func() {
		if err != nil {
			out = nil
			err = errors.Join(err, s.close())
		}
	}()
	s.identity, err = root.Stat(".")
	if err != nil {
		return nil, err
	}
	source := indexrepair.Source{Data: root, Authorize: func(context.Context) error { return s.guard() }}
	capture := func(name string, d fs.DirEntry) error {
		if err := s.guard(); err != nil {
			return err
		}
		if name != "." {
			if err := source.Check(name, false); err != nil {
				return err
			}
		}
		stat, err := root.Lstat(name)
		if err != nil {
			return err
		}
		if stat.Mode()&fs.ModeSymlink != 0 || stat.IsDir() != d.IsDir() || stat.Mode().Type() != d.Type() {
			return indexrepair.ErrSourceChanged
		}
		if !stat.IsDir() && !stat.Mode().IsRegular() {
			return indexrepair.ErrSourcePath
		}
		s.entries = append(s.entries, markdownSourceEntry{name, stat})
		return nil
	}
	if s.directory {
		err = m.archive.walkScratchWithIO(source, ".", capture, op)
	} else {
		name := filepath.Base(local)
		var stat fs.FileInfo
		stat, err = root.Lstat(name)
		if err == nil {
			err = capture(name, fs.FileInfoToDirEntry(stat))
		}
	}
	if err != nil {
		return nil, err
	}
	if len(s.entries) == 0 || !os.SameFile(info, s.entries[0].info) {
		return nil, indexrepair.ErrSourceChanged
	}
	for _, entry := range s.entries {
		if err = s.check(entry); err != nil {
			return nil, err
		}
	}
	return s, s.guard()
}
func (s *markdownImportSource) relative(current string) (string, error) {
	name, err := filepath.Rel(s.folder, current)
	if err != nil {
		return "", err
	}
	name = filepath.ToSlash(name)
	if name != "." && (!fs.ValidPath(name) || strings.Contains(name, "\\")) {
		return "", indexrepair.ErrSourcePath
	}
	return name, nil
}
func (s *markdownImportSource) read(current string) (data []byte, err error) {
	name, err := s.relative(current)
	if err != nil {
		return nil, err
	}
	for _, entry := range s.entries {
		if entry.name == name {
			if !entry.info.Mode().IsRegular() {
				return nil, indexrepair.ErrSourcePath
			}
			if err = s.check(entry); err != nil {
				return nil, err
			}
			file, openErr := s.root.Open(name)
			if openErr != nil {
				return nil, openErr
			}
			defer func() {
				err = errors.Join(err, file.Close())
				if err != nil {
					data = nil
				}
			}()
			var buf bytes.Buffer
			if err = archiveCopy(s.preparation.ctx, &buf, file); err != nil {
				return nil, err
			}
			if err = s.check(entry); err != nil {
				return nil, err
			}
			return buf.Bytes(), nil
		}
	}
	return nil, indexrepair.ErrSourcePath
}
func (s *markdownImportSource) walk(fn fs.WalkDirFunc) error {
	skip := ""
	for _, entry := range s.entries {
		if skip != "" && strings.HasPrefix(entry.name, skip) {
			continue
		}
		skip = ""
		if err := s.check(entry); err != nil {
			return err
		}
		d := fs.FileInfoToDirEntry(entry.info)
		if entry.name == "." {
			d = markdownSourceDirEntry{DirEntry: d, name: filepath.Base(s.local)}
		}
		err := fn(filepath.Join(s.folder, filepath.FromSlash(entry.name)), d, nil)
		if err == fs.SkipDir && entry.info.IsDir() {
			if entry.name == "." {
				return nil
			}
			skip = entry.name + "/"
			continue
		}
		if err != nil {
			return err
		}
	}
	return s.guard()
}
func (s *markdownImportSource) hasMarkdown(current string) (bool, error) {
	name, err := s.relative(current)
	if err != nil {
		return false, err
	}
	prefix := ""
	if name != "." {
		prefix = name + "/"
	}
	for _, entry := range s.entries {
		if strings.HasPrefix(entry.name, prefix) && entry.info.Mode().IsRegular() && (strings.HasSuffix(entry.name, ".md") || strings.HasSuffix(entry.name, ".markdown")) {
			return true, s.check(entry)
		}
	}
	return false, s.guard()
}
func (s *markdownImportSource) siblingMarkdown(current string) (bool, error) {
	name, err := s.relative(current)
	if err != nil {
		return false, err
	}
	// A selected directory's sibling lies outside the chosen source root.
	if name == "." {
		return false, s.guard()
	}
	for _, entry := range s.entries {
		if entry.name == name+".md" || entry.name == name+".markdown" {
			return true, s.check(entry)
		}
	}
	return false, s.guard()
}

// Target paths come from the captured blocktree and are checked against the
// captured data root. A stale/missing target is a failure, never a successful no-op.
func (m *markdownImportPreparation) localTarget(box, to string) (hpath, target string, err error) {
	if err = validateArchiveDestination(box, to); err != nil {
		return
	}
	if err = m.guard(); err != nil {
		return
	}
	r := m.archive.binding.scope.commit.r
	ioctx, closeIO, err := r.openRepairIOCheckedWithCloser(m.ctx, false, m.archive.archiveRootCloser())
	if err != nil {
		return "", "", err
	}
	defer func() {
		err = errors.Join(err, closeIO())
		if err != nil {
			hpath = ""
			target = ""
		}
	}()
	if err = ioctx.Source.Check(box, false); err != nil {
		return
	}
	info, err := ioctx.Source.Data.Stat(box)
	if err != nil {
		return "", "", err
	}
	if !info.IsDir() {
		return "", "", indexrepair.ErrSourcePath
	}
	if r.owner.Box(box) == nil {
		return "", "", ErrBlockNotFound
	}
	if to == "/" {
		return "/", "/", m.guard()
	}
	rows, err := r.registry.Query(m.ctx, indexdb.BlockTreeFilter{Kind: "root-path", BoxID: box, Value: to})
	if err != nil {
		return "", "", err
	}
	if len(rows) != 1 {
		return "", "", ErrBlockNotFound
	}
	row := rows[0]
	if row.BoxID != box || row.Path != to || !util.HasDocExt(to) || row.ID != util.GetTreeID(to) || !path.IsAbs(row.HPath) {
		return "", "", indexrepair.ErrSourcePath
	}
	raw, err := ioctx.Source.Read(m.ctx, path.Join(box, strings.TrimPrefix(to, "/")))
	if err != nil {
		return "", "", err
	}
	tree, _, err := dataparser.ParseJSON(raw, m.archive.binding.scope.commit.parser().ParseOptions)
	if err != nil {
		return "", "", err
	}
	if tree == nil || tree.Root == nil || tree.Root.ID != row.ID {
		return "", "", indexrepair.ErrSourceChanged
	}
	return row.HPath, util.StripDocExt(row.Path), m.guard()
}
