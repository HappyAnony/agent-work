// SPDX-License-Identifier: AGPL-3.0-or-later
package wikiops

import (
	"bytes"
	"context"

	"wiki-kernel/internal/docast"
	"wiki-kernel/internal/docview"
	"wiki-kernel/internal/nativeparser"
	"wiki-kernel/internal/sourcetx"
	"wiki-kernel/internal/workspace"
)

const uiDocVersionsEndpoint = "/api/history/docVersions"

type uiDocVersionsRequest struct {
	Action, BoxID, DocID, Cursor, VersionID, LegacyHistoryPath, VersionHash, ExpectedHash string
}

func runUIDocVersions(ctx context.Context, m *workspace.WorkspaceManager, r UIRequest, view UIView) (any, error) {
	var q uiDocVersionsRequest
	if err := Decode(bytes.NewReader(r.Data), &q); err != nil {
		return nil, err
	}
	if !nativeID.MatchString(q.BoxID) || !nativeID.MatchString(q.DocID) {
		return nil, ErrArgument
	}
	if view.Mode == "docxSingle" && (q.BoxID != view.Entry.BoxID || q.DocID != view.Entry.DocID) {
		return nil, ErrUnauthorized
	}
	req := ContentRequest{ProtocolVersion: 1, StoreID: r.StoreID, PageID: r.PageID, BoxID: q.BoxID, DocID: q.DocID}
	switch q.Action {
	case "list":
		if q.VersionID != "" || q.LegacyHistoryPath != "" || q.VersionHash != "" || q.ExpectedHash != "" || r.OperationID != "" {
			return nil, ErrArgument
		}
		req.Action, req.Limit, req.Cursor = "document.version.list", 32, q.Cursor
	case "preview":
		if q.Cursor != "" || q.ExpectedHash != "" || r.OperationID != "" || !hashPattern.MatchString(q.VersionHash) {
			return nil, ErrArgument
		}
		req.Action, req.VersionID, req.LegacyHistoryPath = "document.version.get", q.VersionID, q.LegacyHistoryPath
	case "restore":
		if q.Cursor != "" || !hashPattern.MatchString(q.ExpectedHash) || !hashPattern.MatchString(q.VersionHash) || !editOperationID.MatchString(r.OperationID) {
			return nil, ErrArgument
		}
		req.Action, req.VersionID, req.LegacyHistoryPath, req.VersionHash = "document.version.apply", q.VersionID, q.LegacyHistoryPath, q.VersionHash
		req.ExpectedHash, req.OperationID = q.ExpectedHash, r.OperationID
	default:
		return nil, ErrArgument
	}
	var result any
	err := m.WithReadContext(ctx, r.StoreID, func(c context.Context) error {
		tree, err := m.Tree(c, r.StoreID)
		if err != nil {
			return err
		}
		page, ok := tree.Get(r.PageID)
		if !ok || page.DeletedAt != nil || view.Mode == "docxSingle" && (page.EntryDocument == nil || *page.EntryDocument != *view.Entry) {
			return ErrStaleSession
		}
		if q.Action == "list" {
			result, err = Content(c, m, req)
			return err
		}
		if q.Action == "restore" {
			req.ExpectedTreeDigest = tree.SourceDigest()
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
			if page.EntryDocument != nil && page.EntryDocument.BoxID == q.BoxID && page.EntryDocument.DocID == q.DocID {
				meta, err := m.SourceFile(c, r.StoreID, "pages/"+r.PageID+"/page.json")
				if err != nil {
					return err
				}
				req.ExpectedPageHash = sourcetx.Hash(meta)
			}
			return nil
		}
		version, err := Content(c, m, req)
		if err != nil {
			return err
		}
		if version.Hash != q.VersionHash {
			return sourcetx.ErrConflict
		}
		_, current, err := m.DocumentSource(c, r.StoreID, r.PageID, q.BoxID, q.DocID)
		if err != nil {
			return err
		}
		raw, err := docast.OriginalTreeInput(version.AST)
		if err != nil {
			return err
		}
		preview, err := docview.RenderHistory(c, raw, "", false, false, nativeparser.NewEditor(nativeparser.EditorOptions{}))
		if err != nil {
			return err
		}
		result = map[string]any{"content": preview.Content, "title": preview.Title, "isLargeDoc": preview.Large, "expectedHash": sourcetx.Hash(current)}
		return nil
	})
	if err != nil || q.Action != "restore" {
		return result, err
	}
	return Content(ctx, m, req)
}
