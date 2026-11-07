// SPDX-License-Identifier: AGPL-3.0-or-later
package wikiops

import (
	"context"
	"path"
	"strings"

	"github.com/88250/lute/ast"
	"wiki-kernel/internal/sourcetx"
	"wiki-kernel/internal/workspace"
)

func runUIBatchLifecycle(ctx context.Context, m *workspace.WorkspaceManager, ui UIRequest, input uiLifecycleData, action string) error {
	paths := input.Paths
	if action == "document.move" {
		paths = input.FromPaths
		if !nativeID.MatchString(input.ToNotebook) || input.ToPath != "/" &&
			(!strings.HasPrefix(input.ToPath, "/") || !documentRelativePath(strings.TrimPrefix(input.ToPath, "/"))) {
			return ErrArgument
		}
	}
	if len(paths) == 0 || len(paths) > 1000 {
		return ErrArgument
	}
	request := ContentRequest{ProtocolVersion: 1, StoreID: ui.StoreID, PageID: ui.PageID, Action: action,
		ToBoxID: input.ToNotebook, OperationID: "ui-batch-" + ast.NewNodeID()}
	_, err := planContentLifecycle(ctx, m, request, func(p *lifecycleEdit) error {
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
		if _, err = l.PrepareNativeReads(p.ctx, false); err != nil {
			return err
		}
		sources := make([]string, 0, len(paths))
		seen := map[string]bool{}
		for _, supplied := range paths {
			id := strings.TrimSuffix(path.Base(supplied), ".docj")
			if !nativeID.MatchString(id) || !strings.HasSuffix(supplied, ".docj") || seen[id] {
				return ErrArgument
			}
			seen[id] = true
			loc, err := l.NativeBlockLocation(p.ctx, id)
			if err != nil {
				return err
			}
			name := loc["box"] + loc["path"]
			if loc["rootID"] != id || supplied != loc["path"] && strings.TrimPrefix(supplied, "/") != name {
				return sourcetx.ErrConflict
			}
			if _, _, err = p.doc(loc["box"], id); err != nil {
				return err
			}
			sources = append(sources, name)
		}
		if action == "document.delete" {
			return p.deleteDocuments(sources, true)
		}
		if input.ToPath != "/" {
			id := strings.TrimSuffix(path.Base(input.ToPath), ".docj")
			name, _, err := p.doc(input.ToNotebook, id)
			if err != nil {
				return err
			}
			if name != input.ToNotebook+input.ToPath {
				return sourcetx.ErrConflict
			}
		}
		if p.page.EntryDocument != nil {
			raw, err := p.read("pages/" + ui.PageID + "/page.json")
			if err != nil {
				return err
			}
			p.request.ExpectedPageHash = sourcetx.Hash(raw)
		}
		return p.moveDocuments(sources, input.ToNotebook, input.ToPath)
	})
	return err
}
