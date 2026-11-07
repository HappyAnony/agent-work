// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"fmt"
	"github.com/88250/lute/ast"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"wiki-kernel/internal/archivepack"
	"wiki-kernel/internal/indexrepair"
	"wiki-kernel/util"
)

type archiveExportOutput struct {
	e                                                          *archiveExportContext
	temp, exports, work, payload                               *os.Root
	tempInfo, exportInfo, workInfo, payloadInfo, publishedInfo os.FileInfo
	workname, folder, prefix, filename                         string
	subdir                                                     string
	subdirInfo                                                 os.FileInfo
	published                                                  bool
	closed                                                     bool
}

type archiveOutputWalkIO struct {
	readDir func(context.Context, indexrepair.Source, string) ([]fs.DirEntry, error)
}
type archiveOutputEntry struct {
	name  string
	info  fs.FileInfo
	empty bool
}

func sameArchiveDirectory(info os.FileInfo, err error, want os.FileInfo) error {
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || !os.SameFile(info, want) {
		return errors.New("archive_output_owner_changed")
	}
	return nil
}
func (o *archiveExportOutput) check() error {
	if o == nil || o.closed {
		return errors.New("archive_output_closed")
	}
	info, err := os.Lstat(o.e.temp)
	if err = sameArchiveDirectory(info, err, o.tempInfo); err != nil {
		return err
	}
	info, err = o.temp.Lstat("export")
	if err = sameArchiveDirectory(info, err, o.exportInfo); err != nil {
		return err
	}
	if o.subdir != "" {
		info, err = o.exports.Lstat(o.subdir)
		if err = sameArchiveDirectory(info, err, o.subdirInfo); err != nil {
			return err
		}
	}
	info, err = o.temp.Lstat(o.workname)
	if err = sameArchiveDirectory(info, err, o.workInfo); err != nil {
		return err
	}
	info, err = o.work.Lstat("content")
	return sameArchiveDirectory(info, err, o.payloadInfo)
}
func (e *archiveExportContext) newArchiveOutput(title string) (o *archiveExportOutput, err error) {
	if err = e.guard(); err != nil {
		return nil, err
	}
	if e.output != nil {
		return nil, errors.New("archive_output_already_bound")
	}
	name := util.FilterFileName(title)
	if name == "" || name == "." || name == ".." {
		name = "Untitled"
	}
	if strings.HasSuffix(name, "..") {
		name += "_"
	}
	if !fs.ValidPath(name) || strings.ContainsAny(name, "/\\") {
		return nil, indexrepair.ErrSourcePath
	}
	if _, err = os.Lstat(e.temp); os.IsNotExist(err) {
		err = os.MkdirAll(e.temp, 0700)
	}
	if err != nil {
		return nil, err
	}
	o = &archiveExportOutput{e: e, prefix: name, workname: ".wiki-archive-" + ast.NewNodeID()}
	defer func() {
		if err != nil {
			err = errors.Join(err, o.finish(false))
			o = nil
		}
	}()
	if o.temp, err = openArchivePublicationSource(e.temp); err != nil {
		return o, err
	}
	o.tempInfo, err = o.temp.Stat(".")
	if err != nil {
		return o, err
	}
	if err = o.temp.Mkdir("export", 0755); err != nil && !os.IsExist(err) {
		return o, err
	}
	info, err := o.temp.Lstat("export")
	if err != nil {
		return o, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return o, indexrepair.ErrSourcePath
	}
	if o.exports, err = o.temp.OpenRoot("export"); err != nil {
		return o, err
	}
	o.exportInfo, err = o.exports.Stat(".")
	if err != nil {
		return o, err
	}
	if err = o.temp.Mkdir(o.workname, 0700); err != nil {
		return o, err
	}
	o.workInfo, err = o.temp.Lstat(o.workname)
	if err != nil {
		return o, err
	}
	if o.work, err = o.temp.OpenRoot(o.workname); err != nil {
		return o, err
	}
	if err = o.work.Mkdir("content", 0755); err != nil {
		return o, err
	}
	if o.payload, err = o.work.OpenRoot("content"); err != nil {
		return o, err
	}
	o.payloadInfo, err = o.payload.Stat(".")
	if err != nil {
		return o, err
	}
	o.folder = filepath.Join(e.temp, o.workname, "content")
	o.filename = name + "-" + strings.TrimPrefix(o.workname, ".wiki-archive-") + util.DocZipExt
	e.output = o
	return o, e.guard()
}
func (e *archiveExportContext) openExportFolder(folder string) (*os.Root, error) {
	if err := e.guard(); err != nil {
		return nil, err
	}
	if e.output == nil {
		return openArchivePublicationSource(folder)
	} // Non-controller/test-owned scratch.
	if folder != e.output.folder {
		return nil, errors.New("archive_output_folder_mismatch")
	}
	return e.output.payload.OpenRoot(".")
}

// Borrowed output descriptors close independently from their output owner. The
// backend is captured before opening and close remains valid after cancellation.
func (e *archiveExportContext) openExportIO(folder string) (*os.Root, func() error, error) {
	if err := e.guard(); err != nil {
		return nil, nil, err
	}
	closer := e.closeRoot
	if closer == nil {
		closer = func(root *os.Root) error { return root.Close() }
	}
	root, err := e.openExportFolder(folder)
	if err != nil {
		return nil, nil, err
	}
	return root, newRepairIOCloser([]*os.Root{root}, closer), nil
}

func checkArchiveOutputManifest(source indexrepair.Source, entries []archiveOutputEntry, guard func() error) error {
	converted := make([]archivepack.Entry, 0, len(entries))
	for _, entry := range entries {
		converted = append(converted, archivepack.Entry{Name: entry.name, Info: entry.info, Empty: entry.empty})
	}
	return archivepack.CheckManifest(source, converted, guard)
}
func archiveOutputManifest(ctx context.Context, source indexrepair.Source, guard func() error, op archiveOutputWalkIO) ([]archiveOutputEntry, error) {
	entries, e := archivepack.Manifest(ctx, source, guard, archivepack.WalkIO{ReadDir: op.readDir})
	if e != nil {
		return nil, e
	}
	out := make([]archiveOutputEntry, 0, len(entries))
	for _, entry := range entries {
		out = append(out, archiveOutputEntry{name: entry.Name, info: entry.Info, empty: entry.Empty})
	}
	return out, nil
}
func writeOwnedArchive(source indexrepair.Source, prefix string, dst io.Writer, guard func() error, copy func(io.Writer, io.Reader) error) error {
	return archivepack.Write(source, prefix, dst, guard, copy)
}
func writeOwnedArchiveWithIO(ctx context.Context, source indexrepair.Source, prefix string, dst io.Writer, guard func() error, copy func(io.Writer, io.Reader) error, op archiveOutputWalkIO) error {
	return archivepack.WriteWithIO(ctx, source, prefix, dst, guard, copy, archivepack.WalkIO{ReadDir: op.readDir})
}
func (o *archiveExportOutput) pack() error {
	return o.packFrom(indexrepair.Source{Data: o.payload}, o.prefix)
}
func (o *archiveExportOutput) packFrom(source indexrepair.Source, prefix string) error {
	return o.publishContent(func(dst io.Writer) error {
		return writeOwnedArchiveWithIO(o.e.operationContext(), source, prefix, dst, o.e.guard, func(dst io.Writer, src io.Reader) error {
			err := archiveCopy(o.e.operationContext(), dst, src)
			if err == nil {
				util.PushEndlessProgress(o.e.outputLanguage(65) + " " + fmt.Sprintf(o.e.outputLanguage(253), prefix))
			}
			return err
		}, archiveOutputWalkIO{})
	})
}
func (o *archiveExportOutput) publishContent(write func(io.Writer) error) error {
	if write == nil {
		return errors.New("export_content_writer_unbound")
	}
	if err := o.e.guard(); err != nil {
		return err
	}
	f, err := o.work.OpenFile("archive.part", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return err
	}
	err = write(f)
	if err == nil {
		err = f.Sync()
	}
	if err == nil {
		o.publishedInfo, err = f.Stat()
	}
	err = errors.Join(err, f.Close())
	if err != nil {
		return err
	}
	if err = o.e.guard(); err != nil {
		return err
	}
	// Link publishes a closed file atomically and refuses an existing filename.
	err = o.temp.Link(o.workname+"/archive.part", "export/"+o.filename)
	if err == nil {
		o.published = true
	}
	return err
}

// A checked optional publication subdirectory retains existing download routes.
func (o *archiveExportOutput) useSubdirectory(name string) error {
	if err := o.e.guard(); err != nil {
		return err
	}
	if o.published || o.subdir != "" || !fs.ValidPath(name) || name == "." || strings.ContainsAny(name, "/\\") {
		return indexrepair.ErrSourcePath
	}
	if err := o.exports.Mkdir(name, 0755); err != nil && !os.IsExist(err) {
		return err
	}
	info, err := o.exports.Lstat(name)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return indexrepair.ErrSourcePath
	}
	o.subdir = name
	o.subdirInfo = info
	o.filename = path.Join(name, o.filename)
	return o.e.guard()
}
func (o *archiveExportOutput) removePublished() error {
	if !o.published || o.exports == nil || o.publishedInfo == nil {
		return nil
	}
	if o.subdir != "" {
		info, err := o.exports.Lstat(o.subdir)
		if err = sameArchiveDirectory(info, err, o.subdirInfo); err != nil {
			return err
		}
	}
	info, err := o.exports.Lstat(o.filename)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !os.SameFile(info, o.publishedInfo) {
		return errors.New("archive_published_owner_changed")
	}
	return o.exports.Remove(o.filename)
}
func (o *archiveExportOutput) finish(keep bool) (err error) {
	if o == nil || o.closed {
		return nil
	}
	if keep {
		err = o.e.guard()
	}
	if o.payload != nil {
		err = errors.Join(err, o.payload.Close())
	}
	if o.work != nil {
		err = errors.Join(err, o.work.Close())
	}
	if o.temp != nil && o.workInfo != nil {
		info, statErr := o.temp.Lstat(o.workname)
		if !os.IsNotExist(statErr) {
			if checkErr := sameArchiveDirectory(info, statErr, o.workInfo); checkErr != nil {
				err = errors.Join(err, checkErr)
			} else {
				err = errors.Join(err, o.temp.RemoveAll(o.workname))
			}
		}
	}
	if !keep || err != nil {
		err = errors.Join(err, o.removePublished())
	}
	o.closed = true
	if o.e.output == o {
		o.e.output = nil
	}
	if o.exports != nil {
		err = errors.Join(err, o.exports.Close())
	}
	if o.temp != nil {
		err = errors.Join(err, o.temp.Close())
	}
	return err
}
