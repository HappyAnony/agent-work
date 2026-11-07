// SPDX-License-Identifier: AGPL-3.0-or-later
package wikiops

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"maps"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/88250/lute/ast"
	"wiki-kernel/conf"
	"wiki-kernel/internal/notebooks"
	"wiki-kernel/internal/sourcetx"
	"wiki-kernel/internal/workspace"
)

var uiNotebookArchiveRoutes = map[string]string{
	"/api/notebook/removeNotebook":         "remove",
	"/api/history/getNotebookHistory":      "list",
	"/api/history/rollbackNotebookHistory": "restore",
}

func runUINotebookArchive(ctx context.Context, m *workspace.WorkspaceManager, ui UIRequest, action string) (any, error) {
	var box string
	switch action {
	case "list":
		var input struct{}
		if err := Decode(bytes.NewReader(ui.Data), &input); err != nil {
			return nil, ErrArgument
		}
		return uiNotebookHistory(ctx, m, ui)
	case "remove":
		var input struct{ Notebook, Callback string }
		if err := Decode(bytes.NewReader(ui.Data), &input); err != nil {
			return nil, ErrArgument
		}
		box = input.Notebook
	case "restore":
		var input struct{ HistoryPath string }
		if err := Decode(bytes.NewReader(ui.Data), &input); err != nil {
			return nil, ErrArgument
		}
		box = input.HistoryPath
	}
	if !nativeID.MatchString(box) || notebooks.Guide(box) {
		return nil, ErrArgument
	}
	request := ContentRequest{ProtocolVersion: 1, StoreID: ui.StoreID, PageID: ui.PageID, BoxID: box,
		Action: "notebook." + action, OperationID: "ui-notebook-" + ast.NewNodeID()}
	_, err := planContentLifecycle(ctx, m, request, func(p *lifecycleEdit) error {
		if p.page.EntryDocument != nil && p.page.EntryDocument.BoxID == box {
			return ErrArgument
		}
		l, err := p.lease()
		if err != nil {
			return err
		}
		defer l.Release()
		cfg, err := l.Configuration(p.ctx)
		if err != nil {
			return err
		}
		if cfg.ReadOnly || cfg.Editor != nil && cfg.Editor.ReadOnly {
			return ErrUnauthorized
		}
		archive := sourcetx.NotebookArchive{Version: 1, PageID: ui.PageID, BoxID: box}
		if action == "restore" {
			return p.restoreNotebookArchive(archive.RecordPath())
		}
		if _, err = l.NativeNotebookConfiguration(p.ctx, box); err != nil {
			return err
		}
		archive.DeletedAt = time.Now().UTC().Format(time.RFC3339)
		return p.removeNotebookArchive(l, archive)
	})
	return nil, err
}

func (p *lifecycleEdit) removeNotebookArchive(l *workspace.Lease, a sourcetx.NotebookArchive) error {
	var ids, removed []string
	copied := map[string]string{}
	for _, name := range slices.Sorted(maps.Keys(p.files)) {
		rel, ok := strings.CutPrefix(name, a.BoxID+"/")
		if !ok {
			continue
		}
		raw, err := p.bytes(name)
		if err != nil {
			return err
		}
		removed = append(removed, name)
		if activeDoc(name) {
			ids = append(ids, strings.TrimSuffix(path.Base(name), ".docj"))
		}
		hash := p.files[name].Hash
		a.Files = append(a.Files, sourcetx.DeletedFile{Path: rel, Hash: hash})
		p.change(a.ArchivePath(rel), "", raw, false)
		p.change(a.ActivePrefix()+rel, hash, nil, true)
		if _, asset, ok := strings.Cut("/"+rel, "/assets/"); ok && validAsset(asset) {
			target := "assets/" + asset
			existing := p.files[target].Hash
			if existing == "" {
				existing = copied[target]
			}
			if existing != "" && existing != hash {
				return sourcetx.ErrConflict
			}
			if existing == "" {
				p.write(target, raw)
				copied[target] = hash
			}
		}
	}
	if len(ids) > 0 {
		changes, err := p.deleteEffects(l, ids, removed, removed)
		if err != nil {
			return err
		}
		if err = p.stageDeleteEffects(&a.Effects, a.EffectPath, changes, map[string]bool{}); err != nil {
			return err
		}
	}
	raw, err := json.Marshal(a)
	if err != nil {
		return err
	}
	p.change(a.RecordPath(), "", raw, false)
	return nil
}

func (p *lifecycleEdit) restoreNotebookArchive(record string) error {
	raw, err := p.read(record)
	if err != nil {
		return err
	}
	a, err := sourcetx.DecodeNotebookArchive(raw, record)
	if err != nil {
		return err
	}
	candidates := map[string][]byte{}
	for _, f := range a.Files {
		saved := a.ArchivePath(f.Path)
		data, err := p.read(saved)
		if err != nil {
			return err
		}
		if sourcetx.Hash(data) != f.Hash {
			return sourcetx.ErrConflict
		}
		if name := a.BoxID + "/" + f.Path; activeDoc(name) {
			candidates[name] = data
		}
		p.change(a.ActivePrefix()+f.Path, "", data, false)
		p.change(saved, f.Hash, nil, true)
	}
	replaced := map[string]bool{}
	if err = p.restoreDeleteEffects(a.Effects, a.EffectPath, candidates, replaced); err != nil {
		return err
	}
	if err = p.validateDocumentIDs(candidates, replaced); err != nil {
		return err
	}
	p.change(record, sourcetx.Hash(raw), nil, true)
	return nil
}

func uiNotebookHistory(ctx context.Context, m *workspace.WorkspaceManager, ui UIRequest) (any, error) {
	type item struct {
		Path  string `json:"path"`
		Title string `json:"title"`
	}
	type group struct {
		Created string `json:"hCreated"`
		Items   []item `json:"items"`
	}
	histories := map[string][]item{}
	err := m.WithReadContext(ctx, ui.StoreID, func(c context.Context) error {
		files, err := m.ArchiveFiles(c, ui.StoreID, "notebooks", ui.PageID, "", "")
		if err != nil {
			return err
		}
		for _, f := range files {
			raw, err := m.SourceFile(c, ui.StoreID, f.Path)
			if err != nil || sourcetx.Hash(raw) != f.Hash {
				return sourcetx.ErrConflict
			}
			a, err := sourcetx.DecodeNotebookArchive(raw, f.Path)
			if err != nil {
				return err
			}
			var data []byte
			for _, name := range []string{".wiki/conf.json", ".siyuan/conf.json"} {
				data, err = m.SourceFile(c, ui.StoreID, a.ArchivePath(name))
				if err == nil {
					break
				}
				if !errors.Is(err, fs.ErrNotExist) {
					return err
				}
			}
			var cfg conf.BoxConf
			if err != nil || json.Unmarshal(data, &cfg) != nil {
				return ErrArgument
			}
			stamp, _ := time.Parse(time.RFC3339, a.DeletedAt)
			key := stamp.Local().Format("2006-01-02 15:04:05")
			histories[key] = append(histories[key], item{Path: a.BoxID, Title: cfg.Name})
		}
		return nil
	})
	groups := make([]group, 0, len(histories))
	for _, stamp := range slices.Backward(slices.Sorted(maps.Keys(histories))) {
		groups = append(groups, group{Created: stamp, Items: histories[stamp]})
	}
	return map[string]any{"histories": groups}, err
}
