// SPDX-License-Identifier: AGPL-3.0-or-later
package wikiops

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"wiki-kernel/internal/editcore"
	"wiki-kernel/internal/sourcetx"
	"wiki-kernel/internal/workspace"
)

func portableEditOperations(operations []ProtyleOperation) ([]*editcore.Operation, error) {
	out := make([]*editcore.Operation, 0, len(operations))
	for _, op := range operations {
		switch op.Action {
		case "update", "insert", "delete", "move", "moveOutlineHeading", "append", "appendInsert", "prependInsert", "foldHeading", "unfoldHeading", "setAttrs", "doUpdateUpdated":
		default:
			return nil, ErrRichText
		}
		var data any
		if len(op.Data) > 0 {
			if e := json.Unmarshal(op.Data, &data); e != nil {
				return nil, ErrArgument
			}
		}
		if op.Action == "setAttrs" {
			if attrs, ok := data.(map[string]any); ok {
				for _, v := range attrs {
					if _, ok := v.(string); !ok && v != nil {
						return nil, ErrArgument
					}
				}
				data = string(op.Data)
			}
		}
		out = append(out, &editcore.Operation{Action: op.Action, ID: op.ID, Data: data, ParentID: op.ParentID, PreviousID: op.PreviousID, NextID: op.NextID})
	}
	if e := editcore.ValidateOperations(context.Background(), out); e != nil {
		return nil, errors.Join(ErrArgument, e)
	}
	return out, nil
}

func contentOriginalEdit(ctx context.Context, m *workspace.WorkspaceManager, r ContentRequest) (out ContentResult, err error) {
	name, source, err := m.DocumentSource(ctx, r.StoreID, r.PageID, r.BoxID, r.DocID)
	if err != nil {
		return out, err
	}
	name = strings.TrimPrefix(name, "pages/"+r.PageID+"/data/")
	q := EditRequest{ProtocolVersion: r.ProtocolVersion, StoreID: r.StoreID, PageID: r.PageID, OperationID: r.OperationID, ExpectedSources: map[string]string{name: r.ExpectedHash}, ExpectedTreeDigest: r.ExpectedTreeDigest, DryRun: r.DryRun}
	if r.Action == "document.history" {
		q.History = &EditHistory{OperationID: r.HistoryID, Redo: r.Redo}
	} else {
		q.Operations, err = portableEditOperations(r.Operations)
		if err != nil {
			return out, err
		}
		q.clipboardAssets = r.ClipboardAssets
	}
	canonical := r
	canonical.DryRun = false
	raw, err := json.Marshal(canonical)
	if err != nil {
		return out, err
	}
	clientHash := sourcetx.Hash(raw)
	// Preserve the older HTML-only delete admission. The owning editor opts
	// into server-side native history; no inverse HTML is used for that path.
	if r.Action == "document.transactions" && !r.NativeHistory {
		_, replay, e := m.LookupReceipt(ctx, r.StoreID, r.OperationID, clientHash)
		if e != nil {
			return out, e
		}
		if !replay {
			root, e := parseNode(source)
			if e != nil {
				return out, e
			}
			for _, op := range r.Operations {
				if op.Action != "delete" && op.Action != "update" {
					continue
				}
				n, _, _ := root.find(op.ID)
				if n == nil || !opaqueForHTMLUndo(n, nativeFields(), map[*astNode]bool{}) {
					continue
				}
				if op.Action == "delete" {
					return out, ErrRichText
				}
				var html string
				if json.Unmarshal(op.Data, &html) != nil {
					return out, ErrArgument
				}
				next, e := domBlock(html, op.ID)
				if e != nil {
					return out, e
				}
				if e = preserveNative(n, next, false); e != nil {
					return out, e
				}
			}
		}
	}
	result, err := editOriginal(ctx, m, q, name, nil, clientHash)
	if errors.Is(err, editcore.ErrOpaqueEdit) {
		err = errors.Join(ErrRichText, err)
	}
	return ContentResult{StoreID: r.StoreID, PageID: r.PageID, Receipt: result.Receipt, Hash: result.Hashes[name], HistoryID: result.HistoryID, OriginalIndexFresh: result.IndexFresh}, err
}
