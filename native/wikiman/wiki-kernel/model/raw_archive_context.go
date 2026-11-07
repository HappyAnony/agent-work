// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"fmt"
	"github.com/88250/go-humanize"
	"io/fs"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/internal/archivepack"
	"wiki-kernel/internal/indexrepair"
	"wiki-kernel/util"
)

type rawArchiveContext struct {
	base                                          *legacyReindexOwner
	ctx                                           context.Context
	read                                          *archiveExportContext
	output                                        *archiveExportOutput
	keep                                          bool
	home, working, repo, workspaceName, container string
	language                                      func(int) string
}

func runRawArchive[T any](ctx context.Context, fn func(*rawArchiveContext) (T, error)) (out T, err error) {
	base, e := captureLegacyReindexOwner(ctx)
	if e != nil {
		return out, e
	}
	a := &rawArchiveContext{base: base, home: util.HomeDir, working: util.WorkingDir, repo: util.RepoDir, workspaceName: util.WorkspaceName, container: util.Container}
	if e = base.owner.WithSettingSnapshot(ctx, func(v conf.AppData) error {
		a.language = func(code int) string { return base.owner.business.Language(v.Lang, code, true, v.CloudRegion) }
		return base.guard(ctx)
	}); e != nil {
		return out, e
	}
	err = base.runFinalized(ctx, func(c, life context.Context) error {
		a.ctx = c
		a.read = &archiveExportContext{temp: base.tempDir, publication: &archivePublication{ctx: c, guard: a.guard, language: a.language}}
		var e error
		out, e = fn(a)
		return e
	}, func(primary error) error {
		if a.output != nil {
			return a.output.finish(a.keep && primary == nil)
		}
		return nil
	})
	if err != nil {
		var zero T
		return zero, err
	}
	return
}
func (a *rawArchiveContext) guard() error { return a.base.guard(a.ctx) }
func (a *rawArchiveContext) newOutput(name string) (*archiveExportOutput, error) {
	if a.output != nil {
		return nil, errors.New("raw_archive_output_already_bound")
	}
	o, e := a.read.newArchiveOutput(name)
	if e != nil {
		return nil, e
	}
	o.filename = strings.TrimSuffix(o.filename, util.DocZipExt) + ".zip"
	a.output = o
	return o, nil
}
func (a *rawArchiveContext) openRoot(name string) (*os.Root, func() error, error) {
	if e := a.guard(); e != nil {
		return nil, nil, e
	}
	info, e := os.Lstat(name)
	if e != nil {
		return nil, nil, e
	}
	if !info.IsDir() || info.Mode()&fs.ModeSymlink != 0 {
		return nil, nil, indexrepair.ErrSourcePath
	}
	root, e := os.OpenRoot(name)
	if e != nil {
		return nil, nil, e
	}
	check := func() error {
		if e := a.guard(); e != nil {
			return e
		}
		now, e := os.Lstat(name)
		if e != nil {
			return e
		}
		if now.Mode()&fs.ModeSymlink != 0 || !os.SameFile(info, now) {
			return indexrepair.ErrSourceChanged
		}
		return nil
	}
	actual, e := root.Stat(".")
	if e == nil && !os.SameFile(info, actual) {
		e = indexrepair.ErrSourceChanged
	}
	if e == nil {
		e = check()
	}
	if e != nil {
		return nil, nil, errors.Join(e, root.Close())
	}
	return root, check, nil
}
func (a *rawArchiveContext) progress(code int) func() {
	a.base.owner.publishSyncUI(a.ctx, func() { util.PushEndlessProgress(a.language(code)) })
	return func() {
		a.base.owner.publishSyncUI(context.WithoutCancel(a.ctx), func() { util.ClearPushProgress(100) })
	}
}
func (a *rawArchiveContext) dataArchive() (out *archiveExportOutput, err error) {
	if e := FlushTxQueueContext(a.ctx); e != nil {
		return nil, e
	}
	root, check, e := a.openRoot(a.base.data)
	if e != nil {
		return nil, e
	}
	defer func() {
		err = errors.Join(err, check(), root.Close())
		if err != nil {
			out = nil
		}
	}()
	stamp := time.Now().Format("20060102150405")
	o, e := a.newOutput(util.FilterFileName(a.workspaceName) + "-" + stamp)
	if e != nil {
		return nil, e
	}
	previous := a.read.publication.guard
	a.read.publication.guard = func() error {
		if e := previous(); e != nil {
			return e
		}
		return check()
	}
	if e = o.packFrom(indexrepair.Source{Data: root}, "data-"+stamp); e != nil {
		return nil, e
	}
	return o, a.guard()
}
func ExportDataContext(ctx context.Context) (string, error) {
	return runRawArchive(ctx, func(a *rawArchiveContext) (string, error) {
		defer a.progress(65)()
		o, e := a.dataArchive()
		if e != nil {
			return "", e
		}
		a.keep = true
		return "/export/" + url.PathEscape(o.filename), a.guard()
	})
}
func ExportResourcesContext(ctx context.Context, paths []string, name string) (string, error) {
	paths = append([]string(nil), paths...)
	return runRawArchive(ctx, func(a *rawArchiveContext) (out string, err error) {
		if !fs.ValidPath(name) || name == "." || strings.ContainsAny(name, "/\\") {
			return "", indexrepair.ErrSourcePath
		}
		if e := FlushTxQueueContext(a.ctx); e != nil {
			return "", e
		}
		root, check, e := a.openRoot(a.base.workspace)
		if e != nil {
			return "", e
		}
		defer func() {
			err = errors.Join(err, check(), root.Close())
			if err != nil {
				out = ""
			}
		}()
		source := indexrepair.Source{Data: root}
		for _, p := range paths {
			if !fs.ValidPath(p) || p == "." || strings.Contains(p, "\\") {
				return "", indexrepair.ErrSourcePath
			}
			if htmlPathContains(filepath.Join(a.base.workspace, filepath.FromSlash(p)), a.base.tempDir) {
				return "", errors.New("export_resource_contains_output")
			}
			if e = source.Check(p, false); e != nil {
				return "", e
			}
		}
		o, e := a.newOutput(name)
		if e != nil {
			return "", e
		}
		target := indexrepair.Source{Data: o.payload, Authorize: func(context.Context) error { return a.read.guard() }}
		for _, p := range paths {
			info, e := root.Lstat(p)
			if e != nil {
				return "", e
			}
			dest := path.Base(p)
			if info.IsDir() {
				if e = target.CopyDirectoryFrom(a.ctx, source, p, dest); e != nil {
					return "", e
				}
			} else {
				raw, e := source.Read(a.ctx, p)
				if e != nil {
					return "", e
				}
				before, e := target.Read(a.ctx, dest)
				if e != nil && !indexrepair.IsSourceNotExist(e) {
					return "", e
				}
				if indexrepair.IsSourceNotExist(e) {
					before = nil
				}
				if e = target.Rewrite(a.ctx, dest, before, raw); e != nil {
					return "", e
				}
			}
		}
		o.prefix = name
		if e = o.pack(); e != nil {
			return "", e
		}
		a.keep = true
		return path.Join("temp", "export", o.filename), a.guard()
	})
}
func ExportSystemLogContext(ctx context.Context) (string, error) {
	return runRawArchive(ctx, func(a *rawArchiveContext) (out string, err error) {
		o, e := a.newOutput("system-log")
		if e != nil {
			return "", e
		}
		target := indexrepair.Source{Data: o.payload, Authorize: func(context.Context) error { return a.read.guard() }}
		selected := []string{filepath.Join(a.home, ".config", "siyuan", "app.log"), filepath.Join(a.home, ".config", "siyuan", "kernel.log"), filepath.Join(a.base.tempDir, "siyuan.log"), filepath.Join(a.base.tempDir, "mobile.log")}
		for _, p := range selected {
			file := capturePDFWatermarkFile(p)
			if indexrepair.IsSourceNotExist(file.err) {
				continue
			}
			raw, e := file.read(512<<20, a.guard)
			if e != nil {
				return "", e
			}
			if e = target.Rewrite(a.ctx, filepath.Base(p), nil, raw); e != nil {
				return "", e
			}
		}
		o.prefix = "log"
		if e = o.pack(); e != nil {
			return "", e
		}
		a.keep = true
		return "/export/" + url.PathEscape(o.filename), a.guard()
	})
}

func (a *rawArchiveContext) desktopDestination(folder string) (*htmlDesktopDestination, error) {
	return captureDesktopDestination(a.guard, folder, []string{a.base.workspace, a.base.data, a.base.tempDir, a.base.history, a.repo, a.working})
}
func (d *htmlDesktopDestination) copyUniqueFile(ctx context.Context, source indexrepair.Source, member, basename string) (out string, err error) {
	if e := d.check(); e != nil {
		return "", e
	}
	if !fs.ValidPath(basename) || strings.ContainsAny(basename, "/\\") {
		return "", indexrepair.ErrSourcePath
	}
	if e := source.Check(member, false); e != nil {
		return "", e
	}
	before, e := source.Data.Lstat(member)
	if e != nil {
		return "", e
	}
	if !before.Mode().IsRegular() {
		return "", indexrepair.ErrSourcePath
	}
	var file *os.File
	name := basename
	for i := 0; i < 10000; i++ {
		if e = d.check(); e != nil {
			return "", e
		}
		if i > 0 {
			name = fmt.Sprintf("%s (%d)%s", strings.TrimSuffix(basename, path.Ext(basename)), i, path.Ext(basename))
		}
		file, e = d.root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
		if e == nil {
			break
		}
		if !os.IsExist(e) {
			return "", e
		}
	}
	if e != nil {
		return "", e
	}
	identity, e := file.Stat()
	if e != nil {
		return "", errors.Join(e, file.Close())
	}
	defer func() {
		if err != nil {
			current, e := d.root.Lstat(name)
			if e == nil && os.SameFile(identity, current) {
				err = errors.Join(err, d.root.Remove(name))
			}
		}
	}()
	input, e := source.Data.Open(member)
	if e != nil {
		return "", errors.Join(e, file.Close())
	}
	opened, e := input.Stat()
	if e == nil && (!os.SameFile(before, opened) || opened.Size() != before.Size() || !opened.ModTime().Equal(before.ModTime())) {
		e = indexrepair.ErrSourceChanged
	}
	if e == nil {
		e = archivepack.CopyContext(ctx, file, input)
	}
	after, statErr := input.Stat()
	e = errors.Join(e, statErr, input.Close())
	if e == nil && (!os.SameFile(before, after) || after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime())) {
		e = indexrepair.ErrSourceChanged
	}
	if e == nil {
		e = file.Sync()
	}
	if e == nil {
		e = source.Check(member, false)
		if e == nil {
			current, statErr := source.Data.Lstat(member)
			e = statErr
			if e == nil && (!os.SameFile(before, current) || before.Size() != current.Size() || !before.ModTime().Equal(current.ModTime())) {
				e = indexrepair.ErrSourceChanged
			}
		}
	}
	e = errors.Join(e, file.Close(), d.check())
	if e != nil {
		return "", e
	}
	current, e := d.root.Lstat(name)
	if e != nil {
		return "", e
	}
	if !os.SameFile(identity, current) || !current.Mode().IsRegular() {
		return "", indexrepair.ErrSourceChanged
	}
	return filepath.Join(d.selected, name), ctx.Err()
}

func ExportDataInFolderContext(ctx context.Context, folder string) (string, error) {
	return runRawArchive(ctx, func(a *rawArchiveContext) (out string, err error) {
		defer a.progress(65)()
		destination, e := a.desktopDestination(folder)
		if e != nil {
			return "", e
		}
		if a.container == util.ContainerStd {
			root, check, e := a.openRoot(a.base.data)
			if e != nil {
				return "", e
			}
			entries, e := archivepack.Manifest(a.ctx, indexrepair.Source{Data: root}, check, archivepack.WalkIO{})
			e = errors.Join(e, root.Close())
			if e != nil {
				return "", e
			}
			size := uint64(0)
			for _, entry := range entries {
				if entry.Info.IsDir() {
					size += 4096
				} else {
					if entry.Info.Size() < 0 {
						return "", indexrepair.ErrSourcePath
					}
					size += uint64(entry.Info.Size())
				}
			}
			tempAncestor, _, _, e := htmlDesktopAncestor(a.base.tempDir)
			if e != nil {
				return "", e
			}
			_, _, tempFree := util.GetDiskUsage(tempAncestor)
			_, _, targetFree := util.GetDiskUsage(destination.physical)
			if tempFree < size*2 {
				return "", errors.New(fmt.Sprintf(a.language(242), humanize.BytesCustomCeil(tempFree, 2), humanize.BytesCustomCeil(size*2, 2)))
			}
			if targetFree < size {
				return "", errors.New(fmt.Sprintf(a.language(242), humanize.BytesCustomCeil(targetFree, 2), humanize.BytesCustomCeil(size, 2)))
			}
		}
		o, e := a.dataArchive()
		if e != nil {
			return "", e
		}
		if e = destination.open(); e != nil {
			return "", e
		}
		defer func() {
			err = errors.Join(err, destination.root.Close())
			if err != nil {
				out = ""
			}
		}()
		written, e := destination.copyUniqueFile(a.ctx, indexrepair.Source{Data: o.exports}, o.filename, o.filename)
		if e != nil {
			return "", e
		}
		return filepath.Base(written), a.guard()
	})
}
