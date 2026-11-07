// SPDX-License-Identifier: AGPL-3.0-or-later
package wikiops

import (
	"context"
	"io/fs"

	"wiki-kernel/internal/doclife"
	"wiki-kernel/internal/workspace"
)

func uiCreateSavePath(reference bool) uiRoute {
	return uiRouteFor(func(q struct{ Notebook string }) (uiCall, error) {
		if !nativeID.MatchString(q.Notebook) {
			return uiCall{}, ErrArgument
		}
		return uiCall{pageWide: true, run: func(ctx context.Context, l *workspace.Lease) (any, error) {
			cfg, err := l.Configuration(ctx)
			if err != nil {
				return nil, err
			}
			if cfg.FileTree == nil {
				return nil, ErrArgument
			}
			notebooks, err := l.ConfigurationNotebooks(ctx)
			if err != nil {
				return nil, err
			}
			opened := make(map[string]bool, len(notebooks))
			for _, notebook := range notebooks {
				opened[notebook.ID] = !notebook.Closed
			}
			if !opened[q.Notebook] {
				return nil, fs.ErrNotExist
			}
			local, err := l.NativeNotebookConfiguration(ctx, q.Notebook)
			if err != nil {
				return nil, err
			}
			globalBox, globalPath := cfg.FileTree.DocCreateSaveBox, cfg.FileTree.DocCreateSavePath
			localBox, localPath := local.DocCreateSaveBox, local.DocCreateSavePath
			if reference {
				globalBox, globalPath = cfg.FileTree.RefCreateSaveBox, cfg.FileTree.RefCreateSavePath
				localBox, localPath = local.RefCreateSaveBox, local.RefCreateSavePath
			}
			box, source := doclife.SelectCreateSavePath(q.Notebook, globalBox, globalPath, localBox, localPath, true, reference, func(id string) bool {
				return opened[id]
			})
			value := source
			if source != "" {
				plan, err := l.PlanNativeTemplate(ctx, workspace.NativeTemplateRequest{Action: "go", Source: source})
				if err != nil {
					return nil, err
				}
				value = plan.Text
			}
			return map[string]string{"box": box, "path": value}, nil
		}}, nil
	})
}
