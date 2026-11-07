//go:build fts5

// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"bytes"
	"context"
	"github.com/88250/lute/ast"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"wiki-kernel/conf"
	"wiki-kernel/util"
)

func TestMarkdownExportEntryFocusedOptionsYFMAndSourceIsolation(t *testing.T) {
	m, box, ids := focusFixture(t)
	m.settings.AddTitle = true
	file := filepath.Join(util.DataDir, box, ids[0]+".docj")
	before, _ := os.ReadFile(file)
	_, md, err := m.content(ids[2], MarkdownContentOptions{YFM: true, AdjustHeadingLevel: true}, false)
	if err != nil || !strings.HasPrefix(md, "---") || !strings.Contains(md, "# Prepared Title") || !strings.Contains(md, "## Focused heading") || !strings.Contains(md, "#### Nested heading") || strings.Contains(md, "Before") || strings.Contains(md, "Next peer") {
		t.Fatal(md, err)
	}
	// Repeated uses begin from independent full first-read bytes, not the prior projection.
	_, whole, err := m.content(ids[0], MarkdownContentOptions{}, false)
	if err != nil || !strings.Contains(whole, "Before") || !strings.Contains(whole, "Next peer") {
		t.Fatal(whole, err)
	}
	after, _ := os.ReadFile(file)
	if !bytes.Equal(before, after) {
		t.Fatal("source changed")
	}
}
func TestMarkdownExportEntryCloudSubscriptionRegionSnapshot(t *testing.T) {
	for _, item := range []struct {
		name       string
		user       *conf.User
		subscriber bool
	}{
		{"none", nil, false}, {"lifetime", &conf.User{UserId: "synthetic", UserSiYuanProExpireTime: -1}, true}, {"positive", &conf.User{UserId: "synthetic", UserSiYuanProExpireTime: 1}, true}, {"disabled", &conf.User{UserId: "synthetic", UserSiYuanProExpireTime: 1, UserSiYuanSubscriptionStatus: 1}, false}, {"free", &conf.User{UserId: "synthetic"}, false},
	} {
		t.Run(item.name, func(t *testing.T) {
			_, _ = markdownFixture(t)
			Conf.m.Lock()
			Conf.User = item.user
			Conf.CloudRegion = 1
			Conf.m.Unlock()
			m, err := captureMarkdownExport(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			expected := ""
			if item.subscriber {
				expected = util.GetCloudAssetsServerForRegion(1) + "synthetic/"
			}
			if m.cloudAssetsBase != expected {
				t.Fatal("subscription/region differs")
			}
			Conf.SetUser(nil)
			oldRegion := util.CurrentCloudRegion
			util.CurrentCloudRegion = 0
			t.Cleanup(func() { util.CurrentCloudRegion = oldRegion })
			if m.cloudAssetsBase != expected {
				t.Fatal("snapshot followed globals")
			}
			if util.GetCloudAssetsServerForRegion(0) != util.GetCloudAssetsServer() || util.GetCloudAssetsServerForRegion(0) == util.GetCloudAssetsServerForRegion(1) {
				t.Fatal("legacy cloud endpoint rule changed")
			}
		})
	}
}
func TestMarkdownExportEntryStandardAssetsAndReferenceDefinitions(t *testing.T) {
	m, box := markdownFixture(t)
	m.settings.AddTitle = false
	m.settings.BlockRefMode = 4
	root, target, paragraph := ast.NewNodeID(), ast.NewNodeID(), ast.NewNodeID()
	markdownDoc(t, m, box, "/"+target+".docj", embedTextNode(paragraph, "External definition"))
	tree := markdownDoc(t, m, box, "/"+root+".docj", footParagraph(ast.NewNodeID(), footRef(paragraph, "reference"), map[string]interface{}{"Type": "NodeImage", "Children": []interface{}{map[string]interface{}{"Type": "NodeBang"}, map[string]interface{}{"Type": "NodeOpenBracket"}, map[string]interface{}{"Type": "NodeLinkText", "Data": "asset"}, map[string]interface{}{"Type": "NodeCloseBracket"}, map[string]interface{}{"Type": "NodeOpenParen"}, map[string]interface{}{"Type": "NodeLinkDest", "Data": "assets/my file.png"}, map[string]interface{}{"Type": "NodeCloseParen"}}}))
	m.cloudAssetsBase = util.GetCloudAssetsServerForRegion(1) + "synthetic/"
	_, md, err := m.content(root, MarkdownContentOptions{AssetsDestSpace2Underscore: true, YFM: true}, true)
	if err != nil || strings.HasPrefix(md, "---") || !strings.Contains(md, "my_file.png") || !strings.Contains(md, "External definition") || !strings.Contains(md, paragraph) {
		t.Fatal(md, err)
	}
	if tree.Root.FirstChild == nil {
		t.Fatal("fixture")
	}
	// Nonstandard export does not add the subscription URL prefix.
	m.settings.BlockRefMode = 0
	_, local, err := m.content(root, MarkdownContentOptions{}, false)
	if err != nil || strings.Contains(local, "synthetic/") {
		t.Fatal(local, err)
	}
}
func TestMarkdownExportEntryActualEntrypointsCaptureAndErrors(t *testing.T) {
	m, box := markdownFixture(t)
	id := ast.NewNodeID()
	markdownDoc(t, m, box, "/"+id+".docj")
	h, md, err := ExportMarkdownContentContext(context.Background(), id, MarkdownContentOptions{})
	if err != nil || h == "" || !strings.Contains(md, "Prepared content") {
		t.Fatal(h, md, err)
	}
	if std, err := ExportStdMarkdownContext(context.Background(), id, MarkdownContentOptions{}); err != nil || !strings.Contains(std, "Prepared content") {
		t.Fatal(std, err)
	}
	for _, ctx := range []context.Context{nil, func() context.Context { ctx, cancel := context.WithCancel(context.Background()); cancel(); return ctx }()} {
		if h, md, err = ExportMarkdownContentContext(ctx, id, MarkdownContentOptions{}); err == nil || h != "" || md != "" {
			t.Fatal(h, md, err)
		}
		if md, err = ExportStdMarkdownContext(ctx, id, MarkdownContentOptions{}); err == nil || md != "" {
			t.Fatal(md, err)
		}
	}
	if h, md, err = ExportMarkdownContentContext(context.Background(), ast.NewNodeID(), MarkdownContentOptions{}); err == nil || h != "" || md != "" {
		t.Fatal(h, md, err)
	}
	// Optional mode defaults come from the captured preferences, not later globals.
	Conf.m.Lock()
	Conf.Export = nil
	Conf.m.Unlock()
	if _, _, err = ExportMarkdownContentContext(context.Background(), id, MarkdownContentOptions{}); err == nil {
		t.Fatal("missing preferences accepted")
	}
}
func TestMarkdownExportEntryAssemblesAVAndThemeAndFailsClosed(t *testing.T) {
	m, a, tree, file := markdownAVFixture(t, "table")
	m.settings.AddTitle = false
	m.themeRoot = t.TempDir()
	m.themeBound = true
	m.themeName = "daylight"
	archiveWrite(t, m.themeRoot, "daylight/theme.css", []byte(`:root { --ink: green; }`))
	_, md, err := m.content(tree.ID, MarkdownContentOptions{FillCSSVar: true}, false)
	if err != nil || !strings.Contains(md, "gamma") || strings.Contains(md, "beta") {
		t.Fatal(md, err)
	}
	// A new request sees corrupt existing dependencies and returns neither hPath nor body.
	if err = os.WriteFile(file, []byte("bad AV"), 0600); err != nil {
		t.Fatal(err)
	}
	h, body, err := ExportMarkdownContentContext(context.Background(), tree.ID, MarkdownContentOptions{})
	if err == nil || h != "" || body != "" || a.ID == "" {
		t.Fatal(h, body, err)
	}
}
