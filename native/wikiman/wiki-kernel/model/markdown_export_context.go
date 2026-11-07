// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/88250/gulu"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"path/filepath"
	"wiki-kernel/conf"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/nativeparser"
	"wiki-kernel/util"
)

// Preparation borrows the existing owned, read-only document/reference ports.
// Deep legacy rendering is separate from the owned resource/conversion/package phase.
type markdownExportContext struct {
	pdfWatermark          pdfWatermarkSnapshot
	pdfFonts              pdfFontInventory
	mergeSort, mergeMax   int
	mergeBound            bool
	htmlResources         htmlResourceSnapshot
	htmlIcon              string
	htmlLocalAnchorPrefix string
	cloudAssetsBase       string
	avScope               *commitAVContext
	read                  *archiveExportContext
	focusTrees            map[*parse.Tree]exportFocusBinding
	annotationFiles       map[string]map[string]json.RawMessage
	annotationSources     map[string]string
	themeRoot             string
	themeName             string
	themeDark             bool
	themeBound            bool
	themeStyles           map[string]string
	settings              conf.Export
	pandocBin             string
	pandocGuard           func() error
	pandocEnabled         bool
	anchorMax             int
	headingMode           int
	searchLimit           int
	searchCaseSensitive   bool
	parserOptions         nativeparser.EditorOptions
}

func captureMarkdownExport(ctx context.Context) (*markdownExportContext, error) {
	read, err := captureArchiveExport(ctx)
	if err != nil {
		return nil, err
	}
	m := &markdownExportContext{read: read, pandocBin: util.PandocBinPath, pandocEnabled: util.Container == util.ContainerStd, themeRoot: util.ThemesPath, htmlLocalAnchorPrefix: "http://" + util.LocalHost + ":" + util.ServerPort + "/#"}
	err = read.binding.scope.commit.r.owner.WithSettingSnapshot(ctx, func(data conf.AppData) error {
		if data.Export == nil {
			return errors.New("markdown_export_preferences_unbound")
		}
		if data.Appearance != nil {
			m.themeBound = true
			m.htmlIcon = data.Appearance.Icon
			m.themeDark = data.Appearance.Mode == 1
			m.themeName = data.Appearance.ThemeLight
			if m.themeDark {
				m.themeName = data.Appearance.ThemeDark
			}
		}
		if data.IsSubscriber() {
			m.cloudAssetsBase = util.GetCloudAssetsServerForRegion(data.CloudRegion) + data.User.UserId + "/"
		}
		if data.FileTree != nil {
			m.mergeSort = data.FileTree.Sort
			m.mergeMax = data.FileTree.MaxListCount
			m.mergeBound = true
		}
		m.settings = *data.Export
		if data.Editor == nil {
			return errors.New("markdown_export_editor_unbound")
		}
		m.anchorMax = data.Editor.BlockRefDynamicAnchorTextMaxLen
		m.headingMode = data.Editor.HeadingEmbedMode
		if data.Search == nil {
			return errors.New("markdown_export_search_unbound")
		}
		m.searchLimit = data.Search.Limit
		m.searchCaseSensitive = data.Search.CaseSensitive
		var err error
		m.parserOptions, err = data.ParserOptions(read.binding.scope.commit.r.owner.customEmojis.Snapshot())
		return err
	})
	if err != nil {
		return nil, err
	}
	m.pdfFonts = capturePDFFontInventory(filepath.Join(util.HomeDir, ".config", "siyuan", "fonts"))
	m.pdfWatermark = capturePDFWatermarkSnapshot(m.settings, util.AppearancePath)
	m.htmlResources = captureHTMLResourceSnapshot(util.WorkingDir, util.AppearancePath, m.themeName, m.htmlIcon, m.themeBound)
	return m, m.guard()
}
func (m *markdownExportContext) guard() error {
	if m == nil || m.read == nil {
		return errors.New("markdown_export_context_unbound")
	}
	return m.read.guard()
}
func (m *markdownExportContext) selectDocuments(ids []string) ([]archiveExportSelection, error) {
	if err := m.guard(); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, errors.New("markdown_export_selection_empty")
	}
	out := make([]archiveExportSelection, 0, len(ids))
	for _, id := range ids {
		if !ast.IsNodeIDPattern(id) {
			return nil, ErrBlockNotFound
		}
		selected, err := m.read.single(id)
		if err != nil {
			return nil, err
		}
		out = append(out, selected)
	}
	return out, m.guard()
}
func (m *markdownExportContext) prepare(selections []archiveExportSelection) (ids []string, trees map[string]*parse.Tree, paths []string, err error) {
	if err = m.guard(); err != nil {
		return
	}
	trees = map[string]*parse.Tree{}
	cache := map[string]*parse.Tree{}
	for _, selection := range selections {
		for i, p := range selection.paths {
			var loaded map[string]*parse.Tree
			loaded, err = m.read.selectedTrees(selection.box, []string{p})
			if err != nil {
				return
			}
			tree := loaded[util.GetTreeID(p)]
			if tree == nil {
				err = errors.New("markdown_export_tree_unbound")
				return
			}
			if err = m.read.references(tree, &ids, trees, cache); err != nil {
				return
			}
			util.PushEndlessProgress(m.read.binding.scope.commit.r.language(65) + " " + fmt.Sprintf(m.read.binding.scope.commit.r.language(70), fmt.Sprintf("%d/%d %s", i+1, len(selection.paths), tree.Root.IALAttr("title"))))
		}
	}
	for _, tree := range trees {
		paths = append(paths, tree.Path)
	}
	paths = gulu.Str.RemoveDuplicatedElem(paths)
	err = m.guard()
	return
}
func (m *markdownExportContext) rootChildrenRefCounts(id string) (map[string]int, error) {
	if err := m.guard(); err != nil {
		return nil, err
	}
	counts, err := (indexdb.References{DB: m.read.binding.scope.commit.r.main.DB}).RootChildrenCounts(m.read.binding.scope.ctx, id)
	if err != nil {
		return nil, err
	}
	return counts, m.guard()
}
