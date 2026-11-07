//go:build fts5

// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"github.com/88250/lute/ast"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
	"wiki-kernel/internal/indexrepair"
	"wiki-kernel/util"
)

func markdownOutputFixture(t *testing.T) (*markdownExportContext, *archiveExportOutput) {
	t.Helper()
	m, _ := markdownFixture(t)
	o, err := m.newOutput("Original 文档", ".md")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { o.finish(false) })
	return m, o
}
func TestMarkdownExportPackageFlatUniqueLifecycle(t *testing.T) {
	m, o := markdownOutputFixture(t)
	old := archiveWrite(t, m.read.temp, "export/Original 文档.md.zip", []byte("prior ZIP"))
	archiveWrite(t, m.read.temp, "export/Original 文档.md/stale.md", []byte("old"))
	o.payload.MkdirAll("nested/empty", 0755)
	if err := m.convert(o, "nested/fresh.md", "", "", "Fresh bytes"); err != nil {
		t.Fatal(err)
	}
	if err := o.pack(); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(m.read.temp, "export", o.filename)
	got := readOutputZip(t, file)
	if string(got["nested/fresh.md"]) != "Fresh bytes" || got["nested/empty/"] == nil || len(got) != 2 {
		t.Fatal(got)
	}
	if err := o.finish(true); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(old)
	if string(raw) != "prior ZIP" {
		t.Fatal("overwrote legacy ZIP")
	}
	if _, err := os.Stat(filepath.Join(m.read.temp, o.workname)); !os.IsNotExist(err) {
		t.Fatal("scratch remains", err)
	}
	// Empty flat output must not contain a synthetic ./ or / wrapper member.
	empty, err := m.newOutput("Empty", ".md")
	if err != nil {
		t.Fatal(err)
	}
	defer empty.finish(false)
	if err = empty.pack(); err != nil {
		t.Fatal(err)
	}
	if got := readOutputZip(t, filepath.Join(m.read.temp, "export", empty.filename)); len(got) != 0 {
		t.Fatal(got)
	}
}
func TestMarkdownExportPackageConcurrentOutputsAndCanceledCleanup(t *testing.T) {
	m, _ := markdownFixture(t)
	names := map[string]bool{}
	outputs := []*archiveExportOutput{}
	contexts := []*markdownExportContext{}
	for n := 0; n < 4; n++ {
		c, err := captureMarkdownExport(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		o, err := c.newOutput("Same", ".md")
		if err != nil {
			t.Fatal(err)
		}
		if names[o.filename] {
			t.Fatal("shared filename")
		}
		names[o.filename] = true
		outputs = append(outputs, o)
		contexts = append(contexts, c)
		t.Cleanup(func() { o.finish(false) })
	}
	var wg sync.WaitGroup
	errs := make(chan error, 4)
	for i, o := range outputs {
		wg.Add(1)
		go func(i int, o *archiveExportOutput) {
			defer wg.Done()
			err := contexts[i].convert(o, "fresh.md", "", "", "fresh")
			if err == nil {
				err = o.pack()
			}
			errs <- errors.Join(err, o.finish(err == nil))
		}(i, o)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	c, err := captureMarkdownExport(ctx)
	if err != nil {
		t.Fatal(err)
	}
	o, err := c.newOutput("Canceled", ".md")
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	if err = c.convert(o, "no.md", "", "", "no"); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err = o.pack(); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err = o.finish(false); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(m.read.temp, o.workname)); !os.IsNotExist(err) {
		t.Fatal(err)
	}
}
func TestMarkdownExportPackageResourcesCapturedPrecedenceAndDirectories(t *testing.T) {
	m, box := markdownFixture(t)
	archiveWrite(t, util.DataDir, box+"/assets/file.txt", []byte("notebook"))
	global := archiveWrite(t, util.DataDir, "assets/file.txt", []byte("global"))
	archiveWrite(t, util.DataDir, "assets/bundle.rtfd/.hidden", []byte("hidden"))
	os.MkdirAll(filepath.Join(util.DataDir, "assets/bundle.rtfd/empty"), 0755)
	o, err := m.newOutput("Resources", ".md")
	if err != nil {
		t.Fatal(err)
	}
	defer o.finish(false)
	r, err := m.openResources(o)
	if err != nil {
		t.Fatal(err)
	}
	defer r.close()
	for _, asset := range []string{"assets/file.txt?download=1", "assets/bundle.rtfd/", "assets/missing.txt"} {
		if err = r.copyAsset(asset, "nested"); err != nil {
			t.Fatal(err)
		}
	}
	got, err := o.payload.ReadFile("nested/assets/file.txt")
	if err != nil || string(got) != "global" {
		t.Fatal(string(got), err)
	}
	got, err = o.payload.ReadFile("nested/assets/bundle.rtfd/.hidden")
	if err != nil || string(got) != "hidden" {
		t.Fatal(string(got), err)
	}
	if info, err := o.payload.Stat("nested/assets/bundle.rtfd/empty"); err != nil || !info.IsDir() {
		t.Fatal(err)
	}
	if raw, _ := os.ReadFile(global); string(raw) != "global" {
		t.Fatal("source rewritten")
	}
	for _, bad := range []string{"assets/../escape", "assets/link/../../escape", "assets/a\\b"} {
		if err = r.copyAsset(bad, "."); err == nil {
			t.Fatal(bad)
		}
	}
}

func TestMarkdownExportPackageCheckedEnumerationBeforePublication(t *testing.T) {
	primary := errors.New("markdown export package enumeration incomplete")
	for _, kind := range []string{"children-incomplete", "children-replaced", "copy-incomplete", "copy-replaced"} {
		t.Run(kind, func(t *testing.T) {
			m, box := markdownFixture(t)
			o, err := m.newOutput("Checked resources", ".md")
			if err != nil {
				t.Fatal(err)
			}
			defer o.finish(false)
			op := markdownExportResourceIO{}
			if strings.HasPrefix(kind, "children-") {
				id := ast.NewNodeID()
				parent := box + "/" + id
				archiveWrite(t, util.DataDir, parent+"/child.docj", []byte("child"))
				op.childrenReadDir = func(ctx context.Context, source indexrepair.Source, name string) ([]fs.DirEntry, error) {
					if name != parent {
						return source.ReadDir(ctx, name)
					}
					if kind == "children-incomplete" {
						return nil, errors.Join(indexrepair.ErrSourceReadIncomplete, os.ErrNotExist, primary)
					}
					entries, readErr := source.ReadDir(ctx, name)
					if readErr != nil {
						return nil, readErr
					}
					if err := source.Data.Rename(parent, parent+".old"); err != nil {
						return nil, err
					}
					if err := source.Data.Mkdir(parent, 0700); err != nil {
						return nil, err
					}
					return entries, nil
				}
				r, openErr := m.openResourcesWithIO(o, op)
				if openErr != nil {
					t.Fatal(openErr)
				}
				defer r.close()
				ok, readErr := r.hasChildren(box, "/"+id+".docj")
				if readErr == nil || ok {
					t.Fatal("failed child enumeration admitted result", ok, readErr)
				}
				if kind == "children-incomplete" {
					if !errors.Is(readErr, primary) || !errors.Is(readErr, indexrepair.ErrSourceReadIncomplete) || indexrepair.IsSourceNotExist(readErr) {
						t.Fatal("incomplete child enumeration error lost or admitted as missing", readErr)
					}
				} else if !errors.Is(readErr, indexrepair.ErrSourceChanged) {
					t.Fatal("replaced child directory accepted", readErr)
				}
				return
			}

			early := archiveWrite(t, util.DataDir, "assets/bundle.rtfd/early.txt", []byte("early"))
			archiveWrite(t, util.DataDir, "assets/bundle.rtfd/z/late.txt", []byte("late"))
			op.copyReadDir = func(ctx context.Context, source indexrepair.Source, name string) ([]fs.DirEntry, error) {
				if name == "assets/bundle.rtfd/z" {
					if kind == "copy-incomplete" {
						return nil, errors.Join(indexrepair.ErrSourceReadIncomplete, os.ErrNotExist, primary)
					}
					if err := source.Data.Rename("assets/bundle.rtfd/early.txt", "assets/bundle.rtfd/early.txt.old"); err != nil {
						return nil, err
					}
					if err := source.Data.WriteFile("assets/bundle.rtfd/early.txt", []byte("replacement"), 0600); err != nil {
						return nil, err
					}
				}
				return source.ReadDir(ctx, name)
			}
			r, openErr := m.openResourcesWithIO(o, op)
			if openErr != nil {
				t.Fatal(openErr)
			}
			defer r.close()
			err = r.copyAsset("assets/bundle.rtfd/", "nested")
			if err == nil {
				t.Fatal("failed resource enumeration admitted copy")
			}
			if _, statErr := o.payload.Lstat("nested/assets/bundle.rtfd"); !os.IsNotExist(statErr) {
				t.Fatal("resource enumeration failure created target directory", statErr)
			}
			if kind == "copy-incomplete" {
				if !errors.Is(err, primary) || !errors.Is(err, indexrepair.ErrSourceReadIncomplete) || indexrepair.IsSourceNotExist(err) {
					t.Fatal("incomplete resource enumeration error lost or admitted as missing", err)
				}
				if raw, readErr := os.ReadFile(early); readErr != nil || string(raw) != "early" {
					t.Fatal("failed resource enumeration changed source", readErr)
				}
			} else if !errors.Is(err, indexrepair.ErrSourceChanged) {
				t.Fatal("scan-time resource replacement accepted", err)
			}
		})
	}
}

func TestMarkdownExportPackageSourcesAndDestinationsRejectLinks(t *testing.T) {
	m, o := markdownOutputFixture(t)
	outside := archiveWrite(t, t.TempDir(), "outside", []byte("outside"))
	os.MkdirAll(filepath.Join(util.DataDir, "assets"), 0755)
	link := filepath.Join(util.DataDir, "assets/link")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	if r, err := m.openResources(o); err == nil {
		r.close()
		t.Fatal("linked source admitted")
	}
	os.Remove(link)
	if err := o.payload.Symlink(outside, "escape.md"); err != nil {
		t.Fatal(err)
	}
	if err := m.convert(o, "escape.md", "", "", "overwritten"); err == nil {
		t.Fatal("linked destination admitted")
	}
	raw, _ := os.ReadFile(outside)
	if string(raw) != "outside" {
		t.Fatal("outside changed")
	}
}
func TestMarkdownExportPackageEmptyParentAndPathValidation(t *testing.T) {
	m, box := markdownFixture(t)
	id := ast.NewNodeID()
	markdownDoc(t, m, box, "/"+id+".docj")
	o, err := m.newOutput("Paths", ".md")
	if err != nil {
		t.Fatal(err)
	}
	defer o.finish(false)
	r, err := m.openResources(o)
	if err != nil {
		t.Fatal(err)
	}
	defer r.close()
	if ok, err := r.hasChildren(box, "/"+id+".docj"); err != nil || ok {
		t.Fatal(ok, err)
	}
	archiveWrite(t, util.DataDir, box+"/"+id+"/"+ast.NewNodeID()+".docj", []byte("child"))
	if ok, err := r.hasChildren(box, "/"+id+".docj"); err != nil || !ok {
		t.Fatal(ok, err)
	}
	if _, err = r.hasChildren(box, "/../escape.docj"); err == nil {
		t.Fatal("bad parent path")
	}
	for _, bad := range []string{"/../outside", "/nested/../../outside"} {
		if _, err = markdownExportPath(bad, ".md"); err == nil {
			t.Fatal(bad)
		}
	}
	if got, err := markdownExportPath("/back\\slash", ".md"); err != nil || got != "back_slash.md" {
		t.Fatal(got, err)
	}
	if got, err := markdownExportPath("/Parent/中文 space", ".md"); err != nil || got != "Parent/中文 space.md" {
		t.Fatal(got, err)
	}
}
func pandocFixture(t *testing.T, body string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX executable fixture")
	}
	file := filepath.Join(t.TempDir(), "pandoc fixture")
	if err := os.WriteFile(file, []byte("#!/bin/sh\n"+body), 0755); err != nil {
		t.Fatal(err)
	}
	return file
}
func TestMarkdownExportPackagePandocCapturedRunnerAndOriginalArguments(t *testing.T) {
	bin := pandocFixture(t, `input="$1"
shift
[ "$1" = "--from" ] && [ "$2" = "gfm" ] || exit 11
shift 2
[ "$1" = "--to" ] && [ "$2" = "epub" ] || exit 12
shift 2
[ "$1" = "--resource-path" ] && [ -d "$2" ] || exit 13
shift 2
[ "$1" = "-s" ] && [ "$2" = "-o" ] || exit 14
cp "$input" "$3"
`)
	m, _ := markdownFixture(t)
	oldBin, oldContainer := util.PandocBinPath, util.Container
	t.Cleanup(func() { util.PandocBinPath = oldBin; util.Container = oldContainer })
	util.PandocBinPath = bin
	util.Container = util.ContainerStd
	c, err := captureMarkdownExport(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	util.PandocBinPath = "/missing/later-pandoc"
	util.Container = "later"
	o, err := c.newOutput("Pandoc", ".epub")
	if err != nil {
		t.Fatal(err)
	}
	defer o.finish(false)
	o.payload.Mkdir("nested space", 0755)
	if err = c.convert(o, "nested space/doc.epub", "gfm", "epub", "converted bytes"); err != nil {
		t.Fatal(err)
	}
	got, err := o.payload.ReadFile("nested space/doc.epub")
	if err != nil || string(got) != "converted bytes" {
		t.Fatal(string(got), err)
	}
	if _, err = o.work.Stat("pandoc-input"); !os.IsNotExist(err) {
		t.Fatal("input remains", err)
	}
	if m.pandocBin == c.pandocBin {
		t.Fatal("fixture did not change runner")
	}
}
func TestMarkdownExportPackagePandocFailureCancellationAndNoOutput(t *testing.T) {
	for _, kind := range []string{"exit", "no-output", "cancel", "missing"} {
		t.Run(kind, func(t *testing.T) {
			m, _ := markdownFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			m.read.binding.scope.ctx = ctx
			body := "exit 7\n"
			if kind == "no-output" {
				body = "exit 0\n"
			}
			if kind == "cancel" {
				body = "while :; do :; done\n"
			}
			m.pandocBin = pandocFixture(t, body)
			m.pandocEnabled = true
			if kind == "missing" {
				m.pandocBin = ""
			}
			o, err := m.newOutput("Failed", ".epub")
			if err != nil {
				t.Fatal(err)
			}
			if kind == "cancel" {
				time.AfterFunc(40*time.Millisecond, cancel)
			}
			if kind == "no-output" {
				if err = o.payload.WriteFile("doc.epub", []byte("stale equal-title output"), 0644); err != nil {
					t.Fatal(err)
				}
			}
			start := time.Now()
			err = m.convert(o, "doc.epub", "gfm", "epub", "private content")
			if err == nil {
				t.Fatal("failed tool admitted")
			}
			if kind == "cancel" && (!errors.Is(err, context.Canceled) || time.Since(start) > 3*time.Second) {
				t.Fatal(err)
			}
			if err = o.finish(false); err != nil {
				t.Fatal(err)
			}
			if _, err = os.Stat(filepath.Join(m.read.temp, o.workname)); !os.IsNotExist(err) {
				t.Fatal("failed work remains", err)
			}
			if _, err = os.Stat(filepath.Join(m.read.temp, "export", o.filename)); !os.IsNotExist(err) {
				t.Fatal("failed URL file exists", err)
			}
		})
	}
}
func TestMarkdownExportPackageControllerDoesNotPublishPartialFailure(t *testing.T) {
	m, box := markdownFixture(t)
	id := ast.NewNodeID()
	markdownDoc(t, m, box, "/"+id+".docj")
	selections, err := m.selectDocuments([]string{id})
	if err != nil {
		t.Fatal(err)
	}
	defs, trees, paths, err := m.prepare(selections)
	if err != nil {
		t.Fatal(err)
	}
	old := archiveWrite(t, m.read.temp, "export/Prepared Title.epub.zip", []byte("prior"))
	m.pandocEnabled = true
	m.pandocBin = pandocFixture(t, `while [ "$1" != "-o" ]; do shift; done
shift
printf partial > "$1"
exit 9
`)
	if zip := exportPandocConvertZip("Prepared Title", paths, defs, "gfm", "epub", ".epub", trees, m); zip != "" {
		t.Fatal(zip)
	}
	raw, _ := os.ReadFile(old)
	if string(raw) != "prior" {
		t.Fatal("prior overwritten")
	}
	entries, _ := os.ReadDir(m.read.temp)
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".wiki-archive-") {
			t.Fatal("leaked work")
		}
	}
	exports, _ := os.ReadDir(filepath.Join(m.read.temp, "export"))
	if len(exports) != 1 {
		t.Fatal("partial ZIP published", exports)
	}
}

func TestMarkdownExportPackageOriginalDuplicateTitleSuffix(t *testing.T) {
	m, box := markdownFixture(t)
	ids := []string{ast.NewNodeID(), ast.NewNodeID()}
	for i, id := range ids {
		markdownDoc(t, m, box, "/"+id+".docj", map[string]interface{}{"ID": ast.NewNodeID(), "Type": "NodeParagraph", "Children": []interface{}{map[string]interface{}{"Type": "NodeText", "Data": []string{"First unique text", "Second unique text"}[i]}}})
	}
	selected, err := m.selectDocuments(ids)
	if err != nil {
		t.Fatal(err)
	}
	defs, trees, paths, err := m.prepare(selected)
	if err != nil {
		t.Fatal(err)
	}
	url := exportPandocConvertZip("Prepared Title", paths, defs, "", "", ".md", trees, m)
	if url == "" {
		t.Fatal("export failed")
	}
	files, _ := os.ReadDir(filepath.Join(m.read.temp, "export"))
	if len(files) != 1 {
		t.Fatal(files)
	}
	got := readOutputZip(t, filepath.Join(m.read.temp, "export", files[0].Name()))
	combined := ""
	for name, raw := range got {
		if name != "Prepared Title.md" && name != "Prepared Title-"+ids[0]+".md" && name != "Prepared Title-"+ids[1]+".md" {
			t.Fatal(name)
		}
		combined += string(raw)
	}
	if len(got) != 2 || !strings.Contains(combined, "First unique text") || !strings.Contains(combined, "Second unique text") {
		t.Fatal(got)
	}
}
