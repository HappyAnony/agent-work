// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package avrender

import (
	"context"
	"errors"
	av "wiki-kernel/internal/avmodel"
)

type FurtherEnvironment struct {
	Read         func(string) (*av.AttributeView, error)
	Render       func(*av.AttributeView, *av.View, string) (av.Viewable, error)
	AllowMissing bool
	Missing      func(error) bool
}

// Original SQL rollup filter support, now usable with workspace-owned sources.
func FurtherCollections(ctx context.Context, a *av.AttributeView, cache map[string]*av.AttributeView, e FurtherEnvironment) (map[string]av.Collection, error) {
	if ctx == nil || a == nil || cache == nil || e.Read == nil || e.Render == nil {
		return nil, ErrEnvironmentUnbound
	}
	ret := map[string]av.Collection{}
	if err := validateFurtherKeys(a); err != nil {
		return nil, err
	}
	for _, kv := range a.KeyValues {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if kv == nil || kv.Key == nil {
			return nil, ErrShape
		}
		if kv.Key.Type != av.KeyTypeRollup {
			continue
		}
		if kv.Key.Rollup == nil {
			return nil, ErrShape
		}
		rel, _ := a.GetKey(kv.Key.Rollup.RelationKeyID)
		if rel == nil {
			continue
		}
		if rel.Relation == nil {
			return nil, ErrShape
		}
		dest := cache[rel.Relation.AvID]
		if dest == nil {
			var err error
			dest, err = e.Read(rel.Relation.AvID)
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			if err != nil || dest == nil {
				if e.AllowMissing || (e.Missing != nil && e.Missing(err)) {
					continue
				}
				if err != nil {
					return nil, err
				}
				return nil, errors.New("av_further_source_missing")
			}
			if dest.ID != rel.Relation.AvID {
				return nil, ErrShape
			}
			cache[dest.ID] = dest
		}
		if dest.ID != rel.Relation.AvID {
			return nil, ErrShape
		}
		if err := validateFurtherKeys(dest); err != nil {
			return nil, err
		}
		key, _ := dest.GetKey(kv.Key.Rollup.KeyID)
		if key == nil {
			continue
		}
		var collection av.Collection
		if key.Type == av.KeyTypeTemplate || (dest.ID != a.ID && (key.Type == av.KeyTypeUpdated || key.Type == av.KeyTypeCreated || key.Type == av.KeyTypeRelation)) {
			if len(dest.Views) == 0 || dest.Views[0] == nil {
				return nil, ErrShape
			}
			viewable, err := e.Render(dest, dest.Views[0], "")
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			if err != nil {
				return nil, err
			}
			if viewable != nil {
				var ok bool
				collection, ok = viewable.(av.Collection)
				if !ok {
					return nil, ErrShape
				}
			}
		}
		ret[kv.Key.ID] = collection
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return ret, nil
}

func validateFurtherKeys(a *av.AttributeView) error {
	for _, kv := range a.KeyValues {
		if kv == nil || kv.Key == nil {
			return ErrShape
		}
	}
	return nil
}
