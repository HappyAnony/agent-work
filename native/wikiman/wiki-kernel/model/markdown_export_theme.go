// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"errors"
	"github.com/88250/lute/parse"
	"path/filepath"
	"strings"
	"wiki-kernel/internal/indexrepair"
)

// Theme resources are device resources, not wiki source. Selection and root
// belong to this export; a later global configuration change cannot retarget it.
// Only parsed values are retained after first use; no borrowed root stays open.
func (m *markdownExportContext) loadThemeStyles() (styles map[string]string, err error) {
	if err = m.guard(); err != nil {
		return nil, err
	}
	if !m.themeBound {
		return nil, errors.New("export_theme_preferences_unbound")
	}
	if m.themeStyles != nil {
		return m.themeStyles, nil
	}
	if !filepath.IsAbs(m.themeRoot) || m.themeName == "" || m.themeName == "." || m.themeName == ".." || strings.ContainsAny(m.themeName, "/\\") {
		return nil, errors.New("export_theme_path_invalid")
	}
	root, err := openArchivePublicationSource(m.themeRoot)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, root.Close()) }()
	source := indexrepair.Source{Data: root}
	base := "daylight"
	if m.themeDark {
		base = "midnight"
	}
	content := ""
	if m.themeName != base {
		raw, e := source.Read(m.read.binding.scope.ctx, base+"/theme.css")
		if e != nil && !indexrepair.IsSourceNotExist(e) {
			return nil, e
		}
		if e == nil {
			content = string(raw) + "\n"
		}
	}
	raw, err := source.Read(m.read.binding.scope.ctx, m.themeName+"/theme.css")
	if indexrepair.IsSourceNotExist(err) {
		styles = map[string]string{}
		err = nil
	} else if err != nil {
		return nil, err
	} else {
		styles = parseThemeStyleVars(content+string(raw), m.themeDark)
	}
	if err = m.guard(); err != nil {
		return nil, err
	}
	m.themeStyles = styles
	return styles, nil
}
func (m *markdownExportContext) fillThemeStyle(tree *parse.Tree) error {
	if err := m.checkExportRenderTree(tree); err != nil {
		return err
	}
	styles, err := m.loadThemeStyles()
	if err != nil {
		return err
	}
	fillThemeStyleValues(tree, styles)
	return m.guard()
}
