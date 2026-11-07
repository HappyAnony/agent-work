// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"wiki-kernel/conf"
	"wiki-kernel/internal/indexrepair"
	"wiki-kernel/util"
)

// The device converter and optional reference document are selected once. The
// owner joins conversion, publication and scratch cleanup before returning.
func ExportDocxContext(ctx context.Context, id, folder string, removeAssets, merge bool) (out string, err error) {
	base, e := captureLegacyReindexOwner(ctx)
	if e != nil {
		return "", e
	}
	err = base.run(ctx, func(op, life context.Context) error {
		m, e := captureMarkdownExport(op)
		if e != nil {
			return e
		}
		destination, e := m.captureHTMLDesktopDestination(folder)
		if e != nil {
			return e
		}
		valid, e := util.ValidatePandocBin(op, m.settings.PandocBin)
		if e != nil {
			return e
		}
		if valid {
			m.pandocBin = m.settings.PandocBin
		} else {
			valid, e = util.ValidatePandocBin(op, m.pandocBin)
			if e != nil {
				return e
			}
			if !valid {
				return util.ErrPandocNotFound
			}
			// Persist only if the setting still matches the version selected above.
			owner := base.owner
			if e = owner.WithSettingSnapshot(op, func(_ conf.AppData) error {
				owner.m.Lock()
				defer owner.m.Unlock()
				if owner.Export == nil || owner.Export.PandocBin != m.settings.PandocBin {
					return indexrepair.ErrSourceChanged
				}
				data, e := owner.AppData.Clone()
				if e != nil {
					return e
				}
				data.Export.PandocBin = m.pandocBin
				if e = owner.saveDataLocked(data); e != nil {
					return e
				}
				owner.AppData = data
				return nil
			}); e != nil {
				return e
			}
		}
		m.pandocEnabled = true
		bin, e := exec.LookPath(m.pandocBin)
		if e != nil {
			return e
		}
		bin, e = filepath.EvalSymlinks(bin)
		if e != nil {
			return e
		}
		bin, e = filepath.Abs(bin)
		if e != nil {
			return e
		}
		tool, e := os.Lstat(bin)
		if e != nil {
			return e
		}
		if !tool.Mode().IsRegular() {
			return indexrepair.ErrSourcePath
		}
		m.pandocBin = bin
		m.pandocGuard = func() error {
			if e := m.guard(); e != nil {
				return e
			}
			now, e := os.Lstat(bin)
			if e != nil {
				return e
			}
			if !os.SameFile(tool, now) || now.Size() != tool.Size() || !now.ModTime().Equal(tool.ModTime()) {
				return indexrepair.ErrSourceChanged
			}
			return nil
		}
		reference := strings.TrimSpace(util.RemoveInvalid(m.settings.DocxTemplate))
		var template []byte
		if reference != "" {
			template, e = capturePDFWatermarkFile(reference).read(512<<20, m.guard)
			if e != nil {
				return e
			}
		}
		return m.withHTMLFile(id, htmlExportFileOptions{Markdown: true, Docx: true, Merge: merge}, func(f htmlExportFile) (err error) {
			referencePath := ""
			if reference != "" {
				if err = f.output.work.WriteFile("reference.docx", template, 0600); err != nil {
					return err
				}
				referencePath = filepath.Join(m.read.temp, f.output.workname, "reference.docx")
			}
			member := f.name + ".docx"
			if err = m.convertWithReference(f.output, member, "html+tex_math_dollars", "docx", strings.ReplaceAll(f.content, "  \n", "<br>\n"), referencePath); err != nil {
				return err
			}
			if err = destination.open(); err != nil {
				return err
			}
			defer func() { err = errors.Join(err, destination.root.Close()) }()
			source := indexrepair.Source{Data: f.output.payload}
			if !removeAssets {
				if info, e := source.Data.Lstat("assets"); e == nil {
					if !info.IsDir() {
						return indexrepair.ErrSourcePath
					}
					target := indexrepair.Source{Data: destination.root, Authorize: func(context.Context) error { return destination.check() }}
					if err = target.CopyDirectoryFrom(op, source, "assets", "assets"); err != nil {
						return err
					}
				} else if !os.IsNotExist(e) {
					return e
				}
			}
			out, err = destination.copyUniqueFile(op, source, member, member)
			return err
		})
	})
	if err != nil {
		return "", err
	}
	return
}
