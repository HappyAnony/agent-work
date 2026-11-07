// SPDX-License-Identifier: AGPL-3.0-or-later
package avrender

import (
	"context"
	"errors"
	"strings"
	"wiki-kernel/internal/avmodel"
)

// SearchText is the original index projection of names and stored cell values.
// It neither evaluates formulas nor repairs or saves the attribute view.
func SearchText(ctx context.Context, a *avmodel.AttributeView) (string, error) {
	if a == nil {
		return "", errors.New("attribute_view_missing")
	}
	var out strings.Builder
	out.WriteString(a.Name)
	out.WriteByte(' ')
	for _, v := range a.Views {
		if e := ctx.Err(); e != nil {
			return "", e
		}
		if v == nil {
			return "", errors.New("attribute_view_invalid_view")
		}
		out.WriteString(v.Name)
		out.WriteByte(' ')
	}
	for _, kv := range a.KeyValues {
		if kv == nil || kv.Key == nil {
			return "", errors.New("attribute_view_invalid_field")
		}
		out.WriteString(kv.Key.Name)
		out.WriteByte(' ')
		for _, v := range kv.Values {
			if e := ctx.Err(); e != nil {
				return "", e
			}
			if v != nil {
				out.WriteString(v.String(true))
				out.WriteByte(' ')
			}
		}
	}
	return strings.TrimSpace(out.String()), ctx.Err()
}
