//go:build fts5

// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"bytes"
	"context"
	"errors"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"wiki-kernel/conf"
	"wiki-kernel/internal/indexrepair"
	"wiki-kernel/util"
)

func htmlResourceFixture(t *testing.T) (*markdownExportContext, *parse.Tree, string, string, map[string][]byte) {
	t.Helper()
	m, box := markdownFixture(t)
	work, appearance := t.TempDir(), t.TempDir()
	files := map[string][]byte{}
	write := func(root, p, value string) {
		name := archiveWrite(t, root, p, []byte(value))
		files[name] = []byte(value)
	}
	write(work, "stage/build/export/index.js", "export script")
	write(work, "stage/protyle/runtime.js", "runtime")
	write(work, "stage/protyle/fonts/font.woff", "font")
	write(appearance, "themes/custom/theme.css", "theme css")
	write(appearance, "themes/custom/.theme", "hidden theme")
	write(appearance, "themes/custom/images/bg.png", "background")
	write(appearance, "icons/custom/icon.js", "custom icon")
	write(appearance, "icons/custom/unused.svg", "unused icon")
	write(appearance, "icons/material/icon.js", "fallback icon")
	write(m.read.binding.scope.commit.r.data, "assets/same.pdf", "global PDF")
	write(m.read.binding.scope.commit.r.data, box+"/assets/same.pdf", "notebook PDF")
	write(m.read.binding.scope.commit.r.data, "assets/bundle.rtfd/.hidden", "bundle hidden")
	os.MkdirAll(filepath.Join(m.read.binding.scope.commit.r.data, "assets/bundle.rtfd/empty"), 0755)
	write(m.read.binding.scope.commit.r.data, "emojis/face.png", "custom emoji")
	doc := markdownDoc(t, m, box, "/"+ast.NewNodeID()+".docj", footParagraph(ast.NewNodeID(), resourceLink("assets/same.pdf?download=1#page=2"), resourceLink("assets/bundle.rtfd/"), map[string]interface{}{"Type": "NodeEmojiImg", "Data": "<img src=\"/emojis/face.png\" />"}))
	docFile := filepath.Join(m.read.binding.scope.commit.r.data, box, doc.ID+".docj")
	files[docFile], _ = os.ReadFile(docFile)
	tree, _, err := m.prepareFocusedExport(doc.ID)
	if err != nil {
		t.Fatal(err)
	}
	tree.Root.SetIALAttr("icon", "face.png")
	m.htmlResources = captureHTMLResourceSnapshot(work, appearance, "custom", "custom", true)
	return m, tree, work, appearance, files
}
func htmlPayload(t *testing.T, m *markdownExportContext, tree *parse.Tree, static bool) map[string][]byte {
	t.Helper()
	o, err := m.newOutput("HTML resources", ".html")
	if err != nil {
		t.Fatal(err)
	}
	defer o.finish(false)
	if err = m.copyHTMLResources(o, tree, static); err != nil {
		t.Fatal(err)
	}
	if err = o.pack(); err != nil {
		t.Fatal(err)
	}
	return readOutputZip(t, filepath.Join(m.read.temp, "export", o.filename))
}
func TestHTMLResourcesOriginalLayoutAssetsEmojiAndRealZIP(t *testing.T) {
	m, tree, _, _, before := htmlResourceFixture(t)
	got := htmlPayload(t, m, tree, true)
	expected := map[string]string{"stage/build/export/index.js": "export script", "stage/protyle/runtime.js": "runtime", "stage/protyle/fonts/font.woff": "font", "appearance/themes/custom/theme.css": "theme css", "appearance/themes/custom/.theme": "hidden theme", "appearance/themes/custom/images/bg.png": "background", "appearance/icons/custom/icon.js": "custom icon", "appearance/icons/material/icon.js": "fallback icon", "emojis/face.png": "custom emoji", "assets/same.pdf": "global PDF", "assets/bundle.rtfd/.hidden": "bundle hidden"}
	for name, value := range expected {
		if string(got[name]) != value {
			t.Fatal(name, string(got[name]))
		}
	}
	if got["assets/bundle.rtfd/empty/"] == nil || got["appearance/icons/custom/unused.svg"] != nil || got["assets/same.pdf?download=1#page=2"] != nil {
		t.Fatal("layout changed")
	}
	for file, want := range before {
		raw, _ := os.ReadFile(file)
		if !bytes.Equal(raw, want) {
			t.Fatal("source modified", file)
		}
	}
	again := htmlPayload(t, m, tree, true)
	if !reflect.DeepEqual(got, again) {
		t.Fatal("repeat changed resources")
	}
}
func TestHTMLResourcesCheckedRootEnumerationBeforeDestination(t *testing.T) {
	primary := errors.New("html resource root enumeration incomplete")
	for _, kind := range []string{"incomplete", "replaced"} {
		t.Run(kind, func(t *testing.T) {
			m, _, _, appearance, _ := htmlResourceFixture(t)
			root := captureHTMLResourceRoot(filepath.Join(appearance, "themes/custom"))
			o, err := m.newOutput("Checked HTML resources", ".html")
			if err != nil {
				t.Fatal(err)
			}
			defer o.finish(false)
			r, err := m.openResources(o)
			if err != nil {
				t.Fatal(err)
			}
			defer r.close()
			op := htmlExportResourceIO{rootReadDir: func(ctx context.Context, source indexrepair.Source, name string) ([]fs.DirEntry, error) {
				if kind == "incomplete" {
					return nil, errors.Join(indexrepair.ErrSourceReadIncomplete, os.ErrNotExist, primary)
				}
				entries, readErr := source.ReadDir(ctx, name)
				if readErr != nil {
					return nil, readErr
				}
				if err := os.Rename(root.path, root.path+".old"); err != nil {
					return nil, err
				}
				if err := os.Mkdir(root.path, 0700); err != nil {
					return nil, err
				}
				return entries, nil
			}}
			err = m.copyHTMLDeviceResourceWithIO(r.target, root, ".", "appearance/themes/custom", false, op)
			if err == nil {
				t.Fatal("failed root enumeration admitted destination")
			}
			if _, statErr := o.payload.Lstat("appearance/themes/custom"); !os.IsNotExist(statErr) {
				t.Fatal("root enumeration failure created destination", statErr)
			}
			if kind == "incomplete" {
				if !errors.Is(err, primary) || !errors.Is(err, indexrepair.ErrSourceReadIncomplete) || indexrepair.IsSourceNotExist(err) {
					t.Fatal("incomplete root enumeration error lost or admitted as missing", err)
				}
			} else if !errors.Is(err, indexrepair.ErrSourceChanged) {
				t.Fatal("replaced resource root accepted", err)
			}
		})
	}
}
func TestHTMLResourcesCapturedRootsAliasesAndConfiguration(t *testing.T) {
	m, tree, work, appearance, _ := htmlResourceFixture(t)
	alias := filepath.Join(t.TempDir(), "appearance")
	if err := os.Symlink(appearance, alias); err != nil {
		t.Fatal(err)
	}
	oldWork, oldAppearance := util.WorkingDir, util.AppearancePath
	util.WorkingDir, util.AppearancePath = work, alias
	t.Cleanup(func() { util.WorkingDir, util.AppearancePath = oldWork, oldAppearance })
	Conf.m.Lock()
	Conf.Appearance = conf.NewAppearance()
	Conf.Appearance.ThemeLight = "custom"
	Conf.Appearance.Icon = "custom"
	Conf.m.Unlock()
	captured, err := captureMarkdownExport(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	selected, _, err := captured.prepareFocusedExport(tree.ID)
	if err != nil {
		t.Fatal(err)
	}
	selected.Root.SetIALAttr("icon", "face.png")
	util.WorkingDir, util.AppearancePath = t.TempDir(), t.TempDir()
	Conf.m.Lock()
	Conf.Appearance.ThemeLight = "elsewhere"
	Conf.Appearance.Icon = "ant"
	Conf.m.Unlock()
	os.Remove(alias)
	if err = os.Symlink(t.TempDir(), alias); err != nil {
		t.Fatal(err)
	}
	got := htmlPayload(t, captured, selected, true)
	if string(got["stage/build/export/index.js"]) != "export script" || string(got["appearance/themes/custom/theme.css"]) != "theme css" || string(got["appearance/icons/custom/icon.js"]) != "custom icon" {
		t.Fatal("followed current globals or alias")
	}
	// A selected theme directory may itself be an explicitly configured alias.
	other := t.TempDir()
	archiveWrite(t, other, "theme.css", []byte("linked theme"))
	os.Rename(filepath.Join(appearance, "themes/custom"), filepath.Join(appearance, "themes/original"))
	os.Symlink(other, filepath.Join(appearance, "themes/custom"))
	m.htmlResources = captureHTMLResourceSnapshot(work, appearance, "custom", "", true)
	os.Remove(filepath.Join(appearance, "themes/custom"))
	os.Symlink(t.TempDir(), filepath.Join(appearance, "themes/custom"))
	if got = htmlPayload(t, m, tree, true); string(got["appearance/themes/custom/theme.css"]) != "linked theme" {
		t.Fatal("theme alias retargeted")
	}
}
func TestHTMLResourcesAssetOnlyModeAndSelectedAnnotation(t *testing.T) {
	m, tree, _, _, _ := htmlResourceFixture(t)
	m.htmlResources = htmlResourceSnapshot{}
	selected := m.read.binding.scope.commit.r.data + "/" + tree.Box + "/assets/same.pdf"
	m.annotationSources = map[string]string{"assets/same.pdf": strings.TrimPrefix(selected, m.read.binding.scope.commit.r.data+"/")}
	got := htmlPayload(t, m, tree, false)
	if string(got["assets/same.pdf"]) != "notebook PDF" || got["stage/build/export/index.js"] != nil || got["emojis/face.png"] != nil {
		t.Fatal("asset-only mode changed")
	}
}
func TestHTMLResourcesOptionalIconsAndEmojiRules(t *testing.T) {
	for _, icon := range []string{"", "ant", "material", "missing-custom"} {
		t.Run(icon, func(t *testing.T) {
			m, tree, work, appearance, _ := htmlResourceFixture(t)
			if icon == "ant" {
				archiveWrite(t, appearance, "icons/ant/icon.js", []byte("ant icon"))
			}
			m.htmlResources = captureHTMLResourceSnapshot(work, appearance, "custom", icon, true)
			got := htmlPayload(t, m, tree, true)
			if icon == "" && (got["appearance/icons/custom/icon.js"] != nil || got["appearance/icons/material/icon.js"] != nil) {
				t.Fatal("empty icon copied fallback")
			}
			if icon == "ant" && (string(got["appearance/icons/ant/icon.js"]) != "ant icon" || got["appearance/icons/material/icon.js"] != nil) {
				t.Fatal("built-in icon fallback changed")
			}
			if (icon == "material" || icon == "missing-custom") && string(got["appearance/icons/material/icon.js"]) != "fallback icon" {
				t.Fatal("material fallback missing")
			}
		})
	}
	m, tree, _, _, _ := htmlResourceFixture(t)
	m.htmlResources.nativeEmoji = map[string]bool{"😀": true}
	tree.Root.SetIALAttr("icon", "😀")
	paths, err := m.htmlEmojiPaths(tree)
	if err != nil || !reflect.DeepEqual(paths, []string{"emojis/face.png"}) {
		t.Fatal(paths, err)
	}
	bad := &ast.Node{Type: ast.NodeEmojiImg, Tokens: []byte("<img src=\"/emojis/broken")}
	tree.Root.FirstChild.AppendChild(bad)
	if _, err = m.htmlEmojiPaths(tree); err == nil {
		t.Fatal("malformed emoji accepted")
	}
	bad.Tokens = []byte("<img src=\"/emojis/%2e%2e/private\" />")
	if _, err = m.htmlEmojiPaths(tree); err == nil {
		t.Fatal("encoded traversal accepted")
	}
	// Chinese theme names and percent-encoded emoji URLs use native ZIP filenames.
	unicodeM, unicodeTree, work, appearance, _ := htmlResourceFixture(t)
	if err := os.Rename(filepath.Join(appearance, "themes/custom"), filepath.Join(appearance, "themes/中文主题")); err != nil {
		t.Fatal(err)
	}
	archiveWrite(t, unicodeM.read.binding.scope.commit.r.data, "emojis/表情.png", []byte("unicode emoji"))
	unicodeM.htmlResources = captureHTMLResourceSnapshot(work, appearance, "中文主题", "custom", true)
	unicodeTree.Root.SetIALAttr("icon", "表情.png")
	unicodeTree.Root.FirstChild.AppendChild(&ast.Node{Type: ast.NodeEmojiImg, Tokens: []byte("<img src=\"/emojis/%E8%A1%A8%E6%83%85.png\" />")})
	unicodeZIP := htmlPayload(t, unicodeM, unicodeTree, true)
	if string(unicodeZIP["appearance/themes/中文主题/theme.css"]) != "theme css" || string(unicodeZIP["emojis/表情.png"]) != "unicode emoji" {
		t.Fatal("Unicode resource filenames changed")
	}

}
func TestHTMLResourcesUnsafeRootsLinksAndOutputFailClosed(t *testing.T) {
	for _, kind := range []string{"theme-name", "icon-name", "work-replaced", "internal-link", "emoji-link", "output-link", "closed-owner", "canceled"} {
		t.Run(kind, func(t *testing.T) {
			m, tree, work, appearance, _ := htmlResourceFixture(t)
			o, err := m.newOutput("Failure", ".html")
			if err != nil {
				t.Fatal(err)
			}
			defer o.finish(false)
			switch kind {
			case "theme-name":
				m.htmlResources = captureHTMLResourceSnapshot(work, appearance, "../private", "", true)
			case "icon-name":
				m.htmlResources = captureHTMLResourceSnapshot(work, appearance, "custom", "../private", true)
			case "work-replaced":
				os.Rename(work, work+"-old")
				os.Mkdir(work, 0700)
			case "internal-link":
				target := archiveWrite(t, t.TempDir(), "external.js", []byte("external"))
				os.Symlink(target, filepath.Join(work, "stage/build/export/link.js"))
			case "emoji-link":
				file := filepath.Join(m.read.binding.scope.commit.r.data, "emojis/face.png")
				os.Remove(file)
				target := archiveWrite(t, t.TempDir(), "external.png", []byte("external"))
				os.Symlink(target, file)
			case "output-link":
				target := t.TempDir()
				o.payload.Symlink(target, "stage")
			case "closed-owner":
				owner := m.read.binding.scope.commit.r.owner
				owner.repoMu.Lock()
				owner.repoClosed = true
				owner.repoMu.Unlock()
				t.Cleanup(func() { owner.repoMu.Lock(); owner.repoClosed = false; owner.repoMu.Unlock() })
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				m.read.binding.scope.ctx = ctx
			}
			if err = m.copyHTMLResources(o, tree, true); err == nil {
				t.Fatal("unsafe copy succeeded")
			}
			entries, _ := os.ReadDir(filepath.Join(m.read.temp, "export"))
			if len(entries) != 0 {
				t.Fatal("partial ZIP published", entries)
			}
		})
	}
}
func TestHTMLResourcesForeignCandidateAndOutputRejected(t *testing.T) {
	m, tree, _, _, _ := htmlResourceFixture(t)
	o, err := m.newOutput("Owned", ".html")
	if err != nil {
		t.Fatal(err)
	}
	defer o.finish(false)
	clone := *tree
	if err = m.copyHTMLResources(o, &clone, true); err == nil {
		t.Fatal("foreign clone accepted")
	}
	other, err := captureMarkdownExport(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := other.newOutput("Foreign", ".html")
	if err != nil {
		t.Fatal(err)
	}
	defer foreign.finish(false)
	if err = m.copyHTMLResources(foreign, tree, true); err == nil {
		t.Fatal("foreign destination accepted")
	}

}

func TestHTMLResourcesLegacyFileExportResourceLayoutParity(t *testing.T) {
	m, tree, work, appearance, _ := htmlResourceFixture(t)
	oldWork, oldAppearance := util.WorkingDir, util.AppearancePath
	util.WorkingDir, util.AppearancePath = work, appearance
	t.Cleanup(func() { util.WorkingDir, util.AppearancePath = oldWork, oldAppearance })
	Conf.m.Lock()
	Conf.Appearance = conf.NewAppearance()
	Conf.Appearance.ThemeLight = "custom"
	Conf.Appearance.Icon = "custom"
	Conf.m.Unlock()
	legacy := t.TempDir()
	name, dom, node := ExportHTML(tree.ID, legacy, false, false, false, false)
	if name == "" || dom == "" || node == nil {
		t.Fatal("legacy file export failed")
	}
	got := htmlPayload(t, m, tree, true)
	for _, relative := range []string{"stage/build/export/index.js", "stage/protyle/runtime.js", "stage/protyle/fonts/font.woff", "appearance/themes/custom/theme.css", "appearance/themes/custom/.theme", "appearance/themes/custom/images/bg.png", "appearance/icons/custom/icon.js", "appearance/icons/material/icon.js", "emojis/face.png", "assets/same.pdf"} {
		original, err := os.ReadFile(filepath.Join(legacy, filepath.FromSlash(relative)))
		if err != nil {
			t.Fatal(relative, err)
		}
		if !bytes.Equal(original, got[relative]) {
			t.Fatal("legacy resource bytes/layout differ", relative)
		}
	}
}
