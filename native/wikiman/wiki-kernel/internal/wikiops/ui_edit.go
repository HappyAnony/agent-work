// SPDX-License-Identifier: AGPL-3.0-or-later
package wikiops

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"

	"wiki-kernel/internal/avmodel"
	"wiki-kernel/internal/editcore"
	"wiki-kernel/internal/sourcetx"
	"wiki-kernel/internal/workspace"
)

type uiEditBatch struct {
	DoOperations   []*editcore.Operation `json:"doOperations"`
	UndoOperations json.RawMessage       `json:"undoOperations,omitempty"`
}

type uiEditInput struct {
	App          string         `json:"app,omitempty"`
	Session      string         `json:"session,omitempty"`
	ReqID        int64          `json:"reqId,omitempty"`
	Transactions []uiEditBatch  `json:"transactions"`
	BoxID        string         `json:"boxId"`
	DocID        string         `json:"docId"`
	ExpectedHash string         `json:"expectedHash"`
	OperationID  string         `json:"operationId"`
	CellVersion  *uiCellVersion `json:"cellVersion,omitempty"`
}

type uiCellVersion struct {
	BlockID string `json:"blockId"`
	Hash    string `json:"hash"`
}

func runUIEdit(ctx context.Context, m *workspace.WorkspaceManager, r UIRequest, view UIView) (any, error) {
	// Rich operations can touch additional source files such as a database or
	// sibling tree. Single views use the document-scoped editor transaction API.
	if view.Mode != "wikiDesktop" {
		return nil, ErrUnauthorized
	}
	var input uiEditInput
	if err := Decode(bytes.NewReader(r.Data), &input); err != nil {
		return nil, err
	}
	if len(input.Transactions) == 0 || len(input.Transactions) > 1000 || !nativeID.MatchString(input.BoxID) || !nativeID.MatchString(input.DocID) ||
		!hashPattern.MatchString(input.ExpectedHash) || !editOperationID.MatchString(input.OperationID) {
		return nil, ErrArgument
	}
	var ops []*editcore.Operation
	for _, batch := range input.Transactions {
		if len(batch.DoOperations) == 0 || len(ops)+len(batch.DoOperations) > 1000 {
			return nil, ErrArgument
		}
		ops = append(ops, batch.DoOperations...)
	}
	for _, op := range ops {
		if op == nil || op.Action == "create" || op.RetData != nil {
			return nil, ErrArgument
		}
		if op.Context != nil && (op.Action != "insert" || len(op.Context) != 1 || op.Context["ignoreProcess"] != "true") {
			return nil, ErrArgument
		}
	}
	if v := input.CellVersion; v != nil {
		if len(ops) > 2 || !nativeID.MatchString(ops[0].AvID) ||
			!nativeID.MatchString(v.BlockID) || !hashPattern.MatchString(v.Hash) {
			return nil, ErrArgument
		}
		op := ops[0]
		if len(ops) == 2 {
			updated, ok := ops[1].Data.(string)
			if (op.Action != "insertAttrViewBlock" && op.Action != "removeAttrViewBlock" && op.Action != "addAttrViewCol") ||
				ops[1].Action != "doUpdateUpdated" || ops[1].ID != v.BlockID || !ok || len(updated) != 14 ||
				strings.Trim(updated, "0123456789") != "" {
				return nil, ErrArgument
			}
		}
		switch op.Action {
		case "updateAttrViewCell":
			if !nativeID.MatchString(op.ID) || !nativeID.MatchString(op.KeyID) || !nativeID.MatchString(op.RowID) {
				return nil, ErrArgument
			}
		case "setAttrViewFilters", "setAttrViewSorts":
			if op.BlockID != v.BlockID || !validUIAVRules(op) {
				return nil, ErrArgument
			}
		case "setAttrViewBlockView":
			if op.BlockID != v.BlockID || !nativeID.MatchString(op.ID) {
				return nil, ErrArgument
			}
		case "addAttrViewView":
			if op.BlockID != v.BlockID || !nativeID.MatchString(op.ID) ||
				(op.Layout != "" && !editcore.ValidAttributeViewManagementLayout(op.Layout)) {
				return nil, ErrArgument
			}
		case "insertAttrViewBlock":
			if op.BlockID != v.BlockID || len(op.Srcs) != 1 || op.Srcs[0]["isDetached"] != true {
				return nil, ErrArgument
			}
			itemID, _ := op.Srcs[0]["itemID"].(string)
			if !nativeID.MatchString(itemID) || op.PreviousID != "" && !nativeID.MatchString(op.PreviousID) {
				return nil, ErrArgument
			}
		case "removeAttrViewBlock":
			if op.BlockID != v.BlockID || len(op.SrcIDs) == 0 || len(op.SrcIDs) > 200 {
				return nil, ErrArgument
			}
			for _, id := range op.SrcIDs {
				if !nativeID.MatchString(id) {
					return nil, ErrArgument
				}
			}
		case "addAttrViewCol":
			if !nativeID.MatchString(op.ID) || op.PreviousID != "" && !nativeID.MatchString(op.PreviousID) ||
				op.Data != nil || len(op.Name) == 0 || len(op.Name) > 256 {
				return nil, ErrArgument
			}
			switch op.Typ {
			case "text", "number", "select", "mSelect", "date", "mAsset", "checkbox", "url", "email", "phone",
				"template", "relation", "rollup", "lineNumber", "created", "updated":
			default:
				return nil, ErrArgument
			}
		default:
			return nil, ErrArgument
		}
	}
	var name string
	err := m.WithReadContext(ctx, r.StoreID, func(c context.Context) error {
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
		var raw []byte
		name, raw, err = m.DocumentSource(c, r.StoreID, r.PageID, input.BoxID, input.DocID)
		if err == nil {
			for _, op := range ops {
				if op.Action != "insertAttrViewTable" {
					continue
				}
				root, e := parseNode(raw)
				if e != nil {
					return e
				}
				node, _, _ := root.find(op.PreviousID)
				if node == nil || node == root {
					return ErrUnauthorized
				}
			}
		}
		if err == nil && input.CellVersion != nil {
			root, e := parseNode(raw)
			if e != nil {
				return e
			}
			node, _, _ := root.find(input.CellVersion.BlockID)
			var avID, nodeType string
			if node == nil || json.Unmarshal(node.fields["Type"], &nodeType) != nil || nodeType != "NodeAttributeView" ||
				json.Unmarshal(node.fields["AttributeViewID"], &avID) != nil || avID != ops[0].AvID {
				return ErrUnauthorized
			}
			var attrs map[string]string
			if rawAttrs := root.fields["Properties"]; len(rawAttrs) > 0 {
				if e = json.Unmarshal(rawAttrs, &attrs); e != nil {
					return e
				}
			}
			if attrs["custom-sy-readonly"] == "true" {
				return ErrUnauthorized
			}
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	name = strings.TrimPrefix(name, "pages/"+r.PageID+"/data/")
	request := EditRequest{ProtocolVersion: 1, StoreID: r.StoreID, PageID: r.PageID,
		OperationID: input.OperationID, Operations: ops, DryRun: true}
	// One client identity binds both preview and commit. The document version
	// from the editor is never replaced by a newer preview hash.
	canonical, _ := json.Marshal(r)
	clientHash := sourcetx.Hash(canonical)
	preview, err := editOriginal(ctx, m, request, name, nil, clientHash)
	if err != nil {
		return nil, err
	}
	request.ExpectedSources = preview.RequiredSources
	if v := input.CellVersion; v != nil && !preview.Replay && request.ExpectedSources["storage/av/"+ops[0].AvID+".json"] != v.Hash {
		return nil, sourcetx.ErrConflict
	}
	if request.ExpectedSources == nil {
		request.ExpectedSources = map[string]string{}
	}
	if expected := request.ExpectedSources[name]; expected != "" && expected != input.ExpectedHash {
		return nil, sourcetx.ErrConflict
	}
	request.ExpectedSources[name] = input.ExpectedHash
	request.DryRun = false
	result, err := editOriginal(ctx, m, request, name, nil, clientHash)
	if err != nil {
		return nil, err
	}
	operations := result.Operations
	if len(operations) != len(ops) {
		operations = ops
	}
	batches := make([]uiEditBatch, 0, len(input.Transactions))
	for _, batch := range input.Transactions {
		count := len(batch.DoOperations)
		batches = append(batches, uiEditBatch{DoOperations: operations[:count]})
		operations = operations[count:]
	}
	return map[string]any{"transactions": batches,
		"hash": result.Hashes[name], "receipt": result.Receipt, "indexFresh": result.IndexFresh, "historyId": result.HistoryID}, nil
}

// Validate editor-supplied rule lists before exposing them to the original renderer.
func validUIAVRules(op *editcore.Operation) bool {
	rules, ok := op.Data.([]any)
	if !ok || len(rules) > 256 {
		return false
	}
	encoded, err := json.Marshal(rules)
	if err != nil || len(encoded) > 256*1024 {
		return false
	}
	seen := map[string]bool{}
	column := func(id string) bool {
		if !nativeID.MatchString(id) || seen[id] {
			return false
		}
		seen[id] = true
		return true
	}
	if op.Action == "setAttrViewSorts" {
		var sorts []*avmodel.ViewSort
		if json.Unmarshal(encoded, &sorts) != nil {
			return false
		}
		for _, sort := range sorts {
			if sort == nil || !column(sort.Column) || sort.Order != avmodel.SortOrderAsc && sort.Order != avmodel.SortOrderDesc {
				return false
			}
		}
		return true
	}
	var filters []*avmodel.ViewFilter
	if json.Unmarshal(encoded, &filters) != nil {
		return false
	}
	for _, filter := range filters {
		if filter == nil || !column(filter.Column) || !validUIAVFilterValue(filter.Value, true) {
			return false
		}
		switch filter.Operator {
		case "=", "!=", ">", ">=", "<", "<=", "Contains", "Does not contains", "Starts with", "Ends with",
			"Is between", "Is empty", "Is not empty", "Is true", "Is false":
		default:
			return false
		}
		switch filter.Qualifier {
		case "", "Any", "All", "None":
		default:
			return false
		}
		for _, date := range []*avmodel.RelativeDate{filter.RelativeDate, filter.RelativeDate2} {
			if date != nil && (date.Count < 1 || date.Count > 10000 || date.Unit < 0 || date.Unit > 3 ||
				date.Direction < -1 || date.Direction > 1) {
				return false
			}
		}
	}
	return true
}

func validUIAVFilterValue(value *avmodel.Value, allowRollup bool) bool {
	if value == nil {
		return false
	}
	for _, option := range value.MSelect {
		if option == nil {
			return false
		}
	}
	for _, asset := range value.MAsset {
		if asset == nil {
			return false
		}
	}
	if value.Rollup != nil {
		if !allowRollup || value.Type != "rollup" || len(value.Rollup.Contents) != 1 ||
			!validUIAVFilterValue(value.Rollup.Contents[0], false) {
			return false
		}
	}
	switch value.Type {
	case "block", "text", "number", "date", "select", "mSelect", "mAsset", "url",
		"email", "phone", "template", "created", "updated", "checkbox", "relation":
		return true
	case "rollup":
		return allowRollup
	}
	return false
}
