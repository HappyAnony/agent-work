// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"wiki-kernel/internal/indexrepair"
	"wiki-kernel/util"
)

type htmlDesktopDestination struct {
	externalGuard                          func() error
	m                                      *markdownExportContext
	selected, ancestor, physical, relative string
	anchorInfo                             os.FileInfo
	root                                   *os.Root
	rootInfo                               os.FileInfo
}

type htmlDesktopIO struct {
	preflightReadDir func(context.Context, indexrepair.Source, string) ([]fs.DirEntry, error)
}

// Resolve only an explicitly selected ancestor, including macOS /var and /tmp
// aliases. The resolved identity is checked again; internal output links are
// never recursively granted. Missing descendants are created under that root.
func htmlDesktopAncestor(p string) (ancestor, physical, relative string, err error) {
	ancestor = p
	for {
		_, err = os.Lstat(ancestor)
		if err == nil {
			break
		}
		if !os.IsNotExist(err) {
			return
		}
		parent := filepath.Dir(ancestor)
		if parent == ancestor {
			return
		}
		ancestor = parent
	}
	physical, err = filepath.EvalSymlinks(ancestor)
	if err != nil {
		return
	}
	relative, err = filepath.Rel(ancestor, p)
	return
}
func htmlPathContains(parent, child string) bool {
	// macOS exports must also protect case-insensitive source aliases.
	rel, err := filepath.Rel(strings.ToLower(parent), strings.ToLower(child))
	return err == nil && (rel == "." || rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)))
}
func (m *markdownExportContext) captureHTMLDesktopDestination(selected string) (*htmlDesktopDestination, error) {
	if e := m.guard(); e != nil {
		return nil, e
	}
	r := m.read.binding.scope.commit.r
	roots := []string{r.workspace, r.data, m.read.temp, util.RepoDir, util.HistoryDir, filepath.Join(r.workspace, "conf"), m.htmlResources.work.path, m.htmlResources.theme.path, m.htmlResources.icon.path, m.htmlResources.fallback.path}
	d, e := captureDesktopDestination(m.guard, selected, roots)
	if e == nil {
		d.m = m
	}
	return d, e
}
func captureDesktopDestination(guard func() error, selected string, roots []string) (*htmlDesktopDestination, error) {
	if err := guard(); err != nil {
		return nil, err
	}
	selected = strings.TrimSpace(selected)
	if !filepath.IsAbs(selected) || strings.Contains(selected, "\\") {
		return nil, indexrepair.ErrSourcePath
	}
	for _, part := range strings.Split(selected, string(os.PathSeparator)) {
		if part == ".." {
			return nil, indexrepair.ErrSourcePath
		}
	}
	selected = filepath.Clean(selected)
	if info, e := os.Lstat(selected); e == nil && (!info.IsDir() || info.Mode()&os.ModeSymlink != 0) {
		return nil, indexrepair.ErrSourcePath
	} else if e != nil && !os.IsNotExist(e) {
		return nil, e
	}
	ancestor, physical, rel, err := htmlDesktopAncestor(selected)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(physical)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, indexrepair.ErrSourcePath
	}
	target := filepath.Join(physical, rel)
	for _, p := range roots {
		if p == "" {
			continue
		}
		if !filepath.IsAbs(p) {
			return nil, errors.New("html_desktop_protected_root_invalid")
		}
		_, base, tail, e := htmlDesktopAncestor(filepath.Clean(p))
		if e != nil {
			return nil, e
		}
		protected := filepath.Join(base, tail)
		if htmlPathContains(protected, target) || htmlPathContains(target, protected) {
			return nil, errors.New("html_desktop_destination_overlaps_source")
		}
	}
	return &htmlDesktopDestination{externalGuard: guard, selected: selected, ancestor: ancestor, physical: physical, relative: rel, anchorInfo: info}, nil
}
func (d *htmlDesktopDestination) check() error {
	if d.externalGuard == nil {
		return errors.New("desktop_export_owner_unbound")
	}
	if err := d.externalGuard(); err != nil {
		return err
	}
	resolved, err := filepath.EvalSymlinks(d.ancestor)
	if err != nil {
		return err
	}
	if resolved != d.physical {
		return errors.New("html_desktop_ancestor_changed")
	}
	info, err := os.Stat(d.physical)
	if err = sameArchiveDirectory(info, err, d.anchorInfo); err != nil {
		return err
	}
	if d.relative != "." {
		anchor, e := openArchivePublicationSource(d.physical)
		if e != nil {
			return e
		}
		e = (indexrepair.Source{Data: anchor}).Check(filepath.ToSlash(d.relative), d.root == nil)
		if e = errors.Join(e, anchor.Close()); e != nil {
			return e
		}
	}
	if d.root != nil {
		info, err = os.Lstat(d.selected)
		if err = sameArchiveDirectory(info, err, d.rootInfo); err != nil {
			return err
		}
		info, err = d.root.Stat(".")
		if err = sameArchiveDirectory(info, err, d.rootInfo); err != nil {
			return err
		}
	}
	return nil
}
func (d *htmlDesktopDestination) open() (err error) {
	if err = d.check(); err != nil {
		return err
	}
	anchor, err := openArchivePublicationSource(d.physical)
	if err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, anchor.Close())
		if err != nil && d.root != nil {
			err = errors.Join(err, d.root.Close())
			d.root = nil
		}
	}()
	info, e := anchor.Stat(".")
	if err = sameArchiveDirectory(info, e, d.anchorInfo); err != nil {
		return err
	}
	if d.relative != "." {
		source := indexrepair.Source{Data: anchor}
		if err = source.Check(filepath.ToSlash(d.relative), true); err != nil {
			return err
		}
		if err = anchor.MkdirAll(d.relative, 0755); err != nil {
			return err
		}
	}
	d.root, err = anchor.OpenRoot(d.relative)
	if err != nil {
		return err
	}
	d.rootInfo, err = d.root.Stat(".")
	if err != nil {
		return err
	}
	return d.check()
}
func (d *htmlDesktopDestination) preflight(source *os.Root) error {
	return d.preflightWithIO(source, htmlDesktopIO{})
}
func (d *htmlDesktopDestination) preflightWithIO(source *os.Root, op htmlDesktopIO) error {
	if err := d.check(); err != nil {
		return err
	}
	target := indexrepair.Source{Data: d.root}
	// The original frontend writes this after the response. Never approve a
	// currently linked or non-regular index, even though resources don't write it.
	if err := target.Check("index.html", true); err != nil {
		return err
	}
	if info, err := d.root.Lstat("index.html"); err == nil && !info.Mode().IsRegular() {
		return indexrepair.ErrSourcePath
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	input := indexrepair.Source{Data: source}
	entries, err := d.m.read.resourceManifest(input, ".", op.preflightReadDir, nil, true)
	if err != nil {
		return err
	}
	for _, entry := range entries[1:] {
		if err = d.check(); err != nil {
			return err
		}
		if err = target.Check(entry.name, true); err != nil {
			return err
		}
		info, statErr := d.root.Lstat(entry.name)
		if os.IsNotExist(statErr) {
			continue
		}
		if statErr != nil {
			return statErr
		}
		if entry.info.IsDir() != info.IsDir() || !entry.info.IsDir() && !info.Mode().IsRegular() {
			return indexrepair.ErrSourcePath
		}
	}
	return d.check()
}
func ExportHTMLDesktopContext(ctx context.Context, id, savePath string, markdown, pdf, keepFold, merge bool) (HTMLExportFolderResult, error) {
	m, err := captureMarkdownExport(ctx)
	if err != nil {
		return HTMLExportFolderResult{}, err
	}
	return m.exportHTMLDesktop(id, savePath, htmlExportFileOptions{Markdown: markdown, PDF: pdf, KeepFold: keepFold, Merge: merge})
}
func (m *markdownExportContext) exportHTMLDesktop(id, savePath string, options htmlExportFileOptions) (result HTMLExportFolderResult, err error) {
	destination, err := m.captureHTMLDesktopDestination(savePath)
	if err != nil {
		return result, err
	}
	err = m.withHTMLFile(id, options, func(f htmlExportFile) (err error) {
		if err = destination.open(); err != nil {
			return err
		}
		defer func() { err = errors.Join(err, destination.root.Close()) }()
		if err = destination.preflight(f.output.payload); err != nil {
			return err
		}
		copier := markdownExportResources{m: m, source: indexrepair.Source{Data: f.output.payload}, target: indexrepair.Source{Data: destination.root, Authorize: func(ctx context.Context) error {
			if e := ctx.Err(); e != nil {
				return e
			}
			return destination.check()
		}}}
		if err = copyHTMLDirectory(&copier); err != nil {
			return err
		}
		if err = destination.check(); err != nil {
			return err
		}
		result = HTMLExportFolderResult{Name: f.name, Content: f.content}
		return nil
	})
	if err != nil {
		result = HTMLExportFolderResult{}
	}
	return result, err
}
