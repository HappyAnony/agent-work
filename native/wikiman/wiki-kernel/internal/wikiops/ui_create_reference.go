// SPDX-License-Identifier: AGPL-3.0-or-later
package wikiops

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"

	"wiki-kernel/internal/editcore"
	"wiki-kernel/internal/sourcetx"
	"wiki-kernel/internal/workspace"
)

const uiCreateReferenceEndpoint = "/api/filetree/createDocWithReference"

func runUICreateDocWithReference(ctx context.Context, m *workspace.WorkspaceManager, r UIRequest, view UIView) (any, error) {
	if view.Mode != "wikiDesktop" {
		return nil, ErrUnauthorized
	}
	var q struct {
		BoxID, DocID, BlockID, ExpectedHash, BlockDOM string
		Notebook, ParentID, ID, Title, HPath          string
	}
	if e := Decode(bytes.NewReader(r.Data), &q); e != nil {
		return nil, e
	}
	if !editOperationID.MatchString(r.OperationID) || !hashPattern.MatchString(q.ExpectedHash) ||
		!nativeID.MatchString(q.BoxID) || !nativeID.MatchString(q.DocID) || !nativeID.MatchString(q.BlockID) ||
		!nativeID.MatchString(q.Notebook) || !nativeID.MatchString(q.ID) || q.ID == q.DocID ||
		q.ParentID != "" && !nativeID.MatchString(q.ParentID) || len(q.BlockDOM) == 0 ||
		len(q.Title) == 0 || len(q.Title) > 1024 || len(q.HPath) > 4096 {
		return nil, ErrArgument
	}
	request := ContentRequest{ProtocolVersion: 1, StoreID: r.StoreID, PageID: r.PageID, Action: "document.create",
		OperationID: r.OperationID, BoxID: q.Notebook, DocID: q.ID, ParentDocID: q.ParentID, Title: q.Title}
	if q.HPath != "" {
		request.CreationMode, request.HPath = "markdown", q.HPath
	}
	if e := request.Validate(); e != nil {
		return nil, e
	}
	canonical, _ := json.Marshal(r)
	result, err := planContentLifecycle(ctx, m, request, func(p *lifecycleEdit) error {
		name, raw, e := p.doc(q.BoxID, q.DocID)
		if e != nil {
			return e
		}
		if sourcetx.Hash(raw) != q.ExpectedHash {
			return sourcetx.ErrConflict
		}
		root, e := parseNode(raw)
		if e != nil {
			return e
		}
		node, _, _ := root.find(q.BlockID)
		if node == nil || node == root {
			return ErrUnauthorized
		}
		l, e := p.lease()
		if e != nil {
			return e
		}
		defer l.Release()
		cfg, e := l.Configuration(p.ctx)
		if e != nil {
			return e
		}
		if cfg.ReadOnly || cfg.Editor != nil && cfg.Editor.ReadOnly {
			return ErrUnauthorized
		}
		var attrs map[string]json.RawMessage
		if value := root.fields["Properties"]; len(value) > 0 {
			if e = json.Unmarshal(value, &attrs); e != nil {
				return e
			}
		}
		if bytes.Equal(attrs["custom-sy-readonly"], []byte(`"true"`)) {
			return ErrUnauthorized
		}
		if e = p.create(); e != nil {
			return e
		}
		if p.result.DocID != q.ID {
			return sourcetx.ErrConflict
		}
		plan, e := l.PlanNativeEdits(p.ctx, []*editcore.Operation{{Action: "update", ID: q.BlockID, Data: q.BlockDOM}})
		if e != nil {
			return e
		}
		if len(plan.Changes) != 1 {
			return ErrArgument
		}
		change := plan.Changes[0]
		if change.Path != p.prefix+name || change.ExpectedHash != q.ExpectedHash || change.Delete {
			return ErrUnauthorized
		}
		for _, existing := range p.tx.Changes {
			if existing.Path == change.Path {
				return sourcetx.ErrConflict
			}
		}
		after, e := parseNode(change.Data)
		if e != nil {
			return e
		}
		block, _, _ := after.find(q.BlockID)
		if block == nil || !hasCreatedDocReference(block, q.ID) {
			return ErrArgument
		}
		p.tx.Changes = append(p.tx.Changes, change)
		return nil
	}, sourcetx.Hash(canonical))
	if err != nil {
		return nil, err
	}
	return map[string]any{"id": result.DocID, "receipt": result.Receipt, "indexFresh": result.OriginalIndexFresh}, nil
}

func hasCreatedDocReference(block *astNode, id string) bool {
	var target, typ string
	if json.Unmarshal(block.fields["TextMarkBlockRefID"], &target) == nil && target == id &&
		json.Unmarshal(block.fields["TextMarkType"], &typ) == nil && strings.Contains(" "+typ+" ", " block-ref ") {
		return true
	}
	for _, child := range block.children {
		if hasCreatedDocReference(child, id) {
			return true
		}
	}
	return false
}
