//go:build fts5

// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"bytes"
	"context"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"wiki-kernel/conf"
	"wiki-kernel/util"
)

func themeFixture(t *testing.T) (*markdownExportContext, *parse.Tree, string) {
	t.Helper()
	m, box := markdownFixture(t)
	m.themeRoot, m.themeName, m.themeBound = t.TempDir(), "custom", true
	id := ast.NewNodeID()
	tree := markdownDoc(t, m, box, "/"+id+".docj")
	tree.Root.FirstChild.KramdownIAL = [][]string{{"style", "color: var(--ink); background-color: var(--paper);"}}
	return m, tree, filepath.Join(m.read.binding.scope.commit.r.data, box, id+".docj")
}
func TestMarkdownExportThemeOriginalMergeModeAndNestedRules(t *testing.T) {
	for _, dark := range []bool{false, true} {
		t.Run(map[bool]string{false: "light", true: "dark"}[dark], func(t *testing.T) {
			m, tree, file := themeFixture(t)
			m.themeDark = dark
			base, mode := "daylight", "light"
			if dark {
				base, mode = "midnight", "dark"
			}
			archiveWrite(t, m.themeRoot, base+"/theme.css", []byte(`:root { --paper: white; --ink: black; --nested: var(--ink); }`))
			archiveWrite(t, m.themeRoot, "custom/theme.css", []byte(`html[data-theme-mode="`+mode+`"] { --ink: blue; } :root { --ink: red; --a: var(--b); --b: var(--a); }`))
			before, _ := os.ReadFile(file)
			styles, err := m.loadThemeStyles()
			if err != nil {
				t.Fatal(err)
			}
			if resolveNestedCSSVar(styles, "--ink") != "blue" || resolveNestedCSSVar(styles, "--paper") != "white" || resolveNestedCSSVar(styles, "--nested") != "blue" || resolveNestedCSSVar(styles, "--a") != "" {
				t.Fatal(styles)
			}
			// The legacy reader and the owned reader share the original parser/priority rules.
			old := util.ThemesPath
			util.ThemesPath = m.themeRoot
			defer func() { util.ThemesPath = old }()
			legacy := getThemeStyleVar("custom", dark)
			for key, value := range legacy {
				if styles[key] != value {
					t.Fatal(key, value, styles[key])
				}
			}
			if err = m.fillThemeStyle(tree); err != nil {
				t.Fatal(err)
			}
			style := tree.Root.FirstChild.IALAttr("style")
			if !strings.Contains(style, "blue") || !strings.Contains(style, "white") {
				t.Fatal(style)
			}
			after, _ := os.ReadFile(file)
			if !bytes.Equal(before, after) {
				t.Fatal("source changed")
			}
		})
	}
}
func TestMarkdownExportThemeCapturedSelectionAndFirstRead(t *testing.T) {
	m, tree, _ := themeFixture(t)
	old := util.ThemesPath
	util.ThemesPath = m.themeRoot
	defer func() { util.ThemesPath = old }()
	Conf.m.Lock()
	Conf.Appearance = conf.NewAppearance()
	Conf.Appearance.ThemeLight = "custom"
	Conf.m.Unlock()
	captured, err := captureMarkdownExport(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// Borrow this fixture's already registered read-only document, not global resources.
	captured.read = m.read
	archiveWrite(t, m.themeRoot, "custom/theme.css", []byte(`:root { --ink: teal; --paper: white; }`))
	Conf.m.Lock()
	Conf.Appearance.ThemeLight = "elsewhere"
	Conf.Appearance.Mode = 1
	Conf.m.Unlock()
	util.ThemesPath = t.TempDir()
	if err = captured.fillThemeStyle(tree); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(tree.Root.FirstChild.IALAttr("style"), "teal") {
		t.Fatal("selection followed globals")
	}
	archiveWrite(t, m.themeRoot, "custom/theme.css", []byte(`:root { --ink: orange; }`))
	styles, err := captured.loadThemeStyles()
	if err != nil || resolveNestedCSSVar(styles, "--ink") != "teal" {
		t.Fatal(styles, err)
	}
}
func TestMarkdownExportThemeMissingAndUnsafeResources(t *testing.T) {
	for _, kind := range []string{"missing-default", "missing-selected", "traversal", "linked-file", "linked-theme", "linked-root", "unbound", "closed", "canceled"} {
		t.Run(kind, func(t *testing.T) {
			m, tree, _ := themeFixture(t)
			archiveWrite(t, m.themeRoot, "custom/theme.css", []byte(`:root { --ink: green; }`))
			switch kind {
			case "missing-selected":
				m.themeName = "absent"
			case "traversal":
				m.themeName = "../custom"
			case "linked-file":
				p := filepath.Join(m.themeRoot, "custom/theme.css")
				os.Remove(p)
				if err := os.Symlink(archiveWrite(t, t.TempDir(), "outside.css", []byte(`:root { --ink: red; }`)), p); err != nil {
					t.Fatal(err)
				}
			case "linked-theme":
				m.themeName = "linked"
				if err := os.Symlink(filepath.Join(m.themeRoot, "custom"), filepath.Join(m.themeRoot, "linked")); err != nil {
					t.Fatal(err)
				}
			case "linked-root":
				p := filepath.Join(t.TempDir(), "linked")
				if err := os.Symlink(m.themeRoot, p); err != nil {
					t.Fatal(err)
				}
				m.themeRoot = p
			case "unbound":
				m.themeBound = false
			case "closed":
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
			before := tree.Root.FirstChild.IALAttr("style")
			err := m.fillThemeStyle(tree)
			if kind == "missing-default" {
				if err != nil || !strings.Contains(tree.Root.FirstChild.IALAttr("style"), "green") {
					t.Fatal(err)
				}
				return
			}
			if kind == "missing-selected" {
				if err != nil || tree.Root.FirstChild.IALAttr("style") != before {
					t.Fatal(err)
				}
				return
			}
			if err == nil || tree.Root.FirstChild.IALAttr("style") != before {
				t.Fatal("unsafe read mutated candidate", err)
			}
		})
	}
}
func TestMarkdownExportThemeOwnedRendererAndFailClosed(t *testing.T) {
	for _, bad := range []bool{false, true} {
		t.Run(map[bool]string{false: "render", true: "missing-port"}[bad], func(t *testing.T) {
			m, tree, _ := themeFixture(t)
			archiveWrite(t, m.themeRoot, "custom/theme.css", []byte(`:root { --ink: purple; --paper: white; }`))
			p := m.referenceRenderer()
			if bad {
				p.theme = nil
			}
			result := exportMarkdownContent0(tree.ID, tree, "", false, false, false, ".md", 0, 0, 0, "#", "#", "", "", false, false, nil, true, true, map[string]*parse.Tree{tree.ID: tree}, p)
			if bad {
				if p.check() == nil || result != "" {
					t.Fatal(result, p.check())
				}
				return
			}
			if p.check() != nil || result == "" || !strings.Contains(tree.Root.FirstChild.IALAttr("style"), "purple") {
				t.Fatal(result, p.check())
			}
		})
	}
	m, tree, _ := themeFixture(t)
	clone := *tree
	if err := m.fillThemeStyle(&clone); err == nil {
		t.Fatal("foreign tree accepted")
	}
	// Unknown variables keep the original var() fallback (cycles checked above).
	styles := map[string]string{"--x": "var(--y)", "--y": "var(--x)"}
	fillThemeStyleValues(tree, styles)
	if !strings.Contains(tree.Root.FirstChild.IALAttr("style"), "var(--ink)") {
		t.Fatal("missing fallback")
	}
}
