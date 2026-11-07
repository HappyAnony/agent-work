// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"fmt"
	"github.com/88250/lute/ast"
	"io/fs"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
	"wiki-kernel/internal/indexrepair"
)

const htmlHandoffLimit = 32
const htmlHandoffTTL = 30 * time.Minute

type htmlExportHandoff struct {
	temp, folder                     string
	tempInfo, exportInfo, folderInfo os.FileInfo
	created                          time.Time
}
type HTMLExportFolderResult struct{ Name, Content, Folder string }

// The record keeps identities, not open roots or a request Context. A later
// request must recapture its own owner and borrow the same recorded directories.
func (h htmlExportHandoff) open() (temp, exports, folder *os.Root, err error) {
	defer func() {
		if err != nil {
			if folder != nil {
				err = errors.Join(err, folder.Close())
				folder = nil
			}
			if exports != nil {
				err = errors.Join(err, exports.Close())
				exports = nil
			}
			if temp != nil {
				err = errors.Join(err, temp.Close())
				temp = nil
			}
		}
	}()
	info, e := os.Lstat(h.temp)
	if err = sameArchiveDirectory(info, e, h.tempInfo); err != nil {
		return
	}
	if temp, err = openArchivePublicationSource(h.temp); err != nil {
		return
	}
	info, e = temp.Lstat("export")
	if err = sameArchiveDirectory(info, e, h.exportInfo); err != nil {
		return
	}
	if exports, err = temp.OpenRoot("export"); err != nil {
		return
	}
	info, e = exports.Lstat(h.folder)
	if err = sameArchiveDirectory(info, e, h.folderInfo); err != nil {
		return
	}
	folder, err = exports.OpenRoot(h.folder)
	if err != nil {
		return
	}
	for _, root := range []struct {
		root *os.Root
		want os.FileInfo
	}{{temp, h.tempInfo}, {exports, h.exportInfo}, {folder, h.folderInfo}} {
		info, e = root.root.Stat(".")
		if err = sameArchiveDirectory(info, e, root.want); err != nil {
			return
		}
	}
	return
}
func (h htmlExportHandoff) remove() (err error) {
	temp, exports, folder, err := h.open()
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, folder.Close(), exports.Close(), temp.Close()) }()
	info, e := exports.Lstat(h.folder)
	if err = sameArchiveDirectory(info, e, h.folderInfo); err != nil {
		return err
	}
	return exports.RemoveAll(h.folder)
}
func htmlCloserName(c *AppConf) string { return fmt.Sprintf("html-export-owner-%p", c) }
func (c *AppConf) CloseHTMLExports(context.Context) (err error) {
	c.htmlExportMu.Lock()
	defer c.htmlExportMu.Unlock()
	c.htmlExportsClosed = true
	for key, h := range c.htmlExports {
		err = errors.Join(err, h.remove())
		delete(c.htmlExports, key)
	}
	unregisterLegacyServiceCloser(htmlCloserName(c))
	return err
}
func (c *AppConf) pruneHTMLExports(now time.Time) (err error) {
	for key, h := range c.htmlExports {
		if now.Sub(h.created) >= htmlHandoffTTL {
			err = errors.Join(err, h.remove())
			delete(c.htmlExports, key)
		}
	}
	return err
}
func ExportHTMLFolderContext(ctx context.Context, id string, markdown, pdf, keepFold, merge bool) (HTMLExportFolderResult, error) {
	m, err := captureMarkdownExport(ctx)
	if err != nil {
		return HTMLExportFolderResult{}, err
	}
	return m.prepareHTMLFolder(id, htmlExportFileOptions{Markdown: markdown, PDF: pdf, KeepFold: keepFold, Merge: merge})
}
func (m *markdownExportContext) prepareHTMLFolder(id string, options htmlExportFileOptions) (result HTMLExportFolderResult, err error) {
	if err = m.guard(); err != nil {
		return result, err
	}
	owner := m.read.binding.scope.commit.r.owner
	var staged *htmlExportHandoff
	completed := false
	defer func() {
		if !completed {
			result = HTMLExportFolderResult{}
			if staged != nil {
				owner.htmlExportMu.Lock()
				delete(owner.htmlExports, staged.folder)
				err = errors.Join(err, staged.remove())
				if len(owner.htmlExports) == 0 {
					unregisterLegacyServiceCloser(htmlCloserName(owner))
				}
				owner.htmlExportMu.Unlock()
			}
		}
	}()
	err = m.withHTMLFile(id, options, func(f htmlExportFile) (err error) {
		owner.htmlExportMu.Lock()
		defer owner.htmlExportMu.Unlock()
		defer func() {
			if err != nil && len(owner.htmlExports) == 0 {
				unregisterLegacyServiceCloser(htmlCloserName(owner))
			}
		}()
		if owner.htmlExportsClosed {
			return errors.New("html_exports_closed")
		}
		if err = owner.pruneHTMLExports(time.Now()); err != nil {
			return err
		}
		if len(owner.htmlExports) >= htmlHandoffLimit {
			return errors.New("html_exports_limit")
		}
		if err = RegisterLegacyServiceCloser(htmlCloserName(owner), owner.CloseHTMLExports); err != nil {
			return err
		}
		prefix := "html-"
		if options.Markdown {
			prefix = "htmlmd-"
		}
		h := htmlExportHandoff{temp: m.read.temp, folder: prefix + id + "-" + ast.NewNodeID(), tempInfo: f.output.tempInfo, exportInfo: f.output.exportInfo, created: time.Now()}
		if err = f.output.exports.Mkdir(h.folder, 0755); err != nil {
			return err
		}
		h.folderInfo, err = f.output.exports.Lstat(h.folder)
		if err != nil {
			return err
		}
		staged = &h
		target, err := f.output.exports.OpenRoot(h.folder)
		if err != nil {
			return err
		}
		copier := markdownExportResources{m: m, source: indexrepair.Source{Data: f.output.payload}, target: indexrepair.Source{Data: target, Authorize: func(ctx context.Context) error {
			if e := ctx.Err(); e != nil {
				return e
			}
			return m.guard()
		}}}
		err = copyHTMLDirectory(&copier)
		err = errors.Join(err, target.Close())
		if err != nil {
			return err
		}
		temp, exports, folder, err := h.open()
		if err != nil {
			return err
		}
		err = errors.Join(folder.Close(), exports.Close(), temp.Close())
		if err != nil {
			return err
		}
		if err = m.guard(); err != nil {
			return err
		}
		if owner.htmlExports == nil {
			owner.htmlExports = map[string]htmlExportHandoff{}
		}
		owner.htmlExports[h.folder] = h
		result = HTMLExportFolderResult{Name: f.name, Content: f.content, Folder: h.folder}
		return nil
	})
	if err == nil {
		owner.htmlExportMu.Lock()
		_, exists := owner.htmlExports[result.Folder]
		if owner.htmlExportsClosed || !exists {
			err = errors.New("html_exports_closed")
		}
		owner.htmlExportMu.Unlock()
	}
	completed = err == nil
	return result, err
}

func ExportBrowserHTMLContext(ctx context.Context, folder, content, name string) (string, error) {
	e, err := captureArchiveExport(ctx)
	if err != nil {
		return "", err
	}
	return (&markdownExportContext{read: e}).completeHTMLFolder(folder, content, name)
}
func (m *markdownExportContext) completeHTMLFolder(folder, content, name string) (zipURL string, err error) {
	if err = m.guard(); err != nil {
		return "", err
	}
	if !fs.ValidPath(folder) || folder == "." || strings.ContainsAny(folder, "/\\") {
		return "", indexrepair.ErrSourcePath
	}
	owner := m.read.binding.scope.commit.r.owner
	owner.htmlExportMu.Lock()
	defer owner.htmlExportMu.Unlock()
	if owner.htmlExportsClosed {
		return "", errors.New("html_exports_closed")
	}
	h, ok := owner.htmlExports[folder]
	if !ok || h.temp != m.read.temp {
		return "", errors.New("html_export_folder_unbound")
	}
	delete(owner.htmlExports, folder) // An accepted callback consumes the record once.
	var temp, exports, source *os.Root
	var o *archiveExportOutput
	keep := false
	defer func() {
		if source != nil {
			err = errors.Join(err, source.Close())
		}
		if exports != nil {
			err = errors.Join(err, exports.Close())
		}
		if temp != nil {
			err = errors.Join(err, temp.Close())
		}
		err = errors.Join(err, h.remove())
		if o != nil {
			err = errors.Join(err, o.finish(keep && err == nil))
		}
		if len(owner.htmlExports) == 0 {
			unregisterLegacyServiceCloser(htmlCloserName(owner))
		}
		if err != nil {
			zipURL = ""
		}
	}()
	if time.Since(h.created) >= htmlHandoffTTL {
		return "", errors.New("html_export_folder_expired")
	}
	temp, exports, source, err = h.open()
	if err != nil {
		return "", err
	}
	o, err = m.newOutput(name, ".html")
	if err != nil {
		return "", err
	}
	copier := markdownExportResources{m: m, source: indexrepair.Source{Data: source}, target: indexrepair.Source{Data: o.payload, Authorize: func(ctx context.Context) error {
		if e := ctx.Err(); e != nil {
			return e
		}
		return m.guard()
	}}}
	if err = copyHTMLDirectory(&copier); err != nil {
		return "", err
	}
	if err = writeHTMLIndex(o, content); err != nil {
		return "", err
	}
	if err = m.guard(); err != nil {
		return "", err
	}
	// Recheck the handoff identities after all source reads, before publication.
	checkTemp, checkExports, checkFolder, err := h.open()
	if err != nil {
		return "", err
	}
	if err = errors.Join(checkFolder.Close(), checkExports.Close(), checkTemp.Close()); err != nil {
		return "", err
	}
	if err = o.pack(); err != nil {
		return "", err
	}
	if err = m.guard(); err != nil {
		return "", err
	}
	keep = true
	return "/export/" + url.PathEscape(filepath.Base(o.filename)), nil
}
func writeHTMLIndex(o *archiveExportOutput, content string) (err error) {
	if err = o.e.guard(); err != nil {
		return err
	}
	target := indexrepair.Source{Data: o.payload}
	if err = target.Check("index.html", true); err != nil {
		return err
	}
	// Only the private copy is edited. A callback never writes into staged assets.
	f, err := o.payload.OpenFile("index.html", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	_, err = f.WriteString(content)
	if err == nil {
		err = f.Sync()
	}
	return errors.Join(err, f.Close(), o.e.guard())
}

type htmlDirectoryIO struct {
	readDir func(context.Context, indexrepair.Source, string) ([]fs.DirEntry, error)
}

// Root handles are already admitted; Source.Check deliberately rejects ".".
// Walk each member with the existing relative-path policy instead of widening it.
func copyHTMLDirectory(copier *markdownExportResources) error {
	return copyHTMLDirectoryWithIO(copier, htmlDirectoryIO{})
}
func copyHTMLDirectoryWithIO(copier *markdownExportResources, op htmlDirectoryIO) error {
	if err := copier.m.guard(); err != nil {
		return err
	}
	entries, err := copier.m.read.resourceManifest(copier.source, ".", op.readDir, nil, true)
	if err != nil {
		return err
	}
	for _, entry := range entries[1:] {
		if path.Dir(entry.name) != "." {
			continue
		}
		if err = copier.copyPath(entry.name, entry.name); err != nil {
			return err
		}
	}
	return copier.m.guard()
}
