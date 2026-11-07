// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"github.com/siyuan-note/dataparser"
	"github.com/siyuan-note/logging"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"wiki-kernel/internal/archivepack"
	"wiki-kernel/internal/indexrepair"
	"wiki-kernel/util"
)

// Captured before reading multipart input. This is a compatibility entry owner,
// not a Native source-write capability or an R04 multi-file transaction.
type ArchiveImportContext struct {
	ctx       context.Context
	binding   *legacyAVBinding
	temp      string
	language  func(int) string
	closeRoot func(*os.Root) error // Optional trusted backend; captured at borrow time.
	scratchMu sync.Mutex
	scratch   map[string]*archiveImportScratch
}

type archiveExtractIO struct {
	readDir      func(context.Context, indexrepair.Source, string) ([]fs.DirEntry, error)
	afterCapture func(indexrepair.Source, string) error
}

func CaptureArchiveImport(ctx context.Context) (*ArchiveImportContext, error) {
	if ctx == nil {
		return nil, errors.New("archive_import_context_unbound")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	b := newLegacyAVBinding(ctx)
	if err := b.liveSourceGuard(); err != nil {
		return nil, err
	}
	e := &ArchiveImportContext{ctx: ctx, binding: b, temp: util.TempDir, language: b.scope.commit.r.language}
	return e, e.guard()
}
func (e *ArchiveImportContext) guard() error {
	if e == nil || e.ctx == nil || e.binding == nil {
		return errors.New("archive_import_context_unbound")
	}
	if err := e.binding.liveSourceGuard(); err != nil {
		return err
	}
	if e.temp != util.TempDir {
		return errors.New("archive_import_temp_owner_changed")
	}
	if err := e.ctx.Err(); err != nil {
		return err
	}
	e.scratchMu.Lock()
	defer e.scratchMu.Unlock()
	for _, d := range e.scratch {
		if err := d.check(); err != nil {
			return err
		}
	}
	return nil
}
func archiveCopy(ctx context.Context, dst io.Writer, src io.Reader) error {
	buf := make([]byte, 64*1024)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, err := src.Read(buf)
		if err2 := ctx.Err(); err2 != nil {
			return err2
		}
		if n > 0 {
			written, e := dst.Write(buf[:n])
			if e != nil {
				return e
			}
			if written != n {
				return io.ErrShortWrite
			}
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func validateArchiveDestination(boxID, toPath string) error {
	if !ast.IsNodeIDPattern(boxID) || !path.IsAbs(toPath) || path.Clean(toPath) != toPath || strings.ContainsAny(toPath, "\\\x00") {
		return errors.New("invalid_archive_destination")
	}
	return nil
}

// A unique request directory avoids shared filenames and never uses the upload's
// name as a destination. Removal is limited to this request's owned scratch.
func (e *ArchiveImportContext) ImportReader(reader io.Reader, boxID, toPath string) (err error) {
	if err := e.guard(); err != nil {
		return err
	}
	if err := validateArchiveDestination(boxID, toPath); err != nil {
		return err
	}
	if reader == nil {
		return errors.New("archive_import_reader_unbound")
	}
	if e.temp == "" {
		return errors.New("archive_import_temp_unbound")
	}
	owned, err := e.newImportScratch("wiki-import-")
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, owned.close()) }()
	upload, err := owned.root.OpenFile("archive.zip", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}

	err = archiveCopy(e.ctx, upload, reader)
	err = errors.Join(err, upload.Close())
	if err != nil {
		return err
	}
	if err = e.guard(); err != nil {
		return err
	}
	return e.importFile(filepath.Join(owned.folder, "archive.zip"), boxID, toPath)
}

type preparedArchiveImport struct {
	scratch, root string
	engine        *lute.Lute
	ids           map[string]string
	trees         map[string]*parse.Tree
	owned         *archiveImportScratch
}

func (p *preparedArchiveImport) close() error { return p.owned.close() }

type archivePrepareIO struct {
	walk archiveScratchWalkIO
	read func(context.Context, indexrepair.Source, string) ([]byte, error)
}
type archivePrepareEntry struct {
	name string
	info fs.FileInfo
}
type archivePrepareManifest struct {
	directories []archivePrepareEntry
	documents   []archivePrepareEntry
}

func captureArchivePrepareEntry(source indexrepair.Source, name string, entry fs.DirEntry) (archivePrepareEntry, error) {
	info, err := source.Data.Lstat(name)
	if err != nil {
		return archivePrepareEntry{}, err
	}
	if info.Mode()&fs.ModeSymlink != 0 || info.IsDir() != entry.IsDir() || info.Mode().Type() != entry.Type() {
		return archivePrepareEntry{}, indexrepair.ErrSourceChanged
	}
	return archivePrepareEntry{name: name, info: info}, nil
}
func (e *ArchiveImportContext) checkArchivePrepareEntry(source indexrepair.Source, candidate archivePrepareEntry) error {
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
func (e *ArchiveImportContext) checkArchivePrepareManifest(source indexrepair.Source, manifest archivePrepareManifest) error {
	for _, candidate := range manifest.directories {
		if err := e.checkArchivePrepareEntry(source, candidate); err != nil {
			return err
		}
	}
	for _, candidate := range manifest.documents {
		if err := e.checkArchivePrepareEntry(source, candidate); err != nil {
			return err
		}
	}
	return e.guard()
}

// Validate every member before extracting any member. Extraction is confined to
// an exclusive scratch root, rejects aliases/links and surfaces CRC/IO failures.
func (e *ArchiveImportContext) extract(zipPath string) (scratch, root string, err error) {
	return e.extractArchive(zipPath, false)
}

// Data ZIPs keep their original IDs; document archives still reject data exports.
func (e *ArchiveImportContext) extractArchive(zipPath string, dataArchive bool) (scratch, root string, err error) {
	return e.extractArchiveWithIO(zipPath, dataArchive, archiveExtractIO{})
}

func (e *ArchiveImportContext) extractArchiveWithIO(zipPath string, dataArchive bool, op archiveExtractIO) (scratch, root string, err error) {
	scratch, err = e.extractArchiveContents(zipPath)
	if err != nil {
		return scratch, "", err
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, e.releaseScratch(scratch))
		}
	}()
	owned := e.scratchAt(scratch)
	if owned == nil {
		return scratch, "", errors.New("archive_extract_owner_unbound")
	}
	target := owned.root
	source := indexrepair.Source{Data: target}
	if op.readDir == nil {
		op.readDir = func(ctx context.Context, source indexrepair.Source, name string) ([]fs.DirEntry, error) {
			return source.ReadDir(ctx, name)
		}
	}
	entries, err := op.readDir(e.ctx, source, ".")
	if err != nil {
		return
	}
	invalidFormat := 199
	if dataArchive {
		invalidFormat = 198
	}
	if len(entries) != 1 || !entries[0].IsDir() {
		return scratch, "", errors.New(e.language(invalidFormat))
	}
	name := entries[0].Name()
	if err = source.Check(name, false); err != nil {
		return
	}
	selected, captureErr := target.Lstat(name)
	if captureErr != nil {
		err = captureErr
		return
	}
	if selected.Mode()&fs.ModeSymlink != 0 || !selected.IsDir() || selected.Mode().Type() != entries[0].Type() {
		err = indexrepair.ErrSourceChanged
		return
	}
	if op.afterCapture != nil {
		if err = op.afterCapture(source, name); err != nil {
			return
		}
	}
	if err = e.guard(); err != nil {
		return
	}
	now, checkErr := target.Lstat(name)
	if checkErr != nil {
		err = checkErr
		return
	}
	if now.Mode()&fs.ModeSymlink != 0 || !now.IsDir() || !os.SameFile(now, selected) || now.Size() != selected.Size() || !now.ModTime().Equal(selected.ModTime()) {
		err = indexrepair.ErrSourceChanged
		return
	}
	root = filepath.Join(scratch, name)
	if !dataArchive && strings.HasPrefix(name, "data-20") && len(name) == len("data-20230321175442") {
		return scratch, "", errors.New(e.language(199))
	}
	if err = e.guard(); err != nil {
		return
	}
	return
}

// Shared checked extraction has no document/data layout policy. All members
// are validated before any output is created; owned scratch cleans up on failure.
func (e *ArchiveImportContext) extractArchiveContents(zipPath string) (scratch string, err error) {
	if err = e.guard(); err != nil {
		return
	}
	inputFile, err := e.openImportArchive(zipPath)
	if err != nil {
		return "", err
	}
	defer func() {
		err = errors.Join(err, inputFile.Close())
		if err != nil && scratch != "" {
			err = errors.Join(err, e.releaseScratch(scratch))
		}
	}()
	info, err := inputFile.Stat()
	if err != nil {
		return "", err
	}
	z, err := zip.NewReader(inputFile, info.Size())
	if err != nil {
		return "", err
	}
	if err = archivepack.ValidateMembers(z.File); err != nil {
		return "", err
	}
	owned, err := e.newImportScratch("wiki-extract-")
	if err != nil {
		return "", err
	}
	scratch = owned.folder
	target := owned.root
	for _, f := range z.File {
		if err = e.guard(); err != nil {
			return
		}
		name := strings.TrimSuffix(f.Name, "/")
		if f.FileInfo().IsDir() {
			err = target.MkdirAll(name, 0755)
			if err != nil {
				return
			}
			continue
		}
		if err = target.MkdirAll(path.Dir(name), 0755); err != nil {
			return
		}
		var input io.ReadCloser
		input, err = f.Open()
		if err != nil {
			return
		}
		var output *os.File
		output, err = target.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
		if err != nil {
			err = errors.Join(err, input.Close())
			return
		}
		err = archiveCopy(e.ctx, output, input)
		err = errors.Join(err, output.Close(), input.Close())
		if err != nil {
			return
		}
	}
	return scratch, e.guard()
}

func (e *ArchiveImportContext) prepare(zipPath string) (out *preparedArchiveImport, err error) {
	return e.prepareWithIO(zipPath, archivePrepareIO{})
}
func (e *ArchiveImportContext) prepareWithIO(zipPath string, op archivePrepareIO) (out *preparedArchiveImport, err error) {
	scratch, unzipRootPath, err := e.extract(zipPath)
	if err != nil {
		return nil, err
	}
	ok := false
	defer func() {
		if !ok || err != nil {
			out = nil
			err = errors.Join(err, e.releaseScratch(scratch))
		}
	}()
	source, closeSource, err := e.openScratch(unzipRootPath)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, closeSource()) }()
	if op.read == nil {
		op.read = func(ctx context.Context, source indexrepair.Source, name string) ([]byte, error) {
			return source.Read(ctx, name)
		}
	}
	manifest := archivePrepareManifest{}
	err = e.walkScratchWithIO(source, ".", func(name string, entry fs.DirEntry) error {
		if !entry.IsDir() && !util.HasDocExt(entry.Name()) {
			return nil
		}
		candidate, captureErr := captureArchivePrepareEntry(source, name, entry)
		if captureErr != nil {
			return captureErr
		}
		if entry.IsDir() {
			manifest.directories = append(manifest.directories, candidate)
			return nil
		}
		if !candidate.info.Mode().IsRegular() {
			return indexrepair.ErrSourcePath
		}
		manifest.documents = append(manifest.documents, candidate)
		return nil
	}, op.walk)
	if err != nil {
		return nil, err
	}
	if err = e.checkArchivePrepareManifest(source, manifest); err != nil {
		return nil, err
	}
	luteEngine := e.binding.scope.commit.parser()
	blockIDs := map[string]string{}
	trees := map[string]*parse.Tree{}

	// 重新生成块 ID
	for i, candidate := range manifest.documents {
		if err = e.checkArchivePrepareEntry(source, candidate); err != nil {
			return nil, err
		}
		data, readErr := op.read(e.ctx, source, candidate.name)
		if nil != readErr {
			logging.LogErrorf("read .docj [%s] failed: %s", candidate.name, readErr)
			err = readErr
			return nil, err
		}
		if err = e.checkArchivePrepareEntry(source, candidate); err != nil {
			return nil, err
		}
		tree, _, parseErr := dataparser.ParseJSON(data, luteEngine.ParseOptions)
		if nil != parseErr {
			logging.LogErrorf("parse .docj [%s] failed: %s", candidate.name, parseErr)
			err = parseErr
			return nil, err
		}
		if err = archivepack.RemapDocumentIDs(tree, blockIDs, func(id string) string {
			return util.TimeFromID(id) + "-" + util.RandString(7)
		}); err != nil {
			return nil, err
		}
		tree.Path = "/" + candidate.name
		trees[tree.ID] = tree
		util.PushEndlessProgress(e.language(73) + " " + fmt.Sprintf(e.language(70), fmt.Sprintf("%d/%d", i+1, len(manifest.documents))))
	}

	replace := archivepack.IDReplacer(blockIDs)
	for _, tree := range trees {
		if err = e.guard(); err != nil {
			return nil, err
		}
		util.PushEndlessProgress(e.language(73) + " " + fmt.Sprintf(e.language(70), tree.Root.IALAttr("title")))
		archivepack.RemapDocumentReferences(tree, blockIDs, replace)
	}
	if err = e.checkArchivePrepareManifest(source, manifest); err != nil {
		return nil, err
	}
	ok = true
	return &preparedArchiveImport{scratch: scratch, root: unzipRootPath, engine: luteEngine, ids: blockIDs, trees: trees, owned: e.scratchAt(scratch)}, nil
}
