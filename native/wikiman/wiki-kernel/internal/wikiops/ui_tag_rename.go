// SPDX-License-Identifier: AGPL-3.0-or-later
package wikiops

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"

	"wiki-kernel/internal/markup"
	"wiki-kernel/internal/sourcetx"
	"wiki-kernel/internal/workspace"
)

const uiTagRenameEndpoint = "/api/tag/renameTag"

type uiTagRename struct {
	OldLabel string `json:"oldLabel"`
	NewLabel string `json:"newLabel"`
}

func runUITagRename(ctx context.Context, m *workspace.WorkspaceManager, request UIRequest, view UIView) (any, error) {
	if view.Mode != "wikiDesktop" {
		return nil, ErrUnauthorized
	}
	var rename uiTagRename
	if err := Decode(bytes.NewReader(request.Data), &rename); err != nil {
		return nil, ErrArgument
	}
	rename.NewLabel = strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(rename.NewLabel), "/"), "/")
	for _, label := range []string{rename.OldLabel, rename.NewLabel} {
		if strings.TrimSpace(label) == "" || len(label) > 1024 || strings.ContainsAny(label, "\x00\r\n,#") {
			return nil, ErrArgument
		}
	}
	if markup.ContainsMarker(rename.NewLabel) != "" {
		return nil, ErrArgument
	}
	raw, err := json.Marshal(request)
	if err != nil {
		return nil, ErrArgument
	}
	clientHash := sourcetx.Hash(raw)
	edit := EditRequest{ProtocolVersion: 1, StoreID: request.StoreID, PageID: request.PageID,
		OperationID: request.OperationID, DryRun: true, tagRename: &rename}
	preview, err := editOriginal(ctx, m, edit, "", nil, clientHash)
	if err != nil {
		return nil, err
	}
	if !preview.Replay && len(preview.RequiredSources) == 0 {
		return map[string]bool{"changed": false}, nil
	}
	edit.DryRun, edit.ExpectedSources = false, preview.RequiredSources
	_, err = editOriginal(ctx, m, edit, "", nil, clientHash)
	return map[string]bool{"changed": true}, err
}
