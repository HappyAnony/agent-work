// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package avrender

import (
	av "wiki-kernel/internal/avmodel"
)

type RelatedEnvironment struct {
	Read         func(string) (*av.AttributeView, error)
	Render       func(*av.AttributeView, *av.View, string) (av.Viewable, error)
	Cache        map[string]*av.AttributeView
	Duration     av.DurationFormatter
	Further      map[string]map[string]av.Collection
	AllowMissing bool // Legacy compatibility only; scoped reads must propagate errors.
	Fallback     func(*av.AttributeView, *av.View, av.Collection) error
}

func FillRelatedValues(attrView *av.AttributeView, collection av.Collection, env RelatedEnvironment) error {
	if env.Read == nil || env.Render == nil || env.Cache == nil {
		return ErrEnvironmentUnbound
	}
	cachedAttrViews := env.Cache
	read := func(id string) (*av.AttributeView, error) {
		if a := cachedAttrViews[id]; a != nil {
			return a, nil
		}
		a, e := env.Read(id)
		if e != nil {
			if env.AllowMissing {
				return nil, nil
			}
			return nil, e
		}
		if a == nil {
			if env.AllowMissing {
				return nil, nil
			}
			return nil, ErrShape
		}
		cachedAttrViews[id] = a
		return a, nil
	}
	var readErr error

	// 渲染关联
	for _, item := range collection.GetItems() {
		for _, value := range item.GetValues() {
			if av.KeyTypeRelation != value.Type {
				continue
			}

			value.Relation.Contents = nil
			relKey, _ := attrView.GetKey(value.KeyID)
			if nil != relKey && nil != relKey.Relation {
				destAv := cachedAttrViews[relKey.Relation.AvID]
				if nil == destAv {
					destAv, readErr = read(relKey.Relation.AvID)
					if readErr != nil {
						return readErr
					}
					if nil != destAv {
						cachedAttrViews[relKey.Relation.AvID] = destAv
					}
				}
				if nil != destAv {
					blocks := map[string]*av.Value{}
					blockValues := destAv.GetBlockKeyValues()
					if nil != blockValues {
						for _, blockValue := range blockValues.Values {
							blocks[blockValue.BlockID] = blockValue
						}
						for _, blockID := range value.Relation.BlockIDs {
							if val := blocks[blockID]; nil != val {
								value.Relation.Contents = append(value.Relation.Contents, val)
							}
						}
					}
				}
			}
		}
	}

	// 渲染汇总
	rollupFurtherCollections := map[string]av.Collection{}
	for _, field := range collection.GetFields() {
		if av.KeyTypeRollup != field.GetType() {
			continue
		}

		rollupKey, _ := attrView.GetKey(field.GetID())
		if nil == rollupKey || nil == rollupKey.Rollup {
			continue
		}

		relKey, _ := attrView.GetKey(rollupKey.Rollup.RelationKeyID)
		if nil == relKey || nil == relKey.Relation {
			continue
		}

		destAv := cachedAttrViews[relKey.Relation.AvID]
		if nil == destAv {
			destAv, readErr = read(relKey.Relation.AvID)
			if readErr != nil {
				return readErr
			}
			if nil != destAv {
				cachedAttrViews[relKey.Relation.AvID] = destAv
			}
		}
		if nil == destAv {
			continue
		}

		destKey, _ := destAv.GetKey(rollupKey.Rollup.KeyID)
		if nil == destKey {
			continue
		}

		isSameAv := destAv.ID == attrView.ID
		var furtherCollection av.Collection
		if av.KeyTypeTemplate == destKey.Type || (!isSameAv && (av.KeyTypeUpdated == destKey.Type || av.KeyTypeCreated == destKey.Type || av.KeyTypeRelation == destKey.Type)) {
			if len(destAv.Views) == 0 {
				return ErrShape
			}
			viewable, renderErr := env.Render(destAv, destAv.Views[0], "")
			if renderErr != nil {
				return renderErr
			}
			if nil != viewable {
				furtherCollection = viewable.(av.Collection)
			} else {
				if env.Fallback == nil {
					return ErrShape
				}
				if err := env.Fallback(destAv, destAv.Views[0], collection); err != nil {
					return err
				}
				furtherCollection = collection
			}
		}
		rollupFurtherCollections[rollupKey.ID] = furtherCollection
	}

	for _, item := range collection.GetItems() {
		for _, value := range item.GetValues() {
			if av.KeyTypeRollup != value.Type {
				continue
			}

			rollupKey, _ := attrView.GetKey(value.KeyID)
			if nil == rollupKey || nil == rollupKey.Rollup {
				break
			}

			relKey, _ := attrView.GetKey(rollupKey.Rollup.RelationKeyID)
			if nil == relKey || nil == relKey.Relation {
				break
			}

			relVal := attrView.GetValue(relKey.ID, item.GetID())
			if nil == relVal || nil == relVal.Relation {
				break
			}

			destAv := cachedAttrViews[relKey.Relation.AvID]
			if nil == destAv {
				destAv, readErr = read(relKey.Relation.AvID)
				if readErr != nil {
					return readErr
				}
				if nil != destAv {
					cachedAttrViews[relKey.Relation.AvID] = destAv
				}
			}
			if nil == destAv {
				break
			}

			destKey, _ := destAv.GetKey(rollupKey.Rollup.KeyID)
			if nil == destKey {
				break
			}

			furtherCollection := rollupFurtherCollections[rollupKey.ID]
			if env.Duration == nil && rollupKey.Rollup.Calc != nil && rollupKey.Rollup.Calc.Operator == av.CalcOperatorRange && (destKey.Type == av.KeyTypeDate || destKey.Type == av.KeyTypeCreated || destKey.Type == av.KeyTypeUpdated) {
				return av.ErrDurationFormatterUnbound
			}
			value.Rollup.BuildContents(destAv.KeyValues, destKey, relVal, rollupKey.Rollup.Calc, furtherCollection, env.Duration)
		}
	}
	if env.Further != nil {
		env.Further[attrView.ID] = rollupFurtherCollections
	}
	return nil
}
