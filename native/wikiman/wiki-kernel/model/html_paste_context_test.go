//go:build fts5

// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"bytes"
	"context"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"wiki-kernel/conf"
	"wiki-kernel/util"
)

func htmlPasteFixture(t *testing.T) (*htmlPasteContext, string) {
	t.Helper()
	m, _, folder := markdownSourceFixture(t)
	owner := m.archive.binding.scope.commit.r.owner
	owner.m.Lock()
	owner.System = conf.NewSystem()
	owner.System.Container = util.ContainerStd
	owner.Editor.Markdown.InlineAsterisk = true
	owner.m.Unlock()
	p, err := captureHTMLPaste(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return p, folder
}
func pasteAssetNames(t *testing.T, p *htmlPasteContext) []string {
	t.Helper()
	folder := filepath.Join(p.preparation.archive.binding.scope.commit.r.data, "assets")
	entries, err := os.ReadDir(folder)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}
func TestHTMLPasteOriginalParagraphTableAndCapturedParser(t *testing.T) {
	p, _ := htmlPasteFixture(t)
	owner := p.preparation.archive.binding.scope.commit.r.owner
	owner.m.Lock()
	owner.Editor.Markdown.InlineAsterisk = false
	owner.System.Container = "android"
	owner.m.Unlock()
	old := util.MarkdownSettings
	poison := *old
	poison.InlineAsterisk = false
	util.MarkdownSettings = &poison
	defer func() { util.MarkdownSettings = old }()
	output, err := p.convert(`<p><strong>Retained bold</strong></p><table><tr><td>Single cell</td></tr></table>`)
	if err != nil || !strings.Contains(output, "Retained bold") || !strings.Contains(output, "Single cell") || strings.Contains(output, "**Retained bold**") || !strings.Contains(output, `data-type="strong"`) {
		t.Fatal(output, err)
	}
	cell, err := p.convert(`<table><tr><td>Single cell only</td></tr></table>`)
	if err != nil || !strings.Contains(cell, "Single cell only") || strings.Contains(cell, `data-type="NodeTable"`) {
		t.Fatal(cell, err)
	}
}
func TestHTMLPasteAbsoluteAndEncodedURLCopiesRetainFreshNamingAndSource(t *testing.T) {
	p, folder := htmlPasteFixture(t)
	file := filepath.Join(folder, "image space-20261101090000-abc0001.png")
	raw := []byte("clipboard fixture bytes")
	if err := os.WriteFile(file, raw, 0600); err != nil {
		t.Fatal(err)
	}
	uri := (&url.URL{Scheme: "file", Path: file}).String()
	for _, source := range []string{file, uri} {
		before := pasteAssetNames(t, p)
		out, err := p.convert(`<p>Original paragraph</p><img alt="picture" width="24" src="` + source + `">`)
		if err != nil || !strings.Contains(out, "Original paragraph") || !strings.Contains(out, "assets/image space-20261101090000-abc0001-") {
			t.Fatal(out, err)
		}
		after := pasteAssetNames(t, p)
		if len(after) != len(before)+1 {
			t.Fatal(before, after)
		}
		old := map[string]bool{}
		for _, n := range before {
			old[n] = true
		}
		for _, name := range after {
			if !old[name] {
				data, err := os.ReadFile(filepath.Join(p.preparation.archive.binding.scope.commit.r.data, "assets", name))
				if err != nil || !bytes.Equal(data, raw) {
					t.Fatal(err)
				}
			}
		}
	}
	if data, err := os.ReadFile(file); err != nil || !bytes.Equal(data, raw) {
		t.Fatal("clipboard source changed", err)
	}
}
func TestHTMLPasteRelativeRemoteMissingAndNonDesktopStayLiteral(t *testing.T) {
	p, folder := htmlPasteFixture(t)
	file := filepath.Join(folder, "local.txt")
	os.WriteFile(file, []byte("local"), 0600)
	before := len(pasteAssetNames(t, p))
	for _, source := range []string{"relative.png", "https://example.com/image.png", filepath.Join(folder, "missing.png"), "file:///invalid%ZZ"} {
		out, err := p.convert(`<p><a href="` + source + `">Retained link</a></p>`)
		if err != nil || !strings.Contains(out, "Retained link") {
			t.Fatal(out, err)
		}
	}
	p.container = "android"
	out, err := p.convert(`<p><a href="` + file + `">Mobile link</a></p>`)
	if err != nil || !strings.Contains(out, file) || len(pasteAssetNames(t, p)) != before {
		t.Fatal(out, err)
	}
}
func TestHTMLPasteLocalDirectoriesPreserveNestedAndEmptyChildren(t *testing.T) {
	p, folder := htmlPasteFixture(t)
	dir := filepath.Join(folder, "Clipboard directory")
	if err := os.MkdirAll(filepath.Join(dir, "empty"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "nested"), 0700); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "nested", "part.bin"), []byte("nested content"), 0600)
	out, err := p.convert(`<a href="` + (&url.URL{Scheme: "file", Path: dir}).String() + `">Directory link</a>`)
	if err != nil || !strings.Contains(out, "assets/Clipboard directory-") {
		t.Fatal(out, err)
	}
	names := pasteAssetNames(t, p)
	found := false
	for _, name := range names {
		if strings.HasPrefix(name, "Clipboard directory-") {
			root := filepath.Join(p.preparation.archive.binding.scope.commit.r.data, "assets", name)
			data, err := os.ReadFile(filepath.Join(root, "nested/part.bin"))
			if err != nil || string(data) != "nested content" {
				t.Fatal(err)
			}
			if info, err := os.Stat(filepath.Join(root, "empty")); err != nil || !info.IsDir() {
				t.Fatal(err)
			}
			found = true
		}
	}
	if !found {
		t.Fatal("directory attachment absent")
	}
}
func TestHTMLPasteReadWriteAndSourceLinkFailuresReturnNoDOM(t *testing.T) {
	for _, kind := range []string{"read", "write", "link"} {
		t.Run(kind, func(t *testing.T) {
			p, folder := htmlPasteFixture(t)
			file := filepath.Join(folder, "local.png")
			os.WriteFile(file, []byte("private source"), 0600)
			switch kind {
			case "read":
				if os.Geteuid() == 0 {
					t.Skip("root bypasses permissions")
				}
				os.Chmod(file, 0000)
				defer os.Chmod(file, 0600)
			case "write":
				if os.Geteuid() == 0 {
					t.Skip("root bypasses permissions")
				}
				target := filepath.Join(p.preparation.archive.binding.scope.commit.r.data, "assets")
				os.MkdirAll(target, 0755)
				os.Chmod(target, 0500)
				defer os.Chmod(target, 0755)
			case "link":
				alias := filepath.Join(folder, "alias.png")
				if err := os.Symlink(file, alias); err != nil {
					t.Fatal(err)
				}
				file = alias
			}
			out, err := p.convert(`<img src="` + (&url.URL{Scheme: "file", Path: file}).String() + `">`)
			if err == nil || out != "" {
				t.Fatal("failed local copy returned DOM", out, err)
			}
		})
	}
}
func TestHTMLPasteSelectedSourceAndTargetCloseFailuresReturnNoDOM(t *testing.T) {
	for _, kind := range []string{"source", "target"} {
		t.Run(kind, func(t *testing.T) {
			p, folder := htmlPasteFixture(t)
			file := filepath.Join(folder, "local.png")
			os.WriteFile(file, []byte("original source"), 0600)
			sentinel := errors.New("paste close failure")
			r := p.preparation.archive.binding.scope.commit.r
			p.preparation.archive.closeRoot = func(root *os.Root) error {
				err := root.Close()
				if kind == "source" && filepath.Base(root.Name()) == "Knowledge" || kind == "target" && root.Name() == r.data {
					return errors.Join(err, sentinel)
				}
				return err
			}
			out, err := p.convert(`<img src="` + (&url.URL{Scheme: "file", Path: file}).String() + `">`)
			if !errors.Is(err, sentinel) || out != "" {
				t.Fatal(out, err)
			}
		})
	}
}
func TestHTMLPasteCancellationReadonlyAndOwnerReplacementRejectWork(t *testing.T) {
	for _, kind := range []string{"cancel", "readonly", "owner"} {
		t.Run(kind, func(t *testing.T) {
			p, _ := htmlPasteFixture(t)
			switch kind {
			case "cancel":
				ctx, cancel := context.WithCancel(p.preparation.ctx)
				p.preparation.ctx = ctx
				p.preparation.archive.ctx = ctx
				cancel()
			case "readonly":
				owner := p.preparation.archive.binding.scope.commit.r.owner
				owner.m.Lock()
				owner.persistenceReadOnly = true
				owner.m.Unlock()
				defer func() { owner.m.Lock(); owner.persistenceReadOnly = false; owner.m.Unlock() }()
			case "owner":
				old := Conf
				Conf = NewAppConf()
				defer func() { Conf = old }()
			}
			out, err := p.convert(`<p>Rejected output</p>`)
			if err == nil || out != "" {
				t.Fatal(out, err)
			}
		})
	}
}
func TestHTMLPasteCaptureUsesOneDetachedContainerPolicy(t *testing.T) {
	p, folder := htmlPasteFixture(t)
	file := filepath.Join(folder, "local.txt")
	os.WriteFile(file, []byte("local text"), 0600)
	owner := p.preparation.archive.binding.scope.commit.r.owner
	owner.m.Lock()
	owner.System.Container = "android"
	owner.m.Unlock()
	before := len(pasteAssetNames(t, p))
	if out, err := p.convert(`<a href="` + file + `">Captured desktop link</a>`); err != nil || !strings.Contains(out, "assets/local-") {
		t.Fatal(out, err)
	}
	if len(pasteAssetNames(t, p)) != before+1 {
		t.Fatal("captured desktop policy lost")
	}
	fresh, err := captureHTMLPaste(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if out, err := fresh.convert(`<a href="` + file + `">Current mobile link</a>`); err != nil || !strings.Contains(out, file) {
		t.Fatal(out, err)
	}
	if len(pasteAssetNames(t, p)) != before+1 {
		t.Fatal("mobile policy copied local source")
	}
}
