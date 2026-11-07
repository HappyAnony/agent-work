// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"io/fs"
	"maps"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"wiki-kernel/internal/indexrepair"
	"wiki-kernel/util"
)

type htmlResourceRoot struct {
	path string
	info os.FileInfo
	err  error
}
type htmlResourceSnapshot struct {
	work, theme, icon, fallback htmlResourceRoot
	themeName, iconName         string
	bound                       bool
	nativeEmoji                 map[string]bool
}
type htmlExportResourceIO struct {
	rootReadDir func(context.Context, indexrepair.Source, string) ([]fs.DirEntry, error)
}

func htmlResourceName(name string) bool {
	return name != "" && name != "." && name != ".." && !strings.ContainsAny(name, "/\\")
}

// Explicit device-root aliases are resolved once at capture. Internal links
// remain subject to Source.Check; aliases are not a general tree-follow grant.
func captureHTMLResourceRoot(p string) (ret htmlResourceRoot) {
	if !filepath.IsAbs(p) {
		ret.err = indexrepair.ErrSourcePath
		return
	}
	ret.path, ret.err = filepath.EvalSymlinks(p)
	if ret.err != nil {
		return
	}
	ret.info, ret.err = os.Lstat(ret.path)
	if ret.err == nil && (!ret.info.IsDir() || ret.info.Mode()&os.ModeSymlink != 0) {
		ret.err = indexrepair.ErrSourcePath
	}
	return
}
func captureHTMLResourceSnapshot(work, appearance, theme, icon string, bound bool) (s htmlResourceSnapshot) {
	s.bound = bound
	s.themeName = theme
	s.iconName = icon
	s.nativeEmoji = maps.Clone(util.NativeEmojiChars)
	s.work = captureHTMLResourceRoot(work)
	base := captureHTMLResourceRoot(appearance)
	invalid := func() htmlResourceRoot { return htmlResourceRoot{err: indexrepair.ErrSourcePath} }
	if !bound || !htmlResourceName(theme) {
		s.theme = invalid()
	} else if base.err != nil {
		s.theme = base
	} else {
		s.theme = captureHTMLResourceRoot(filepath.Join(base.path, "themes", theme))
	}
	if icon != "" {
		if !htmlResourceName(icon) {
			s.icon = invalid()
		} else if base.err != nil {
			s.icon = base
		} else {
			s.icon = captureHTMLResourceRoot(filepath.Join(base.path, "icons", icon))
		}
	}
	if icon != "" && icon != "ant" && icon != "material" {
		if base.err != nil {
			s.fallback = base
		} else {
			s.fallback = captureHTMLResourceRoot(filepath.Join(base.path, "icons", "material"))
		}
	}
	return
}
func (c htmlResourceRoot) open() (*os.Root, error) {
	if c.err != nil {
		return nil, c.err
	}
	if c.info == nil {
		return nil, errors.New("html_resource_root_unbound")
	}
	root, err := openArchivePublicationSource(c.path)
	if err != nil {
		return nil, err
	}
	info, err := root.Stat(".")
	if err == nil && !os.SameFile(info, c.info) {
		err = errors.New("html_resource_root_changed")
	}
	if err != nil {
		return nil, errors.Join(err, root.Close())
	}
	return root, nil
}
func (m *markdownExportContext) copyHTMLDeviceResource(target indexrepair.Source, c htmlResourceRoot, from, to string, optional bool) (err error) {
	return m.copyHTMLDeviceResourceWithIO(target, c, from, to, optional, htmlExportResourceIO{})
}
func (m *markdownExportContext) copyHTMLDeviceResourceWithIO(target indexrepair.Source, c htmlResourceRoot, from, to string, optional bool, op htmlExportResourceIO) (err error) {
	if err = m.guard(); err != nil {
		return err
	}
	root, err := c.open()
	if optional && os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, root.Close()) }()
	source := indexrepair.Source{Data: root}
	copier := &markdownExportResources{m: m, source: source, target: target}
	if from != "." {
		if optional {
			if err = copier.source.Check(from, false); os.IsNotExist(err) {
				return nil
			} else if err != nil {
				return err
			}
		}
		return copier.copyPath(from, to)
	}
	before, err := os.Lstat(c.path)
	if err != nil {
		return err
	}
	if !before.IsDir() || before.Mode()&fs.ModeSymlink != 0 || !os.SameFile(before, c.info) {
		return indexrepair.ErrSourceChanged
	}
	readDir := op.rootReadDir
	if readDir == nil {
		readDir = func(ctx context.Context, source indexrepair.Source, name string) ([]fs.DirEntry, error) {
			return source.ReadDir(ctx, name)
		}
	}
	entries, err := readDir(m.read.binding.scope.ctx, source, ".")
	if err != nil {
		return err
	}
	if err = m.guard(); err != nil {
		return err
	}
	after, err := os.Lstat(c.path)
	if err != nil {
		return err
	}
	if !after.IsDir() || after.Mode()&fs.ModeSymlink != 0 || !os.SameFile(after, before) || after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) {
		return indexrepair.ErrSourceChanged
	}
	if err = target.Check(to, true); err != nil {
		return err
	}
	if err = target.Data.MkdirAll(to, 0755); err != nil {
		return err
	}
	for _, entry := range entries {
		if err = copier.copyPath(entry.Name(), path.Join(to, entry.Name())); err != nil {
			return err
		}
	}
	if err = target.Data.Chtimes(to, c.info.ModTime(), c.info.ModTime()); err != nil {
		return err
	}
	return m.guard()
}
func (m *markdownExportContext) htmlEmojiPaths(tree *parse.Tree) (result []string, err error) {
	seen := map[string]bool{}
	add := func(value string) error {
		if strings.Contains(value, "://") || strings.HasPrefix(value, "data:") {
			return nil
		}
		decoded, e := url.PathUnescape(value)
		if e != nil {
			return e
		}
		relative := strings.TrimPrefix(decoded, "/")
		if !strings.HasPrefix(relative, "emojis/") || !fs.ValidPath(relative) || strings.Contains(relative, "\\") {
			return indexrepair.ErrSourcePath
		}
		if !seen[relative] {
			result = append(result, relative)
			seen[relative] = true
		}
		return nil
	}
	if icon := tree.Root.IALAttr("icon"); icon != "" && !strings.Contains(icon, "://") && !strings.HasPrefix(icon, "api/icon/") && !m.htmlResources.nativeEmoji[icon] {
		if err = add("/emojis/" + icon); err != nil {
			return nil, err
		}
	}
	ast.Walk(tree.Root, func(n *ast.Node, enter bool) ast.WalkStatus {
		if err = m.guard(); err != nil {
			return ast.WalkStop
		}
		if !enter || n.Type != ast.NodeEmojiImg {
			return ast.WalkContinue
		}
		text := string(n.Tokens)
		start := strings.Index(text, "src=\"")
		if start < 0 {
			return ast.WalkContinue
		}
		text = text[start+5:]
		end := strings.IndexByte(text, '"')
		if end < 0 {
			err = errors.New("html_emoji_source_invalid")
			return ast.WalkStop
		}
		err = add(text[:end])
		if err != nil {
			return ast.WalkStop
		}
		return ast.WalkContinue
	})
	if err != nil {
		return nil, err
	}
	return result, m.guard()
}

// Only the admitted export's private payload is a destination. File controllers
// can publish it later; this resource port never publishes partial output.
func (m *markdownExportContext) copyHTMLResources(o *archiveExportOutput, tree *parse.Tree, static bool) (err error) {
	if err = m.checkExportRenderTree(tree); err != nil {
		return err
	}
	if o == nil || o.e != m.read || m.read.output != o {
		return errors.New("html_resource_output_mismatch")
	}
	r, err := m.openResources(o)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, r.close()) }()
	assets, err := getAssetsLinkDestsWithAV(tree.Root, m.read.readExportView)
	if err != nil {
		return err
	}
	for _, asset := range assets {
		if err = r.copyAsset(asset, ""); err != nil {
			return err
		}
	}
	if !static {
		return m.guard()
	}
	s := m.htmlResources
	if !s.bound || !htmlResourceName(s.themeName) || s.iconName != "" && !htmlResourceName(s.iconName) {
		return errors.New("html_resource_preferences_invalid")
	}
	for _, name := range []string{"stage/build/export", "stage/protyle"} {
		if err = m.copyHTMLDeviceResource(r.target, s.work, name, name, false); err != nil {
			return err
		}
	}
	if err = m.copyHTMLDeviceResource(r.target, s.theme, ".", "appearance/themes/"+s.themeName, false); err != nil {
		return err
	}
	if s.iconName != "" && s.iconName != "ant" && s.iconName != "material" {
		if err = m.copyHTMLDeviceResource(r.target, s.fallback, "icon.js", "appearance/icons/material/icon.js", true); err != nil {
			return err
		}
	}
	if s.iconName != "" {
		if err = m.copyHTMLDeviceResource(r.target, s.icon, "icon.js", "appearance/icons/"+s.iconName+"/icon.js", true); err != nil {
			return err
		}
	}
	emojis, err := m.htmlEmojiPaths(tree)
	if err != nil {
		return err
	}
	for _, name := range emojis {
		if err = r.source.Check(name, false); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return err
		}
		if err = r.copyPath(name, name); err != nil {
			return err
		}
	}
	return m.guard()
}
