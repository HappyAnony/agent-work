// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package avmodel

import (
	"context"
	"errors"
	"wiki-kernel/internal/avquery"
)

// No view rendering/upgrade/SQL lookup is needed for identity conversion.
func (a *AttributeView) IdentityMap(ctx context.Context, ids []string, toItems bool) (map[string]string, error) {
	if a == nil {
		return nil, errors.New("invalid_av_identity_source")
	}
	for _, kv := range a.KeyValues {
		if kv == nil || kv.Key == nil {
			return nil, errors.New("invalid_av_identity_key")
		}
	}
	primary := a.GetBlockKeyValues()
	if primary == nil {
		return nil, errors.New("av_identity_primary_missing")
	}
	return avquery.IdentityMap(ctx, primary.Values, ids, toItems, func(v *Value) (string, string, error) {
		if v == nil || v.Block == nil {
			return "", "", errors.New("invalid_av_identity_value")
		}
		return v.BlockID, v.Block.ID, nil
	})
}
