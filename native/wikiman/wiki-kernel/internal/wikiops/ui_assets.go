// SPDX-License-Identifier: AGPL-3.0-or-later
package wikiops

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"net/url"
	"strings"

	"github.com/88250/lute/ast"
	"wiki-kernel/internal/workspace"
)

var uiAssetActions = map[string]string{
	"/api/asset/getFileAnnotation": "asset.get",
	"/api/asset/setFileAnnotation": "asset.put",
	"/api/asset/renameAsset":       "asset.rename",
	"/api/asset/removeUnusedAsset": "asset.delete",
}

func runUIAsset(ctx context.Context, m *workspace.WorkspaceManager, r UIRequest, view UIView, action string) (any, error) {
	var input struct{ Path, OldPath, NewName, Data string }
	if err := Decode(bytes.NewReader(r.Data), &input); err != nil {
		return nil, err
	}
	name := input.Path
	if action == "asset.rename" {
		name = input.OldPath
	}
	name, err := url.PathUnescape(name)
	if err != nil || !strings.HasPrefix(strings.TrimPrefix(name, "/"), "assets/") {
		return nil, ErrArgument
	}
	name = strings.TrimPrefix(strings.TrimPrefix(name, "/"), "assets/")
	annotation := action == "asset.get" || action == "asset.put"
	if !validAsset(name) || annotation && !strings.HasSuffix(name, ".sya") {
		return nil, ErrArgument
	}
	request := ContentRequest{ProtocolVersion: 1, StoreID: r.StoreID, PageID: r.PageID, Action: "asset.get", AssetName: name}
	if action == "asset.get" {
		var result ContentResult
		if view.Mode == "docxSingle" {
			result, err = DocumentAsset(ctx, m, r.StoreID, r.PageID, view.Entry.BoxID, view.Entry.DocID, name, true)
		} else {
			result, err = Content(ctx, m, request)
		}
		if err != nil {
			return nil, err
		}
		if !json.Valid(result.Data) {
			return nil, ErrArgument
		}
		return map[string]string{"data": string(result.Data)}, nil
	}
	if view.Mode != "wikiDesktop" {
		return nil, ErrUnauthorized
	}
	err = m.WithReadContext(ctx, r.StoreID, func(c context.Context) error {
		l, e := m.Acquire(c, r.StoreID, r.PageID)
		if e != nil {
			return e
		}
		defer l.Release()
		cfg, e := l.Configuration(c)
		if e != nil {
			return e
		}
		if cfg.ReadOnly || cfg.Editor != nil && cfg.Editor.ReadOnly {
			return ErrUnauthorized
		}
		if annotation {
			asset := request
			asset.AssetName = strings.TrimSuffix(name, ".sya")
			if _, e = Content(c, m, asset); e != nil {
				return e
			}
		}
		current, e := Content(c, m, request)
		if e != nil && !(annotation && errors.Is(e, fs.ErrNotExist)) {
			return e
		}
		if e == nil {
			request.ExpectedHash = current.Hash
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	request.Action, request.OperationID, request.NewAssetName = action, "ui-asset-"+ast.NewNodeID(), input.NewName
	if annotation {
		if !json.Valid([]byte(input.Data)) {
			return nil, ErrArgument
		}
		request.Data = []byte(input.Data)
	}
	result, err := Content(ctx, m, request)
	if err != nil {
		return nil, err
	}
	switch action {
	case "asset.rename":
		return map[string]string{"newPath": "assets/" + result.AssetName}, nil
	case "asset.delete":
		return map[string]string{"path": "assets/" + name}, nil
	}
	return nil, nil
}
