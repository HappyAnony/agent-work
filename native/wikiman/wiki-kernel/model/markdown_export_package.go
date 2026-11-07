// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/88250/gulu"

	"wiki-kernel/internal/indexrepair"
	"wiki-kernel/util"
)

// Reuse archive publication ownership, while preserving Markdown's flat ZIP.
func (m *markdownExportContext) newOutput(title, ext string) (*archiveExportOutput, error) {
	if len(ext) < 2 || ext[0] != '.' || strings.ContainsAny(ext, "/\\") || !fs.ValidPath(ext) {
		return nil, indexrepair.ErrSourcePath
	}
	o, err := m.read.newArchiveOutput(path.Base(title))
	if err != nil {
		return nil, err
	}
	o.prefix = ""
	o.filename = strings.TrimSuffix(o.filename, util.DocZipExt) + ext + ".zip"
	return o, nil
}
func markdownExportPath(hpath, ext string) (string, error) {
	dir, name := path.Split(hpath)
	for _, part := range strings.Split(dir, "/") {
		if part == ".." {
			return "", indexrepair.ErrSourcePath
		}
	}
	dir = util.FilterFilePath(dir)
	name = util.FilterFileName(name)
	// HPath is notebook-absolute; output members are payload-relative.
	relative := strings.TrimPrefix(path.Join(dir, name)+ext, "/")
	if !fs.ValidPath(relative) || strings.Contains(relative, "\\") {
		return "", indexrepair.ErrSourcePath
	}

	return relative, nil
}

type markdownExportResources struct {
	m              *markdownExportContext
	source, target indexrepair.Source
	index          map[string]string
	io             markdownExportResourceIO
}
type markdownExportResourceIO struct {
	childrenReadDir func(context.Context, indexrepair.Source, string) ([]fs.DirEntry, error)
	copyReadDir     func(context.Context, indexrepair.Source, string) ([]fs.DirEntry, error)
}

func (m *markdownExportContext) openResources(o *archiveExportOutput) (ret *markdownExportResources, err error) {
	return m.openResourcesWithIO(o, markdownExportResourceIO{})
}
func (m *markdownExportContext) openResourcesWithIO(o *archiveExportOutput, op markdownExportResourceIO) (ret *markdownExportResources, err error) {
	if err = m.guard(); err != nil {
		return nil, err
	}
	root, err := openArchivePublicationSource(m.read.binding.scope.commit.r.data)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, root.Close())
		}
	}()
	index, err := m.read.assetIndex(indexrepair.Source{Data: root})
	if err != nil {
		return nil, err
	}
	ret = &markdownExportResources{m: m, source: indexrepair.Source{Data: root}, index: index, io: op, target: indexrepair.Source{Data: o.payload, Authorize: func(ctx context.Context) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		return m.guard()
	}}}
	return ret, m.guard()
}
func (r *markdownExportResources) close() error { return r.source.Data.Close() }
func (r *markdownExportResources) directoryEntries(name string, readDir func(context.Context, indexrepair.Source, string) ([]fs.DirEntry, error)) ([]fs.DirEntry, error) {
	if err := r.m.guard(); err != nil {
		return nil, err
	}
	if err := r.source.Check(name, false); err != nil {
		return nil, err
	}
	info, err := r.source.Data.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&fs.ModeSymlink != 0 {
		return nil, indexrepair.ErrSourcePath
	}
	if readDir == nil {
		readDir = func(ctx context.Context, source indexrepair.Source, name string) ([]fs.DirEntry, error) {
			return source.ReadDir(ctx, name)
		}
	}
	entries, err := readDir(r.m.read.binding.scope.ctx, r.source, name)
	if err != nil {
		return nil, err
	}
	if err = r.m.guard(); err != nil {
		return nil, err
	}
	now, err := r.source.Data.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !now.IsDir() || now.Mode()&fs.ModeSymlink != 0 || !os.SameFile(now, info) || now.Size() != info.Size() || !now.ModTime().Equal(info.ModTime()) {
		return nil, indexrepair.ErrSourceChanged
	}
	return entries, r.m.guard()
}
func (r *markdownExportResources) hasChildren(box, docPath string) (bool, error) {
	if err := r.m.guard(); err != nil {
		return false, err
	}
	if err := validateArchiveDestination(box, docPath); err != nil {
		return false, err
	}
	name := path.Join(box, strings.TrimPrefix(util.StripDocExt(docPath), "/"))
	if err := r.source.Check(name, false); os.IsNotExist(err) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	entries, err := r.directoryEntries(name, r.io.childrenReadDir)
	if err != nil {
		return false, err
	}
	return len(entries) > 0, r.m.guard()
}
func (r *markdownExportResources) copyAsset(asset, folder string) error {
	// Fragment is navigation metadata, not part of the file path. Strip it
	// before destination decoding so an encoded literal hash remains a filename.
	if i := strings.IndexByte(asset, '#'); i >= 0 {
		asset = asset[:i]
	}
	normalized, err := normalizeExportAsset(asset)
	if err != nil {
		return err
	}
	from, ok := r.index[normalized]
	if selected, exists := r.m.annotationSources[normalized]; exists {
		from, ok = selected, true
	}
	if !ok {
		return r.m.guard()
	} // Original missing-resource omission.
	return r.copyPath(from, path.Join(folder, strings.TrimSuffix(normalized, "/")))
}
func (r *markdownExportResources) copyPath(from, to string) error {
	if err := r.m.guard(); err != nil {
		return err
	}
	if err := r.source.Check(from, false); err != nil {
		return err
	}
	info, err := r.source.Data.Lstat(from)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return r.m.read.copyExportFile(r.source, r.target, from, to)
	}
	entries, err := r.m.read.resourceManifest(r.source, from, r.io.copyReadDir, nil, true)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err = r.m.read.checkResourceManifest(r.source, []archiveExportResourceEntry{entry}); err != nil {
			return err
		}
		dest := to
		if entry.name != from {
			dest = path.Join(to, strings.TrimPrefix(entry.name, from+"/"))
		}
		if entry.info.IsDir() {
			if err = r.target.Check(dest, true); err != nil {
				return err
			}
			if err = r.target.Data.MkdirAll(dest, 0755); err != nil {
				return err
			}
			continue
		}
		if err = r.m.read.copyExportFile(r.source, r.target, entry.name, dest); err != nil {
			return err
		}
	}
	for i := len(entries) - 1; i >= 0; i-- {
		entry := entries[i]
		if !entry.info.IsDir() {
			continue
		}
		if err = r.m.read.checkResourceManifest(r.source, []archiveExportResourceEntry{entry}); err != nil {
			return err
		}
		dest := to
		if entry.name != from {
			dest = path.Join(to, strings.TrimPrefix(entry.name, from+"/"))
		}
		if err = r.target.Data.Chtimes(dest, entry.info.ModTime(), entry.info.ModTime()); err != nil {
			return err
		}
	}
	return r.m.read.checkResourceManifest(r.source, entries)
}

// The selected legacy device capability is captured once at entry. No process-
// global Pandoc path, workspace or scratch directory is read during conversion.
// Initializing/installing that device capability remains a separate legacy step.
func (m *markdownExportContext) convert(o *archiveExportOutput, member, from, to, content string) (err error) {
	return m.convertWithReference(o, member, from, to, content, "")
}
func (m *markdownExportContext) convertWithReference(o *archiveExportOutput, member, from, to, content, reference string) (err error) {
	if err = m.guard(); err != nil {
		return err
	}
	target := indexrepair.Source{Data: o.payload}
	if err = target.Check(member, true); err != nil {
		return err
	}
	if from == "" || to == "" || to == "md" {
		file, e := o.payload.OpenFile(member, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
		if e != nil {
			return e
		}
		_, err = io.WriteString(file, content)
		if err == nil {
			err = file.Sync()
		}
		return errors.Join(err, file.Close(), m.guard())
	}
	if !m.pandocEnabled || m.pandocBin == "" {
		return util.ErrPandocNotFound
	}
	const input = "pandoc-input"
	file, err := o.work.OpenFile(input, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	_, err = io.WriteString(file, content)
	err = errors.Join(err, file.Close())
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, o.work.Remove(input)) }()
	if err = m.guard(); err != nil {
		return err
	}
	// A successful tool exit must produce this conversion's file, rather than
	// accidentally accepting a prior equal-title result already in our payload.
	if info, statErr := o.payload.Lstat(member); statErr == nil {
		if !info.Mode().IsRegular() {
			return indexrepair.ErrSourcePath
		}
		if err = o.payload.Remove(member); err != nil {
			return err
		}
	} else if !os.IsNotExist(statErr) {
		return statErr
	}
	out := filepath.Join(o.folder, filepath.FromSlash(member))
	args := []string{filepath.Join(m.read.temp, o.workname, input), "--from", from, "--to", to, "--resource-path", filepath.Dir(out), "-s", "-o", out}
	if m.pandocGuard != nil {
		if err = m.pandocGuard(); err != nil {
			return err
		}
	}
	if reference != "" {
		args = append(args, "--reference-doc", reference)
	}
	cmd := exec.CommandContext(m.read.binding.scope.ctx, m.pandocBin, args...)
	// Do not retain/export potentially private document contents printed by tools.
	gulu.CmdAttr(cmd)
	cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
	cmd.WaitDelay = time.Second
	if err = cmd.Run(); err != nil {
		return errors.Join(err, m.read.binding.scope.ctx.Err())
	}
	if m.pandocGuard != nil {
		if err = m.pandocGuard(); err != nil {
			return err
		}
	}
	if err = m.guard(); err != nil {
		return err
	}
	if err = target.Check(member, false); err != nil {
		return err
	}
	info, err := o.payload.Stat(member)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return indexrepair.ErrSourcePath
	}
	file, err = o.payload.OpenFile(member, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	return errors.Join(file.Sync(), file.Close(), m.guard())
}
