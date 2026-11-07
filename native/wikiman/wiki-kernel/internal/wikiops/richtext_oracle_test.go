// SPDX-License-Identifier: AGPL-3.0-or-later
package wikiops

import (
	"context"
	"encoding/json"
	"wiki-kernel/internal/docast"
)

// Historical HTML projection used only by compatibility tests.
func applyProtyle(ctx context.Context, raw []byte, id string, operations []ProtyleOperation) ([]byte, error) {
	return applyProtyleHistory(ctx, raw, id, operations, false)
}
func applyProtyleHistory(ctx context.Context, raw []byte, id string, operations []ProtyleOperation, nativeHistory bool) ([]byte, error) {
	if len(operations) == 0 || len(operations) > 1000 {
		return nil, ErrArgument
	}
	parser := docast.Parser{}
	if _, _, err := parser.Parse(ctx, raw, id); err != nil {
		return nil, err
	}
	root, err := parseNode(raw)
	if err != nil {
		return nil, err
	}
	for _, op := range operations {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !nativeID.MatchString(op.ID) || op.ID == id {
			return nil, ErrArgument
		}
		current, parent, index := root.find(op.ID)
		switch op.Action {
		case "update", "insert":
			if op.Action == "update" && (op.NextID != "" || op.ParentID != "" || op.PreviousID != "") {
				return nil, ErrRichText
			}
			var html string
			if json.Unmarshal(op.Data, &html) != nil {
				return nil, ErrArgument
			}
			next, err := domBlock(html, op.ID)
			if err != nil {
				return nil, err
			}
			if op.Action == "update" {
				if current == nil || parent == nil {
					return nil, ErrArgument
				}
				if err = preserveNative(current, next, nativeHistory); err != nil {
					return nil, err
				}
				parent.children[index] = next
				parent.dirty = true
			} else {
				if current != nil {
					return nil, ErrArgument
				}
				if err = insertNative(root, next, op); err != nil {
					return nil, err
				}
			}
		case "delete", "move":
			if current == nil || parent == nil || len(op.Data) > 0 {
				return nil, ErrRichText
			}
			if op.Action == "delete" && (op.ParentID != "" || op.PreviousID != "" || op.NextID != "") {
				return nil, ErrRichText
			}
			if op.Action == "delete" && !nativeHistory && opaqueForHTMLUndo(current, nativeFields(), map[*astNode]bool{}) {
				return nil, ErrRichText
			}
			if op.Action == "move" {
				if sub, _, _ := current.find(op.ParentID); op.ParentID != "" && sub != nil {
					return nil, ErrArgument
				}
				if sub, _, _ := current.find(op.PreviousID); op.PreviousID != "" && sub != nil {
					return nil, ErrArgument
				}
			}
			parent.children = append(parent.children[:index], parent.children[index+1:]...)
			parent.dirty = true
			if op.Action == "move" {
				if err = insertNative(root, current, op); err != nil {
					return nil, err
				}
			}
		case "setAttrs", "foldHeading", "unfoldHeading":
			if current == nil || op.ParentID != "" || op.PreviousID != "" || op.NextID != "" {
				return nil, ErrArgument
			}
			props := map[string]json.RawMessage{}
			_ = json.Unmarshal(current.fields["Properties"], &props)
			var updates map[string]json.RawMessage
			if op.Action == "setAttrs" {
				data := op.Data
				var text string
				if json.Unmarshal(data, &text) == nil {
					data = []byte(text)
				}
				if json.Unmarshal(data, &updates) != nil || updates == nil {
					return nil, ErrArgument
				}
			} else {
				fold := "1"
				if op.Action == "unfoldHeading" {
					fold = "0"
				}
				updates = map[string]json.RawMessage{"fold": json.RawMessage(`"` + fold + `"`)}
			}
			for key, value := range updates {
				if key == "id" || key == "title" || key == "type" {
					return nil, ErrRichText
				}
				if string(value) == "null" {
					delete(props, key)
				} else {
					var text string
					if json.Unmarshal(value, &text) != nil {
						return nil, ErrArgument
					}
					props[key] = value
				}
			}
			current.fields["Properties"], _ = json.Marshal(props)
			current.dirty = true
		default:
			return nil, ErrRichText
		}
	}
	next, _, err := root.encode()
	if err != nil {
		return nil, err
	}
	if _, _, err = parser.Parse(ctx, next, id); err != nil {
		return nil, err
	}
	return next, nil
}
func insertNative(root, node *astNode, op ProtyleOperation) error {
	if op.PreviousID != "" && op.NextID != "" {
		return ErrArgument
	}
	var parent *astNode
	insert := 0
	if op.PreviousID != "" {
		_, p, index := root.find(op.PreviousID)
		if p == nil {
			return ErrArgument
		}
		parent = p
		insert = index + 1
		if op.ParentID != "" && parent.id() != op.ParentID {
			return ErrArgument
		}
	} else if op.NextID != "" {
		_, p, index := root.find(op.NextID)
		if p == nil || (op.ParentID != "" && p.id() != op.ParentID) {
			return ErrArgument
		}
		parent = p
		insert = index
	} else {
		parent, _, _ = root.find(op.ParentID)
	}
	if parent == nil {
		return ErrArgument
	}
	parent.children = append(parent.children, nil)
	copy(parent.children[insert+1:], parent.children[insert:])
	parent.children[insert] = node
	parent.dirty = true
	return nil
}
