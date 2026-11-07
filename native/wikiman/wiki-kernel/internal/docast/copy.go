// SPDX-License-Identifier: AGPL-3.0-or-later
package docast

import (
	"context"
	"encoding/json"
)

// SerializeDocumentCopy transfers opaque fields by the original duplicate
// algorithm's stable traversal order. The generated IDs and reference rewrites
// belong to the candidate. Unknown values are carried without decoding numbers.
func SerializeDocumentCopy(ctx context.Context, before []byte, beforeID string, next []byte, nextID string) ([]byte, error) {
	parser := Parser{}
	from, _, err := parser.Parse(ctx, before, beforeID)
	if err != nil {
		return nil, err
	}
	to, _, err := parser.Parse(ctx, next, nextID)
	if err != nil {
		return nil, err
	}
	if len(from) != len(to) {
		return nil, ErrOpaqueEdit
	}
	ids := map[string]string{}
	for i, span := range from {
		var left, right struct{ Type string }
		if json.Unmarshal(before[span.Start:span.End], &left) != nil || json.Unmarshal(next[to[i].Start:to[i].End], &right) != nil || left.Type != right.Type {
			return nil, ErrOpaqueEdit
		}
		ids[span.ID] = to[i].ID
	}
	var rekey func(json.RawMessage) (json.RawMessage, error)
	rekey = func(raw json.RawMessage) (json.RawMessage, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var node rawObject
		if err := json.Unmarshal(raw, &node); err != nil {
			return nil, err
		}
		if id := ids[jsonString(node, "ID")]; id != "" {
			node["ID"], _ = json.Marshal(id)
		}
		if children, ok := node["Children"]; ok {
			var list []json.RawMessage
			if err := json.Unmarshal(children, &list); err != nil {
				return nil, err
			}
			for i, v := range list {
				next, err := rekey(v)
				if err != nil {
					return nil, err
				}
				list[i] = next
			}
			node["Children"], _ = json.Marshal(list)
		}
		return json.Marshal(node)
	}
	previous, err := rekey(before)
	if err != nil {
		return nil, err
	}
	return SerializeDocument([][]byte{previous}, next)
}
