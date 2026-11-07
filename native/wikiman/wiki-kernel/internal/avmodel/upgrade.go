// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package avmodel

import (
	"errors"
	"fmt"
	"time"
)

const CurrentSpec = 4

type UpgradeEnvironment struct {
	Now     func() time.Time
	NewID   func() string
	Warning func(error)
}

func (e UpgradeEnvironment) warn(err error) {
	if e.Warning != nil {
		e.Warning(err)
	}
}
func timePrefix(s string) string {
	if len(s) > 14 {
		return s[:14]
	}
	return s
}

var ErrSourceVersion = errors.New("unsupported AV source version")
var ErrUpgradeShape = errors.New("invalid AV compatibility source")

// UpgradeInMemory is the original Spec 0..4 projection, with explicit effects.
// It never loads/saves source. Its caller owns the candidate and generated IDs.
func UpgradeInMemory(a *AttributeView, e UpgradeEnvironment) error {
	if a == nil || a.Spec < 0 || a.Spec > CurrentSpec {
		return ErrSourceVersion
	}
	if a.Spec == CurrentSpec {
		return nil
	}
	if e.Now == nil || e.NewID == nil {
		return errors.New("AV compatibility environment unbound")
	}
	for _, kv := range a.KeyValues {
		if kv == nil || kv.Key == nil {
			return ErrUpgradeShape
		}
		for _, v := range kv.Values {
			if v == nil || (kv.Key.Type == KeyTypeBlock && v.Block == nil) {
				return ErrUpgradeShape
			}
			if kv.Key.Type == KeyTypeRelation && v.Relation == nil {
				v.Relation = &ValueRelation{}
			}
			if kv.Key.Type == KeyTypeRollup && v.Rollup == nil {
				v.Rollup = &ValueRollup{}
			}
		}
	}
	for _, v := range a.Views {
		if v == nil {
			return ErrUpgradeShape
		}
		if v.Table != nil {
			if v.Table.BaseLayout == nil {
				return ErrUpgradeShape
			}
			for _, c := range v.Table.Columns {
				if c == nil || c.BaseField == nil {
					return ErrUpgradeShape
				}
			}
			for _, f := range v.Table.Filters {
				if f == nil {
					return ErrUpgradeShape
				}
			}
			for _, s := range v.Table.Sorts {
				if s == nil {
					return ErrUpgradeShape
				}
			}
		}
		for _, f := range v.Filters {
			if f == nil {
				return ErrUpgradeShape
			}
		}
		for _, s := range v.Sorts {
			if s == nil {
				return ErrUpgradeShape
			}
		}
	}
	upgradeSpec1(a, e)
	upgradeSpec2(a, e)
	upgradeSpec3(a, e)
	upgradeSpec4(a, e)
	return nil
}
func upgradeSpec1(av *AttributeView, env UpgradeEnvironment) {
	if 1 <= av.Spec {
		return
	}

	now := env.Now().UnixMilli()
	for _, kv := range av.KeyValues {
		switch kv.Key.Type {
		case KeyTypeBlock:
			// 补全 block 的创建时间和更新时间
			for _, v := range kv.Values {
				if 0 == v.Block.Created {
					env.warn(fmt.Errorf("block [%s] created time is empty", v.BlockID))
					if "" == v.BlockID {
						v.BlockID = env.NewID()
					}

					createdStr := timePrefix(v.BlockID)
					created, parseErr := time.ParseInLocation("20060102150405", createdStr, time.Local)
					if nil == parseErr {
						v.Block.Created = created.UnixMilli()
					} else {
						v.Block.Created = now
					}
				}
				if 0 == v.Block.Updated {
					env.warn(fmt.Errorf("block [%s] updated time is empty", v.BlockID))
					v.Block.Updated = v.Block.Created
				}
			}
		case KeyTypeNumber:
			for _, v := range kv.Values {
				if nil != v.Number && 0 != v.Number.Content && !v.Number.IsNotEmpty {
					v.Number.IsNotEmpty = true
				}
			}
		}

		for _, v := range kv.Values {
			if "" == kv.Key.ID {
				kv.Key.ID = env.NewID()
				for _, val := range kv.Values {
					val.KeyID = kv.Key.ID
				}
				if "" == v.KeyID {
					env.warn(fmt.Errorf("value [%s] key id is empty", v.ID))
					v.KeyID = kv.Key.ID
				}

				// 校验日期 IsNotEmpty
				if KeyTypeDate == kv.Key.Type {
					if nil != v.Date && 0 != v.Date.Content && !v.Date.IsNotEmpty {
						v.Date.IsNotEmpty = true
					}
				}

				// 校验数字 IsNotEmpty
				if KeyTypeNumber == kv.Key.Type {
					if nil != v.Number && 0 != v.Number.Content && !v.Number.IsNotEmpty {
						v.Number.IsNotEmpty = true
					}
				}

				// 清空关联实际值
				if KeyTypeRelation == kv.Key.Type {
					v.Relation.Contents = nil
				}

				// 清空汇总实际值
				if KeyTypeRollup == kv.Key.Type {
					v.Rollup.Contents = nil
				}

				for _, view := range av.Views {
					switch view.LayoutType {
					case LayoutTypeTable:
						for _, column := range view.Table.Columns {
							if "" == column.ID {
								column.ID = kv.Key.ID
								break
							}
						}
					}
				}
			}

			// 补全值的创建时间和更新时间
			if "" == v.ID {
				env.warn(fmt.Errorf("value id is empty"))
				v.ID = env.NewID()
			}

			if 0 == v.CreatedAt {
				env.warn(fmt.Errorf("value [%s] created time is empty", v.ID))
				createdStr := timePrefix(v.ID)
				created, parseErr := time.ParseInLocation("20060102150405", createdStr, time.Local)
				if nil == parseErr {
					v.CreatedAt = created.UnixMilli()
				} else {
					v.CreatedAt = now
				}
			}

			if 0 == v.UpdatedAt {
				env.warn(fmt.Errorf("value [%s] updated time is empty", v.ID))
				v.UpdatedAt = v.CreatedAt
			}
		}
	}

	// 补全过滤规则 Value
	for _, view := range av.Views {
		if nil != view.Table {
			for _, f := range view.Table.Filters {
				if nil != f.Value {
					continue
				}

				if k, _ := av.GetKey(f.Column); nil != k {
					f.Value = &Value{Type: k.Type}
				}
			}
		}
	}

	av.Spec = 1
}

func upgradeSpec2(av *AttributeView, env UpgradeEnvironment) {
	if 2 <= av.Spec {
		return
	}

	// 如果存在 view.table.filters/sorts/pageSize 则复制覆盖到 view.filters/sorts/pageSize
	for _, view := range av.Views {
		if 1 > len(view.Filters) {
			view.Filters = []*ViewFilter{}
		}
		if 1 > len(view.Sorts) {
			view.Sorts = []*ViewSort{}
		}
		if 1 > view.PageSize {
			view.PageSize = ViewDefaultPageSize
		}

		if nil != view.Table {
			if 0 < len(view.Table.Filters) && 1 > len(view.Filters) {
				view.Filters = append(view.Filters, view.Table.Filters...)
			}
			if 0 < len(view.Table.Sorts) && 1 > len(view.Sorts) {
				view.Sorts = append(view.Sorts, view.Table.Sorts...)
			}
			if 0 < view.Table.PageSize {
				view.PageSize = view.Table.PageSize
			}
			view.Table.ShowIcon = true
		}

		// 清理过滤和排序规则中不存在的键
		tmpFilters := []*ViewFilter{}
		for _, f := range view.Filters {
			if k, _ := av.GetKey(f.Column); nil != k {
				tmpFilters = append(tmpFilters, f)
			}
		}
		view.Filters = tmpFilters

		tmpSorts := []*ViewSort{}
		for _, s := range view.Sorts {
			if k, _ := av.GetKey(s.Column); nil != k {
				tmpSorts = append(tmpSorts, s)
			}
		}
		view.Sorts = tmpSorts
	}

	av.Spec = 2
}

func upgradeSpec3(av *AttributeView, env UpgradeEnvironment) {
	if 3 <= av.Spec {
		return
	}

	// 将 view.table.rowIds 或 view.gallery.cardIds 复制到 view.itemIds
	for _, view := range av.Views {
		if 0 < len(view.ItemIDs) {
			continue
		}

		switch view.LayoutType {
		case LayoutTypeTable:
			if nil != view.Table {
				view.ItemIDs = view.Table.RowIDs
			}
		case LayoutTypeGallery:
			if nil != view.Gallery {
				view.ItemIDs = view.Gallery.CardIDs
			}
		}
	}

	av.Spec = 3
}

func upgradeSpec4(av *AttributeView, env UpgradeEnvironment) {
	if 4 <= av.Spec {
		return
	}

	for _, keyValues := range av.KeyValues {
		switch keyValues.Key.Type {
		case KeyTypeCreated:
			if nil == keyValues.Key.Created {
				keyValues.Key.Created = &Created{IncludeTime: true}
			}
		case KeyTypeUpdated:
			if nil == keyValues.Key.Updated {
				keyValues.Key.Updated = &Updated{IncludeTime: true}
			}
		}
	}

	av.Spec = 4
}
