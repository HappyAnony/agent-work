// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package avquery

import (
	"context"
	"errors"
)

func EmptyIdentityMap(ids []string) map[string]string {
	ret := map[string]string{}
	for _, id := range ids {
		ret[id] = ""
	}
	return ret
}

// Original two-way lookup: no detached/existence filtering, and last match wins.
func IdentityMap[T any](ctx context.Context, values []T, ids []string, toItems bool, describe func(T) (itemID, boundID string, err error)) (map[string]string, error) {
	if ctx == nil || describe == nil {
		return nil, errors.New("av_identity_dependencies_unbound")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	ret := EmptyIdentityMap(ids)
	for _, value := range values {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		item, bound, err := describe(value)
		if err != nil {
			return nil, err
		}
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		key, result := item, bound
		if toItems {
			key, result = bound, item
		}
		if _, ok := ret[key]; ok {
			ret[key] = result
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return ret, nil
}
