// SPDX-License-Identifier: AGPL-3.0-or-later
package wikiops

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"maps"

	"github.com/88250/lute/ast"
	"wiki-kernel/conf"
	"wiki-kernel/internal/sourcetx"
	"wiki-kernel/internal/workspace"
)

var uiWorkspaceSettings = map[string]string{
	"/api/setting/setEditor":    "editor",
	"/api/setting/setFiletree":  "fileTree",
	"/api/setting/setSearch":    "search",
	"/api/setting/setFlashcard": "flashcard",
}

func saveUIWorkspaceSetting(ctx context.Context, m *workspace.WorkspaceManager, r UIRequest, kind string) (any, error) {
	var request sourcetx.Request
	var value json.RawMessage
	err := m.WithReadContext(ctx, r.StoreID, func(c context.Context) error {
		tree, err := m.Tree(c, r.StoreID)
		if err != nil {
			return err
		}
		page, ok := tree.Get(r.PageID)
		if !ok || page.DeletedAt != nil {
			return fs.ErrNotExist
		}
		l, err := m.Acquire(c, r.StoreID, r.PageID)
		if err != nil {
			return err
		}
		defer l.Release()
		cfg, err := l.Configuration(c)
		if err != nil {
			return err
		}
		if cfg.ReadOnly || cfg.Editor != nil && cfg.Editor.ReadOnly {
			return ErrUnauthorized
		}
		plan, err := l.PlanNativeSetting(c, kind, r.Data)
		if err != nil {
			return err
		}
		for scope := range plan.Patches {
			if scope != conf.WorkspacePreference {
				return ErrUnauthorized
			}
		}
		for _, effect := range plan.Setting.Effects {
			if effect.Kind == "readonly" || effect.Kind == "history" {
				return ErrUnauthorized
			}
		}
		if kind == "editor" && plan.Setting.Data.Editor != nil && plan.Setting.Data.Editor.AllowHTMLBLockScript {
			return ErrUnauthorized
		}
		value = plan.Setting.Value
		patch, changed := plan.Patches[conf.WorkspacePreference]
		if !changed {
			return nil
		}
		name, err := sourcetx.WorkspacePreferencesPath(r.PageID)
		if err != nil {
			return err
		}
		doc := conf.PreferenceDocument{Version: 1, Scope: conf.WorkspacePreference, Values: map[string]json.RawMessage{}}
		var expected string
		raw, err := m.SourceFile(c, r.StoreID, name)
		if err == nil {
			doc, err = conf.DecodePreferences(raw, conf.WorkspacePreference)
			if err != nil {
				return err
			}
			expected = sourcetx.Hash(raw)
		} else if !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		maps.Copy(doc.Values, patch.Set)
		for _, field := range patch.Remove {
			delete(doc.Values, field)
		}
		request, err = sourcetx.WorkspacePreferencesRequest(r.PageID, "ui-pref-"+ast.NewNodeID(), expected, doc)
		request.ExpectedTreeDigest = tree.SourceDigest()
		return err
	})
	if err != nil {
		return nil, err
	}
	if request.OperationID != "" {
		if _, err = m.Apply(ctx, r.StoreID, request); err != nil {
			return nil, err
		}
	}
	return value, nil
}
