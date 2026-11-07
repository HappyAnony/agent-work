// SPDX-License-Identifier: AGPL-3.0-or-later
package wikiops

import (
	"context"
	"maps"
	"net/http"
	"regexp"
	"slices"
	"strings"

	"github.com/siyuan-note/dataparser"
	"wiki-kernel/internal/assetflow"
	"wiki-kernel/internal/docast"
	"wiki-kernel/internal/nativeparser"
	"wiki-kernel/internal/sourcetx"
	"wiki-kernel/internal/workspace"
)

var clipboardAssetName = regexp.MustCompile(`^clipboard-[a-f0-9]{8}(-[a-f0-9]{4}){3}-[a-f0-9]{12}\.(png|jpeg|gif|webp)$`)

func validClipboardAssets(assets map[string][]byte) bool {
	if len(assets) > 20 {
		return false
	}
	total := 0
	for name, data := range assets {
		_, suffix, _ := strings.Cut(name, ".")
		total += len(data)
		if total > 8<<20 || !clipboardAssetName.MatchString(name) || http.DetectContentType(data) != "image/"+suffix {
			return false
		}
	}
	return true
}

func stageClipboardAssets(ctx context.Context, tx *sourcetx.Request, prefix, document string, assets map[string][]byte) error {
	var current []byte
	for _, change := range tx.Changes {
		if change.Path == prefix+document && !change.Delete {
			current = change.Data
			break
		}
	}
	if current == nil {
		return ErrArgument
	}
	input, err := docast.OriginalTreeInput(current)
	if err != nil {
		return err
	}
	engine := nativeparser.NewEditor(nativeparser.EditorOptions{})
	tree, _, err := dataparser.ParseJSON(input, engine.ParseOptions)
	if err != nil {
		return err
	}
	if tree == nil || tree.Root == nil {
		return ErrArgument
	}
	references, err := assetflow.ImageDests(ctx, tree.Root)
	if err != nil {
		return err
	}
	for _, name := range slices.Sorted(maps.Keys(assets)) {
		rel := "assets/" + name
		if !slices.Contains(references, rel) {
			return ErrArgument
		}
		tx.Changes = append(tx.Changes, sourcetx.Change{Path: prefix + rel, Data: assets[name]})
	}
	return nil
}

func guardClipboardAssetUndo(ctx context.Context, m *workspace.WorkspaceManager, r EditRequest, tx *sourcetx.Request, prefix string) error {
	var assets []string
	after := map[string]sourcetx.Change{}
	for _, change := range tx.Changes {
		after[change.Path] = change
		if name, ok := strings.CutPrefix(change.Path, prefix+"assets/"); ok && change.Delete && clipboardAssetName.MatchString(name) {
			assets = append(assets, "assets/"+name)
		}
	}
	if len(assets) == 0 {
		return nil
	}
	files, guards, err := m.PageReadGuards(ctx, r.StoreID, r.PageID)
	if err != nil {
		return err
	}
	for name, expected := range tx.ReadGuards.Files {
		if guards.Files[name] != expected {
			return sourcetx.ErrConflict
		}
	}
	for _, file := range files {
		rel := strings.TrimPrefix(file.Path, prefix)
		if !activeDoc(rel) && !(strings.HasPrefix(rel, "storage/av/") && strings.HasSuffix(rel, ".json")) {
			continue
		}
		if change, found := after[file.Path]; found {
			if !change.Delete && slices.ContainsFunc(assets, func(asset string) bool {
				return retainedAssetReference(change.Data, asset)
			}) {
				return sourcetx.ErrConflict
			}
			continue
		}
		data, err := m.SourceFile(ctx, r.StoreID, file.Path)
		if err != nil {
			return err
		}
		if sourcetx.Hash(data) != file.Hash || slices.ContainsFunc(assets, func(asset string) bool {
			return retainedAssetReference(data, asset)
		}) {
			return sourcetx.ErrConflict
		}
	}
	tx.ReadGuards = guards
	p := lifecycleEdit{ctx: ctx, m: m, request: ContentRequest{StoreID: r.StoreID, PageID: r.PageID}, tx: *tx}
	for _, asset := range assets {
		if err := p.checkRetainedAsset(asset); err != nil {
			return err
		}
	}
	return nil
}
