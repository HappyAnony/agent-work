// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"fmt"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/html"
	"github.com/88250/lute/parse"
	"io/fs"
	"os"
	"path"
	"strings"
	"wiki-kernel/av"
	"wiki-kernel/internal/indexrepair"
	"wiki-kernel/treenode"
	"wiki-kernel/util"
)

type archiveExportView struct {
	view *av.AttributeView
	raw  []byte
}

type archiveExportResourceIO struct {
	indexReadDir func(context.Context, indexrepair.Source, string) ([]fs.DirEntry, error)
	copyReadDir  func(context.Context, indexrepair.Source, string) ([]fs.DirEntry, error)
}
type archiveExportResourceEntry struct {
	name string
	info fs.FileInfo
}

func (e *archiveExportContext) readExportView(id string) (*av.AttributeView, error) {
	if err := e.guard(); err != nil {
		return nil, err
	}
	if id == "" {
		return nil, nil
	}
	if !ast.IsNodeIDPattern(id) {
		return nil, errors.New("invalid_export_reference_av_id")
	}
	if saved, ok := e.views[id]; ok {
		return saved.view, nil
	}
	raw, err := e.binding.scope.source("storage/av/" + id + ".json")
	if os.IsNotExist(err) {
		e.views[id] = archiveExportView{}
		return nil, e.guard()
	}
	if err != nil {
		return nil, err
	}
	view, err := av.DecodeAttributeView(raw)
	if err != nil {
		return nil, err
	}
	if view == nil || view.ID != id {
		return nil, errors.New("invalid_export_reference_av")
	}
	for _, kv := range view.KeyValues {
		if kv == nil || kv.Key == nil {
			return nil, errors.New("invalid_export_reference_av_key")
		}
		for _, value := range kv.Values {
			if value == nil {
				return nil, errors.New("invalid_export_reference_value")
			}
			for _, asset := range value.MAsset {
				if asset == nil {
					return nil, errors.New("invalid_export_asset_value")
				}
			}
		}
	}
	if err = e.guard(); err != nil {
		return nil, err
	}
	e.views[id] = archiveExportView{view: view, raw: raw}
	return view, nil
}

func (e *archiveExportContext) checkResourceManifest(source indexrepair.Source, entries []archiveExportResourceEntry) error {
	for _, entry := range entries {
		if err := e.guard(); err != nil {
			return err
		}
		if entry.name != "." {
			if err := source.Check(entry.name, false); err != nil {
				return err
			}
		}
		info, err := source.Data.Lstat(entry.name)
		if err != nil {
			return err
		}
		if info.Mode()&fs.ModeSymlink != 0 || info.IsDir() != entry.info.IsDir() || info.Mode().Type() != entry.info.Mode().Type() || !os.SameFile(info, entry.info) || info.Size() != entry.info.Size() || !info.ModTime().Equal(entry.info.ModTime()) {
			return indexrepair.ErrSourceChanged
		}
	}
	return e.guard()
}

func (e *archiveExportContext) resourceManifest(source indexrepair.Source, root string, readDir func(context.Context, indexrepair.Source, string) ([]fs.DirEntry, error), skip func(fs.DirEntry) bool, regularOnly bool) ([]archiveExportResourceEntry, error) {
	if readDir == nil {
		readDir = func(ctx context.Context, source indexrepair.Source, name string) ([]fs.DirEntry, error) {
			return source.ReadDir(ctx, name)
		}
	}
	if source.Data == nil {
		return nil, indexrepair.ErrSourcePath
	}
	if root != "." {
		if err := source.Check(root, false); err != nil {
			return nil, err
		}
	}
	rootInfo, err := source.Data.Lstat(root)
	if err != nil {
		return nil, err
	}
	if !rootInfo.IsDir() || rootInfo.Mode()&fs.ModeSymlink != 0 {
		return nil, indexrepair.ErrSourcePath
	}
	out := []archiveExportResourceEntry{{name: root, info: rootInfo}}
	var scan func(string) error
	scan = func(folder string) error {
		if err := e.guard(); err != nil {
			return err
		}
		entries, err := readDir(e.binding.scope.ctx, source, folder)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if skip != nil && skip(entry) {
				continue
			}
			if err = e.guard(); err != nil {
				return err
			}
			name := path.Join(folder, entry.Name())
			if err = source.Check(name, false); err != nil {
				return err
			}
			info, statErr := source.Data.Lstat(name)
			if statErr != nil {
				return statErr
			}
			if info.Mode()&fs.ModeSymlink != 0 || info.IsDir() != entry.IsDir() || info.Mode().Type() != entry.Type() {
				return indexrepair.ErrSourceChanged
			}
			if regularOnly && !info.IsDir() && !info.Mode().IsRegular() {
				return indexrepair.ErrSourcePath
			}
			out = append(out, archiveExportResourceEntry{name: name, info: info})
			if info.IsDir() {
				if err = scan(name); err != nil {
					return err
				}
			}
		}
		return e.guard()
	}
	if err = scan(root); err != nil {
		return nil, err
	}
	if err = e.checkResourceManifest(source, out); err != nil {
		return nil, err
	}
	return out, nil
}

// Preserve notebook traversal order, last notebook match, then global-assets
// precedence. Discovery does not repair notebooks or follow source links.
func (e *archiveExportContext) assetIndex(source indexrepair.Source) (map[string]string, error) {
	return e.assetIndexWithIO(source, archiveExportResourceIO{})
}
func (e *archiveExportContext) assetIndexWithIO(source indexrepair.Source, op archiveExportResourceIO) (map[string]string, error) {
	type tree struct {
		entries   []archiveExportResourceEntry
		assetRoot string
	}
	var trees []tree
	var all []archiveExportResourceEntry
	scan := func(root, assetRoot string) error {
		entries, err := e.resourceManifest(source, root, op.indexReadDir, func(entry fs.DirEntry) bool { return isSkipFile(entry.Name()) }, false)
		if err != nil {
			return err
		}
		trees = append(trees, tree{entries: entries, assetRoot: assetRoot})
		all = append(all, entries...)
		return nil
	}
	for _, box := range e.assetBoxes {
		if err := scan(box, ""); err != nil {
			return nil, err
		}
	}
	if err := source.Check("assets", false); err == nil {
		if err = scan("assets", "assets"); err != nil {
			return nil, err
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	if err := e.checkResourceManifest(source, all); err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, current := range trees {
		roots := map[string]string{current.entries[0].name: current.assetRoot}
		for _, entry := range current.entries[1:] {
			assetRoot := roots[path.Dir(entry.name)]
			if assetRoot == "" && entry.info.IsDir() && path.Base(entry.name) == "assets" {
				assetRoot = entry.name
			}
			roots[entry.name] = assetRoot
			if assetRoot == "" || entry.name == assetRoot {
				continue
			}
			key := "assets/" + strings.TrimPrefix(entry.name, assetRoot+"/")
			if entry.info.IsDir() {
				key += "/"
			}
			out[key] = entry.name
		}
	}
	return out, e.guard()
}
func normalizeExportAsset(asset string) (string, error) {
	asset = string(html.DecodeDestination([]byte(asset)))
	if i := strings.LastIndex(asset, "?"); i >= 0 {
		asset = asset[:i]
	}
	relative := strings.TrimSuffix(asset, "/")
	if !strings.HasPrefix(asset, "assets/") || !fs.ValidPath(relative) || strings.Contains(asset, "\\") {
		return "", indexrepair.ErrSourcePath
	}
	return asset, nil
}
func (e *archiveExportContext) exportResources(folder string, trees map[string]*parse.Tree) (repairIOErr error) {
	return e.exportResourcesWithIO(folder, trees, archiveExportResourceIO{})
}
func (e *archiveExportContext) exportResourcesWithIO(folder string, trees map[string]*parse.Tree, op archiveExportResourceIO) (repairIOErr error) {
	if err := e.guard(); err != nil {
		return err
	}
	io, closeIO, err := e.binding.scope.commit.r.openRepairIOChecked(e.binding.scope.ctx, false)
	if err != nil {
		return err
	}
	defer func() { repairIOErr = joinRepairCloseError(repairIOErr, closeIO()) }()
	index, err := e.assetIndexWithIO(io.Source, op)
	if err != nil {
		return err
	}
	root, closeRoot, err := e.openExportIO(folder)
	if err != nil {
		return err
	}
	defer func() { repairIOErr = joinRepairCloseError(repairIOErr, closeRoot()) }()
	target := indexrepair.Source{Data: root, Authorize: func(ctx context.Context) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		return e.guard()
	}}
	writeRaw := func(to string, raw []byte) error {
		before, err := target.Read(e.binding.scope.ctx, to)
		if err != nil && !indexrepair.IsSourceNotExist(err) {
			return err
		}
		return target.Rewrite(e.binding.scope.ctx, to, before, raw)
	}
	var copyPath func(string, string) error
	copyPath = func(from, to string) error {
		if err := e.guard(); err != nil {
			return err
		}
		if err := io.Source.Check(from, false); err != nil {
			return err
		}
		info, err := io.Source.Data.Lstat(from)
		if err != nil {
			return err
		}
		if info.IsDir() {
			entries, err := e.resourceManifest(io.Source, from, op.copyReadDir, nil, true)
			if err != nil {
				return err
			}
			for _, entry := range entries {
				if err = e.checkResourceManifest(io.Source, []archiveExportResourceEntry{entry}); err != nil {
					return err
				}
				dest := to
				if entry.name != from {
					dest = path.Join(to, strings.TrimPrefix(entry.name, from+"/"))
				}
				if entry.info.IsDir() {
					if err = target.Check(dest, true); err != nil {
						return err
					}
					if err = target.Data.MkdirAll(dest, 0755); err != nil {
						return err
					}
					continue
				}
				if err = e.copyExportFile(io.Source, target, entry.name, dest); err != nil {
					return err
				}
			}
			for i := len(entries) - 1; i >= 0; i-- {
				entry := entries[i]
				if !entry.info.IsDir() {
					continue
				}
				if err = e.checkResourceManifest(io.Source, []archiveExportResourceEntry{entry}); err != nil {
					return err
				}
				dest := to
				if entry.name != from {
					dest = path.Join(to, strings.TrimPrefix(entry.name, from+"/"))
				}
				if err = target.Data.Chtimes(dest, entry.info.ModTime(), entry.info.ModTime()); err != nil {
					return err
				}
			}
			return e.checkResourceManifest(io.Source, entries)
		}
		return e.copyExportFile(io.Source, target, from, to)
	}
	copied := map[string]bool{}
	copyAsset := func(asset string, annotations bool) error {
		if err := e.guard(); err != nil {
			return err
		}
		util.PushEndlessProgress(e.binding.scope.commit.r.language(65) + " " + fmt.Sprintf(e.binding.scope.commit.r.language(70), asset))
		normalized, err := normalizeExportAsset(asset)
		if err != nil {
			return err
		}
		if copied[normalized] {
			return nil
		}
		from, ok := index[normalized]
		if !ok {
			return nil
		} // Preserve dangling-resource omission.
		to := strings.TrimSuffix(normalized, "/")
		if err = copyPath(from, to); err != nil {
			return err
		}
		if annotations && strings.HasSuffix(strings.ToLower(from), ".pdf") {
			if info, err := io.Source.Data.Stat(from); err != nil {
				return err
			} else if !info.IsDir() {
				if err = io.Source.Check(from+".sya", false); err == nil {
					if err = copyPath(from+".sya", to+".sya"); err != nil {
						return err
					}
				} else if !os.IsNotExist(err) {
					return err
				}
			}
		}
		copied[normalized] = true
		return nil
	}
	direct := []string{}
	for _, tree := range trees {
		if err = e.guard(); err != nil {
			return err
		}
		assets, err := getAssetsLinkDestsWithAV(tree.Root, e.readExportView)
		if err != nil {
			return err
		}
		if cover := treenode.GetDocTitleImgPath(tree.Root); cover != "" && util.IsAssetLinkDest([]byte(cover)) {
			assets = append(assets, cover)
		}
		for _, asset := range assets {
			if err = copyAsset(asset, true); err != nil {
				return err
			}
		}
		for _, emoji := range emojisInTree(tree) {
			name := strings.TrimPrefix(emoji, "/")
			if !strings.HasPrefix(name, "emojis/") || !fs.ValidPath(name) || strings.Contains(name, "\\") {
				return indexrepair.ErrSourcePath
			}
			if err = copyPath(name, name); err != nil {
				return err
			}
		}
		ast.Walk(tree.Root, func(n *ast.Node, entering bool) ast.WalkStatus {
			if entering && n.IsBlock() {
				if n.Type == ast.NodeAttributeView {
					direct = append(direct, n.AttributeViewID)
				}
				for _, id := range strings.Split(n.IALAttr(av.NodeAttrNameAvs), ",") {
					direct = append(direct, strings.TrimSpace(id))
				}
			}
			return ast.WalkContinue
		})
	}
	visited := map[string]bool{}
	var copyView func(string) error
	copyView = func(id string) error {
		if id == "" || visited[id] {
			return e.guard()
		}
		visited[id] = true
		view, err := e.readExportView(id)
		if err != nil || view == nil {
			return err
		}
		if err = writeRaw("storage/av/"+id+".json", e.views[id].raw); err != nil {
			return err
		}
		for _, kv := range view.KeyValues {
			if kv.Key.Type == av.KeyTypeRelation && kv.Key.Relation != nil {
				if err = copyView(kv.Key.Relation.AvID); err != nil {
					return err
				}
			}
		}
		return e.guard()
	}
	// Related-only AVs retain the old raw-copy-only policy. Direct AVs also copy
	// their mAsset values; URL values are collected by AV nodes in the tree above.
	handled := map[string]bool{}
	for _, id := range direct {
		if id == "" || handled[id] {
			continue
		}
		handled[id] = true
		view, err := e.readExportView(id)
		if err != nil {
			return err
		}
		if view == nil {
			continue
		}
		if err = copyView(id); err != nil {
			return err
		}
		for _, kv := range view.KeyValues {
			if kv.Key.Type == av.KeyTypeMAsset {
				for _, value := range kv.Values {
					for _, asset := range value.MAsset {
						if util.IsAssetLinkDest([]byte(asset.Content)) {
							if err = copyAsset(asset.Content, false); err != nil {
								return err
							}
						}
					}
				}
			}
		}
	}
	return e.guard()
}
