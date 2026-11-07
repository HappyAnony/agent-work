// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package avmodel

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

type RepairEnvironment struct {
	Now      func() time.Time
	NewID    func() string
	Location *time.Location
}

// CheckView retains the original repair values and changed-marker semantics.
// Source publication belongs to the caller; candidate changes alone grant no write.
func CheckView(attrView *AttributeView, view *View, env RepairEnvironment) (changed bool, err error) {
	if attrView == nil || view == nil || env.Now == nil || env.NewID == nil || env.Location == nil {
		return false, ErrUpgradeShape
	}
	for _, v := range attrView.Views {
		if v == nil {
			return false, ErrUpgradeShape
		}
	}
	for _, kv := range attrView.KeyValues {
		if kv == nil || kv.Key == nil {
			return false, ErrUpgradeShape
		}
		for _, v := range kv.Values {
			if v == nil {
				return false, ErrUpgradeShape
			}
		}
	}
	for _, f := range view.Filters {
		if f == nil {
			return false, errors.New("invalid AV filter")
		}
	}
	for _, f := range view.Sorts {
		if f == nil {
			return false, errors.New("invalid AV sort")
		}
	}
	// 字段删除以后需要删除设置的过滤和排序
	tmpFilters := []*ViewFilter{}
	for _, f := range view.Filters {
		if k, _ := attrView.GetKey(f.Column); nil != k {
			tmpFilters = append(tmpFilters, f)
		}
	}
	changed = len(tmpFilters) != len(view.Filters)
	view.Filters = tmpFilters

	tmpSorts := []*ViewSort{}
	for _, s := range view.Sorts {
		if k, _ := attrView.GetKey(s.Column); nil != k {
			tmpSorts = append(tmpSorts, s)
		}
	}
	if !changed {
		changed = len(tmpSorts) != len(view.Sorts)
	}
	view.Sorts = tmpSorts

	// 字段删除以后需要删除设置的分组
	if nil != view.Group {
		if k, _ := attrView.GetKey(view.Group.Field); nil == k {
			view.Group = nil
		}
	}

	// 订正视图类型
	validViews := make([]*View, 0, len(attrView.Views))
	for _, v := range attrView.Views {
		if LayoutTypeGallery == v.LayoutType && v.Gallery == nil {
			changed = true
			if v.Table != nil {
				v.LayoutType = LayoutTypeTable
			} else {
				continue
			}
		}
		validViews = append(validViews, v)
	}
	attrView.Views = validViews

	now := env.Now().UnixMilli()

	// 订正字段类型
	for _, kv := range attrView.KeyValues {
		for _, v := range kv.Values {
			if v.Type != kv.Key.Type {
				v.Type = kv.Key.Type
				if KeyTypeBlock == v.Type && nil == v.Block {
					v.Block = &ValueBlock{}
					if nil != v.Text {
						v.Block.Content = v.Text.Content
					}
					if "" == v.BlockID {
						v.BlockID = env.NewID()
					}
					createdStr := v.BlockID
					if len(createdStr) > 14 {
						createdStr = createdStr[:14]
					}
					created, parseErr := time.ParseInLocation("20060102150405", createdStr, env.Location)
					if nil == parseErr {
						v.Block.Created = created.UnixMilli()
					} else {
						v.Block.Created = now
					}
					v.Block.Updated = v.Block.Created
				}
				changed = true
			}
		}
	}

	attrView.Name = strings.ReplaceAll(attrView.Name, "\n", " ")
	// 截断超长的数据库标题 Limit the database title to 512 characters https://github.com/siyuan-note/siyuan/issues/15459
	if 512 < utf8.RuneCountInString(attrView.Name) {
		attrView.Name = string([]rune(attrView.Name)[:512])
		changed = true
	}

	return changed, nil
}
